// Package blueprint provides blueprint loading, facet processing, composition, and writing for the Windsor CLI.
package blueprint

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/runtime"
)

// =============================================================================
// Types
// =============================================================================

// BaseBlueprintComposer provides the default implementation of the BlueprintComposer interface.
type BaseBlueprintComposer struct {
	runtime             *runtime.Runtime
	commonSubstitutions map[string]string
	shims               *Shims
	excludedFacets      []ExcludedFacet
}

// CrdInstallLayer is a CRD kustomization the provisioner will synthesize: a source (empty for the
// default/project source) and the references it owns. Derived from the composed blueprint by CrdLayers.
type CrdInstallLayer struct {
	Source string
	Refs   []string
}

// =============================================================================
// Interfaces
// =============================================================================

// BlueprintComposer combines processed blueprints from multiple loaders into a final composed blueprint.
// It applies the composition algorithm: Sources (in order) → Template (if not in sources) → User overlay.
type BlueprintComposer interface {
	Compose(loaders []BlueprintLoader, initLoaderNames []string, userBlueprintPath string, configScope map[string]any) (*blueprintv1alpha1.Blueprint, error)
}

// =============================================================================
// Constructor
// =============================================================================

// NewBlueprintComposer creates a new BlueprintComposer that merges multiple blueprints into one.
// The runtime provides access to configuration and context. Optional overrides allow setting
// common substitutions that will be applied to all kustomizations in the composed blueprint.
func NewBlueprintComposer(rt *runtime.Runtime) *BaseBlueprintComposer {
	if rt == nil {
		panic("runtime is required")
	}

	return &BaseBlueprintComposer{
		runtime:             rt,
		commonSubstitutions: make(map[string]string),
		shims:               NewShims(),
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// Compose merges blueprints from multiple loaders into a single unified blueprint. Blueprints are
// merged in order: sources (in the order they appear in the user's Sources array, filtered by
// install:true) → user blueprint as final overlay. The actual merging of individual components
// and kustomizations is delegated to Blueprint.StrategicMerge. After merging, Compose ensures all
// source loaders' source names are present in the result's Sources array so components can resolve
// references (e.g. source: "core"). The "template" source is only included when the local template
// directory exists. Missing sources are added from the loader's blueprint or as minimal entries;
// for OCI loaders without a matching source entry, URL and Ref are taken from any OCI source in
// the loader's blueprint. When configScope is non-nil, it is used when evaluating user blueprint
// terraform inputs so config-block refs resolve.
func (c *BaseBlueprintComposer) Compose(loaders []BlueprintLoader, initLoaderNames []string, userBlueprintPath string, configScope map[string]any) (*blueprintv1alpha1.Blueprint, error) {
	result := DefaultBlueprint.DeepCopy()

	if len(loaders) == 0 {
		return result, nil
	}

	var userBlueprint *blueprintv1alpha1.Blueprint
	var sourceLoaders []BlueprintLoader

	for _, loader := range loaders {
		if loader == nil {
			continue
		}
		name := loader.GetSourceName()
		bp := loader.GetBlueprint()
		if bp == nil {
			continue
		}

		if name == "user" {
			userBlueprint = bp
		} else {
			sourceLoaders = append(sourceLoaders, loader)
		}
	}

	orderedSourceBlueprints := c.orderSources(userBlueprint, sourceLoaders, initLoaderNames)

	if err := result.StrategicMerge(orderedSourceBlueprints...); err != nil {
		return nil, err
	}

	for _, loader := range sourceLoaders {
		sourceName := loader.GetSourceName()
		bp := loader.GetBlueprint()
		if bp == nil {
			continue
		}

		if sourceName == "template" {
			if c.runtime == nil || c.runtime.TemplateRoot == "" {
				continue
			}
			if _, err := os.Stat(c.runtime.TemplateRoot); os.IsNotExist(err) {
				continue
			}
		}

		found := false
		for _, existingSource := range result.Sources {
			if existingSource.Name == sourceName {
				found = true
				break
			}
		}

		if !found {
			var sourceToAdd blueprintv1alpha1.Source
			foundInSource := false
			for _, source := range bp.Sources {
				if source.Name == sourceName {
					sourceToAdd = source
					foundInSource = true
					break
				}
			}

			if !foundInSource {
				trueVal := true
				sourceToAdd = blueprintv1alpha1.Source{
					Name:    sourceName,
					Install: &blueprintv1alpha1.BoolExpression{Value: &trueVal, IsExpr: false},
				}
				for _, s := range bp.Sources {
					if strings.HasPrefix(s.Url, "oci://") {
						sourceToAdd.Url = s.Url
						sourceToAdd.Ref = s.Ref
						break
					}
				}
			}

			result.Sources = append(result.Sources, sourceToAdd)
		}
	}

	if err := c.applyUserBlueprint(result, userBlueprint, userBlueprintPath, configScope); err != nil {
		return nil, err
	}

	c.setContextMetadata(result)
	c.applyCommonSubstitutions(result)
	if err := c.discoverContextPatches(result); err != nil {
		return nil, werror.Wrap(err, "failed to discover context patches")
	}
	if err := c.applyPerKustomizationSubstitutions(result); err != nil {
		return nil, werror.Wrap(err, "failed to apply per-kustomization substitutions")
	}
	c.dropEmptyCompositionFragments(result)
	c.resolveTierDependencies(result)
	c.finalizeCrdLayers(result)
	c.applyCrdLayerBarrier(result)
	c.applyGlobalDependencyBarrier(result)
	c.orderComponentsBySourceDepth(result, userBlueprint, sourceLoaders)
	validationErr := errors.Join(c.validateSources(result), c.validateReservedNames(result), c.validateDependencies(result))
	return result, validationErr
}

// SetExcludedFacets provides the facets dropped by a false when: during facet processing, so
// validateDependencies can attribute a dangling dependency to the excluded facet that would have
// provided it and name the condition that excluded it.
func (c *BaseBlueprintComposer) SetExcludedFacets(excluded []ExcludedFacet) {
	c.excludedFacets = excluded
}

// SetCommonSubstitutions configures substitution values that will be added to all kustomizations
// during composition. These typically include context-wide values like cluster name, domain, or
// environment that should be available to every kustomization's postBuild substitution.
func (c *BaseBlueprintComposer) SetCommonSubstitutions(substitutions map[string]string) {
	c.commonSubstitutions = substitutions
}

// CrdLayers returns the CRD kustomization layers a composed blueprint implies: the default/project
// layer (empty source) from bp.Crds, then one per install source that vendors CRDs, alphabetically by
// name. The composer wires the stack to these names and the provisioner materializes them, so both
// must agree on the set — they share this single derivation.
func CrdLayers(bp *blueprintv1alpha1.Blueprint) []CrdInstallLayer {
	var layers []CrdInstallLayer
	if len(bp.Crds) > 0 {
		layers = append(layers, CrdInstallLayer{Source: "", Refs: bp.Crds})
	}
	idx := make([]int, 0, len(bp.Sources))
	for i := range bp.Sources {
		if len(bp.Sources[i].Crds) > 0 && sourceInstalls(bp.Sources[i]) {
			idx = append(idx, i)
		}
	}
	sort.Slice(idx, func(a, b int) bool { return bp.Sources[idx[a]].Name < bp.Sources[idx[b]].Name })
	for _, i := range idx {
		layers = append(layers, CrdInstallLayer{Source: bp.Sources[i].Name, Refs: bp.Sources[i].Crds})
	}
	return layers
}

// =============================================================================
// Private Methods
// =============================================================================

// applyUserBlueprint applies the user blueprint to the composed result as an override layer.
// The user blueprint can override existing components, add new components, or disable components
// via enabled:false. Non-deferred expressions in the user's terraform inputs (e.g. yaml(), file(),
// config-block refs like talos_common_docker.patches) are evaluated before merging using sourceFilePath
// and configScope when non-nil so relative paths and config-block references resolve.
func (c *BaseBlueprintComposer) applyUserBlueprint(result *blueprintv1alpha1.Blueprint, user *blueprintv1alpha1.Blueprint, sourceFilePath string, configScope map[string]any) error {
	if user == nil {
		return nil
	}

	if user.Repository.Url == "" {
		result.Repository = blueprintv1alpha1.Repository{}
	}

	userCopy := user.DeepCopy()
	if c.runtime != nil && c.runtime.Evaluator != nil {
		for i := range userCopy.TerraformComponents {
			if userCopy.TerraformComponents[i].Inputs != nil {
				evaluated, err := c.runtime.Evaluator.EvaluateMap(userCopy.TerraformComponents[i].Inputs, sourceFilePath, configScope, false)
				if err != nil {
					return werror.Wrap(err, "evaluate user blueprint terraform inputs")
				}
				if normalized, ok := normalizeDeferredValue(evaluated).(map[string]any); ok {
					userCopy.TerraformComponents[i].Inputs = normalized
				} else {
					userCopy.TerraformComponents[i].Inputs = evaluated
				}
			}
		}
	}

	return result.StrategicMerge(userCopy)
}

// dropEmptyCompositionFragments removes template/expr parsing placeholders and empty entries
// left after facet processing: empty Components, empty-key and empty-value Substitutions/Inputs,
// and recursively in nested maps and slices. These should not appear in the final composed blueprint.
func (c *BaseBlueprintComposer) dropEmptyCompositionFragments(blueprint *blueprintv1alpha1.Blueprint) {
	if blueprint == nil {
		return
	}
	for i := range blueprint.Kustomizations {
		components := blueprint.Kustomizations[i].Components
		n := 0
		for _, comp := range components {
			if comp != "" {
				components[n] = comp
				n++
			}
		}
		blueprint.Kustomizations[i].Components = components[:n]

		if blueprint.Kustomizations[i].Substitutions != nil {
			pruneEmptyValue(blueprint.Kustomizations[i].Substitutions)
		}
	}
	for i := range blueprint.TerraformComponents {
		if blueprint.TerraformComponents[i].Inputs != nil {
			pruneEmptyValue(blueprint.TerraformComponents[i].Inputs)
		}
	}
	if blueprint.Substitutions != nil {
		pruneEmptyValue(blueprint.Substitutions)
	}
	if blueprint.ConfigMaps != nil {
		for _, m := range blueprint.ConfigMaps {
			if m != nil {
				pruneEmptyValue(m)
			}
		}
	}
}

// pruneEmptyValue recursively removes empty keys and empty-string values from maps, and
// empty string elements from slices. Mutates maps in place; returns a new slice for []any.
func pruneEmptyValue(v any) any {
	if v == nil {
		return nil
	}
	switch val := v.(type) {
	case map[string]string:
		delete(val, "")
		for k, s := range val {
			if s == "" {
				delete(val, k)
			}
		}
		return val
	case map[string]any:
		delete(val, "")
		for k, item := range val {
			if item == "" {
				delete(val, k)
			} else {
				val[k] = pruneEmptyValue(item)
			}
		}
		return val
	case []any:
		out := make([]any, 0, len(val))
		for _, item := range val {
			if item == "" {
				continue
			}
			out = append(out, pruneEmptyValue(item))
		}
		return out
	default:
		return v
	}
}

// setContextMetadata sets the blueprint metadata name and description based on the current context.
// The name is set to the context name and the description reflects that this is the context's blueprint.
func (c *BaseBlueprintComposer) setContextMetadata(blueprint *blueprintv1alpha1.Blueprint) {
	if c.runtime == nil {
		return
	}

	contextName := c.runtime.ContextName
	if contextName == "" {
		return
	}

	blueprint.Metadata.Name = contextName
	blueprint.Metadata.Description = fmt.Sprintf("Blueprint for the %s context", contextName)
}

// orderSources orders source blueprints according to the user blueprint's Sources array, then
// appends init-loaded blueprints (names in initLoaderNames) that are not listed in the user blueprint.
// OCI sources with install omitted (nil) are merged for backward compatibility; otherwise only
// sources with install:true are included. If userBlueprint is nil (first-time init), all loaders are included.
func (c *BaseBlueprintComposer) orderSources(userBlueprint *blueprintv1alpha1.Blueprint, sourceLoaders []BlueprintLoader, initLoaderNames []string) []*blueprintv1alpha1.Blueprint {
	loaderMap := make(map[string]BlueprintLoader)
	for _, loader := range sourceLoaders {
		loaderMap[loader.GetSourceName()] = loader
	}

	initNamesSet := make(map[string]bool)
	for _, n := range initLoaderNames {
		initNamesSet[n] = true
	}

	userSourceNames := make(map[string]bool)
	if userBlueprint != nil {
		for _, s := range userBlueprint.Sources {
			if s.Name != "" {
				userSourceNames[s.Name] = true
			}
		}
	}

	var orderedBps []*blueprintv1alpha1.Blueprint
	added := make(map[string]bool)

	if userBlueprint != nil {
		for _, source := range userBlueprint.Sources {
			if source.Name == "" {
				continue
			}
			if !c.sourceShouldBeMerged(source) {
				continue
			}
			if loader, exists := loaderMap[source.Name]; exists && !added[source.Name] {
				if bp := loader.GetBlueprint(); bp != nil {
					orderedBps = append(orderedBps, bp)
					added[source.Name] = true
				}
			}
		}
		for _, loader := range sourceLoaders {
			name := loader.GetSourceName()
			if added[name] {
				continue
			}
			if !initNamesSet[name] || userSourceNames[name] {
				continue
			}
			if bp := loader.GetBlueprint(); bp != nil {
				orderedBps = append(orderedBps, bp)
				added[name] = true
			}
		}
		if loader, exists := loaderMap["template"]; exists && !added["template"] {
			if bp := loader.GetBlueprint(); bp != nil {
				orderedBps = append(orderedBps, bp)
				added["template"] = true
			}
		}
	} else {
		for _, loader := range sourceLoaders {
			name := loader.GetSourceName()
			if !added[name] {
				if bp := loader.GetBlueprint(); bp != nil {
					orderedBps = append(orderedBps, bp)
					added[name] = true
				}
			}
		}
	}

	return orderedBps
}

// sourceShouldBeMerged returns true if the source's components should be merged into the composed blueprint.
// OCI sources with Install omitted (nil) are treated as merge for backward compatibility; otherwise Install must be true.
func (c *BaseBlueprintComposer) sourceShouldBeMerged(source blueprintv1alpha1.Source) bool {
	return sourceInstalls(source)
}

// orderComponentsBySourceDepth stably reorders the composed blueprint's components so a component from a
// source composes before a component from the blueprint that references it, recursively. Depth comes from
// the source-reference graph; the primary/user blueprint (component Source "") is deepest. The sort is
// stable, so each source keeps its own prefix/ordinal order, and because dependencies flow from a
// referencing blueprint down into its sources (higher depth to lower), ascending-depth order keeps every
// dependency ahead of its dependents. Explicit ordinals stay within a source and never cross a boundary.
// A component whose Source is absent from the graph is treated as base level, since nothing establishes
// it as layered on top of another source; this is decided here rather than left to a map's zero value.
func (c *BaseBlueprintComposer) orderComponentsBySourceDepth(result *blueprintv1alpha1.Blueprint, userBlueprint *blueprintv1alpha1.Blueprint, sourceLoaders []BlueprintLoader) {
	depth := c.sourceDepths(userBlueprint, sourceLoaders)
	if len(depth) == 0 {
		return
	}
	depthOf := func(source string) int {
		if d, ok := depth[source]; ok {
			return d
		}
		return 0
	}
	sort.SliceStable(result.TerraformComponents, func(i, j int) bool {
		return depthOf(result.TerraformComponents[i].Source) < depthOf(result.TerraformComponents[j].Source)
	})
	sort.SliceStable(result.Kustomizations, func(i, j int) bool {
		return depthOf(result.Kustomizations[i].Source) < depthOf(result.Kustomizations[j].Source)
	})
	sort.SliceStable(result.FluxSystems, func(i, j int) bool {
		return depthOf(result.FluxSystems[i].Source) < depthOf(result.FluxSystems[j].Source)
	})
}

// sourceDepths computes each source's depth in the reference graph: a source that references no other
// in-set source is depth 0, and any source is one deeper than the deepest source it references. The
// primary/user blueprint is keyed by "" (the Source stamped on its own components) and references the
// sources it lists. Returns an empty map when there is no referencing blueprint, so single-source
// compositions skip reordering entirely. Sources are walked in sorted order and a source caught in a
// reference cycle is pinned to base depth, so a cyclic graph still resolves to the same depths on every
// run rather than varying with map iteration order.
func (c *BaseBlueprintComposer) sourceDepths(userBlueprint *blueprintv1alpha1.Blueprint, sourceLoaders []BlueprintLoader) map[string]int {
	refs := make(map[string][]string)
	inSet := make(map[string]bool)
	for _, loader := range sourceLoaders {
		name := loader.GetSourceName()
		inSet[name] = true
		if bp := loader.GetBlueprint(); bp != nil {
			for _, s := range bp.Sources {
				if s.Name != "" {
					refs[name] = append(refs[name], s.Name)
				}
			}
		}
	}
	if userBlueprint == nil {
		return map[string]int{}
	}
	inSet[""] = true
	for _, s := range userBlueprint.Sources {
		if s.Name != "" {
			refs[""] = append(refs[""], s.Name)
		}
	}

	depth := make(map[string]int)
	cyclic := make(map[string]bool)
	var compute func(name string, stack map[string]bool) int
	compute = func(name string, stack map[string]bool) int {
		if d, ok := depth[name]; ok {
			return d
		}
		if stack[name] {
			cyclic[name] = true
			return 0
		}
		stack[name] = true
		max := 0
		for _, dep := range refs[name] {
			if inSet[dep] {
				if d := compute(dep, stack) + 1; d > max {
					max = d
				}
			}
		}
		stack[name] = false
		if cyclic[name] {
			max = 0
		}
		depth[name] = max
		return max
	}
	names := make([]string, 0, len(inSet))
	for name := range inSet {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		compute(name, make(map[string]bool))
	}
	return depth
}

// validateSources checks that install is only used on OCI sources. Git and other non-OCI sources
// cannot be installed (merged); install is supported only for oci:// URLs.
func (c *BaseBlueprintComposer) validateSources(bp *blueprintv1alpha1.Blueprint) error {
	for _, s := range bp.Sources {
		if s.Name == "" {
			continue
		}
		if !s.Install.IsInstalled() {
			continue
		}
		if s.Url == "" {
			continue
		}
		if !strings.HasPrefix(s.Url, "oci://") {
			return fmt.Errorf("source %q has install: true but URL %q is not an OCI source (oci://); install is only supported for OCI sources", s.Name, s.Url)
		}
	}
	return nil
}

// validateReservedNames rejects a user-authored kustomization that takes a name in the synthesized
// CRD layer namespace ("crds" or "crds-<source>"). Those names are owned by the crds: layers the
// provisioner materializes; allowing a kustomize: entry to claim one would collide with the
// synthesized entry and silently change how its PostBuild is built, so it is an error rather than a
// silent override.
func (c *BaseBlueprintComposer) validateReservedNames(bp *blueprintv1alpha1.Blueprint) error {
	for _, k := range bp.Kustomizations {
		if blueprintv1alpha1.IsCrdLayerName(k.Name) {
			return fmt.Errorf("kustomization name %q is reserved for the CRD layer; use the crds: section instead", k.Name)
		}
	}
	return nil
}

// =============================================================================
// Interface Compliance
// =============================================================================

var _ BlueprintComposer = (*BaseBlueprintComposer)(nil)
