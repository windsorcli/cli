package blueprint

import (
	kustomizev1 "github.com/fluxcd/kustomize-controller/api/v1"
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
)

const deferredPlaceholder = "<deferred>"

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
		cp := r.DeepCopy()
		if !raw {
			applyDeferredPathsToFluxKustomization(cp, deferredPaths)
		}
		return *cp
	case *kustomizev1.Kustomization:
		if r == nil {
			return r
		}
		cp := r.DeepCopy()
		if !raw {
			applyDeferredPathsToFluxKustomization(cp, deferredPaths)
		}
		return cp
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
		if deferredPaths["kustomize."+name+".path"] {
			bp.Kustomizations[i].Path = deferredPlaceholder
		}
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
