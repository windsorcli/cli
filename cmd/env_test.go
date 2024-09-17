package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/helpers"
)

// MockContainer is a mock implementation of the DI container
type MockContainer struct {
	di.RealContainer
	resolveAllError error
}

func (m *MockContainer) ResolveAll(targetType interface{}) ([]interface{}, error) {
	if m.resolveAllError != nil {
		return nil, m.resolveAllError
	}
	return m.RealContainer.ResolveAll(targetType)
}

func setupTestEnvCmd(mockHandler config.ConfigHandler, mockHelpers []interface{}, resolveAllError error) (*MockContainer, func() (string, error)) {
	// Create a new mock DI container
	container := &MockContainer{
		RealContainer:   *di.NewContainer(),
		resolveAllError: resolveAllError,
	}

	// Register the mock config handler
	container.Register("configHandler", mockHandler)

	// Register the mock helpers
	for i, helper := range mockHelpers {
		container.Register(fmt.Sprintf("mockHelper%d", i), helper)
	}

	// Initialize the cmd package with the container
	Initialize(container)

	// Capture stdout
	oldOutput := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	return container, func() (string, error) {
		w.Close()
		var buf bytes.Buffer
		io.Copy(&buf, r)
		os.Stdout = oldOutput
		return buf.String(), nil
	}
}

func TestEnvCmd_Success(t *testing.T) {
	mockHandler := config.NewMockConfigHandler(
		func(path string) error { return nil },
		func(key string) (string, error) {
			if key == "context" {
				return "test-context", nil
			}
			return "", errors.New("key not found")
		},
		nil, // SetConfigValueFunc
		nil, // SaveConfigFunc
		func(key string) (map[string]interface{}, error) {
			if key == "contexts.test-context.environment" {
				return map[string]interface{}{
					"VAR1": "value1",
					"VAR2": "value2",
				}, nil
			}
			return nil, errors.New("context not found")
		},
		nil, // ListKeysFunc
	)
	mockHelpers := []interface{}{
		helpers.NewMockHelper(
			nil, // GetEnvVarsFunc
			func() error {
				fmt.Println("export VAR1='value1'")
				fmt.Println("export VAR2='value2'")
				return nil
			},
		),
	}
	expectedOutput := "export VAR1='value1'\nexport VAR2='value2'\n"

	_, getOutput := setupTestEnvCmd(mockHandler, mockHelpers, nil)

	// Execute the env command
	rootCmd.SetArgs([]string{"env"})
	err := rootCmd.Execute()
	actualOutput, _ := getOutput()

	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if actualOutput != expectedOutput {
		t.Errorf("Expected output '%s', got '%s'", expectedOutput, actualOutput)
	}
}

func TestEnvCmd_ResolveAllError(t *testing.T) {
	mockHandler := config.NewMockConfigHandler(
		func(path string) error { return nil },
		func(key string) (string, error) { return "", nil },
		nil, // SetConfigValueFunc
		nil, // SaveConfigFunc
		nil, // GetNestedMapFunc
		nil, // ListKeysFunc
	)
	mockHelpers := []interface{}{}
	expectedError := "Error resolving helpers: mock resolve all error"

	_, _ = setupTestEnvCmd(mockHandler, mockHelpers, errors.New("mock resolve all error"))

	// Capture stderr
	oldOutput := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	// Execute the env command
	rootCmd.SetArgs([]string{"env"})
	err := rootCmd.Execute()
	w.Close()
	var buf bytes.Buffer
	io.Copy(&buf, r)
	os.Stderr = oldOutput

	if err == nil {
		t.Fatalf("Expected error, got nil")
	}
	if !strings.Contains(err.Error(), expectedError) {
		t.Errorf("Expected error message to contain '%s', got '%s'", expectedError, err.Error())
	}
}

func TestEnvCmd_PrintEnvVarsError(t *testing.T) {
	mockHandler := config.NewMockConfigHandler(
		func(path string) error { return nil },
		func(key string) (string, error) { return "", nil },
		nil, // SetConfigValueFunc
		nil, // SaveConfigFunc
		nil, // GetNestedMapFunc
		nil, // ListKeysFunc
	)
	mockHelpers := []interface{}{
		helpers.NewMockHelper(
			nil, // GetEnvVarsFunc
			func() error {
				return errors.New("mock print env vars error")
			},
		),
	}
	expectedError := "Error printing environment variables: mock print env vars error"

	_, _ = setupTestEnvCmd(mockHandler, mockHelpers, nil)

	// Capture stderr
	oldOutput := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	// Execute the env command
	rootCmd.SetArgs([]string{"env"})
	err := rootCmd.Execute()
	w.Close()
	var buf bytes.Buffer
	io.Copy(&buf, r)
	os.Stderr = oldOutput

	if err == nil {
		t.Fatalf("Expected error, got nil")
	}
	if !strings.Contains(err.Error(), expectedError) {
		t.Errorf("Expected error message to contain '%s', got '%s'", expectedError, err.Error())
	}
}
