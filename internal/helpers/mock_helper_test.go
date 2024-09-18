package helpers

import (
	"errors"
	"testing"
)

func TestMockHelper_GetEnvVars(t *testing.T) {
	tests := []struct {
		name           string
		getEnvVarsFunc func() (map[string]string, error)
		expectedResult map[string]string
		expectedError  error
	}{
		{
			name: "Success",
			getEnvVarsFunc: func() (map[string]string, error) {
				return map[string]string{
					"VAR1": "value1",
					"VAR2": "value2",
				}, nil
			},
			expectedResult: map[string]string{
				"VAR1": "value1",
				"VAR2": "value2",
			},
			expectedError: nil,
		},
		{
			name: "Error",
			getEnvVarsFunc: func() (map[string]string, error) {
				return nil, errors.New("error getting environment variables")
			},
			expectedResult: nil,
			expectedError:  errors.New("error getting environment variables"),
		},
		{
			name:           "NilFunction",
			getEnvVarsFunc: nil,
			expectedResult: nil,
			expectedError:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockHelper := NewMockHelper(tt.getEnvVarsFunc)

			result, err := mockHelper.GetEnvVars()
			if err != nil && tt.expectedError == nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if err == nil && tt.expectedError != nil {
				t.Fatalf("expected error %v, got nil", tt.expectedError)
			}
			if err != nil && tt.expectedError != nil && err.Error() != tt.expectedError.Error() {
				t.Fatalf("expected error %v, got %v", tt.expectedError, err)
			}
			if !equalMaps(result, tt.expectedResult) {
				t.Fatalf("expected result %v, got %v", tt.expectedResult, result)
			}
		})
	}
}

// Helper function to compare two maps
func equalMaps(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
