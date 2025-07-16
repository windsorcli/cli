package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestEnvCmd(t *testing.T) {
	setup := func(t *testing.T) (*bytes.Buffer, *bytes.Buffer) {
		t.Helper()
		stdout, stderr := captureOutput(t)
		rootCmd.SetOut(stdout)
		rootCmd.SetErr(stderr)
		return stdout, stderr
	}

	setupTrustedDirectory := func(t *testing.T) func() {
		t.Helper()

		// Set up a temporary directory structure with trusted file
		tmpDir := t.TempDir()
		testDir := filepath.Join(tmpDir, "project")
		if err := os.MkdirAll(testDir, 0755); err != nil {
			t.Fatalf("Failed to create test directory: %v", err)
		}

		// Create trusted file
		trustedDir := filepath.Join(tmpDir, ".config", "windsor")
		if err := os.MkdirAll(trustedDir, 0755); err != nil {
			t.Fatalf("Failed to create trusted directory: %v", err)
		}

		trustedFile := filepath.Join(trustedDir, ".trusted")
		realTestDir, _ := filepath.EvalSymlinks(testDir)
		trustedContent := realTestDir + "\n"
		if err := os.WriteFile(trustedFile, []byte(trustedContent), 0644); err != nil {
			t.Fatalf("Failed to create trusted file: %v", err)
		}

		// Change to test directory
		originalDir, err := os.Getwd()
		if err != nil {
			t.Fatalf("Failed to get current directory: %v", err)
		}

		if err := os.Chdir(testDir); err != nil {
			t.Fatalf("Failed to change directory: %v", err)
		}

		// Mock home directory
		originalHome := os.Getenv("HOME")
		os.Setenv("HOME", tmpDir)

		// Return cleanup function
		return func() {
			os.Chdir(originalDir)
			os.Setenv("HOME", originalHome)
		}
	}

	t.Run("Success", func(t *testing.T) {
		// Given proper output capture and trusted directory
		_, stderr := setup(t)
		cleanup := setupTrustedDirectory(t)
		defer cleanup()

		rootCmd.SetArgs([]string{"env"})

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

	t.Run("SuccessWithDecrypt", func(t *testing.T) {
		// Given proper output capture and trusted directory
		_, stderr := setup(t)
		cleanup := setupTrustedDirectory(t)
		defer cleanup()

		rootCmd.SetArgs([]string{"env", "--decrypt"})

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

	t.Run("SuccessWithHook", func(t *testing.T) {
		// Given proper output capture
		_, stderr := setup(t)

		rootCmd.SetArgs([]string{"env", "--hook"})

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

	t.Run("SuccessWithVerbose", func(t *testing.T) {
		// Given proper output capture and trusted directory
		_, stderr := setup(t)
		cleanup := setupTrustedDirectory(t)
		defer cleanup()

		rootCmd.SetArgs([]string{"env", "--verbose"})

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

	t.Run("SuccessWithAllFlags", func(t *testing.T) {
		// Given proper output capture
		_, stderr := setup(t)

		rootCmd.SetArgs([]string{"env", "--decrypt", "--hook", "--verbose"})

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
}
