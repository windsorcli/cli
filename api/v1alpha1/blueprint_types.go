// Package v1alpha1 contains types for the v1alpha1 API group
// +groupName=blueprints.windsorcli.dev
package v1alpha1

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/fluxcd/pkg/apis/kustomize"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

// DurationString represents a duration that marshals/unmarshals as a string (e.g., "5m").
// It can be converted to metav1.Duration for use with Kubernetes APIs.
type DurationString struct {
	Duration time.Duration
}

// MarshalYAML implements yaml.Marshaler to write duration as a string.
func (d *DurationString) MarshalYAML() (any, error) {
	if d == nil || d.Duration == 0 {
		return nil, nil
	}
	return d.Duration.String(), nil
}

// UnmarshalYAML implements yaml.Unmarshaler to read duration from a string or object.
func (d *DurationString) UnmarshalYAML(unmarshal func(any) error) error {
	var s string
	if err := unmarshal(&s); err == nil {
		if s == "" {
			d.Duration = 0
			return nil
		}
		dur, err := time.ParseDuration(s)
		if err != nil {
			return fmt.Errorf("invalid duration %q: %w", s, err)
		}
		d.Duration = dur
		return nil
	}

	var obj map[string]any
	if err := unmarshal(&obj); err != nil {
		return err
	}
	if durationStr, ok := obj["duration"].(string); ok {
		if durationStr == "" {
			d.Duration = 0
			return nil
		}
		dur, err := time.ParseDuration(durationStr)
		if err != nil {
			return fmt.Errorf("invalid duration %q: %w", durationStr, err)
		}
		d.Duration = dur
		return nil
	}
	return fmt.Errorf("duration must be a string or an object with a 'duration' field")
}

// ToMetaV1Duration converts DurationString to *metav1.Duration.
func (d *DurationString) ToMetaV1Duration() *metav1.Duration {
	if d == nil || d.Duration == 0 {
		return nil
	}
	return &metav1.Duration{Duration: d.Duration}
}

// FromMetaV1Duration creates a DurationString from *metav1.Duration.
func FromMetaV1Duration(d *metav1.Duration) *DurationString {
	if d == nil || d.Duration == 0 {
		return nil
	}
	return &DurationString{Duration: d.Duration}
}

// BoolExpression represents a boolean value that can be unmarshaled from a boolean or a string expression.
// String expressions are preserved for later evaluation during facet processing.
type BoolExpression struct {
	Value  *bool
	Expr   string
	IsExpr bool
}

// MarshalYAML implements yaml.Marshaler to write boolean as a bool or expression string.
func (b *BoolExpression) MarshalYAML() (any, error) {
	if b == nil {
		return nil, nil
	}
	if b.IsExpr && b.Expr != "" {
		return b.Expr, nil
	}
	if b.Value != nil {
		return *b.Value, nil
	}
	return nil, nil
}

// UnmarshalYAML implements yaml.Unmarshaler to read boolean from a bool or string expression.
func (b *BoolExpression) UnmarshalYAML(unmarshal func(any) error) error {
	var boolVal bool
	if err := unmarshal(&boolVal); err == nil {
		b.Value = &boolVal
		b.IsExpr = false
		b.Expr = ""
		return nil
	}

	var strVal string
	if err := unmarshal(&strVal); err == nil {
		if strings.Contains(strVal, "${") {
			b.Value = nil
			b.IsExpr = true
			b.Expr = strVal
		} else {
			boolVal := strVal == "true"
			b.Value = &boolVal
			b.IsExpr = false
			b.Expr = ""
		}
		return nil
	}

	return fmt.Errorf("bool expression must be a boolean or string")
}

// ToBool returns the boolean value, or nil if it's an expression.
func (b *BoolExpression) ToBool() *bool {
	if b == nil {
		return nil
	}
	return b.Value
}

// DeepCopy creates a deep copy of the BoolExpression.
func (b *BoolExpression) DeepCopy() *BoolExpression {
	if b == nil {
		return nil
	}
	copy := &BoolExpression{
		IsExpr: b.IsExpr,
		Expr:   b.Expr,
	}
	if b.Value != nil {
		valueCopy := *b.Value
		copy.Value = &valueCopy
	}
	return copy
}

// IsEnabled returns true if the BoolExpression is nil (default) or if its value is true.
// Returns false only if the value is explicitly set to false.
// This is used for fields like Enabled that default to true.
func (b *BoolExpression) IsEnabled() bool {
	if b == nil {
		return true
	}
	if b.IsExpr {
		return true
	}
	if b.Value == nil {
		return true
	}
	return *b.Value
}

// IsInstalled returns true only if the BoolExpression is explicitly set to true.
// Returns false if nil (default), if it's an expression, or if explicitly false.
// This is used for fields like Install that default to false.
func (b *BoolExpression) IsInstalled() bool {
	if b == nil {
		return false
	}
	if b.IsExpr {
		return false
	}
	if b.Value == nil {
		return false
	}
	return *b.Value
}

// IntExpression represents an integer value that can be unmarshaled from an integer or a string expression.
// String expressions are preserved for later evaluation during facet processing.
type IntExpression struct {
	Value  *int
	Expr   string
	IsExpr bool
}

// MarshalYAML implements yaml.Marshaler to write integer as an int or expression string.
func (i *IntExpression) MarshalYAML() (any, error) {
	if i == nil {
		return nil, nil
	}
	if i.IsExpr && i.Expr != "" {
		return i.Expr, nil
	}
	if i.Value != nil {
		return *i.Value, nil
	}
	return nil, nil
}

// UnmarshalYAML implements yaml.Unmarshaler to read integer from an int or string expression.
func (i *IntExpression) UnmarshalYAML(unmarshal func(any) error) error {
	var intVal int
	if err := unmarshal(&intVal); err == nil {
		i.Value = &intVal
		i.IsExpr = false
		i.Expr = ""
		return nil
	}

	var strVal string
	if err := unmarshal(&strVal); err == nil {
		if strings.Contains(strVal, "${") {
			i.Value = nil
			i.IsExpr = true
			i.Expr = strVal
		} else {
			var parsed int
			if _, err := fmt.Sscanf(strVal, "%d", &parsed); err != nil {
				return fmt.Errorf("invalid integer: %q", strVal)
			}
			i.Value = &parsed
			i.IsExpr = false
			i.Expr = ""
		}
		return nil
	}

	return fmt.Errorf("int expression must be an integer or string")
}

// ToInt returns the integer value, or nil if it's an expression.
func (i *IntExpression) ToInt() *int {
	if i == nil {
		return nil
	}
	return i.Value
}

// DeepCopy creates a deep copy of the IntExpression.
func (i *IntExpression) DeepCopy() *IntExpression {
	if i == nil {
		return nil
	}
	copy := &IntExpression{
		IsExpr: i.IsExpr,
		Expr:   i.Expr,
	}
	if i.Value != nil {
		valueCopy := *i.Value
		copy.Value = &valueCopy
	}
	return copy
}

// SecretEntry is one `secrets:` entry on a flux system tier: the data that backs a Kubernetes Secret
// plus the namespaces it is placed into. Namespaces is optional — when omitted the target auto-resolves
// to the single namespace the owning kustomization creates (unchanged from the original behavior); when
// set it names the target(s) explicitly, so a system that owns more than one namespace (e.g. pki, which
// owns system-pki and system-pki-trust) can place a secret and a single entry can fan out to several
// namespaces. It is a plain struct decoded directly by the YAML decoder — data lives under `data:` so a
// data key can never collide with the `namespaces:` selector.
type SecretEntry struct {
	// Namespaces are the namespaces to place this Secret into. Empty means auto-resolve the single
	// namespace the owning kustomization creates; each named namespace must be one that kustomization
	// provably created.
	Namespaces []string `yaml:"namespaces,omitempty"`

	// Data is the Secret's contents: data key -> reference to a schema property, resolved JIT at
	// placement and never rendered in plaintext. Materializes as an Opaque Secret by default; a
	// ".dockerconfigjson" key, or "docker-username" plus "docker-password" (optional
	// "docker-server", defaulting to "ghcr.io"), materializes as kubernetes.io/dockerconfigjson
	// instead, for use as an imagePullSecret — see ApplySecret/secretTypeAndData.
	Data map[string]string `yaml:"data,omitempty"`
}

// DeepCopy returns a deep copy of the SecretEntry, sharing no slice or map with the original.
func (s SecretEntry) DeepCopy() SecretEntry {
	return SecretEntry{
		Namespaces: slices.Clone(s.Namespaces),
		Data:       maps.Clone(s.Data),
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

	// Crds is the flat list of vendored CRD references the blueprint installs from the default/project
	// source (e.g. "cert-manager-1.16.2"). It is authored and asserted as a bare scalar list, the same
	// form facets use. CRDs carried by an OCI source are not listed here — they ride with that Source
	// (see Source.Crds) and install in the background when the source is install:true. The provisioner
	// materializes these into the "crds" kustomization, bound to the default source, ahead of the stack.
	Crds []string `yaml:"crds,omitempty"`

	// Kustomizations are plain Flux Kustomization configs (the kustomize: passthrough).
	Kustomizations []Kustomization `yaml:"kustomize,omitempty"`

	// FluxSystems are system entries (the flux: collection): each compiles to an install plus
	// resources-variant Kustomizations. Distinct from Kustomizations, which are 1:1 passthroughs.
	FluxSystems []FluxSystem `yaml:"flux,omitempty"`

	// Substitutions are top-level key/value pairs evaluated with facet scope and injected into
	// values-common, making them available to all kustomizations via PostBuild substitution.
	// Values may use expression syntax (e.g. "${dns.domain}") resolved against facet config blocks.
	Substitutions map[string]string `yaml:"substitutions,omitempty"`

	// ConfigMaps are standalone ConfigMaps to be created, not tied to specific kustomizations.
	// These ConfigMaps are referenced by all kustomizations in PostBuild substitution.
	ConfigMaps map[string]map[string]string `yaml:"configMaps,omitempty"`

	// Messages are operator-facing post-run notes contributed by active facets. They carry raw
	// text/when templates through composition; GenerateResolved evaluates each against composed
	// scope, keeping only the when-true entries with interpolated text for the command to print.
	Messages []Message `yaml:"messages,omitempty"`

	// Config is the composed config scope: facet config: block names mapped to their fully
	// evaluated values. TestRunner populates this field so a test case's expect:/exclude: can
	// assert on it; production composition and windsor show leave it unset.
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

	// Install determines if the source should be merged (components merged into final blueprint).
	// Defaults to false if not specified.
	// Supports expressions in facets: use "${some.condition ?? true}" for dynamic values.
	Install *BoolExpression `yaml:"install,omitempty"`

	// Crds is the flat list of CRD references this source vendors at <source>/kustomize/crds/<ref>.
	// Populated by the composer from the source's included facets. When the source is install:true,
	// the provisioner materializes these into a background "crds-<name>" kustomization bound to this
	// source, so its CRDs install ahead of the stack without the blueprint author listing them.
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

// TerraformComponent defines a Terraform module in a blueprint.
type TerraformComponent struct {
	// Name of the terraform component. If provided, this becomes the unique identifier
	// instead of Path. Used for referencing in dependencies and context variables.
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
	// These values can be expressions using ${} syntax (e.g., "${cluster.name}") or literals.
	// Values with ${} are evaluated as expressions, plain values are passed through as literals.
	// These are used for generating tfvars files and are not written to the final context blueprint.yaml.
	Inputs map[string]any `yaml:"inputs,omitempty"`

	// Destroy determines if the component should be destroyed during down operations.
	// Defaults to true if not specified.
	// Supports expressions in facets: use "${cluster.destroy ?? true}" for dynamic values.
	Destroy *BoolExpression `yaml:"destroy,omitempty"`

	// Parallelism limits the number of concurrent operations as Terraform walks the graph.
	// This corresponds to the -parallelism flag in terraform apply/destroy commands.
	// Supports expressions in facets: use "${cluster.parallelism ?? 10}" for dynamic values.
	Parallelism *IntExpression `yaml:"parallelism,omitempty"`

	// Enabled determines if the component should be included in the final blueprint.
	// Defaults to true if not specified.
	// Supports expressions in facets: use "${some.condition ?? true}" for dynamic values.
	Enabled *BoolExpression `yaml:"enabled,omitempty"`

	// InputOrigins maps each input key to the facet file path that defined it.
	// Used internally for resolving relative paths in deferred expression evaluation.
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
type Kustomization struct {
	// Name of the kustomization. Omitted when empty so an unnamed inline flux resources variant
	// (a Kustomization embedded in FluxVariant) does not emit a blank name: into the composed output.
	Name string `yaml:"name,omitempty"`

	// Path of the kustomization. Defaults to Name.
	Path string `yaml:"path,omitempty"`

	// Source of the kustomization.
	Source string `yaml:"source,omitempty"`

	// Namespace overrides the namespace where the Flux Kustomization object itself lives.
	// When unset, the gitops namespace is used. DependsOn references always resolve in the
	// gitops namespace, so cross-namespace dependencies are not currently supported.
	Namespace string `yaml:"namespace,omitempty"`

	// TargetNamespace populates spec.targetNamespace, instructing Flux to override the
	// namespace of every resource reconciled by this kustomization.
	TargetNamespace string `yaml:"targetNamespace,omitempty"`

	// DependsOn lists dependencies of this kustomization.
	DependsOn []string `yaml:"dependsOn,omitempty"`

	// Interval for applying the kustomization.
	Interval *DurationString `yaml:"interval,omitempty"`

	// RetryInterval before retrying a failed kustomization.
	RetryInterval *DurationString `yaml:"retryInterval,omitempty"`

	// Timeout for the kustomization to complete. Governs install/reconcile waits.
	// Also seeds the destroy delete-wait floor when DeleteTimeout is unset.
	Timeout *DurationString `yaml:"timeout,omitempty"`

	// DeleteTimeout bounds how long destroy waits for this kustomization to delete.
	// Set it when delete outlasts install, for example a managed database.
	// Falls back to a Timeout-derived heuristic when unset.
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

	// Destroy determines if the kustomization should be destroyed during down operations.
	// Defaults to true if not specified.
	// Supports expressions in facets: use "${cluster.destroy ?? true}" for dynamic values.
	Destroy *BoolExpression `yaml:"destroy,omitempty"`

	// DestroyOnly indicates that this kustomization should only run during destroy operations.
	// When true, the kustomization is skipped during apply/up operations and only executed during destroy.
	// Destroy-only kustomizations run before regular kustomizations during destroy, in normal dependency order.
	DestroyOnly *bool `yaml:"destroyOnly,omitempty"`

	// Enabled determines if the kustomization should be included in the final blueprint.
	// Defaults to true if not specified.
	// Supports expressions in facets: use "${some.condition ?? true}" for dynamic values.
	Enabled *BoolExpression `yaml:"enabled,omitempty"`

	// Substitutions contains values for post-build variable replacement,
	// collected and stored in ConfigMaps for use by Flux postBuild substitution.
	// All values are converted to strings as required by Flux variable substitution.
	// These are used for generating ConfigMaps and are not written to the final context blueprint.yaml.
	Substitutions map[string]string `yaml:"substitutions,omitempty"`

	// Substitute is the preferred spelling of Substitutions; both keys merge into Substitutions
	// during composition. It exists so `flux:` system tiers and `kustomize:` entries can spell
	// postBuild substitutions the same short way (matching Flux's `postBuild.substitute`).
	Substitute map[string]string `yaml:"substitute,omitempty"`

	// Secrets carries a flux system's Secrets onto its compiled namespace-owning tier so the imperative
	// placement step can find them after flattening: Secret name -> SecretEntry (target namespaces plus
	// data), resolved JIT at placement, never during composition. It is yaml:"-" so it is never
	// marshaled into show/apply output or the written blueprint, keeping secret material out of every
	// serialized surface by construction.
	Secrets map[string]SecretEntry `yaml:"-"`

	// Decryption configures in-cluster decryption for this kustomization's manifests, mapping to Flux's
	// spec.decryption. Nil leaves decryption unset (Flux default: no decryption).
	Decryption *Decryption `yaml:"decryption,omitempty"`
}

// FluxSystem is a system entry under a blueprint or facet's `flux:` list — a functional layer that
// compiles to an install Kustomization plus zero or more resources-variant Kustomizations. The
// descriptor carries identity, lifecycle, and merge fields; the Install/Resources tiers reuse the
// Kustomization type and carry the Flux operational properties per tier. It holds no operational
// properties itself — those belong on the tiers that produce Kustomization objects.
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

	// DependsOn are cross-layer edges to other systems, attached to the install tier when present
	// (resources reach them transitively) or to each resources variant otherwise. It never names the
	// intra-system install edge — that edge is implicit.
	DependsOn []string `yaml:"dependsOn,omitempty"`

	// Strategy governs how a system with this Name merges across facets (merge | replace | remove).
	Strategy string `yaml:"strategy,omitempty"`

	// Ordinal overrides the facet's ordinal for this system's merge precedence.
	Ordinal *int `yaml:"ordinal,omitempty"`

	// GlobalDependency inverts the system's edges: when true, every kustomization and system outside
	// this system's own dependency closure is wired to depend on its terminal tier — its resources tier
	// if it has one, else its install tier. It lets a cluster-wide precondition (e.g. admission policies
	// that must be enforcing before any workload lands) be declared once here rather than repeated as a
	// dependsOn on every consumer. The closure exclusion keeps the system, and anything it depends on,
	// from ordering after itself.
	GlobalDependency bool `yaml:"globalDependency,omitempty"`

	// Install is the controller/operator tier (at most one). Its Components/Substitute and operational
	// fields flow to "<name>-install"; its Name/Path/DependsOn are derived by the composer.
	Install *Kustomization `yaml:"install,omitempty"`

	// Resources are the custom-resource tier variants, all sharing "<path>/resources".
	Resources []FluxVariant `yaml:"resources,omitempty"`

	// Secrets declares Kubernetes Secrets for this system, keyed by Secret name; each entry carries the
	// Secret's data (data key -> reference to a schema property marked `sensitive: true`) and, optionally,
	// the namespaces to place it into. Unlike Substitute (plaintext ConfigMap material), Secrets back real
	// Secrets placed into the system's namespace(s) and are resolved and materialized separately — their
	// values are never rendered or written in plaintext.
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
