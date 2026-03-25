package config

import "strings"

// The ConfigHelpers are shared low-level map and path utilities for config operations.
// They provide nested map traversal, existence checks, and path parsing/mutation primitives,
// The ConfigHelpers keep repeated map-path manipulation logic centralized and reusable,
// and support accessor, resolver, and persistence workflows with consistent behavior.

// =============================================================================
// Helpers
// =============================================================================

// getValueByPathFromMap returns the value in a nested map[string]any at the location specified by the pathKeys slice.
// It traverses the map according to the keys, returning the value found at the leaf, or nil if any key is missing or the value is not a map.
func getValueByPathFromMap(data map[string]any, pathKeys []string) any {
	if len(pathKeys) == 0 {
		return nil
	}

	current := any(data)
	for _, key := range pathKeys {
		if m, ok := current.(map[string]any); ok {
			val, exists := m[key]
			if !exists {
				return nil
			}
			current = val
		} else {
			return nil
		}
	}

	return current
}

// hasValueAtPath returns true when the full key path exists in the provided map.
func hasValueAtPath(data map[string]any, pathKeys []string) bool {
	if len(pathKeys) == 0 {
		return false
	}
	current := any(data)
	for _, key := range pathKeys {
		next, ok := current.(map[string]any)
		if !ok {
			return false
		}
		val, exists := next[key]
		if !exists {
			return false
		}
		current = val
	}
	return true
}

// setValueInMap sets a value in a nested map structure at the specified path, creating intermediate maps as needed.
func setValueInMap(data map[string]any, pathKeys []string, value any) {
	if len(pathKeys) == 0 {
		return
	}

	if len(pathKeys) == 1 {
		data[pathKeys[0]] = value
		return
	}

	current := data
	for i := 0; i < len(pathKeys)-1; i++ {
		key := pathKeys[i]

		if existing, ok := current[key]; ok {
			if existingMap, ok := existing.(map[string]any); ok {
				current = existingMap
			} else {
				newMap := make(map[string]any)
				current[key] = newMap
				current = newMap
			}
		} else {
			newMap := make(map[string]any)
			current[key] = newMap
			current = newMap
		}
	}

	current[pathKeys[len(pathKeys)-1]] = value
}

// parsePath splits a hierarchical path string into its individual key segments.
// It supports dotted notation and bracket notation for keys, returning a slice of key strings.
// For example, "foo.bar[baz]" would be parsed into []string{"foo", "bar", "baz"}.
func parsePath(path string) []string {
	var keys []string
	var currentKey strings.Builder
	inBracket := false

	for _, char := range path {
		switch char {
		case '.':
			if !inBracket {
				if currentKey.Len() > 0 {
					keys = append(keys, currentKey.String())
					currentKey.Reset()
				}
			} else {
				currentKey.WriteRune(char)
			}
		case '[':
			inBracket = true
			if currentKey.Len() > 0 {
				keys = append(keys, currentKey.String())
				currentKey.Reset()
			}
		case ']':
			inBracket = false
		default:
			currentKey.WriteRune(char)
		}
	}

	if currentKey.Len() > 0 {
		keys = append(keys, currentKey.String())
	}

	return keys
}
