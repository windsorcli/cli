package cmd

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/shell"
	"github.com/windsorcli/cli/pkg/di"
)

func TestContextCmd(t *testing.T) {
	setup := func(t *testing.T) (*bytes.Buffer, *bytes.Buffer) {
		t.Helper()

		// Clear environment variables that could affect tests
		origContext := os.Getenv("WINDSOR_CONTEXT")
		os.Unsetenv("WINDSOR_CONTEXT")
		t.Cleanup(func() {
			if origContext != "" {
				os.Setenv("WINDSOR_CONTEXT", origContext)
			}
		})

		// Change to a temporary directory without a config file
		origDir, err := os.Getwd()
		if err != nil {
			t.Fatalf("Failed to get working directory: %v", err)
		}

		tmpDir := t.TempDir()
		if err := os.Chdir(tmpDir); err != nil {
			t.Fatalf("Failed to change to temp directory: %v", err)
		}

		// Cleanup to change back to original directory
		t.Cleanup(func() {
			if err := os.Chdir(origDir); err != nil {
				t.Logf("Warning: Failed to change back to original directory: %v", err)
			}
		})

		stdout, stderr := captureOutput(t)
		rootCmd.SetOut(stdout)
		rootCmd.SetErr(stderr)
		return stdout, stderr
	}

	t.Run("GetContext", func(t *testing.T) {
		// Given proper output capture in a directory without config
		stdout, _ := setup(t)
		// Don't set up mocks - we want to test real behavior

		rootCmd.SetArgs([]string{"context", "get"})

		// When executing the command
		err := Execute()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And should output default context (real behavior)
		output := stdout.String()
		expectedOutput := "local\n"
		if output != expectedOutput {
			t.Errorf("Expected output %q, got %q", expectedOutput, output)
		}
	})

	t.Run("SetContextNoArgs", func(t *testing.T) {
		// Given proper output capture in a directory without config
		_, _ = setup(t)
		setupMocks(t)

		rootCmd.SetArgs([]string{"context", "set"})

		// When executing the command
		err := Execute()

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

	t.Run("SetContext", func(t *testing.T) {
		// Given proper output capture in a directory without config
		_, _ = setup(t)
		setupMocks(t)

		rootCmd.SetArgs([]string{"context", "set", "new-context"})

		// When executing the command
		err := Execute()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("GetContextAlias", func(t *testing.T) {
		// Given proper output capture in a directory without config
		stdout, _ := setup(t)
		// Don't set up mocks - we want to test real behavior

		rootCmd.SetArgs([]string{"get-context"})

		// When executing the command
		err := Execute()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And should output the current context (may be "local" or previously set context)
		output := stdout.String()
		if output == "" {
			t.Error("Expected some output, got empty string")
		}
	})

	t.Run("SetContextAliasNoArgs", func(t *testing.T) {
		// Given proper output capture in a directory without config
		_, _ = setup(t)
		setupMocks(t)

		rootCmd.SetArgs([]string{"set-context"})

		// When executing the command
		err := Execute()

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

	t.Run("SetContextAlias", func(t *testing.T) {
		// Given proper output capture in a directory without config
		_, _ = setup(t)
		setupMocks(t)

		rootCmd.SetArgs([]string{"set-context", "new-context"})

		// When executing the command
		err := Execute()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}

// =============================================================================
// Test Error Scenarios
// =============================================================================

func TestContextCmd_ErrorScenarios(t *testing.T) {
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

	t.Run("GetContext_HandlesNewRuntimeError", func(t *testing.T) {
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

		rootCmd.SetArgs([]string{"context", "get"})

		err := Execute()

		if err == nil {
			t.Error("Expected error when NewRuntime fails")
		}

		if !strings.Contains(err.Error(), "failed to initialize context") {
			t.Errorf("Expected error about context initialization, got: %v", err)
		}
	})

	t.Run("GetContext_HandlesLoadConfigError", func(t *testing.T) {
		setup(t)
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
		mockShell.InitializeFunc = func() error {
			return nil
		}
		injector.Register("shell", mockShell)

		ctx := context.WithValue(context.Background(), injectorKey, injector)
		rootCmd.SetContext(ctx)

		rootCmd.SetArgs([]string{"context", "get"})

		err := Execute()

		if err == nil {
			t.Error("Expected error when LoadConfig fails")
		}

		if !strings.Contains(err.Error(), "failed to load config") {
			t.Errorf("Expected error about config loading, got: %v", err)
		}
	})

	t.Run("SetContext_HandlesNewRuntimeError", func(t *testing.T) {
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

		rootCmd.SetArgs([]string{"context", "set", "test-context"})

		err := Execute()

		if err == nil {
			t.Error("Expected error when NewRuntime fails")
		}

		if !strings.Contains(err.Error(), "failed to initialize context") {
			t.Errorf("Expected error about context initialization, got: %v", err)
		}
	})

	t.Run("SetContext_HandlesLoadConfigError", func(t *testing.T) {
		setup(t)
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
		mockShell.InitializeFunc = func() error {
			return nil
		}
		injector.Register("shell", mockShell)

		ctx := context.WithValue(context.Background(), injectorKey, injector)
		rootCmd.SetContext(ctx)

		rootCmd.SetArgs([]string{"context", "set", "test-context"})

		err := Execute()

		if err == nil {
			t.Error("Expected error when LoadConfig fails")
		}

		if !strings.Contains(err.Error(), "failed to load config") {
			t.Errorf("Expected error about config loading, got: %v", err)
		}
	})

	t.Run("SetContext_HandlesWriteResetTokenError", func(t *testing.T) {
		setup(t)
		mocks := setupMocks(t)
		mocks.Shell.WriteResetTokenFunc = func() (string, error) {
			return "", fmt.Errorf("write reset token failed")
		}
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		rootCmd.SetContext(ctx)

		rootCmd.SetArgs([]string{"context", "set", "test-context"})

		err := Execute()

		if err == nil {
			t.Error("Expected error when WriteResetToken fails")
		}

		if !strings.Contains(err.Error(), "failed to write reset token") {
			t.Errorf("Expected error about reset token, got: %v", err)
		}
	})

	t.Run("SetContext_HandlesSetContextError", func(t *testing.T) {
		setup(t)
		injector := di.NewInjector()
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
		mockConfigHandler.SetContextFunc = func(context string) error {
			return fmt.Errorf("set context failed")
		}
		injector.Register("configHandler", mockConfigHandler)

		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) {
			return t.TempDir(), nil
		}
		mockShell.WriteResetTokenFunc = func() (string, error) {
			return "mock-reset-token", nil
		}
		mockShell.InitializeFunc = func() error {
			return nil
		}
		injector.Register("shell", mockShell)

		ctx := context.WithValue(context.Background(), injectorKey, injector)
		rootCmd.SetContext(ctx)

		rootCmd.SetArgs([]string{"context", "set", "test-context"})

		err := Execute()

		if err == nil {
			t.Error("Expected error when SetContext fails")
		}

		if !strings.Contains(err.Error(), "failed to set context") {
			t.Errorf("Expected error about setting context, got: %v", err)
		}
	})
}
