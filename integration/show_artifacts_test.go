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

// seedChartCache copies the fixture's packaged charts into the chart cache of dir, so the
// command renders each HelmRelease without network access.
func seedChartCache(t *testing.T, dir string) {
	t.Helper()
	seeds := map[string]string{
		"https___kubernetes.github.io_ingress-nginx/ingress-nginx-4.15.1.tgz": "ingress-nginx-4.15.1.tgz",
		"ghcr.io_stefanprodan_charts_podinfo/podinfo-6.9.2.tgz":               "podinfo-6.9.2.tgz",
		"registry.example.com_windsor_podinfo/podinfo-6.9.2.tgz":              "podinfo-6.9.2.tgz",
	}
	for dest, src := range seeds {
		data, err := os.ReadFile(filepath.Join(dir, "charts", src))
		if err != nil {
			t.Fatalf("read chart %s: %v", src, err)
		}
		path := filepath.Join(dir, ".windsor", "cache", "charts", filepath.FromSlash(dest))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatalf("create chart cache: %v", err)
		}
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatalf("write chart %s: %v", dest, err)
		}
	}
}

// =============================================================================
// Integration Tests
// =============================================================================

func TestShowArtifacts_ListsChartsAndImages(t *testing.T) {
	t.Parallel()
	skipIfFluxNotInstalled(t)
	dir, env := helpers.CopyFixtureOnly(t, "show-artifacts")
	helpers.MarkAsGitRepo(t, dir)
	if _, stderr, err := helpers.RunCLI(dir, []string{"init", "local"}, env); err != nil {
		t.Fatalf("init local: %v\nstderr: %s", err, stderr)
	}
	seedChartCache(t, dir)
	env = append(env, "WINDSOR_CONTEXT=local")
	stdout, stderr, err := helpers.RunCLI(dir, []string{"show", "artifacts", "--charts-to", "registry.example.com/windsor"}, env)
	if err != nil {
		t.Fatalf("show artifacts: %v\nstderr: %s", err, stderr)
	}
	var inv struct {
		Artifacts []struct {
			Kind        string `yaml:"kind"`
			Reference   string `yaml:"reference"`
			Destination string `yaml:"destination"`
		} `yaml:"artifacts"`
	}
	if err := yaml.Unmarshal(stdout, &inv); err != nil {
		t.Fatalf("parse artifacts YAML: %v\n%s", err, stdout)
	}
	got := make(map[string]string)
	for _, a := range inv.Artifacts {
		got[a.Kind+" "+a.Reference] = a.Destination
	}
	want := map[string]string{
		"chart ghcr.io/stefanprodan/charts/podinfo":        "registry.example.com/windsor/podinfo:6.9.2",
		"chart https://kubernetes.github.io/ingress-nginx": "registry.example.com/windsor/ingress-nginx:4.15.1",
		"image docker.io/library/nginx":                    "",
		"image registry.k8s.io/ingress-nginx/controller":   "",
		"image ghcr.io/stefanprodan/podinfo":               "",
	}
	for key, dest := range want {
		if d, ok := got[key]; !ok || d != dest {
			t.Errorf("expected %s with destination %q, got %v", key, dest, got)
		}
	}
}

func TestShowArtifacts_FluxMirrorLeavesOutChartsInTheRegistry(t *testing.T) {
	t.Parallel()
	skipIfFluxNotInstalled(t)
	dir, env := helpers.CopyFixtureOnly(t, "show-artifacts")
	helpers.MarkAsGitRepo(t, dir)
	if _, stderr, err := helpers.RunCLI(dir, []string{"init", "local"}, env); err != nil {
		t.Fatalf("init local: %v\nstderr: %s", err, stderr)
	}
	seedChartCache(t, dir)
	env = append(env, "WINDSOR_CONTEXT=local")
	args := []string{"show", "artifacts", "-o", "flux-mirror", "--charts-to", "registry.example.com/windsor",
		"--set", "substitutions.common.chart_registry=oci://registry.example.com/windsor"}
	stdout, stderr, err := helpers.RunCLI(dir, args, env)
	if err != nil {
		t.Fatalf("show artifacts -o flux-mirror: %v\nstderr: %s", err, stderr)
	}
	out := string(stdout)
	if !strings.Contains(out, "apiVersion: mirror.plugin.fluxcd.io/v1beta1") || !strings.Contains(out, "name: ingress-nginx") {
		t.Errorf("expected a Flux Mirror config with the HTTP chart, got:\n%s", out)
	}
	if strings.Contains(out, "podinfo") {
		t.Errorf("expected the chart in the registry to stay out of the config, got:\n%s", out)
	}
	if !strings.Contains(string(stderr), "podinfo:6.9.2 is already in registry.example.com/windsor") {
		t.Errorf("expected stderr to name the chart in the registry, got: %s", stderr)
	}
}

func TestShowArtifacts_FluxMirrorNeedsChartsTo(t *testing.T) {
	t.Parallel()
	skipIfFluxNotInstalled(t)
	dir, env := helpers.CopyFixtureOnly(t, "show-artifacts")
	helpers.MarkAsGitRepo(t, dir)
	if _, stderr, err := helpers.RunCLI(dir, []string{"init", "local"}, env); err != nil {
		t.Fatalf("init local: %v\nstderr: %s", err, stderr)
	}
	seedChartCache(t, dir)
	env = append(env, "WINDSOR_CONTEXT=local")
	_, stderr, err := helpers.RunCLI(dir, []string{"show", "artifacts", "-o", "flux-mirror"}, env)
	if err == nil {
		t.Fatal("expected failure but command succeeded")
	}
	if !strings.Contains(string(stderr), "--charts-to") {
		t.Errorf("expected stderr to mention --charts-to, got: %s", stderr)
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
