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
// blueprint's first-class `crds:` section (a flat scalar list, not kustomization objects), and that
// the stack's root depends on the synthesized "crds" layer via the barrier.
func TestShowBlueprint_CrdsFacetSection(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "facet-crds")
	env = append(env, "WINDSOR_CONTEXT=default")
	stdout, stderr, err := helpers.RunCLI(dir, []string{"show", "blueprint"}, env)
	if err != nil {
		t.Fatalf("show blueprint: %v\nstderr: %s", err, stderr)
	}

	var bp struct {
		Crds      []string `yaml:"crds"`
		Kustomize []struct {
			Name      string   `yaml:"name"`
			DependsOn []string `yaml:"dependsOn"`
		} `yaml:"kustomize"`
	}
	if err := yaml.Unmarshal(stdout, &bp); err != nil {
		t.Fatalf("parse blueprint YAML: %v\nstdout: %s", err, stdout)
	}

	// crds: is a flat scalar list carrying the reference (the fixture is purely local)
	if !slices.Contains(bp.Crds, "cert-manager-1.16.2") {
		t.Fatalf("expected cert-manager-1.16.2 in the crds: list, got %+v", bp.Crds)
	}

	// The CRD layer is not folded into kustomize: — and the stack's root depends on "crds" (the
	// barrier), so Flux orders the CRD layer first without the provisioner waiting. The fixture is
	// purely local, so the template source collapses to the default and the layer keeps the bare name.
	var certManager *struct {
		Name      string   `yaml:"name"`
		DependsOn []string `yaml:"dependsOn"`
	}
	for i := range bp.Kustomize {
		if strings.HasPrefix(bp.Kustomize[i].Name, "crds") || bp.Kustomize[i].Name == "cert-manager-1.16.2" {
			t.Errorf("did not expect the CRD layer in the kustomize: section, got %+v", bp.Kustomize)
		}
		if bp.Kustomize[i].Name == "cert-manager" {
			certManager = &bp.Kustomize[i]
		}
	}
	if certManager == nil {
		t.Fatalf("expected cert-manager in the kustomize: section, got %+v", bp.Kustomize)
	}
	if !slices.Contains(certManager.DependsOn, "crds") {
		t.Errorf("expected cert-manager (a root) to depend on the crds layer, got %v", certManager.DependsOn)
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
				Crds      []string `yaml:"crds"`
				Kustomize []struct {
					Name      string   `yaml:"name"`
					DependsOn []string `yaml:"dependsOn"`
				} `yaml:"kustomize"`
			}
			if err := yaml.Unmarshal(stdout, &bp); err != nil {
				t.Fatalf("parse blueprint YAML: %v\nstdout: %s", err, stdout)
			}

			// Only the active driver's CRD reference is present in the crds: list
			if !slices.Contains(bp.Crds, tc.want) {
				t.Errorf("expected CRD %q in crds: for context %s, got %+v", tc.want, tc.context, bp.Crds)
			}
			if slices.Contains(bp.Crds, tc.absent) {
				t.Errorf("did not expect CRD %q for context %s, got %+v", tc.absent, tc.context, bp.Crds)
			}

			// The gateway root is wired to the crds layer via the barrier
			var gateway *struct {
				Name      string   `yaml:"name"`
				DependsOn []string `yaml:"dependsOn"`
			}
			for i := range bp.Kustomize {
				if bp.Kustomize[i].Name == "gateway" {
					gateway = &bp.Kustomize[i]
				}
			}
			if gateway == nil {
				t.Fatalf("expected gateway in the kustomize: section for context %s, got %+v", tc.context, bp.Kustomize)
			}
			if !slices.Contains(gateway.DependsOn, "crds") {
				t.Errorf("expected gateway wired to the crds layer via the barrier, got %v", gateway.DependsOn)
			}
		})
	}
}

// TestShowBlueprint_InstallResourcesTiers verifies that flux: system entries appear in the
// composed blueprint's flux: section (not kustomize:), that their tiers have correct paths and
// components, that the implicit install→resources edge is encoded in the flux: structure, and
// that a plain kustomize: consumer depending on the system by its bare name resolves to its
// install tier.
func TestShowBlueprint_InstallResourcesTiers(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "facet-tiers")
	env = append(env, "WINDSOR_CONTEXT=default")
	stdout, stderr, err := helpers.RunCLI(dir, []string{"show", "blueprint"}, env)
	if err != nil {
		t.Fatalf("show blueprint: %v\nstderr: %s", err, stderr)
	}

	var bp struct {
		Flux []struct {
			Name    string `yaml:"name"`
			Path    string `yaml:"path"`
			Install struct {
				Components []string `yaml:"components"`
			} `yaml:"install"`
			Resources []struct {
				Components []string `yaml:"components"`
			} `yaml:"resources"`
		} `yaml:"flux"`
		Kustomize []struct {
			Name      string   `yaml:"name"`
			DependsOn []string `yaml:"dependsOn"`
		} `yaml:"kustomize"`
	}
	if err := yaml.Unmarshal(stdout, &bp); err != nil {
		t.Fatalf("parse blueprint YAML: %v\nstdout: %s", err, stdout)
	}

	// cert-manager must be in flux:, not kustomize:.
	var certMgr *struct {
		Name    string `yaml:"name"`
		Path    string `yaml:"path"`
		Install struct {
			Components []string `yaml:"components"`
		} `yaml:"install"`
		Resources []struct {
			Components []string `yaml:"components"`
		} `yaml:"resources"`
	}
	for i := range bp.Flux {
		if bp.Flux[i].Name == "cert-manager" {
			certMgr = &bp.Flux[i]
			break
		}
	}
	if certMgr == nil {
		t.Fatalf("expected cert-manager in flux:, got flux=%+v", bp.Flux)
	}
	if certMgr.Path != "pki/cert-manager" {
		t.Errorf("expected path pki/cert-manager, got %q", certMgr.Path)
	}
	if !slices.Contains(certMgr.Install.Components, "helm-release") {
		t.Errorf("expected install components [helm-release], got %v", certMgr.Install.Components)
	}
	if len(certMgr.Resources) == 0 || !slices.Contains(certMgr.Resources[0].Components, "private-issuer/ca") {
		t.Errorf("expected resources[0] components [private-issuer/ca], got %v", certMgr.Resources)
	}

	// cert-manager must NOT appear under kustomize:.
	for _, k := range bp.Kustomize {
		if k.Name == "cert-manager-install" || k.Name == "cert-manager-resources" {
			t.Errorf("flux system tier %q must not appear under kustomize:", k.Name)
		}
	}

	// Plain kustomize: consumer's bare-name dependency must resolve to the install tier.
	var dns *struct {
		Name      string   `yaml:"name"`
		DependsOn []string `yaml:"dependsOn"`
	}
	for i := range bp.Kustomize {
		if bp.Kustomize[i].Name == "dns" {
			dns = &bp.Kustomize[i]
			break
		}
	}
	if dns == nil {
		t.Fatalf("expected dns in kustomize:, got %+v", bp.Kustomize)
	}
	if !slices.Contains(dns.DependsOn, "cert-manager-install") {
		t.Errorf("expected dns bare-name dependency to resolve to cert-manager-install, got %v", dns.DependsOn)
	}
}

// TestShowBlueprint_FluxSystemFieldRemoval verifies that a facet's "strategy: remove" flux: entry
// subtracts only the named dependsOn/install.components/resources[].name fields from a flux system
// already declared in the base blueprint.yaml, leaving the rest of the system (and any resources
// variant not named in the removal) intact -- Blueprint.RemoveFluxSystem's field-level semantics,
// matching how kustomize:'s "remove" strategy already behaves, rather than deleting the system.
func TestShowBlueprint_FluxSystemFieldRemoval(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "facet-tiers-remove")
	env = append(env, "WINDSOR_CONTEXT=default")
	stdout, stderr, err := helpers.RunCLI(dir, []string{"show", "blueprint"}, env)
	if err != nil {
		t.Fatalf("show blueprint: %v\nstderr: %s", err, stderr)
	}

	var bp struct {
		Flux []struct {
			Name      string   `yaml:"name"`
			DependsOn []string `yaml:"dependsOn"`
			Install   struct {
				Components []string `yaml:"components"`
			} `yaml:"install"`
			Resources []struct {
				Name       string   `yaml:"name"`
				Components []string `yaml:"components"`
			} `yaml:"resources"`
		} `yaml:"flux"`
	}
	if err := yaml.Unmarshal(stdout, &bp); err != nil {
		t.Fatalf("parse blueprint YAML: %v\nstdout: %s", err, stdout)
	}

	var gateway *struct {
		Name      string   `yaml:"name"`
		DependsOn []string `yaml:"dependsOn"`
		Install   struct {
			Components []string `yaml:"components"`
		} `yaml:"install"`
		Resources []struct {
			Name       string   `yaml:"name"`
			Components []string `yaml:"components"`
		} `yaml:"resources"`
	}
	for i := range bp.Flux {
		if bp.Flux[i].Name == "gateway" {
			gateway = &bp.Flux[i]
			break
		}
	}
	if gateway == nil {
		t.Fatalf("expected gateway to survive field-level removal, got flux=%+v", bp.Flux)
	}

	if slices.Contains(gateway.DependsOn, "pki-install") {
		t.Errorf("expected pki-install to be removed from dependsOn, got %v", gateway.DependsOn)
	}

	if slices.Contains(gateway.Install.Components, "envoy-metrics") {
		t.Errorf("expected envoy-metrics to be removed from install components, got %v", gateway.Install.Components)
	}
	if !slices.Contains(gateway.Install.Components, "envoy") {
		t.Errorf("expected envoy to remain in install components, got %v", gateway.Install.Components)
	}

	if len(gateway.Resources) != 1 || gateway.Resources[0].Name != "external" {
		t.Errorf("expected only the 'external' resources variant to remain, got %+v", gateway.Resources)
	}
}

// TestShowBlueprint_FluxSystemCrossFacetMerge verifies that two facets contributing to the same
// flux: system name at different ordinals still merge under the default "merge" strategy instead
// of the higher-ordinal facet wholesale-replacing the lower-ordinal one's install/resources: pki's
// cert-manager install/resources (ordinal 0) and addon-cert-manager-extra's additional install
// component and resources variant (ordinal 400, from the "addon-" filename prefix) must both
// survive in the composed blueprint.
func TestShowBlueprint_FluxSystemCrossFacetMerge(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "facet-tiers")
	env = append(env, "WINDSOR_CONTEXT=default")
	stdout, stderr, err := helpers.RunCLI(dir, []string{"show", "blueprint"}, env)
	if err != nil {
		t.Fatalf("show blueprint: %v\nstderr: %s", err, stderr)
	}

	var bp struct {
		Flux []struct {
			Name    string `yaml:"name"`
			Install struct {
				Components []string `yaml:"components"`
			} `yaml:"install"`
			Resources []struct {
				Name       string   `yaml:"name"`
				Components []string `yaml:"components"`
			} `yaml:"resources"`
		} `yaml:"flux"`
	}
	if err := yaml.Unmarshal(stdout, &bp); err != nil {
		t.Fatalf("parse blueprint YAML: %v\nstdout: %s", err, stdout)
	}

	var certMgr *struct {
		Name    string `yaml:"name"`
		Install struct {
			Components []string `yaml:"components"`
		} `yaml:"install"`
		Resources []struct {
			Name       string   `yaml:"name"`
			Components []string `yaml:"components"`
		} `yaml:"resources"`
	}
	for i := range bp.Flux {
		if bp.Flux[i].Name == "cert-manager" {
			certMgr = &bp.Flux[i]
			break
		}
	}
	if certMgr == nil {
		t.Fatalf("expected cert-manager in flux:, got flux=%+v", bp.Flux)
	}

	if !slices.Contains(certMgr.Install.Components, "helm-release") || !slices.Contains(certMgr.Install.Components, "helm-release-extra-metrics") {
		t.Errorf("expected install components to union across facets, got %v", certMgr.Install.Components)
	}

	var hasDefaultVariant, hasExtraVariant bool
	for _, r := range certMgr.Resources {
		if r.Name == "" && slices.Contains(r.Components, "private-issuer/ca") {
			hasDefaultVariant = true
		}
		if r.Name == "extra" && slices.Contains(r.Components, "public-issuer") {
			hasExtraVariant = true
		}
	}
	if !hasDefaultVariant {
		t.Errorf("expected the lower-ordinal facet's unnamed resources variant to survive, got %+v", certMgr.Resources)
	}
	if !hasExtraVariant {
		t.Errorf("expected the higher-ordinal facet's resources variant to be added rather than replace the system, got %+v", certMgr.Resources)
	}
}

// TestShowBlueprint_FluxSystemSameOrdinalVariantMerge verifies the original review-flagged
// scenario: two facets at the SAME ordinal each contributing a resources variant under the same
// key (both unnamed, so both would compile to "cert-manager-resources") merge into a single
// variant instead of surviving as two colliding entries. pki's and pki-extra's unnamed variants
// (both ordinal 0, since neither filename matches an ordinal-prefix rule) must union into one
// resources entry carrying both facets' components, not appear as two separate list entries.
func TestShowBlueprint_FluxSystemSameOrdinalVariantMerge(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "facet-tiers")
	env = append(env, "WINDSOR_CONTEXT=default")
	stdout, stderr, err := helpers.RunCLI(dir, []string{"show", "blueprint"}, env)
	if err != nil {
		t.Fatalf("show blueprint: %v\nstderr: %s", err, stderr)
	}

	var bp struct {
		Flux []struct {
			Name      string `yaml:"name"`
			Resources []struct {
				Name       string   `yaml:"name"`
				Components []string `yaml:"components"`
			} `yaml:"resources"`
		} `yaml:"flux"`
	}
	if err := yaml.Unmarshal(stdout, &bp); err != nil {
		t.Fatalf("parse blueprint YAML: %v\nstdout: %s", err, stdout)
	}

	var certMgr *struct {
		Name      string `yaml:"name"`
		Resources []struct {
			Name       string   `yaml:"name"`
			Components []string `yaml:"components"`
		} `yaml:"resources"`
	}
	for i := range bp.Flux {
		if bp.Flux[i].Name == "cert-manager" {
			certMgr = &bp.Flux[i]
			break
		}
	}
	if certMgr == nil {
		t.Fatalf("expected cert-manager in flux:, got flux=%+v", bp.Flux)
	}

	var unnamedCount int
	var unnamedComponents []string
	for _, r := range certMgr.Resources {
		if r.Name == "" {
			unnamedCount++
			unnamedComponents = r.Components
		}
	}
	if unnamedCount != 1 {
		t.Fatalf("expected pki's and pki-extra's same-key unnamed variant to merge into a single resources entry, got %d unnamed entries in %+v", unnamedCount, certMgr.Resources)
	}
	if !slices.Contains(unnamedComponents, "private-issuer/ca") || !slices.Contains(unnamedComponents, "private-issuer/wildcard") {
		t.Errorf("expected the merged unnamed variant to union both facets' components, got %v", unnamedComponents)
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
