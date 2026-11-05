//go:build integration
// +build integration

package integration

import (
	"strings"
	"testing"

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

	stdout, stderr, err = helpers.RunCLI(dir, []string{"show", "artifacts", "-o", "flux-mirror"}, env)
	if err != nil {
		t.Fatalf("show artifacts -o flux-mirror: %v\nstderr: %s", err, stderr)
	}
	if out := string(stdout); !strings.Contains(out, "apiVersion: mirror.plugin.fluxcd.io/v1beta1") {
		t.Errorf("expected a Flux Mirror config, got:\n%s", out)
	}

	stdout, stderr, err = helpers.RunCLI(dir, []string{"show", "artifacts", "-o", "list"}, env)
	if err != nil {
		t.Fatalf("show artifacts -o list: %v\nstderr: %s", err, stderr)
	}
	if len(stdout) != 0 {
		t.Errorf("expected no lines, got %q", stdout)
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
