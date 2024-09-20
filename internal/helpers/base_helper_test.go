package helpers

import (
	"errors"
	"testing"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/shell"
)

// Helper function to create a mock config handler
func createMockConfigHandler(getContextFunc func(string) (string, error), getNestedMapFunc func(string) (map[string]interface{}, error)) *config.MockConfigHandler {
	return config.NewMockConfigHandler(
		func(path string) error { return nil },
		getContextFunc,
		func(key string, value string) error { return nil }, // SetConfigValueFunc
		func(path string) error { return nil },              // SaveConfigFunc
		getNestedMapFunc,
		func(key string) ([]string, error) { return nil, nil }, // ListKeysFunc
	)
}

// Helper function to create a mock shell
func createMockShell(getProjectRootFunc func() (string, error)) *shell.MockShell {
	return &shell.MockShell{
		GetProjectRootFunc: getProjectRootFunc,
	}
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
	baseHelper := NewBaseHelper(mockConfigHandler, mockShell)

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
		baseHelper := NewBaseHelper(mockConfigHandler, mockShell)

		// When calling GetEnvVars
		expectedError := errors.New("error retrieving context: error retrieving context")

		_, err := baseHelper.GetEnvVars()
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
		baseHelper := NewBaseHelper(mockConfigHandler, mockShell)

		// When calling GetEnvVars
		expectedError := errors.New("non-string value found in environment variables for context test-context")

		_, err := baseHelper.GetEnvVars()
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
				return nil, errors.New("context not found")
			},
		)
		baseHelper := NewBaseHelper(mockConfigHandler, mockShell)

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
		baseHelper := NewBaseHelper(mockConfigHandler, mockShell)

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
		baseHelper := NewBaseHelper(mockConfigHandler, mockShell)

		// When calling GetEnvVars
		_, err := baseHelper.GetEnvVars()
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		expectedError := "error retrieving project root: failed to get project root"
		if err.Error() != expectedError {
			t.Fatalf("expected error %s, got %s", expectedError, err.Error())
		}
	})
}
