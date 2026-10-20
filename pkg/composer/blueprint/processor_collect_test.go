package blueprint

import (
	"slices"
	"strings"
	"testing"
	"time"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/runtime/evaluator"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestProcessor_ProcessFacets_Substitutions(t *testing.T) {
	t.Run("CollectsStaticFacetSubstitutionsIntoTarget", func(t *testing.T) {
		// Given a facet with top-level substitutions
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)
		target := &blueprintv1alpha1.Blueprint{}
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "dns"},
				Substitutions: map[string]string{
					"private_dns": "10.0.0.1",
					"public_dns":  "8.8.8.8",
				},
			},
		}

		// When processing facets
		_, err := processor.ProcessFacets(target, facets)

		// Then target.Substitutions should contain the facet values
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if target.Substitutions == nil {
			t.Fatal("Expected target.Substitutions to be initialized")
		}
		if target.Substitutions["private_dns"] != "10.0.0.1" {
			t.Errorf("Expected private_dns='10.0.0.1', got '%s'", target.Substitutions["private_dns"])
		}
		if target.Substitutions["public_dns"] != "8.8.8.8" {
			t.Errorf("Expected public_dns='8.8.8.8', got '%s'", target.Substitutions["public_dns"])
		}
	})

	t.Run("EvaluatesFacetSubstitutionExpressionsWithConfigScope", func(t *testing.T) {
		// Given a facet with config block and substitution expression referencing it
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{}, nil
		}
		processor := NewBlueprintProcessor(mocks.Runtime)
		target := &blueprintv1alpha1.Blueprint{}
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "dns"},
				Config: []blueprintv1alpha1.ConfigBlock{
					{
						Name: "dns",
						Body: map[string]any{
							"value": map[string]any{
								"private": "10.0.0.1",
								"public":  "8.8.8.8",
							},
						},
					},
				},
				Substitutions: map[string]string{
					"private_dns": "${dns.private}",
					"public_dns":  "${dns.public}",
				},
			},
		}

		// When processing facets
		_, err := processor.ProcessFacets(target, facets)

		// Then substitution expressions should be evaluated against config scope
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if target.Substitutions["private_dns"] != "10.0.0.1" {
			t.Errorf("Expected private_dns='10.0.0.1', got '%s'", target.Substitutions["private_dns"])
		}
		if target.Substitutions["public_dns"] != "8.8.8.8" {
			t.Errorf("Expected public_dns='8.8.8.8', got '%s'", target.Substitutions["public_dns"])
		}
	})

	t.Run("LaterOrdinalFacetSubstitutionsOverrideEarlier", func(t *testing.T) {
		// Given two facets with overlapping substitution keys, higher ordinal wins
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)
		target := &blueprintv1alpha1.Blueprint{}
		lowOrdinal := 100
		highOrdinal := 200
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata:      blueprintv1alpha1.Metadata{Name: "base"},
				Ordinal:       &lowOrdinal,
				Substitutions: map[string]string{"private_dns": "base-value"},
			},
			{
				Metadata:      blueprintv1alpha1.Metadata{Name: "override"},
				Ordinal:       &highOrdinal,
				Substitutions: map[string]string{"private_dns": "override-value"},
			},
		}

		// When processing facets
		_, err := processor.ProcessFacets(target, facets)

		// Then the higher-ordinal facet's value should win
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if target.Substitutions["private_dns"] != "override-value" {
			t.Errorf("Expected private_dns='override-value', got '%s'", target.Substitutions["private_dns"])
		}
	})

	t.Run("ExcludesFacetSubstitutionsWhenFacetConditionFalse", func(t *testing.T) {
		// Given a facet with a false when condition
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"enabled": false}, nil
		}
		processor := NewBlueprintProcessor(mocks.Runtime)
		target := &blueprintv1alpha1.Blueprint{}
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata:      blueprintv1alpha1.Metadata{Name: "conditional"},
				When:          "enabled == true",
				Substitutions: map[string]string{"private_dns": "10.0.0.1"},
			},
		}

		// When processing facets
		_, err := processor.ProcessFacets(target, facets)

		// Then substitutions from excluded facet should not appear
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if target.Substitutions != nil && target.Substitutions["private_dns"] != "" {
			t.Errorf("Expected private_dns to be absent, got '%s'", target.Substitutions["private_dns"])
		}
	})

	t.Run("RecordsTraceForEachSubstitutionKey", func(t *testing.T) {
		// Given a facet with static substitutions and a trace collector attached
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)
		tc := NewTraceCollector()
		processor.SetTraceCollector(tc)
		target := &blueprintv1alpha1.Blueprint{}
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "dns"},
				Substitutions: map[string]string{
					"private_dns": "10.0.0.1",
					"public_dns":  "8.8.8.8",
				},
			},
		}

		// When processing facets
		_, err := processor.ProcessFacets(target, facets)
		tc.Finalize(target, nil, "")

		// Then the trace collector should have a contribution for each substitution key
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		tracePrivate, errPrivate := tc.GetTrace("substitutions.private_dns")
		if errPrivate != nil || tracePrivate == nil {
			t.Errorf("Expected trace for substitutions.private_dns, got err=%v trace=%v", errPrivate, tracePrivate)
		}
		tracePublic, errPublic := tc.GetTrace("substitutions.public_dns")
		if errPublic != nil || tracePublic == nil {
			t.Errorf("Expected trace for substitutions.public_dns, got err=%v trace=%v", errPublic, tracePublic)
		}
	})
}

func TestProcessor_ProcessFacets_KustomizationPath(t *testing.T) {
	t.Run("EvaluatesPathExpressionAgainstConfigScope", func(t *testing.T) {
		// Given a facet with config block and a kustomization path expression referencing it
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{}, nil
		}
		processor := NewBlueprintProcessor(mocks.Runtime)
		target := &blueprintv1alpha1.Blueprint{}
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "dns"},
				Config: []blueprintv1alpha1.ConfigBlock{
					{Name: "platform", Body: map[string]any{"value": "aws"}},
				},
				Kustomizations: []blueprintv1alpha1.ConditionalKustomization{
					{Kustomization: blueprintv1alpha1.Kustomization{Name: "dns", Path: "kustomize/${platform}/dns"}},
				},
			},
		}

		// When processing facets
		_, err := processor.ProcessFacets(target, facets)

		// Then the path expression resolves against config scope
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if got := target.Kustomizations[0].Path; got != "kustomize/aws/dns" {
			t.Errorf("Expected path 'kustomize/aws/dns', got %q", got)
		}
	})

	t.Run("RefusesPathThatReferencesAnUnappliedTerraformOutput", func(t *testing.T) {
		// Given a kustomization path expression that defers to a not-yet-applied
		// terraform output; a path names a directory in the repo, so unlike a
		// substitution it can never legitimately wait on infrastructure state
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{}, nil
		}
		realEval := evaluator.NewExpressionEvaluator(mocks.ConfigHandler, mocks.Runtime.ProjectRoot, mocks.Runtime.ConfigRoot)
		realEval.Register("terraform_output", func(params []any, deferred bool) (any, error) {
			if !deferred {
				return nil, &evaluator.DeferredError{
					Expression: "terraform_output(\"cluster\", \"path\")",
					Message:    "deferred",
				}
			}
			return "resolved", nil
		}, new(func(string, string) any))
		mocks.Evaluator.EvaluateFunc = realEval.Evaluate
		mocks.Evaluator.EvaluateMapFunc = realEval.EvaluateMap

		processor := NewBlueprintProcessor(mocks.Runtime)
		target := &blueprintv1alpha1.Blueprint{}
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "dns"},
				Kustomizations: []blueprintv1alpha1.ConditionalKustomization{
					{Kustomization: blueprintv1alpha1.Kustomization{Name: "dns", Path: "${terraform_output(\"cluster\", \"path\")}"}},
				},
			},
		}

		// When processing facets
		_, err := processor.ProcessFacets(target, facets)

		// Then composition refuses, naming the offending kustomization and expression
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "dns") || !strings.Contains(err.Error(), `terraform_output("cluster", "path")`) {
			t.Errorf("Expected an error naming the kustomization and the deferred expression, got: %v", err)
		}
	})

	t.Run("RefusesPathThatReferencesAnUnresolvedSecret", func(t *testing.T) {
		// Given a kustomization path expression that defers via secret(), not
		// terraform_output() — the error must not assume a single deferral source
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{}, nil
		}
		realEval := evaluator.NewExpressionEvaluator(mocks.ConfigHandler, mocks.Runtime.ProjectRoot, mocks.Runtime.ConfigRoot)
		realEval.Register("secret", func(params []any, deferred bool) (any, error) {
			if !deferred {
				return nil, &evaluator.DeferredError{
					Expression: `secret("vault", "item", "field")`,
					Message:    "deferred",
				}
			}
			return "resolved", nil
		}, new(func(string, string, string) any))
		mocks.Evaluator.EvaluateFunc = realEval.Evaluate
		mocks.Evaluator.EvaluateMapFunc = realEval.EvaluateMap

		processor := NewBlueprintProcessor(mocks.Runtime)
		target := &blueprintv1alpha1.Blueprint{}
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "dns"},
				Kustomizations: []blueprintv1alpha1.ConditionalKustomization{
					{Kustomization: blueprintv1alpha1.Kustomization{Name: "dns", Path: `${secret("vault", "item", "field")}`}},
				},
			},
		}

		// When processing facets
		_, err := processor.ProcessFacets(target, facets)

		// Then composition refuses, naming the actual expression rather than
		// assuming it was a terraform_output()
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), `secret("vault", "item", "field")`) {
			t.Errorf("Expected an error naming the deferred secret() expression, got: %v", err)
		}
		if strings.Contains(err.Error(), "terraform") {
			t.Errorf("Expected no mention of terraform for a secret()-deferred path, got: %v", err)
		}
	})
}

func TestProcessor_ProcessFacets_Backend(t *testing.T) {
	t.Run("FacetBackendCopiedToTarget", func(t *testing.T) {
		// Given a single facet naming a backend terminus
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)
		target := &blueprintv1alpha1.Blueprint{}
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "cluster-facet"},
				Backend:  "cluster",
			},
		}

		// When processing facets
		_, err := processor.ProcessFacets(target, facets)

		// Then target.Backend should hold the facet value
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if target.Backend != "cluster" {
			t.Errorf("Expected target.Backend=\"cluster\", got %q", target.Backend)
		}
	})

	t.Run("LaterOrdinalFacetBackendOverridesEarlier", func(t *testing.T) {
		// Given two facets that both declare Backend, the higher ordinal wins
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)
		target := &blueprintv1alpha1.Blueprint{}
		lowOrdinal := 100
		highOrdinal := 200
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "base"},
				Ordinal:  &lowOrdinal,
				Backend:  "base-backend",
			},
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "override"},
				Ordinal:  &highOrdinal,
				Backend:  "cluster",
			},
		}

		// When processing facets
		_, err := processor.ProcessFacets(target, facets)

		// Then the higher-ordinal facet's value wins
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if target.Backend != "cluster" {
			t.Errorf("Expected target.Backend=\"cluster\", got %q", target.Backend)
		}
	})

	t.Run("FacetWithEmptyBackendDoesNotClearTarget", func(t *testing.T) {
		// Given a pre-set target.Backend and a facet that does not declare Backend
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)
		target := &blueprintv1alpha1.Blueprint{Backend: "pre-existing"}
		facets := []blueprintv1alpha1.Facet{
			{Metadata: blueprintv1alpha1.Metadata{Name: "silent"}},
		}

		// When processing facets
		_, err := processor.ProcessFacets(target, facets)

		// Then target.Backend is preserved
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if target.Backend != "pre-existing" {
			t.Errorf("Expected target.Backend preserved, got %q", target.Backend)
		}
	})

	t.Run("ExcludesFacetBackendWhenWhenIsFalse", func(t *testing.T) {
		// Given a facet whose 'when' evaluates false, its Backend should not contribute
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"enabled": false}, nil
		}
		processor := NewBlueprintProcessor(mocks.Runtime)
		target := &blueprintv1alpha1.Blueprint{}
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "conditional"},
				When:     "enabled == true",
				Backend:  "cluster",
			},
		}

		// When processing facets
		_, err := processor.ProcessFacets(target, facets)

		// Then target.Backend stays empty
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if target.Backend != "" {
			t.Errorf("Expected target.Backend empty (facet excluded), got %q", target.Backend)
		}
	})

}

func TestProcessor_ProcessFacets_Crds(t *testing.T) {
	allRefs := func(bp *blueprintv1alpha1.Blueprint) []string {
		refs := slices.Clone(bp.Crds)
		for _, source := range bp.Sources {
			refs = append(refs, source.Crds...)
		}
		return refs
	}
	requireCrd := func(t *testing.T, bp *blueprintv1alpha1.Blueprint, ref string) {
		t.Helper()
		if !slices.Contains(allRefs(bp), ref) {
			t.Fatalf("Expected crd reference %q in crds: layer, got %v", ref, bp.Crds)
		}
	}

	t.Run("RecordsCrdReference", func(t *testing.T) {
		// Given a facet that declares a CRD reference
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "addon-private-ca"},
				Crds:     []string{"cert-manager-1.16.2"},
			},
		}

		// When processing facets
		target := &blueprintv1alpha1.Blueprint{}
		_, err := processor.ProcessFacets(target, facets)

		// Then the reference lands on the crds: layer, not as a kustomization
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		requireCrd(t, target, "cert-manager-1.16.2")
		for _, k := range target.Kustomizations {
			if k.Name == "cert-manager-1.16.2" {
				t.Errorf("Expected no synthesized kustomization for the CRD ref, got %v", target.Kustomizations)
			}
		}
	})

	t.Run("DedupesCrdAcrossFacets", func(t *testing.T) {
		// Given two facets declaring the same CRD reference
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		facets := []blueprintv1alpha1.Facet{
			{Metadata: blueprintv1alpha1.Metadata{Name: "cni"}, Crds: []string{"gateway-api-1.5.1"}},
			{Metadata: blueprintv1alpha1.Metadata{Name: "gateway"}, Crds: []string{"gateway-api-1.5.1"}},
		}

		// When processing facets
		target := &blueprintv1alpha1.Blueprint{}
		_, err := processor.ProcessFacets(target, facets)

		// Then the shared CRD reference appears exactly once
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		count := 0
		for _, ref := range allRefs(target) {
			if ref == "gateway-api-1.5.1" {
				count++
			}
		}
		if count != 1 {
			t.Errorf("Expected exactly 1 gateway-api-1.5.1 reference, got %d (%v)", count, target.Crds)
		}
	})

	t.Run("RecordsCrdWithoutFacetKustomizations", func(t *testing.T) {
		// Given a facet that declares a CRD reference but contributes no kustomizations
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		facets := []blueprintv1alpha1.Facet{
			{Metadata: blueprintv1alpha1.Metadata{Name: "crd-only"}, Crds: []string{"cert-manager-1.16.2"}},
		}

		// When processing facets
		target := &blueprintv1alpha1.Blueprint{}
		_, err := processor.ProcessFacets(target, facets)

		// Then the CRD layer is still recorded
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		requireCrd(t, target, "cert-manager-1.16.2")
	})

	t.Run("ExcludesCrdsFromInactiveFacet", func(t *testing.T) {
		// Given a facet whose 'when' is false that declares a CRD reference
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "inactive"},
				When:     "enabled == true",
				Crds:     []string{"cert-manager-1.16.2"},
			},
		}

		// When processing facets
		target := &blueprintv1alpha1.Blueprint{}
		_, err := processor.ProcessFacets(target, facets)

		// Then no CRD reference is recorded for the excluded facet
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if slices.Contains(allRefs(target), "cert-manager-1.16.2") {
			t.Errorf("Expected no CRD reference from excluded facet, got %v", target.Crds)
		}
	})

	t.Run("EmitsMultipleCrdsFromOneFacet", func(t *testing.T) {
		// Given a facet declaring more than one CRD reference
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "multi"},
				Crds:     []string{"cert-manager-1.16.2", "gateway-api-1.5.1"},
				Kustomizations: []blueprintv1alpha1.ConditionalKustomization{
					{Kustomization: blueprintv1alpha1.Kustomization{Name: "app", Path: "app"}},
				},
			},
		}

		// When processing facets
		target := &blueprintv1alpha1.Blueprint{}
		_, err := processor.ProcessFacets(target, facets)

		// Then both CRD references are recorded
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		requireCrd(t, target, "cert-manager-1.16.2")
		requireCrd(t, target, "gateway-api-1.5.1")
	})

	t.Run("SkipsEmptyCrdReference", func(t *testing.T) {
		// Given a facet with an empty-string CRD reference alongside a real one
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "sparse"},
				Crds:     []string{"", "cert-manager-1.16.2"},
			},
		}

		// When processing facets
		target := &blueprintv1alpha1.Blueprint{}
		_, err := processor.ProcessFacets(target, facets)

		// Then the empty reference is dropped from the crds: layer
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if slices.Contains(allRefs(target), "") {
			t.Errorf("Expected no empty crd reference, got %v", target.Crds)
		}
		requireCrd(t, target, "cert-manager-1.16.2")
	})

	t.Run("UnionsWithBaseBlueprintCrds", func(t *testing.T) {
		// Given a base blueprint that already carries a crds: reference on the default source
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		target := &blueprintv1alpha1.Blueprint{
			Crds: []string{"cert-manager-1.16.2"},
		}

		facets := []blueprintv1alpha1.Facet{
			{Metadata: blueprintv1alpha1.Metadata{Name: "gateway"}, Crds: []string{"gateway-api-1.5.1"}},
		}

		// When a facet from the same (default) source contributes another reference
		_, err := processor.ProcessFacets(target, facets)

		// Then both land in the default crds list, deduped and sorted
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !slices.Equal(target.Crds, []string{"cert-manager-1.16.2", "gateway-api-1.5.1"}) {
			t.Errorf("Expected unioned sorted crds, got %v", target.Crds)
		}
	})

	t.Run("SelectsCrdsByDriverExpression", func(t *testing.T) {
		// Given one facet that selects different CRDs per driver via inline expressions
		cases := []struct{ driver, want, absent string }{
			{"cilium", "gateway-api-1.5.1", "envoy-gateway-1.7.1"},
			{"envoy", "envoy-gateway-1.7.1", "gateway-api-1.5.1"},
		}
		for _, tc := range cases {
			t.Run(tc.driver, func(t *testing.T) {
				mocks := setupProcessorMocks(t)
				processor := NewBlueprintProcessor(mocks.Runtime)
				driver := tc.driver
				mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
					return map[string]any{"gateway": map[string]any{"driver": driver}}, nil
				}

				facets := []blueprintv1alpha1.Facet{
					{
						Metadata: blueprintv1alpha1.Metadata{Name: "gateway"},
						Crds: []string{
							"${gateway.driver == 'cilium' ? 'gateway-api-1.5.1' : ''}",
							"${gateway.driver == 'envoy' ? 'envoy-gateway-1.7.1' : ''}",
						},
						Kustomizations: []blueprintv1alpha1.ConditionalKustomization{
							{
								When:          "gateway.driver == '" + tc.driver + "'",
								Kustomization: blueprintv1alpha1.Kustomization{Name: "gateway", Path: "gateway"},
							},
						},
					},
				}

				// When processing facets for the selected driver
				target := &blueprintv1alpha1.Blueprint{}
				_, err := processor.ProcessFacets(target, facets)

				// Then only the active driver's CRD is recorded; the other never appears
				if err != nil {
					t.Fatalf("Expected no error, got %v", err)
				}
				requireCrd(t, target, tc.want)
				if slices.Contains(allRefs(target), tc.absent) {
					t.Errorf("Expected %q absent for driver %s, got %v", tc.absent, tc.driver, target.Crds)
				}
			})
		}
	})

	t.Run("RejectsPathTraversalCrdReference", func(t *testing.T) {
		// Given a facet whose CRD reference contains path-traversal components
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		for _, ref := range []string{"../manifests/secrets", "nested/dir", "..", "a\\b"} {
			facets := []blueprintv1alpha1.Facet{
				{Metadata: blueprintv1alpha1.Metadata{Name: "bad"}, Crds: []string{ref}},
			}

			// When processing facets
			target := &blueprintv1alpha1.Blueprint{}
			_, err := processor.ProcessFacets(target, facets)

			// Then composition fails rather than emitting a path outside crds/
			if err == nil {
				t.Errorf("Expected error for crd ref %q, got nil", ref)
			}
		}
	})

	t.Run("LeavesFacetsWithoutCrdsUnchanged", func(t *testing.T) {
		// Given a facet with a kustomization and no CRD references
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "plain"},
				Kustomizations: []blueprintv1alpha1.ConditionalKustomization{
					{Kustomization: blueprintv1alpha1.Kustomization{Name: "app", Path: "app", DependsOn: []string{"infra"}}},
				},
			},
		}

		// When processing facets
		target := &blueprintv1alpha1.Blueprint{}
		_, err := processor.ProcessFacets(target, facets)

		// Then no crds: layer appears and dependsOn is untouched
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(target.Crds) != 0 {
			t.Errorf("Expected no crds: layer, got %v", target.Crds)
		}
		if len(target.Kustomizations) != 1 {
			t.Fatalf("Expected exactly 1 kustomization, got %d", len(target.Kustomizations))
		}
		k := target.Kustomizations[0]
		if len(k.DependsOn) != 1 || k.DependsOn[0] != "infra" {
			t.Errorf("Expected dependsOn=[infra] unchanged, got %v", k.DependsOn)
		}
	})

	t.Run("BindsCrdsToNamedSource", func(t *testing.T) {
		// Given a facet from a named source that declares a CRD reference
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		facets := []blueprintv1alpha1.Facet{
			{Metadata: blueprintv1alpha1.Metadata{Name: "addon-private-ca"}, Crds: []string{"cert-manager-1.16.2"}},
		}

		// When processing facets for the "core" source
		target := &blueprintv1alpha1.Blueprint{}
		_, err := processor.ProcessFacets(target, facets, "core")

		// Then the ref rides with the "core" source (not the default crds list), so the provisioner
		// installs it from core in the background
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(target.Crds) != 0 {
			t.Errorf("Expected no default-source crds, got %v", target.Crds)
		}
		if len(target.Sources) != 1 || target.Sources[0].Name != "core" || !slices.Contains(target.Sources[0].Crds, "cert-manager-1.16.2") {
			t.Fatalf("Expected cert-manager carried by the core source, got %+v", target.Sources)
		}
	})

	t.Run("PutsDefaultSourceCrdsOnTheBlueprint", func(t *testing.T) {
		// Given a facet from the default/primary source that declares a CRD reference
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)

		facets := []blueprintv1alpha1.Facet{
			{Metadata: blueprintv1alpha1.Metadata{Name: "addon-private-ca"}, Crds: []string{"cert-manager-1.16.2"}},
		}

		// When processing facets for the primary source
		target := &blueprintv1alpha1.Blueprint{}
		_, err := processor.ProcessFacets(target, facets, "primary")

		// Then the ref lands on the blueprint's flat crds list (default/project source), not a named source
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !slices.Equal(target.Crds, []string{"cert-manager-1.16.2"}) {
			t.Errorf("Expected cert-manager on the default crds list, got %v", target.Crds)
		}
		if len(target.Sources) != 0 {
			t.Errorf("Expected no named source created, got %+v", target.Sources)
		}
	})

	t.Run("SplitsCrdsAcrossSourcesByProvenance", func(t *testing.T) {
		// Given facets from two different sources are processed into the same blueprint
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)
		target := &blueprintv1alpha1.Blueprint{}

		// When each source's facets are processed in turn
		if _, err := processor.ProcessFacets(target, []blueprintv1alpha1.Facet{
			{Metadata: blueprintv1alpha1.Metadata{Name: "core-ca"}, Crds: []string{"cert-manager-1.16.2"}},
		}, "core"); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if _, err := processor.ProcessFacets(target, []blueprintv1alpha1.Facet{
			{Metadata: blueprintv1alpha1.Metadata{Name: "acme"}, Crds: []string{"acme-crd-2.0.0"}},
		}, "acme-oci"); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then each source carries its own CRDs — a flux Kustomization reads from one source root
		if len(target.Sources) != 2 {
			t.Fatalf("Expected two sources carrying crds, got %+v", target.Sources)
		}
		if !slices.Contains(target.Sources[0].Crds, "cert-manager-1.16.2") || target.Sources[0].Name != "core" {
			t.Errorf("Expected core to carry cert-manager, got %+v", target.Sources[0])
		}
		if !slices.Contains(target.Sources[1].Crds, "acme-crd-2.0.0") || target.Sources[1].Name != "acme-oci" {
			t.Errorf("Expected acme-oci to carry acme-crd, got %+v", target.Sources[1])
		}
	})
}

func TestProcessor_ProcessFacets_Tiers(t *testing.T) {
	find := func(bp *blueprintv1alpha1.Blueprint, name string) (blueprintv1alpha1.Kustomization, bool) {
		for _, k := range bp.AllKustomizations() {
			if k.Name == name {
				return k, true
			}
		}
		return blueprintv1alpha1.Kustomization{}, false
	}
	process := func(t *testing.T, system blueprintv1alpha1.FluxSystem) *blueprintv1alpha1.Blueprint {
		t.Helper()
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)
		facets := []blueprintv1alpha1.Facet{{
			Metadata:    blueprintv1alpha1.Metadata{Name: system.Name},
			FluxSystems: []blueprintv1alpha1.FluxSystem{system},
		}}
		target := &blueprintv1alpha1.Blueprint{}
		if _, err := processor.ProcessFacets(target, facets); err != nil {
			t.Fatalf("ProcessFacets: %v", err)
		}
		return target
	}

	t.Run("CompilesInstallAndResourcesTiersAtTierPaths", func(t *testing.T) {
		target := process(t, blueprintv1alpha1.FluxSystem{
			Name:      "cert-manager",
			Path:      "pki/cert-manager",
			Install:   &blueprintv1alpha1.Kustomization{Components: []string{"helm-release"}},
			Resources: []blueprintv1alpha1.FluxVariant{{Kustomization: blueprintv1alpha1.Kustomization{Components: []string{"private-issuer/ca"}}}},
		})
		install, ok := find(target, "cert-manager-install")
		if !ok || install.Path != "pki/cert-manager/install" {
			t.Fatalf("expected cert-manager-install at pki/cert-manager/install, got %+v", install)
		}
		res, ok := find(target, "cert-manager-resources")
		if !ok || res.Path != "pki/cert-manager/resources" {
			t.Fatalf("expected cert-manager-resources at pki/cert-manager/resources, got %+v", res)
		}
		if !slices.Contains(install.Components, "helm-release") || !slices.Contains(res.Components, "private-issuer/ca") {
			t.Errorf("tier components wrong: install=%v resources=%v", install.Components, res.Components)
		}
		if !slices.Contains(res.DependsOn, "cert-manager-install") {
			t.Errorf("expected resources to depend on cert-manager-install, got %v", res.DependsOn)
		}
	})

	t.Run("PathDefaultsToName", func(t *testing.T) {
		target := process(t, blueprintv1alpha1.FluxSystem{
			Name:    "dns",
			Install: &blueprintv1alpha1.Kustomization{Components: []string{"coredns-controller"}},
		})
		install, ok := find(target, "dns-install")
		if !ok || install.Path != "dns/install" {
			t.Fatalf("expected dns-install at dns/install (path defaults to name), got %+v", install)
		}
	})

	t.Run("NamedResourcesVariantsShareThePathWithOwnSubstitutions", func(t *testing.T) {
		target := process(t, blueprintv1alpha1.FluxSystem{
			Name:    "gateway",
			Path:    "gateway",
			Install: &blueprintv1alpha1.Kustomization{Components: []string{"envoy"}},
			Resources: []blueprintv1alpha1.FluxVariant{
				{Kustomization: blueprintv1alpha1.Kustomization{Name: "internal", Components: []string{"internal"}, Substitute: map[string]string{"gateway_name": "internal"}}},
				{Kustomization: blueprintv1alpha1.Kustomization{Name: "external", Components: []string{"external"}, Substitute: map[string]string{"gateway_name": "external"}}},
			},
		})
		in, ok := find(target, "gateway-resources-internal")
		if !ok || in.Path != "gateway/resources" || in.Substitutions["gateway_name"] != "internal" {
			t.Fatalf("internal variant wrong: %+v", in)
		}
		ex, ok := find(target, "gateway-resources-external")
		if !ok || ex.Path != "gateway/resources" || ex.Substitutions["gateway_name"] != "external" {
			t.Fatalf("external variant wrong: %+v", ex)
		}
		for _, v := range []blueprintv1alpha1.Kustomization{in, ex} {
			if !slices.Contains(v.DependsOn, "gateway-install") {
				t.Errorf("expected %s to depend on gateway-install, got %v", v.Name, v.DependsOn)
			}
		}
	})

	t.Run("InstallSubstituteFlowsToInstallKustomization", func(t *testing.T) {
		target := process(t, blueprintv1alpha1.FluxSystem{
			Name:    "lb",
			Install: &blueprintv1alpha1.Kustomization{Components: []string{"aws-lb-controller"}, Substitute: map[string]string{"cluster_name": "prod"}},
		})
		install, ok := find(target, "lb-install")
		if !ok || install.Substitutions["cluster_name"] != "prod" {
			t.Fatalf("expected install substitute merged into substitutions, got %+v", install)
		}
	})

	t.Run("SystemDependsOnAttachesToInstallAndResourcesReachItTransitively", func(t *testing.T) {
		target := process(t, blueprintv1alpha1.FluxSystem{
			Name:      "gateway",
			Path:      "gateway",
			DependsOn: []string{"pki-install", "lb-install"},
			Install:   &blueprintv1alpha1.Kustomization{Components: []string{"envoy"}},
			Resources: []blueprintv1alpha1.FluxVariant{{Kustomization: blueprintv1alpha1.Kustomization{Components: []string{"internal"}}}},
		})
		install, _ := find(target, "gateway-install")
		if !slices.Contains(install.DependsOn, "pki-install") || !slices.Contains(install.DependsOn, "lb-install") {
			t.Errorf("expected system deps on install, got %v", install.DependsOn)
		}
		res, _ := find(target, "gateway-resources")
		if slices.Contains(res.DependsOn, "pki-install") {
			t.Errorf("expected resources to reach cross-layer deps transitively via install, got %v", res.DependsOn)
		}
		if !slices.Contains(res.DependsOn, "gateway-install") {
			t.Errorf("expected resources -> gateway-install, got %v", res.DependsOn)
		}
	})

	t.Run("NoInstallAttachesSystemDependsOnToEachVariant", func(t *testing.T) {
		target := process(t, blueprintv1alpha1.FluxSystem{
			Name:      "issuers",
			Path:      "pki/issuers",
			DependsOn: []string{"cert-manager-install"},
			Resources: []blueprintv1alpha1.FluxVariant{{Kustomization: blueprintv1alpha1.Kustomization{Components: []string{"public-issuer"}}}},
		})
		res, ok := find(target, "issuers-resources")
		if !ok {
			t.Fatalf("expected issuers-resources, got %+v", target.AllKustomizations())
		}
		if !slices.Contains(res.DependsOn, "cert-manager-install") {
			t.Errorf("expected system deps on the variant, got %v", res.DependsOn)
		}
		if slices.Contains(res.DependsOn, "issuers-install") {
			t.Errorf("did not expect an implicit install edge without an install tier: %v", res.DependsOn)
		}
	})

	t.Run("VariantDependsOnIsAppended", func(t *testing.T) {
		target := process(t, blueprintv1alpha1.FluxSystem{
			Name:      "gateway",
			Path:      "gateway",
			Install:   &blueprintv1alpha1.Kustomization{Components: []string{"envoy"}},
			Resources: []blueprintv1alpha1.FluxVariant{{Kustomization: blueprintv1alpha1.Kustomization{Components: []string{"internal"}, DependsOn: []string{"dns"}}}},
		})
		res, _ := find(target, "gateway-resources")
		if !slices.Contains(res.DependsOn, "gateway-install") || !slices.Contains(res.DependsOn, "dns") {
			t.Errorf("expected [gateway-install, dns], got %v", res.DependsOn)
		}
	})

	t.Run("InstallOnlyEmitsSingleTier", func(t *testing.T) {
		target := process(t, blueprintv1alpha1.FluxSystem{
			Name:    "metallb",
			Path:    "lb/metallb",
			Install: &blueprintv1alpha1.Kustomization{Components: []string{"helm-release"}},
		})
		install, ok := find(target, "metallb-install")
		if !ok || install.Path != "lb/metallb/install" {
			t.Fatalf("expected metallb-install at lb/metallb/install, got %+v", install)
		}
		if _, ok := find(target, "metallb-resources"); ok {
			t.Error("did not expect a resources tier")
		}
	})

	t.Run("VariantWhenFalseIsDropped", func(t *testing.T) {
		target := process(t, blueprintv1alpha1.FluxSystem{
			Name:    "gateway",
			Path:    "gateway",
			Install: &blueprintv1alpha1.Kustomization{Components: []string{"envoy"}},
			Resources: []blueprintv1alpha1.FluxVariant{
				{Kustomization: blueprintv1alpha1.Kustomization{Name: "internal", Components: []string{"internal"}}, When: "false"},
				{Kustomization: blueprintv1alpha1.Kustomization{Name: "external", Components: []string{"external"}}, When: "true"},
			},
		})
		if _, ok := find(target, "gateway-resources-internal"); ok {
			t.Error("expected the when:false variant to be dropped")
		}
		if _, ok := find(target, "gateway-resources-external"); !ok {
			t.Error("expected the when:true variant to be kept")
		}
	})

	t.Run("InstallRendersWhenComponentsPruneToEmpty", func(t *testing.T) {
		target := process(t, blueprintv1alpha1.FluxSystem{
			Name:      "gateway",
			Path:      "gateway",
			Install:   &blueprintv1alpha1.Kustomization{Components: []string{"${false ? 'helm-release' : ''}"}},
			Resources: []blueprintv1alpha1.FluxVariant{{Kustomization: blueprintv1alpha1.Kustomization{Components: []string{"internal"}}}},
		})
		install, ok := find(target, "gateway-install")
		if !ok {
			t.Fatalf("expected an install tier even when its components prune to empty, got %+v", target.AllKustomizations())
		}
		if len(install.Components) != 0 {
			t.Errorf("expected empty components, got %v", install.Components)
		}
		res, ok := find(target, "gateway-resources")
		if !ok {
			t.Fatalf("expected gateway-resources, got %+v", target.AllKustomizations())
		}
		if !slices.Contains(res.DependsOn, "gateway-install") {
			t.Errorf("expected the implicit install edge since install rendered, got %v", res.DependsOn)
		}
	})

	t.Run("ResourcesVariantKeptWhenComponentsPruneToEmpty", func(t *testing.T) {
		target := process(t, blueprintv1alpha1.FluxSystem{
			Name:      "policy",
			Path:      "policy",
			Install:   &blueprintv1alpha1.Kustomization{Components: []string{"kyverno"}},
			Resources: []blueprintv1alpha1.FluxVariant{{Kustomization: blueprintv1alpha1.Kustomization{Components: []string{"${false ? 'resource-limits' : ''}"}}}},
		})
		res, ok := find(target, "policy-resources")
		if !ok {
			t.Fatalf("expected policy-resources to be kept even with no components, got %+v", target.AllKustomizations())
		}
		if len(res.Components) != 0 {
			t.Errorf("expected empty components, got %v", res.Components)
		}
	})

	t.Run("DuplicateUnnamedVariantsReturnsError", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)
		facets := []blueprintv1alpha1.Facet{{
			Metadata: blueprintv1alpha1.Metadata{Name: "gateway"},
			FluxSystems: []blueprintv1alpha1.FluxSystem{{
				Name: "gateway",
				Resources: []blueprintv1alpha1.FluxVariant{
					{Kustomization: blueprintv1alpha1.Kustomization{Components: []string{"a"}}},
					{Kustomization: blueprintv1alpha1.Kustomization{Components: []string{"b"}}},
				},
			}},
		}}
		_, err := processor.ProcessFacets(&blueprintv1alpha1.Blueprint{}, facets)
		if err == nil {
			t.Fatal("expected error for duplicate unnamed resources variant, got nil")
		}
	})

	t.Run("CrossFacetSameNamedVariantDeepMerges", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "gateway-a"},
				FluxSystems: []blueprintv1alpha1.FluxSystem{{
					Name:      "gateway",
					Resources: []blueprintv1alpha1.FluxVariant{{Kustomization: blueprintv1alpha1.Kustomization{Name: "internal", Components: []string{"a"}}}},
				}},
			},
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "gateway-b"},
				FluxSystems: []blueprintv1alpha1.FluxSystem{{
					Name:      "gateway",
					Resources: []blueprintv1alpha1.FluxVariant{{Kustomization: blueprintv1alpha1.Kustomization{Name: "internal", Components: []string{"b"}}}},
				}},
			},
		}
		target := &blueprintv1alpha1.Blueprint{}
		if _, err := processor.ProcessFacets(target, facets); err != nil {
			t.Fatalf("ProcessFacets: %v", err)
		}
		count := 0
		var got blueprintv1alpha1.Kustomization
		for _, k := range target.AllKustomizations() {
			if k.Name == "gateway-resources-internal" {
				count++
				got = k
			}
		}
		if count != 1 {
			t.Fatalf("expected a single gateway-resources-internal after cross-facet merge, got %d", count)
		}
		if !slices.Contains(got.Components, "a") || !slices.Contains(got.Components, "b") {
			t.Errorf("expected merged variant components to union [a b], got %v", got.Components)
		}
	})

	t.Run("CrossOrdinalFacetsStillMergeFluxSystem", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)
		ordProvider := 200
		ordAddon := 300
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "provider-gateway"},
				Ordinal:  &ordProvider,
				FluxSystems: []blueprintv1alpha1.FluxSystem{{
					Name:      "gateway",
					Install:   &blueprintv1alpha1.Kustomization{Components: []string{"envoy"}},
					Resources: []blueprintv1alpha1.FluxVariant{{Kustomization: blueprintv1alpha1.Kustomization{Name: "internal", Components: []string{"internal"}}}},
				}},
			},
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "addon-gateway-external"},
				Ordinal:  &ordAddon,
				FluxSystems: []blueprintv1alpha1.FluxSystem{{
					Name:      "gateway",
					Resources: []blueprintv1alpha1.FluxVariant{{Kustomization: blueprintv1alpha1.Kustomization{Name: "external", Components: []string{"external"}}}},
				}},
			},
		}
		target := &blueprintv1alpha1.Blueprint{}
		if _, err := processor.ProcessFacets(target, facets); err != nil {
			t.Fatalf("ProcessFacets: %v", err)
		}
		install, ok := find(target, "gateway-install")
		if !ok || !slices.Contains(install.Components, "envoy") {
			t.Fatalf("expected the lower-ordinal facet's install to survive a higher-ordinal merge, got %+v", target.AllKustomizations())
		}
		if _, ok := find(target, "gateway-resources-internal"); !ok {
			t.Errorf("expected the lower-ordinal facet's resources variant to survive, got %+v", target.AllKustomizations())
		}
		if _, ok := find(target, "gateway-resources-external"); !ok {
			t.Errorf("expected the higher-ordinal facet's resources variant to be added rather than replace the system, got %+v", target.AllKustomizations())
		}
	})

	t.Run("SameOrdinalInstallDeepMerges", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "gateway-a"},
				FluxSystems: []blueprintv1alpha1.FluxSystem{{
					Name:    "gateway",
					Install: &blueprintv1alpha1.Kustomization{Components: []string{"envoy"}},
				}},
			},
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "gateway-b"},
				FluxSystems: []blueprintv1alpha1.FluxSystem{{
					Name:    "gateway",
					Install: &blueprintv1alpha1.Kustomization{Components: []string{"cert-manager"}},
				}},
			},
		}
		target := &blueprintv1alpha1.Blueprint{}
		if _, err := processor.ProcessFacets(target, facets); err != nil {
			t.Fatalf("ProcessFacets: %v", err)
		}
		install, ok := find(target, "gateway-install")
		if !ok {
			t.Fatalf("expected gateway-install, got %+v", target.AllKustomizations())
		}
		if !slices.Contains(install.Components, "envoy") || !slices.Contains(install.Components, "cert-manager") {
			t.Errorf("expected install components to union [cert-manager envoy], got %v", install.Components)
		}
	})

	t.Run("HigherOrdinalRemoveDeletesFluxSystem", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)
		ordProvider := 200
		ordRemoval := 300
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "provider-gateway"},
				Ordinal:  &ordProvider,
				FluxSystems: []blueprintv1alpha1.FluxSystem{{
					Name:    "gateway",
					Install: &blueprintv1alpha1.Kustomization{Components: []string{"envoy"}},
				}},
			},
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "option-remove-gateway"},
				Ordinal:  &ordRemoval,
				FluxSystems: []blueprintv1alpha1.FluxSystem{{
					Name:     "gateway",
					Strategy: "remove",
				}},
			},
		}
		target := &blueprintv1alpha1.Blueprint{}
		if _, err := processor.ProcessFacets(target, facets); err != nil {
			t.Fatalf("ProcessFacets: %v", err)
		}
		if _, ok := find(target, "gateway-install"); ok {
			t.Errorf("expected a higher-ordinal remove to delete the system, but gateway-install survived: %+v", target.AllKustomizations())
		}
		for _, sys := range target.FluxSystems {
			if sys.Name == "gateway" {
				t.Errorf("expected no gateway FluxSystem entry after a higher-ordinal remove, got %+v", sys)
			}
		}
	})

	t.Run("EqualOrdinalRemoveStrategyPrecedenceDeletesFluxSystem", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "gateway-a"},
				FluxSystems: []blueprintv1alpha1.FluxSystem{{
					Name:    "gateway",
					Install: &blueprintv1alpha1.Kustomization{Components: []string{"envoy"}},
				}},
			},
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "gateway-b"},
				FluxSystems: []blueprintv1alpha1.FluxSystem{{
					Name:     "gateway",
					Strategy: "remove",
				}},
			},
		}
		target := &blueprintv1alpha1.Blueprint{}
		if _, err := processor.ProcessFacets(target, facets); err != nil {
			t.Fatalf("ProcessFacets: %v", err)
		}
		if _, ok := find(target, "gateway-install"); ok {
			t.Errorf("expected equal-ordinal remove (higher strategy precedence) to delete the system, but gateway-install survived: %+v", target.AllKustomizations())
		}
		for _, sys := range target.FluxSystems {
			if sys.Name == "gateway" {
				t.Errorf("expected no gateway FluxSystem entry after remove wins on strategy precedence, got %+v", sys)
			}
		}
	})

	t.Run("EqualOrdinalRemoveFacetsAccumulateAndStripFieldsFromPreexistingSystem", func(t *testing.T) {
		mocks := setupProcessorMocks(t)
		processor := NewBlueprintProcessor(mocks.Runtime)
		facets := []blueprintv1alpha1.Facet{
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "remove-install-component"},
				FluxSystems: []blueprintv1alpha1.FluxSystem{{
					Name:     "gateway",
					Strategy: "remove",
					Install:  &blueprintv1alpha1.Kustomization{Components: []string{"envoy"}},
				}},
			},
			{
				Metadata: blueprintv1alpha1.Metadata{Name: "remove-resources-variant"},
				FluxSystems: []blueprintv1alpha1.FluxSystem{{
					Name:      "gateway",
					Strategy:  "remove",
					Resources: []blueprintv1alpha1.FluxVariant{{Kustomization: blueprintv1alpha1.Kustomization{Name: "internal"}}},
				}},
			},
		}
		// A pre-existing FluxSystem, standing in for one already declared in the loader's parsed
		// blueprint.yaml (bp := loader.GetBlueprint() in handler.go's processLoader) before facets
		// are layered on, so the two remove-strategy facets above have a real entry to subtract from.
		target := &blueprintv1alpha1.Blueprint{
			FluxSystems: []blueprintv1alpha1.FluxSystem{{
				Name:    "gateway",
				Install: &blueprintv1alpha1.Kustomization{Components: []string{"envoy", "keep_component"}},
				Resources: []blueprintv1alpha1.FluxVariant{
					{Kustomization: blueprintv1alpha1.Kustomization{Name: "internal", Components: []string{"internal"}}},
					{Kustomization: blueprintv1alpha1.Kustomization{Name: "external", Components: []string{"external"}}},
				},
			}},
		}
		if _, err := processor.ProcessFacets(target, facets); err != nil {
			t.Fatalf("ProcessFacets: %v", err)
		}

		var sys blueprintv1alpha1.FluxSystem
		found := false
		for _, s := range target.FluxSystems {
			if s.Name == "gateway" {
				sys, found = s, true
			}
		}
		if !found {
			t.Fatalf("expected the gateway FluxSystem to survive field-level removal, got %+v", target.FluxSystems)
		}
		if sys.Install == nil || len(sys.Install.Components) != 1 || sys.Install.Components[0] != "keep_component" {
			t.Errorf("expected only 'keep_component' to remain on install, got %+v", sys.Install)
		}
		if len(sys.Resources) != 1 || sys.Resources[0].Name != "external" {
			t.Errorf("expected only the 'external' resources variant to remain, got %+v", sys.Resources)
		}
	})

	t.Run("InstallWithNoComponentsAtAllIsKept", func(t *testing.T) {
		target := process(t, blueprintv1alpha1.FluxSystem{
			Name:    "cert-manager",
			Install: &blueprintv1alpha1.Kustomization{Timeout: &blueprintv1alpha1.DurationString{Duration: 5 * time.Minute}},
		})
		if len(target.FluxSystems) != 1 || target.FluxSystems[0].Install == nil {
			t.Fatalf("expected install to survive with no components declared, got %+v", target.FluxSystems)
		}
		if len(target.FluxSystems[0].Install.Components) != 0 {
			t.Errorf("expected install components to stay empty, got %v", target.FluxSystems[0].Install.Components)
		}
	})

	t.Run("ResourcesInheritsTimeoutAndIntervalFromInstallWhenUnset", func(t *testing.T) {
		target := process(t, blueprintv1alpha1.FluxSystem{
			Name: "lb",
			Path: "lb",
			Install: &blueprintv1alpha1.Kustomization{
				Components: []string{"metallb"},
				Timeout:    &blueprintv1alpha1.DurationString{Duration: 5 * time.Minute},
				Interval:   &blueprintv1alpha1.DurationString{Duration: 5 * time.Minute},
			},
			Resources: []blueprintv1alpha1.FluxVariant{{Kustomization: blueprintv1alpha1.Kustomization{Components: []string{"metallb/arp"}}}},
		})
		res, ok := find(target, "lb-resources")
		if !ok {
			t.Fatalf("expected lb-resources, got %+v", target.AllKustomizations())
		}
		if res.Timeout == nil || res.Timeout.Duration != 5*time.Minute {
			t.Errorf("expected resources timeout to inherit 5m from install, got %+v", res.Timeout)
		}
		if res.Interval == nil || res.Interval.Duration != 5*time.Minute {
			t.Errorf("expected resources interval to inherit 5m from install, got %+v", res.Interval)
		}
	})

	t.Run("ResourcesExplicitTimeoutAndIntervalAreNotOverriddenByInstall", func(t *testing.T) {
		target := process(t, blueprintv1alpha1.FluxSystem{
			Name: "policy",
			Path: "policy",
			Install: &blueprintv1alpha1.Kustomization{
				Components: []string{"kyverno"},
				Timeout:    &blueprintv1alpha1.DurationString{Duration: 30 * time.Minute},
				Interval:   &blueprintv1alpha1.DurationString{Duration: 5 * time.Minute},
			},
			Resources: []blueprintv1alpha1.FluxVariant{{Kustomization: blueprintv1alpha1.Kustomization{
				Components: []string{"kyverno/resource-limits-requests"},
				Timeout:    &blueprintv1alpha1.DurationString{Duration: 5 * time.Minute},
				Interval:   &blueprintv1alpha1.DurationString{Duration: 5 * time.Minute},
			}}},
		})
		res, ok := find(target, "policy-resources")
		if !ok {
			t.Fatalf("expected policy-resources, got %+v", target.AllKustomizations())
		}
		if res.Timeout == nil || res.Timeout.Duration != 5*time.Minute {
			t.Errorf("expected the variant's own 5m timeout to survive, got %+v", res.Timeout)
		}
		if res.Interval == nil || res.Interval.Duration != 5*time.Minute {
			t.Errorf("expected the variant's own 5m interval to survive, got %+v", res.Interval)
		}
	})

	t.Run("ResourcesTimeoutAndIntervalStayUnsetWithNoInstallTier", func(t *testing.T) {
		target := process(t, blueprintv1alpha1.FluxSystem{
			Name:      "issuers",
			Path:      "pki/issuers",
			Resources: []blueprintv1alpha1.FluxVariant{{Kustomization: blueprintv1alpha1.Kustomization{Components: []string{"public-issuer"}}}},
		})
		res, ok := find(target, "issuers-resources")
		if !ok {
			t.Fatalf("expected issuers-resources, got %+v", target.AllKustomizations())
		}
		if res.Timeout != nil || res.Interval != nil {
			t.Errorf("expected no install tier to leave timeout/interval unset, got timeout=%+v interval=%+v", res.Timeout, res.Interval)
		}
	})
}

func TestProcessor_ProcessFacets_StripsFluxSystemWhen(t *testing.T) {
	t.Run("EmittedSystemHasNoWhenAfterInclusion", func(t *testing.T) {
		// Given a facet whose flux system is gated on composition-only derived config: talos_enabled
		// comes from the facet's own config: block, so it exists during composition but not in the
		// emitted values-* ConfigMaps downstream.
		mocks := setupProcessorMocks(t)
		mocks.ConfigHandler.GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"platform": "hetzner"}, nil
		}
		processor := NewBlueprintProcessor(mocks.Runtime)
		target := &blueprintv1alpha1.Blueprint{}
		facets := []blueprintv1alpha1.Facet{{
			Metadata: blueprintv1alpha1.Metadata{Name: "platform"},
			Config: []blueprintv1alpha1.ConfigBlock{
				{Name: "talos_enabled", Body: map[string]any{"value": true}},
			},
			FluxSystems: []blueprintv1alpha1.FluxSystem{{
				Name:    "cni",
				When:    "talos_enabled",
				Install: &blueprintv1alpha1.Kustomization{Components: []string{"cilium"}},
			}},
		}}

		// When the blueprint is composed
		if _, err := processor.ProcessFacets(target, facets); err != nil {
			t.Fatalf("ProcessFacets: %v", err)
		}

		// Then the system is included — its when evaluated true during composition — but the
		// resolved when is stripped from the emitted system so it can't be re-evaluated (and misfire)
		// against a downstream scope that lacks talos_enabled.
		var cni *blueprintv1alpha1.FluxSystem
		for i := range target.FluxSystems {
			if target.FluxSystems[i].Name == "cni" {
				cni = &target.FluxSystems[i]
			}
		}
		if cni == nil {
			t.Fatalf("expected cni flux system to be included, got %+v", target.FluxSystems)
		}
		if cni.When != "" {
			t.Errorf("expected emitted FluxSystem.When to be stripped, got %q", cni.When)
		}
	})
}

// =============================================================================
// Test Private Methods
// =============================================================================

func TestValidateSystemSecrets(t *testing.T) {
	processor := func(t *testing.T) *BaseBlueprintProcessor {
		return NewBlueprintProcessor(setupProcessorMocks(t).Runtime)
	}

	t.Run("AcceptsSensitivePropertyReference", func(t *testing.T) {
		err := processor(t).validateSystemSecrets("cdn", map[string]blueprintv1alpha1.SecretEntry{"cloudflare-creds": {Data: map[string]string{"api_token": "${cdn.cloudflare_api_key}"}}})
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("AcceptsNonSensitiveReferenceWithOptionalDefault", func(t *testing.T) {
		// A reference to a non-sensitive property (with a ?? default) is fine — it goes into a Secret
		err := processor(t).validateSystemSecrets("telemetry", map[string]blueprintv1alpha1.SecretEntry{
			"alertmanager-notification-slack": {Data: map[string]string{"url": "${telemetry.alerts.slack.webhook_url ?? ''}"}},
		})
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("AcceptsDirectSecretExpression", func(t *testing.T) {
		err := processor(t).validateSystemSecrets("telemetry", map[string]blueprintv1alpha1.SecretEntry{
			"alertmanager-slack": {Data: map[string]string{"webhook_url": `${secret("Developer", "slack_webhooks", "url")}`}},
		})
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("AcceptsHybridExpression", func(t *testing.T) {
		// An expression embedded in surrounding text resolves to a valid secret value and is allowed
		err := processor(t).validateSystemSecrets("cdn", map[string]blueprintv1alpha1.SecretEntry{"creds": {Data: map[string]string{"url": "https://api.example.com/${cdn.token}"}}})
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("RejectsPlaintextLiteral", func(t *testing.T) {
		// A hardcoded literal would be serialized with the blueprint (git-committed), so it fails closed
		err := processor(t).validateSystemSecrets("cdn", map[string]blueprintv1alpha1.SecretEntry{"creds": {Data: map[string]string{"token": "actual-secret-value"}}})
		if err == nil || !strings.Contains(err.Error(), "not a plaintext literal") {
			t.Errorf("Expected plaintext-literal error, got %v", err)
		}
	})

	t.Run("RejectsUnterminatedExpression", func(t *testing.T) {
		// A malformed, unterminated interpolation is caught at composition rather than at apply time
		err := processor(t).validateSystemSecrets("cdn", map[string]blueprintv1alpha1.SecretEntry{"creds": {Data: map[string]string{"token": "${cdn.token"}}})
		if err == nil || !strings.Contains(err.Error(), "unterminated expression") {
			t.Errorf("Expected unterminated-expression error, got %v", err)
		}
	})

	t.Run("RejectsEntryWithNoDataKeys", func(t *testing.T) {
		// An entry with no data keys — a genuinely empty entry, or the retired flat form whose keys the
		// decoder silently discarded — fails loudly rather than silently placing nothing
		err := processor(t).validateSystemSecrets("cdn", map[string]blueprintv1alpha1.SecretEntry{"creds": {Namespaces: []string{"system-dns"}}})
		if err == nil || !strings.Contains(err.Error(), "no data keys") {
			t.Errorf("Expected no-data-keys error, got %v", err)
		}
	})
}

// =============================================================================
// Test Helpers
// =============================================================================

func TestMergeSecretData(t *testing.T) {
	t.Run("UnionsSecretsAndDataKeysWithOverlayWinning", func(t *testing.T) {
		// Given two facets contributing secrets: a shared secret with an overlapping data key,
		// and a secret unique to each side
		base := map[string]blueprintv1alpha1.SecretEntry{
			"shared":    {Data: map[string]string{"a": "1", "b": "2"}},
			"base-only": {Data: map[string]string{"x": "9"}},
		}
		overlay := map[string]blueprintv1alpha1.SecretEntry{
			"shared":       {Data: map[string]string{"b": "override", "c": "3"}},
			"overlay-only": {Data: map[string]string{"y": "8"}},
		}

		// When merging
		got := mergeSecretData(base, overlay)

		// Then secrets union, data keys union, and overlay wins the data-key conflict
		if got["shared"].Data["a"] != "1" || got["shared"].Data["b"] != "override" || got["shared"].Data["c"] != "3" {
			t.Errorf("Unexpected shared secret merge: %v", got["shared"])
		}
		if got["base-only"].Data["x"] != "9" || got["overlay-only"].Data["y"] != "8" {
			t.Errorf("Expected unique secrets preserved, got %v", got)
		}
	})

	t.Run("UnionsNamespacesForASharedSecret", func(t *testing.T) {
		// Given two facets targeting the same secret at different namespaces
		base := map[string]blueprintv1alpha1.SecretEntry{"acme-dns": {Namespaces: []string{"system-pki"}, Data: map[string]string{"token": "${x.t}"}}}
		overlay := map[string]blueprintv1alpha1.SecretEntry{"acme-dns": {Namespaces: []string{"system-dns", "system-pki"}, Data: map[string]string{"token": "${x.t}"}}}

		// When merging
		got := mergeSecretData(base, overlay)

		// Then the namespaces union without duplicating the shared one
		ns := got["acme-dns"].Namespaces
		if len(ns) != 2 || !slices.Contains(ns, "system-pki") || !slices.Contains(ns, "system-dns") {
			t.Errorf("Expected namespaces to union to [system-pki system-dns], got %v", ns)
		}
	})

	t.Run("ReturnsNilWhenBothEmpty", func(t *testing.T) {
		if got := mergeSecretData(nil, nil); got != nil {
			t.Errorf("Expected nil, got %v", got)
		}
	})

	t.Run("DoesNotMutateInputs", func(t *testing.T) {
		// Given a base with a shared secret
		base := map[string]blueprintv1alpha1.SecretEntry{"shared": {Data: map[string]string{"a": "1"}}}
		overlay := map[string]blueprintv1alpha1.SecretEntry{"shared": {Data: map[string]string{"b": "2"}}}

		// When merging
		_ = mergeSecretData(base, overlay)

		// Then the base's inner map is unchanged
		if len(base["shared"].Data) != 1 || base["shared"].Data["a"] != "1" {
			t.Errorf("Expected base unmutated, got %v", base)
		}
		if _, exists := base["shared"].Data["b"]; exists {
			t.Error("Expected base inner map to not gain overlay key")
		}
	})
}
