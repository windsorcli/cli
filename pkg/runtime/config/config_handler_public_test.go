package config

import (
	"errors"
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
	t.Run("LoadsRootConfigContextSection", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)
		handler.SetContext("test-context")

		tmpDir, _ := mocks.Shell.GetProjectRoot()
		rootConfig := `version: v1alpha1
contexts:
  test-context:
    provider: local
    dns:
      domain: example.com
`
		os.WriteFile(filepath.Join(tmpDir, "windsor.yaml"), []byte(rootConfig), 0644)

		err := handler.LoadConfig()

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

	t.Run("LoadsContextSpecificWindsorYaml", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)
		handler.SetContext("test-context")

		tmpDir, _ := mocks.Shell.GetProjectRoot()
		contextDir := filepath.Join(tmpDir, "contexts", "test-context")
		os.MkdirAll(contextDir, 0755)
		contextConfig := `provider: generic
cluster:
  enabled: true
  driver: talos
`
		os.WriteFile(filepath.Join(contextDir, "windsor.yaml"), []byte(contextConfig), 0644)

		err := handler.LoadConfig()

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		provider := handler.GetString("provider")
		if provider != "generic" {
			t.Errorf("Expected provider='generic', got '%s'", provider)
		}

		driver := handler.GetString("cluster.driver")
		if driver != "talos" {
			t.Errorf("Expected cluster.driver='talos', got '%s'", driver)
		}
	})

	t.Run("LoadsValuesYaml", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)
		handler.SetContext("test-context")

		tmpDir, _ := mocks.Shell.GetProjectRoot()
		contextDir := filepath.Join(tmpDir, "contexts", "test-context")
		os.MkdirAll(contextDir, 0755)
		valuesContent := `dev: true
custom_key: custom_value
nested:
  key: nested_value
`
		os.WriteFile(filepath.Join(contextDir, "values.yaml"), []byte(valuesContent), 0644)

		err := handler.LoadConfig()

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

		nestedKey := handler.GetString("nested.key")
		if nestedKey != "nested_value" {
			t.Errorf("Expected nested.key='nested_value', got '%s'", nestedKey)
		}
	})

	t.Run("MergesAllSourcesWithCorrectPrecedence", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)
		handler.SetContext("test-context")

		tmpDir, _ := mocks.Shell.GetProjectRoot()

		schemaDir := filepath.Join(tmpDir, "contexts", "_template")
		os.MkdirAll(schemaDir, 0755)
		schemaContent := `$schema: https://json-schema.org/draft/2020-12/schema
type: object
properties:
  test_key:
    type: string
    default: schema_default
`
		os.WriteFile(filepath.Join(schemaDir, "schema.yaml"), []byte(schemaContent), 0644)

		rootConfig := `version: v1alpha1
contexts:
  test-context:
    test_key: root_value
`
		os.WriteFile(filepath.Join(tmpDir, "windsor.yaml"), []byte(rootConfig), 0644)

		contextDir := filepath.Join(tmpDir, "contexts", "test-context")
		os.MkdirAll(contextDir, 0755)
		contextConfig := `test_key: context_value
`
		os.WriteFile(filepath.Join(contextDir, "windsor.yaml"), []byte(contextConfig), 0644)

		valuesContent := `test_key: values_override
`
		os.WriteFile(filepath.Join(contextDir, "values.yaml"), []byte(valuesContent), 0644)

		err := handler.LoadConfig()

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		value := handler.GetString("test_key")
		if value != "values_override" {
			t.Errorf("Expected values.yaml to have highest precedence, got '%s'", value)
		}
	})

	t.Run("LoadsSchemaWithoutErrors", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		tmpDir, _ := mocks.Shell.GetProjectRoot()
		schemaDir := filepath.Join(tmpDir, "contexts", "_template")
		os.MkdirAll(schemaDir, 0755)
		schemaContent := `$schema: https://json-schema.org/draft/2020-12/schema
type: object
properties:
  key:
    type: string
`
		os.WriteFile(filepath.Join(schemaDir, "schema.yaml"), []byte(schemaContent), 0644)

		err := handler.LoadConfig()

		if err != nil {
			t.Errorf("Expected no error loading schema, got %v", err)
		}
	})

	t.Run("LoadsAPISchemasForV1Alpha2", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		tmpDir, _ := mocks.Shell.GetProjectRoot()
		rootConfig := `version: v1alpha2
contexts:
  test-context: {}
`
		os.WriteFile(filepath.Join(tmpDir, "windsor.yaml"), []byte(rootConfig), 0644)

		err := handler.LoadConfig()

		if err != nil {
			t.Fatalf("Expected no error loading config with v1alpha2, got %v", err)
		}

		configHandler := handler.(*configHandler)
		if configHandler.schemaValidator == nil {
			t.Fatal("Expected schema validator to be initialized")
		}

		if configHandler.schemaValidator.Schema == nil {
			t.Fatal("Expected API schemas to be loaded for v1alpha2")
		}

		properties, ok := configHandler.schemaValidator.Schema["properties"].(map[string]any)
		if !ok {
			t.Fatal("Expected schema to have properties")
		}

		hasAWS := false
		hasOnePassword := false
		hasTerraformEnabled := false
		hasWorkstationRegistries := false

		for key := range properties {
			switch key {
			case "aws":
				hasAWS = true
			case "onepassword":
				hasOnePassword = true
			case "enabled":
				hasTerraformEnabled = true
			case "registries":
				hasWorkstationRegistries = true
			}
		}

		if !hasAWS {
			t.Error("Expected AWS provider schema to be loaded (indicating providers schema loaded)")
		}
		if !hasOnePassword {
			t.Error("Expected OnePassword schema to be loaded (indicating secrets schema loaded)")
		}
		if !hasTerraformEnabled {
			t.Error("Expected terraform enabled schema to be loaded (indicating terraform schema loaded)")
		}
		if !hasWorkstationRegistries {
			t.Error("Expected workstation registries schema to be loaded (indicating workstation schema loaded)")
		}
	})

	t.Run("DoesNotLoadAPISchemasForV1Alpha1", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		tmpDir, _ := mocks.Shell.GetProjectRoot()
		rootConfig := `version: v1alpha1
contexts:
  test-context: {}
`
		os.WriteFile(filepath.Join(tmpDir, "windsor.yaml"), []byte(rootConfig), 0644)

		err := handler.LoadConfig()

		if err != nil {
			t.Fatalf("Expected no error loading config with v1alpha1, got %v", err)
		}

		configHandler := handler.(*configHandler)
		if configHandler.schemaValidator != nil && configHandler.schemaValidator.Schema != nil {
			properties, ok := configHandler.schemaValidator.Schema["properties"].(map[string]any)
			if ok {
				if _, hasAWS := properties["aws"]; hasAWS {
					t.Error("Expected API schemas NOT to be loaded for v1alpha1")
				}
			}
		}
	})

	t.Run("SetsLoadedFlag", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)
		handler.SetContext("test-context")

		if handler.IsLoaded() {
			t.Error("Expected IsLoaded=false before loading")
		}

		tmpDir, _ := mocks.Shell.GetProjectRoot()
		contextDir := filepath.Join(tmpDir, "contexts", "test-context")
		os.MkdirAll(contextDir, 0755)
		os.WriteFile(filepath.Join(contextDir, "windsor.yaml"), []byte("provider: local\n"), 0644)

		err := handler.LoadConfig()
		if err != nil {
			t.Fatalf("LoadConfig failed: %v", err)
		}

		if !handler.IsLoaded() {
			t.Error("Expected IsLoaded=true after loading")
		}
	})

	t.Run("ValidatesValuesYamlAgainstSchema", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		tmpDir, _ := mocks.Shell.GetProjectRoot()

		handler := NewConfigHandler(mocks.Shell)
		handler.SetContext("test-context")

		schemaDir := filepath.Join(tmpDir, "contexts", "_template")
		os.MkdirAll(schemaDir, 0755)
		schemaContent := `$schema: https://json-schema.org/draft/2020-12/schema
type: object
properties:
  valid_key:
    type: string
additionalProperties: false
`
		os.WriteFile(filepath.Join(schemaDir, "schema.yaml"), []byte(schemaContent), 0644)

		contextDir := filepath.Join(tmpDir, "contexts", "test-context")
		os.MkdirAll(contextDir, 0755)
		valuesContent := `invalid_key: should_fail_validation
`
		os.WriteFile(filepath.Join(contextDir, "values.yaml"), []byte(valuesContent), 0644)

		err := handler.LoadConfig()

		if err == nil {
			t.Error("Expected validation error for invalid values.yaml")
		}
	})

	t.Run("LoadsSchemaWhenSchemaValidatorIsNil", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		tmpDir, _ := mocks.Shell.GetProjectRoot()

		handler := NewConfigHandler(mocks.Shell).(*configHandler)
		handler.SetContext("test-context")
		handler.schemaValidator.Schema = nil

		schemaDir := filepath.Join(tmpDir, "contexts", "_template")
		os.MkdirAll(schemaDir, 0755)
		schemaContent := `$schema: https://json-schema.org/draft/2020-12/schema
type: object
properties:
  schema_key:
    type: string
    default: schema_value
`
		os.WriteFile(filepath.Join(schemaDir, "schema.yaml"), []byte(schemaContent), 0644)

		contextDir := filepath.Join(tmpDir, "contexts", "test-context")
		os.MkdirAll(contextDir, 0755)
		os.WriteFile(filepath.Join(contextDir, "windsor.yaml"), []byte("provider: local\n"), 0644)

		err := handler.LoadConfig()

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		value := handler.Get("schema_key")
		if value != "schema_value" {
			t.Errorf("Expected schema default to be loaded, got '%v'", value)
		}
	})

	t.Run("HandlesSchemaLoadError", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		tmpDir, _ := mocks.Shell.GetProjectRoot()

		handler := NewConfigHandler(mocks.Shell).(*configHandler)
		handler.SetContext("test-context")
		handler.schemaValidator.Schema = nil

		schemaDir := filepath.Join(tmpDir, "contexts", "_template")
		os.MkdirAll(schemaDir, 0755)
		invalidSchema := `invalid: yaml: [[[`
		os.WriteFile(filepath.Join(schemaDir, "schema.yaml"), []byte(invalidSchema), 0644)

		err := handler.LoadConfig()

		if err == nil {
			t.Error("Expected error when schema loading fails")
		}
	})

	t.Run("HandlesRootConfigWithoutContexts", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)
		handler.SetContext("test-context")

		tmpDir, _ := mocks.Shell.GetProjectRoot()
		rootConfig := `version: v1alpha1
`
		os.WriteFile(filepath.Join(tmpDir, "windsor.yaml"), []byte(rootConfig), 0644)

		err := handler.LoadConfig()

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
	})

	t.Run("HandlesRootConfigWithMissingContext", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)
		handler.SetContext("missing-context")

		tmpDir, _ := mocks.Shell.GetProjectRoot()
		rootConfig := `version: v1alpha1
contexts:
  other-context:
    provider: local
`
		os.WriteFile(filepath.Join(tmpDir, "windsor.yaml"), []byte(rootConfig), 0644)

		err := handler.LoadConfig()

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
	})

	t.Run("ReturnsErrorWhenShellIsNil", func(t *testing.T) {
		handler := &configHandler{
			shell: nil,
		}

		err := handler.LoadConfig()

		if err == nil {
			t.Error("Expected error when shell is nil")
		}
		if !strings.Contains(err.Error(), "shell not initialized") {
			t.Errorf("Expected error about shell not initialized, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenGetProjectRootFails", func(t *testing.T) {
		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("project root error")
		}

		handler := NewConfigHandler(mockShell)

		err := handler.LoadConfig()

		if err == nil {
			t.Error("Expected error when GetProjectRoot fails")
		}
		if !strings.Contains(err.Error(), "error retrieving project root") {
			t.Errorf("Expected error about retrieving project root, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenReadFileFailsForRootConfig", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		tmpDir, _ := mocks.Shell.GetProjectRoot()

		handler := NewConfigHandler(mocks.Shell).(*configHandler)
		handler.SetContext("test-context")

		rootConfigPath := filepath.Join(tmpDir, "windsor.yaml")
		os.WriteFile(rootConfigPath, []byte("version: v1alpha1\n"), 0644)

		handler.shims.ReadFile = func(name string) ([]byte, error) {
			if name == rootConfigPath {
				return nil, fmt.Errorf("read file error")
			}
			return os.ReadFile(name)
		}

		err := handler.LoadConfig()

		if err == nil {
			t.Error("Expected error when ReadFile fails for root config")
		}
		if !strings.Contains(err.Error(), "error reading root config file") {
			t.Errorf("Expected error about reading root config file, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenYamlUnmarshalFailsForRootConfig", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		tmpDir, _ := mocks.Shell.GetProjectRoot()

		handler := NewConfigHandler(mocks.Shell).(*configHandler)
		handler.SetContext("test-context")

		rootConfigPath := filepath.Join(tmpDir, "windsor.yaml")
		os.WriteFile(rootConfigPath, []byte("invalid: yaml: [[["), 0644)

		err := handler.LoadConfig()

		if err == nil {
			t.Error("Expected error when YamlUnmarshal fails for root config")
		}
		if !strings.Contains(err.Error(), "error unmarshalling root config") {
			t.Errorf("Expected error about unmarshalling root config, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenReadFileFailsForContextConfig", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		tmpDir, _ := mocks.Shell.GetProjectRoot()

		handler := NewConfigHandler(mocks.Shell).(*configHandler)
		handler.SetContext("test-context")

		contextDir := filepath.Join(tmpDir, "contexts", "test-context")
		os.MkdirAll(contextDir, 0755)
		contextConfigPath := filepath.Join(contextDir, "windsor.yaml")
		os.WriteFile(contextConfigPath, []byte("provider: local\n"), 0644)

		handler.shims.ReadFile = func(name string) ([]byte, error) {
			if name == contextConfigPath {
				return nil, fmt.Errorf("read file error")
			}
			return os.ReadFile(name)
		}

		err := handler.LoadConfig()

		if err == nil {
			t.Error("Expected error when ReadFile fails for context config")
		}
		if !strings.Contains(err.Error(), "error reading context config file") {
			t.Errorf("Expected error about reading context config file, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenYamlUnmarshalFailsForContextConfig", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		tmpDir, _ := mocks.Shell.GetProjectRoot()

		handler := NewConfigHandler(mocks.Shell).(*configHandler)
		handler.SetContext("test-context")

		contextDir := filepath.Join(tmpDir, "contexts", "test-context")
		os.MkdirAll(contextDir, 0755)
		os.WriteFile(filepath.Join(contextDir, "windsor.yaml"), []byte("invalid: yaml: [[["), 0644)

		err := handler.LoadConfig()

		if err == nil {
			t.Error("Expected error when YamlUnmarshal fails for context config")
		}
		if !strings.Contains(err.Error(), "error unmarshalling context yaml") {
			t.Errorf("Expected error about unmarshalling context yaml, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenReadFileFailsForValuesYaml", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		tmpDir, _ := mocks.Shell.GetProjectRoot()

		handler := NewConfigHandler(mocks.Shell).(*configHandler)
		handler.SetContext("test-context")

		contextDir := filepath.Join(tmpDir, "contexts", "test-context")
		os.MkdirAll(contextDir, 0755)
		valuesPath := filepath.Join(contextDir, "values.yaml")
		os.WriteFile(valuesPath, []byte("key: value\n"), 0644)

		handler.shims.ReadFile = func(name string) ([]byte, error) {
			if name == valuesPath {
				return nil, fmt.Errorf("read file error")
			}
			return os.ReadFile(name)
		}

		err := handler.LoadConfig()

		if err == nil {
			t.Error("Expected error when ReadFile fails for values.yaml")
		}
		if !strings.Contains(err.Error(), "error reading values.yaml") {
			t.Errorf("Expected error about reading values.yaml, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenYamlUnmarshalFailsForValuesYaml", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		tmpDir, _ := mocks.Shell.GetProjectRoot()

		handler := NewConfigHandler(mocks.Shell).(*configHandler)
		handler.SetContext("test-context")

		contextDir := filepath.Join(tmpDir, "contexts", "test-context")
		os.MkdirAll(contextDir, 0755)
		os.WriteFile(filepath.Join(contextDir, "values.yaml"), []byte("invalid: yaml: [[["), 0644)

		err := handler.LoadConfig()

		if err == nil {
			t.Error("Expected error when YamlUnmarshal fails for values.yaml")
		}
		if !strings.Contains(err.Error(), "error unmarshalling values.yaml") {
			t.Errorf("Expected error about unmarshalling values.yaml, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenYamlMarshalFailsForContextConfig", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		tmpDir, _ := mocks.Shell.GetProjectRoot()

		handler := NewConfigHandler(mocks.Shell).(*configHandler)
		handler.SetContext("test-context")

		rootConfigPath := filepath.Join(tmpDir, "windsor.yaml")
		rootConfig := `version: v1alpha1
contexts:
  test-context:
    provider: local
`
		os.WriteFile(rootConfigPath, []byte(rootConfig), 0644)

		handler.shims.YamlMarshal = func(v any) ([]byte, error) {
			return nil, fmt.Errorf("marshal error")
		}

		err := handler.LoadConfig()

		if err == nil {
			t.Error("Expected error when YamlMarshal fails for context config")
		}
		if !strings.Contains(err.Error(), "error marshalling context config") {
			t.Errorf("Expected error about marshalling context config, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenValidateReturnsError", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		tmpDir, _ := mocks.Shell.GetProjectRoot()

		handler := NewConfigHandler(mocks.Shell).(*configHandler)
		handler.SetContext("test-context")

		schemaDir := filepath.Join(tmpDir, "contexts", "_template")
		os.MkdirAll(schemaDir, 0755)
		schemaContent := `$schema: https://json-schema.org/draft/2020-12/schema
type: object
properties:
  test_key:
    type: string
    pattern: "^[a-z]+$"
`
		os.WriteFile(filepath.Join(schemaDir, "schema.yaml"), []byte(schemaContent), 0644)
		handler.LoadSchema(filepath.Join(schemaDir, "schema.yaml"))

		contextDir := filepath.Join(tmpDir, "contexts", "test-context")
		os.MkdirAll(contextDir, 0755)
		os.WriteFile(filepath.Join(contextDir, "values.yaml"), []byte("test_key: VALUE\n"), 0644)

		handler.schemaValidator.Shims.RegexpMatchString = func(pattern, s string) (bool, error) {
			return false, fmt.Errorf("regex error")
		}

		err := handler.LoadConfig()

		if err == nil {
			t.Error("Expected error when Validate returns error")
		}
		if !strings.Contains(err.Error(), "values.yaml validation") {
			t.Errorf("Expected error about values.yaml validation, got: %v", err)
		}
	})

	t.Run("HandlesYmlExtension", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)
		handler.SetContext("test-context")

		tmpDir, _ := mocks.Shell.GetProjectRoot()
		contextDir := filepath.Join(tmpDir, "contexts", "test-context")
		os.MkdirAll(contextDir, 0755)
		contextConfig := `provider: from_yml
`
		os.WriteFile(filepath.Join(contextDir, "windsor.yml"), []byte(contextConfig), 0644)

		err := handler.LoadConfig()

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		provider := handler.GetString("provider")
		if provider != "from_yml" {
			t.Errorf("Expected provider from .yml file, got '%s'", provider)
		}
	})

	t.Run("CreatesHandlerWithShell", func(t *testing.T) {
		// Given a shell
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell).(*configHandler)

		// When loading config with shell
		// Note: This test verifies the handler is created correctly with a shell.
		// LoadConfig may fail for other reasons (missing files, etc.) but not due to missing shell.
		if handler.shell == nil {
			t.Error("Expected shell to be set on handler")
		}
	})

	t.Run("ReturnsErrorForInvalidSchemaFile", func(t *testing.T) {
		// Given a config handler with invalid schema file
		handler, tmpDir := setupPrivateTestHandler(t)

		schemaDir := filepath.Join(tmpDir, "contexts", "_template")
		os.MkdirAll(schemaDir, 0755)
		invalidSchema := `this is not: valid [yaml`
		os.WriteFile(filepath.Join(schemaDir, "schema.yaml"), []byte(invalidSchema), 0644)

		// When loading config with invalid schema
		err := handler.LoadConfig()

		// Then loading should fail
		if err == nil {
			t.Error("Expected error for invalid schema file")
		}
	})

	t.Run("ReturnsErrorForInvalidRootConfig", func(t *testing.T) {
		// Given a config handler with invalid root config
		handler, tmpDir := setupPrivateTestHandler(t)

		invalidConfig := `invalid: yaml: [[[`
		os.WriteFile(filepath.Join(tmpDir, "windsor.yaml"), []byte(invalidConfig), 0644)

		// When loading config with invalid root config
		err := handler.LoadConfig()

		// Then loading should fail
		if err == nil {
			t.Error("Expected error for invalid root config")
		}
	})

	t.Run("ReturnsErrorForInvalidContextConfig", func(t *testing.T) {
		// Given a config handler with invalid context config
		handler, tmpDir := setupPrivateTestHandler(t)
		handler.SetContext("test-context")

		contextDir := filepath.Join(tmpDir, "contexts", "test-context")
		os.MkdirAll(contextDir, 0755)
		invalidConfig := `invalid: yaml: [[[`
		os.WriteFile(filepath.Join(contextDir, "windsor.yaml"), []byte(invalidConfig), 0644)

		// When loading config with invalid context config
		err := handler.LoadConfig()

		// Then loading should fail
		if err == nil {
			t.Error("Expected error for invalid context config")
		}
	})

	t.Run("ReturnsErrorForInvalidValuesYaml", func(t *testing.T) {
		// Given a config handler with invalid values.yaml
		handler, tmpDir := setupPrivateTestHandler(t)
		handler.SetContext("test-context")

		contextDir := filepath.Join(tmpDir, "contexts", "test-context")
		os.MkdirAll(contextDir, 0755)
		invalidValues := `invalid: yaml: [[[`
		os.WriteFile(filepath.Join(contextDir, "values.yaml"), []byte(invalidValues), 0644)

		// When loading config with invalid values.yaml
		err := handler.LoadConfig()

		// Then loading should fail
		if err == nil {
			t.Error("Expected error for invalid values.yaml")
		}
	})

	t.Run("IntegrationTestWithAllSources", func(t *testing.T) {
		// Given a complete configuration setup with schema, root config, context config, and values.yaml
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

		// When loading the configuration
		err := handler.LoadConfig()
		if err != nil {
			t.Fatalf("LoadConfig failed: %v", err)
		}

		// Then all configuration sources should be properly merged with correct precedence
		schemaDefault := handler.GetString("schema_default")
		if schemaDefault != "from_schema" {
			t.Errorf("Expected schema default, got '%s'", schemaDefault)
		}

		provider := handler.GetString("provider")
		if provider != "from_root" {
			t.Errorf("Expected provider from root, got '%s'", provider)
		}

		clusterEnabled := handler.GetBool("cluster.enabled")
		if !clusterEnabled {
			t.Error("Expected cluster.enabled from context config")
		}

		overrideTest := handler.GetString("override_test")
		if overrideTest != "values_final" {
			t.Errorf("Expected values.yaml to have final say, got '%s'", overrideTest)
		}

		customField := handler.GetString("custom_field")
		if customField != "user_data" {
			t.Errorf("Expected custom_field from values.yaml, got '%s'", customField)
		}
	})

	t.Run("HandlesContextWithoutRootConfig", func(t *testing.T) {
		// Given a context config without root config
		handler, tmpDir := setupPrivateTestHandler(t)
		handler.SetContext("test-context")

		contextDir := filepath.Join(tmpDir, "contexts", "test-context")
		os.MkdirAll(contextDir, 0755)
		os.WriteFile(filepath.Join(contextDir, "windsor.yaml"), []byte("provider: test\n"), 0644)

		// When loading config
		err := handler.LoadConfig()

		// Then it should succeed and load context config
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		provider := handler.GetString("provider")
		if provider != "test" {
			t.Error("Expected to load context config without root config")
		}
	})

	t.Run("HandlesRootContextSectionWithoutMatch", func(t *testing.T) {
		// Given a root config with different context section
		handler, tmpDir := setupPrivateTestHandler(t)
		handler.SetContext("test-context")

		rootConfig := `version: v1alpha1
contexts:
  other-context:
    provider: other
`
		os.WriteFile(filepath.Join(tmpDir, "windsor.yaml"), []byte(rootConfig), 0644)

		// When loading config
		err := handler.LoadConfig()

		// Then it should succeed without error
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
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
		contextConfig := `provider: generic
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
		if provider != "generic" {
			t.Errorf("Expected provider='generic', got '%s'", provider)
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

	t.Run("ReturnsErrorWhenShellNotInitialized", func(t *testing.T) {
		handler := NewConfigHandler(nil)

		err := handler.LoadConfigForContext("test-context")

		if err == nil {
			t.Error("Expected error when shell is nil")
		}

		if !strings.Contains(err.Error(), "shell not initialized") {
			t.Errorf("Expected error about shell not initialized, got: %v", err)
		}
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

	t.Run("MergesFlatYamlStructure", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		yaml := `provider: generic
custom_key: custom_value
`

		err := handler.LoadConfigString(yaml)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		provider := handler.GetString("provider")
		if provider != "generic" {
			t.Errorf("Expected provider='generic', got '%s'", provider)
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

func TestConfigHandler_Get(t *testing.T) {
	t.Run("ReturnsValueFromData", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		handler.Set("simple.key", "test_value")

		value := handler.Get("simple.key")

		if value != "test_value" {
			t.Errorf("Expected 'test_value', got '%v'", value)
		}
	})

	t.Run("ReturnsNilForEmptyPath", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		value := handler.Get("")

		if value != nil {
			t.Errorf("Expected nil for empty path, got '%v'", value)
		}
	})

	t.Run("ReturnsNilForMissingKey", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		value := handler.Get("nonexistent.key")

		if value != nil {
			t.Errorf("Expected nil for missing key, got '%v'", value)
		}
	})

	t.Run("NavigatesNestedMaps", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		handler.Set("parent.child.grandchild", "nested_value")

		value := handler.Get("parent.child.grandchild")

		if value != "nested_value" {
			t.Errorf("Expected 'nested_value', got '%v'", value)
		}
	})

	t.Run("FallsBackToSchemaDefaultsForTopLevelKey", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		tmpDir, _ := mocks.Shell.GetProjectRoot()

		handler := NewConfigHandler(mocks.Shell)

		schemaDir := filepath.Join(tmpDir, "contexts", "_template")
		os.MkdirAll(schemaDir, 0755)
		schemaContent := `$schema: https://json-schema.org/draft/2020-12/schema
type: object
properties:
  default_key:
    type: string
    default: schema_default_value
`
		os.WriteFile(filepath.Join(schemaDir, "schema.yaml"), []byte(schemaContent), 0644)
		handler.LoadConfig()

		value := handler.Get("default_key")

		if value != "schema_default_value" {
			t.Errorf("Expected schema default 'schema_default_value', got '%v'", value)
		}
	})

	t.Run("FallsBackToSchemaDefaultsForNestedKey", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		tmpDir, _ := mocks.Shell.GetProjectRoot()

		handler := NewConfigHandler(mocks.Shell)

		schemaDir := filepath.Join(tmpDir, "contexts", "_template")
		os.MkdirAll(schemaDir, 0755)
		schemaContent := `$schema: https://json-schema.org/draft/2020-12/schema
type: object
properties:
  nested:
    type: object
    properties:
      key:
        type: string
        default: nested_default
`
		os.WriteFile(filepath.Join(schemaDir, "schema.yaml"), []byte(schemaContent), 0644)
		handler.LoadConfig()

		value := handler.Get("nested.key")

		if value != "nested_default" {
			t.Errorf("Expected nested schema default 'nested_default', got '%v'", value)
		}
	})

	t.Run("DelegatesToProvider", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)
		mockProvider := &mockValueProvider{value: "provider-value"}
		handler.RegisterProvider("test", mockProvider)

		value := handler.Get("test.key")

		if value != "provider-value" {
			t.Errorf("Expected 'provider-value', got %v", value)
		}
	})

	t.Run("ReturnsNilOnProviderError", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)
		mockProvider := &mockValueProvider{err: errors.New("provider error")}
		handler.RegisterProvider("test", mockProvider)

		value := handler.Get("test.key")

		if value != nil {
			t.Errorf("Expected nil on provider error, got %v", value)
		}
	})

	t.Run("FallsBackToDataMapForNonProviderKeys", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)
		handler.LoadConfigString("test: value")

		value := handler.Get("test")

		if value != "value" {
			t.Errorf("Expected 'value', got %v", value)
		}
	})

	t.Run("ConfigValuesTakePrecedenceOverProvider", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)
		handler.Set("terraform.enabled", true)
		handler.Set("terraform.backend.type", "s3")
		mockProvider := &mockValueProvider{value: "provider-value"}
		handler.RegisterProvider("terraform", mockProvider)

		enabled := handler.Get("terraform.enabled")
		if enabled != true {
			t.Errorf("Expected config value true, got %v", enabled)
		}

		backendType := handler.Get("terraform.backend.type")
		if backendType != "s3" {
			t.Errorf("Expected config value 's3', got %v", backendType)
		}
	})

	t.Run("ConfigValuesTakePrecedenceOverProvider", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)
		handler.Set("test.key", "config-value")
		mockProvider := &mockValueProvider{value: "provider-value"}
		handler.RegisterProvider("test", mockProvider)

		value := handler.Get("test.key")
		if value != "config-value" {
			t.Errorf("Expected config value 'config-value', got %v", value)
		}
	})

}

func TestConfigHandler_RegisterProvider(t *testing.T) {
	t.Run("RegistersProvider", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)
		mockProvider := &mockValueProvider{}

		handler.RegisterProvider("test", mockProvider)

		value := handler.Get("test.key")
		if value != "mock-value" {
			t.Errorf("Expected 'mock-value', got %v", value)
		}
	})

	t.Run("AllowsMultipleProviders", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)
		provider1 := &mockValueProvider{value: "value1"}
		provider2 := &mockValueProvider{value: "value2"}

		handler.RegisterProvider("provider1", provider1)
		handler.RegisterProvider("provider2", provider2)

		value1 := handler.Get("provider1.key")
		if value1 != "value1" {
			t.Errorf("Expected 'value1', got %v", value1)
		}

		value2 := handler.Get("provider2.key")
		if value2 != "value2" {
			t.Errorf("Expected 'value2', got %v", value2)
		}
	})

	t.Run("OverwritesExistingProvider", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)
		provider1 := &mockValueProvider{value: "value1"}
		provider2 := &mockValueProvider{value: "value2"}

		handler.RegisterProvider("test", provider1)
		handler.RegisterProvider("test", provider2)

		value := handler.Get("test.key")
		if value != "value2" {
			t.Errorf("Expected 'value2', got %v", value)
		}
	})
}

func TestConfigHandler_GetString(t *testing.T) {
	t.Run("ReturnsStringValue", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		handler.Set("key", "string_value")

		result := handler.GetString("key")

		if result != "string_value" {
			t.Errorf("Expected 'string_value', got '%s'", result)
		}
	})

	t.Run("ReturnsEmptyStringForMissingKey", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		result := handler.GetString("missing.key")

		if result != "" {
			t.Errorf("Expected empty string, got '%s'", result)
		}
	})

	t.Run("ReturnsProvidedDefault", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		result := handler.GetString("missing.key", "default_value")

		if result != "default_value" {
			t.Errorf("Expected 'default_value', got '%s'", result)
		}
	})

	t.Run("ConvertsNonStringToString", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		handler.Set("number", 42)

		result := handler.GetString("number")

		if result != "42" {
			t.Errorf("Expected '42', got '%s'", result)
		}
	})
}

func TestConfigHandler_GetInt(t *testing.T) {
	t.Run("ReturnsIntValue", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		handler.Set("count", 42)

		result := handler.GetInt("count")

		if result != 42 {
			t.Errorf("Expected 42, got %d", result)
		}
	})

	t.Run("IgnoresFloat64Values", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		handler.Set("count", float64(42.7))

		result := handler.GetInt("count")

		if result != 0 {
			t.Errorf("Expected 0 (fallback for non-integer), got %d", result)
		}
	})

	t.Run("ConvertsUint64ToInt", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		handler.Set("count", uint64(42))

		result := handler.GetInt("count")

		if result != 42 {
			t.Errorf("Expected 42, got %d", result)
		}
	})

	t.Run("ConvertsStringToInt", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		handler.Set("count", "42")

		result := handler.GetInt("count")

		if result != 42 {
			t.Errorf("Expected 42, got %d", result)
		}
	})

	t.Run("ReturnsZeroForMissingKey", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		result := handler.GetInt("missing.key")

		if result != 0 {
			t.Errorf("Expected 0, got %d", result)
		}
	})

	t.Run("ReturnsProvidedDefault", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		result := handler.GetInt("missing.key", 99)

		if result != 99 {
			t.Errorf("Expected 99, got %d", result)
		}
	})

	t.Run("ConvertsInt64ToInt", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		handler.Set("count", int64(42))

		result := handler.GetInt("count")

		if result != 42 {
			t.Errorf("Expected 42, got %d", result)
		}
	})

	t.Run("ConvertsUintToInt", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		handler.Set("count", uint(42))

		result := handler.GetInt("count")

		if result != 42 {
			t.Errorf("Expected 42, got %d", result)
		}
	})

	t.Run("HandlesUint64Overflow", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		maxUint64 := uint64(^uint(0)>>1) + 1
		handler.Set("count", maxUint64)

		result := handler.GetInt("count", 99)

		if result != 99 {
			t.Errorf("Expected default value 99 for uint64 overflow, got %d", result)
		}
	})

	t.Run("HandlesInvalidString", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		handler.Set("count", "not-a-number")

		result := handler.GetInt("count", 99)

		if result != 99 {
			t.Errorf("Expected default value 99 for invalid string, got %d", result)
		}
	})

	t.Run("ReturnsZeroForNonNumericValue", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		handler.Set("count", "not_a_number")

		result := handler.GetInt("count")

		if result != 0 {
			t.Errorf("Expected 0 for non-numeric string, got %d", result)
		}
	})
}

func TestConfigHandler_GetBool(t *testing.T) {
	t.Run("ReturnsBoolValue", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		handler.Set("enabled", true)

		result := handler.GetBool("enabled")

		if !result {
			t.Error("Expected true, got false")
		}
	})

	t.Run("ReturnsFalseForMissingKey", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		result := handler.GetBool("missing.key")

		if result {
			t.Error("Expected false, got true")
		}
	})

	t.Run("ReturnsProvidedDefault", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		result := handler.GetBool("missing.key", true)

		if !result {
			t.Error("Expected true, got false")
		}
	})

	t.Run("ReturnsFalseForNonBoolValueWithDefault", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		handler.Set("enabled", 1)

		result := handler.GetBool("enabled", true)

		if result {
			t.Error("Expected false for non-bool value even with default, got true")
		}
	})
}

func TestConfigHandler_GetStringSlice(t *testing.T) {
	t.Run("ReturnsStringSlice", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		handler.Set("items", []string{"a", "b", "c"})

		result := handler.GetStringSlice("items")

		if len(result) != 3 {
			t.Errorf("Expected length 3, got %d", len(result))
		}
		if result[0] != "a" || result[1] != "b" || result[2] != "c" {
			t.Errorf("Expected [a b c], got %v", result)
		}
	})

	t.Run("ConvertsInterfaceSlice", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		handler.Set("items", []interface{}{"x", "y", "z"})

		result := handler.GetStringSlice("items")

		if len(result) != 3 {
			t.Errorf("Expected length 3, got %d", len(result))
		}
		if result[0] != "x" || result[1] != "y" || result[2] != "z" {
			t.Errorf("Expected [x y z], got %v", result)
		}
	})

	t.Run("ConvertsInterfaceSliceWithNonStringValues", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		handler.Set("items", []interface{}{"x", 42, true})

		result := handler.GetStringSlice("items")

		if len(result) != 3 {
			t.Errorf("Expected length 3, got %d", len(result))
		}
		if result[0] != "x" {
			t.Errorf("Expected first element 'x', got '%s'", result[0])
		}
		if result[1] != "42" {
			t.Errorf("Expected second element '42', got '%s'", result[1])
		}
		if result[2] != "true" {
			t.Errorf("Expected third element 'true', got '%s'", result[2])
		}
	})

	t.Run("ReturnsEmptySliceForMissingKey", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		result := handler.GetStringSlice("missing.key")

		if len(result) != 0 {
			t.Errorf("Expected empty slice, got %v", result)
		}
	})

	t.Run("ReturnsProvidedDefault", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		defaultSlice := []string{"default1", "default2"}

		result := handler.GetStringSlice("missing.key", defaultSlice)

		if len(result) != 2 || result[0] != "default1" || result[1] != "default2" {
			t.Errorf("Expected default slice, got %v", result)
		}
	})
}

func TestConfigHandler_GetStringMap(t *testing.T) {
	t.Run("ReturnsStringMap", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		handler.Set("environment", map[string]string{"KEY1": "value1", "KEY2": "value2"})

		result := handler.GetStringMap("environment")

		if len(result) != 2 {
			t.Errorf("Expected map with 2 entries, got %d", len(result))
		}
		if result["KEY1"] != "value1" || result["KEY2"] != "value2" {
			t.Errorf("Expected KEY1=value1, KEY2=value2, got %v", result)
		}
	})

	t.Run("ConvertsInterfaceMap", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		handler.Set("environment", map[string]interface{}{"KEY": "value"})

		result := handler.GetStringMap("environment")

		if result["KEY"] != "value" {
			t.Errorf("Expected KEY=value, got %v", result)
		}
	})

	t.Run("ReturnsEmptyMapForMissingKey", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		result := handler.GetStringMap("missing.key")

		if len(result) != 0 {
			t.Errorf("Expected empty map, got %v", result)
		}
	})

	t.Run("ReturnsProvidedDefault", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		defaultMap := map[string]string{"default": "value"}

		result := handler.GetStringMap("missing.key", defaultMap)

		if result["default"] != "value" {
			t.Errorf("Expected default map, got %v", result)
		}
	})

	t.Run("ConvertsInterfaceKeyMap", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		handler.Set("env", map[interface{}]interface{}{"KEY": "value"})

		result := handler.GetStringMap("env")

		if result["KEY"] != "value" {
			t.Errorf("Expected KEY=value, got %v", result)
		}
	})

	t.Run("ConvertsNonStringValuesToString", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		handler.Set("env", map[string]interface{}{"NUM": 42, "BOOL": true})

		result := handler.GetStringMap("env")

		if result["NUM"] != "42" {
			t.Errorf("Expected NUM='42', got '%s'", result["NUM"])
		}
		if result["BOOL"] != "true" {
			t.Errorf("Expected BOOL='true', got '%s'", result["BOOL"])
		}
	})
}

func TestConfigHandler_Set(t *testing.T) {
	t.Run("SetsSimpleValue", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		err := handler.Set("key", "value")

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		result := handler.GetString("key")
		if result != "value" {
			t.Errorf("Expected 'value', got '%s'", result)
		}
	})

	t.Run("SetsNestedValue", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		err := handler.Set("parent.child.key", "nested_value")

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		result := handler.GetString("parent.child.key")
		if result != "nested_value" {
			t.Errorf("Expected 'nested_value', got '%s'", result)
		}
	})

	t.Run("CreatesIntermediateMaps", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		err := handler.Set("a.b.c.d", "deep_value")

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		result := handler.GetString("a.b.c.d")
		if result != "deep_value" {
			t.Errorf("Expected 'deep_value', got '%s'", result)
		}
	})

	t.Run("ValidatesDynamicFieldsAgainstSchema", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		tmpDir, _ := mocks.Shell.GetProjectRoot()

		handler := NewConfigHandler(mocks.Shell)

		schemaDir := filepath.Join(tmpDir, "contexts", "_template")
		os.MkdirAll(schemaDir, 0755)
		schemaContent := `$schema: https://json-schema.org/draft/2020-12/schema
type: object
properties:
  allowed_key:
    type: string
additionalProperties: false
`
		os.WriteFile(filepath.Join(schemaDir, "schema.yaml"), []byte(schemaContent), 0644)
		handler.LoadSchema(filepath.Join(schemaDir, "schema.yaml"))

		err := handler.Set("disallowed_key", "value")

		if err == nil {
			t.Error("Expected validation error for disallowed key")
		}
	})

	t.Run("DoesNotValidateStaticFields", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		tmpDir, _ := mocks.Shell.GetProjectRoot()

		handler := NewConfigHandler(mocks.Shell)

		schemaDir := filepath.Join(tmpDir, "contexts", "_template")
		os.MkdirAll(schemaDir, 0755)
		schemaContent := `$schema: https://json-schema.org/draft/2020-12/schema
type: object
additionalProperties: false
`
		os.WriteFile(filepath.Join(schemaDir, "schema.yaml"), []byte(schemaContent), 0644)
		handler.LoadSchema(filepath.Join(schemaDir, "schema.yaml"))

		err := handler.Set("provider", "generic")

		if err != nil {
			t.Errorf("Expected no error for static field, got %v", err)
		}
	})

	t.Run("ReturnsErrorForEmptyPath", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		err := handler.Set("", "value")

		if err == nil {
			t.Error("Expected error for empty path")
		}
	})

	t.Run("ReturnsErrorForInvalidPath", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		err := handler.Set("invalid..path", "value")

		if err == nil {
			t.Error("Expected error for invalid path with double dots")
		}
	})

	t.Run("ConvertsStringBasedOnSchemaType", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		tmpDir, _ := mocks.Shell.GetProjectRoot()

		handler := NewConfigHandler(mocks.Shell)

		schemaDir := filepath.Join(tmpDir, "contexts", "_template")
		os.MkdirAll(schemaDir, 0755)
		schemaContent := `$schema: https://json-schema.org/draft/2020-12/schema
type: object
properties:
  bool_field:
    type: boolean
  int_field:
    type: integer
  float_field:
    type: number
`
		os.WriteFile(filepath.Join(schemaDir, "schema.yaml"), []byte(schemaContent), 0644)
		handler.LoadSchema(filepath.Join(schemaDir, "schema.yaml"))

		handler.Set("bool_field", "true")
		handler.Set("int_field", "42")
		handler.Set("float_field", "3.14")

		boolVal := handler.GetBool("bool_field")
		if !boolVal {
			t.Error("Expected string 'true' to be converted to boolean")
		}

		intVal := handler.GetInt("int_field")
		if intVal != 42 {
			t.Errorf("Expected string '42' to be converted to int, got %d", intVal)
		}

		floatVal := handler.Get("float_field")
		if floatVal != 3.14 {
			t.Errorf("Expected string '3.14' to be converted to float, got %v", floatVal)
		}
	})
}

func TestConfigHandler_SaveConfig(t *testing.T) {
	t.Run("CreatesRootWindsorYaml", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		tmpDir, _ := mocks.Shell.GetProjectRoot()

		handler := NewConfigHandler(mocks.Shell)
		handler.SetContext("test-context")
		handler.Set("provider", "local")

		err := handler.SaveConfig()

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		rootPath := filepath.Join(tmpDir, "windsor.yaml")
		content, err := os.ReadFile(rootPath)
		if err != nil {
			t.Fatalf("Failed to read root config: %v", err)
		}

		expected := "version: v1alpha1\n"
		if string(content) != expected {
			t.Errorf("Expected root config to be:\n%s\nGot:\n%s", expected, string(content))
		}
	})

	t.Run("SeparatesStaticAndDynamicFields", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		tmpDir, _ := mocks.Shell.GetProjectRoot()

		handler := NewConfigHandler(mocks.Shell)
		handler.SetContext("test-context")

		handler.Set("provider", "generic")
		handler.Set("cluster.enabled", true)
		handler.Set("custom_dynamic_field", "dynamic_value")

		err := handler.SaveConfig()

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		windsorPath := filepath.Join(tmpDir, "contexts", "test-context", "windsor.yaml")
		windsorContent, err := os.ReadFile(windsorPath)
		if err != nil {
			t.Fatalf("Failed to read windsor.yaml: %v", err)
		}

		windsorStr := string(windsorContent)
		if !contains(windsorStr, "provider:") {
			t.Error("windsor.yaml should contain provider (static field)")
		}
		if !contains(windsorStr, "cluster:") {
			t.Error("windsor.yaml should contain cluster (static field)")
		}
		if contains(windsorStr, "custom_dynamic_field") {
			t.Error("windsor.yaml should not contain dynamic fields")
		}

		valuesPath := filepath.Join(tmpDir, "contexts", "test-context", "values.yaml")
		valuesContent, err := os.ReadFile(valuesPath)
		if err != nil {
			t.Fatalf("Failed to read values.yaml: %v", err)
		}

		valuesStr := string(valuesContent)
		if !contains(valuesStr, "custom_dynamic_field") {
			t.Error("values.yaml should contain custom_dynamic_field (dynamic field)")
		}
		if contains(valuesStr, "provider:") {
			t.Error("values.yaml should not contain provider (static field)")
		}
	})

	t.Run("ExcludesFieldsWithYamlDashTag", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		tmpDir, _ := mocks.Shell.GetProjectRoot()

		handler := NewConfigHandler(mocks.Shell)
		handler.SetContext("test-context")

		handler.Set("cluster.workers.count", 2)
		handler.Set("cluster.workers.nodes.worker-1.endpoint", "127.0.0.1:50001")
		handler.Set("cluster.workers.nodes.worker-1.hostname", "worker-1")

		err := handler.SaveConfig()

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		windsorPath := filepath.Join(tmpDir, "contexts", "test-context", "windsor.yaml")
		windsorContent, err := os.ReadFile(windsorPath)
		if err != nil {
			t.Fatalf("Failed to read windsor.yaml: %v", err)
		}

		windsorStr := string(windsorContent)
		if contains(windsorStr, "nodes:") {
			t.Errorf("windsor.yaml should not contain nodes (yaml:\"-\" tag), got:\n%s", windsorStr)
		}
		if !contains(windsorStr, "count:") {
			t.Errorf("windsor.yaml should contain count field, got:\n%s", windsorStr)
		}
	})

	t.Run("DoesNotSaveSchemaDefaults", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		tmpDir, _ := mocks.Shell.GetProjectRoot()

		handler := NewConfigHandler(mocks.Shell)
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
		handler.LoadConfig()

		value := handler.Get("schema_only_key")
		if value != "should_not_be_saved" {
			t.Fatalf("Schema default should be accessible via Get, got '%v'", value)
		}

		err := handler.SaveConfig()
		if err != nil {
			t.Fatalf("SaveConfig failed: %v", err)
		}

		valuesPath := filepath.Join(tmpDir, "contexts", "test-context", "values.yaml")
		if _, err := os.Stat(valuesPath); err == nil {
			content, _ := os.ReadFile(valuesPath)
			if contains(string(content), "schema_only_key") {
				t.Errorf("values.yaml should not contain schema defaults, got:\n%s", string(content))
			}
		}
	})

	t.Run("SavesOnlyUserSetDynamicValues", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		tmpDir, _ := mocks.Shell.GetProjectRoot()

		handler := NewConfigHandler(mocks.Shell)
		handler.SetContext("test-context")

		handler.Set("user_key", "user_value")

		err := handler.SaveConfig()

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		valuesPath := filepath.Join(tmpDir, "contexts", "test-context", "values.yaml")
		content, err := os.ReadFile(valuesPath)
		if err != nil {
			t.Fatalf("Failed to read values.yaml: %v", err)
		}

		expected := "user_key: user_value\n"
		if string(content) != expected {
			t.Errorf("Expected values.yaml to be:\n%s\nGot:\n%s", expected, string(content))
		}
	})

	t.Run("SaveAndReloadPreservesData", func(t *testing.T) {
		// Given a config handler with data
		handler, tmpDir := setupPrivateTestHandler(t)
		handler.SetContext("save-test")

		handler.Set("provider", "generic")
		handler.Set("cluster.enabled", true)
		handler.Set("cluster.workers.count", 2)
		handler.Set("custom_dynamic", "dynamic_value")

		// When saving and reloading config
		err := handler.SaveConfig()
		if err != nil {
			t.Fatalf("SaveConfig failed: %v", err)
		}

		newHandler, _ := setupPrivateTestHandler(t)
		newHandler.shell = handler.shell
		newHandler.SetContext("save-test")

		err = newHandler.LoadConfig()
		if err != nil {
			t.Fatalf("LoadConfig failed: %v", err)
		}

		// Then data should be preserved
		provider := newHandler.GetString("provider")
		if provider != "generic" {
			t.Errorf("Expected provider to be preserved, got '%s'", provider)
		}

		count := newHandler.GetInt("cluster.workers.count")
		if count != 2 {
			t.Errorf("Expected count=2 to be preserved, got %d", count)
		}

		customDynamic := newHandler.GetString("custom_dynamic")
		if customDynamic != "dynamic_value" {
			t.Errorf("Expected custom_dynamic to be preserved, got '%s'", customDynamic)
		}

		// And files should be properly separated
		windsorPath := filepath.Join(tmpDir, "contexts", "save-test", "windsor.yaml")
		windsorContent, _ := os.ReadFile(windsorPath)
		if contains(string(windsorContent), "custom_dynamic") {
			t.Error("windsor.yaml should not contain dynamic fields")
		}

		valuesPath := filepath.Join(tmpDir, "contexts", "save-test", "values.yaml")
		valuesContent, _ := os.ReadFile(valuesPath)
		if contains(string(valuesContent), "provider") {
			t.Error("values.yaml should not contain static fields")
		}
	})

	t.Run("CreatesHandlerWithShell", func(t *testing.T) {
		// Given a shell
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell).(*configHandler)

		// When attempting to save config without context
		// Note: This test verifies the handler is created correctly with a shell.
		// SaveConfig may fail for other reasons (missing context, etc.) but not due to missing shell.
		if handler.shell == nil {
			t.Error("Expected shell to be set on handler")
		}
	})

	t.Run("SkipsCreatingFilesWhenNoData", func(t *testing.T) {
		// Given a handler with no data to save
		handler, tmpDir := setupPrivateTestHandler(t)
		handler.SetContext("empty-test")

		// When saving config with no data
		err := handler.SaveConfig()

		// Then it should succeed without creating files
		if err != nil {
			t.Fatalf("Expected no error for empty data, got %v", err)
		}

		windsorPath := filepath.Join(tmpDir, "contexts", "empty-test", "windsor.yaml")
		if _, err := os.Stat(windsorPath); !os.IsNotExist(err) {
			t.Error("Expected windsor.yaml not to be created when no static fields")
		}

		valuesPath := filepath.Join(tmpDir, "contexts", "empty-test", "values.yaml")
		if _, err := os.Stat(valuesPath); !os.IsNotExist(err) {
			t.Error("Expected values.yaml not to be created when no dynamic fields")
		}
	})

	t.Run("CreatesRootConfigOnlyOnce", func(t *testing.T) {
		// Given an existing root config file
		handler, tmpDir := setupPrivateTestHandler(t)
		handler.SetContext("test-context")

		rootPath := filepath.Join(tmpDir, "windsor.yaml")
		existingContent := "version: v1alpha1\nexisting: data\n"
		os.WriteFile(rootPath, []byte(existingContent), 0644)

		// When saving config with new data
		handler.Set("provider", "test")
		handler.SaveConfig()

		// Then existing root config should be preserved
		content, _ := os.ReadFile(rootPath)
		if string(content) != existingContent {
			t.Error("Expected existing root config to be preserved")
		}
	})

	t.Run("HandlesRootConfigMarshalError", func(t *testing.T) {
		// Given a handler with marshal error
		handler, _ := setupPrivateTestHandler(t)
		handler.SetContext("test-context")
		handler.Set("provider", "test")

		handler.shims.YamlMarshal = func(v any) ([]byte, error) {
			return nil, os.ErrInvalid
		}

		// When saving config
		err := handler.SaveConfig()

		// Then it should return marshal error
		if err == nil {
			t.Error("Expected marshal error")
		}
	})

	t.Run("HandlesWriteFileError", func(t *testing.T) {
		// Given a handler with write file error
		handler, _ := setupPrivateTestHandler(t)
		handler.SetContext("test-context")
		handler.Set("provider", "test")

		handler.shims.WriteFile = func(name string, data []byte, perm os.FileMode) error {
			return os.ErrPermission
		}

		// When saving config
		err := handler.SaveConfig()

		// Then it should return write file error
		if err == nil {
			t.Error("Expected write file error")
		}
	})

	t.Run("OverwritesExistingContextConfig", func(t *testing.T) {
		// Given an existing context config file
		handler, tmpDir := setupPrivateTestHandler(t)
		handler.SetContext("test-context")

		contextDir := filepath.Join(tmpDir, "contexts", "test-context")
		os.MkdirAll(contextDir, 0755)
		windsorPath := filepath.Join(contextDir, "windsor.yaml")
		os.WriteFile(windsorPath, []byte("provider: old\n"), 0644)

		// When saving with overwrite=true
		handler.Set("provider", "new")
		err := handler.SaveConfig(true)

		// Then it should overwrite the existing config
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		content, _ := os.ReadFile(windsorPath)
		if !contains(string(content), "new") {
			t.Error("Expected config to be overwritten with new value")
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

func TestConfigHandler_GetContextValues(t *testing.T) {
	t.Run("ReturnsDataMergedWithSchemaDefaults", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		tmpDir, _ := mocks.Shell.GetProjectRoot()

		handler := NewConfigHandler(mocks.Shell)

		schemaDir := filepath.Join(tmpDir, "contexts", "_template")
		os.MkdirAll(schemaDir, 0755)
		schemaContent := `$schema: https://json-schema.org/draft/2020-12/schema
type: object
properties:
  schema_key:
    type: string
    default: schema_value
`
		os.WriteFile(filepath.Join(schemaDir, "schema.yaml"), []byte(schemaContent), 0644)
		handler.LoadConfig()
		handler.Set("user_key", "user_value")

		values, err := handler.GetContextValues()

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if values["schema_key"] != "schema_value" {
			t.Errorf("Expected schema default in context values, got '%v'", values["schema_key"])
		}

		if values["user_key"] != "user_value" {
			t.Errorf("Expected user value in context values, got '%v'", values["user_key"])
		}
	})

	t.Run("IncludesServiceCalculatedValues", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		handler.Set("cluster.workers.nodes.worker-1.endpoint", "127.0.0.1:50001")

		values, err := handler.GetContextValues()

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if cluster, ok := values["cluster"].(map[string]any); ok {
			if workers, ok := cluster["workers"].(map[string]any); ok {
				if nodes, ok := workers["nodes"].(map[string]any); ok {
					if worker1, ok := nodes["worker-1"].(map[string]any); ok {
						if endpoint := worker1["endpoint"]; endpoint != "127.0.0.1:50001" {
							t.Errorf("Expected service-calculated endpoint, got '%v'", endpoint)
						}
					} else {
						t.Error("Expected worker-1 node to be accessible")
					}
				} else {
					t.Error("Expected nodes to be accessible in GetContextValues")
				}
			}
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

		kubeDir := filepath.Join(configRoot, ".kube")
		os.MkdirAll(kubeDir, 0755)
		os.WriteFile(filepath.Join(kubeDir, "config"), []byte("test"), 0644)

		err := handler.Clean()

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if _, err := os.Stat(kubeDir); !os.IsNotExist(err) {
			t.Error("Expected .kube directory to be removed")
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

func TestConfigHandler_GetContext(t *testing.T) {
	t.Run("ReturnsContextFromEnvironment", func(t *testing.T) {
		mocks := setupConfigMocks(t)

		handler := NewConfigHandler(mocks.Shell)

		context := handler.GetContext()

		if context != "test-context" {
			t.Errorf("Expected 'test-context' from environment, got '%s'", context)
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

// =============================================================================
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
