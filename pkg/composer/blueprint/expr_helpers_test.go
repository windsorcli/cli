package blueprint

import (
	"testing"

	"github.com/windsorcli/cli/pkg/runtime/evaluator"
)

// =============================================================================
// Test Helpers
// =============================================================================

func TestEvaluateWithOrigins(t *testing.T) {
	t.Run("UsesSubKeyOriginsForDeepMergedInputMaps", func(t *testing.T) {
		facetPathByKey := make(map[string]string)
		mockEvaluator := evaluator.NewMockExpressionEvaluator()
		mockEvaluator.EvaluateMapFunc = func(values map[string]any, facetPath string, scope map[string]any, evaluateDeferred bool) (map[string]any, error) {
			for k := range values {
				facetPathByKey[k] = facetPath
			}
			return values, nil
		}

		value := map[string]any{
			"from_base":    "${file(\"./base.yaml\")}",
			"from_overlay": "${file(\"./overlay.yaml\")}",
		}
		origins := map[string]string{
			"config.from_base":    "facets/base/facet.yaml",
			"config.from_overlay": "facets/overlay/facet.yaml",
		}

		evaluated, err := EvaluateWithOrigins(mockEvaluator, "config", value, origins, nil, false)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		result, ok := evaluated.(map[string]any)
		if !ok {
			t.Fatalf("Expected evaluated value to be map[string]any, got %T", evaluated)
		}
		if got := result["from_base"]; got != "${file(\"./base.yaml\")}" {
			t.Fatalf("Expected from_base to be preserved, got %v", got)
		}
		if got := result["from_overlay"]; got != "${file(\"./overlay.yaml\")}" {
			t.Fatalf("Expected from_overlay to be preserved, got %v", got)
		}
		if got := facetPathByKey["from_base"]; got != "facets/base/facet.yaml" {
			t.Fatalf("Expected from_base to evaluate with base facet path, got %q", got)
		}
		if got := facetPathByKey["from_overlay"]; got != "facets/overlay/facet.yaml" {
			t.Fatalf("Expected from_overlay to evaluate with overlay facet path, got %q", got)
		}
	})
}

func TestExpressionIsDerivedFromBlock(t *testing.T) {
	t.Run("ReturnsTrueWhenBlockRefIsFunctionArgument", func(t *testing.T) {
		got := expressionIsDerivedFromBlock("${string(talos_common.common_patch)}", "talos_common")
		if !got {
			t.Error("Expected function argument reference to be treated as derived")
		}
	})

	t.Run("ReturnsTrueWhenNestedFunctionUsesBlockRef", func(t *testing.T) {
		expr := "${len(string(talos_common.common_patch)) > 0}"
		got := expressionIsDerivedFromBlock(expr, "talos_common")
		if !got {
			t.Error("Expected nested function argument reference to be treated as derived")
		}
	})

	t.Run("ReturnsFalseForSimplePropertyRead", func(t *testing.T) {
		got := expressionIsDerivedFromBlock("${workstation.runtime ?? 'colima'}", "workstation")
		if got {
			t.Error("Expected simple property reads to not be treated as derived")
		}
	})

	t.Run("ReturnsFalseForNonFunctionComparison", func(t *testing.T) {
		got := expressionIsDerivedFromBlock("${talos_common.common_patch != nil}", "talos_common")
		if got {
			t.Error("Expected non-function comparisons to not be treated as derived")
		}
	})

	t.Run("ReturnsFalseWhenBlockNameDoesNotMatch", func(t *testing.T) {
		got := expressionIsDerivedFromBlock("${string(other_block.common_patch)}", "talos_common")
		if got {
			t.Error("Expected references to different blocks to not be treated as derived")
		}
	})

	t.Run("ReturnsFalseForMalformedExpression", func(t *testing.T) {
		got := expressionIsDerivedFromBlock("${string(talos_common.common_patch)", "talos_common")
		if got {
			t.Error("Expected malformed expressions to not be treated as derived")
		}
	})
}
