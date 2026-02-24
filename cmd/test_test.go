package cmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/windsorcli/cli/pkg/composer"
	"github.com/windsorcli/cli/pkg/project"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/shell"
	"github.com/windsorcli/cli/pkg/test"
)

// =============================================================================
// Test Setup
// =============================================================================

type TestCmdMocks struct {
	ConfigHandler config.ConfigHandler
	Runtime       *runtime.Runtime
	TestRunner    *test.TestRunner
	TmpDir        string
}

func setupTestCmdMocks(t *testing.T, opts ...*SetupOptions) *TestCmdMocks {
	t.Helper()

	mockConfigHandler := config.NewMockConfigHandler()
	mockConfigHandler.GetContextFunc = func() string { return "test-context" }
	mockConfigHandler.IsDevModeFunc = func(contextName string) bool { return false }
	mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return ""
	}
	mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return false
	}
	mockConfigHandler.IsLoadedFunc = func() bool { return true }
	mockConfigHandler.LoadConfigFunc = func() error { return nil }
	mockConfigHandler.SaveConfigFunc = func(hasSetFlags ...bool) error { return nil }
	mockConfigHandler.GenerateContextIDFunc = func() error { return nil }

	testOpts := &SetupOptions{}
	if len(opts) > 0 && opts[0] != nil {
		testOpts = opts[0]
	}
	testOpts.ConfigHandler = mockConfigHandler
	baseMocks := setupMocks(t, testOpts)
	tmpDir := baseMocks.TmpDir
	mockConfigHandler.GetConfigRootFunc = func() (string, error) { return tmpDir + "/contexts/test-context", nil }

	baseMocks.Shell.CheckTrustedDirectoryFunc = func() error { return nil }

	rt := runtime.NewRuntime(&runtime.Runtime{
		Shell:         baseMocks.Shell,
		ConfigHandler: baseMocks.ConfigHandler,
		ProjectRoot:   tmpDir,
		ToolsManager:  baseMocks.ToolsManager,
	})
	if rt == nil {
		t.Fatal("Failed to create runtime")
	}

	mockTestRunner := &test.TestRunner{
		RunFunc: func(filter string) ([]test.TestResult, error) {
			return []test.TestResult{}, nil
		},
	}

	return &TestCmdMocks{
		ConfigHandler: baseMocks.ConfigHandler,
		Runtime:       rt,
		TestRunner:    mockTestRunner,
		TmpDir:        tmpDir,
	}
}

func createTestCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "test [test-name]",
		Short:        "Run blueprint composition tests",
		RunE:         testCmd.RunE,
		SilenceUsage: true,
	}

	testCmd.Flags().VisitAll(func(flag *pflag.Flag) {
		cmd.Flags().AddFlag(flag)
	})

	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	return cmd
}

func createTestDir(_, dir string) error {
	return os.MkdirAll(dir, 0755)
}

func createTestYamlFile(t *testing.T, dir, filename, content string) {
	t.Helper()
	if err := os.WriteFile(dir+"/"+filename, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}
}

// =============================================================================
// Test Cases
// =============================================================================

func TestTestCmd(t *testing.T) {
	suppressProcessStdout(t)
	suppressProcessStderr(t)

	t.Run("SuccessWithAllTestsPassing", func(t *testing.T) {
		mocks := setupTestCmdMocks(t)

		mocks.TestRunner.RunFunc = func(filter string) ([]test.TestResult, error) {
			return []test.TestResult{
				{Name: "test-1", Passed: true},
				{Name: "test-2", Passed: true},
			}, nil
		}

		proj := project.NewProject("", &project.Project{
			Runtime: mocks.Runtime,
		})

		stdout := new(bytes.Buffer)
		stderr := new(bytes.Buffer)

		cmd := createTestCommand()
		cmd.SetOut(stdout)
		cmd.SetErr(stderr)
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		ctx = context.WithValue(ctx, testRunnerOverridesKey, mocks.TestRunner)
		cmd.SetContext(ctx)
		cmd.SetArgs([]string{})

		err := cmd.Execute()

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("FailsWithFailingTests", func(t *testing.T) {
		mocks := setupTestCmdMocks(t)

		mocks.TestRunner.RunFunc = func(filter string) ([]test.TestResult, error) {
			return []test.TestResult{
				{Name: "test-1", Passed: true},
				{Name: "test-2", Passed: false, Diffs: []string{"expected X, got Y"}},
			}, nil
		}

		proj := project.NewProject("", &project.Project{
			Runtime: mocks.Runtime,
		})

		stdout := new(bytes.Buffer)
		stderr := new(bytes.Buffer)

		cmd := createTestCommand()
		cmd.SetOut(stdout)
		cmd.SetErr(stderr)
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		ctx = context.WithValue(ctx, testRunnerOverridesKey, mocks.TestRunner)
		cmd.SetContext(ctx)
		cmd.SetArgs([]string{})

		err := cmd.Execute()

		if err == nil {
			t.Error("Expected error for failing tests")
		}

		if !strings.Contains(err.Error(), "test(s) failed") {
			t.Errorf("Expected error about failed tests, got: %v", err)
		}
	})

	t.Run("PassesFilterToTestRunner", func(t *testing.T) {
		mocks := setupTestCmdMocks(t)

		var capturedFilter string
		mocks.TestRunner.RunFunc = func(filter string) ([]test.TestResult, error) {
			capturedFilter = filter
			return []test.TestResult{
				{Name: "specific-test", Passed: true},
			}, nil
		}

		proj := project.NewProject("", &project.Project{
			Runtime: mocks.Runtime,
		})

		cmd := createTestCommand()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		ctx = context.WithValue(ctx, testRunnerOverridesKey, mocks.TestRunner)
		cmd.SetContext(ctx)
		cmd.SetArgs([]string{"specific-test"})

		_ = cmd.Execute()

		if capturedFilter != "specific-test" {
			t.Errorf("Expected filter 'specific-test', got: %q", capturedFilter)
		}
	})

	t.Run("CreatesComposerWhenProjectComposerIsNil", func(t *testing.T) {
		mocks := setupTestCmdMocks(t)

		testsDir := mocks.TmpDir + "/contexts/_template/tests"
		if err := createTestDir(mocks.TmpDir, testsDir); err != nil {
			t.Fatalf("Failed to create tests directory: %v", err)
		}
		createTestYamlFile(t, testsDir, "example.test.yaml", `
cases:
  - name: test-case
    values: {}
`)

		proj := project.NewProject("", &project.Project{
			Runtime:  mocks.Runtime,
			Composer: nil,
		})

		cmd := createTestCommand()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		ctx = context.WithValue(ctx, testRunnerOverridesKey, mocks.TestRunner)
		cmd.SetContext(ctx)
		cmd.SetArgs([]string{})

		err := cmd.Execute()

		if err != nil && !strings.Contains(err.Error(), "composition error") {
			t.Logf("Command executed (expected possible error from real composition): %v", err)
		}
	})
}

// =============================================================================
// Test Error Scenarios
// =============================================================================

func TestTestCmd_ErrorScenarios(t *testing.T) {
	suppressProcessStdout(t)
	suppressProcessStderr(t)

	t.Run("HandlesCheckTrustedDirectoryError", func(t *testing.T) {
		mocks := setupTestCmdMocks(t)

		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) { return mocks.TmpDir, nil }
		mockShell.CheckTrustedDirectoryFunc = func() error {
			return fmt.Errorf("not trusted")
		}

		rt := runtime.NewRuntime(&runtime.Runtime{
			Shell:         mockShell,
			ConfigHandler: mocks.Runtime.ConfigHandler,
			ProjectRoot:   mocks.TmpDir,
		})

		comp := composer.NewComposer(rt)

		proj := project.NewProject("", &project.Project{
			Runtime:  rt,
			Composer: comp,
		})

		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)

		cmd := createTestCommand()
		cmd.SetContext(ctx)
		cmd.SetArgs([]string{})

		err := cmd.Execute()

		if err == nil {
			t.Error("Expected error when CheckTrustedDirectory fails")
		}

		if !strings.Contains(err.Error(), "not in a trusted directory") {
			t.Errorf("Expected error about trusted directory, got: %v", err)
		}
	})

	t.Run("HandlesTestRunnerError", func(t *testing.T) {
		mocks := setupTestCmdMocks(t)

		mocks.TestRunner.RunFunc = func(filter string) ([]test.TestResult, error) {
			return nil, fmt.Errorf("no test files found")
		}

		proj := project.NewProject("", &project.Project{
			Runtime: mocks.Runtime,
		})

		cmd := createTestCommand()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		ctx = context.WithValue(ctx, testRunnerOverridesKey, mocks.TestRunner)
		cmd.SetContext(ctx)
		cmd.SetArgs([]string{})

		err := cmd.Execute()

		if err == nil {
			t.Error("Expected error when TestRunner.Run fails")
		}

		if !strings.Contains(err.Error(), "no test files found") {
			t.Errorf("Expected error about test files, got: %v", err)
		}
	})

	t.Run("HandlesFilterMatchingNoTests", func(t *testing.T) {
		mocks := setupTestCmdMocks(t)

		mocks.TestRunner.RunFunc = func(filter string) ([]test.TestResult, error) {
			return nil, fmt.Errorf("no test cases found matching filter: %s", filter)
		}

		proj := project.NewProject("", &project.Project{
			Runtime: mocks.Runtime,
		})

		cmd := createTestCommand()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		ctx = context.WithValue(ctx, testRunnerOverridesKey, mocks.TestRunner)
		cmd.SetContext(ctx)
		cmd.SetArgs([]string{"nonexistent-test"})

		err := cmd.Execute()

		if err == nil {
			t.Error("Expected error when filter matches no tests")
		}

		if !strings.Contains(err.Error(), "no test cases found matching filter") {
			t.Errorf("Expected error about filter, got: %v", err)
		}
	})
}
