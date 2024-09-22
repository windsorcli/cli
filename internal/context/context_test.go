package context

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/shell"
)

// Helper function to create a mock config handler
func createMockConfigHandler(getConfigValueFunc func(string) (string, error), setConfigValueFunc func(string, string) error, saveConfigFunc func(string) error) *config.MockConfigHandler {
	return config.NewMockConfigHandler(
		func(path string) error { return nil },
		getConfigValueFunc,
		setConfigValueFunc,
		saveConfigFunc,
		func(key string) (map[string]interface{}, error) { return nil, nil },
		func(key string) ([]string, error) { return nil, nil },
	)
}

// Helper function to create a mock shell
func createMockShell(getProjectRootFunc func() (string, error)) *shell.MockShell {
	return &shell.MockShell{
		GetProjectRootFunc: getProjectRootFunc,
	}
}

func TestContext(t *testing.T) {
	t.Run("GetContext", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			// Given a mock config handler that returns a context
			mockConfigHandler := createMockConfigHandler(
				func(key string) (string, error) {
					if key == "context" {
						return "test-context", nil
					}
					return "", errors.New("key not found")
				},
				nil,
				nil,
			)
			mockShell := createMockShell(nil)
			context := NewContext(mockConfigHandler, mockShell)

			// When calling GetContext
			contextValue, err := context.GetContext()

			// Then the context should be returned without error
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if contextValue != "test-context" {
				t.Fatalf("expected context 'test-context', got %s", contextValue)
			}
		})

		t.Run("Error", func(t *testing.T) {
			// Given a mock config handler that returns an error
			mockConfigHandler := createMockConfigHandler(
				func(key string) (string, error) {
					return "", errors.New("error retrieving context")
				},
				nil,
				nil,
			)
			mockShell := createMockShell(nil)
			context := NewContext(mockConfigHandler, mockShell)

			// When calling GetContext
			_, err := context.GetContext()

			// Then an error should be returned
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			expectedError := "error retrieving context: error retrieving context"
			if err.Error() != expectedError {
				t.Fatalf("expected error %s, got %s", expectedError, err.Error())
			}
		})
	})

	t.Run("SetContext", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			// Given a mock config handler that sets and saves the context successfully
			mockConfigHandler := createMockConfigHandler(
				nil,
				func(key, value string) error {
					if key == "context" && value == "new-context" {
						return nil
					}
					return errors.New("error setting context")
				},
				func(path string) error {
					return nil
				},
			)
			mockShell := createMockShell(nil)
			context := NewContext(mockConfigHandler, mockShell)

			// When calling SetContext
			err := context.SetContext("new-context")

			// Then no error should be returned
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
		})

		t.Run("SetConfigValueError", func(t *testing.T) {
			// Given a mock config handler that returns an error when setting the context
			mockConfigHandler := createMockConfigHandler(
				nil,
				func(key, value string) error {
					return errors.New("error setting context")
				},
				nil,
			)
			mockShell := createMockShell(nil)
			context := NewContext(mockConfigHandler, mockShell)

			// When calling SetContext
			err := context.SetContext("new-context")

			// Then an error should be returned
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			expectedError := "error setting context: error setting context"
			if err.Error() != expectedError {
				t.Fatalf("expected error %s, got %s", expectedError, err.Error())
			}
		})

		t.Run("SaveConfigError", func(t *testing.T) {
			// Given a mock config handler that returns an error when saving the config
			mockConfigHandler := createMockConfigHandler(
				nil,
				func(key, value string) error {
					return nil
				},
				func(path string) error {
					return errors.New("error saving config")
				},
			)
			mockShell := createMockShell(nil)
			context := NewContext(mockConfigHandler, mockShell)

			// When calling SetContext
			err := context.SetContext("new-context")

			// Then an error should be returned
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			expectedError := "error saving config: error saving config"
			if err.Error() != expectedError {
				t.Fatalf("expected error %s, got %s", expectedError, err.Error())
			}
		})
	})

	t.Run("GetConfigRoot", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			// Given a mock config handler and shell that return valid values
			mockConfigHandler := createMockConfigHandler(
				func(key string) (string, error) {
					if key == "context" {
						return "test-context", nil
					}
					return "", errors.New("key not found")
				},
				nil,
				nil,
			)
			mockShell := createMockShell(
				func() (string, error) {
					return "/mock/project/root", nil
				},
			)
			context := NewContext(mockConfigHandler, mockShell)

			// When calling GetConfigRoot
			configRoot, err := context.GetConfigRoot()

			// Then the config root should be returned without error
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			expectedConfigRoot := filepath.Join("/mock/project/root", "contexts", "test-context")
			if configRoot != expectedConfigRoot {
				t.Fatalf("expected config root %s, got %s", expectedConfigRoot, configRoot)
			}
		})

		t.Run("GetContextError", func(t *testing.T) {
			// Given a mock config handler that returns an error when getting the context
			mockConfigHandler := createMockConfigHandler(
				func(key string) (string, error) {
					return "", errors.New("error retrieving context")
				},
				nil,
				nil,
			)
			mockShell := createMockShell(nil)
			context := NewContext(mockConfigHandler, mockShell)

			// When calling GetConfigRoot
			_, err := context.GetConfigRoot()

			// Then an error should be returned
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			expectedError := "error retrieving context: error retrieving context"
			if err.Error() != expectedError {
				t.Fatalf("expected error %s, got %s", expectedError, err.Error())
			}
		})

		t.Run("GetProjectRootError", func(t *testing.T) {
			// Given a mock shell that returns an error when getting the project root
			mockConfigHandler := createMockConfigHandler(
				func(key string) (string, error) {
					return "test-context", nil
				},
				nil,
				nil,
			)
			mockShell := createMockShell(
				func() (string, error) {
					return "", errors.New("error retrieving project root")
				},
			)
			context := NewContext(mockConfigHandler, mockShell)

			// When calling GetConfigRoot
			_, err := context.GetConfigRoot()

			// Then an error should be returned
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			expectedError := "error retrieving project root: error retrieving project root"
			if err.Error() != expectedError {
				t.Fatalf("expected error %s, got %s", expectedError, err.Error())
			}
		})
	})
}
