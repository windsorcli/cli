package test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/goccy/go-yaml"
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/composer/artifact"
	"github.com/windsorcli/cli/pkg/composer/blueprint"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/evaluator"
	"github.com/windsorcli/cli/pkg/runtime/shell"
	"github.com/windsorcli/cli/pkg/runtime/terraform"
)

// The TestRunner is a static test execution engine for Windsor blueprint composition.
// It provides discovery and execution of test cases defined in YAML files,
// validating that blueprint composition produces expected outputs given specific inputs.
// It enables regression testing of facet logic and blueprint composition without live infrastructure.

// SkipDefaultBlueprintURL is a sentinel for DefaultBlueprintURL that disables loading the default OCI blueprint (used in unit tests).
const SkipDefaultBlueprintURL = " "

// =============================================================================
// Types
// =============================================================================

// TestResult represents the result of running a single test case.
type TestResult struct {
	Name       string
	Passed     bool
	Diffs      []string
	SourceFile string
}

// TestRunner discovers and executes blueprint composition tests.
type TestRunner struct {
	projectRoot         string
	baseShell           shell.Shell
	baseProjectRoot     string
	artifactBuilder     artifact.Artifact
	DefaultBlueprintURL string
	RunFunc             func(filter string) ([]TestResult, error)
}

type testCaseWithFile struct {
	testCase blueprintv1alpha1.TestCase
	fileName string
}

// =============================================================================
// Constructor
// =============================================================================

// NewTestRunner creates a new TestRunner using the provided runtime and artifact builder.
// It stores base dependencies for creating isolated runtime instances for each test case, ensuring test isolation.
func NewTestRunner(rt *runtime.Runtime, artifactBuilder artifact.Artifact) *TestRunner {
	return &TestRunner{
		projectRoot:     rt.ProjectRoot,
		baseShell:       rt.Shell,
		baseProjectRoot: rt.ProjectRoot,
		artifactBuilder: artifactBuilder,
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// RunAndPrint discovers and executes test cases, printing results to stdout.
// If filter is provided, only tests matching the filter name are executed.
// Returns an error if tests fail or execution encounters an error.
func (r *TestRunner) RunAndPrint(filter string) error {
	results, err := r.Run(filter)
	if err != nil {
		return err
	}
	return r.printResults(results)
}

// Run discovers and executes test cases, returning results for each test.
// If filter is provided, only tests matching the filter name are executed.
func (r *TestRunner) Run(filter string) ([]TestResult, error) {
	restore := r.withTestContextEnv()
	defer restore()

	if r.RunFunc != nil {
		return r.RunFunc(filter)
	}

	testCasesWithFiles, err := r.discoverTestCases(filter)
	if err != nil {
		return nil, err
	}
	return r.runTestCases(testCasesWithFiles)
}

// =============================================================================
// Private Methods
// =============================================================================

// runTestCases executes the given test cases and returns their results with SourceFile set.
// Used by Run and by tests that need to run a specific list of cases then print.
func (r *TestRunner) runTestCases(testCasesWithFiles []testCaseWithFile) ([]TestResult, error) {
	results := make([]TestResult, 0, len(testCasesWithFiles))
	for _, tcf := range testCasesWithFiles {
		result, err := r.runTestCase(tcf.testCase)
		if err != nil {
			return nil, fmt.Errorf("failed to run test case %s: %w", tcf.testCase.Name, err)
		}
		result.SourceFile = tcf.fileName
		results = append(results, result)
	}
	return results, nil
}

// discoverTestCases finds all .test.yaml files under contexts/_template/tests, parses them,
// and returns test cases that match the filter (or all if filter is empty). Returns an error
// for discovery failure, missing test directory, parse failure, or when filter is set and no case matches.
func (r *TestRunner) discoverTestCases(filter string) ([]testCaseWithFile, error) {
	testsDir := filepath.Join(r.projectRoot, "contexts", "_template", "tests")
	testFiles, err := r.discoverTestFiles(testsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to discover test files: %w", err)
	}
	if len(testFiles) == 0 {
		return nil, fmt.Errorf("no test files found in %s", testsDir)
	}
	var out []testCaseWithFile
	for _, testFilePath := range testFiles {
		testFile, err := r.parseTestFile(testFilePath)
		if err != nil {
			return nil, fmt.Errorf("failed to parse test file %s: %w", testFilePath, err)
		}
		fileName := filepath.Base(testFilePath)
		for _, tc := range testFile.Cases {
			if filter != "" && tc.Name != filter {
				continue
			}
			out = append(out, testCaseWithFile{testCase: tc, fileName: fileName})
		}
	}
	if len(out) == 0 && filter != "" {
		return nil, fmt.Errorf("no test cases found matching filter: %s", filter)
	}
	return out, nil
}

// withTestContextEnv sets WINDSOR_CONTEXT to "test" and captures the project's .windsor/context
// file when the path is under project root. Returns a restore function that reverts the env var
// and writes back (or removes) the context file; call it in defer so test runs do not mutate context.
func (r *TestRunner) withTestContextEnv() (restore func()) {
	originalContext := os.Getenv("WINDSOR_CONTEXT")
	_ = os.Setenv("WINDSOR_CONTEXT", "test")
	cleanedRoot := filepath.Clean(r.projectRoot)
	contextFile := filepath.Join(cleanedRoot, ".windsor", "context")
	rel, relErr := filepath.Rel(cleanedRoot, filepath.Clean(contextFile))
	pathSafe := relErr == nil && !strings.HasPrefix(rel, "..")
	var originalContextFile []byte
	if pathSafe {
		originalContextFile, _ = os.ReadFile(contextFile) // #nosec G304 - path constrained to project root via filepath.Rel
	}
	return func() {
		if originalContext != "" {
			_ = os.Setenv("WINDSOR_CONTEXT", originalContext)
		} else {
			_ = os.Unsetenv("WINDSOR_CONTEXT")
		}
		if pathSafe {
			if originalContextFile != nil {
				_ = os.WriteFile(contextFile, originalContextFile, 0600)
			} else {
				_ = os.Remove(contextFile)
			}
		}
	}
}

// createGenerator creates a function that generates blueprints from test values.
// It sets up an isolated runtime environment for each test case, ensuring complete test isolation by creating
// a fresh ConfigHandler instance and configuring the runtime to use only the _template directory. The generator
// applies test values directly without loading any context files, ensuring tests only use explicitly provided inputs.
// If terraformOutputs are provided, it registers a mock TerraformProvider to supply mock outputs for terraform_output()
// expressions. This allows tests to validate blueprint composition that depends on Terraform outputs without requiring
// actual Terraform state. Returns a function that takes test values and returns a composed blueprint or an error.
func (r *TestRunner) createGenerator(terraformOutputs map[string]map[string]any) func(values map[string]any) (*blueprintv1alpha1.Blueprint, error) {
	return func(values map[string]any) (*blueprintv1alpha1.Blueprint, error) {
		freshConfigHandler := config.NewConfigHandler(r.baseShell)
		if err := freshConfigHandler.SetContext("test"); err != nil {
			return nil, fmt.Errorf("failed to set test context: %w", err)
		}

		rt := runtime.NewRuntime(&runtime.Runtime{
			Shell:         r.baseShell,
			ConfigHandler: freshConfigHandler,
			ProjectRoot:   r.baseProjectRoot,
			ContextName:   "test",
			ConfigRoot:    filepath.Join(r.baseProjectRoot, "contexts", "_template"),
			TemplateRoot:  filepath.Join(r.baseProjectRoot, "contexts", "_template"),
		})

		if rt.ConfigHandler != nil {
			schemaPath := filepath.Join(r.baseProjectRoot, "contexts", "_template", "schema.yaml")
			if err := rt.ConfigHandler.LoadSchema(schemaPath); err != nil {
				if !errors.Is(err, os.ErrNotExist) {
					return nil, fmt.Errorf("failed to load schema: %w", err)
				}
			}
		}

		for key, value := range values {
			if key == "_testName" || key == "context" {
				continue
			}
			normalized := normalizeTestValue(value)
			if err := rt.ConfigHandler.Set(key, normalized); err != nil {
				return nil, fmt.Errorf("failed to set value %s: %w", key, err)
			}
		}

		if err := rt.InitializeComponents(); err != nil {
			return nil, fmt.Errorf("failed to initialize components: %w", err)
		}

		if len(terraformOutputs) > 0 {
			mockProvider := &terraform.MockTerraformProvider{
				GetTerraformOutputsFunc: func(componentID string) (map[string]any, error) {
					if outputs, exists := terraformOutputs[componentID]; exists {
						return outputs, nil
					}
					return make(map[string]any), nil
				},
			}
			if rt.ConfigHandler.GetBool("terraform.enabled", false) {
				rt.TerraformProvider = mockProvider
			}
			registerTerraformOutputHelperForMock(mockProvider, rt.Evaluator)
		}

		testBlueprintHandler := blueprint.NewBlueprintHandler(rt, r.artifactBuilder)

		defaultURL := r.DefaultBlueprintURL
		if defaultURL == "" {
			defaultURL = constants.GetEffectiveBlueprintURL()
		}
		var initURLs []string
		if defaultURL != "" && defaultURL != SkipDefaultBlueprintURL {
			templateRoot := filepath.Join(r.baseProjectRoot, "contexts", "_template")
			if _, err := os.Stat(templateRoot); err == nil {
				initURLs = nil
			} else {
				useDefaultOCI := values["platform"] != nil || values["provider"] != nil
				if useDefaultOCI {
					initURLs = []string{defaultURL}
				} else {
					initURLs = nil
				}
			}
		}
		if err := testBlueprintHandler.LoadBlueprint(initURLs...); err != nil {
			return nil, fmt.Errorf("failed to load blueprint: %w", err)
		}

		bp := testBlueprintHandler.Generate()
		if bp == nil {
			return nil, fmt.Errorf("failed to generate blueprint")
		}

		return bp, nil
	}
}

// normalizeTestValue converts YAML-unmarshaled values (e.g. map[interface{}]interface{}) to
// map[string]any and []any so ConfigHandler and expression evaluation resolve nested keys correctly.
func normalizeTestValue(v any) any {
	if v == nil {
		return v
	}
	if m := blueprintv1alpha1.ToMapStringAny(v); m != nil {
		return m
	}
	if s := blueprintv1alpha1.ToSliceAny(v); s != nil {
		return s
	}
	return v
}

// printResults formats and prints test results to stdout in a human-readable format.
// It displays passing tests with a checkmark (✓) and failing tests with an X (✗), along with
// any differences or error messages for failed tests. After printing all results, it displays
// a summary showing the total number of passed and failed tests. Returns an error if any tests
// failed, allowing callers to propagate test failures up the call stack.
func (r *TestRunner) printResults(results []TestResult) error {
	if len(results) == 0 {
		return nil
	}

	passed := 0
	failed := 0
	currentFile := ""
	for _, result := range results {
		if result.SourceFile != "" && result.SourceFile != currentFile {
			if currentFile != "" {
				fmt.Fprintf(os.Stdout, "\n")
			}
			fmt.Fprintf(os.Stdout, "=== %s ===\n", result.SourceFile)
			currentFile = result.SourceFile
		}
		if result.Passed {
			passed++
			fmt.Fprintf(os.Stdout, "✓ %s\n", result.Name)
		} else {
			failed++
			fmt.Fprintf(os.Stdout, "✗ %s\n", result.Name)
			for _, diff := range result.Diffs {
				fmt.Fprintf(os.Stdout, "  %s\n", diff)
			}
		}
	}

	fmt.Fprintf(os.Stdout, "\n")
	if failed > 0 {
		fmt.Fprintf(os.Stdout, "FAIL: %d of %d test(s) failed\n", failed, len(results))
		return fmt.Errorf("%d test(s) failed", failed)
	}

	fmt.Fprintf(os.Stdout, "PASS: %d test(s) passed\n", passed)
	return nil
}

// discoverTestFiles recursively searches the tests directory for all files matching the .test.yaml pattern.
// It walks the directory tree starting from testsDir and collects all file paths that end with .test.yaml.
// If the tests directory does not exist, it returns an empty slice and no error, allowing callers to handle
// the missing directory case gracefully. Returns a slice of absolute file paths to test files, or an error
// if the directory walk encounters a filesystem error.
func (r *TestRunner) discoverTestFiles(testsDir string) ([]string, error) {
	if _, err := os.Stat(testsDir); os.IsNotExist(err) {
		return nil, nil
	}

	var testFiles []string

	err := filepath.Walk(testsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, ".test.yaml") {
			testFiles = append(testFiles, path)
		}
		return nil
	})

	return testFiles, err
}

// parseTestFile reads a YAML test file from disk and unmarshals it into a TestFile structure.
// It handles file I/O errors and YAML parsing errors, returning descriptive errors that include
// the file path for easier debugging. The parsed TestFile contains all test cases defined in
// the file, which can then be executed by the test runner.
func (r *TestRunner) parseTestFile(path string) (*blueprintv1alpha1.TestFile, error) {
	// #nosec G304 - Test file paths are derived from walking the project directory, intentional CLI behavior
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var testFile blueprintv1alpha1.TestFile
	if err := yaml.Unmarshal(data, &testFile); err != nil {
		return nil, err
	}

	return &testFile, nil
}

// runTestCase executes a single test case by generating a blueprint from test values and validating it.
// It creates an isolated blueprint generator, applies test values, and composes a blueprint. If the test
// case expects an error (ExpectError=true), it verifies that composition fails. Otherwise, it validates
// the composed blueprint structure, checks for expected components and properties, verifies excluded
// components are absent, and performs automatic validation checks (duplicates, circular dependencies, etc.).
// Returns a TestResult indicating whether the test passed and any differences or errors found.
func (r *TestRunner) runTestCase(tc blueprintv1alpha1.TestCase) (TestResult, error) {
	result := TestResult{
		Name:   tc.Name,
		Passed: true,
	}

	testValues := make(map[string]any)
	for k, v := range tc.Values {
		testValues[k] = v
	}
	testValues["_testName"] = tc.Name

	generator := r.createGenerator(tc.TerraformOutputs)
	bp, err := generator(testValues)

	if tc.ExpectError {
		if err == nil {
			result.Passed = false
			result.Diffs = append(result.Diffs, "expected composition to fail, but it succeeded")
			return result, nil
		}
		return result, nil
	}

	if err != nil {
		result.Passed = false
		result.Diffs = append(result.Diffs, fmt.Sprintf("composition error: %v", err))
		return result, nil
	}

	validationErrors := r.validateBlueprint(bp)
	if len(validationErrors) > 0 {
		result.Passed = false
		result.Diffs = append(result.Diffs, validationErrors...)
	}

	if tc.Expect != nil {
		diffs := r.matchBlueprint(bp, tc.Expect)
		if len(diffs) > 0 {
			result.Passed = false
			result.Diffs = append(result.Diffs, diffs...)
		}
	}

	if tc.Exclude != nil {
		diffs := r.matchExclusions(bp, tc.Exclude)
		if len(diffs) > 0 {
			result.Passed = false
			result.Diffs = append(result.Diffs, diffs...)
		}
	}

	return result, nil
}

// matchBlueprint compares the actual composed blueprint against expected blueprint structure and returns
// a list of differences. It uses partial matching semantics: only fields explicitly specified in the expect
// blueprint are validated, allowing tests to focus on specific aspects without asserting the entire structure.
// The function validates both Terraform components and Kustomizations, checking for presence and matching
// properties. For each expected component, it searches the actual blueprint, reports missing components,
// and compares specified properties (source, path, dependsOn, etc.). Returns an empty slice if all
// expectations are met, or a list of descriptive difference messages if mismatches are found.
func (r *TestRunner) matchBlueprint(bp *blueprintv1alpha1.Blueprint, expect *blueprintv1alpha1.Blueprint) []string {
	var diffs []string

	if expect == nil {
		return diffs
	}

	for _, expectTf := range expect.TerraformComponents {
		found := r.findTerraformComponent(bp, expectTf)
		if found == nil {
			identifier := expectTf.Name
			if identifier == "" {
				identifier = expectTf.Path
			}
			diffs = append(diffs, fmt.Sprintf("terraform component not found: %s", identifier))
			continue
		}

		componentDiffs := r.matchTerraformComponent(found, expectTf)
		diffs = append(diffs, componentDiffs...)
	}

	for _, expectK := range expect.Kustomizations {
		found := r.findKustomization(bp, expectK)
		if found == nil {
			diffs = append(diffs, fmt.Sprintf("kustomization not found: %s", expectK.Name))
			continue
		}

		kustomizeDiffs := r.matchKustomization(found, expectK)
		diffs = append(diffs, kustomizeDiffs...)
	}

	return diffs
}

// matchExclusions verifies that components specified in the exclude blueprint are NOT present in the
// actual composed blueprint. This allows tests to assert that certain components should be excluded
// based on test conditions (e.g., a component should not exist when a feature flag is disabled).
// Uses partial matching: components are identified by name or path for Terraform components, and by
// name for Kustomizations. If any excluded component is found in the blueprint, a descriptive error
// message is added to the differences list. Returns an empty slice if all exclusions are satisfied.
func (r *TestRunner) matchExclusions(bp *blueprintv1alpha1.Blueprint, exclude *blueprintv1alpha1.Blueprint) []string {
	var diffs []string

	if exclude == nil {
		return diffs
	}

	for _, excludeTf := range exclude.TerraformComponents {
		found := r.findTerraformComponent(bp, excludeTf)
		if found != nil {
			identifier := excludeTf.Name
			if identifier == "" {
				identifier = excludeTf.Path
			}
			diffs = append(diffs, fmt.Sprintf("terraform component should not exist: %s", identifier))
		}
	}

	for _, excludeK := range exclude.Kustomizations {
		found := r.findKustomization(bp, excludeK)
		if found != nil {
			diffs = append(diffs, fmt.Sprintf("kustomization should not exist: %s", excludeK.Name))
		}
	}

	return diffs
}

// findTerraformComponent searches the blueprint's Terraform components for a component matching the
// expected component's name or path. It performs a linear search through all Terraform components,
// checking first for a name match (if expect.Name is non-empty), then for a path match (if expect.Path
// is non-empty). This allows tests to identify components by either identifier. Returns a pointer to
// the matching component if found, or nil if no match exists.
func (r *TestRunner) findTerraformComponent(bp *blueprintv1alpha1.Blueprint, expect blueprintv1alpha1.TerraformComponent) *blueprintv1alpha1.TerraformComponent {
	for i := range bp.TerraformComponents {
		tc := &bp.TerraformComponents[i]
		if expect.Name != "" && tc.Name == expect.Name {
			return tc
		}
		if expect.Path != "" && tc.Path == expect.Path {
			return tc
		}
	}
	return nil
}

// findKustomization searches the blueprint's Kustomizations for a kustomization matching the expected
// kustomization's name. It performs a linear search through all Kustomizations, comparing names exactly.
// Returns a pointer to the matching kustomization if found, or nil if no kustomization with the
// specified name exists in the blueprint.
func (r *TestRunner) findKustomization(bp *blueprintv1alpha1.Blueprint, expect blueprintv1alpha1.Kustomization) *blueprintv1alpha1.Kustomization {
	for i := range bp.Kustomizations {
		k := &bp.Kustomizations[i]
		if k.Name == expect.Name {
			return k
		}
	}
	return nil
}

// matchTerraformComponent compares an actual Terraform component against expected properties and returns
// a list of differences. It uses partial matching: only properties explicitly set in the expect component
// are validated. The function checks source, path, dependsOn, and inputs fields. For dependsOn, it verifies that
// all expected dependencies are present in the actual component's dependency list. For inputs, it performs
// strict value equality checking - each expected key must exist with the exact expected value. Returns an empty
// slice if all specified properties match, or a list of formatted difference messages describing mismatches.
func (r *TestRunner) matchTerraformComponent(actual *blueprintv1alpha1.TerraformComponent, expect blueprintv1alpha1.TerraformComponent) []string {
	var diffs []string
	identifier := expect.Name
	if identifier == "" {
		identifier = expect.Path
	}

	if expect.Source != "" && actual.Source != expect.Source {
		diffs = append(diffs, fmt.Sprintf("terraform[%s].source: expected %q, got %q", identifier, expect.Source, actual.Source))
	}

	if expect.Path != "" && actual.Path != expect.Path {
		diffs = append(diffs, fmt.Sprintf("terraform[%s].path: expected %q, got %q", identifier, expect.Path, actual.Path))
	}

	if len(expect.DependsOn) > 0 {
		for _, dep := range expect.DependsOn {
			if !contains(actual.DependsOn, dep) {
				diffs = append(diffs, fmt.Sprintf("terraform[%s].dependsOn: missing %q", identifier, dep))
			}
		}
	}

	if len(expect.Inputs) > 0 {
		for key, expectedValue := range expect.Inputs {
			actualValue, exists := actual.Inputs[key]
			if !exists {
				diffs = append(diffs, fmt.Sprintf("terraform[%s].inputs[%s]: key not found", identifier, key))
				continue
			}
			if !deepEqualInputsValue(expectedValue, actualValue) {
				diffs = append(diffs, fmt.Sprintf("terraform[%s].inputs[%s]: expected %v, got %v", identifier, key, expectedValue, actualValue))
			}
		}
	}

	return diffs
}

// matchKustomization compares an actual Kustomization against expected properties and returns a list
// of differences. It uses partial matching: only properties explicitly set in the expect kustomization
// are validated. The function checks path, source, dependsOn, components, and substitutions fields. For
// dependsOn and components, it verifies that all expected items are present in the actual kustomization's
// lists. For substitutions, it performs strict value equality checking - each expected key must exist with
// the exact expected value. Returns an empty slice if all specified properties match, or a list of
// formatted difference messages describing mismatches, with each message prefixed by the kustomization
// name for clarity.
func (r *TestRunner) matchKustomization(actual *blueprintv1alpha1.Kustomization, expect blueprintv1alpha1.Kustomization) []string {
	var diffs []string

	if expect.Path != "" && actual.Path != expect.Path {
		diffs = append(diffs, fmt.Sprintf("kustomize[%s].path: expected %q, got %q", expect.Name, expect.Path, actual.Path))
	}

	if expect.Source != "" && actual.Source != expect.Source {
		diffs = append(diffs, fmt.Sprintf("kustomize[%s].source: expected %q, got %q", expect.Name, expect.Source, actual.Source))
	}

	if len(expect.DependsOn) > 0 {
		for _, dep := range expect.DependsOn {
			if !contains(actual.DependsOn, dep) {
				diffs = append(diffs, fmt.Sprintf("kustomize[%s].dependsOn: missing %q", expect.Name, dep))
			}
		}
	}

	if len(expect.Components) > 0 {
		for _, comp := range expect.Components {
			if !contains(actual.Components, comp) {
				diffs = append(diffs, fmt.Sprintf("kustomize[%s].components: missing %q", expect.Name, comp))
			}
		}
	}

	if len(expect.Substitutions) > 0 {
		for key, expectedValue := range expect.Substitutions {
			actualValue, exists := actual.Substitutions[key]
			if !exists {
				diffs = append(diffs, fmt.Sprintf("kustomize[%s].substitutions[%s]: key not found", expect.Name, key))
				continue
			}
			if expectedValue != actualValue {
				diffs = append(diffs, fmt.Sprintf("kustomize[%s].substitutions[%s]: expected %q, got %q", expect.Name, key, expectedValue, actualValue))
			}
		}
	}

	return diffs
}

// validateBlueprint performs comprehensive structural validation on a composed blueprint to ensure
// it meets integrity requirements. It checks for duplicate Terraform components (by ID), duplicate
// Kustomizations (by name), duplicate components within Kustomization component lists, circular
// dependencies in both Terraform and Kustomization dependency graphs, and invalid dependencies
// (components that reference non-existent dependencies). These validations catch composition errors
// and blueprint configuration mistakes early. Returns a slice of descriptive error messages for
// each validation failure found, or an empty slice if the blueprint is valid.
func (r *TestRunner) validateBlueprint(bp *blueprintv1alpha1.Blueprint) []string {
	var errs []string

	errs = append(errs, r.validateDuplicateTerraformComponents(bp)...)
	errs = append(errs, r.validateDuplicateKustomizations(bp)...)
	errs = append(errs, r.validateDuplicateKustomizationComponents(bp)...)
	errs = append(errs, r.validateCircularDependencies(bp)...)
	errs = append(errs, r.validateInvalidDependencies(bp)...)

	return errs
}

// validateDuplicateTerraformComponents checks for duplicate Terraform components in the blueprint by
// comparing component IDs (which are derived from Name or Path via GetID()). It maintains a set of
// seen component IDs and reports an error for each duplicate encountered. Duplicate components indicate
// a composition error where multiple facets or configurations contributed the same component without
// proper merging or replacement. Returns a slice of error messages, one for each duplicate found.
func (r *TestRunner) validateDuplicateTerraformComponents(bp *blueprintv1alpha1.Blueprint) []string {
	var errs []string
	ids := make(map[string]struct{})
	for _, tf := range bp.TerraformComponents {
		id := tf.GetID()
		if _, exists := ids[id]; exists {
			errs = append(errs, fmt.Sprintf("duplicate terraform component ID: %s", id))
		}
		ids[id] = struct{}{}
	}
	return errs
}

// validateDuplicateKustomizations checks for duplicate Kustomizations in the blueprint by comparing
// kustomization names. It maintains a set of seen names and reports an error for each duplicate
// encountered. Duplicate kustomizations indicate a composition error where multiple facets contributed
// the same kustomization without proper merging or replacement. Returns a slice of error messages,
// one for each duplicate found.
func (r *TestRunner) validateDuplicateKustomizations(bp *blueprintv1alpha1.Blueprint) []string {
	var errs []string
	names := make(map[string]struct{})
	for _, k := range bp.Kustomizations {
		if _, exists := names[k.Name]; exists {
			errs = append(errs, fmt.Sprintf("duplicate kustomization name: %s", k.Name))
		}
		names[k.Name] = struct{}{}
	}
	return errs
}

// validateDuplicateKustomizationComponents checks for duplicate component references within each
// Kustomization's Components list. Empty strings are placeholders (e.g. conditional slot with no
// component) and may appear multiple times; only non-empty duplicate component names are reported.
// Returns a slice of error messages, one for each duplicate found.
func (r *TestRunner) validateDuplicateKustomizationComponents(bp *blueprintv1alpha1.Blueprint) []string {
	var errs []string
	for _, k := range bp.Kustomizations {
		components := make(map[string]struct{})
		for _, comp := range k.Components {
			if comp == "" {
				continue
			}
			if _, exists := components[comp]; exists {
				errs = append(errs, fmt.Sprintf("duplicate component %q in kustomization %q", comp, k.Name))
			}
			components[comp] = struct{}{}
		}
	}
	return errs
}

// validateCircularDependencies checks for circular dependency chains in both Terraform components
// and Kustomizations. It builds dependency graphs for each component type and uses depth-first search
// to detect cycles. Circular dependencies would cause infinite loops or undefined ordering during
// blueprint application, so they must be caught and reported. The function validates Terraform
// components separately from Kustomizations, as they have independent dependency graphs. Returns a
// slice of error messages describing each circular dependency found, including the full cycle path.
func (r *TestRunner) validateCircularDependencies(bp *blueprintv1alpha1.Blueprint) []string {
	var errs []string

	tfGraph := make(map[string][]string)
	tfIDs := make(map[string]struct{})
	for _, tf := range bp.TerraformComponents {
		id := tf.GetID()
		tfIDs[id] = struct{}{}
		tfGraph[id] = tf.DependsOn
	}
	errs = append(errs, detectCycles(tfGraph, tfIDs, "terraform component")...)

	kGraph := make(map[string][]string)
	kNames := make(map[string]struct{})
	for _, k := range bp.Kustomizations {
		kNames[k.Name] = struct{}{}
		kGraph[k.Name] = k.DependsOn
	}
	errs = append(errs, detectCycles(kGraph, kNames, "kustomization")...)

	return errs
}

// validateInvalidDependencies checks that all components referenced in DependsOn fields actually
// exist in the composed blueprint. It builds sets of valid Terraform component IDs and Kustomization
// names, then validates that every dependency reference points to an existing component. After blueprint
// composition completes, invalid dependencies should have been filtered out by the composer's validation
// logic, so any remaining invalid dependencies indicate a bug in the composition or validation code.
// This check serves as a safety net to catch composition errors. Returns a slice of error messages
// describing each invalid dependency found, identifying both the component with the invalid dependency
// and the non-existent component it references.
func (r *TestRunner) validateInvalidDependencies(bp *blueprintv1alpha1.Blueprint) []string {
	var errs []string

	tfIDs := make(map[string]struct{})
	for _, tf := range bp.TerraformComponents {
		tfIDs[tf.GetID()] = struct{}{}
	}

	kNames := make(map[string]struct{})
	for _, k := range bp.Kustomizations {
		kNames[k.Name] = struct{}{}
	}

	for _, tf := range bp.TerraformComponents {
		for _, dep := range tf.DependsOn {
			if _, exists := tfIDs[dep]; !exists {
				errs = append(errs, fmt.Sprintf("terraform component %q depends on non-existent component %q", tf.GetID(), dep))
			}
		}
	}

	for _, k := range bp.Kustomizations {
		for _, dep := range k.DependsOn {
			if _, exists := kNames[dep]; !exists {
				errs = append(errs, fmt.Sprintf("kustomization %q depends on non-existent kustomization %q", k.Name, dep))
			}
		}
	}

	return errs
}

// =============================================================================
// Helpers
// =============================================================================

// registerTerraformOutputHelperForMock registers a mock implementation of the terraform_output() expression helper
// for use in test scenarios. This allows tests to provide mock Terraform output values without requiring actual
// Terraform state or infrastructure. The helper validates that exactly two string arguments (component ID and output key)
// are provided. Unlike production, it always evaluates immediately (ignores the deferred flag) since mock outputs
// are always available during test execution. If the key exists in the component's outputs, it returns the value
// (which can be any type: string, array, object, etc.). If the key does not exist, it returns nil (not an error),
// enabling the ?? fallback operator to work correctly in expressions.
func registerTerraformOutputHelperForMock(mockProvider *terraform.MockTerraformProvider, eval evaluator.ExpressionEvaluator) {
	eval.Register("terraform_output", func(params []any, deferred bool) (any, error) {
		if len(params) != 2 {
			return nil, fmt.Errorf("terraform_output() requires exactly 2 arguments (component, key), got %d", len(params))
		}
		component, ok := params[0].(string)
		if !ok {
			return nil, fmt.Errorf("terraform_output() component must be a string, got %T", params[0])
		}
		key, ok := params[1].(string)
		if !ok {
			return nil, fmt.Errorf("terraform_output() key must be a string, got %T", params[1])
		}

		outputs, err := mockProvider.GetTerraformOutputs(component)
		if err != nil {
			return nil, fmt.Errorf("failed to get terraform outputs for component '%s': %w", component, err)
		}

		if value, exists := outputs[key]; exists {
			return value, nil
		}

		return nil, nil
	}, new(func(string, string) any))
}

// detectCycles performs a depth-first search to detect cycles in a dependency graph represented
// as a map from node names to their dependency lists. It uses a recursion stack to track the current
// path and identify back edges that indicate cycles. The validNodes set filters out references to
// non-existent nodes (which are handled by validateInvalidDependencies). When a cycle is detected,
// it constructs a readable cycle path showing the circular dependency chain. The nodeType parameter
// is used in error messages to indicate whether the cycle is in Terraform components or Kustomizations.
// Returns a slice of error messages, one for each cycle found.
func detectCycles(graph map[string][]string, validNodes map[string]struct{}, nodeType string) []string {
	var errs []string
	visited := make(map[string]bool)
	recursionStack := make(map[string]bool)

	var dfs func(node string, path []string)
	dfs = func(node string, path []string) {
		visited[node] = true
		recursionStack[node] = true
		path = append(path, node)

		for _, neighbor := range graph[node] {
			if _, exists := validNodes[neighbor]; !exists {
				continue
			}
			if recursionStack[neighbor] {
				cyclePath := []string{}
				found := false
				for _, pNode := range path {
					if pNode == neighbor {
						found = true
					}
					if found {
						cyclePath = append(cyclePath, pNode)
					}
				}
				cyclePath = append(cyclePath, neighbor)
				errs = append(errs, fmt.Sprintf("circular dependency detected in %s: %s", nodeType, strings.Join(cyclePath, " -> ")))
				continue
			}
			if !visited[neighbor] {
				dfs(neighbor, path)
			}
		}
		recursionStack[node] = false
	}

	for node := range graph {
		if !visited[node] {
			dfs(node, []string{})
		}
	}
	return errs
}

// contains checks whether a string slice contains a specific string value.
// It performs a linear search through the slice, returning true if the item is found
// and false otherwise. This is a simple utility function used throughout the test runner
// for checking membership in dependency lists, component lists, and other string collections.
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// deepEqualInputsValue compares expected and actual terraform input values with subset semantics
// so tests can assert minimal shape (e.g. instances with only name, role, disks). For slice-of-maps
// and map-of-maps, actual may contain extra keys; for slices, length and order must match and each
// expected element must be a subset of the corresponding actual element. Falls back to deepEqual
// for scalars and when subset semantics do not apply.
func deepEqualInputsValue(expected, actual any) bool {
	if expected == nil && actual == nil {
		return true
	}
	if expected == nil || actual == nil {
		return false
	}
	switch e := expected.(type) {
	case []any:
		a, ok := actual.([]any)
		if !ok || len(e) != len(a) {
			return false
		}
		for i := range e {
			if !deepEqualInputsValue(e[i], a[i]) {
				return false
			}
		}
		return true
	case map[string]any:
		a, ok := actual.(map[string]any)
		if !ok {
			return false
		}
		for k, ev := range e {
			av, exists := a[k]
			if !exists || !deepEqualInputsValue(ev, av) {
				return false
			}
		}
		return true
	default:
		return deepEqual(expected, actual)
	}
}

// deepEqual performs deep equality comparison between two arbitrary values.
// It handles maps, slices, and scalar values recursively, returning true if the values
// are structurally and value-equal. For maps, both must have the same keys with equal values.
// For slices, both must have the same length with equal elements in the same order.
// For scalar values, it uses standard Go equality comparison with fmt.Sprintf fallback
// to handle type differences (e.g., int vs float64 from YAML parsing).
func deepEqual(expected, actual any) bool {
	if expected == nil && actual == nil {
		return true
	}
	if expected == nil || actual == nil {
		return false
	}

	switch e := expected.(type) {
	case map[string]any:
		a, ok := actual.(map[string]any)
		if !ok {
			return false
		}
		if len(e) != len(a) {
			return false
		}
		for k, ev := range e {
			av, exists := a[k]
			if !exists || !deepEqual(ev, av) {
				return false
			}
		}
		return true

	case []any:
		a, ok := actual.([]any)
		if !ok {
			return false
		}
		if len(e) != len(a) {
			return false
		}
		for i := range e {
			if !deepEqual(e[i], a[i]) {
				return false
			}
		}
		return true

	default:
		if expected == actual {
			return true
		}
		return fmt.Sprintf("%v", expected) == fmt.Sprintf("%v", actual)
	}
}
