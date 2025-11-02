package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/context/shell"
)

// =============================================================================
// Test Setup
// =============================================================================

func setupPrivateTestHandler(t *testing.T) (*configHandler, string) {
	t.Helper()

	tmpDir := t.TempDir()
	injector := di.NewInjector()

	mockShell := shell.NewMockShell()
	mockShell.GetProjectRootFunc = func() (string, error) {
		return tmpDir, nil
	}
	injector.Register("shell", mockShell)

	handler := NewConfigHandler(injector).(*configHandler)
	handler.Initialize()

	return handler, tmpDir
}

// =============================================================================
// Test Private Methods
// =============================================================================

func TestConfigHandler_getValueByPathFromMap(t *testing.T) {
	t.Run("ReturnsValueFromSimplePath", func(t *testing.T) {
		data := map[string]any{
			"key": "value",
		}

		result := getValueByPathFromMap(data, []string{"key"})

		if result != "value" {
			t.Errorf("Expected 'value', got '%v'", result)
		}
	})

	t.Run("NavigatesNestedMaps", func(t *testing.T) {
		data := map[string]any{
			"parent": map[string]any{
				"child": map[string]any{
					"key": "nested_value",
				},
			},
		}

		result := getValueByPathFromMap(data, []string{"parent", "child", "key"})

		if result != "nested_value" {
			t.Errorf("Expected 'nested_value', got '%v'", result)
		}
	})

	t.Run("ReturnsNilForMissingKey", func(t *testing.T) {
		data := map[string]any{
			"key": "value",
		}

		result := getValueByPathFromMap(data, []string{"missing"})

		if result != nil {
			t.Errorf("Expected nil, got '%v'", result)
		}
	})

	t.Run("ReturnsNilForEmptyPath", func(t *testing.T) {
		data := map[string]any{
			"key": "value",
		}

		result := getValueByPathFromMap(data, []string{})

		if result != nil {
			t.Errorf("Expected nil for empty path, got '%v'", result)
		}
	})

	t.Run("ReturnsNilForMissingIntermediateKey", func(t *testing.T) {
		data := map[string]any{
			"parent": "not_a_map",
		}

		result := getValueByPathFromMap(data, []string{"parent", "child"})

		if result != nil {
			t.Errorf("Expected nil when intermediate key is not a map, got '%v'", result)
		}
	})
}

func TestConfigHandler_setValueInMap(t *testing.T) {
	t.Run("SetsSingleLevelValue", func(t *testing.T) {
		data := make(map[string]any)

		setValueInMap(data, []string{"key"}, "value")

		if data["key"] != "value" {
			t.Errorf("Expected value to be set, got %v", data)
		}
	})

	t.Run("CreatesNestedMaps", func(t *testing.T) {
		data := make(map[string]any)

		setValueInMap(data, []string{"parent", "child", "key"}, "nested_value")

		parent, ok := data["parent"].(map[string]any)
		if !ok {
			t.Fatal("Expected parent to be a map")
		}

		child, ok := parent["child"].(map[string]any)
		if !ok {
			t.Fatal("Expected child to be a map")
		}

		if child["key"] != "nested_value" {
			t.Errorf("Expected 'nested_value', got '%v'", child["key"])
		}
	})

	t.Run("OverwritesExistingValue", func(t *testing.T) {
		data := map[string]any{
			"key": "old_value",
		}

		setValueInMap(data, []string{"key"}, "new_value")

		if data["key"] != "new_value" {
			t.Errorf("Expected value to be overwritten, got %v", data["key"])
		}
	})

	t.Run("HandlesEmptyPathGracefully", func(t *testing.T) {
		data := make(map[string]any)

		setValueInMap(data, []string{}, "value")

		if len(data) != 0 {
			t.Error("Expected empty path to be handled gracefully")
		}
	})
}

func TestConfigHandler_convertInterfaceMap(t *testing.T) {
	t.Run("ConvertsSimpleInterfaceMap", func(t *testing.T) {
		handler, _ := setupPrivateTestHandler(t)

		input := map[interface{}]interface{}{
			"key1": "value1",
			"key2": "value2",
		}

		result := handler.convertInterfaceMap(input)

		if result["key1"] != "value1" {
			t.Errorf("Expected key1='value1', got '%v'", result["key1"])
		}
		if result["key2"] != "value2" {
			t.Errorf("Expected key2='value2', got '%v'", result["key2"])
		}
	})

	t.Run("ConvertsNestedInterfaceMaps", func(t *testing.T) {
		handler, _ := setupPrivateTestHandler(t)

		input := map[interface{}]interface{}{
			"parent": map[interface{}]interface{}{
				"child": "nested_value",
			},
		}

		result := handler.convertInterfaceMap(input)

		parent, ok := result["parent"].(map[string]any)
		if !ok {
			t.Fatal("Expected parent to be converted to map[string]any")
		}

		if parent["child"] != "nested_value" {
			t.Errorf("Expected nested value, got '%v'", parent["child"])
		}
	})

	t.Run("SkipsNonStringKeys", func(t *testing.T) {
		handler, _ := setupPrivateTestHandler(t)

		input := map[interface{}]interface{}{
			123:   "numeric_key",
			"key": "string_key",
		}

		result := handler.convertInterfaceMap(input)

		if len(result) != 1 {
			t.Errorf("Expected only string keys to be included, got %d keys", len(result))
		}
		if result["key"] != "string_key" {
			t.Error("Expected string key to be included")
		}
	})
}

func TestConfigHandler_separateStaticAndDynamicFields(t *testing.T) {
	t.Run("SeparatesBasedOnStaticSchema", func(t *testing.T) {
		handler, _ := setupPrivateTestHandler(t)

		data := map[string]any{
			"provider":    "generic",
			"cluster":     map[string]any{"enabled": true},
			"custom_key":  "custom_value",
			"another_key": "another_value",
		}

		staticFields, dynamicFields := handler.separateStaticAndDynamicFields(data)

		if staticFields["provider"] != "generic" {
			t.Error("Expected provider in static fields")
		}
		if staticFields["cluster"] == nil {
			t.Error("Expected cluster in static fields")
		}
		if staticFields["custom_key"] != nil {
			t.Error("Expected custom_key NOT in static fields")
		}

		if dynamicFields["custom_key"] != "custom_value" {
			t.Error("Expected custom_key in dynamic fields")
		}
		if dynamicFields["another_key"] != "another_value" {
			t.Error("Expected another_key in dynamic fields")
		}
		if dynamicFields["provider"] != nil {
			t.Error("Expected provider NOT in dynamic fields")
		}
	})
}

func TestConfigHandler_isKeyInStaticSchema(t *testing.T) {
	t.Run("ReturnsTrueForStaticFields", func(t *testing.T) {
		handler, _ := setupPrivateTestHandler(t)

		staticKeys := []string{"provider", "cluster", "dns", "docker", "git", "network", "terraform", "vm"}
		for _, key := range staticKeys {
			result := handler.isKeyInStaticSchema(key)

			if !result {
				t.Errorf("Expected '%s' to be in static schema", key)
			}
		}
	})

	t.Run("ReturnsFalseForDynamicFields", func(t *testing.T) {
		handler, _ := setupPrivateTestHandler(t)

		dynamicKeys := []string{"custom_key", "user_field", "dev", "my_variable"}
		for _, key := range dynamicKeys {
			result := handler.isKeyInStaticSchema(key)

			if result {
				t.Errorf("Expected '%s' NOT to be in static schema", key)
			}
		}
	})

	t.Run("HandlesNestedPaths", func(t *testing.T) {
		handler, _ := setupPrivateTestHandler(t)

		result := handler.isKeyInStaticSchema("cluster.workers.count")

		if !result {
			t.Error("Expected nested cluster path to be recognized as static")
		}
	})
}

func TestConfigHandler_deepMerge(t *testing.T) {
	t.Run("MergesSimpleMaps", func(t *testing.T) {
		handler, _ := setupPrivateTestHandler(t)

		base := map[string]any{
			"key1": "value1",
		}
		overlay := map[string]any{
			"key2": "value2",
		}

		result := handler.deepMerge(base, overlay)

		if result["key1"] != "value1" {
			t.Error("Expected base key to be preserved")
		}
		if result["key2"] != "value2" {
			t.Error("Expected overlay key to be added")
		}
	})

	t.Run("OverridesNonMapValues", func(t *testing.T) {
		handler, _ := setupPrivateTestHandler(t)

		base := map[string]any{
			"key": "old_value",
		}
		overlay := map[string]any{
			"key": "new_value",
		}

		result := handler.deepMerge(base, overlay)

		if result["key"] != "new_value" {
			t.Errorf("Expected 'new_value', got '%v'", result["key"])
		}
	})

	t.Run("MergesNestedMaps", func(t *testing.T) {
		handler, _ := setupPrivateTestHandler(t)

		base := map[string]any{
			"parent": map[string]any{
				"key1": "value1",
			},
		}
		overlay := map[string]any{
			"parent": map[string]any{
				"key2": "value2",
			},
		}

		result := handler.deepMerge(base, overlay)

		parent, ok := result["parent"].(map[string]any)
		if !ok {
			t.Fatal("Expected parent to be a map")
		}

		if parent["key1"] != "value1" {
			t.Error("Expected nested base key to be preserved")
		}
		if parent["key2"] != "value2" {
			t.Error("Expected nested overlay key to be added")
		}
	})
}

func TestConfigHandler_mapToContext(t *testing.T) {
	t.Run("ConvertsMapToContextStruct", func(t *testing.T) {
		handler, _ := setupPrivateTestHandler(t)

		data := map[string]any{
			"provider": "test_provider",
			"dns": map[string]any{
				"domain": "test.local",
			},
		}

		result := handler.mapToContext(data)

		if result == nil {
			t.Fatal("Expected non-nil result")
		}
		if result.Provider == nil || *result.Provider != "test_provider" {
			t.Error("Expected provider to be converted")
		}
		if result.DNS == nil || result.DNS.Domain == nil || *result.DNS.Domain != "test.local" {
			t.Error("Expected dns.domain to be converted")
		}
	})

	t.Run("ExcludesNodesFieldWithYamlDashTag", func(t *testing.T) {
		handler, _ := setupPrivateTestHandler(t)

		data := map[string]any{
			"cluster": map[string]any{
				"workers": map[string]any{
					"count": float64(2),
					"nodes": map[string]any{
						"worker-1": map[string]any{
							"endpoint": "127.0.0.1:50001",
						},
					},
				},
			},
		}

		result := handler.mapToContext(data)

		if result == nil || result.Cluster == nil {
			t.Fatal("Expected cluster.workers to exist")
		}

		if len(result.Cluster.Workers.Nodes) > 0 {
			t.Error("Expected nodes field to be excluded due to yaml:\"-\" tag")
		}

		if result.Cluster.Workers.Count == nil || *result.Cluster.Workers.Count != 2 {
			t.Error("Expected count field to be included")
		}
	})

	t.Run("HandlesMarshalError", func(t *testing.T) {
		handler, _ := setupPrivateTestHandler(t)

		handler.shims.YamlMarshal = func(v any) ([]byte, error) {
			return nil, os.ErrInvalid
		}

		result := handler.mapToContext(map[string]any{"test": "value"})

		if result == nil {
			t.Error("Expected empty context on marshal error, got nil")
		}
	})

	t.Run("HandlesUnmarshalError", func(t *testing.T) {
		handler, _ := setupPrivateTestHandler(t)

		handler.shims.YamlUnmarshal = func(data []byte, v any) error {
			return os.ErrInvalid
		}

		result := handler.mapToContext(map[string]any{"test": "value"})

		if result == nil {
			t.Error("Expected empty context on unmarshal error, got nil")
		}
	})
}

func TestConfigHandler_parsePath(t *testing.T) {
	t.Run("ParsesSimpleDotNotation", func(t *testing.T) {
		result := parsePath("parent.child.key")

		expected := []string{"parent", "child", "key"}
		if len(result) != len(expected) {
			t.Fatalf("Expected %d keys, got %d", len(expected), len(result))
		}
		for i, key := range expected {
			if result[i] != key {
				t.Errorf("Expected key[%d]='%s', got '%s'", i, key, result[i])
			}
		}
	})

	t.Run("ParsesBracketNotation", func(t *testing.T) {
		result := parsePath("parent[child].key")

		expected := []string{"parent", "child", "key"}
		if len(result) != len(expected) {
			t.Fatalf("Expected %d keys, got %d", len(expected), len(result))
		}
		for i, key := range expected {
			if result[i] != key {
				t.Errorf("Expected key[%d]='%s', got '%s'", i, key, result[i])
			}
		}
	})

	t.Run("HandlesMixedNotation", func(t *testing.T) {
		result := parsePath("a.b[c].d")

		expected := []string{"a", "b", "c", "d"}
		if len(result) != len(expected) {
			t.Fatalf("Expected %d keys, got %d", len(expected), len(result))
		}
		for i, key := range expected {
			if result[i] != key {
				t.Errorf("Expected key[%d]='%s', got '%s'", i, key, result[i])
			}
		}
	})

	t.Run("HandlesSingleKey", func(t *testing.T) {
		result := parsePath("key")

		if len(result) != 1 || result[0] != "key" {
			t.Errorf("Expected ['key'], got %v", result)
		}
	})
}

func TestConfigHandler_convertStringValue(t *testing.T) {
	t.Run("ConvertsTrueString", func(t *testing.T) {
		handler, _ := setupPrivateTestHandler(t)

		result := handler.convertStringValue("true")

		if result != true {
			t.Errorf("Expected boolean true, got %v (%T)", result, result)
		}
	})

	t.Run("ConvertsFalseString", func(t *testing.T) {
		handler, _ := setupPrivateTestHandler(t)

		result := handler.convertStringValue("false")

		if result != false {
			t.Errorf("Expected boolean false, got %v (%T)", result, result)
		}
	})

	t.Run("ConvertsIntegerString", func(t *testing.T) {
		handler, _ := setupPrivateTestHandler(t)

		result := handler.convertStringValue("42")

		if result != 42 {
			t.Errorf("Expected integer 42, got %v (%T)", result, result)
		}
	})

	t.Run("ConvertsFloatString", func(t *testing.T) {
		handler, _ := setupPrivateTestHandler(t)

		result := handler.convertStringValue("3.14")

		if result != 3.14 {
			t.Errorf("Expected float 3.14, got %v (%T)", result, result)
		}
	})

	t.Run("LeavesOtherStringsAsIs", func(t *testing.T) {
		handler, _ := setupPrivateTestHandler(t)

		result := handler.convertStringValue("regular_string")

		if result != "regular_string" {
			t.Errorf("Expected 'regular_string', got %v (%T)", result, result)
		}
	})

	t.Run("PassesThroughNonStringValues", func(t *testing.T) {
		handler, _ := setupPrivateTestHandler(t)

		result := handler.convertStringValue(42)

		if result != 42 {
			t.Errorf("Expected 42 to pass through, got %v", result)
		}
	})

	t.Run("ConvertsWithSchemaTypeInfo", func(t *testing.T) {
		// Given a config handler with schema type information
		handler, tmpDir := setupPrivateTestHandler(t)

		schemaDir := filepath.Join(tmpDir, "contexts", "_template")
		os.MkdirAll(schemaDir, 0755)
		schemaContent := `$schema: https://json-schema.org/draft/2020-12/schema
type: object
properties:
  typed_bool:
    type: boolean
  typed_int:
    type: integer
  typed_number:
    type: number
`
		os.WriteFile(filepath.Join(schemaDir, "schema.yaml"), []byte(schemaContent), 0644)
		handler.LoadSchema(filepath.Join(schemaDir, "schema.yaml"))

		// When converting strings with schema type info
		boolResult := handler.convertStringValue("true")
		intResult := handler.convertStringValue("42")

		// Then values should be converted according to schema
		if boolResult != true {
			t.Errorf("Expected true, got %v", boolResult)
		}
		if intResult != 42 {
			t.Errorf("Expected 42, got %v", intResult)
		}
	})

	t.Run("FallsBackToPatternMatching", func(t *testing.T) {
		// Given a config handler without schema
		handler, _ := setupPrivateTestHandler(t)

		// When converting strings without schema
		boolResult := handler.convertStringValue("false")
		intResult := handler.convertStringValue("123")
		floatResult := handler.convertStringValue("1.5")

		// Then values should be converted using pattern matching
		if boolResult != false {
			t.Errorf("Expected false from pattern, got %v", boolResult)
		}
		if intResult != 123 {
			t.Errorf("Expected 123 from pattern, got %v", intResult)
		}
		if floatResult != 1.5 {
			t.Errorf("Expected 1.5 from pattern, got %v", floatResult)
		}
	})
}

func TestConfigHandler_convertStringToType(t *testing.T) {
	t.Run("ConvertsBooleanTrue", func(t *testing.T) {
		handler, _ := setupPrivateTestHandler(t)

		result := handler.convertStringToType("true", "boolean")

		if result != true {
			t.Errorf("Expected true, got %v", result)
		}
	})

	t.Run("ConvertsBooleanFalse", func(t *testing.T) {
		handler, _ := setupPrivateTestHandler(t)

		result := handler.convertStringToType("false", "boolean")

		if result != false {
			t.Errorf("Expected false, got %v", result)
		}
	})

	t.Run("ConvertsBooleanCaseInsensitive", func(t *testing.T) {
		handler, _ := setupPrivateTestHandler(t)

		result := handler.convertStringToType("TRUE", "boolean")

		if result != true {
			t.Errorf("Expected true for 'TRUE', got %v", result)
		}
	})

	t.Run("ConvertsInteger", func(t *testing.T) {
		handler, _ := setupPrivateTestHandler(t)

		result := handler.convertStringToType("42", "integer")

		if result != 42 {
			t.Errorf("Expected 42, got %v", result)
		}
	})

	t.Run("ConvertsNumber", func(t *testing.T) {
		handler, _ := setupPrivateTestHandler(t)

		result := handler.convertStringToType("3.14", "number")

		if result != 3.14 {
			t.Errorf("Expected 3.14, got %v", result)
		}
	})

	t.Run("ReturnsStringForStringType", func(t *testing.T) {
		handler, _ := setupPrivateTestHandler(t)

		result := handler.convertStringToType("test", "string")

		if result != "test" {
			t.Errorf("Expected 'test', got %v", result)
		}
	})

	t.Run("ReturnsNilForInvalidBoolean", func(t *testing.T) {
		handler, _ := setupPrivateTestHandler(t)

		result := handler.convertStringToType("not_a_bool", "boolean")

		if result != nil {
			t.Errorf("Expected nil for invalid boolean, got %v", result)
		}
	})

	t.Run("ReturnsNilForInvalidInteger", func(t *testing.T) {
		handler, _ := setupPrivateTestHandler(t)

		result := handler.convertStringToType("not_an_int", "integer")

		if result != nil {
			t.Errorf("Expected nil for invalid integer, got %v", result)
		}
	})

	t.Run("ReturnsNilForInvalidNumber", func(t *testing.T) {
		handler, _ := setupPrivateTestHandler(t)

		result := handler.convertStringToType("not_a_number", "number")

		if result != nil {
			t.Errorf("Expected nil for invalid number, got %v", result)
		}
	})

	t.Run("ReturnsNilForUnknownType", func(t *testing.T) {
		handler, _ := setupPrivateTestHandler(t)

		result := handler.convertStringToType("value", "unknown_type")

		if result != nil {
			t.Errorf("Expected nil for unknown type, got %v", result)
		}
	})
}

func TestConfigHandler_convertStringByPattern(t *testing.T) {
	t.Run("RecognizesBooleanTrue", func(t *testing.T) {
		handler, _ := setupPrivateTestHandler(t)

		result := handler.convertStringByPattern("true")

		if result != true {
			t.Errorf("Expected true, got %v", result)
		}
	})

	t.Run("RecognizesBooleanFalse", func(t *testing.T) {
		handler, _ := setupPrivateTestHandler(t)

		result := handler.convertStringByPattern("false")

		if result != false {
			t.Errorf("Expected false, got %v", result)
		}
	})

	t.Run("RecognizesInteger", func(t *testing.T) {
		handler, _ := setupPrivateTestHandler(t)

		result := handler.convertStringByPattern("42")

		if result != 42 {
			t.Errorf("Expected 42, got %v", result)
		}
	})

	t.Run("RecognizesFloat", func(t *testing.T) {
		handler, _ := setupPrivateTestHandler(t)

		result := handler.convertStringByPattern("3.14")

		if result != 3.14 {
			t.Errorf("Expected 3.14, got %v", result)
		}
	})

	t.Run("ReturnsStringWhenNoPatternMatches", func(t *testing.T) {
		handler, _ := setupPrivateTestHandler(t)

		result := handler.convertStringByPattern("regular_string")

		if result != "regular_string" {
			t.Errorf("Expected 'regular_string', got %v", result)
		}
	})
}

func TestConfigHandler_getExpectedTypeFromSchema(t *testing.T) {
	t.Run("ReturnsTypeFromSchema", func(t *testing.T) {
		handler, tmpDir := setupPrivateTestHandler(t)

		schemaDir := filepath.Join(tmpDir, "contexts", "_template")
		os.MkdirAll(schemaDir, 0755)
		schemaContent := `$schema: https://json-schema.org/draft/2020-12/schema
type: object
properties:
  bool_key:
    type: boolean
  int_key:
    type: integer
  num_key:
    type: number
`
		os.WriteFile(filepath.Join(schemaDir, "schema.yaml"), []byte(schemaContent), 0644)
		handler.LoadSchema(filepath.Join(schemaDir, "schema.yaml"))

		boolType := handler.getExpectedTypeFromSchema("bool_key")
		if boolType != "boolean" {
			t.Errorf("Expected 'boolean', got '%s'", boolType)
		}

		intType := handler.getExpectedTypeFromSchema("int_key")
		if intType != "integer" {
			t.Errorf("Expected 'integer', got '%s'", intType)
		}

		numType := handler.getExpectedTypeFromSchema("num_key")
		if numType != "number" {
			t.Errorf("Expected 'number', got '%s'", numType)
		}
	})

	t.Run("ReturnsEmptyForMissingKey", func(t *testing.T) {
		handler, tmpDir := setupPrivateTestHandler(t)

		schemaDir := filepath.Join(tmpDir, "contexts", "_template")
		os.MkdirAll(schemaDir, 0755)
		schemaContent := `$schema: https://json-schema.org/draft/2020-12/schema
type: object
properties: {}
`
		os.WriteFile(filepath.Join(schemaDir, "schema.yaml"), []byte(schemaContent), 0644)
		handler.LoadSchema(filepath.Join(schemaDir, "schema.yaml"))

		typeStr := handler.getExpectedTypeFromSchema("missing_key")

		if typeStr != "" {
			t.Errorf("Expected empty string for missing key, got '%s'", typeStr)
		}
	})

	t.Run("ReturnsEmptyWhenNoSchemaLoaded", func(t *testing.T) {
		handler, _ := setupPrivateTestHandler(t)

		typeStr := handler.getExpectedTypeFromSchema("any_key")

		if typeStr != "" {
			t.Errorf("Expected empty string when no schema, got '%s'", typeStr)
		}
	})

	t.Run("HandlesInvalidPropertiesType", func(t *testing.T) {
		// Given a config handler with invalid schema properties
		handler, _ := setupPrivateTestHandler(t)

		handler.schemaValidator = &SchemaValidator{
			Schema: map[string]any{
				"properties": "not_a_map",
			},
		}

		// When getting type for any key
		typeStr := handler.getExpectedTypeFromSchema("any_key")

		// Then empty string should be returned
		if typeStr != "" {
			t.Errorf("Expected empty for invalid properties, got '%s'", typeStr)
		}
	})

	t.Run("HandlesInvalidPropertySchema", func(t *testing.T) {
		// Given a config handler with invalid property schema
		handler, _ := setupPrivateTestHandler(t)

		handler.schemaValidator = &SchemaValidator{
			Schema: map[string]any{
				"properties": map[string]any{
					"test_key": "not_a_schema",
				},
			},
		}

		// When getting type for the key
		typeStr := handler.getExpectedTypeFromSchema("test_key")

		// Then empty string should be returned
		if typeStr != "" {
			t.Errorf("Expected empty for invalid property schema, got '%s'", typeStr)
		}
	})
}
