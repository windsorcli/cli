package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/helpers"
)

// MockContainer is a mock implementation of the DI container
type MockContainer struct {
	di.DIContainer
	resolveAllError error
}

func (m *MockContainer) ResolveAll(targetType interface{}) ([]interface{}, error) {
	if m.resolveAllError != nil {
		return nil, m.resolveAllError
	}
	return m.DIContainer.ResolveAll(targetType)
}

func (m *MockContainer) Resolve(name string) (interface{}, error) {
	instance, err := m.DIContainer.Resolve(name)
	if err != nil {
		return nil, fmt.Errorf("no instance registered with name %s", name)
	}
	return instance, nil
}

type MockShell struct {
	PrintEnvVarsFunc func(envVars map[string]string)
	output           *bytes.Buffer
}

func (m *MockShell) PrintEnvVars(envVars map[string]string) {
	if m.PrintEnvVarsFunc != nil {
		m.PrintEnvVarsFunc(envVars)
	} else {
		keys := make([]string, 0, len(envVars))
		for key := range envVars {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			fmt.Fprintf(m.output, "export %s=\"%s\"\n", key, envVars[key])
		}
	}
}

func (m *MockShell) GetProjectRoot() (string, error) {
	return "", nil
}

func setupTestEnvCmd(mockHandler config.ConfigHandler, mockHelpers []interface{}, resolveAllError error) (*MockContainer, func() (string, error)) {
	// Create a new mock DI container
	container := &MockContainer{
		DIContainer:     *di.NewContainer(),
		resolveAllError: resolveAllError,
	}

	// Register the mock config handler
	container.Register("configHandler", mockHandler)

	// Register the mock helpers
	for i, helper := range mockHelpers {
		container.Register(fmt.Sprintf("mockHelper%d", i), helper)
	}

	// Register the mock shell
	mockShell := &MockShell{output: new(bytes.Buffer)}
	container.Register("shell", mockShell)

	// Initialize the cmd package with the container
	Initialize(container)

	return container, func() (string, error) {
		return mockShell.output.String(), nil
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
		helpers.NewMockHelper(func() (map[string]string, error) {
			return map[string]string{
				"VAR1": "value1",
				"VAR2": "value2",
			}, nil
		}),
	}

	_, getOutput := setupTestEnvCmd(mockHandler, mockHelpers, nil)

	// Execute the env command
	rootCmd.SetArgs([]string{"env"})
	err := rootCmd.Execute()
	actualOutput, _ := getOutput()

	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	expectedOutput := "export VAR1=\"value1\"\nexport VAR2=\"value2\"\n"
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
		helpers.NewMockHelper(func() (map[string]string, error) {
			return nil, errors.New("mock print env vars error")
		}),
	}
	expectedError := "Error getting environment variables: mock print env vars error"

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

func TestEnvCmd_ResolveShellError(t *testing.T) {
	mockHandler := config.NewMockConfigHandler(
		func(path string) error { return nil },
		func(key string) (string, error) { return "", nil },
		nil, // SetConfigValueFunc
		nil, // SaveConfigFunc
		nil, // GetNestedMapFunc
		nil, // ListKeysFunc
	)
	mockHelpers := []interface{}{
		helpers.NewMockHelper(func() (map[string]string, error) {
			return map[string]string{
				"VAR1": "value1",
				"VAR2": "value2",
			}, nil
		}),
	}
	expectedError := "Error resolving shell: no instance registered with name shell"

	// Create a new mock DI container with an error for resolving the shell
	container := &MockContainer{
		DIContainer:     *di.NewContainer(),
		resolveAllError: nil,
	}
	container.Register("configHandler", mockHandler)
	for i, helper := range mockHelpers {
		container.Register(fmt.Sprintf("mockHelper%d", i), helper)
	}

	// Do not register the shell to simulate the error
	Initialize(container)

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
