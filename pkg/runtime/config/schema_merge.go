package config

import "maps"

// Schema composition for the multi-file schema layout: SchemaValidator accumulates a merged
// schema by deep-merging each loaded fragment over the previous one. The merge is pure
// (no validator state), so it lives apart from validation to keep both surfaces obvious. The
// invariants the merge preserves are operator-visible: properties union with overlay winning
// on conflict, required arrays union and de-duplicate, items merge recursively when both sides
// are object-typed.

// =============================================================================
// Private Methods
// =============================================================================

// mergeSchema merges two schemas, with the overlay schema's properties overriding the base
// schema's properties. The merge is deep for nested objects, and later schemas take precedence
// for conflicting properties. Required fields are merged by combining arrays and removing
// duplicates.
func (sv *SchemaValidator) mergeSchema(base, overlay map[string]any) map[string]any {
	merged := make(map[string]any)

	for k, v := range base {
		merged[k] = v
	}

	for k, v := range overlay {
		switch k {
		case "properties":
			merged[k] = sv.mergeProperties(base["properties"], v)
		case "required":
			merged[k] = sv.mergeRequired(base["required"], v)
		case "items":
			merged[k] = sv.mergeItemsSchema(base["items"], v)
		default:
			merged[k] = v
		}
	}

	return merged
}

// mergeProperties merges two property maps, with overlay properties overriding base properties.
// Nested object properties are merged recursively.
func (sv *SchemaValidator) mergeProperties(base, overlay any) map[string]any {
	merged := make(map[string]any)

	baseProps, ok := base.(map[string]any)
	if ok {
		maps.Copy(merged, baseProps)
	}

	overlayProps, ok := overlay.(map[string]any)
	if !ok {
		return merged
	}

	for k, overlayProp := range overlayProps {
		overlayPropMap, ok := overlayProp.(map[string]any)
		if !ok {
			merged[k] = overlayProp
			continue
		}

		baseProp, exists := merged[k]
		if !exists {
			merged[k] = overlayPropMap
			continue
		}

		basePropMap, ok := baseProp.(map[string]any)
		if !ok {
			merged[k] = overlayPropMap
			continue
		}

		if baseType, ok := basePropMap["type"].(string); ok && baseType == "object" {
			if overlayType, ok := overlayPropMap["type"].(string); ok && overlayType == "object" {
				mergedProp := make(map[string]any)
				for bk, bv := range basePropMap {
					mergedProp[bk] = bv
				}
				for ok, ov := range overlayPropMap {
					switch ok {
					case "properties":
						mergedProp[ok] = sv.mergeProperties(basePropMap["properties"], ov)
					case "required":
						mergedProp[ok] = sv.mergeRequired(basePropMap["required"], ov)
					case "items":
						mergedProp[ok] = sv.mergeItemsSchema(basePropMap["items"], ov)
					default:
						mergedProp[ok] = ov
					}
				}
				merged[k] = mergedProp
			} else {
				merged[k] = overlayPropMap
			}
		} else {
			merged[k] = overlayPropMap
		}
	}

	return merged
}

// mergeItemsSchema merges two array item schemas, with overlay properties overriding base
// properties. If both items are object types, they are merged recursively similar to
// properties.
func (sv *SchemaValidator) mergeItemsSchema(base, overlay any) map[string]any {
	overlayItems, ok := overlay.(map[string]any)
	if !ok {
		if baseItems, ok := base.(map[string]any); ok {
			return baseItems
		}
		return make(map[string]any)
	}

	baseItems, ok := base.(map[string]any)
	if !ok {
		return overlayItems
	}

	baseType, baseIsObject := baseItems["type"].(string)
	overlayType, overlayIsObject := overlayItems["type"].(string)

	if baseIsObject && baseType == "object" && overlayIsObject && overlayType == "object" {
		mergedItems := make(map[string]any)
		for k, v := range baseItems {
			mergedItems[k] = v
		}
		for k, v := range overlayItems {
			switch k {
			case "properties":
				mergedItems[k] = sv.mergeProperties(baseItems["properties"], v)
			case "required":
				mergedItems[k] = sv.mergeRequired(baseItems["required"], v)
			case "items":
				mergedItems[k] = sv.mergeItemsSchema(baseItems["items"], v)
			default:
				mergedItems[k] = v
			}
		}
		return mergedItems
	}

	return overlayItems
}

// mergeRequired merges two required field arrays, combining them and removing duplicates.
func (sv *SchemaValidator) mergeRequired(base, overlay any) []any {
	requiredMap := make(map[string]bool)
	var merged []any

	if baseSlice, ok := base.([]any); ok {
		for _, req := range baseSlice {
			if reqStr, ok := req.(string); ok {
				if !requiredMap[reqStr] {
					requiredMap[reqStr] = true
					merged = append(merged, req)
				}
			}
		}
	}

	if overlaySlice, ok := overlay.([]any); ok {
		for _, req := range overlaySlice {
			if reqStr, ok := req.(string); ok {
				if !requiredMap[reqStr] {
					requiredMap[reqStr] = true
					merged = append(merged, req)
				}
			}
		}
	}

	return merged
}
