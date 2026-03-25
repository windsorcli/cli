package config

import (
	"runtime"
	"testing"

	"github.com/windsorcli/cli/pkg/constants"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestConfigHandler_GetContextValues_Resolve(t *testing.T) {
	t.Run("AppliesWorkstationArchDefaultWhenMissing", func(t *testing.T) {
		handler, _ := setupPrivateTestHandler(t)

		values, err := handler.GetContextValues()
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		workstationValues, ok := values["workstation"].(map[string]any)
		if !ok {
			t.Fatalf("Expected workstation map, got %T", values["workstation"])
		}

		expectedArch := runtime.GOARCH
		if expectedArch == "arm" {
			expectedArch = "arm64"
		}
		if workstationValues["arch"] != expectedArch {
			t.Errorf("Expected workstation.arch=%s, got %v", expectedArch, workstationValues["arch"])
		}
	})

	t.Run("EnsuresClusterStructureMaps", func(t *testing.T) {
		handler, _ := setupPrivateTestHandler(t)

		values, err := handler.GetContextValues()
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		clusterValues, ok := values["cluster"].(map[string]any)
		if !ok {
			t.Fatalf("Expected cluster map, got %T", values["cluster"])
		}
		controlplanesValues, ok := clusterValues["controlplanes"].(map[string]any)
		if !ok {
			t.Fatalf("Expected controlplanes map, got %T", clusterValues["controlplanes"])
		}
		if _, ok := controlplanesValues["nodes"].(map[string]any); !ok {
			t.Fatalf("Expected controlplanes.nodes map, got %T", controlplanesValues["nodes"])
		}
		workersValues, ok := clusterValues["workers"].(map[string]any)
		if !ok {
			t.Fatalf("Expected workers map, got %T", clusterValues["workers"])
		}
		if _, ok := workersValues["nodes"].(map[string]any); !ok {
			t.Fatalf("Expected workers.nodes map, got %T", workersValues["nodes"])
		}
	})

	t.Run("DerivesControlplaneAndWorkerCountFromExplicitNodes", func(t *testing.T) {
		handler, _ := setupPrivateTestHandler(t)

		if err := handler.Set("cluster.controlplanes.nodes.cp1.endpoint", "127.0.0.1:50001"); err != nil {
			t.Fatalf("Expected no error setting cp1, got %v", err)
		}
		if err := handler.Set("cluster.controlplanes.nodes.cp2.endpoint", "127.0.0.1:50002"); err != nil {
			t.Fatalf("Expected no error setting cp2, got %v", err)
		}
		if err := handler.Set("cluster.workers.nodes.w1.endpoint", "127.0.0.1:50101"); err != nil {
			t.Fatalf("Expected no error setting w1, got %v", err)
		}
		if err := handler.Set("cluster.workers.nodes.w2.endpoint", "127.0.0.1:50102"); err != nil {
			t.Fatalf("Expected no error setting w2, got %v", err)
		}

		values, err := handler.GetContextValues()
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		clusterValues := values["cluster"].(map[string]any)
		controlplanesValues := clusterValues["controlplanes"].(map[string]any)
		workersValues := clusterValues["workers"].(map[string]any)

		if controlplanesValues["count"] != 2 {
			t.Errorf("Expected controlplanes count=2, got %v", controlplanesValues["count"])
		}
		if workersValues["count"] != 2 {
			t.Errorf("Expected workers count=2, got %v", workersValues["count"])
		}
	})

	t.Run("DerivesSchedulableWhenNotExplicit", func(t *testing.T) {
		handler, _ := setupPrivateTestHandler(t)

		if err := handler.Set("cluster.controlplanes.count", 1); err != nil {
			t.Fatalf("Expected no error setting controlplanes.count, got %v", err)
		}
		if err := handler.Set("cluster.workers.count", 0); err != nil {
			t.Fatalf("Expected no error setting workers.count, got %v", err)
		}

		values, err := handler.GetContextValues()
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		clusterValues := values["cluster"].(map[string]any)
		controlplanesValues := clusterValues["controlplanes"].(map[string]any)
		schedulable, ok := controlplanesValues["schedulable"].(bool)
		if !ok {
			t.Fatalf("Expected bool schedulable, got %T", controlplanesValues["schedulable"])
		}
		if !schedulable {
			t.Errorf("Expected schedulable=true, got %v", schedulable)
		}
	})

	t.Run("AppliesDedicatedVsSchedulableClusterResourceDefaults", func(t *testing.T) {
		schedulableHandler, _ := setupPrivateTestHandler(t)

		if err := schedulableHandler.Set("cluster.controlplanes.count", 1); err != nil {
			t.Fatalf("Expected no error setting controlplanes.count, got %v", err)
		}
		if err := schedulableHandler.Set("cluster.workers.count", 0); err != nil {
			t.Fatalf("Expected no error setting workers.count, got %v", err)
		}

		values, err := schedulableHandler.GetContextValues()
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		clusterValues := values["cluster"].(map[string]any)
		controlplanesValues := clusterValues["controlplanes"].(map[string]any)
		workersValues := clusterValues["workers"].(map[string]any)
		if controlplanesValues["cpu"] != constants.DefaultControlPlaneCPUSchedulable {
			t.Errorf("Expected schedulable controlplane cpu=%d, got %v", constants.DefaultControlPlaneCPUSchedulable, controlplanesValues["cpu"])
		}
		if controlplanesValues["memory"] != constants.DefaultControlPlaneMemorySchedulable {
			t.Errorf("Expected schedulable controlplane memory=%d, got %v", constants.DefaultControlPlaneMemorySchedulable, controlplanesValues["memory"])
		}
		if workersValues["cpu"] != constants.DefaultWorkerCPU {
			t.Errorf("Expected worker cpu=%d, got %v", constants.DefaultWorkerCPU, workersValues["cpu"])
		}
		if workersValues["memory"] != constants.DefaultWorkerMemory {
			t.Errorf("Expected worker memory=%d, got %v", constants.DefaultWorkerMemory, workersValues["memory"])
		}

		dedicatedHandler, _ := setupPrivateTestHandler(t)
		if err := dedicatedHandler.Set("cluster.controlplanes.count", 1); err != nil {
			t.Fatalf("Expected no error setting controlplanes.count, got %v", err)
		}
		if err := dedicatedHandler.Set("cluster.workers.count", 1); err != nil {
			t.Fatalf("Expected no error setting workers.count, got %v", err)
		}
		values, err = dedicatedHandler.GetContextValues()
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		clusterValues = values["cluster"].(map[string]any)
		controlplanesValues = clusterValues["controlplanes"].(map[string]any)
		if controlplanesValues["cpu"] != constants.DefaultControlPlaneCPUDedicated {
			t.Errorf("Expected dedicated controlplane cpu=%d, got %v", constants.DefaultControlPlaneCPUDedicated, controlplanesValues["cpu"])
		}
		if controlplanesValues["memory"] != constants.DefaultControlPlaneMemoryDedicated {
			t.Errorf("Expected dedicated controlplane memory=%d, got %v", constants.DefaultControlPlaneMemoryDedicated, controlplanesValues["memory"])
		}
	})

	t.Run("PreservesExplicitClusterOverrides", func(t *testing.T) {
		handler, _ := setupPrivateTestHandler(t)

		if err := handler.Set("cluster.controlplanes.count", 3); err != nil {
			t.Fatalf("Expected no error setting controlplanes.count, got %v", err)
		}
		if err := handler.Set("cluster.controlplanes.cpu", 99); err != nil {
			t.Fatalf("Expected no error setting controlplanes.cpu, got %v", err)
		}
		if err := handler.Set("cluster.controlplanes.memory", 11111); err != nil {
			t.Fatalf("Expected no error setting controlplanes.memory, got %v", err)
		}
		if err := handler.Set("cluster.workers.count", 4); err != nil {
			t.Fatalf("Expected no error setting workers.count, got %v", err)
		}
		if err := handler.Set("cluster.workers.cpu", 77); err != nil {
			t.Fatalf("Expected no error setting workers.cpu, got %v", err)
		}
		if err := handler.Set("cluster.workers.memory", 8888); err != nil {
			t.Fatalf("Expected no error setting workers.memory, got %v", err)
		}

		values, err := handler.GetContextValues()
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		clusterValues := values["cluster"].(map[string]any)
		controlplanesValues := clusterValues["controlplanes"].(map[string]any)
		workersValues := clusterValues["workers"].(map[string]any)

		if controlplanesValues["count"] != 3 {
			t.Errorf("Expected controlplanes.count=3, got %v", controlplanesValues["count"])
		}
		if controlplanesValues["cpu"] != 99 {
			t.Errorf("Expected controlplanes.cpu=99, got %v", controlplanesValues["cpu"])
		}
		if controlplanesValues["memory"] != 11111 {
			t.Errorf("Expected controlplanes.memory=11111, got %v", controlplanesValues["memory"])
		}
		if workersValues["count"] != 4 {
			t.Errorf("Expected workers.count=4, got %v", workersValues["count"])
		}
		if workersValues["cpu"] != 77 {
			t.Errorf("Expected workers.cpu=77, got %v", workersValues["cpu"])
		}
		if workersValues["memory"] != 8888 {
			t.Errorf("Expected workers.memory=8888, got %v", workersValues["memory"])
		}
	})
}
