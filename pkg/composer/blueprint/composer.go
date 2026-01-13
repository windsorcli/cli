package blueprint

import (
	"fmt"
	"strings"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/runtime"
)

// BlueprintComposer combines processed blueprints from multiple loaders into a final composed blueprint.
// It applies the composition algorithm: Sources → Primary → User overlay.
type BlueprintComposer interface {
	Compose(loaders []BlueprintLoader) (*blueprintv1alpha1.Blueprint, error)
}

// =============================================================================
// Types
// =============================================================================

// BaseBlueprintComposer provides the default implementation of the BlueprintComposer interface.
type BaseBlueprintComposer struct {
	runtime             *runtime.Runtime
	commonSubstitutions map[string]string
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
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// Compose merges blueprints from multiple loaders into a single unified blueprint. Loaders are
// categorized by source name: "primary" for the base template, "user" for user overrides, and
// all others as sources. The merge order is Sources → Primary → User, where each subsequent
// layer can override or extend previous layers. Before applying the user blueprint, the result
// is filtered to only include components explicitly selected by the user. The actual merging
// of individual components and kustomizations is delegated to Blueprint.StrategicMerge.
func (c *BaseBlueprintComposer) Compose(loaders []BlueprintLoader) (*blueprintv1alpha1.Blueprint, error) {
	result := DefaultBlueprint.DeepCopy()

	if len(loaders) == 0 {
		return result, nil
	}

	var primary *blueprintv1alpha1.Blueprint
	var user *blueprintv1alpha1.Blueprint
	var sourceBps []*blueprintv1alpha1.Blueprint

	for _, loader := range loaders {
		name := loader.GetSourceName()
		bp := loader.GetBlueprint()
		if bp == nil {
			continue
		}

		switch name {
		case "primary":
			primary = bp
		case "user":
			user = bp
		default:
			sourceBps = append(sourceBps, bp)
		}
	}

	if err := result.StrategicMerge(sourceBps...); err != nil {
		return nil, err
	}
	if primary != nil {
		if err := result.StrategicMerge(primary); err != nil {
			return nil, err
		}
	}

	if err := c.applyUserBlueprint(result, user); err != nil {
		return nil, err
	}

	c.setContextMetadata(result)

	c.applyCommonSubstitutions(result)

	return result, nil
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

// SetCommonSubstitutions configures substitution values that will be added to all kustomizations
// during composition. These typically include context-wide values like cluster name, domain, or
// environment that should be available to every kustomization's postBuild substitution.
func (c *BaseBlueprintComposer) SetCommonSubstitutions(substitutions map[string]string) {
	c.commonSubstitutions = substitutions
}

// =============================================================================
// Private Methods
// =============================================================================

// applyUserBlueprint applies the user blueprint to the composed result, filtering and merging.
// Filters terraform components, kustomizations, and sources to only those selected by the user.
// Clears repository if user doesn't define one. After filtering, merges user's values as overrides.
// If no user blueprint exists, all items from primary/sources are retained unchanged.
func (c *BaseBlueprintComposer) applyUserBlueprint(result *blueprintv1alpha1.Blueprint, user *blueprintv1alpha1.Blueprint) error {
	if user == nil {
		return nil
	}

	if len(user.TerraformComponents) > 0 {
		userTfIDs := make(map[string]bool)
		for _, comp := range user.TerraformComponents {
			userTfIDs[comp.GetID()] = true
		}

		var filtered []blueprintv1alpha1.TerraformComponent
		for _, comp := range result.TerraformComponents {
			if userTfIDs[comp.GetID()] {
				filtered = append(filtered, comp)
			}
		}
		result.TerraformComponents = filtered
	}

	if len(user.Kustomizations) > 0 {
		userKustNames := make(map[string]bool)
		for _, k := range user.Kustomizations {
			userKustNames[k.Name] = true
		}

		var filtered []blueprintv1alpha1.Kustomization
		for _, k := range result.Kustomizations {
			if userKustNames[k.Name] {
				filtered = append(filtered, k)
			}
		}
		result.Kustomizations = filtered
	}

	if user.Repository.Url == "" {
		result.Repository = blueprintv1alpha1.Repository{}
	}

	if len(user.Sources) == 0 {
		result.Sources = nil
	} else {
		userSourceNames := make(map[string]bool)
		for _, s := range user.Sources {
			userSourceNames[s.Name] = true
		}

		var filtered []blueprintv1alpha1.Source
		for _, s := range result.Sources {
			if userSourceNames[s.Name] {
				filtered = append(filtered, s)
			}
		}
		result.Sources = filtered
	}

	return result.StrategicMerge(user)
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
	if loadBalancerIPRange != "-" {
		mergedCommonValues["LOADBALANCER_IP_RANGE"] = loadBalancerIPRange
	}
	if lbStart != "" {
		mergedCommonValues["LOADBALANCER_IP_START"] = lbStart
	}
	if lbEnd != "" {
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

// =============================================================================
// Interface Compliance
// =============================================================================

var _ BlueprintComposer = (*BaseBlueprintComposer)(nil)
