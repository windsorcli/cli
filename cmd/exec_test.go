package cmd

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/context/config"
	"github.com/windsorcli/cli/pkg/context/env"
	"github.com/windsorcli/cli/pkg/context/shell"
	"github.com/windsorcli/cli/pkg/di"
)

func TestExecCmd(t *testing.T) {
	t.Cleanup(func() {
		rootCmd.SetContext(context.Background())
	})

	createTestCmd := func() *cobra.Command {
		return &cobra.Command{
			Use:          "exec [command]",
			Short:        "Execute a shell command with environment variables",
			Long:         "Execute a shell command with environment variables set for the application.",
			Args:         cobra.MinimumNArgs(1),
			SilenceUsage: true,
			RunE:         execCmd.RunE,
		}
	}

	setup := func(t *testing.T) (*bytes.Buffer, *bytes.Buffer) {
		t.Helper()
		stdout, stderr := captureOutput(t)
		rootCmd.SetOut(stdout)
		rootCmd.SetErr(stderr)
		return stdout, stderr
	}

	t.Run("Success", func(t *testing.T) {
		setup(t)
		mocks := setupMocks(t)
		mocks.Shell.ExecFunc = func(command string, args ...string) (string, error) {
			return "", nil
		}
		cmd := createTestCmd()
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		cmd.SetContext(ctx)

		args := []string{"go", "version"}
		cmd.SetArgs(args)

		err := cmd.Execute()

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("SuccessWithMultipleArgs", func(t *testing.T) {
		setup(t)
		mocks := setupMocks(t)
		var capturedCommand string
		var capturedArgs []string
		mocks.Shell.ExecFunc = func(command string, args ...string) (string, error) {
			capturedCommand = command
			capturedArgs = args
			return "", nil
		}
		cmd := createTestCmd()
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		cmd.SetContext(ctx)

		args := []string{"test-command", "arg1", "arg2"}
		cmd.SetArgs(args)

		err := cmd.Execute()

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if capturedCommand != "test-command" {
			t.Errorf("Expected command to be 'test-command', got %v", capturedCommand)
		}
		if len(capturedArgs) != 2 || capturedArgs[0] != "arg1" || capturedArgs[1] != "arg2" {
			t.Errorf("Expected args to be ['arg1', 'arg2'], got %v", capturedArgs)
		}
	})

	t.Run("NoCommandProvided", func(t *testing.T) {
		setup(t)
		mocks := setupMocks(t)
		cmd := createTestCmd()
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		cmd.SetContext(ctx)

		args := []string{}
		cmd.SetArgs(args)

		err := cmd.Execute()

		if err == nil {
			t.Error("Expected error, got nil")
		}

		expectedError := "requires at least 1 arg(s), only received 0"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("SuccessWithVerbose", func(t *testing.T) {
		setup(t)
		mocks := setupMocks(t)
		mocks.Shell.ExecFunc = func(command string, args ...string) (string, error) {
			return "", nil
		}
		cmd := createTestCmd()
		cmd.Flags().Bool("verbose", false, "Show verbose output")
		cmd.Flags().Set("verbose", "true")
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		cmd.SetContext(ctx)

		args := []string{"go", "version"}
		cmd.SetArgs(args)

		err := cmd.Execute()

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}

// =============================================================================
// Test Error Scenarios
// =============================================================================

func TestExecCmd_ErrorScenarios(t *testing.T) {
	t.Cleanup(func() {
		rootCmd.SetContext(context.Background())
	})

	setup := func(t *testing.T) (*bytes.Buffer, *bytes.Buffer) {
		t.Helper()
		stdout, stderr := captureOutput(t)
		rootCmd.SetOut(stdout)
		rootCmd.SetErr(stderr)
		return stdout, stderr
	}

	t.Run("HandlesNewContextError", func(t *testing.T) {
		setup(t)
		injector := di.NewInjector()
		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("project root error")
		}
		mockShell.InitializeFunc = func() error {
			return nil
		}
		injector.Register("shell", mockShell)
		ctx := context.WithValue(context.Background(), injectorKey, injector)
		rootCmd.SetContext(ctx)
		t.Cleanup(func() {
			rootCmd.SetContext(context.Background())
			rootCmd.SetArgs([]string{})
			verbose = false
		})

		rootCmd.SetArgs([]string{"exec", "go", "version"})

		err := Execute()

		if err == nil {
			t.Error("Expected error when NewContext fails")
		}

		if !strings.Contains(err.Error(), "failed to initialize context") {
			t.Errorf("Expected error about context initialization, got: %v", err)
		}
	})

	t.Run("HandlesCheckTrustedDirectoryError", func(t *testing.T) {
		setup(t)
		// Reset context and verbose before setting up test
		rootCmd.SetContext(context.Background())
		verbose = false
		mocks := setupMocks(t)
		mocks.Shell.CheckTrustedDirectoryFunc = func() error {
			return fmt.Errorf("not trusted")
		}
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		rootCmd.SetContext(ctx)
		t.Cleanup(func() {
			rootCmd.SetContext(context.Background())
			rootCmd.SetArgs([]string{})
			verbose = false
		})

		rootCmd.SetArgs([]string{"exec", "go", "version"})

		err := Execute()

		if err == nil {
			t.Error("Expected error when CheckTrustedDirectory fails")
		}

		if !strings.Contains(err.Error(), "not in a trusted directory") {
			t.Errorf("Expected error about trusted directory, got: %v", err)
		}
	})

	t.Run("HandlesHandleSessionResetError", func(t *testing.T) {
		setup(t)
		// Reset context and verbose before setting up test
		rootCmd.SetContext(context.Background())
		verbose = false
		mocks := setupMocks(t)
		mocks.Shell.CheckResetFlagsFunc = func() (bool, error) {
			return false, fmt.Errorf("reset check failed")
		}
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		rootCmd.SetContext(ctx)
		t.Cleanup(func() {
			rootCmd.SetContext(context.Background())
			rootCmd.SetArgs([]string{})
			verbose = false
		})

		rootCmd.SetArgs([]string{"exec", "go", "version"})

		err := Execute()

		if err == nil {
			t.Error("Expected error when HandleSessionReset fails")
		}

		if !strings.Contains(err.Error(), "failed to check reset flags") {
			t.Errorf("Expected error about reset flags, got: %v", err)
		}
	})

	t.Run("HandlesLoadConfigError", func(t *testing.T) {
		setup(t)
		// Reset context and verbose before setting up test
		rootCmd.SetContext(context.Background())
		verbose = false
		injector := di.NewInjector()
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.LoadConfigFunc = func() error {
			return fmt.Errorf("config load failed")
		}
		mockConfigHandler.InitializeFunc = func() error {
			return nil
		}
		mockConfigHandler.GetContextFunc = func() string {
			return "test-context"
		}
		injector.Register("configHandler", mockConfigHandler)

		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) {
			return t.TempDir(), nil
		}
		mockShell.CheckTrustedDirectoryFunc = func() error {
			return nil
		}
		mockShell.CheckResetFlagsFunc = func() (bool, error) {
			return false, nil
		}
		mockShell.InitializeFunc = func() error {
			return nil
		}
		injector.Register("shell", mockShell)

		ctx := context.WithValue(context.Background(), injectorKey, injector)
		rootCmd.SetContext(ctx)
		t.Cleanup(func() {
			rootCmd.SetContext(context.Background())
			rootCmd.SetArgs([]string{})
			verbose = false
		})

		rootCmd.SetArgs([]string{"exec", "go", "version"})

		err := Execute()

		if err == nil {
			t.Error("Expected error when LoadConfig fails")
		}

		if !strings.Contains(err.Error(), "config load failed") && !strings.Contains(err.Error(), "failed to load config") {
			t.Errorf("Expected error about config loading, got: %v", err)
		}
	})

	t.Run("HandlesLoadEnvironmentError", func(t *testing.T) {
		setup(t)
		// Reset context and verbose before setting up test
		rootCmd.SetContext(context.Background())
		verbose = false
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.LoadConfigFunc = func() error {
			return nil
		}
		mockConfigHandler.InitializeFunc = func() error {
			return nil
		}
		mockConfigHandler.GetContextFunc = func() string {
			return "test-context"
		}
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "docker.enabled" {
				return true
			}
			return false
		}
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			return ""
		}
		mocks := setupMocks(t, &SetupOptions{ConfigHandler: mockConfigHandler})

		mockDockerEnvPrinter := env.NewMockEnvPrinter()
		mockDockerEnvPrinter.InitializeFunc = func() error {
			return nil
		}
		mockDockerEnvPrinter.GetEnvVarsFunc = func() (map[string]string, error) {
			return nil, fmt.Errorf("failed to get env vars")
		}
		mockDockerEnvPrinter.GetAliasFunc = func() (map[string]string, error) {
			return make(map[string]string), nil
		}
		mocks.Injector.Register("dockerEnvPrinter", mockDockerEnvPrinter)

		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		rootCmd.SetContext(ctx)
		t.Cleanup(func() {
			rootCmd.SetContext(context.Background())
			rootCmd.SetArgs([]string{})
			verbose = false
		})

		rootCmd.SetArgs([]string{"exec", "go", "version"})

		err := Execute()

		if err != nil && !strings.Contains(err.Error(), "failed to load environment") {
			t.Errorf("Expected error about environment loading, got: %v", err)
		}
	})

	t.Run("HandlesExecutePostEnvHooksErrorWithVerbose", func(t *testing.T) {
		setup(t)
		// Reset context and verbose before setting up test
		rootCmd.SetContext(context.Background())
		verbose = false
		mocks := setupMocks(t)
		mockWindsorEnvPrinter := env.NewMockEnvPrinter()
		mockWindsorEnvPrinter.InitializeFunc = func() error {
			return nil
		}
		mockWindsorEnvPrinter.GetEnvVarsFunc = func() (map[string]string, error) {
			return make(map[string]string), nil
		}
		mockWindsorEnvPrinter.GetAliasFunc = func() (map[string]string, error) {
			return make(map[string]string), nil
		}
		mockWindsorEnvPrinter.PostEnvHookFunc = func(directory ...string) error {
			return fmt.Errorf("hook failed")
		}
		mocks.Injector.Register("windsorEnv", mockWindsorEnvPrinter)
		mocks.Shell.ExecFunc = func(command string, args ...string) (string, error) {
			return "", nil
		}

		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		rootCmd.SetContext(ctx)
		t.Cleanup(func() {
			rootCmd.SetContext(context.Background())
			rootCmd.SetArgs([]string{})
			verbose = false
		})

		rootCmd.SetArgs([]string{"exec", "--verbose", "go", "version"})

		err := Execute()

		if err != nil && !strings.Contains(err.Error(), "failed to execute post env hooks") && !strings.Contains(err.Error(), "failed to load environment") {
			t.Errorf("Expected error about post env hooks or environment loading, got: %v", err)
		}
	})

	t.Run("SwallowsExecutePostEnvHooksErrorWithoutVerbose", func(t *testing.T) {
		setup(t)
		// Reset context and verbose before setting up test
		rootCmd.SetContext(context.Background())
		verbose = false
		mocks := setupMocks(t)
		mockWindsorEnvPrinter := env.NewMockEnvPrinter()
		mockWindsorEnvPrinter.InitializeFunc = func() error {
			return nil
		}
		mockWindsorEnvPrinter.GetEnvVarsFunc = func() (map[string]string, error) {
			return make(map[string]string), nil
		}
		mockWindsorEnvPrinter.GetAliasFunc = func() (map[string]string, error) {
			return make(map[string]string), nil
		}
		mockWindsorEnvPrinter.PostEnvHookFunc = func(directory ...string) error {
			return fmt.Errorf("hook failed")
		}
		mocks.Injector.Register("windsorEnv", mockWindsorEnvPrinter)
		mocks.Shell.ExecFunc = func(command string, args ...string) (string, error) {
			return "", nil
		}

		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		rootCmd.SetContext(ctx)
		t.Cleanup(func() {
			rootCmd.SetContext(context.Background())
			rootCmd.SetArgs([]string{})
			verbose = false
		})

		rootCmd.SetArgs([]string{"exec", "go", "version"})

		err := Execute()

		if err != nil {
			t.Errorf("Expected no error when verbose is false, got: %v", err)
		}
	})

	t.Run("HandlesShellExecError", func(t *testing.T) {
		setup(t)
		// Reset context and verbose before setting up test
		rootCmd.SetContext(context.Background())
		verbose = false
		mocks := setupMocks(t)
		mocks.Shell.ExecFunc = func(command string, args ...string) (string, error) {
			return "", fmt.Errorf("command execution failed")
		}
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		rootCmd.SetContext(ctx)
		t.Cleanup(func() {
			rootCmd.SetContext(context.Background())
			rootCmd.SetArgs([]string{})
			verbose = false
		})

		rootCmd.SetArgs([]string{"exec", "go", "version"})

		err := Execute()

		if err == nil {
			t.Error("Expected error when Shell.Exec fails")
		}

		if !strings.Contains(err.Error(), "failed to execute command") {
			t.Errorf("Expected error about command execution, got: %v", err)
		}
	})
}
