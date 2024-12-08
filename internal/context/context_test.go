package context

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/windsorcli/cli/internal/config"
	"github.com/windsorcli/cli/internal/di"
	"github.com/windsorcli/cli/internal/shell"
)

type MockComponents struct {
	Injector          di.Injector
	MockConfigHandler *config.MockConfigHandler
	MockShell         *shell.MockShell
}

func setSafeContextMocks(mockInjector ...di.Injector) *MockComponents {
	var injector di.Injector
	if len(mockInjector) > 0 {
		injector = mockInjector[0]
	} else {
		injector = di.NewMockInjector()
	}

	mockConfigHandler := config.NewMockConfigHandler()
	mockShell := shell.NewMockShell()

	injector.Register("configHandler", mockConfigHandler)
	injector.Register("shell", mockShell)

	return &MockComponents{
		Injector:          injector,
		MockConfigHandler: mockConfigHandler,
		MockShell:         mockShell,
	}
}

func TestContext_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a set of safe context mocks
		mocks := setSafeContextMocks()

		// When a new ContextHandler is created and initialized
		contextHandler := NewContextHandler(mocks.Injector)
		err := contextHandler.Initialize()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("ResolvedInstanceNotConfigHandler", func(t *testing.T) {
		// Given a mock injector that resolves to an incorrect type for configHandler
		mocks := setSafeContextMocks()
		mocks.Injector.Register("configHandler", "not a config handler")

		// When a new ContextHandler is created and initialized
		contextHandler := NewContextHandler(mocks.Injector)
		err := contextHandler.Initialize()

		// Then an error should be returned
		if err == nil || err.Error() != "error resolving configHandler" {
			t.Fatalf("expected error for incorrect configHandler type, got %v", err)
		}
	})

	t.Run("ResolvedInstanceNotShell", func(t *testing.T) {
		// Given a mock injector that resolves to an incorrect type for shell
		mocks := setSafeContextMocks()
		mocks.Injector.Register("shell", "not a shell")

		// When a new ContextHandler is created and initialized
		contextHandler := NewContextHandler(mocks.Injector)
		err := contextHandler.Initialize()

		// Then an error should be returned
		if err == nil || err.Error() != "error resolving shell" {
			t.Fatalf("expected error for incorrect shell type, got %v", err)
		}
	})
}

func TestContext_GetContext(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock config handler that returns a context
		mocks := setSafeContextMocks()
		mocks.MockConfigHandler.GetFunc = func(key string) interface{} {
			if key == "context" {
				return "test-context"
			}
			return nil
		}

		context := NewContextHandler(mocks.Injector)
		err := context.Initialize()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// When calling GetContext
		contextValue := context.GetContext()

		// Then the context should be returned without error
		if contextValue != "test-context" {
			t.Fatalf("expected context 'test-context', got %s", contextValue)
		}
	})

	t.Run("GetContextDefaultsToLocal", func(t *testing.T) {
		// Given a config handler that returns an empty string
		mocks := setSafeContextMocks()
		mocks.MockConfigHandler.GetFunc = func(key string) interface{} {
			return nil
		}

		// Create a new Context instance
		context := NewContextHandler(mocks.Injector)
		err := context.Initialize()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// When GetContext is called
		actualContext := context.GetContext()

		// Then the context should default to "local"
		expectedContext := "local"
		if actualContext != expectedContext {
			t.Errorf("Expected context %q, got %q", expectedContext, actualContext)
		}
	})
}

func TestContext_SetContext(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock config handler that sets and saves the context successfully
		mocks := setSafeContextMocks()
		mocks.MockConfigHandler.SetFunc = func(key string, value interface{}) error {
			if key == "context" && value == "new-context" {
				return nil
			}
			return nil
		}
		mocks.MockConfigHandler.SaveConfigFunc = func(path string) error {
			return nil
		}

		context := NewContextHandler(mocks.Injector)
		err := context.Initialize()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// When calling SetContext
		err = context.SetContext("new-context")

		// Then no error should be returned
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("SaveConfigError", func(t *testing.T) {
		// Given a mock config handler that returns an error when saving the config
		mocks := setSafeContextMocks()
		mocks.MockConfigHandler.SaveConfigFunc = func(path string) error {
			return fmt.Errorf("error saving config")
		}

		context := NewContextHandler(mocks.Injector)
		err := context.Initialize()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// When calling SetContext
		err = context.SetContext("new-context")

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected error, got none")
		}
		expectedError := "error saving config: error saving config"
		if err.Error() != expectedError {
			t.Fatalf("expected error %s, got %s", expectedError, err.Error())
		}
	})

	t.Run("SetContextError", func(t *testing.T) {
		// Given a mock config handler that returns an error when setting the context
		mocks := setSafeContextMocks()
		mocks.MockConfigHandler.SetFunc = func(key string, value interface{}) error {
			if key == "context" {
				return fmt.Errorf("error setting context")
			}
			return nil
		}

		context := NewContextHandler(mocks.Injector)
		err := context.Initialize()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// When calling SetContext
		err = context.SetContext("new-context")

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected error, got none")
		}
		if !strings.Contains(err.Error(), "error setting context") {
			t.Fatalf("expected error to contain 'error setting context', got %s", err.Error())
		}
	})
}

func TestContext_GetConfigRoot(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock config handler and shell that return valid values
		mocks := setSafeContextMocks()
		mocks.MockConfigHandler.GetFunc = func(key string) interface{} {
			if key == "context" {
				return "test-context"
			}
			return nil
		}
		mocks.MockShell.GetProjectRootFunc = func() (string, error) {
			return "/mock/project/root", nil
		}

		context := NewContextHandler(mocks.Injector)
		err := context.Initialize()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

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

	t.Run("GetProjectRootError", func(t *testing.T) {
		// Given a mock shell that returns an error
		mocks := setSafeContextMocks()
		mocks.MockShell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("error getting project root")
		}

		context := NewContextHandler(mocks.Injector)
		err := context.Initialize()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// When calling GetConfigRoot
		_, err = context.GetConfigRoot()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected error, got none")
		}
		expectedError := "error getting project root"
		if err.Error() != expectedError {
			t.Fatalf("expected error %s, got %s", expectedError, err.Error())
		}
	})
}
