package config

import (
	"reflect"
	"slices"
	"strings"
	"sync"

	"github.com/goccy/go-yaml"

	cliconfig "github.com/windsorcli/cli/api/config/v1alpha2"
)

// The KeyOrder is the order in which the CLI writes context value keys.
// It provides the order of the v1alpha2 header fields at each level, read from their yaml tags.
// Header keys come first, in field order. The other keys follow in alphabetical order.
// Full values.yaml writes and the annotated values output use this order.

// =============================================================================
// Types
// =============================================================================

// keyOrder holds the header key order at one level of the values tree.
type keyOrder struct {
	keys     []string
	children map[string]*keyOrder
	elem     *keyOrder
}

// headerOrder caches the key order of the v1alpha2 header.
var headerOrder = sync.OnceValue(func() *keyOrder {
	return keyOrderOf(reflect.TypeFor[cliconfig.Context]())
})

// =============================================================================
// Private Methods
// =============================================================================

// sort returns keys with the header keys first, in field order, and the other keys in
// alphabetical order. A nil keyOrder sorts every key in alphabetical order.
func (o *keyOrder) sort(keys []string) []string {
	var header, rest []string
	present := make(map[string]bool, len(keys))
	for _, key := range keys {
		present[key] = true
	}
	if o != nil {
		for _, key := range o.keys {
			if present[key] {
				header = append(header, key)
				delete(present, key)
			}
		}
	}
	for key := range present {
		rest = append(rest, key)
	}
	slices.Sort(rest)
	return append(header, rest...)
}

// child returns the key order under key, or nil when the header does not type that key.
func (o *keyOrder) child(key string) *keyOrder {
	if o == nil {
		return nil
	}
	if c, ok := o.children[key]; ok {
		return c
	}
	return o.elem
}

// =============================================================================
// Helpers
// =============================================================================

// keyOrderOf builds the key order of typ. A struct gives its yaml tag names in field order. A map
// with a struct element gives the element's order for every key. Other types give nil.
func keyOrderOf(typ reflect.Type) *keyOrder {
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	switch typ.Kind() {
	case reflect.Struct:
		order := &keyOrder{children: map[string]*keyOrder{}}
		for i := range typ.NumField() {
			field := typ.Field(i)
			name, _, _ := strings.Cut(field.Tag.Get("yaml"), ",")
			if name == "" || name == "-" {
				continue
			}
			order.keys = append(order.keys, name)
			if child := keyOrderOf(field.Type); child != nil {
				order.children[name] = child
			}
		}
		return order
	case reflect.Map:
		if elem := keyOrderOf(typ.Elem()); elem != nil {
			return &keyOrder{elem: elem}
		}
	}
	return nil
}

// orderedValues converts values into a yaml.MapSlice in header order, at every level of nested
// maps. Values that are not maps keep their content.
func orderedValues(values map[string]any, order *keyOrder) yaml.MapSlice {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	ordered := make(yaml.MapSlice, 0, len(values))
	for _, key := range order.sort(keys) {
		value := values[key]
		if nested, ok := value.(map[string]any); ok {
			value = orderedValues(nested, order.child(key))
		}
		ordered = append(ordered, yaml.MapItem{Key: key, Value: value})
	}
	return ordered
}
