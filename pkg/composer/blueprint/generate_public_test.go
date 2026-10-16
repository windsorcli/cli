package blueprint_test

import (
	"reflect"
	"sort"
	"testing"

	"github.com/windsorcli/cli/pkg/composer/blueprint"
)

// =============================================================================
// Test Setup
// =============================================================================

// deferredFacet declares a network component, a cluster component whose input reads a network
// output, and a kustomization whose substitution reads the same output.
const deferredFacet = "terraform:\n- name: net\n  path: net\n- name: cluster\n  path: cluster\n  dependsOn: [net]\n  inputs:\n    vpc: ${terraform_output('net', 'vpc_id')}\n    plain: x\nkustomize:\n- name: k\n  path: k\n  substitutions:\n    vpc: ${terraform_output('net', 'vpc_id')}\n    platform: ${platform}\n"

const deferredExpression = "${terraform_output('net', 'vpc_id')}"

// =============================================================================
// Test Public Methods
// =============================================================================

func TestGenerate(t *testing.T) {
	t.Run("ReturnsNilBeforeABlueprintComposes", func(t *testing.T) {
		// Given a handler that has not loaded a blueprint
		f := newFixture(t)
		h := blueprint.NewBlueprintHandler(f.runtime, f.artifact)

		// When the blueprint is generated
		bp := h.Generate()

		// Then no blueprint is returned
		if bp != nil {
			t.Errorf("expected nil, got %+v", bp)
		}
	})

	t.Run("KeepsDeferredExpressions", func(t *testing.T) {
		// Given values that read a terraform output
		f := newFixture(t)
		f.project(t)
		f.facet(t, "stack", deferredFacet)

		// When the blueprint is generated
		bp := f.mustLoad(t).Generate()

		// Then the deferred values keep their expressions, and other values resolve
		if got := component(bp, "cluster").Inputs; got["vpc"] != deferredExpression || got["plain"] != "x" {
			t.Errorf("unexpected cluster inputs %v", got)
		}
		if got := kustomization(bp, "k").Substitutions; got["vpc"] != deferredExpression || got["platform"] != "docker" {
			t.Errorf("unexpected substitutions %v", got)
		}
	})
}

func TestGenerateResolved(t *testing.T) {
	t.Run("ResolvesSubstitutionsFromTerraformOutputs", func(t *testing.T) {
		// Given a deferred substitution and an applied network output
		f := newFixture(t)
		f.project(t)
		f.facet(t, "stack", deferredFacet)
		h := f.mustLoad(t)
		f.outputs["net"] = map[string]any{"vpc_id": "vpc-1"}

		// When the resolved blueprint is generated
		bp, err := h.GenerateResolved()

		// Then the substitution holds the output
		if err != nil {
			t.Fatalf("GenerateResolved: %v", err)
		}
		if got := kustomization(bp, "k").Substitutions["vpc"]; got != "vpc-1" {
			t.Errorf("expected vpc-1, got %q", got)
		}
	})

	t.Run("ResolvesAMissingOutputToEmpty", func(t *testing.T) {
		// Given a deferred substitution and no applied outputs
		f := newFixture(t)
		f.project(t)
		f.facet(t, "stack", deferredFacet)
		h := f.mustLoad(t)

		// When the resolved blueprint is generated
		bp, err := h.GenerateResolved()

		// Then the substitution is empty
		if err != nil {
			t.Fatalf("GenerateResolved: %v", err)
		}
		if got, ok := kustomization(bp, "k").Substitutions["vpc"]; !ok || got != "" {
			t.Errorf("expected an empty vpc substitution, got %q (present=%v)", got, ok)
		}
	})

	t.Run("LeavesTerraformInputsDeferred", func(t *testing.T) {
		// Given a deferred terraform input and an applied network output
		f := newFixture(t)
		f.project(t)
		f.facet(t, "stack", deferredFacet)
		h := f.mustLoad(t)
		f.outputs["net"] = map[string]any{"vpc_id": "vpc-1"}

		// When the resolved blueprint is generated
		bp, _ := h.GenerateResolved()

		// Then the input keeps its expression for terraform to resolve at apply time
		if got := component(bp, "cluster").Inputs["vpc"]; got != deferredExpression {
			t.Errorf("expected the deferred expression, got %v", got)
		}
	})

	t.Run("LeavesTheComposedBlueprintUnchanged", func(t *testing.T) {
		// Given a deferred substitution and an applied network output
		f := newFixture(t)
		f.project(t)
		f.facet(t, "stack", deferredFacet)
		h := f.mustLoad(t)
		f.outputs["net"] = map[string]any{"vpc_id": "vpc-1"}

		// When the resolved blueprint is generated
		_, _ = h.GenerateResolved()

		// Then Generate still returns the deferred expression
		if got := kustomization(h.Generate(), "k").Substitutions["vpc"]; got != deferredExpression {
			t.Errorf("expected the deferred expression, got %q", got)
		}
	})
}

func TestGetDeferredPaths(t *testing.T) {
	t.Run("ListsEveryDeferredValue", func(t *testing.T) {
		// Given an input and a substitution that read a terraform output
		f := newFixture(t)
		f.project(t)
		f.facet(t, "stack", deferredFacet)

		// When the deferred paths are read
		got := f.mustLoad(t).GetDeferredPaths()

		// Then both paths are listed
		want := map[string]bool{"terraform.cluster.inputs.vpc": true, "kustomize.k.substitutions.vpc": true}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("expected %v, got %v", want, got)
		}
	})
}

func TestDeferredSubstitutionsInScope(t *testing.T) {
	t.Run("ReportsWhetherAKustomizationInScopeHasADeferredSubstitution", func(t *testing.T) {
		// Given a kustomization with a deferred substitution
		f := newFixture(t)
		f.project(t)
		f.facet(t, "stack", deferredFacet)
		h := f.mustLoad(t)

		// When scopes with and without that kustomization are checked
		inScope := h.DeferredSubstitutionsInScope(map[string]bool{"k": true})
		outOfScope := h.DeferredSubstitutionsInScope(map[string]bool{"other": true})

		// Then only the scope that holds it reports true
		if !inScope || outOfScope {
			t.Errorf("expected true and false, got %v and %v", inScope, outOfScope)
		}
	})
}

func TestGetTerraformComponents(t *testing.T) {
	t.Run("ReturnsTheComposedComponents", func(t *testing.T) {
		// Given two terraform components
		f := newFixture(t)
		f.project(t)
		f.facet(t, "stack", deferredFacet)

		// When the terraform components are read
		got := f.mustLoad(t).GetTerraformComponents()

		// Then both are returned in dependency order
		var ids []string
		for _, c := range got {
			ids = append(ids, c.GetID())
		}
		if !reflect.DeepEqual(ids, []string{"net", "cluster"}) {
			t.Errorf("expected [net cluster], got %v", ids)
		}
	})
}

func TestGetLocalTemplateData(t *testing.T) {
	t.Run("ReturnsTheTemplateFiles", func(t *testing.T) {
		// Given a template blueprint and one facet
		f := newFixture(t)
		f.project(t)
		f.facet(t, "stack", deferredFacet)
		h := f.mustLoad(t)

		// When the local template data is read
		data, err := h.GetLocalTemplateData()

		// Then it holds each template file by its path under the template
		if err != nil {
			t.Fatalf("GetLocalTemplateData: %v", err)
		}
		var keys []string
		for key := range data {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		if !reflect.DeepEqual(keys, []string{"blueprint.yaml", "facets/stack.yaml"}) {
			t.Errorf("unexpected template files %v", keys)
		}
	})
}
