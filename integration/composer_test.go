//go:build integration
// +build integration

package integration

import (
	"slices"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/windsorcli/cli/integration/helpers"
)

func TestShowBlueprint_FacetCompositionFixture(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "facet-composition")
	env = append(env, "WINDSOR_CONTEXT=default")
	stdout, stderr, err := helpers.RunCLI(dir, []string{"show", "blueprint"}, env)
	if err != nil {
		t.Fatalf("show blueprint: %v\nstderr: %s", err, stderr)
	}
	var bp map[string]any
	if err := yaml.Unmarshal(stdout, &bp); err != nil {
		t.Fatalf("parse blueprint YAML: %v", err)
	}
	if bp["kind"] != "Blueprint" {
		t.Errorf("expected kind Blueprint, got %v", bp["kind"])
	}
	metadata, _ := bp["metadata"].(map[string]any)
	if metadata == nil {
		t.Error("expected metadata in blueprint")
	}
	if _, hasKustomize := bp["kustomize"]; !hasKustomize {
		t.Error("expected kustomize key in blueprint")
	}
}

func TestWindsorTest_FacetCompositionFixture(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "facet-composition")
	env = append(env, "WINDSOR_CONTEXT=test")
	stdout, stderr, err := helpers.RunCLI(dir, []string{"test"}, env)
	if err != nil {
		t.Fatalf("windsor test: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	out := string(stdout) + string(stderr)
	if !strings.Contains(out, "PASS") && !strings.Contains(out, "✓") {
		t.Errorf("expected PASS or ✓ in output: %s", out)
	}
}

func TestWindsorTest_DerivedConfigFixture(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "derived-config")
	env = append(env, "WINDSOR_CONTEXT=test")
	stdout, stderr, err := helpers.RunCLI(dir, []string{"test"}, env)
	if err != nil {
		t.Fatalf("windsor test: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	out := string(stdout) + string(stderr)
	if !strings.Contains(out, "PASS") && !strings.Contains(out, "✓") {
		t.Errorf("expected PASS or ✓ in output: %s", out)
	}
}

// TestShowBlueprint_CrdsFacetSection verifies a facet's `crds:` declaration composes into the
// blueprint's first-class `crds:` section: the reference becomes a kustomization at crds/<ref>
// with pruning disabled and wait enabled, separate from the `kustomize:` layer.
func TestShowBlueprint_CrdsFacetSection(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "facet-crds")
	env = append(env, "WINDSOR_CONTEXT=default")
	stdout, stderr, err := helpers.RunCLI(dir, []string{"show", "blueprint"}, env)
	if err != nil {
		t.Fatalf("show blueprint: %v\nstderr: %s", err, stderr)
	}

	var bp struct {
		Crds []struct {
			Name  string `yaml:"name"`
			Path  string `yaml:"path"`
			Prune *bool  `yaml:"prune"`
			Wait  *bool  `yaml:"wait"`
		} `yaml:"crds"`
		Kustomize []struct {
			Name string `yaml:"name"`
		} `yaml:"kustomize"`
	}
	if err := yaml.Unmarshal(stdout, &bp); err != nil {
		t.Fatalf("parse blueprint YAML: %v\nstdout: %s", err, stdout)
	}

	var crd *struct {
		Name  string `yaml:"name"`
		Path  string `yaml:"path"`
		Prune *bool  `yaml:"prune"`
		Wait  *bool  `yaml:"wait"`
	}
	for i := range bp.Crds {
		if bp.Crds[i].Name == "cert-manager-1.16.2" {
			crd = &bp.Crds[i]
		}
	}
	if crd == nil {
		t.Fatalf("expected cert-manager-1.16.2 in the crds: section, got %+v", bp.Crds)
	}
	if crd.Path != "crds/cert-manager-1.16.2" {
		t.Errorf("expected CRD path 'crds/cert-manager-1.16.2', got %q", crd.Path)
	}
	if crd.Prune == nil || *crd.Prune != false {
		t.Errorf("expected CRD prune=false, got %v", crd.Prune)
	}
	if crd.Wait == nil || *crd.Wait != true {
		t.Errorf("expected CRD wait=true, got %v", crd.Wait)
	}

	// The CRD lives in crds:, not folded into kustomize:.
	for _, k := range bp.Kustomize {
		if k.Name == "cert-manager-1.16.2" {
			t.Errorf("did not expect the CRD in the kustomize: section, got %+v", bp.Kustomize)
		}
	}
}

// TestShowBlueprint_CrdsDriverSelection verifies a single facet selecting different CRDs per
// driver via inline `${...}` expressions: only the active driver's CRD lands in the crds: section,
// while the other driver's CRD never appears.
func TestShowBlueprint_CrdsDriverSelection(t *testing.T) {
	t.Parallel()
	cases := []struct{ context, want, absent string }{
		{"cilium", "gateway-api-1.5.1", "envoy-gateway-1.7.1"},
		{"envoy", "envoy-gateway-1.7.1", "gateway-api-1.5.1"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.context, func(t *testing.T) {
			t.Parallel()
			dir, env := helpers.PrepareFixture(t, "facet-crds")
			env = append(env, "WINDSOR_CONTEXT="+tc.context)
			stdout, stderr, err := helpers.RunCLI(dir, []string{"show", "blueprint"}, env)
			if err != nil {
				t.Fatalf("show blueprint: %v\nstderr: %s", err, stderr)
			}

			var bp struct {
				Crds []struct {
					Name string `yaml:"name"`
				} `yaml:"crds"`
			}
			if err := yaml.Unmarshal(stdout, &bp); err != nil {
				t.Fatalf("parse blueprint YAML: %v\nstdout: %s", err, stdout)
			}

			foundWant := false
			for _, k := range bp.Crds {
				if k.Name == tc.want {
					foundWant = true
				}
				if k.Name == tc.absent {
					t.Errorf("did not expect CRD %q for context %s, got %+v", tc.absent, tc.context, bp.Crds)
				}
			}
			if !foundWant {
				t.Errorf("expected CRD %q in crds: for context %s, got %+v", tc.want, tc.context, bp.Crds)
			}
		})
	}
}

// TestShowBlueprint_InstallResourcesTiers verifies a tiered facet entry expands into separate
// install and resources kustomizations sharing the entry's path, and that a consumer depending on
// the vendor by its bare name resolves to the install tier.
func TestShowBlueprint_InstallResourcesTiers(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "facet-tiers")
	env = append(env, "WINDSOR_CONTEXT=default")
	stdout, stderr, err := helpers.RunCLI(dir, []string{"show", "blueprint"}, env)
	if err != nil {
		t.Fatalf("show blueprint: %v\nstderr: %s", err, stderr)
	}

	var bp struct {
		Kustomize []struct {
			Name       string   `yaml:"name"`
			Path       string   `yaml:"path"`
			Components []string `yaml:"components"`
			DependsOn  []string `yaml:"dependsOn"`
		} `yaml:"kustomize"`
	}
	if err := yaml.Unmarshal(stdout, &bp); err != nil {
		t.Fatalf("parse blueprint YAML: %v\nstdout: %s", err, stdout)
	}
	byName := map[string]struct {
		path       string
		components []string
		dependsOn  []string
	}{}
	for _, k := range bp.Kustomize {
		byName[k.Name] = struct {
			path       string
			components []string
			dependsOn  []string
		}{k.Path, k.Components, k.DependsOn}
	}

	install, ok := byName["cert-manager-install"]
	if !ok {
		t.Fatalf("expected cert-manager-install, got %+v", bp.Kustomize)
	}
	res, ok := byName["cert-manager-resources"]
	if !ok {
		t.Fatalf("expected cert-manager-resources, got %+v", bp.Kustomize)
	}
	if install.path != "pki/cert-manager" || res.path != "pki/cert-manager" {
		t.Errorf("expected both tiers at pki/cert-manager, got install=%q resources=%q", install.path, res.path)
	}
	if !slices.Contains(install.components, "helm-release") {
		t.Errorf("expected install components [helm-release], got %v", install.components)
	}
	if !slices.Contains(res.components, "private-issuer/ca") {
		t.Errorf("expected resources components [private-issuer/ca], got %v", res.components)
	}
	if !slices.Contains(res.dependsOn, "cert-manager-install") {
		t.Errorf("expected resources to depend on cert-manager-install, got %v", res.dependsOn)
	}

	dns, ok := byName["dns"]
	if !ok {
		t.Fatalf("expected dns, got %+v", bp.Kustomize)
	}
	if !slices.Contains(dns.dependsOn, "cert-manager-install") {
		t.Errorf("expected dns bare-name dependency to resolve to cert-manager-install, got %v", dns.dependsOn)
	}
}

// TestShowBlueprint_FacetRequires_Satisfied verifies the success path: when every required
// path resolves to a present, non-empty value, ProcessFacets returns no error and the
// blueprint renders normally.
func TestShowBlueprint_FacetRequires_Satisfied(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "facet-requires")
	env = append(env, "WINDSOR_CONTEXT=ok")
	stdout, stderr, err := helpers.RunCLI(dir, []string{"show", "blueprint"}, env)
	if err != nil {
		t.Fatalf("show blueprint: %v\nstderr: %s", err, stderr)
	}
	var bp map[string]any
	if err := yaml.Unmarshal(stdout, &bp); err != nil {
		t.Fatalf("parse blueprint YAML: %v\nstdout: %s", err, stdout)
	}
	if bp["kind"] != "Blueprint" {
		t.Errorf("expected kind Blueprint, got %v", bp["kind"])
	}
}

// TestShowBlueprint_FacetRequires_MissingProducesAggregatedError verifies the failure path:
// when required paths are missing, the command fails and stderr contains the aggregated error
// — every missing path, the block-level message, and the count summary — grouped under the
// effective condition heading rather than per-facet.
func TestShowBlueprint_FacetRequires_MissingProducesAggregatedError(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "facet-requires")
	env = append(env, "WINDSOR_CONTEXT=missing")
	_, stderr, err := helpers.RunCLI(dir, []string{"show", "blueprint"}, env)
	if err == nil {
		t.Fatalf("expected show blueprint to fail with missing requirements, got nil error\nstderr: %s", stderr)
	}
	out := string(stderr)
	if !strings.Contains(out, "the following required values are not set in values.yaml:") {
		t.Errorf("expected aggregated error lead, got: %s", out)
	}
	for _, path := range []string{"- dns.domain", "- aws.region", "- aws.account_id"} {
		if !strings.Contains(out, path) {
			t.Errorf("expected stderr to list %q, got: %s", path, out)
		}
	}
	if !strings.Contains(out, "AWS platform needs region and account_id set.") {
		t.Errorf("expected block message in stderr, got: %s", out)
	}
	if !strings.Contains(out, "\nNotes:\n") {
		t.Errorf("expected Notes section in stderr, got: %s", out)
	}
	if strings.Contains(out, "Because") || strings.Contains(out, "platform ?? '')") {
		t.Errorf("expected no condition expression in output, got: %s", out)
	}
	for _, frame := range []string{"failed to load blueprint data", "failed to compose blueprint", "failed to process facets for"} {
		if strings.Contains(out, frame) {
			t.Errorf("expected wrapped chain frame %q to be absent (RequirementsError should be passed through unwrapped), got: %s", frame, out)
		}
	}
}

// TestShowBlueprint_FacetComponentRequires_Satisfied verifies the happy path for per-component
// requires: when the component's When is true and the required inputs are present, show blueprint
// succeeds and emits a Blueprint document.
func TestShowBlueprint_FacetComponentRequires_Satisfied(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "facet-component-requires")
	env = append(env, "WINDSOR_CONTEXT=ok")
	stdout, stderr, err := helpers.RunCLI(dir, []string{"show", "blueprint"}, env)
	if err != nil {
		t.Fatalf("show blueprint: %v\nstderr: %s", err, stderr)
	}
	var bp map[string]any
	if err := yaml.Unmarshal(stdout, &bp); err != nil {
		t.Fatalf("parse blueprint YAML: %v\nstdout: %s", err, stdout)
	}
	if bp["kind"] != "Blueprint" {
		t.Errorf("expected kind Blueprint, got %v", bp["kind"])
	}
}

// TestShowBlueprint_FacetComponentRequires_MissingProducesComposedConditionError verifies the
// failure path: when a per-component required path is missing, the error groups the miss under the
// AND of facet.When and component.When, not just the facet condition.
func TestShowBlueprint_FacetComponentRequires_MissingProducesComposedConditionError(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "facet-component-requires")
	env = append(env, "WINDSOR_CONTEXT=missing")
	_, stderr, err := helpers.RunCLI(dir, []string{"show", "blueprint"}, env)
	if err == nil {
		t.Fatalf("expected show blueprint to fail with missing component requirements, got nil error\nstderr: %s", stderr)
	}
	out := string(stderr)
	if !strings.Contains(out, "the following required values are not set in values.yaml:") {
		t.Errorf("expected aggregated error lead, got: %s", out)
	}
	if !strings.Contains(out, "- aws.cilium_role_arn") {
		t.Errorf("expected stderr to list aws.cilium_role_arn as a bullet, got: %s", out)
	}
	if !strings.Contains(out, "The cilium terraform component needs an IAM role ARN.") {
		t.Errorf("expected block message in stderr, got: %s", out)
	}
	if strings.Contains(out, "Because") || strings.Contains(out, "cni.driver") {
		t.Errorf("expected no condition expression in output, got: %s", out)
	}
	for _, frame := range []string{"failed to load blueprint data", "failed to compose blueprint", "failed to process facets for"} {
		if strings.Contains(out, frame) {
			t.Errorf("expected wrapped chain frame %q to be absent (RequirementsError should be passed through unwrapped), got: %s", frame, out)
		}
	}
}

// TestShowBlueprint_FacetComponentRequires_ComponentExcludedSkipsRequires verifies that a
// per-component require does not fire when the component's own When evaluates to false. The
// fixture sets cni.driver to a non-cilium value, so the cilium terraform component is excluded
// and its missing aws.cilium_role_arn is not surfaced.
func TestShowBlueprint_FacetComponentRequires_ComponentExcludedSkipsRequires(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "facet-component-requires")
	env = append(env, "WINDSOR_CONTEXT=skipped")
	stdout, stderr, err := helpers.RunCLI(dir, []string{"show", "blueprint"}, env)
	if err != nil {
		t.Fatalf("show blueprint: %v\nstderr: %s", err, stderr)
	}
	var bp map[string]any
	if err := yaml.Unmarshal(stdout, &bp); err != nil {
		t.Fatalf("parse blueprint YAML: %v\nstdout: %s", err, stdout)
	}
	if bp["kind"] != "Blueprint" {
		t.Errorf("expected kind Blueprint, got %v", bp["kind"])
	}
}
