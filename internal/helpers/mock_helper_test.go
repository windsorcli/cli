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

		// Set the PostEnvExecFunc
		mockHelper.SetPostEnvExecFunc(func() error {
			return nil
		})

		// When calling PostEnvExec
		err := mockHelper.PostEnvExec()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("Error", func(t *testing.T) {
		// Given a MockHelper instance with an error PostEnvExecFunc
		mockShell := createMockShell(func() (string, error) { return "", nil })
		mockHelper := NewMockHelper(nil, mockShell)

		// Set the PostEnvExecFunc to return an error
		mockHelper.SetPostEnvExecFunc(func() error {
			return errors.New("post env exec error")
		})

		// When calling PostEnvExec
		expectedError := errors.New("post env exec error")
		err := mockHelper.PostEnvExec()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected error %v, got nil", expectedError)
		}
		if err.Error() != expectedError.Error() {
			t.Fatalf("expected error %v, got %v", expectedError, err)
		}
	})

	t.Run("NilFunction", func(t *testing.T) {
		// Given a MockHelper instance with a nil PostEnvExecFunc
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

func TestMockHelper_SetConfig(t *testing.T) {
	t.Run("SetConfigStub", func(t *testing.T) {
		helper := NewMockHelper(nil, nil)

		// When: SetConfig is called
		err := helper.SetConfig("some_key", "some_value")

		// Then: it should return no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("SetConfigFunc", func(t *testing.T) {
		// Given: a mock helper with a SetConfigFunc
		expectedError := errors.New("mock error setting config")
		helper := NewMockHelper(nil, nil)
		helper.SetConfigFunc = func(key, value string) error {
			if key == "some_key" && value == "some_value" {
				return expectedError
			}
			return nil
		}

		// When: SetConfig is called with the expected key and value
		err := helper.SetConfig("some_key", "some_value")

		// Then: it should return the expected error
		if err == nil {
			t.Fatalf("expected error %v, got nil", expectedError)
		}
		if err.Error() != expectedError.Error() {
			t.Fatalf("expected error %v, got %v", expectedError, err)
		}
	})
}

func TestMockHelper_SetSetConfigFunc(t *testing.T) {
	t.Run("SetSetConfigFunc", func(t *testing.T) {
		// Given: a mock helper
		helper := NewMockHelper(nil, nil)

		// Define a mock SetConfigFunc
		expectedError := errors.New("mock error setting config")
		mockSetConfigFunc := func(key, value string) error {
			if key == "test_key" && value == "test_value" {
				return expectedError
			}
			return nil
		}

		// When: SetSetConfigFunc is called
		helper.SetSetConfigFunc(mockSetConfigFunc)

		// Then: the SetConfigFunc should be set and return the expected error
		err := helper.SetConfig("test_key", "test_value")
		if err == nil {
			t.Fatalf("expected error %v, got nil", expectedError)
		}
		if err.Error() != expectedError.Error() {
			t.Fatalf("expected error %v, got %v", expectedError, err)
		}
	})
}
