package context

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/shell"
)

func assertError(t *testing.T, err error, shouldError bool) {
	if shouldError && err == nil {
		t.Errorf("Expected error, got nil")
	} else if !shouldError && err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestContext(t *testing.T) {
	t.Run("GetContext", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			// Given a mock config handler that returns a context
			mockConfigHandler := config.NewMockConfigHandler()
			mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) (string, error) {
				if key == "context" {
					return "test-context", nil
				}
				return "", errors.New("key not found")
			}
			mockShell := shell.NewMockShell("unix")

			context := NewContext(mockConfigHandler, mockShell)

			// When calling GetContext
			contextValue, err := context.GetContext()

			// Then the context should be returned without error
			assertError(t, err, false)
			if contextValue != "test-context" {
				t.Fatalf("expected context 'test-context', got %s", contextValue)
			}
		})

		t.Run("Error", func(t *testing.T) {
			// Given a mock config handler that returns an error
			mockConfigHandler := config.NewMockConfigHandler()
			mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) (string, error) {
				return "", errors.New("error retrieving context")
			}
			mockShell := shell.NewMockShell("unix")

			context := NewContext(mockConfigHandler, mockShell)

			// When calling GetContext
			_, err := context.GetContext()

			// Then an error should be returned
			assertError(t, err, true)
			expectedError := "error retrieving context: error retrieving context"
			if err.Error() != expectedError {
				t.Fatalf("expected error %s, got %s", expectedError, err.Error())
			}
		})

		t.Run("GetContextDefaultsToLocal", func(t *testing.T) {
			// Given a config handler that returns an empty string
			mockHandler := config.NewMockConfigHandler()
			mockHandler.GetStringFunc = func(key string, defaultValue ...string) (string, error) {
				return "", nil
			}
			mockShell := shell.NewMockShell("cmd")

			// Create a new Context instance
			context := NewContext(mockHandler, mockShell)

			// When GetContext is called
			actualContext, err := context.GetContext()

			// Then no error should be returned
			if err != nil {
				t.Fatalf("Expected no error, got %v", err)
			}

			// And the context should default to "local"
			expectedContext := "local"
			if actualContext != expectedContext {
				t.Errorf("Expected context %q, got %q", expectedContext, actualContext)
			}
		})
	})

	t.Run("SetContext", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			// Given a mock config handler that sets and saves the context successfully
			mockConfigHandler := config.NewMockConfigHandler()
			mockConfigHandler.SetFunc = func(key string, value interface{}) error {
				if key == "context" && value == "new-context" {
					return nil
				}
				return errors.New("error setting context")
			}
			mockConfigHandler.SaveConfigFunc = func(path string) error {
				return nil
			}
			mockShell := shell.NewMockShell("unix")

			context := NewContext(mockConfigHandler, mockShell)

			// When calling SetContext
			err := context.SetContext("new-context")

			// Then no error should be returned
			assertError(t, err, false)
		})

		t.Run("SetConfigValueError", func(t *testing.T) {
			// Given a mock config handler that returns an error when setting the context
			mockConfigHandler := config.NewMockConfigHandler()
			mockConfigHandler.SetFunc = func(key string, value interface{}) error {
				return errors.New("error setting context")
			}
			mockShell := shell.NewMockShell("unix")

			context := NewContext(mockConfigHandler, mockShell)

			// When calling SetContext
			err := context.SetContext("new-context")

			// Then an error should be returned
			assertError(t, err, true)
			expectedError := "error setting context: error setting context"
			if err.Error() != expectedError {
				t.Fatalf("expected error %s, got %s", expectedError, err.Error())
			}
		})

		t.Run("SaveConfigError", func(t *testing.T) {
			// Given a mock config handler that returns an error when saving the config
			mockConfigHandler := config.NewMockConfigHandler()
			mockConfigHandler.SetFunc = func(key string, value interface{}) error {
				return nil
			}
			mockConfigHandler.SaveConfigFunc = func(path string) error {
				return errors.New("error saving config")
			}
			mockShell := shell.NewMockShell("unix")

			context := NewContext(mockConfigHandler, mockShell)

			// When calling SetContext
			err := context.SetContext("new-context")

			// Then an error should be returned
			assertError(t, err, true)
			expectedError := "error saving config: error saving config"
			if err.Error() != expectedError {
				t.Fatalf("expected error %s, got %s", expectedError, err.Error())
			}
		})
	})

	t.Run("GetConfigRoot", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			// Given a mock config handler and shell that return valid values
			mockConfigHandler := config.NewMockConfigHandler()
			mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) (string, error) {
				if key == "context" {
					return "test-context", nil
				}
				return "", errors.New("key not found")
			}
			mockShell := shell.NewMockShell("unix")
			mockShell.GetProjectRootFunc = func() (string, error) {
				return "/mock/project/root", nil
			}

			context := NewContext(mockConfigHandler, mockShell)

			// When calling GetConfigRoot
			configRoot, err := context.GetConfigRoot()

			// Then the config root should be returned without error
			assertError(t, err, false)
			expectedConfigRoot := filepath.Join("/mock/project/root", "contexts", "test-context")
			if configRoot != expectedConfigRoot {
				t.Fatalf("expected config root %s, got %s", expectedConfigRoot, configRoot)
			}
		})

		t.Run("GetProjectRootError", func(t *testing.T) {
			// Given a mock shell that returns an error when getting the project root
			mockConfigHandler := config.NewMockConfigHandler()
			mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) (string, error) {
				return "test-context", nil
			}
			mockShell := shell.NewMockShell("unix")
			mockShell.GetProjectRootFunc = func() (string, error) {
				return "", errors.New("error retrieving project root")
			}

			context := NewContext(mockConfigHandler, mockShell)

			// When calling GetConfigRoot
			_, err := context.GetConfigRoot()

			// Then an error should be returned
			assertError(t, err, true)
			expectedError := "error retrieving project root: error retrieving project root"
			if err.Error() != expectedError {
				t.Fatalf("expected error %s, got %s", expectedError, err.Error())
			}
		})
	})

	t.Run("GetContextError", func(t *testing.T) {
		// Given a config handler that returns an error
		mockHandler := config.NewMockConfigHandler()
		mockHandler.GetStringFunc = func(key string, defaultValue ...string) (string, error) {
			return "", errors.New("mock error")
		}
		mockShell := shell.NewMockShell("cmd")

		// Create a new Context instance
		context := NewContext(mockHandler, mockShell)

		// When GetConfigRoot is called
		_, err := context.GetConfigRoot()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		// And the error should be wrapped correctly
		expectedError := "error retrieving context: error retrieving context: mock error"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})
}
