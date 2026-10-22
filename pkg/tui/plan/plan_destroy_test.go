package plan

import (
	"fmt"
	"strings"
	"testing"

	fluxinfra "github.com/windsorcli/cli/pkg/provisioner/flux"
	terraforminfra "github.com/windsorcli/cli/pkg/provisioner/terraform"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestDestroySummary(t *testing.T) {
	t.Run("HeaderReadsDestroyPlanNotPlanSummary", func(t *testing.T) {
		// The header text disambiguates apply-vs-destroy at a glance for the
		// operator. Distinct from Summary's "Windsor Plan Summary".
		var buf strings.Builder
		DestroySummary(&buf, nil, nil, true)
		out := buf.String()
		if !strings.Contains(out, "Windsor Destroy Plan") {
			t.Errorf("expected Windsor Destroy Plan header, got:\n%s", out)
		}
		if strings.Contains(out, "Windsor Plan Summary") {
			t.Errorf("did not expect apply-side header, got:\n%s", out)
		}
	})

	t.Run("RendersIsNewAsNoStateForTerraform", func(t *testing.T) {
		// End-to-end check that a destroy-side TerraformComponentPlan with
		// IsNew=true does not regress to the apply-side "(new)" string.
		var buf strings.Builder
		DestroySummary(&buf,
			[]terraforminfra.TerraformComponentPlan{{ComponentID: "vpc", IsNew: true}},
			nil, true)
		out := buf.String()
		if !strings.Contains(out, "(no state)") {
			t.Errorf("expected (no state) for IsNew terraform component, got:\n%s", out)
		}
		if strings.Contains(out, "(new)") {
			t.Errorf("destroy renderer must not emit (new) — IsNew dual-meaning regression, got:\n%s", out)
		}
	})

	t.Run("RendersIsNewAsNotDeployedForKustomize", func(t *testing.T) {
		var buf strings.Builder
		DestroySummary(&buf, nil,
			[]fluxinfra.KustomizePlan{{Name: "monitoring", IsNew: true}},
			true)
		out := buf.String()
		if !strings.Contains(out, "(not deployed)") {
			t.Errorf("expected (not deployed) for IsNew kustomization, got:\n%s", out)
		}
		if strings.Contains(out, "(new)") {
			t.Errorf("destroy renderer must not emit (new), got:\n%s", out)
		}
	})

	t.Run("EnumeratesDeleteResourcesUnderEachComponent", func(t *testing.T) {
		// Operators see the same "- <address>" list shape they get on the apply
		// side, just with delete-only entries.
		var buf strings.Builder
		DestroySummary(&buf,
			[]terraforminfra.TerraformComponentPlan{{
				ComponentID: "cluster", Path: "cluster/aws-eks", Destroy: 2,
				Resources: []terraforminfra.ResourceChange{
					{Address: "aws_eks_cluster.main", Action: terraforminfra.ActionDelete},
					{Address: "aws_iam_role.eks", Action: terraforminfra.ActionDelete},
				},
			}},
			[]fluxinfra.KustomizePlan{{
				Name: "monitoring", Removed: 1,
				Resources: []fluxinfra.ResourceChange{
					{Address: "Deployment/monitoring/grafana", Action: fluxinfra.ActionDelete},
				},
			}},
			true)
		out := buf.String()
		for _, want := range []string{
			"- aws_eks_cluster.main",
			"- aws_iam_role.eks",
			"- Deployment/monitoring/grafana",
		} {
			if !strings.Contains(out, want) {
				t.Errorf("expected output to contain %q, got:\n%s", want, out)
			}
		}
	})
}

func TestDestroySummaryJSON(t *testing.T) {
	t.Run("EmitsResourcesWithDeleteAction", func(t *testing.T) {
		var buf strings.Builder
		err := DestroySummaryJSON(&buf,
			[]terraforminfra.TerraformComponentPlan{{
				ComponentID: "cluster",
				Destroy:     2,
				Resources: []terraforminfra.ResourceChange{
					{Address: "aws_eks_cluster.main", Action: terraforminfra.ActionDelete},
				},
			}},
			[]fluxinfra.KustomizePlan{{
				Name:    "monitoring",
				Removed: 1,
				Resources: []fluxinfra.ResourceChange{
					{Address: "Deployment/monitoring/grafana", Action: fluxinfra.ActionDelete},
				},
			}},
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		for _, want := range []string{
			`"address": "aws_eks_cluster.main"`,
			`"action": "delete"`,
			`"address": "Deployment/monitoring/grafana"`,
		} {
			if !strings.Contains(out, want) {
				t.Errorf("expected JSON to contain %q, got:\n%s", want, out)
			}
		}
		// Schema should not carry the apply-side add/change/no_changes counts
		// for kustomize since they don't apply to destroy.
		if strings.Contains(out, `"add":`) || strings.Contains(out, `"change":`) {
			t.Errorf("destroy JSON should not carry apply-side counts, got:\n%s", out)
		}
	})
}

// =============================================================================
// Test Helpers
// =============================================================================

func TestFormatTerraformDestroyPlan(t *testing.T) {
	t.Run("RendersNoStateWhenIsNew", func(t *testing.T) {
		// IsNew on the destroy path means "no state to destroy" — must render
		// "(no state)" rather than the apply-side "(new)" per the IsNew dual-
		// meaning contract on TerraformComponentPlan.
		got := formatTerraformDestroyPlan(terraforminfra.TerraformComponentPlan{
			ComponentID: "vpc",
			IsNew:       true,
		}, true)
		if got != "(no state)" {
			t.Errorf("expected (no state), got %q", got)
		}
	})

	t.Run("ErrorTakesPrecedenceOverIsNew", func(t *testing.T) {
		got := formatTerraformDestroyPlan(terraforminfra.TerraformComponentPlan{
			ComponentID: "vpc",
			IsNew:       true,
			Err:         fmt.Errorf("boom"),
		}, true)
		if !strings.Contains(got, "boom") {
			t.Errorf("expected error message to win, got %q", got)
		}
	})

	t.Run("RendersDestroyCountFromResources", func(t *testing.T) {
		// When Resources is populated, the count is the resource list length —
		// the header row matches what the operator sees expanded below it.
		got := formatTerraformDestroyPlan(terraforminfra.TerraformComponentPlan{
			ComponentID: "cluster",
			Destroy:     5,
			Resources: []terraforminfra.ResourceChange{
				{Address: "aws_eks.main", Action: terraforminfra.ActionDelete},
				{Address: "aws_iam_role.eks", Action: terraforminfra.ActionDelete},
			},
		}, true)
		if got != "-2" {
			t.Errorf("expected -2 (resource-derived), got %q", got)
		}
	})

	t.Run("FallsBackToDestroyFieldWhenResourcesEmpty", func(t *testing.T) {
		// Without Resources we fall back to the raw Destroy field for honesty.
		got := formatTerraformDestroyPlan(terraforminfra.TerraformComponentPlan{
			ComponentID: "vpc",
			Destroy:     7,
		}, true)
		if got != "-7" {
			t.Errorf("expected -7, got %q", got)
		}
	})

	t.Run("RendersNothingToDestroyWhenZero", func(t *testing.T) {
		// Edge case: state exists but plan -destroy reports zero deletions.
		// Should not silently pretend there's work; explicit message instead.
		got := formatTerraformDestroyPlan(terraforminfra.TerraformComponentPlan{
			ComponentID: "vpc",
		}, true)
		if got != "(nothing to destroy)" {
			t.Errorf("expected (nothing to destroy), got %q", got)
		}
	})
}

func TestFormatKustomizeDestroyPlan(t *testing.T) {
	t.Run("RendersNotDeployedWhenIsNew", func(t *testing.T) {
		// IsNew on the kustomize destroy path means "kustomization absent from
		// cluster" — render as "(not deployed)" to match operator vocabulary.
		got := formatKustomizeDestroyPlan(fluxinfra.KustomizePlan{
			Name:  "monitoring",
			IsNew: true,
		}, true)
		if got != "(not deployed)" {
			t.Errorf("expected (not deployed), got %q", got)
		}
	})

	t.Run("RendersDestroyCountFromResources", func(t *testing.T) {
		got := formatKustomizeDestroyPlan(fluxinfra.KustomizePlan{
			Name:    "monitoring",
			Removed: 99,
			Resources: []fluxinfra.ResourceChange{
				{Address: "Deployment/monitoring/grafana", Action: fluxinfra.ActionDelete},
				{Address: "ConfigMap/monitoring/grafana-config", Action: fluxinfra.ActionDelete},
				{Address: "Service/monitoring/grafana", Action: fluxinfra.ActionDelete},
			},
		}, true)
		if got != "-3" {
			t.Errorf("expected -3 (resource-derived), got %q", got)
		}
	})

	t.Run("RendersNothingToDestroyForEmptyKustomization", func(t *testing.T) {
		// Kustomization exists but has empty inventory (e.g., suspended) — say
		// so explicitly.
		got := formatKustomizeDestroyPlan(fluxinfra.KustomizePlan{
			Name: "monitoring",
		}, true)
		if got != "(nothing to destroy)" {
			t.Errorf("expected (nothing to destroy), got %q", got)
		}
	})
}
