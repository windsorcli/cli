package config

import (
	"fmt"
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

// SensitiveRedactionMarker is the placeholder shown in place of a sensitive value in display
// output, so operators see that a field exists and is sensitive without its value being printed.
const SensitiveRedactionMarker = "<sensitive>"

// RenderValuesWithDescriptions renders context values as YAML annotated with schema descriptions.
// It walks the union of schema properties and effective values so that every configurable field
// appears in the output. Set keys render as normal YAML with description comments; unset
// schema-defined keys render as commented-out reference lines. Top-level keys are separated
// by blank lines. Keys come in header order. When schema is nil, values are rendered as plain
// YAML without comments.
func RenderValuesWithDescriptions(values map[string]any, schema map[string]any) string {
	return renderValuesBlock(values, schema, headerOrder(), "")
}

// RedactSensitiveValues replaces, in place, every existing value at a sensitive path with
// SensitiveRedactionMarker so display surfaces (e.g. windsor show values) never print secret
// material while still revealing that the field is set. A "*" path segment matches any single
// key. Absent paths are left untouched — redaction never fabricates keys. The same map is
// returned for convenience.
func RedactSensitiveValues(values map[string]any, sensitivePaths []string) map[string]any {
	for _, path := range sensitivePaths {
		redactValueAtPath(values, parsePath(path))
	}
	return values
}

// =============================================================================
// Private Functions
// =============================================================================

// redactValueAtPath overwrites the leaf addressed by segments with SensitiveRedactionMarker. A "*"
// segment matches every key at that level: as the final segment it redacts each existing value,
// otherwise it recurses into every child. Missing keys are ignored; no keys are created.
func redactValueAtPath(node any, segments []string) {
	current, ok := node.(map[string]any)
	if !ok || len(segments) == 0 {
		return
	}
	segment := segments[0]
	if len(segments) == 1 {
		if segment == "*" {
			for key := range current {
				current[key] = SensitiveRedactionMarker
			}
			return
		}
		if _, exists := current[segment]; exists {
			current[segment] = SensitiveRedactionMarker
		}
		return
	}
	if segment == "*" {
		for _, child := range current {
			redactValueAtPath(child, segments[1:])
		}
		return
	}
	if child, exists := current[segment]; exists {
		redactValueAtPath(child, segments[1:])
	}
}

// renderValuesBlock renders one level of RenderValuesWithDescriptions and recurses into objects.
// marshalKeyValue formats scalars and arrays. order sets the key order, and indent sets the depth.
// A schema key without a value renders as a commented-out key, so users can find it.
// An object without renderable sub-properties renders as one commented-out key.
func renderValuesBlock(values map[string]any, schema map[string]any, order *keyOrder, indent string) string {
	var sb strings.Builder

	var schemaProps map[string]any
	if schema != nil {
		if props, ok := schema["properties"].(map[string]any); ok {
			schemaProps = props
		}
	}

	keys := order.sort(unionKeys(values, schemaProps))
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
					sb.WriteString(renderValuesBlock(v, propSchema, order.child(key), indent+"  "))
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
			propType, _ := propSchema["type"].(string)
			if propType == "object" {
				subContent := renderValuesBlock(nil, propSchema, order.child(key), indent+"  ")
				if subContent != "" {
					if desc != "" {
						sb.WriteString(indent + "# " + desc + "\n")
					}
					sb.WriteString(indent + key + ":\n")
					sb.WriteString(subContent)
				} else {
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

// unionKeys returns the union of keys from values and schemaProps, in no fixed order.
// Either argument may be nil; duplicates are deduplicated.
func unionKeys(values, schemaProps map[string]any) []string {
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
	return keys
}
