package blueprint_test

import (
	"reflect"
	"testing"

	"github.com/windsorcli/cli/pkg/composer/blueprint"
	"github.com/windsorcli/cli/pkg/runtime/evaluator"
)

// =============================================================================
// Test Public Methods
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

		evaluated, err := blueprint.EvaluateWithOrigins(mockEvaluator, "config", value, origins, nil, false)
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

	t.Run("EvaluatesAValueAgainstTheScope", func(t *testing.T) {
		// Given an evaluator and a scope that holds a region
		f := newFixture(t)
		scope := map[string]any{"region": "us-east-1"}

		// When an input with no origin is evaluated
		got, err := blueprint.EvaluateWithOrigins(f.runtime.Evaluator, "terraform.c.inputs.region", "${region}", nil, scope, false)

		// Then it resolves against the scope
		if err != nil || got != "us-east-1" {
			t.Errorf("expected us-east-1, got %v (err=%v)", got, err)
		}
	})

	t.Run("EvaluatesEachSubKeyAgainstItsOrigin", func(t *testing.T) {
		// Given a map input whose sub-keys come from different facets
		f := newFixture(t)
		f.write(t, "contexts/_template/facets/a.yaml", "")
		f.write(t, "contexts/_template/facets/data/a.txt", "from-a")
		origins := map[string]string{"inputs.cfg.a": f.root + "/contexts/_template/facets/a.yaml"}
		value := map[string]any{"a": "${file('data/a.txt')}", "b": "plain"}

		// When the input is evaluated
		got, err := blueprint.EvaluateWithOrigins(f.runtime.Evaluator, "inputs.cfg", value, origins, nil, false)

		// Then the sub-key with an origin resolves a path relative to its facet
		if err != nil {
			t.Fatalf("EvaluateWithOrigins: %v", err)
		}
		if !reflect.DeepEqual(got, map[string]any{"a": "from-a", "b": "plain"}) {
			t.Errorf("unexpected value %v", got)
		}
	})
}
