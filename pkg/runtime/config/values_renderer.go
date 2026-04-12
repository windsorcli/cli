package config

import (
	"fmt"
	"sort"
	"strings"

	"github.com/goccy/go-yaml"
)

// The ValuesRenderer produces annotated YAML for context values.
// It walks the union of schema properties and effective values so that every
// configurable field appears in the output. Keys present in the effective values
// are rendered as normal YAML; schema-defined keys absent from effective values
// are rendered as commented-out lines, making the output a complete reference template.

// =============================================================================
// Public Functions
// =============================================================================

// RenderValuesWithDescriptions renders context values as YAML annotated with schema descriptions.
// It walks the union of schema properties and effective values so that every configurable field
// appears in the output. Set keys render as normal YAML with description comments; unset
// schema-defined keys render as commented-out reference lines. Top-level keys are separated
// by blank lines. When schema is nil, values are rendered as plain YAML without comments.
func RenderValuesWithDescriptions(values map[string]any, schema map[string]any) string {
	return renderValuesBlock(values, schema, "")
}

// =============================================================================
// Private Functions
// =============================================================================

// renderValuesBlock is the recursive implementation for RenderValuesWithDescriptions.
// It accumulates output into a strings.Builder, delegating scalar and array formatting
// to marshalKeyValue. The indent parameter tracks the current nesting depth.
func renderValuesBlock(values map[string]any, schema map[string]any, indent string) string {
	var sb strings.Builder

	var schemaProps map[string]any
	if schema != nil {
		if props, ok := schema["properties"].(map[string]any); ok {
			schemaProps = props
		}
	}

	keys := sortedUnionKeys(values, schemaProps)
	for i, key := range keys {
		var val any
		var inValues bool
		if values != nil {
			val, inValues = values[key]
		}

		var desc string
		var propSchema map[string]any
		if schemaProps != nil {
			if raw, ok := schemaProps[key]; ok {
				if ps, ok := raw.(map[string]any); ok {
					propSchema = ps
					if d, ok := ps["description"].(string); ok {
						desc = strings.TrimSpace(d)
					}
				}
			}
		}

		if inValues {
			if desc != "" {
				sb.WriteString(indent + "# " + desc + "\n")
			}
			switch v := val.(type) {
			case map[string]any:
				if len(v) == 0 {
					sb.WriteString(indent + key + ": {}\n")
				} else {
					sb.WriteString(indent + key + ":\n")
					sb.WriteString(renderValuesBlock(v, propSchema, indent+"  "))
				}
			case []any:
				b, err := yaml.Marshal(val)
				if err == nil {
					sb.WriteString(indent + key + ":\n")
					for _, line := range strings.Split(strings.TrimRight(string(b), "\n"), "\n") {
						sb.WriteString(indent + "  " + line + "\n")
					}
				} else {
					sb.WriteString(indent + key + ": []\n")
				}
			default:
				_ = v
				sb.WriteString(marshalKeyValue(key, val, indent))
			}
		} else {
			// Key is defined in the schema but absent from effective values — render as a
			// commented-out reference so users can discover it without reading the schema file.
			propType, _ := propSchema["type"].(string)
			if propType == "object" {
				subContent := renderValuesBlock(nil, propSchema, indent+"  ")
				if subContent != "" {
					if desc != "" {
						sb.WriteString(indent + "# " + desc + "\n")
					}
					sb.WriteString(indent + key + ":\n")
					sb.WriteString(subContent)
				} else {
					// Object has no renderable sub-properties (e.g. additionalProperties-only) —
					// show as a single commented-out key so the field remains discoverable.
					if desc != "" {
						sb.WriteString(indent + "# " + desc + "\n")
					}
					sb.WriteString(indent + "# " + key + ":\n")
				}
			} else {
				if desc != "" {
					sb.WriteString(indent + "# " + desc + "\n")
				}
				sb.WriteString(indent + "# " + key + ":\n")
			}
		}

		if indent == "" && i < len(keys)-1 {
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// marshalKeyValue serializes a key-value pair as indented YAML, correctly handling
// multi-line scalars by prefixing every output line with indent.
func marshalKeyValue(key string, val any, indent string) string {
	b, err := yaml.Marshal(map[string]any{key: val})
	if err != nil {
		return indent + key + ": " + fmt.Sprintf("%v", val) + "\n"
	}
	result := strings.TrimRight(string(b), "\n")
	if indent == "" {
		return result + "\n"
	}
	lines := strings.Split(result, "\n")
	var sb strings.Builder
	for _, line := range lines {
		sb.WriteString(indent + line + "\n")
	}
	return sb.String()
}

// sortedUnionKeys returns the sorted union of keys from values and schemaProps.
// Either argument may be nil; duplicates are deduplicated.
func sortedUnionKeys(values, schemaProps map[string]any) []string {
	seen := make(map[string]bool)
	for k := range values {
		seen[k] = true
	}
	for k := range schemaProps {
		seen[k] = true
	}
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
