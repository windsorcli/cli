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
	"github.com/windsorcli/cli/api/v1alpha1/vm"
)

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

func TestNewYamlConfigHandler(t *testing.T) {
	setup := func(t *testing.T) (*YamlConfigHandler, *Mocks) {
		mocks := setupMocks(t)
		handler := NewYamlConfigHandler(mocks.Injector)
		handler.shims = mocks.Shims

		return handler, mocks
	}
	t.Run("Success", func(t *testing.T) {
		handler, _ := setup(t)

		// Then the handler should be successfully created and not be nil
		if handler == nil {
			t.Fatal("Expected non-nil YamlConfigHandler")
		}
	})
}

// =============================================================================
// Public Methods
// =============================================================================

func TestYamlConfigHandler_LoadConfig(t *testing.T) {
	setup := func(t *testing.T) (*YamlConfigHandler, *Mocks) {
		mocks := setupMocks(t)
		handler := NewYamlConfigHandler(mocks.Injector)
		handler.shims = mocks.Shims
		if err := handler.Initialize(); err != nil {
			t.Fatalf("Failed to initialize handler: %v", err)
		}
		return handler, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a set of safe mocks and a YamlConfigHandler
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
		if handler.path != configPath {
			t.Errorf("Expected path = %v, got = %v", configPath, handler.path)
		}
	})

	t.Run("CreateEmptyConfigFileIfNotExist", func(t *testing.T) {
		// Given a set of safe mocks and a YamlConfigHandler
		handler, _ := setup(t)

		// And a mocked osStat that returns ErrNotExist
		handler.shims.Stat = func(_ string) (os.FileInfo, error) {
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
		// Given a set of safe mocks and a YamlConfigHandler
		handler, _ := setup(t)

		// And a mocked osReadFile that returns an error
		handler.shims.ReadFile = func(filename string) ([]byte, error) {
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
		// Given a set of safe mocks and a YamlConfigHandler
		handler, _ := setup(t)

		// And a mocked yamlUnmarshal that returns an error
		handler.shims.YamlUnmarshal = func(data []byte, v any) error {
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
		// Given a set of safe mocks and a YamlConfigHandler
		handler, _ := setup(t)

		// And a mocked yamlUnmarshal that sets an unsupported version
		handler.shims.YamlUnmarshal = func(data []byte, v any) error {
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

func TestYamlConfigHandler_Get(t *testing.T) {
	setup := func(t *testing.T) (*YamlConfigHandler, *Mocks) {
		mocks := setupMocks(t)
		handler := NewYamlConfigHandler(mocks.Injector)
		handler.shims = mocks.Shims
		if err := handler.Initialize(); err != nil {
			t.Fatalf("Failed to initialize handler: %v", err)
		}
		return handler, mocks
	}

	t.Run("KeyNotUnderContexts", func(t *testing.T) {
		// Given a set of safe mocks and a YamlConfigHandler
		handler, mocks := setup(t)

		// And a mocked shell that returns a project root
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "/mock/project/root", nil
		}

		// And a mocked shims that handles context file
		handler.shims.ReadFile = func(filename string) ([]byte, error) {
			if filename == "/mock/project/root/.windsor/context" {
				return []byte("local"), nil
			}
			return nil, fmt.Errorf("file not found")
		}

		// And a config with proper initialization
		handler.config = v1alpha1.Config{
			Version: "v1alpha1",
			Contexts: map[string]*v1alpha1.Context{
				"local": {
					Environment: map[string]string{},
				},
			},
		}

		// And the context is set
		handler.context = "local"

		// When getting a key not under contexts
		val := handler.Get("nonContextKey")

		// Then nil should be returned
		if val != nil {
			t.Errorf("Expected nil for non-context key, got %v", val)
		}
	})

	t.Run("InvalidPath", func(t *testing.T) {
		// Given a set of safe mocks and a YamlConfigHandler
		handler, _ := setup(t)

		// When calling Get with an empty path
		value := handler.Get("")

		// Then nil should be returned
		if value != nil {
			t.Errorf("Expected nil for empty path, got %v", value)
		}
	})
}

func TestYamlConfigHandler_SaveConfig(t *testing.T) {
	setup := func(t *testing.T) (*YamlConfigHandler, *Mocks) {
		mocks := setupMocks(t)
		handler := NewYamlConfigHandler(mocks.Injector)
		handler.shims = mocks.Shims
		if err := handler.Initialize(); err != nil {
			t.Fatalf("Failed to initialize handler: %v", err)
		}
		return handler, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a YamlConfigHandler with a mocked shell
		handler, mocks := setup(t)

		tempDir := t.TempDir()
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return tempDir, nil
		}

		// And a context is set
		handler.context = "test-context"

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
		if _, err := handler.shims.Stat(rootConfigPath); os.IsNotExist(err) {
			t.Fatalf("Root config file was not created at %s", rootConfigPath)
		}

		// And the context config should exist
		contextConfigPath := filepath.Join(tempDir, "contexts", "test-context", "windsor.yaml")
		if _, err := handler.shims.Stat(contextConfigPath); os.IsNotExist(err) {
			t.Fatalf("Context config file was not created at %s", contextConfigPath)
		}
	})

	t.Run("WithOverwriteFalse", func(t *testing.T) {
		// Given a YamlConfigHandler with existing config files
		handler, mocks := setup(t)

		tempDir := t.TempDir()
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return tempDir, nil
		}

		handler.context = "test-context"

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
		// Given a YamlConfigHandler without initialized shell
		handler, _ := setup(t)
		handler.shell = nil

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
		// Given a YamlConfigHandler with shell that fails to get project root
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
		// Given a YamlConfigHandler with existing root config
		handler, mocks := setup(t)

		tempDir := t.TempDir()
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return tempDir, nil
		}

		handler.context = "test-context"

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
		// Given a YamlConfigHandler with context existing in root config
		handler, mocks := setup(t)

		tempDir := t.TempDir()
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return tempDir, nil
		}

		handler.context = "existing-context"

		// Setup config with existing context in root
		handler.config.Contexts = map[string]*v1alpha1.Context{
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
		// Given a YamlConfigHandler with existing context config file
		handler, mocks := setup(t)

		tempDir := t.TempDir()
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return tempDir, nil
		}

		handler.context = "test-context"

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
		// Given a YamlConfigHandler with marshal error for root config
		handler, mocks := setup(t)

		tempDir := t.TempDir()
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return tempDir, nil
		}

		handler.context = "test-context"

		// Override Stat to return not exists (so files will be created)
		handler.shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		// Mock YamlMarshal to return error
		handler.shims.YamlMarshal = func(v interface{}) ([]byte, error) {
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
		// Given a YamlConfigHandler with write error for root config
		handler, mocks := setup(t)

		tempDir := t.TempDir()
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return tempDir, nil
		}

		handler.context = "test-context"

		// Override Stat to return not exists (so files will be created)
		handler.shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		// Mock WriteFile to return error
		handler.shims.WriteFile = func(filename string, data []byte, perm os.FileMode) error {
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
		// Given a YamlConfigHandler with directory creation error
		handler, mocks := setup(t)

		tempDir := t.TempDir()
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return tempDir, nil
		}

		handler.context = "test-context"

		// Override Stat to return not exists (so files will be created)
		handler.shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		// Mock MkdirAll to return error
		handler.shims.MkdirAll = func(path string, perm os.FileMode) error {
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
		// Given a YamlConfigHandler with marshal error for context config
		handler, mocks := setup(t)

		tempDir := t.TempDir()
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return tempDir, nil
		}

		handler.context = "test-context"

		// Override Stat to return not exists (so files will be created)
		handler.shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		// Track marshal calls to return error on second call (context config)
		marshalCallCount := 0
		handler.shims.YamlMarshal = func(v interface{}) ([]byte, error) {
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
		// Given a YamlConfigHandler with write error for context config
		handler, mocks := setup(t)

		tempDir := t.TempDir()
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return tempDir, nil
		}

		handler.context = "test-context"

		// Override Stat to return not exists (so files will be created)
		handler.shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		// Track write calls to return error on second call (context config)
		writeCallCount := 0
		handler.shims.WriteFile = func(filename string, data []byte, perm os.FileMode) error {
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
		// Given a YamlConfigHandler with both root and context configs existing
		handler, mocks := setup(t)

		tempDir := t.TempDir()
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return tempDir, nil
		}

		handler.context = "test-context"

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
		// Given a YamlConfigHandler with empty version
		handler, mocks := setup(t)

		tempDir := t.TempDir()
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return tempDir, nil
		}

		handler.context = "test-context"
		handler.config.Version = ""

		// Override shims to actually work with the real filesystem
		handler.shims.WriteFile = func(filename string, data []byte, perm os.FileMode) error {
			return os.WriteFile(filename, data, perm)
		}
		handler.shims.MkdirAll = func(path string, perm os.FileMode) error {
			return os.MkdirAll(path, perm)
		}
		handler.shims.Stat = func(name string) (os.FileInfo, error) {
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
		// Given a YamlConfigHandler with existing root config
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
		handler.context = "new-context"
		handler.Set("contexts.new-context.provider", "local")

		// When SaveConfig is called
		err := handler.SaveConfig()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And the context config should be created since the context is not in root config
		contextConfigPath := filepath.Join(tempDir, "contexts", "new-context", "windsor.yaml")
		if _, err := handler.shims.Stat(contextConfigPath); os.IsNotExist(err) {
			t.Fatalf("Context config file was not created at %s, but should have been since context is not in root config", contextConfigPath)
		}

		// And the root config should not be overwritten
		rootContent, _ := os.ReadFile(rootConfigPath)
		if !strings.Contains(string(rootContent), "different-context") {
			t.Errorf("Root config appears to have been overwritten")
		}
	})

	t.Run("CreateContextConfigWhenRootConfigExistsWithoutContexts", func(t *testing.T) {
		// Given a YamlConfigHandler with existing root config that has NO contexts section
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
		handler.context = "local"
		handler.Set("contexts.local.provider", "local")

		// When SaveConfig is called
		err := handler.SaveConfig()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And the context config should be created since the context is not in root config
		contextConfigPath := filepath.Join(tempDir, "contexts", "local", "windsor.yaml")
		if _, err := handler.shims.Stat(contextConfigPath); os.IsNotExist(err) {
			t.Fatalf("Context config file was not created at %s, but should have been since context is not in root config", contextConfigPath)
		}

		// And the root config should not be overwritten
		rootContent, _ := os.ReadFile(rootConfigPath)
		if !strings.Contains(string(rootContent), "version: v1alpha1") {
			t.Errorf("Root config appears to have been overwritten")
		}
	})

	t.Run("SimulateInitPipelineWorkflow", func(t *testing.T) {
		// Given a YamlConfigHandler simulating the exact init pipeline workflow
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
		if _, err := handler.shims.Stat(contextConfigPath); os.IsNotExist(err) {
			t.Errorf("Context config file was not created at %s, this reproduces the user's issue", contextConfigPath)
		}

		// And the root config should not be overwritten
		rootContent, _ := os.ReadFile(rootConfigPath)
		if !strings.Contains(string(rootContent), "version: v1alpha1") {
			t.Errorf("Root config appears to have been overwritten")
		}
	})

	t.Run("DebugSaveConfigLogic", func(t *testing.T) {
		// Given a YamlConfigHandler with existing root config with no contexts
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
		handler.context = "local"
		handler.Set("contexts.local.provider", "local")

		// Debug: Check what's in the config before SaveConfig
		t.Logf("Config.Contexts before SaveConfig: %+v", handler.config.Contexts)
		if handler.config.Contexts != nil {
			if _, exists := handler.config.Contexts["local"]; exists {
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
		if _, err := handler.shims.Stat(contextConfigPath); os.IsNotExist(err) {
			t.Logf("Context config file was NOT created at %s", contextConfigPath)
		} else {
			t.Logf("Context config file WAS created at %s", contextConfigPath)
		}
	})

	t.Run("ContextNotSetInRootConfigInitially", func(t *testing.T) {
		// Given a YamlConfigHandler that mimics the exact init flow
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
		handler.context = "local"

		// Debug: Check state before adding any context data
		t.Logf("Config.Contexts before setting any context data: %+v", handler.config.Contexts)

		// When SaveConfig is called without any context configuration being set
		err := handler.SaveConfig()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// Check if context config was created
		contextConfigPath := filepath.Join(tempDir, "contexts", "local", "windsor.yaml")
		if _, err := handler.shims.Stat(contextConfigPath); os.IsNotExist(err) {
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
		t.Logf("Config before SaveConfig: %+v", handler.config)
		if handler.config.Contexts != nil {
			if ctx, exists := handler.config.Contexts["local"]; exists {
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
		if _, err := handler.shims.Stat(contextConfigPath); os.IsNotExist(err) {
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
}

func TestYamlConfigHandler_GetString(t *testing.T) {
	setup := func(t *testing.T) (*YamlConfigHandler, *Mocks) {
		mocks := setupMocks(t)
		handler := NewYamlConfigHandler(mocks.Injector)
		handler.shims = mocks.Shims
		if err := handler.Initialize(); err != nil {
			t.Fatalf("Failed to initialize handler: %v", err)
		}
		return handler, mocks
	}

	t.Run("WithNonExistentKey", func(t *testing.T) {
		// Given a handler with a context set
		handler, _ := setup(t)
		handler.context = "default"

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
		handler.context = "default"

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
		handler.context = "default"
		handler.config = v1alpha1.Config{
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

func TestYamlConfigHandler_GetInt(t *testing.T) {
	setup := func(t *testing.T) (*YamlConfigHandler, *Mocks) {
		mocks := setupMocks(t)
		handler := NewYamlConfigHandler(mocks.Injector)
		handler.shims = mocks.Shims
		if err := handler.Initialize(); err != nil {
			t.Fatalf("Failed to initialize handler: %v", err)
		}
		return handler, mocks
	}

	t.Run("WithExistingNonIntegerKey", func(t *testing.T) {
		// Given a handler with a context and non-integer value
		handler, _ := setup(t)
		handler.context = "default"
		handler.config = v1alpha1.Config{
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
		handler.context = "default"

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
		handler.context = "default"

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
		handler.context = "default"
		handler.config = v1alpha1.Config{
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

func TestYamlConfigHandler_GetBool(t *testing.T) {
	setup := func(t *testing.T) (*YamlConfigHandler, *Mocks) {
		mocks := setupMocks(t)
		handler := NewYamlConfigHandler(mocks.Injector)
		handler.shims = mocks.Shims
		if err := handler.Initialize(); err != nil {
			t.Fatalf("Failed to initialize handler: %v", err)
		}
		return handler, mocks
	}

	t.Run("WithExistingBooleanKey", func(t *testing.T) {
		// Given a handler with a context and boolean value
		handler, _ := setup(t)
		handler.context = "default"
		handler.config = v1alpha1.Config{
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
		handler.context = "default"

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
		handler.context = "default"

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
		handler.context = "default"

		// When getting a non-existent key with a default value
		got := handler.GetBool("nonExistentKey", false)

		// Then the provided default value should be returned
		expectedValue := false
		if got != expectedValue {
			t.Errorf("GetBool() = %v, expected %v", got, expectedValue)
		}
	})
}

func TestYamlConfigHandler_GetStringSlice(t *testing.T) {
	setup := func(t *testing.T) (*YamlConfigHandler, *Mocks) {
		mocks := setupMocks(t)
		handler := NewYamlConfigHandler(mocks.Injector)
		handler.shims = mocks.Shims
		if err := handler.Initialize(); err != nil {
			t.Fatalf("Failed to initialize handler: %v", err)
		}
		return handler, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a handler with a context containing a slice value
		handler, _ := setup(t)
		handler.context = "default"
		handler.config.Contexts = map[string]*v1alpha1.Context{
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
		handler.context = "default"

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
		handler.context = "default"
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
		handler.context = "default"
		handler.Set("contexts.default.cluster.workers.hostports", 123) // Set an int instead of slice

		// When retrieving the value using GetStringSlice
		value := handler.GetStringSlice("cluster.workers.hostports")

		// Then the returned slice should be empty
		if len(value) != 0 {
			t.Errorf("Expected empty slice due to type mismatch, got %v", value)
		}
	})
}

func TestYamlConfigHandler_GetStringMap(t *testing.T) {
	setup := func(t *testing.T) (*YamlConfigHandler, *Mocks) {
		mocks := setupMocks(t)
		handler := NewYamlConfigHandler(mocks.Injector)
		handler.shims = mocks.Shims
		if err := handler.Initialize(); err != nil {
			t.Fatalf("Failed to initialize handler: %v", err)
		}
		return handler, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a handler with a context set
		handler, _ := setup(t)
		handler.context = "default"
		handler.config.Contexts = map[string]*v1alpha1.Context{
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
		handler.context = "default"

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
		handler.context = "default"
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
		handler.context = "default"
		handler.Set("contexts.default.environment", 123) // Set an int instead of map

		// When retrieving the value using GetStringMap
		value := handler.GetStringMap("environment")

		// Then the returned map should be empty
		if len(value) != 0 {
			t.Errorf("Expected empty map due to type mismatch, got %v", value)
		}
	})
}

func TestYamlConfigHandler_GetConfig(t *testing.T) {
	setup := func(t *testing.T) (*YamlConfigHandler, *Mocks) {
		mocks := setupMocks(t)
		handler := NewYamlConfigHandler(mocks.Injector)
		if err := handler.Initialize(); err != nil {
			t.Fatalf("Failed to initialize handler: %v", err)
		}
		handler.shims = mocks.Shims
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
		handler.context = "nonexistent"

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
		handler.context = "test"

		// And a context with environment variables
		handler.config.Contexts = map[string]*v1alpha1.Context{
			"test": {
				Environment: map[string]string{
					"TEST_VAR": "test_value",
				},
			},
		}

		// And default context with different environment variables
		handler.defaultContextConfig = v1alpha1.Context{
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
		handler.context = "test"

		// And a context with environment variables that override defaults
		handler.config.Contexts = map[string]*v1alpha1.Context{
			"test": {
				Environment: map[string]string{
					"SHARED_VAR": "context_value",
				},
			},
		}

		// And default context with the same environment variable
		handler.defaultContextConfig = v1alpha1.Context{
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

// TestGetValueByPath tests the getValueByPath function
func Test_getValueByPath(t *testing.T) {
	t.Run("EmptyPathKeys", func(t *testing.T) {
		// Given an empty pathKeys slice for value lookup
		var current any
		pathKeys := []string{}

		// When calling getValueByPath with empty pathKeys
		value := getValueByPath(current, pathKeys)

		// Then nil should be returned as the path is invalid
		if value != nil {
			t.Errorf("Expected value to be nil, got %v", value)
		}
	})

	t.Run("InvalidCurrentValue", func(t *testing.T) {
		// Given a nil current value and a valid path key
		var current any = nil
		pathKeys := []string{"key"}

		// When calling getValueByPath with nil current value
		value := getValueByPath(current, pathKeys)

		// Then nil should be returned as the current value is invalid
		if value != nil {
			t.Errorf("Expected value to be nil, got %v", value)
		}
	})

	t.Run("MapKeyTypeMismatch", func(t *testing.T) {
		// Given a map with int keys but attempting to access with a string key
		current := map[int]string{1: "one", 2: "two"}
		pathKeys := []string{"1"}

		// When calling getValueByPath with mismatched key type
		value := getValueByPath(current, pathKeys)

		// Then nil should be returned due to key type mismatch
		if value != nil {
			t.Errorf("Expected value to be nil, got %v", value)
		}
	})

	t.Run("MapSuccess", func(t *testing.T) {
		// Given a map with a string key and corresponding value
		current := map[string]string{"key": "testValue"}
		pathKeys := []string{"key"}

		// When calling getValueByPath with a valid key
		value := getValueByPath(current, pathKeys)

		// Then the corresponding value should be returned successfully
		if value == nil {
			t.Errorf("Expected value to be 'testValue', got nil")
		}
		expectedValue := "testValue"
		if value != expectedValue {
			t.Errorf("Expected value '%s', got '%v'", expectedValue, value)
		}
	})

	t.Run("CannotSetField", func(t *testing.T) {
		// Given a struct with an unexported field that cannot be set
		type TestStruct struct {
			unexportedField string `yaml:"unexportedfield"`
		}
		testStruct := &TestStruct{}
		currValue := reflect.ValueOf(testStruct)
		pathKeys := []string{"unexportedfield"}
		value := "testValue"
		fullPath := "unexportedfield"

		// When attempting to set a value on the unexported field
		err := setValueByPath(currValue, pathKeys, value, fullPath)

		// Then an error should be returned indicating the field cannot be set
		expectedErr := "cannot set field"
		if err == nil || err.Error() != expectedErr {
			t.Errorf("Expected error '%s', got '%v'", expectedErr, err)
		}
	})

	t.Run("RecursiveFailure", func(t *testing.T) {
		// Given a nested map structure without the target field
		level3Map := map[string]any{}
		level2Map := map[string]any{"level3": level3Map}
		level1Map := map[string]any{"level2": level2Map}
		testMap := map[string]any{"level1": level1Map}
		currValue := reflect.ValueOf(testMap)
		pathKeys := []string{"level1", "level2", "nonexistentfield"}
		value := "newValue"
		fullPath := "level1.level2.nonexistentfield"

		// When attempting to set a value at a non-existent nested path
		err := setValueByPath(currValue, pathKeys, value, fullPath)

		// Then an error should be returned indicating the invalid path
		expectedErr := "Invalid path: level1.level2.nonexistentfield"
		if err == nil || err.Error() != expectedErr {
			t.Errorf("Expected error '%s', got '%v'", expectedErr, err)
		}
	})

	t.Run("AssignValueTypeMismatch", func(t *testing.T) {
		// Given a struct with an int field that cannot accept a string slice
		type TestStruct struct {
			IntField int `yaml:"intfield"`
		}
		testStruct := &TestStruct{}
		currValue := reflect.ValueOf(testStruct)
		pathKeys := []string{"intfield"}
		value := []string{"incompatibleType"} // A slice, which is incompatible with int
		fullPath := "intfield"

		// When attempting to assign an incompatible value type
		err := setValueByPath(currValue, pathKeys, value, fullPath)

		// Then an error should be returned indicating the type mismatch
		expectedErr := "cannot assign value of type []string to field of type int"
		if err == nil || err.Error() != expectedErr {
			t.Errorf("Expected error '%s', got '%v'", expectedErr, err)
		}
	})

	t.Run("AssignPointerValueTypeMismatch", func(t *testing.T) {
		// Given a struct with a pointer field that cannot accept a string slice
		type TestStruct struct {
			IntPtrField *int `yaml:"intptrfield"`
		}
		testStruct := &TestStruct{}
		currValue := reflect.ValueOf(testStruct)
		pathKeys := []string{"intptrfield"}
		value := []string{"incompatibleType"} // A slice, which is incompatible with *int
		fullPath := "intptrfield"

		// When attempting to assign an incompatible value type to a pointer field
		err := setValueByPath(currValue, pathKeys, value, fullPath)

		// Then an error should be returned indicating the pointer type mismatch
		expectedErr := "cannot assign value of type []string to field of type *int"
		if err == nil || err.Error() != expectedErr {
			t.Errorf("Expected error '%s', got '%v'", expectedErr, err)
		}
	})

	t.Run("AssignNonPointerField", func(t *testing.T) {
		// Given a struct with a string field that can be directly assigned
		type TestStruct struct {
			StringField string `yaml:"stringfield"`
		}
		testStruct := &TestStruct{}
		currValue := reflect.ValueOf(testStruct)
		pathKeys := []string{"stringfield"}
		value := "testValue" // Directly assignable to string
		fullPath := "stringfield"

		// When assigning a compatible value to the field
		err := setValueByPath(currValue, pathKeys, value, fullPath)

		// Then the field should be set without error
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if testStruct.StringField != "testValue" {
			t.Errorf("Expected StringField to be 'testValue', got '%v'", testStruct.StringField)
		}
	})

	t.Run("AssignConvertibleType", func(t *testing.T) {
		// Given a struct with an int field that can accept a convertible float value
		type TestStruct struct {
			IntField int `yaml:"intfield"`
		}
		testStruct := &TestStruct{}
		currValue := reflect.ValueOf(testStruct)
		pathKeys := []string{"intfield"}
		value := 42.0 // A float64, which is convertible to int
		fullPath := "intfield"

		// When assigning a value that can be converted to the field's type
		err := setValueByPath(currValue, pathKeys, value, fullPath)

		// Then the field should be set without error
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if testStruct.IntField != 42 {
			t.Errorf("Expected IntField to be 42, got '%v'", testStruct.IntField)
		}
	})
}

func Test_parsePath(t *testing.T) {
	t.Run("EmptyPath", func(t *testing.T) {
		// Given an empty path string to parse
		path := ""

		// When calling parsePath with the empty string
		pathKeys := parsePath(path)

		// Then an empty slice should be returned
		if len(pathKeys) != 0 {
			t.Errorf("Expected pathKeys to be empty, got %v", pathKeys)
		}
	})

	t.Run("SingleKey", func(t *testing.T) {
		// Given a path with a single key
		path := "key"

		// When calling parsePath with a single key
		pathKeys := parsePath(path)

		// Then a slice with only that key should be returned
		expected := []string{"key"}
		if !reflect.DeepEqual(pathKeys, expected) {
			t.Errorf("Expected pathKeys to be %v, got %v", expected, pathKeys)
		}
	})

	t.Run("MultipleKeys", func(t *testing.T) {
		// Given a path with multiple keys separated by dots
		path := "key1.key2.key3"

		// When calling parsePath with dot notation
		pathKeys := parsePath(path)

		// Then a slice containing all the keys should be returned
		expected := []string{"key1", "key2", "key3"}
		if !reflect.DeepEqual(pathKeys, expected) {
			t.Errorf("Expected pathKeys to be %v, got %v", expected, pathKeys)
		}
	})

	t.Run("KeysWithBrackets", func(t *testing.T) {
		// Given a path with keys using bracket notation
		path := "key1[key2][key3]"

		// When calling parsePath with bracket notation
		pathKeys := parsePath(path)

		// Then a slice containing all the keys without brackets should be returned
		expected := []string{"key1", "key2", "key3"}
		if !reflect.DeepEqual(pathKeys, expected) {
			t.Errorf("Expected pathKeys to be %v, got %v", expected, pathKeys)
		}
	})

	t.Run("MixedDotAndBracketNotation", func(t *testing.T) {
		// Given a path with mixed dot and bracket notation
		path := "key1.key2[key3].key4[key5]"

		// When calling parsePath with mixed notation
		pathKeys := parsePath(path)

		// Then a slice with all keys regardless of notation should be returned
		expected := []string{"key1", "key2", "key3", "key4", "key5"}
		if !reflect.DeepEqual(pathKeys, expected) {
			t.Errorf("Expected pathKeys to be %v, got %v", expected, pathKeys)
		}
	})

	t.Run("DotInsideBrackets", func(t *testing.T) {
		// Given a path with a dot inside bracket notation
		path := "key1[key2.key3]"

		// When calling parsePath with a dot inside brackets
		pathKeys := parsePath(path)

		// Then the dot inside brackets should be treated as part of the key
		expected := []string{"key1", "key2.key3"}
		if !reflect.DeepEqual(pathKeys, expected) {
			t.Errorf("Expected pathKeys to be %v, got %v", expected, pathKeys)
		}
	})
}

func Test_assignValue(t *testing.T) {
	t.Run("CannotSetField", func(t *testing.T) {
		// Given an unexported field that cannot be set
		var unexportedField struct {
			unexported int
		}
		fieldValue := reflect.ValueOf(&unexportedField).Elem().Field(0)

		// When attempting to assign a value to it
		_, err := assignValue(fieldValue, 10)

		// Then an error should be returned
		if err == nil {
			t.Errorf("Expected an error for non-settable field, got nil")
		}
		expectedError := "cannot set field"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("PointerTypeMismatchNonConvertible", func(t *testing.T) {
		// Given a pointer field of type *int
		var field *int
		fieldValue := reflect.ValueOf(&field).Elem()

		// When attempting to assign a string value to it
		value := "not an int"
		_, err := assignValue(fieldValue, value)

		// Then an error should be returned indicating type mismatch
		if err == nil {
			t.Errorf("Expected an error for pointer type mismatch, got nil")
		}
		expectedError := "cannot assign value of type string to field of type *int"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ValueTypeMismatchNonConvertible", func(t *testing.T) {
		// Given a field of type int
		var field int
		fieldValue := reflect.ValueOf(&field).Elem()

		// When attempting to assign a non-convertible string value to it
		value := "not convertible to int"
		_, err := assignValue(fieldValue, value)

		// Then an error should be returned indicating type mismatch
		if err == nil {
			t.Errorf("Expected an error for non-convertible type mismatch, got nil")
		}
		expectedError := "cannot assign value of type string to field of type int"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})
}

func Test_convertValue(t *testing.T) {
	t.Run("ConvertStringToBool", func(t *testing.T) {
		// Given a string value that can be converted to bool
		value := "true"
		targetType := reflect.TypeOf(true)

		// When converting the value
		result, err := convertValue(value, targetType)

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		// And the result should be a bool
		if result != true {
			t.Errorf("Expected true, got %v", result)
		}
	})

	t.Run("ConvertStringToInt", func(t *testing.T) {
		// Given a string value that can be converted to int
		value := "42"
		targetType := reflect.TypeOf(int(0))

		// When converting the value
		result, err := convertValue(value, targetType)

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		// And the result should be an int
		if result != 42 {
			t.Errorf("Expected 42, got %v", result)
		}
	})

	t.Run("ConvertStringToFloat", func(t *testing.T) {
		// Given a string value that can be converted to float
		value := "3.14"
		targetType := reflect.TypeOf(float64(0))

		// When converting the value
		result, err := convertValue(value, targetType)

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		// And the result should be a float
		if result != 3.14 {
			t.Errorf("Expected 3.14, got %v", result)
		}
	})

	t.Run("ConvertStringToPointer", func(t *testing.T) {
		// Given a string value that can be converted to a pointer type
		value := "42"
		targetType := reflect.TypeOf((*int)(nil))

		// When converting the value
		result, err := convertValue(value, targetType)

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		// And the result should be a pointer to int
		if ptr, ok := result.(*int); !ok || *ptr != 42 {
			t.Errorf("Expected *int(42), got %v", result)
		}
	})

	t.Run("UnsupportedType", func(t *testing.T) {
		// Given a string value and an unsupported target type
		value := "test"
		targetType := reflect.TypeOf([]string{})

		// When converting the value
		_, err := convertValue(value, targetType)

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error for unsupported type")
		}

		// And the error message should indicate the unsupported type
		expectedErr := "unsupported type conversion from string to []string"
		if err.Error() != expectedErr {
			t.Errorf("Expected error '%s', got '%s'", expectedErr, err.Error())
		}
	})

	t.Run("InvalidNumericValue", func(t *testing.T) {
		// Given an invalid numeric string value
		value := "not a number"
		targetType := reflect.TypeOf(int(0))

		// When converting the value
		_, err := convertValue(value, targetType)

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error for invalid numeric value")
		}
	})

	t.Run("UintTypes", func(t *testing.T) {
		// Given a string value and uint target types
		value := "42"
		targetTypes := []reflect.Type{
			reflect.TypeOf(uint(0)),
			reflect.TypeOf(uint8(0)),
			reflect.TypeOf(uint16(0)),
			reflect.TypeOf(uint32(0)),
			reflect.TypeOf(uint64(0)),
		}

		// When converting the value to each type
		for _, targetType := range targetTypes {
			result, err := convertValue(value, targetType)

			// Then no error should be returned
			if err != nil {
				t.Fatalf("convertValue() unexpected error for %v: %v", targetType, err)
			}

			// And the value should be correctly converted
			switch targetType.Kind() {
			case reflect.Uint:
				if result != uint(42) {
					t.Errorf("convertValue() = %v, want %v for %v", result, uint(42), targetType)
				}
			case reflect.Uint8:
				if result != uint8(42) {
					t.Errorf("convertValue() = %v, want %v for %v", result, uint8(42), targetType)
				}
			case reflect.Uint16:
				if result != uint16(42) {
					t.Errorf("convertValue() = %v, want %v for %v", result, uint16(42), targetType)
				}
			case reflect.Uint32:
				if result != uint32(42) {
					t.Errorf("convertValue() = %v, want %v for %v", result, uint32(42), targetType)
				}
			case reflect.Uint64:
				if result != uint64(42) {
					t.Errorf("convertValue() = %v, want %v for %v", result, uint64(42), targetType)
				}
			}
		}
	})

	t.Run("IntTypes", func(t *testing.T) {
		// Given a string value and int target types
		value := "42"
		targetTypes := []reflect.Type{
			reflect.TypeOf(int8(0)),
			reflect.TypeOf(int16(0)),
			reflect.TypeOf(int32(0)),
			reflect.TypeOf(int64(0)),
		}

		// When converting the value to each type
		for _, targetType := range targetTypes {
			result, err := convertValue(value, targetType)

			// Then no error should be returned
			if err != nil {
				t.Fatalf("convertValue() unexpected error for %v: %v", targetType, err)
			}

			// And the value should be correctly converted
			switch targetType.Kind() {
			case reflect.Int8:
				if result != int8(42) {
					t.Errorf("convertValue() = %v, want %v for %v", result, int8(42), targetType)
				}
			case reflect.Int16:
				if result != int16(42) {
					t.Errorf("convertValue() = %v, want %v for %v", result, int16(42), targetType)
				}
			case reflect.Int32:
				if result != int32(42) {
					t.Errorf("convertValue() = %v, want %v for %v", result, int32(42), targetType)
				}
			case reflect.Int64:
				if result != int64(42) {
					t.Errorf("convertValue() = %v, want %v for %v", result, int64(42), targetType)
				}
			}
		}
	})

	t.Run("Float32", func(t *testing.T) {
		// Given a string value and float32 target type
		value := "3.14"
		targetType := reflect.TypeOf(float32(0))

		// When converting the value
		result, err := convertValue(value, targetType)

		// Then no error should be returned
		if err != nil {
			t.Fatalf("convertValue() unexpected error: %v", err)
		}

		// And the value should be correctly converted
		if result != float32(3.14) {
			t.Errorf("convertValue() = %v, want %v", result, float32(3.14))
		}
	})

	t.Run("StringToFloatOverflow", func(t *testing.T) {
		// Given a string value that would overflow float32
		value := "3.4028236e+38"
		targetType := reflect.TypeOf(float32(0))

		// When converting the value
		_, err := convertValue(value, targetType)

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error for float overflow")
		}

		// And the error message should indicate overflow
		if !strings.Contains(err.Error(), "float overflow") {
			t.Errorf("Expected error containing 'float overflow', got '%s'", err.Error())
		}
	})
}

func TestYamlConfigHandler_SetDefault(t *testing.T) {
	setup := func(t *testing.T) (*YamlConfigHandler, *Mocks) {
		mocks := setupMocks(t)
		handler := NewYamlConfigHandler(mocks.Injector)
		if err := handler.Initialize(); err != nil {
			t.Fatalf("Failed to initialize handler: %v", err)
		}
		handler.shims = mocks.Shims
		return handler, mocks
	}

	t.Run("SetDefaultWithExistingContext", func(t *testing.T) {
		// Given a handler with a context set
		handler, _ := setup(t)
		defaultContext := v1alpha1.Context{
			Environment: map[string]string{
				"ENV_VAR": "value",
			},
		}

		// And a context is set
		handler.Set("context", "local")

		// When setting the default context
		err := handler.SetDefault(defaultContext)

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		// And the default context should be set correctly
		if handler.defaultContextConfig.Environment["ENV_VAR"] != "value" {
			t.Errorf("SetDefault() = %v, expected %v", handler.defaultContextConfig.Environment["ENV_VAR"], "value")
		}
	})

	t.Run("SetDefaultWithNoContext", func(t *testing.T) {
		// Given a handler with no context set
		handler, _ := setup(t)
		handler.context = ""
		defaultContext := v1alpha1.Context{
			Environment: map[string]string{
				"ENV_VAR": "value",
			},
		}

		// When setting the default context
		err := handler.SetDefault(defaultContext)

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		// And the default context should be set correctly
		if handler.defaultContextConfig.Environment["ENV_VAR"] != "value" {
			t.Errorf("SetDefault() = %v, expected %v", handler.defaultContextConfig.Environment["ENV_VAR"], "value")
		}
	})

	t.Run("SetDefaultUsedInSubsequentOperations", func(t *testing.T) {
		// Given a handler with an existing context
		handler, _ := setup(t)
		handler.context = "existing-context"
		handler.config.Contexts = map[string]*v1alpha1.Context{
			"existing-context": {},
		}

		// And a default context configuration
		defaultConf := v1alpha1.Context{
			Environment: map[string]string{"DEFAULT_VAR": "default_val"},
		}

		// When setting the default context
		err := handler.SetDefault(defaultConf)

		// Then no error should be returned
		if err != nil {
			t.Fatalf("SetDefault() unexpected error: %v", err)
		}

		// And the default context should be set correctly
		if handler.defaultContextConfig.Environment == nil || handler.defaultContextConfig.Environment["DEFAULT_VAR"] != "default_val" {
			t.Errorf("Expected defaultContextConfig environment to be %v, got %v", defaultConf.Environment, handler.defaultContextConfig.Environment)
		}

		// And the existing context should not be modified
		if handler.config.Contexts["existing-context"] == nil {
			t.Errorf("SetDefault incorrectly overwrote existing context config")
		}
	})

	t.Run("SetDefaultMergesWithExistingContext", func(t *testing.T) {
		// Given a handler with an existing context containing some values
		handler, _ := setup(t)
		handler.context = "test"
		handler.config.Contexts = map[string]*v1alpha1.Context{
			"test": {
				ID: ptrString("existing-id"),
				VM: &vm.VMConfig{
					Driver: ptrString("docker-desktop"),
				},
				Environment: map[string]string{
					"EXISTING_VAR": "existing_value",
					"OVERRIDE_VAR": "context_value",
				},
			},
		}

		// And a default context with overlapping and additional values
		defaultContext := v1alpha1.Context{
			VM: &vm.VMConfig{
				CPU: ptrInt(4),
			},
			Environment: map[string]string{
				"DEFAULT_VAR":  "default_value",
				"OVERRIDE_VAR": "default_value",
			},
		}

		// When setting the default context
		err := handler.SetDefault(defaultContext)

		// Then no error should be returned
		if err != nil {
			t.Fatalf("SetDefault() unexpected error: %v", err)
		}

		// And the context should merge defaults with existing values
		ctx := handler.config.Contexts["test"]
		if ctx == nil {
			t.Fatal("Context was removed during SetDefault")
		}

		// Existing values should be preserved
		if ctx.ID == nil || *ctx.ID != "existing-id" {
			t.Errorf("Expected ID to be preserved as 'existing-id', got %v", ctx.ID)
		}
		if ctx.VM.Driver == nil || *ctx.VM.Driver != "docker-desktop" {
			t.Errorf("Expected VM driver to be preserved as 'docker-desktop', got %v", ctx.VM.Driver)
		}
		if ctx.Environment["EXISTING_VAR"] != "existing_value" {
			t.Errorf("Expected EXISTING_VAR to be preserved as 'existing_value', got '%s'", ctx.Environment["EXISTING_VAR"])
		}
		if ctx.Environment["OVERRIDE_VAR"] != "context_value" {
			t.Errorf("Expected OVERRIDE_VAR to keep context value 'context_value', got '%s'", ctx.Environment["OVERRIDE_VAR"])
		}

		// Default values should be added where not present
		if ctx.VM.CPU == nil || *ctx.VM.CPU != 4 {
			t.Errorf("Expected VM CPU to be added from default as 4, got %v", ctx.VM.CPU)
		}
		if ctx.Environment["DEFAULT_VAR"] != "default_value" {
			t.Errorf("Expected DEFAULT_VAR to be added from default as 'default_value', got '%s'", ctx.Environment["DEFAULT_VAR"])
		}
	})

	t.Run("SetDefaultMergesComplexNestedStructures", func(t *testing.T) {
		// Given a handler with an existing context containing some values
		handler, _ := setup(t)
		handler.context = "test"
		handler.config.Contexts = map[string]*v1alpha1.Context{
			"test": {
				ID: ptrString("existing-id"),
				Environment: map[string]string{
					"EXISTING_VAR": "existing_value",
				},
			},
		}

		// And a default context with additional values
		defaultContext := v1alpha1.Context{
			Environment: map[string]string{
				"DEFAULT_VAR": "default_value",
			},
		}

		// When setting the default context
		err := handler.SetDefault(defaultContext)

		// Then no error should be returned
		if err != nil {
			t.Fatalf("SetDefault() unexpected error: %v", err)
		}

		// And the context should have both existing and default values
		ctx := handler.config.Contexts["test"]
		if ctx == nil {
			t.Fatal("Context was removed during SetDefault")
		}

		// Existing values should be preserved
		if ctx.ID == nil || *ctx.ID != "existing-id" {
			t.Errorf("Expected ID to be preserved as 'existing-id', got %v", ctx.ID)
		}
		if ctx.Environment["EXISTING_VAR"] != "existing_value" {
			t.Errorf("Expected EXISTING_VAR to be preserved as 'existing_value', got '%s'", ctx.Environment["EXISTING_VAR"])
		}

		// Default values should be added where not present
		if ctx.Environment["DEFAULT_VAR"] != "default_value" {
			t.Errorf("Expected DEFAULT_VAR to be added from default as 'default_value', got '%s'", ctx.Environment["DEFAULT_VAR"])
		}
	})

	t.Run("SetDefaultWithNilContextsMap", func(t *testing.T) {
		// Given a handler with a nil contexts map
		handler, _ := setup(t)
		handler.context = "test"
		handler.config.Contexts = nil

		// And a default context
		defaultContext := v1alpha1.Context{
			Environment: map[string]string{
				"DEFAULT_VAR": "default_value",
			},
		}

		// When setting the default context
		err := handler.SetDefault(defaultContext)

		// Then no error should be returned
		if err != nil {
			t.Fatalf("SetDefault() unexpected error: %v", err)
		}

		// And the contexts map should be created with the default
		if handler.config.Contexts == nil {
			t.Fatal("Expected contexts map to be created")
		}

		ctx := handler.config.Contexts["test"]
		if ctx == nil {
			t.Fatal("Expected test context to be created")
		}
		if ctx.Environment["DEFAULT_VAR"] != "default_value" {
			t.Errorf("Expected DEFAULT_VAR to be 'default_value', got '%s'", ctx.Environment["DEFAULT_VAR"])
		}
	})
}

func TestYamlConfigHandler_SetContextValue(t *testing.T) {
	setup := func(t *testing.T) (*YamlConfigHandler, *Mocks) {
		mocks := setupMocks(t)
		handler := NewYamlConfigHandler(mocks.Injector)
		if err := handler.Initialize(); err != nil {
			t.Fatalf("Failed to initialize handler: %v", err)
		}
		handler.shims = mocks.Shims
		handler.path = filepath.Join(t.TempDir(), "config.yaml")
		return handler, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a handler with a context set
		handler, _ := setup(t)
		handler.context = "test"

		// And a context with an empty environment map
		actualContext := handler.GetContext()
		handler.config.Contexts = map[string]*v1alpha1.Context{
			actualContext: {},
		}

		// When setting a value in the context environment
		err := handler.SetContextValue("environment.TEST_VAR", "test_value")

		// Then no error should be returned
		if err != nil {
			t.Fatalf("SetContextValue() unexpected error: %v", err)
		}

		// And the value should be correctly set in the context
		expected := "test_value"
		if val := handler.config.Contexts[actualContext].Environment["TEST_VAR"]; val != expected {
			t.Errorf("SetContextValue() did not correctly set value, expected %s, got %s", expected, val)
		}
	})

	t.Run("EmptyPath", func(t *testing.T) {
		// Given a handler with a context set
		handler, _ := setup(t)

		// When attempting to set a value with an empty path
		err := handler.SetContextValue("", "test_value")

		// Then an error should be returned
		if err == nil {
			t.Errorf("SetContextValue() with empty path did not return an error")
		}

		// And the error message should be as expected
		expectedErr := "path cannot be empty"
		if err.Error() != expectedErr {
			t.Errorf("Expected error message '%s', got '%s'", expectedErr, err.Error())
		}
	})

	t.Run("SetFails", func(t *testing.T) {
		// Given a handler with a context set
		handler, _ := setup(t)
		handler.context = "test"

		// When attempting to set a value with an invalid path
		err := handler.SetContextValue("invalid..path", "test_value")

		// Then an error should be returned
		if err == nil {
			t.Errorf("SetContextValue() with invalid path did not return an error")
		}
	})

	t.Run("ConvertStringToBool", func(t *testing.T) {
		handler, _ := setup(t)
		handler.context = "default"
		handler.config.Contexts = map[string]*v1alpha1.Context{
			"default": {},
		}

		// Set initial bool value
		if err := handler.SetContextValue("environment.BOOL_VAR", "true"); err != nil {
			t.Fatalf("Failed to set initial bool value: %v", err)
		}

		// Override with string "false"
		if err := handler.SetContextValue("environment.BOOL_VAR", "false"); err != nil {
			t.Fatalf("Failed to set string bool value: %v", err)
		}

		val := handler.GetString("environment.BOOL_VAR")
		if val != "false" {
			t.Errorf("Expected false, got %v", val)
		}
	})

	t.Run("ConvertStringToInt", func(t *testing.T) {
		handler, _ := setup(t)
		handler.context = "default"
		handler.config.Contexts = map[string]*v1alpha1.Context{
			"default": {},
		}

		// Set initial int value
		if err := handler.SetContextValue("environment.INT_VAR", "42"); err != nil {
			t.Fatalf("Failed to set initial int value: %v", err)
		}

		// Override with string "100"
		if err := handler.SetContextValue("environment.INT_VAR", "100"); err != nil {
			t.Fatalf("Failed to set string int value: %v", err)
		}

		val := handler.GetString("environment.INT_VAR")
		if val != "100" {
			t.Errorf("Expected 100, got %v", val)
		}
	})

	t.Run("ConvertStringToFloat", func(t *testing.T) {
		handler, _ := setup(t)
		handler.context = "default"
		handler.config.Contexts = map[string]*v1alpha1.Context{
			"default": {},
		}

		// Set initial float value
		if err := handler.SetContextValue("environment.FLOAT_VAR", "3.14"); err != nil {
			t.Fatalf("Failed to set initial float value: %v", err)
		}

		// Override with string "6.28"
		if err := handler.SetContextValue("environment.FLOAT_VAR", "6.28"); err != nil {
			t.Fatalf("Failed to set string float value: %v", err)
		}

		val := handler.GetString("environment.FLOAT_VAR")
		if val != "6.28" {
			t.Errorf("Expected 6.28, got %v", val)
		}
	})

	t.Run("ConvertStringToBoolPointer", func(t *testing.T) {
		// Given a handler with a default context
		handler, _ := setup(t)
		handler.context = "default"
		handler.config.Contexts = map[string]*v1alpha1.Context{
			"default": {},
		}

		// When setting a string "false" to a bool pointer field (dns.enabled)
		if err := handler.SetContextValue("dns.enabled", "false"); err != nil {
			t.Fatalf("Failed to set dns.enabled=false from string: %v", err)
		}

		// Then the value should be correctly set as a boolean
		config := handler.GetConfig()
		if config.DNS == nil || config.DNS.Enabled == nil || *config.DNS.Enabled != false {
			t.Errorf("Expected dns.enabled to be false, got %v", config.DNS.Enabled)
		}

		// And when setting "true" as well
		if err := handler.SetContextValue("dns.enabled", "true"); err != nil {
			t.Fatalf("Failed to set dns.enabled=true from string: %v", err)
		}

		config = handler.GetConfig()
		if config.DNS == nil || config.DNS.Enabled == nil || *config.DNS.Enabled != true {
			t.Errorf("Expected dns.enabled to be true, got %v", config.DNS.Enabled)
		}
	})
}

func TestYamlConfigHandler_LoadConfigString(t *testing.T) {
	setup := func(t *testing.T) (*YamlConfigHandler, *Mocks) {
		mocks := setupMocks(t)
		handler := NewYamlConfigHandler(mocks.Injector)
		handler.shims = mocks.Shims
		if err := handler.Initialize(); err != nil {
			t.Fatalf("Failed to initialize handler: %v", err)
		}
		return handler, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a handler with a context set
		handler, _ := setup(t)
		handler.SetContext("test")

		// And a valid YAML configuration string
		yamlContent := `
version: v1alpha1
contexts:
  test:
    environment:
      TEST_VAR: test_value`

		// When loading the configuration string
		err := handler.LoadConfigString(yamlContent)

		// Then no error should be returned
		if err != nil {
			t.Fatalf("LoadConfigString() unexpected error: %v", err)
		}

		// And the value should be correctly loaded
		value := handler.GetString("environment.TEST_VAR")
		if value != "test_value" {
			t.Errorf("Expected TEST_VAR = 'test_value', got '%s'", value)
		}
	})

	t.Run("EmptyContent", func(t *testing.T) {
		// Given a handler with a context set
		handler, _ := setup(t)

		// When loading an empty configuration string
		err := handler.LoadConfigString("")

		// Then no error should be returned
		if err != nil {
			t.Fatalf("LoadConfigString() unexpected error: %v", err)
		}
	})

	t.Run("InvalidYAML", func(t *testing.T) {
		// Given a handler with a context set
		handler, _ := setup(t)

		// And an invalid YAML string
		yamlContent := `invalid: yaml: content: [}`

		// When loading the invalid YAML
		err := handler.LoadConfigString(yamlContent)

		// Then an error should be returned
		if err == nil {
			t.Fatal("LoadConfigString() expected error for invalid YAML")
		}

		// And the error message should indicate YAML unmarshalling failure
		if !strings.Contains(err.Error(), "error unmarshalling yaml") {
			t.Errorf("Expected error about invalid YAML, got: %v", err)
		}
	})

	t.Run("UnsupportedVersion", func(t *testing.T) {
		// Given a handler with a context set
		handler, _ := setup(t)

		// And a YAML string with an unsupported version
		yamlContent := `
version: v2alpha1
contexts:
  test: {}`

		// When loading the YAML with unsupported version
		err := handler.LoadConfigString(yamlContent)

		// Then an error should be returned
		if err == nil {
			t.Fatal("LoadConfigString() expected error for unsupported version")
		}

		// And the error message should indicate unsupported version
		if !strings.Contains(err.Error(), "unsupported config version") {
			t.Errorf("Expected error about unsupported version, got: %v", err)
		}
	})
}

func Test_makeAddressable(t *testing.T) {
	t.Run("AlreadyAddressable", func(t *testing.T) {
		// Given an addressable value
		var x int = 42
		v := reflect.ValueOf(&x).Elem()

		// When making it addressable
		result := makeAddressable(v)

		// Then the same value should be returned
		if result.Interface() != v.Interface() {
			t.Errorf("makeAddressable() = %v, want %v", result.Interface(), v.Interface())
		}
	})

	t.Run("NonAddressable", func(t *testing.T) {
		// Given a non-addressable value
		v := reflect.ValueOf(42)

		// When making it addressable
		result := makeAddressable(v)

		// Then a new addressable value should be returned
		if !result.CanAddr() {
			t.Error("makeAddressable() returned non-addressable value")
		}
		if result.Interface() != v.Interface() {
			t.Errorf("makeAddressable() = %v, want %v", result.Interface(), v.Interface())
		}
	})

	t.Run("NilValue", func(t *testing.T) {
		// Given a nil value
		var v reflect.Value

		// When making it addressable
		result := makeAddressable(v)

		// Then a zero value should be returned
		if result.IsValid() {
			t.Error("makeAddressable() returned valid value for nil input")
		}
	})
}

func TestYamlConfigHandler_ConvertValue(t *testing.T) {
	t.Run("StringToString", func(t *testing.T) {
		// Given a string value and target type
		value := "test"
		targetType := reflect.TypeOf("")

		// When converting the value
		result, err := convertValue(value, targetType)

		// Then no error should be returned
		if err != nil {
			t.Fatalf("convertValue() unexpected error: %v", err)
		}

		// And the value should be correctly converted
		if result != "test" {
			t.Errorf("convertValue() = %v, want %v", result, "test")
		}
	})

	t.Run("StringToInt", func(t *testing.T) {
		// Given a string value and target type
		value := "42"
		targetType := reflect.TypeOf(0)

		// When converting the value
		result, err := convertValue(value, targetType)

		// Then no error should be returned
		if err != nil {
			t.Fatalf("convertValue() unexpected error: %v", err)
		}

		// And the value should be correctly converted
		if result != 42 {
			t.Errorf("convertValue() = %v, want %v", result, 42)
		}
	})

	t.Run("StringToIntOverflow", func(t *testing.T) {
		// Given a string value that would overflow int8
		value := "128"
		targetType := reflect.TypeOf(int8(0))

		// When converting the value
		_, err := convertValue(value, targetType)

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error for integer overflow")
		}

		// And the error message should indicate overflow
		expectedErr := "integer overflow: 128 is outside the range of int8"
		if err.Error() != expectedErr {
			t.Errorf("Expected error '%s', got '%s'", expectedErr, err.Error())
		}
	})

	t.Run("StringToUintOverflow", func(t *testing.T) {
		// Given a string value that would overflow uint8
		value := "256"
		targetType := reflect.TypeOf(uint8(0))

		// When converting the value
		_, err := convertValue(value, targetType)
		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error for integer overflow")
		}

		// And the error message should indicate overflow
		expectedErr := "integer overflow: 256 is outside the range of uint8"
		if err.Error() != expectedErr {
			t.Errorf("Expected error '%s', got '%s'", expectedErr, err.Error())
		}
	})

	t.Run("StringToFloatOverflow", func(t *testing.T) {
		// Given a string value that would overflow float32
		value := "3.4028236e+38"
		targetType := reflect.TypeOf(float32(0))

		// When converting the value
		_, err := convertValue(value, targetType)

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error for float overflow")
		}

		// And the error message should indicate overflow
		if !strings.Contains(err.Error(), "float overflow") {
			t.Errorf("Expected error containing 'float overflow', got '%s'", err.Error())
		}
	})

	t.Run("StringToFloat", func(t *testing.T) {
		// Given a string value and target type
		value := "3.14"
		targetType := reflect.TypeOf(float64(0))

		// When converting the value
		result, err := convertValue(value, targetType)

		// Then no error should be returned
		if err != nil {
			t.Fatalf("convertValue() unexpected error: %v", err)
		}

		// And the value should be correctly converted
		if result != 3.14 {
			t.Errorf("convertValue() = %v, want %v", result, 3.14)
		}
	})

	t.Run("StringToBool", func(t *testing.T) {
		// Given a string value and target type
		value := "true"
		targetType := reflect.TypeOf(true)

		// When converting the value
		result, err := convertValue(value, targetType)

		// Then no error should be returned
		if err != nil {
			t.Fatalf("convertValue() unexpected error: %v", err)
		}

		// And the value should be correctly converted
		if result != true {
			t.Errorf("convertValue() = %v, want %v", result, true)
		}
	})
}

func TestYamlConfigHandler_Set(t *testing.T) {
	setup := func(t *testing.T) (*YamlConfigHandler, *Mocks) {
		mocks := setupMocks(t)
		handler := NewYamlConfigHandler(mocks.Injector)
		handler.shims = mocks.Shims
		if err := handler.Initialize(); err != nil {
			t.Fatalf("Failed to initialize handler: %v", err)
		}
		return handler, mocks
	}

	t.Run("InvalidPath", func(t *testing.T) {
		// Given a handler with a context set
		handler, _ := setup(t)

		// When setting a value with an invalid path
		err := handler.Set("", "value")

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Set() unexpected error: %v", err)
		}
	})

	t.Run("SetValueByPathError", func(t *testing.T) {
		// Given a handler with a context set
		handler, _ := setup(t)

		// And a mocked setValueByPath that returns an error
		handler.shims.YamlMarshal = func(v any) ([]byte, error) {
			return nil, fmt.Errorf("mocked error")
		}

		// When setting a value
		err := handler.Set("test.path", "value")

		// Then an error should be returned
		if err == nil {
			t.Fatal("Set() expected error, got nil")
		}
	})
}

func Test_setValueByPath(t *testing.T) {
	t.Run("EmptyPathKeys", func(t *testing.T) {
		// Given empty pathKeys
		currValue := reflect.ValueOf(struct{}{})
		pathKeys := []string{}
		value := "test"
		fullPath := "test.path"

		// When calling setValueByPath with empty pathKeys
		err := setValueByPath(currValue, pathKeys, value, fullPath)

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error for empty pathKeys")
		}
		expectedErr := "pathKeys cannot be empty"
		if err.Error() != expectedErr {
			t.Errorf("Expected error '%s', got '%s'", expectedErr, err.Error())
		}
	})

	t.Run("StructFieldNotFound", func(t *testing.T) {
		// Given a struct and a non-existent field
		type TestStruct struct {
			Field string `yaml:"field"`
		}
		currValue := reflect.ValueOf(&TestStruct{}).Elem()
		pathKeys := []string{"nonexistent"}
		value := "test"
		fullPath := "nonexistent"

		// When calling setValueByPath with non-existent field
		err := setValueByPath(currValue, pathKeys, value, fullPath)

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error for non-existent field")
		}
		expectedErr := "field not found: nonexistent"
		if err.Error() != expectedErr {
			t.Errorf("Expected error '%s', got '%s'", expectedErr, err.Error())
		}
	})

	t.Run("StructFieldSuccess", func(t *testing.T) {
		// Given a struct with a field
		type TestStruct struct {
			Field string `yaml:"field"`
		}
		currValue := reflect.ValueOf(&TestStruct{}).Elem()
		pathKeys := []string{"field"}
		value := "test"
		fullPath := "field"

		// When calling setValueByPath with valid field
		err := setValueByPath(currValue, pathKeys, value, fullPath)

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		// And the field should be set correctly
		if currValue.Field(0).String() != "test" {
			t.Errorf("Expected field value 'test', got '%s'", currValue.Field(0).String())
		}
	})

	t.Run("MapKeyTypeMismatch", func(t *testing.T) {
		// Given a map with int keys but trying to set with string key
		currValue := reflect.ValueOf(&map[int]string{}).Elem()
		pathKeys := []string{"key"}
		value := "test"
		fullPath := "key"

		// When calling setValueByPath with mismatched key type
		err := setValueByPath(currValue, pathKeys, value, fullPath)

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error for key type mismatch")
		}
		expectedErr := "key type mismatch: expected int, got string"
		if err.Error() != expectedErr {
			t.Errorf("Expected error '%s', got '%s'", expectedErr, err.Error())
		}
	})

	t.Run("MapValueTypeMismatch", func(t *testing.T) {
		// Given a map with string values but trying to set with a non-convertible type
		currValue := reflect.ValueOf(&map[string]string{}).Elem()
		pathKeys := []string{"key"}
		value := struct{}{} // Use a struct{} which cannot be converted to string
		fullPath := "key"

		// When calling setValueByPath with mismatched value type
		err := setValueByPath(currValue, pathKeys, value, fullPath)

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error for value type mismatch")
		}
		expectedErr := "value type mismatch for key key: expected string, got struct {}"
		if err.Error() != expectedErr {
			t.Errorf("Expected error '%s', got '%s'", expectedErr, err.Error())
		}
	})

	t.Run("MapSuccess", func(t *testing.T) {
		// Given a map with string keys and values
		currValue := reflect.ValueOf(&map[string]string{}).Elem()
		pathKeys := []string{"key"}
		value := "test"
		fullPath := "key"

		// When calling setValueByPath with valid key and value
		err := setValueByPath(currValue, pathKeys, value, fullPath)

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		// And the value should be set correctly
		if currValue.MapIndex(reflect.ValueOf("key")).String() != "test" {
			t.Errorf("Expected map value 'test', got '%s'", currValue.MapIndex(reflect.ValueOf("key")).String())
		}
	})

	t.Run("InvalidPath", func(t *testing.T) {
		// Given an invalid path type
		currValue := reflect.ValueOf(42)
		pathKeys := []string{"key"}
		value := "test"
		fullPath := "key"

		// When calling setValueByPath with invalid path type
		err := setValueByPath(currValue, pathKeys, value, fullPath)

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error for invalid path")
		}
		expectedErr := "Invalid path: key"
		if err.Error() != expectedErr {
			t.Errorf("Expected error '%s', got '%s'", expectedErr, err.Error())
		}
	})

	t.Run("NestedStruct", func(t *testing.T) {
		// Given a nested struct
		type InnerStruct struct {
			Field string `yaml:"field"`
		}
		type OuterStruct struct {
			Inner InnerStruct `yaml:"inner"`
		}
		currValue := reflect.ValueOf(&OuterStruct{}).Elem()
		pathKeys := []string{"inner", "field"}
		value := "test"
		fullPath := "inner.field"

		// When calling setValueByPath with nested path
		err := setValueByPath(currValue, pathKeys, value, fullPath)

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		// And the nested field should be set correctly
		inner := currValue.Field(0)
		if inner.Field(0).String() != "test" {
			t.Errorf("Expected nested field value 'test', got '%s'", inner.Field(0).String())
		}
	})

	t.Run("NestedMap", func(t *testing.T) {
		// Given a nested map
		currValue := reflect.ValueOf(&map[string]map[string]string{}).Elem()
		pathKeys := []string{"outer", "inner"}
		value := "test"
		fullPath := "outer.inner"

		// When calling setValueByPath with nested path
		err := setValueByPath(currValue, pathKeys, value, fullPath)

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		// And the nested value should be set correctly
		outer := currValue.MapIndex(reflect.ValueOf("outer"))
		if !outer.IsValid() {
			t.Fatal("Expected outer map to exist")
		}
		inner := outer.MapIndex(reflect.ValueOf("inner"))
		if !inner.IsValid() {
			t.Fatal("Expected inner map to exist")
		}
		if inner.String() != "test" {
			t.Errorf("Expected nested value 'test', got '%s'", inner.String())
		}
	})

	t.Run("PointerField", func(t *testing.T) {
		// Given a struct with a pointer field
		type TestStruct struct {
			Field *string `yaml:"field"`
		}
		currValue := reflect.ValueOf(&TestStruct{}).Elem()
		pathKeys := []string{"field"}
		value := "test"
		fullPath := "field"

		// When calling setValueByPath with pointer field
		err := setValueByPath(currValue, pathKeys, value, fullPath)

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		// And the pointer field should be set correctly
		field := currValue.Field(0)
		if field.IsNil() {
			t.Fatal("Expected pointer field to be non-nil")
		}
		if field.Elem().String() != "test" {
			t.Errorf("Expected pointer field value 'test', got '%s'", field.Elem().String())
		}
	})

	t.Run("PointerMap", func(t *testing.T) {
		// Given a map with pointer values
		currValue := reflect.ValueOf(&map[string]*string{}).Elem()
		pathKeys := []string{"key"}
		str := "test"
		value := &str // Use a pointer to string
		fullPath := "key"

		// When calling setValueByPath with pointer map
		err := setValueByPath(currValue, pathKeys, value, fullPath)

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		// And the pointer value should be set correctly
		val := currValue.MapIndex(reflect.ValueOf("key"))
		if !val.IsValid() || val.IsNil() {
			t.Fatal("Expected map value to be non-nil")
		}
		if val.Elem().String() != "test" {
			t.Errorf("Expected pointer value 'test', got '%s'", val.Elem().String())
		}
	})

	t.Run("NestedMapWithNilValue", func(t *testing.T) {
		// Given a nested map with a nil value
		m := map[string]map[string]string{
			"outer": nil,
		}
		currValue := reflect.ValueOf(&m).Elem()
		pathKeys := []string{"outer", "inner"}
		value := "test"
		fullPath := "outer.inner"

		// When calling setValueByPath with nested path
		err := setValueByPath(currValue, pathKeys, value, fullPath)

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		// And the nested value should be set correctly
		outer := currValue.MapIndex(reflect.ValueOf("outer"))
		if !outer.IsValid() {
			t.Fatal("Expected outer map to exist")
		}
		inner := outer.MapIndex(reflect.ValueOf("inner"))
		if !inner.IsValid() {
			t.Fatal("Expected inner map to exist")
		}
		if inner.String() != "test" {
			t.Errorf("Expected nested value 'test', got '%s'", inner.String())
		}
	})
}

func TestYamlConfigHandler_GenerateContextID(t *testing.T) {
	setup := func(t *testing.T) (*YamlConfigHandler, *Mocks) {
		mocks := setupMocks(t)
		handler := NewYamlConfigHandler(mocks.Injector)
		handler.shims = mocks.Shims
		if err := handler.Initialize(); err != nil {
			t.Fatalf("Failed to initialize handler: %v", err)
		}
		return handler, mocks
	}

	t.Run("WhenContextIDExists", func(t *testing.T) {
		// Given a set of safe mocks and a YamlConfigHandler
		handler, _ := setup(t)

		// And an existing context ID
		existingID := "w1234567"
		handler.SetContextValue("id", existingID)

		// When GenerateContextID is called
		err := handler.GenerateContextID()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("GenerateContextID() unexpected error: %v", err)
		}

		// And the existing ID should remain unchanged
		if got := handler.GetString("id"); got != existingID {
			t.Errorf("Expected ID = %v, got = %v", existingID, got)
		}
	})

	t.Run("WhenContextIDDoesNotExist", func(t *testing.T) {
		// Given a set of safe mocks and a YamlConfigHandler
		handler, _ := setup(t)

		// When GenerateContextID is called
		err := handler.GenerateContextID()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("GenerateContextID() unexpected error: %v", err)
		}

		// And a new ID should be generated
		id := handler.GetString("id")
		if id == "" {
			t.Fatal("Expected non-empty ID")
		}

		// And the ID should start with 'w' and be 8 characters long
		if len(id) != 8 || !strings.HasPrefix(id, "w") {
			t.Errorf("Expected ID to start with 'w' and be 8 characters long, got: %s", id)
		}
	})

	t.Run("WhenRandomGenerationFails", func(t *testing.T) {
		// Given a set of safe mocks and a YamlConfigHandler
		handler, _ := setup(t)

		// And a mocked crypto/rand that fails
		handler.shims.CryptoRandRead = func([]byte) (int, error) {
			return 0, fmt.Errorf("mocked crypto/rand error")
		}

		// When GenerateContextID is called
		err := handler.GenerateContextID()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}

		// And the error message should be as expected
		expectedError := "failed to generate random context ID: mocked crypto/rand error"
		if err.Error() != expectedError {
			t.Errorf("Expected error = %v, got = %v", expectedError, err)
		}
	})
}

func TestYamlMarshalWithDefinedPaths(t *testing.T) {
	setup := func(t *testing.T) *YamlConfigHandler {
		t.Helper()
		mocks := setupMocks(t)
		handler := NewYamlConfigHandler(mocks.Injector)
		handler.shims = mocks.Shims
		err := handler.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize YamlConfigHandler: %v", err)
		}
		return handler
	}

	t.Run("IgnoreYamlMinusTag", func(t *testing.T) {
		// Given a struct with a YAML minus tag
		type testStruct struct {
			Public  string `yaml:"public"`
			private string `yaml:"-"`
		}
		input := testStruct{Public: "value", private: "ignored"}

		// When marshalling the struct
		handler := setup(t)
		result, err := handler.YamlMarshalWithDefinedPaths(input)

		// Then no error should be returned
		if err != nil {
			t.Errorf("YamlMarshalWithDefinedPaths failed: %v", err)
		}

		// And the public field should be included
		if !strings.Contains(string(result), "public: value") {
			t.Errorf("Expected 'public: value' in result, got: %s", string(result))
		}

		// And the ignored field should be excluded
		if strings.Contains(string(result), "ignored") {
			t.Errorf("Expected 'ignored' not to be in result, got: %s", string(result))
		}
	})

	t.Run("NilInput", func(t *testing.T) {
		// When marshalling nil input
		handler := setup(t)
		_, err := handler.YamlMarshalWithDefinedPaths(nil)

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error for nil input, got nil")
		}

		// And the error message should be appropriate
		if !strings.Contains(err.Error(), "invalid input: nil value") {
			t.Errorf("Expected error about nil input, got: %v", err)
		}
	})

	t.Run("EmptySlice", func(t *testing.T) {
		// Given an empty slice
		input := []string{}

		// When marshalling the slice
		handler := setup(t)
		result, err := handler.YamlMarshalWithDefinedPaths(input)

		// Then no error should be returned
		if err != nil {
			t.Errorf("YamlMarshalWithDefinedPaths failed: %v", err)
		}

		// And the result should be an empty array
		if string(result) != "[]\n" {
			t.Errorf("Expected '[]\n', got: %s", string(result))
		}
	})

	t.Run("NoYamlTag", func(t *testing.T) {
		// Given a struct with no YAML tags
		type testStruct struct {
			Field string
		}
		input := testStruct{Field: "value"}

		// When marshalling the struct
		handler := setup(t)
		result, err := handler.YamlMarshalWithDefinedPaths(input)

		// Then no error should be returned
		if err != nil {
			t.Errorf("YamlMarshalWithDefinedPaths failed: %v", err)
		}

		// And the field name should be used as is
		if !strings.Contains(string(result), "Field: value") {
			t.Errorf("Expected 'Field: value' in result, got: %s", string(result))
		}
	})

	t.Run("CustomYamlTag", func(t *testing.T) {
		// Given a struct with a custom YAML tag
		type testStruct struct {
			Field string `yaml:"custom_field"`
		}
		input := testStruct{Field: "value"}

		// When marshalling the struct
		handler := setup(t)
		result, err := handler.YamlMarshalWithDefinedPaths(input)

		// Then no error should be returned
		if err != nil {
			t.Errorf("YamlMarshalWithDefinedPaths failed: %v", err)
		}

		// And the custom field name should be used
		if !strings.Contains(string(result), "custom_field: value") {
			t.Errorf("Expected 'custom_field: value' in result, got: %s", string(result))
		}
	})

	t.Run("MapWithCustomTags", func(t *testing.T) {
		// Given a map with nested structs using custom YAML tags
		type nestedStruct struct {
			Value string `yaml:"custom_value"`
		}
		input := map[string]nestedStruct{
			"key": {Value: "test"},
		}

		// When marshalling the map
		handler := setup(t)
		result, err := handler.YamlMarshalWithDefinedPaths(input)

		// Then no error should be returned
		if err != nil {
			t.Errorf("YamlMarshalWithDefinedPaths failed: %v", err)
		}

		// And the map key should be preserved
		if !strings.Contains(string(result), "key:") {
			t.Errorf("Expected 'key:' in result, got: %s", string(result))
		}

		// And the nested custom field name should be used
		if !strings.Contains(string(result), "  custom_value: test") {
			t.Errorf("Expected '  custom_value: test' in result, got: %s", string(result))
		}
	})

	t.Run("DefaultFieldName", func(t *testing.T) {
		// Given a struct with default field names
		data := struct {
			Field string
		}{
			Field: "value",
		}

		// When marshalling the struct
		handler := setup(t)
		result, err := handler.YamlMarshalWithDefinedPaths(data)

		// Then no error should be returned
		if err != nil {
			t.Errorf("YamlMarshalWithDefinedPaths failed: %v", err)
		}

		// And the default field name should be used
		if !strings.Contains(string(result), "Field: value") {
			t.Errorf("Expected 'Field: value' in result, got: %s", string(result))
		}
	})

	t.Run("FuncType", func(t *testing.T) {
		// When marshalling a function type
		handler := setup(t)
		_, err := handler.YamlMarshalWithDefinedPaths(func() {})

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error for func type, got nil")
		}

		// And the error message should be appropriate
		if !strings.Contains(err.Error(), "unsupported value type func") {
			t.Errorf("Expected error about unsupported value type, got: %v", err)
		}
	})

	t.Run("UnsupportedType", func(t *testing.T) {
		// When marshalling an unsupported type
		handler := setup(t)
		_, err := handler.YamlMarshalWithDefinedPaths(make(chan int))

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error for unsupported type, got nil")
		}

		// And the error message should be appropriate
		if !strings.Contains(err.Error(), "unsupported value type") {
			t.Errorf("Expected error about unsupported value type, got: %v", err)
		}
	})

	t.Run("MapWithNilValues", func(t *testing.T) {
		// Given a map with nil values
		input := map[string]any{
			"key1": nil,
			"key2": "value2",
		}

		// When marshalling the map
		handler := setup(t)
		result, err := handler.YamlMarshalWithDefinedPaths(input)

		// Then no error should be returned
		if err != nil {
			t.Errorf("YamlMarshalWithDefinedPaths failed: %v", err)
		}

		// And nil values should be represented as null
		if !strings.Contains(string(result), "key1: null") {
			t.Errorf("Expected 'key1: null' in result, got: %s", string(result))
		}

		// And non-nil values should be preserved
		if !strings.Contains(string(result), "key2: value2") {
			t.Errorf("Expected 'key2: value2' in result, got: %s", string(result))
		}
	})

	t.Run("SliceWithNilValues", func(t *testing.T) {
		// Given a slice with nil values
		input := []any{nil, "value", nil}

		// When marshalling the slice
		handler := setup(t)
		result, err := handler.YamlMarshalWithDefinedPaths(input)

		// Then no error should be returned
		if err != nil {
			t.Errorf("YamlMarshalWithDefinedPaths failed: %v", err)
		}

		// And nil values should be represented as null
		if !strings.Contains(string(result), "- null") {
			t.Errorf("Expected '- null' in result, got: %s", string(result))
		}

		// And non-nil values should be preserved
		if !strings.Contains(string(result), "- value") {
			t.Errorf("Expected '- value' in result, got: %s", string(result))
		}
	})

	t.Run("StructWithPrivateFields", func(t *testing.T) {
		// Given a struct with both public and private fields
		type testStruct struct {
			Public  string
			private string
		}
		input := testStruct{Public: "value", private: "ignored"}

		// When marshalling the struct
		handler := setup(t)
		result, err := handler.YamlMarshalWithDefinedPaths(input)

		// Then no error should be returned
		if err != nil {
			t.Errorf("YamlMarshalWithDefinedPaths failed: %v", err)
		}

		// And public fields should be included
		if !strings.Contains(string(result), "Public: value") {
			t.Errorf("Expected 'Public: value' in result, got: %s", string(result))
		}

		// And private fields should be excluded
		if strings.Contains(string(result), "private") {
			t.Errorf("Expected 'private' not to be in result, got: %s", string(result))
		}
	})

	t.Run("StructWithYamlTag", func(t *testing.T) {
		// Given a struct with a YAML tag
		type testStruct struct {
			Field string `yaml:"custom_name"`
		}
		input := testStruct{Field: "value"}

		// When marshalling the struct
		handler := setup(t)
		result, err := handler.YamlMarshalWithDefinedPaths(input)

		// Then no error should be returned
		if err != nil {
			t.Errorf("YamlMarshalWithDefinedPaths failed: %v", err)
		}

		// And the custom field name should be used
		if !strings.Contains(string(result), "custom_name: value") {
			t.Errorf("Expected 'custom_name: value' in result, got: %s", string(result))
		}
	})

	t.Run("NestedStructs", func(t *testing.T) {
		// Given nested structs
		type nested struct {
			Value string
		}
		type parent struct {
			Nested nested
		}
		input := parent{Nested: nested{Value: "test"}}

		// When marshalling the nested structs
		handler := setup(t)
		result, err := handler.YamlMarshalWithDefinedPaths(input)

		// Then no error should be returned
		if err != nil {
			t.Errorf("YamlMarshalWithDefinedPaths failed: %v", err)
		}

		// And the parent field should be included
		if !strings.Contains(string(result), "Nested:") {
			t.Errorf("Expected 'Nested:' in result, got: %s", string(result))
		}

		// And the nested field should be properly indented
		if !strings.Contains(string(result), "  Value: test") {
			t.Errorf("Expected '  Value: test' in result, got: %s", string(result))
		}
	})

	t.Run("NumericTypes", func(t *testing.T) {
		// Given a struct with various numeric types
		type numbers struct {
			Int     int     `yaml:"int"`
			Int8    int8    `yaml:"int8"`
			Int16   int16   `yaml:"int16"`
			Int32   int32   `yaml:"int32"`
			Int64   int64   `yaml:"int64"`
			Uint    uint    `yaml:"uint"`
			Uint8   uint8   `yaml:"uint8"`
			Uint16  uint16  `yaml:"uint16"`
			Uint32  uint32  `yaml:"uint32"`
			Uint64  uint64  `yaml:"uint64"`
			Float32 float32 `yaml:"float32"`
			Float64 float64 `yaml:"float64"`
		}
		input := numbers{
			Int: 1, Int8: 2, Int16: 3, Int32: 4, Int64: 5,
			Uint: 6, Uint8: 7, Uint16: 8, Uint32: 9, Uint64: 10,
			Float32: 11.1, Float64: 12.2,
		}

		// When marshalling the struct
		handler := setup(t)
		result, err := handler.YamlMarshalWithDefinedPaths(input)

		// Then no error should be returned
		if err != nil {
			t.Errorf("YamlMarshalWithDefinedPaths failed: %v", err)
		}

		// And all numeric values should be correctly represented
		for _, expected := range []string{
			"int: 1", "int8: 2", "int16: 3", "int32: 4", "int64: 5",
			"uint: 6", "uint8: 7", "uint16: 8", "uint32: 9", "uint64: 10",
			"float32: 11.1", "float64: 12.2",
		} {
			if !strings.Contains(string(result), expected) {
				t.Errorf("Expected '%s' in result, got: %s", expected, string(result))
			}
		}
	})

	t.Run("BooleanType", func(t *testing.T) {
		// Given a struct with boolean fields
		type boolStruct struct {
			True  bool `yaml:"true"`
			False bool `yaml:"false"`
		}
		input := boolStruct{True: true, False: false}

		// When marshalling the struct
		handler := setup(t)
		result, err := handler.YamlMarshalWithDefinedPaths(input)

		// Then no error should be returned
		if err != nil {
			t.Errorf("YamlMarshalWithDefinedPaths failed: %v", err)
		}

		// And the boolean values should be correctly represented
		if !strings.Contains(string(result), `"true": true`) {
			t.Errorf("Expected '\"true\": true' in result, got: %s", string(result))
		}
		if !strings.Contains(string(result), `"false": false`) {
			t.Errorf("Expected '\"false\": false' in result, got: %s", string(result))
		}
	})

	t.Run("NilPointerAndInterface", func(t *testing.T) {
		// Given a struct with nil pointers and interfaces
		type testStruct struct {
			NilPtr       *string              `yaml:"nil_ptr"`
			NilInterface any                  `yaml:"nil_interface"`
			NilMap       map[string]string    `yaml:"nil_map"`
			NilSlice     []string             `yaml:"nil_slice"`
			NilStruct    *struct{ Field int } `yaml:"nil_struct"`
		}
		input := testStruct{}

		// When marshalling the struct
		handler := setup(t)
		result, err := handler.YamlMarshalWithDefinedPaths(input)

		// Then no error should be returned
		if err != nil {
			t.Errorf("YamlMarshalWithDefinedPaths failed: %v", err)
		}

		// And nil interfaces should be represented as empty objects
		if !strings.Contains(string(result), "nil_interface: {}") {
			t.Errorf("Expected 'nil_interface: {}' in result, got: %s", string(result))
		}

		// And nil slices should be represented as empty arrays
		if !strings.Contains(string(result), "nil_slice: []") {
			t.Errorf("Expected 'nil_slice: []' in result, got: %s", string(result))
		}

		// And nil maps should be represented as empty objects
		if !strings.Contains(string(result), "nil_map: {}") {
			t.Errorf("Expected 'nil_map: {}' in result, got: %s", string(result))
		}

		// And nil structs should be represented as empty objects
		if !strings.Contains(string(result), "nil_struct: {}") {
			t.Errorf("Expected 'nil_struct: {}' in result, got: %s", string(result))
		}
	})

	t.Run("SliceWithNilElements", func(t *testing.T) {
		// Given a slice with nil elements
		type elem struct {
			Field string
		}
		input := []*elem{nil, {Field: "value"}, nil}

		// When marshalling the slice
		handler := setup(t)
		result, err := handler.YamlMarshalWithDefinedPaths(input)

		// Then no error should be returned
		if err != nil {
			t.Errorf("YamlMarshalWithDefinedPaths failed: %v", err)
		}

		// And non-nil elements should be correctly represented
		if !strings.Contains(string(result), "Field: value") {
			t.Errorf("Expected 'Field: value' in result, got: %s", string(result))
		}
	})

	t.Run("ErrorInSliceConversion", func(t *testing.T) {
		// Given a slice containing an unsupported type
		input := []any{make(chan int)}

		// When attempting to marshal the slice
		handler := setup(t)
		_, err := handler.YamlMarshalWithDefinedPaths(input)

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error for slice with unsupported type, got nil")
		}

		// And the error should indicate the slice conversion issue
		if !strings.Contains(err.Error(), "error converting slice element") {
			t.Errorf("Expected error about slice conversion, got: %v", err)
		}
	})

	t.Run("ErrorInMapConversion", func(t *testing.T) {
		// Given a map containing an unsupported type
		input := map[string]any{
			"channel": make(chan int),
		}

		// When attempting to marshal the map
		handler := setup(t)
		_, err := handler.YamlMarshalWithDefinedPaths(input)

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error for map with unsupported type, got nil")
		}

		// And the error should indicate the map conversion issue
		if !strings.Contains(err.Error(), "error converting map value") {
			t.Errorf("Expected error about map conversion, got: %v", err)
		}
	})

	t.Run("ErrorInStructFieldConversion", func(t *testing.T) {
		// Given a struct containing an unsupported field type
		type testStruct struct {
			Channel chan int
		}
		input := testStruct{Channel: make(chan int)}

		// When attempting to marshal the struct
		handler := setup(t)
		_, err := handler.YamlMarshalWithDefinedPaths(input)

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error for struct with unsupported field type, got nil")
		}

		// And the error should indicate the field conversion issue
		if !strings.Contains(err.Error(), "error converting field") {
			t.Errorf("Expected error about field conversion, got: %v", err)
		}
	})

	t.Run("YamlMarshalError", func(t *testing.T) {
		// Given a config handler with mocked YAML marshalling that fails
		handler := setup(t)

		// And a mock YAML marshaller that returns an error
		handler.shims.YamlMarshal = func(v any) ([]byte, error) {
			return nil, fmt.Errorf("mock yaml marshal error")
		}

		// And a simple struct to marshal
		input := struct{ Field string }{Field: "value"}

		// When marshalling the struct
		_, err := handler.YamlMarshalWithDefinedPaths(input)

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error from yaml marshal, got nil")
		}

		// And the error should indicate the YAML marshalling issue
		if !strings.Contains(err.Error(), "error marshalling yaml") {
			t.Errorf("Expected error about yaml marshalling, got: %v", err)
		}
	})
}

// Test specifically for the flag override issue we're experiencing
func TestYamlConfigHandler_FlagOverrideIssue(t *testing.T) {
	setup := func(t *testing.T) (*YamlConfigHandler, *Mocks) {
		mocks := setupMocks(t)
		handler := NewYamlConfigHandler(mocks.Injector)
		if err := handler.Initialize(); err != nil {
			t.Fatalf("Failed to initialize handler: %v", err)
		}
		handler.shims = mocks.Shims
		return handler, mocks
	}

	t.Run("FlagValuePreservedAfterSetDefault", func(t *testing.T) {
		// Given a handler that simulates loading existing config (like init pipeline does)
		handler, _ := setup(t)
		handler.context = "local"

		// Simulate existing config with different VM driver
		existingConfig := `version: v1alpha1
contexts:
  local:
    id: existing-id
    vm:
      driver: existing-driver`

		err := handler.LoadConfigString(existingConfig)
		if err != nil {
			t.Fatalf("Failed to load config: %v", err)
		}

		// Simulate flag being set (like cmd/init.go does)
		err = handler.SetContextValue("vm.driver", "colima")
		if err != nil {
			t.Fatalf("Failed to set flag value: %v", err)
		}

		// Verify flag value is set correctly before SetDefault
		vmDriver := handler.GetString("vm.driver")
		if vmDriver != "colima" {
			t.Errorf("Expected vm.driver to be 'colima' before SetDefault, got '%s'", vmDriver)
		}

		// Simulate SetDefault being called (like init pipeline does)
		// Use a default config that has no VM section (like DefaultConfig_Full)
		defaultConfig := v1alpha1.Context{
			Environment: map[string]string{
				"DEFAULT_VAR": "default_value",
			},
			Provider: ptrString("local"),
		}

		err = handler.SetDefault(defaultConfig)
		if err != nil {
			t.Fatalf("Failed to set default: %v", err)
		}

		// Then the flag value should still be preserved after SetDefault
		vmDriverAfter := handler.GetString("vm.driver")
		if vmDriverAfter != "colima" {
			t.Errorf("Expected vm.driver to remain 'colima' after SetDefault, got '%s'", vmDriverAfter)
		}

		// And the default values should be added
		provider := handler.GetString("provider")
		if provider != "local" {
			t.Errorf("Expected provider to be added as 'local', got '%s'", provider)
		}
	})
}

// =============================================================================
// LoadContextConfig Tests
// =============================================================================

func TestYamlConfigHandler_LoadContextConfig(t *testing.T) {
	setup := func(t *testing.T) (*YamlConfigHandler, *Mocks) {
		mocks := setupMocks(t)
		handler := NewYamlConfigHandler(mocks.Injector)
		handler.shims = mocks.Shims
		if err := handler.Initialize(); err != nil {
			t.Fatalf("Failed to initialize handler: %v", err)
		}
		return handler, mocks
	}

	t.Run("SuccessWithContextConfig", func(t *testing.T) {
		// Given a YamlConfigHandler with existing config
		handler, mocks := setup(t)

		// Load base configuration
		baseConfig := `version: v1alpha1
contexts:
  production:
    provider: aws
    environment:
      BASE_VAR: base_value`
		if err := handler.LoadConfigString(baseConfig); err != nil {
			t.Fatalf("Failed to load base config: %v", err)
		}

		// Set current context to production
		if err := handler.SetContext("production"); err != nil {
			t.Fatalf("Failed to set context: %v", err)
		}

		// Override the shim to return the correct context
		handler.shims.Getenv = func(key string) string {
			if key == "WINDSOR_CONTEXT" {
				return "production"
			}
			return ""
		}

		// Create context-specific config file
		projectRoot, _ := mocks.Shell.GetProjectRootFunc()
		contextDir := filepath.Join(projectRoot, "contexts", "production")
		if err := os.MkdirAll(contextDir, 0755); err != nil {
			t.Fatalf("Failed to create context directory: %v", err)
		}

		contextConfigPath := filepath.Join(contextDir, "windsor.yaml")
		contextConfig := `provider: local
environment:
  CONTEXT_VAR: context_value
  BASE_VAR: overridden_value
aws:
  enabled: true`

		if err := os.WriteFile(contextConfigPath, []byte(contextConfig), 0644); err != nil {
			t.Fatalf("Failed to write context config: %v", err)
		}

		// Override shims to allow reading the actual context file
		handler.shims.Stat = func(name string) (os.FileInfo, error) {
			return os.Stat(name)
		}
		handler.shims.ReadFile = func(filename string) ([]byte, error) {
			return os.ReadFile(filename)
		}

		// When LoadContextConfig is called
		err := handler.LoadContextConfig()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("LoadContextConfig() unexpected error: %v", err)
		}

		// And the context configuration should be merged
		if handler.GetString("provider") != "local" {
			t.Errorf("Expected provider to be overridden to 'local', got '%s'", handler.GetString("provider"))
		}
		if handler.GetString("environment.CONTEXT_VAR") != "context_value" {
			t.Errorf("Expected CONTEXT_VAR to be 'context_value', got '%s'", handler.GetString("environment.CONTEXT_VAR"))
		}
		if handler.GetString("environment.BASE_VAR") != "overridden_value" {
			t.Errorf("Expected BASE_VAR to be overridden to 'overridden_value', got '%s'", handler.GetString("environment.BASE_VAR"))
		}
		if !handler.GetBool("aws.enabled") {
			t.Error("Expected aws.enabled to be true")
		}
	})

	t.Run("SuccessWithYmlExtension", func(t *testing.T) {
		// Given a YamlConfigHandler
		handler, mocks := setup(t)
		if err := handler.SetContext("local"); err != nil {
			t.Fatalf("Failed to set context: %v", err)
		}

		// Override the shim to return the correct context
		handler.shims.Getenv = func(key string) string {
			if key == "WINDSOR_CONTEXT" {
				return "local"
			}
			return ""
		}

		// Create context-specific config file with .yml extension
		projectRoot, _ := mocks.Shell.GetProjectRootFunc()
		contextDir := filepath.Join(projectRoot, "contexts", "local")
		if err := os.MkdirAll(contextDir, 0755); err != nil {
			t.Fatalf("Failed to create context directory: %v", err)
		}

		contextConfigPath := filepath.Join(contextDir, "windsor.yml")
		contextConfig := `provider: local
environment:
  TEST_VAR: test_value`

		if err := os.WriteFile(contextConfigPath, []byte(contextConfig), 0644); err != nil {
			t.Fatalf("Failed to write context config: %v", err)
		}

		// Override shims to allow reading the actual context file
		handler.shims.Stat = func(name string) (os.FileInfo, error) {
			return os.Stat(name)
		}
		handler.shims.ReadFile = func(filename string) ([]byte, error) {
			return os.ReadFile(filename)
		}

		// When LoadContextConfig is called
		err := handler.LoadContextConfig()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("LoadContextConfig() unexpected error: %v", err)
		}

		// And the context configuration should be loaded
		if handler.GetString("provider") != "local" {
			t.Errorf("Expected provider to be 'local', got '%s'", handler.GetString("provider"))
		}
		if handler.GetString("environment.TEST_VAR") != "test_value" {
			t.Errorf("Expected TEST_VAR to be 'test_value', got '%s'", handler.GetString("environment.TEST_VAR"))
		}
	})

	t.Run("SuccessWithoutContextConfig", func(t *testing.T) {
		// Given a YamlConfigHandler without context-specific config
		handler, _ := setup(t)
		if err := handler.SetContext("nonexistent"); err != nil {
			t.Fatalf("Failed to set context: %v", err)
		}

		// When LoadContextConfig is called
		err := handler.LoadContextConfig()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("LoadContextConfig() unexpected error: %v", err)
		}
	})

	t.Run("ErrorReadingContextConfigFile", func(t *testing.T) {
		// Given a YamlConfigHandler
		handler, mocks := setup(t)
		if err := handler.SetContext("test"); err != nil {
			t.Fatalf("Failed to set context: %v", err)
		}

		// And a context config file that exists but cannot be read
		projectRoot, _ := mocks.Shell.GetProjectRootFunc()
		contextDir := filepath.Join(projectRoot, "contexts", "test")
		if err := os.MkdirAll(contextDir, 0755); err != nil {
			t.Fatalf("Failed to create context directory: %v", err)
		}

		contextConfigPath := filepath.Join(contextDir, "windsor.yaml")
		if err := os.WriteFile(contextConfigPath, []byte("test"), 0644); err != nil {
			t.Fatalf("Failed to write context config: %v", err)
		}

		// Mock ReadFile to return an error
		handler.shims.ReadFile = func(filename string) ([]byte, error) {
			// Normalize path separators for cross-platform compatibility
			normalizedPath := filepath.ToSlash(filename)
			if strings.Contains(normalizedPath, "contexts/test/windsor.yaml") {
				return nil, fmt.Errorf("mocked read error")
			}
			return os.ReadFile(filename)
		}

		// When LoadContextConfig is called
		err := handler.LoadContextConfig()

		// Then an error should be returned
		if err == nil {
			t.Fatal("LoadContextConfig() expected error, got nil")
		}

		// And the error message should contain the expected text
		expectedError := "error reading context config file: mocked read error"
		if err.Error() != expectedError {
			t.Errorf("LoadContextConfig() error = %v, expected '%s'", err, expectedError)
		}
	})

	t.Run("ErrorUnmarshallingContextConfig", func(t *testing.T) {
		// Given a YamlConfigHandler
		handler, mocks := setup(t)
		if err := handler.SetContext("test"); err != nil {
			t.Fatalf("Failed to set context: %v", err)
		}

		// Override the shim to return the correct context
		handler.shims.Getenv = func(key string) string {
			if key == "WINDSOR_CONTEXT" {
				return "test"
			}
			return ""
		}

		// And a context config file with invalid YAML
		projectRoot, _ := mocks.Shell.GetProjectRootFunc()
		contextDir := filepath.Join(projectRoot, "contexts", "test")
		if err := os.MkdirAll(contextDir, 0755); err != nil {
			t.Fatalf("Failed to create context directory: %v", err)
		}

		contextConfigPath := filepath.Join(contextDir, "windsor.yaml")
		invalidYaml := `provider: aws
invalid yaml: [
`
		if err := os.WriteFile(contextConfigPath, []byte(invalidYaml), 0644); err != nil {
			t.Fatalf("Failed to write context config: %v", err)
		}

		// Override shims to allow reading the actual context file
		handler.shims.Stat = func(name string) (os.FileInfo, error) {
			return os.Stat(name)
		}
		handler.shims.ReadFile = func(filename string) ([]byte, error) {
			return os.ReadFile(filename)
		}

		// When LoadContextConfig is called
		err := handler.LoadContextConfig()

		// Then an error should be returned
		if err == nil {
			t.Fatal("LoadContextConfig() expected error, got nil")
		}

		// And the error message should contain the expected text
		if !strings.Contains(err.Error(), "error unmarshalling context yaml") {
			t.Errorf("LoadContextConfig() error = %v, expected to contain 'error unmarshalling context yaml'", err)
		}
	})

	t.Run("ErrorShellNotInitialized", func(t *testing.T) {
		// Given a YamlConfigHandler without shell
		handler, _ := setup(t)
		handler.shell = nil

		// When LoadContextConfig is called
		err := handler.LoadContextConfig()

		// Then an error should be returned
		if err == nil {
			t.Fatal("LoadContextConfig() expected error, got nil")
		}

		// And the error message should be as expected
		expectedError := "shell not initialized"
		if err.Error() != expectedError {
			t.Errorf("LoadContextConfig() error = %v, expected '%s'", err, expectedError)
		}
	})

	t.Run("ErrorGettingProjectRoot", func(t *testing.T) {
		// Given a YamlConfigHandler with shell that returns error
		handler, mocks := setup(t)
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("mocked project root error")
		}

		// When LoadContextConfig is called
		err := handler.LoadContextConfig()

		// Then an error should be returned
		if err == nil {
			t.Fatal("LoadContextConfig() expected error, got nil")
		}

		// And the error message should be as expected
		expectedError := "error retrieving project root: mocked project root error"
		if err.Error() != expectedError {
			t.Errorf("LoadContextConfig() error = %v, expected '%s'", err, expectedError)
		}
	})

	t.Run("SimulateInitPipelineWorkflow", func(t *testing.T) {
		// Given a YamlConfigHandler simulating the exact init pipeline workflow
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
		if _, err := handler.shims.Stat(contextConfigPath); os.IsNotExist(err) {
			t.Errorf("Context config file was not created at %s, this reproduces the user's issue", contextConfigPath)
		}

		// And the root config should not be overwritten
		rootContent, _ := os.ReadFile(rootConfigPath)
		if !strings.Contains(string(rootContent), "version: v1alpha1") {
			t.Errorf("Root config appears to have been overwritten")
		}
	})

	t.Run("ContextNotSetInRootConfigInitially", func(t *testing.T) {
		// Given a YamlConfigHandler that mimics the exact init flow
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
		handler.context = "local"

		// Debug: Check state before adding any context data
		t.Logf("Config.Contexts before setting any context data: %+v", handler.config.Contexts)

		// When SaveConfig is called without any context configuration being set
		err := handler.SaveConfig()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// Check if context config was created
		contextConfigPath := filepath.Join(tempDir, "contexts", "local", "windsor.yaml")
		if _, err := handler.shims.Stat(contextConfigPath); os.IsNotExist(err) {
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
		t.Logf("Config before SaveConfig: %+v", handler.config)
		if handler.config.Contexts != nil {
			if ctx, exists := handler.config.Contexts["local"]; exists {
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
		if _, err := handler.shims.Stat(contextConfigPath); os.IsNotExist(err) {
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
}
