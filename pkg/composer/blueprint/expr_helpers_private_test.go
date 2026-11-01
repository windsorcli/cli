package blueprint

import (
	"testing"
)

// =============================================================================
// Test Private Methods
// =============================================================================

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
