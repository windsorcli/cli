package helpers

import (
	"errors"
	"strings"
	"testing"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/shell"
)

// Helper function for error assertion
func assertError(t *testing.T, err error, shouldError bool) {
	if shouldError && err == nil {
		t.Errorf("Expected error, got nil")
	} else if !shouldError && err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestBaseHelper(t *testing.T) {
	t.Run("NewBaseHelper", func(t *testing.T) {
		t.Run("ErrorResolvingConfigHandler", func(t *testing.T) {
			diContainer := di.NewContainer()

			// Given a DI container without cliConfigHandler registered
			diContainer.Register("shell", &shell.MockShell{})
			diContainer.Register("context", &context.MockContext{})

			// When creating a new BaseHelper
			_, err := NewBaseHelper(diContainer)

			// Then an error should be returned
			assertError(t, err, true)
			if err == nil || !strings.Contains(err.Error(), "error resolving cliConfigHandler") {
				t.Fatalf("expected error resolving cliConfigHandler, got %v", err)
			}
		})

		t.Run("ErrorResolvingShell", func(t *testing.T) {
			diContainer := di.NewContainer()

			// Given a DI container with cliConfigHandler registered but without shell registered
			diContainer.Register("cliConfigHandler", &config.MockConfigHandler{})
			diContainer.Register("context", &context.MockContext{})

			// When creating a new BaseHelper
			_, err := NewBaseHelper(diContainer)

			// Then an error should be returned
			assertError(t, err, true)
			if err == nil || !strings.Contains(err.Error(), "error resolving shell") {
				t.Fatalf("expected error resolving shell, got %v", err)
			}
		})

		t.Run("ErrorResolvingContext", func(t *testing.T) {
			diContainer := di.NewContainer()

			// Given a DI container with cliConfigHandler and shell registered but without context registered
			diContainer.Register("cliConfigHandler", &config.MockConfigHandler{})
			diContainer.Register("shell", &shell.MockShell{})

			// When creating a new BaseHelper
			_, err := NewBaseHelper(diContainer)

			// Then an error should be returned
			assertError(t, err, true)
			if err == nil || !strings.Contains(err.Error(), "error resolving context") {
				t.Fatalf("expected error resolving context, got %v", err)
			}
		})
	})

	t.Run("GetEnvVars", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			// Given a mock config handler and shell
			mockConfigHandler := config.NewMockConfigHandler(nil, nil, nil, nil, nil, nil)
			mockConfigHandler.GetConfigValueFunc = func(key string) (string, error) {
				if key == "context" {
					return "test-context", nil
				}
				return "", errors.New("key not found")
			}
			mockConfigHandler.GetNestedMapFunc = func(key string) (map[string]interface{}, error) {
				if key == "contexts.test-context.environment" {
					return map[string]interface{}{
						"VAR1": "value1",
						"VAR2": "value2",
					}, nil
				}
				return nil, errors.New("context not found")
			}

			mockShell := &shell.MockShell{}
			mockShell.GetProjectRootFunc = func() (string, error) {
				return "/mock/project/root", nil
			}

			mockContext := &context.MockContext{
				GetContextFunc: func() (string, error) {
					return "test-context", nil
				},
				GetConfigRootFunc: func() (string, error) {
					return "/mock/project/root/contexts/test-context", nil
				},
			}

			// Create DI container and register mocks
			diContainer := di.NewContainer()
			diContainer.Register("cliConfigHandler", mockConfigHandler)
			diContainer.Register("shell", mockShell)
			diContainer.Register("context", mockContext)

			// When creating a new BaseHelper
			baseHelper, err := NewBaseHelper(diContainer)
			if err != nil {
				t.Fatalf("NewBaseHelper() error = %v", err)
			}

			// And calling GetEnvVars
			expectedResult := map[string]string{
				"VAR1":                 "value1",
				"VAR2":                 "value2",
				"WINDSOR_CONTEXT":      "test-context",
				"WINDSOR_PROJECT_ROOT": "/mock/project/root",
			}

			result, err := baseHelper.GetEnvVars()
			assertError(t, err, false)

			// Then the result should match the expected result
			if len(result) != len(expectedResult) {
				t.Fatalf("expected map length %d, got %d", len(expectedResult), len(result))
			}

			for k, v := range expectedResult {
				if result[k] != v {
					t.Fatalf("expected key-value pair %s:%s, got %s:%s", k, v, k, result[k])
				}
			}
		})

		t.Run("ErrorRetrievingContext", func(t *testing.T) {
			// Given a mock config handler that returns an error for context
			mockConfigHandler := config.NewMockConfigHandler(nil, nil, nil, nil, nil, nil)
			mockConfigHandler.GetConfigValueFunc = func(key string) (string, error) {
				return "", errors.New("error retrieving context")
			}
			mockContext := &context.MockContext{
				GetContextFunc: func() (string, error) {
					return "", errors.New("error retrieving context")
				},
				GetConfigRootFunc: func() (string, error) {
					return "/mock/project/root/contexts/test-context", nil
				},
			}

			// Create DI container and register mocks
			diContainer := di.NewContainer()
			diContainer.Register("cliConfigHandler", mockConfigHandler)
			diContainer.Register("shell", &shell.MockShell{
				GetProjectRootFunc: func() (string, error) {
					return "/mock/project/root", nil
				},
			})
			diContainer.Register("context", mockContext)

			// When creating a new BaseHelper
			baseHelper, err := NewBaseHelper(diContainer)
			if err != nil {
				t.Fatalf("NewBaseHelper() error = %v", err)
			}

			// And calling GetEnvVars
			expectedError := errors.New("error retrieving context: error retrieving context")

			_, err = baseHelper.GetEnvVars()
			// Then an error should be returned
			assertError(t, err, true)
			if err.Error() != expectedError.Error() {
				t.Fatalf("expected error %v, got %v", expectedError, err)
			}
		})

		t.Run("NonStringEnvVar", func(t *testing.T) {
			// Given a mock config handler with a non-string environment variable
			mockConfigHandler := config.NewMockConfigHandler(nil, nil, nil, nil, nil, nil)
			mockConfigHandler.GetConfigValueFunc = func(key string) (string, error) {
				if key == "context" {
					return "test-context", nil
				}
				return "", errors.New("key not found")
			}
			mockConfigHandler.GetNestedMapFunc = func(key string) (map[string]interface{}, error) {
				if key == "contexts.test-context.environment" {
					return map[string]interface{}{
						"VAR1": 123, // Non-string value
					}, nil
				}
				return nil, errors.New("context not found")
			}

			// Create DI container and register mocks
			diContainer := di.NewContainer()
			diContainer.Register("cliConfigHandler", mockConfigHandler)
			diContainer.Register("shell", &shell.MockShell{
				GetProjectRootFunc: func() (string, error) {
					return "/mock/project/root", nil
				},
			})
			diContainer.Register("context", &context.MockContext{
				GetContextFunc: func() (string, error) {
					return "test-context", nil
				},
				GetConfigRootFunc: func() (string, error) {
					return "/mock/project/root/contexts/test-context", nil
				},
			})

			// When creating a new BaseHelper
			baseHelper, err := NewBaseHelper(diContainer)
			if err != nil {
				t.Fatalf("NewBaseHelper() error = %v", err)
			}

			// And calling GetEnvVars
			expectedError := errors.New("non-string value found in environment variables for context test-context")

			_, err = baseHelper.GetEnvVars()
			// Then an error should be returned
			assertError(t, err, true)
			if err.Error() != expectedError.Error() {
				t.Fatalf("expected error %v, got %v", expectedError, err)
			}
		})

		t.Run("EmptyEnvVars", func(t *testing.T) {
			// Given a mock config handler with empty environment variables
			mockConfigHandler := config.NewMockConfigHandler(nil, nil, nil, nil, nil, nil)
			mockConfigHandler.GetConfigValueFunc = func(key string) (string, error) {
				if key == "context" {
					return "test-context", nil
				}
				return "", errors.New("key not found")
			}
			mockConfigHandler.GetNestedMapFunc = func(key string) (map[string]interface{}, error) {
				if key == "contexts.test-context.environment" {
					return map[string]interface{}{}, nil
				}
				return map[string]interface{}{}, nil
			}

			// Create DI container and register mocks
			diContainer := di.NewContainer()
			diContainer.Register("cliConfigHandler", mockConfigHandler)
			diContainer.Register("shell", &shell.MockShell{
				GetProjectRootFunc: func() (string, error) {
					return "/mock/project/root", nil
				},
			})
			diContainer.Register("context", &context.MockContext{
				GetContextFunc: func() (string, error) {
					return "test-context", nil
				},
				GetConfigRootFunc: func() (string, error) {
					return "/mock/project/root/contexts/test-context", nil
				},
			})

			// When creating a new BaseHelper
			baseHelper, err := NewBaseHelper(diContainer)
			if err != nil {
				t.Fatalf("NewBaseHelper() error = %v", err)
			}

			// And calling GetEnvVars
			expectedResult := map[string]string{
				"WINDSOR_CONTEXT":      "test-context",
				"WINDSOR_PROJECT_ROOT": "/mock/project/root",
			}

			result, err := baseHelper.GetEnvVars()
			assertError(t, err, false)

			// Then the result should match the expected result
			if len(result) != len(expectedResult) {
				t.Fatalf("expected map length %d, got %d", len(expectedResult), len(result))
			}

			for k, v := range expectedResult {
				if result[k] != v {
					t.Fatalf("expected key-value pair %s:%s, got %s:%s", k, v, k, result[k])
				}
			}
		})

		t.Run("GetNestedMapError", func(t *testing.T) {
			// Given a mock config handler that returns an error for GetNestedMap
			mockConfigHandler := config.NewMockConfigHandler(nil, nil, nil, nil, nil, nil)
			mockConfigHandler.GetConfigValueFunc = func(key string) (string, error) {
				if key == "context" {
					return "test-context", nil
				}
				return "", errors.New("key not found")
			}
			mockConfigHandler.GetNestedMapFunc = func(key string) (map[string]interface{}, error) {
				return nil, errors.New("error getting nested map")
			}

			// Create DI container and register mocks
			diContainer := di.NewContainer()
			diContainer.Register("cliConfigHandler", mockConfigHandler)
			diContainer.Register("shell", &shell.MockShell{
				GetProjectRootFunc: func() (string, error) {
					return "/mock/project/root", nil
				},
			})
			diContainer.Register("context", &context.MockContext{
				GetContextFunc: func() (string, error) {
					return "test-context", nil
				},
				GetConfigRootFunc: func() (string, error) {
					return "/mock/project/root/contexts/test-context", nil
				},
			})

			// When creating a new BaseHelper
			baseHelper, err := NewBaseHelper(diContainer)
			if err != nil {
				t.Fatalf("NewBaseHelper() error = %v", err)
			}

			// And calling GetEnvVars
			expectedResult := map[string]string{
				"WINDSOR_CONTEXT":      "test-context",
				"WINDSOR_PROJECT_ROOT": "/mock/project/root",
			}

			result, err := baseHelper.GetEnvVars()
			assertError(t, err, false)

			// Then the result should match the expected result
			if len(result) != len(expectedResult) {
				t.Fatalf("expected map length %d, got %d", len(expectedResult), len(result))
			}

			for k, v := range expectedResult {
				if result[k] != v {
					t.Fatalf("expected key-value pair %s:%s, got %s:%s", k, v, k, result[k])
				}
			}
		})

		t.Run("ProjectRootError", func(t *testing.T) {
			// Given a mock shell that returns an error for GetProjectRoot
			mockShell := &shell.MockShell{}
			mockShell.GetProjectRootFunc = func() (string, error) {
				return "", errors.New("failed to get project root")
			}

			// Create DI container and register mocks
			diContainer := di.NewContainer()
			diContainer.Register("cliConfigHandler", config.NewMockConfigHandler(
				nil, nil, nil, nil, nil, nil,
			))
			diContainer.Register("shell", mockShell)
			diContainer.Register("context", &context.MockContext{
				GetContextFunc: func() (string, error) {
					return "test-context", nil
				},
				GetConfigRootFunc: func() (string, error) {
					return "/mock/project/root/contexts/test-context", nil
				},
			})

			// When creating a new BaseHelper
			baseHelper, err := NewBaseHelper(diContainer)
			if err != nil {
				t.Fatalf("NewBaseHelper() error = %v", err)
			}

			// And calling GetEnvVars
			_, err = baseHelper.GetEnvVars()
			// Then an error should be returned
			assertError(t, err, true)

			expectedError := "error retrieving project root: failed to get project root"
			if err.Error() != expectedError {
				t.Fatalf("expected error %s, got %s", expectedError, err.Error())
			}
		})
	})

	t.Run("PostEnvExec", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			// Given a BaseHelper instance
			mockConfigHandler := config.NewMockConfigHandler(nil, nil, nil, nil, nil, nil)
			mockShell := &shell.MockShell{}
			mockContext := &context.MockContext{
				GetContextFunc:    func() (string, error) { return "", nil },
				GetConfigRootFunc: func() (string, error) { return "", nil },
			}

			// Create DI container and register mocks
			diContainer := di.NewContainer()
			diContainer.Register("cliConfigHandler", mockConfigHandler)
			diContainer.Register("shell", mockShell)
			diContainer.Register("context", mockContext)

			// When creating a new BaseHelper
			baseHelper, err := NewBaseHelper(diContainer)
			if err != nil {
				t.Fatalf("NewBaseHelper() error = %v", err)
			}

			// And calling PostEnvExec
			err = baseHelper.PostEnvExec()

			// Then no error should be returned
			assertError(t, err, false)
		})
	})

	t.Run("SetConfig", func(t *testing.T) {
		t.Run("SetConfigStub", func(t *testing.T) {
			// Given a BaseHelper instance
			mockConfigHandler := &config.MockConfigHandler{}
			mockShell := &shell.MockShell{}
			mockContext := &context.MockContext{}

			// Create DI container and register mocks
			diContainer := di.NewContainer()
			diContainer.Register("cliConfigHandler", mockConfigHandler)
			diContainer.Register("context", mockContext)
			diContainer.Register("shell", mockShell)

			// When creating a new BaseHelper
			baseHelper, err := NewBaseHelper(diContainer)
			if err != nil {
				t.Fatalf("NewBaseHelper() error = %v", err)
			}

			// And calling SetConfig
			err = baseHelper.SetConfig("some_key", "some_value")

			// Then no error should be returned
			assertError(t, err, false)
		})
	})

	t.Run("GetContainerConfig", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			// Given a mock config handler, shell, and context
			mockConfigHandler := &config.MockConfigHandler{}
			mockShell := &shell.MockShell{}
			mockContext := &context.MockContext{}

			// Create DI container and register mocks
			diContainer := di.NewContainer()
			diContainer.Register("cliConfigHandler", mockConfigHandler)
			diContainer.Register("shell", mockShell)
			diContainer.Register("context", mockContext)

			// When creating a new BaseHelper
			baseHelper, err := NewBaseHelper(diContainer)
			if err != nil {
				t.Fatalf("NewBaseHelper() error = %v", err)
			}

			// And calling GetContainerConfig
			containerConfig, err := baseHelper.GetContainerConfig()
			assertError(t, err, false)

			// Then the result should be nil as per the stub implementation
			if containerConfig != nil {
				t.Errorf("expected nil, got %v", containerConfig)
			}
		})
	})
}
