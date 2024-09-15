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

func TestEnvCmd_Success_Linux(t *testing.T) {
	mockHandler := &config.MockConfigHandler{
		GetConfigValueFunc: func(key string) (string, error) {
			if key == "context" {
				return "test-context", nil
			}
			return "", errors.New("key not found")
		},
	}

	configHandler = mockHandler

	// Mock the OS
	originalGOOS := goos
	goos = "linux"
	defer func() { goos = originalGOOS }()

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Add the env command
	rootCmd.AddCommand(envCmd)
	rootCmd.SetArgs([]string{"env"})

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

	// Verify the output
	expectedOutput := "export WINDSORCONTEXT=test-context\n"
	if actualOutput != expectedOutput {
		t.Errorf("Expected output '%s', got '%s'", expectedOutput, actualOutput)
	}

	// Remove the env command after the test
	rootCmd.RemoveCommand(envCmd)
}

func TestEnvCmd_Success_Windows(t *testing.T) {
	mockHandler := &config.MockConfigHandler{
		GetConfigValueFunc: func(key string) (string, error) {
			if key == "context" {
				return "test-context", nil
			}
			return "", errors.New("key not found")
		},
	}

	configHandler = mockHandler

	// Mock the OS
	originalGOOS := goos
	goos = "windows"
	defer func() { goos = originalGOOS }()

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Add the env command
	rootCmd.AddCommand(envCmd)
	rootCmd.SetArgs([]string{"env"})

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

	// Verify the output
	expectedOutput := "set WINDSORCONTEXT=test-context\n"
	if actualOutput != expectedOutput {
		t.Errorf("Expected output '%s', got '%s'", expectedOutput, actualOutput)
	}

	// Remove the env command after the test
	rootCmd.RemoveCommand(envCmd)
}

func TestEnvCmd_GetConfigValueError(t *testing.T) {
	mockHandler := &config.MockConfigHandler{
		GetConfigValueFunc: func(key string) (string, error) {
			return "", errors.New("get config value error")
		},
	}

	configHandler = mockHandler

	// Capture stderr
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	// Add the env command
	rootCmd.AddCommand(envCmd)
	rootCmd.SetArgs([]string{"env"})

	// Execute the command
	err := rootCmd.Execute()
	w.Close()
	var buf bytes.Buffer
	io.Copy(&buf, r)
	os.Stderr = oldStderr
	actualErrorMsg := buf.String()

	if err == nil {
		t.Fatalf("Expected error, got nil")
	}

	if !strings.Contains(actualErrorMsg, "get config value error") {
		t.Errorf("Expected error message to contain 'get config value error', got '%s'", actualErrorMsg)
	}

	// Remove the env command after the test
	rootCmd.RemoveCommand(envCmd)
}
