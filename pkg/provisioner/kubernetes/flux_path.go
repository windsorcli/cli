// The flux path helper converts a blueprint kustomization to a Flux Kustomization for a project.
// It provides FluxKustomization, which puts the base directory in front of the path of the project's own git source.
// A bundle and every remote source keep the standard layout, so their paths stay as the blueprint names them.
// Every caller that builds or reads a Flux Kustomization uses this helper, so the cluster and the local plan agree.

package kubernetes

import (
	"path"

	kustomizev1 "github.com/fluxcd/kustomize-controller/api/v1"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/constants"
)

// =============================================================================
// Public Methods
// =============================================================================

// FluxKustomization converts k with Kustomization.ToFluxKustomization. When the result reads the
// default source as a GitRepository, the source is the project itself, and basePrefix goes in front
// of spec.path. basePrefix is the base directory relative to the project root, and is empty for none.
func FluxKustomization(k *blueprintv1alpha1.Kustomization, basePrefix, namespace, defaultSourceName string, sources []blueprintv1alpha1.Source, mode constants.GitopsMode, configMaps ...map[string]map[string]string) kustomizev1.Kustomization {
	flux := k.ToFluxKustomization(namespace, defaultSourceName, sources, mode, configMaps...)
	if basePrefix != "" && flux.Spec.SourceRef.Kind == "GitRepository" && flux.Spec.SourceRef.Name == defaultSourceName {
		flux.Spec.Path = path.Join(basePrefix, flux.Spec.Path)
	}
	return flux
}
