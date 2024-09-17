package cmd

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/helpers"
)

// MockBaseHelper is a mock implementation of the BaseHelper interface
type MockBaseHelper struct {
	helpers.Helper
	GetEnvVarsFunc   func() (map[string]string, error)
	PrintEnvVarsFunc func() error
}

func (m *MockBaseHelper) GetEnvVars() (map[string]string, error) {
	if m.GetEnvVarsFunc != nil {
		return m.GetEnvVarsFunc()
	}
	return nil, nil
}

func (m *MockBaseHelper) PrintEnvVars() error {
	if m.PrintEnvVarsFunc != nil {
		return m.PrintEnvVarsFunc()
	}
	return nil
}

func TestEnvCmd_Success(t *testing.T) {
	mockBaseHelper := &MockBaseHelper{
		PrintEnvVarsFunc: func() error {
			return nil
		},
	}

	container := &di.Container{
		ConfigHandler: &config.MockConfigHandler{},
		BaseHelper:    mockBaseHelper,
	}

	Initialize(container)

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	rootCmd.SetArgs([]string{"env"})
	err := rootCmd.Execute()

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	io.Copy(&buf, r)

	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	expectedOutput := "" // Adjust this based on what you expect to be printed
	actualOutput := buf.String()
	if actualOutput != expectedOutput {
		t.Errorf("Expected output '%s', got '%s'", expectedOutput, actualOutput)
	}
}

func TestEnvCmd_Error(t *testing.T) {
	mockBaseHelper := &MockBaseHelper{
		PrintEnvVarsFunc: func() error {
			return errors.New("mock error")
		},
	}

	container := &di.Container{
		ConfigHandler: &config.MockConfigHandler{},
		BaseHelper:    mockBaseHelper,
	}

	Initialize(container)

	// Capture stderr
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	rootCmd.SetArgs([]string{"env"})
	err := rootCmd.Execute()

	w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	io.Copy(&buf, r)

	if err == nil {
		t.Fatalf("Expected error, got nil")
	}

	expectedError := "Error printing environment variables: mock error"
	actualError := buf.String()
	if !strings.Contains(actualError, expectedError) {
		t.Errorf("Expected error message to contain '%s', got '%s'", expectedError, actualError)
	}
}
