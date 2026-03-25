package config

import (
	"runtime"
	"strconv"
	"strings"

	"github.com/windsorcli/cli/pkg/constants"
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
	c.applyWorkstationDefaults(result)
	c.ensureClusterStructure(result)
	c.applyClusterTopologyDefaults(result)
	c.applyClusterResourceDefaults(result)

	return result, nil
}

// =============================================================================
// Private Methods
// =============================================================================

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

// applyClusterTopologyDefaults computes count and schedulable defaults from explicit values and node shape.
func (c *configHandler) applyClusterTopologyDefaults(values map[string]any) {
	clusterMap, ok := values["cluster"].(map[string]any)
	if !ok || clusterMap == nil {
		return
	}
	controlplanesMap, controlplanesOK := clusterMap["controlplanes"].(map[string]any)
	workersMap, workersOK := clusterMap["workers"].(map[string]any)
	if !controlplanesOK || controlplanesMap == nil || !workersOK || workersMap == nil {
		return
	}

	hasExplicitControlplaneCount := hasValueAtPath(c.data, []string{"cluster", "controlplanes", "count"})
	controlplaneCount, hasControlplaneCount := getMapInt(values, "cluster.controlplanes.count")
	if !hasExplicitControlplaneCount || !hasControlplaneCount {
		if derivedCount, derived := getExplicitNodeCountFromData(c.data, []string{"cluster", "controlplanes", "nodes"}); derived {
			controlplaneCount = derivedCount
		} else {
			controlplaneCount = 1
		}
		controlplanesMap["count"] = controlplaneCount
	}

	hasExplicitWorkersCount := hasValueAtPath(c.data, []string{"cluster", "workers", "count"})
	workersCount, hasWorkersCount := getMapInt(values, "cluster.workers.count")
	if !hasExplicitWorkersCount || !hasWorkersCount {
		if derivedCount, derived := getExplicitNodeCountFromData(c.data, []string{"cluster", "workers", "nodes"}); derived {
			workersCount = derivedCount
		} else {
			workersCount = 0
		}
		workersMap["count"] = workersCount
	}

	schedulable, hasSchedulable := controlplanesMap["schedulable"].(bool)
	hasExplicitSchedulable := hasValueAtPath(c.data, []string{"cluster", "controlplanes", "schedulable"})
	if !hasSchedulable || !hasExplicitSchedulable {
		schedulable = workersCount == 0 && controlplaneCount == 1
		controlplanesMap["schedulable"] = schedulable
	}
}

// applyClusterResourceDefaults injects CPU and memory defaults into cluster values when absent.
func (c *configHandler) applyClusterResourceDefaults(values map[string]any) {
	clusterMap, ok := values["cluster"].(map[string]any)
	if !ok || clusterMap == nil {
		return
	}
	controlplanesMap, controlplanesOK := clusterMap["controlplanes"].(map[string]any)
	workersMap, workersOK := clusterMap["workers"].(map[string]any)
	if !controlplanesOK || controlplanesMap == nil || !workersOK || workersMap == nil {
		return
	}

	schedulable, _ := controlplanesMap["schedulable"].(bool)
	defaultControlplaneCPU := constants.DefaultControlPlaneCPUDedicated
	defaultControlplaneMemory := constants.DefaultControlPlaneMemoryDedicated
	if schedulable {
		defaultControlplaneCPU = constants.DefaultControlPlaneCPUSchedulable
		defaultControlplaneMemory = constants.DefaultControlPlaneMemorySchedulable
	}
	if _, exists := controlplanesMap["cpu"]; !exists {
		controlplanesMap["cpu"] = defaultControlplaneCPU
	}
	if _, exists := controlplanesMap["memory"]; !exists {
		controlplanesMap["memory"] = defaultControlplaneMemory
	}
	if _, exists := workersMap["cpu"]; !exists {
		workersMap["cpu"] = constants.DefaultWorkerCPU
	}
	if _, exists := workersMap["memory"]; !exists {
		workersMap["memory"] = constants.DefaultWorkerMemory
	}
}

// getExplicitNodeCountFromData returns node map length only when nodes were explicitly provided in loaded config data.
func getExplicitNodeCountFromData(data map[string]any, pathKeys []string) (int, bool) {
	if !hasValueAtPath(data, pathKeys) {
		return 0, false
	}
	nodesAny := getValueByPathFromMap(data, pathKeys)
	nodes, ok := nodesAny.(map[string]any)
	if !ok {
		return 0, true
	}
	return len(nodes), true
}

// getMapInt returns an int value from a map key path with permissive numeric conversion.
func getMapInt(data map[string]any, path string) (int, bool) {
	value := getValueByPathFromMap(data, parsePath(path))
	if value == nil {
		return 0, false
	}
	switch v := value.(type) {
	case int:
		return v, true
	case int64:
		maxInt := int64(^uint(0) >> 1)
		minInt := -maxInt - 1
		if v > maxInt || v < minInt {
			return 0, false
		}
		return int(v), true
	case uint64:
		if v > uint64(^uint(0)>>1) {
			return 0, false
		}
		return int(v), true
	case uint:
		if v > uint(^uint(0)>>1) {
			return 0, false
		}
		return int(v), true
	case float64:
		return int(v), true
	case string:
		if parsed, err := strconv.Atoi(v); err == nil {
			return parsed, true
		}
	}
	return 0, false
}
