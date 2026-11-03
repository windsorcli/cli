package plan_test

import (
	"fmt"
	"strings"
	"testing"

	fluxinfra "github.com/windsorcli/cli/pkg/provisioner/flux"
	terraforminfra "github.com/windsorcli/cli/pkg/provisioner/terraform"
	"github.com/windsorcli/cli/pkg/tui/plan"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestDestroySummary(t *testing.T) {
	t.Run("HeaderReadsDestroyPlanNotPlanSummary", func(t *testing.T) {
		// The header text disambiguates apply-vs-destroy at a glance for the
		// operator. Distinct from Summary's "Windsor Plan Summary".
		var buf strings.Builder
		plan.DestroySummary(&buf, nil, nil, true)
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
		plan.DestroySummary(&buf,
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
		plan.DestroySummary(&buf, nil,
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
		plan.DestroySummary(&buf,
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
		err := plan.DestroySummaryJSON(&buf,
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

func TestDestroySummary_TerraformRow(t *testing.T) {
	t.Run("RendersNoStateWhenIsNew", func(t *testing.T) {
		// Given a terraform plan for a component with no state
		p := terraforminfra.TerraformComponentPlan{
			ComponentID: "vpc",
			IsNew:       true,
		}

		// When the destroy summary renders
		got := destroyTerraformRow(t, p, "vpc")

		// Then the row reads (no state)
		if got != "(no state)" {
			t.Errorf("expected (no state), got %q", got)
		}
	})

	t.Run("ErrorTakesPrecedenceOverIsNew", func(t *testing.T) {
		// Given a new terraform plan that also failed
		p := terraforminfra.TerraformComponentPlan{
			ComponentID: "vpc",
			IsNew:       true,
			Err:         fmt.Errorf("boom"),
		}

		// When the destroy summary renders
		got := destroyTerraformRow(t, p, "vpc")

		// Then the row shows the error
		if !strings.Contains(got, "boom") {
			t.Errorf("expected error message to win, got %q", got)
		}
	})

	t.Run("RendersDestroyCountFromResources", func(t *testing.T) {
		// Given a terraform plan with a destroy count and two deleted resources
		p := terraforminfra.TerraformComponentPlan{
			ComponentID: "cluster",
			Destroy:     5,
			Resources: []terraforminfra.ResourceChange{
				{Address: "aws_eks.main", Action: terraforminfra.ActionDelete},
				{Address: "aws_iam_role.eks", Action: terraforminfra.ActionDelete},
			},
		}

		// When the destroy summary renders
		got := destroyTerraformRow(t, p, "cluster")

		// Then the row counts the resources
		if got != "-2" {
			t.Errorf("expected -2, got %q", got)
		}
	})

	t.Run("FallsBackToDestroyFieldWhenResourcesEmpty", func(t *testing.T) {
		// Given a terraform plan with a destroy count and no resources
		p := terraforminfra.TerraformComponentPlan{
			ComponentID: "vpc",
			Destroy:     7,
		}

		// When the destroy summary renders
		got := destroyTerraformRow(t, p, "vpc")

		// Then the row shows the destroy count
		if got != "-7" {
			t.Errorf("expected -7, got %q", got)
		}
	})

	t.Run("RendersNothingToDestroyWhenZero", func(t *testing.T) {
		// Given a terraform plan with state and zero deletions
		p := terraforminfra.TerraformComponentPlan{
			ComponentID: "vpc",
		}

		// When the destroy summary renders
		got := destroyTerraformRow(t, p, "vpc")

		// Then the row reads (nothing to destroy)
		if got != "(nothing to destroy)" {
			t.Errorf("expected (nothing to destroy), got %q", got)
		}
	})
}

func TestDestroySummary_KustomizeRow(t *testing.T) {
	t.Run("RendersNotDeployedWhenIsNew", func(t *testing.T) {
		// Given a kustomization plan that is absent from the cluster
		p := fluxinfra.KustomizePlan{
			Name:  "monitoring",
			IsNew: true,
		}

		// When the destroy summary renders
		got := destroyKustomizeRow(t, p, "monitoring")

		// Then the row reads (not deployed)
		if got != "(not deployed)" {
			t.Errorf("expected (not deployed), got %q", got)
		}
	})

	t.Run("RendersDestroyCountFromResources", func(t *testing.T) {
		// Given a kustomization plan with a removed-line count and three deleted resources
		p := fluxinfra.KustomizePlan{
			Name:    "monitoring",
			Removed: 99,
			Resources: []fluxinfra.ResourceChange{
				{Address: "Deployment/monitoring/grafana", Action: fluxinfra.ActionDelete},
				{Address: "ConfigMap/monitoring/grafana-config", Action: fluxinfra.ActionDelete},
				{Address: "Service/monitoring/grafana", Action: fluxinfra.ActionDelete},
			},
		}

		// When the destroy summary renders
		got := destroyKustomizeRow(t, p, "monitoring")

		// Then the row counts the resources
		if got != "-3" {
			t.Errorf("expected -3, got %q", got)
		}
	})

	t.Run("RendersNothingToDestroyForEmptyKustomization", func(t *testing.T) {
		// Given a deployed kustomization plan with an empty inventory
		p := fluxinfra.KustomizePlan{
			Name: "monitoring",
		}

		// When the destroy summary renders
		got := destroyKustomizeRow(t, p, "monitoring")

		// Then the row reads (nothing to destroy)
		if got != "(nothing to destroy)" {
			t.Errorf("expected (nothing to destroy), got %q", got)
		}
	})
}

func TestDestroySummary_Color(t *testing.T) {
	t.Run("ColorsCountsAndErrors", func(t *testing.T) {
		// Given a terraform plan with a destroy count and a failed kustomization plan
		tfPlans := []terraforminfra.TerraformComponentPlan{{ComponentID: "vpc", Destroy: 3}}
		k8sPlans := []fluxinfra.KustomizePlan{
			{Name: "broken", Err: fmt.Errorf("boom\ndetail")},
			{Name: "gone", Resources: []fluxinfra.ResourceChange{{Address: "Service/x/a", Action: fluxinfra.ActionDelete}}},
		}

		// When the destroy summary renders with color
		var buf strings.Builder
		plan.DestroySummary(&buf, tfPlans, k8sPlans, false)

		// Then the counts and errors carry ANSI colors and the error detail follows its row
		out := buf.String()
		for _, want := range []string{"\033[31m-3\033[0m", "\033[31m(error: boom)\033[0m", "  detail\n", "\033[31m-1\033[0m"} {
			if !strings.Contains(out, want) {
				t.Errorf("expected output to contain %q, got:\n%s", want, out)
			}
		}
	})

	t.Run("RendersKustomizeErrorWithoutColor", func(t *testing.T) {
		// Given a failed kustomization plan
		p := fluxinfra.KustomizePlan{Name: "monitoring", Err: fmt.Errorf("boom")}

		// When the destroy summary renders
		got := destroyKustomizeRow(t, p, "monitoring")

		// Then the row shows the error
		if got != "(error: boom)" {
			t.Errorf("expected (error: boom), got %q", got)
		}
	})
}

// =============================================================================
// Test Helpers
// =============================================================================

// destroyTerraformRow renders p with DestroySummary and returns the status text of its row.
func destroyTerraformRow(t *testing.T, p terraforminfra.TerraformComponentPlan, name string) string {
	t.Helper()
	var buf strings.Builder
	plan.DestroySummary(&buf, []terraforminfra.TerraformComponentPlan{p}, nil, true)
	return rowStatus(t, buf.String(), name)
}

// destroyKustomizeRow renders p with DestroySummary and returns the status text of its row.
func destroyKustomizeRow(t *testing.T, p fluxinfra.KustomizePlan, name string) string {
	t.Helper()
	var buf strings.Builder
	plan.DestroySummary(&buf, nil, []fluxinfra.KustomizePlan{p}, true)
	return rowStatus(t, buf.String(), name)
}
