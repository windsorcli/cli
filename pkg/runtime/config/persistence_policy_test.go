package config

import "testing"

// =============================================================================
// Test Public Methods
// =============================================================================

func TestPersistencePolicy_Partition(t *testing.T) {
	t.Run("RoutesWorkstationKeyToWorkstationPartition", func(t *testing.T) {
		policy := newPersistencePolicy()
		data := map[string]any{
			"workstation": map[string]any{"runtime": "colima"},
			"custom":      "value",
		}

		partition := policy.Partition(data, persistencePolicyInput{})

		if _, ok := partition.Workstation["workstation"]; !ok {
			t.Fatalf("Expected workstation key in workstation partition")
		}
		if _, ok := partition.Values["workstation"]; ok {
			t.Fatalf("Did not expect workstation key in values partition")
		}
		if partition.Values["custom"] != "value" {
			t.Errorf("Expected custom key in values partition, got %v", partition.Values["custom"])
		}
	})

	t.Run("RoutesPlatformToWorkstationWhenDevContext", func(t *testing.T) {
		policy := newPersistencePolicy()
		data := map[string]any{
			"platform": "docker",
		}

		partition := policy.Partition(data, persistencePolicyInput{
			IsDevMode: true,
		})

		if partition.Workstation["platform"] != "docker" {
			t.Errorf("Expected platform in workstation partition, got %v", partition.Workstation["platform"])
		}
		if _, ok := partition.Values["platform"]; ok {
			t.Errorf("Did not expect platform in values partition")
		}
	})

	t.Run("RoutesPlatformToWorkstationWhenRuntimeConfigured", func(t *testing.T) {
		policy := newPersistencePolicy()
		data := map[string]any{
			"platform": "docker",
		}

		partition := policy.Partition(data, persistencePolicyInput{
			WorkstationRuntime: "colima",
		})

		if partition.Workstation["platform"] != "docker" {
			t.Errorf("Expected platform in workstation partition, got %v", partition.Workstation["platform"])
		}
		if _, ok := partition.Values["platform"]; ok {
			t.Errorf("Did not expect platform in values partition")
		}
	})

	t.Run("RoutesPlatformToValuesWhenDevAndRuntimeNotSet", func(t *testing.T) {
		policy := newPersistencePolicy()
		data := map[string]any{
			"platform": "docker",
		}

		partition := policy.Partition(data, persistencePolicyInput{})

		if partition.Values["platform"] != "docker" {
			t.Errorf("Expected platform in values partition, got %v", partition.Values["platform"])
		}
		if _, ok := partition.Workstation["platform"]; ok {
			t.Errorf("Did not expect platform in workstation partition")
		}
	})

	t.Run("RoutesNonManagedKeysToValuesPartition", func(t *testing.T) {
		policy := newPersistencePolicy()
		data := map[string]any{
			"provider": "docker",
			"cluster":  map[string]any{"workers": map[string]any{"count": 2}},
		}

		partition := policy.Partition(data, persistencePolicyInput{})

		if partition.Values["provider"] != "docker" {
			t.Errorf("Expected provider in values partition, got %v", partition.Values["provider"])
		}
		if _, ok := partition.Workstation["provider"]; ok {
			t.Errorf("Did not expect provider in workstation partition")
		}
		if _, ok := partition.Values["cluster"]; !ok {
			t.Errorf("Expected cluster in values partition")
		}
	})
}
