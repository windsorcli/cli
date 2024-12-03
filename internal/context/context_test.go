package context

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/shell"
)

func TestContext_GetContext(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock config handler that returns a context
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetFunc = func(key string) (interface{}, error) {
			if key == "context" {
				return "test-context", nil
			}
			return nil, nil
		}
		mockShell := shell.NewMockShell()

		context := NewBaseContextHandler(mockConfigHandler, mockShell)

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

	t.Run("GetContextDefaultsToLocal", func(t *testing.T) {
		// Given a config handler that returns an empty string
		mockHandler := config.NewMockConfigHandler()
		mockHandler.GetFunc = func(key string) (interface{}, error) {
			return nil, nil
		}
		mockShell := shell.NewMockShell()

		// Create a new Context instance
		context := NewBaseContextHandler(mockHandler, mockShell)

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
}

func TestContext_SetContext(t *testing.T) {
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
		mockShell := shell.NewMockShell()

		context := NewBaseContextHandler(mockConfigHandler, mockShell)

		// When calling SetContext
		err := context.SetContext("new-context")

		// Then no error should be returned
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("SetConfigValueError", func(t *testing.T) {
		// Given a mock config handler that returns an error when setting the context
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.SetFunc = func(key string, value interface{}) error {
			return errors.New("error setting context")
		}
		mockShell := shell.NewMockShell()

		context := NewBaseContextHandler(mockConfigHandler, mockShell)

		// When calling SetContext
		err := context.SetContext("new-context")

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected error, got none")
		}
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
		mockShell := shell.NewMockShell()

		context := NewBaseContextHandler(mockConfigHandler, mockShell)

		// When calling SetContext
		err := context.SetContext("new-context")

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected error, got none")
		}
		expectedError := "error saving config: error saving config"
		if err.Error() != expectedError {
			t.Fatalf("expected error %s, got %s", expectedError, err.Error())
		}
	})
}

func TestContext_GetConfigRoot(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock config handler and shell that return valid values
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetFunc = func(key string) (interface{}, error) {
			if key == "context" {
				return "test-context", nil
			}
			return nil, nil
		}
		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) {
			return "/mock/project/root", nil
		}

		context := NewBaseContextHandler(mockConfigHandler, mockShell)

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
		// Given a mock config handler that returns an error for Get
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetFunc = func(key string) (interface{}, error) {
			return nil, errors.New("error retrieving context")
		}
		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) {
			return "/mock/project/root", nil
		}

		context := NewBaseContextHandler(mockConfigHandler, mockShell)

		// When calling GetConfigRoot
		_, err := context.GetConfigRoot()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected error, got none")
		}
		if !strings.Contains(err.Error(), "error retrieving context") {
			t.Fatalf("expected error to contain 'error retrieving context', got %s", err.Error())
		}
	})

	t.Run("GetProjectRootError", func(t *testing.T) {
		// Given a mock shell that returns an error when getting the project root
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetFunc = func(key string) (interface{}, error) {
			return "test-context", nil
		}
		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) {
			return "", errors.New("error retrieving project root")
		}

		context := NewBaseContextHandler(mockConfigHandler, mockShell)

		// When calling GetConfigRoot
		_, err := context.GetConfigRoot()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected error, got none")
		}
		expectedError := "error retrieving project root: error retrieving project root"
		if err.Error() != expectedError {
			t.Fatalf("expected error %s, got %s", expectedError, err.Error())
		}
	})
}
