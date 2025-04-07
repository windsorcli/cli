package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/secrets"
	"github.com/windsorcli/cli/pkg/shell"
)

func TestBaseConfigHandler_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		injector := di.NewInjector()

		injector.Register("shell", shell.NewMockShell())
		handler := NewBaseConfigHandler(injector)
		err := handler.Initialize()
		if err != nil {
			t.Errorf("Expected error = %v, got = %v", nil, err)
		}
	})

	t.Run("ErrorResolvingConfigHandler", func(t *testing.T) {
		injector := di.NewInjector()

		// Register nil for config handler to simulate error in resolving config handler
		injector.Register("configHandler", nil)

		handler := NewBaseConfigHandler(injector)
		err := handler.Initialize()
		if err == nil {
			t.Errorf("Expected error when resolving config handler, got nil")
		}
	})
}

func TestBaseConfigHandler_SetSecretsProvider(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		injector := di.NewInjector()
		handler := NewBaseConfigHandler(injector)
		secretsProvider := secrets.NewMockSecretsProvider()

		handler.SetSecretsProvider(secretsProvider)

		if len(handler.secretsProviders) != 1 || handler.secretsProviders[0] != secretsProvider {
			t.Errorf("Expected secretsProvider to be set")
		}
	})
}

func TestBaseConfigHandler_GetContext(t *testing.T) {
	// Reset all mocks before each test
	defer func() {
		osGetenv = os.Getenv
	}()

	t.Run("Success", func(t *testing.T) {
		// Given a mock shell that returns a valid project root and context file
		mocks := setupSafeMocks()
		mocks.MockShell.GetProjectRootFunc = func() (string, error) {
			return "/mock/project/root", nil
		}

		// Mock osReadFile to return a specific context
		osReadFile = func(filename string) ([]byte, error) {
			if filename == filepath.Join("/mock/project/root", ".windsor", "context") {
				return []byte("test-context"), nil
			}
			return nil, fmt.Errorf("file not found")
		}

		// Mock osMkdirAll to simulate successful directory creation
		osMkdirAll = func(path string, perm os.FileMode) error {
			if path == filepath.Join("/mock/project/root", ".windsor") {
				return nil
			}
			return fmt.Errorf("error creating directory")
		}

		// Mock os.Getenv to return no context
		osGetenv = func(key string) string {
			return ""
		}

		configHandler := NewBaseConfigHandler(mocks.Injector)
		err := configHandler.Initialize()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// When calling GetContext
		contextValue := configHandler.GetContext()

		// Then the context should be returned without error
		if contextValue != "test-context" {
			t.Errorf("expected context 'test-context', got %s", contextValue)
		}
	})

	t.Run("GetContextDefaultsToLocal", func(t *testing.T) {
		// Given a mock shell that returns a valid project root but no context file
		mocks := setupSafeMocks()
		mocks.MockShell.GetProjectRootFunc = func() (string, error) {
			return "/mock/project/root", nil
		}

		// Mock osReadFile to simulate file not found
		osReadFile = func(_ string) ([]byte, error) {
			return nil, fmt.Errorf("file not found")
		}

		// Mock os.Getenv to return no context
		osGetenv = func(key string) string {
			return ""
		}

		// Create a new Context instance
		configHandler := NewBaseConfigHandler(mocks.Injector)
		err := configHandler.Initialize()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// When GetContext is called
		actualContext := configHandler.GetContext()

		// Then the context should default to "local"
		expectedContext := "local"
		if actualContext != expectedContext {
			t.Errorf("Expected context %q, got %q", expectedContext, actualContext)
		}
	})

	t.Run("ContextAlreadyDefined", func(t *testing.T) {
		// Mock os.Getenv to return a predefined context
		osGetenv = func(key string) string {
			if key == "WINDSOR_CONTEXT" {
				return "predefined-context"
			}
			return ""
		}

		// Given a mock shell and a pre-defined context
		mocks := setupSafeMocks()
		mocks.MockShell.GetProjectRootFunc = func() (string, error) {
			return "/mock/project/root", nil
		}

		// Create a new Context instance
		configHandler := NewBaseConfigHandler(mocks.Injector)
		err := configHandler.Initialize()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// When GetContext is called
		actualContext := configHandler.GetContext()

		// Then the pre-defined context should be returned
		expectedContext := "predefined-context"
		if actualContext != expectedContext {
			t.Errorf("Expected context %q, got %q", expectedContext, actualContext)
		}
	})
}

func TestConfigHandler_SetContext(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock shell that returns a valid project root
		mocks := setupSafeMocks()
		mocks.MockShell.GetProjectRootFunc = func() (string, error) {
			return "/mock/project/root", nil
		}

		// Mock osMkdirAll to simulate successful directory creation
		osMkdirAll = func(path string, perm os.FileMode) error {
			if path == filepath.Join("/mock/project/root", ".windsor") {
				return nil
			}
			return fmt.Errorf("error creating directory")
		}

		// Mock osWriteFile to simulate successful write
		osWriteFile = func(filename string, data []byte, perm os.FileMode) error {
			if filename == filepath.Join("/mock/project/root", ".windsor", "context") && string(data) == "new-context" {
				return nil
			}
			return fmt.Errorf("error writing file")
		}

		configHandler := NewBaseConfigHandler(mocks.Injector)
		err := configHandler.Initialize()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// When calling SetContext
		err = configHandler.SetContext("new-context")

		// Then no error should be returned
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("GetProjectRootError", func(t *testing.T) {
		// Given a mock shell that returns an error for GetProjectRoot
		mocks := setupSafeMocks()
		mocks.MockShell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("mocked error inside GetProjectRoot")
		}

		configHandler := NewBaseConfigHandler(mocks.Injector)
		err := configHandler.Initialize()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// When calling SetContext
		err = configHandler.SetContext("new-context")

		// Then an error should be returned
		if err == nil || err.Error() != "error getting project root: mocked error inside GetProjectRoot" {
			t.Fatalf("expected error 'error getting project root: mocked error inside GetProjectRoot', got %v", err)
		}
	})

	t.Run("MkdirAllError", func(t *testing.T) {
		// Given a mock shell that returns a valid project root
		mocks := setupSafeMocks()
		mocks.MockShell.GetProjectRootFunc = func() (string, error) {
			return "/mock/project/root", nil
		}

		// Mock osMkdirAll to simulate an error
		osMkdirAll = func(path string, perm os.FileMode) error {
			return fmt.Errorf("error creating directory")
		}

		configHandler := NewBaseConfigHandler(mocks.Injector)
		err := configHandler.Initialize()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// When calling SetContext
		err = configHandler.SetContext("new-context")

		// Then an error should be returned
		if err == nil || err.Error() != "error ensuring context directory exists: error creating directory" {
			t.Fatalf("expected error 'error ensuring context directory exists: error creating directory', got %v", err)
		}
	})

	t.Run("WriteFileError", func(t *testing.T) {
		// Given a mock shell that returns a valid project root
		mocks := setupSafeMocks()
		mocks.MockShell.GetProjectRootFunc = func() (string, error) {
			return "/mock/project/root", nil
		}

		// Mock osMkdirAll to simulate successful directory creation
		osMkdirAll = func(path string, perm os.FileMode) error {
			return nil
		}

		// Mock osWriteFile to simulate an error
		osWriteFile = func(filename string, data []byte, perm os.FileMode) error {
			return fmt.Errorf("error writing file")
		}

		configHandler := NewBaseConfigHandler(mocks.Injector)
		err := configHandler.Initialize()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// When calling SetContext
		err = configHandler.SetContext("new-context")

		// Then an error should be returned
		if err == nil || err.Error() != "error writing context to file: error writing file" {
			t.Fatalf("expected error 'error writing context to file: error writing file', got %v", err)
		}
	})
}

func TestConfigHandler_GetConfigRoot(t *testing.T) {
	// Reset all mocks after each test
	defer func() {
		osGetenv = os.Getenv
	}()

	t.Run("Success", func(t *testing.T) {
		// Given a mock shell that returns valid values
		mocks := setupSafeMocks()
		mocks.MockShell.GetProjectRootFunc = func() (string, error) {
			return "/mock/project/root", nil
		}

		// Mock osReadFile to simulate reading the context from a file
		osReadFile = func(filename string) ([]byte, error) {
			if filename == filepath.Join("/mock/project/root", ".windsor", "context") {
				return []byte("test-context"), nil
			}
			return nil, fmt.Errorf("error reading file")
		}

		// Mock os.Getenv to return no context
		osGetenv = func(key string) string {
			if key == "WINDSOR_CONTEXT" {
				return "test-context"
			}
			return ""
		}

		configHandler := NewBaseConfigHandler(mocks.Injector)
		err := configHandler.Initialize()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// When calling GetConfigRoot
		configRoot, err := configHandler.GetConfigRoot()

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
		mocks := setupSafeMocks()
		mocks.MockShell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("error getting project root")
		}

		configHandler := NewBaseConfigHandler(mocks.Injector)
		err := configHandler.Initialize()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// When calling GetConfigRoot
		_, err = configHandler.GetConfigRoot()

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

func TestConfigHandler_Clean(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock context handler
		mocks := setupSafeMocks()

		// When calling Clean
		configHandler := NewBaseConfigHandler(mocks.Injector)
		err := configHandler.Initialize()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		mocks.MockShell.GetProjectRootFunc = func() (string, error) {
			return "/mock/project/root", nil
		}

		// Mock osStat to simulate the directory exists
		osStat = func(_ string) (os.FileInfo, error) {
			return nil, nil
		}

		// Mock osRemoveAll to simulate successful deletion
		osRemoveAll = func(path string) error {
			return nil
		}

		err = configHandler.Clean()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("ErrorGettingConfigRoot", func(t *testing.T) {
		// Given a mock context handler that returns an error when getting the config root
		mocks := setupSafeMocks()
		mocks.MockShell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("error getting project root")
		}

		configHandler := NewBaseConfigHandler(mocks.Injector)
		err := configHandler.Initialize()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// When calling Clean
		err = configHandler.Clean()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected error, got none")
		}
		expectedError := "error getting config root: error getting project root"
		if err.Error() != expectedError {
			t.Fatalf("expected error %s, got %s", expectedError, err.Error())
		}
	})

	t.Run("ErrorDeletingDirectory", func(t *testing.T) {
		// Given a mock context handler
		mocks := setupSafeMocks()

		mocks.MockShell.GetProjectRootFunc = func() (string, error) {
			return "/mock/project/root", nil
		}

		// Mock osStat to simulate the directory exists
		osStat = func(_ string) (os.FileInfo, error) {
			return nil, nil
		}

		// Mock osRemoveAll to return an error
		osRemoveAll = func(path string) error {
			return fmt.Errorf("error deleting %s", path)
		}

		configHandler := NewBaseConfigHandler(mocks.Injector)
		err := configHandler.Initialize()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// When calling Clean
		err = configHandler.Clean()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected error, got none")
		}
		if !strings.Contains(err.Error(), "error deleting") {
			t.Fatalf("expected error containing 'error deleting', got %s", err.Error())
		}
	})
}

func TestConfigHandler_IsLoaded(t *testing.T) {
	t.Run("IsLoadedTrue", func(t *testing.T) {
		// Given a config handler with loaded set to true
		configHandler := &BaseConfigHandler{loaded: true}

		// When calling IsLoaded
		isLoaded := configHandler.IsLoaded()

		// Then it should return true
		if !isLoaded {
			t.Errorf("expected IsLoaded to return true, got false")
		}
	})

	t.Run("IsLoadedFalse", func(t *testing.T) {
		// Given a config handler with loaded set to false
		configHandler := &BaseConfigHandler{loaded: false}

		// When calling IsLoaded
		isLoaded := configHandler.IsLoaded()

		// Then it should return false
		if isLoaded {
			t.Errorf("expected IsLoaded to return false, got true")
		}
	})
}
