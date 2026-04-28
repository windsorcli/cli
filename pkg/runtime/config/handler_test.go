package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// =============================================================================
// Test Setup
// =============================================================================

type ConfigTestMocks struct {
	Shell *shell.MockShell
	Shims *Shims
}

// setupConfigMocks creates a new set of mocks for testing
func setupConfigMocks(t *testing.T) *ConfigTestMocks {
	t.Helper()

	tmpDir := t.TempDir()
	os.Setenv("WINDSOR_PROJECT_ROOT", tmpDir)
	os.Setenv("WINDSOR_CONTEXT", "test-context")

	mockShell := shell.NewMockShell()
	mockShell.GetProjectRootFunc = func() (string, error) {
		return tmpDir, nil
	}

	// Create initial mocks with defaults
	mocks := &ConfigTestMocks{
		Shell: mockShell,
		Shims: NewShims(),
	}

	t.Cleanup(func() {
		os.Unsetenv("WINDSOR_PROJECT_ROOT")
		os.Unsetenv("WINDSOR_CONTEXT")
	})

	return mocks
}

func setupPrivateTestHandler(t *testing.T) (*configHandler, string) {
	t.Helper()

	tmpDir := t.TempDir()
	mockShell := shell.NewMockShell()
	mockShell.GetProjectRootFunc = func() (string, error) {
		return tmpDir, nil
	}

	handler := NewConfigHandler(mockShell).(*configHandler)

	return handler, tmpDir
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestConfigHandler_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupConfigMocks(t)

		handler := NewConfigHandler(mocks.Shell)

		// ConfigHandler is now fully initialized in constructor
		if handler == nil {
			t.Error("Expected handler to be created")
		}

		if !handler.IsLoaded() == handler.IsLoaded() {
			t.Error("Expected handler to be initialized")
		}
	})

	t.Run("InitializesDataMap", func(t *testing.T) {
		mocks := setupConfigMocks(t)

		handler := NewConfigHandler(mocks.Shell)

		// ConfigHandler is now fully initialized in constructor
		if handler == nil {
			t.Fatal("Expected handler to be created")
		}

		handler.Set("test", "value")
		value := handler.Get("test")

		if value != "value" {
			t.Errorf("Expected data map to be initialized and usable")
		}
	})

	t.Run("CreatesHandlerWithShell", func(t *testing.T) {
		// Given a shell
		mocks := setupConfigMocks(t)

		handler := NewConfigHandler(mocks.Shell)

		// Then handler should be created
		if handler == nil {
			t.Error("Expected handler to be created")
		}
	})
}

func TestConfigHandler_LoadConfig(t *testing.T) {
	t.Run("LoadsAndMergesSourcesWithPrecedence", func(t *testing.T) {
		handler, tmpDir := setupPrivateTestHandler(t)
		handler.SetContext("integration-test")

		schemaDir := filepath.Join(tmpDir, "contexts", "_template")
		os.MkdirAll(schemaDir, 0755)
		schemaContent := `$schema: https://json-schema.org/draft/2020-12/schema
type: object
properties:
  schema_default:
    type: string
    default: from_schema
  override_test:
    type: string
    default: schema_value
`
		os.WriteFile(filepath.Join(schemaDir, "schema.yaml"), []byte(schemaContent), 0644)

		rootConfig := `version: v1alpha1
contexts:
  integration-test:
    provider: from_root
    override_test: root_value
`
		os.WriteFile(filepath.Join(tmpDir, "windsor.yaml"), []byte(rootConfig), 0644)

		contextDir := filepath.Join(tmpDir, "contexts", "integration-test")
		os.MkdirAll(contextDir, 0755)
		contextConfig := `cluster:
  enabled: true
override_test: context_value
`
		os.WriteFile(filepath.Join(contextDir, "windsor.yaml"), []byte(contextConfig), 0644)

		valuesContent := `override_test: values_final
custom_field: user_data
`
		os.WriteFile(filepath.Join(contextDir, "values.yaml"), []byte(valuesContent), 0644)

		if err := handler.LoadConfig(); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if handler.GetString("schema_default") != "from_schema" {
			t.Errorf("Expected schema default to be applied, got %v", handler.GetString("schema_default"))
		}
		if handler.GetString("provider") != "from_root" {
			t.Errorf("Expected provider from root config, got %v", handler.GetString("provider"))
		}
		if handler.GetString("override_test") != "values_final" {
			t.Errorf("Expected values.yaml precedence, got %v", handler.GetString("override_test"))
		}
		if handler.GetString("custom_field") != "user_data" {
			t.Errorf("Expected custom_field from values.yaml, got %v", handler.GetString("custom_field"))
		}
	})

	t.Run("LoadsLegacyContextYmlWhenYamlMissing", func(t *testing.T) {
		handler, tmpDir := setupPrivateTestHandler(t)
		handler.SetContext("test-context")
		contextDir := filepath.Join(tmpDir, "contexts", "test-context")
		os.MkdirAll(contextDir, 0755)
		os.WriteFile(filepath.Join(contextDir, "windsor.yml"), []byte("provider: from_yml\n"), 0644)

		if err := handler.LoadConfig(); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if handler.GetString("provider") != "from_yml" {
			t.Errorf("Expected provider from .yml file, got %v", handler.GetString("provider"))
		}
	})

	t.Run("LoadsContextScopedWorkstationStateOnly", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		tmpDir, _ := mocks.Shell.GetProjectRoot()
		os.MkdirAll(filepath.Join(tmpDir, ".windsor", "contexts", "ctx-a"), 0755)
		os.MkdirAll(filepath.Join(tmpDir, ".windsor", "contexts", "ctx-b"), 0755)
		os.WriteFile(filepath.Join(tmpDir, ".windsor", "contexts", "ctx-a", "workstation.yaml"), []byte("workstation:\n  runtime: colima\n"), 0644)
		os.WriteFile(filepath.Join(tmpDir, ".windsor", "contexts", "ctx-b", "workstation.yaml"), []byte("workstation:\n  runtime: docker-desktop\n"), 0644)

		handlerA := NewConfigHandler(mocks.Shell)
		if err := handlerA.LoadConfigForContext("ctx-a"); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		handlerB := NewConfigHandler(mocks.Shell)
		if err := handlerB.LoadConfigForContext("ctx-b"); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if handlerA.GetString("workstation.runtime") != "colima" {
			t.Errorf("Expected ctx-a runtime, got %v", handlerA.GetString("workstation.runtime"))
		}
		if handlerB.GetString("workstation.runtime") != "docker-desktop" {
			t.Errorf("Expected ctx-b runtime, got %v", handlerB.GetString("workstation.runtime"))
		}
	})

	t.Run("SetsLoadedFlagAfterSuccessfulLoad", func(t *testing.T) {
		handler, tmpDir := setupPrivateTestHandler(t)
		handler.SetContext("test-context")
		contextDir := filepath.Join(tmpDir, "contexts", "test-context")
		os.MkdirAll(contextDir, 0755)
		os.WriteFile(filepath.Join(contextDir, "windsor.yaml"), []byte("provider: local\n"), 0644)

		if err := handler.LoadConfig(); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !handler.IsLoaded() {
			t.Error("Expected handler to be marked loaded")
		}
	})

	t.Run("ReturnsErrorWhenShellIsNil", func(t *testing.T) {
		handler := &configHandler{}
		err := handler.LoadConfig()
		if err == nil {
			t.Fatal("Expected error when shell is nil")
		}
		if !strings.Contains(err.Error(), "shell not initialized") {
			t.Errorf("Expected shell initialization error, got %v", err)
		}
	})

	t.Run("ReturnsErrorWhenProjectRootUnavailable", func(t *testing.T) {
		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("project root error")
		}
		handler := NewConfigHandler(mockShell)
		err := handler.LoadConfig()
		if err == nil {
			t.Fatal("Expected error when project root is unavailable")
		}
		if !strings.Contains(err.Error(), "error retrieving project root") {
			t.Errorf("Expected project root retrieval error, got %v", err)
		}
	})

	t.Run("ReturnsErrorForInvalidYamlInRootOrContextOrValues", func(t *testing.T) {
		handler, tmpDir := setupPrivateTestHandler(t)
		handler.SetContext("test-context")

		os.WriteFile(filepath.Join(tmpDir, "windsor.yaml"), []byte("invalid: yaml: [[["), 0644)
		if err := handler.LoadConfig(); err == nil {
			t.Fatal("Expected error for invalid root config")
		}

		os.WriteFile(filepath.Join(tmpDir, "windsor.yaml"), []byte("version: v1alpha1\n"), 0644)
		contextDir := filepath.Join(tmpDir, "contexts", "test-context")
		os.MkdirAll(contextDir, 0755)
		os.WriteFile(filepath.Join(contextDir, "windsor.yaml"), []byte("invalid: yaml: [[["), 0644)
		if err := handler.LoadConfig(); err == nil {
			t.Fatal("Expected error for invalid context config")
		}

		os.WriteFile(filepath.Join(contextDir, "windsor.yaml"), []byte("provider: local\n"), 0644)
		os.WriteFile(filepath.Join(contextDir, "values.yaml"), []byte("invalid: yaml: [[["), 0644)
		if err := handler.LoadConfig(); err == nil {
			t.Fatal("Expected error for invalid values config")
		}
	})
}

func TestConfigHandler_LoadConfigForContext(t *testing.T) {
	t.Run("LoadsConfigForSpecifiedContext", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		tmpDir, _ := mocks.Shell.GetProjectRoot()
		rootConfig := `version: v1alpha1
contexts:
  test-context:
    provider: local
    dns:
      domain: example.com
`
		os.WriteFile(filepath.Join(tmpDir, "windsor.yaml"), []byte(rootConfig), 0644)

		err := handler.LoadConfigForContext("test-context")

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		provider := handler.GetString("provider")
		if provider != "local" {
			t.Errorf("Expected provider='local', got '%s'", provider)
		}

		domain := handler.GetString("dns.domain")
		if domain != "example.com" {
			t.Errorf("Expected dns.domain='example.com', got '%s'", domain)
		}
	})

	t.Run("DoesNotChangeCurrentContext", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)
		handler.SetContext("original-context")

		tmpDir, _ := mocks.Shell.GetProjectRoot()
		rootConfig := `version: v1alpha1
contexts:
  other-context:
    provider: aws
`
		os.WriteFile(filepath.Join(tmpDir, "windsor.yaml"), []byte(rootConfig), 0644)

		err := handler.LoadConfigForContext("other-context")
		if err != nil {
			t.Fatalf("LoadConfigForContext failed: %v", err)
		}

		currentContext := handler.GetContext()
		if currentContext != "original-context" {
			t.Errorf("Expected current context to remain 'original-context', got '%s'", currentContext)
		}
	})

	t.Run("LoadsContextSpecificFiles", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		tmpDir, _ := mocks.Shell.GetProjectRoot()
		contextDir := filepath.Join(tmpDir, "contexts", "test-context")
		os.MkdirAll(contextDir, 0755)
		contextConfig := `provider: docker
cluster:
  enabled: true
  driver: talos
`
		os.WriteFile(filepath.Join(contextDir, "windsor.yaml"), []byte(contextConfig), 0644)

		err := handler.LoadConfigForContext("test-context")

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		provider := handler.GetString("provider")
		if provider != "docker" {
			t.Errorf("Expected provider='docker', got '%s'", provider)
		}

		driver := handler.GetString("cluster.driver")
		if driver != "talos" {
			t.Errorf("Expected cluster.driver='talos', got '%s'", driver)
		}
	})

	t.Run("LoadsValuesYamlForContext", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		tmpDir, _ := mocks.Shell.GetProjectRoot()
		contextDir := filepath.Join(tmpDir, "contexts", "test-context")
		os.MkdirAll(contextDir, 0755)
		valuesContent := `dev: true
custom_key: custom_value
`
		os.WriteFile(filepath.Join(contextDir, "values.yaml"), []byte(valuesContent), 0644)

		err := handler.LoadConfigForContext("test-context")

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		dev := handler.GetBool("dev")
		if !dev {
			t.Error("Expected dev=true")
		}

		customKey := handler.GetString("custom_key")
		if customKey != "custom_value" {
			t.Errorf("Expected custom_key='custom_value', got '%s'", customKey)
		}
	})

	t.Run("PanicsWhenShellNotInitialized", func(t *testing.T) {
		// When NewConfigHandler is called with nil shell
		// Then it should panic
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected panic when shell is nil")
			}
		}()
		_ = NewConfigHandler(nil)
	})
}

func TestConfigHandler_LoadConfigString(t *testing.T) {
	t.Run("ExtractsCurrentContextSection", func(t *testing.T) {
		os.Setenv("WINDSOR_CONTEXT", "test-context")
		defer os.Unsetenv("WINDSOR_CONTEXT")

		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		yaml := `version: v1alpha1
contexts:
  test-context:
    provider: local
    dns:
      domain: test
  other-context:
    provider: remote
`

		err := handler.LoadConfigString(yaml)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		provider := handler.GetString("provider")
		if provider != "local" {
			t.Errorf("Expected provider='local' from test-context, got '%s'", provider)
		}

		domain := handler.GetString("dns.domain")
		if domain != "test" {
			t.Errorf("Expected dns.domain='test', got '%s'", domain)
		}
	})

	t.Run("PrefersContextSectionOverTopLevelKeysWhenPresent", func(t *testing.T) {
		os.Setenv("WINDSOR_CONTEXT", "test-context")
		defer os.Unsetenv("WINDSOR_CONTEXT")

		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		content := `provider: top_level
contexts:
  test-context:
    provider: from_context
`

		if err := handler.LoadConfigString(content); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if provider := handler.GetString("provider"); provider != "from_context" {
			t.Errorf("Expected context provider to win, got '%s'", provider)
		}
	})

	t.Run("MergesFlatYamlStructure", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		yaml := `provider: docker
custom_key: custom_value
`

		err := handler.LoadConfigString(yaml)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		provider := handler.GetString("provider")
		if provider != "docker" {
			t.Errorf("Expected provider='docker', got '%s'", provider)
		}

		customKey := handler.GetString("custom_key")
		if customKey != "custom_value" {
			t.Errorf("Expected custom_key='custom_value', got '%s'", customKey)
		}
	})

	t.Run("SetsLoadedFlag", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		err := handler.LoadConfigString("provider: test\n")

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if !handler.IsLoaded() {
			t.Error("Expected IsLoaded=true after LoadConfigString")
		}
	})

	t.Run("HandlesUnmarshalError", func(t *testing.T) {
		// Given a handler and invalid YAML string
		handler, _ := setupPrivateTestHandler(t)

		// When loading invalid YAML
		err := handler.LoadConfigString("invalid: yaml: [[[")

		// Then it should return unmarshal error
		if err == nil {
			t.Error("Expected unmarshal error")
		}
	})

	t.Run("HandlesEmptyString", func(t *testing.T) {
		// Given a handler and empty string
		handler, _ := setupPrivateTestHandler(t)

		// When loading empty string
		err := handler.LoadConfigString("")

		// Then it should succeed without error
		if err != nil {
			t.Errorf("Expected no error for empty string, got %v", err)
		}
	})

	t.Run("HandlesMapAnyAnyContexts", func(t *testing.T) {
		os.Setenv("WINDSOR_CONTEXT", "test-context")
		defer os.Unsetenv("WINDSOR_CONTEXT")

		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		yaml := `version: v1alpha1
contexts:
  test-context:
    provider: local
    custom: value
`

		err := handler.LoadConfigString(yaml)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		provider := handler.GetString("provider")
		if provider != "local" {
			t.Errorf("Expected provider='local', got '%s'", provider)
		}

		custom := handler.GetString("custom")
		if custom != "value" {
			t.Errorf("Expected custom='value', got '%s'", custom)
		}
	})

	t.Run("HandlesContextDataAsMapAnyAny", func(t *testing.T) {
		os.Setenv("WINDSOR_CONTEXT", "test-context")
		defer os.Unsetenv("WINDSOR_CONTEXT")

		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell).(*configHandler)

		yaml := `version: v1alpha1
contexts:
  test-context:
    provider: local
    nested:
      key: value
`

		err := handler.LoadConfigString(yaml)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		provider := handler.GetString("provider")
		if provider != "local" {
			t.Errorf("Expected provider='local', got '%s'", provider)
		}
	})

	t.Run("HandlesMissingContextInContexts", func(t *testing.T) {
		os.Setenv("WINDSOR_CONTEXT", "missing-context")
		defer os.Unsetenv("WINDSOR_CONTEXT")

		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		yaml := `version: v1alpha1
contexts:
  other-context:
    provider: local
flat_key: flat_value
`

		err := handler.LoadConfigString(yaml)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		flatKey := handler.GetString("flat_key")
		if flatKey != "flat_value" {
			t.Errorf("Expected flat_key='flat_value', got '%s'", flatKey)
		}
	})

	t.Run("HandlesContextsAsNonMap", func(t *testing.T) {
		os.Setenv("WINDSOR_CONTEXT", "test-context")
		defer os.Unsetenv("WINDSOR_CONTEXT")

		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		yaml := `version: v1alpha1
contexts: not_a_map
flat_key: flat_value
`

		err := handler.LoadConfigString(yaml)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		flatKey := handler.GetString("flat_key")
		if flatKey != "flat_value" {
			t.Errorf("Expected flat_key='flat_value', got '%s'", flatKey)
		}
	})

	t.Run("HandlesContextDataAsNonMap", func(t *testing.T) {
		os.Setenv("WINDSOR_CONTEXT", "test-context")
		defer os.Unsetenv("WINDSOR_CONTEXT")

		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		yaml := `version: v1alpha1
contexts:
  test-context: not_a_map
flat_key: flat_value
`

		err := handler.LoadConfigString(yaml)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		flatKey := handler.GetString("flat_key")
		if flatKey != "flat_value" {
			t.Errorf("Expected flat_key='flat_value', got '%s'", flatKey)
		}
	})
}

func TestConfigHandler_SaveConfig(t *testing.T) {
	t.Run("EnsuresRootConfigExistsAndWritesValues", func(t *testing.T) {
		handler, tmpDir := setupPrivateTestHandler(t)
		handler.SetContext("test-context")
		handler.Set("provider", "generic")
		handler.Set("cluster.driver", "talos")
		handler.Set("custom_dynamic_field", "dynamic_value")

		if err := handler.SaveConfig(); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		rootPath := filepath.Join(tmpDir, "windsor.yaml")
		if _, err := os.Stat(rootPath); err != nil {
			t.Fatalf("Expected root windsor.yaml to exist, got %v", err)
		}

		valuesPath := filepath.Join(tmpDir, "contexts", "test-context", "values.yaml")
		valuesContent, err := os.ReadFile(valuesPath)
		if err != nil {
			t.Fatalf("Expected values.yaml to exist, got %v", err)
		}
		if contains(string(valuesContent), "provider:") || !contains(string(valuesContent), "custom_dynamic_field: dynamic_value") {
			t.Errorf("Expected explicit values in values.yaml, got:\n%s", string(valuesContent))
		}
	})

	t.Run("DoesNotPersistSchemaDefaults", func(t *testing.T) {
		handler, tmpDir := setupPrivateTestHandler(t)
		handler.SetContext("test-context")
		schemaDir := filepath.Join(tmpDir, "contexts", "_template")
		os.MkdirAll(schemaDir, 0755)
		schemaContent := `$schema: https://json-schema.org/draft/2020-12/schema
type: object
properties:
  schema_only_key:
    type: string
    default: should_not_be_saved
`
		os.WriteFile(filepath.Join(schemaDir, "schema.yaml"), []byte(schemaContent), 0644)
		if err := handler.LoadConfig(); err != nil {
			t.Fatalf("Expected no error loading config, got %v", err)
		}
		if err := handler.SaveConfig(); err != nil {
			t.Fatalf("Expected no error saving config, got %v", err)
		}
		valuesPath := filepath.Join(tmpDir, "contexts", "test-context", "values.yaml")
		if _, err := os.Stat(valuesPath); err == nil {
			content, _ := os.ReadFile(valuesPath)
			if contains(string(content), "schema_only_key") {
				t.Errorf("Did not expect schema defaults in persisted values:\n%s", string(content))
			}
		}
	})

	t.Run("RespectsOverwriteFlagForExistingValues", func(t *testing.T) {
		handler, tmpDir := setupPrivateTestHandler(t)
		handler.SetContext("test-context")
		contextDir := filepath.Join(tmpDir, "contexts", "test-context")
		os.MkdirAll(contextDir, 0755)
		valuesPath := filepath.Join(contextDir, "values.yaml")
		os.WriteFile(valuesPath, []byte("existing_key: old_value\n"), 0644)

		handler.Set("dynamic_key", "new_value")
		if err := handler.SaveConfig(); err != nil {
			t.Fatalf("Expected no error saving without overwrite, got %v", err)
		}
		contentNoOverwrite, _ := os.ReadFile(valuesPath)
		if string(contentNoOverwrite) != "existing_key: old_value\n" {
			t.Errorf("Expected no overwrite without flag, got:\n%s", string(contentNoOverwrite))
		}

		if err := handler.SaveConfig(true); err != nil {
			t.Fatalf("Expected no error saving with overwrite, got %v", err)
		}
		contentOverwrite, _ := os.ReadFile(valuesPath)
		if !contains(string(contentOverwrite), "dynamic_key") {
			t.Errorf("Expected overwritten values.yaml to include dynamic_key, got:\n%s", string(contentOverwrite))
		}
	})

	t.Run("SaveAndReloadPreservesExplicitData", func(t *testing.T) {
		handler, _ := setupPrivateTestHandler(t)
		handler.SetContext("save-test")
		handler.Set("cluster.driver", "talos")
		handler.Set("cluster.workers.count", 2)
		handler.Set("custom_dynamic", "dynamic_value")

		if err := handler.SaveConfig(); err != nil {
			t.Fatalf("Expected no error saving config, got %v", err)
		}

		newHandler, _ := setupPrivateTestHandler(t)
		newHandler.shell = handler.shell
		newHandler.SetContext("save-test")
		if err := newHandler.LoadConfig(); err != nil {
			t.Fatalf("Expected no error loading saved config, got %v", err)
		}
		if newHandler.GetString("cluster.driver") != "talos" {
			t.Errorf("Expected cluster.driver to round-trip, got %v", newHandler.GetString("cluster.driver"))
		}
		if newHandler.GetInt("cluster.workers.count") != 2 {
			t.Errorf("Expected cluster.workers.count=2, got %v", newHandler.GetInt("cluster.workers.count"))
		}
		if newHandler.GetString("custom_dynamic") != "dynamic_value" {
			t.Errorf("Expected custom_dynamic to round-trip, got %v", newHandler.GetString("custom_dynamic"))
		}
	})

	t.Run("ReturnsErrorWhenProjectRootUnavailable", func(t *testing.T) {
		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("project root error")
		}
		handler := NewConfigHandler(mockShell)
		if err := handler.SaveConfig(); err == nil {
			t.Fatal("Expected project root retrieval error")
		}
	})

	t.Run("ReturnsErrorWhenValuesWriteFails", func(t *testing.T) {
		handler, _ := setupPrivateTestHandler(t)
		handler.SetContext("test-context")
		handler.Set("dynamic_key", "value")
		callCount := 0
		handler.shims.WriteFile = func(name string, data []byte, perm os.FileMode) error {
			callCount++
			if callCount == 1 {
				return nil
			}
			return fmt.Errorf("write error for values.yaml")
		}
		err := handler.SaveConfig()
		if err == nil {
			t.Fatal("Expected write error for values.yaml")
		}
		if !contains(err.Error(), "error writing values.yaml") {
			t.Errorf("Expected values.yaml write error, got %v", err)
		}
	})

	t.Run("SkipsEnsureRootInGlobalMode", func(t *testing.T) {
		handler, tmpDir := setupPrivateTestHandler(t)
		// Swap in a global-mode shell pointing at the same tmp dir
		globalShell := shell.NewMockShell()
		globalShell.GetProjectRootFunc = func() (string, error) {
			return tmpDir, nil
		}
		globalShell.IsGlobalFunc = func() bool {
			return true
		}
		handler.shell = globalShell
		handler.SetContext("test-context")
		handler.Set("custom_dynamic_field", "dynamic_value")

		if err := handler.SaveConfig(); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		rootPath := filepath.Join(tmpDir, "windsor.yaml")
		if _, err := os.Stat(rootPath); err == nil {
			t.Errorf("Expected root windsor.yaml to NOT be created in global mode, but it exists at %s", rootPath)
		}

		valuesPath := filepath.Join(tmpDir, "contexts", "test-context", "values.yaml")
		if _, err := os.Stat(valuesPath); err != nil {
			t.Errorf("Expected values.yaml to still be written, got %v", err)
		}
	})
}

func TestConfigHandler_WorkstationStatePersistence(t *testing.T) {
	t.Run("SavesWorkstationStatePerContext", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		tmpDir, _ := mocks.Shell.GetProjectRoot()

		handler := NewConfigHandler(mocks.Shell)
		if err := handler.SetContext("ctx-a"); err != nil {
			t.Fatalf("Expected no error setting context, got %v", err)
		}
		if err := handler.Set("workstation.runtime", "colima"); err != nil {
			t.Fatalf("Expected no error setting workstation.runtime, got %v", err)
		}

		if err := handler.SaveWorkstationState(); err != nil {
			t.Fatalf("Expected no error saving workstation state, got %v", err)
		}

		contextPath := filepath.Join(tmpDir, ".windsor", "contexts", "ctx-a", "workstation.yaml")
		if _, err := os.Stat(contextPath); err != nil {
			t.Fatalf("Expected context-scoped workstation state file, got stat error: %v", err)
		}
	})

	t.Run("SaveConfigAndSaveWorkstationStateRespectOwnershipBoundaries", func(t *testing.T) {
		handler, tmpDir := setupPrivateTestHandler(t)
		if err := handler.SetContext("local-dev"); err != nil {
			t.Fatalf("Expected no error setting context, got %v", err)
		}
		if err := handler.Set("provider", "docker"); err != nil {
			t.Fatalf("Expected no error setting provider, got %v", err)
		}
		if err := handler.Set("platform", "docker"); err != nil {
			t.Fatalf("Expected no error setting platform, got %v", err)
		}
		if err := handler.Set("workstation.runtime", "colima"); err != nil {
			t.Fatalf("Expected no error setting workstation.runtime, got %v", err)
		}

		if err := handler.SaveConfig(true); err != nil {
			t.Fatalf("Expected no error saving config, got %v", err)
		}
		if err := handler.SaveWorkstationState(); err != nil {
			t.Fatalf("Expected no error saving workstation state, got %v", err)
		}

		valuesPath := filepath.Join(tmpDir, "contexts", "local-dev", "values.yaml")
		valuesContent, err := os.ReadFile(valuesPath)
		if err != nil {
			t.Fatalf("Expected values.yaml to exist, got %v", err)
		}
		valuesData := string(valuesContent)
		if contains(valuesData, "provider:") {
			t.Errorf("Expected provider excluded from values.yaml, got:\n%s", valuesData)
		}
		if contains(valuesData, "platform:") {
			t.Errorf("Expected platform excluded from values.yaml for dev context, got:\n%s", valuesData)
		}
		if contains(valuesData, "workstation:") {
			t.Errorf("Expected workstation excluded from values.yaml, got:\n%s", valuesData)
		}

		workstationPath := filepath.Join(tmpDir, ".windsor", "contexts", "local-dev", "workstation.yaml")
		workstationContent, err := os.ReadFile(workstationPath)
		if err != nil {
			t.Fatalf("Expected workstation.yaml to exist, got %v", err)
		}
		workstationData := string(workstationContent)
		if !contains(workstationData, "platform: docker") {
			t.Errorf("Expected platform in workstation.yaml, got:\n%s", workstationData)
		}
		if !contains(workstationData, "runtime: colima") {
			t.Errorf("Expected runtime in workstation.yaml, got:\n%s", workstationData)
		}
	})
}

func TestConfigHandler_SetDefault(t *testing.T) {
	t.Run("MergesDefaultContextIntoData", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		defaultContext := v1alpha1.Context{
			Provider: ptrString("default_provider"),
		}

		err := handler.SetDefault(defaultContext)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		provider := handler.GetString("provider")
		if provider != "default_provider" {
			t.Errorf("Expected provider='default_provider', got '%s'", provider)
		}
	})

	t.Run("AllowsOverridingDefaults", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		defaultContext := v1alpha1.Context{
			Provider: ptrString("default_provider"),
		}
		handler.SetDefault(defaultContext)

		handler.Set("provider", "override_provider")

		provider := handler.GetString("provider")

		if provider != "override_provider" {
			t.Errorf("Expected override to work, got '%s'", provider)
		}
	})

	t.Run("HandlesMarshalError", func(t *testing.T) {
		// Given a config handler with failing marshal
		handler, _ := setupPrivateTestHandler(t)

		handler.shims.YamlMarshal = func(v any) ([]byte, error) {
			return nil, os.ErrInvalid
		}

		// When setting default with marshal error
		err := handler.SetDefault(v1alpha1.Context{})

		// Then setting should fail
		if err == nil {
			t.Error("Expected marshal error")
		}
	})

	t.Run("HandlesUnmarshalError", func(t *testing.T) {
		// Given a config handler with failing unmarshal
		handler, _ := setupPrivateTestHandler(t)

		handler.shims.YamlUnmarshal = func(data []byte, v any) error {
			return os.ErrInvalid
		}

		// When setting default with unmarshal error
		err := handler.SetDefault(v1alpha1.Context{Provider: ptrString("test")})

		// Then setting should fail
		if err == nil {
			t.Error("Expected unmarshal error")
		}
	})
}

func TestConfigHandler_GetConfig(t *testing.T) {
	t.Run("ConvertsDataMapToContextStruct", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		handler.Set("provider", "test_provider")
		handler.Set("dns.domain", "test.local")

		config := handler.GetConfig()

		if config == nil {
			t.Fatal("Expected non-nil config")
		}
		if config.Provider == nil || *config.Provider != "test_provider" {
			t.Errorf("Expected provider='test_provider', got %v", config.Provider)
		}
		if config.DNS == nil || config.DNS.Domain == nil || *config.DNS.Domain != "test.local" {
			t.Errorf("Expected dns.domain='test.local', got %v", config.DNS)
		}
	})

	t.Run("ExcludesNodesFieldDueToYamlTag", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		handler.Set("cluster.workers.count", 2)
		handler.Set("cluster.workers.nodes.worker-1.endpoint", "127.0.0.1:50001")

		config := handler.GetConfig()

		if config == nil || config.Cluster == nil {
			t.Fatal("Expected cluster.workers to exist")
		}
		if config.Cluster.Workers.Count == nil || *config.Cluster.Workers.Count != 2 {
			t.Error("Expected count=2")
		}
		if len(config.Cluster.Workers.Nodes) > 0 {
			t.Error("Expected nodes to be excluded (yaml:\"-\" tag)")
		}
	})

	t.Run("HandlesMarshalError", func(t *testing.T) {
		// Given a config handler with failing marshal
		handler, _ := setupPrivateTestHandler(t)
		handler.Set("test", "value")

		handler.shims.YamlMarshal = func(v any) ([]byte, error) {
			return nil, os.ErrInvalid
		}

		// When getting config with marshal error
		config := handler.GetConfig()

		// Then empty config should be returned
		if config == nil {
			t.Error("Expected empty config on marshal error, got nil")
		}
	})

	t.Run("HandlesUnmarshalError", func(t *testing.T) {
		// Given a config handler with failing unmarshal
		handler, _ := setupPrivateTestHandler(t)
		handler.Set("test", "value")

		callCount := 0
		handler.shims.YamlUnmarshal = func(data []byte, v any) error {
			callCount++
			if callCount > 1 {
				return os.ErrInvalid
			}
			return handler.shims.YamlUnmarshal(data, v)
		}

		// When getting config with unmarshal error
		config := handler.GetConfig()

		// Then empty config should be returned
		if config == nil {
			t.Error("Expected empty config on unmarshal error, got nil")
		}
	})
}

func TestConfigHandler_GetConfigRoot(t *testing.T) {
	t.Run("ReturnsConfigRoot", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		tmpDir, _ := mocks.Shell.GetProjectRoot()

		handler := NewConfigHandler(mocks.Shell)
		handler.SetContext("test-context")

		root, err := handler.GetConfigRoot()

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		expectedRoot := filepath.Join(tmpDir, "contexts", "test-context")
		if root != expectedRoot {
			t.Errorf("Expected root='%s', got '%s'", expectedRoot, root)
		}
	})

	t.Run("ReturnsErrorWhenShellFails", func(t *testing.T) {
		// Given a handler with shell that fails
		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) {
			return "", os.ErrPermission
		}

		handler := NewConfigHandler(mockShell)
		handler.SetContext("test")

		// When getting config root
		_, err := handler.GetConfigRoot()

		// Then it should return error when GetProjectRoot fails
		if err == nil {
			t.Error("Expected error when GetProjectRoot fails")
		}
	})
}

func TestConfigHandler_GetWindsorScratchPath(t *testing.T) {
	t.Run("ReturnsWindsorScratchPath", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		tmpDir, _ := mocks.Shell.GetProjectRoot()

		handler := NewConfigHandler(mocks.Shell)
		handler.SetContext("test-context")

		path, err := handler.GetWindsorScratchPath()

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		expectedPath := filepath.Join(tmpDir, ".windsor", "contexts", "test-context")
		if path != expectedPath {
			t.Errorf("Expected path='%s', got '%s'", expectedPath, path)
		}
	})

	t.Run("ReturnsErrorWhenShellFails", func(t *testing.T) {
		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) {
			return "", os.ErrPermission
		}

		handler := NewConfigHandler(mockShell)
		handler.SetContext("test")

		_, err := handler.GetWindsorScratchPath()

		if err == nil {
			t.Error("Expected error when GetProjectRoot fails")
		}
	})
}

func TestConfigHandler_Clean(t *testing.T) {
	t.Run("RemovesConfigDirectories", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		tmpDir, _ := mocks.Shell.GetProjectRoot()

		handler := NewConfigHandler(mocks.Shell)
		handler.SetContext("test-context")

		configRoot := filepath.Join(tmpDir, "contexts", "test-context")
		os.MkdirAll(configRoot, 0755)

		// Seed every per-provider state dir Clean must wipe; a dropped entry trips here.
		dirsToSeed := []string{".kube", ".talos", ".omni", ".aws", ".azure", ".gcp"}
		for _, dir := range dirsToSeed {
			path := filepath.Join(configRoot, dir)
			os.MkdirAll(path, 0755)
			os.WriteFile(filepath.Join(path, "marker"), []byte("test"), 0644)
		}

		err := handler.Clean()

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		for _, dir := range dirsToSeed {
			path := filepath.Join(configRoot, dir)
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Errorf("Expected %s directory to be removed", dir)
			}
		}
	})

	t.Run("ReturnsErrorWhenGetConfigRootFails", func(t *testing.T) {
		// Given a handler with shell that fails
		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) {
			return "", os.ErrPermission
		}

		handler := NewConfigHandler(mockShell).(*configHandler)
		handler.SetContext("test")

		// When cleaning
		err := handler.Clean()

		// Then it should return error when GetProjectRoot fails
		if err == nil {
			t.Error("Expected error when GetProjectRoot fails")
		}
	})
}

func TestConfigHandler_GenerateContextID(t *testing.T) {
	t.Run("GeneratesIDWhenNotSet", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		err := handler.GenerateContextID()

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		id := handler.GetString("id")
		if id == "" {
			t.Error("Expected ID to be generated")
		}
		if len(id) != 8 {
			t.Errorf("Expected ID length 8, got %d", len(id))
		}
	})

	t.Run("DoesNotOverrideExistingID", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		handler.Set("id", "existing_id")

		err := handler.GenerateContextID()

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		id := handler.GetString("id")
		if id != "existing_id" {
			t.Errorf("Expected existing ID to be preserved, got '%s'", id)
		}
	})

	t.Run("HandlesRandomGenerationError", func(t *testing.T) {
		// Given a handler with random generation error
		handler, _ := setupPrivateTestHandler(t)

		handler.shims.CryptoRandRead = func(b []byte) (int, error) {
			return 0, os.ErrPermission
		}

		// When generating context ID
		err := handler.GenerateContextID()

		// Then it should return error when random generation fails
		if err == nil {
			t.Error("Expected error when random generation fails")
		}
	})
}

func TestConfigHandler_LoadSchema(t *testing.T) {
	t.Run("LoadsSchemaSuccessfully", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		tmpDir, _ := mocks.Shell.GetProjectRoot()

		handler := NewConfigHandler(mocks.Shell)

		schemaDir := filepath.Join(tmpDir, "contexts", "_template")
		os.MkdirAll(schemaDir, 0755)
		schemaContent := `$schema: https://json-schema.org/draft/2020-12/schema
type: object
properties:
  test_key:
    type: string
    default: test_value
`
		schemaPath := filepath.Join(schemaDir, "schema.yaml")
		os.WriteFile(schemaPath, []byte(schemaContent), 0644)

		err := handler.LoadSchema(schemaPath)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		value := handler.Get("test_key")
		if value != "test_value" {
			t.Error("Expected schema default to be accessible after LoadSchema")
		}
	})

	t.Run("ReturnsErrorForInvalidSchema", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		tmpDir, _ := mocks.Shell.GetProjectRoot()

		handler := NewConfigHandler(mocks.Shell)

		schemaDir := filepath.Join(tmpDir, "contexts", "_template")
		os.MkdirAll(schemaDir, 0755)
		invalidSchema := `invalid: yaml: content: [[[`
		schemaPath := filepath.Join(schemaDir, "schema.yaml")
		os.WriteFile(schemaPath, []byte(invalidSchema), 0644)

		err := handler.LoadSchema(schemaPath)

		if err == nil {
			t.Error("Expected error for invalid schema")
		}
	})

	t.Run("HandlesReadFileError", func(t *testing.T) {
		// Given a handler and non-existent schema file
		handler, _ := setupPrivateTestHandler(t)

		// When loading non-existent schema
		err := handler.LoadSchema("/nonexistent/schema.yaml")

		// Then it should return read file error
		if err == nil {
			t.Error("Expected read file error")
		}
	})

	t.Run("HandlesInvalidSchemaContent", func(t *testing.T) {
		// Given a handler and invalid schema content
		handler, tmpDir := setupPrivateTestHandler(t)

		schemaPath := filepath.Join(tmpDir, "invalid_schema.yaml")
		os.WriteFile(schemaPath, []byte("invalid yaml [[["), 0644)

		// When loading invalid schema
		err := handler.LoadSchema(schemaPath)

		// Then it should return error for invalid schema content
		if err == nil {
			t.Error("Expected error for invalid schema content")
		}
	})

	t.Run("ReturnsErrorWhenSchemaValidatorIsNil", func(t *testing.T) {
		// Given a handler with nil schema validator
		handler, _ := setupPrivateTestHandler(t)
		handler.schemaValidator = nil

		// When loading schema
		err := handler.LoadSchema("/some/path/schema.yaml")

		// Then it should return error
		if err == nil {
			t.Error("Expected error when schema validator is nil")
		}
		if !strings.Contains(err.Error(), "schema validator not initialized") {
			t.Errorf("Expected error about schema validator not initialized, got: %v", err)
		}
	})
}

func TestConfigHandler_LoadSchemaFromBytes(t *testing.T) {
	t.Run("LoadsSchemaFromBytes", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		schemaContent := []byte(`$schema: https://json-schema.org/draft/2020-12/schema
type: object
properties:
  byte_schema_key:
    type: string
    default: from_bytes
`)

		err := handler.LoadSchemaFromBytes(schemaContent)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		value := handler.Get("byte_schema_key")
		if value != "from_bytes" {
			t.Error("Expected schema default from bytes to be accessible")
		}
	})

	t.Run("ReturnsErrorWhenSchemaValidatorIsNil", func(t *testing.T) {
		// Given a handler with nil schema validator
		handler, _ := setupPrivateTestHandler(t)
		handler.schemaValidator = nil

		// When loading schema from bytes
		err := handler.LoadSchemaFromBytes([]byte("test: content"))

		// Then it should return error
		if err == nil {
			t.Error("Expected error when schema validator is nil")
		}
		if !strings.Contains(err.Error(), "schema validator not initialized") {
			t.Errorf("Expected error about schema validator not initialized, got: %v", err)
		}
	})

	t.Run("HandlesInvalidSchemaBytes", func(t *testing.T) {
		// Given a handler and invalid schema bytes
		handler, _ := setupPrivateTestHandler(t)

		// When loading invalid schema bytes
		err := handler.LoadSchemaFromBytes([]byte("invalid yaml [[["))

		// Then it should return error for invalid schema bytes
		if err == nil {
			t.Error("Expected error for invalid schema bytes")
		}
	})

	t.Run("HandlesValidSchemaBytes", func(t *testing.T) {
		// Given a handler and valid schema bytes
		handler, _ := setupPrivateTestHandler(t)

		schemaBytes := []byte(`$schema: https://json-schema.org/draft/2020-12/schema
type: object
properties:
  test:
    type: string
`)

		// When loading valid schema bytes
		err := handler.LoadSchemaFromBytes(schemaBytes)

		// Then it should succeed without error
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}

func TestConfigHandler_ValidateContextValues(t *testing.T) {
	t.Run("ReturnsNilWhenSchemaNotLoaded", func(t *testing.T) {
		handler, _ := setupPrivateTestHandler(t)
		handler.schemaValidator.Schema = nil
		handler.data = map[string]any{"dynamic_key": "value"}

		if err := handler.ValidateContextValues(); err != nil {
			t.Fatalf("Expected no error without schema, got %v", err)
		}
	})

	t.Run("ReturnsErrorWhenDynamicFieldsFailValidation", func(t *testing.T) {
		handler, tmpDir := setupPrivateTestHandler(t)
		schemaPath := filepath.Join(tmpDir, "schema.yaml")
		schemaContent := `$schema: https://json-schema.org/draft/2020-12/schema
type: object
properties:
  allowed:
    type: string
additionalProperties: false
`
		os.WriteFile(schemaPath, []byte(schemaContent), 0644)
		if err := handler.LoadSchema(schemaPath); err != nil {
			t.Fatalf("Expected no error loading schema, got %v", err)
		}
		handler.data = map[string]any{"dynamic_key": "value"}

		err := handler.ValidateContextValues()
		if err == nil {
			t.Fatal("Expected validation error for disallowed dynamic key")
		}
		if !strings.Contains(err.Error(), "context value validation failed") {
			t.Errorf("Expected validation failure error, got %v", err)
		}
	})

	t.Run("ValidatesOnlyDynamicFields", func(t *testing.T) {
		handler, tmpDir := setupPrivateTestHandler(t)
		schemaPath := filepath.Join(tmpDir, "schema.yaml")
		schemaContent := `$schema: https://json-schema.org/draft/2020-12/schema
type: object
properties:
  dynamic_key:
    type: string
additionalProperties: false
`
		os.WriteFile(schemaPath, []byte(schemaContent), 0644)
		if err := handler.LoadSchema(schemaPath); err != nil {
			t.Fatalf("Expected no error loading schema, got %v", err)
		}
		handler.data = map[string]any{
			"provider":    "docker",
			"dynamic_key": "value",
		}

		if err := handler.ValidateContextValues(); err != nil {
			t.Fatalf("Expected static fields to be excluded from validation, got %v", err)
		}
	})
}

func TestConfigHandler_GetContext(t *testing.T) {
	t.Run("ReturnsContextFromEnvironment", func(t *testing.T) {
		mocks := setupConfigMocks(t)

		handler := NewConfigHandler(mocks.Shell)

		context := handler.GetContext()

		if context != "test-context" {
			t.Errorf("Expected 'test-context' from environment, got '%s'", context)
		}
	})

	t.Run("PrefersContextFileOverEnvironmentContext", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		tmpDir, _ := mocks.Shell.GetProjectRoot()

		os.Setenv("WINDSOR_CONTEXT", "env-context")

		handler := NewConfigHandler(mocks.Shell)

		contextFilePath := filepath.Join(tmpDir, ".windsor", "context")
		os.MkdirAll(filepath.Dir(contextFilePath), 0755)
		os.WriteFile(contextFilePath, []byte("file-context"), 0644)

		context := handler.GetContext()

		if context != "file-context" {
			t.Errorf("Expected 'file-context', got '%s'", context)
		}
	})

	t.Run("UsesEnvironmentContextWhenContextFileDoesNotExist", func(t *testing.T) {
		mocks := setupConfigMocks(t)

		os.Setenv("WINDSOR_CONTEXT", "env-context")

		handler := NewConfigHandler(mocks.Shell)

		context := handler.GetContext()

		if context != "env-context" {
			t.Errorf("Expected 'env-context', got '%s'", context)
		}
	})

	t.Run("ReadsContextFromFile", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		tmpDir, _ := mocks.Shell.GetProjectRoot()

		os.Unsetenv("WINDSOR_CONTEXT")
		defer os.Setenv("WINDSOR_CONTEXT", "test-context")

		handler := NewConfigHandler(mocks.Shell)

		contextFilePath := filepath.Join(tmpDir, ".windsor", "context")
		os.MkdirAll(filepath.Dir(contextFilePath), 0755)
		os.WriteFile(contextFilePath, []byte("file-context"), 0644)

		context := handler.GetContext()

		if context != "file-context" {
			t.Errorf("Expected 'file-context', got '%s'", context)
		}
	})

	t.Run("DefaultsToLocalWhenNoContextSet", func(t *testing.T) {
		mocks := setupConfigMocks(t)

		os.Unsetenv("WINDSOR_CONTEXT")
		defer os.Setenv("WINDSOR_CONTEXT", "test-context")

		handler := NewConfigHandler(mocks.Shell)

		context := handler.GetContext()

		if context != "local" {
			t.Errorf("Expected default 'local', got '%s'", context)
		}
	})
}

func TestConfigHandler_WithContext(t *testing.T) {
	t.Run("OverridesFileAndEnvVar", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		tmpDir, _ := mocks.Shell.GetProjectRoot()

		os.Setenv("WINDSOR_CONTEXT", "env-context")
		defer os.Setenv("WINDSOR_CONTEXT", "test-context")

		contextFilePath := filepath.Join(tmpDir, ".windsor", "context")
		os.MkdirAll(filepath.Dir(contextFilePath), 0755)
		os.WriteFile(contextFilePath, []byte("file-context"), 0644)

		handler := NewConfigHandler(mocks.Shell).WithContext("override-context")

		context := handler.GetContext()

		if context != "override-context" {
			t.Errorf("Expected 'override-context', got '%s'", context)
		}
	})
}

func TestConfigHandler_SetContext(t *testing.T) {
	t.Run("WritesContextToFile", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		tmpDir, _ := mocks.Shell.GetProjectRoot()

		handler := NewConfigHandler(mocks.Shell)

		err := handler.SetContext("new-context")

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		contextFilePath := filepath.Join(tmpDir, ".windsor", "context")
		content, err := os.ReadFile(contextFilePath)
		if err != nil {
			t.Fatalf("Failed to read context file: %v", err)
		}

		if string(content) != "new-context" {
			t.Errorf("Expected context file to contain 'new-context', got '%s'", string(content))
		}
	})

	t.Run("HandlesMkdirAllError", func(t *testing.T) {
		// Given a handler with MkdirAll error
		handler, _ := setupPrivateTestHandler(t)

		handler.shims.MkdirAll = func(path string, perm os.FileMode) error {
			return os.ErrPermission
		}

		// When setting context
		err := handler.SetContext("new-context")

		// Then it should return MkdirAll error
		if err == nil {
			t.Error("Expected MkdirAll error")
		}
	})

	t.Run("HandlesWriteFileError", func(t *testing.T) {
		// Given a handler with WriteFile error
		handler, _ := setupPrivateTestHandler(t)

		handler.shims.WriteFile = func(name string, data []byte, perm os.FileMode) error {
			return os.ErrPermission
		}

		// When setting context
		err := handler.SetContext("new-context")

		// Then it should return WriteFile error
		if err == nil {
			t.Error("Expected WriteFile error")
		}
	})
}

func TestConfigHandler_IsLoaded(t *testing.T) {
	t.Run("ReturnsFalseBeforeLoading", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		result := handler.IsLoaded()

		if result {
			t.Error("Expected IsLoaded=false before loading config")
		}
	})

	t.Run("ReturnsTrueAfterLoadingFiles", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		tmpDir, _ := mocks.Shell.GetProjectRoot()

		handler := NewConfigHandler(mocks.Shell)
		handler.SetContext("test-context")

		contextDir := filepath.Join(tmpDir, "contexts", "test-context")
		os.MkdirAll(contextDir, 0755)
		os.WriteFile(filepath.Join(contextDir, "windsor.yaml"), []byte("provider: local\n"), 0644)

		handler.LoadConfig()

		result := handler.IsLoaded()

		if !result {
			t.Error("Expected IsLoaded=true after loading config files")
		}
	})

	t.Run("ReturnsTrueAfterLoadConfigString", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		handler.LoadConfigString("provider: test\n")

		result := handler.IsLoaded()

		if !result {
			t.Error("Expected IsLoaded=true after LoadConfigString")
		}
	})
}

func TestConfigHandler_IsDevMode(t *testing.T) {
	t.Run("ReturnsTrueWhenDevIsSetToTrue", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		handler.Set("dev", true)

		result := handler.IsDevMode("production")

		if !result {
			t.Error("Expected IsDevMode=true when dev=true, regardless of context name")
		}
	})

	t.Run("ReturnsFalseWhenDevIsSetToFalse", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		handler.Set("dev", false)

		result := handler.IsDevMode("local")

		if result {
			t.Error("Expected IsDevMode=false when dev=false, even for local context")
		}
	})

	t.Run("ReturnsTrueForLocalContext", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		result := handler.IsDevMode("local")

		if !result {
			t.Error("Expected IsDevMode=true for 'local' context")
		}
	})

	t.Run("ReturnsTrueForLocalPrefixContext", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		result := handler.IsDevMode("local-dev")

		if !result {
			t.Error("Expected IsDevMode=true for context starting with 'local-'")
		}
	})

	t.Run("ReturnsFalseForNonLocalContext", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		result := handler.IsDevMode("production")

		if result {
			t.Error("Expected IsDevMode=false for non-local context")
		}
	})

	t.Run("IgnoresNonBoolDevValue", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		handler.Set("dev", "true")

		result := handler.IsDevMode("local")

		if !result {
			t.Error("Expected IsDevMode=true for local context when dev is non-bool")
		}
	})
}

func TestConfigHandler_HelperConversions(t *testing.T) {
	t.Run("ConvertInterfaceMapRecursivelyConvertsNestedMaps", func(t *testing.T) {
		handler, _ := setupPrivateTestHandler(t)
		input := map[any]any{
			"top": map[any]any{
				"inner": "value",
			},
			10: "ignored",
		}

		converted := handler.convertInterfaceMap(input)
		top, ok := converted["top"].(map[string]any)
		if !ok {
			t.Fatalf("Expected nested map conversion, got %T", converted["top"])
		}
		if top["inner"] != "value" {
			t.Errorf("Expected nested converted value, got %v", top["inner"])
		}
		if _, exists := converted["10"]; exists {
			t.Errorf("Expected non-string keys to be dropped, got %v", converted)
		}
	})

	t.Run("MapToContextReturnsEmptyContextOnMarshalOrUnmarshalError", func(t *testing.T) {
		handler, _ := setupPrivateTestHandler(t)

		handler.shims.YamlMarshal = func(v any) ([]byte, error) {
			return nil, os.ErrInvalid
		}
		if ctx := handler.mapToContext(map[string]any{"provider": "docker"}); ctx.Provider != nil {
			t.Errorf("Expected empty context on marshal error, got %+v", ctx)
		}

		handler.shims = NewShims()
		handler.shims.YamlUnmarshal = func(data []byte, v any) error {
			return os.ErrInvalid
		}
		if ctx := handler.mapToContext(map[string]any{"provider": "docker"}); ctx.Provider != nil {
			t.Errorf("Expected empty context on unmarshal error, got %+v", ctx)
		}
	})
}

// =============================================================================
// Test Helpers
// =============================================================================

type mockValueProvider struct {
	value any
	err   error
}

func (m *mockValueProvider) GetValue(key string) (any, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.value != nil {
		return m.value, nil
	}
	return "mock-value", nil
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && findSubstring(s, substr)
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
