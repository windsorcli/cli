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
	t.Run("GetEnvVarsFuncSet", func(t *testing.T) {
		// Given a mock helper with a set GetEnvVarsFunc
		expectedEnvVars := map[string]string{"VAR1": "value1"}
		mockHelper := NewMockHelper(func() (map[string]string, error) {
			return expectedEnvVars, nil
		})

		// When calling GetEnvVars
		result, err := mockHelper.GetEnvVars()

		// Then the result should match the expected environment variables and no error should be returned
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if len(result) != len(expectedEnvVars) {
			t.Fatalf("expected map length %d, got %d", len(expectedEnvVars), len(result))
		}
		for k, v := range expectedEnvVars {
			if result[k] != v {
				t.Fatalf("expected key-value pair %s:%s, got %s:%s", k, v, k, result[k])
			}
		}
	})

	t.Run("GetEnvVarsFuncNotSet", func(t *testing.T) {
		// Given a mock helper without a set GetEnvVarsFunc
		mockHelper := NewMockHelper(nil)

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

	t.Run("Error", func(t *testing.T) {
		// Given a mock helper with an error getEnvVarsFunc
		mockHelper := NewMockHelper(func() (map[string]string, error) {
			return nil, errors.New("error getting environment variables")
		})

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
		mockHelper := NewMockHelper(func() (map[string]string, error) {
			return nil, nil
		})

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
		mockHelper := NewMockHelper(func() (map[string]string, error) {
			return nil, nil
		})

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
		mockHelper := NewMockHelper(func() (map[string]string, error) {
			return nil, nil
		})

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
		mockHelper := NewMockHelper(func() (map[string]string, error) {
			return nil, nil
		})

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
		mockHelper := NewMockHelper(func() (map[string]string, error) {
			return nil, nil
		})

		// When: SetConfig is called
		err := mockHelper.SetConfig("some_key", "some_value")

		// Then: it should return no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("SetConfigFunc", func(t *testing.T) {
		// Given: a mock helper with a SetConfigFunc
		expectedError := errors.New("mock error setting config")
		mockHelper := NewMockHelper(func() (map[string]string, error) {
			return nil, nil
		})
		mockHelper.SetConfigFunc = func(key, value string) error {
			if key == "some_key" && value == "some_value" {
				return expectedError
			}
			return nil
		}

		// When: SetConfig is called with the expected key and value
		err := mockHelper.SetConfig("some_key", "some_value")

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
		mockHelper := NewMockHelper(func() (map[string]string, error) {
			return nil, nil
		})

		// Define a mock SetConfigFunc
		expectedError := errors.New("mock error setting config")
		mockSetConfigFunc := func(key, value string) error {
			if key == "test_key" && value == "test_value" {
				return expectedError
			}
			return nil
		}

		// When: SetSetConfigFunc is called
		mockHelper.SetSetConfigFunc(mockSetConfigFunc)

		// Then: the SetConfigFunc should be set and return the expected error
		err := mockHelper.SetConfig("test_key", "test_value")
		if err == nil {
			t.Fatalf("expected error %v, got nil", expectedError)
		}
		if err.Error() != expectedError.Error() {
			t.Fatalf("expected error %v, got %v", expectedError, err)
		}
	})
}

func TestMockHelper_GetContainerConfig(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given: a mock helper
		helper := NewMockHelper(func() (map[string]string, error) {
			return nil, nil
		})

		// When: GetContainerConfig is called
		containerConfig, err := helper.GetContainerConfig()
		if err != nil {
			t.Fatalf("GetContainerConfig() error = %v", err)
		}

		// Then: the result should be nil as per the stub implementation
		if containerConfig != nil {
			t.Errorf("expected nil, got %v", containerConfig)
		}
	})
}
