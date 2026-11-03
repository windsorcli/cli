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

func TestRenderPlanSummary(t *testing.T) {
	t.Run("ListsTerraformResourcesUnderComponentSortedDestructiveFirst", func(t *testing.T) {
		// Operators read top-down — destructive changes (delete, replace)
		// deserve to surface above creates and updates.
		var buf strings.Builder
		plan.Summary(&buf,
			[]terraforminfra.TerraformComponentPlan{{
				ComponentID: "cluster", Path: "cluster/aws-eks",
				Add: 1, Change: 1, Destroy: 1,
				Resources: []terraforminfra.ResourceChange{
					{Address: "aws_s3_bucket.logs", Action: terraforminfra.ActionCreate},
					{Address: "aws_iam_role.eks", Action: terraforminfra.ActionUpdate},
					{Address: "aws_db_instance.primary", Action: terraforminfra.ActionReplace},
					{Address: "aws_security_group.legacy", Action: terraforminfra.ActionDelete},
				},
			}},
			nil, nil, true)

		out := buf.String()
		for _, want := range []string{
			"- aws_security_group.legacy",
			"± aws_db_instance.primary",
			"~ aws_iam_role.eks",
			"+ aws_s3_bucket.logs",
		} {
			if !strings.Contains(out, want) {
				t.Errorf("expected output to contain %q, got:\n%s", want, out)
			}
		}
		delIdx := strings.Index(out, "- aws_security_group.legacy")
		addIdx := strings.Index(out, "+ aws_s3_bucket.logs")
		if delIdx < 0 || addIdx < 0 || delIdx > addIdx {
			t.Errorf("expected delete to render before create, got delIdx=%d addIdx=%d", delIdx, addIdx)
		}
	})

	t.Run("ListsKustomizeResourcesWithKindNamespaceName", func(t *testing.T) {
		var buf strings.Builder
		plan.Summary(&buf, nil,
			[]fluxinfra.KustomizePlan{{
				Name:  "monitoring",
				Added: 4, Removed: 2,
				Resources: []fluxinfra.ResourceChange{
					{Address: "Deployment/monitoring/grafana", Action: fluxinfra.ActionCreate},
					{Address: "ConfigMap/monitoring/grafana-config", Action: fluxinfra.ActionUpdate},
					{Address: "Service/monitoring/legacy-exporter", Action: fluxinfra.ActionDelete},
				},
			}},
			nil, true)

		out := buf.String()
		for _, want := range []string{
			"- Service/monitoring/legacy-exporter",
			"~ ConfigMap/monitoring/grafana-config",
			"+ Deployment/monitoring/grafana",
		} {
			if !strings.Contains(out, want) {
				t.Errorf("expected output to contain %q, got:\n%s", want, out)
			}
		}
	})

	t.Run("HeaderCountMatchesResourceListCountForReplaceOnly", func(t *testing.T) {
		// Regression test for the original confusion: a single replace was
		// reported as "+1 ~0 -1" (two changes from terraform's accounting)
		// but only one indented "± ..." line. The header now derives counts
		// from Resources, so it shows ±1 and lines up one-to-one with the
		// resource list.
		var buf strings.Builder
		plan.Summary(&buf,
			[]terraforminfra.TerraformComponentPlan{{
				ComponentID: "cluster", Path: "cluster/azure-aks",
				Add: 1, Destroy: 1,
				Resources: []terraforminfra.ResourceChange{
					{Address: "azurerm_federated_identity_credential.external_dns[0]", Action: terraforminfra.ActionReplace},
				},
			}},
			nil, nil, true)
		out := buf.String()
		if !strings.Contains(out, "±1") {
			t.Errorf("expected header to show ±1 for the single replace, got:\n%s", out)
		}
		if strings.Contains(out, "+1  ~0  -1") {
			t.Errorf("did not expect double-counted '+1  ~0  -1' header, got:\n%s", out)
		}
	})

	t.Run("EmitsFooterHintWhenChangesPresent", func(t *testing.T) {
		var buf strings.Builder
		plan.Summary(&buf,
			[]terraforminfra.TerraformComponentPlan{{ComponentID: "vpc", Add: 1}},
			nil, nil, true)
		out := buf.String()
		if !strings.Contains(out, "windsor plan terraform <name>") {
			t.Errorf("expected footer hint in output, got:\n%s", out)
		}
	})

	t.Run("OmitsFooterHintWhenNoChanges", func(t *testing.T) {
		var buf strings.Builder
		plan.Summary(&buf,
			[]terraforminfra.TerraformComponentPlan{{ComponentID: "vpc", NoChanges: true}},
			[]fluxinfra.KustomizePlan{{Name: "monitoring"}},
			nil, true)
		out := buf.String()
		if strings.Contains(out, "for full diffs") {
			t.Errorf("did not expect footer hint, got:\n%s", out)
		}
	})

	t.Run("EmitsFooterHintForNewComponents", func(t *testing.T) {
		var buf strings.Builder
		plan.Summary(&buf,
			[]terraforminfra.TerraformComponentPlan{{ComponentID: "vpc", IsNew: true}},
			nil, nil, true)
		out := buf.String()
		if !strings.Contains(out, "for full diffs") {
			t.Errorf("expected footer hint for new component, got:\n%s", out)
		}
	})

	t.Run("CapsLongResourceListsWithSummaryLine", func(t *testing.T) {
		resources := make([]fluxinfra.ResourceChange, 0, 25)
		for i := 0; i < 25; i++ {
			resources = append(resources, fluxinfra.ResourceChange{
				Address: fmt.Sprintf("ConfigMap/default/cm-%02d", i),
				Action:  fluxinfra.ActionCreate,
			})
		}
		var buf strings.Builder
		plan.Summary(&buf, nil,
			[]fluxinfra.KustomizePlan{{Name: "policy", Added: 25, Resources: resources}},
			nil, true)
		out := buf.String()
		if !strings.Contains(out, "… and 5 more") {
			t.Errorf("expected truncation marker, got:\n%s", out)
		}
		if strings.Contains(out, "cm-24") {
			t.Errorf("expected resources past the cap to be hidden, got:\n%s", out)
		}
	})
}

func TestRenderPlanSummaryJSON(t *testing.T) {
	t.Run("EmitsIsNewFlag", func(t *testing.T) {
		var buf strings.Builder
		err := plan.SummaryJSON(&buf, []terraforminfra.TerraformComponentPlan{
			{ComponentID: "vpc", IsNew: true},
			{ComponentID: "ec2", Add: 5},
		}, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, `"is_new": true`) {
			t.Errorf("expected is_new: true for vpc row, got %s", out)
		}
	})

	t.Run("EmitsResourcesForBothLayers", func(t *testing.T) {
		// Resource enumeration is the whole point of this output — both
		// terraform and flux entries serialize their per-resource changes
		// with the shared action vocabulary.
		var buf strings.Builder
		err := plan.SummaryJSON(&buf,
			[]terraforminfra.TerraformComponentPlan{{
				ComponentID: "cluster",
				Add:         1, Change: 1, Destroy: 1,
				Resources: []terraforminfra.ResourceChange{
					{Address: "aws_s3_bucket.logs", Action: terraforminfra.ActionCreate},
					{Address: "aws_iam_role.eks", Action: terraforminfra.ActionUpdate},
					{Address: "aws_security_group.legacy", Action: terraforminfra.ActionDelete},
				},
			}},
			[]fluxinfra.KustomizePlan{{
				Name:  "monitoring",
				Added: 5,
				Resources: []fluxinfra.ResourceChange{
					{Address: "Deployment/monitoring/grafana", Action: fluxinfra.ActionCreate},
					{Address: "ConfigMap/monitoring/grafana-config", Action: fluxinfra.ActionUpdate},
				},
			}},
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		for _, want := range []string{
			`"address": "aws_s3_bucket.logs"`,
			`"action": "create"`,
			`"address": "aws_iam_role.eks"`,
			`"action": "update"`,
			`"address": "aws_security_group.legacy"`,
			`"action": "delete"`,
			`"address": "Deployment/monitoring/grafana"`,
			`"address": "ConfigMap/monitoring/grafana-config"`,
		} {
			if !strings.Contains(out, want) {
				t.Errorf("expected JSON to contain %q, got:\n%s", want, out)
			}
		}
	})
}

func TestSummary_TerraformRow(t *testing.T) {
	t.Run("RendersNewWhenIsNew", func(t *testing.T) {
		// Given a terraform plan for a component with no state
		p := terraforminfra.TerraformComponentPlan{
			ComponentID: "vpc",
			IsNew:       true,
		}

		// When the summary renders
		got := summaryTerraformRow(t, p, "vpc")

		// Then the row reads (new)
		if got != "(new)" {
			t.Errorf("expected (new), got %q", got)
		}
	})

	t.Run("ErrorTakesPrecedenceOverIsNew", func(t *testing.T) {
		// Given a new terraform plan that also failed
		p := terraforminfra.TerraformComponentPlan{
			ComponentID: "vpc",
			IsNew:       true,
			Err:         fmt.Errorf("boom"),
		}

		// When the summary renders
		got := summaryTerraformRow(t, p, "vpc")

		// Then the row shows the error
		if !strings.Contains(got, "boom") {
			t.Errorf("expected error message to win, got %q", got)
		}
	})

	t.Run("RendersRawCountsWhenResourcesEmpty", func(t *testing.T) {
		// Given a terraform plan with summary counts and no resources
		p := terraforminfra.TerraformComponentPlan{
			ComponentID: "vpc",
			Add:         3, Change: 1, Destroy: 2,
		}

		// When the summary renders
		got := summaryTerraformRow(t, p, "vpc")

		// Then the row shows the summary counts without a replace bucket
		if !strings.Contains(got, "+3") || !strings.Contains(got, "~1") || !strings.Contains(got, "-2") {
			t.Errorf("expected +3 ~1 -2, got %q", got)
		}
		if strings.Contains(got, "±") {
			t.Errorf("did not expect ± bucket without resources, got %q", got)
		}
	})

	t.Run("DerivesCountsFromResourcesAndShowsReplaceBucket", func(t *testing.T) {
		// Given a terraform plan whose only resource change is a replace
		p := terraforminfra.TerraformComponentPlan{
			ComponentID: "cluster",
			Add:         1, Destroy: 1,
			Resources: []terraforminfra.ResourceChange{
				{Address: "aws_eks.main", Action: terraforminfra.ActionReplace},
			},
		}

		// When the summary renders
		got := summaryTerraformRow(t, p, "cluster")

		// Then the row derives its counts from the resources and shows ±1
		if !strings.Contains(got, "+0") || !strings.Contains(got, "~0") || !strings.Contains(got, "-0") {
			t.Errorf("expected zero buckets, got %q", got)
		}
		if !strings.Contains(got, "±1") {
			t.Errorf("expected ±1 for the replace, got %q", got)
		}
	})

	t.Run("OmitsReplaceBucketWhenZero", func(t *testing.T) {
		// Given a terraform plan with a create and an update
		p := terraforminfra.TerraformComponentPlan{
			ComponentID: "vpc",
			Resources: []terraforminfra.ResourceChange{
				{Address: "aws_s3.a", Action: terraforminfra.ActionCreate},
				{Address: "aws_s3.b", Action: terraforminfra.ActionUpdate},
			},
		}

		// When the summary renders
		got := summaryTerraformRow(t, p, "vpc")

		// Then the row has no replace bucket
		if strings.Contains(got, "±") {
			t.Errorf("did not expect ± bucket when no replaces, got %q", got)
		}
		if !strings.Contains(got, "+1") || !strings.Contains(got, "~1") || !strings.Contains(got, "-0") {
			t.Errorf("expected +1 ~1 -0, got %q", got)
		}
	})

	t.Run("PrefersPathAsDisplayName", func(t *testing.T) {
		// Given a terraform plan with a component ID and a path
		p := terraforminfra.TerraformComponentPlan{
			ComponentID: "cluster",
			Path:        "cluster/aws-eks",
			IsNew:       true,
		}

		// When the summary renders
		got := summaryTerraformRow(t, p, "cluster/aws-eks")

		// Then the row is named by the path
		if got != "(new)" {
			t.Errorf("expected (new), got %q", got)
		}
	})

	t.Run("FallsBackToComponentIDAsDisplayName", func(t *testing.T) {
		// Given a terraform plan with no path
		p := terraforminfra.TerraformComponentPlan{
			ComponentID: "backend",
			IsNew:       true,
		}

		// When the summary renders
		got := summaryTerraformRow(t, p, "backend")

		// Then the row is named by the component ID
		if got != "(new)" {
			t.Errorf("expected (new), got %q", got)
		}
	})
}

func TestSummary_KustomizeRow(t *testing.T) {
	t.Run("NewKustomizationRendersBareNew", func(t *testing.T) {
		// Given a new kustomization plan with added lines
		p := fluxinfra.KustomizePlan{
			Name:  "policy-base",
			IsNew: true,
			Added: 3,
		}

		// When the summary renders
		got := summaryKustomizeRow(t, p, "policy-base")

		// Then the row reads (new)
		if got != "(new)" {
			t.Errorf("expected (new), got %q", got)
		}
	})

	t.Run("DerivesCountsFromResources", func(t *testing.T) {
		// Given a kustomization plan with line counts and resources
		p := fluxinfra.KustomizePlan{
			Name:  "monitoring",
			Added: 99, Removed: 99,
			Resources: []fluxinfra.ResourceChange{
				{Address: "Deployment/x/a", Action: fluxinfra.ActionCreate},
				{Address: "ConfigMap/x/b", Action: fluxinfra.ActionUpdate},
				{Address: "Service/x/c", Action: fluxinfra.ActionDelete},
			},
		}

		// When the summary renders
		got := summaryKustomizeRow(t, p, "monitoring")

		// Then the row derives its counts from the resources
		if !strings.Contains(got, "+1") || !strings.Contains(got, "~1") || !strings.Contains(got, "-1") {
			t.Errorf("expected per-resource counts, got %q", got)
		}
		if strings.Contains(got, "lines") {
			t.Errorf("did not expect line-count format when Resources populated, got %q", got)
		}
	})
	t.Run("RendersFirstErrorLine", func(t *testing.T) {
		// Given a kustomization plan with a multi-line error
		p := fluxinfra.KustomizePlan{
			Name: "monitoring",
			Err:  fmt.Errorf("build failed\r\nsecond line"),
		}

		// When the summary renders
		got := summaryKustomizeRow(t, p, "monitoring")

		// Then the row shows only the first line of the error
		if got != "(error: build failed)" {
			t.Errorf("expected (error: build failed), got %q", got)
		}
	})

	t.Run("RendersNewWhenDegradedAndNew", func(t *testing.T) {
		// Given a degraded plan for a new kustomization
		p := fluxinfra.KustomizePlan{
			Name:     "monitoring",
			Degraded: true,
			IsNew:    true,
		}

		// When the summary renders
		got := summaryKustomizeRow(t, p, "monitoring")

		// Then the row reads (new)
		if got != "(new)" {
			t.Errorf("expected (new), got %q", got)
		}
	})

	t.Run("RendersExistingWhenDegraded", func(t *testing.T) {
		// Given a degraded plan for a deployed kustomization
		p := fluxinfra.KustomizePlan{
			Name:     "monitoring",
			Degraded: true,
			Added:    4,
		}

		// When the summary renders
		got := summaryKustomizeRow(t, p, "monitoring")

		// Then the row reads (existing)
		if got != "(existing)" {
			t.Errorf("expected (existing), got %q", got)
		}
	})

	t.Run("RendersLineCountsWithoutResources", func(t *testing.T) {
		// Given a kustomization plan with line counts and no resources
		p := fluxinfra.KustomizePlan{
			Name:    "monitoring",
			Added:   4,
			Removed: 2,
		}

		// When the summary renders
		got := summaryKustomizeRow(t, p, "monitoring")

		// Then the row shows the line counts
		if got != "+4  -2  lines" {
			t.Errorf("expected +4  -2  lines, got %q", got)
		}
	})

	t.Run("RendersNoChanges", func(t *testing.T) {
		// Given a kustomization plan with no changes
		p := fluxinfra.KustomizePlan{
			Name: "monitoring",
		}

		// When the summary renders
		got := summaryKustomizeRow(t, p, "monitoring")

		// Then the row reads (no changes)
		if got != "(no changes)" {
			t.Errorf("expected (no changes), got %q", got)
		}
	})
}

func TestSummary_Color(t *testing.T) {
	t.Run("ColorsCountsAndResourceSymbols", func(t *testing.T) {
		// Given a terraform plan with one change of each action
		tfPlans := []terraforminfra.TerraformComponentPlan{{
			ComponentID: "cluster",
			Resources: []terraforminfra.ResourceChange{
				{Address: "aws_s3.a", Action: terraforminfra.ActionCreate},
				{Address: "aws_s3.b", Action: terraforminfra.ActionUpdate},
				{Address: "aws_s3.c", Action: terraforminfra.ActionDelete},
				{Address: "aws_s3.d", Action: terraforminfra.ActionReplace},
			},
		}}

		// When the summary renders with color
		var buf strings.Builder
		plan.Summary(&buf, tfPlans, nil, nil, false)

		// Then the counts and the resource symbols carry ANSI colors
		out := buf.String()
		for _, want := range []string{"\033[32m+1\033[0m", "\033[33m~1\033[0m", "\033[31m-1\033[0m", "\033[33m±1\033[0m", "\033[32m+\033[0m aws_s3.a", "\033[33m~\033[0m aws_s3.b", "\033[31m-\033[0m aws_s3.c", "\033[33m±\033[0m aws_s3.d"} {
			if !strings.Contains(out, want) {
				t.Errorf("expected output to contain %q, got:\n%s", want, out)
			}
		}
	})

	t.Run("ColorsStatusTextAndErrors", func(t *testing.T) {
		// Given a new terraform plan, a failed kustomization plan, and a kustomization with line counts
		tfPlans := []terraforminfra.TerraformComponentPlan{{ComponentID: "vpc", IsNew: true}}
		k8sPlans := []fluxinfra.KustomizePlan{
			{Name: "broken", Err: fmt.Errorf("boom")},
			{Name: "lines", Added: 2, Removed: 1},
			{Name: "fresh", IsNew: true},
		}

		// When the summary renders with color
		var buf strings.Builder
		plan.Summary(&buf, tfPlans, k8sPlans, nil, false)

		// Then each status carries its ANSI color
		out := buf.String()
		if n := strings.Count(out, "\033[36m(new)\033[0m"); n != 2 {
			t.Errorf("expected 2 colored (new) rows, got %d in:\n%s", n, out)
		}
		for _, want := range []string{"\033[31m(error: boom)\033[0m", "\033[32m+2\033[0m  \033[31m-1\033[0m  lines"} {
			if !strings.Contains(out, want) {
				t.Errorf("expected output to contain %q, got:\n%s", want, out)
			}
		}
	})
}

func TestSummary_Footer(t *testing.T) {
	t.Run("RendersDiffHintAndCallerHints", func(t *testing.T) {
		// Given a kustomization plan with changes and a two-line hint
		k8sPlans := []fluxinfra.KustomizePlan{{Name: "monitoring", Added: 1}}
		hints := []string{"first hint\nsecond hint"}

		// When the summary renders
		var buf strings.Builder
		plan.Summary(&buf, nil, k8sPlans, hints, true)

		// Then the footer has the diff hint and each hint line
		out := buf.String()
		for _, want := range []string{"for full diffs.", "  first hint\n", "  second hint\n"} {
			if !strings.Contains(out, want) {
				t.Errorf("expected output to contain %q, got:\n%s", want, out)
			}
		}
	})

	t.Run("OmitsDiffHintWhenOnlyFailures", func(t *testing.T) {
		// Given a failed terraform plan and a degraded kustomization plan
		tfPlans := []terraforminfra.TerraformComponentPlan{{ComponentID: "vpc", Add: 1, Err: fmt.Errorf("boom")}}
		k8sPlans := []fluxinfra.KustomizePlan{{Name: "monitoring", Added: 1, Degraded: true}}

		// When the summary renders
		var buf strings.Builder
		plan.Summary(&buf, tfPlans, k8sPlans, nil, true)

		// Then the footer has no diff hint
		if strings.Contains(buf.String(), "for full diffs.") {
			t.Errorf("did not expect the diff hint, got:\n%s", buf.String())
		}
	})
}

// =============================================================================
// Test Helpers
// =============================================================================

// rowStatus returns the status text of the summary row named name, or fails the test.
func rowStatus(t *testing.T, output, name string) string {
	t.Helper()
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(line, "  "+name+" ") {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, name))
		}
	}
	t.Fatalf("expected a row named %q in output:\n%s", name, output)
	return ""
}

// summaryTerraformRow renders p with Summary and returns the status text of its row.
func summaryTerraformRow(t *testing.T, p terraforminfra.TerraformComponentPlan, name string) string {
	t.Helper()
	var buf strings.Builder
	plan.Summary(&buf, []terraforminfra.TerraformComponentPlan{p}, nil, nil, true)
	return rowStatus(t, buf.String(), name)
}

// summaryKustomizeRow renders p with Summary and returns the status text of its row.
func summaryKustomizeRow(t *testing.T, p fluxinfra.KustomizePlan, name string) string {
	t.Helper()
	var buf strings.Builder
	plan.Summary(&buf, nil, []fluxinfra.KustomizePlan{p}, nil, true)
	return rowStatus(t, buf.String(), name)
}
