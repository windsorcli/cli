package blueprint

import (
	"math"
	"reflect"
	"testing"
)

// =============================================================================
// Test Private Methods
// =============================================================================

func TestProcessor_mergeHelpers(t *testing.T) {
	t.Run("deepMergeMapMergesNestedMaps", func(t *testing.T) {
		base := map[string]any{"a": 1, "b": map[string]any{"x": 10}}
		overlay := map[string]any{"b": map[string]any{"y": 20}, "c": 3}
		out := deepMergeMap(base, overlay)
		if out["a"].(int) != 1 {
			t.Errorf("Expected a=1, got %v", out["a"])
		}
		if out["c"].(int) != 3 {
			t.Errorf("Expected c=3, got %v", out["c"])
		}
		b := out["b"].(map[string]any)
		if b["x"].(int) != 10 {
			t.Errorf("Expected b.x=10, got %v", b["x"])
		}
		if b["y"].(int) != 20 {
			t.Errorf("Expected b.y=20, got %v", b["y"])
		}
	})

	t.Run("MergeScopeMapsDeepMergesSameBlockName", func(t *testing.T) {
		globalScope := map[string]any{
			"talos": map[string]any{"controlplanes": []any{"cp0"}, "workers": []any{"w0"}},
		}
		facetConfig := map[string]any{
			"talos": map[string]any{"workers": []any{"w1", "w2"}},
		}
		out := MergeScopeMaps(globalScope, facetConfig)
		talos := out["talos"].(map[string]any)
		if _, ok := talos["controlplanes"]; !ok {
			t.Error("Expected controlplanes from global to remain")
		}
		workers := talos["workers"].([]any)
		if len(workers) != 2 {
			t.Errorf("Expected overlay workers to overwrite, got %d", len(workers))
		}
	})

	t.Run("OperatorOverlayConfigBlocksBuildsAnEntryForACollidingName", func(t *testing.T) {
		globalScope := map[string]any{
			"cluster": map[string]any{"controlplanes": map[string]any{"cpu": 7}},
		}
		operatorOverlay := map[string]any{
			"cluster": map[string]any{"controlplanes": map[string]any{"cpu": 3}},
		}
		incoming := operatorOverlayConfigBlocks(globalScope, operatorOverlay)
		cluster, ok := incoming["cluster"]
		if !ok {
			t.Fatal("Expected a synthetic entry for 'cluster'")
		}
		if cluster.Strategy != "merge" {
			t.Errorf("Expected strategy 'merge', got %q", cluster.Strategy)
		}
		if cluster.Ordinal == nil || *cluster.Ordinal != math.MaxInt {
			t.Errorf("Expected ordinal math.MaxInt, got %v", cluster.Ordinal)
		}
		if !reflect.DeepEqual(cluster.Body["value"], operatorOverlay["cluster"]) {
			t.Errorf("Expected body value %v, got %v", operatorOverlay["cluster"], cluster.Body["value"])
		}
	})

	t.Run("OperatorOverlayConfigBlocksSkipsNamesAbsentFromGlobalScope", func(t *testing.T) {
		globalScope := map[string]any{
			"cluster": map[string]any{"controlplanes": map[string]any{"cpu": 7}},
		}
		operatorOverlay := map[string]any{
			"cluster":   map[string]any{"controlplanes": map[string]any{"cpu": 3}},
			"terraform": map[string]any{"enabled": true},
		}
		incoming := operatorOverlayConfigBlocks(globalScope, operatorOverlay)
		if _, exists := incoming["terraform"]; exists {
			t.Error("Expected an operator key with no matching config block to be skipped")
		}
	})

	t.Run("OperatorOverlayConfigBlocksHandlesNilGlobalScope", func(t *testing.T) {
		if incoming := operatorOverlayConfigBlocks(nil, map[string]any{"cluster": map[string]any{"cpu": 3}}); incoming != nil {
			t.Errorf("Expected nil for nil globalScope, got %v", incoming)
		}
	})

	t.Run("OperatorOverlayConfigBlocksHandlesEmptyOverlay", func(t *testing.T) {
		globalScope := map[string]any{"cluster": map[string]any{"cpu": 7}}
		if incoming := operatorOverlayConfigBlocks(globalScope, nil); incoming != nil {
			t.Errorf("Expected nil for an empty overlay, got %v", incoming)
		}
	})

	t.Run("OperatorOverlayConfigBlocksMergedIntoScopeOverridesCollidingKeyAndKeepsSiblings", func(t *testing.T) {
		// Given a merge helper's synthetic entry fed through the same mergeConfigBlocks
		// pipeline every facet-vs-facet conflict already uses
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)
		globalScope := map[string]any{
			"cluster": map[string]any{"controlplanes": map[string]any{"cpu": 7, "memory": 16}},
		}
		operatorOverlay := map[string]any{
			"cluster": map[string]any{"controlplanes": map[string]any{"cpu": 3}},
		}
		incoming := operatorOverlayConfigBlocks(globalScope, operatorOverlay)

		// When merged via mergeConfigBlocks
		out, _, err := processor.mergeConfigBlocks(globalScope, nil, incoming)
		if err != nil {
			t.Fatalf("mergeConfigBlocks failed: %v", err)
		}

		// Then the operator's value wins at the colliding key, and the untouched sibling remains
		controlplanes := out["cluster"].(map[string]any)["controlplanes"].(map[string]any)
		if controlplanes["cpu"] != 3 {
			t.Errorf("Expected operator cpu=3 to win, got %v", controlplanes["cpu"])
		}
		if controlplanes["memory"] != 16 {
			t.Errorf("Expected untouched key memory=16 to remain, got %v", controlplanes["memory"])
		}
	})
}
