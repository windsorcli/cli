package terraform

import (
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2/hclwrite"
)

// =============================================================================
// Test Helpers
// =============================================================================

func Test_writeVariable(t *testing.T) {
	t.Run("WritesVariableWithDescription", func(t *testing.T) {
		// Given a body and variable info with description
		file := hclwrite.NewEmptyFile()
		body := file.Body()
		variables := []VariableInfo{
			{Name: "test", Description: "Test variable"},
		}

		// When writing variable
		writeVariable(body, "test", "value", variables)

		// Then it should include description comment
		content := string(file.Bytes())
		if !strings.Contains(content, "# Test variable") {
			t.Errorf("Expected description comment, got: %s", content)
		}
	})

	t.Run("SkipsSensitiveVariable", func(t *testing.T) {
		// Given a body and variable info with sensitive flag
		file := hclwrite.NewEmptyFile()
		body := file.Body()
		variables := []VariableInfo{
			{Name: "test", Sensitive: true},
		}

		// When writing variable
		writeVariable(body, "test", "value", variables)

		// Then it should include sensitive comment and not write value
		content := string(file.Bytes())
		if !strings.Contains(content, "# test = \"(sensitive)\"") {
			t.Errorf("Expected sensitive comment, got: %s", content)
		}
		if strings.Contains(content, "test = \"value\"") {
			t.Error("Expected sensitive variable value to be skipped")
		}
	})

	t.Run("WritesStringWithNewlinesAsHeredoc", func(t *testing.T) {
		// Given a body and a string value with newlines
		file := hclwrite.NewEmptyFile()
		body := file.Body()
		variables := []VariableInfo{}

		// When writing variable with multiline string
		writeVariable(body, "test", "line1\nline2\nline3", variables)

		// Then it should use heredoc format
		content := string(file.Bytes())
		if !strings.Contains(content, "<<-EOT") && !strings.Contains(content, "<<EOF") {
			t.Errorf("Expected heredoc format, got: %s", content)
		}
	})

	t.Run("WritesMapValue", func(t *testing.T) {
		// Given a body and a map value
		file := hclwrite.NewEmptyFile()
		body := file.Body()
		variables := []VariableInfo{}

		// When writing variable with map value
		writeVariable(body, "test", map[string]any{"key": "value"}, variables)

		// Then it should format as map
		content := string(file.Bytes())
		if !strings.Contains(content, "test =") {
			t.Errorf("Expected map assignment, got: %s", content)
		}
	})

	t.Run("WritesSimpleValue", func(t *testing.T) {
		// Given a body and a simple value
		file := hclwrite.NewEmptyFile()
		body := file.Body()
		variables := []VariableInfo{}

		// When writing variable with simple value
		writeVariable(body, "test", "value", variables)

		// Then it should write attribute
		content := string(file.Bytes())
		if !strings.Contains(content, "test") {
			t.Errorf("Expected variable assignment, got: %s", content)
		}
	})
}

func Test_formatValue(t *testing.T) {
	t.Run("FormatsString", func(t *testing.T) {
		// Given a string value
		value := "test"

		// When formatting the value
		result := formatValue(value)

		// Then it should be quoted
		if result != `"test"` {
			t.Errorf("Expected '\"test\"', got '%s'", result)
		}
	})

	t.Run("FormatsEmptyStringSlice", func(t *testing.T) {
		// Given an empty string slice
		value := []string{}

		// When formatting the value
		result := formatValue(value)

		// Then it should be empty array
		if result != "[]" {
			t.Errorf("Expected '[]', got '%s'", result)
		}
	})

	t.Run("FormatsStringSlice", func(t *testing.T) {
		// Given a string slice
		value := []string{"a", "b", "c"}

		// When formatting the value
		result := formatValue(value)

		// Then it should be formatted array
		expected := `["a", "b", "c"]`
		if result != expected {
			t.Errorf("Expected '%s', got '%s'", expected, result)
		}
	})

	t.Run("FormatsEmptyAnySlice", func(t *testing.T) {
		// Given an empty any slice
		value := []any{}

		// When formatting the value
		result := formatValue(value)

		// Then it should be empty array
		if result != "[]" {
			t.Errorf("Expected '[]', got '%s'", result)
		}
	})

	t.Run("FormatsAnySlice", func(t *testing.T) {
		// Given an any slice
		value := []any{"a", 1, true}

		// When formatting the value
		result := formatValue(value)

		// Then it should be formatted array
		if !strings.Contains(result, `"a"`) {
			t.Errorf("Expected result to contain '\"a\"', got '%s'", result)
		}
	})

	t.Run("FormatsEmptyMap", func(t *testing.T) {
		// Given an empty map
		value := map[string]any{}

		// When formatting the value
		result := formatValue(value)

		// Then it should be empty object
		if result != "{}" {
			t.Errorf("Expected '{}', got '%s'", result)
		}
	})

	t.Run("FormatsMap", func(t *testing.T) {
		// Given a map
		value := map[string]any{
			"key1": "value1",
			"key2": 42,
		}

		// When formatting the value
		result := formatValue(value)

		// Then it should be formatted object
		if !strings.Contains(result, "key1") || !strings.Contains(result, "key2") {
			t.Errorf("Expected result to contain keys, got '%s'", result)
		}
	})

	t.Run("FormatsNestedMap", func(t *testing.T) {
		// Given a nested map
		value := map[string]any{
			"outer": map[string]any{
				"inner": "value",
			},
		}

		// When formatting the value
		result := formatValue(value)

		// Then it should be formatted nested object
		if !strings.Contains(result, "outer") || !strings.Contains(result, "inner") {
			t.Errorf("Expected result to contain nested keys, got '%s'", result)
		}
	})

	t.Run("FormatsNil", func(t *testing.T) {
		// Given a nil value
		var value any = nil

		// When formatting the value
		result := formatValue(value)

		// Then it should be null
		if result != "null" {
			t.Errorf("Expected 'null', got '%s'", result)
		}
	})

	t.Run("FormatsNumber", func(t *testing.T) {
		// Given a number value
		value := 42

		// When formatting the value
		result := formatValue(value)

		// Then it should be formatted as string
		if result != "42" {
			t.Errorf("Expected '42', got '%s'", result)
		}
	})

	t.Run("FormatsBoolean", func(t *testing.T) {
		// Given a boolean value
		value := true

		// When formatting the value
		result := formatValue(value)

		// Then it should be formatted as string
		if result != "true" {
			t.Errorf("Expected 'true', got '%s'", result)
		}
	})

	t.Run("FormatsMapWithEmptyValues", func(t *testing.T) {
		// Given a map with empty nested values
		value := map[string]any{
			"empty_map":   map[string]any{},
			"empty_array": []any{},
		}

		// When formatting the value
		result := formatValue(value)

		// Then it should format empty values correctly
		if !strings.Contains(result, "empty_map = {}") {
			t.Errorf("Expected result to contain 'empty_map = {}', got '%s'", result)
		}
		if !strings.Contains(result, "empty_array = []") {
			t.Errorf("Expected result to contain 'empty_array = []', got '%s'", result)
		}
	})
}
