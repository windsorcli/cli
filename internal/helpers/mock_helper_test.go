package helpers

import (
	"errors"
	"reflect"
	"testing"

	"github.com/compose-spec/compose-go/types"
	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/shell"
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

func TestMockHelper(t *testing.T) {
	t.Run("GetEnvVars", func(t *testing.T) {
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
	})

	t.Run("PostEnvExec", func(t *testing.T) {
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
	})

	t.Run("GetContainerConfig", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			// Given: a mock helper with a GetContainerConfigFunc
			expectedConfig := []types.ServiceConfig{
				{
					Name:  "service1",
					Image: "nginx:latest",
				},
			}
			mockHelper := &MockHelper{
				GetContainerConfigFunc: func() ([]types.ServiceConfig, error) {
					return expectedConfig, nil
				},
			}

			// When: GetContainerConfig is called
			containerConfig, err := mockHelper.GetContainerConfig()
			if err != nil {
				t.Fatalf("GetContainerConfig() error = %v", err)
			}

			// Then: the result should match the expected configuration
			if !reflect.DeepEqual(containerConfig, expectedConfig) {
				t.Errorf("expected %v, got %v", expectedConfig, containerConfig)
			}
		})

		t.Run("Error", func(t *testing.T) {
			// Given: a mock helper with a GetContainerConfigFunc that returns an error
			expectedError := errors.New("mock error getting container config")
			mockHelper := &MockHelper{
				GetContainerConfigFunc: func() ([]types.ServiceConfig, error) {
					return nil, expectedError
				},
			}

			// When: GetContainerConfig is called
			_, err := mockHelper.GetContainerConfig()
			if err == nil {
				t.Fatalf("expected error %v, got nil", expectedError)
			}
			if err.Error() != expectedError.Error() {
				t.Fatalf("expected error %v, got %v", expectedError, err)
			}
		})
	})

	t.Run("SetGetContainerConfigFunc", func(t *testing.T) {
		t.Run("SetGetContainerConfigFunc", func(t *testing.T) {
			// Given: a mock helper
			mockHelper := NewMockHelper(func() (map[string]string, error) {
				return nil, nil
			})

			// Define a mock GetContainerConfigFunc
			expectedConfig := []types.ServiceConfig{
				{
					Name:  "service1",
					Image: "nginx:latest",
				},
			}
			mockGetContainerConfigFunc := func() ([]types.ServiceConfig, error) {
				return expectedConfig, nil
			}

			// When: SetGetContainerConfigFunc is called
			mockHelper.SetGetContainerConfigFunc(mockGetContainerConfigFunc)

			// Then: the GetContainerConfigFunc should be set and return the expected configuration
			containerConfig, err := mockHelper.GetContainerConfig()
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if !reflect.DeepEqual(containerConfig, expectedConfig) {
				t.Errorf("expected %v, got %v", expectedConfig, containerConfig)
			}
		})
	})

	t.Run("WriteConfig", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			// Given: a mock config handler, context, and shell
			mockConfigHandler := config.NewMockConfigHandler()
			mockContext := context.NewMockContext()
			mockContext.GetContextFunc = func() (string, error) {
				return "test-context", nil
			}
			mockContext.GetConfigRootFunc = func() (string, error) {
				return "/path/to/config", nil
			}
			mockShell, _ := shell.NewMockShell("unix")

			// Create DI container and register mocks
			diContainer := di.NewContainer()
			diContainer.Register("cliConfigHandler", mockConfigHandler)
			diContainer.Register("context", mockContext)
			diContainer.Register("shell", mockShell)

			// Create an instance of MockHelper
			mockHelper := NewMockHelper(func() (map[string]string, error) {
				return nil, nil
			})
			mockHelper.WriteConfigFunc = func() error {
				return nil
			}

			// When: WriteConfig is called
			err := mockHelper.WriteConfig()

			// Then: no error should be returned
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
		})

		t.Run("SetWriteConfigFunc", func(t *testing.T) {
			// Given: a mock helper
			mockHelper := NewMockHelper(func() (map[string]string, error) {
				return nil, nil
			})

			// Define a mock WriteConfigFunc
			expectedError := errors.New("mock error writing config")
			mockWriteConfigFunc := func() error {
				return expectedError
			}

			// When: SetWriteConfigFunc is called
			mockHelper.SetWriteConfigFunc(mockWriteConfigFunc)

			// Then: the WriteConfigFunc should be set and return the expected error
			err := mockHelper.WriteConfig()
			if err == nil {
				t.Fatalf("expected error %v, got nil", expectedError)
			}
			if err.Error() != expectedError.Error() {
				t.Fatalf("expected error %v, got %v", expectedError, err)
			}
		})

		t.Run("WriteConfigFuncNotSet", func(t *testing.T) {
			// Given: a mock helper without a WriteConfigFunc set
			mockHelper := &MockHelper{}

			// When: WriteConfig is called
			err := mockHelper.WriteConfig()

			// Then: it should return no error
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
		})
	})
}
