package helpers

import (
	"errors"
	"testing"
)

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

func TestMockHelper_GetEnvVars(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock helper with a successful getEnvVarsFunc
		mockHelper := NewMockHelper(func() (map[string]string, error) {
			return map[string]string{
				"VAR1": "value1",
				"VAR2": "value2",
			}, nil
		}, nil)

		// When calling GetEnvVars
		expectedResult := map[string]string{
			"VAR1": "value1",
			"VAR2": "value2",
		}
		result, err := mockHelper.GetEnvVars()

		// Then the result should match the expected result
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if !equalMaps(result, expectedResult) {
			t.Fatalf("expected result %v, got %v", expectedResult, result)
		}
	})

	t.Run("Error", func(t *testing.T) {
		// Given a mock helper with an error getEnvVarsFunc
		mockHelper := NewMockHelper(func() (map[string]string, error) {
			return nil, errors.New("error getting environment variables")
		}, nil)

		// When calling GetEnvVars
		expectedError := errors.New("error getting environment variables")
		_, err := mockHelper.GetEnvVars()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected error %v, got nil", expectedError)
		}
		if err.Error() != expectedError.Error() {
			t.Fatalf("expected error %v, got %v", expectedError, err)
		}
	})

	t.Run("NilFunction", func(t *testing.T) {
		// Given a mock helper with a nil getEnvVarsFunc
		mockHelper := NewMockHelper(nil, nil)

		// When calling GetEnvVars
		result, err := mockHelper.GetEnvVars()

		// Then the result should be nil and no error should be returned
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if result != nil {
			t.Fatalf("expected result nil, got %v", result)
		}
	})
}

func TestMockHelper_PostEnvExec(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a MockHelper instance
		mockShell := createMockShell(func() (string, error) { return "", nil })
		mockHelper := NewMockHelper(nil, mockShell)

		// When calling PostEnvExec
		err := mockHelper.PostEnvExec()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}
