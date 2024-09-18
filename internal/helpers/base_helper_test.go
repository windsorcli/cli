package helpers

import (
	"errors"
	"sort"
	"testing"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/shell"
)

func TestBaseHelper_GetEnvVars_Success(t *testing.T) {
	mockConfigHandler := config.NewMockConfigHandler(
		func(path string) error { return nil },
		func(key string) (string, error) {
			if key == "context" {
				return "test-context", nil
			}
			return "", errors.New("key not found")
		},
		func(key string, value string) error { return nil }, // SetConfigValueFunc
		func(path string) error { return nil },              // SaveConfigFunc
		func(key string) (map[string]interface{}, error) {
			if key == "contexts.test-context.environment" {
				return map[string]interface{}{
					"VAR1": "value1",
					"VAR2": "value2",
				}, nil
			}
			return nil, errors.New("context not found")
		},
		func(key string) ([]string, error) { return nil, nil }, // ListKeysFunc
	)
	mockShell := &shell.MockShell{
		GetProjectRootFunc: func() (string, error) {
			return "/mock/project/root", nil
		},
	}
	baseHelper := NewBaseHelper(mockConfigHandler, mockShell)

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

	// Sort and compare maps
	if len(result) != len(expectedResult) {
		t.Fatalf("expected map length %d, got %d", len(expectedResult), len(result))
	}

	resultKeys := make([]string, 0, len(result))
	expectedKeys := make([]string, 0, len(expectedResult))

	for k := range result {
		resultKeys = append(resultKeys, k)
	}
	for k := range expectedResult {
		expectedKeys = append(expectedKeys, k)
	}

	sort.Strings(resultKeys)
	sort.Strings(expectedKeys)

	for i, k := range resultKeys {
		if k != expectedKeys[i] || result[k] != expectedResult[expectedKeys[i]] {
			t.Fatalf("expected key-value pair %s:%s, got %s:%s", k, expectedResult[expectedKeys[i]], k, result[k])
		}
	}
}

func TestBaseHelper_GetEnvVars_ErrorRetrievingContext(t *testing.T) {
	mockConfigHandler := config.NewMockConfigHandler(
		func(path string) error { return nil },
		func(key string) (string, error) {
			return "", errors.New("error retrieving context")
		},
		func(key string, value string) error { return nil },                  // SetConfigValueFunc
		func(path string) error { return nil },                               // SaveConfigFunc
		func(key string) (map[string]interface{}, error) { return nil, nil }, // GetNestedMapFunc
		func(key string) ([]string, error) { return nil, nil },               // ListKeysFunc
	)
	mockShell := &shell.MockShell{}
	baseHelper := NewBaseHelper(mockConfigHandler, mockShell)

	expectedError := errors.New("error retrieving context: error retrieving context")

	_, err := baseHelper.GetEnvVars()
	if err == nil {
		t.Fatalf("expected error %v, got nil", expectedError)
	}
	if err.Error() != expectedError.Error() {
		t.Fatalf("expected error %v, got %v", expectedError, err)
	}
}

func TestBaseHelper_GetEnvVars_NonStringEnvVar(t *testing.T) {
	mockConfigHandler := config.NewMockConfigHandler(
		func(path string) error { return nil },
		func(key string) (string, error) {
			if key == "context" {
				return "test-context", nil
			}
			return "", errors.New("key not found")
		},
		func(key string, value string) error { return nil }, // SetConfigValueFunc
		func(path string) error { return nil },              // SaveConfigFunc
		func(key string) (map[string]interface{}, error) {
			if key == "contexts.test-context.environment" {
				return map[string]interface{}{
					"VAR1": 123, // Non-string value
				}, nil
			}
			return nil, errors.New("context not found")
		},
		func(key string) ([]string, error) { return nil, nil }, // ListKeysFunc
	)
	mockShell := &shell.MockShell{}
	baseHelper := NewBaseHelper(mockConfigHandler, mockShell)

	expectedError := errors.New("non-string value found in environment variables for context test-context")

	_, err := baseHelper.GetEnvVars()
	if err == nil {
		t.Fatalf("expected error %v, got nil", expectedError)
	}
	if err.Error() != expectedError.Error() {
		t.Fatalf("expected error %v, got %v", expectedError, err)
	}
}

func TestBaseHelper_GetEnvVars_EmptyEnvVars(t *testing.T) {
	mockConfigHandler := config.NewMockConfigHandler(
		func(path string) error { return nil },
		func(key string) (string, error) {
			if key == "context" {
				return "test-context", nil
			}
			return "", errors.New("key not found")
		},
		func(key string, value string) error { return nil }, // SetConfigValueFunc
		func(path string) error { return nil },              // SaveConfigFunc
		func(key string) (map[string]interface{}, error) {
			if key == "contexts.test-context.environment" {
				return map[string]interface{}{}, nil
			}
			return nil, errors.New("context not found")
		},
		func(key string) ([]string, error) { return nil, nil }, // ListKeysFunc
	)
	mockShell := &shell.MockShell{
		GetProjectRootFunc: func() (string, error) {
			return "/mock/project/root", nil
		},
	}
	baseHelper := NewBaseHelper(mockConfigHandler, mockShell)

	expectedResult := map[string]string{
		"WINDSOR_CONTEXT":      "test-context",
		"WINDSOR_PROJECT_ROOT": "/mock/project/root",
	}

	result, err := baseHelper.GetEnvVars()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Sort and compare maps
	if len(result) != len(expectedResult) {
		t.Fatalf("expected map length %d, got %d", len(expectedResult), len(result))
	}

	resultKeys := make([]string, 0, len(result))
	expectedKeys := make([]string, 0, len(expectedResult))

	for k := range result {
		resultKeys = append(resultKeys, k)
	}
	for k := range expectedResult {
		expectedKeys = append(expectedKeys, k)
	}

	sort.Strings(resultKeys)
	sort.Strings(expectedKeys)

	for i, k := range resultKeys {
		if k != expectedKeys[i] || result[k] != expectedResult[expectedKeys[i]] {
			t.Fatalf("expected key-value pair %s:%s, got %s:%s", k, expectedResult[expectedKeys[i]], k, result[k])
		}
	}
}

func TestBaseHelper_GetEnvVars_GetNestedMapError(t *testing.T) {
	mockConfigHandler := config.NewMockConfigHandler(
		func(path string) error { return nil },
		func(key string) (string, error) {
			if key == "context" {
				return "test-context", nil
			}
			return "", errors.New("key not found")
		},
		func(key string, value string) error { return nil }, // SetConfigValueFunc
		func(path string) error { return nil },              // SaveConfigFunc
		func(key string) (map[string]interface{}, error) {
			return nil, errors.New("error getting nested map")
		},
		func(key string) ([]string, error) { return nil, nil }, // ListKeysFunc
	)
	mockShell := &shell.MockShell{
		GetProjectRootFunc: func() (string, error) {
			return "/mock/project/root", nil
		},
	}
	baseHelper := NewBaseHelper(mockConfigHandler, mockShell)

	expectedResult := map[string]string{
		"WINDSOR_CONTEXT":      "test-context",
		"WINDSOR_PROJECT_ROOT": "/mock/project/root",
	}

	result, err := baseHelper.GetEnvVars()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Sort and compare maps
	if len(result) != len(expectedResult) {
		t.Fatalf("expected map length %d, got %d", len(expectedResult), len(result))
	}

	resultKeys := make([]string, 0, len(result))
	expectedKeys := make([]string, 0, len(expectedResult))

	for k := range result {
		resultKeys = append(resultKeys, k)
	}
	for k := range expectedResult {
		expectedKeys = append(expectedKeys, k)
	}

	sort.Strings(resultKeys)
	sort.Strings(expectedKeys)

	for i, k := range resultKeys {
		if k != expectedKeys[i] || result[k] != expectedResult[expectedKeys[i]] {
			t.Fatalf("expected key-value pair %s:%s, got %s:%s", k, expectedResult[expectedKeys[i]], k, result[k])
		}
	}
}

func TestBaseHelper_GetEnvVars(t *testing.T) {
	mockConfigHandler := config.NewMockConfigHandler(
		func(path string) error { return nil },
		func(key string) (string, error) {
			if key == "context" {
				return "test-context", nil
			}
			return "", errors.New("key not found")
		},
		func(key string, value string) error { return nil }, // SetConfigValueFunc
		func(path string) error { return nil },              // SaveConfigFunc
		func(key string) (map[string]interface{}, error) {
			if key == "contexts.test-context.environment" {
				return map[string]interface{}{
					"VAR1": "value1",
					"VAR2": "value2",
				}, nil
			}
			return nil, errors.New("context not found")
		},
		func(key string) ([]string, error) { return nil, nil }, // ListKeysFunc
	)

	mockShell := &shell.MockShell{
		GetProjectRootFunc: func() (string, error) {
			return "/mock/project/root", nil
		},
	}

	baseHelper := NewBaseHelper(mockConfigHandler, mockShell)

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

	// Compare maps
	if len(result) != len(expectedResult) {
		t.Fatalf("expected map length %d, got %d", len(expectedResult), len(result))
	}

	for k, v := range expectedResult {
		if result[k] != v {
			t.Fatalf("expected key-value pair %s:%s, got %s:%s", k, v, k, result[k])
		}
	}
}

func TestBaseHelper_GetEnvVars_ProjectRootError(t *testing.T) {
	mockConfigHandler := config.NewMockConfigHandler(
		func(path string) error { return nil },
		func(key string) (string, error) {
			if key == "context" {
				return "test-context", nil
			}
			return "", errors.New("key not found")
		},
		func(key string, value string) error { return nil }, // SetConfigValueFunc
		func(path string) error { return nil },              // SaveConfigFunc
		func(key string) (map[string]interface{}, error) {
			if key == "contexts.test-context.environment" {
				return map[string]interface{}{
					"VAR1": "value1",
					"VAR2": "value2",
				}, nil
			}
			return nil, errors.New("context not found")
		},
		func(key string) ([]string, error) { return nil, nil }, // ListKeysFunc
	)

	mockShell := &shell.MockShell{
		GetProjectRootFunc: func() (string, error) {
			return "", errors.New("failed to get project root")
		},
	}

	baseHelper := NewBaseHelper(mockConfigHandler, mockShell)

	_, err := baseHelper.GetEnvVars()
	if err == nil {
		t.Fatalf("expected an error, got nil")
	}

	expectedError := "error retrieving project root: failed to get project root"
	if err.Error() != expectedError {
		t.Fatalf("expected error %s, got %s", expectedError, err.Error())
	}
}
