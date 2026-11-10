//go:build integration
// +build integration

package integration

import (
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
