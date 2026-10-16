package blueprint_test

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/composer/blueprint"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestExplain(t *testing.T) {
	t.Run("FailsWithoutATraceCollector", func(t *testing.T) {
		// Given a loaded blueprint without a trace collector
		f := newFixture(t)
		f.project(t)
		f.facet(t, "k", "kustomize:\n- name: k\n  path: k\n  substitutions:\n    platform: ${platform}\n")
		h := f.mustLoad(t)

		// When a path is explained
		_, err := h.Explain("kustomize.k.substitutions.platform")

		// Then an error is returned
		if err == nil {
			t.Error("expected an error")
		}
	})

	t.Run("TracesAValueToTheFacetThatSetIt", func(t *testing.T) {
		// Given a blueprint loaded with a trace collector
		f := newFixture(t)
		f.project(t)
		f.facet(t, "k", "kustomize:\n- name: k\n  path: k\n  substitutions:\n    platform: ${platform}\n")
		if err := f.config.LoadConfig(); err != nil {
			t.Fatalf("LoadConfig: %v", err)
		}
		h := blueprint.NewBlueprintHandler(f.runtime, f.artifact)
		h.SetTraceCollector(blueprint.NewTraceCollector())
		if err := h.LoadBlueprint(); err != nil {
			t.Fatalf("LoadBlueprint: %v", err)
		}

		// When the substitution is explained
		trace, err := h.Explain("kustomize.k.substitutions.platform")

		// Then the trace names the value, the facet, the expression, and the scope it read
		if err != nil {
			t.Fatalf("Explain: %v", err)
		}
		if trace.Value != "docker" || len(trace.Contributions) != 1 {
			t.Fatalf("unexpected trace %+v", trace)
		}
		c := trace.Contributions[0]
		if c.SourceName != "template" || c.FacetPath != filepath.Join("facets", "k.yaml") || c.Expression != "${platform}" || !c.Effective {
			t.Errorf("unexpected contribution %+v", c)
		}
		if len(c.ScopeRefs) != 1 || c.ScopeRefs[0].Name != "platform" {
			t.Errorf("expected one platform scope reference, got %+v", c.ScopeRefs)
		}
	})
}

// =============================================================================
// Test Public Functions
// =============================================================================

func TestParseExplainPath(t *testing.T) {
	cases := []struct {
		path string
		want blueprint.ExplainPath
	}{
		{"terraform.net.inputs.cidr", blueprint.ExplainPath{Kind: blueprint.ExplainPathKindTerraformInput, Segment: "net", Key: "cidr"}},
		{"kustomize.k.substitutions.a.b", blueprint.ExplainPath{Kind: blueprint.ExplainPathKindKustomizeSubstitution, Segment: "k", Key: "a.b"}},
		{"kustomize.k.components", blueprint.ExplainPath{Kind: blueprint.ExplainPathKindKustomizeComponents, Segment: "k"}},
		{"configMaps.values-common.domain", blueprint.ExplainPath{Kind: blueprint.ExplainPathKindConfigMap, Segment: "values-common", Key: "domain"}},
		{"substitutions.domain", blueprint.ExplainPath{Kind: blueprint.ExplainPathKindSubstitution, Key: "domain"}},
	}
	for _, c := range cases {
		t.Run(c.path, func(t *testing.T) {
			got, err := blueprint.ParseExplainPath(c.path)
			if err != nil || got != c.want {
				t.Errorf("expected %+v, got %+v (err=%v)", c.want, got, err)
			}
			if got.String() != c.path {
				t.Errorf("expected String() to round-trip to %q, got %q", c.path, got.String())
			}
		})
	}

	for _, path := range []string{"", "terraform", "terraform.net.outputs.x", "kustomize.k.paths.x", "configMaps.only", "unknown.a.b"} {
		t.Run("Rejects"+path, func(t *testing.T) {
			if _, err := blueprint.ParseExplainPath(path); err == nil {
				t.Errorf("expected an error for %q", path)
			}
		})
	}
}

func TestCrdLayers(t *testing.T) {
	t.Run("ListsTheProjectLayerThenInstalledSourcesByName", func(t *testing.T) {
		// Given project CRDs and three sources, one of which does not install
		installed := blueprintv1alpha1.BoolExpression{Value: ptr(true)}
		notInstalled := blueprintv1alpha1.BoolExpression{Value: ptr(false)}
		bp := &blueprintv1alpha1.Blueprint{
			Crds: []string{"project/a"},
			Sources: []blueprintv1alpha1.Source{
				{Name: "zeta", Install: &installed, Crds: []string{"zeta/a"}},
				{Name: "alpha", Install: &installed, Crds: []string{"alpha/a"}},
				{Name: "off", Install: &notInstalled, Crds: []string{"off/a"}},
				{Name: "none", Install: &installed},
			},
		}

		// When the CRD layers are derived
		got := blueprint.CrdLayers(bp)

		// Then the project layer comes first, then installed sources with CRDs in name order
		want := []blueprint.CrdInstallLayer{
			{Source: "", Refs: []string{"project/a"}},
			{Source: "alpha", Refs: []string{"alpha/a"}},
			{Source: "zeta", Refs: []string{"zeta/a"}},
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("expected %+v, got %+v", want, got)
		}
	})
}

func TestValidateComposedBlueprint(t *testing.T) {
	t.Run("AcceptsABackendThatNamesAComponent", func(t *testing.T) {
		// Given a blueprint whose backend names a declared component
		bp := &blueprintv1alpha1.Blueprint{Backend: "state", TerraformComponents: []blueprintv1alpha1.TerraformComponent{{Name: "state", Path: "state"}}}

		// When it is validated
		err := blueprint.ValidateComposedBlueprint(bp)

		// Then it is accepted
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("AcceptsANilBlueprintAndAnEmptyBackend", func(t *testing.T) {
		// Given a nil blueprint and a blueprint with no backend
		// When both are validated
		nilErr := blueprint.ValidateComposedBlueprint(nil)
		emptyErr := blueprint.ValidateComposedBlueprint(&blueprintv1alpha1.Blueprint{})

		// Then both are accepted
		if nilErr != nil || emptyErr != nil {
			t.Errorf("expected no errors, got %v and %v", nilErr, emptyErr)
		}
	})

	t.Run("RejectsABackendThatNamesNoComponent", func(t *testing.T) {
		// Given a blueprint whose backend names no declared component
		bp := &blueprintv1alpha1.Blueprint{Backend: "missing"}

		// When it is validated
		err := blueprint.ValidateComposedBlueprint(bp)

		// Then it is rejected as invalid
		if !errors.Is(err, blueprint.ErrBlueprintInvalid) {
			t.Errorf("expected ErrBlueprintInvalid, got %v", err)
		}
	})
}

func TestRenderForDisplay(t *testing.T) {
	t.Run("ReplacesDeferredValuesAndDropsMessages", func(t *testing.T) {
		// Given a blueprint with a deferred input, a deferred substitution, and a message
		bp := &blueprintv1alpha1.Blueprint{
			Messages:            []blueprintv1alpha1.Message{{Text: "note"}},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{{Name: "c", Path: "c", Inputs: map[string]any{"vpc": "${x}", "plain": "y"}}},
			Kustomizations:      []blueprintv1alpha1.Kustomization{{Name: "k", Substitutions: map[string]string{"vpc": "${x}"}}},
		}
		deferred := map[string]bool{"terraform.c.inputs.vpc": true, "kustomize.k.substitutions.vpc": true}

		// When it is rendered for display
		got := blueprint.RenderForDisplay(bp, false, deferred).(*blueprintv1alpha1.Blueprint)

		// Then deferred values show a placeholder, messages are cleared, and the input is unchanged
		if got.TerraformComponents[0].Inputs["vpc"] != "<deferred>" || got.TerraformComponents[0].Inputs["plain"] != "y" {
			t.Errorf("unexpected inputs %v", got.TerraformComponents[0].Inputs)
		}
		if got.Kustomizations[0].Substitutions["vpc"] != "<deferred>" || got.Messages != nil {
			t.Errorf("unexpected display blueprint %+v", got)
		}
		if bp.TerraformComponents[0].Inputs["vpc"] != "${x}" || bp.Messages == nil {
			t.Error("expected the input blueprint unchanged")
		}
	})

	t.Run("KeepsDeferredExpressionsWhenRaw", func(t *testing.T) {
		// Given a blueprint with a deferred input
		bp := &blueprintv1alpha1.Blueprint{TerraformComponents: []blueprintv1alpha1.TerraformComponent{{Name: "c", Path: "c", Inputs: map[string]any{"vpc": "${x}"}}}}

		// When it is rendered raw
		got := blueprint.RenderForDisplay(bp, true, map[string]bool{"terraform.c.inputs.vpc": true}).(*blueprintv1alpha1.Blueprint)

		// Then the expression is kept
		if got.TerraformComponents[0].Inputs["vpc"] != "${x}" {
			t.Errorf("expected the expression, got %v", got.TerraformComponents[0].Inputs["vpc"])
		}
	})

	t.Run("PassesOtherTypesThrough", func(t *testing.T) {
		// Given a value that is not a blueprint or kustomization
		value := map[string]string{"a": "b"}

		// When it is rendered
		got := blueprint.RenderForDisplay(value, false, nil)

		// Then it is returned unchanged
		if !reflect.DeepEqual(got, value) {
			t.Errorf("expected %v, got %v", value, got)
		}
	})
}

func TestEvaluateWithOrigins(t *testing.T) {
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

// =============================================================================
// Test Helpers
// =============================================================================

// ptr returns a pointer to v.
func ptr[T any](v T) *T {
	return &v
}
