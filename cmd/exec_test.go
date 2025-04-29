package cmd

import (
	"bytes"
	"fmt"
	"testing"

	ctrl "github.com/windsorcli/cli/pkg/controller"
	"github.com/windsorcli/cli/pkg/env"
	"github.com/windsorcli/cli/pkg/secrets"
	"github.com/windsorcli/cli/pkg/shell"
)

func TestExecCmd(t *testing.T) {
	setup := func(t *testing.T, opts ...*SetupOptions) (*Mocks, *bytes.Buffer, *bytes.Buffer) {
		t.Helper()
		mocks := setupMocks(t, opts...)
		stdout, stderr := captureOutput(t)
		rootCmd.SetOut(stdout)
		rootCmd.SetErr(stderr)

		// Setup common mocks
		mockShell := shell.NewMockShell()
		mockShell.ExecFunc = func(command string, args ...string) (string, error) {
			return "command output", nil
		}
		mocks.Controller.ResolveShellFunc = func() shell.Shell {
			return mockShell
		}

		mockSecretsProvider := secrets.NewMockSecretsProvider(mocks.Injector)
		mockSecretsProvider.LoadSecretsFunc = func() error {
			return nil
		}
		mocks.Controller.ResolveAllSecretsProvidersFunc = func() []secrets.SecretsProvider {
			return []secrets.SecretsProvider{mockSecretsProvider}
		}

		mockEnvPrinter := env.NewMockEnvPrinter()
		mockEnvPrinter.GetEnvVarsFunc = func() (map[string]string, error) {
			return map[string]string{"TEST_VAR": "test_value"}, nil
		}
		mockEnvPrinter.PostEnvHookFunc = func() error {
			return nil
		}
		mocks.Controller.ResolveAllEnvPrintersFunc = func() []env.EnvPrinter {
			return []env.EnvPrinter{mockEnvPrinter}
		}

		return mocks, stdout, stderr
	}

	t.Run("Success", func(t *testing.T) {
		// Given a set of mocks with proper configuration
		mocks, _, stderr := setup(t, &SetupOptions{
			ConfigStr: `
contexts:
  default:
    tools:
      enabled: true`,
		})

		rootCmd.SetArgs([]string{"exec", "--", "test-command", "arg1"})

		// When executing the command
		err := Execute(mocks.Controller)

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}

		// And stderr should be empty
		if stderr.String() != "" {
			t.Error("Expected empty stderr")
		}
	})

	t.Run("NoCommand", func(t *testing.T) {
		// Given a set of mocks with proper configuration
		mocks, _, _ := setup(t)

		rootCmd.SetArgs([]string{"exec", "--"})

		// When executing the command without a command
		err := Execute(mocks.Controller)

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}

		// And error should contain usage message
		expectedError := "no command provided"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("InitializationError", func(t *testing.T) {
		// Given a set of mocks with initialization error
		mocks, _, _ := setup(t)

		// Mock controller to return initialization error
		mocks.Controller.InitializeWithRequirementsFunc = func(req ctrl.Requirements) error {
			return fmt.Errorf("initialization failed")
		}

		rootCmd.SetArgs([]string{"exec", "--", "test-command"})

		// When executing the command
		err := Execute(mocks.Controller)

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}

		// And error should contain initialization message
		expectedError := "Error initializing: initialization failed"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("LoadSecretsError", func(t *testing.T) {
		// Given a set of mocks with proper configuration
		mocks, _, _ := setup(t, &SetupOptions{
			ConfigStr: `
contexts:
  default:
    tools:
      enabled: true`,
		})

		// Override secrets provider to return error
		mockSecretsProvider := secrets.NewMockSecretsProvider(mocks.Injector)
		mockSecretsProvider.LoadSecretsFunc = func() error {
			return fmt.Errorf("secrets loading failed")
		}
		mocks.Controller.ResolveAllSecretsProvidersFunc = func() []secrets.SecretsProvider {
			return []secrets.SecretsProvider{mockSecretsProvider}
		}

		rootCmd.SetArgs([]string{"exec", "--", "test-command"})

		// When executing the command
		err := Execute(mocks.Controller)

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}

		// And error should contain secrets error message
		expectedError := "Error loading secrets: secrets loading failed"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("GetEnvVarsError", func(t *testing.T) {
		// Given a set of mocks with proper configuration
		mocks, _, _ := setup(t, &SetupOptions{
			ConfigStr: `
contexts:
  default:
    tools:
      enabled: true`,
		})

		// Override env printer to return error
		mockEnvPrinter := env.NewMockEnvPrinter()
		mockEnvPrinter.GetEnvVarsFunc = func() (map[string]string, error) {
			return nil, fmt.Errorf("env vars error")
		}
		mocks.Controller.ResolveAllEnvPrintersFunc = func() []env.EnvPrinter {
			return []env.EnvPrinter{mockEnvPrinter}
		}

		rootCmd.SetArgs([]string{"exec", "--", "test-command"})

		// When executing the command
		err := Execute(mocks.Controller)

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}

		// And error should contain env vars error message
		expectedError := "Error getting environment variables: env vars error"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("PostEnvHookError", func(t *testing.T) {
		// Given a set of mocks with proper configuration
		mocks, _, _ := setup(t, &SetupOptions{
			ConfigStr: `
contexts:
  default:
    tools:
      enabled: true`,
		})

		// Override env printer to return error in PostEnvHook
		mockEnvPrinter := env.NewMockEnvPrinter()
		mockEnvPrinter.GetEnvVarsFunc = func() (map[string]string, error) {
			return map[string]string{"TEST_VAR": "test_value"}, nil
		}
		mockEnvPrinter.PostEnvHookFunc = func() error {
			return fmt.Errorf("post env hook error")
		}
		mocks.Controller.ResolveAllEnvPrintersFunc = func() []env.EnvPrinter {
			return []env.EnvPrinter{mockEnvPrinter}
		}

		rootCmd.SetArgs([]string{"exec", "--", "test-command"})

		// When executing the command
		err := Execute(mocks.Controller)

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}

		// And error should contain post env hook error message
		expectedError := "Error executing PostEnvHook: post env hook error"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("NoShellFound", func(t *testing.T) {
		// Given a set of mocks with proper configuration
		mocks, _, _ := setup(t, &SetupOptions{
			ConfigStr: `
contexts:
  default:
    tools:
      enabled: true`,
		})

		// Override shell to return nil
		mocks.Controller.ResolveShellFunc = func() shell.Shell {
			return nil
		}

		rootCmd.SetArgs([]string{"exec", "--", "test-command"})

		// When executing the command
		err := Execute(mocks.Controller)

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}

		// And error should contain no shell message
		expectedError := "No shell found"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ExecError", func(t *testing.T) {
		// Given a set of mocks with proper configuration
		mocks, _, _ := setup(t, &SetupOptions{
			ConfigStr: `
contexts:
  default:
    tools:
      enabled: true`,
		})

		// Override shell to return exec error
		mockShell := shell.NewMockShell()
		mockShell.ExecFunc = func(command string, args ...string) (string, error) {
			return "", fmt.Errorf("exec failed")
		}
		mocks.Controller.ResolveShellFunc = func() shell.Shell {
			return mockShell
		}

		rootCmd.SetArgs([]string{"exec", "--", "test-command"})

		// When executing the command
		err := Execute(mocks.Controller)

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}

		// And error should contain exec error message
		expectedError := "command execution failed: exec failed"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("SecretsLoadingError", func(t *testing.T) {
		// Given a set of mocks with proper configuration
		mocks, _, _ := setup(t, &SetupOptions{
			ConfigStr: `
contexts:
  default:
    tools:
      enabled: true`,
		})

		// Override secrets provider to return error
		mockSecretsProvider := secrets.NewMockSecretsProvider(mocks.Injector)
		mockSecretsProvider.LoadSecretsFunc = func() error {
			return fmt.Errorf("secrets loading failed")
		}
		mocks.Controller.ResolveAllSecretsProvidersFunc = func() []secrets.SecretsProvider {
			return []secrets.SecretsProvider{mockSecretsProvider}
		}

		rootCmd.SetArgs([]string{"exec", "--", "test-command"})

		// When executing the command
		err := Execute(mocks.Controller)

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}

		// And error should contain secrets loading message
		expectedError := "Error loading secrets: secrets loading failed"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ShellExecutionError", func(t *testing.T) {
		// Given a set of mocks with proper configuration
		mocks, _, _ := setup(t, &SetupOptions{
			ConfigStr: `
contexts:
  default:
    tools:
      enabled: true`,
		})

		// Override shell to return execution error
		mockShell := shell.NewMockShell()
		mockShell.ExecFunc = func(command string, args ...string) (string, error) {
			return "", fmt.Errorf("command execution failed")
		}
		mocks.Controller.ResolveShellFunc = func() shell.Shell {
			return mockShell
		}

		rootCmd.SetArgs([]string{"exec", "--", "test-command"})

		// When executing the command
		err := Execute(mocks.Controller)

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}

		// And error should contain command execution error message
		expectedError := "command execution failed: command execution failed"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("SetenvError", func(t *testing.T) {
		// Given a set of mocks with proper configuration
		mocks, _, _ := setup(t, &SetupOptions{
			ConfigStr: `
contexts:
  default:
    tools:
      enabled: true`,
		})

		// Store original shims and replace with mock
		originalShims := shims
		shims = &Shims{
			Setenv: func(key, value string) error {
				return fmt.Errorf("setenv failed")
			},
		}
		defer func() { shims = originalShims }()

		rootCmd.SetArgs([]string{"exec", "--", "test-command"})

		// When executing the command
		err := Execute(mocks.Controller)

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}

		// And error should contain setenv error message
		expectedError := "Error setting environment variable TEST_VAR: setenv failed"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})
}
