package blueprint

import (
	kustomizev1 "github.com/fluxcd/kustomize-controller/api/v1"
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
)

const deferredPlaceholder = "<deferred>"

// RenderDeferredPlaceholders rewrites deferred values to a placeholder unless raw mode is enabled.
// Deferred fields are selected from deferredPaths.
func RenderDeferredPlaceholders(resource any, raw bool, deferredPaths map[string]bool) any {
	if raw || resource == nil {
		return resource
	}
	if len(deferredPaths) == 0 {
		return resource
	}
	if rendered, ok := renderWithDeferredPaths(resource, deferredPaths); ok {
		return rendered
	}
	return resource
}

// renderWithDeferredPaths rewrites known deferred paths on supported resource types.
func renderWithDeferredPaths(resource any, deferredPaths map[string]bool) (any, bool) {
	switch r := resource.(type) {
	case *blueprintv1alpha1.Blueprint:
		if r == nil {
			return r, true
		}
		cp := r.DeepCopy()
		applyDeferredPathsToBlueprint(cp, deferredPaths)
		return cp, true
	case blueprintv1alpha1.Blueprint:
		cp := r.DeepCopy()
		applyDeferredPathsToBlueprint(cp, deferredPaths)
		return *cp, true
	case kustomizev1.Kustomization:
		cp := r.DeepCopy()
		applyDeferredPathsToFluxKustomization(cp, deferredPaths)
		return *cp, true
	case *kustomizev1.Kustomization:
		if r == nil {
			return r, true
		}
		cp := r.DeepCopy()
		applyDeferredPathsToFluxKustomization(cp, deferredPaths)
		return cp, true
	default:
		return nil, false
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
	for i := range bp.Kustomizations {
		name := bp.Kustomizations[i].Name
		if deferredPaths["kustomize."+name+".path"] {
			bp.Kustomizations[i].Path = deferredPlaceholder
		}
		for key := range bp.Kustomizations[i].Substitutions {
			if deferredPaths["kustomize."+name+".substitutions."+key] {
				bp.Kustomizations[i].Substitutions[key] = deferredPlaceholder
			}
		}
	}
}

// applyDeferredPathsToFluxKustomization rewrites deferred fields on flux kustomization output.
func applyDeferredPathsToFluxKustomization(k *kustomizev1.Kustomization, deferredPaths map[string]bool) {
	if k == nil {
		return
	}
	pathKey := "kustomize." + k.Name + ".path"
	if deferredPaths[pathKey] {
		k.Spec.Path = deferredPlaceholder
	}
}
