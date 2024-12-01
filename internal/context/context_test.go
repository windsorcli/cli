package context

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/shell"
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

	t.Run("ErrorResolvingConfigHandler", func(t *testing.T) {
		mockInjector := di.NewMockInjector()
		// Given a mock injector that resolves to an incorrect type for configHandler
		mockInjector.SetResolveError("configHandler", fmt.Errorf("error resolving configHandler"))

		mocks := setSafeContextMocks(mockInjector)

		// When a new ContextHandler is created and initialized
		contextHandler := NewContextHandler(mocks.Injector)
		err := contextHandler.Initialize()

		// Then an error should be returned
		expectedError := "error resolving configHandler: error resolving configHandler"
		if err == nil || err.Error() != expectedError {
			t.Fatalf("expected error resolving configHandler, got %v", err)
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
		if err == nil || !strings.Contains(err.Error(), "resolved instance is not a ConfigHandler") {
			t.Fatalf("expected error for incorrect configHandler type, got %v", err)
		}
	})

	t.Run("ErrorResolvingShell", func(t *testing.T) {
		mockInjector := di.NewMockInjector()
		mockInjector.SetResolveError("shell", fmt.Errorf("error resolving shell"))

		mocks := setSafeContextMocks(mockInjector)

		// When a new ContextHandler is created and initialized
		contextHandler := NewContextHandler(mocks.Injector)
		err := contextHandler.Initialize()

		// Then an error should be returned
		if err == nil || !strings.Contains(err.Error(), "error resolving shell") {
			t.Fatalf("expected error resolving shell, got %v", err)
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
		if err == nil || !strings.Contains(err.Error(), "resolved instance is not a Shell") {
			t.Fatalf("expected error for incorrect shell type, got %v", err)
		}
	})
}

func TestContext_GetContext(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock config handler that returns a context
		mocks := setSafeContextMocks()
		mocks.MockConfigHandler.GetFunc = func(key string) (interface{}, error) {
			if key == "context" {
				return "test-context", nil
			}
			return nil, nil
		}

		context := NewContextHandler(mocks.Injector)
		err := context.Initialize()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

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
		mocks := setSafeContextMocks()
		mocks.MockConfigHandler.GetFunc = func(key string) (interface{}, error) {
			return nil, nil
		}

		// Create a new Context instance
		context := NewContextHandler(mocks.Injector)
		err := context.Initialize()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

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
		mocks := setSafeContextMocks()
		mocks.MockConfigHandler.SetFunc = func(key string, value interface{}) error {
			if key == "context" && value == "new-context" {
				return nil
			}
			return fmt.Errorf("error setting context")
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

	t.Run("SetConfigValueError", func(t *testing.T) {
		// Given a mock config handler that returns an error when setting the context
		mocks := setSafeContextMocks()
		mocks.MockConfigHandler.SetFunc = func(key string, value interface{}) error {
			return fmt.Errorf("error setting context")
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
		expectedError := "error setting context: error setting context"
		if err.Error() != expectedError {
			t.Fatalf("expected error %s, got %s", expectedError, err.Error())
		}
	})

	t.Run("SaveConfigError", func(t *testing.T) {
		// Given a mock config handler that returns an error when saving the config
		mocks := setSafeContextMocks()
		mocks.MockConfigHandler.SetFunc = func(key string, value interface{}) error {
			return nil
		}
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
}

func TestContext_GetConfigRoot(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock config handler and shell that return valid values
		mocks := setSafeContextMocks()
		mocks.MockConfigHandler.GetFunc = func(key string) (interface{}, error) {
			if key == "context" {
				return "test-context", nil
			}
			return nil, nil
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

	t.Run("GetContextError", func(t *testing.T) {
		// Given a mock config handler that returns an error for Get
		mocks := setSafeContextMocks()
		mocks.MockConfigHandler.GetFunc = func(key string) (interface{}, error) {
			return nil, fmt.Errorf("error retrieving context")
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
		_, err = context.GetConfigRoot()

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
		mocks := setSafeContextMocks()
		mocks.MockConfigHandler.GetFunc = func(key string) (interface{}, error) {
			return "test-context", nil
		}
		mocks.MockShell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("error retrieving project root")
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
		expectedError := "error retrieving project root: error retrieving project root"
		if err.Error() != expectedError {
			t.Fatalf("expected error %s, got %s", expectedError, err.Error())
		}
	})
}
