package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// =============================================================================
// Test Public Methods
// =============================================================================

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
		if handler.GetString("platform") != "from_root" {
			t.Errorf("Expected platform from root config, got %v", handler.GetString("platform"))
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
		if handler.GetString("platform") != "from_yml" {
			t.Errorf("Expected platform from .yml file, got %v", handler.GetString("platform"))
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

		platform := handler.GetString("platform")
		if platform != "local" {
			t.Errorf("Expected platform='local', got '%s'", platform)
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

		platform := handler.GetString("platform")
		if platform != "docker" {
			t.Errorf("Expected platform='docker', got '%s'", platform)
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
    platform: local
    dns:
      domain: test
  other-context:
    platform: remote
`

		err := handler.LoadConfigString(yaml)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		platform := handler.GetString("platform")
		if platform != "local" {
			t.Errorf("Expected platform='local' from test-context, got '%s'", platform)
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

		content := `platform: top_level
contexts:
  test-context:
    platform: from_context
`

		if err := handler.LoadConfigString(content); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if platform := handler.GetString("platform"); platform != "from_context" {
			t.Errorf("Expected context platform to win, got '%s'", platform)
		}
	})

	t.Run("MergesFlatYamlStructure", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		yaml := `platform: docker
custom_key: custom_value
`

		err := handler.LoadConfigString(yaml)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		platform := handler.GetString("platform")
		if platform != "docker" {
			t.Errorf("Expected platform='docker', got '%s'", platform)
		}

		customKey := handler.GetString("custom_key")
		if customKey != "custom_value" {
			t.Errorf("Expected custom_key='custom_value', got '%s'", customKey)
		}
	})

	t.Run("SetsLoadedFlag", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		err := handler.LoadConfigString("platform: test\n")

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
    platform: local
    custom: value
`

		err := handler.LoadConfigString(yaml)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		platform := handler.GetString("platform")
		if platform != "local" {
			t.Errorf("Expected platform='local', got '%s'", platform)
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
    platform: local
    nested:
      key: value
`

		err := handler.LoadConfigString(yaml)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		platform := handler.GetString("platform")
		if platform != "local" {
			t.Errorf("Expected platform='local', got '%s'", platform)
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
    platform: local
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
		if !contains(string(valuesContent), "custom_dynamic_field: dynamic_value") {
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

		// LoadConfig first, as every real caller does, so existing_key carries into
		// c.data. A save that never loaded it would look like a drop otherwise.
		if err := handler.LoadConfig(); err != nil {
			t.Fatalf("Expected no error loading config, got %v", err)
		}
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

	t.Run("ClearsPendingWritesAfterASuccessfulSaveSoALaterSaveDoesNotReapplyAStaleDelete", func(t *testing.T) {
		handler, tmpDir := setupPrivateTestHandler(t)
		handler.SetContext("test-context")
		contextDir := filepath.Join(tmpDir, "contexts", "test-context")
		os.MkdirAll(contextDir, 0755)
		valuesPath := filepath.Join(contextDir, "values.yaml")
		os.WriteFile(valuesPath, []byte("provider: docker\n"), 0644)

		if err := handler.LoadConfig(); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		handler.Set("provider", nil)
		if err := handler.SaveConfig(true); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		firstSave, _ := os.ReadFile(valuesPath)
		if contains(string(firstSave), "provider:") {
			t.Fatalf("Expected provider removed by the first save, got:\n%s", string(firstSave))
		}

		// Something else legitimately writes provider back afterward. A later save,
		// triggered by an unrelated Set, must not silently strip it again just
		// because it was deleted once, earlier in this handler's life.
		os.WriteFile(valuesPath, []byte("provider: docker\nid: abc123\n"), 0644)
		handler.Set("id", "abc123")
		if err := handler.SaveConfig(true); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		secondSave, _ := os.ReadFile(valuesPath)
		if !contains(string(secondSave), "provider: docker") {
			t.Errorf("Expected the stale delete not to reapply on a later save, got:\n%s", string(secondSave))
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
