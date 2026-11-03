// Package v1alpha1 contains types for the v1alpha1 API group
// +groupName=blueprints.windsorcli.dev
package v1alpha1

import (
	"maps"
	"slices"
	"strings"

	"github.com/fluxcd/pkg/apis/kustomize"
)

// =============================================================================
// Constants
// =============================================================================

// CrdLayerName is the base name of the synthesized kustomizations that install the vendored CRD
// layer. One CRD kustomization is materialized per source that carries CRDs: the layer for the
// default/project source takes this name verbatim, and a layer for a named source takes
// "crds-<source>" (see CrdKustomizationName). The stack depends on these layers by name, and
// ToFluxKustomization keys its PostBuild skip on them (see IsCrdLayerName) — so the "crds" name and
// the "crds-" namespace are reserved: the composer rejects user-authored kustomizations that claim
// them.
const CrdLayerName = "crds"

// IsCrdLayerName reports whether name belongs to the synthesized CRD layer namespace: the base
// "crds" name or any per-source "crds-<source>" name.
func IsCrdLayerName(name string) bool {
	return name == CrdLayerName || strings.HasPrefix(name, CrdLayerName+"-")
}

// =============================================================================
// Types
// =============================================================================

// SecretEntry is one `secrets:` entry on a flux system tier: the data that backs a Kubernetes Secret
// plus the namespaces it is placed into. Empty Namespaces places the Secret in the single namespace
// the owning kustomization creates. A named namespace must be one that kustomization created, so one
// entry can fan out to several namespaces of a system such as pki.
// Data values reference schema properties, resolve at placement, and never render in plaintext.
// An empty Type comes from the data keys: a ".dockerconfigjson" key, or "docker-username" plus
// "docker-password" (and an optional "docker-server", default "ghcr.io"), makes a
// kubernetes.io/dockerconfigjson Secret. Any other data makes an Opaque Secret.
type SecretEntry struct {
	// Namespaces are the namespaces to place this Secret into.
	Namespaces []string `yaml:"namespaces,omitempty"`

	// Data maps each Secret data key to a reference to a schema property.
	Data map[string]string `yaml:"data,omitempty"`

	// Type is the Kubernetes Secret type, such as "kubernetes.io/tls" for a Gateway listener.
	Type string `yaml:"type,omitempty"`
}

// DeepCopy returns a deep copy of the SecretEntry, sharing no slice or map with the original.
func (s SecretEntry) DeepCopy() SecretEntry {
	return SecretEntry{
		Namespaces: slices.Clone(s.Namespaces),
		Data:       maps.Clone(s.Data),
		Type:       s.Type,
	}
}

// Decryption configures in-cluster decryption of a Kustomization's manifests, mapping to Flux's
// spec.decryption. When set, kustomize-controller decrypts encrypted files in the source with the
// referenced key Secret during reconciliation; leaving it nil keeps Flux's default of no decryption.
type Decryption struct {
	// Provider is the decryption backend, e.g. "sops".
	Provider string `yaml:"provider,omitempty"`

	// SecretRef names the in-cluster Secret holding the decryption key.
	SecretRef *DecryptionSecretRef `yaml:"secretRef,omitempty"`
}

// DecryptionSecretRef names the Secret that holds the decryption key material, resolved in the
// Kustomization's own namespace.
type DecryptionSecretRef struct {
	// Name is the Secret's name.
	Name string `yaml:"name,omitempty"`
}

// DeepCopy returns a deep copy of the Decryption, sharing no pointer with the original.
func (d *Decryption) DeepCopy() *Decryption {
	if d == nil {
		return nil
	}
	c := &Decryption{Provider: d.Provider}
	if d.SecretRef != nil {
		c.SecretRef = &DecryptionSecretRef{Name: d.SecretRef.Name}
	}
	return c
}

// HealthCheckExpr evaluates the health of a custom resource with CEL, mapping to an entry in Flux's
// spec.healthCheckExprs. It gates a waiting Kustomization on resources kstatus reports Current
// immediately — those exposing no status.observedGeneration and no Reconciling/Stalled condition.
type HealthCheckExpr struct {
	// APIVersion of the custom resource under evaluation.
	APIVersion string `yaml:"apiVersion,omitempty"`

	// Kind of the custom resource under evaluation.
	Kind string `yaml:"kind,omitempty"`

	// Current is the CEL expression matching the resource's desired state.
	Current string `yaml:"current,omitempty"`

	// InProgress is the CEL expression matching a resource still converging.
	InProgress string `yaml:"inProgress,omitempty"`

	// Failed is the CEL expression matching a resource that failed to converge.
	Failed string `yaml:"failed,omitempty"`
}

// HealthCheckRef names an external resource a Kustomization waits on before Ready, mirroring
// Flux's spec.healthChecks. Setting any entry forces Wait to false and disables HealthCheckExprs,
// since Flux only evaluates those when Wait is true.
type HealthCheckRef struct {
	// APIVersion of the resource to wait on.
	APIVersion string `yaml:"apiVersion,omitempty"`

	// Kind of the resource to wait on.
	Kind string `yaml:"kind,omitempty"`

	// Name of the resource to wait on.
	Name string `yaml:"name,omitempty"`

	// Namespace of the resource to wait on. Defaults to the Kustomization's own namespace.
	Namespace string `yaml:"namespace,omitempty"`
}

// Blueprint is a configuration blueprint for initializing a project.
// Crds install from the default source into the "crds" kustomization ahead of the stack. CRDs of an
// OCI source belong to that Source instead. Substitutions may use expressions such as "${dns.domain}"
// and go into values-common for every kustomization. TestRunner sets Config so a test case can assert
// on it. Production composition and windsor show leave Config unset.
type Blueprint struct {
	// Kind is the blueprint type, following Kubernetes conventions.
	Kind string `yaml:"kind"`

	// ApiVersion is the API schema version of the blueprint.
	ApiVersion string `yaml:"apiVersion"`

	// Metadata includes the blueprint's name and description.
	Metadata Metadata `yaml:"metadata"`

	// Backend names the terraform component that terminates the backend's component chain.
	Backend string `yaml:"backend,omitempty"`

	// Repository details the source repository of the blueprint.
	Repository Repository `yaml:"repository,omitempty"`

	// Sources are external resources referenced by the blueprint.
	Sources []Source `yaml:"sources,omitempty"`

	// TerraformComponents are Terraform modules in the blueprint.
	TerraformComponents []TerraformComponent `yaml:"terraform,omitempty"`

	// Crds lists the vendored CRD references, such as "cert-manager-1.16.2", from the default source.
	Crds []string `yaml:"crds,omitempty"`

	// Kustomizations are plain Flux Kustomization configs (the kustomize: passthrough).
	Kustomizations []Kustomization `yaml:"kustomize,omitempty"`

	// FluxSystems are the flux: entries, each compiled to install and resources Kustomizations.
	FluxSystems []FluxSystem `yaml:"flux,omitempty"`

	// Substitutions are key/value pairs, evaluated with facet scope, that go into values-common.
	Substitutions map[string]string `yaml:"substitutions,omitempty"`

	// ConfigMaps are standalone ConfigMaps that every kustomization uses for PostBuild substitution.
	ConfigMaps map[string]map[string]string `yaml:"configMaps,omitempty"`

	// Messages are post-run notes from active facets, which GenerateResolved evaluates against composed scope.
	Messages []Message `yaml:"messages,omitempty"`

	// Config maps each facet config block name to its evaluated value.
	Config map[string]any `yaml:"config,omitempty"`
}

// CrdKustomizationName returns the name of the kustomization the provisioner synthesizes for a CRD
// source: "crds" for the default/project source (empty name) and "crds-<source>" for a named source.
// The barrier makes every root kustomization depend on this name so the stack waits on the CRDs.
func CrdKustomizationName(source string) string {
	if source == "" {
		return CrdLayerName
	}
	return CrdLayerName + "-" + source
}

// Metadata describes a blueprint.
type Metadata struct {
	// Name is the blueprint's unique identifier.
	Name string `yaml:"name"`

	// Description is a brief overview of the blueprint.
	Description string `yaml:"description,omitempty"`
}

// Repository contains source code repository info.
type Repository struct {
	// Url is the repository location.
	Url string `yaml:"url,omitempty"`

	// Ref details the branch, tag, or commit to use.
	Ref Reference `yaml:"ref,omitempty"`

	// SecretName is the secret for repository access.
	SecretName *string `yaml:"secretName,omitempty"`
}

// Source is an external resource referenced by a blueprint.
// Install defaults to false. Facets can set Install with an expression, such as "${some.condition ?? true}".
// The composer fills Crds from the source's facets. When Install is true, the provisioner installs
// them in a background "crds-<name>" kustomization ahead of the stack.
type Source struct {
	// Name identifies the source.
	Name string `yaml:"name"`

	// Url is the source location.
	Url string `yaml:"url,omitempty"`

	// PathPrefix is a prefix to the source path.
	PathPrefix string `yaml:"pathPrefix,omitempty"`

	// Ref details the branch, tag, or commit to use.
	Ref Reference `yaml:"ref,omitempty"`

	// SecretName is the secret for source access.
	SecretName string `yaml:"secretName,omitempty"`

	// Install merges the source's components into the final blueprint.
	Install *BoolExpression `yaml:"install,omitempty"`

	// Crds lists the CRD references this source vendors at <source>/kustomize/crds/<ref>.
	Crds []string `yaml:"crds,omitempty"`
}

// IsLocalTemplateSource returns true when the source is the template source with no URL (local context template).
func IsLocalTemplateSource(source Source) bool {
	return source.Name == "template" && source.Url == ""
}

// HasRemoteTemplateSource returns true when sources include a template source with a non-empty URL.
func HasRemoteTemplateSource(sources []Source) bool {
	for _, s := range sources {
		if s.Name == "template" && s.Url != "" {
			return true
		}
	}
	return false
}

// Reference details a specific version or state of a repository or source.
type Reference struct {
	// Branch to use.
	Branch string `yaml:"branch,omitempty"`

	// Tag to use.
	Tag string `yaml:"tag,omitempty"`

	// SemVer to use.
	SemVer string `yaml:"semver,omitempty"`

	// Name of the reference.
	Name string `yaml:"name,omitempty"`

	// Commit hash to use.
	Commit string `yaml:"commit,omitempty"`
}

// TerraformComponent defines a Terraform module in a blueprint. Name, when set, is the component ID
// instead of Path. An Inputs value with ${} syntax, such as "${cluster.name}", is an expression, and
// any other value is a literal. Inputs go into tfvars files and not into the context blueprint.yaml.
// Destroy and Enabled default to true. Parallelism maps to the terraform -parallelism flag.
// Facets can set Destroy, Enabled, and Parallelism with expressions, such as "${cluster.destroy ?? true}".
type TerraformComponent struct {
	// Name of the terraform component, which is its ID when set.
	Name string `yaml:"name,omitempty"`

	// Source of the Terraform module.
	Source string `yaml:"source,omitempty"`

	// Path of the Terraform module.
	Path string `yaml:"path"`

	// FullPath is the complete path, not serialized to YAML.
	FullPath string `yaml:"-"`

	// DependsOn lists dependencies of this terraform component.
	DependsOn []string `yaml:"dependsOn,omitempty"`

	// Inputs are configuration values for the module.
	Inputs map[string]any `yaml:"inputs,omitempty"`

	// Destroy determines if the component is destroyed during down operations.
	Destroy *BoolExpression `yaml:"destroy,omitempty"`

	// Parallelism limits the number of concurrent operations as Terraform walks the graph.
	Parallelism *IntExpression `yaml:"parallelism,omitempty"`

	// Enabled determines if the component is included in the final blueprint.
	Enabled *BoolExpression `yaml:"enabled,omitempty"`

	// InputOrigins maps each input key to the facet file that defined it, to resolve relative paths.
	InputOrigins map[string]string `yaml:"-"`
}

// GetID returns the unique identifier for this terraform component.
// If Name is provided, it returns Name; otherwise, it returns Path.
func (t *TerraformComponent) GetID() string {
	if t.Name != "" {
		return t.Name
	}
	return t.Path
}

// DeepCopy creates a deep copy of the TerraformComponent object.
func (t *TerraformComponent) DeepCopy() *TerraformComponent {
	if t == nil {
		return nil
	}

	dependsOnCopy := make([]string, len(t.DependsOn))
	copy(dependsOnCopy, t.DependsOn)

	return &TerraformComponent{
		Name:         t.Name,
		Source:       t.Source,
		Path:         t.Path,
		FullPath:     t.FullPath,
		DependsOn:    dependsOnCopy,
		Inputs:       maps.Clone(t.Inputs),
		Destroy:      t.Destroy.DeepCopy(),
		Parallelism:  t.Parallelism.DeepCopy(),
		Enabled:      t.Enabled.DeepCopy(),
		InputOrigins: maps.Clone(t.InputOrigins),
	}
}

// BlueprintPatch represents a patch in the blueprint format.
// This is converted to kustomize.Patch during processing.
// Supports both blueprint format (Path) and Flux format (Patch + Target).
type BlueprintPatch struct {
	// Path to the patch file relative to the kustomization (blueprint format).
	Path string `yaml:"path,omitempty"`

	// Patch content as YAML string (Flux format).
	Patch string `yaml:"patch,omitempty"`

	// Target selector for the patch (Flux format).
	Target *kustomize.Selector `yaml:"target,omitempty"`
}

// Kustomization represents a kustomization configuration.
// An unset Namespace uses the gitops namespace. DependsOn always resolves in the gitops namespace,
// so cross-namespace dependencies are not supported. Timeout governs install and reconcile waits,
// and seeds the destroy wait when DeleteTimeout is unset. Destroy and Enabled default to true.
// A DestroyOnly kustomization runs only during destroy, before the regular kustomizations.
// Substitute and Substitutions merge during composition and go into ConfigMaps, not the context blueprint.yaml.
type Kustomization struct {
	// Name of the kustomization, omitted when empty so an unnamed resources variant emits no name.
	Name string `yaml:"name,omitempty"`

	// Path of the kustomization. Defaults to Name.
	Path string `yaml:"path,omitempty"`

	// Source of the kustomization.
	Source string `yaml:"source,omitempty"`

	// Namespace overrides the namespace of the Flux Kustomization object itself.
	Namespace string `yaml:"namespace,omitempty"`

	// TargetNamespace sets spec.targetNamespace, so Flux overrides the namespace of every reconciled resource.
	TargetNamespace string `yaml:"targetNamespace,omitempty"`

	// DependsOn lists dependencies of this kustomization.
	DependsOn []string `yaml:"dependsOn,omitempty"`

	// Interval for applying the kustomization.
	Interval *DurationString `yaml:"interval,omitempty"`

	// RetryInterval before retrying a failed kustomization.
	RetryInterval *DurationString `yaml:"retryInterval,omitempty"`

	// Timeout for the kustomization to complete.
	Timeout *DurationString `yaml:"timeout,omitempty"`

	// DeleteTimeout bounds how long destroy waits for this kustomization to delete, such as a managed database.
	DeleteTimeout *DurationString `yaml:"deleteTimeout,omitempty"`

	// Patches to apply to the kustomization.
	Patches []BlueprintPatch `yaml:"patches,omitempty"`

	// Wait for the kustomization to be fully applied.
	Wait *bool `yaml:"wait,omitempty"`

	// Force apply the kustomization.
	Force *bool `yaml:"force,omitempty"`

	// Prune enables garbage collection of resources that are no longer present in the source.
	Prune *bool `yaml:"prune,omitempty"`

	// HealthCheckExprs are CEL health checks for custom resources kstatus cannot evaluate.
	HealthCheckExprs []HealthCheckExpr `yaml:"healthCheckExprs,omitempty"`

	// HealthChecks are external resources to wait on before Ready. See HealthCheckRef.
	HealthChecks []HealthCheckRef `yaml:"healthChecks,omitempty"`

	// Components to include in the kustomization.
	Components []string `yaml:"components,omitempty"`

	// Destroy determines if the kustomization is destroyed during down operations.
	Destroy *BoolExpression `yaml:"destroy,omitempty"`

	// DestroyOnly skips the kustomization during apply and up, and runs it only during destroy.
	DestroyOnly *bool `yaml:"destroyOnly,omitempty"`

	// Enabled determines if the kustomization is included in the final blueprint.
	Enabled *BoolExpression `yaml:"enabled,omitempty"`

	// Substitutions are string values for Flux postBuild substitution.
	Substitutions map[string]string `yaml:"substitutions,omitempty"`

	// Substitute is the preferred spelling of Substitutions, matching Flux's postBuild.substitute.
	Substitute map[string]string `yaml:"substitute,omitempty"`

	// Secrets maps Secret names to entries on a compiled tier, and is never marshaled.
	Secrets map[string]SecretEntry `yaml:"-"`

	// Decryption maps to Flux's spec.decryption. Nil leaves decryption unset.
	Decryption *Decryption `yaml:"decryption,omitempty"`
}

// FluxSystem is a system entry under a blueprint or facet's `flux:` list — a functional layer that
// compiles to an install Kustomization plus zero or more resources-variant Kustomizations. The
// descriptor carries identity, lifecycle, and merge fields; the Install/Resources tiers reuse the
// Kustomization type and carry the Flux operational properties per tier. It holds no operational
// properties itself — those belong on the tiers that produce Kustomization objects.
// DependsOn attaches to the install tier when present, else to each resources variant, and never
// names the implicit install edge. GlobalDependency makes every kustomization and system outside
// this system's dependency closure depend on its terminal tier. Secrets back real Secrets with
// values from schema properties marked sensitive, which never render in plaintext.
type FluxSystem struct {
	// Name identifies the system; tiers compile to "<name>-install" / "<name>-resources[-<variant>]".
	Name string `yaml:"name"`

	// Path is the base; tiers reconcile from "<path>/install" and "<path>/resources". Defaults to Name.
	Path string `yaml:"path,omitempty"`

	// Source is the blueprint source that provides the tier bases.
	Source string `yaml:"source,omitempty"`

	// Enabled includes or excludes the whole system. Defaults to true.
	Enabled *BoolExpression `yaml:"enabled,omitempty"`

	// Destroy governs whether the system's kustomizations are removed on teardown. Defaults to true.
	Destroy *BoolExpression `yaml:"destroy,omitempty"`

	// When gates the system; combined (AND) with each resources variant's own condition.
	When string `yaml:"when,omitempty"`

	// DependsOn are cross-layer edges to other systems.
	DependsOn []string `yaml:"dependsOn,omitempty"`

	// Strategy governs how a system with this Name merges across facets (merge | replace | remove).
	Strategy string `yaml:"strategy,omitempty"`

	// Ordinal overrides the facet's ordinal for this system's merge precedence.
	Ordinal *int `yaml:"ordinal,omitempty"`

	// GlobalDependency makes the system a precondition, such as admission policies, for the rest of the stack.
	GlobalDependency bool `yaml:"globalDependency,omitempty"`

	// Install is the controller tier that compiles to "<name>-install". The composer sets its Name, Path, and DependsOn.
	Install *Kustomization `yaml:"install,omitempty"`

	// Resources are the custom-resource tier variants, all sharing "<path>/resources".
	Resources []FluxVariant `yaml:"resources,omitempty"`

	// Secrets declares Kubernetes Secrets for this system, keyed by Secret name.
	Secrets map[string]SecretEntry `yaml:"secrets,omitempty"`
}

// FluxVariant is one resources-tier Kustomization of a system. It reuses Kustomization for its
// components, substitutions, and operational properties; the authored `name` becomes the variant
// suffix ("<system>-resources-<name>") and `dependsOn` is appended to the implicit install edge.
type FluxVariant struct {
	Kustomization `yaml:",inline"`

	// When gates this variant; combined (AND) with the system's own condition.
	When string `yaml:"when,omitempty"`
}

// PostBuild is a post-build step to run after the kustomization is applied.
type PostBuild struct {
	// Substitute is a map of resources to substitute from.
	Substitute map[string]string `yaml:"substitute,omitempty"`

	// SubstituteFrom is a list of resources to substitute from.
	SubstituteFrom []SubstituteReference `yaml:"substituteFrom,omitempty"`
}

// SubstituteReference is a reference to a resource to substitute from.
type SubstituteReference struct {
	// Kind of the resource to substitute from.
	Kind string `yaml:"kind"`

	// Name of the resource to substitute from.
	Name string `yaml:"name"`

	// Optional indicates if the resource is optional.
	Optional bool `yaml:"optional,omitempty"`
}

// =============================================================================
// Public Methods
// =============================================================================

// BackendComponentID returns Blueprint.Backend, or "" when no backend is declared.
func (b *Blueprint) BackendComponentID() string {
	if b == nil {
		return ""
	}
	return b.Backend
}

// BackendComponents returns the named backend component plus every component declared
// before it in TerraformComponents, in declaration order. Returns nil when Backend is
// unset or names a component not present (validation catches the latter at load).
func (b *Blueprint) BackendComponents() []*TerraformComponent {
	if b == nil || b.Backend == "" {
		return nil
	}
	for i := range b.TerraformComponents {
		if b.TerraformComponents[i].GetID() == b.Backend {
			components := make([]*TerraformComponent, 0, i+1)
			for j := 0; j <= i; j++ {
				components = append(components, &b.TerraformComponents[j])
			}
			return components
		}
	}
	return nil
}

// IsBackendComponent reports whether the component ID appears in BackendComponents.
func (b *Blueprint) IsBackendComponent(id string) bool {
	if id == "" {
		return false
	}
	for _, c := range b.BackendComponents() {
		if c.GetID() == id {
			return true
		}
	}
	return false
}

// DeepCopy creates a deep copy of the Blueprint object.
func (b *Blueprint) DeepCopy() *Blueprint {
	if b == nil {
		return nil
	}

	metadataCopy := Metadata{
		Name:        b.Metadata.Name,
		Description: b.Metadata.Description,
	}

	repositoryCopy := Repository{
		Url: b.Repository.Url,
		Ref: Reference{
			Commit: b.Repository.Ref.Commit,
			Name:   b.Repository.Ref.Name,
			SemVer: b.Repository.Ref.SemVer,
			Tag:    b.Repository.Ref.Tag,
			Branch: b.Repository.Ref.Branch,
		},
	}
	if b.Repository.SecretName != nil {
		secretNameCopy := *b.Repository.SecretName
		repositoryCopy.SecretName = &secretNameCopy
	}

	sourcesCopy := make([]Source, len(b.Sources))
	for i, source := range b.Sources {
		sourcesCopy[i] = Source{
			Name:       source.Name,
			Url:        source.Url,
			PathPrefix: source.PathPrefix,
			Ref: Reference{
				Branch: source.Ref.Branch,
				Tag:    source.Ref.Tag,
				SemVer: source.Ref.SemVer,
				Name:   source.Ref.Name,
				Commit: source.Ref.Commit,
			},
			SecretName: source.SecretName,
			Install:    source.Install.DeepCopy(),
			Crds:       slices.Clone(source.Crds),
		}
	}

	terraformComponentsCopy := make([]TerraformComponent, len(b.TerraformComponents))
	for i, component := range b.TerraformComponents {
		terraformComponentsCopy[i] = *component.DeepCopy()
	}

	kustomizationsCopy := make([]Kustomization, len(b.Kustomizations))
	for i, kustomization := range b.Kustomizations {
		kustomizationsCopy[i] = *kustomization.DeepCopy()
	}

	configMapsCopy := make(map[string]map[string]string)
	if b.ConfigMaps != nil {
		for name, data := range b.ConfigMaps {
			configMapsCopy[name] = maps.Clone(data)
		}
	}

	fluxSystemsCopy := make([]FluxSystem, len(b.FluxSystems))
	for i, sys := range b.FluxSystems {
		fluxSystemsCopy[i] = *sys.DeepCopy()
	}

	return &Blueprint{
		Kind:                b.Kind,
		ApiVersion:          b.ApiVersion,
		Metadata:            metadataCopy,
		Backend:             b.Backend,
		Repository:          repositoryCopy,
		Sources:             sourcesCopy,
		TerraformComponents: terraformComponentsCopy,
		Crds:                slices.Clone(b.Crds),
		Kustomizations:      kustomizationsCopy,
		FluxSystems:         fluxSystemsCopy,
		Substitutions:       maps.Clone(b.Substitutions),
		ConfigMaps:          configMapsCopy,
		Messages:            slices.Clone(b.Messages),
		Config:              deepCopyMapStringAny(b.Config),
	}
}

// DeepCopy creates a deep copy of the Kustomization object.
func (k *Kustomization) DeepCopy() *Kustomization {
	if k == nil {
		return nil
	}

	return &Kustomization{
		Name:             k.Name,
		Path:             k.Path,
		Source:           k.Source,
		Namespace:        k.Namespace,
		TargetNamespace:  k.TargetNamespace,
		DependsOn:        slices.Clone(k.DependsOn),
		Interval:         k.Interval,
		RetryInterval:    k.RetryInterval,
		Timeout:          k.Timeout,
		DeleteTimeout:    k.DeleteTimeout,
		Patches:          slices.Clone(k.Patches),
		Wait:             k.Wait,
		Force:            k.Force,
		Prune:            k.Prune,
		HealthCheckExprs: slices.Clone(k.HealthCheckExprs),
		HealthChecks:     slices.Clone(k.HealthChecks),
		Components:       slices.Clone(k.Components),
		Destroy:          k.Destroy.DeepCopy(),
		DestroyOnly:      k.DestroyOnly,
		Enabled:          k.Enabled.DeepCopy(),
		Substitutions:    maps.Clone(k.Substitutions),
		Substitute:       maps.Clone(k.Substitute),
		Secrets:          cloneSecretData(k.Secrets),
		Decryption:       k.Decryption.DeepCopy(),
	}
}

// DeepCopy returns a deep copy of the FluxVariant, including its embedded Kustomization.
func (v *FluxVariant) DeepCopy() *FluxVariant {
	if v == nil {
		return nil
	}
	return &FluxVariant{
		Kustomization: *v.Kustomization.DeepCopy(),
		When:          v.When,
	}
}

// DeepCopy returns a deep copy of the FluxSystem, including its Install tier and Resources
// variants (each deep-copied via Kustomization.DeepCopy/FluxVariant.DeepCopy).
func (s *FluxSystem) DeepCopy() *FluxSystem {
	if s == nil {
		return nil
	}

	var ordinalCopy *int
	if s.Ordinal != nil {
		o := *s.Ordinal
		ordinalCopy = &o
	}
	resourcesCopy := make([]FluxVariant, len(s.Resources))
	for i, v := range s.Resources {
		resourcesCopy[i] = *v.DeepCopy()
	}

	return &FluxSystem{
		Name:             s.Name,
		Path:             s.Path,
		Source:           s.Source,
		Enabled:          s.Enabled.DeepCopy(),
		Destroy:          s.Destroy.DeepCopy(),
		When:             s.When,
		DependsOn:        slices.Clone(s.DependsOn),
		Strategy:         s.Strategy,
		Ordinal:          ordinalCopy,
		GlobalDependency: s.GlobalDependency,
		Install:          s.Install.DeepCopy(),
		Resources:        resourcesCopy,
		Secrets:          cloneSecretData(s.Secrets),
	}
}

// =============================================================================
// Helpers
// =============================================================================

// cloneSecretData deep-copies a secret map (Secret name -> SecretEntry) so a clone shares no slice or
// map with the original.
func cloneSecretData(in map[string]SecretEntry) map[string]SecretEntry {
	if in == nil {
		return nil
	}
	out := make(map[string]SecretEntry, len(in))
	for name, entry := range in {
		out[name] = entry.DeepCopy()
	}
	return out
}
