// Package blueprint provides blueprint loading, facet processing, composition, and writing for the Windsor CLI.
package blueprint

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/fluxcd/pkg/apis/kustomize"
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/runtime"
)

// BlueprintComposer combines processed blueprints from multiple loaders into a final composed blueprint.
// It applies the composition algorithm: Sources (in order) → Template (if not in sources) → User overlay.
type BlueprintComposer interface {
	Compose(loaders []BlueprintLoader, initLoaderNames []string, userBlueprintPath string, configScope map[string]any) (*blueprintv1alpha1.Blueprint, error)
}

// =============================================================================
// Types
// =============================================================================

// BaseBlueprintComposer provides the default implementation of the BlueprintComposer interface.
type BaseBlueprintComposer struct {
	runtime             *runtime.Runtime
	commonSubstitutions map[string]string
	shims               *Shims
}

// CrdInstallLayer is a CRD kustomization the provisioner will synthesize: a source (empty for the
// default/project source) and the references it owns. Derived from the composed blueprint by CrdLayers.
type CrdInstallLayer struct {
	Source string
	Refs   []string
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
		return nil, fmt.Errorf("failed to discover context patches: %w", err)
	}
	if err := c.applyPerKustomizationSubstitutions(result); err != nil {
		return nil, fmt.Errorf("failed to apply per-kustomization substitutions: %w", err)
	}
	c.dropEmptyCompositionFragments(result)
	c.resolveTierDependencies(result)
	c.finalizeCrdLayers(result)
	c.applyCrdLayerBarrier(result)
	c.applyGlobalDependencyBarrier(result)
	validationErr := errors.Join(c.validateSources(result), c.validateReservedNames(result), c.validateDependencies(result))
	return result, validationErr
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
					return fmt.Errorf("evaluate user blueprint terraform inputs: %w", err)
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

// applyCommonSubstitutions extracts common substitutions from values.yaml, merges legacy special
// variables (DOMAIN, CONTEXT, etc.) from the runtime config, and creates a ConfigMap called
// "values-common" in the blueprint. This ConfigMap is used by kustomizations for postBuild
// substitutions. The method combines values from the commonSubstitutions field (set via
// SetCommonSubstitutions), values from the "common" key in substitutions from values.yaml,
// and legacy variables extracted from the config handler.
func (c *BaseBlueprintComposer) applyCommonSubstitutions(blueprint *blueprintv1alpha1.Blueprint) {
	mergedCommonValues := make(map[string]string)

	if c.commonSubstitutions != nil {
		for k, v := range c.commonSubstitutions {
			mergedCommonValues[k] = v
		}
	}

	for k, v := range blueprint.Substitutions {
		mergedCommonValues[k] = v
	}

	if c.runtime != nil && c.runtime.ConfigHandler != nil {
		values, err := c.runtime.ConfigHandler.GetContextValues()
		if err == nil {
			if substitutions, ok := values["substitutions"].(map[string]any); ok {
				if common, ok := substitutions["common"].(map[string]any); ok {
					for k, v := range common {
						mergedCommonValues[k] = fmt.Sprintf("%v", v)
					}
				}
			}
		}

		c.mergeLegacySpecialVariables(mergedCommonValues)
	}

	if len(mergedCommonValues) > 0 {
		if blueprint.ConfigMaps == nil {
			blueprint.ConfigMaps = make(map[string]map[string]string)
		}
		blueprint.ConfigMaps["values-common"] = mergedCommonValues
	}
}

// mergeLegacySpecialVariables extracts legacy configuration values from the runtime config handler
// and adds them to the merged common values map. These include DOMAIN, CONTEXT, CONTEXT_ID,
// LOADBALANCER_IP_RANGE, REGISTRY_URL, LOCAL_VOLUME_PATH, and BUILD_ID. These variables are
// maintained for backward compatibility with existing kustomizations that reference them.
func (c *BaseBlueprintComposer) mergeLegacySpecialVariables(mergedCommonValues map[string]string) {
	if c.runtime == nil || c.runtime.ConfigHandler == nil {
		return
	}

	domain := c.runtime.ConfigHandler.GetString("dns.domain")
	context := c.runtime.ConfigHandler.GetContext()
	contextID := c.runtime.ConfigHandler.GetString("id")
	lbStart := c.runtime.ConfigHandler.GetString("network.loadbalancer_ips.start")
	lbEnd := c.runtime.ConfigHandler.GetString("network.loadbalancer_ips.end")
	registryURL := c.runtime.ConfigHandler.GetString("docker.registry_url")
	localVolumePaths := c.runtime.ConfigHandler.GetStringSlice("cluster.workers.volumes")

	loadBalancerIPRange := fmt.Sprintf("%s-%s", lbStart, lbEnd)

	var localVolumePath string
	if len(localVolumePaths) > 0 {
		parts := strings.Split(localVolumePaths[0], ":")
		if len(parts) > 1 {
			localVolumePath = parts[1]
		}
	}

	if domain != "" {
		mergedCommonValues["DOMAIN"] = domain
	}
	if context != "" {
		mergedCommonValues["CONTEXT"] = context
	}
	if contextID != "" {
		mergedCommonValues["CONTEXT_ID"] = contextID
	}
	skipLoadBalancerKeys := c.runtime.ConfigHandler.GetString("workstation.runtime") == "docker-desktop"
	if !skipLoadBalancerKeys && loadBalancerIPRange != "-" {
		mergedCommonValues["LOADBALANCER_IP_RANGE"] = loadBalancerIPRange
	}
	if !skipLoadBalancerKeys && lbStart != "" {
		mergedCommonValues["LOADBALANCER_IP_START"] = lbStart
	}
	if !skipLoadBalancerKeys && lbEnd != "" {
		mergedCommonValues["LOADBALANCER_IP_END"] = lbEnd
	}
	if registryURL != "" {
		mergedCommonValues["REGISTRY_URL"] = registryURL
	}
	if localVolumePath != "" {
		mergedCommonValues["LOCAL_VOLUME_PATH"] = localVolumePath
	}

	buildID, err := c.runtime.GetBuildID()
	if err == nil && buildID != "" {
		mergedCommonValues["BUILD_ID"] = buildID
	}
}

// discoverContextPatches discovers and adds patches from the context directory to kustomizations.
// Patches are discovered from contexts/<context>/patches/<kustomization-name>/ and added to the
// corresponding kustomization. Supports both strategic merge patches (standard Kubernetes YAML)
// and JSON 6902 patches (with a patches field containing JSON 6902 operations).
func (c *BaseBlueprintComposer) discoverContextPatches(blueprint *blueprintv1alpha1.Blueprint) error {
	if c.runtime == nil || c.runtime.ConfigRoot == "" {
		return nil
	}

	patchesDir := filepath.Join(c.runtime.ConfigRoot, "patches")
	if _, err := c.shims.Stat(patchesDir); os.IsNotExist(err) {
		return nil
	}

	entries, err := c.shims.ReadDir(patchesDir)
	if err != nil {
		return fmt.Errorf("failed to read patches directory: %w", err)
	}

	kustomizationMap := make(map[string]*blueprintv1alpha1.Kustomization)
	for i := range blueprint.Kustomizations {
		kustomizationMap[blueprint.Kustomizations[i].Name] = &blueprint.Kustomizations[i]
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		kustomizationName := entry.Name()
		kustomization, exists := kustomizationMap[kustomizationName]
		if !exists {
			continue
		}

		kustomizationPatchesDir := filepath.Join(patchesDir, kustomizationName)
		patchFiles, err := c.shims.ReadDir(kustomizationPatchesDir)
		if err != nil {
			continue
		}

		for _, patchFile := range patchFiles {
			if patchFile.IsDir() {
				continue
			}

			fileName := patchFile.Name()
			if !strings.HasSuffix(fileName, ".yaml") && !strings.HasSuffix(fileName, ".yml") {
				continue
			}

			patchPath := filepath.Join(kustomizationPatchesDir, fileName)
			patchData, err := c.shims.ReadFile(patchPath)
			if err != nil {
				continue
			}

			patch, err := c.parsePatch(patchData, fileName)
			if err != nil {
				continue
			}

			if patch != nil {
				kustomization.Patches = append(kustomization.Patches, *patch)
			}
		}
	}

	return nil
}

// parsePatch parses a patch file and returns a BlueprintPatch. It detects whether the patch is
// a strategic merge patch (standard Kubernetes YAML) or a JSON 6902 patch (with a patches field).
// For JSON 6902 patches, it extracts the target selector from the resource metadata.
// Returns nil if the patch data is empty or whitespace-only.
func (c *BaseBlueprintComposer) parsePatch(data []byte, fileName string) (*blueprintv1alpha1.BlueprintPatch, error) {
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil, nil
	}

	var patchContent map[string]any
	if err := c.shims.YamlUnmarshal(data, &patchContent); err != nil {
		return nil, fmt.Errorf("failed to unmarshal patch file %s: %w", fileName, err)
	}

	if len(patchContent) == 0 {
		return nil, nil
	}

	if patches, ok := patchContent["patches"].([]any); ok {
		kind, hasKind := patchContent["kind"].(string)
		if !hasKind || kind == "" {
			return &blueprintv1alpha1.BlueprintPatch{
				Patch: string(data),
			}, nil
		}

		jsonPatchOps, err := c.shims.YamlMarshal(patches)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal JSON patch operations: %w", err)
		}

		target := &kustomize.Selector{
			Kind: kind,
		}
		if metadata, ok := patchContent["metadata"].(map[string]any); ok {
			if name, ok := metadata["name"].(string); ok {
				target.Name = name
			}
			if namespace, ok := metadata["namespace"].(string); ok {
				target.Namespace = namespace
			}
		}

		return &blueprintv1alpha1.BlueprintPatch{
			Patch:  string(jsonPatchOps),
			Target: target,
		}, nil
	}

	return &blueprintv1alpha1.BlueprintPatch{
		Patch: string(data),
	}, nil
}

// applyPerKustomizationSubstitutions extracts per-kustomization substitutions from values.yaml
// and creates ConfigMaps for each kustomization. Substitutions are stored in ConfigMaps named
// "values-<kustomization-name>" and are used for postBuild substitution in Flux kustomizations.
func (c *BaseBlueprintComposer) applyPerKustomizationSubstitutions(blueprint *blueprintv1alpha1.Blueprint) error {
	if c.runtime == nil || c.runtime.ConfigHandler == nil {
		return nil
	}

	values, err := c.runtime.ConfigHandler.GetContextValues()
	if err != nil {
		return nil
	}

	substitutions, ok := values["substitutions"].(map[string]any)
	if !ok {
		return nil
	}

	if blueprint.ConfigMaps == nil {
		blueprint.ConfigMaps = make(map[string]map[string]string)
	}

	for i := range blueprint.Kustomizations {
		kustomization := &blueprint.Kustomizations[i]
		kustomizationName := kustomization.Name

		if kustSubstitutions, ok := substitutions[kustomizationName].(map[string]any); ok {
			configMapName := fmt.Sprintf("values-%s", kustomizationName)
			configMapData := make(map[string]string)

			for k, v := range kustSubstitutions {
				configMapData[k] = fmt.Sprintf("%v", v)
			}

			if len(configMapData) > 0 {
				if kustomizationName == "common" {
					if existingConfigMap, exists := blueprint.ConfigMaps["values-common"]; exists {
						maps.Copy(existingConfigMap, configMapData)
					} else {
						blueprint.ConfigMaps["values-common"] = configMapData
					}
				} else {
					blueprint.ConfigMaps[configMapName] = configMapData
				}

				if kustomization.Substitutions == nil {
					kustomization.Substitutions = make(map[string]string)
				}
				maps.Copy(kustomization.Substitutions, configMapData)
			}
		}
	}

	return nil
}

// sourceInstalls reports whether a source's components — including its vendored CRDs — are merged and
// installed: an OCI source with install omitted defaults to true for backward compatibility, otherwise
// install must be explicitly true. sourceShouldBeMerged delegates here so the two never diverge.
func sourceInstalls(source blueprintv1alpha1.Source) bool {
	if source.Install == nil {
		return strings.HasPrefix(source.Url, "oci://")
	}
	return source.Install.IsInstalled()
}

// finalizeCrdLayers assigns each CRD reference to exactly one owner before the barrier wires the stack
// to them, so two kustomizations never apply the same CRD object. The default/project list (bp.Crds)
// owns first, then install sources alphabetically by name; a reference claimed by an earlier owner is
// pruned from later ones. Only install-eligible sources (the same sourceInstalls predicate CrdLayers
// materializes on) may claim ownership: a non-install source claiming a ref would strip it from a real
// owner while never installing it itself, silently orphaning the CRD. A local template source collapses
// into the default/project list (the same condition ToFluxKustomization uses), so a purely local
// blueprint installs its CRDs as "crds" rather than a "crds-template" layer. Ownership by name keeps a
// CRD stable across source reordering.
func (c *BaseBlueprintComposer) finalizeCrdLayers(bp *blueprintv1alpha1.Blueprint) {
	if !blueprintv1alpha1.HasRemoteTemplateSource(bp.Sources) {
		for i := range bp.Sources {
			if bp.Sources[i].Name != "template" || len(bp.Sources[i].Crds) == 0 {
				continue
			}
			for _, ref := range bp.Sources[i].Crds {
				if !slices.Contains(bp.Crds, ref) {
					bp.Crds = append(bp.Crds, ref)
				}
			}
			bp.Sources[i].Crds = nil
		}
	}

	claimed := make(map[string]struct{})
	bp.Crds = claimCrdRefs(bp.Crds, claimed)

	idx := make([]int, 0, len(bp.Sources))
	for i := range bp.Sources {
		if len(bp.Sources[i].Crds) > 0 && sourceInstalls(bp.Sources[i]) {
			idx = append(idx, i)
		}
	}
	sort.Slice(idx, func(a, b int) bool { return bp.Sources[idx[a]].Name < bp.Sources[idx[b]].Name })
	for _, i := range idx {
		bp.Sources[i].Crds = claimCrdRefs(bp.Sources[i].Crds, claimed)
	}
}

// claimCrdRefs returns refs minus any already in claimed, recording the survivors as claimed, sorted.
func claimCrdRefs(refs []string, claimed map[string]struct{}) []string {
	out := make([]string, 0, len(refs))
	for _, ref := range refs {
		if _, taken := claimed[ref]; taken {
			continue
		}
		claimed[ref] = struct{}{}
		out = append(out, ref)
	}
	sort.Strings(out)
	return out
}

// applyCrdLayerBarrier orders the vendored CRD layers ahead of the whole stack: when the blueprint
// implies CRD layers, every root — a plain kustomize: entry or flux: system with no dependsOn of its
// own — is made to depend on every synthesized CRD kustomization. Dependents of those roots reach the
// layers transitively, so the stack reconciles after the CRDs are Established — without any facet
// author naming a CRD in dependsOn, and without the provisioner waiting: Flux honors the edges on
// every reconcile. The CRD kustomizations themselves are materialized by the provisioner from the
// same CrdLayers derivation.
func (c *BaseBlueprintComposer) applyCrdLayerBarrier(bp *blueprintv1alpha1.Blueprint) {
	layers := CrdLayers(bp)
	if len(layers) == 0 {
		return
	}
	names := make([]string, len(layers))
	for i, layer := range layers {
		names[i] = blueprintv1alpha1.CrdKustomizationName(layer.Source)
	}
	for i := range bp.Kustomizations {
		k := &bp.Kustomizations[i]
		if len(k.DependsOn) == 0 {
			k.DependsOn = slices.Clone(names)
		}
	}
	for i := range bp.FluxSystems {
		if len(bp.FluxSystems[i].DependsOn) == 0 {
			bp.FluxSystems[i].DependsOn = slices.Clone(names)
		}
	}
}

// applyGlobalDependencyBarrier wires every kustomization and flux system outside a global-dependency
// system's own dependency closure to depend on that system's terminal tier — its resources tier if it
// has one, else its install tier. It is the inverse of dependsOn: a system declares itself required by
// the whole cluster once (globalDependency: true) instead of every consumer naming it. The closure
// exclusion keeps the system, and anything it transitively depends on, from ordering after itself — the
// same way the crds layer sits ahead of the stack without depending on it. It runs after
// applyCrdLayerBarrier so a global system rooted at the crds layer already carries that edge when its
// closure is computed.
func (c *BaseBlueprintComposer) applyGlobalDependencyBarrier(bp *blueprintv1alpha1.Blueprint) {
	type barrier struct {
		terminals []string
		closure   map[string]struct{}
	}

	depsByName := make(map[string][]string)
	for _, k := range bp.AllKustomizations() {
		depsByName[k.Name] = k.DependsOn
	}

	var barriers []barrier
	for _, sys := range bp.FluxSystems {
		if !sys.GlobalDependency {
			continue
		}
		terminals := terminalTierNames(sys)
		if len(terminals) == 0 {
			continue
		}
		barriers = append(barriers, barrier{
			terminals: terminals,
			closure:   dependencyClosure(sys.TierNames(), depsByName),
		})
	}
	if len(barriers) == 0 {
		return
	}

	for _, b := range barriers {
		for i := range bp.Kustomizations {
			if _, inClosure := b.closure[bp.Kustomizations[i].Name]; inClosure {
				continue
			}
			bp.Kustomizations[i].DependsOn = appendMissing(bp.Kustomizations[i].DependsOn, b.terminals)
		}
		for i := range bp.FluxSystems {
			if fluxSystemInClosure(bp.FluxSystems[i], b.closure) {
				continue
			}
			bp.FluxSystems[i].DependsOn = appendMissing(bp.FluxSystems[i].DependsOn, b.terminals)
		}
	}
}

// terminalTierNames returns the compiled kustomization names at which a system is fully reconciled: its
// resources tier names when it has a resources tier, otherwise its install tier. These are the targets a
// globalDependency system's consumers wait on.
func terminalTierNames(sys blueprintv1alpha1.FluxSystem) []string {
	names := sys.TierNames()
	if len(sys.Resources) == 0 {
		return names
	}
	installName := sys.Name + "-install"
	terminals := make([]string, 0, len(names))
	for _, n := range names {
		if n != installName {
			terminals = append(terminals, n)
		}
	}
	return terminals
}

// dependencyClosure returns the set of kustomization names reachable from seeds by following dependsOn
// edges (seeds included), over the compiled dependency graph depsByName.
func dependencyClosure(seeds []string, depsByName map[string][]string) map[string]struct{} {
	closure := make(map[string]struct{})
	queue := slices.Clone(seeds)
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		if _, seen := closure[name]; seen {
			continue
		}
		closure[name] = struct{}{}
		queue = append(queue, depsByName[name]...)
	}
	return closure
}

// fluxSystemInClosure reports whether any of a system's compiled tiers is in closure. It is how a
// globalDependency barrier skips the system it depends on (wiring it would form a cycle), the global
// system itself included, since a system's own tiers are always in its closure.
func fluxSystemInClosure(sys blueprintv1alpha1.FluxSystem, closure map[string]struct{}) bool {
	for _, name := range sys.TierNames() {
		if _, ok := closure[name]; ok {
			return true
		}
	}
	return false
}

// appendMissing returns deps with each addition not already present appended in order, so the barrier
// never duplicates an edge a consumer already declares.
func appendMissing(deps []string, additions []string) []string {
	for _, add := range additions {
		if !slices.Contains(deps, add) {
			deps = append(deps, add)
		}
	}
	return deps
}

// resolveTierDependencies rewrites a dependsOn reference to a vendor's bare name into its install
// tier when the vendor was authored with install/resources tiers. A facet that depends on
// "cert-manager" resolves to "cert-manager-install" when no kustomization named "cert-manager"
// exists but "cert-manager-install" does, so "depend on cert-manager" means "wait for its
// controller". An exact name match always wins, so the legacy flat form is unaffected.
func (c *BaseBlueprintComposer) resolveTierDependencies(bp *blueprintv1alpha1.Blueprint) {
	names := make(map[string]struct{}, len(bp.Kustomizations)+len(bp.FluxSystems))
	for _, k := range bp.Kustomizations {
		names[k.Name] = struct{}{}
	}
	for _, sys := range bp.FluxSystems {
		if sys.Install != nil {
			names[sys.Name+"-install"] = struct{}{}
		}
	}
	resolve := func(deps []string) {
		for j, dep := range deps {
			if _, ok := names[dep]; ok {
				continue
			}
			if _, ok := names[dep+"-install"]; ok {
				deps[j] = dep + "-install"
			}
		}
	}
	for i := range bp.Kustomizations {
		resolve(bp.Kustomizations[i].DependsOn)
	}
	for i := range bp.FluxSystems {
		resolve(bp.FluxSystems[i].DependsOn)
	}
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

// validateDependencies checks that all component dependencies reference components that exist in the final blueprint.
// This validation happens after all composition is complete to ensure dependencies are valid in the final state.
func (c *BaseBlueprintComposer) validateDependencies(bp *blueprintv1alpha1.Blueprint) error {
	tfIDs := make(map[string]struct{})
	for _, tf := range bp.TerraformComponents {
		tfIDs[tf.GetID()] = struct{}{}
	}

	allK := bp.AllKustomizations()
	kNames := make(map[string]struct{}, len(allK)+len(bp.Sources)+1)
	for _, k := range allK {
		kNames[k.Name] = struct{}{}
	}
	for _, layer := range CrdLayers(bp) {
		kNames[blueprintv1alpha1.CrdKustomizationName(layer.Source)] = struct{}{}
	}

	for _, tf := range bp.TerraformComponents {
		for _, dep := range tf.DependsOn {
			if _, exists := tfIDs[dep]; !exists {
				return fmt.Errorf("terraform component %q depends on non-existent component %q", tf.GetID(), dep)
			}
		}
	}

	for _, k := range allK {
		for _, dep := range k.DependsOn {
			if _, exists := kNames[dep]; !exists {
				return fmt.Errorf("kustomization %q depends on non-existent kustomization %q", k.Name, dep)
			}
		}
	}

	return nil
}

// =============================================================================
// Interface Compliance
// =============================================================================

var _ BlueprintComposer = (*BaseBlueprintComposer)(nil)
