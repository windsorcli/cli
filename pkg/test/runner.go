// The TestRunner runs the static tests of a Windsor blueprint.
// It finds the test cases in the YAML files under contexts/_template/tests and composes the blueprint for each case.
// It compares the composed blueprint with the expected result, so facet logic is tested without infrastructure.
// It provides the constructor, the run functions, the test case discovery, and the result output.

package test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/goccy/go-yaml"
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/internal/shims"
	"github.com/windsorcli/cli/pkg/composer/artifact"
	"github.com/windsorcli/cli/pkg/composer/blueprint"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/evaluator"
	"github.com/windsorcli/cli/pkg/runtime/shell"
	"github.com/windsorcli/cli/pkg/runtime/terraform"
)

// =============================================================================
// Constants
// =============================================================================

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
	shims               *shims.Shims
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
		shims:           shims.NewShims(),
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

// createGenerator creates a function that generates blueprints from test values.
// It sets up an isolated runtime environment for each test case, ensuring complete test isolation by creating
// a fresh ConfigHandler instance and configuring the runtime to use only the _template directory. The generator
// applies test values directly without loading any context files, ensuring tests only use explicitly provided inputs.
// If terraformOutputs are provided, it registers a mock TerraformProvider to supply mock outputs for terraform_output()
// expressions. This allows tests to validate blueprint composition that depends on Terraform outputs without requiring
// actual Terraform state. The env map supplies a hermetic environment for env() expressions, resolved only
// from these entries so composition never reads the host env; WINDSOR_CONTEXT defaults to "test" and the
// env map overrides it. Context isolation itself comes from the fresh ConfigHandler's .WithContext("test"),
// which outranks the .windsor/context file and the WINDSOR_CONTEXT env var in GetContext.
// When applySchemaDefaults is true the case composes with schema defaults materialized as production does,
// rather than the test path's default skip, so a facet that reads a schema-defaulted field resolves it.
// Returns a function that takes test values and returns a composed blueprint or an error.
func (r *TestRunner) createGenerator(terraformOutputs map[string]map[string]any, env map[string]string, applySchemaDefaults bool) func(values map[string]any) (*blueprintv1alpha1.Blueprint, error) {
	return func(values map[string]any) (*blueprintv1alpha1.Blueprint, error) {
		freshConfigHandler := config.NewConfigHandler(r.baseShell).WithContext("test")
		freshConfigHandler.SetApplySchemaDefaults(applySchemaDefaults)

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

		if err := rt.ConfigHandler.ValidateContextValues(); err != nil {
			return nil, err
		}

		if err := rt.InitializeComponents(); err != nil {
			return nil, fmt.Errorf("failed to initialize components: %w", err)
		}

		effectiveEnv := map[string]string{"WINDSOR_CONTEXT": "test"}
		for key, value := range env {
			effectiveEnv[key] = value
		}
		rt.Evaluator.SetEnvLookup(func(name string) (string, bool) {
			value, present := effectiveEnv[name]
			return value, present
		})

		testBlueprintHandler := blueprint.NewBlueprintHandler(rt, r.artifactBuilder)

		referencedComponents := make(map[string]struct{})
		var referencedMu sync.Mutex

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
			recordReference := func(componentID string) {
				referencedMu.Lock()
				referencedComponents[componentID] = struct{}{}
				referencedMu.Unlock()
			}
			registerTerraformOutputHelperForMock(mockProvider, rt.Evaluator, recordReference)
		}

		defaultURL := r.DefaultBlueprintURL
		if defaultURL == "" {
			defaultURL = constants.GetEffectiveBlueprintURL()
		}
		var initURLs []string
		if defaultURL != "" && defaultURL != SkipDefaultBlueprintURL {
			templateRoot := filepath.Join(r.baseProjectRoot, "contexts", "_template")
			if _, err := r.shims.Stat(templateRoot); err == nil {
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
		bp.Config = testBlueprintHandler.GetConfigScope()

		if err := validateTerraformOutputReferences(bp, referencedComponents, &referencedMu); err != nil {
			return nil, err
		}

		return bp, nil
	}
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
	if _, err := r.shims.Stat(testsDir); os.IsNotExist(err) {
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
	data, err := r.shims.ReadFile(path)
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

	generator := r.createGenerator(tc.TerraformOutputs, tc.Env, tc.ApplySchemaDefaults)
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

// =============================================================================
// Helpers
// =============================================================================

// validateTerraformOutputReferences fails composition when a terraform_output() expression named a
// component that the composed blueprint does not contain. This mirrors the live runtime error raised
// at env-var build time ("component not found: <name>") so the failure surfaces in windsor test rather
// than leaking into windsor bootstrap. Missing names are sorted and joined into a single error so the
// operator sees every offending reference in one pass rather than fixing one and re-running.
func validateTerraformOutputReferences(bp *blueprintv1alpha1.Blueprint, referenced map[string]struct{}, mu *sync.Mutex) error {
	if bp == nil || len(referenced) == 0 {
		return nil
	}

	registered := make(map[string]struct{}, len(bp.TerraformComponents)*2)
	for _, c := range bp.TerraformComponents {
		if c.Path != "" {
			registered[c.Path] = struct{}{}
		}
		if c.Name != "" {
			registered[c.Name] = struct{}{}
		}
	}

	mu.Lock()
	defer mu.Unlock()

	var missing []string
	for ref := range referenced {
		if _, ok := registered[ref]; !ok {
			missing = append(missing, ref)
		}
	}
	if len(missing) == 0 {
		return nil
	}

	sort.Strings(missing)
	if len(missing) == 1 {
		return fmt.Errorf("component not found: %s", missing[0])
	}
	return fmt.Errorf("component not found: %s", strings.Join(missing, ", "))
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

// registerTerraformOutputHelperForMock registers a mock implementation of the terraform_output() expression helper
// for use in test scenarios. This allows tests to provide mock Terraform output values without requiring actual
// Terraform state or infrastructure. The helper validates that exactly two string arguments (component ID and output key)
// are provided. Unlike production, it always evaluates immediately (ignores the deferred flag) since mock outputs
// are always available during test execution.
//
// The recordReference callback (when non-nil) is invoked with each component ID the expression references. The runner
// uses this to collect a deferred validation set: after composition completes, any referenced component absent from the
// composed blueprint surfaces as "component not found: <name>", matching the live runtime error raised at env-var
// building time. Validation must run after Generate() because facet processing and the composer evaluate inputs before
// any single facet's components reach the composedBlueprint; an inline check inside the helper would false-positive on
// same-facet sibling references. The callback is optional so helper-level unit tests need not wire it.
//
// If the key exists in the supplied mock outputs, the value is returned; if not, nil is returned (not an error) so the
// ?? fallback operator continues to work for deferred wiring patterns (key absent on a registered component).
func registerTerraformOutputHelperForMock(mockProvider *terraform.MockTerraformProvider, eval evaluator.ExpressionEvaluator, recordReference func(componentID string)) {
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

		if recordReference != nil {
			recordReference(component)
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
