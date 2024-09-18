package helpers

import (
	"errors"
	"sort"
	"testing"

	"github.com/windsor-hotel/cli/internal/config"
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
	baseHelper := NewBaseHelper(mockConfigHandler)

	expectedResult := map[string]string{
		"VAR1":           "value1",
		"VAR2":           "value2",
		"WINDSORCONTEXT": "test-context",
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
		nil, // SetConfigValueFunc
		nil, // SaveConfigFunc
		nil, // GetNestedMapFunc
		nil, // ListKeysFunc
	)
	baseHelper := NewBaseHelper(mockConfigHandler)

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
		nil, // SetConfigValueFunc
		nil, // SaveConfigFunc
		func(key string) (map[string]interface{}, error) {
			if key == "contexts.test-context.environment" {
				return map[string]interface{}{
					"VAR1": 123, // Non-string value
				}, nil
			}
			return nil, errors.New("context not found")
		},
		nil, // ListKeysFunc
	)
	baseHelper := NewBaseHelper(mockConfigHandler)

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
		nil, // SetConfigValueFunc
		nil, // SaveConfigFunc
		func(key string) (map[string]interface{}, error) {
			if key == "contexts.test-context.environment" {
				return map[string]interface{}{}, nil
			}
			return nil, errors.New("context not found")
		},
		nil, // ListKeysFunc
	)
	baseHelper := NewBaseHelper(mockConfigHandler)

	expectedResult := map[string]string{
		"WINDSORCONTEXT": "test-context",
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
		nil, // SetConfigValueFunc
		nil, // SaveConfigFunc
		func(key string) (map[string]interface{}, error) {
			return nil, errors.New("error getting nested map")
		},
		nil, // ListKeysFunc
	)
	baseHelper := NewBaseHelper(mockConfigHandler)

	expectedResult := map[string]string{
		"WINDSORCONTEXT": "test-context",
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
