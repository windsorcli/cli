package cmd

import (
	"bytes"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/spf13/cobra"
	"github.com/windsor-hotel/cli/internal/interfaces"
)

// MockConfigHandler is a mock implementation of the ConfigHandler interface
type MockConfigHandler struct {
	LoadConfigErr error
}

func (m *MockConfigHandler) LoadConfig(path string) error {
	return m.LoadConfigErr
}

func (m *MockConfigHandler) GetConfigValue(key string) (string, error) {
	return "", nil
}

func (m *MockConfigHandler) SetConfigValue(key, value string) error {
	return nil
}

func (m *MockConfigHandler) SaveConfig(path string) error {
	return nil
}

// Ensure MockConfigHandler implements ConfigHandler
var _ interfaces.ConfigHandler = (*MockConfigHandler)(nil)

var originalExitFunc = exitFunc

func setupTest(t *testing.T) {
	// Reset exitFunc to the original function
	exitFunc = originalExitFunc

	// Ensure the rootCmd is clean by removing any added subcommands
	rootCmd.ResetFlags()
	rootCmd.SetArgs([]string{})
	rootCmd.PersistentPreRunE = preRunLoadConfig

	// Cleanup after the test
	t.Cleanup(func() {
		exitFunc = originalExitFunc
		rootCmd.ResetFlags()
		rootCmd.SetArgs([]string{})
		rootCmd.PersistentPreRunE = preRunLoadConfig
	})
}

func TestPreRunLoadConfig_Success(t *testing.T) {
	setupTest(t)

	mockHandler := &MockConfigHandler{
		LoadConfigErr: nil,
	}

	Initialize(mockHandler)

	err := preRunLoadConfig(nil, nil)
	if err != nil {
		t.Fatalf("preRunLoadConfig() error = %v, expected nil", err)
	}
}

func TestPreRunLoadConfig_Failure(t *testing.T) {
	setupTest(t)

	mockHandler := &MockConfigHandler{
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
	setupTest(t)

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
	setupTest(t)

	// Mock exitFunc to capture the exit code
	var exitCode int
	exitFunc = func(code int) {
		exitCode = code
	}

	// Initialize with a successful config handler
	mockHandler := &MockConfigHandler{
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
	setupTest(t)

	var exitCode int
	var stderr bytes.Buffer
	exitFunc = func(code int) {
		exitCode = code
	}
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	defer func() {
		os.Stderr = oldStderr
		w.Close()
	}()

	mockHandler := &MockConfigHandler{
		LoadConfigErr: errors.New("config load error"),
	}
	Initialize(mockHandler)

	dummyCmd := &cobra.Command{
		Use: "dummy",
		Run: func(cmd *cobra.Command, args []string) {},
	}
	rootCmd.AddCommand(dummyCmd)
	rootCmd.SetArgs([]string{"dummy"})

	Execute()

	w.Close()
	io.Copy(&stderr, r)

	if exitCode != 1 {
		t.Errorf("exitFunc was not called with code 1, got %d", exitCode)
	}

	expectedErrorMsg := "Error loading config file: config load error\n"
	actualErrorMsg := stderr.String()

	// Extract the actual error message from the output
	if len(actualErrorMsg) > len(expectedErrorMsg) {
		actualErrorMsg = actualErrorMsg[len(actualErrorMsg)-len(expectedErrorMsg):]
	}

	if actualErrorMsg != expectedErrorMsg {
		t.Errorf("Expected error message '%s', got '%s'", expectedErrorMsg, actualErrorMsg)
	}

	rootCmd.RemoveCommand(dummyCmd)
}
