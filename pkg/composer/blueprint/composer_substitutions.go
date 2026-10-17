// The composer_substitutions file holds the substitution and patch steps of the BlueprintComposer.
// It provides the common substitutions ConfigMap and the per-kustomization substitutions.
// It applies context patches from contexts/<context>/patches/<name>/ to the matching entry.
// A patch directory name matches a kustomization name or a FluxSystem tier name.

package blueprint

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/fluxcd/pkg/apis/kustomize"
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/internal/presenter"
	"github.com/windsorcli/cli/internal/werror"
)

// =============================================================================
// Private Methods
// =============================================================================

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

// discoverContextPatches applies the patches in contexts/<context>/patches/<name>/ to the matching
// kustomization or FluxSystem tier. The name is a kustomization name, a compiled tier name, or a
// FluxSystem name (see buildFluxTierMap). An unmatched directory causes a warning only. A patch
// directory or file that cannot be read or parsed is an error. It skips entries whose name starts
// with a dot, such as macOS AppleDouble files.
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
		return werror.Wrap(err, "failed to read patches directory")
	}

	kustomizationMap := make(map[string]*blueprintv1alpha1.Kustomization)
	for i := range blueprint.Kustomizations {
		kustomizationMap[blueprint.Kustomizations[i].Name] = &blueprint.Kustomizations[i]
	}
	fluxTierMap := buildFluxTierMap(blueprint)
	validNames := validPatchTargetNames(kustomizationMap, fluxTierMap)

	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		if !entry.IsDir() {
			continue
		}

		kustomizationName := entry.Name()
		kustomization, exists := kustomizationMap[kustomizationName]
		if !exists {
			kustomization, exists = fluxTierMap[kustomizationName]
		}
		if !exists {
			presenter.Warn(presenter.Background(), fmt.Sprintf("Windsor ignores the patches in %s, because no kustomization or FluxSystem tier has that name. Valid names: %s",
				projectRelativePath(c.runtime, filepath.Join(patchesDir, kustomizationName)), validNames))
			continue
		}

		kustomizationPatchesDir := filepath.Join(patchesDir, kustomizationName)
		patchFiles, err := c.shims.ReadDir(kustomizationPatchesDir)
		if err != nil {
			return werror.Wrap(err, "failed to read patches directory %s", kustomizationPatchesDir)
		}

		for _, patchFile := range patchFiles {
			if patchFile.IsDir() {
				continue
			}

			fileName := patchFile.Name()
			if strings.HasPrefix(fileName, ".") {
				continue
			}
			if !strings.HasSuffix(fileName, ".yaml") && !strings.HasSuffix(fileName, ".yml") {
				continue
			}

			patchPath := filepath.Join(kustomizationPatchesDir, fileName)
			patchData, err := c.shims.ReadFile(patchPath)
			if err != nil {
				return werror.Wrap(err, "failed to read patch file %s", patchPath)
			}

			patch, err := c.parsePatch(patchData, patchPath)
			if err != nil {
				return err
			}

			if patch != nil {
				kustomization.Patches = append(kustomization.Patches, *patch)
			}
		}
	}

	return nil
}

// validPatchTargetNames returns the sorted, deduplicated, comma-separated list of names a
// context-patch directory may match. The list combines every plain kustomization name with
// every compiled tier name for a FluxSystem. Windsor shows this list in the unmatched-directory
// warning. This lets an operator see the valid names without cross-referencing the blueprint.
func validPatchTargetNames(kustomizationMap, fluxTierMap map[string]*blueprintv1alpha1.Kustomization) string {
	merged := accumulateMapKeys(kustomizationMap, fluxTierMap)
	names := make([]string, 0, len(merged))
	for name := range merged {
		names = append(names, name)
	}
	sort.Strings(names)
	if len(names) == 0 {
		return "(none)"
	}
	return strings.Join(names, ", ")
}

// buildFluxTierMap maps each name that a patch directory can use to a FluxSystem tier. Each compiled
// tier name ("<system>-install", "<system>-resources[-<variant>]") maps to its tier. The system name
// maps to the install tier, or to the only resources variant when there is no install tier. A system
// name that could mean more than one tier is not in the map. The pointers refer to the blueprint's
// tiers, so callers can change them in place.
func buildFluxTierMap(blueprint *blueprintv1alpha1.Blueprint) map[string]*blueprintv1alpha1.Kustomization {
	tierMap := make(map[string]*blueprintv1alpha1.Kustomization)
	for i := range blueprint.FluxSystems {
		sys := &blueprint.FluxSystems[i]

		var bareCandidate *blueprintv1alpha1.Kustomization
		bareAmbiguous := false

		if sys.Install != nil {
			tierMap[sys.Name+"-install"] = sys.Install
			bareCandidate = sys.Install
		}

		for j := range sys.Resources {
			variantName := sys.Name + "-resources"
			if sys.Resources[j].Name != "" {
				variantName += "-" + sys.Resources[j].Name
			}
			tierMap[variantName] = &sys.Resources[j].Kustomization
			if bareCandidate == nil {
				bareCandidate = &sys.Resources[j].Kustomization
			} else if sys.Install == nil {
				bareAmbiguous = true
			}
		}

		if bareCandidate != nil && !bareAmbiguous {
			tierMap[sys.Name] = bareCandidate
		}
	}
	return tierMap
}

// parsePatch parses a patch file and returns a BlueprintPatch. It detects whether the patch is
// a strategic merge patch (standard Kubernetes YAML) or a JSON 6902 patch (with a patches field).
// For JSON 6902 patches, it extracts the target selector from the resource metadata.
// Returns nil if the patch data is empty or whitespace-only.
func (c *BaseBlueprintComposer) parsePatch(data []byte, path string) (*blueprintv1alpha1.BlueprintPatch, error) {
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil, nil
	}

	var patchContent map[string]any
	if err := c.shims.YamlUnmarshal(data, &patchContent); err != nil {
		return nil, werror.Wrap(err, "failed to parse patch file %s", path)
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
			return nil, werror.Wrap(err, "failed to marshal JSON patch operations")
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
