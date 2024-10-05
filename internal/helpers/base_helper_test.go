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

// Helper function to create a mock config handler
func createMockConfigHandler(getConfigValueFunc func(string) (string, error), getNestedMapFunc func(string) (map[string]interface{}, error)) *config.MockConfigHandler {
	return config.NewMockConfigHandler(
		func(path string) error { return nil },
		getConfigValueFunc,
		func(key, value string) error { return nil },
		func(path string) error { return nil },
		getNestedMapFunc,
		func(key string) ([]string, error) { return nil, nil },
	)
}

// Helper function to create a mock shell
func createMockShell(getProjectRootFunc func() (string, error)) *shell.MockShell {
	return &shell.MockShell{
		GetProjectRootFunc: getProjectRootFunc,
	}
}

func TestNewBaseHelper(t *testing.T) {
	t.Run("ErrorResolvingConfigHandler", func(t *testing.T) {
		diContainer := di.NewContainer()

		// Do not register configHandler to simulate resolution error
		diContainer.Register("shell", &shell.MockShell{})
		diContainer.Register("context", &context.MockContext{})

		_, err := NewBaseHelper(diContainer)
		if err == nil || !strings.Contains(err.Error(), "error resolving configHandler") {
			t.Fatalf("expected error resolving configHandler, got %v", err)
		}
	})

	t.Run("ErrorResolvingShell", func(t *testing.T) {
		diContainer := di.NewContainer()

		// Register configHandler but not shell to simulate resolution error
		diContainer.Register("configHandler", &config.MockConfigHandler{})
		diContainer.Register("context", &context.MockContext{})

		_, err := NewBaseHelper(diContainer)
		if err == nil || !strings.Contains(err.Error(), "error resolving shell") {
			t.Fatalf("expected error resolving shell, got %v", err)
		}
	})

	t.Run("ErrorResolvingContext", func(t *testing.T) {
		diContainer := di.NewContainer()

		// Register configHandler and shell but not context to simulate resolution error
		diContainer.Register("configHandler", &config.MockConfigHandler{})
		diContainer.Register("shell", &shell.MockShell{})

		_, err := NewBaseHelper(diContainer)
		if err == nil || !strings.Contains(err.Error(), "error resolving context") {
			t.Fatalf("expected error resolving context, got %v", err)
		}
	})
}

func TestBaseHelper_GetEnvVars(t *testing.T) {
	// Given a mock config handler and shell
	mockConfigHandler := createMockConfigHandler(
		func(key string) (string, error) {
			if key == "context" {
				return "test-context", nil
			}
			return "", errors.New("key not found")
		},
		func(key string) (map[string]interface{}, error) {
			if key == "contexts.test-context.environment" {
				return map[string]interface{}{
					"VAR1": "value1",
					"VAR2": "value2",
				}, nil
			}
			return nil, errors.New("context not found")
		},
	)
	mockShell := createMockShell(func() (string, error) {
		return "/mock/project/root", nil
	})
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
	diContainer.Register("configHandler", mockConfigHandler)
	diContainer.Register("shell", mockShell)
	diContainer.Register("context", mockContext)

	baseHelper, err := NewBaseHelper(diContainer)
	if err != nil {
		t.Fatalf("NewBaseHelper() error = %v", err)
	}

	t.Run("Success", func(t *testing.T) {
		// When calling GetEnvVars
		expectedResult := map[string]string{
			"VAR1":                 "value1",
			"VAR2":                 "value2",
			"WINDSOR_CONTEXT":      "test-context",
			"WINDSOR_PROJECT_ROOT": "/mock/project/root",
		}

		result, err := baseHelper.GetEnvVars()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

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
		mockConfigHandler := createMockConfigHandler(
			func(key string) (string, error) {
				return "", errors.New("error retrieving context")
			},
			func(key string) (map[string]interface{}, error) { return nil, nil },
		)
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
		diContainer.Register("configHandler", mockConfigHandler)
		diContainer.Register("shell", mockShell)
		diContainer.Register("context", mockContext)

		baseHelper, err := NewBaseHelper(diContainer)
		if err != nil {
			t.Fatalf("NewBaseHelper() error = %v", err)
		}

		// When calling GetEnvVars
		expectedError := errors.New("error retrieving context: error retrieving context")

		_, err = baseHelper.GetEnvVars()
		if err == nil {
			t.Fatalf("expected error %v, got nil", expectedError)
		}
		if err.Error() != expectedError.Error() {
			t.Fatalf("expected error %v, got %v", expectedError, err)
		}
	})

	t.Run("NonStringEnvVar", func(t *testing.T) {
		// Given a mock config handler with a non-string environment variable
		mockConfigHandler := createMockConfigHandler(
			func(key string) (string, error) {
				if key == "context" {
					return "test-context", nil
				}
				return "", errors.New("key not found")
			},
			func(key string) (map[string]interface{}, error) {
				if key == "contexts.test-context.environment" {
					return map[string]interface{}{
						"VAR1": 123, // Non-string value
					}, nil
				}
				return nil, errors.New("context not found")
			},
		)

		// Create DI container and register mocks
		diContainer := di.NewContainer()
		diContainer.Register("configHandler", mockConfigHandler)
		diContainer.Register("shell", mockShell)
		diContainer.Register("context", mockContext)

		baseHelper, err := NewBaseHelper(diContainer)
		if err != nil {
			t.Fatalf("NewBaseHelper() error = %v", err)
		}

		// When calling GetEnvVars
		expectedError := errors.New("non-string value found in environment variables for context test-context")

		_, err = baseHelper.GetEnvVars()
		if err == nil {
			t.Fatalf("expected error %v, got nil", expectedError)
		}
		if err.Error() != expectedError.Error() {
			t.Fatalf("expected error %v, got %v", expectedError, err)
		}
	})

	t.Run("EmptyEnvVars", func(t *testing.T) {
		// Given a mock config handler with empty environment variables
		mockConfigHandler := createMockConfigHandler(
			func(key string) (string, error) {
				if key == "context" {
					return "test-context", nil
				}
				return "", errors.New("key not found")
			},
			func(key string) (map[string]interface{}, error) {
				if key == "contexts.test-context.environment" {
					return map[string]interface{}{}, nil
				}
				return map[string]interface{}{}, nil
			},
		)

		// Create DI container and register mocks
		diContainer := di.NewContainer()
		diContainer.Register("configHandler", mockConfigHandler)
		diContainer.Register("shell", mockShell)
		diContainer.Register("context", mockContext)

		baseHelper, err := NewBaseHelper(diContainer)
		if err != nil {
			t.Fatalf("NewBaseHelper() error = %v", err)
		}

		// When calling GetEnvVars
		expectedResult := map[string]string{
			"WINDSOR_CONTEXT":      "test-context",
			"WINDSOR_PROJECT_ROOT": "/mock/project/root",
		}

		result, err := baseHelper.GetEnvVars()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

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
		mockConfigHandler := createMockConfigHandler(
			func(key string) (string, error) {
				if key == "context" {
					return "test-context", nil
				}
				return "", errors.New("key not found")
			},
			func(key string) (map[string]interface{}, error) {
				return nil, errors.New("error getting nested map")
			},
		)

		// Create DI container and register mocks
		diContainer := di.NewContainer()
		diContainer.Register("configHandler", mockConfigHandler)
		diContainer.Register("shell", mockShell)
		diContainer.Register("context", mockContext)

		baseHelper, err := NewBaseHelper(diContainer)
		if err != nil {
			t.Fatalf("NewBaseHelper() error = %v", err)
		}

		// When calling GetEnvVars
		expectedResult := map[string]string{
			"WINDSOR_CONTEXT":      "test-context",
			"WINDSOR_PROJECT_ROOT": "/mock/project/root",
		}

		result, err := baseHelper.GetEnvVars()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

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
		mockShell := createMockShell(func() (string, error) {
			return "", errors.New("failed to get project root")
		})

		// Create DI container and register mocks
		diContainer := di.NewContainer()
		diContainer.Register("configHandler", mockConfigHandler)
		diContainer.Register("shell", mockShell)
		diContainer.Register("context", mockContext)

		baseHelper, err := NewBaseHelper(diContainer)
		if err != nil {
			t.Fatalf("NewBaseHelper() error = %v", err)
		}

		// When calling GetEnvVars
		_, err = baseHelper.GetEnvVars()
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		expectedError := "error retrieving project root: failed to get project root"
		if err.Error() != expectedError {
			t.Fatalf("expected error %s, got %s", expectedError, err.Error())
		}
	})
}

func TestBaseHelper_PostEnvExec(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a BaseHelper instance
		mockConfigHandler := createMockConfigHandler(
			func(key string) (string, error) { return "", nil },
			func(key string) (map[string]interface{}, error) { return nil, nil },
		)
		mockShell := createMockShell(func() (string, error) { return "", nil })
		mockContext := &context.MockContext{
			GetContextFunc:    func() (string, error) { return "", nil },
			GetConfigRootFunc: func() (string, error) { return "", nil },
		}

		// Create DI container and register mocks
		diContainer := di.NewContainer()
		diContainer.Register("configHandler", mockConfigHandler)
		diContainer.Register("shell", mockShell)
		diContainer.Register("context", mockContext)

		baseHelper, err := NewBaseHelper(diContainer)
		if err != nil {
			t.Fatalf("NewBaseHelper() error = %v", err)
		}

		// When calling PostEnvExec
		err = baseHelper.PostEnvExec()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestBaseHelper_SetConfig(t *testing.T) {
	mockConfigHandler := &config.MockConfigHandler{}
	mockShell := &shell.MockShell{}
	mockContext := &context.MockContext{}

	// Create DI container and register mocks
	diContainer := di.NewContainer()
	diContainer.Register("configHandler", mockConfigHandler)
	diContainer.Register("context", mockContext)
	diContainer.Register("shell", mockShell)

	baseHelper, err := NewBaseHelper(diContainer)
	if err != nil {
		t.Fatalf("NewBaseHelper() error = %v", err)
	}

	t.Run("SetConfigStub", func(t *testing.T) {
		// When: SetConfig is called
		err := baseHelper.SetConfig("some_key", "some_value")

		// Then: it should return no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}
