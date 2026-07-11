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

func TestShowValues_DefaultFixture(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "default")
	env = append(env, "WINDSOR_CONTEXT=default")
	stdout, stderr, err := helpers.RunCLI(dir, []string{"show", "values"}, env)
	if err != nil {
		t.Fatalf("show values: %v\nstderr: %s", err, stderr)
	}
	output := string(stdout)
	// Output must be parseable as YAML (comments are stripped by the parser)
	var values map[string]any
	if err := yaml.Unmarshal(stdout, &values); err != nil {
		t.Fatalf("parse values YAML: %v", err)
	}
	if values == nil {
		t.Error("expected non-nil values map")
	}
	// Schema for the default fixture defines descriptions on provider, vm, workstation fields;
	// at minimum the schema comment marker should appear if any described field is present.
	if strings.Contains(output, "provider:") && !strings.Contains(output, "#") {
		t.Errorf("expected description comments in output when schema is loaded, got:\n%s", output)
	}
}

func TestShowValues_JSONFlag(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "default")
	env = append(env, "WINDSOR_CONTEXT=default")
	stdout, stderr, err := helpers.RunCLI(dir, []string{"show", "values", "--json"}, env)
	if err != nil {
		t.Fatalf("show values --json: %v\nstderr: %s", err, stderr)
	}
	var values map[string]any
	if err := yaml.Unmarshal(stdout, &values); err != nil {
		t.Fatalf("parse values JSON: %v", err)
	}
	if values == nil {
		t.Error("expected non-nil values map")
	}
	if strings.Contains(string(stdout), "#") {
		t.Errorf("expected no comments in JSON output, got:\n%s", stdout)
	}
}

func TestShowBlueprint_DefaultFixture(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "default")
	env = append(env, "WINDSOR_CONTEXT=default")
	stdout, stderr, err := helpers.RunCLI(dir, []string{"show", "blueprint"}, env)
	if err != nil {
		t.Fatalf("show blueprint: %v\nstderr: %s", err, stderr)
	}
	if strings.Contains(string(stderr), "non-existent") || strings.Contains(string(stderr), "csi") {
		t.Errorf("stderr should not contain 'non-existent' or 'csi': %s", stderr)
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
}

func TestShowBlueprint_DefaultRendersDeferredPlaceholder(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "facet-composition")
	env = append(env, "WINDSOR_CONTEXT=default")
	stdout, stderr, err := helpers.RunCLI(dir, []string{"show", "blueprint"}, env)
	if err != nil {
		t.Fatalf("show blueprint: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(string(stdout), "<deferred>") {
		t.Fatalf("expected output to include <deferred>, got:\n%s", stdout)
	}
	if strings.Contains(string(stdout), "${terraform_output(") {
		t.Fatalf("expected output to hide deferred expression text, got:\n%s", stdout)
	}
}

func TestShowKustomization_PluralAliasBehavesIdentically(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "facet-tiers")
	env = append(env, "WINDSOR_CONTEXT=default")
	stdout, stderr, err := helpers.RunCLI(dir, []string{"show", "kustomizations"}, env)
	if err != nil {
		t.Fatalf("show kustomizations: %v\nstderr: %s", err, stderr)
	}
	names := strings.Fields(string(stdout))
	if !slices.Contains(names, "dns") || !slices.Contains(names, "cert-manager-install") {
		t.Errorf("expected plural alias listing to contain dns and cert-manager-install, got %v", names)
	}
}

func TestShowKustomization_NoArgsListsPlainAndFluxSystemTierNames(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "facet-tiers")
	env = append(env, "WINDSOR_CONTEXT=default")
	stdout, stderr, err := helpers.RunCLI(dir, []string{"show", "kustomization"}, env)
	if err != nil {
		t.Fatalf("show kustomization: %v\nstderr: %s", err, stderr)
	}
	names := strings.Fields(string(stdout))
	for _, want := range []string{"dns", "cert-manager-install", "cert-manager-resources", "cert-manager-resources-extra"} {
		if !slices.Contains(names, want) {
			t.Errorf("expected listing to contain %q, got %v", want, names)
		}
	}
}

func TestShowKustomization_TierNameRendersCompiledKustomization(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "facet-tiers")
	env = append(env, "WINDSOR_CONTEXT=default")
	stdout, stderr, err := helpers.RunCLI(dir, []string{"show", "kustomization", "cert-manager-install"}, env)
	if err != nil {
		t.Fatalf("show kustomization cert-manager-install: %v\nstderr: %s", err, stderr)
	}
	var k struct {
		Metadata struct {
			Name string `yaml:"name"`
		} `yaml:"metadata"`
		Spec struct {
			Path string `yaml:"path"`
		} `yaml:"spec"`
	}
	if err := yaml.Unmarshal(stdout, &k); err != nil {
		t.Fatalf("parse kustomization YAML: %v\nstdout: %s", err, stdout)
	}
	if k.Metadata.Name != "cert-manager-install" {
		t.Errorf("expected metadata.name cert-manager-install, got %q", k.Metadata.Name)
	}
	if k.Spec.Path != "kustomize/pki/cert-manager/install" {
		t.Errorf("expected spec.path kustomize/pki/cert-manager/install, got %q", k.Spec.Path)
	}
}

func TestShowKustomization_PathDefaultsToNameWhenUnset(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "facet-tiers")
	env = append(env, "WINDSOR_CONTEXT=default")
	stdout, stderr, err := helpers.RunCLI(dir, []string{"show", "kustomization", "dns"}, env)
	if err != nil {
		t.Fatalf("show kustomization dns: %v\nstderr: %s", err, stderr)
	}
	var k struct {
		Spec struct {
			Path string `yaml:"path"`
		} `yaml:"spec"`
	}
	if err := yaml.Unmarshal(stdout, &k); err != nil {
		t.Fatalf("parse kustomization YAML: %v\nstdout: %s", err, stdout)
	}
	if k.Spec.Path != "kustomize/dns" {
		t.Errorf("expected spec.path kustomize/dns, got %q", k.Spec.Path)
	}
}

func TestShowKustomization_SystemNameReturnsTierListError(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "facet-tiers")
	env = append(env, "WINDSOR_CONTEXT=default")
	_, stderr, err := helpers.RunCLI(dir, []string{"show", "kustomization", "cert-manager"}, env)
	if err == nil {
		t.Fatal("expected failure but command succeeded")
	}
	if !strings.Contains(string(stderr), "cert-manager-install") {
		t.Errorf("expected stderr to list tier names, got: %s", stderr)
	}
}

func TestShowBlueprint_RawPreservesDeferredExpressions(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "facet-composition")
	env = append(env, "WINDSOR_CONTEXT=default")
	stdout, stderr, err := helpers.RunCLI(dir, []string{"show", "blueprint", "--raw"}, env)
	if err != nil {
		t.Fatalf("show blueprint --raw: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(string(stdout), `terraform_output("compute", "controlplanes")`) {
		t.Fatalf("expected raw output to include deferred expression text, got:\n%s", stdout)
	}
	if strings.Contains(string(stdout), "<deferred>") {
		t.Fatalf("expected raw output to not include <deferred>, got:\n%s", stdout)
	}
}
