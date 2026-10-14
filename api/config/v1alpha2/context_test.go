package v1alpha2_test

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"

	"github.com/windsorcli/cli/api/config/v1alpha2"
)

// =============================================================================
// Test Setup
// =============================================================================

// loadSchema decodes the embedded schema fragment.
func loadSchema(t *testing.T) map[string]any {
	t.Helper()
	var schema map[string]any
	if err := yaml.Unmarshal(v1alpha2.Schema(), &schema); err != nil {
		t.Fatalf("decode schema: %v", err)
	}
	return schema
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestSchema(t *testing.T) {
	t.Run("IsADraft2020Fragment", func(t *testing.T) {
		// Given the embedded schema
		schema := loadSchema(t)

		// When its dialect and root type are read
		dialect := schema["$schema"]
		rootType := schema["type"]

		// Then it is a draft 2020-12 object schema
		if dialect != "https://json-schema.org/draft/2020-12/schema" || rootType != "object" {
			t.Errorf("unexpected dialect %v or type %v", dialect, rootType)
		}
	})

	t.Run("DoesNotCloseAnyObject", func(t *testing.T) {
		// Given the embedded schema
		schema := loadSchema(t)

		// When every object node is checked for additionalProperties: false
		closed := closedPaths(schema, "")

		// Then none is closed, so blueprints can extend every object
		if len(closed) != 0 {
			t.Errorf("expected no closed objects, got %v", closed)
		}
	})

	t.Run("MatchesTheContextType", func(t *testing.T) {
		// Given the embedded schema and the Context type
		schema := loadSchema(t)

		// When the type and the schema are compared in both directions
		var mismatches []string
		compareType(reflect.TypeFor[v1alpha2.Context](), schema, "", &mismatches)

		// Then every field has a property of the matching type, and every property has a field
		if len(mismatches) != 0 {
			t.Errorf("type and schema disagree:\n  %s", strings.Join(mismatches, "\n  "))
		}
	})

	t.Run("ReturnsACopy", func(t *testing.T) {
		// Given a copy of the schema
		first := v1alpha2.Schema()

		// When the copy changes
		first[0] = 'x'

		// Then a later copy is unchanged
		if v1alpha2.Schema()[0] == 'x' {
			t.Error("expected an independent copy")
		}
	})
}

func TestPlatforms(t *testing.T) {
	t.Run("MatchesTheSchemaEnum", func(t *testing.T) {
		// Given the platform enum in the schema
		properties := loadSchema(t)["properties"].(map[string]any)
		enum := properties["platform"].(map[string]any)["enum"].([]any)

		// When it is compared with the supported platform list
		var got []string
		for _, value := range enum {
			got = append(got, value.(string))
		}

		// Then the two lists are equal, in the same order
		if !reflect.DeepEqual(got, v1alpha2.Platforms) {
			t.Errorf("schema enum %v does not match Platforms %v", got, v1alpha2.Platforms)
		}
	})
}

// =============================================================================
// Test Helpers
// =============================================================================

// compareType records every disagreement between the Go type typ and the schema node at path.
func compareType(typ reflect.Type, node map[string]any, path string, mismatches *[]string) {
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	want := schemaTypeFor(typ)
	if got, _ := node["type"].(string); got != want {
		*mismatches = append(*mismatches, label(path)+": schema type "+got+", Go type wants "+want)
		return
	}
	switch typ.Kind() {
	case reflect.Struct:
		properties, _ := node["properties"].(map[string]any)
		seen := map[string]bool{}
		for i := range typ.NumField() {
			field := typ.Field(i)
			name, _, _ := strings.Cut(field.Tag.Get("yaml"), ",")
			seen[name] = true
			child, ok := properties[name].(map[string]any)
			if !ok {
				*mismatches = append(*mismatches, label(join(path, name))+": field has no schema property")
				continue
			}
			compareType(field.Type, child, join(path, name), mismatches)
		}
		var extra []string
		for name := range properties {
			if !seen[name] {
				extra = append(extra, join(path, name))
			}
		}
		sort.Strings(extra)
		for _, p := range extra {
			*mismatches = append(*mismatches, p+": schema property has no field")
		}
	case reflect.Map:
		element := typ.Elem()
		if element.Kind() == reflect.Interface {
			return
		}
		items, ok := node["additionalProperties"].(map[string]any)
		if !ok {
			*mismatches = append(*mismatches, label(path)+": map field needs additionalProperties")
			return
		}
		compareType(element, items, join(path, "*"), mismatches)
	}
}

// schemaTypeFor returns the JSON Schema type name for a Go type.
func schemaTypeFor(typ reflect.Type) string {
	switch typ.Kind() {
	case reflect.String:
		return "string"
	case reflect.Bool:
		return "boolean"
	case reflect.Int, reflect.Int64:
		return "integer"
	case reflect.Struct, reflect.Map:
		return "object"
	default:
		return "unsupported:" + typ.Kind().String()
	}
}

// closedPaths returns the path of every object node that sets additionalProperties: false.
func closedPaths(node map[string]any, path string) []string {
	var closed []string
	if node["additionalProperties"] == false {
		closed = append(closed, label(path))
	}
	properties, _ := node["properties"].(map[string]any)
	for name, child := range properties {
		if childMap, ok := child.(map[string]any); ok {
			closed = append(closed, closedPaths(childMap, join(path, name))...)
		}
	}
	if items, ok := node["additionalProperties"].(map[string]any); ok {
		closed = append(closed, closedPaths(items, join(path, "*"))...)
	}
	return closed
}

// join appends name to a dotted path.
func join(path, name string) string {
	if path == "" {
		return name
	}
	return path + "." + name
}

// label returns path, or "(root)" for the empty path.
func label(path string) string {
	if path == "" {
		return "(root)"
	}
	return path
}
