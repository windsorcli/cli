package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// =============================================================================
// Test Accessors
// =============================================================================

func TestConfigHandler_Accessors(t *testing.T) {
	t.Run("GetReturnsNilForMissingOrEmptyPath", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		if value := handler.Get(""); value != nil {
			t.Errorf("Expected nil for empty path, got %v", value)
		}
		if value := handler.Get("missing.path"); value != nil {
			t.Errorf("Expected nil for missing path, got %v", value)
		}
	})

	t.Run("GetReturnsStoredAndNestedValues", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		if err := handler.Set("parent.child.key", "nested_value"); err != nil {
			t.Fatalf("Expected no error setting nested value, got %v", err)
		}

		if value := handler.Get("parent.child.key"); value != "nested_value" {
			t.Errorf("Expected nested value, got %v", value)
		}
	})

	t.Run("GetFallsBackToSchemaDefault", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		tmpDir, _ := mocks.Shell.GetProjectRoot()
		handler := NewConfigHandler(mocks.Shell)

		schemaDir := filepath.Join(tmpDir, "contexts", "_template")
		if err := os.MkdirAll(schemaDir, 0755); err != nil {
			t.Fatalf("Expected no error creating schema dir, got %v", err)
		}
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
		if err := os.WriteFile(filepath.Join(schemaDir, "schema.yaml"), []byte(schemaContent), 0644); err != nil {
			t.Fatalf("Expected no error writing schema, got %v", err)
		}
		if err := handler.LoadConfig(); err != nil {
			t.Fatalf("Expected no error loading config, got %v", err)
		}

		if value := handler.Get("nested.key"); value != "nested_default" {
			t.Errorf("Expected schema default value, got %v", value)
		}
	})

	t.Run("ProviderLookupUsesPrefixAndConfigPrecedence", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)
		provider := &mockValueProvider{value: "provider-value"}
		handler.RegisterProvider("test", provider)

		if value := handler.Get("test.key"); value != "provider-value" {
			t.Errorf("Expected provider value, got %v", value)
		}

		if err := handler.Set("test.key", "config-value"); err != nil {
			t.Fatalf("Expected no error setting config value, got %v", err)
		}
		if value := handler.Get("test.key"); value != "config-value" {
			t.Errorf("Expected config value precedence, got %v", value)
		}
	})

	t.Run("ProviderErrorReturnsNil", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)
		handler.RegisterProvider("test", &mockValueProvider{err: errors.New("provider error")})

		if value := handler.Get("test.key"); value != nil {
			t.Errorf("Expected nil on provider error, got %v", value)
		}
	})

	t.Run("GetStringCoversValueAndDefault", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		if err := handler.Set("key", 42); err != nil {
			t.Fatalf("Expected no error setting value, got %v", err)
		}
		if got := handler.GetString("key"); got != "42" {
			t.Errorf("Expected converted string '42', got %s", got)
		}
		if got := handler.GetString("missing", "fallback"); got != "fallback" {
			t.Errorf("Expected fallback string, got %s", got)
		}
	})

	t.Run("GetIntCoversConversionsAndDefaults", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		if err := handler.Set("a", "42"); err != nil {
			t.Fatalf("Expected no error setting value, got %v", err)
		}
		if err := handler.Set("b", uint64(7)); err != nil {
			t.Fatalf("Expected no error setting value, got %v", err)
		}
		if err := handler.Set("c", "bad"); err != nil {
			t.Fatalf("Expected no error setting value, got %v", err)
		}

		if got := handler.GetInt("a"); got != 42 {
			t.Errorf("Expected 42, got %d", got)
		}
		if got := handler.GetInt("b"); got != 7 {
			t.Errorf("Expected 7, got %d", got)
		}
		if got := handler.GetInt("c", 99); got != 99 {
			t.Errorf("Expected default 99, got %d", got)
		}
	})

	t.Run("GetBoolCoversValueAndDefault", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		if err := handler.Set("enabled", true); err != nil {
			t.Fatalf("Expected no error setting value, got %v", err)
		}
		if got := handler.GetBool("enabled"); !got {
			t.Error("Expected true for bool value")
		}
		if got := handler.GetBool("missing", true); !got {
			t.Error("Expected default true for missing value")
		}
	})

	t.Run("GetStringSliceConvertsInterfaceSlice", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		if err := handler.Set("items", []any{"x", 42, true}); err != nil {
			t.Fatalf("Expected no error setting value, got %v", err)
		}

		items := handler.GetStringSlice("items")
		if len(items) != 3 {
			t.Fatalf("Expected 3 items, got %d", len(items))
		}
		if items[0] != "x" || items[1] != "42" || items[2] != "true" {
			t.Errorf("Expected [x 42 true], got %v", items)
		}
	})

	t.Run("GetStringMapConvertsMapValues", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := NewConfigHandler(mocks.Shell)

		if err := handler.Set("env", map[string]any{"NUM": 42, "BOOL": true}); err != nil {
			t.Fatalf("Expected no error setting value, got %v", err)
		}

		env := handler.GetStringMap("env")
		if env["NUM"] != "42" || env["BOOL"] != "true" {
			t.Errorf("Expected converted string map, got %v", env)
		}
	})

	t.Run("SetValidatesPathsAndSchema", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		tmpDir, _ := mocks.Shell.GetProjectRoot()
		handler := NewConfigHandler(mocks.Shell)

		if err := handler.Set("valid.path", "value"); err != nil {
			t.Fatalf("Expected no error for valid set, got %v", err)
		}
		if err := handler.Set("", "value"); err == nil {
			t.Fatal("Expected error for empty path")
		}
		if err := handler.Set("invalid..path", "value"); err == nil {
			t.Fatal("Expected error for invalid path")
		}

		schemaDir := filepath.Join(tmpDir, "contexts", "_template")
		if err := os.MkdirAll(schemaDir, 0755); err != nil {
			t.Fatalf("Expected no error creating schema dir, got %v", err)
		}
		schemaContent := `$schema: https://json-schema.org/draft/2020-12/schema
type: object
properties:
  allowed_key:
    type: string
additionalProperties: false
`
		if err := os.WriteFile(filepath.Join(schemaDir, "schema.yaml"), []byte(schemaContent), 0644); err != nil {
			t.Fatalf("Expected no error writing schema, got %v", err)
		}
		if err := handler.LoadSchema(filepath.Join(schemaDir, "schema.yaml")); err != nil {
			t.Fatalf("Expected no error loading schema, got %v", err)
		}

		if err := handler.Set("disallowed_key", "value"); err == nil {
			t.Fatal("Expected schema validation error for disallowed key")
		}
	})
}
