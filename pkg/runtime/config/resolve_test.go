package config

import (
	"runtime"
	"testing"
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

	t.Run("DoesNotDeriveCountFromNodes", func(t *testing.T) {
		// The generic resolver no longer synthesizes cluster topology; count is left to explicit
		// config or facet `?? N` defaulting (#3062).
		handler, _ := setupPrivateTestHandler(t)

		if err := handler.Set("cluster.controlplanes.nodes.cp1.endpoint", "127.0.0.1:50001"); err != nil {
			t.Fatalf("Expected no error setting cp1, got %v", err)
		}
		if err := handler.Set("cluster.workers.nodes.w1.endpoint", "127.0.0.1:50101"); err != nil {
			t.Fatalf("Expected no error setting w1, got %v", err)
		}

		values, err := handler.GetContextValues()
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		clusterValues := values["cluster"].(map[string]any)
		controlplanesValues := clusterValues["controlplanes"].(map[string]any)
		workersValues := clusterValues["workers"].(map[string]any)
		if v, exists := controlplanesValues["count"]; exists {
			t.Errorf("Expected controlplanes.count not fabricated from nodes, got %v", v)
		}
		if v, exists := workersValues["count"]; exists {
			t.Errorf("Expected workers.count not fabricated from nodes, got %v", v)
		}
	})

	t.Run("LeavesWorkersCountUnsetWhenNoExplicitCountOrNodes", func(t *testing.T) {
		// Given a handler with no workers count and no worker nodes
		handler, _ := setupPrivateTestHandler(t)

		values, err := handler.GetContextValues()
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then workers.count is left unset (nil), so a facet can default it via `?? N` rather than
		// receiving a zero-value that is indistinguishable from an explicit 0
		workersValues := values["cluster"].(map[string]any)["workers"].(map[string]any)
		if v, present := workersValues["count"]; present {
			t.Errorf("Expected workers.count unset when neither count nor nodes are provided, got %v", v)
		}
	})

	t.Run("PreservesExplicitZeroWorkersCount", func(t *testing.T) {
		// Given an explicitly-set workers.count of 0
		handler, _ := setupPrivateTestHandler(t)
		if err := handler.Set("cluster.workers.count", 0); err != nil {
			t.Fatalf("Expected no error setting workers.count, got %v", err)
		}

		values, err := handler.GetContextValues()
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then the explicit 0 is preserved and present (distinct from unset)
		workersValues := values["cluster"].(map[string]any)["workers"].(map[string]any)
		v, present := workersValues["count"]
		if !present || v != 0 {
			t.Errorf("Expected explicit workers.count=0 preserved, got %v (present=%v)", v, present)
		}
	})

	t.Run("MaterializesNestedSchemaDefaultForUnsetField", func(t *testing.T) {
		// Given a schema with a nested default and a non-test context
		handler, _ := setupPrivateTestHandler(t)
		if err := handler.SetContext("local"); err != nil {
			t.Fatalf("Expected no error setting context, got %v", err)
		}
		schema := []byte("$schema: https://json-schema.org/draft/2020-12/schema\n" +
			"type: object\n" +
			"properties:\n" +
			"  network:\n" +
			"    type: object\n" +
			"    properties:\n" +
			"      cidr_block:\n" +
			"        type: string\n" +
			"        default: \"10.5.0.0/16\"\n")
		if err := handler.LoadSchemaFromBytes(schema); err != nil {
			t.Fatalf("Expected no error loading schema, got %v", err)
		}

		values, err := handler.GetContextValues()
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then the unset field resolves to its schema default (so cidrhost(...) succeeds downstream)
		networkValues, ok := values["network"].(map[string]any)
		if !ok {
			t.Fatalf("Expected network map, got %T", values["network"])
		}
		if networkValues["cidr_block"] != "10.5.0.0/16" {
			t.Errorf("Expected network.cidr_block schema default, got %v", networkValues["cidr_block"])
		}
	})

	t.Run("DoesNotDeriveSchedulable", func(t *testing.T) {
		// schedulable is no longer synthesized by the generic resolver; the consuming facets derive
		// it from count values via `?? ` fallbacks (#3062).
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

		controlplanesValues := values["cluster"].(map[string]any)["controlplanes"].(map[string]any)
		if v, exists := controlplanesValues["schedulable"]; exists {
			t.Errorf("Expected schedulable not fabricated by the resolver, got %v", v)
		}
	})

	t.Run("DoesNotFabricateClusterResourceDefaults", func(t *testing.T) {
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
		workersValues := clusterValues["workers"].(map[string]any)
		if _, exists := controlplanesValues["cpu"]; exists {
			t.Errorf("Expected controlplanes.cpu to be absent, got %v", controlplanesValues["cpu"])
		}
		if _, exists := controlplanesValues["memory"]; exists {
			t.Errorf("Expected controlplanes.memory to be absent, got %v", controlplanesValues["memory"])
		}
		if _, exists := workersValues["cpu"]; exists {
			t.Errorf("Expected workers.cpu to be absent, got %v", workersValues["cpu"])
		}
		if _, exists := workersValues["memory"]; exists {
			t.Errorf("Expected workers.memory to be absent, got %v", workersValues["memory"])
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

	t.Run("DerivesTalosDriverFromOmniPlatform", func(t *testing.T) {
		handler, _ := setupPrivateTestHandler(t)
		if err := handler.Set("platform", "omni"); err != nil {
			t.Fatalf("Expected no error setting platform, got %v", err)
		}

		values, err := handler.GetContextValues()
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		clusterValues, ok := values["cluster"].(map[string]any)
		if !ok {
			t.Fatalf("Expected cluster map, got %T", values["cluster"])
		}
		if clusterValues["driver"] != "talos" {
			t.Errorf("Expected cluster.driver=talos, got %v", clusterValues["driver"])
		}
	})

	t.Run("DerivesTalosDriverFromHypervPlatform", func(t *testing.T) {
		handler, _ := setupPrivateTestHandler(t)
		if err := handler.Set("platform", "hyperv"); err != nil {
			t.Fatalf("Expected no error setting platform, got %v", err)
		}

		values, err := handler.GetContextValues()
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		clusterValues, ok := values["cluster"].(map[string]any)
		if !ok {
			t.Fatalf("Expected cluster map, got %T", values["cluster"])
		}
		if clusterValues["driver"] != "talos" {
			t.Errorf("Expected cluster.driver=talos, got %v", clusterValues["driver"])
		}
	})

	t.Run("DerivesTalosDriverFromVspherePlatform", func(t *testing.T) {
		handler, _ := setupPrivateTestHandler(t)
		if err := handler.Set("platform", "vsphere"); err != nil {
			t.Fatalf("Expected no error setting platform, got %v", err)
		}

		values, err := handler.GetContextValues()
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		clusterValues, ok := values["cluster"].(map[string]any)
		if !ok {
			t.Fatalf("Expected cluster map, got %T", values["cluster"])
		}
		if clusterValues["driver"] != "talos" {
			t.Errorf("Expected cluster.driver=talos, got %v", clusterValues["driver"])
		}
	})

	t.Run("DerivesTalosDriverFromHetznerPlatform", func(t *testing.T) {
		handler, _ := setupPrivateTestHandler(t)
		if err := handler.Set("platform", "hetzner"); err != nil {
			t.Fatalf("Expected no error setting platform, got %v", err)
		}

		values, err := handler.GetContextValues()
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		clusterValues, ok := values["cluster"].(map[string]any)
		if !ok {
			t.Fatalf("Expected cluster map, got %T", values["cluster"])
		}
		if clusterValues["driver"] != "talos" {
			t.Errorf("Expected cluster.driver=talos, got %v", clusterValues["driver"])
		}
	})

	t.Run("DerivesCloudDriverFromPlatform", func(t *testing.T) {
		handler, _ := setupPrivateTestHandler(t)
		if err := handler.Set("platform", "azure"); err != nil {
			t.Fatalf("Expected no error setting platform, got %v", err)
		}

		values, err := handler.GetContextValues()
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		clusterValues := values["cluster"].(map[string]any)
		if clusterValues["driver"] != "aks" {
			t.Errorf("Expected cluster.driver=aks, got %v", clusterValues["driver"])
		}
	})

	t.Run("DoesNotOverrideExplicitDerivedTargets", func(t *testing.T) {
		handler, _ := setupPrivateTestHandler(t)
		if err := handler.Set("platform", "aws"); err != nil {
			t.Fatalf("Expected no error setting platform, got %v", err)
		}
		if err := handler.Set("cluster.driver", "talos"); err != nil {
			t.Fatalf("Expected no error setting cluster.driver, got %v", err)
		}
		if err := handler.Set("aws.enabled", false); err != nil {
			t.Fatalf("Expected no error setting aws.enabled, got %v", err)
		}

		values, err := handler.GetContextValues()
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		clusterValues := values["cluster"].(map[string]any)
		if clusterValues["driver"] != "talos" {
			t.Errorf("Expected explicit cluster.driver to remain talos, got %v", clusterValues["driver"])
		}
	})
}
