//go:build integration
// +build integration

package integration

import (
	"os"
	"path/filepath"
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

// TestShowBlueprint_SourceBackendOverridesSavedContextBackend confirms that init does not save
// the source backend into the context blueprint.yaml. It also confirms that a backend saved there
// by an older CLI does not override the backend the source declares, and that Windsor warns
// the operator to remove it.
func TestShowBlueprint_SourceBackendOverridesSavedContextBackend(t *testing.T) {
	t.Parallel()
	dir, env := helpers.CopyFixtureOnly(t, "backend-first")
	helpers.MarkAsGitRepo(t, dir)
	if _, stderr, err := helpers.RunCLI(dir, []string{"init", "local"}, env); err != nil {
		t.Fatalf("init local: %v\nstderr: %s", err, stderr)
	}
	env = append(env, "WINDSOR_CONTEXT=local")

	contextPath := filepath.Join(dir, "contexts", "local", "blueprint.yaml")
	saved, err := os.ReadFile(contextPath)
	if err != nil {
		t.Fatalf("read context blueprint: %v", err)
	}
	if strings.Contains(string(saved), "backend:") {
		t.Fatalf("expected init not to save the backend, got:\n%s", saved)
	}
	legacy := append(saved, []byte("backend: \"null\"\n")...)
	if err := os.WriteFile(contextPath, legacy, 0o644); err != nil {
		t.Fatalf("write context blueprint: %v", err)
	}

	stdout, stderr, err := helpers.RunCLI(dir, []string{"show", "blueprint"}, env)
	if err != nil {
		t.Fatalf("show blueprint: %v\nstderr: %s", err, stderr)
	}
	var bp map[string]any
	if err := yaml.Unmarshal(stdout, &bp); err != nil {
		t.Fatalf("parse blueprint YAML: %v", err)
	}
	if bp["backend"] != "backend" {
		t.Errorf("expected backend %q from the source, got %v", "backend", bp["backend"])
	}
	if !strings.Contains(string(stderr), "Remove backend: null from") {
		t.Errorf("expected a warning to remove the saved backend, got stderr:\n%s", stderr)
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

func TestShowKustomization_RendersDecryptionThroughFacetMerge(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "facet-tiers")
	env = append(env, "WINDSOR_CONTEXT=default")
	// cert-manager's install tier declares decryption in pki.yaml and merges across three facets,
	// so a rendered spec.decryption proves the field survives composition and reaches the Flux CR.
	stdout, stderr, err := helpers.RunCLI(dir, []string{"show", "kustomization", "cert-manager-install"}, env)
	if err != nil {
		t.Fatalf("show kustomization cert-manager-install: %v\nstderr: %s", err, stderr)
	}
	var k struct {
		Spec struct {
			Decryption struct {
				Provider  string `yaml:"provider"`
				SecretRef struct {
					Name string `yaml:"name"`
				} `yaml:"secretRef"`
			} `yaml:"decryption"`
		} `yaml:"spec"`
	}
	if err := yaml.Unmarshal(stdout, &k); err != nil {
		t.Fatalf("parse kustomization YAML: %v\nstdout: %s", err, stdout)
	}
	if k.Spec.Decryption.Provider != "sops" {
		t.Errorf("expected spec.decryption.provider sops, got %q\nstdout: %s", k.Spec.Decryption.Provider, stdout)
	}
	if k.Spec.Decryption.SecretRef.Name != "sops-age" {
		t.Errorf("expected spec.decryption.secretRef.name sops-age, got %q", k.Spec.Decryption.SecretRef.Name)
	}
}

func TestShowKustomization_RendersHealthCheckExprsAccumulatedAcrossFacets(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "facet-tiers")
	env = append(env, "WINDSOR_CONTEXT=default")
	// pki.yaml gates cert-manager's install tier on Issuer and addon-cert-manager-extra.yaml adds a
	// ClusterIssuer gate to the same tier, so both rendering on spec.healthCheckExprs proves the
	// field survives cross-facet merge and reaches the Flux CR.
	stdout, stderr, err := helpers.RunCLI(dir, []string{"show", "kustomization", "cert-manager-install"}, env)
	if err != nil {
		t.Fatalf("show kustomization cert-manager-install: %v\nstderr: %s", err, stderr)
	}
	var k struct {
		Spec struct {
			HealthCheckExprs []struct {
				APIVersion string `yaml:"apiVersion"`
				Kind       string `yaml:"kind"`
				Current    string `yaml:"current"`
			} `yaml:"healthCheckExprs"`
		} `yaml:"spec"`
	}
	if err := yaml.Unmarshal(stdout, &k); err != nil {
		t.Fatalf("parse kustomization YAML: %v\nstdout: %s", err, stdout)
	}
	if len(k.Spec.HealthCheckExprs) != 2 {
		t.Fatalf("expected 2 health check expressions, got %d\nstdout: %s", len(k.Spec.HealthCheckExprs), stdout)
	}
	kinds := []string{k.Spec.HealthCheckExprs[0].Kind, k.Spec.HealthCheckExprs[1].Kind}
	if !slices.Contains(kinds, "Issuer") || !slices.Contains(kinds, "ClusterIssuer") {
		t.Errorf("expected Issuer and ClusterIssuer gates, got %v", kinds)
	}
	for _, expr := range k.Spec.HealthCheckExprs {
		if expr.APIVersion != "cert-manager.io/v1" {
			t.Errorf("expected apiVersion cert-manager.io/v1, got %q", expr.APIVersion)
		}
		if expr.Current != "status.conditions.exists(e, e.type == 'Ready' && e.status == 'True')" {
			t.Errorf("expected Ready CEL expression for %s, got %q", expr.Kind, expr.Current)
		}
	}
}

func TestShowKustomization_OmitsHealthCheckExprsWhenUnset(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "facet-tiers")
	env = append(env, "WINDSOR_CONTEXT=default")
	stdout, stderr, err := helpers.RunCLI(dir, []string{"show", "kustomization", "dns"}, env)
	if err != nil {
		t.Fatalf("show kustomization dns: %v\nstderr: %s", err, stderr)
	}
	if strings.Contains(string(stdout), "healthCheckExprs") {
		t.Errorf("expected no healthCheckExprs on a kustomization that declares none, got:\n%s", stdout)
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

// seedRemovalCoreSource writes the core source of the facet-remove-across-sources fixture to the OCI
// cache in dir. The core source contributes the cluster terraform component, the web kustomization,
// and the gateway flux system.
func seedRemovalCoreSource(t *testing.T, dir string) {
	t.Helper()
	seedSourceArtifact(t, dir, "core", "v0.1.0", map[string]string{
		"metadata.yaml":            "name: core\nversion: v0.1.0\n",
		"_template/blueprint.yaml": "kind: Blueprint\napiVersion: blueprints.windsorcli.dev/v1alpha1\nmetadata:\n  name: core\n",
		"_template/facets/web.yaml": "kind: Facet\napiVersion: blueprints.windsorcli.dev/v1alpha1\nmetadata:\n  name: web\n" +
			"terraform:\n- path: cluster\n  inputs:\n    size: 3\n    debug: true\n" +
			"kustomize:\n- name: web\n  path: web\n  components:\n  - app\n  - debug\n" +
			"flux:\n- name: gateway\n  path: gateway\n  install:\n    components:\n    - envoy\n    - envoy-metrics\n",
	})
}

// prepareRemovalFixture copies the facet-remove-across-sources fixture, seeds its core source, and
// runs windsor init. It returns the directory and an environment that selects the local context, which
// init creates.
func prepareRemovalFixture(t *testing.T) (string, []string) {
	t.Helper()
	dir, env := helpers.CopyFixtureOnly(t, "facet-remove-across-sources")
	seedRemovalCoreSource(t, dir)
	helpers.MarkAsGitRepo(t, dir)
	if _, stderr, err := helpers.RunCLI(dir, []string{"init"}, env); err != nil {
		t.Fatalf("windsor init: %v\nstderr: %s", err, stderr)
	}
	return dir, append(env, "WINDSOR_CONTEXT=local")
}

// showClusterInputs runs windsor show blueprint in dir and returns the inputs of the cluster component.
func showClusterInputs(t *testing.T, dir string, env []string) map[string]any {
	t.Helper()
	stdout, stderr, err := helpers.RunCLI(dir, []string{"show", "blueprint"}, env)
	if err != nil {
		t.Fatalf("show blueprint: %v\nstderr: %s", err, stderr)
	}
	var bp struct {
		Terraform []struct {
			Path   string         `yaml:"path"`
			Inputs map[string]any `yaml:"inputs"`
		} `yaml:"terraform"`
	}
	if err := yaml.Unmarshal(stdout, &bp); err != nil {
		t.Fatalf("parse blueprint YAML: %v\nstdout: %s", err, stdout)
	}
	for _, c := range bp.Terraform {
		if c.Path == "cluster" {
			return c.Inputs
		}
	}
	t.Fatalf("expected terraform component cluster\nstdout: %s", stdout)
	return nil
}

// showKustomizationComponents runs windsor show kustomization name in dir and returns spec.components.
func showKustomizationComponents(t *testing.T, dir string, env []string, name string) []string {
	t.Helper()
	stdout, stderr, err := helpers.RunCLI(dir, []string{"show", "kustomization", name}, env)
	if err != nil {
		t.Fatalf("show kustomization %s: %v\nstderr: %s", name, err, stderr)
	}
	var k struct {
		Spec struct {
			Components []string `yaml:"components"`
		} `yaml:"spec"`
	}
	if err := yaml.Unmarshal(stdout, &k); err != nil {
		t.Fatalf("parse kustomization YAML: %v\nstdout: %s", err, stdout)
	}
	return k.Spec.Components
}

// A template facet with strategy remove deletes the named fields from the terraform component, the
// kustomization, and the flux system that the core source contributes.
func TestShowKustomization_RemovesFieldsThatASourceContributes(t *testing.T) {
	t.Parallel()
	dir, env := prepareRemovalFixture(t)

	web := showKustomizationComponents(t, dir, env, "web")
	if !slices.Contains(web, "app") || slices.Contains(web, "debug") {
		t.Errorf("expected web components with app and without debug, got %v", web)
	}

	install := showKustomizationComponents(t, dir, env, "gateway-install")
	if !slices.Contains(install, "envoy") || slices.Contains(install, "envoy-metrics") {
		t.Errorf("expected gateway-install components with envoy and without envoy-metrics, got %v", install)
	}

	inputs := showClusterInputs(t, dir, env)
	if _, ok := inputs["debug"]; ok || inputs["size"] == nil {
		t.Errorf("expected cluster inputs with size and without debug, got %v", inputs)
	}
}

// A removal whose condition is false leaves the fields that the core source contributes.
func TestShowKustomization_KeepsSourceFieldsWhenRemovalConditionIsFalse(t *testing.T) {
	t.Parallel()
	dir, env := prepareRemovalFixture(t)
	appendValues(t, dir, "web:\n  strip: false\n")

	web := showKustomizationComponents(t, dir, env, "web")
	if !slices.Contains(web, "app") || !slices.Contains(web, "debug") {
		t.Errorf("expected web components with app and debug, got %v", web)
	}

	install := showKustomizationComponents(t, dir, env, "gateway-install")
	if !slices.Contains(install, "envoy") || !slices.Contains(install, "envoy-metrics") {
		t.Errorf("expected gateway-install components with envoy and envoy-metrics, got %v", install)
	}

	inputs := showClusterInputs(t, dir, env)
	if inputs["debug"] == nil || inputs["size"] == nil {
		t.Errorf("expected cluster inputs with size and debug, got %v", inputs)
	}
}

func TestShowBlueprint_FullAcceptsRawAndJSON(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "facet-composition")
	env = append(env, "WINDSOR_CONTEXT=default")
	for _, args := range [][]string{
		{"show", "blueprint", "--full"},
		{"show", "blueprint", "--full", "--raw"},
		{"show", "blueprint", "--full", "--json"},
	} {
		if _, stderr, err := helpers.RunCLI(dir, args, env); err != nil {
			t.Fatalf("%v: %v\nstderr: %s", args, err, stderr)
		}
	}
}
