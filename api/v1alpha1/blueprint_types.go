// Package v1alpha1 contains types for the v1alpha1 API group
// +groupName=blueprints.windsorcli.dev
package v1alpha1

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	kustomizev1 "github.com/fluxcd/kustomize-controller/api/v1"
	"github.com/fluxcd/pkg/apis/kustomize"
	"github.com/windsorcli/cli/pkg/constants"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

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

// Blueprint is a configuration blueprint for initializing a project.
type Blueprint struct {
	// Kind is the blueprint type, following Kubernetes conventions.
	Kind string `yaml:"kind"`

	// ApiVersion is the API schema version of the blueprint.
	ApiVersion string `yaml:"apiVersion"`

	// Metadata includes the blueprint's name and description.
	Metadata Metadata `yaml:"metadata"`

	// Repository details the source repository of the blueprint.
	Repository Repository `yaml:"repository,omitempty"`

	// Sources are external resources referenced by the blueprint.
	Sources []Source `yaml:"sources"`

	// TerraformComponents are Terraform modules in the blueprint.
	TerraformComponents []TerraformComponent `yaml:"terraform"`

	// Kustomizations are kustomization configs in the blueprint.
	Kustomizations []Kustomization `yaml:"kustomize"`

	// ConfigMaps are standalone ConfigMaps to be created, not tied to specific kustomizations.
	// These ConfigMaps are referenced by all kustomizations in PostBuild substitution.
	ConfigMaps map[string]map[string]string `yaml:"configMaps,omitempty"`
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
	Url string `yaml:"url"`

	// Ref details the branch, tag, or commit to use.
	Ref Reference `yaml:"ref"`

	// SecretName is the secret for repository access.
	SecretName *string `yaml:"secretName,omitempty"`
}

// Source is an external resource referenced by a blueprint.
type Source struct {
	// Name identifies the source.
	Name string `yaml:"name"`

	// Url is the source location.
	Url string `yaml:"url"`

	// PathPrefix is a prefix to the source path.
	PathPrefix string `yaml:"pathPrefix,omitempty"`

	// Ref details the branch, tag, or commit to use.
	Ref Reference `yaml:"ref,omitempty"`

	// SecretName is the secret for source access.
	SecretName string `yaml:"secretName,omitempty"`
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
}

// =============================================================================
// Public Methods
// =============================================================================

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

	inputsCopy := maps.Clone(t.Inputs)

	dependsOnCopy := make([]string, len(t.DependsOn))
	copy(dependsOnCopy, t.DependsOn)

	return &TerraformComponent{
		Name:        t.Name,
		Source:      t.Source,
		Path:        t.Path,
		FullPath:    t.FullPath,
		DependsOn:   dependsOnCopy,
		Inputs:      inputsCopy,
		Destroy:     t.Destroy,
		Parallelism: t.Parallelism,
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
	// Name of the kustomization.
	Name string `yaml:"name"`

	// Path of the kustomization.
	Path string `yaml:"path"`

	// Source of the kustomization.
	Source string `yaml:"source,omitempty"`

	// DependsOn lists dependencies of this kustomization.
	DependsOn []string `yaml:"dependsOn,omitempty"`

	// Interval for applying the kustomization.
	Interval *DurationString `yaml:"interval,omitempty"`

	// RetryInterval before retrying a failed kustomization.
	RetryInterval *DurationString `yaml:"retryInterval,omitempty"`

	// Timeout for the kustomization to complete.
	Timeout *DurationString `yaml:"timeout,omitempty"`

	// Patches to apply to the kustomization.
	Patches []BlueprintPatch `yaml:"patches,omitempty"`

	// Wait for the kustomization to be fully applied.
	Wait *bool `yaml:"wait,omitempty"`

	// Force apply the kustomization.
	Force *bool `yaml:"force,omitempty"`

	// Prune enables garbage collection of resources that are no longer present in the source.
	Prune *bool `yaml:"prune,omitempty"`

	// Components to include in the kustomization.
	Components []string `yaml:"components,omitempty"`

	// Cleanup lists resources to clean up after the kustomization is applied.
	Cleanup []string `yaml:"cleanup,omitempty"`

	// Destroy determines if the kustomization should be destroyed during down operations.
	// Defaults to true if not specified.
	// Supports expressions in facets: use "${cluster.destroy ?? true}" for dynamic values.
	Destroy *BoolExpression `yaml:"destroy,omitempty"`

	// DestroyOnly indicates that this kustomization should only run during destroy operations.
	// When true, the kustomization is skipped during apply/up operations and only executed during destroy.
	// Destroy-only kustomizations run before regular kustomizations during destroy, in normal dependency order.
	DestroyOnly *bool `yaml:"destroyOnly,omitempty"`

	// Substitutions contains values for post-build variable replacement,
	// collected and stored in ConfigMaps for use by Flux postBuild substitution.
	// All values are converted to strings as required by Flux variable substitution.
	// These are used for generating ConfigMaps and are not written to the final context blueprint.yaml.
	Substitutions map[string]string `yaml:"substitutions,omitempty"`
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

	return &Blueprint{
		Kind:                b.Kind,
		ApiVersion:          b.ApiVersion,
		Metadata:            metadataCopy,
		Repository:          repositoryCopy,
		Sources:             sourcesCopy,
		TerraformComponents: terraformComponentsCopy,
		Kustomizations:      kustomizationsCopy,
		ConfigMaps:          configMapsCopy,
	}
}

// StrategicMerge performs a strategic merge of the provided overlay Blueprints into the receiver Blueprint.
// This method appends to array fields, deep merges map fields, and updates scalar fields if present in the overlays.
// It is designed for feature composition, enabling the combination of multiple features into a single blueprint.
func (b *Blueprint) StrategicMerge(overlays ...*Blueprint) error {
	for _, overlay := range overlays {
		if overlay == nil {
			continue
		}

		if overlay.Kind != "" {
			b.Kind = overlay.Kind
		}
		if overlay.ApiVersion != "" {
			b.ApiVersion = overlay.ApiVersion
		}

		if overlay.Metadata.Name != "" {
			b.Metadata.Name = overlay.Metadata.Name
		}
		if overlay.Metadata.Description != "" {
			b.Metadata.Description = overlay.Metadata.Description
		}

		if overlay.Repository.Url != "" {
			b.Repository = overlay.Repository
		}

		sourceMap := make(map[string]Source)
		for _, source := range b.Sources {
			sourceMap[source.Name] = source
		}
		for _, overlaySource := range overlay.Sources {
			if overlaySource.Name != "" {
				sourceMap[overlaySource.Name] = overlaySource
			}
		}
		b.Sources = make([]Source, 0, len(sourceMap))
		for _, source := range sourceMap {
			b.Sources = append(b.Sources, source)
		}

		for _, overlayComponent := range overlay.TerraformComponents {
			if err := b.strategicMergeTerraformComponent(overlayComponent); err != nil {
				return err
			}
		}

		for _, overlayK := range overlay.Kustomizations {
			if err := b.strategicMergeKustomization(overlayK); err != nil {
				return err
			}
		}

		if overlay.ConfigMaps != nil {
			if b.ConfigMaps == nil {
				b.ConfigMaps = make(map[string]map[string]string)
			}
			for name, data := range overlay.ConfigMaps {
				if b.ConfigMaps[name] == nil {
					b.ConfigMaps[name] = make(map[string]string)
				}
				maps.Copy(b.ConfigMaps[name], data)
			}
		}
	}

	return b.validateTerraformComponents()
}

// ReplaceTerraformComponent replaces an existing TerraformComponent with the provided component.
// Components are matched by component ID (name if provided, otherwise Path).
// If a matching component exists, it is completely replaced. Otherwise, the component is appended.
// Returns an error if a dependency cycle is detected during sorting or if component IDs are not unique.
func (b *Blueprint) ReplaceTerraformComponent(component TerraformComponent) error {
	componentID := component.GetID()

	for i, existing := range b.TerraformComponents {
		existingID := existing.GetID()

		if existingID == componentID {
			b.TerraformComponents[i] = component
			if err := b.sortTerraform(); err != nil {
				return err
			}
			return b.validateTerraformComponents()
		}
	}
	b.TerraformComponents = append(b.TerraformComponents, component)
	if err := b.sortTerraform(); err != nil {
		return err
	}
	return b.validateTerraformComponents()
}

// ReplaceKustomization replaces an existing Kustomization with the provided kustomization.
// If a kustomization with the same Name exists, it is completely replaced. Otherwise, the kustomization is appended.
// Returns an error if a dependency cycle is detected during sorting.
func (b *Blueprint) ReplaceKustomization(kustomization Kustomization) error {
	for i, existing := range b.Kustomizations {
		if existing.Name == kustomization.Name {
			b.Kustomizations[i] = kustomization
			return b.sortKustomize()
		}
	}
	b.Kustomizations = append(b.Kustomizations, kustomization)
	return b.sortKustomize()
}

// RemoveTerraformComponent removes specified non-index fields from an existing TerraformComponent.
// Components are matched by component ID (name if provided, otherwise Path).
// It removes inputs, dependencies, and other fields that are specified in the removal component.
// Index fields (Name, Path, Source) are not affected. If no matching component exists, no action is taken.
func (b *Blueprint) RemoveTerraformComponent(removal TerraformComponent) error {
	removalID := removal.GetID()
	for i, existing := range b.TerraformComponents {
		existingID := existing.GetID()

		if existingID == removalID {
			if len(removal.Inputs) > 0 && existing.Inputs != nil {
				for key := range removal.Inputs {
					delete(existing.Inputs, key)
				}
			}

			if len(removal.DependsOn) > 0 {
				existing.DependsOn = slices.DeleteFunc(existing.DependsOn, func(dep string) bool {
					return slices.Contains(removal.DependsOn, dep)
				})
			}

			b.TerraformComponents[i] = existing
			return b.sortTerraform()
		}
	}
	return nil
}

// RemoveKustomization removes specified non-index fields from an existing Kustomization.
// It finds a kustomization matching the same Name, then removes patches, components, dependencies,
// cleanup items, and substitutions that are specified in the removal kustomization.
// The index field (Name) is not affected. If no matching kustomization exists, no action is taken.
func (b *Blueprint) RemoveKustomization(removal Kustomization) error {
	for i, existing := range b.Kustomizations {
		if existing.Name == removal.Name {
			if len(removal.DependsOn) > 0 {
				existing.DependsOn = slices.DeleteFunc(existing.DependsOn, func(dep string) bool {
					return slices.Contains(removal.DependsOn, dep)
				})
			}

			if len(removal.Components) > 0 {
				existing.Components = slices.DeleteFunc(existing.Components, func(comp string) bool {
					return slices.Contains(removal.Components, comp)
				})
			}

			if len(removal.Cleanup) > 0 {
				existing.Cleanup = slices.DeleteFunc(existing.Cleanup, func(cleanup string) bool {
					return slices.Contains(removal.Cleanup, cleanup)
				})
			}

			if len(removal.Patches) > 0 {
				existing.Patches = slices.DeleteFunc(existing.Patches, func(patch BlueprintPatch) bool {
					for _, removalPatch := range removal.Patches {
						if removalPatch.Path != "" && patch.Path == removalPatch.Path {
							return true
						}
						if removalPatch.Patch != "" && patch.Patch == removalPatch.Patch {
							return true
						}
					}
					return false
				})
			}

			if len(removal.Substitutions) > 0 && existing.Substitutions != nil {
				for key := range removal.Substitutions {
					delete(existing.Substitutions, key)
				}
			}

			b.Kustomizations[i] = existing
			return b.sortKustomize()
		}
	}
	return nil
}

// DeepCopy creates a deep copy of the Kustomization object.
func (k *Kustomization) DeepCopy() *Kustomization {
	if k == nil {
		return nil
	}

	return &Kustomization{
		Name:          k.Name,
		Path:          k.Path,
		Source:        k.Source,
		DependsOn:     slices.Clone(k.DependsOn),
		Interval:      k.Interval,
		RetryInterval: k.RetryInterval,
		Timeout:       k.Timeout,
		Patches:       slices.Clone(k.Patches),
		Wait:          k.Wait,
		Force:         k.Force,
		Prune:         k.Prune,
		Components:    slices.Clone(k.Components),
		Cleanup:       slices.Clone(k.Cleanup),
		Destroy:       k.Destroy,
		Substitutions: maps.Clone(k.Substitutions),
	}
}

// ToFluxKustomization converts a blueprint Kustomization to a Flux Kustomization.
// It takes the namespace for the kustomization, the default source name to use if no source is specified,
// and the list of sources to determine the source kind (GitRepository or OCIRepository).
// PostBuild is constructed based on the kustomization's Substitutions field.
func (k *Kustomization) ToFluxKustomization(namespace string, defaultSourceName string, sources []Source) kustomizev1.Kustomization {
	dependsOn := make([]kustomizev1.DependencyReference, len(k.DependsOn))
	for idx, dep := range k.DependsOn {
		dependsOn[idx] = kustomizev1.DependencyReference{
			Name:      dep,
			Namespace: namespace,
		}
	}

	sourceName := k.Source
	if sourceName == "" {
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
		path = "kustomize"
	} else {
		path = strings.ReplaceAll(path, "\\", "/")
		if path != "kustomize" && !strings.HasPrefix(path, "kustomize/") {
			path = "kustomize/" + path
		}
	}

	interval := metav1.Duration{Duration: constants.DefaultFluxKustomizationInterval}
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

	return kustomizev1.Kustomization{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Kustomization",
			APIVersion: "kustomize.toolkit.fluxcd.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      k.Name,
			Namespace: namespace,
		},
		Spec: kustomizev1.KustomizationSpec{
			SourceRef: kustomizev1.CrossNamespaceSourceReference{
				Kind: sourceKind,
				Name: sourceName,
			},
			Path:           path,
			DependsOn:      dependsOn,
			Interval:       interval,
			RetryInterval:  &retryInterval,
			Timeout:        &timeout,
			Wait:           wait,
			Force:          force,
			Prune:          prune,
			DeletionPolicy: deletionPolicy,
			Patches:        patches,
			Components:     k.Components,
			PostBuild:      postBuild,
		},
	}
}

// =============================================================================
// Private Methods
// =============================================================================

// validateTerraformComponents validates that all terraform component IDs are unique.
// Component IDs are either the Name (if provided) or Path (if no name).
// Returns an error if duplicate IDs are found.
func (b *Blueprint) validateTerraformComponents() error {
	idToComponent := make(map[string]TerraformComponent)

	for _, component := range b.TerraformComponents {
		componentID := component.GetID()

		if existing, exists := idToComponent[componentID]; exists {
			return fmt.Errorf("duplicate terraform component ID %q: component with name %q and path %q conflicts with existing component with name %q and path %q", componentID, component.Name, component.Path, existing.Name, existing.Path)
		}

		idToComponent[componentID] = component
	}

	return nil
}

// strategicMergeTerraformComponent performs a strategic merge of the provided TerraformComponent into the Blueprint.
// It merges values, appends unique dependencies, updates fields if provided, and maintains dependency order.
// Components are matched by component ID (name if provided, otherwise Path).
// Returns an error if a dependency cycle is detected during sorting or if component IDs are not unique.
func (b *Blueprint) strategicMergeTerraformComponent(component TerraformComponent) error {
	componentID := component.GetID()

	for i, existing := range b.TerraformComponents {
		existingID := existing.GetID()

		if existingID == componentID {
			if len(component.Inputs) > 0 {
				if existing.Inputs == nil {
					existing.Inputs = make(map[string]any)
				}
				existing.Inputs = b.deepMergeMaps(existing.Inputs, component.Inputs)
			}
			for _, dep := range component.DependsOn {
				if !slices.Contains(existing.DependsOn, dep) {
					existing.DependsOn = append(existing.DependsOn, dep)
				}
			}
			if component.Destroy != nil {
				existing.Destroy = component.Destroy
			}
			if component.Parallelism != nil {
				existing.Parallelism = component.Parallelism
			}
			if component.Name != "" {
				existing.Name = component.Name
			}
			b.TerraformComponents[i] = existing
			if err := b.sortTerraform(); err != nil {
				return err
			}
			return b.validateTerraformComponents()
		}
	}

	b.TerraformComponents = append(b.TerraformComponents, component)
	if err := b.sortTerraform(); err != nil {
		return err
	}
	return b.validateTerraformComponents()
}

// strategicMergeKustomization performs a strategic merge of the provided Kustomization into the Blueprint.
// It merges unique components and dependencies, updates fields if provided, and maintains dependency order.
// Patches from the provided kustomization are appended to existing patches.
// Returns an error if a dependency cycle is detected during sorting.
func (b *Blueprint) strategicMergeKustomization(kustomization Kustomization) error {
	if kustomization.Name == "" {
		return nil
	}

	for i, existing := range b.Kustomizations {
		if existing.Name == kustomization.Name {
			for _, component := range kustomization.Components {
				if !slices.Contains(existing.Components, component) {
					existing.Components = append(existing.Components, component)
				}
			}
			slices.Sort(existing.Components)
			for _, dep := range kustomization.DependsOn {
				if !slices.Contains(existing.DependsOn, dep) {
					existing.DependsOn = append(existing.DependsOn, dep)
				}
			}
			if kustomization.Path != "" {
				existing.Path = kustomization.Path
			}
			if kustomization.Source != "" {
				existing.Source = kustomization.Source
			}
			if kustomization.Destroy != nil {
				existing.Destroy = kustomization.Destroy
			}
			if kustomization.Interval != nil {
				existing.Interval = kustomization.Interval
			}
			if kustomization.RetryInterval != nil {
				existing.RetryInterval = kustomization.RetryInterval
			}
			if kustomization.Timeout != nil {
				existing.Timeout = kustomization.Timeout
			}
			if len(kustomization.Patches) > 0 {
				existing.Patches = append(existing.Patches, kustomization.Patches...)
			}
			b.Kustomizations[i] = existing
			return b.sortKustomize()
		}
	}
	b.Kustomizations = append(b.Kustomizations, kustomization)
	return b.sortKustomize()
}

// sortKustomize reorders the Blueprint's Kustomizations so that dependencies precede dependents.
// It first applies a topological sort to ensure dependency order, then groups kustomizations with similar name prefixes adjacently.
// Returns an error if a dependency cycle is detected.
func (b *Blueprint) sortKustomize() error {
	if len(b.Kustomizations) <= 1 {
		return nil
	}
	nameToIndex := make(map[string]int)
	for i, k := range b.Kustomizations {
		nameToIndex[k.Name] = i
	}
	sorted := b.basicTopologicalSort(nameToIndex)
	if sorted == nil {
		return fmt.Errorf("dependency cycle detected in kustomizations")
	}
	sorted = b.groupSimilarPrefixes(sorted, nameToIndex)
	newKustomizations := make([]Kustomization, len(b.Kustomizations))
	for i, sortedIndex := range sorted {
		newKustomizations[i] = b.Kustomizations[sortedIndex]
	}
	b.Kustomizations = newKustomizations
	return nil
}

// basicTopologicalSort computes a topological ordering of kustomizations based on dependencies.
// Returns a slice of indices into the Kustomizations slice, ordered so dependencies precede dependents.
// Returns nil if a cycle is detected in the dependency graph.
func (b *Blueprint) basicTopologicalSort(nameToIndex map[string]int) []int {
	var sorted []int
	visited := make(map[int]bool)
	visiting := make(map[int]bool)

	var visit func(int) error
	visit = func(componentIndex int) error {
		if visiting[componentIndex] {
			return fmt.Errorf("cycle detected in dependency graph involving kustomization '%s'", b.Kustomizations[componentIndex].Name)
		}
		if visited[componentIndex] {
			return nil
		}

		visiting[componentIndex] = true
		for _, depName := range b.Kustomizations[componentIndex].DependsOn {
			if depIndex, exists := nameToIndex[depName]; exists {
				if err := visit(depIndex); err != nil {
					visiting[componentIndex] = false
					return err
				}
			}
		}
		visiting[componentIndex] = false
		visited[componentIndex] = true
		sorted = append(sorted, componentIndex)
		return nil
	}

	for i := range b.Kustomizations {
		if !visited[i] {
			if err := visit(i); err != nil {
				fmt.Printf("Error: %v\n", err)
				return nil
			}
		}
	}
	return sorted
}

// groupSimilarPrefixes returns a new ordering of kustomization indices so components with similar
// name prefixes are grouped. It groups kustomizations by the prefix before the first hyphen in
// their name, then processes each group in the order they appear in the input slice. For groups
// with multiple components, it sorts by dependency depth (shallowest first), then by name if
// depths are equal. The resulting slice preserves dependency order and groups related
// kustomizations adjacently.
func (b *Blueprint) groupSimilarPrefixes(sorted []int, nameToIndex map[string]int) []int {
	prefixGroups := make(map[string][]int)
	for _, idx := range sorted {
		prefix := strings.Split(b.Kustomizations[idx].Name, "-")[0]
		prefixGroups[prefix] = append(prefixGroups[prefix], idx)
	}

	var newSorted []int
	processed := make(map[int]bool)

	for _, originalIdx := range sorted {
		if processed[originalIdx] {
			continue
		}

		prefix := strings.Split(b.Kustomizations[originalIdx].Name, "-")[0]
		group := prefixGroups[prefix]

		if len(group) == 1 {
			newSorted = append(newSorted, group[0])
			processed[group[0]] = true
		} else {
			slices.SortFunc(group, func(i, j int) int {
				depthI := b.calculateDependencyDepth(i, nameToIndex)
				depthJ := b.calculateDependencyDepth(j, nameToIndex)
				if depthI != depthJ {
					return depthI - depthJ
				}
				return strings.Compare(b.Kustomizations[i].Name, b.Kustomizations[j].Name)
			})

			for _, idx := range group {
				if !processed[idx] {
					newSorted = append(newSorted, idx)
					processed[idx] = true
				}
			}
		}
	}

	return newSorted
}

// calculateDependencyDepth returns the maximum dependency depth for the specified kustomization index.
// It recursively traverses the dependency graph using the provided name-to-index mapping, computing
// the longest path from the given component to any leaf dependency. A component with no dependencies
// has depth 0. Cycles are not detected and may cause stack overflow.
func (b *Blueprint) calculateDependencyDepth(componentIndex int, nameToIndex map[string]int) int {
	k := b.Kustomizations[componentIndex]
	if len(k.DependsOn) == 0 {
		return 0
	}

	maxDepth := 0
	for _, depName := range k.DependsOn {
		if depIndex, exists := nameToIndex[depName]; exists {
			depth := b.calculateDependencyDepth(depIndex, nameToIndex)
			if depth+1 > maxDepth {
				maxDepth = depth + 1
			}
		}
	}
	return maxDepth
}

// sortTerraform reorders the Blueprint's TerraformComponents so that dependencies precede dependents.
// It applies a topological sort to ensure dependency order. Components without dependencies come first.
// Returns an error if a dependency cycle is detected.
func (b *Blueprint) sortTerraform() error {
	if len(b.TerraformComponents) <= 1 {
		return nil
	}

	idToIndex := make(map[string]int)
	for i, component := range b.TerraformComponents {
		idToIndex[component.GetID()] = i
	}

	sorted := b.terraformTopologicalSort(idToIndex)
	if sorted == nil {
		return fmt.Errorf("dependency cycle detected in terraform components")
	}

	newComponents := make([]TerraformComponent, len(b.TerraformComponents))
	for i, sortedIndex := range sorted {
		newComponents[i] = b.TerraformComponents[sortedIndex]
	}
	b.TerraformComponents = newComponents
	return nil
}

// terraformTopologicalSort returns a topological ordering of Terraform components based on their dependencies.
// It receives a mapping from component IDs to their respective indices within the TerraformComponents slice,
// and produces a slice of indices such that all dependencies come before any component that depends on them.
// If a cycle is detected in the dependency graph, the function returns nil. The returned slice is ordered
// with respect to the topological sort, ensuring that all dependencies precede their dependents.
func (b *Blueprint) terraformTopologicalSort(idToIndex map[string]int) []int {
	var sorted []int
	visited := make(map[int]bool)
	visiting := make(map[int]bool)

	var visit func(int) error
	visit = func(componentIndex int) error {
		if visiting[componentIndex] {
			return fmt.Errorf("cycle detected in dependency graph involving terraform component '%s'", b.TerraformComponents[componentIndex].GetID())
		}
		if visited[componentIndex] {
			return nil
		}

		visiting[componentIndex] = true
		for _, depID := range b.TerraformComponents[componentIndex].DependsOn {
			if depIndex, exists := idToIndex[depID]; exists {
				if err := visit(depIndex); err != nil {
					visiting[componentIndex] = false
					return err
				}
			}
		}
		visiting[componentIndex] = false
		visited[componentIndex] = true
		sorted = append(sorted, componentIndex)
		return nil
	}

	for i := range b.TerraformComponents {
		if !visited[i] {
			if err := visit(i); err != nil {
				fmt.Printf("Error: %v\n", err)
				return nil
			}
		}
	}

	return sorted
}

// deepMergeMaps returns a new map from a deep merge of base and overlay maps.
// Overlay values take precedence; nested maps merge recursively. Non-map overlay values replace base values.
func (b *Blueprint) deepMergeMaps(base, overlay map[string]any) map[string]any {
	result := maps.Clone(base)
	for k, overlayValue := range overlay {
		if baseValue, exists := result[k]; exists {
			if baseMap, baseIsMap := baseValue.(map[string]any); baseIsMap {
				if overlayMap, overlayIsMap := overlayValue.(map[string]any); overlayIsMap {
					result[k] = b.deepMergeMaps(baseMap, overlayMap)
					continue
				}
			}
		}
		result[k] = overlayValue
	}
	return result
}
