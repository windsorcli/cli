package config

import (
	"runtime"
	"strings"
)

// The ConfigResolver is an effective-values materialization component for runtime config.
// It provides schema/data merge and ordered derivation of workstation and cluster defaults,
// The ConfigResolver keeps domain-specific default synthesis in one canonical mechanism,
// and avoids indirection-heavy resolver object models while preserving deterministic behavior.

// =============================================================================
// Public Methods
// =============================================================================

// GetContextValues returns a merged configuration map composed of schema defaults and current config data.
// The result provides all configuration values, with schema defaults filled in for missing keys, ensuring
// downstream consumers (such as blueprint processing) always receive a complete set of config values.
// If the schema validator or schema is unavailable, only the currently loaded data is returned.
// It also ensures that cluster.controlplanes.nodes and cluster.workers.nodes are initialized as empty maps
// even though they are not serialized to YAML, so template expressions can safely evaluate them.
// For test contexts, schema defaults are skipped to ensure tests only use explicitly provided values.
func (c *configHandler) GetContextValues() (map[string]any, error) {
	result := make(map[string]any)

	contextName := c.GetContext()
	skipSchemaDefaults := contextName == "test" || strings.HasPrefix(contextName, "test-")
	if !skipSchemaDefaults && c.schemaValidator != nil && c.schemaValidator.Schema != nil {
		defaults, err := c.schemaValidator.GetSchemaDefaults()
		if err == nil && defaults != nil {
			result = c.deepMerge(result, defaults)
		}
	}

	result = c.deepMerge(result, c.data)
	c.applyPlatformDerivedDefaults(result)
	c.applyWorkstationDefaults(result)
	c.ensureClusterStructure(result)

	return result, nil
}

// =============================================================================
// Private Methods
// =============================================================================

// applyPlatformDerivedDefaults injects derived platform defaults into effective values without mutating stored config.
func (c *configHandler) applyPlatformDerivedDefaults(values map[string]any) {
	platform := ""
	if p, ok := getValueByPathFromMap(values, parsePath("platform")).(string); ok {
		platform = p
	}
	if platform == "" {
		if p, ok := getValueByPathFromMap(values, parsePath("provider")).(string); ok {
			platform = p
		}
	}

	switch platform {
	case "docker", "incus", "metal", "hetzner", "omni", "hyperv", "vsphere":
		c.setDerivedValueIfMissing(values, "cluster.driver", "talos")
	case "aws":
		c.setDerivedValueIfMissing(values, "cluster.driver", "eks")
	case "azure":
		c.setDerivedValueIfMissing(values, "cluster.driver", "aks")
	case "gcp":
		c.setDerivedValueIfMissing(values, "cluster.driver", "gke")
	}
}

// setDerivedValueIfMissing writes a derived value only when the path is not already present.
func (c *configHandler) setDerivedValueIfMissing(values map[string]any, path string, value any) {
	pathKeys := parsePath(path)
	if hasValueAtPath(values, pathKeys) {
		return
	}
	setValueInMap(values, pathKeys, value)
}

// applyWorkstationDefaults materializes workstation defaults required by runtime consumers.
func (c *configHandler) applyWorkstationDefaults(values map[string]any) {
	workstationMap, _ := values["workstation"].(map[string]any)
	if workstationMap == nil {
		workstationMap = make(map[string]any)
		values["workstation"] = workstationMap
	}
	if _, exists := workstationMap["arch"]; exists {
		return
	}

	arch := runtime.GOARCH
	if arch == "arm" {
		arch = "arm64"
	}
	workstationMap["arch"] = arch
}

// ensureClusterStructure initializes cluster/controlplanes/workers map shape for evaluators.
func (c *configHandler) ensureClusterStructure(values map[string]any) {
	clusterMap, ok := values["cluster"].(map[string]any)
	if !ok {
		if _, exists := values["cluster"]; !exists || values["cluster"] == nil {
			clusterMap = map[string]any{
				"controlplanes": map[string]any{
					"nodes": make(map[string]any),
				},
				"workers": map[string]any{
					"nodes": make(map[string]any),
				},
			}
			values["cluster"] = clusterMap
		}
		return
	}

	controlplanesMap, controlplanesIsMap := clusterMap["controlplanes"].(map[string]any)
	if !controlplanesIsMap || controlplanesMap == nil {
		controlplanesMap = map[string]any{}
		clusterMap["controlplanes"] = controlplanesMap
	}
	if _, exists := controlplanesMap["nodes"]; !exists {
		controlplanesMap["nodes"] = make(map[string]any)
	}

	workersMap, workersIsMap := clusterMap["workers"].(map[string]any)
	if !workersIsMap || workersMap == nil {
		workersMap = map[string]any{}
		clusterMap["workers"] = workersMap
	}
	if _, exists := workersMap["nodes"]; !exists {
		workersMap["nodes"] = make(map[string]any)
	}
}

