package cmd

import (
	"bytes"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/spf13/cobra"
	"github.com/windsor-hotel/cli/internal/config"
)

func TestPreRunLoadConfig_Success(t *testing.T) {
	mockHandler := &config.MockConfigHandler{
		LoadConfigErr: nil,
	}

	Initialize(mockHandler)

	err := preRunLoadConfig(nil, nil)
	if err != nil {
		t.Fatalf("preRunLoadConfig() error = %v, expected nil", err)
	}
}

func TestPreRunLoadConfig_Failure(t *testing.T) {
	mockHandler := &config.MockConfigHandler{
		LoadConfigErr: errors.New("config load error"),
	}

	Initialize(mockHandler)

	err := preRunLoadConfig(nil, nil)
	if err == nil {
		t.Fatalf("preRunLoadConfig() expected error, got nil")
	}
	if err.Error() != "Error loading config file: config load error" {
		t.Fatalf("preRunLoadConfig() error = %v, expected 'Error loading config file: config load error'", err)
	}
}

func TestPreRunLoadConfig_NoConfigHandler(t *testing.T) {
	// Ensure configHandler is not initialized
	configHandler = nil

	err := preRunLoadConfig(nil, nil)
	if err == nil {
		t.Fatalf("preRunLoadConfig() expected error, got nil")
	}
	expectedError := "configHandler is not initialized"
	if err.Error() != expectedError {
		t.Fatalf("preRunLoadConfig() error = %v, expected '%s'", err, expectedError)
	}
}

func TestExecute(t *testing.T) {
	// Mock exitFunc to capture the exit code
	var exitCode int
	exitFunc = func(code int) {
		exitCode = code
	}

	// Initialize with a successful config handler
	mockHandler := &config.MockConfigHandler{
		LoadConfigErr: nil,
	}
	Initialize(mockHandler)

	// Add a dummy subcommand to trigger PersistentPreRunE
	dummyCmd := &cobra.Command{
		Use: "dummy",
		Run: func(cmd *cobra.Command, args []string) {},
	}
	rootCmd.AddCommand(dummyCmd)
	rootCmd.SetArgs([]string{"dummy"})

	// Execute the command
	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	// Verify that exitFunc was not called
	if exitCode != 0 {
		t.Errorf("exitFunc was called with code %d, expected 0", exitCode)
	}

	// Remove the dummy subcommand after the test
	rootCmd.RemoveCommand(dummyCmd)
}

func TestExecute_LoadConfigError(t *testing.T) {
	// Mock exitFunc to capture the exit code
	var exitCode int
	exitFunc = func(code int) {
		exitCode = code
	}

	mockHandler := &config.MockConfigHandler{
		LoadConfigErr: errors.New("config load error"),
	}
	Initialize(mockHandler)

	dummyCmd := &cobra.Command{
		Use: "dummy",
		Run: func(cmd *cobra.Command, args []string) {},
	}
	rootCmd.AddCommand(dummyCmd)
	rootCmd.SetArgs([]string{"dummy"})

	// Capture stderr
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	Execute()

	w.Close()
	var buf bytes.Buffer
	io.Copy(&buf, r)
	os.Stderr = oldStderr
	actualErrorMsg := buf.String()

	if exitCode != 1 {
		t.Errorf("exitFunc was not called with code 1, got %d", exitCode)
	}

	expectedErrorMsg := "Error loading config file: config load error\n"
	if !containsErrorMessage(actualErrorMsg, expectedErrorMsg) {
		t.Errorf("Expected error message '%s', got '%s'", expectedErrorMsg, actualErrorMsg)
	}

	rootCmd.RemoveCommand(dummyCmd)
}

// containsErrorMessage checks if the actual error message contains the expected error message
func containsErrorMessage(actual, expected string) bool {
	return bytes.Contains([]byte(actual), []byte(expected))
}
