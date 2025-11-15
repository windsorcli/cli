package cmd

import (
	"bytes"
	"context"
	"fmt"
	"testing"
)

func TestHookCmd(t *testing.T) {
	setup := func(t *testing.T) (*bytes.Buffer, *bytes.Buffer) {
		t.Helper()
		stdout, stderr := captureOutput(t)
		rootCmd.SetOut(stdout)
		rootCmd.SetErr(stderr)
		return stdout, stderr
	}

	t.Run("Success", func(t *testing.T) {
		// Given proper output capture and mock setup
		_, stderr := setup(t)
		setupMocks(t)

		// Set up command context with injector
		ctx := context.Background()
		rootCmd.SetContext(ctx)

		rootCmd.SetArgs([]string{"hook", "zsh"})

		// When executing the command
		err := Execute()

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
		// Given proper output capture
		setup(t)

		rootCmd.SetArgs([]string{"hook"})

		// When executing the command without shell name
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

	t.Run("UnsupportedShell", func(t *testing.T) {
		// Given proper output capture and mock setup
		setup(t)
		mocks := setupMocks(t)

		// Configure mock shell to return error for unsupported shell
		mocks.Shell.InstallHookFunc = func(shellName string) error {
			if shellName == "unsupported" {
				return fmt.Errorf("Unsupported shell: %s", shellName)
			}
			return nil
		}

		// Set up command context with injector
		ctx := context.Background()
		rootCmd.SetContext(ctx)

		rootCmd.SetArgs([]string{"hook", "unsupported"})

		// When executing the command with unsupported shell
		err := Execute()

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
			return
		}

		// And error should contain unsupported shell message
		if !contains(err.Error(), "Unsupported shell") {
			t.Errorf("Expected error to contain 'Unsupported shell', got %q", err.Error())
		}
	})
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || (len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || containsSubstring(s, substr))))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
