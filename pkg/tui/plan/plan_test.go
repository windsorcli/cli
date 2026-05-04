package plan

import (
	"fmt"
	"strings"
	"testing"

	fluxinfra "github.com/windsorcli/cli/pkg/provisioner/flux"
	terraforminfra "github.com/windsorcli/cli/pkg/provisioner/terraform"
)

func TestFormatTerraformPlan(t *testing.T) {
	t.Run("RendersNewWhenIsNew", func(t *testing.T) {
		// IsNew with no error renders as "(new)" — plan was not run because
		// the component has no state in the configured backend.
		got := formatTerraformPlan(terraforminfra.TerraformComponentPlan{
			ComponentID: "vpc",
			IsNew:       true,
		}, true)
		if got != "(new)" {
			t.Errorf("expected (new), got %q", got)
		}
	})

	t.Run("ErrorTakesPrecedenceOverIsNew", func(t *testing.T) {
		// A non-nil Err always renders as the error — IsNew is only set when
		// classification succeeds.
		got := formatTerraformPlan(terraforminfra.TerraformComponentPlan{
			ComponentID: "vpc",
			IsNew:       true,
			Err:         fmt.Errorf("boom"),
		}, true)
		if !strings.Contains(got, "boom") {
			t.Errorf("expected error message to win, got %q", got)
		}
	})

	t.Run("RendersRawCountsWhenResourcesEmpty", func(t *testing.T) {
		// Without a Resources slice we fall back to the change_summary fields
		// (terraform's own accounting).
		got := formatTerraformPlan(terraforminfra.TerraformComponentPlan{
			ComponentID: "vpc",
			Add:         3, Change: 1, Destroy: 2,
		}, true)
		if !strings.Contains(got, "+3") || !strings.Contains(got, "~1") || !strings.Contains(got, "-2") {
			t.Errorf("expected +3 ~1 -2, got %q", got)
		}
		if strings.Contains(got, "±") {
			t.Errorf("did not expect ± bucket without resources, got %q", got)
		}
	})

	t.Run("DerivesCountsFromResourcesAndShowsReplaceBucket", func(t *testing.T) {
		// A pure replace shows in change_summary as +1 -1 (terraform's own
		// double-counting). When Resources is populated, the renderer must
		// derive counts from it and surface the replace as ±1, so the count
		// line matches the indented resource list one-to-one.
		got := formatTerraformPlan(terraforminfra.TerraformComponentPlan{
			ComponentID: "cluster",
			Add:         1, Destroy: 1,
			Resources: []terraforminfra.ResourceChange{
				{Address: "aws_eks.main", Action: terraforminfra.ActionReplace},
			},
		}, true)
		if !strings.Contains(got, "+0") || !strings.Contains(got, "~0") || !strings.Contains(got, "-0") {
			t.Errorf("expected zero buckets, got %q", got)
		}
		if !strings.Contains(got, "±1") {
			t.Errorf("expected ±1 for the replace, got %q", got)
		}
	})

	t.Run("OmitsReplaceBucketWhenZero", func(t *testing.T) {
		// Rows without replaces should look identical to the pre-replace-
		// tracking format — "+a  ~c  -d" with no trailing ±.
		got := formatTerraformPlan(terraforminfra.TerraformComponentPlan{
			ComponentID: "vpc",
			Resources: []terraforminfra.ResourceChange{
				{Address: "aws_s3.a", Action: terraforminfra.ActionCreate},
				{Address: "aws_s3.b", Action: terraforminfra.ActionUpdate},
			},
		}, true)
		if strings.Contains(got, "±") {
			t.Errorf("did not expect ± bucket when no replaces, got %q", got)
		}
		if !strings.Contains(got, "+1") || !strings.Contains(got, "~1") || !strings.Contains(got, "-0") {
			t.Errorf("expected +1 ~1 -0, got %q", got)
		}
	})
}

func TestTerraformDisplayName(t *testing.T) {
	t.Run("PrefersPathWhenSet", func(t *testing.T) {
		// Path locates the underlying module; ComponentID is the short alias.
		got := terraformDisplayName(terraforminfra.TerraformComponentPlan{
			ComponentID: "cluster",
			Path:        "cluster/aws-eks",
		})
		if got != "cluster/aws-eks" {
			t.Errorf("expected cluster/aws-eks, got %q", got)
		}
	})

	t.Run("FallsBackToComponentIDWhenPathEmpty", func(t *testing.T) {
		// Components without an explicit Path still need to render — the
		// ComponentID (Name fallback) is the next best identifier.
		got := terraformDisplayName(terraforminfra.TerraformComponentPlan{
			ComponentID: "backend",
		})
		if got != "backend" {
			t.Errorf("expected backend, got %q", got)
		}
	})
}

func TestFormatKustomizePlan(t *testing.T) {
	t.Run("NewKustomizationRendersBareNew", func(t *testing.T) {
		// Brand-new kustomizations align with terraform's bare "(new)" — the
		// resource count from `kustomize build` rarely matches what flux
		// applies after dedup, so keep both layers visually consistent.
		got := formatKustomizePlan(fluxinfra.KustomizePlan{
			Name:  "policy-base",
			IsNew: true,
			Added: 3,
		}, true)
		if got != "(new)" {
			t.Errorf("expected (new), got %q", got)
		}
	})

	t.Run("DerivesCountsFromResources", func(t *testing.T) {
		// When Resources is populated (the common case in v2.3+ flux diff or
		// kustomize build), the count line is derived per-resource so it
		// matches what's listed below.
		got := formatKustomizePlan(fluxinfra.KustomizePlan{
			Name: "monitoring",
			// Added/Removed (line counts) intentionally non-zero to confirm
			// they're ignored once Resources is set.
			Added: 99, Removed: 99,
			Resources: []fluxinfra.ResourceChange{
				{Address: "Deployment/x/a", Action: fluxinfra.ActionCreate},
				{Address: "ConfigMap/x/b", Action: fluxinfra.ActionUpdate},
				{Address: "Service/x/c", Action: fluxinfra.ActionDelete},
			},
		}, true)
		if !strings.Contains(got, "+1") || !strings.Contains(got, "~1") || !strings.Contains(got, "-1") {
			t.Errorf("expected per-resource counts, got %q", got)
		}
		if strings.Contains(got, "lines") {
			t.Errorf("did not expect line-count format when Resources populated, got %q", got)
		}
	})
}

func TestRenderPlanSummaryJSON(t *testing.T) {
	t.Run("EmitsIsNewFlag", func(t *testing.T) {
		var buf strings.Builder
		err := SummaryJSON(&buf, []terraforminfra.TerraformComponentPlan{
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
		err := SummaryJSON(&buf,
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

func TestRenderPlanSummary(t *testing.T) {
	t.Run("ListsTerraformResourcesUnderComponentSortedDestructiveFirst", func(t *testing.T) {
		// Operators read top-down — destructive changes (delete, replace)
		// deserve to surface above creates and updates.
		var buf strings.Builder
		Summary(&buf,
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
		Summary(&buf, nil,
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
		Summary(&buf,
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
		Summary(&buf,
			[]terraforminfra.TerraformComponentPlan{{ComponentID: "vpc", Add: 1}},
			nil, nil, true)
		out := buf.String()
		if !strings.Contains(out, "windsor plan terraform <name>") {
			t.Errorf("expected footer hint in output, got:\n%s", out)
		}
	})

	t.Run("OmitsFooterHintWhenNoChanges", func(t *testing.T) {
		var buf strings.Builder
		Summary(&buf,
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
		Summary(&buf,
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
		Summary(&buf, nil,
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
