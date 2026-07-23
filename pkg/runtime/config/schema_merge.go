package config

import (
	"maps"
	"reflect"
)

// Schema composition for the multi-file schema layout: SchemaValidator accumulates a merged
// schema by deep-merging each loaded fragment over the previous one. The merge is pure
// (no validator state), so it lives apart from validation to keep both surfaces obvious. The
// invariants the merge preserves are operator-visible: properties union with overlay winning
// on conflict, required arrays union and de-duplicate, items merge recursively when both sides
// are object-typed, and validation keywords merge conservatively (most restrictive wins) so
// the result does not depend on the order fragments happen to load in.

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
			merged[k] = sv.mergeKeyword(k, base, v)
		}
	}

	return merged
}

// mergeProperties merges two property maps. Each property is itself a schema, so a property
// present in both fragments is merged with mergeSchema — unioning nested properties and
// applying conservative validation-keyword merging to scalar leaves. A property is replaced
// wholesale only when the two fragments give it conflicting types (a genuine redefinition).
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

		basePropMap, ok := merged[k].(map[string]any)
		if !ok {
			merged[k] = overlayPropMap
			continue
		}

		if typesConflict(basePropMap, overlayPropMap) {
			merged[k] = overlayPropMap
			continue
		}

		merged[k] = sv.mergeSchema(basePropMap, overlayPropMap)
	}

	return merged
}

// mergeItemsSchema merges two array item schemas. An item schema is a schema, so matching
// items are merged with mergeSchema; the overlay replaces the base wholesale only when the
// item types conflict.
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

	if typesConflict(baseItems, overlayItems) {
		return overlayItems
	}

	return sv.mergeSchema(baseItems, overlayItems)
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

// mergeKeyword combines a single validation keyword conservatively so the most restrictive
// constraint across fragments wins, making the merged schema independent of the order
// fragments load in: additionalProperties collapses to false once any fragment closes the
// object, minimum-style bounds keep the larger value, maximum-style bounds keep the smaller,
// and enum intersects. Any other keyword (including pattern and default) takes the overlay.
func (sv *SchemaValidator) mergeKeyword(key string, base map[string]any, overlayVal any) any {
	baseVal, ok := base[key]
	if !ok {
		return overlayVal
	}

	switch key {
	case "additionalProperties":
		if baseVal == false || overlayVal == false {
			return false
		}
		return overlayVal
	case "minimum", "exclusiveMinimum", "minLength", "minItems", "minProperties":
		return tighterBound(baseVal, overlayVal, true)
	case "maximum", "exclusiveMaximum", "maxLength", "maxItems", "maxProperties":
		return tighterBound(baseVal, overlayVal, false)
	case "enum":
		return intersectEnum(baseVal, overlayVal)
	default:
		return overlayVal
	}
}

// =============================================================================
// Helpers
// =============================================================================

// typesConflict reports whether two schema fragments assign the same node incompatible
// declared types. A missing type on either side is not a conflict, so a fragment that adds
// constraints without restating the type still merges.
func typesConflict(base, overlay map[string]any) bool {
	baseType, baseOk := base["type"].(string)
	overlayType, overlayOk := overlay["type"].(string)
	return baseOk && overlayOk && baseType != overlayType
}

// tighterBound returns the more restrictive of two numeric schema bounds: the larger when
// keepLarger is set (minimum-style keywords), the smaller otherwise (maximum-style). The
// original value is returned so its numeric type is preserved; a non-numeric operand falls
// back to the overlay.
func tighterBound(base, overlay any, keepLarger bool) any {
	bv, bok := numericValue(base)
	ov, ook := numericValue(overlay)
	if !bok || !ook {
		return overlay
	}
	if keepLarger {
		if bv >= ov {
			return base
		}
		return overlay
	}
	if bv <= ov {
		return base
	}
	return overlay
}

// intersectEnum returns the enum members common to both fragments, preserving base order. A
// non-array operand falls back to the overlay.
func intersectEnum(base, overlay any) any {
	baseSlice, bok := base.([]any)
	overlaySlice, ook := overlay.([]any)
	if !bok || !ook {
		return overlay
	}
	var merged []any
	for _, b := range baseSlice {
		for _, o := range overlaySlice {
			if enumEqual(b, o) {
				merged = append(merged, b)
				break
			}
		}
	}
	return merged
}

// enumEqual compares two enum members, treating numeric values equal across integer and
// float encodings and falling back to a deep comparison otherwise.
func enumEqual(a, b any) bool {
	if av, aok := numericValue(a); aok {
		if bv, bok := numericValue(b); bok {
			return av == bv
		}
	}
	return reflect.DeepEqual(a, b)
}

// numericValue reports the float64 value of any Go numeric type, so schema bounds compare
// regardless of the integer or float kind the YAML decoder produced.
func numericValue(v any) (float64, bool) {
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(rv.Int()), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return float64(rv.Uint()), true
	case reflect.Float32, reflect.Float64:
		return rv.Float(), true
	default:
		return 0, false
	}
}
