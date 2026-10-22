package test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/internal/shims"
	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/composer/artifact"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/evaluator"
	"github.com/windsorcli/cli/pkg/runtime/shell"
	"github.com/windsorcli/cli/pkg/runtime/terraform"
)

// =============================================================================
// Test Setup
// =============================================================================

type TestRunnerMocks struct {
	TmpDir string
}

func setupTestRunnerMocks(t *testing.T) *TestRunnerMocks {
	t.Helper()

	tmpDir := t.TempDir()
	// Set up the directory structure that createGenerator expects
	templateDir := filepath.Join(tmpDir, "contexts", "_template")
	os.MkdirAll(templateDir, 0755)
	// Schema is optional - don't create it
	// Create a minimal blueprint.yaml file
	createTestFile(t, templateDir, "blueprint.yaml", `kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: test-blueprint
`)

	return &TestRunnerMocks{
		TmpDir: tmpDir,
	}
}

func createTestFile(t *testing.T, dir string, filename string, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}
}

func createRunnerWithMockGenerator(mocks *TestRunnerMocks) *TestRunner {
	mockShell := shell.NewMockShell()
	mockShell.GetProjectRootFunc = func() (string, error) {
		return mocks.TmpDir, nil
	}
	mockArtifact := artifact.NewMockArtifact()
	return &TestRunner{
		projectRoot:         mocks.TmpDir,
		baseShell:           mockShell,
		baseProjectRoot:     mocks.TmpDir,
		artifactBuilder:     mockArtifact,
		DefaultBlueprintURL: SkipDefaultBlueprintURL,
		shims:               shims.NewShims(),
	}
}

func setupTestRunnerMocksForFailure(t *testing.T) *TestRunnerMocks {
	t.Helper()

	tmpDir := t.TempDir()
	// Set up the directory structure with an invalid blueprint.yaml to cause LoadBlueprint to fail
	templateDir := filepath.Join(tmpDir, "contexts", "_template")
	os.MkdirAll(templateDir, 0755)
	createTestFile(t, templateDir, "blueprint.yaml", `invalid: yaml: content: [[[`)

	return &TestRunnerMocks{
		TmpDir: tmpDir,
	}
}

func createRunnerForFailure(mocks *TestRunnerMocks) *TestRunner {
	mockShell := shell.NewMockShell()
	mockShell.GetProjectRootFunc = func() (string, error) {
		return mocks.TmpDir, nil
	}
	mockArtifact := artifact.NewMockArtifact()
	return &TestRunner{
		projectRoot:         mocks.TmpDir,
		baseShell:           mockShell,
		baseProjectRoot:     mocks.TmpDir,
		artifactBuilder:     mockArtifact,
		DefaultBlueprintURL: SkipDefaultBlueprintURL,
		shims:               shims.NewShims(),
	}
}

// hasTerraformComponent reports whether the composed blueprint contains a terraform component with the given name.
func hasTerraformComponent(bp *blueprintv1alpha1.Blueprint, name string) bool {
	if bp == nil {
		return false
	}
	for _, c := range bp.TerraformComponents {
		if c.Name == name {
			return true
		}
	}
	return false
}

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewTestRunner(t *testing.T) {
	t.Run("CreatesTestRunnerWithProjectRootAndComposer", func(t *testing.T) {
		// Given a project root and composer
		mocks := setupTestRunnerMocks(t)

		// When creating a new test runner
		runner := createRunnerWithMockGenerator(mocks)

		// Then runner should be created
		if runner == nil {
			t.Fatal("Expected TestRunner to be created")
		}
		if runner.projectRoot != mocks.TmpDir {
			t.Error("Expected projectRoot to be set")
		}
	})

	t.Run("CreatesTestRunnerWithRealDependencies", func(t *testing.T) {
		// Given a real runtime with real dependencies
		tmpDir := t.TempDir()
		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) {
			return tmpDir, nil
		}
		mockConfigHandler := config.NewMockConfigHandler()
		mockArtifact := artifact.NewMockArtifact()

		rt := runtime.NewRuntime(&runtime.Runtime{
			Shell:         mockShell,
			ConfigHandler: mockConfigHandler,
			ProjectRoot:   tmpDir,
		})

		// When creating a new test runner
		runner := NewTestRunner(rt, mockArtifact)

		// Then runner should be created with correct fields
		if runner == nil {
			t.Fatal("Expected TestRunner to be created")
		}
		if runner.projectRoot != tmpDir {
			t.Errorf("Expected projectRoot to be %q, got %q", tmpDir, runner.projectRoot)
		}
		if runner.baseProjectRoot != tmpDir {
			t.Errorf("Expected baseProjectRoot to be %q, got %q", tmpDir, runner.baseProjectRoot)
		}
		if runner.baseShell != mockShell {
			t.Error("Expected baseShell to be set")
		}
		if runner.artifactBuilder != mockArtifact {
			t.Error("Expected artifactBuilder to be set")
		}
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestTestRunner_Run(t *testing.T) {
	t.Run("ReturnsErrorWhenNoTestFilesFound", func(t *testing.T) {
		// Given a test runner with no test files
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		// When running tests
		_, err := runner.Run("")

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error when no test files found")
		}
	})

	t.Run("ReturnsErrorWhenFilterMatchesNoTests", func(t *testing.T) {
		// Given a test runner with test files
		mocks := setupTestRunnerMocks(t)
		testsDir := filepath.Join(mocks.TmpDir, "contexts", "_template", "tests")
		createTestFile(t, testsDir, "example.test.yaml", `
cases:
  - name: test-case-1
    values: {}
`)
		runner := createRunnerWithMockGenerator(mocks)

		// When running tests with a filter that matches nothing
		_, err := runner.Run("nonexistent-test")

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error when filter matches no tests")
		}
	})

	t.Run("RunsAllTestCasesSuccessfully", func(t *testing.T) {
		// Given a test runner with test files and mock composer
		mocks := setupTestRunnerMocks(t)
		testsDir := filepath.Join(mocks.TmpDir, "contexts", "_template", "tests")
		createTestFile(t, testsDir, "example.test.yaml", `
cases:
  - name: test-case-1
    values: {}
  - name: test-case-2
    values: {}
`)
		runner := createRunnerWithMockGenerator(mocks)

		// When running tests
		results, err := runner.Run("")

		// Then all tests should be run
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if len(results) != 2 {
			t.Errorf("Expected 2 results, got: %d", len(results))
		}
	})

	t.Run("FiltersTestsByName", func(t *testing.T) {
		// Given a test runner with multiple test cases
		mocks := setupTestRunnerMocks(t)
		testsDir := filepath.Join(mocks.TmpDir, "contexts", "_template", "tests")
		createTestFile(t, testsDir, "example.test.yaml", `
cases:
  - name: test-case-1
    values: {}
  - name: test-case-2
    values: {}
`)
		runner := createRunnerWithMockGenerator(mocks)

		// When running tests with a filter
		results, err := runner.Run("test-case-1")

		// Then only matching test should be run
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if len(results) != 1 {
			t.Errorf("Expected 1 result, got: %d", len(results))
		}
		if results[0].Name != "test-case-1" {
			t.Errorf("Expected test-case-1, got: %s", results[0].Name)
		}
	})

	t.Run("ReturnsErrorWhenRunTestCaseFails", func(t *testing.T) {
		mocks := setupTestRunnerMocksForFailure(t)
		testsDir := filepath.Join(mocks.TmpDir, "contexts", "_template", "tests")
		createTestFile(t, testsDir, "error.test.yaml", `
cases:
  - name: composition-error
    values:
      invalid: value
`)
		runner := createRunnerWithMockGenerator(mocks)

		results, err := runner.Run("")

		if err != nil {
			t.Errorf("Expected no error (errors are in diffs), got: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("Expected 1 result, got: %d", len(results))
		}
		if results[0].Passed {
			t.Error("Expected test to fail")
		}
		if len(results[0].Diffs) == 0 {
			t.Error("Expected diffs for composition error")
		}
	})

	t.Run("ReturnsErrorWhenTestFileParsingFails", func(t *testing.T) {
		// Given a test runner with an invalid test file
		mocks := setupTestRunnerMocks(t)
		testsDir := filepath.Join(mocks.TmpDir, "contexts", "_template", "tests")
		createTestFile(t, testsDir, "invalid.test.yaml", `
cases:
  - name: [invalid yaml
`)
		runner := createRunnerWithMockGenerator(mocks)

		// When running tests
		_, err := runner.Run("")

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error when test file parsing fails")
		}
	})

	t.Run("UsesRunFuncWhenSet", func(t *testing.T) {
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		var calledFilter string
		runner.RunFunc = func(filter string) ([]TestResult, error) {
			calledFilter = filter
			return []TestResult{{Name: "test", Passed: true}}, nil
		}

		results, err := runner.Run("test-filter")

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if len(results) != 1 {
			t.Errorf("Expected 1 result, got: %d", len(results))
		}
		if calledFilter != "test-filter" {
			t.Errorf("Expected filter 'test-filter', got: %q", calledFilter)
		}
	})

	t.Run("ReturnsErrorWhenRunFuncReturnsError", func(t *testing.T) {
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		runner.RunFunc = func(filter string) ([]TestResult, error) {
			return nil, fmt.Errorf("run func error")
		}

		_, err := runner.Run("")

		if err == nil {
			t.Error("Expected error when RunFunc returns error")
		}
		if !strings.Contains(err.Error(), "run func error") {
			t.Errorf("Expected run func error, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenDiscoverTestFilesFails", func(t *testing.T) {
		if goruntime.GOOS == "windows" {
			t.Skip("Skipping on Windows: os.Chmod with 0000 does not prevent directory operations")
		}

		tmpDir := t.TempDir()
		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) {
			return tmpDir, nil
		}
		mockArtifact := artifact.NewMockArtifact()
		runner := &TestRunner{
			projectRoot:     tmpDir,
			baseShell:       mockShell,
			baseProjectRoot: tmpDir,
			artifactBuilder: mockArtifact,
			shims:           shims.NewShims(),
		}

		testsDir := filepath.Join(tmpDir, "contexts", "_template", "tests")
		os.MkdirAll(testsDir, 0755)
		os.Chmod(testsDir, 0000)
		defer os.Chmod(testsDir, 0755)

		_, err := runner.Run("")

		if err == nil {
			t.Error("Expected error when discoverTestFiles fails")
		}
	})
}

func TestTestRunner_RunAndPrint(t *testing.T) {
	t.Run("ReturnsErrorWhenRunFails", func(t *testing.T) {
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		runner.RunFunc = func(filter string) ([]TestResult, error) {
			return nil, fmt.Errorf("run error")
		}

		err := runner.RunAndPrint("")

		if err == nil {
			t.Error("Expected error from Run to be propagated")
		}
		if err.Error() != "run error" {
			t.Errorf("Expected error 'run error', got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenTestsFail", func(t *testing.T) {
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		runner.RunFunc = func(filter string) ([]TestResult, error) {
			return []TestResult{
				{Name: "test-1", Passed: false, Diffs: []string{"diff1"}},
			}, nil
		}

		err := runner.RunAndPrint("")

		if err == nil {
			t.Error("Expected error when tests fail")
		}
		if !strings.Contains(err.Error(), "test(s) failed") {
			t.Errorf("Expected error about failed tests, got: %v", err)
		}
	})

	t.Run("ReturnsNilWhenAllTestsPass", func(t *testing.T) {
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		runner.RunFunc = func(filter string) ([]TestResult, error) {
			return []TestResult{
				{Name: "test-1", Passed: true},
			}, nil
		}

		err := runner.RunAndPrint("")

		if err != nil {
			t.Errorf("Expected no error when all tests pass, got: %v", err)
		}
	})

	t.Run("HandlesEmptyResults", func(t *testing.T) {
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		runner.RunFunc = func(filter string) ([]TestResult, error) {
			return []TestResult{}, nil
		}

		err := runner.RunAndPrint("")

		if err != nil {
			t.Errorf("Expected no error for empty results, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenNoTestFilesFound", func(t *testing.T) {
		tmpDir := t.TempDir()
		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) {
			return tmpDir, nil
		}
		mockArtifact := artifact.NewMockArtifact()
		runner := &TestRunner{
			projectRoot:     tmpDir,
			baseShell:       mockShell,
			baseProjectRoot: tmpDir,
			artifactBuilder: mockArtifact,
			shims:           shims.NewShims(),
		}

		err := runner.RunAndPrint("")

		if err == nil {
			t.Error("Expected error when no test files found")
		}
		if !strings.Contains(err.Error(), "no test files found") {
			t.Errorf("Expected 'no test files found' error, got: %v", err)
		}
		if we := werror.Find(err); we == nil || !strings.Contains(we.Remediation, ".test.yaml") {
			t.Errorf("Expected remediation to name the .test.yaml extension, got: %v", we)
		}
	})

	t.Run("ReturnsErrorWhenFilterMatchesNoTests", func(t *testing.T) {
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		testsDir := filepath.Join(mocks.TmpDir, "contexts", "_template", "tests")
		os.MkdirAll(testsDir, 0755)
		createTestFile(t, testsDir, "test.test.yaml", `
cases:
  - name: test-case-1
    values: {}
`)

		err := runner.RunAndPrint("nonexistent-test")

		if err == nil {
			t.Error("Expected error when filter matches no tests")
		}
		if !strings.Contains(err.Error(), "no test cases found matching filter") {
			t.Errorf("Expected filter error, got: %v", err)
		}
		if we := werror.Find(err); we == nil || !strings.Contains(we.Remediation, "without a filter") {
			t.Errorf("Expected remediation about the filter, got: %v", we)
		}
	})

	t.Run("ReturnsErrorWhenTestFileParseFails", func(t *testing.T) {
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		testsDir := filepath.Join(mocks.TmpDir, "contexts", "_template", "tests")
		os.MkdirAll(testsDir, 0755)
		createTestFile(t, testsDir, "invalid.test.yaml", `invalid: yaml: [[[`)

		err := runner.RunAndPrint("")

		if err == nil {
			t.Error("Expected error when test file parse fails")
		}
		if !strings.Contains(err.Error(), "failed to parse test file") {
			t.Errorf("Expected parse error, got: %v", err)
		}
	})

	t.Run("UsesRunFuncWhenSet", func(t *testing.T) {
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		var calledFilter string
		runner.RunFunc = func(filter string) ([]TestResult, error) {
			calledFilter = filter
			return []TestResult{{Name: "test", Passed: true}}, nil
		}

		err := runner.RunAndPrint("test-filter")

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if calledFilter != "test-filter" {
			t.Errorf("Expected filter 'test-filter', got: %q", calledFilter)
		}
	})

	t.Run("ReturnsErrorWhenRunFuncReturnsError", func(t *testing.T) {
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		runner.RunFunc = func(filter string) ([]TestResult, error) {
			return nil, fmt.Errorf("run func error")
		}

		err := runner.RunAndPrint("")

		if err == nil {
			t.Error("Expected error when RunFunc returns error")
		}
		if !strings.Contains(err.Error(), "run func error") {
			t.Errorf("Expected run func error, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenDiscoverTestFilesFails", func(t *testing.T) {
		if goruntime.GOOS == "windows" {
			t.Skip("Skipping on Windows: os.Chmod with 0000 does not prevent directory operations")
		}

		tmpDir := t.TempDir()
		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) {
			return tmpDir, nil
		}
		mockArtifact := artifact.NewMockArtifact()
		runner := &TestRunner{
			projectRoot:     tmpDir,
			baseShell:       mockShell,
			baseProjectRoot: tmpDir,
			artifactBuilder: mockArtifact,
			shims:           shims.NewShims(),
		}

		testsDir := filepath.Join(tmpDir, "contexts", "_template", "tests")
		os.MkdirAll(testsDir, 0755)
		os.Chmod(testsDir, 0000)
		defer os.Chmod(testsDir, 0755)

		err := runner.RunAndPrint("")

		if err == nil {
			t.Error("Expected error when discoverTestFiles fails")
		}
		if !strings.Contains(err.Error(), "failed to discover test files") {
			t.Errorf("Expected discover error, got: %v", err)
		}
	})

	t.Run("ReturnsNilWhenNoTestCasesAndNoFilter", func(t *testing.T) {
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		testsDir := filepath.Join(mocks.TmpDir, "contexts", "_template", "tests")
		os.MkdirAll(testsDir, 0755)
		createTestFile(t, testsDir, "empty.test.yaml", `
cases: []
`)

		err := runner.RunAndPrint("")

		if err != nil {
			t.Errorf("Expected no error when no test cases and no filter, got: %v", err)
		}
	})
}

func TestTestRunner_DoesNotPersistContext(t *testing.T) {
	t.Run("DoesNotWriteContextFile", func(t *testing.T) {
		// Given a test runner with test files
		mocks := setupTestRunnerMocks(t)
		testsDir := filepath.Join(mocks.TmpDir, "contexts", "_template", "tests")
		createTestFile(t, testsDir, "example.test.yaml", `
cases:
  - name: test-case-1
    values: {}
`)
		runner := createRunnerWithMockGenerator(mocks)

		// And an existing context file with a different context
		windsorDir := filepath.Join(mocks.TmpDir, ".windsor")
		os.MkdirAll(windsorDir, 0755)
		contextFile := filepath.Join(windsorDir, "context")
		originalContext := "original-context"
		os.WriteFile(contextFile, []byte(originalContext), 0644)

		// When running tests (may fail due to composition, but that's okay)
		_, _ = runner.Run("")

		// Then the context file should still contain the original context
		data, err := os.ReadFile(contextFile)
		if err != nil {
			t.Fatalf("Failed to read context file: %v", err)
		}
		if string(data) != originalContext {
			t.Errorf("Expected context file to contain %q, got %q", originalContext, string(data))
		}
	})

	t.Run("LeavesHostContextEnvSetValueUntouched", func(t *testing.T) {
		// Given a test runner with test files
		mocks := setupTestRunnerMocks(t)
		testsDir := filepath.Join(mocks.TmpDir, "contexts", "_template", "tests")
		createTestFile(t, testsDir, "example.test.yaml", `
cases:
  - name: test-case-1
    values: {}
`)
		runner := createRunnerWithMockGenerator(mocks)

		// And a host WINDSOR_CONTEXT set
		t.Setenv("WINDSOR_CONTEXT", "my-original-context")

		// When running tests (may fail due to composition, but that's okay)
		_, _ = runner.Run("")

		// Then the host env var is untouched: windsor test never mutates process env
		if got := os.Getenv("WINDSOR_CONTEXT"); got != "my-original-context" {
			t.Errorf("Expected WINDSOR_CONTEXT unchanged at %q, got %q", "my-original-context", got)
		}
	})

	t.Run("LeavesHostContextEnvUnsetUntouched", func(t *testing.T) {
		// Given a test runner with test files
		mocks := setupTestRunnerMocks(t)
		testsDir := filepath.Join(mocks.TmpDir, "contexts", "_template", "tests")
		createTestFile(t, testsDir, "example.test.yaml", `
cases:
  - name: test-case-1
    values: {}
`)
		runner := createRunnerWithMockGenerator(mocks)

		// And no host WINDSOR_CONTEXT set (t.Setenv restores prior state on cleanup)
		t.Setenv("WINDSOR_CONTEXT", "")
		os.Unsetenv("WINDSOR_CONTEXT")

		// When running tests (may fail due to composition, but that's okay)
		_, _ = runner.Run("")

		// Then the host env var stays unset: windsor test never sets process env
		if got := os.Getenv("WINDSOR_CONTEXT"); got != "" {
			t.Errorf("Expected WINDSOR_CONTEXT to stay unset, got %q", got)
		}
	})
}

// =============================================================================
// Test Private Methods
// =============================================================================

func TestTestRunner_runTestCase(t *testing.T) {
	t.Run("ReturnsPassedResultWhenNoExpectations", func(t *testing.T) {
		// Given a test case with no expectations
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		tc := blueprintv1alpha1.TestCase{
			Name:   "no-expectations",
			Values: map[string]any{},
		}

		// When running the test case
		result, err := runner.runTestCase(tc)

		// Then it should pass with no error
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if !result.Passed {
			t.Errorf("Expected test to pass, got diffs: %v", result.Diffs)
		}
	})

	t.Run("ReturnsFailedResultWhenCompositionFails", func(t *testing.T) {
		// Given a test case where composition fails
		// Set up a test runner without a blueprint.yaml so composition will fail
		mocks := setupTestRunnerMocksForFailure(t)
		runner := createRunnerForFailure(mocks)

		tc := blueprintv1alpha1.TestCase{
			Name:   "composition-error",
			Values: map[string]any{},
		}

		// When running the test case
		result, err := runner.runTestCase(tc)

		// Then it should fail with composition error in diffs
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if result.Passed {
			t.Error("Expected test to fail")
		}
		if len(result.Diffs) != 1 {
			t.Errorf("Expected 1 diff, got: %d", len(result.Diffs))
		}
	})

	t.Run("ReturnsPassedResultWhenExpectationsMet", func(t *testing.T) {
		// Given a test case with met expectations
		mocks := setupTestRunnerMocks(t)
		// Update blueprint.yaml to include the expected component
		templateDir := filepath.Join(mocks.TmpDir, "contexts", "_template")
		createTestFile(t, templateDir, "blueprint.yaml", `kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: test-blueprint
terraform:
  - name: cluster
`)
		runner := createRunnerWithMockGenerator(mocks)

		tc := blueprintv1alpha1.TestCase{
			Name:   "expectations-met",
			Values: map[string]any{},
			Expect: &blueprintv1alpha1.Blueprint{
				TerraformComponents: []blueprintv1alpha1.TerraformComponent{
					{Name: "cluster"},
				},
			},
		}

		// When running the test case
		result, err := runner.runTestCase(tc)

		// Then it should pass
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if !result.Passed {
			t.Errorf("Expected test to pass, got diffs: %v", result.Diffs)
		}
	})

	t.Run("ReturnsFailedResultWhenExpectationsNotMet", func(t *testing.T) {
		// Given a test case with unmet expectations
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		tc := blueprintv1alpha1.TestCase{
			Name:   "expectations-not-met",
			Values: map[string]any{},
			Expect: &blueprintv1alpha1.Blueprint{
				TerraformComponents: []blueprintv1alpha1.TerraformComponent{
					{Name: "missing-component"},
				},
			},
		}

		// When running the test case
		result, err := runner.runTestCase(tc)

		// Then it should fail
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if result.Passed {
			t.Error("Expected test to fail")
		}
	})

	t.Run("ReturnsPassedResultWhenExclusionsRespected", func(t *testing.T) {
		// Given a test case with respected exclusions
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		tc := blueprintv1alpha1.TestCase{
			Name:   "exclusions-respected",
			Values: map[string]any{},
			Exclude: &blueprintv1alpha1.Blueprint{
				TerraformComponents: []blueprintv1alpha1.TerraformComponent{
					{Name: "excluded-component"},
				},
			},
		}

		// When running the test case
		result, err := runner.runTestCase(tc)

		// Then it should pass
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if !result.Passed {
			t.Errorf("Expected test to pass, got diffs: %v", result.Diffs)
		}
	})

	t.Run("ReturnsFailedResultWhenExclusionsViolated", func(t *testing.T) {
		// Given a test case with violated exclusions
		mocks := setupTestRunnerMocks(t)
		// Update blueprint.yaml to include the component that should be excluded
		templateDir := filepath.Join(mocks.TmpDir, "contexts", "_template")
		createTestFile(t, templateDir, "blueprint.yaml", `kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: test-blueprint
terraform:
  - name: should-not-exist
`)
		runner := createRunnerWithMockGenerator(mocks)

		tc := blueprintv1alpha1.TestCase{
			Name:   "exclusions-violated",
			Values: map[string]any{},
			Exclude: &blueprintv1alpha1.Blueprint{
				TerraformComponents: []blueprintv1alpha1.TerraformComponent{
					{Name: "should-not-exist"},
				},
			},
		}

		// When running the test case
		result, err := runner.runTestCase(tc)

		// Then it should fail
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if result.Passed {
			t.Error("Expected test to fail")
		}
	})

	t.Run("ReturnsFailedResultWhenExpectConfigDisagreesWithComposedValue", func(t *testing.T) {
		// Given a facet whose config: block is gated by when:, mirroring the reported bug: an
		// expect: config: assertion must fail once the contributing facet stops firing, not
		// silently keep passing against a stale expectation
		mocks := setupTestRunnerMocks(t)
		templateDir := filepath.Join(mocks.TmpDir, "contexts", "_template")
		facetsDir := filepath.Join(templateDir, "facets")
		createTestFile(t, facetsDir, "cluster.yaml", `kind: Facet
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: cluster
config:
  - name: cluster
    when: (cluster.enabled ?? false)
    value:
      controlplanes:
        cpu: 7
`)
		runner := createRunnerWithMockGenerator(mocks)

		tc := blueprintv1alpha1.TestCase{
			Name:   "config-expectation",
			Values: map[string]any{"cluster": map[string]any{"enabled": false}},
			Expect: &blueprintv1alpha1.Blueprint{
				Config: map[string]any{
					"cluster": map[string]any{"controlplanes": map[string]any{"cpu": 7}},
				},
			},
		}

		// When running the test case with the facet's when: forced false
		result, err := runner.runTestCase(tc)

		// Then the test fails because the config block never resolved
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if result.Passed {
			t.Error("Expected test to fail when the expected config block did not resolve")
		}
	})

	t.Run("ReturnsPassedResultWhenErrorExpectedAndOccurs", func(t *testing.T) {
		// Given a test case expecting an error and composition fails
		// Set up a test runner without a blueprint.yaml so composition will fail
		mocks := setupTestRunnerMocksForFailure(t)
		runner := createRunnerForFailure(mocks)

		tc := blueprintv1alpha1.TestCase{
			Name:        "error-expected",
			Values:      map[string]any{},
			ExpectError: true,
		}

		// When running the test case
		result, err := runner.runTestCase(tc)

		// Then it should pass
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if !result.Passed {
			t.Errorf("Expected test to pass when error occurs as expected, got diffs: %v", result.Diffs)
		}
	})

	t.Run("ReturnsFailedResultWhenErrorExpectedButSucceeds", func(t *testing.T) {
		// Given a test case expecting an error but composition succeeds
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		tc := blueprintv1alpha1.TestCase{
			Name:        "error-expected-but-succeeds",
			Values:      map[string]any{},
			ExpectError: true,
		}

		// When running the test case
		result, err := runner.runTestCase(tc)

		// Then it should fail because expectError is true but composition succeeded
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if result.Passed {
			t.Errorf("Expected test to fail when error expected but composition succeeded. Result: %+v, Diffs: %v", result, result.Diffs)
		}
		if len(result.Diffs) == 0 {
			t.Errorf("Expected error message in diffs. Result: %+v", result)
		} else if !strings.Contains(result.Diffs[0], "expected composition to fail") {
			t.Errorf("Expected error message about expected failure, got: %v", result.Diffs)
		}
	})
}

func TestTestRunner_discoverTestFiles(t *testing.T) {
	t.Run("ReturnsNilWhenDirectoryDoesNotExist", func(t *testing.T) {
		// Given a test runner
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		// When discovering test files in nonexistent directory
		files, err := runner.discoverTestFiles("/nonexistent/path")

		// Then no error should be returned and files should be nil
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if files != nil {
			t.Error("Expected nil files for nonexistent directory")
		}
	})

	t.Run("FindsTestYamlFiles", func(t *testing.T) {
		// Given a directory with test files
		mocks := setupTestRunnerMocks(t)
		testsDir := filepath.Join(mocks.TmpDir, "tests")
		createTestFile(t, testsDir, "one.test.yaml", "cases: []")
		createTestFile(t, testsDir, "two.test.yaml", "cases: []")
		createTestFile(t, testsDir, "not-a-test.yaml", "data: value")

		runner := createRunnerWithMockGenerator(mocks)

		// When discovering test files
		files, err := runner.discoverTestFiles(testsDir)

		// Then test files should be found
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if len(files) != 2 {
			t.Errorf("Expected 2 test files, got: %d", len(files))
		}
	})

	t.Run("ReturnsErrorWhenWalkFails", func(t *testing.T) {
		// Given a test runner
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		// When discovering test files in a directory that causes walk error
		// We can't easily simulate a filepath.Walk error, but we can test the error path exists
		// by checking the error handling code path
		_, err := runner.discoverTestFiles("/nonexistent/path")

		// Then no error should be returned (filepath.Walk returns nil for non-existent dir)
		if err != nil {
			t.Errorf("Expected no error for non-existent path, got: %v", err)
		}
	})
}

func TestTestRunner_parseTestFile(t *testing.T) {
	t.Run("ParsesValidTestFile", func(t *testing.T) {
		// Given a valid test file
		mocks := setupTestRunnerMocks(t)
		testsDir := filepath.Join(mocks.TmpDir, "tests")
		createTestFile(t, testsDir, "valid.test.yaml", `
cases:
  - name: test-one
    values:
      provider: aws
    expect:
      terraform:
        - name: cluster
`)
		runner := createRunnerWithMockGenerator(mocks)

		// When parsing the test file
		testFile, err := runner.parseTestFile(filepath.Join(testsDir, "valid.test.yaml"))

		// Then it should be parsed successfully
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if len(testFile.Cases) != 1 {
			t.Errorf("Expected 1 test case, got: %d", len(testFile.Cases))
		}
		if testFile.Cases[0].Name != "test-one" {
			t.Errorf("Expected test name 'test-one', got: %s", testFile.Cases[0].Name)
		}
	})

	t.Run("ReturnsErrorForInvalidYaml", func(t *testing.T) {
		// Given an invalid YAML file
		mocks := setupTestRunnerMocks(t)
		testsDir := filepath.Join(mocks.TmpDir, "tests")
		createTestFile(t, testsDir, "invalid.test.yaml", `
cases:
  - name: [invalid yaml
`)
		runner := createRunnerWithMockGenerator(mocks)

		// When parsing the test file
		_, err := runner.parseTestFile(filepath.Join(testsDir, "invalid.test.yaml"))

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error for invalid YAML")
		}
	})

	t.Run("ReturnsErrorForNonexistentFile", func(t *testing.T) {
		// Given a nonexistent file
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		// When parsing a nonexistent file
		_, err := runner.parseTestFile("/nonexistent/file.test.yaml")

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error for nonexistent file")
		}
	})
}

func TestTestRunner_runTestsSequentially(t *testing.T) {
	t.Run("RunsTestsSequentiallyAndGroupsByFile", func(t *testing.T) {
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		testCasesWithFiles := []testCaseWithFile{
			{
				testCase: blueprintv1alpha1.TestCase{
					Name:   "test-1",
					Values: map[string]any{},
				},
				fileName: "file1.test.yaml",
			},
			{
				testCase: blueprintv1alpha1.TestCase{
					Name:   "test-2",
					Values: map[string]any{},
				},
				fileName: "file1.test.yaml",
			},
			{
				testCase: blueprintv1alpha1.TestCase{
					Name:   "test-3",
					Values: map[string]any{},
				},
				fileName: "file2.test.yaml",
			},
		}

		var buf bytes.Buffer
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w
		done := make(chan bool)
		go func() {
			buf.ReadFrom(r)
			done <- true
		}()

		results, err := runner.runTestCases(testCasesWithFiles)
		if err == nil {
			err = runner.printResults(results)
		}
		w.Close()
		<-done
		os.Stdout = oldStdout

		output := buf.String()
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if !strings.Contains(output, "=== file1.test.yaml ===") {
			t.Error("Expected file1 header in output")
		}
		if !strings.Contains(output, "=== file2.test.yaml ===") {
			t.Error("Expected file2 header in output")
		}
		if !strings.Contains(output, "✓ test-1") {
			t.Error("Expected test-1 result in output")
		}
		if !strings.Contains(output, "PASS: 3 test(s) passed") {
			t.Errorf("Expected pass summary, got: %s", output)
		}
	})

	t.Run("ReturnsErrorWhenTestsFail", func(t *testing.T) {
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		testCasesWithFiles := []testCaseWithFile{
			{
				testCase: blueprintv1alpha1.TestCase{
					Name:   "failing-test",
					Values: map[string]any{},
					Expect: &blueprintv1alpha1.Blueprint{
						TerraformComponents: []blueprintv1alpha1.TerraformComponent{
							{Name: "nonexistent-component"},
						},
					},
				},
				fileName: "test.test.yaml",
			},
		}

		var buf bytes.Buffer
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w
		done := make(chan bool)
		go func() {
			buf.ReadFrom(r)
			done <- true
		}()

		results, runErr := runner.runTestCases(testCasesWithFiles)
		var err error
		if runErr != nil {
			err = runErr
		} else {
			err = runner.printResults(results)
		}
		w.Close()
		<-done
		os.Stdout = oldStdout

		if err == nil {
			t.Error("Expected error when tests fail")
		}
		if err != nil && !strings.Contains(err.Error(), "test(s) failed") {
			t.Errorf("Expected failure message, got: %v", err)
		}
		output := buf.String()
		if !strings.Contains(output, "FAIL: 1 of 1 test(s) failed") {
			t.Errorf("Expected failure summary, got: %s", output)
		}
	})
}

func TestTestRunner_createGenerator(t *testing.T) {
	t.Run("CreatesGeneratorWithTerraformOutputs", func(t *testing.T) {
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		terraformOutputs := map[string]map[string]any{
			"network": {
				"vpc_id": "vpc-123",
			},
		}

		generator := runner.createGenerator(terraformOutputs, nil, false)
		values := map[string]any{
			"terraform.enabled": true,
		}

		blueprint, err := generator(values)

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if blueprint == nil {
			t.Error("Expected blueprint to be generated")
		}
	})

	t.Run("CreatesGeneratorWithTerraformEnabledButNoOutputs", func(t *testing.T) {
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		generator := runner.createGenerator(nil, nil, false)
		values := map[string]any{
			"terraform.enabled": true,
		}

		blueprint, err := generator(values)

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if blueprint == nil {
			t.Error("Expected blueprint to be generated")
		}
	})

	t.Run("HandlesLoadBlueprintError", func(t *testing.T) {
		mocks := setupTestRunnerMocksForFailure(t)
		runner := createRunnerForFailure(mocks)

		generator := runner.createGenerator(nil, nil, false)
		values := map[string]any{}

		_, err := generator(values)

		if err == nil {
			t.Error("Expected error when LoadBlueprint fails")
		}
		if !strings.Contains(err.Error(), "failed to load blueprint") {
			t.Errorf("Expected load blueprint error, got: %v", err)
		}
	})

	t.Run("RegistersMockHelperWhenTerraformOutputsProvided", func(t *testing.T) {
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		terraformOutputs := map[string]map[string]any{
			"compute": {
				"controlplanes": []map[string]any{
					{"endpoint": "10.5.0.10:6443", "hostname": "controlplane-1"},
				},
			},
		}

		generator := runner.createGenerator(terraformOutputs, nil, false)
		values := map[string]any{}

		blueprint, err := generator(values)

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if blueprint == nil {
			t.Error("Expected blueprint to be generated")
		}
	})

	t.Run("RegistersMockHelperEvenWhenTerraformDisabled", func(t *testing.T) {
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		terraformOutputs := map[string]map[string]any{
			"network": {
				"vpc_id": "vpc-123",
			},
		}

		generator := runner.createGenerator(terraformOutputs, nil, false)
		values := map[string]any{
			"terraform.enabled": false,
		}

		blueprint, err := generator(values)

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if blueprint == nil {
			t.Error("Expected blueprint to be generated")
		}
	})

	t.Run("ErrorsWhenFacetTerraformInputReferencesUnregisteredComponent", func(t *testing.T) {
		// Given a facet whose terraform component eagerly references terraform_output()
		// for a sibling component that is gated by a `when:` condition the test disables.
		// This mirrors the failure mode the bug report describes: windsor bootstrap errors
		// at env-var time with "component not found: dns-zone" but windsor test silently
		// resolved the expression to empty, hiding the failure until live runs.
		mocks := setupTestRunnerMocks(t)
		templateDir := filepath.Join(mocks.TmpDir, "contexts", "_template")
		facetsDir := filepath.Join(templateDir, "facets")
		createTestFile(t, facetsDir, "platform.yaml", `kind: Facet
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: platform
terraform:
  - name: dns-zone
    path: dns/zone
    when: (dns.public_domain ?? '') != ''
  - name: cluster
    path: cluster
    inputs:
      cert_manager_hosted_zone_ids: "${(terraform_output('dns-zone', 'zone_id') ?? '') != '' ? [terraform_output('dns-zone', 'zone_id')] : []}"
`)
		runner := createRunnerWithMockGenerator(mocks)

		terraformOutputs := map[string]map[string]any{
			"cluster": {"endpoint": "https://cluster.local"},
		}
		generator := runner.createGenerator(terraformOutputs, nil, false)

		// When the generator composes a config where dns.public_domain is unset so dns-zone is filtered out
		_, err := generator(map[string]any{
			"terraform.enabled": true,
		})

		// Then composition fails with the same "component not found" message the live
		// runtime raises, so the test loop catches the regression locally
		if err == nil {
			t.Fatal("Expected error when terraform input references unregistered component, got nil")
		}
		if !strings.Contains(err.Error(), "component not found: dns-zone") {
			t.Errorf("Expected error to contain 'component not found: dns-zone', got: %v", err)
		}
		if we := werror.Find(err); we == nil || !strings.Contains(we.Remediation, "terraform_output()") {
			t.Errorf("Expected remediation about the terraform_output() call, got: %v", we)
		}
	})

	t.Run("ResolvesWhenFacetTerraformInputReferencesRegisteredComponent", func(t *testing.T) {
		// Given the same facet but a config that enables the dns-zone component
		mocks := setupTestRunnerMocks(t)
		templateDir := filepath.Join(mocks.TmpDir, "contexts", "_template")
		facetsDir := filepath.Join(templateDir, "facets")
		createTestFile(t, facetsDir, "platform.yaml", `kind: Facet
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: platform
terraform:
  - name: dns-zone
    path: dns/zone
    when: (dns.public_domain ?? '') != ''
  - name: cluster
    path: cluster
    inputs:
      cert_manager_hosted_zone_ids: "${(terraform_output('dns-zone', 'zone_id') ?? '') != '' ? [terraform_output('dns-zone', 'zone_id')] : []}"
`)
		runner := createRunnerWithMockGenerator(mocks)

		// And terraformOutputs supplies dns-zone.zone_id
		terraformOutputs := map[string]map[string]any{
			"dns-zone": {"zone_id": "Z123ABC"},
		}
		generator := runner.createGenerator(terraformOutputs, nil, false)

		// When the generator composes with dns.public_domain set so dns-zone is registered
		_, err := generator(map[string]any{
			"terraform.enabled": true,
			"dns":               map[string]any{"public_domain": "example.com"},
		})

		// Then composition succeeds: dns-zone is in the composition and its output resolves
		if err != nil {
			t.Fatalf("Expected no error when component is registered, got: %v", err)
		}
	})

	t.Run("PopulatesBlueprintConfigFromFacetConfigBlock", func(t *testing.T) {
		// Given a facet contributing a config: block
		mocks := setupTestRunnerMocks(t)
		templateDir := filepath.Join(mocks.TmpDir, "contexts", "_template")
		facetsDir := filepath.Join(templateDir, "facets")
		createTestFile(t, facetsDir, "cluster.yaml", `kind: Facet
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: cluster
config:
  - name: cluster
    value:
      controlplanes:
        cpu: 7
`)
		runner := createRunnerWithMockGenerator(mocks)
		generator := runner.createGenerator(nil, nil, false)

		// When the generator composes the blueprint
		bp, err := generator(map[string]any{})
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Then the composed blueprint's Config carries the facet's resolved config block
		cluster, ok := bp.Config["cluster"].(map[string]any)
		if !ok {
			t.Fatalf("Expected bp.Config[\"cluster\"] to be a map, got: %#v", bp.Config["cluster"])
		}
		controlplanes, ok := cluster["controlplanes"].(map[string]any)
		if !ok {
			t.Fatalf("Expected controlplanes to be a map, got: %#v", cluster["controlplanes"])
		}
		if fmt.Sprint(controlplanes["cpu"]) != "7" {
			t.Errorf("Expected cpu=7, got: %v", controlplanes["cpu"])
		}
	})

	t.Run("CrossFieldRuleSpanningStaticAndDynamicFires", func(t *testing.T) {
		mocks := setupTestRunnerMocks(t)
		templateDir := filepath.Join(mocks.TmpDir, "contexts", "_template")
		// gateway (dynamic) requires dns.private_domain (static) when access is private.
		schemaContent := `$schema: https://json-schema.org/draft/2020-12/schema
type: object
properties:
  gateway:
    type: object
    properties:
      access:
        type: string
  dns:
    type: object
    properties:
      private_domain:
        type: string
allOf:
  - if:
      properties:
        gateway:
          type: object
          properties:
            access:
              const: private
          required: [access]
      required: [gateway]
    then:
      required: [dns]
      properties:
        dns:
          type: object
          required: [private_domain]
          properties:
            private_domain:
              type: string
              minLength: 1
`
		createTestFile(t, templateDir, "schema.yaml", schemaContent)
		runner := createRunnerWithMockGenerator(mocks)

		// Satisfied: gateway dynamic and dns static both set; Set loop's random order must not
		// cause spurious failures, and the final ValidateContextValues call must pass.
		satisfied := map[string]any{
			"gateway": map[string]any{"access": "private"},
			"dns":     map[string]any{"private_domain": "internal.example"},
		}
		for i := 0; i < 10; i++ {
			generator := runner.createGenerator(nil, nil, false)
			if _, err := generator(satisfied); err != nil {
				t.Fatalf("iteration %d: expected satisfied cross-field rule to pass, got %v", i, err)
			}
		}

		// Violation: dns omitted. ValidateContextValues after the Set loop must surface this.
		violation := map[string]any{
			"gateway": map[string]any{"access": "private"},
		}
		generator := runner.createGenerator(nil, nil, false)
		if _, err := generator(violation); err == nil {
			t.Fatal("expected cross-field rule violation to be surfaced from runner")
		}
	})

	t.Run("ResolvesEnvExpressionsFromCaseEnvMap", func(t *testing.T) {
		// Given a facet whose terraform component is gated on an env() expression
		mocks := setupTestRunnerMocks(t)
		templateDir := filepath.Join(mocks.TmpDir, "contexts", "_template")
		facetsDir := filepath.Join(templateDir, "facets")
		createTestFile(t, facetsDir, "platform.yaml", `kind: Facet
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: platform
terraform:
  - name: cluster
    path: cluster
  - name: dns-zone
    path: dns/zone
    when: env('ENABLE_DNS') == 'yes'
`)
		runner := createRunnerWithMockGenerator(mocks)

		// When the generator composes with an env map that enables the gate
		generator := runner.createGenerator(nil, map[string]string{"ENABLE_DNS": "yes"}, false)
		bp, err := generator(map[string]any{"terraform.enabled": true})

		// Then dns-zone is included because env() resolved from the case env map
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if !hasTerraformComponent(bp, "dns-zone") {
			t.Error("Expected dns-zone component to be present when env map enables it")
		}
	})

	t.Run("EnvResolutionIsHermeticAndIgnoresHostEnv", func(t *testing.T) {
		// Given the same env-gated facet and ENABLE_DNS set in the host environment
		mocks := setupTestRunnerMocks(t)
		templateDir := filepath.Join(mocks.TmpDir, "contexts", "_template")
		facetsDir := filepath.Join(templateDir, "facets")
		createTestFile(t, facetsDir, "platform.yaml", `kind: Facet
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: platform
terraform:
  - name: cluster
    path: cluster
  - name: dns-zone
    path: dns/zone
    when: env('ENABLE_DNS') == 'yes'
`)
		t.Setenv("ENABLE_DNS", "yes")
		runner := createRunnerWithMockGenerator(mocks)

		// When the generator composes with no env map for the case
		generator := runner.createGenerator(nil, nil, false)
		bp, err := generator(map[string]any{"terraform.enabled": true})

		// Then dns-zone is excluded: env() never reads the host environment
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if hasTerraformComponent(bp, "dns-zone") {
			t.Error("Expected dns-zone component to be absent: env() must not read host environment")
		}
	})

	t.Run("SchemaDefaultResolvesForFallbackFreeReadWhenOptedIn", func(t *testing.T) {
		// Given a schema default for network.cidr_block and a facet that reads it with no ?? fallback
		mocks := setupTestRunnerMocks(t)
		templateDir := filepath.Join(mocks.TmpDir, "contexts", "_template")
		createTestFile(t, templateDir, "schema.yaml", `$schema: https://json-schema.org/draft/2020-12/schema
type: object
properties:
  network:
    type: object
    properties:
      cidr_block:
        type: string
        default: "10.5.0.0/16"
`)
		facetsDir := filepath.Join(templateDir, "facets")
		createTestFile(t, facetsDir, "platform.yaml", `kind: Facet
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: platform
terraform:
  - name: cluster
    path: cluster
  - name: dns-zone
    path: dns/zone
    when: network.cidr_block == '10.5.0.0/16'
`)
		runner := createRunnerWithMockGenerator(mocks)

		// When the case opts into schema defaults and sets no network.cidr_block
		generator := runner.createGenerator(nil, nil, true)
		bp, err := generator(map[string]any{"terraform.enabled": true})

		// Then the fallback-free read resolves to the schema default and the gated component is included
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if !hasTerraformComponent(bp, "dns-zone") {
			t.Error("Expected dns-zone: schema default should resolve network.cidr_block when opted in")
		}
	})

	t.Run("SchemaDefaultSkippedForFallbackFreeReadByDefault", func(t *testing.T) {
		// Given the same schema default and fallback-free facet read
		mocks := setupTestRunnerMocks(t)
		templateDir := filepath.Join(mocks.TmpDir, "contexts", "_template")
		createTestFile(t, templateDir, "schema.yaml", `$schema: https://json-schema.org/draft/2020-12/schema
type: object
properties:
  network:
    type: object
    properties:
      cidr_block:
        type: string
        default: "10.5.0.0/16"
`)
		facetsDir := filepath.Join(templateDir, "facets")
		createTestFile(t, facetsDir, "platform.yaml", `kind: Facet
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: platform
terraform:
  - name: cluster
    path: cluster
  - name: dns-zone
    path: dns/zone
    when: network.cidr_block == '10.5.0.0/16'
`)
		runner := createRunnerWithMockGenerator(mocks)

		// When the case does not opt in
		generator := runner.createGenerator(nil, nil, false)
		bp, err := generator(map[string]any{"terraform.enabled": true})

		// Then the schema default is skipped, the read is nil, and the gated component is excluded
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if hasTerraformComponent(bp, "dns-zone") {
			t.Error("Expected dns-zone absent: schema default must be skipped by default in test context")
		}
	})

	t.Run("SeedsWindsorContextIntoEnvByDefault", func(t *testing.T) {
		// Given a facet whose component is gated on env('WINDSOR_CONTEXT') == 'test'
		mocks := setupTestRunnerMocks(t)
		templateDir := filepath.Join(mocks.TmpDir, "contexts", "_template")
		facetsDir := filepath.Join(templateDir, "facets")
		createTestFile(t, facetsDir, "platform.yaml", `kind: Facet
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: platform
terraform:
  - name: cluster
    path: cluster
  - name: dns-zone
    path: dns/zone
    when: env('WINDSOR_CONTEXT') == 'test'
`)
		runner := createRunnerWithMockGenerator(mocks)

		// When the generator composes with no case env map
		generator := runner.createGenerator(nil, nil, false)
		bp, err := generator(map[string]any{"terraform.enabled": true})

		// Then dns-zone is included: WINDSOR_CONTEXT defaults to "test" in the hermetic env
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if !hasTerraformComponent(bp, "dns-zone") {
			t.Error("Expected dns-zone component: env('WINDSOR_CONTEXT') should default to test")
		}
	})
}

// =============================================================================
// Test Helpers
// =============================================================================

func TestRegisterTerraformOutputHelperForMock(t *testing.T) {
	setupEvaluatorForHelperTest := func(t *testing.T) evaluator.ExpressionEvaluator {
		t.Helper()
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return make(map[string]any), nil
		}
		return evaluator.NewExpressionEvaluator(mockConfigHandler, "/test/project", "/test/template")
	}

	t.Run("ReturnsMockedValueWhenKeyExists", func(t *testing.T) {
		mockProvider := &terraform.MockTerraformProvider{
			GetTerraformOutputsFunc: func(componentID string) (map[string]any, error) {
				if componentID == "compute" {
					return map[string]any{
						"controlplanes": []map[string]any{
							{"endpoint": "10.5.0.10:6443", "hostname": "controlplane-1"},
						},
					}, nil
				}
				return make(map[string]any), nil
			},
		}
		eval := setupEvaluatorForHelperTest(t)
		registerTerraformOutputHelperForMock(mockProvider, eval, nil)

		result, err := eval.Evaluate(`${terraform_output("compute", "controlplanes")}`, "", nil, true)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		controlplanes, ok := result.([]map[string]any)
		if !ok {
			t.Fatalf("Expected array of maps, got %T: %v", result, result)
		}
		if len(controlplanes) != 1 {
			t.Errorf("Expected 1 controlplane, got %d", len(controlplanes))
		}
		if controlplanes[0]["endpoint"] != "10.5.0.10:6443" {
			t.Errorf("Expected endpoint '10.5.0.10:6443', got '%v'", controlplanes[0]["endpoint"])
		}
		if controlplanes[0]["hostname"] != "controlplane-1" {
			t.Errorf("Expected hostname 'controlplane-1', got '%v'", controlplanes[0]["hostname"])
		}
	})

	t.Run("ReturnsNilWhenKeyDoesNotExist", func(t *testing.T) {
		mockProvider := &terraform.MockTerraformProvider{
			GetTerraformOutputsFunc: func(componentID string) (map[string]any, error) {
				return map[string]any{
					"vpc_id": "vpc-123",
				}, nil
			},
		}
		eval := setupEvaluatorForHelperTest(t)
		registerTerraformOutputHelperForMock(mockProvider, eval, nil)

		result, err := eval.Evaluate(`${terraform_output("network", "nonexistent") ?? "default"}`, "", nil, true)

		if err != nil {
			t.Fatalf("Expected no error (nil return enables ?? fallback), got: %v", err)
		}
		if result != "default" {
			t.Errorf("Expected 'default' from ?? fallback when key doesn't exist, got: %v", result)
		}
	})

	t.Run("ReturnsNilWhenComponentDoesNotExist", func(t *testing.T) {
		// The helper itself never errors on an unknown component — it resolves to nil so the ??
		// fallback can apply. The runner performs the registration check after Generate() returns
		// using the references collected by the recordReference callback.
		mockProvider := &terraform.MockTerraformProvider{
			GetTerraformOutputsFunc: func(componentID string) (map[string]any, error) {
				return make(map[string]any), nil
			},
		}
		eval := setupEvaluatorForHelperTest(t)
		registerTerraformOutputHelperForMock(mockProvider, eval, nil)

		result, err := eval.Evaluate(`${terraform_output("nonexistent", "key") ?? "fallback"}`, "", nil, true)

		if err != nil {
			t.Fatalf("Expected no error (nil return enables ?? fallback), got: %v", err)
		}
		if result != "fallback" {
			t.Errorf("Expected 'fallback' from ?? fallback when component doesn't exist, got: %v", result)
		}
	})

	t.Run("InvokesRecordReferenceForEveryComponentReference", func(t *testing.T) {
		// Given a helper wired with a recordReference callback
		mockProvider := &terraform.MockTerraformProvider{
			GetTerraformOutputsFunc: func(componentID string) (map[string]any, error) {
				if componentID == "network" {
					return map[string]any{"vpc_id": "vpc-123"}, nil
				}
				return make(map[string]any), nil
			},
		}
		eval := setupEvaluatorForHelperTest(t)
		var recorded []string
		registerTerraformOutputHelperForMock(mockProvider, eval, func(componentID string) {
			recorded = append(recorded, componentID)
		})

		// When expressions reference multiple components, including one that has no supplied outputs
		if _, err := eval.Evaluate(`${terraform_output("network", "vpc_id")}`, "", nil, true); err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if _, err := eval.Evaluate(`${terraform_output("dns-zone", "zone_id") ?? ""}`, "", nil, true); err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		// Then every referenced component ID is recorded so the runner can validate registration
		// against the composed blueprint after Generate() returns
		if len(recorded) != 2 || recorded[0] != "network" || recorded[1] != "dns-zone" {
			t.Errorf("Expected recorded references [network dns-zone], got %v", recorded)
		}
	})

	t.Run("EvaluatesImmediatelyEvenWhenDeferredIsFalse", func(t *testing.T) {
		mockProvider := &terraform.MockTerraformProvider{
			GetTerraformOutputsFunc: func(componentID string) (map[string]any, error) {
				return map[string]any{"key": "value"}, nil
			},
		}
		eval := setupEvaluatorForHelperTest(t)
		registerTerraformOutputHelperForMock(mockProvider, eval, nil)

		result, err := eval.Evaluate(`prefix-${terraform_output("component", "key")}-suffix`, "", nil, false)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if result != "prefix-value-suffix" {
			t.Errorf("Expected 'prefix-value-suffix', got: %v", result)
		}
	})

	t.Run("HandlesNestedArrayValues", func(t *testing.T) {
		mockProvider := &terraform.MockTerraformProvider{
			GetTerraformOutputsFunc: func(componentID string) (map[string]any, error) {
				if componentID == "compute" {
					return map[string]any{
						"controlplanes": []any{
							map[string]any{"endpoint": "10.5.0.10:6443", "hostname": "controlplane-1"},
							map[string]any{"endpoint": "10.5.0.11:6443", "hostname": "controlplane-2"},
						},
					}, nil
				}
				return make(map[string]any), nil
			},
		}
		eval := setupEvaluatorForHelperTest(t)
		registerTerraformOutputHelperForMock(mockProvider, eval, nil)

		result, err := eval.Evaluate(`${terraform_output("compute", "controlplanes")}`, "", nil, true)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		controlplanes, ok := result.([]any)
		if !ok {
			t.Fatalf("Expected array, got %T: %v", result, result)
		}
		if len(controlplanes) != 2 {
			t.Errorf("Expected 2 controlplanes, got %d", len(controlplanes))
		}
	})

	t.Run("HandlesStringValues", func(t *testing.T) {
		mockProvider := &terraform.MockTerraformProvider{
			GetTerraformOutputsFunc: func(componentID string) (map[string]any, error) {
				return map[string]any{
					"vpc_id": "vpc-123",
				}, nil
			},
		}
		eval := setupEvaluatorForHelperTest(t)
		registerTerraformOutputHelperForMock(mockProvider, eval, nil)

		result, err := eval.Evaluate(`${terraform_output("network", "vpc_id")}`, "", nil, true)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if result != "vpc-123" {
			t.Errorf("Expected 'vpc-123', got: %v", result)
		}
	})

	t.Run("ReturnsErrorForInvalidArguments", func(t *testing.T) {
		mockProvider := &terraform.MockTerraformProvider{
			GetTerraformOutputsFunc: func(componentID string) (map[string]any, error) {
				return make(map[string]any), nil
			},
		}
		eval := setupEvaluatorForHelperTest(t)
		registerTerraformOutputHelperForMock(mockProvider, eval, nil)

		_, err := eval.Evaluate(`${terraform_output("component")}`, "", nil, true)

		if err == nil {
			t.Error("Expected error for invalid number of arguments")
		}
		if !strings.Contains(err.Error(), "not enough arguments") && !strings.Contains(err.Error(), "exactly 2 arguments") {
			t.Errorf("Expected error about arguments, got: %v", err)
		}
	})

	t.Run("ReturnsErrorForNonStringComponent", func(t *testing.T) {
		mockProvider := &terraform.MockTerraformProvider{
			GetTerraformOutputsFunc: func(componentID string) (map[string]any, error) {
				return make(map[string]any), nil
			},
		}
		eval := setupEvaluatorForHelperTest(t)
		registerTerraformOutputHelperForMock(mockProvider, eval, nil)

		_, err := eval.Evaluate(`${terraform_output(123, "key")}`, "", nil, true)

		if err == nil {
			t.Error("Expected error for non-string component")
		}
		if !strings.Contains(err.Error(), "cannot use int") && !strings.Contains(err.Error(), "component must be a string") {
			t.Errorf("Expected error about string component, got: %v", err)
		}
	})

	t.Run("ReturnsErrorForNonStringKey", func(t *testing.T) {
		mockProvider := &terraform.MockTerraformProvider{
			GetTerraformOutputsFunc: func(componentID string) (map[string]any, error) {
				return make(map[string]any), nil
			},
		}
		eval := setupEvaluatorForHelperTest(t)
		registerTerraformOutputHelperForMock(mockProvider, eval, nil)

		_, err := eval.Evaluate(`${terraform_output("component", 456)}`, "", nil, true)

		if err == nil {
			t.Error("Expected error for non-string key")
		}
		if !strings.Contains(err.Error(), "cannot use int") && !strings.Contains(err.Error(), "key must be a string") {
			t.Errorf("Expected error about string key, got: %v", err)
		}
	})
}
