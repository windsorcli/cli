package cmd

import (
	"bytes"
	"fmt"
	"testing"

	ctrl "github.com/windsorcli/cli/pkg/controller"
	"github.com/windsorcli/cli/pkg/shell"
)

func TestHookCmd(t *testing.T) {
	setup := func(t *testing.T, opts ...*SetupOptions) (*Mocks, *bytes.Buffer, *bytes.Buffer) {
		t.Helper()
		mocks := setupMocks(t, opts...)
		stdout, stderr := captureOutput(t)
		rootCmd.SetOut(stdout)
		rootCmd.SetErr(stderr)
		return mocks, stdout, stderr
	}

	t.Run("Success", func(t *testing.T) {
		// Given a set of mocks with proper configuration
		mocks, _, stderr := setup(t)

		// Mock the shell methods
		mockShell := shell.NewMockShell()
		mockShell.InstallHookFunc = func(shellName string) error {
			return nil
		}
		mocks.Controller.ResolveShellFunc = func() shell.Shell {
			return mockShell
		}

		rootCmd.SetArgs([]string{"hook", "zsh"})

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

	t.Run("NoShellName", func(t *testing.T) {
		// Given a set of mocks with proper configuration
		mocks, _, _ := setup(t)

		rootCmd.SetArgs([]string{"hook"})

		// When executing the command without shell name
		err := Execute(mocks.Controller)

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}

		// And error should contain usage message
		expectedError := "No shell name provided"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("UnsupportedShell", func(t *testing.T) {
		// Given a set of mocks with proper configuration
		mocks, _, _ := setup(t)

		// Mock the shell methods to return unsupported shell error
		mockShell := shell.NewMockShell()
		mockShell.InstallHookFunc = func(shellName string) error {
			return fmt.Errorf("Unsupported shell: %s", shellName)
		}
		mocks.Controller.ResolveShellFunc = func() shell.Shell {
			return mockShell
		}

		rootCmd.SetArgs([]string{"hook", "unsupported"})

		// When executing the command with unsupported shell
		err := Execute(mocks.Controller)

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}

		// And error should contain unsupported shell message
		expectedError := "Unsupported shell: unsupported"
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

		rootCmd.SetArgs([]string{"hook", "zsh"})

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

	t.Run("InstallHookError", func(t *testing.T) {
		// Given a set of mocks with install hook error
		mocks, _, _ := setup(t)

		// Mock the shell methods to return error
		mockShell := shell.NewMockShell()
		mockShell.InstallHookFunc = func(shellName string) error {
			return fmt.Errorf("hook installation failed")
		}
		mocks.Controller.ResolveShellFunc = func() shell.Shell {
			return mockShell
		}

		rootCmd.SetArgs([]string{"hook", "zsh"})

		// When executing the command
		err := Execute(mocks.Controller)

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}

		// And error should contain install hook message
		expectedError := "hook installation failed"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})
}
