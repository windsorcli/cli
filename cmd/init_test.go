package cmd

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/windsor-hotel/cli/internal/config"
)

func TestInitCmd_Success(t *testing.T) {
	mockHandler := &config.MockConfigHandler{
		LoadConfigFunc:     func(path string) error { return nil },
		SetConfigValueFunc: func(key, value string) error { return nil },
		SaveConfigFunc:     func(path string) error { return nil },
	}

	configHandler = mockHandler

	// Mock exitFunc to capture the exit code
	var exitCode int
	exitFunc = func(code int) {
		exitCode = code
	}

	// Add the init command
	rootCmd.AddCommand(initCmd)
	rootCmd.SetArgs([]string{"init", "testContext"})

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Execute the command
	err := rootCmd.Execute()
	w.Close()
	var buf bytes.Buffer
	io.Copy(&buf, r)
	os.Stdout = oldStdout
	actualOutput := buf.String()

	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	// Verify that exitFunc was not called
	if exitCode != 0 {
		t.Errorf("exitFunc was called with code %d, expected 0", exitCode)
	}

	// Verify the output
	expectedOutput := "Initialization successful\n"
	if actualOutput != expectedOutput {
		t.Errorf("Expected output '%s', got '%s'", expectedOutput, actualOutput)
	}

	// Remove the init command after the test
	rootCmd.RemoveCommand(initCmd)
}

func TestInitCmd_SetConfigValueError(t *testing.T) {
	mockHandler := &config.MockConfigHandler{
		SetConfigValueFunc: func(key, value string) error { return errors.New("set config value error") },
	}

	configHandler = mockHandler

	// Mock exitFunc to capture the exit code
	var exitCode int
	exitFunc = func(code int) {
		exitCode = code
	}

	// Capture stderr
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	// Add the init command
	rootCmd.AddCommand(initCmd)
	rootCmd.SetArgs([]string{"init", "testContext"})

	// Execute the command
	Execute()

	w.Close()
	var buf bytes.Buffer
	io.Copy(&buf, r)
	os.Stderr = oldStderr
	actualErrorMsg := buf.String()

	if exitCode != 1 {
		t.Errorf("exitFunc was not called with code 1, got %d", exitCode)
	}

	if !strings.Contains(actualErrorMsg, "set config value error") {
		t.Errorf("Expected error message to contain 'set config value error', got '%s'", actualErrorMsg)
	}

	// Remove the init command after the test
	rootCmd.RemoveCommand(initCmd)
}

func TestInitCmd_SaveConfigError(t *testing.T) {
	mockHandler := &config.MockConfigHandler{
		SaveConfigFunc: func(path string) error { return errors.New("save config error") },
	}

	configHandler = mockHandler

	// Mock exitFunc to capture the exit code
	var exitCode int
	exitFunc = func(code int) {
		exitCode = code
	}

	// Capture stderr
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	// Add the init command
	rootCmd.AddCommand(initCmd)
	rootCmd.SetArgs([]string{"init", "testContext"})

	// Execute the command
	Execute()

	w.Close()
	var buf bytes.Buffer
	io.Copy(&buf, r)
	os.Stderr = oldStderr
	actualErrorMsg := buf.String()

	if exitCode != 1 {
		t.Errorf("exitFunc was not called with code 1, got %d", exitCode)
	}

	if !strings.Contains(actualErrorMsg, "save config error") {
		t.Errorf("Expected error message to contain 'save config error', got '%s'", actualErrorMsg)
	}

	// Remove the init command after the test
	rootCmd.RemoveCommand(initCmd)
}
