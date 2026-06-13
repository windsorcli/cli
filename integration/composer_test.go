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

// TestShowBlueprint_CrdsFacetSection verifies a facet's `crds:` section composes into the
// blueprint: the referenced CRD becomes its own kustomization at kustomize/crds/<ref> with
// pruning disabled and wait enabled, and the facet's own kustomization is wired to depend on it.
func TestShowBlueprint_CrdsFacetSection(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "facet-crds")
	env = append(env, "WINDSOR_CONTEXT=default")
	stdout, stderr, err := helpers.RunCLI(dir, []string{"show", "blueprint"}, env)
	if err != nil {
		t.Fatalf("show blueprint: %v\nstderr: %s", err, stderr)
	}

	var bp struct {
		Kustomize []struct {
			Name      string   `yaml:"name"`
			Path      string   `yaml:"path"`
			Prune     *bool    `yaml:"prune"`
			Wait      *bool    `yaml:"wait"`
			DependsOn []string `yaml:"dependsOn"`
		} `yaml:"kustomize"`
	}
	if err := yaml.Unmarshal(stdout, &bp); err != nil {
		t.Fatalf("parse blueprint YAML: %v\nstdout: %s", err, stdout)
	}

	byName := map[string]int{}
	for i, k := range bp.Kustomize {
		byName[k.Name] = i
	}

	crdIdx, ok := byName["cert-manager-1.16.2"]
	if !ok {
		t.Fatalf("expected synthesized CRD kustomization 'cert-manager-1.16.2', got %+v", bp.Kustomize)
	}
	crd := bp.Kustomize[crdIdx]
	if crd.Path != "crds/cert-manager-1.16.2" {
		t.Errorf("expected CRD path 'crds/cert-manager-1.16.2', got %q", crd.Path)
	}
	if crd.Prune == nil || *crd.Prune != false {
		t.Errorf("expected CRD prune=false, got %v", crd.Prune)
	}
	if crd.Wait == nil || *crd.Wait != true {
		t.Errorf("expected CRD wait=true, got %v", crd.Wait)
	}

	installIdx, ok := byName["cert-manager"]
	if !ok {
		t.Fatalf("expected facet kustomization 'cert-manager', got %+v", bp.Kustomize)
	}
	install := bp.Kustomize[installIdx]
	if !slices.Contains(install.DependsOn, "cert-manager-1.16.2") {
		t.Errorf("expected cert-manager to depend on cert-manager-1.16.2, got %v", install.DependsOn)
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
