// The flux functions expand a flux system into its tier kustomizations.
// They also convert a Kustomization to the Flux Kustomization resource.

package v1alpha1

import (
	"fmt"
	"path"
	"slices"
	"strings"

	kustomizev1 "github.com/fluxcd/kustomize-controller/api/v1"
	"github.com/fluxcd/pkg/apis/kustomize"
	meta "github.com/fluxcd/pkg/apis/meta"
	"github.com/windsorcli/cli/pkg/constants"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// =============================================================================
// Public Methods
// =============================================================================

// AllKustomizations returns a flat list of every compiled Kustomization — plain kustomize: entries
// first, then the install and resources tiers compiled from each FluxSystem. Expressions on stored
// FluxSystems are already evaluated; this method only performs name/path/dependency assembly.
func (b *Blueprint) AllKustomizations() []Kustomization {
	all := make([]Kustomization, 0, len(b.Kustomizations))
	all = append(all, b.Kustomizations...)
	for _, sys := range b.FluxSystems {
		all = append(all, compileFluxSystemTiers(sys)...)
	}
	return all
}

// TierNames returns the compiled Kustomization names this system produces (its install tier, if
// any, followed by each resources variant), in compiled order.
func (sys FluxSystem) TierNames() []string {
	tiers := compileFluxSystemTiers(sys)
	names := make([]string, len(tiers))
	for i, t := range tiers {
		names[i] = t.Name
	}
	return names
}

// EffectivePath returns the system's Path. If Path is empty, it returns the Name.
// This is the single source for the default that compileFluxSystemTiers applies to each tier.
// Use this method, not the Path field, to read a system's path outside tier compilation.
// A test assertion against a FluxSystem before tier compilation is one example.
func (sys FluxSystem) EffectivePath() string {
	if sys.Path == "" {
		return sys.Name
	}
	return sys.Path
}

// ToFluxKustomization converts a blueprint Kustomization to a Flux Kustomization in namespace.
// k.Namespace overrides namespace for the object, and DependsOn always resolves in namespace.
// A Path-less k uses k.Name as its path, and an unset Interval follows the source (see
// constants.FluxKustomizationInterval). The mode argument does not change the result.
// The first configMaps entry adds blueprint-level ConfigMaps to postBuild.substituteFrom.
// CRD layers get no PostBuild, because envsubst fails on the ${...} text in vendored CRDs.
func (k *Kustomization) ToFluxKustomization(namespace string, defaultSourceName string, sources []Source, mode constants.GitopsMode, configMaps ...map[string]map[string]string) kustomizev1.Kustomization {
	dependsOn := make([]kustomizev1.DependencyReference, len(k.DependsOn))
	for idx, dep := range k.DependsOn {
		dependsOn[idx] = kustomizev1.DependencyReference{
			Name:      dep,
			Namespace: namespace,
		}
	}

	objectNamespace := namespace
	sourceRefNamespace := ""
	if k.Namespace != "" && k.Namespace != namespace {
		objectNamespace = k.Namespace
		sourceRefNamespace = namespace
	}

	sourceName, sourceKind := k.fluxSourceRef(defaultSourceName, sources)
	path := k.FluxPath()

	interval := metav1.Duration{Duration: constants.FluxKustomizationInterval(sourceName == defaultSourceName)}
	if k.Interval != nil && k.Interval.Duration != 0 {
		interval = metav1.Duration{Duration: k.Interval.Duration}
	}

	retryInterval := metav1.Duration{Duration: constants.DefaultFluxKustomizationRetryInterval}
	if k.RetryInterval != nil && k.RetryInterval.Duration != 0 {
		retryInterval = metav1.Duration{Duration: k.RetryInterval.Duration}
	}

	timeout := metav1.Duration{Duration: constants.DefaultFluxKustomizationTimeout}
	if k.Timeout != nil && k.Timeout.Duration != 0 {
		timeout = metav1.Duration{Duration: k.Timeout.Duration}
	}

	wait := constants.DefaultFluxKustomizationWait
	if k.Wait != nil {
		wait = *k.Wait
	}
	if len(k.HealthChecks) > 0 {
		wait = false
	}

	force := constants.DefaultFluxKustomizationForce
	if k.Force != nil {
		force = *k.Force
	}

	prune := true
	if k.Prune != nil {
		prune = *k.Prune
	}

	deletionPolicy := "MirrorPrune"
	destroy := k.Destroy.ToBool()
	if destroy == nil || *destroy {
		deletionPolicy = "WaitForTermination"
	}

	patches := toFluxPatches(k.Patches)
	healthCheckExprs := toFluxHealthCheckExprs(k.HealthCheckExprs)
	healthChecks := toFluxHealthChecks(k.HealthChecks)
	postBuild := k.fluxPostBuild(configMaps...)
	decryption := toFluxDecryption(k.Decryption)

	return kustomizev1.Kustomization{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Kustomization",
			APIVersion: "kustomize.toolkit.fluxcd.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      k.Name,
			Namespace: objectNamespace,
		},
		Spec: kustomizev1.KustomizationSpec{
			SourceRef: kustomizev1.CrossNamespaceSourceReference{
				Kind:      sourceKind,
				Name:      sourceName,
				Namespace: sourceRefNamespace,
			},
			Path:             path,
			DependsOn:        dependsOn,
			Interval:         interval,
			RetryInterval:    &retryInterval,
			Timeout:          &timeout,
			Wait:             wait,
			Force:            force,
			Prune:            prune,
			HealthCheckExprs: healthCheckExprs,
			HealthChecks:     healthChecks,
			DeletionPolicy:   deletionPolicy,
			Patches:          patches,
			Components:       k.Components,
			PostBuild:        postBuild,
			TargetNamespace:  k.TargetNamespace,
			Decryption:       decryption,
		},
	}
}

// FluxPath returns the slash-separated path of k under the kustomize/ directory.
// A Path-less k uses k.Name.
func (k *Kustomization) FluxPath() string {
	path := k.Path
	if path == "" {
		path = k.Name
	}
	path = strings.ReplaceAll(path, "\\", "/")
	if path != "kustomize" && !strings.HasPrefix(path, "kustomize/") {
		path = "kustomize/" + path
	}
	return path
}

// =============================================================================
// Private Methods
// =============================================================================

// fluxSourceRef returns the source name and kind for k. An empty Source, or "template" without a
// remote template source, resolves to defaultSourceName. An oci:// source is an OCIRepository;
// every other source is a GitRepository.
func (k *Kustomization) fluxSourceRef(defaultSourceName string, sources []Source) (string, string) {
	sourceName := k.Source
	if sourceName == "" {
		sourceName = defaultSourceName
	}
	if sourceName == "template" && !HasRemoteTemplateSource(sources) {
		sourceName = defaultSourceName
	}

	sourceKind := "GitRepository"
	for _, source := range sources {
		if source.Name == sourceName && strings.HasPrefix(source.Url, "oci://") {
			sourceKind = "OCIRepository"
			break
		}
	}
	return sourceName, sourceKind
}

// fluxPostBuild returns the PostBuild substitution sources for k, or nil when there are none.
// It lists values-<name> when k has Substitutions, then each ConfigMap in the first configMaps entry.
// CRD layers always return nil.
func (k *Kustomization) fluxPostBuild(configMaps ...map[string]map[string]string) *kustomizev1.PostBuild {
	if IsCrdLayerName(k.Name) {
		return nil
	}
	var postBuild *kustomizev1.PostBuild
	if len(k.Substitutions) > 0 {
		configMapName := fmt.Sprintf("values-%s", k.Name)
		postBuild = &kustomizev1.PostBuild{
			SubstituteFrom: []kustomizev1.SubstituteReference{
				{
					Kind:     "ConfigMap",
					Name:     configMapName,
					Optional: false,
				},
			},
		}
	}
	if len(configMaps) > 0 && len(configMaps[0]) > 0 {
		if postBuild == nil {
			postBuild = &kustomizev1.PostBuild{
				SubstituteFrom: make([]kustomizev1.SubstituteReference, 0),
			}
		}
		for name := range configMaps[0] {
			postBuild.SubstituteFrom = append(postBuild.SubstituteFrom, kustomizev1.SubstituteReference{
				Kind:     "ConfigMap",
				Name:     name,
				Optional: false,
			})
		}
	}
	return postBuild
}

// =============================================================================
// Helpers
// =============================================================================

// compileFluxSystemTiers converts a pre-evaluated FluxSystem into its tier Kustomizations without
// any expression evaluation. Install compiles to "<name>-install" at "<path>/install" whenever
// declared, with or without components. A resources variant is never gated on its own component
// count either. A timeout-less install tier falls back to DefaultFluxKustomizationInstallTimeout.
// The system's Secrets attach to its namespace-owning tier — the install tier when declared, else
// the first resources tier (out[0]).
func compileFluxSystemTiers(sys FluxSystem) []Kustomization {
	name := sys.Name
	base := sys.EffectivePath()
	installName := name + "-install"

	var out []Kustomization

	if sys.Install != nil {
		k := *sys.Install.DeepCopy()
		k.Name = installName
		k.Path = path.Join(base, "install")
		k.DependsOn = slices.Clone(sys.DependsOn)
		applySystemTierDefaults(&k, sys)
		if k.Timeout == nil {
			k.Timeout = &DurationString{Duration: constants.DefaultFluxKustomizationInstallTimeout}
		}
		out = append(out, k)
	}

	for _, v := range sys.Resources {
		variantName := name + "-resources"
		if v.Name != "" {
			variantName += "-" + v.Name
		}
		k := *v.Kustomization.DeepCopy()
		k.Name = variantName
		k.Path = path.Join(base, "resources")
		if sys.Install != nil {
			k.DependsOn = append([]string{installName}, v.DependsOn...)
		} else {
			k.DependsOn = append(slices.Clone(sys.DependsOn), v.DependsOn...)
		}
		applySystemTierDefaults(&k, sys)
		out = append(out, k)
	}

	if len(sys.Secrets) > 0 && len(out) > 0 {
		out[0].Secrets = cloneSecretData(sys.Secrets)
	}
	return out
}

// applySystemTierDefaults fills a compiled tier Kustomization's Source/Enabled/Destroy from the
// owning FluxSystem when the tier didn't set its own, and clears Substitute (already merged into
// Substitutions during collection, so it carries no further meaning on the compiled tier).
func applySystemTierDefaults(k *Kustomization, sys FluxSystem) {
	if k.Source == "" {
		k.Source = sys.Source
	}
	if k.Enabled == nil {
		k.Enabled = sys.Enabled
	}
	if k.Destroy == nil {
		k.Destroy = sys.Destroy
	}
	k.Substitute = nil
}

// toFluxHealthCheckExprs converts blueprint health check expressions to their Flux equivalents,
// returning nil for an empty input so spec.healthCheckExprs is omitted from the rendered resource.
func toFluxHealthCheckExprs(exprs []HealthCheckExpr) []kustomize.CustomHealthCheck {
	var converted []kustomize.CustomHealthCheck
	for _, expr := range exprs {
		converted = append(converted, kustomize.CustomHealthCheck{
			APIVersion: expr.APIVersion,
			Kind:       expr.Kind,
			HealthCheckExpressions: kustomize.HealthCheckExpressions{
				Current:    expr.Current,
				InProgress: expr.InProgress,
				Failed:     expr.Failed,
			},
		})
	}
	return converted
}

// toFluxHealthChecks converts blueprint external health check references to their Flux
// equivalents, returning nil for an empty input so spec.healthChecks is omitted from the
// rendered resource.
func toFluxHealthChecks(refs []HealthCheckRef) []meta.NamespacedObjectKindReference {
	var converted []meta.NamespacedObjectKindReference
	for _, ref := range refs {
		converted = append(converted, meta.NamespacedObjectKindReference{
			APIVersion: ref.APIVersion,
			Kind:       ref.Kind,
			Name:       ref.Name,
			Namespace:  ref.Namespace,
		})
	}
	return converted
}

// toFluxPatches converts blueprint patches to Flux patches. It drops a patch that has neither
// inline content nor a path.
func toFluxPatches(blueprintPatches []BlueprintPatch) []kustomize.Patch {
	patches := make([]kustomize.Patch, 0, len(blueprintPatches))
	for _, p := range blueprintPatches {
		patchContent := p.Patch
		if patchContent == "" && p.Path == "" {
			continue
		}
		var target *kustomize.Selector
		if p.Target != nil {
			target = &kustomize.Selector{
				Kind:      p.Target.Kind,
				Name:      p.Target.Name,
				Namespace: p.Target.Namespace,
			}
		}
		patches = append(patches, kustomize.Patch{
			Patch:  patchContent,
			Target: target,
		})
	}
	return patches
}

// toFluxDecryption converts a blueprint Decryption to a Flux Decryption, or returns nil when d is nil.
func toFluxDecryption(d *Decryption) *kustomizev1.Decryption {
	if d == nil {
		return nil
	}
	decryption := &kustomizev1.Decryption{Provider: d.Provider}
	if d.SecretRef != nil {
		decryption.SecretRef = &meta.LocalObjectReference{Name: d.SecretRef.Name}
	}
	return decryption
}
