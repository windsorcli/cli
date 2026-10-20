package blueprint

import (
	"slices"
	"strings"
	"testing"
	"time"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/internal/werror"
)

// =============================================================================
// Test Private Methods
// =============================================================================

func TestProcessor_updateTerraformComponentEntry(t *testing.T) {
	mocks := setupProcessorMocks(t)
	processor := NewBlueprintProcessor(mocks.Runtime)

	t.Run("ReplacesWhenNewStrategyHasHigherPrecedence", func(t *testing.T) {
		// Given existing entry with merge strategy
		entries := map[string]*blueprintv1alpha1.ConditionalTerraformComponent{
			"vpc": {
				Strategy:           "merge",
				TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"key1": "value1"}},
			},
		}

		// When updating with replace strategy
		new := &blueprintv1alpha1.ConditionalTerraformComponent{
			TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"key2": "value2"}},
		}
		err := processor.updateTerraformComponentEntry("vpc", new, "replace", entries, nil)

		// Then entry should be replaced
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if entries["vpc"].Strategy != "replace" {
			t.Errorf("Expected strategy 'replace', got '%s'", entries["vpc"].Strategy)
		}
		if entries["vpc"].Inputs["key2"] != "value2" {
			t.Error("Expected new inputs to be set")
		}
	})

	t.Run("PreMergesWhenBothHaveMergeStrategy", func(t *testing.T) {
		// Given existing entry with merge strategy
		entries := map[string]*blueprintv1alpha1.ConditionalTerraformComponent{
			"vpc": {
				Strategy:           "merge",
				TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"key1": "value1"}},
			},
		}

		// When updating with merge strategy
		new := &blueprintv1alpha1.ConditionalTerraformComponent{
			TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"key2": "value2"}},
		}
		err := processor.updateTerraformComponentEntry("vpc", new, "merge", entries, nil)

		// Then entries should be pre-merged
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if entries["vpc"].Strategy != "merge" {
			t.Errorf("Expected strategy 'merge', got '%s'", entries["vpc"].Strategy)
		}
		inputs := entries["vpc"].Inputs
		if inputs["key1"] != "value1" {
			t.Errorf("Expected key1 preserved, got %v", inputs["key1"])
		}
		if inputs["key2"] != "value2" {
			t.Errorf("Expected key2 added, got %v", inputs["key2"])
		}
	})

	t.Run("IgnoresWhenNewStrategyHasLowerPrecedence", func(t *testing.T) {
		// Given existing entry with replace strategy
		entries := map[string]*blueprintv1alpha1.ConditionalTerraformComponent{
			"vpc": {
				Strategy:           "replace",
				TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"key1": "value1"}},
			},
		}

		// When updating with merge strategy
		new := &blueprintv1alpha1.ConditionalTerraformComponent{
			TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"key2": "value2"}},
		}
		err := processor.updateTerraformComponentEntry("vpc", new, "merge", entries, nil)

		// Then entry should remain unchanged
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if entries["vpc"].Strategy != "replace" {
			t.Errorf("Expected strategy 'replace' to be preserved, got '%s'", entries["vpc"].Strategy)
		}
		if entries["vpc"].Inputs["key1"] != "value1" {
			t.Error("Expected original inputs to be preserved")
		}
	})

	t.Run("AccumulatesRemovalsWhenBothHaveRemoveStrategy", func(t *testing.T) {
		// Given existing entry with remove strategy
		entries := map[string]*blueprintv1alpha1.ConditionalTerraformComponent{
			"vpc": {
				Strategy:           "remove",
				TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"key1": nil}, DependsOn: []string{"dep1"}},
			},
		}

		// When updating with remove strategy
		new := &blueprintv1alpha1.ConditionalTerraformComponent{
			TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"key2": nil}, DependsOn: []string{"dep2"}},
		}
		err := processor.updateTerraformComponentEntry("vpc", new, "remove", entries, nil)

		// Then removals should be accumulated
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if entries["vpc"].Strategy != "remove" {
			t.Errorf("Expected strategy 'remove', got '%s'", entries["vpc"].Strategy)
		}
		inputs := entries["vpc"].Inputs
		if inputs == nil {
			t.Fatal("Expected inputs map to exist")
		}
		if _, exists := inputs["key1"]; !exists {
			t.Error("Expected key1 to be in removal list")
		}
		if _, exists := inputs["key2"]; !exists {
			t.Error("Expected key2 to be in removal list")
		}
		deps := entries["vpc"].DependsOn
		if !contains(deps, "dep1") {
			t.Error("Expected dep1 to be in removal list")
		}
		if !contains(deps, "dep2") {
			t.Error("Expected dep2 to be in removal list")
		}
	})

	t.Run("ReplacesWhenBothHaveReplaceStrategy", func(t *testing.T) {
		// Given existing entry with replace strategy
		entries := map[string]*blueprintv1alpha1.ConditionalTerraformComponent{
			"vpc": {
				Strategy:           "replace",
				TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"key1": "value1"}},
			},
		}

		// When updating with replace strategy
		new := &blueprintv1alpha1.ConditionalTerraformComponent{
			TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"key2": "value2"}},
		}
		err := processor.updateTerraformComponentEntry("vpc", new, "replace", entries, nil)

		// Then new entry should replace existing
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if entries["vpc"].Strategy != "replace" {
			t.Errorf("Expected strategy 'replace', got '%s'", entries["vpc"].Strategy)
		}
		if entries["vpc"].Inputs["key2"] != "value2" {
			t.Error("Expected new inputs to replace old ones")
		}
		if entries["vpc"].Inputs["key1"] != nil {
			t.Error("Expected old inputs to be replaced")
		}
	})

	t.Run("ReplacesWhenNewHasHigherOrdinal", func(t *testing.T) {
		// Given existing entry with replace strategy and ordinal 0
		entries := map[string]*blueprintv1alpha1.ConditionalTerraformComponent{
			"vpc": {
				Strategy:           "replace",
				Ordinal:            intPtr(0),
				TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"key1": "value1"}},
			},
		}

		// When updating with merge strategy but higher ordinal
		new := &blueprintv1alpha1.ConditionalTerraformComponent{
			Ordinal:            intPtr(100),
			TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"key2": "value2"}},
		}
		err := processor.updateTerraformComponentEntry("vpc", new, "merge", entries, nil)

		// Then entry should be replaced despite lower strategy
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if entries["vpc"].Strategy != "merge" {
			t.Errorf("Expected strategy 'merge', got '%s'", entries["vpc"].Strategy)
		}
		if ordinalOf(entries["vpc"]) != 100 {
			t.Errorf("Expected ordinal 100, got %d", ordinalOf(entries["vpc"]))
		}
		if entries["vpc"].Inputs["key2"] != "value2" {
			t.Error("Expected new inputs to be set")
		}
	})

	t.Run("IgnoresWhenNewHasLowerOrdinal", func(t *testing.T) {
		// Given existing entry with merge strategy and ordinal 100
		entries := map[string]*blueprintv1alpha1.ConditionalTerraformComponent{
			"vpc": {
				Strategy:           "merge",
				Ordinal:            intPtr(100),
				TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"key1": "value1"}},
			},
		}

		// When updating with replace strategy but lower ordinal
		new := &blueprintv1alpha1.ConditionalTerraformComponent{
			Ordinal:            intPtr(0),
			TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"key2": "value2"}},
		}
		err := processor.updateTerraformComponentEntry("vpc", new, "replace", entries, nil)

		// Then entry should remain unchanged
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if entries["vpc"].Strategy != "merge" {
			t.Errorf("Expected strategy 'merge' to be preserved, got '%s'", entries["vpc"].Strategy)
		}
		if ordinalOf(entries["vpc"]) != 100 {
			t.Errorf("Expected ordinal 100 to be preserved, got %d", ordinalOf(entries["vpc"]))
		}
		if entries["vpc"].Inputs["key1"] != "value1" {
			t.Error("Expected original inputs to be preserved")
		}
	})

	t.Run("MergesAdditivelyWhenNewHasLowerOrdinal", func(t *testing.T) {
		// Given existing entry with merge strategy, ordinal 100, a dependency, and a scalar
		// field set
		parallelism := 10
		entries := map[string]*blueprintv1alpha1.ConditionalTerraformComponent{
			"vpc": {
				Strategy: "merge",
				Ordinal:  intPtr(100),
				TerraformComponent: blueprintv1alpha1.TerraformComponent{
					Path:        "vpc",
					DependsOn:   []string{"base"},
					Parallelism: &blueprintv1alpha1.IntExpression{Value: &parallelism},
				},
			},
		}

		// When a lower-ordinal facet contributes a merge with an additional dependency and no
		// conflicting parallelism
		new := &blueprintv1alpha1.ConditionalTerraformComponent{
			Ordinal: intPtr(50),
			TerraformComponent: blueprintv1alpha1.TerraformComponent{
				Path:      "vpc",
				DependsOn: []string{"extra"},
			},
		}
		err := processor.updateTerraformComponentEntry("vpc", new, "merge", entries, nil)

		// Then the higher ordinal is preserved, its scalar field is untouched, and the lower
		// ordinal's dependency is additively folded in rather than dropped
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if ordinalOf(entries["vpc"]) != 100 {
			t.Errorf("Expected higher ordinal 100 to be preserved, got %d", ordinalOf(entries["vpc"]))
		}
		if entries["vpc"].Parallelism == nil || *entries["vpc"].Parallelism.Value != 10 {
			t.Errorf("Expected higher-ordinal parallelism to be preserved, got %v", entries["vpc"].Parallelism)
		}
		if !slices.Contains(entries["vpc"].DependsOn, "base") || !slices.Contains(entries["vpc"].DependsOn, "extra") {
			t.Errorf("Expected both dependencies to accumulate, got %v", entries["vpc"].DependsOn)
		}
	})

	t.Run("IgnoresLowerOrdinalMergeWhenExistingIsRemoveTombstone", func(t *testing.T) {
		// Given a higher-ordinal facet already removed this entry
		entries := map[string]*blueprintv1alpha1.ConditionalTerraformComponent{
			"vpc": {
				Strategy:           "remove",
				Ordinal:            intPtr(100),
				TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc"},
			},
		}

		// When a lower-ordinal facet attempts to merge
		new := &blueprintv1alpha1.ConditionalTerraformComponent{
			Ordinal:            intPtr(50),
			TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", DependsOn: []string{"extra"}},
		}
		err := processor.updateTerraformComponentEntry("vpc", new, "merge", entries, nil)

		// Then the remove tombstone is not resurrected
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if entries["vpc"].Strategy != "remove" {
			t.Errorf("Expected remove tombstone to be preserved, got strategy %q", entries["vpc"].Strategy)
		}
	})

	t.Run("UsesStrategyPrecedenceWhenOrdinalsEqual", func(t *testing.T) {
		// Given existing entry with merge strategy and ordinal 50
		entries := map[string]*blueprintv1alpha1.ConditionalTerraformComponent{
			"vpc": {
				Strategy:           "merge",
				Ordinal:            intPtr(50),
				TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"key1": "value1"}},
			},
		}

		// When updating with replace strategy and same ordinal
		new := &blueprintv1alpha1.ConditionalTerraformComponent{
			Ordinal:            intPtr(50),
			TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"key2": "value2"}},
		}
		err := processor.updateTerraformComponentEntry("vpc", new, "replace", entries, nil)

		// Then entry should be replaced due to higher strategy precedence
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if entries["vpc"].Strategy != "replace" {
			t.Errorf("Expected strategy 'replace', got '%s'", entries["vpc"].Strategy)
		}
		if ordinalOf(entries["vpc"]) != 50 {
			t.Errorf("Expected ordinal 50, got %d", ordinalOf(entries["vpc"]))
		}
	})

	t.Run("PreservesOrdinalInMergedResult", func(t *testing.T) {
		// Given existing entry with merge strategy and ordinal 25
		entries := map[string]*blueprintv1alpha1.ConditionalTerraformComponent{
			"vpc": {
				Strategy:           "merge",
				Ordinal:            intPtr(25),
				TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"key1": "value1"}},
			},
		}

		// When updating with merge strategy and same ordinal
		new := &blueprintv1alpha1.ConditionalTerraformComponent{
			Ordinal:            intPtr(25),
			TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"key2": "value2"}},
		}
		err := processor.updateTerraformComponentEntry("vpc", new, "merge", entries, nil)

		// Then ordinal should be preserved in merged result
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if ordinalOf(entries["vpc"]) != 25 {
			t.Errorf("Expected ordinal 25, got %d", ordinalOf(entries["vpc"]))
		}
	})

	t.Run("ReturnsErrorForInvalidStrategy", func(t *testing.T) {
		// Given existing entry with merge strategy
		entries := map[string]*blueprintv1alpha1.ConditionalTerraformComponent{
			"vpc": {
				Strategy:           "merge",
				TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc"},
			},
		}

		// When updating with invalid strategy
		new := &blueprintv1alpha1.ConditionalTerraformComponent{
			TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc"},
		}
		err := processor.updateTerraformComponentEntry("vpc", new, "invalid", entries, nil)

		// Then should return error
		if err == nil {
			t.Error("Expected error for invalid strategy")
		}
		if err != nil && !strings.Contains(err.Error(), "invalid strategy") {
			t.Errorf("Expected error about invalid strategy, got: %v", err)
		}
		if werr := werror.Find(err); werr == nil || !strings.Contains(werr.Remediation, "merge, replace, or remove") {
			t.Errorf("Expected remediation containing %q, got %v", "merge, replace, or remove", werr)
		}
	})

	t.Run("ReturnsErrorForInvalidStrategyEvenWithHigherOrdinal", func(t *testing.T) {
		// Given existing entry with merge strategy and ordinal 0
		entries := map[string]*blueprintv1alpha1.ConditionalTerraformComponent{
			"vpc": {
				Strategy:           "merge",
				Ordinal:            intPtr(0),
				TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"key1": "value1"}},
			},
		}

		// When updating with invalid strategy but higher ordinal
		new := &blueprintv1alpha1.ConditionalTerraformComponent{
			Ordinal:            intPtr(100),
			TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"key2": "value2"}},
		}
		err := processor.updateTerraformComponentEntry("vpc", new, "typo", entries, nil)

		// Then should return error before checking ordinal
		if err == nil {
			t.Error("Expected error for invalid strategy, even with higher ordinal")
		}
		if err != nil && !strings.Contains(err.Error(), "invalid strategy") {
			t.Errorf("Expected error about invalid strategy, got: %v", err)
		}
		if entries["vpc"].Inputs["key1"] != "value1" {
			t.Error("Expected original entry to remain unchanged when invalid strategy is rejected")
		}
	})
}

func TestProcessor_updateKustomizationEntry(t *testing.T) {
	mocks := setupProcessorMocks(t)
	processor := NewBlueprintProcessor(mocks.Runtime)

	t.Run("ReplacesWhenNewStrategyHasHigherPrecedence", func(t *testing.T) {
		// Given existing entry with merge strategy
		entries := map[string]*blueprintv1alpha1.ConditionalKustomization{
			"app": {
				Strategy:      "merge",
				Kustomization: blueprintv1alpha1.Kustomization{Name: "app", Substitutions: map[string]string{"key1": "value1"}},
			},
		}

		// When updating with replace strategy
		new := &blueprintv1alpha1.ConditionalKustomization{
			Kustomization: blueprintv1alpha1.Kustomization{Name: "app", Substitutions: map[string]string{"key2": "value2"}},
		}
		err := processor.updateKustomizationEntry("app", new, "replace", entries, nil)

		// Then entry should be replaced
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if entries["app"].Strategy != "replace" {
			t.Errorf("Expected strategy 'replace', got '%s'", entries["app"].Strategy)
		}
		if entries["app"].Substitutions["key2"] != "value2" {
			t.Error("Expected new substitutions to be set")
		}
	})

	t.Run("PreMergesWhenBothHaveMergeStrategy", func(t *testing.T) {
		// Given existing entry with merge strategy
		entries := map[string]*blueprintv1alpha1.ConditionalKustomization{
			"app": {
				Strategy:      "merge",
				Kustomization: blueprintv1alpha1.Kustomization{Name: "app", Path: "existing-path"},
			},
		}

		// When updating with merge strategy
		new := &blueprintv1alpha1.ConditionalKustomization{
			Kustomization: blueprintv1alpha1.Kustomization{Name: "app", Path: "new-path"},
		}
		err := processor.updateKustomizationEntry("app", new, "merge", entries, nil)

		// Then entries should be pre-merged
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if entries["app"].Strategy != "merge" {
			t.Errorf("Expected strategy 'merge', got '%s'", entries["app"].Strategy)
		}
		if entries["app"].Path != "new-path" {
			t.Errorf("Expected path to be updated to 'new-path', got '%s'", entries["app"].Path)
		}
	})

	t.Run("IgnoresWhenNewStrategyHasLowerPrecedence", func(t *testing.T) {
		// Given existing entry with replace strategy
		entries := map[string]*blueprintv1alpha1.ConditionalKustomization{
			"app": {
				Strategy:      "replace",
				Kustomization: blueprintv1alpha1.Kustomization{Name: "app", Substitutions: map[string]string{"key1": "value1"}},
			},
		}

		// When updating with merge strategy
		new := &blueprintv1alpha1.ConditionalKustomization{
			Kustomization: blueprintv1alpha1.Kustomization{Name: "app", Substitutions: map[string]string{"key2": "value2"}},
		}
		err := processor.updateKustomizationEntry("app", new, "merge", entries, nil)

		// Then entry should remain unchanged
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if entries["app"].Strategy != "replace" {
			t.Errorf("Expected strategy 'replace' to be preserved, got '%s'", entries["app"].Strategy)
		}
		if entries["app"].Substitutions["key1"] != "value1" {
			t.Error("Expected original substitutions to be preserved")
		}
	})

	t.Run("AccumulatesRemovalsWhenBothHaveRemoveStrategy", func(t *testing.T) {
		// Given existing entry with remove strategy
		entries := map[string]*blueprintv1alpha1.ConditionalKustomization{
			"app": {
				Strategy:      "remove",
				Kustomization: blueprintv1alpha1.Kustomization{Name: "app", Substitutions: map[string]string{"key1": ""}, DependsOn: []string{"dep1"}, Components: []string{"comp1"}},
			},
		}

		// When updating with remove strategy
		new := &blueprintv1alpha1.ConditionalKustomization{
			Kustomization: blueprintv1alpha1.Kustomization{Name: "app", Substitutions: map[string]string{"key2": ""}, DependsOn: []string{"dep2"}, Components: []string{"comp2"}},
		}
		err := processor.updateKustomizationEntry("app", new, "remove", entries, nil)

		// Then removals should be accumulated
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if entries["app"].Strategy != "remove" {
			t.Errorf("Expected strategy 'remove', got '%s'", entries["app"].Strategy)
		}
		subs := entries["app"].Substitutions
		if subs == nil {
			t.Fatal("Expected substitutions map to exist")
		}
		if _, exists := subs["key1"]; !exists {
			t.Error("Expected key1 to be in removal list")
		}
		if _, exists := subs["key2"]; !exists {
			t.Error("Expected key2 to be in removal list")
		}
		deps := entries["app"].DependsOn
		if !contains(deps, "dep1") {
			t.Error("Expected dep1 to be in removal list")
		}
		if !contains(deps, "dep2") {
			t.Error("Expected dep2 to be in removal list")
		}
		comps := entries["app"].Components
		if !contains(comps, "comp1") {
			t.Error("Expected comp1 to be in removal list")
		}
		if !contains(comps, "comp2") {
			t.Error("Expected comp2 to be in removal list")
		}
	})

	t.Run("ReplacesWhenBothHaveReplaceStrategy", func(t *testing.T) {
		// Given existing entry with replace strategy
		entries := map[string]*blueprintv1alpha1.ConditionalKustomization{
			"app": {
				Strategy:      "replace",
				Kustomization: blueprintv1alpha1.Kustomization{Name: "app", Substitutions: map[string]string{"key1": "value1"}},
			},
		}

		// When updating with replace strategy
		new := &blueprintv1alpha1.ConditionalKustomization{
			Kustomization: blueprintv1alpha1.Kustomization{Name: "app", Substitutions: map[string]string{"key2": "value2"}},
		}
		err := processor.updateKustomizationEntry("app", new, "replace", entries, nil)

		// Then new entry should replace existing
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if entries["app"].Strategy != "replace" {
			t.Errorf("Expected strategy 'replace', got '%s'", entries["app"].Strategy)
		}
		if entries["app"].Substitutions["key2"] != "value2" {
			t.Error("Expected new substitutions to replace old ones")
		}
		if entries["app"].Substitutions["key1"] != "" {
			t.Error("Expected old substitutions to be replaced")
		}
	})

	t.Run("ReplacesWhenNewHasHigherOrdinal", func(t *testing.T) {
		// Given existing entry with replace strategy and ordinal 0
		entries := map[string]*blueprintv1alpha1.ConditionalKustomization{
			"app": {
				Strategy:      "replace",
				Ordinal:       intPtr(0),
				Kustomization: blueprintv1alpha1.Kustomization{Name: "app", Substitutions: map[string]string{"key1": "value1"}},
			},
		}

		// When updating with merge strategy but higher ordinal
		new := &blueprintv1alpha1.ConditionalKustomization{
			Ordinal:       intPtr(100),
			Kustomization: blueprintv1alpha1.Kustomization{Name: "app", Substitutions: map[string]string{"key2": "value2"}},
		}
		err := processor.updateKustomizationEntry("app", new, "merge", entries, nil)

		// Then entry should be replaced despite lower strategy
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if entries["app"].Strategy != "merge" {
			t.Errorf("Expected strategy 'merge', got '%s'", entries["app"].Strategy)
		}
		if ordinalOfK(entries["app"]) != 100 {
			t.Errorf("Expected ordinal 100, got %d", ordinalOfK(entries["app"]))
		}
		if entries["app"].Substitutions["key2"] != "value2" {
			t.Error("Expected new substitutions to be set")
		}
	})

	t.Run("IgnoresWhenNewHasLowerOrdinal", func(t *testing.T) {
		// Given existing entry with merge strategy and ordinal 100
		entries := map[string]*blueprintv1alpha1.ConditionalKustomization{
			"app": {
				Strategy:      "merge",
				Ordinal:       intPtr(100),
				Kustomization: blueprintv1alpha1.Kustomization{Name: "app", Substitutions: map[string]string{"key1": "value1"}},
			},
		}

		// When updating with replace strategy but lower ordinal
		new := &blueprintv1alpha1.ConditionalKustomization{
			Ordinal:       intPtr(0),
			Kustomization: blueprintv1alpha1.Kustomization{Name: "app", Substitutions: map[string]string{"key2": "value2"}},
		}
		err := processor.updateKustomizationEntry("app", new, "replace", entries, nil)

		// Then entry should remain unchanged
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if entries["app"].Strategy != "merge" {
			t.Errorf("Expected strategy 'merge' to be preserved, got '%s'", entries["app"].Strategy)
		}
		if ordinalOfK(entries["app"]) != 100 {
			t.Errorf("Expected ordinal 100 to be preserved, got %d", ordinalOfK(entries["app"]))
		}
		if entries["app"].Substitutions["key1"] != "value1" {
			t.Error("Expected original substitutions to be preserved")
		}
	})

	t.Run("MergesAdditivelyWhenNewHasLowerOrdinal", func(t *testing.T) {
		// Given existing entry with merge strategy, ordinal 100, an existing component, and
		// a scalar field set
		entries := map[string]*blueprintv1alpha1.ConditionalKustomization{
			"app": {
				Strategy: "merge",
				Ordinal:  intPtr(100),
				Kustomization: blueprintv1alpha1.Kustomization{
					Name:       "app",
					Path:       "existing-path",
					Components: []string{"base"},
				},
			},
		}

		// When a lower-ordinal facet contributes a merge with an additional component and no
		// conflicting path
		new := &blueprintv1alpha1.ConditionalKustomization{
			Ordinal: intPtr(50),
			Kustomization: blueprintv1alpha1.Kustomization{
				Name:       "app",
				Components: []string{"extra"},
			},
		}
		err := processor.updateKustomizationEntry("app", new, "merge", entries, nil)

		// Then the higher ordinal is preserved, its scalar field is untouched, and the lower
		// ordinal's component is additively folded in rather than dropped
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if ordinalOfK(entries["app"]) != 100 {
			t.Errorf("Expected higher ordinal 100 to be preserved, got %d", ordinalOfK(entries["app"]))
		}
		if entries["app"].Path != "existing-path" {
			t.Errorf("Expected higher-ordinal path to be preserved, got %q", entries["app"].Path)
		}
		if !slices.Contains(entries["app"].Components, "base") || !slices.Contains(entries["app"].Components, "extra") {
			t.Errorf("Expected both components to accumulate, got %v", entries["app"].Components)
		}
	})

	t.Run("IgnoresLowerOrdinalMergeWhenExistingIsRemoveTombstone", func(t *testing.T) {
		// Given a higher-ordinal facet already removed this entry
		entries := map[string]*blueprintv1alpha1.ConditionalKustomization{
			"app": {
				Strategy:      "remove",
				Ordinal:       intPtr(100),
				Kustomization: blueprintv1alpha1.Kustomization{Name: "app"},
			},
		}

		// When a lower-ordinal facet attempts to merge
		new := &blueprintv1alpha1.ConditionalKustomization{
			Ordinal:       intPtr(50),
			Kustomization: blueprintv1alpha1.Kustomization{Name: "app", Components: []string{"extra"}},
		}
		err := processor.updateKustomizationEntry("app", new, "merge", entries, nil)

		// Then the remove tombstone is not resurrected
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if entries["app"].Strategy != "remove" {
			t.Errorf("Expected remove tombstone to be preserved, got strategy %q", entries["app"].Strategy)
		}
	})

	t.Run("UsesStrategyPrecedenceWhenOrdinalsEqual", func(t *testing.T) {
		// Given existing entry with merge strategy and ordinal 50
		entries := map[string]*blueprintv1alpha1.ConditionalKustomization{
			"app": {
				Strategy:      "merge",
				Ordinal:       intPtr(50),
				Kustomization: blueprintv1alpha1.Kustomization{Name: "app", Substitutions: map[string]string{"key1": "value1"}},
			},
		}

		// When updating with replace strategy and same ordinal
		new := &blueprintv1alpha1.ConditionalKustomization{
			Ordinal:       intPtr(50),
			Kustomization: blueprintv1alpha1.Kustomization{Name: "app", Substitutions: map[string]string{"key2": "value2"}},
		}
		err := processor.updateKustomizationEntry("app", new, "replace", entries, nil)

		// Then entry should be replaced due to higher strategy precedence
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if entries["app"].Strategy != "replace" {
			t.Errorf("Expected strategy 'replace', got '%s'", entries["app"].Strategy)
		}
		if ordinalOfK(entries["app"]) != 50 {
			t.Errorf("Expected ordinal 50, got %d", ordinalOfK(entries["app"]))
		}
	})

	t.Run("PreservesOrdinalInMergedResult", func(t *testing.T) {
		// Given existing entry with merge strategy and ordinal 25
		entries := map[string]*blueprintv1alpha1.ConditionalKustomization{
			"app": {
				Strategy:      "merge",
				Ordinal:       intPtr(25),
				Kustomization: blueprintv1alpha1.Kustomization{Name: "app", Substitutions: map[string]string{"key1": "value1"}},
			},
		}

		// When updating with merge strategy and same ordinal
		new := &blueprintv1alpha1.ConditionalKustomization{
			Ordinal:       intPtr(25),
			Kustomization: blueprintv1alpha1.Kustomization{Name: "app", Substitutions: map[string]string{"key2": "value2"}},
		}
		err := processor.updateKustomizationEntry("app", new, "merge", entries, nil)

		// Then ordinal should be preserved in merged result
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if ordinalOfK(entries["app"]) != 25 {
			t.Errorf("Expected ordinal 25, got %d", ordinalOfK(entries["app"]))
		}
	})

	t.Run("ReturnsErrorForInvalidStrategy", func(t *testing.T) {
		// Given existing entry with merge strategy
		entries := map[string]*blueprintv1alpha1.ConditionalKustomization{
			"app": {
				Strategy:      "merge",
				Kustomization: blueprintv1alpha1.Kustomization{Name: "app"},
			},
		}

		// When updating with invalid strategy
		new := &blueprintv1alpha1.ConditionalKustomization{
			Kustomization: blueprintv1alpha1.Kustomization{Name: "app"},
		}
		err := processor.updateKustomizationEntry("app", new, "typo", entries, nil)

		// Then should return error
		if err == nil {
			t.Error("Expected error for invalid strategy")
		}
		if err != nil && !strings.Contains(err.Error(), "invalid strategy") {
			t.Errorf("Expected error about invalid strategy, got: %v", err)
		}
	})

	t.Run("ReturnsErrorForInvalidStrategyEvenWithHigherOrdinal", func(t *testing.T) {
		// Given existing entry with merge strategy and ordinal 0
		entries := map[string]*blueprintv1alpha1.ConditionalKustomization{
			"app": {
				Strategy:      "merge",
				Ordinal:       intPtr(0),
				Kustomization: blueprintv1alpha1.Kustomization{Name: "app", Substitutions: map[string]string{"key1": "value1"}},
			},
		}

		// When updating with invalid strategy but higher ordinal
		new := &blueprintv1alpha1.ConditionalKustomization{
			Ordinal:       intPtr(100),
			Kustomization: blueprintv1alpha1.Kustomization{Name: "app", Substitutions: map[string]string{"key2": "value2"}},
		}
		err := processor.updateKustomizationEntry("app", new, "typo", entries, nil)

		// Then should return error before checking ordinal
		if err == nil {
			t.Error("Expected error for invalid strategy, even with higher ordinal")
		}
		if err != nil && !strings.Contains(err.Error(), "invalid strategy") {
			t.Errorf("Expected error about invalid strategy, got: %v", err)
		}
		if entries["app"].Substitutions["key1"] != "value1" {
			t.Error("Expected original entry to remain unchanged when invalid strategy is rejected")
		}
	})
}

func TestProcessor_updateKustomizationEntry_AdditionalCoverage(t *testing.T) {
	t.Run("RemovesKustomizationWhenNewConditionIsFalse", func(t *testing.T) {
		// Given an existing kustomization without condition
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		entries := map[string]*blueprintv1alpha1.ConditionalKustomization{
			"app": {
				Strategy:      "merge",
				Kustomization: blueprintv1alpha1.Kustomization{Name: "app"},
			},
		}

		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"enabled": false}, nil
		}

		// When updating with a new kustomization that has false condition
		new := &blueprintv1alpha1.ConditionalKustomization{
			When:          "enabled == true",
			Kustomization: blueprintv1alpha1.Kustomization{Name: "app", Path: "new-path"},
		}
		err := processor.updateKustomizationEntry("app", new, "merge", entries, nil)

		// Then kustomization should be removed (new condition is false)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if _, exists := entries["app"]; exists {
			t.Error("Expected kustomization to be removed when new condition is false")
		}
	})

	t.Run("PreservesKustomizationWhenBothConditionsAreTrue", func(t *testing.T) {
		// Given an existing kustomization with condition
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		entries := map[string]*blueprintv1alpha1.ConditionalKustomization{
			"app": {
				When:          "enabled1 == true",
				Strategy:      "merge",
				Kustomization: blueprintv1alpha1.Kustomization{Name: "app", Path: "old-path"},
			},
		}

		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"enabled1": true,
				"enabled2": true,
			}, nil
		}

		// When updating with a new kustomization that also has true condition
		new := &blueprintv1alpha1.ConditionalKustomization{
			When:          "enabled2 == true",
			Kustomization: blueprintv1alpha1.Kustomization{Name: "app", Path: "new-path"},
		}
		err := processor.updateKustomizationEntry("app", new, "merge", entries, nil)

		// Then kustomization should be preserved and merged
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if _, exists := entries["app"]; !exists {
			t.Fatal("Expected kustomization to be preserved when both conditions are true")
		}
		if entries["app"].Path != "new-path" {
			t.Errorf("Expected path to be updated to 'new-path', got '%s'", entries["app"].Path)
		}
	})

	t.Run("RemovesKustomizationWhenBothConditionsAreFalse", func(t *testing.T) {
		// Given an existing kustomization with condition
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		entries := map[string]*blueprintv1alpha1.ConditionalKustomization{
			"app": {
				When:          "enabled1 == true",
				Strategy:      "merge",
				Kustomization: blueprintv1alpha1.Kustomization{Name: "app"},
			},
		}

		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"enabled1": false,
				"enabled2": false,
			}, nil
		}

		// When updating with a new kustomization that also has false condition
		new := &blueprintv1alpha1.ConditionalKustomization{
			When:          "enabled2 == true",
			Kustomization: blueprintv1alpha1.Kustomization{Name: "app", Path: "new-path"},
		}
		err := processor.updateKustomizationEntry("app", new, "merge", entries, nil)

		// Then kustomization should be removed (existing condition is false)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if _, exists := entries["app"]; exists {
			t.Error("Expected kustomization to be removed when existing condition is false")
		}
	})
}

func TestProcessor_updateFluxSystemEntry(t *testing.T) {
	mocks := setupProcessorMocks(t)
	processor := NewBlueprintProcessor(mocks.Runtime)

	t.Run("ReplacesWhenNewHasHigherOrdinal", func(t *testing.T) {
		// Given existing entry at ordinal 100
		entries := map[string]*blueprintv1alpha1.FluxSystem{
			"cni": {
				Name:    "cni",
				Ordinal: intPtr(100),
				Install: &blueprintv1alpha1.Kustomization{Components: []string{"a"}},
			},
		}

		// When a higher-ordinal merge contribution arrives
		new := &blueprintv1alpha1.FluxSystem{
			Name:    "cni",
			Ordinal: intPtr(200),
			Install: &blueprintv1alpha1.Kustomization{Components: []string{"b"}},
		}
		err := processor.updateFluxSystemEntry("cni", new, "merge", entries)

		// Then components accumulate and the higher ordinal is recorded
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if ordinalOfFS(entries["cni"]) != 200 {
			t.Errorf("Expected ordinal 200, got %d", ordinalOfFS(entries["cni"]))
		}
		if !slices.Contains(entries["cni"].Install.Components, "a") || !slices.Contains(entries["cni"].Install.Components, "b") {
			t.Errorf("Expected both components to accumulate, got %v", entries["cni"].Install.Components)
		}
	})

	t.Run("MergesAdditivelyWhenNewHasLowerOrdinal", func(t *testing.T) {
		// Given existing entry at ordinal 200 with an install component and a scalar field set
		existingTimeout := &blueprintv1alpha1.DurationString{Duration: 5 * time.Minute}
		entries := map[string]*blueprintv1alpha1.FluxSystem{
			"cni": {
				Name:    "cni",
				Ordinal: intPtr(200),
				Install: &blueprintv1alpha1.Kustomization{
					Components: []string{"cilium"},
					Timeout:    existingTimeout,
				},
			},
		}

		// When a lower-ordinal facet contributes a merge with a patch and no conflicting timeout
		new := &blueprintv1alpha1.FluxSystem{
			Name:    "cni",
			Ordinal: intPtr(100),
			Install: &blueprintv1alpha1.Kustomization{
				Patches: []blueprintv1alpha1.BlueprintPatch{{Patch: "lower-ordinal-patch"}},
			},
		}
		err := processor.updateFluxSystemEntry("cni", new, "merge", entries)

		// Then the higher ordinal is preserved, its scalar field is untouched, and the lower
		// ordinal's patch is additively folded in rather than dropped
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if ordinalOfFS(entries["cni"]) != 200 {
			t.Errorf("Expected higher ordinal 200 to be preserved, got %d", ordinalOfFS(entries["cni"]))
		}
		if entries["cni"].Install.Timeout != existingTimeout {
			t.Errorf("Expected higher-ordinal timeout to be preserved, got %v", entries["cni"].Install.Timeout)
		}
		if !slices.Contains(entries["cni"].Install.Components, "cilium") {
			t.Errorf("Expected higher-ordinal component to be preserved, got %v", entries["cni"].Install.Components)
		}
		if len(entries["cni"].Install.Patches) != 1 || entries["cni"].Install.Patches[0].Patch != "lower-ordinal-patch" {
			t.Errorf("Expected lower-ordinal patch to be folded in, got %v", entries["cni"].Install.Patches)
		}
	})

	t.Run("IgnoresReplaceWhenNewHasLowerOrdinal", func(t *testing.T) {
		// Given existing entry at ordinal 200
		entries := map[string]*blueprintv1alpha1.FluxSystem{
			"cni": {
				Name:    "cni",
				Ordinal: intPtr(200),
				Install: &blueprintv1alpha1.Kustomization{Components: []string{"cilium"}},
			},
		}

		// When a lower-ordinal facet attempts to replace
		new := &blueprintv1alpha1.FluxSystem{
			Name:    "cni",
			Ordinal: intPtr(100),
			Install: &blueprintv1alpha1.Kustomization{Components: []string{"other"}},
		}
		err := processor.updateFluxSystemEntry("cni", new, "replace", entries)

		// Then the higher-ordinal entry is untouched — a lower-precedence facet cannot forcibly
		// overwrite a higher-precedence one, unlike a "merge" contribution
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(entries["cni"].Install.Components) != 1 || entries["cni"].Install.Components[0] != "cilium" {
			t.Errorf("Expected entry to remain unchanged, got %v", entries["cni"].Install.Components)
		}
	})

	t.Run("IgnoresLowerOrdinalMergeWhenExistingIsRemoveTombstone", func(t *testing.T) {
		// Given a higher-ordinal facet already removed this system
		entries := map[string]*blueprintv1alpha1.FluxSystem{
			"cni": {
				Name:     "cni",
				Ordinal:  intPtr(200),
				Strategy: "remove",
			},
		}

		// When a lower-ordinal facet attempts to merge
		new := &blueprintv1alpha1.FluxSystem{
			Name:    "cni",
			Ordinal: intPtr(100),
			Install: &blueprintv1alpha1.Kustomization{Components: []string{"cilium"}},
		}
		err := processor.updateFluxSystemEntry("cni", new, "merge", entries)

		// Then the remove tombstone is not resurrected
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if entries["cni"].Strategy != "remove" {
			t.Errorf("Expected remove tombstone to be preserved, got strategy %q", entries["cni"].Strategy)
		}
	})

	t.Run("UsesStrategyPrecedenceWhenOrdinalsEqual", func(t *testing.T) {
		// Given existing entry with merge strategy and ordinal 50
		entries := map[string]*blueprintv1alpha1.FluxSystem{
			"cni": {
				Name:    "cni",
				Ordinal: intPtr(50),
				Install: &blueprintv1alpha1.Kustomization{Components: []string{"cilium"}},
			},
		}

		// When updating with replace strategy at the same ordinal
		new := &blueprintv1alpha1.FluxSystem{
			Name:    "cni",
			Ordinal: intPtr(50),
			Install: &blueprintv1alpha1.Kustomization{Components: []string{"other"}},
		}
		err := processor.updateFluxSystemEntry("cni", new, "replace", entries)

		// Then replace wins over merge at equal ordinal via strategy precedence
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if entries["cni"].Strategy != "replace" {
			t.Errorf("Expected strategy 'replace', got %q", entries["cni"].Strategy)
		}
	})

	t.Run("ReturnsErrorForInvalidStrategy", func(t *testing.T) {
		entries := map[string]*blueprintv1alpha1.FluxSystem{
			"cni": {Name: "cni", Ordinal: intPtr(100)},
		}
		new := &blueprintv1alpha1.FluxSystem{Name: "cni", Ordinal: intPtr(200)}
		err := processor.updateFluxSystemEntry("cni", new, "typo", entries)

		if err == nil || !strings.Contains(err.Error(), "invalid strategy") {
			t.Errorf("Expected invalid strategy error, got: %v", err)
		}
	})
}

func TestProcessor_applyCollectedComponents(t *testing.T) {
	mocks := setupProcessorMocks(t)
	processor := NewBlueprintProcessor(mocks.Runtime)

	t.Run("AppliesReplaceThenMergeThenRemove", func(t *testing.T) {
		// Given collected components with all strategies
		target := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "vpc", Inputs: map[string]any{"existing": "value", "toRemove": "value"}},
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "app", Substitutions: map[string]string{"existing": "value", "toRemove": "value"}},
			},
		}

		terraformByID := map[string]*blueprintv1alpha1.ConditionalTerraformComponent{
			"vpc": {
				Strategy:           "remove",
				TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"toRemove": nil}},
			},
		}

		kustomizationByName := map[string]*blueprintv1alpha1.ConditionalKustomization{
			"app": {
				Strategy:      "remove",
				Kustomization: blueprintv1alpha1.Kustomization{Name: "app", Substitutions: map[string]string{"toRemove": ""}},
			},
		}

		// When applying collected components
		err := processor.applyCollectedComponents(target, collectedComponents{terraformByID: terraformByID, kustomizationByName: kustomizationByName}, nil)

		// Then specified fields should be removed (removes are applied last)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if target.TerraformComponents[0].Inputs["toRemove"] != nil {
			t.Error("Expected 'toRemove' input to be removed")
		}
		if target.TerraformComponents[0].Inputs["existing"] != "value" {
			t.Error("Expected 'existing' input to be preserved")
		}
		if target.Kustomizations[0].Substitutions["toRemove"] != "" {
			t.Error("Expected 'toRemove' substitution to be removed")
		}
		if target.Kustomizations[0].Substitutions["existing"] != "value" {
			t.Error("Expected 'existing' substitution to be preserved")
		}
	})

	t.Run("AppliesReplaceStrategy", func(t *testing.T) {
		// Given collected components with replace strategy
		target := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "vpc", Inputs: map[string]any{"old": "value"}},
			},
		}

		terraformByID := map[string]*blueprintv1alpha1.ConditionalTerraformComponent{
			"vpc": {
				Strategy:           "replace",
				TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"new": "value"}},
			},
		}

		// When applying collected components
		err := processor.applyCollectedComponents(target, collectedComponents{terraformByID: terraformByID}, nil)

		// Then component should be replaced
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if target.TerraformComponents[0].Inputs["new"] != "value" {
			t.Error("Expected component to be replaced")
		}
		if target.TerraformComponents[0].Inputs["old"] != nil {
			t.Error("Expected old inputs to be replaced")
		}
	})

	t.Run("AppliesMergeStrategy", func(t *testing.T) {
		// Given collected components with merge strategy
		target := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "vpc", Inputs: map[string]any{"existing": "value"}},
			},
		}

		terraformByID := map[string]*blueprintv1alpha1.ConditionalTerraformComponent{
			"vpc": {
				Strategy:           "merge",
				TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"new": "value"}},
			},
		}

		// When applying collected components
		err := processor.applyCollectedComponents(target, collectedComponents{terraformByID: terraformByID}, nil)

		// Then component should be merged
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if target.TerraformComponents[0].Inputs["existing"] != "value" {
			t.Error("Expected existing inputs to be preserved")
		}
		if target.TerraformComponents[0].Inputs["new"] != "value" {
			t.Error("Expected new inputs to be added")
		}
	})

	t.Run("HandlesEmptyStrategyAsMerge", func(t *testing.T) {
		// Given collected components with empty strategy
		target := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "vpc", Inputs: map[string]any{"existing": "value"}},
			},
		}

		terraformByID := map[string]*blueprintv1alpha1.ConditionalTerraformComponent{
			"vpc": {
				Strategy:           "",
				TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"new": "value"}},
			},
		}

		// When applying collected components
		err := processor.applyCollectedComponents(target, collectedComponents{terraformByID: terraformByID}, nil)

		// Then should default to merge
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if target.TerraformComponents[0].Inputs["existing"] != "value" {
			t.Error("Expected existing inputs to be preserved (merge behavior)")
		}
	})

	t.Run("HandlesKustomizationStrategies", func(t *testing.T) {
		// Given collected kustomizations with different strategies
		target := &blueprintv1alpha1.Blueprint{
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "app", Substitutions: map[string]string{"existing": "value"}},
			},
		}

		kustomizationByName := map[string]*blueprintv1alpha1.ConditionalKustomization{
			"app": {
				Strategy:      "replace",
				Kustomization: blueprintv1alpha1.Kustomization{Name: "app", Substitutions: map[string]string{"new": "value"}},
			},
		}

		// When applying collected components
		err := processor.applyCollectedComponents(target, collectedComponents{kustomizationByName: kustomizationByName}, nil)

		// Then kustomization should be replaced
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if target.Kustomizations[0].Substitutions["new"] != "value" {
			t.Error("Expected kustomization to be replaced")
		}
		if target.Kustomizations[0].Substitutions["existing"] != "" {
			t.Error("Expected old substitutions to be replaced")
		}
	})

	t.Run("HandlesMultipleComponentsWithDifferentStrategies", func(t *testing.T) {
		// Given multiple components with different strategies
		target := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "vpc", Inputs: map[string]any{"existing": "value"}},
				{Path: "rds", Inputs: map[string]any{"existing": "value"}},
			},
		}

		terraformByID := map[string]*blueprintv1alpha1.ConditionalTerraformComponent{
			"vpc": {
				Strategy:           "remove",
				TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"existing": nil}},
			},
			"rds": {
				Strategy:           "merge",
				TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "rds", Inputs: map[string]any{"new": "value"}},
			},
		}

		// When applying collected components
		err := processor.applyCollectedComponents(target, collectedComponents{terraformByID: terraformByID}, nil)

		// Then strategies should be applied correctly
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if target.TerraformComponents[0].Inputs["existing"] != nil {
			t.Error("Expected 'existing' input to be removed from vpc")
		}
		if target.TerraformComponents[1].Inputs["existing"] != "value" {
			t.Error("Expected 'existing' input to be preserved in rds")
		}
		if target.TerraformComponents[1].Inputs["new"] != "value" {
			t.Error("Expected 'new' input to be added to rds")
		}
	})

	t.Run("AppliesStrategiesInCorrectOrder", func(t *testing.T) {
		// Given a component that will be merged, then replaced, then have fields removed
		target := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "vpc", Inputs: map[string]any{"existing": "value", "toRemove": "value"}},
			},
		}

		terraformByID := map[string]*blueprintv1alpha1.ConditionalTerraformComponent{
			"vpc": {
				Strategy:           "merge",
				TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"merged": "value"}},
			},
		}

		// First apply merge
		err := processor.applyCollectedComponents(target, collectedComponents{terraformByID: terraformByID}, nil)
		if err != nil {
			t.Fatalf("Expected no error on merge, got %v", err)
		}

		// Then apply replace
		terraformByID = map[string]*blueprintv1alpha1.ConditionalTerraformComponent{
			"vpc": {
				Strategy:           "replace",
				TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"replaced": "value"}},
			},
		}
		err = processor.applyCollectedComponents(target, collectedComponents{terraformByID: terraformByID}, nil)
		if err != nil {
			t.Fatalf("Expected no error on replace, got %v", err)
		}

		// Finally apply remove
		terraformByID = map[string]*blueprintv1alpha1.ConditionalTerraformComponent{
			"vpc": {
				Strategy:           "remove",
				TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"replaced": nil}},
			},
		}
		err = processor.applyCollectedComponents(target, collectedComponents{terraformByID: terraformByID}, nil)
		if err != nil {
			t.Fatalf("Expected no error on remove, got %v", err)
		}

		// Then verify final state: merge added "merged", replace replaced everything with "replaced", remove removed "replaced"
		if target.TerraformComponents[0].Inputs["replaced"] != nil {
			t.Error("Expected 'replaced' input to be removed (remove applied last)")
		}
		if target.TerraformComponents[0].Inputs["merged"] != nil {
			t.Error("Expected 'merged' input to be gone (replaced)")
		}
		if target.TerraformComponents[0].Inputs["existing"] != nil {
			t.Error("Expected 'existing' input to be gone (replaced)")
		}
	})
}

func TestProcessor_ConditionReevaluationDuringMerge(t *testing.T) {
	t.Run("RemovesComponentWhenExistingConditionBecomesFalse", func(t *testing.T) {
		// Given an existing component with a condition that becomes false
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		entries := map[string]*blueprintv1alpha1.ConditionalTerraformComponent{
			"vpc": {
				When:               "enabled == true",
				Strategy:           "merge",
				TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc"},
			},
		}

		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"enabled": false}, nil
		}

		// When updating with a new component
		new := &blueprintv1alpha1.ConditionalTerraformComponent{
			TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"new": "value"}},
		}
		err := processor.updateTerraformComponentEntry("vpc", new, "merge", entries, nil)

		// Then component should be removed (existing condition is false)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if _, exists := entries["vpc"]; exists {
			t.Error("Expected component to be removed when existing condition is false")
		}
	})

	t.Run("RemovesComponentWhenNewConditionIsFalse", func(t *testing.T) {
		// Given an existing component without condition
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		entries := map[string]*blueprintv1alpha1.ConditionalTerraformComponent{
			"vpc": {
				Strategy:           "merge",
				TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc"},
			},
		}

		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"enabled": false}, nil
		}

		// When updating with a new component that has false condition
		new := &blueprintv1alpha1.ConditionalTerraformComponent{
			When:               "enabled == true",
			TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"new": "value"}},
		}
		err := processor.updateTerraformComponentEntry("vpc", new, "merge", entries, nil)

		// Then component should be removed (new condition is false)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if _, exists := entries["vpc"]; exists {
			t.Error("Expected component to be removed when new condition is false")
		}
	})

	t.Run("RemovesComponentWhenBothConditionsAreFalse", func(t *testing.T) {
		// Given an existing component with condition
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		entries := map[string]*blueprintv1alpha1.ConditionalTerraformComponent{
			"vpc": {
				When:               "enabled1 == true",
				Strategy:           "merge",
				TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc"},
			},
		}

		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"enabled1": false,
				"enabled2": false,
			}, nil
		}

		// When updating with a new component that also has false condition
		new := &blueprintv1alpha1.ConditionalTerraformComponent{
			When:               "enabled2 == true",
			TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"new": "value"}},
		}
		err := processor.updateTerraformComponentEntry("vpc", new, "merge", entries, nil)

		// Then component should be removed (existing condition is false)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if _, exists := entries["vpc"]; exists {
			t.Error("Expected component to be removed when existing condition is false")
		}
	})

	t.Run("PreservesComponentWhenBothConditionsAreTrue", func(t *testing.T) {
		// Given an existing component with condition
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		entries := map[string]*blueprintv1alpha1.ConditionalTerraformComponent{
			"vpc": {
				When:               "enabled1 == true",
				Strategy:           "merge",
				TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"old": "value"}},
			},
		}

		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"enabled1": true,
				"enabled2": true,
			}, nil
		}

		// When updating with a new component that also has true condition
		new := &blueprintv1alpha1.ConditionalTerraformComponent{
			When:               "enabled2 == true",
			TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"new": "value"}},
		}
		err := processor.updateTerraformComponentEntry("vpc", new, "merge", entries, nil)

		// Then component should be preserved and merged
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if _, exists := entries["vpc"]; !exists {
			t.Fatal("Expected component to be preserved when both conditions are true")
		}
		if entries["vpc"].Inputs["old"] != "value" {
			t.Error("Expected old inputs to be preserved")
		}
		if entries["vpc"].Inputs["new"] != "value" {
			t.Error("Expected new inputs to be added")
		}
	})

	t.Run("KustomizationRemovedWhenConditionBecomesFalse", func(t *testing.T) {
		// Given an existing kustomization with condition
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		entries := map[string]*blueprintv1alpha1.ConditionalKustomization{
			"app": {
				When:          "enabled == true",
				Strategy:      "merge",
				Kustomization: blueprintv1alpha1.Kustomization{Name: "app"},
			},
		}

		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"enabled": false}, nil
		}

		// When updating with a new kustomization
		new := &blueprintv1alpha1.ConditionalKustomization{
			Kustomization: blueprintv1alpha1.Kustomization{Name: "app", Path: "new-path"},
		}
		err := processor.updateKustomizationEntry("app", new, "merge", entries, nil)

		// Then kustomization should be removed (existing condition is false)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if _, exists := entries["app"]; exists {
			t.Error("Expected kustomization to be removed when existing condition is false")
		}
	})
}

func TestProcessor_PreservingWhenFieldDuringMerge(t *testing.T) {
	t.Run("CombinesWhenConditionsWithAndForMerge", func(t *testing.T) {
		// Given existing and new components with different when conditions
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		entries := map[string]*blueprintv1alpha1.ConditionalTerraformComponent{
			"vpc": {
				When:               "enabled1 == true",
				Strategy:           "merge",
				TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"old": "value"}},
			},
		}

		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"enabled1": true,
				"enabled2": true,
			}, nil
		}

		// When updating with merge strategy
		new := &blueprintv1alpha1.ConditionalTerraformComponent{
			When:               "enabled2 == true",
			TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"new": "value"}},
		}
		err := processor.updateTerraformComponentEntry("vpc", new, "merge", entries, nil)

		// Then When field should be combined with &&
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		expectedWhen := "(enabled1 == true) && (enabled2 == true)"
		if entries["vpc"].When != expectedWhen {
			t.Errorf("Expected When to be '%s', got '%s'", expectedWhen, entries["vpc"].When)
		}
	})

	t.Run("PreservesExistingWhenWhenNewHasNoWhen", func(t *testing.T) {
		// Given existing component with when condition
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		entries := map[string]*blueprintv1alpha1.ConditionalTerraformComponent{
			"vpc": {
				When:               "enabled == true",
				Strategy:           "merge",
				TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc"},
			},
		}

		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"enabled": true}, nil
		}

		// When updating with component without when
		new := &blueprintv1alpha1.ConditionalTerraformComponent{
			TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"new": "value"}},
		}
		err := processor.updateTerraformComponentEntry("vpc", new, "merge", entries, nil)

		// Then existing When should be preserved
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if entries["vpc"].When != "enabled == true" {
			t.Errorf("Expected When to be 'enabled == true', got '%s'", entries["vpc"].When)
		}
	})

	t.Run("UsesNewWhenWhenExistingHasNoWhen", func(t *testing.T) {
		// Given existing component without when condition
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		entries := map[string]*blueprintv1alpha1.ConditionalTerraformComponent{
			"vpc": {
				Strategy:           "merge",
				TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc"},
			},
		}

		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"enabled": true}, nil
		}

		// When updating with component with when
		new := &blueprintv1alpha1.ConditionalTerraformComponent{
			When:               "enabled == true",
			TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"new": "value"}},
		}
		err := processor.updateTerraformComponentEntry("vpc", new, "merge", entries, nil)

		// Then new When should be used
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if entries["vpc"].When != "enabled == true" {
			t.Errorf("Expected When to be 'enabled == true', got '%s'", entries["vpc"].When)
		}
	})

	t.Run("CombinesWhenConditionsForRemoveStrategy", func(t *testing.T) {
		// Given existing component with when condition
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		entries := map[string]*blueprintv1alpha1.ConditionalTerraformComponent{
			"vpc": {
				When:               "enabled1 == true",
				Strategy:           "remove",
				TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"key1": nil}},
			},
		}

		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"enabled1": true,
				"enabled2": true,
			}, nil
		}

		// When updating with remove strategy
		new := &blueprintv1alpha1.ConditionalTerraformComponent{
			When:               "enabled2 == true",
			TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc", Inputs: map[string]any{"key2": nil}},
		}
		err := processor.updateTerraformComponentEntry("vpc", new, "remove", entries, nil)

		// Then When field should be combined with &&
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		expectedWhen := "(enabled1 == true) && (enabled2 == true)"
		if entries["vpc"].When != expectedWhen {
			t.Errorf("Expected When to be '%s', got '%s'", expectedWhen, entries["vpc"].When)
		}
	})

	t.Run("KustomizationCombinesWhenConditions", func(t *testing.T) {
		// Given existing kustomization with when condition
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		entries := map[string]*blueprintv1alpha1.ConditionalKustomization{
			"app": {
				When:          "enabled1 == true",
				Strategy:      "merge",
				Kustomization: blueprintv1alpha1.Kustomization{Name: "app"},
			},
		}

		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"enabled1": true,
				"enabled2": true,
			}, nil
		}

		// When updating with merge strategy
		new := &blueprintv1alpha1.ConditionalKustomization{
			When:          "enabled2 == true",
			Kustomization: blueprintv1alpha1.Kustomization{Name: "app", Path: "new-path"},
		}
		err := processor.updateKustomizationEntry("app", new, "merge", entries, nil)

		// Then When field should be combined with &&
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		expectedWhen := "(enabled1 == true) && (enabled2 == true)"
		if entries["app"].When != expectedWhen {
			t.Errorf("Expected When to be '%s', got '%s'", expectedWhen, entries["app"].When)
		}
	})
}

func TestProcessor_PostProcessingConditionEvaluation(t *testing.T) {
	t.Run("RemovesTerraformComponentWithFalseConditionAfterMerge", func(t *testing.T) {
		// Given collected components with a condition that will be false
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		target := &blueprintv1alpha1.Blueprint{}

		terraformByID := map[string]*blueprintv1alpha1.ConditionalTerraformComponent{
			"vpc": {
				When:               "enabled == true",
				Strategy:           "merge",
				TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc"},
			},
		}

		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"enabled": false}, nil
		}

		// When applying collected components
		err := processor.applyCollectedComponents(target, collectedComponents{terraformByID: terraformByID}, nil)

		// Then component should be removed (condition is false)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(target.TerraformComponents) != 0 {
			t.Errorf("Expected 0 terraform components, got %d", len(target.TerraformComponents))
		}
	})

	t.Run("RemovesKustomizationWithFalseConditionAfterMerge", func(t *testing.T) {
		// Given collected kustomizations with a condition that will be false
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		target := &blueprintv1alpha1.Blueprint{}

		kustomizationByName := map[string]*blueprintv1alpha1.ConditionalKustomization{
			"app": {
				When:          "enabled == true",
				Strategy:      "merge",
				Kustomization: blueprintv1alpha1.Kustomization{Name: "app"},
			},
		}

		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"enabled": false}, nil
		}

		// When applying collected components
		err := processor.applyCollectedComponents(target, collectedComponents{kustomizationByName: kustomizationByName}, nil)

		// Then kustomization should be removed (condition is false)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(target.Kustomizations) != 0 {
			t.Errorf("Expected 0 kustomizations, got %d", len(target.Kustomizations))
		}
	})

	t.Run("PreservesComponentWithTrueConditionAfterMerge", func(t *testing.T) {
		// Given collected components with a condition that will be true
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		target := &blueprintv1alpha1.Blueprint{}

		terraformByID := map[string]*blueprintv1alpha1.ConditionalTerraformComponent{
			"vpc": {
				When:               "enabled == true",
				Strategy:           "merge",
				TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc"},
			},
		}

		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"enabled": true}, nil
		}

		// When applying collected components
		err := processor.applyCollectedComponents(target, collectedComponents{terraformByID: terraformByID}, nil)

		// Then component should be preserved (condition is true)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(target.TerraformComponents) != 1 {
			t.Errorf("Expected 1 terraform component, got %d", len(target.TerraformComponents))
		}
		if target.TerraformComponents[0].Path != "vpc" {
			t.Errorf("Expected path='vpc', got '%s'", target.TerraformComponents[0].Path)
		}
	})

	t.Run("RemovesComponentWithCombinedFalseCondition", func(t *testing.T) {
		// Given collected components with combined condition that will be false
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		target := &blueprintv1alpha1.Blueprint{}

		terraformByID := map[string]*blueprintv1alpha1.ConditionalTerraformComponent{
			"vpc": {
				When:               "(enabled1 == true) && (enabled2 == true)",
				Strategy:           "merge",
				TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc"},
			},
		}

		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{
				"enabled1": true,
				"enabled2": false,
			}, nil
		}

		// When applying collected components
		err := processor.applyCollectedComponents(target, collectedComponents{terraformByID: terraformByID}, nil)

		// Then component should be removed (combined condition is false)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(target.TerraformComponents) != 0 {
			t.Errorf("Expected 0 terraform components, got %d", len(target.TerraformComponents))
		}
	})

	t.Run("ReturnsErrorForInvalidConditionDuringPostProcessing", func(t *testing.T) {
		// Given collected components with invalid condition
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		target := &blueprintv1alpha1.Blueprint{}

		terraformByID := map[string]*blueprintv1alpha1.ConditionalTerraformComponent{
			"vpc": {
				When:               "invalid syntax {{{}",
				Strategy:           "merge",
				TerraformComponent: blueprintv1alpha1.TerraformComponent{Path: "vpc"},
			},
		}

		// When applying collected components
		err := processor.applyCollectedComponents(target, collectedComponents{terraformByID: terraformByID}, nil)

		// Then should return error
		if err == nil {
			t.Error("Expected error for invalid condition during post-processing")
		}
		if err != nil && !strings.Contains(err.Error(), "re-evaluating") {
			t.Errorf("Expected error about re-evaluating, got: %v", err)
		}
	})
}

// =============================================================================
// Test Helpers
// =============================================================================

func TestStrategyPrecedence(t *testing.T) {
	t.Run("ReturnsCorrectPrecedenceForRemove", func(t *testing.T) {
		prec := strategyPrecedence["remove"]
		if prec != 3 {
			t.Errorf("Expected precedence 3 for remove, got %d", prec)
		}
	})

	t.Run("ReturnsCorrectPrecedenceForReplace", func(t *testing.T) {
		prec := strategyPrecedence["replace"]
		if prec != 2 {
			t.Errorf("Expected precedence 2 for replace, got %d", prec)
		}
	})

	t.Run("ReturnsCorrectPrecedenceForMerge", func(t *testing.T) {
		prec := strategyPrecedence["merge"]
		if prec != 1 {
			t.Errorf("Expected precedence 1 for merge, got %d", prec)
		}
	})

	t.Run("ReturnsZeroForUnknownStrategy", func(t *testing.T) {
		prec := strategyPrecedence["unknown"]
		if prec != 0 {
			t.Errorf("Expected precedence 0 for unknown, got %d", prec)
		}
	})

	t.Run("ReturnsZeroForEmptyStrategy", func(t *testing.T) {
		prec := strategyPrecedence[""]
		if prec != 0 {
			t.Errorf("Expected precedence 0 for empty, got %d", prec)
		}
	})
}

func intPtr(i int) *int { return &i }

func ordinalOf(c *blueprintv1alpha1.ConditionalTerraformComponent) int {
	if c == nil || c.Ordinal == nil {
		return 0
	}
	return *c.Ordinal
}

func ordinalOfK(c *blueprintv1alpha1.ConditionalKustomization) int {
	if c == nil || c.Ordinal == nil {
		return 0
	}
	return *c.Ordinal
}

func ordinalOfFS(s *blueprintv1alpha1.FluxSystem) int {
	if s == nil || s.Ordinal == nil {
		return 0
	}
	return *s.Ordinal
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
