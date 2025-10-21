package cmd

import (
	"bytes"
	"os"
	"testing"
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
