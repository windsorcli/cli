package cmd

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/helpers"
	"github.com/windsor-hotel/cli/internal/shell"
)

// Helper function to create a new container and register mock handlers
func setupEnvTestContainer(mockHandler config.ConfigHandler, mockHelpers []interface{}, resolveAllError error) *di.MockContainer {
	// Create a new mock DI container
	container := di.NewMockContainer()
	if resolveAllError != nil {
		container.SetResolveAllError(resolveAllError)
	}

	// Register the mock config handler
	container.Register("configHandler", mockHandler)

	// Register the mock helpers
	for i, helper := range mockHelpers {
		container.Register(fmt.Sprintf("mockHelper%d", i), helper)
	}

	// Register the mock shell
	mockShell, _ := shell.NewMockShell("unix")
	container.Register("shell", mockShell)

	// Initialize the cmd package with the container
	Initialize(container)

	return container
}

func TestEnvCmd(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a valid config handler and helpers
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
			}, &shell.MockShell{}),
		}

		setupEnvTestContainer(mockHandler, mockHelpers, nil)

		// When the env command is executed
		output := captureStdout(func() {
			rootCmd.SetArgs([]string{"env"})
			err := rootCmd.Execute()
			if err != nil {
				t.Fatalf("rootCmd.Execute() error = %v", err)
			}
		})

		// Then the output should be as expected
		expectedOutput := "VAR1=value1\nVAR2=value2\n"
		if output != expectedOutput {
			t.Errorf("Expected output '%s', got '%s'", expectedOutput, output)
		}
	})

	t.Run("HelperError", func(t *testing.T) {
		// Given a helper that returns an error
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
			}, &shell.MockShell{}),
		}
		expectedError := "Error getting environment variables: mock print env vars error"

		setupEnvTestContainer(mockHandler, mockHelpers, nil)

		// When the env command is executed
		rootCmd.SetArgs([]string{"env"})
		err := rootCmd.Execute()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), expectedError) {
			t.Errorf("Expected error message to contain '%s', got '%s'", expectedError, err.Error())
		}
	})

	t.Run("ResolveAllError", func(t *testing.T) {
		// Given a resolve all error
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

		setupEnvTestContainer(mockHandler, mockHelpers, errors.New("mock resolve all error"))

		// When the env command is executed
		rootCmd.SetArgs([]string{"env"})
		err := rootCmd.Execute()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), expectedError) {
			t.Errorf("Expected error message to contain '%s', got '%s'", expectedError, err.Error())
		}
	})

	t.Run("ResolveShellError", func(t *testing.T) {
		// Given a shell resolve error
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
			}, &shell.MockShell{}),
		}
		expectedError := "Error resolving shell: no instance registered with name shell"

		// Create a new mock DI container with an error for resolving the shell
		container := di.NewMockContainer()
		container.Register("configHandler", mockHandler)
		for i, helper := range mockHelpers {
			container.Register(fmt.Sprintf("mockHelper%d", i), helper)
		}

		// Do not register the shell to simulate the error
		Initialize(container)

		// When the env command is executed
		rootCmd.SetArgs([]string{"env"})
		err := rootCmd.Execute()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), expectedError) {
			t.Errorf("Expected error message to contain '%s', got '%s'", expectedError, err.Error())
		}
	})
}
