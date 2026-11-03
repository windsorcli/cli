package test_test

import (
	"fmt"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/test"

	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/composer/artifact"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/shell"
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

func createRunnerWithMockGenerator(mocks *TestRunnerMocks) *test.TestRunner {
	mockShell := shell.NewMockShell()
	mockShell.GetProjectRootFunc = func() (string, error) {
		return mocks.TmpDir, nil
	}
	mockArtifact := artifact.NewMockArtifact()
	runner := test.NewTestRunner(&runtime.Runtime{Shell: mockShell, ProjectRoot: mocks.TmpDir}, mockArtifact)
	runner.DefaultBlueprintURL = test.SkipDefaultBlueprintURL
	return runner
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
		if runner.ProjectRoot() != mocks.TmpDir {
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
		runner := test.NewTestRunner(rt, mockArtifact)

		// Then runner should be created with correct fields
		if runner == nil {
			t.Fatal("Expected TestRunner to be created")
		}
		if runner.ProjectRoot() != tmpDir {
			t.Errorf("Expected projectRoot to be %q, got %q", tmpDir, runner.ProjectRoot())
		}
		if runner.BaseProjectRoot() != tmpDir {
			t.Errorf("Expected baseProjectRoot to be %q, got %q", tmpDir, runner.BaseProjectRoot())
		}
		if runner.BaseShell() != mockShell {
			t.Error("Expected baseShell to be set")
		}
		if runner.ArtifactBuilder() != mockArtifact {
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
		runner.RunFunc = func(filter string) ([]test.TestResult, error) {
			calledFilter = filter
			return []test.TestResult{{Name: "test", Passed: true}}, nil
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

		runner.RunFunc = func(filter string) ([]test.TestResult, error) {
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
		runner := test.NewTestRunner(&runtime.Runtime{Shell: mockShell, ProjectRoot: tmpDir}, mockArtifact)

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

		runner.RunFunc = func(filter string) ([]test.TestResult, error) {
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

		runner.RunFunc = func(filter string) ([]test.TestResult, error) {
			return []test.TestResult{
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

		runner.RunFunc = func(filter string) ([]test.TestResult, error) {
			return []test.TestResult{
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

		runner.RunFunc = func(filter string) ([]test.TestResult, error) {
			return []test.TestResult{}, nil
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
		runner := test.NewTestRunner(&runtime.Runtime{Shell: mockShell, ProjectRoot: tmpDir}, mockArtifact)

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
		runner.RunFunc = func(filter string) ([]test.TestResult, error) {
			calledFilter = filter
			return []test.TestResult{{Name: "test", Passed: true}}, nil
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

		runner.RunFunc = func(filter string) ([]test.TestResult, error) {
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
		runner := test.NewTestRunner(&runtime.Runtime{Shell: mockShell, ProjectRoot: tmpDir}, mockArtifact)

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
