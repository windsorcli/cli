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
	Shims           *Shims
}

type SetupOptions struct {
	Injector      di.Injector
	ConfigHandler ConfigHandler
	ConfigStr     string
}

func setupShims(t *testing.T) *Shims {
	t.Helper()
	shims := NewShims()
	shims.Stat = func(name string) (os.FileInfo, error) {
		return nil, nil
	}
	shims.ReadFile = func(filename string) ([]byte, error) {
		return []byte("dummy: data"), nil
	}
	shims.WriteFile = func(name string, data []byte, perm os.FileMode) error {
		return nil
	}
	shims.Getenv = func(key string) string {
		if key == "WINDSOR_CONTEXT" {
			return "test"
		}
		return ""
	}
	return shims
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

	mockShell := shell.NewMockShell(injector)
	mockShell.GetProjectRootFunc = func() (string, error) {
		return tmpDir, nil
	}
	injector.Register("shell", mockShell)

	mockSecretsProvider := secrets.NewMockSecretsProvider(injector)
	injector.Register("secretsProvider", mockSecretsProvider)

	mockConfigHandler := NewMockConfigHandler()
	mockConfigHandler.GetConfigFunc = func() *v1alpha1.Context {
		return &v1alpha1.Context{}
	}
	injector.Register("configHandler", mockConfigHandler)

	mockShims := setupShims(t)

	t.Cleanup(func() {
		os.Unsetenv("WINDSOR_PROJECT_ROOT")

		if err := os.Chdir(origDir); err != nil {
			t.Logf("Warning: Failed to change back to original directory: %v", err)
		}
	})

	return &Mocks{
		Shell:           mockShell,
		SecretsProvider: mockSecretsProvider,
		Injector:        injector,
		Shims:           mockShims,
	}
}

// =============================================================================
// Test Public Methods
// =============================================================================

// TestBaseConfigHandler_Initialize tests the initialization of the BaseConfigHandler
func TestBaseConfigHandler_Initialize(t *testing.T) {
	setup := func(t *testing.T) (*BaseConfigHandler, *Mocks) {
		mocks := setupMocks(t)
		handler := NewBaseConfigHandler(mocks.Injector)
		handler.shims = mocks.Shims
		return handler, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a properly configured BaseConfigHandler
		handler, _ := setup(t)

		// When Initialize is called
		err := handler.Initialize()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected error = %v, got = %v", nil, err)
		}
	})

	t.Run("ErrorResolvingShell", func(t *testing.T) {
		// Given a BaseConfigHandler with a missing shell component
		handler, mocks := setup(t)
		mocks.Injector.Register("shell", nil)

		// When Initialize is called
		err := handler.Initialize()

		// Then an error should be returned
		if err == nil {
			t.Errorf("Expected error when resolving shell, got nil")
		}
	})
}

// TestConfigHandler_IsLoaded tests the IsLoaded method of the ConfigHandler
func TestConfigHandler_IsLoaded(t *testing.T) {
	setup := func(t *testing.T) (*BaseConfigHandler, *Mocks) {
		mocks := setupMocks(t)
		handler := NewBaseConfigHandler(mocks.Injector)
		handler.shims = mocks.Shims
		return handler, mocks
	}

	t.Run("IsLoadedTrue", func(t *testing.T) {
		// Given a ConfigHandler with loaded=true
		handler, _ := setup(t)
		handler.loaded = true

		// When IsLoaded is called
		isLoaded := handler.IsLoaded()

		// Then it should return true
		if !isLoaded {
			t.Errorf("expected IsLoaded to return true, got false")
		}
	})

	t.Run("IsLoadedFalse", func(t *testing.T) {
		// Given a ConfigHandler with loaded=false
		handler, _ := setup(t)
		handler.loaded = false

		// When IsLoaded is called
		isLoaded := handler.IsLoaded()

		// Then it should return false
		if isLoaded {
			t.Errorf("expected IsLoaded to return false, got true")
		}
	})
}

// TestBaseConfigHandler_GetContext tests the GetContext method of the BaseConfigHandler
func TestBaseConfigHandler_GetContext(t *testing.T) {
	setup := func(t *testing.T) (*BaseConfigHandler, *Mocks) {
		mocks := setupMocks(t)
		handler := NewBaseConfigHandler(mocks.Injector)
		handler.shims = mocks.Shims
		return handler, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a properly configured BaseConfigHandler with a context file
		handler, mocks := setup(t)
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "/mock/project/root", nil
		}
		mocks.Shims.ReadFile = func(filename string) ([]byte, error) {
			if filename == filepath.Join("/mock/project/root", ".windsor", "context") {
				return []byte("test-context"), nil
			}
			return nil, fmt.Errorf("file not found")
		}
		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error {
			if path == filepath.Join("/mock/project/root", ".windsor") {
				return nil
			}
			return fmt.Errorf("error creating directory")
		}
		mocks.Shims.Getenv = func(key string) string {
			return ""
		}
		err := handler.Initialize()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// When GetContext is called
		contextValue := handler.GetContext()

		// Then it should return the context from the file
		if contextValue != "test-context" {
			t.Errorf("expected context 'test-context', got %s", contextValue)
		}
	})

	t.Run("EmptyContextFile", func(t *testing.T) {
		// Given a BaseConfigHandler with an empty context file
		handler, mocks := setup(t)
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "/mock/project/root", nil
		}
		mocks.Shims.ReadFile = func(filename string) ([]byte, error) {
			if filename == filepath.Join("/mock/project/root", ".windsor", "context") {
				return []byte(""), nil
			}
			return nil, fmt.Errorf("file not found")
		}
		mocks.Shims.Getenv = func(key string) string {
			return ""
		}
		err := handler.Initialize()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// When GetContext is called
		contextValue := handler.GetContext()

		// Then it should return an empty string
		if contextValue != "" {
			t.Errorf("expected empty context for empty file, got %s", contextValue)
		}
	})

	t.Run("GetContextDefaultsToLocal", func(t *testing.T) {
		// Given a BaseConfigHandler with no context file
		handler, mocks := setup(t)
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "/mock/project/root", nil
		}
		mocks.Shims.ReadFile = func(_ string) ([]byte, error) {
			return nil, fmt.Errorf("file not found")
		}
		mocks.Shims.Getenv = func(key string) string {
			return ""
		}
		err := handler.Initialize()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// When GetContext is called
		actualContext := handler.GetContext()

		// Then it should default to "local"
		expectedContext := "local"
		if actualContext != expectedContext {
			t.Errorf("Expected context %q, got %q", expectedContext, actualContext)
		}
	})

	t.Run("GetProjectRootError", func(t *testing.T) {
		// Given a BaseConfigHandler with a GetProjectRoot error
		handler, mocks := setup(t)
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("project root error")
		}
		mocks.Shims.Getenv = func(key string) string {
			return ""
		}
		err := handler.Initialize()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// When GetContext is called
		contextValue := handler.GetContext()

		// Then it should default to "local"
		if contextValue != "local" {
			t.Errorf("expected context 'local' when GetProjectRoot fails, got %s", contextValue)
		}
	})

	t.Run("ContextAlreadyDefined", func(t *testing.T) {
		// Given a BaseConfigHandler with a predefined context in environment
		handler, mocks := setup(t)
		mocks.Shims.Getenv = func(key string) string {
			if key == "WINDSOR_CONTEXT" {
				return "predefined-context"
			}
			return ""
		}
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "/mock/project/root", nil
		}
		err := handler.Initialize()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// When GetContext is called
		actualContext := handler.GetContext()

		// Then it should return the predefined context
		expectedContext := "predefined-context"
		if actualContext != expectedContext {
			t.Errorf("Expected context %q, got %q", expectedContext, actualContext)
		}
	})
}

// TestConfigHandler_SetContext tests the SetContext method of the ConfigHandler
func TestConfigHandler_SetContext(t *testing.T) {
	setup := func(t *testing.T) (*BaseConfigHandler, *Mocks) {
		mocks := setupMocks(t)
		handler := NewBaseConfigHandler(mocks.Injector)
		handler.shims = mocks.Shims
		return handler, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a properly configured BaseConfigHandler
		handler, mocks := setup(t)
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "/mock/project/root", nil
		}
		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error {
			if path == filepath.Join("/mock/project/root", ".windsor") {
				return nil
			}
			return fmt.Errorf("error creating directory")
		}
		mocks.Shims.WriteFile = func(filename string, data []byte, perm os.FileMode) error {
			if filename == filepath.Join("/mock/project/root", ".windsor", "context") && string(data) == "new-context" {
				return nil
			}
			return fmt.Errorf("error writing file")
		}
		err := handler.Initialize()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// When SetContext is called with a new context
		err = handler.SetContext("new-context")

		// Then no error should be returned
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("GetProjectRootError", func(t *testing.T) {
		// Given a BaseConfigHandler with a GetProjectRoot error
		handler, mocks := setup(t)
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("mocked error inside GetProjectRoot")
		}
		err := handler.Initialize()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// When SetContext is called
		err = handler.SetContext("new-context")

		// Then an error should be returned
		if err == nil || err.Error() != "error getting project root: mocked error inside GetProjectRoot" {
			t.Fatalf("expected error 'error getting project root: mocked error inside GetProjectRoot', got %v", err)
		}
	})

	t.Run("MkdirAllError", func(t *testing.T) {
		// Given a BaseConfigHandler with a MkdirAll error
		handler, mocks := setup(t)
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "/mock/project/root", nil
		}
		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error {
			return fmt.Errorf("error creating directory")
		}
		err := handler.Initialize()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// When SetContext is called
		err = handler.SetContext("new-context")

		// Then an error should be returned
		if err == nil || err.Error() != "error ensuring context directory exists: error creating directory" {
			t.Fatalf("expected error 'error ensuring context directory exists: error creating directory', got %v", err)
		}
	})

	t.Run("WriteFileError", func(t *testing.T) {
		// Given a BaseConfigHandler with a WriteFile error
		handler, mocks := setup(t)
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "/mock/project/root", nil
		}
		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error {
			return nil
		}
		mocks.Shims.WriteFile = func(filename string, data []byte, perm os.FileMode) error {
			return fmt.Errorf("error writing file")
		}
		err := handler.Initialize()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// When SetContext is called
		err = handler.SetContext("new-context")

		// Then an error should be returned
		if err == nil || err.Error() != "error writing context to file: error writing file" {
			t.Fatalf("expected error 'error writing context to file: error writing file', got %v", err)
		}
	})

	t.Run("SetenvError", func(t *testing.T) {
		// Given a BaseConfigHandler with a Setenv error
		handler, mocks := setup(t)
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "/mock/project/root", nil
		}
		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error {
			return nil
		}
		mocks.Shims.WriteFile = func(filename string, data []byte, perm os.FileMode) error {
			return nil
		}
		mocks.Shims.Setenv = func(key, value string) error {
			return fmt.Errorf("setenv error")
		}
		err := handler.Initialize()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// When SetContext is called
		err = handler.SetContext("test-context")

		// Then an error should be returned
		if err == nil {
			t.Fatal("expected error, got none")
		}
		expectedError := "error setting WINDSOR_CONTEXT environment variable: setenv error"
		if err.Error() != expectedError {
			t.Fatalf("expected error %q, got %q", expectedError, err.Error())
		}
	})
}

// TestConfigHandler_GetConfigRoot tests the GetConfigRoot method of the ConfigHandler
func TestConfigHandler_GetConfigRoot(t *testing.T) {
	setup := func(t *testing.T) (*BaseConfigHandler, *Mocks) {
		mocks := setupMocks(t)
		handler := NewBaseConfigHandler(mocks.Injector)
		handler.shims = mocks.Shims
		return handler, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a properly configured BaseConfigHandler with a context
		handler, mocks := setup(t)
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "/mock/project/root", nil
		}
		mocks.Shims.ReadFile = func(filename string) ([]byte, error) {
			if filename == filepath.Join("/mock/project/root", ".windsor", "context") {
				return []byte("test-context"), nil
			}
			return nil, fmt.Errorf("error reading file")
		}
		mocks.Shims.Getenv = func(key string) string {
			if key == "WINDSOR_CONTEXT" {
				return "test-context"
			}
			return ""
		}
		err := handler.Initialize()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// When GetConfigRoot is called
		configRoot, err := handler.GetConfigRoot()

		// Then it should return the correct config root path
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		expectedConfigRoot := filepath.Join("/mock/project/root", "contexts", "test-context")
		if configRoot != expectedConfigRoot {
			t.Fatalf("expected config root %s, got %s", expectedConfigRoot, configRoot)
		}
	})

	t.Run("GetProjectRootError", func(t *testing.T) {
		// Given a BaseConfigHandler with a GetProjectRoot error
		handler, mocks := setup(t)
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("error getting project root")
		}
		err := handler.Initialize()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// When GetConfigRoot is called
		_, err = handler.GetConfigRoot()

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

// TestConfigHandler_Clean tests the Clean method of the ConfigHandler
func TestConfigHandler_Clean(t *testing.T) {
	setup := func(t *testing.T) (*BaseConfigHandler, *Mocks) {
		mocks := setupMocks(t)
		handler := NewBaseConfigHandler(mocks.Injector)
		handler.shims = mocks.Shims
		return handler, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a properly configured BaseConfigHandler
		handler, mocks := setup(t)
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "/mock/project/root", nil
		}
		mocks.Shims.RemoveAll = func(path string) error {
			return nil
		}
		err := handler.Initialize()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// When Clean is called
		err = handler.Clean()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("ErrorGettingConfigRoot", func(t *testing.T) {
		// Given a BaseConfigHandler with a GetProjectRoot error
		handler, mocks := setup(t)
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("error getting project root")
		}
		err := handler.Initialize()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// When Clean is called
		err = handler.Clean()

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
		// Given a BaseConfigHandler with a RemoveAll error
		handler, mocks := setup(t)
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "/mock/project/root", nil
		}
		mocks.Shims.RemoveAll = func(path string) error {
			return fmt.Errorf("error deleting %s", path)
		}
		err := handler.Initialize()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// When Clean is called
		err = handler.Clean()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected error, got none")
		}
		if !strings.Contains(err.Error(), "error deleting") {
			t.Fatalf("expected error containing 'error deleting', got %s", err.Error())
		}
	})
}

func TestBaseConfigHandler_SetSecretsProvider(t *testing.T) {
	t.Run("AddsProvider", func(t *testing.T) {
		// Given a new config handler
		mocks := setupMocks(t)
		handler := NewBaseConfigHandler(mocks.Injector)

		// And a mock secrets provider
		mockProvider := secrets.NewMockSecretsProvider(mocks.Injector)

		// When setting the secrets provider
		handler.SetSecretsProvider(mockProvider)

		// Then the provider should be added to the list
		if len(handler.secretsProviders) != 1 {
			t.Errorf("Expected 1 secrets provider, got %d", len(handler.secretsProviders))
		}
		if handler.secretsProviders[0] != mockProvider {
			t.Errorf("Expected provider to be added, got %v", handler.secretsProviders[0])
		}
	})

	t.Run("AddsMultipleProviders", func(t *testing.T) {
		// Given a new config handler
		mocks := setupMocks(t)
		handler := NewBaseConfigHandler(mocks.Injector)

		// And multiple mock secrets providers
		mockProvider1 := secrets.NewMockSecretsProvider(mocks.Injector)
		mockProvider2 := secrets.NewMockSecretsProvider(mocks.Injector)

		// When setting multiple secrets providers
		handler.SetSecretsProvider(mockProvider1)
		handler.SetSecretsProvider(mockProvider2)

		// Then all providers should be added to the list
		if len(handler.secretsProviders) != 2 {
			t.Errorf("Expected 2 secrets providers, got %d", len(handler.secretsProviders))
		}
		if handler.secretsProviders[0] != mockProvider1 {
			t.Errorf("Expected first provider to be added, got %v", handler.secretsProviders[0])
		}
		if handler.secretsProviders[1] != mockProvider2 {
			t.Errorf("Expected second provider to be added, got %v", handler.secretsProviders[1])
		}
	})
}
