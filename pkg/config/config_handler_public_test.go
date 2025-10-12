package config

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/api/v1alpha1/aws"
	"github.com/windsorcli/cli/api/v1alpha1/cluster"
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

func setupMocks(t *testing.T, opts ...*SetupOptions) *Mocks {
	t.Helper()

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}

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
// Helper Functions
// =============================================================================

// stringPtr returns a pointer to the provided string
func stringPtr(s string) *string {
	return &s
}

// =============================================================================
// Constructor
// =============================================================================

func TestNewConfigHandler(t *testing.T) {
	setup := func(t *testing.T) (ConfigHandler, *Mocks) {
		mocks := setupMocks(t)
		handler := NewConfigHandler(mocks.Injector)
		handler.(*configHandler).shims = mocks.Shims

		return handler, mocks
	}
	t.Run("Success", func(t *testing.T) {
		handler, _ := setup(t)

		// Then the handler should be successfully created and not be nil
		if handler == nil {
			t.Fatal("Expected non-nil configHandler")
		}
	})
}

// =============================================================================
// Public Methods
// =============================================================================

func TestConfigHandler_LoadConfig(t *testing.T) {
	setup := func(t *testing.T) (ConfigHandler, *Mocks) {
		mocks := setupMocks(t)
		handler := NewConfigHandler(mocks.Injector)
		handler.(*configHandler).shims = mocks.Shims
		if err := handler.Initialize(); err != nil {
			t.Fatalf("Failed to initialize handler: %v", err)
		}
		return handler, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a set of safe mocks and a configHandler
		handler, _ := setup(t)

		// And a valid config path
		tempDir := t.TempDir()
		configPath := filepath.Join(tempDir, "config.yaml")

		// When LoadConfig is called with the valid path
		err := handler.LoadConfig(configPath)

		// Then no error should be returned
		if err != nil {
			t.Fatalf("LoadConfig() unexpected error: %v", err)
		}

		// And the path should be set correctly
		if handler.(*configHandler).path != configPath {
			t.Errorf("Expected path = %v, got = %v", configPath, handler.(*configHandler).path)
		}
	})

	t.Run("CreateEmptyConfigFileIfNotExist", func(t *testing.T) {
		// Given a set of safe mocks and a configHandler
		handler, _ := setup(t)

		// And a mocked osStat that returns ErrNotExist
		handler.(*configHandler).shims.Stat = func(_ string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		// When LoadConfig is called with a non-existent path
		err := handler.LoadConfig("test_config.yaml")

		// Then no error should be returned
		if err != nil {
			t.Fatalf("LoadConfig() unexpected error: %v", err)
		}
	})

	t.Run("ReadFileError", func(t *testing.T) {
		// Given a set of safe mocks and a configHandler
		handler, _ := setup(t)

		// And a mocked osReadFile that returns an error
		handler.(*configHandler).shims.ReadFile = func(filename string) ([]byte, error) {
			return nil, fmt.Errorf("mocked error reading file")
		}

		// When LoadConfig is called
		err := handler.LoadConfig("mocked_config.yaml")

		// Then an error should be returned
		if err == nil {
			t.Fatalf("LoadConfig() expected error, got nil")
		}

		// And the error message should be as expected
		expectedError := "error reading config file: mocked error reading file"
		if err.Error() != expectedError {
			t.Errorf("LoadConfig() error = %v, expected '%s'", err, expectedError)
		}
	})

	t.Run("UnmarshalError", func(t *testing.T) {
		// Given a set of safe mocks and a configHandler
		handler, _ := setup(t)

		// And a mocked yamlUnmarshal that returns an error
		handler.(*configHandler).shims.YamlUnmarshal = func(data []byte, v any) error {
			return fmt.Errorf("mocked error unmarshalling yaml")
		}

		// When LoadConfig is called
		err := handler.LoadConfig("mocked_path.yaml")

		// Then an error should be returned
		if err == nil {
			t.Fatalf("LoadConfig() expected error, got nil")
		}

		// And the error message should be as expected
		expectedError := "error unmarshalling yaml: mocked error unmarshalling yaml"
		if err.Error() != expectedError {
			t.Errorf("LoadConfig() error = %v, expected '%s'", err, expectedError)
		}
	})

	t.Run("UnsupportedConfigVersion", func(t *testing.T) {
		// Given a set of safe mocks and a configHandler
		handler, _ := setup(t)

		// And a mocked yamlUnmarshal that sets an unsupported version
		handler.(*configHandler).shims.YamlUnmarshal = func(data []byte, v any) error {
			if config, ok := v.(*v1alpha1.Config); ok {
				config.Version = "unsupported_version"
			}
			return nil
		}

		// When LoadConfig is called
		err := handler.LoadConfig("mocked_path.yaml")

		// Then an error should be returned
		if err == nil {
			t.Fatalf("LoadConfig() expected error, got nil")
		}

		// And the error message should be as expected
		expectedError := "unsupported config version: unsupported_version"
		if err.Error() != expectedError {
			t.Errorf("LoadConfig() error = %v, expected '%s'", err, expectedError)
		}
	})
}

func TestConfigHandler_Get(t *testing.T) {
	setup := func(t *testing.T) (ConfigHandler, *Mocks) {
		mocks := setupMocks(t)
		handler := NewConfigHandler(mocks.Injector)
		handler.(*configHandler).shims = mocks.Shims
		if err := handler.Initialize(); err != nil {
			t.Fatalf("Failed to initialize handler: %v", err)
		}
		return handler, mocks
	}

	t.Run("KeyNotUnderContexts", func(t *testing.T) {
		// Given a set of safe mocks and a configHandler
		handler, mocks := setup(t)

		// And a mocked shell that returns a project root
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "/mock/project/root", nil
		}

		// And a mocked shims that handles context file
		handler.(*configHandler).shims.ReadFile = func(filename string) ([]byte, error) {
			if filename == "/mock/project/root/.windsor/context" {
				return []byte("local"), nil
			}
			return nil, fmt.Errorf("file not found")
		}

		// And a config with proper initialization
		handler.(*configHandler).config = v1alpha1.Config{
			Version: "v1alpha1",
			Contexts: map[string]*v1alpha1.Context{
				"local": {
					Environment: map[string]string{},
				},
			},
		}

		// And the context is set
		handler.(*configHandler).context = "local"

		// When getting a key not under contexts
		val := handler.Get("nonContextKey")

		// Then nil should be returned
		if val != nil {
			t.Errorf("Expected nil for non-context key, got %v", val)
		}
	})

	t.Run("InvalidPath", func(t *testing.T) {
		// Given a set of safe mocks and a configHandler
		handler, _ := setup(t)

		// When calling Get with an empty path
		value := handler.Get("")

		// Then nil should be returned
		if value != nil {
			t.Errorf("Expected nil for empty path, got %v", value)
		}
	})

	t.Run("PrecedenceAndSchemaDefaults", func(t *testing.T) {
		// Given a handler with schema validator and various data sources
		handler, _ := setup(t)
		handler.(*configHandler).context = "test"
		handler.(*configHandler).loaded = true

		// Set up schema validator with defaults
		handler.(*configHandler).schemaValidator = &SchemaValidator{
			Schema: map[string]any{
				"properties": map[string]any{
					"SCHEMA_KEY": map[string]any{
						"default": "schema_default_value",
					},
				},
			},
		}

		// Test schema defaults for single-key path
		value := handler.Get("SCHEMA_KEY")
		expected := "schema_default_value"
		if value != expected {
			t.Errorf("Expected schema default value '%s', got '%v'", expected, value)
		}

		// Test that multi-key paths don't use schema defaults
		value = handler.Get("contexts.test.SCHEMA_KEY")
		if value != nil {
			t.Errorf("Expected nil for multi-key path, got '%v'", value)
		}

		// Test contextValues precedence
		handler.(*configHandler).contextValues = map[string]any{
			"TEST_VAR": "values_value",
		}
		value = handler.Get("contexts.test.TEST_VAR")
		expected = "values_value"
		if value != expected {
			t.Errorf("Expected contextValues value '%s', got '%v'", expected, value)
		}

		// Test that contextValues are not checked when not loaded
		handler.(*configHandler).loaded = false
		value = handler.Get("contexts.test.TEST_VAR")
		if value != nil {
			t.Errorf("Expected nil when not loaded, got '%v'", value)
		}
	})
}

func TestConfigHandler_SaveConfig(t *testing.T) {
	setup := func(t *testing.T) (ConfigHandler, *Mocks) {
		mocks := setupMocks(t)
		handler := NewConfigHandler(mocks.Injector)
		handler.(*configHandler).shims = mocks.Shims
		if err := handler.Initialize(); err != nil {
			t.Fatalf("Failed to initialize handler: %v", err)
		}
		return handler, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a configHandler with a mocked shell
		handler, mocks := setup(t)

		tempDir := t.TempDir()
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return tempDir, nil
		}

		// And a context is set
		handler.(*configHandler).context = "test-context"

		// And some configuration data
		handler.Set("contexts.test-context.provider", "local")

		// When SaveConfig is called
		err := handler.SaveConfig()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And the root windsor.yaml should exist with only version
		rootConfigPath := filepath.Join(tempDir, "windsor.yaml")
		if _, err := handler.(*configHandler).shims.Stat(rootConfigPath); os.IsNotExist(err) {
			t.Fatalf("Root config file was not created at %s", rootConfigPath)
		}

		// And the context config should exist
		contextConfigPath := filepath.Join(tempDir, "contexts", "test-context", "windsor.yaml")
		if _, err := handler.(*configHandler).shims.Stat(contextConfigPath); os.IsNotExist(err) {
			t.Fatalf("Context config file was not created at %s", contextConfigPath)
		}
	})

	t.Run("WithOverwriteFalse", func(t *testing.T) {
		// Given a configHandler with existing config files
		handler, mocks := setup(t)

		tempDir := t.TempDir()
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return tempDir, nil
		}

		handler.(*configHandler).context = "test-context"

		// Create existing files
		rootConfigPath := filepath.Join(tempDir, "windsor.yaml")
		os.WriteFile(rootConfigPath, []byte("existing content"), 0644)

		contextDir := filepath.Join(tempDir, "contexts", "test-context")
		os.MkdirAll(contextDir, 0755)
		contextConfigPath := filepath.Join(contextDir, "windsor.yaml")
		os.WriteFile(contextConfigPath, []byte("existing context content"), 0644)

		// When SaveConfig is called with overwrite false
		err := handler.SaveConfig(false)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And the files should still contain the original content
		rootContent, _ := os.ReadFile(rootConfigPath)
		if string(rootContent) != "existing content" {
			t.Errorf("Root config file was overwritten when it shouldn't have been")
		}

		contextContent, _ := os.ReadFile(contextConfigPath)
		if string(contextContent) != "existing context content" {
			t.Errorf("Context config file was overwritten when it shouldn't have been")
		}
	})

	t.Run("ShellNotInitialized", func(t *testing.T) {
		// Given a configHandler without initialized shell
		handler, _ := setup(t)
		handler.(*configHandler).shell = nil

		// When SaveConfig is called
		err := handler.SaveConfig()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if err.Error() != "shell not initialized" {
			t.Errorf("Expected 'shell not initialized' error, got %v", err)
		}
	})

	t.Run("GetProjectRootError", func(t *testing.T) {
		// Given a configHandler with shell that fails to get project root
		handler, mocks := setup(t)
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("project root failed")
		}

		// When SaveConfig is called
		err := handler.SaveConfig()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error retrieving project root") {
			t.Errorf("Expected 'error retrieving project root' in error, got %v", err)
		}
	})

	t.Run("RootConfigExists_SkipsRootCreation", func(t *testing.T) {
		// Given a configHandler with existing root config
		handler, mocks := setup(t)

		tempDir := t.TempDir()
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return tempDir, nil
		}

		handler.(*configHandler).context = "test-context"

		// Create existing root config
		rootConfigPath := filepath.Join(tempDir, "windsor.yaml")
		originalContent := "version: v1alpha1\nexisting: config"
		os.WriteFile(rootConfigPath, []byte(originalContent), 0644)

		// When SaveConfig is called
		err := handler.SaveConfig()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And the root config should not be overwritten
		content, _ := os.ReadFile(rootConfigPath)
		if string(content) != originalContent {
			t.Errorf("Root config was overwritten when it should be preserved")
		}
	})

	t.Run("ContextExistsInRoot_SkipsContextCreation", func(t *testing.T) {
		// Given a configHandler with context existing in root config
		handler, mocks := setup(t)

		tempDir := t.TempDir()
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return tempDir, nil
		}

		handler.(*configHandler).context = "existing-context"

		// Setup config with existing context in root
		handler.(*configHandler).config.Contexts = map[string]*v1alpha1.Context{
			"existing-context": {
				Provider: stringPtr("local"),
			},
		}

		// Create existing root config file
		rootConfigPath := filepath.Join(tempDir, "windsor.yaml")
		os.WriteFile(rootConfigPath, []byte("version: v1alpha1"), 0644)

		// When SaveConfig is called
		err := handler.SaveConfig()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And the context config should not be created
		contextConfigPath := filepath.Join(tempDir, "contexts", "existing-context", "windsor.yaml")
		if _, err := os.Stat(contextConfigPath); !os.IsNotExist(err) {
			t.Errorf("Context config was created when it shouldn't have been")
		}
	})

	t.Run("ContextConfigExists_SkipsContextCreation", func(t *testing.T) {
		// Given a configHandler with existing context config file
		handler, mocks := setup(t)

		tempDir := t.TempDir()
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return tempDir, nil
		}

		handler.(*configHandler).context = "test-context"

		// Create existing context config
		contextDir := filepath.Join(tempDir, "contexts", "test-context")
		os.MkdirAll(contextDir, 0755)
		contextConfigPath := filepath.Join(contextDir, "windsor.yaml")
		originalContent := "provider: local\nexisting: config"
		os.WriteFile(contextConfigPath, []byte(originalContent), 0644)

		// When SaveConfig is called
		err := handler.SaveConfig()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And the context config should not be overwritten
		content, _ := os.ReadFile(contextConfigPath)
		if string(content) != originalContent {
			t.Errorf("Context config was overwritten when it should be preserved")
		}
	})

	t.Run("RootConfigMarshalError", func(t *testing.T) {
		// Given a configHandler with marshal error for root config
		handler, mocks := setup(t)

		tempDir := t.TempDir()
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return tempDir, nil
		}

		handler.(*configHandler).context = "test-context"

		// Override Stat to return not exists (so files will be created)
		handler.(*configHandler).shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		// Mock YamlMarshal to return error
		handler.(*configHandler).shims.YamlMarshal = func(v interface{}) ([]byte, error) {
			return nil, fmt.Errorf("marshal error")
		}

		// When SaveConfig is called
		err := handler.SaveConfig()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error marshalling root config") {
			t.Errorf("Expected 'error marshalling root config' in error, got %v", err)
		}
	})

	t.Run("RootConfigWriteError", func(t *testing.T) {
		// Given a configHandler with write error for root config
		handler, mocks := setup(t)

		tempDir := t.TempDir()
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return tempDir, nil
		}

		handler.(*configHandler).context = "test-context"

		// Override Stat to return not exists (so files will be created)
		handler.(*configHandler).shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		// Mock WriteFile to return error
		handler.(*configHandler).shims.WriteFile = func(filename string, data []byte, perm os.FileMode) error {
			return fmt.Errorf("write error")
		}

		// When SaveConfig is called
		err := handler.SaveConfig()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error writing root config") {
			t.Errorf("Expected 'error writing root config' in error, got %v", err)
		}
	})

	t.Run("ContextDirectoryCreationError", func(t *testing.T) {
		// Given a configHandler with directory creation error
		handler, mocks := setup(t)

		tempDir := t.TempDir()
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return tempDir, nil
		}

		handler.(*configHandler).context = "test-context"

		// Override Stat to return not exists (so files will be created)
		handler.(*configHandler).shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		// Mock MkdirAll to return error
		handler.(*configHandler).shims.MkdirAll = func(path string, perm os.FileMode) error {
			return fmt.Errorf("mkdir error")
		}

		// When SaveConfig is called
		err := handler.SaveConfig()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error creating context directory") {
			t.Errorf("Expected 'error creating context directory' in error, got %v", err)
		}
	})

	t.Run("ContextConfigMarshalError", func(t *testing.T) {
		// Given a configHandler with marshal error for context config
		handler, mocks := setup(t)

		tempDir := t.TempDir()
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return tempDir, nil
		}

		handler.(*configHandler).context = "test-context"

		// Override Stat to return not exists (so files will be created)
		handler.(*configHandler).shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		// Track marshal calls to return error on second call (context config)
		marshalCallCount := 0
		handler.(*configHandler).shims.YamlMarshal = func(v interface{}) ([]byte, error) {
			marshalCallCount++
			if marshalCallCount == 2 {
				return nil, fmt.Errorf("context marshal error")
			}
			return []byte("version: v1alpha1"), nil
		}

		// When SaveConfig is called
		err := handler.SaveConfig()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error marshalling context config") {
			t.Errorf("Expected 'error marshalling context config' in error, got %v", err)
		}
	})

	t.Run("ContextConfigWriteError", func(t *testing.T) {
		// Given a configHandler with write error for context config
		handler, mocks := setup(t)

		tempDir := t.TempDir()
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return tempDir, nil
		}

		handler.(*configHandler).context = "test-context"

		// Override Stat to return not exists (so files will be created)
		handler.(*configHandler).shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		// Track write calls to return error on second call (context config)
		writeCallCount := 0
		handler.(*configHandler).shims.WriteFile = func(filename string, data []byte, perm os.FileMode) error {
			writeCallCount++
			if writeCallCount == 2 {
				return fmt.Errorf("context write error")
			}
			return nil
		}

		// When SaveConfig is called
		err := handler.SaveConfig()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error writing context config") {
			t.Errorf("Expected 'error writing context config' in error, got %v", err)
		}
	})

	t.Run("BothFilesExist_NoOperationsPerformed", func(t *testing.T) {
		// Given a configHandler with both root and context configs existing
		handler, mocks := setup(t)

		tempDir := t.TempDir()
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return tempDir, nil
		}

		handler.(*configHandler).context = "test-context"

		// Create both existing files
		rootConfigPath := filepath.Join(tempDir, "windsor.yaml")
		originalRootContent := "version: v1alpha1\nexisting: root"
		os.WriteFile(rootConfigPath, []byte(originalRootContent), 0644)

		contextDir := filepath.Join(tempDir, "contexts", "test-context")
		os.MkdirAll(contextDir, 0755)
		contextConfigPath := filepath.Join(contextDir, "windsor.yaml")
		originalContextContent := "provider: local\nexisting: context"
		os.WriteFile(contextConfigPath, []byte(originalContextContent), 0644)

		// When SaveConfig is called
		err := handler.SaveConfig()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And both files should remain unchanged
		rootContent, _ := os.ReadFile(rootConfigPath)
		if string(rootContent) != originalRootContent {
			t.Errorf("Root config was modified when it shouldn't have been")
		}

		contextContent, _ := os.ReadFile(contextConfigPath)
		if string(contextContent) != originalContextContent {
			t.Errorf("Context config was modified when it shouldn't have been")
		}
	})

	t.Run("EmptyVersion_UsesEmptyString", func(t *testing.T) {
		// Given a configHandler with empty version
		handler, mocks := setup(t)

		tempDir := t.TempDir()
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return tempDir, nil
		}

		handler.(*configHandler).context = "test-context"
		handler.(*configHandler).config.Version = ""

		// Override shims to actually work with the real filesystem
		handler.(*configHandler).shims.WriteFile = func(filename string, data []byte, perm os.FileMode) error {
			return os.WriteFile(filename, data, perm)
		}
		handler.(*configHandler).shims.MkdirAll = func(path string, perm os.FileMode) error {
			return os.MkdirAll(path, perm)
		}
		handler.(*configHandler).shims.Stat = func(name string) (os.FileInfo, error) {
			return os.Stat(name)
		}

		// When SaveConfig is called
		err := handler.SaveConfig()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And the root config should contain empty version
		rootConfigPath := filepath.Join(tempDir, "windsor.yaml")
		content, _ := os.ReadFile(rootConfigPath)
		if !strings.Contains(string(content), "version: \"\"") && !strings.Contains(string(content), "version:") {
			t.Errorf("Expected version field in config, got: %s", string(content))
		}
	})

	t.Run("CreateContextConfigWhenNotInRootConfig", func(t *testing.T) {
		// Given a configHandler with existing root config
		handler, mocks := setup(t)

		tempDir := t.TempDir()
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return tempDir, nil
		}

		// Create existing root config that doesn't include the current context
		rootConfigPath := filepath.Join(tempDir, "windsor.yaml")
		rootConfig := `version: v1alpha1
contexts:
  different-context:
    provider: local`
		os.WriteFile(rootConfigPath, []byte(rootConfig), 0644)

		// Load the existing root config
		if err := handler.LoadConfig(rootConfigPath); err != nil {
			t.Fatalf("Failed to load root config: %v", err)
		}

		// Set the current context to one not defined in root config
		handler.(*configHandler).context = "new-context"
		handler.Set("contexts.new-context.provider", "local")

		// When SaveConfig is called
		err := handler.SaveConfig()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And the context config should be created since the context is not in root config
		contextConfigPath := filepath.Join(tempDir, "contexts", "new-context", "windsor.yaml")
		if _, err := handler.(*configHandler).shims.Stat(contextConfigPath); os.IsNotExist(err) {
			t.Fatalf("Context config file was not created at %s, but should have been since context is not in root config", contextConfigPath)
		}

		// And the root config should not be overwritten
		rootContent, _ := os.ReadFile(rootConfigPath)
		if !strings.Contains(string(rootContent), "different-context") {
			t.Errorf("Root config appears to have been overwritten")
		}
	})

	t.Run("CreateContextConfigWhenRootConfigExistsWithoutContexts", func(t *testing.T) {
		// Given a configHandler with existing root config that has NO contexts section
		handler, mocks := setup(t)

		tempDir := t.TempDir()
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return tempDir, nil
		}

		// Create existing root config with only version (this is the most common case for user's issue)
		rootConfigPath := filepath.Join(tempDir, "windsor.yaml")
		rootConfig := `version: v1alpha1`
		os.WriteFile(rootConfigPath, []byte(rootConfig), 0644)

		// Load the existing root config
		if err := handler.LoadConfig(rootConfigPath); err != nil {
			t.Fatalf("Failed to load root config: %v", err)
		}

		// Set the current context to local (typical init scenario)
		handler.(*configHandler).context = "local"
		handler.Set("contexts.local.provider", "local")

		// When SaveConfig is called
		err := handler.SaveConfig()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And the context config should be created since the context is not in root config
		contextConfigPath := filepath.Join(tempDir, "contexts", "local", "windsor.yaml")
		if _, err := handler.(*configHandler).shims.Stat(contextConfigPath); os.IsNotExist(err) {
			t.Fatalf("Context config file was not created at %s, but should have been since context is not in root config", contextConfigPath)
		}

		// And the root config should not be overwritten
		rootContent, _ := os.ReadFile(rootConfigPath)
		if !strings.Contains(string(rootContent), "version: v1alpha1") {
			t.Errorf("Root config appears to have been overwritten")
		}
	})

	t.Run("SimulateInitPipelineWorkflow", func(t *testing.T) {
		// Given a configHandler simulating the exact init pipeline workflow
		handler, mocks := setup(t)

		tempDir := t.TempDir()
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return tempDir, nil
		}

		// Create existing root config with only version (common in real scenarios)
		rootConfigPath := filepath.Join(tempDir, "windsor.yaml")
		rootConfig := `version: v1alpha1`
		os.WriteFile(rootConfigPath, []byte(rootConfig), 0644)

		// Step 1: Load existing config like init pipeline does in BasePipeline.Initialize
		if err := handler.LoadConfig(rootConfigPath); err != nil {
			t.Fatalf("Failed to load root config: %v", err)
		}

		// Step 2: Set context like init pipeline does
		if err := handler.SetContext("local"); err != nil {
			t.Fatalf("Failed to set context: %v", err)
		}

		// Step 3: Set default configuration like init pipeline does
		if err := handler.SetDefault(DefaultConfig); err != nil {
			t.Fatalf("Failed to set default config: %v", err)
		}

		// Step 4: Generate context ID like init pipeline does
		if err := handler.GenerateContextID(); err != nil {
			t.Fatalf("Failed to generate context ID: %v", err)
		}

		// Step 5: Save config like init pipeline does
		err := handler.SaveConfig()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And the context config should be created since context is not defined in root
		contextConfigPath := filepath.Join(tempDir, "contexts", "local", "windsor.yaml")
		if _, err := handler.(*configHandler).shims.Stat(contextConfigPath); os.IsNotExist(err) {
			t.Errorf("Context config file was not created at %s, this reproduces the user's issue", contextConfigPath)
		}

		// And the root config should not be overwritten
		rootContent, _ := os.ReadFile(rootConfigPath)
		if !strings.Contains(string(rootContent), "version: v1alpha1") {
			t.Errorf("Root config appears to have been overwritten")
		}
	})

	t.Run("DebugSaveConfigLogic", func(t *testing.T) {
		// Given a configHandler with existing root config with no contexts
		handler, mocks := setup(t)

		tempDir := t.TempDir()
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return tempDir, nil
		}

		// Create existing root config with only version (user's scenario)
		rootConfigPath := filepath.Join(tempDir, "windsor.yaml")
		rootConfig := `version: v1alpha1`
		os.WriteFile(rootConfigPath, []byte(rootConfig), 0644)

		// Load the existing root config
		if err := handler.LoadConfig(rootConfigPath); err != nil {
			t.Fatalf("Failed to load root config: %v", err)
		}

		// Set context and config values
		handler.(*configHandler).context = "local"
		handler.Set("contexts.local.provider", "local")

		// Debug: Check what's in the config before SaveConfig
		t.Logf("Config.Contexts before SaveConfig: %+v", handler.(*configHandler).config.Contexts)
		if handler.(*configHandler).config.Contexts != nil {
			if _, exists := handler.(*configHandler).config.Contexts["local"]; exists {
				t.Logf("local context exists in root config")
			} else {
				t.Logf("local context does NOT exist in root config")
			}
		} else {
			t.Logf("Config.Contexts is nil")
		}

		// When SaveConfig is called
		err := handler.SaveConfig()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// Check if context config was created
		contextConfigPath := filepath.Join(tempDir, "contexts", "local", "windsor.yaml")
		if _, err := handler.(*configHandler).shims.Stat(contextConfigPath); os.IsNotExist(err) {
			t.Logf("Context config file was NOT created at %s", contextConfigPath)
		} else {
			t.Logf("Context config file WAS created at %s", contextConfigPath)
		}
	})

	t.Run("ContextNotSetInRootConfigInitially", func(t *testing.T) {
		// Given a configHandler that mimics the exact init flow
		handler, mocks := setup(t)

		tempDir := t.TempDir()
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return tempDir, nil
		}

		// Create existing root config with only version (user's scenario)
		rootConfigPath := filepath.Join(tempDir, "windsor.yaml")
		rootConfig := `version: v1alpha1`
		os.WriteFile(rootConfigPath, []byte(rootConfig), 0644)

		// Load the existing root config
		if err := handler.LoadConfig(rootConfigPath); err != nil {
			t.Fatalf("Failed to load root config: %v", err)
		}

		// Set the context but DON'T call Set() to add context data yet
		handler.(*configHandler).context = "local"

		// Debug: Check state before adding any context data
		t.Logf("Config.Contexts before setting any context data: %+v", handler.(*configHandler).config.Contexts)

		// When SaveConfig is called without any context configuration being set
		err := handler.SaveConfig()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// Check if context config was created
		contextConfigPath := filepath.Join(tempDir, "contexts", "local", "windsor.yaml")
		if _, err := handler.(*configHandler).shims.Stat(contextConfigPath); os.IsNotExist(err) {
			t.Errorf("Context config file was NOT created at %s - this reproduces the user's issue", contextConfigPath)
		} else {
			t.Logf("Context config file WAS created at %s", contextConfigPath)
		}
	})

	t.Run("ReproduceActualIssue", func(t *testing.T) {
		// Given a real-world scenario where a root windsor.yaml exists with only version
		handler, mocks := setup(t)

		tempDir := t.TempDir()
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return tempDir, nil
		}

		// Create existing root config with only version (exact user scenario)
		rootConfigPath := filepath.Join(tempDir, "windsor.yaml")
		rootConfig := `version: v1alpha1`
		os.WriteFile(rootConfigPath, []byte(rootConfig), 0644)

		// Step 1: Load existing config like init pipeline does
		if err := handler.LoadConfig(rootConfigPath); err != nil {
			t.Fatalf("Failed to load root config: %v", err)
		}

		// Step 2: Set context
		if err := handler.SetContext("local"); err != nil {
			t.Fatalf("Failed to set context: %v", err)
		}

		// Step 3: Set default configuration (this would add context data)
		if err := handler.SetDefault(DefaultConfig); err != nil {
			t.Fatalf("Failed to set default config: %v", err)
		}

		// Step 4: Generate context ID
		if err := handler.GenerateContextID(); err != nil {
			t.Fatalf("Failed to generate context ID: %v", err)
		}

		// Debug: Check config state before SaveConfig
		t.Logf("Config before SaveConfig: %+v", handler.(*configHandler).config)
		if handler.(*configHandler).config.Contexts != nil {
			if ctx, exists := handler.(*configHandler).config.Contexts["local"]; exists {
				t.Logf("local context exists in config: %+v", ctx)
			} else {
				t.Logf("local context does NOT exist in config")
			}
		} else {
			t.Logf("Config.Contexts is nil")
		}

		// Step 5: Save config (the critical call)
		err := handler.SaveConfig()
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// Check if context config file was created
		contextConfigPath := filepath.Join(tempDir, "contexts", "local", "windsor.yaml")
		if _, err := handler.(*configHandler).shims.Stat(contextConfigPath); os.IsNotExist(err) {
			t.Errorf("Context config file was NOT created at %s - this is the bug!", contextConfigPath)
		} else {
			content, _ := os.ReadFile(contextConfigPath)
			t.Logf("Context config file WAS created with content: %s", string(content))
		}

		// Check root config wasn't overwritten
		rootContent, _ := os.ReadFile(rootConfigPath)
		if !strings.Contains(string(rootContent), "version: v1alpha1") {
			t.Errorf("Root config appears to have been overwritten: %s", string(rootContent))
		}
	})

	t.Run("SavesContextValuesWhenLoaded", func(t *testing.T) {
		// Given a configHandler with loaded contextValues
		handler, mocks := setup(t)

		tempDir := t.TempDir()
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return tempDir, nil
		}

		// Use real filesystem operations for this test
		handler.(*configHandler).shims = NewShims()
		handler.(*configHandler).shims.Getenv = func(key string) string {
			if key == "WINDSOR_CONTEXT" {
				return "test-context"
			}
			return ""
		}

		handler.(*configHandler).context = "test-context"
		handler.(*configHandler).loaded = true
		handler.(*configHandler).contextValues = map[string]any{
			"test_key": "test_value",
			"number":   42,
		}

		// When SaveConfig is called
		err := handler.SaveConfig()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And values.yaml should be created with the context values
		valuesPath := filepath.Join(tempDir, "contexts", "test-context", "values.yaml")
		if _, err := os.Stat(valuesPath); os.IsNotExist(err) {
			t.Fatalf("values.yaml was not created at %s", valuesPath)
		}

		// And the content should match contextValues
		content, err := os.ReadFile(valuesPath)
		if err != nil {
			t.Fatalf("Failed to read values.yaml: %v", err)
		}
		if !strings.Contains(string(content), "test_key") {
			t.Errorf("values.yaml should contain 'test_key', got: %s", string(content))
		}
		if !strings.Contains(string(content), "test_value") {
			t.Errorf("values.yaml should contain 'test_value', got: %s", string(content))
		}
	})

	t.Run("SavesContextValuesEvenWhenNotLoaded", func(t *testing.T) {
		handler, mocks := setup(t)

		tempDir := t.TempDir()
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return tempDir, nil
		}

		contextValue := "test-context"
		handler.(*configHandler).shims.WriteFile = os.WriteFile
		handler.(*configHandler).shims.MkdirAll = os.MkdirAll
		handler.(*configHandler).shims.Stat = os.Stat
		handler.(*configHandler).shims.Getenv = func(key string) string {
			if key == "WINDSOR_CONTEXT" {
				return contextValue
			}
			return ""
		}
		handler.(*configHandler).shims.Setenv = func(key, value string) error {
			if key == "WINDSOR_CONTEXT" {
				contextValue = value
			}
			return nil
		}

		if err := handler.SetContext("test-context"); err != nil {
			t.Fatalf("Failed to set context: %v", err)
		}

		handler.(*configHandler).loaded = false
		handler.(*configHandler).contextValues = map[string]any{
			"test_key": "test_value",
		}

		err := handler.SaveConfig()

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		valuesPath := filepath.Join(tempDir, "contexts", "test-context", "values.yaml")

		if _, err := os.Stat(valuesPath); os.IsNotExist(err) {
			contextDir := filepath.Join(tempDir, "contexts", "test-context")
			files, _ := os.ReadDir(contextDir)
			t.Logf("Files in context directory: %v", files)
			t.Errorf("values.yaml should have been created even when not loaded")
		}

		content, err := os.ReadFile(valuesPath)
		if err != nil {
			t.Fatalf("Failed to read values.yaml: %v", err)
		}

		if !strings.Contains(string(content), "test_value") {
			t.Errorf("values.yaml should contain 'test_value', got: %s", string(content))
		}
	})

	t.Run("SkipsSavingContextValuesWhenNil", func(t *testing.T) {
		// Given a configHandler with nil contextValues
		handler, mocks := setup(t)

		tempDir := t.TempDir()
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return tempDir, nil
		}

		handler.(*configHandler).context = "test-context"
		handler.(*configHandler).loaded = true
		handler.(*configHandler).contextValues = nil

		// When SaveConfig is called
		err := handler.SaveConfig()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And values.yaml should NOT be created
		valuesPath := filepath.Join(tempDir, "contexts", "test-context", "values.yaml")
		if _, err := os.Stat(valuesPath); !os.IsNotExist(err) {
			t.Errorf("values.yaml should not have been created when contextValues is nil")
		}
	})

	t.Run("SkipsSavingContextValuesWhenEmpty", func(t *testing.T) {
		// Given a configHandler with empty contextValues
		handler, mocks := setup(t)

		tempDir := t.TempDir()
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return tempDir, nil
		}

		handler.(*configHandler).context = "test-context"
		handler.(*configHandler).loaded = true
		handler.(*configHandler).contextValues = map[string]any{}

		// When SaveConfig is called
		err := handler.SaveConfig()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And values.yaml should NOT be created
		valuesPath := filepath.Join(tempDir, "contexts", "test-context", "values.yaml")
		if _, err := os.Stat(valuesPath); !os.IsNotExist(err) {
			t.Errorf("values.yaml should not have been created when contextValues is empty")
		}
	})

	t.Run("SaveContextValuesError", func(t *testing.T) {
		// Given a configHandler with contextValues and a write error
		handler, mocks := setup(t)

		tempDir := t.TempDir()
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return tempDir, nil
		}

		// Use real filesystem operations but mock WriteFile to fail for values.yaml
		handler.(*configHandler).shims = NewShims()
		handler.(*configHandler).shims.Getenv = func(key string) string {
			if key == "WINDSOR_CONTEXT" {
				return "test-context"
			}
			return ""
		}
		handler.(*configHandler).shims.WriteFile = func(filename string, data []byte, perm os.FileMode) error {
			if strings.Contains(filename, "values.yaml") {
				return fmt.Errorf("write error")
			}
			return os.WriteFile(filename, data, perm)
		}

		handler.(*configHandler).context = "test-context"
		handler.(*configHandler).loaded = true
		handler.(*configHandler).contextValues = map[string]any{
			"test_key": "test_value",
		}

		// When SaveConfig is called
		err := handler.SaveConfig()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error saving values.yaml") {
			t.Errorf("Expected 'error saving values.yaml' in error, got %v", err)
		}
	})
}

func TestConfigHandler_GetString(t *testing.T) {
	setup := func(t *testing.T) (ConfigHandler, *Mocks) {
		mocks := setupMocks(t)
		handler := NewConfigHandler(mocks.Injector)
		handler.(*configHandler).shims = mocks.Shims
		if err := handler.Initialize(); err != nil {
			t.Fatalf("Failed to initialize handler: %v", err)
		}
		return handler, mocks
	}

	t.Run("WithNonExistentKey", func(t *testing.T) {
		// Given a handler with a context set
		handler, _ := setup(t)
		handler.(*configHandler).context = "default"

		// When getting a non-existent key
		got := handler.GetString("nonExistentKey")

		// Then an empty string should be returned
		expectedValue := ""
		if got != expectedValue {
			t.Errorf("GetString() = %v, expected %v", got, expectedValue)
		}
	})

	t.Run("GetStringWithDefaultValue", func(t *testing.T) {
		// Given a handler with a context set
		handler, _ := setup(t)
		handler.(*configHandler).context = "default"

		// When getting a non-existent key with a default value
		defaultValue := "defaultString"
		value := handler.GetString("non.existent.key", defaultValue)

		// Then the default value should be returned
		if value != defaultValue {
			t.Errorf("Expected value '%v', got '%v'", defaultValue, value)
		}
	})

	t.Run("WithExistingKey", func(t *testing.T) {
		// Given a handler with a context and existing key-value pair
		handler, _ := setup(t)
		handler.(*configHandler).context = "default"
		handler.(*configHandler).config = v1alpha1.Config{
			Contexts: map[string]*v1alpha1.Context{
				"default": {
					Environment: map[string]string{
						"existingKey": "existingValue",
					},
				},
			},
		}

		// When getting an existing key
		got := handler.GetString("environment.existingKey")

		// Then the value should be returned as a string
		expectedValue := "existingValue"
		if got != expectedValue {
			t.Errorf("GetString() = %v, expected %v", got, expectedValue)
		}
	})
}

func TestConfigHandler_GetInt(t *testing.T) {
	setup := func(t *testing.T) (ConfigHandler, *Mocks) {
		mocks := setupMocks(t)
		handler := NewConfigHandler(mocks.Injector)
		handler.(*configHandler).shims = mocks.Shims
		if err := handler.Initialize(); err != nil {
			t.Fatalf("Failed to initialize handler: %v", err)
		}
		return handler, mocks
	}

	t.Run("WithExistingNonIntegerKey", func(t *testing.T) {
		// Given a handler with a context and non-integer value
		handler, _ := setup(t)
		handler.(*configHandler).context = "default"
		handler.(*configHandler).config = v1alpha1.Config{
			Contexts: map[string]*v1alpha1.Context{
				"default": {
					AWS: &aws.AWSConfig{
						AWSEndpointURL: ptrString("notAnInt"),
					},
				},
			},
		}

		// When getting a key with non-integer value
		value := handler.GetInt("aws.aws_endpoint_url")

		// Then the default integer value should be returned
		expectedValue := 0
		if value != expectedValue {
			t.Errorf("Expected value %v, got %v", expectedValue, value)
		}
	})

	t.Run("WithNonExistentKey", func(t *testing.T) {
		// Given a handler with a context set
		handler, _ := setup(t)
		handler.(*configHandler).context = "default"

		// When getting a non-existent key
		value := handler.GetInt("nonExistentKey")

		// Then the default integer value should be returned
		expectedValue := 0
		if value != expectedValue {
			t.Errorf("Expected value %v, got %v", expectedValue, value)
		}
	})

	t.Run("WithNonExistentKeyAndDefaultValue", func(t *testing.T) {
		// Given a handler with a context set
		handler, _ := setup(t)
		handler.(*configHandler).context = "default"

		// When getting a non-existent key with a default value
		got := handler.GetInt("nonExistentKey", 99)

		// Then the provided default value should be returned
		expectedValue := 99
		if got != expectedValue {
			t.Errorf("GetInt() = %v, expected %v", got, expectedValue)
		}
	})

	t.Run("WithExistingIntegerKey", func(t *testing.T) {
		// Given a handler with a context and integer value
		handler, _ := setup(t)
		handler.(*configHandler).context = "default"
		handler.(*configHandler).config = v1alpha1.Config{
			Contexts: map[string]*v1alpha1.Context{
				"default": {
					Cluster: &cluster.ClusterConfig{
						ControlPlanes: cluster.NodeGroupConfig{
							Count: ptrInt(3),
						},
					},
				},
			},
		}

		// When getting an existing integer key
		got := handler.GetInt("cluster.controlplanes.count")

		// Then the integer value should be returned
		expectedValue := 3
		if got != expectedValue {
			t.Errorf("GetInt() = %v, expected %v", got, expectedValue)
		}
	})
}

func TestConfigHandler_GetBool(t *testing.T) {
	setup := func(t *testing.T) (ConfigHandler, *Mocks) {
		mocks := setupMocks(t)
		handler := NewConfigHandler(mocks.Injector)
		handler.(*configHandler).shims = mocks.Shims
		if err := handler.Initialize(); err != nil {
			t.Fatalf("Failed to initialize handler: %v", err)
		}
		return handler, mocks
	}

	t.Run("WithExistingBooleanKey", func(t *testing.T) {
		// Given a handler with a context and boolean value
		handler, _ := setup(t)
		handler.(*configHandler).context = "default"
		handler.(*configHandler).config = v1alpha1.Config{
			Contexts: map[string]*v1alpha1.Context{
				"default": {
					AWS: &aws.AWSConfig{
						Enabled: ptrBool(true),
					},
				},
			},
		}

		// When getting an existing boolean key
		got := handler.GetBool("aws.enabled")

		// Then the boolean value should be returned
		expectedValue := true
		if got != expectedValue {
			t.Errorf("GetBool() = %v, expected %v", got, expectedValue)
		}
	})

	t.Run("WithExistingNonBooleanKey", func(t *testing.T) {
		// Given a handler with a context set
		handler, _ := setup(t)
		handler.(*configHandler).context = "default"

		// When setting a non-boolean value for the key
		handler.Set("contexts.default.aws.aws_endpoint_url", "notABool")

		// When getting an existing key with a non-boolean value
		value := handler.GetBool("aws.aws_endpoint_url")
		expectedValue := false

		// Then the default boolean value should be returned
		if value != expectedValue {
			t.Errorf("Expected value %v, got %v", expectedValue, value)
		}
	})

	t.Run("WithNonExistentKey", func(t *testing.T) {
		// Given a handler with a context set
		handler, _ := setup(t)
		handler.(*configHandler).context = "default"

		// When getting a non-existent key
		value := handler.GetBool("nonExistentKey")
		expectedValue := false

		// Then the default boolean value should be returned
		if value != expectedValue {
			t.Errorf("Expected value %v, got %v", expectedValue, value)
		}
	})

	t.Run("WithNonExistentKeyAndDefaultValue", func(t *testing.T) {
		// Given a handler with a context set
		handler, _ := setup(t)
		handler.(*configHandler).context = "default"

		// When getting a non-existent key with a default value
		got := handler.GetBool("nonExistentKey", false)

		// Then the provided default value should be returned
		expectedValue := false
		if got != expectedValue {
			t.Errorf("GetBool() = %v, expected %v", got, expectedValue)
		}
	})
}

func TestConfigHandler_GetStringSlice(t *testing.T) {
	setup := func(t *testing.T) (ConfigHandler, *Mocks) {
		mocks := setupMocks(t)
		handler := NewConfigHandler(mocks.Injector)
		handler.(*configHandler).shims = mocks.Shims
		if err := handler.Initialize(); err != nil {
			t.Fatalf("Failed to initialize handler: %v", err)
		}
		return handler, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a handler with a context containing a slice value
		handler, _ := setup(t)
		handler.(*configHandler).context = "default"
		handler.(*configHandler).config.Contexts = map[string]*v1alpha1.Context{
			"default": {
				Cluster: &cluster.ClusterConfig{
					Workers: cluster.NodeGroupConfig{
						HostPorts: []string{"50000:50002/tcp", "30080:8080/tcp", "30443:8443/tcp"},
					},
				},
			},
		}

		// When retrieving the slice value using GetStringSlice
		value := handler.GetStringSlice("cluster.workers.hostports")

		// Then the returned slice should match the expected slice
		expectedSlice := []string{"50000:50002/tcp", "30080:8080/tcp", "30443:8443/tcp"}
		if !reflect.DeepEqual(value, expectedSlice) {
			t.Errorf("Expected GetStringSlice to return %v, got %v", expectedSlice, value)
		}
	})

	t.Run("WithNonExistentKey", func(t *testing.T) {
		// Given a handler with a context set
		handler, _ := setup(t)
		handler.(*configHandler).context = "default"

		// When retrieving a non-existent key using GetStringSlice
		value := handler.GetStringSlice("nonExistentKey")

		// Then the returned value should be an empty slice
		if len(value) != 0 {
			t.Errorf("Expected GetStringSlice with non-existent key to return an empty slice, got %v", value)
		}
	})

	t.Run("WithNonExistentKeyAndDefaultValue", func(t *testing.T) {
		// Given a handler with a context set
		handler, _ := setup(t)
		handler.(*configHandler).context = "default"
		defaultValue := []string{"default1", "default2"}

		// When retrieving a non-existent key with a default value
		value := handler.GetStringSlice("nonExistentKey", defaultValue)

		// Then the returned value should match the default value
		if !reflect.DeepEqual(value, defaultValue) {
			t.Errorf("Expected GetStringSlice with default to return %v, got %v", defaultValue, value)
		}
	})

	t.Run("TypeMismatch", func(t *testing.T) {
		// Given a handler where the key exists but is not a slice
		handler, _ := setup(t)
		handler.(*configHandler).context = "default"
		handler.Set("contexts.default.cluster.workers.hostports", 123) // Set an int instead of slice

		// When retrieving the value using GetStringSlice
		value := handler.GetStringSlice("cluster.workers.hostports")

		// Then the returned slice should be empty
		if len(value) != 0 {
			t.Errorf("Expected empty slice due to type mismatch, got %v", value)
		}
	})
}

func TestConfigHandler_GetStringMap(t *testing.T) {
	setup := func(t *testing.T) (ConfigHandler, *Mocks) {
		mocks := setupMocks(t)
		handler := NewConfigHandler(mocks.Injector)
		handler.(*configHandler).shims = mocks.Shims
		if err := handler.Initialize(); err != nil {
			t.Fatalf("Failed to initialize handler: %v", err)
		}
		return handler, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a handler with a context set
		handler, _ := setup(t)
		handler.(*configHandler).context = "default"
		handler.(*configHandler).config.Contexts = map[string]*v1alpha1.Context{
			"default": {
				Environment: map[string]string{
					"KEY1": "value1",
					"KEY2": "value2",
				},
			},
		}

		// When retrieving the map value using GetStringMap
		value := handler.GetStringMap("environment")

		// Then the returned map should match the expected map
		expectedMap := map[string]string{"KEY1": "value1", "KEY2": "value2"}
		if !reflect.DeepEqual(value, expectedMap) {
			t.Errorf("Expected GetStringMap to return %v, got %v", expectedMap, value)
		}
	})

	t.Run("WithNonExistentKey", func(t *testing.T) {
		// Given a handler with a context set
		handler, _ := setup(t)
		handler.(*configHandler).context = "default"

		// When retrieving a non-existent key using GetStringMap
		value := handler.GetStringMap("nonExistentKey")

		// Then the returned value should be an empty map
		if !reflect.DeepEqual(value, map[string]string{}) {
			t.Errorf("Expected GetStringMap with non-existent key to return an empty map, got %v", value)
		}
	})

	t.Run("WithNonExistentKeyAndDefaultValue", func(t *testing.T) {
		// Given a handler with a context set
		handler, _ := setup(t)
		handler.(*configHandler).context = "default"
		defaultValue := map[string]string{"defaultKey1": "defaultValue1", "defaultKey2": "defaultValue2"}

		// When retrieving a non-existent key with a default value
		value := handler.GetStringMap("nonExistentKey", defaultValue)

		// Then the returned value should match the default value
		if !reflect.DeepEqual(value, defaultValue) {
			t.Errorf("Expected GetStringMap with default to return %v, got %v", defaultValue, value)
		}
	})

	t.Run("TypeMismatch", func(t *testing.T) {
		// Given a handler where the key exists but is not a map[string]string
		handler, _ := setup(t)
		handler.(*configHandler).context = "default"
		handler.Set("contexts.default.environment", 123) // Set an int instead of map

		// When retrieving the value using GetStringMap
		value := handler.GetStringMap("environment")

		// Then the returned map should be empty
		if len(value) != 0 {
			t.Errorf("Expected empty map due to type mismatch, got %v", value)
		}
	})
}

func TestConfigHandler_GetConfig(t *testing.T) {
	setup := func(t *testing.T) (ConfigHandler, *Mocks) {
		mocks := setupMocks(t)
		handler := NewConfigHandler(mocks.Injector)
		if err := handler.Initialize(); err != nil {
			t.Fatalf("Failed to initialize handler: %v", err)
		}
		handler.(*configHandler).shims = mocks.Shims
		return handler, mocks
	}

	t.Run("EmptyContext", func(t *testing.T) {
		// Given a handler with no context set
		handler, _ := setup(t)

		// When getting the config
		config := handler.GetConfig()

		// Then the default config should be returned
		if config == nil {
			t.Fatal("Expected default config, got nil")
		}
	})

	t.Run("NonExistentContext", func(t *testing.T) {
		// Given a handler with a non-existent context
		handler, _ := setup(t)
		handler.(*configHandler).context = "nonexistent"

		// When getting the config
		config := handler.GetConfig()

		// Then the default config should be returned
		if config == nil {
			t.Fatal("Expected default config, got nil")
		}
	})

	t.Run("ExistingContext", func(t *testing.T) {
		// Given a handler with an existing context
		handler, _ := setup(t)
		handler.(*configHandler).context = "test"

		// And a context with environment variables
		handler.(*configHandler).config.Contexts = map[string]*v1alpha1.Context{
			"test": {
				Environment: map[string]string{
					"TEST_VAR": "test_value",
				},
			},
		}

		// And default context with different environment variables
		handler.(*configHandler).defaultContextConfig = v1alpha1.Context{
			Environment: map[string]string{
				"DEFAULT_VAR": "default_value",
			},
		}

		// When getting the config
		config := handler.GetConfig()

		// Then the merged config should be returned
		if config == nil {
			t.Fatal("Expected merged config, got nil")
		}

		// And it should contain both environment variables
		if config.Environment["TEST_VAR"] != "test_value" {
			t.Errorf("Expected TEST_VAR to be 'test_value', got '%s'", config.Environment["TEST_VAR"])
		}
		if config.Environment["DEFAULT_VAR"] != "default_value" {
			t.Errorf("Expected DEFAULT_VAR to be 'default_value', got '%s'", config.Environment["DEFAULT_VAR"])
		}
	})

	t.Run("ContextOverridesDefault", func(t *testing.T) {
		// Given a handler with an existing context
		handler, _ := setup(t)
		handler.(*configHandler).context = "test"

		// And a context with environment variables that override defaults
		handler.(*configHandler).config.Contexts = map[string]*v1alpha1.Context{
			"test": {
				Environment: map[string]string{
					"SHARED_VAR": "context_value",
				},
			},
		}

		// And default context with the same environment variable
		handler.(*configHandler).defaultContextConfig = v1alpha1.Context{
			Environment: map[string]string{
				"SHARED_VAR": "default_value",
			},
		}

		// When getting the config
		config := handler.GetConfig()

		// Then the context value should override the default
		if config.Environment["SHARED_VAR"] != "context_value" {
			t.Errorf("Expected SHARED_VAR to be 'context_value', got '%s'", config.Environment["SHARED_VAR"])
		}
	})
}
