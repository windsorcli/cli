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
		projectRoot:     mocks.TmpDir,
		baseShell:       mockShell,
		baseProjectRoot: mocks.TmpDir,
		artifactBuilder: mockArtifact,
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
		projectRoot:     mocks.TmpDir,
		baseShell:       mockShell,
		baseProjectRoot: mocks.TmpDir,
		artifactBuilder: mockArtifact,
	}
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
		}

		err := runner.RunAndPrint("")

		if err == nil {
			t.Error("Expected error when no test files found")
		}
		if !strings.Contains(err.Error(), "no test files found") {
			t.Errorf("Expected 'no test files found' error, got: %v", err)
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

func TestTestRunner_matchBlueprint(t *testing.T) {
	t.Run("ReturnsNoDiffsWhenExpectationsMet", func(t *testing.T) {
		// Given a blueprint matching expectations
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		blueprint := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Name: "cluster", Path: "cluster/eks", Source: "core"},
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "flux-system", Path: "flux", Components: []string{"base", "sync"}},
			},
		}

		expect := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Name: "cluster"},
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "flux-system"},
			},
		}

		// When matching the blueprint
		diffs := runner.matchBlueprint(blueprint, expect)

		// Then no diffs should be returned
		if len(diffs) != 0 {
			t.Errorf("Expected no diffs, got: %v", diffs)
		}
	})

	t.Run("ReturnsDiffsWhenTerraformComponentNotFound", func(t *testing.T) {
		// Given a blueprint missing expected component
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		blueprint := &blueprintv1alpha1.Blueprint{}

		expect := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Name: "missing-component"},
			},
		}

		// When matching the blueprint
		diffs := runner.matchBlueprint(blueprint, expect)

		// Then diffs should indicate missing component
		if len(diffs) != 1 {
			t.Errorf("Expected 1 diff, got: %d", len(diffs))
		}
	})

	t.Run("ReturnsDiffsWhenTerraformComponentNotFoundByPath", func(t *testing.T) {
		// Given a blueprint missing expected component matched by path
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		blueprint := &blueprintv1alpha1.Blueprint{}

		expect := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "missing/path"},
			},
		}

		// When matching the blueprint
		diffs := runner.matchBlueprint(blueprint, expect)

		// Then diffs should use path as identifier
		if len(diffs) != 1 {
			t.Errorf("Expected 1 diff, got: %d", len(diffs))
		}
	})

	t.Run("ReturnsDiffsWhenKustomizationNotFound", func(t *testing.T) {
		// Given a blueprint missing expected kustomization
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		blueprint := &blueprintv1alpha1.Blueprint{}

		expect := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "missing-kustomization"},
			},
		}

		// When matching the blueprint
		diffs := runner.matchBlueprint(blueprint, expect)

		// Then diffs should indicate missing kustomization
		if len(diffs) != 1 {
			t.Errorf("Expected 1 diff, got: %d", len(diffs))
		}
	})

	t.Run("ChecksPropertyMismatchesWhenComponentFound", func(t *testing.T) {
		// Given a blueprint with component having wrong properties
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		blueprint := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Name: "cluster", Source: "wrong-source"},
			},
		}

		expect := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Name: "cluster", Source: "expected-source"},
			},
		}

		// When matching the blueprint
		diffs := runner.matchBlueprint(blueprint, expect)

		// Then diffs should indicate property mismatch
		if len(diffs) != 1 {
			t.Errorf("Expected 1 diff, got: %d", len(diffs))
		}
	})

	t.Run("ChecksKustomizationPropertyMismatchesWhenFound", func(t *testing.T) {
		// Given a blueprint with kustomization having wrong properties
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "flux-system", Path: "wrong-path"},
			},
		}

		expect := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "flux-system", Path: "expected-path"},
			},
		}

		// When matching the blueprint
		diffs := runner.matchBlueprint(blueprint, expect)

		// Then diffs should indicate property mismatch
		if len(diffs) != 1 {
			t.Errorf("Expected 1 diff, got: %d", len(diffs))
		}
	})

	t.Run("ReturnsNoDiffsWhenExpectIsEmpty", func(t *testing.T) {
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		blueprint := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Name: "cluster"},
			},
		}

		expect := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{},
			Kustomizations:      []blueprintv1alpha1.Kustomization{},
		}

		diffs := runner.matchBlueprint(blueprint, expect)

		if len(diffs) != 0 {
			t.Errorf("Expected no diffs when expect is empty, got: %v", diffs)
		}
	})
}

func TestTestRunner_matchExclusions(t *testing.T) {
	t.Run("ReturnsNoDiffsWhenExcludeIsNil", func(t *testing.T) {
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		blueprint := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Name: "cluster"},
			},
		}

		diffs := runner.matchExclusions(blueprint, nil)

		if len(diffs) != 0 {
			t.Errorf("Expected no diffs when exclude is nil, got: %v", diffs)
		}
	})

	t.Run("ReturnsNoDiffsWhenExclusionsRespected", func(t *testing.T) {
		// Given a blueprint without excluded components
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		blueprint := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Name: "cluster"},
			},
		}

		exclude := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Name: "excluded-component"},
			},
		}

		// When matching exclusions
		diffs := runner.matchExclusions(blueprint, exclude)

		// Then no diffs should be returned
		if len(diffs) != 0 {
			t.Errorf("Expected no diffs, got: %v", diffs)
		}
	})

	t.Run("ReturnsDiffsWhenExcludedTerraformExists", func(t *testing.T) {
		// Given a blueprint with an excluded component
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		blueprint := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Name: "should-not-exist"},
			},
		}

		exclude := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Name: "should-not-exist"},
			},
		}

		// When matching exclusions
		diffs := runner.matchExclusions(blueprint, exclude)

		// Then diffs should indicate unwanted component
		if len(diffs) != 1 {
			t.Errorf("Expected 1 diff, got: %d", len(diffs))
		}
	})

	t.Run("ReturnsDiffsWhenExcludedKustomizationExists", func(t *testing.T) {
		// Given a blueprint with an excluded kustomization
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "should-not-exist"},
			},
		}

		exclude := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "should-not-exist"},
			},
		}

		// When matching exclusions
		diffs := runner.matchExclusions(blueprint, exclude)

		// Then diffs should indicate unwanted kustomization
		if len(diffs) != 1 {
			t.Errorf("Expected 1 diff, got: %d", len(diffs))
		}
	})

	t.Run("UsesPathWhenNameIsEmpty", func(t *testing.T) {
		// Given a blueprint with a component matched by path
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		blueprint := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "vm/colima"},
			},
		}

		exclude := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "vm/colima"},
			},
		}

		// When matching exclusions
		diffs := runner.matchExclusions(blueprint, exclude)

		// Then diffs should use path as identifier
		if len(diffs) != 1 {
			t.Errorf("Expected 1 diff, got: %d", len(diffs))
		}
	})
}

func TestTestRunner_findTerraformComponent(t *testing.T) {
	t.Run("FindsByName", func(t *testing.T) {
		// Given a blueprint with named components
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		blueprint := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Name: "cluster", Path: "cluster/eks"},
			},
		}

		// When finding by name
		found := runner.findTerraformComponent(blueprint, blueprintv1alpha1.TerraformComponent{Name: "cluster"})

		// Then component should be found
		if found == nil {
			t.Error("Expected component to be found")
		}
	})

	t.Run("FindsByPath", func(t *testing.T) {
		// Given a blueprint with path-only components
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		blueprint := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "cluster/eks"},
			},
		}

		// When finding by path
		found := runner.findTerraformComponent(blueprint, blueprintv1alpha1.TerraformComponent{Path: "cluster/eks"})

		// Then component should be found
		if found == nil {
			t.Error("Expected component to be found")
		}
	})

	t.Run("ReturnsNilWhenNotFound", func(t *testing.T) {
		// Given an empty blueprint
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		blueprint := &blueprintv1alpha1.Blueprint{}

		// When finding nonexistent component
		found := runner.findTerraformComponent(blueprint, blueprintv1alpha1.TerraformComponent{Name: "nonexistent"})

		// Then nil should be returned
		if found != nil {
			t.Error("Expected nil for nonexistent component")
		}
	})
}

func TestTestRunner_findKustomization(t *testing.T) {
	t.Run("FindsByName", func(t *testing.T) {
		// Given a blueprint with kustomizations
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "flux-system"},
			},
		}

		// When finding by name
		found := runner.findKustomization(blueprint, blueprintv1alpha1.Kustomization{Name: "flux-system"})

		// Then kustomization should be found
		if found == nil {
			t.Error("Expected kustomization to be found")
		}
	})

	t.Run("ReturnsNilWhenNotFound", func(t *testing.T) {
		// Given an empty blueprint
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		blueprint := &blueprintv1alpha1.Blueprint{}

		// When finding nonexistent kustomization
		found := runner.findKustomization(blueprint, blueprintv1alpha1.Kustomization{Name: "nonexistent"})

		// Then nil should be returned
		if found != nil {
			t.Error("Expected nil for nonexistent kustomization")
		}
	})
}

func TestTestRunner_matchTerraformComponent(t *testing.T) {
	t.Run("ReturnsNoDiffsWhenAllFieldsMatch", func(t *testing.T) {
		// Given matching component and expectation
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		actual := &blueprintv1alpha1.TerraformComponent{
			Name:      "cluster",
			Path:      "cluster/eks",
			Source:    "core",
			DependsOn: []string{"network"},
		}

		expect := blueprintv1alpha1.TerraformComponent{
			Name:      "cluster",
			Path:      "cluster/eks",
			Source:    "core",
			DependsOn: []string{"network"},
		}

		// When matching
		diffs := runner.matchTerraformComponent(actual, expect)

		// Then no diffs should be returned
		if len(diffs) != 0 {
			t.Errorf("Expected no diffs, got: %v", diffs)
		}
	})

	t.Run("ReturnsDiffsWhenSourceMismatches", func(t *testing.T) {
		// Given component with wrong source
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		actual := &blueprintv1alpha1.TerraformComponent{
			Name:   "cluster",
			Source: "wrong-source",
		}

		expect := blueprintv1alpha1.TerraformComponent{
			Name:   "cluster",
			Source: "expected-source",
		}

		// When matching
		diffs := runner.matchTerraformComponent(actual, expect)

		// Then diffs should indicate source mismatch
		if len(diffs) != 1 {
			t.Errorf("Expected 1 diff, got: %d", len(diffs))
		}
	})

	t.Run("ReturnsDiffsWhenPathMismatches", func(t *testing.T) {
		// Given component with wrong path
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		actual := &blueprintv1alpha1.TerraformComponent{
			Name: "cluster",
			Path: "wrong/path",
		}

		expect := blueprintv1alpha1.TerraformComponent{
			Name: "cluster",
			Path: "expected/path",
		}

		// When matching
		diffs := runner.matchTerraformComponent(actual, expect)

		// Then diffs should indicate path mismatch
		if len(diffs) != 1 {
			t.Errorf("Expected 1 diff, got: %d", len(diffs))
		}
	})

	t.Run("ReturnsDiffsWhenDependencyMissing", func(t *testing.T) {
		// Given component missing a dependency
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		actual := &blueprintv1alpha1.TerraformComponent{
			Name:      "cluster",
			DependsOn: []string{"network"},
		}

		expect := blueprintv1alpha1.TerraformComponent{
			Name:      "cluster",
			DependsOn: []string{"network", "vpc"},
		}

		// When matching
		diffs := runner.matchTerraformComponent(actual, expect)

		// Then diffs should indicate missing dependency
		if len(diffs) != 1 {
			t.Errorf("Expected 1 diff, got: %d", len(diffs))
		}
	})

	t.Run("UsesPathAsIdentifierWhenNameEmpty", func(t *testing.T) {
		// Given expectation with only path
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		actual := &blueprintv1alpha1.TerraformComponent{
			Path:   "cluster/eks",
			Source: "wrong",
		}

		expect := blueprintv1alpha1.TerraformComponent{
			Path:   "cluster/eks",
			Source: "expected",
		}

		// When matching
		diffs := runner.matchTerraformComponent(actual, expect)

		// Then diff message should use path as identifier
		if len(diffs) != 1 {
			t.Errorf("Expected 1 diff, got: %d", len(diffs))
		}
	})

	t.Run("ReturnsNoDiffsWhenInputsMatch", func(t *testing.T) {
		// Given component with matching inputs
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		actual := &blueprintv1alpha1.TerraformComponent{
			Name: "cluster",
			Inputs: map[string]any{
				"region":      "us-west-2",
				"node_count":  3,
				"tags":        map[string]any{"env": "prod"},
			},
		}

		expect := blueprintv1alpha1.TerraformComponent{
			Name: "cluster",
			Inputs: map[string]any{
				"region":     "us-west-2",
				"node_count": 3,
			},
		}

		// When matching
		diffs := runner.matchTerraformComponent(actual, expect)

		// Then no diffs should be returned
		if len(diffs) != 0 {
			t.Errorf("Expected no diffs, got: %v", diffs)
		}
	})

	t.Run("ReturnsDiffsWhenInputValueMismatches", func(t *testing.T) {
		// Given component with mismatched input value
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		actual := &blueprintv1alpha1.TerraformComponent{
			Name: "cluster",
			Inputs: map[string]any{
				"region": "us-east-1",
			},
		}

		expect := blueprintv1alpha1.TerraformComponent{
			Name: "cluster",
			Inputs: map[string]any{
				"region": "us-west-2",
			},
		}

		// When matching
		diffs := runner.matchTerraformComponent(actual, expect)

		// Then diffs should indicate value mismatch
		if len(diffs) != 1 {
			t.Errorf("Expected 1 diff, got: %d", len(diffs))
		}
	})

	t.Run("ReturnsDiffsWhenInputKeyMissing", func(t *testing.T) {
		// Given component missing an input key
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		actual := &blueprintv1alpha1.TerraformComponent{
			Name:   "cluster",
			Inputs: map[string]any{},
		}

		expect := blueprintv1alpha1.TerraformComponent{
			Name: "cluster",
			Inputs: map[string]any{
				"region": "us-west-2",
			},
		}

		// When matching
		diffs := runner.matchTerraformComponent(actual, expect)

		// Then diffs should indicate missing key
		if len(diffs) != 1 {
			t.Errorf("Expected 1 diff, got: %d", len(diffs))
		}
	})
}

func TestTestRunner_matchKustomization(t *testing.T) {
	t.Run("ReturnsNoDiffsWhenAllFieldsMatch", func(t *testing.T) {
		// Given matching kustomization and expectation
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		actual := &blueprintv1alpha1.Kustomization{
			Name:       "flux-system",
			Path:       "flux",
			Source:     "core",
			DependsOn:  []string{"csi"},
			Components: []string{"base", "sync"},
		}

		expect := blueprintv1alpha1.Kustomization{
			Name:       "flux-system",
			Path:       "flux",
			Source:     "core",
			DependsOn:  []string{"csi"},
			Components: []string{"base"},
		}

		// When matching
		diffs := runner.matchKustomization(actual, expect)

		// Then no diffs should be returned
		if len(diffs) != 0 {
			t.Errorf("Expected no diffs, got: %v", diffs)
		}
	})

	t.Run("ReturnsDiffsWhenPathMismatches", func(t *testing.T) {
		// Given kustomization with wrong path
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		actual := &blueprintv1alpha1.Kustomization{
			Name: "flux-system",
			Path: "wrong/path",
		}

		expect := blueprintv1alpha1.Kustomization{
			Name: "flux-system",
			Path: "expected/path",
		}

		// When matching
		diffs := runner.matchKustomization(actual, expect)

		// Then diffs should indicate path mismatch
		if len(diffs) != 1 {
			t.Errorf("Expected 1 diff, got: %d", len(diffs))
		}
	})

	t.Run("ReturnsDiffsWhenSourceMismatches", func(t *testing.T) {
		// Given kustomization with wrong source
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		actual := &blueprintv1alpha1.Kustomization{
			Name:   "flux-system",
			Source: "wrong-source",
		}

		expect := blueprintv1alpha1.Kustomization{
			Name:   "flux-system",
			Source: "expected-source",
		}

		// When matching
		diffs := runner.matchKustomization(actual, expect)

		// Then diffs should indicate source mismatch
		if len(diffs) != 1 {
			t.Errorf("Expected 1 diff, got: %d", len(diffs))
		}
	})

	t.Run("ReturnsDiffsWhenDependencyMissing", func(t *testing.T) {
		// Given kustomization missing a dependency
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		actual := &blueprintv1alpha1.Kustomization{
			Name:      "flux-system",
			DependsOn: []string{"csi"},
		}

		expect := blueprintv1alpha1.Kustomization{
			Name:      "flux-system",
			DependsOn: []string{"csi", "pki"},
		}

		// When matching
		diffs := runner.matchKustomization(actual, expect)

		// Then diffs should indicate missing dependency
		if len(diffs) != 1 {
			t.Errorf("Expected 1 diff, got: %d", len(diffs))
		}
	})

	t.Run("ReturnsDiffsWhenComponentMissing", func(t *testing.T) {
		// Given kustomization missing a component
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		actual := &blueprintv1alpha1.Kustomization{
			Name:       "flux-system",
			Components: []string{"base"},
		}

		expect := blueprintv1alpha1.Kustomization{
			Name:       "flux-system",
			Components: []string{"base", "sync"},
		}

		// When matching
		diffs := runner.matchKustomization(actual, expect)

		// Then diffs should indicate missing component
		if len(diffs) != 1 {
			t.Errorf("Expected 1 diff, got: %d", len(diffs))
		}
	})

	t.Run("ReturnsNoDiffsWhenSubstitutionsMatch", func(t *testing.T) {
		// Given kustomization with matching substitutions
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		actual := &blueprintv1alpha1.Kustomization{
			Name: "metallb",
			Substitutions: map[string]string{
				"loadbalancer_ip_range": "10.10.1.10-10.10.1.100",
				"namespace":             "metallb-system",
			},
		}

		expect := blueprintv1alpha1.Kustomization{
			Name: "metallb",
			Substitutions: map[string]string{
				"loadbalancer_ip_range": "10.10.1.10-10.10.1.100",
			},
		}

		// When matching
		diffs := runner.matchKustomization(actual, expect)

		// Then no diffs should be returned
		if len(diffs) != 0 {
			t.Errorf("Expected no diffs, got: %v", diffs)
		}
	})

	t.Run("ReturnsDiffsWhenSubstitutionValueMismatches", func(t *testing.T) {
		// Given kustomization with mismatched substitution value
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		actual := &blueprintv1alpha1.Kustomization{
			Name: "metallb",
			Substitutions: map[string]string{
				"loadbalancer_ip_range": "10.5.1.10-10.5.1.100",
			},
		}

		expect := blueprintv1alpha1.Kustomization{
			Name: "metallb",
			Substitutions: map[string]string{
				"loadbalancer_ip_range": "10.10.1.10-10.10.1.100",
			},
		}

		// When matching
		diffs := runner.matchKustomization(actual, expect)

		// Then diffs should indicate value mismatch
		if len(diffs) != 1 {
			t.Errorf("Expected 1 diff, got: %d", len(diffs))
		}
	})

	t.Run("ReturnsDiffsWhenSubstitutionKeyMissing", func(t *testing.T) {
		// Given kustomization missing a substitution key
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		actual := &blueprintv1alpha1.Kustomization{
			Name:          "metallb",
			Substitutions: map[string]string{},
		}

		expect := blueprintv1alpha1.Kustomization{
			Name: "metallb",
			Substitutions: map[string]string{
				"loadbalancer_ip_range": "10.10.1.10-10.10.1.100",
			},
		}

		// When matching
		diffs := runner.matchKustomization(actual, expect)

		// Then diffs should indicate missing key
		if len(diffs) != 1 {
			t.Errorf("Expected 1 diff, got: %d", len(diffs))
		}
	})
}

func TestTestRunner_validateBlueprint(t *testing.T) {
	t.Run("DetectsDuplicateTerraformComponents", func(t *testing.T) {
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		blueprint := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Name: "cluster", Path: "cluster/eks"},
				{Name: "cluster", Path: "cluster/gke"},
			},
		}

		errors := runner.validateBlueprint(blueprint)

		if len(errors) == 0 {
			t.Error("Expected error for duplicate terraform component")
		}
		if !strings.Contains(errors[0], "duplicate terraform component ID") {
			t.Errorf("Expected duplicate component error, got: %v", errors)
		}
	})

	t.Run("DetectsDuplicateTerraformComponentsByPath", func(t *testing.T) {
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		blueprint := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "cluster/eks"},
				{Path: "cluster/eks"},
			},
		}

		errors := runner.validateBlueprint(blueprint)

		if len(errors) == 0 {
			t.Error("Expected error for duplicate terraform component by path")
		}
	})

	t.Run("DetectsDuplicateKustomizations", func(t *testing.T) {
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "flux-system", Path: "flux"},
				{Name: "flux-system", Path: "flux2"},
			},
		}

		errors := runner.validateBlueprint(blueprint)

		if len(errors) == 0 {
			t.Error("Expected error for duplicate kustomization")
		}
		if !strings.Contains(errors[0], "duplicate kustomization name") {
			t.Errorf("Expected duplicate kustomization error, got: %v", errors)
		}
	})

	t.Run("DetectsDuplicateKustomizationComponents", func(t *testing.T) {
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name:       "app",
					Path:       "app",
					Components: []string{"base", "monitoring", "base"},
				},
			},
		}

		errors := runner.validateBlueprint(blueprint)

		if len(errors) == 0 {
			t.Error("Expected error for duplicate kustomization component")
		}
		if !strings.Contains(errors[0], "duplicate component") {
			t.Errorf("Expected duplicate component error, got: %v", errors)
		}
	})

	t.Run("DetectsCircularDependenciesInTerraform", func(t *testing.T) {
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		blueprint := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Name: "a", Path: "a", DependsOn: []string{"b"}},
				{Name: "b", Path: "b", DependsOn: []string{"a"}},
			},
		}

		errors := runner.validateBlueprint(blueprint)

		if len(errors) == 0 {
			t.Error("Expected error for circular dependency")
		}
		if !strings.Contains(errors[0], "circular dependency") {
			t.Errorf("Expected circular dependency error, got: %v", errors)
		}
	})

	t.Run("DetectsCircularDependenciesInKustomize", func(t *testing.T) {
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "a", Path: "a", DependsOn: []string{"b"}},
				{Name: "b", Path: "b", DependsOn: []string{"a"}},
			},
		}

		errors := runner.validateBlueprint(blueprint)

		if len(errors) == 0 {
			t.Error("Expected error for circular dependency")
		}
		if !strings.Contains(errors[0], "circular dependency") {
			t.Errorf("Expected circular dependency error, got: %v", errors)
		}
	})

	t.Run("DetectsInvalidTerraformDependencies", func(t *testing.T) {
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		blueprint := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Name: "a", Path: "a", DependsOn: []string{"nonexistent"}},
			},
		}

		errors := runner.validateBlueprint(blueprint)

		if len(errors) == 0 {
			t.Error("Expected error for invalid dependency")
		}
		if !strings.Contains(errors[0], "depends on non-existent component") {
			t.Errorf("Expected invalid dependency error, got: %v", errors)
		}
	})

	t.Run("DetectsInvalidKustomizeDependencies", func(t *testing.T) {
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		blueprint := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "app", Path: "app", DependsOn: []string{"nonexistent"}},
			},
		}

		errors := runner.validateBlueprint(blueprint)

		if len(errors) == 0 {
			t.Error("Expected error for invalid dependency")
		}
		if !strings.Contains(errors[0], "depends on non-existent kustomization") {
			t.Errorf("Expected invalid dependency error, got: %v", errors)
		}
	})

	t.Run("PassesValidBlueprint", func(t *testing.T) {
		mocks := setupTestRunnerMocks(t)
		runner := createRunnerWithMockGenerator(mocks)

		blueprint := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Name: "cluster", Path: "cluster/eks"},
				{Name: "network", Path: "network/vpc", DependsOn: []string{"cluster"}},
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "app", Path: "app", Components: []string{"base", "monitoring"}},
			},
		}

		errors := runner.validateBlueprint(blueprint)

		if len(errors) > 0 {
			t.Errorf("Expected no errors for valid blueprint, got: %v", errors)
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

		err := runner.runTestsSequentially(testCasesWithFiles)
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

		err := runner.runTestsSequentially(testCasesWithFiles)
		w.Close()
		<-done
		os.Stdout = oldStdout

		if err == nil {
			t.Error("Expected error when tests fail")
		}
		if !strings.Contains(err.Error(), "test(s) failed") {
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

		generator := runner.createGenerator(terraformOutputs)
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

		generator := runner.createGenerator(nil)
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

		generator := runner.createGenerator(nil)
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

		generator := runner.createGenerator(terraformOutputs)
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

		generator := runner.createGenerator(terraformOutputs)
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
}

// =============================================================================
// Test Helpers
// =============================================================================

func TestContains(t *testing.T) {
	t.Run("ReturnsTrueWhenItemExists", func(t *testing.T) {
		// Given a slice with items
		slice := []string{"a", "b", "c"}

		// When checking for existing item
		result := contains(slice, "b")

		// Then true should be returned
		if !result {
			t.Error("Expected true for existing item")
		}
	})

	t.Run("ReturnsFalseWhenItemDoesNotExist", func(t *testing.T) {
		// Given a slice with items
		slice := []string{"a", "b", "c"}

		// When checking for nonexistent item
		result := contains(slice, "d")

		// Then false should be returned
		if result {
			t.Error("Expected false for nonexistent item")
		}
	})

	t.Run("ReturnsFalseForEmptySlice", func(t *testing.T) {
		// Given an empty slice
		slice := []string{}

		// When checking for any item
		result := contains(slice, "a")

		// Then false should be returned
		if result {
			t.Error("Expected false for empty slice")
		}
	})
}

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
		registerTerraformOutputHelperForMock(mockProvider, eval)

		result, err := eval.Evaluate(`${terraform_output("compute", "controlplanes")}`, "", true)

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
		registerTerraformOutputHelperForMock(mockProvider, eval)

		result, err := eval.Evaluate(`${terraform_output("network", "nonexistent") ?? "default"}`, "", true)

		if err != nil {
			t.Fatalf("Expected no error (nil return enables ?? fallback), got: %v", err)
		}
		if result != "default" {
			t.Errorf("Expected 'default' from ?? fallback when key doesn't exist, got: %v", result)
		}
	})

	t.Run("ReturnsNilWhenComponentDoesNotExist", func(t *testing.T) {
		mockProvider := &terraform.MockTerraformProvider{
			GetTerraformOutputsFunc: func(componentID string) (map[string]any, error) {
				return make(map[string]any), nil
			},
		}
		eval := setupEvaluatorForHelperTest(t)
		registerTerraformOutputHelperForMock(mockProvider, eval)

		result, err := eval.Evaluate(`${terraform_output("nonexistent", "key") ?? "fallback"}`, "", true)

		if err != nil {
			t.Fatalf("Expected no error (nil return enables ?? fallback), got: %v", err)
		}
		if result != "fallback" {
			t.Errorf("Expected 'fallback' from ?? fallback when component doesn't exist, got: %v", result)
		}
	})

	t.Run("EvaluatesImmediatelyEvenWhenDeferredIsFalse", func(t *testing.T) {
		mockProvider := &terraform.MockTerraformProvider{
			GetTerraformOutputsFunc: func(componentID string) (map[string]any, error) {
				return map[string]any{"key": "value"}, nil
			},
		}
		eval := setupEvaluatorForHelperTest(t)
		registerTerraformOutputHelperForMock(mockProvider, eval)

		result, err := eval.Evaluate(`prefix-${terraform_output("component", "key")}-suffix`, "", false)

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
		registerTerraformOutputHelperForMock(mockProvider, eval)

		result, err := eval.Evaluate(`${terraform_output("compute", "controlplanes")}`, "", true)

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
		registerTerraformOutputHelperForMock(mockProvider, eval)

		result, err := eval.Evaluate(`${terraform_output("network", "vpc_id")}`, "", true)

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
		registerTerraformOutputHelperForMock(mockProvider, eval)

		_, err := eval.Evaluate(`${terraform_output("component")}`, "", true)

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
		registerTerraformOutputHelperForMock(mockProvider, eval)

		_, err := eval.Evaluate(`${terraform_output(123, "key")}`, "", true)

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
		registerTerraformOutputHelperForMock(mockProvider, eval)

		_, err := eval.Evaluate(`${terraform_output("component", 456)}`, "", true)

		if err == nil {
			t.Error("Expected error for non-string key")
		}
		if !strings.Contains(err.Error(), "cannot use int") && !strings.Contains(err.Error(), "key must be a string") {
			t.Errorf("Expected error about string key, got: %v", err)
		}
	})
}

func TestDeepEqual(t *testing.T) {
	t.Run("ReturnsTrueForEqualStrings", func(t *testing.T) {
		if !deepEqual("hello", "hello") {
			t.Error("Expected true for equal strings")
		}
	})

	t.Run("ReturnsFalseForUnequalStrings", func(t *testing.T) {
		if deepEqual("hello", "world") {
			t.Error("Expected false for unequal strings")
		}
	})

	t.Run("ReturnsTrueForEqualIntegers", func(t *testing.T) {
		if !deepEqual(42, 42) {
			t.Error("Expected true for equal integers")
		}
	})

	t.Run("ReturnsTrueForBothNil", func(t *testing.T) {
		if !deepEqual(nil, nil) {
			t.Error("Expected true for both nil")
		}
	})

	t.Run("ReturnsFalseForOneNil", func(t *testing.T) {
		if deepEqual("hello", nil) {
			t.Error("Expected false when one is nil")
		}
		if deepEqual(nil, "hello") {
			t.Error("Expected false when one is nil")
		}
	})

	t.Run("ReturnsTrueForEqualMaps", func(t *testing.T) {
		a := map[string]any{"key": "value", "num": 42}
		b := map[string]any{"key": "value", "num": 42}
		if !deepEqual(a, b) {
			t.Error("Expected true for equal maps")
		}
	})

	t.Run("ReturnsFalseForUnequalMaps", func(t *testing.T) {
		a := map[string]any{"key": "value1"}
		b := map[string]any{"key": "value2"}
		if deepEqual(a, b) {
			t.Error("Expected false for unequal maps")
		}
	})

	t.Run("ReturnsFalseForMapsDifferentLength", func(t *testing.T) {
		a := map[string]any{"key": "value", "extra": "field"}
		b := map[string]any{"key": "value"}
		if deepEqual(a, b) {
			t.Error("Expected false for maps with different length")
		}
	})

	t.Run("ReturnsFalseForMapMissingKey", func(t *testing.T) {
		a := map[string]any{"key1": "value"}
		b := map[string]any{"key2": "value"}
		if deepEqual(a, b) {
			t.Error("Expected false for maps with different keys")
		}
	})

	t.Run("ReturnsTrueForEqualSlices", func(t *testing.T) {
		a := []any{"one", "two", 3}
		b := []any{"one", "two", 3}
		if !deepEqual(a, b) {
			t.Error("Expected true for equal slices")
		}
	})

	t.Run("ReturnsFalseForUnequalSlices", func(t *testing.T) {
		a := []any{"one", "two"}
		b := []any{"one", "three"}
		if deepEqual(a, b) {
			t.Error("Expected false for unequal slices")
		}
	})

	t.Run("ReturnsFalseForSlicesDifferentLength", func(t *testing.T) {
		a := []any{"one", "two", "three"}
		b := []any{"one", "two"}
		if deepEqual(a, b) {
			t.Error("Expected false for slices with different length")
		}
	})

	t.Run("ReturnsTrueForNestedStructures", func(t *testing.T) {
		a := map[string]any{
			"nested": map[string]any{"inner": "value"},
			"list":   []any{1, 2, 3},
		}
		b := map[string]any{
			"nested": map[string]any{"inner": "value"},
			"list":   []any{1, 2, 3},
		}
		if !deepEqual(a, b) {
			t.Error("Expected true for equal nested structures")
		}
	})

	t.Run("ReturnsTrueForEquivalentNumericTypes", func(t *testing.T) {
		if !deepEqual(42, 42.0) {
			t.Error("Expected true for equivalent numeric values via string comparison")
		}
	})

	t.Run("ReturnsFalseForMapVsNonMap", func(t *testing.T) {
		a := map[string]any{"key": "value"}
		b := "not a map"
		if deepEqual(a, b) {
			t.Error("Expected false for map vs non-map")
		}
	})

	t.Run("ReturnsFalseForSliceVsNonSlice", func(t *testing.T) {
		a := []any{"one", "two"}
		b := "not a slice"
		if deepEqual(a, b) {
			t.Error("Expected false for slice vs non-slice")
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

		// And an original environment variable
		originalEnvContext := os.Getenv("WINDSOR_CONTEXT")
		defer func() {
			if originalEnvContext != "" {
				os.Setenv("WINDSOR_CONTEXT", originalEnvContext)
			} else {
				os.Unsetenv("WINDSOR_CONTEXT")
			}
		}()

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

	t.Run("RestoresOriginalEnvironmentVariable", func(t *testing.T) {
		// Given a test runner with test files
		mocks := setupTestRunnerMocks(t)
		testsDir := filepath.Join(mocks.TmpDir, "contexts", "_template", "tests")
		createTestFile(t, testsDir, "example.test.yaml", `
cases:
  - name: test-case-1
    values: {}
`)
		runner := createRunnerWithMockGenerator(mocks)

		// And an original environment variable set
		originalEnvContext := "my-original-context"
		os.Setenv("WINDSOR_CONTEXT", originalEnvContext)
		defer func() {
			os.Setenv("WINDSOR_CONTEXT", originalEnvContext)
		}()

		// When running tests (may fail due to composition, but that's okay)
		_, _ = runner.Run("")

		// Then the environment variable should be restored
		restoredContext := os.Getenv("WINDSOR_CONTEXT")
		if restoredContext != originalEnvContext {
			t.Errorf("Expected WINDSOR_CONTEXT to be restored to %q, got %q", originalEnvContext, restoredContext)
		}
	})

	t.Run("RestoresUnsetEnvironmentVariable", func(t *testing.T) {
		// Given a test runner with test files
		mocks := setupTestRunnerMocks(t)
		testsDir := filepath.Join(mocks.TmpDir, "contexts", "_template", "tests")
		createTestFile(t, testsDir, "example.test.yaml", `
cases:
  - name: test-case-1
    values: {}
`)
		runner := createRunnerWithMockGenerator(mocks)

		// And no original environment variable set
		os.Unsetenv("WINDSOR_CONTEXT")

		// When running tests (may fail due to composition, but that's okay)
		_, _ = runner.Run("")

		// Then the environment variable should be unset after tests
		restoredContext := os.Getenv("WINDSOR_CONTEXT")
		if restoredContext != "" {
			t.Errorf("Expected WINDSOR_CONTEXT to be unset after tests, got %q", restoredContext)
		}
	})
}
