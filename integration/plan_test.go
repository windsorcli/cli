//go:build integration
// +build integration

package integration

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/windsorcli/cli/integration/helpers"
)

// skipIfFluxNotInstalled skips the test when the flux CLI is absent or not functional.
// The check runs from a temp directory so that aqua (if present) resolves the binary
// the same way Windsor's subprocess will, rather than from the repository root.
func skipIfFluxNotInstalled(t *testing.T) {
	t.Helper()
	cmd := exec.Command("flux", "version", "--client")
	cmd.Dir = t.TempDir()
	if err := cmd.Run(); err != nil {
		t.Skipf("flux CLI not available (%v) — skipping kustomize plan integration test", err)
	}
}

// =============================================================================
// Integration Tests
// =============================================================================

func TestPlanTerraform_FailsWhenNotInTrustedDirectory(t *testing.T) {
	t.Parallel()
	dir, env := helpers.CopyFixtureOnly(t, "default")
	_, stderr, err := helpers.RunCLI(dir, []string{"plan", "terraform", "cluster"}, env)
	if err == nil {
		t.Fatal("expected failure but command succeeded")
	}
	if !strings.Contains(string(stderr), "trusted") {
		t.Errorf("expected stderr to mention 'trusted', got: %s", stderr)
	}
}

func TestPlanTerraform_FailsWithNoArgument(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "default")
	_, _, err := helpers.RunCLI(dir, []string{"plan", "terraform"}, env)
	if err == nil {
		t.Fatal("expected failure but command succeeded")
	}
}

func TestPlanTerraform_FailsForNonexistentComponent(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "default")
	env = append(env, "WINDSOR_CONTEXT=default")
	_, stderr, err := helpers.RunCLI(dir, []string{"plan", "terraform", "nonexistent"}, env)
	if err == nil {
		t.Fatal("expected failure but command succeeded")
	}
	if !strings.Contains(string(stderr), "nonexistent") && !strings.Contains(string(stderr), "not found") && !strings.Contains(string(stderr), "error") {
		t.Errorf("expected stderr to mention the component or an error, got: %s", stderr)
	}
}

func TestPlanTerraform_SucceedsWithMinimalLocalConfig(t *testing.T) {
	t.Parallel()
	dir, env := helpers.CopyFixtureOnly(t, "plan")
	_, stderr, err := helpers.RunCLI(dir, []string{"init", "local"}, env)
	if err != nil {
		t.Fatalf("init local: %v\nstderr: %s", err, stderr)
	}
	env = append(env, "WINDSOR_CONTEXT=local")
	_, stderr, err = helpers.RunCLI(dir, []string{"plan", "terraform", "null"}, env)
	if err != nil {
		t.Fatalf("plan terraform null: %v\nstderr: %s", err, stderr)
	}
}

func TestPlanKustomize_FailsWhenNotInTrustedDirectory(t *testing.T) {
	t.Parallel()
	dir, env := helpers.CopyFixtureOnly(t, "plan")
	_, stderr, err := helpers.RunCLI(dir, []string{"plan", "kustomize", "my-app"}, env)
	if err == nil {
		t.Fatal("expected failure but command succeeded")
	}
	if !strings.Contains(string(stderr), "trusted") {
		t.Errorf("expected stderr to mention 'trusted', got: %s", stderr)
	}
}

func TestPlanKustomize_FailsWithNoArgument(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "plan")
	_, _, err := helpers.RunCLI(dir, []string{"plan", "kustomize"}, env)
	if err == nil {
		t.Fatal("expected failure but command succeeded")
	}
}

func TestPlanKustomize_K8sAliasFailsWithNoArgument(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "plan")
	_, _, err := helpers.RunCLI(dir, []string{"plan", "k8s"}, env)
	if err == nil {
		t.Fatal("expected failure for missing argument on k8s alias, got nil")
	}
}

func TestPlanKustomize_SucceedsWithEmptyKustomizations(t *testing.T) {
	t.Parallel()
	skipIfFluxNotInstalled(t)
	dir, env := helpers.CopyFixtureOnly(t, "plan")
	_, stderr, err := helpers.RunCLI(dir, []string{"init", "local"}, env)
	if err != nil {
		t.Fatalf("init local: %v\nstderr: %s", err, stderr)
	}
	env = append(env, "WINDSOR_CONTEXT=local")
	_, stderr, err = helpers.RunCLI(dir, []string{"plan", "kustomize", "all"}, env)
	if err != nil {
		t.Fatalf("plan kustomize all: %v\nstderr: %s", err, stderr)
	}
}

func TestPlanKustomize_FailsWhenKustomizationNotInBlueprint(t *testing.T) {
	t.Parallel()
	skipIfFluxNotInstalled(t)
	dir, env := helpers.PrepareFixture(t, "plan")
	env = append(env, "WINDSOR_CONTEXT=local")
	_, stderr, err := helpers.RunCLI(dir, []string{"plan", "kustomize", "nonexistent"}, env)
	if err == nil {
		t.Fatal("expected failure but command succeeded")
	}
	if !strings.Contains(string(stderr), "nonexistent") && !strings.Contains(string(stderr), "not found") {
		t.Errorf("expected stderr to mention component name or 'not found', got: %s", stderr)
	}
}
