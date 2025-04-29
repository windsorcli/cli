package cmd

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/controller"
	"github.com/windsorcli/cli/pkg/shell"
)

func TestContextCmd(t *testing.T) {
	setup := func(t *testing.T, opts ...*SetupOptions) (*Mocks, *bytes.Buffer, *bytes.Buffer) {
		t.Helper()

		// Setup mocks with default options
		mocks := setupMocks(t, opts...)

		// Setup command args and output
		stdout, stderr := captureOutput(t)
		rootCmd.SetOut(stdout)
		rootCmd.SetErr(stderr)

		return mocks, stdout, stderr
	}

	t.Run("GetContext", func(t *testing.T) {
		// Given a set of mocks with proper configuration
		mocks, stdout, stderr := setup(t, &SetupOptions{
			ConfigStr: `
contexts:
  default:
    tools:
      enabled: true`,
		})

		rootCmd.SetArgs([]string{"context", "get"})

		// When executing the command
		err := Execute(mocks.Controller)

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}

		// And output should contain current context
		output := stdout.String()
		if output != "default\n" {
			t.Errorf("Expected 'default', got: %q", output)
		}
		if stderr.String() != "" {
			t.Error("Expected empty stderr")
		}
	})

	t.Run("GetContextNoConfig", func(t *testing.T) {
		// Given a set of mocks with no configuration
		mocks, _, _ := setup(t)

		rootCmd.SetArgs([]string{"context", "get"})

		// When executing the command
		err := Execute(mocks.Controller)

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}

		// And error should contain init message
		expectedError := "No context is available. Have you run `windsor init`?"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("SetContext", func(t *testing.T) {
		// Given a set of mocks with proper configuration
		mocks, stdout, stderr := setup(t, &SetupOptions{
			ConfigStr: `
contexts:
  default:
    tools:
      enabled: true`,
		})

		// Mock the shell methods
		mockShell := shell.NewMockShell()
		mockShell.WriteResetTokenFunc = func() (string, error) {
			return "mock-token", nil
		}
		mocks.Controller.ResolveShellFunc = func() shell.Shell {
			return mockShell
		}

		rootCmd.SetArgs([]string{"context", "set", "new-context"})

		// When executing the command
		err := Execute(mocks.Controller)

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}

		// And output should contain success message
		output := stdout.String()
		if output != "Context set to: new-context\n" {
			t.Errorf("Expected 'Context set to: new-context', got: %q", output)
		}
		if stderr.String() != "" {
			t.Error("Expected empty stderr")
		}
	})

	t.Run("SetContextNoArgs", func(t *testing.T) {
		// Given a set of mocks with proper configuration
		mocks, _, _ := setup(t, &SetupOptions{
			ConfigStr: `
contexts:
  default:
    tools:
      enabled: true`,
		})

		rootCmd.SetArgs([]string{"context", "set"})

		// When executing the command
		err := Execute(mocks.Controller)

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}

		// And error should contain usage message
		expectedError := "accepts 1 arg(s), received 0"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("SetContextNoConfig", func(t *testing.T) {
		// Given a set of mocks with no configuration
		mocks, _, _ := setup(t)

		// Mock initialization to return error
		mocks.Controller.InitializeWithRequirementsFunc = func(req controller.Requirements) error {
			return fmt.Errorf("No context is available. Have you run `windsor init`?")
		}

		rootCmd.SetArgs([]string{"context", "set", "new-context"})

		// When executing the command
		err := Execute(mocks.Controller)

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}

		// And error should contain init message
		expectedError := "Error initializing environment components: No context is available. Have you run `windsor init`?"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("GetContextEnvError", func(t *testing.T) {
		// Given a set of mocks with proper configuration
		mocks, _, _ := setup(t, &SetupOptions{
			ConfigStr: `
contexts:
  default:
    tools:
      enabled: true`,
		})

		// Mock SetEnvironmentVariables to return error
		mocks.Controller.SetEnvironmentVariablesFunc = func() error {
			return fmt.Errorf("env error")
		}

		rootCmd.SetArgs([]string{"context", "get"})

		// When executing the command
		err := Execute(mocks.Controller)

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}

		// And error should contain env error message
		expectedError := "Error setting environment variables: env error"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("SetContextEnvError", func(t *testing.T) {
		// Given a set of mocks with proper configuration
		mocks, _, _ := setup(t, &SetupOptions{
			ConfigStr: `
contexts:
  default:
    tools:
      enabled: true`,
		})

		// Mock SetEnvironmentVariables to return error
		mocks.Controller.SetEnvironmentVariablesFunc = func() error {
			return fmt.Errorf("env error")
		}

		rootCmd.SetArgs([]string{"context", "set", "new-context"})

		// When executing the command
		err := Execute(mocks.Controller)

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}

		// And error should contain env error message
		expectedError := "Error setting environment variables: env error"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("SetContextError", func(t *testing.T) {
		// Given a set of mocks with proper configuration
		mocks, _, _ := setup(t, &SetupOptions{
			ConfigStr: `
contexts:
  default:
    tools:
      enabled: true`,
		})

		// Mock the shell methods
		mockShell := shell.NewMockShell()
		mockShell.WriteResetTokenFunc = func() (string, error) {
			return "mock-token", nil
		}
		mocks.Controller.ResolveShellFunc = func() shell.Shell {
			return mockShell
		}

		// Mock config handler to return error on SetContext
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.SetContextFunc = func(context string) error {
			return fmt.Errorf("set context error")
		}
		mocks.Controller.ResolveConfigHandlerFunc = func() config.ConfigHandler {
			return mockConfigHandler
		}

		rootCmd.SetArgs([]string{"context", "set", "new-context"})

		// When executing the command
		err := Execute(mocks.Controller)

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}

		// And error should contain set context error message
		expectedError := "Error setting context: set context error"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("GetContextAlias", func(t *testing.T) {
		// Given a set of mocks with proper configuration
		mocks, stdout, stderr := setup(t, &SetupOptions{
			ConfigStr: `
contexts:
  default:
    tools:
      enabled: true`,
		})

		rootCmd.SetArgs([]string{"get-context"})

		// When executing the command
		err := Execute(mocks.Controller)

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}

		// And output should contain current context
		output := stdout.String()
		if output != "default\n" {
			t.Errorf("Expected 'default', got: %q", output)
		}
		if stderr.String() != "" {
			t.Error("Expected empty stderr")
		}
	})

	t.Run("SetContextAlias", func(t *testing.T) {
		// Given a set of mocks with proper configuration
		mocks, stdout, stderr := setup(t, &SetupOptions{
			ConfigStr: `
contexts:
  default:
    tools:
      enabled: true`,
		})

		// Mock the shell methods
		mockShell := shell.NewMockShell()
		mockShell.WriteResetTokenFunc = func() (string, error) {
			return "mock-token", nil
		}
		mocks.Controller.ResolveShellFunc = func() shell.Shell {
			return mockShell
		}

		rootCmd.SetArgs([]string{"set-context", "new-context"})

		// When executing the command
		err := Execute(mocks.Controller)

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}

		// And output should contain success message
		output := stdout.String()
		if output != "Context set to: new-context\n" {
			t.Errorf("Expected 'Context set to: new-context', got: %q", output)
		}
		if stderr.String() != "" {
			t.Error("Expected empty stderr")
		}
	})

	t.Run("SetContextAliasNoArgs", func(t *testing.T) {
		// Given a set of mocks with proper configuration
		mocks, _, _ := setup(t, &SetupOptions{
			ConfigStr: `
contexts:
  default:
    tools:
      enabled: true`,
		})

		rootCmd.SetArgs([]string{"set-context"})

		// When executing the command
		err := Execute(mocks.Controller)

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}

		// And error should contain usage message
		expectedError := "accepts 1 arg(s), received 0"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("SetContextResetTokenError", func(t *testing.T) {
		// Given a set of mocks with proper configuration
		mocks, _, _ := setup(t, &SetupOptions{
			ConfigStr: `
contexts:
  default:
    tools:
      enabled: true`,
		})

		// Mock the shell methods to return error
		mockShell := shell.NewMockShell()
		mockShell.WriteResetTokenFunc = func() (string, error) {
			return "", fmt.Errorf("reset token error")
		}
		mocks.Controller.ResolveShellFunc = func() shell.Shell {
			return mockShell
		}

		rootCmd.SetArgs([]string{"context", "set", "new-context"})

		// When executing the command
		err := Execute(mocks.Controller)

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}

		// And error should contain reset token error message
		expectedError := "Error writing reset token: reset token error"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("SetContextAliasResetTokenError", func(t *testing.T) {
		// Given a set of mocks with proper configuration
		mocks, _, _ := setup(t, &SetupOptions{
			ConfigStr: `
contexts:
  default:
    tools:
      enabled: true`,
		})

		// Mock the shell methods to return error
		mockShell := shell.NewMockShell()
		mockShell.WriteResetTokenFunc = func() (string, error) {
			return "", fmt.Errorf("reset token error")
		}
		mocks.Controller.ResolveShellFunc = func() shell.Shell {
			return mockShell
		}

		rootCmd.SetArgs([]string{"set-context", "new-context"})

		// When executing the command
		err := Execute(mocks.Controller)

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}

		// And error should contain reset token error message
		expectedError := "Error writing reset token: reset token error"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})
}
