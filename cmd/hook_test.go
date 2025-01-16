package cmd

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/config"
	ctrl "github.com/windsorcli/cli/pkg/controller"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
)

func setupSafeHookCmdMocks() *MockObjects {
	injector := di.NewInjector()
	mockController := ctrl.NewMockController(injector)

	// Setup mock context handler
	mockConfigHandler := config.NewMockConfigHandler()
	mockConfigHandler.GetContextFunc = func() string {
		return "test-context"
	}
	injector.Register("configHandler", mockConfigHandler)

	mockShell := shell.NewMockShell()
	mockShell.InstallHookFunc = func(shellName string) error {
		if shellName == "" {
			return fmt.Errorf("No shell name provided")
		}
		return nil
	}
	mockController.ResolveShellFunc = func() shell.Shell {
		return mockShell
	}

	return &MockObjects{
		Controller: mockController,
		Shell:      mockShell,
	}
}

func TestHookCmd(t *testing.T) {
	originalExitFunc := exitFunc
	exitFunc = mockExit
	t.Cleanup(func() {
		exitFunc = originalExitFunc
	})

	t.Run("Success", func(t *testing.T) {
		defer resetRootCmd()

		// Setup mock controller
		mocks := setupSafeHookCmdMocks()

		// Capture stdout using captureStdout
		output := captureStdout(func() {
			rootCmd.SetArgs([]string{"hook", "bash"})
			err := Execute(mocks.Controller)
			if err != nil {
				t.Fatalf("Expected no error, got %v", err)
			}
		})

		// Then the output should be empty as InstallHook does not return output
		expectedOutput := ""
		if output != expectedOutput {
			t.Errorf("Expected output to be %q, got %q", expectedOutput, output)
		}
	})

	t.Run("NoShellNameProvided", func(t *testing.T) {
		defer resetRootCmd()

		// Setup mock controller
		mocks := setupSafeHookCmdMocks()

		// Capture stderr
		var buf bytes.Buffer
		rootCmd.SetErr(&buf)
		rootCmd.SetArgs([]string{"hook"})
		err := Execute(mocks.Controller)
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		output := buf.String()

		// Verify output
		expectedOutput := "No shell name provided"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("ErrorInstallingHook", func(t *testing.T) {
		defer resetRootCmd()

		// Setup mock controller
		mocks := setupSafeHookCmdMocks()
		mocks.Shell.InstallHookFunc = func(shellName string) error {
			return fmt.Errorf("hook installation error")
		}

		// Capture stderr
		var buf bytes.Buffer
		rootCmd.SetErr(&buf)

		// When the hook command is executed
		rootCmd.SetArgs([]string{"hook", "bash"})
		err := Execute(mocks.Controller)
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		output := buf.String()

		// Then the output should indicate the error
		expectedOutput := "hook installation error"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})
}
