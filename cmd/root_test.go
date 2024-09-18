package cmd

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/di"
)

func TestPreRunLoadConfig_Success(t *testing.T) {
	// Create a new container for each test
	container := di.NewContainer()

	// Register a mock config handler
	mockHandler := config.NewMockConfigHandler(
		func(path string) error { return nil },
		func(key string) (string, error) { return "value", nil },
		nil, nil, nil, nil,
	)
	container.Register("configHandler", mockHandler)

	// Initialize the cmd package with the container
	Initialize(container)

	// Execute the preRunLoadConfig function
	err := preRunLoadConfig(nil, nil)
	if err != nil {
		t.Fatalf("preRunLoadConfig() error = %v, expected no error", err)
	}
}

func TestPreRunLoadConfig_NoConfigHandler(t *testing.T) {
	// Ensure configHandler is nil
	configHandler = nil

	// Execute the preRunLoadConfig function
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

	// Create a new container for each test
	container := di.NewContainer()

	// Register a mock config handler
	mockHandler := config.NewMockConfigHandler(
		func(path string) error { return nil },
		func(key string) (string, error) { return "value", nil },
		nil, nil, nil, nil,
	)
	container.Register("configHandler", mockHandler)

	// Initialize the cmd package with the container
	Initialize(container)

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

func TestInitialize_Success(t *testing.T) {
	// Create a new container for each test
	container := di.NewContainer()

	// Register a mock config handler
	mockHandler := config.NewMockConfigHandler(
		func(path string) error { return nil },
		func(key string) (string, error) { return "value", nil },
		nil, nil, nil, nil,
	)
	container.Register("configHandler", mockHandler)

	// Capture stderr
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	// Mock exitFunc to capture the exit code
	var exitCode int
	exitFunc = func(code int) {
		exitCode = code
	}

	// Initialize the cmd package with the container
	Initialize(container)

	w.Close()
	var buf bytes.Buffer
	io.Copy(&buf, r)
	os.Stderr = oldStderr
	actualErrorMsg := buf.String()

	// Verify that exitFunc was not called
	if exitCode != 0 {
		t.Errorf("exitFunc was called with code %d, expected 0", exitCode)
	}

	// Verify that no error message was printed to stderr
	if actualErrorMsg != "" {
		t.Errorf("Expected no error message, got '%s'", actualErrorMsg)
	}
}

func TestInitialize_Error(t *testing.T) {
	// Create a new container for each test
	container := di.NewContainer()

	// Do not register a config handler

	// Capture stderr
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	// Mock exitFunc to capture the exit code
	var exitCode int
	exitFunc = func(code int) {
		exitCode = code
	}

	// Initialize the cmd package with the container
	Initialize(container)

	w.Close()
	var buf bytes.Buffer
	io.Copy(&buf, r)
	os.Stderr = oldStderr
	actualErrorMsg := buf.String()

	// Verify that exitFunc was called with code 1
	if exitCode != 1 {
		t.Errorf("exitFunc was not called with code 1, got %d", exitCode)
	}

	expectedErrorMsg := "Error resolving configHandler: no instance registered with name configHandler\n"
	if !strings.Contains(actualErrorMsg, expectedErrorMsg) {
		t.Errorf("Expected error message to contain '%s', got '%s'", expectedErrorMsg, actualErrorMsg)
	}
}
