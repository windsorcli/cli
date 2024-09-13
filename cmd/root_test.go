package cmd

import (
	"errors"
	"testing"

	"github.com/spf13/cobra"
)

func TestExecute_NoError(t *testing.T) {
	// Save the original exitFunc and restore it after the test
	originalExitFunc := exitFunc
	defer func() { exitFunc = originalExitFunc }()

	// Mock exitFunc to track if it's called
	exitCalled := false
	exitFunc = func(code int) {
		exitCalled = true
	}

	// Set rootCmd's RunE to a function that returns nil (no error)
	rootCmd.RunE = func(cmd *cobra.Command, args []string) error {
		return nil
	}

	// Execute the command
	Execute()

	// Verify that exitFunc was not called
	if exitCalled {
		t.Errorf("exitFunc was called when it should not have been")
	}
}

func TestExecute_WithError(t *testing.T) {
	// Save the original exitFunc and restore it after the test
	originalExitFunc := exitFunc
	defer func() { exitFunc = originalExitFunc }()

	// Mock exitFunc to capture the exit code
	var exitCode int
	exitFunc = func(code int) {
		exitCode = code
	}

	// Set rootCmd's RunE to a function that returns an error
	rootCmd.RunE = func(cmd *cobra.Command, args []string) error {
		return errors.New("test error")
	}

	// Execute the command
	Execute()

	// Verify that exitFunc was called with code 1
	if exitCode != 1 {
		t.Errorf("exitFunc was not called with code 1, got %d", exitCode)
	}
}
