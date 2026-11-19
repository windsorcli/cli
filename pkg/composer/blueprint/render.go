// The RenderForDisplay function prepares a composed resource for CLI output.
// It provides a copy of the resource without the fields that are not for display.
// It replaces deferred values with a placeholder unless the caller asks for raw output.
// CollapseLongStrings shortens long strings for YAML display.
// A resource type that it does not support passes through unchanged.

package blueprint

import (
	"fmt"
	"reflect"
	"strings"

	kustomizev1 "github.com/fluxcd/kustomize-controller/api/v1"
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
)

// =============================================================================
// Constants
// =============================================================================

// deferredPlaceholder replaces a deferred value in display output.
const deferredPlaceholder = "<deferred>"

// MaxDisplayLines is the longest string, in lines, that YAML display output prints in full.
const MaxDisplayLines = 20

// longStringPlaceholder replaces a string longer than MaxDisplayLines. The verb is the line count.
const longStringPlaceholder = "<multi-line string, %d lines; use --full to expand>"

// =============================================================================
// Helpers
// =============================================================================

// RenderForDisplay returns a copy of resource ready for CLI output: composed-blueprint fields not
// meant for display (Messages, resolved separately by GenerateResolved for bootstrap/up to print)
// are cleared, and unless raw is true, deferred values named in deferredPaths are rewritten to a
// placeholder. Unsupported resource types pass through unchanged.
func RenderForDisplay(resource any, raw bool, deferredPaths map[string]bool) any {
	switch r := resource.(type) {
	case *blueprintv1alpha1.Blueprint:
		if r == nil {
			return r
		}
		cp := r.DeepCopy()
		cp.Messages = nil
		if !raw {
			applyDeferredPathsToBlueprint(cp, deferredPaths)
		}
		return cp
	case blueprintv1alpha1.Blueprint:
		cp := r.DeepCopy()
		cp.Messages = nil
		if !raw {
			applyDeferredPathsToBlueprint(cp, deferredPaths)
		}
		return *cp
	case kustomizev1.Kustomization:
		return *r.DeepCopy()
	case *kustomizev1.Kustomization:
		if r == nil {
			return r
		}
		return r.DeepCopy()
	default:
		return resource
	}
}

// applyDeferredPathsToBlueprint rewrites deferred terraform input/substitution/path values.
func applyDeferredPathsToBlueprint(bp *blueprintv1alpha1.Blueprint, deferredPaths map[string]bool) {
	if bp == nil {
		return
	}
	for i := range bp.TerraformComponents {
		componentID := bp.TerraformComponents[i].GetID()
		for key := range bp.TerraformComponents[i].Inputs {
			if deferredPaths["terraform."+componentID+".inputs."+key] {
				bp.TerraformComponents[i].Inputs[key] = deferredPlaceholder
			}
		}
	}
	for key := range bp.Substitutions {
		if deferredPaths["substitutions."+key] {
			bp.Substitutions[key] = deferredPlaceholder
		}
	}
	for name, configMap := range bp.ConfigMaps {
		for key := range configMap {
			if deferredPaths["configmaps."+name+"."+key] {
				bp.ConfigMaps[name][key] = deferredPlaceholder
			}
		}
	}
	for i := range bp.Kustomizations {
		name := bp.Kustomizations[i].Name
		for key := range bp.Kustomizations[i].Substitutions {
			if deferredPaths["kustomize."+name+".substitutions."+key] {
				bp.Kustomizations[i].Substitutions[key] = deferredPlaceholder
			}
		}
	}
	for i := range bp.FluxSystems {
		sys := &bp.FluxSystems[i]
		if sys.Install != nil {
			for key := range sys.Install.Substitutions {
				if deferredPaths["flux."+sys.Name+".install.substitutions."+key] {
					sys.Install.Substitutions[key] = deferredPlaceholder
				}
			}
		}
		for j := range sys.Resources {
			resourcesPrefix := fluxResourcesSubstitutionPrefix(sys.Name, sys.Resources[j].Name)
			for key := range sys.Resources[j].Substitutions {
				if deferredPaths[resourcesPrefix+key] {
					sys.Resources[j].Substitutions[key] = deferredPlaceholder
				}
			}
		}
	}
}

// CollapseLongStrings replaces every string value in resource that has more than
// MaxDisplayLines lines with a placeholder that names the line count and the --full flag.
// It walks structs, maps, slices, pointers and interfaces, and it edits resource in place.
// Pass a copy that RenderForDisplay returned, never the composed blueprint. A resource that is
// not a pointer passes through unchanged.
func CollapseLongStrings(resource any) any {
	v := reflect.ValueOf(resource)
	if v.Kind() != reflect.Ptr || v.IsNil() {
		return resource
	}
	collapseValue(v.Elem())
	return resource
}

// collapseValue rewrites long strings inside v. The caller MUST pass a settable value.
func collapseValue(v reflect.Value) {
	switch v.Kind() {
	case reflect.String:
		if v.CanSet() {
			if n := countLines(v.String()); n > MaxDisplayLines {
				v.SetString(fmt.Sprintf(longStringPlaceholder, n))
			}
		}
	case reflect.Ptr:
		if !v.IsNil() {
			collapseValue(v.Elem())
		}
	case reflect.Interface:
		if v.IsNil() || !v.CanSet() {
			return
		}
		cp := reflect.New(v.Elem().Type()).Elem()
		cp.Set(v.Elem())
		collapseValue(cp)
		v.Set(cp)
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if v.Field(i).CanSet() {
				collapseValue(v.Field(i))
			}
		}
	case reflect.Slice:
		for i := 0; i < v.Len(); i++ {
			collapseValue(v.Index(i))
		}
	case reflect.Map:
		for _, key := range v.MapKeys() {
			cp := reflect.New(v.Type().Elem()).Elem()
			cp.Set(v.MapIndex(key))
			collapseValue(cp)
			v.SetMapIndex(key, cp)
		}
	}
}

// countLines returns the number of lines in s, ignoring one trailing newline.
func countLines(s string) int {
	return strings.Count(strings.TrimSuffix(s, "\n"), "\n") + 1
}
