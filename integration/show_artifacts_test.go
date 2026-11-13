//go:build integration
// +build integration

package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/windsorcli/cli/integration/helpers"
)

// =============================================================================
// Integration Tests
// =============================================================================

func TestShowArtifacts_PrintsEachOutputFormat(t *testing.T) {
	t.Parallel()
	dir, env := helpers.CopyFixtureOnly(t, "show-artifacts")
	helpers.MarkAsGitRepo(t, dir)
	if _, stderr, err := helpers.RunCLI(dir, []string{"init", "local"}, env); err != nil {
		t.Fatalf("init local: %v\nstderr: %s", err, stderr)
	}
	env = append(env, "WINDSOR_CONTEXT=local")

	stdout, stderr, err := helpers.RunCLI(dir, []string{"show", "artifacts"}, env)
	if err != nil {
		t.Fatalf("show artifacts: %v\nstderr: %s", err, stderr)
	}
	if out := string(stdout); !strings.Contains(out, "version: v1alpha1") || !strings.Contains(out, "context: local") {
		t.Errorf("expected an inventory document, got:\n%s", out)
	}

	stdout, stderr, err = helpers.RunCLI(dir, []string{"show", "artifacts", "-o", "flux-mirror", "--charts-to", "registry.example.com/windsor"}, env)
	if err != nil {
		t.Fatalf("show artifacts -o flux-mirror: %v\nstderr: %s", err, stderr)
	}
	if out := string(stdout); !strings.Contains(out, "apiVersion: mirror.plugin.fluxcd.io/v1beta2") {
		t.Errorf("expected a Flux Mirror config, got:\n%s", out)
	}

	stdout, stderr, err = helpers.RunCLI(dir, []string{"show", "artifacts", "-o", "list", "--charts-to", "registry.example.com/windsor"}, env)
	if err != nil {
		t.Fatalf("show artifacts -o list: %v\nstderr: %s", err, stderr)
	}
	if out := string(stdout); !strings.Contains(out, "registry.example.com/windsor/cert-manager:v1.17.0") {
		t.Errorf("expected the chart destination, got %q", out)
	}

	stdout, stderr, err = helpers.RunCLI(dir, []string{"show", "artifacts", "-o", "cyclonedx"}, env)
	if err != nil {
		t.Fatalf("show artifacts -o cyclonedx: %v\nstderr: %s", err, stderr)
	}
	if out := string(stdout); !strings.Contains(out, `"specVersion": "1.6"`) || !strings.Contains(out, "pkg:generic/cert-manager@v1.17.0") {
		t.Errorf("expected a CycloneDX BOM with the chart, got:\n%s", out)
	}
}

func TestShowArtifacts_ListsTheArtifactTagsOfTheProject(t *testing.T) {
	t.Parallel()
	dir, env := helpers.CopyFixtureOnly(t, "show-artifacts")
	helpers.MarkAsGitRepo(t, dir)
	if _, stderr, err := helpers.RunCLI(dir, []string{"init", "local"}, env); err != nil {
		t.Fatalf("init local: %v\nstderr: %s", err, stderr)
	}
	env = append(env, "WINDSOR_CONTEXT=local")

	stdout, stderr, err := helpers.RunCLI(dir, []string{"show", "artifacts", "--charts-to", "registry.example.com/windsor"}, env)
	if err != nil {
		t.Fatalf("show artifacts: %v\nstderr: %s", err, stderr)
	}
	var inv struct {
		Artifacts []struct {
			Kind        string `yaml:"kind"`
			Reference   string `yaml:"reference"`
			Version     string `yaml:"version"`
			Destination string `yaml:"destination"`
			NoReferrers bool   `yaml:"noReferrers"`
		} `yaml:"artifacts"`
	}
	if err := yaml.Unmarshal(stdout, &inv); err != nil {
		t.Fatalf("parse artifacts YAML: %v\n%s", err, stdout)
	}
	got := make(map[string]string)
	for _, a := range inv.Artifacts {
		got[a.Kind+" "+a.Reference+" "+a.Version] = a.Destination
		if want := a.Reference == "ghcr.io/example/tooling"; a.NoReferrers != want {
			t.Errorf("expected noReferrers %v for %s, got %v", want, a.Reference, a.NoReferrers)
		}
	}
	want := map[string]string{
		"chart https://charts.jetstack.io v1.17.0": "registry.example.com/windsor/cert-manager:v1.17.0",
		"image ghcr.io/example/worker 1.2.3":       "",
		"image ghcr.io/example/tooling 2.0.0":      "",
		"image ghcr.io/example/vars 2.5.0":         "",
	}
	if len(got) != len(want) {
		t.Errorf("expected %v, got %v", want, got)
	}
	for key, dest := range want {
		if d, ok := got[key]; !ok || d != dest {
			t.Errorf("expected %s with destination %q, got %v", key, dest, got)
		}
	}
}

func TestShowArtifacts_ScopesTheListToTheBlueprintUnlessAll(t *testing.T) {
	t.Parallel()
	dir, env := helpers.CopyFixtureOnly(t, "show-artifacts")
	helpers.MarkAsGitRepo(t, dir)
	if _, stderr, err := helpers.RunCLI(dir, []string{"init", "local"}, env); err != nil {
		t.Fatalf("init local: %v\nstderr: %s", err, stderr)
	}
	env = append(env, "WINDSOR_CONTEXT=local")

	stdout, stderr, err := helpers.RunCLI(dir, []string{"show", "artifacts"}, env)
	if err != nil {
		t.Fatalf("show artifacts: %v\nstderr: %s", err, stderr)
	}
	if out := string(stdout); strings.Contains(out, "ghcr.io/example/unused") || !strings.Contains(out, "ghcr.io/example/worker") {
		t.Errorf("expected the used image and not the unused image, got:\n%s", out)
	}
	if out := string(stdout); strings.Contains(out, "ghcr.io/example/idle") || !strings.Contains(out, "ghcr.io/example/tooling") {
		t.Errorf("expected the used facet image and not the idle facet image, got:\n%s", out)
	}
	if out := string(stdout); !strings.Contains(out, "ghcr.io/example/vars") {
		t.Errorf("expected the image of the .tfvars tag, got:\n%s", out)
	}

	stdout, stderr, err = helpers.RunCLI(dir, []string{"show", "artifacts", "--all"}, env)
	if err != nil {
		t.Fatalf("show artifacts --all: %v\nstderr: %s", err, stderr)
	}
	if out := string(stdout); !strings.Contains(out, "ghcr.io/example/unused") || !strings.Contains(out, "ghcr.io/example/worker") || !strings.Contains(out, "ghcr.io/example/idle") || !strings.Contains(out, "ghcr.io/example/vars") {
		t.Errorf("expected every image, got:\n%s", out)
	}
}

func TestShowArtifacts_RejectsAnUnknownOutputFormat(t *testing.T) {
	t.Parallel()
	dir, env := helpers.CopyFixtureOnly(t, "show-artifacts")
	helpers.MarkAsGitRepo(t, dir)
	_, stderr, err := helpers.RunCLI(dir, []string{"show", "artifacts", "-o", "json"}, env)
	if err == nil {
		t.Fatal("expected failure but command succeeded")
	}
	if !strings.Contains(string(stderr), `invalid output format "json"`) {
		t.Errorf("expected stderr to name the format, got: %s", stderr)
	}
}

func TestShowArtifacts_FailsWhenNotInTrustedDirectory(t *testing.T) {
	t.Parallel()
	dir, env := helpers.CopyFixtureOnly(t, "show-artifacts")
	_, stderr, err := helpers.RunCLI(dir, []string{"show", "artifacts"}, env)
	if err == nil {
		t.Fatal("expected failure but command succeeded")
	}
	if !strings.Contains(string(stderr), "trusted") {
		t.Errorf("expected stderr to mention 'trusted', got: %s", stderr)
	}
}

func TestShowArtifacts_GivesImagesTheRegistriesMirror(t *testing.T) {
	t.Parallel()
	dir, env := helpers.CopyFixtureOnly(t, "show-artifacts")
	helpers.MarkAsGitRepo(t, dir)
	if _, stderr, err := helpers.RunCLI(dir, []string{"init", "local", "--set", "registries[ghcr.io].endpoint=http://mirror.test:5000"}, env); err != nil {
		t.Fatalf("init local: %v\nstderr: %s", err, stderr)
	}
	values, err := os.ReadFile(filepath.Join(dir, "contexts", "local", "values.yaml"))
	if err != nil {
		t.Fatalf("read values.yaml: %v", err)
	}
	if !strings.Contains(string(values), "\nregistries:\n") || strings.Contains(string(values), "docker:") {
		t.Errorf("expected init to write the top-level registries key, got:\n%s", values)
	}
	env = append(env, "WINDSOR_CONTEXT=local")

	stdout, stderr, err := helpers.RunCLI(dir, []string{"show", "artifacts", "-o", "list", "--charts-to", "registry.example.com/windsor"}, env)
	if err != nil {
		t.Fatalf("show artifacts: %v\nstderr: %s", err, stderr)
	}
	if out := string(stdout); !strings.Contains(out, "mirror.test:5000/example/worker:1.2.3\tghcr.io/example/worker:1.2.3") {
		t.Errorf("expected the image on the mirror, got:\n%s", out)
	}
	if strings.Contains(string(stderr), "docker.registries") {
		t.Errorf("expected no docker.registries warning, got: %s", stderr)
	}
}

func TestShowArtifacts_ReadsDockerRegistriesWithAWarning(t *testing.T) {
	t.Parallel()
	dir, env := helpers.CopyFixtureOnly(t, "show-artifacts")
	helpers.MarkAsGitRepo(t, dir)
	if _, stderr, err := helpers.RunCLI(dir, []string{"init", "local"}, env); err != nil {
		t.Fatalf("init local: %v\nstderr: %s", err, stderr)
	}
	appendValues(t, dir, "docker:\n  registries:\n    ghcr.io:\n      endpoint: mirror.test:5000\n")
	env = append(env, "WINDSOR_CONTEXT=local")

	stdout, stderr, err := helpers.RunCLI(dir, []string{"show", "artifacts", "-o", "list", "--charts-to", "registry.example.com/windsor"}, env)
	if err != nil {
		t.Fatalf("show artifacts: %v\nstderr: %s", err, stderr)
	}
	if out := string(stdout); !strings.Contains(out, "mirror.test:5000/example/worker:1.2.3\tghcr.io/example/worker:1.2.3") {
		t.Errorf("expected the image on the mirror, got:\n%s", out)
	}
	if !strings.Contains(string(stderr), "docker.registries is deprecated") || !strings.Contains(string(stderr), "top-level registries key") {
		t.Errorf("expected the docker.registries warning, got: %s", stderr)
	}
}

func TestShowArtifacts_FailsWhenBothRegistriesKeysAreSet(t *testing.T) {
	t.Parallel()
	dir, env := helpers.CopyFixtureOnly(t, "show-artifacts")
	helpers.MarkAsGitRepo(t, dir)
	if _, stderr, err := helpers.RunCLI(dir, []string{"init", "local"}, env); err != nil {
		t.Fatalf("init local: %v\nstderr: %s", err, stderr)
	}
	appendValues(t, dir, "registries:\n  ghcr.io:\n    endpoint: mirror.test:5000\ndocker:\n  registries:\n    quay.io:\n      endpoint: mirror.test:5000\n")
	env = append(env, "WINDSOR_CONTEXT=local")

	_, stderr, err := helpers.RunCLI(dir, []string{"show", "artifacts"}, env)
	if err == nil {
		t.Fatal("expected failure but command succeeded")
	}
	if !strings.Contains(string(stderr), "both registries and docker.registries") {
		t.Errorf("expected stderr to name both keys, got: %s", stderr)
	}
}

// appendValues appends content to the values.yaml of the local context in dir.
func appendValues(t *testing.T, dir, content string) {
	t.Helper()
	path := filepath.Join(dir, "contexts", "local", "values.yaml")
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("read values.yaml: %v", err)
	}
	if err := os.WriteFile(path, append(existing, []byte("\n"+content)...), 0o600); err != nil {
		t.Fatalf("write values.yaml: %v", err)
	}
}

func TestShowValues_GivesFacetsOneRegistriesMap(t *testing.T) {
	t.Parallel()
	dir, env := helpers.CopyFixtureOnly(t, "show-artifacts")
	helpers.MarkAsGitRepo(t, dir)
	if _, stderr, err := helpers.RunCLI(dir, []string{"init", "local"}, env); err != nil {
		t.Fatalf("init local: %v\nstderr: %s", err, stderr)
	}
	appendValues(t, dir, "docker:\n  registries:\n    ghcr.io:\n      endpoint: mirror.test:5000\n")
	env = append(env, "WINDSOR_CONTEXT=local")

	stdout, stderr, err := helpers.RunCLI(dir, []string{"show", "values", "--json"}, env)
	if err != nil {
		t.Fatalf("show values: %v\nstderr: %s", err, stderr)
	}
	var values struct {
		Registries map[string]struct {
			Endpoint string `yaml:"endpoint"`
		} `yaml:"registries"`
	}
	if err := yaml.Unmarshal(stdout, &values); err != nil {
		t.Fatalf("parse values JSON: %v\n%s", err, stdout)
	}
	if got := values.Registries["ghcr.io"].Endpoint; got != "mirror.test:5000" {
		t.Errorf("expected registries to hold the docker.registries entry, got:\n%s", stdout)
	}
}
