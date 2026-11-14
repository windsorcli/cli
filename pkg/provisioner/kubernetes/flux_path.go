// The flux path helper converts a blueprint kustomization to a Flux Kustomization for a project.
// It provides FluxKustomization, which maps the kustomize/ path of the project's own git source to the configured kustomize directory.
// A bundle and every remote source keep the standard layout, so their paths stay as the blueprint names them.
// Every caller that builds or reads a Flux Kustomization uses this helper, so the cluster and the local plan agree.

package kubernetes

import (
	kustomizev1 "github.com/fluxcd/kustomize-controller/api/v1"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/runtime/paths"
)

// =============================================================================
// Public Methods
// =============================================================================

// FluxKustomization converts k with Kustomization.ToFluxKustomization. When the result reads the
// default source as a GitRepository, the source is the project itself, and spec.path moves from
// kustomize/... to the kustomize directory of resolver. A nil resolver leaves the path as it is.
func FluxKustomization(k *blueprintv1alpha1.Kustomization, resolver *paths.Resolver, namespace, defaultSourceName string, sources []blueprintv1alpha1.Source, mode constants.GitopsMode, configMaps ...map[string]map[string]string) kustomizev1.Kustomization {
	flux := k.ToFluxKustomization(namespace, defaultSourceName, sources, mode, configMaps...)
	if resolver != nil && flux.Spec.SourceRef.Kind == "GitRepository" && flux.Spec.SourceRef.Name == defaultSourceName {
		flux.Spec.Path = resolver.RebaseKustomize(flux.Spec.Path)
	}
	return flux
}
