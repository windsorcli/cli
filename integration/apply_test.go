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

func TestApplyTerraform_SucceedsWithMinimalLocalConfig(t *testing.T) {
	t.Parallel()
	dir, env := helpers.CopyFixtureOnly(t, "plan")
	_, stderr, err := helpers.RunCLI(dir, []string{"init", "local"}, env)
	if err != nil {
		t.Fatalf("init local: %v\nstderr: %s", err, stderr)
	}
	env = append(env, "WINDSOR_CONTEXT=local")
	_, stderr, err = helpers.RunCLI(dir, []string{"apply", "terraform", "null"}, env)
	if err != nil {
		t.Fatalf("apply terraform null: %v\nstderr: %s", err, stderr)
	}
}

func TestApplyTerraform_FailsWhenNotInTrustedDirectory(t *testing.T) {
	t.Parallel()
	dir, env := helpers.CopyFixtureOnly(t, "plan")
	_, stderr, err := helpers.RunCLI(dir, []string{"apply", "terraform", "null"}, env)
	if err == nil {
		t.Fatal("expected failure but command succeeded")
	}
	if !strings.Contains(string(stderr), "trusted") {
		t.Errorf("expected stderr to mention 'trusted', got: %s", stderr)
	}
}

func TestApplyTerraform_FailsWithNoArgument(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "plan")
	_, _, err := helpers.RunCLI(dir, []string{"apply", "terraform"}, env)
	if err == nil {
		t.Fatal("expected failure but command succeeded")
	}
}

func TestApplyTerraform_FailsForNonexistentComponent(t *testing.T) {
	t.Parallel()
	dir, env := helpers.CopyFixtureOnly(t, "plan")
	_, stderr, err := helpers.RunCLI(dir, []string{"init", "local"}, env)
	if err != nil {
		t.Fatalf("init local: %v\nstderr: %s", err, stderr)
	}
	env = append(env, "WINDSOR_CONTEXT=local")
	_, stderr, err = helpers.RunCLI(dir, []string{"apply", "terraform", "nonexistent"}, env)
	if err == nil {
		t.Fatal("expected failure but command succeeded")
	}
	if !strings.Contains(string(stderr), "nonexistent") && !strings.Contains(string(stderr), "not found") && !strings.Contains(string(stderr), "error") {
		t.Errorf("expected stderr to mention the component or an error, got: %s", stderr)
	}
}

func TestApplyKustomize_AcceptsWaitFlag(t *testing.T) {
	t.Parallel()
	dir, env := helpers.CopyFixtureOnly(t, "plan")
	_, stderr, err := helpers.RunCLI(dir, []string{"init", "local"}, env)
	if err != nil {
		t.Fatalf("init local: %v\nstderr: %s", err, stderr)
	}
	env = append(env, "WINDSOR_CONTEXT=local")
	_, stderr, err = helpers.RunCLI(dir, []string{"apply", "kustomize", "--wait"}, env)
	if strings.Contains(string(stderr), "unknown flag") {
		t.Errorf("--wait should be a recognised flag, got: %s", stderr)
	}
	_ = err // failure is expected without a live cluster; the flag must be accepted
}

func TestApply_AcceptsWaitFlag(t *testing.T) {
	t.Parallel()
	dir, env := helpers.CopyFixtureOnly(t, "plan")
	_, stderr, err := helpers.RunCLI(dir, []string{"init", "local"}, env)
	if err != nil {
		t.Fatalf("init local: %v\nstderr: %s", err, stderr)
	}
	env = append(env, "WINDSOR_CONTEXT=local")
	_, stderr, err = helpers.RunCLI(dir, []string{"apply", "--wait"}, env)
	if strings.Contains(string(stderr), "unknown flag") {
		t.Errorf("--wait should be a recognised flag on apply, got: %s", stderr)
	}
	_ = err // may fail due to infrastructure not being available; flag must be accepted
}

// TestApplyTerraform_MissingBinary_ShowsActionableError verifies the registry-formatted
// missing-tool error reaches the user end-to-end: when an `apply terraform` preflight
// fails because terraform is not on PATH, stderr must include the vendor download URL
// so the operator has a copy-pasteable next step pointing at the authoritative install
// instructions. init runs with --set terraform.enabled=true so the preflight check
// actually fires (without that gate, the provisioner would shell out directly and surface
// a raw exec error instead of the formatted one). The "null" component arg is an
// arbitrary positional placeholder — the preflight check fails inside Initialize before
// the command body reads componentID, so no matching component needs to exist in the plan
// fixture.
func TestApplyTerraform_MissingBinary_ShowsActionableError(t *testing.T) {
	t.Parallel()
	dir, env := helpers.CopyFixtureOnly(t, "plan")

	if _, stderr, err := helpers.RunCLI(dir, []string{"init", "local", "--set", "terraform.enabled=true"}, env); err != nil {
		t.Fatalf("init local --set terraform.enabled=true: %v\nstderr: %s", err, stderr)
	}

	stripped := append(helpers.MinimalPATHEnv(env), "WINDSOR_CONTEXT=local")

	_, stderr, err := helpers.RunCLI(dir, []string{"apply", "terraform", "null"}, stripped)
	if err == nil {
		t.Fatal("expected apply terraform to fail when terraform is not on PATH, but it succeeded")
	}
	out := string(stderr)
	if !strings.Contains(out, "not found on PATH") {
		t.Errorf("expected stderr to mention 'not found on PATH', got: %s", out)
	}
	if !strings.Contains(out, "Install:") {
		t.Errorf("expected stderr to include an 'Install:' vendor URL hint, got: %s", out)
	}
	if !strings.Contains(out, "developer.hashicorp.com") {
		t.Errorf("expected stderr to include the vendor install URL, got: %s", out)
	}
	if strings.Contains(out, "aqua g -i") {
		t.Errorf("expected stderr to OMIT third-party 'aqua g -i' hint, got: %s", out)
	}
}
