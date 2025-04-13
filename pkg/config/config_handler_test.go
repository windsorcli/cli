package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/secrets"
	"github.com/windsorcli/cli/pkg/shell"
)

// =============================================================================
// Test Setup
// =============================================================================

type Mocks struct {
	Injector        di.Injector
	ConfigHandler   *ConfigHandler
	Shell           *shell.MockShell
	SecretsProvider *secrets.MockSecretsProvider
}

type SetupOptions struct {
	Injector        di.Injector
	ConfigHandler   ConfigHandler
	ConfigStr       string
	SecretsProvider *secrets.MockSecretsProvider
}

// Global test setup helper that creates a temporary directory and mocks
// This is used by most test functions to establish a clean test environment
func setupMocks(t *testing.T, opts ...*SetupOptions) *Mocks {
	t.Helper()

	// Store original working directory
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}

	// Create temp dir using testing.TempDir()
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to change to temp directory: %v", err)
	}

	os.Setenv("WINDSOR_PROJECT_ROOT", tmpDir)

	var injector di.Injector
	if len(opts) > 0 {
		injector = opts[0].Injector
	} else {
		injector = di.NewInjector()
	}

	mockShell := &shell.MockShell{}
	mockShell.GetProjectRootFunc = func() (string, error) {
		return tmpDir, nil
	}
	injector.Register("shell", mockShell)

	mockSecretsProvider := &secrets.MockSecretsProvider{}
	injector.Register("secretsProvider", mockSecretsProvider)

	mockConfigHandler := NewMockConfigHandler()
	mockConfigHandler.GetConfigFunc = func() *v1alpha1.Context {
		return &v1alpha1.Context{}
	}
	injector.Register("configHandler", mockConfigHandler)

	osStat = func(name string) (os.FileInfo, error) {
		return nil, nil
	}
	osReadFile = func(filename string) ([]byte, error) {
		return []byte("dummy: data"), nil
	}
	osWriteFile = func(name string, data []byte, perm os.FileMode) error {
		return nil
	}
	osMkdirAll = func(path string, perm os.FileMode) error {
		return nil
	}

	originalOsStat := osStat
	originalOsReadFile := osReadFile
	originalOsWriteFile := osWriteFile
	originalOsMkdirAll := osMkdirAll

	t.Cleanup(func() {
		osStat = originalOsStat
		osReadFile = originalOsReadFile
		osWriteFile = originalOsWriteFile
		osMkdirAll = originalOsMkdirAll

		os.Unsetenv("WINDSOR_PROJECT_ROOT")

		if err := os.Chdir(origDir); err != nil {
			t.Logf("Warning: Failed to change back to original directory: %v", err)
		}
	})

	return &Mocks{
		Shell:           mockShell,
		SecretsProvider: mockSecretsProvider,
		Injector:        injector,
	}
}

// =============================================================================
// Test Runners
// =============================================================================

func TestBaseConfigHandler_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a properly configured dependency injector with required services
		mocks := setupMocks(t)

		// When initializing a new BaseConfigHandler
		handler := NewBaseConfigHandler(mocks.Injector)
		err := handler.Initialize()

		// Then initialization should complete without errors
		if err != nil {
			t.Errorf("Expected error = %v, got = %v", nil, err)
		}
	})

	t.Run("ErrorResolvingShell", func(t *testing.T) {
		// Given a dependency injector with a missing shell component
		mocks := setupMocks(t)
		mocks.Injector.Register("shell", nil)

		// When initializing a BaseConfigHandler with the incomplete injector
		handler := NewBaseConfigHandler(mocks.Injector)
		err := handler.Initialize()

		// Then an error should be returned due to missing shell dependency
		if err == nil {
			t.Errorf("Expected error when resolving shell, got nil")
		}
	})
}

func TestBaseConfigHandler_SetSecretsProvider(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a BaseConfigHandler instance and a secrets provider
		mocks := setupMocks(t)
		handler := NewBaseConfigHandler(mocks.Injector)
		secretsProvider := secrets.NewMockSecretsProvider(mocks.Injector)

		// When setting the secrets provider on the handler
		handler.SetSecretsProvider(secretsProvider)

		// Then the secrets provider should be correctly registered with the handler
		if len(handler.secretsProviders) != 1 || handler.secretsProviders[0] != secretsProvider {
			t.Errorf("Expected secretsProvider to be set correctly in the handler's providers list")
		}
	})
}

func TestConfigHandler_IsLoaded(t *testing.T) {
	t.Run("IsLoadedTrue", func(t *testing.T) {
		// Given a config handler with loaded state set to true
		configHandler := &BaseConfigHandler{loaded: true}

		// When checking if the configuration is loaded
		isLoaded := configHandler.IsLoaded()

		// Then it should return true
		if !isLoaded {
			t.Errorf("expected IsLoaded to return true, got false")
		}
	})

	t.Run("IsLoadedFalse", func(t *testing.T) {
		// Given a config handler with loaded state set to false
		configHandler := &BaseConfigHandler{loaded: false}

		// When checking if the configuration is loaded
		isLoaded := configHandler.IsLoaded()

		// Then it should return false
		if isLoaded {
			t.Errorf("expected IsLoaded to return false, got true")
		}
	})
}

func TestBaseConfigHandler_GetContext(t *testing.T) {
	// Reset all mocks before each test
	defer func() {
		osGetenv = os.Getenv
	}()

	t.Run("Success", func(t *testing.T) {
		// Given a mock shell with a valid project root and a context file
		mocks := setupMocks(t)
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "/mock/project/root", nil
		}

		// And a mock file system that returns a specific context value
		osReadFile = func(filename string) ([]byte, error) {
			if filename == filepath.Join("/mock/project/root", ".windsor", "context") {
				return []byte("test-context"), nil
			}
			return nil, fmt.Errorf("file not found")
		}

		// And successful directory creation
		osMkdirAll = func(path string, perm os.FileMode) error {
			if path == filepath.Join("/mock/project/root", ".windsor") {
				return nil
			}
			return fmt.Errorf("error creating directory")
		}

		// And no context specified in environment variables
		osGetenv = func(key string) string {
			return ""
		}

		// When initializing the config handler and retrieving the context
		configHandler := NewBaseConfigHandler(mocks.Injector)
		err := configHandler.Initialize()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		contextValue := configHandler.GetContext()

		// Then the context from the file should be returned
		if contextValue != "test-context" {
			t.Errorf("expected context 'test-context', got %s", contextValue)
		}
	})

	t.Run("EmptyContextFile", func(t *testing.T) {
		// Given a mock shell with a valid project root
		mocks := setupMocks(t)
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "/mock/project/root", nil
		}

		// And a mock file system that returns an empty context file
		osReadFile = func(filename string) ([]byte, error) {
			if filename == filepath.Join("/mock/project/root", ".windsor", "context") {
				return []byte(""), nil
			}
			return nil, fmt.Errorf("file not found")
		}

		// And no context specified in environment variables
		osGetenv = func(key string) string {
			return ""
		}

		// When initializing the config handler and retrieving the context
		configHandler := NewBaseConfigHandler(mocks.Injector)
		err := configHandler.Initialize()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		contextValue := configHandler.GetContext()

		// Then an empty context should be returned
		if contextValue != "" {
			t.Errorf("expected empty context for empty file, got %s", contextValue)
		}
	})

	t.Run("GetContextDefaultsToLocal", func(t *testing.T) {
		// Given a mock shell with a valid project root but no context file
		mocks := setupMocks(t)
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "/mock/project/root", nil
		}

		// And a mock file system that simulates no context file
		osReadFile = func(_ string) ([]byte, error) {
			return nil, fmt.Errorf("file not found")
		}

		// And no context specified in environment variables
		osGetenv = func(key string) string {
			return ""
		}

		// When initializing the config handler and retrieving the context
		configHandler := NewBaseConfigHandler(mocks.Injector)
		err := configHandler.Initialize()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		actualContext := configHandler.GetContext()

		// Then the context should default to "local"
		expectedContext := "local"
		if actualContext != expectedContext {
			t.Errorf("Expected context %q, got %q", expectedContext, actualContext)
		}
	})

	t.Run("GetProjectRootError", func(t *testing.T) {
		// Given a mock shell that fails to get project root
		mocks := setupMocks(t)
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("project root error")
		}

		// And no context specified in environment variables
		osGetenv = func(key string) string {
			return ""
		}

		// When initializing the config handler and retrieving the context
		configHandler := NewBaseConfigHandler(mocks.Injector)
		err := configHandler.Initialize()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		contextValue := configHandler.GetContext()

		// Then the default "local" context should be returned
		if contextValue != "local" {
			t.Errorf("expected context 'local' when GetProjectRoot fails, got %s", contextValue)
		}
	})

	t.Run("ContextAlreadyDefined", func(t *testing.T) {
		// Given a context defined in environment variables
		osGetenv = func(key string) string {
			if key == "WINDSOR_CONTEXT" {
				return "predefined-context"
			}
			return ""
		}

		// And a mock shell with a valid project root
		mocks := setupMocks(t)
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "/mock/project/root", nil
		}

		// When initializing the config handler and retrieving the context
		configHandler := NewBaseConfigHandler(mocks.Injector)
		err := configHandler.Initialize()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		actualContext := configHandler.GetContext()

		// Then the context from the environment variable should be returned
		expectedContext := "predefined-context"
		if actualContext != expectedContext {
			t.Errorf("Expected context %q, got %q", expectedContext, actualContext)
		}
	})
}

func TestConfigHandler_SetContext(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock shell with a valid project root
		mocks := setupMocks(t)
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "/mock/project/root", nil
		}

		// And a mock file system that allows directory creation
		osMkdirAll = func(path string, perm os.FileMode) error {
			if path == filepath.Join("/mock/project/root", ".windsor") {
				return nil
			}
			return fmt.Errorf("error creating directory")
		}

		// And a mock file system that allows writing to the context file
		osWriteFile = func(filename string, data []byte, perm os.FileMode) error {
			if filename == filepath.Join("/mock/project/root", ".windsor", "context") && string(data) == "new-context" {
				return nil
			}
			return fmt.Errorf("error writing file")
		}

		// When initializing a config handler and setting a new context
		configHandler := NewBaseConfigHandler(mocks.Injector)
		err := configHandler.Initialize()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		err = configHandler.SetContext("new-context")

		// Then no error should be returned
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("GetProjectRootError", func(t *testing.T) {
		// Given a mock shell that fails to get project root
		mocks := setupMocks(t)
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("mocked error inside GetProjectRoot")
		}

		// When initializing a config handler and attempting to set a context
		configHandler := NewBaseConfigHandler(mocks.Injector)
		err := configHandler.Initialize()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		err = configHandler.SetContext("new-context")

		// Then an error about project root should be returned
		if err == nil || err.Error() != "error getting project root: mocked error inside GetProjectRoot" {
			t.Fatalf("expected error 'error getting project root: mocked error inside GetProjectRoot', got %v", err)
		}
	})

	t.Run("MkdirAllError", func(t *testing.T) {
		// Given a mock shell with a valid project root
		mocks := setupMocks(t)
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "/mock/project/root", nil
		}

		// And a mock file system that fails to create directories
		osMkdirAll = func(path string, perm os.FileMode) error {
			return fmt.Errorf("error creating directory")
		}

		// When initializing a config handler and attempting to set a context
		configHandler := NewBaseConfigHandler(mocks.Injector)
		err := configHandler.Initialize()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		err = configHandler.SetContext("new-context")

		// Then an error about directory creation should be returned
		if err == nil || err.Error() != "error ensuring context directory exists: error creating directory" {
			t.Fatalf("expected error 'error ensuring context directory exists: error creating directory', got %v", err)
		}
	})

	t.Run("WriteFileError", func(t *testing.T) {
		// Given a mock shell with a valid project root
		mocks := setupMocks(t)
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "/mock/project/root", nil
		}

		// And a mock file system that allows directory creation
		osMkdirAll = func(path string, perm os.FileMode) error {
			return nil
		}

		// But a mock file system that fails to write files
		osWriteFile = func(filename string, data []byte, perm os.FileMode) error {
			return fmt.Errorf("error writing file")
		}

		// When initializing a config handler and attempting to set a context
		configHandler := NewBaseConfigHandler(mocks.Injector)
		err := configHandler.Initialize()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		err = configHandler.SetContext("new-context")

		// Then an error about file writing should be returned
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
		// Given a mock shell with a valid project root
		mocks := setupMocks(t)
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "/mock/project/root", nil
		}

		// And a mock file system that returns the test context
		osReadFile = func(filename string) ([]byte, error) {
			if filename == filepath.Join("/mock/project/root", ".windsor", "context") {
				return []byte("test-context"), nil
			}
			return nil, fmt.Errorf("error reading file")
		}

		// And a context specified in environment variables
		osGetenv = func(key string) string {
			if key == "WINDSOR_CONTEXT" {
				return "test-context"
			}
			return ""
		}

		// When initializing the config handler and getting the config root path
		configHandler := NewBaseConfigHandler(mocks.Injector)
		err := configHandler.Initialize()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		configRoot, err := configHandler.GetConfigRoot()

		// Then the correct config root path should be returned without error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		expectedConfigRoot := filepath.Join("/mock/project/root", "contexts", "test-context")
		if configRoot != expectedConfigRoot {
			t.Fatalf("expected config root %s, got %s", expectedConfigRoot, configRoot)
		}
	})

	t.Run("GetProjectRootError", func(t *testing.T) {
		// Given a mock shell that fails to get project root
		mocks := setupMocks(t)
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("error getting project root")
		}

		// When initializing the config handler and attempting to get the config root
		configHandler := NewBaseConfigHandler(mocks.Injector)
		err := configHandler.Initialize()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		_, err = configHandler.GetConfigRoot()

		// Then an error about project root should be returned
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
		// Given a mock shell with a valid project root
		mocks := setupMocks(t)
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "/mock/project/root", nil
		}

		// And a mock file system where the directory exists
		osStat = func(_ string) (os.FileInfo, error) {
			return nil, nil
		}

		// And a mock file system that allows directory deletion
		osRemoveAll = func(path string) error {
			return nil
		}

		// When initializing a config handler and cleaning resources
		configHandler := NewBaseConfigHandler(mocks.Injector)
		err := configHandler.Initialize()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		err = configHandler.Clean()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("ErrorGettingConfigRoot", func(t *testing.T) {
		// Given a mock shell that fails to get project root
		mocks := setupMocks(t)
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("error getting project root")
		}

		// When initializing a config handler and attempting to clean resources
		configHandler := NewBaseConfigHandler(mocks.Injector)
		err := configHandler.Initialize()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		err = configHandler.Clean()

		// Then an error about config root should be returned
		if err == nil {
			t.Fatalf("expected error, got none")
		}
		expectedError := "error getting config root: error getting project root"
		if err.Error() != expectedError {
			t.Fatalf("expected error %s, got %s", expectedError, err.Error())
		}
	})

	t.Run("ErrorDeletingDirectory", func(t *testing.T) {
		// Given a mock shell with a valid project root
		mocks := setupMocks(t)
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "/mock/project/root", nil
		}

		// And a mock file system where the directory exists
		osStat = func(_ string) (os.FileInfo, error) {
			return nil, nil
		}

		// But a mock file system that fails to delete directories
		osRemoveAll = func(path string) error {
			return fmt.Errorf("error deleting %s", path)
		}

		// When initializing a config handler and attempting to clean resources
		configHandler := NewBaseConfigHandler(mocks.Injector)
		err := configHandler.Initialize()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		err = configHandler.Clean()

		// Then an error about directory deletion should be returned
		if err == nil {
			t.Fatalf("expected error, got none")
		}
		if !strings.Contains(err.Error(), "error deleting") {
			t.Fatalf("expected error containing 'error deleting', got %s", err.Error())
		}
	})
}
