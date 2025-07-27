package cmd

import (
	"bytes"
	"os"
	"testing"
)

func TestContextCmd(t *testing.T) {
	setup := func(t *testing.T) (*bytes.Buffer, *bytes.Buffer) {
		t.Helper()

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

	t.Run("GetContextNoConfig", func(t *testing.T) {
		// Given proper output capture in a directory without config
		_, _ = setup(t)

		rootCmd.SetArgs([]string{"context", "get"})

		// When executing the command
		err := Execute()

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}

		// And error should contain init message
		expectedError := "Error executing context pipeline: No context is available. Have you run `windsor init`?"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("SetContextNoArgs", func(t *testing.T) {
		// Given proper output capture in a directory without config
		_, _ = setup(t)

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

	t.Run("SetContextNoConfig", func(t *testing.T) {
		// Given proper output capture in a directory without config
		_, _ = setup(t)

		rootCmd.SetArgs([]string{"context", "set", "new-context"})

		// When executing the command
		err := Execute()

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}

		// And error should contain init message
		expectedError := "Error executing context pipeline: No context is available. Have you run `windsor init`?"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("GetContextAliasNoConfig", func(t *testing.T) {
		// Given proper output capture in a directory without config
		_, _ = setup(t)

		rootCmd.SetArgs([]string{"get-context"})

		// When executing the command
		err := Execute()

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}

		// And error should contain init message
		expectedError := "Error executing context pipeline: No context is available. Have you run `windsor init`?"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("SetContextAliasNoArgs", func(t *testing.T) {
		// Given proper output capture in a directory without config
		_, _ = setup(t)

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

	t.Run("SetContextAliasNoConfig", func(t *testing.T) {
		// Given proper output capture in a directory without config
		_, _ = setup(t)

		rootCmd.SetArgs([]string{"set-context", "new-context"})

		// When executing the command
		err := Execute()

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}

		// And error should contain init message
		expectedError := "Error executing context pipeline: No context is available. Have you run `windsor init`?"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})
}
