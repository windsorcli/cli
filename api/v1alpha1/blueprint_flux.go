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

// ToFluxKustomization converts a blueprint Kustomization to a Flux Kustomization.
// It takes the default namespace for the kustomization (overridden per-kustomization
// by k.Namespace when set), the default source name to use if no source is specified,
// and the list of sources to determine the source kind (GitRepository or OCIRepository).
// A Path-less k defaults to k.Name, matching FluxSystem's own path-defaulting.
// k.TargetNamespace is passed through to spec.targetNamespace so Flux rewrites the
// namespace of every reconciled resource. DependsOn references are always resolved
// in the default namespace.
// The default Interval depends on whether k resolves to the blueprint's own repository or a
// named vendor source (see constants.FluxKustomizationInterval); mode is accepted for call-site
// stability but no longer affects it. A blueprint-level Interval override always wins.
// An optional configMaps argument (blueprint-level ConfigMaps such as values-common) is added to
// postBuild.substituteFrom so they are available to all kustomizations, matching what the provisioner applies.
// PostBuild is constructed from the kustomization's Substitutions field, except for the synthesized CRD layers
// (IsCrdLayerName(k.Name)), which skip PostBuild entirely: vendored CRDs need no substitution and their
// ${...} description text would otherwise fail envsubst (e.g. fluent-operator's ${record.to_json}).
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

	path := k.Path
	if path == "" {
		path = k.Name
	}
	path = strings.ReplaceAll(path, "\\", "/")
	if path != "kustomize" && !strings.HasPrefix(path, "kustomize/") {
		path = "kustomize/" + path
	}

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

	patches := make([]kustomize.Patch, 0, len(k.Patches))
	for _, p := range k.Patches {
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

	healthCheckExprs := toFluxHealthCheckExprs(k.HealthCheckExprs)
	healthChecks := toFluxHealthChecks(k.HealthChecks)

	var postBuild *kustomizev1.PostBuild
	if !IsCrdLayerName(k.Name) {
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
	}

	var decryption *kustomizev1.Decryption
	if k.Decryption != nil {
		decryption = &kustomizev1.Decryption{Provider: k.Decryption.Provider}
		if k.Decryption.SecretRef != nil {
			decryption.SecretRef = &meta.LocalObjectReference{Name: k.Decryption.SecretRef.Name}
		}
	}

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
