//go:build integration
// +build integration

package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gofrs/flock"

	"github.com/windsorcli/cli/integration/helpers"
)

// =============================================================================
// Integration Tests
// =============================================================================

func TestApplyTerraform_SucceedsWithMinimalLocalConfig(t *testing.T) {
	t.Parallel()
	dir, env := helpers.CopyFixtureOnly(t, "plan")
	helpers.MarkAsGitRepo(t, dir)
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
	helpers.MarkAsGitRepo(t, dir)
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
	helpers.MarkAsGitRepo(t, dir)
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
	helpers.MarkAsGitRepo(t, dir)
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

func TestApply_AcceptsPruneFlag(t *testing.T) {
	t.Parallel()
	dir, env := helpers.CopyFixtureOnly(t, "plan")
	helpers.MarkAsGitRepo(t, dir)
	_, stderr, err := helpers.RunCLI(dir, []string{"init", "local"}, env)
	if err != nil {
		t.Fatalf("init local: %v\nstderr: %s", err, stderr)
	}
	env = append(env, "WINDSOR_CONTEXT=local")
	_, stderr, err = helpers.RunCLI(dir, []string{"apply", "--prune"}, env)
	if strings.Contains(string(stderr), "unknown flag") {
		t.Errorf("--prune should be a recognised flag on apply, got: %s", stderr)
	}
	_ = err // may fail due to infrastructure not being available; the flag must be accepted
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
	helpers.MarkAsGitRepo(t, dir)

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

// TestApplyTerraform_FailsFastOnLockContentionByDefault verifies that with no
// --lock-timeout flag, a command contending for an already-held stack lock fails
// immediately with a busy error rather than silently blocking for minutes.
func TestApplyTerraform_FailsFastOnLockContentionByDefault(t *testing.T) {
	t.Parallel()
	dir, env := helpers.CopyFixtureOnly(t, "plan")
	helpers.MarkAsGitRepo(t, dir)
	_, stderr, err := helpers.RunCLI(dir, []string{"init", "local"}, env)
	if err != nil {
		t.Fatalf("init local: %v\nstderr: %s", err, stderr)
	}
	env = append(env, "WINDSOR_CONTEXT=local")

	scratch := filepath.Join(dir, ".windsor", "contexts", "local")
	if err := os.MkdirAll(scratch, 0o755); err != nil {
		t.Fatalf("mkdir scratch: %v", err)
	}
	held := flock.New(filepath.Join(scratch, ".stacklock"))
	locked, err := held.TryLock()
	if err != nil || !locked {
		t.Fatalf("hold lock: locked=%v err=%v", locked, err)
	}
	t.Cleanup(func() { _ = held.Unlock() })

	start := time.Now()
	_, stderr, err = helpers.RunCLI(dir, []string{"apply", "terraform", "null"}, env)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected failure while the stack lock is held, command succeeded")
	}
	if !strings.Contains(string(stderr), "stack lock") || !strings.Contains(string(stderr), "is held by") {
		t.Errorf("expected a stack-lock busy error, got: %s", stderr)
	}
	if elapsed > 4*time.Second {
		t.Errorf("expected the default (no --lock-timeout) to fail immediately, took %v", elapsed)
	}
}

// TestApplyTerraform_LockTimeoutFlagWaitsBeforeFailing verifies that --lock-timeout
// causes a command to retry against a contended stack lock for roughly the given
// duration before surfacing the same busy error, confirming the flag is wired
// through to the underlying Acquire call.
func TestApplyTerraform_LockTimeoutFlagWaitsBeforeFailing(t *testing.T) {
	t.Parallel()
	dir, env := helpers.CopyFixtureOnly(t, "plan")
	helpers.MarkAsGitRepo(t, dir)
	_, stderr, err := helpers.RunCLI(dir, []string{"init", "local"}, env)
	if err != nil {
		t.Fatalf("init local: %v\nstderr: %s", err, stderr)
	}
	env = append(env, "WINDSOR_CONTEXT=local")

	scratch := filepath.Join(dir, ".windsor", "contexts", "local")
	if err := os.MkdirAll(scratch, 0o755); err != nil {
		t.Fatalf("mkdir scratch: %v", err)
	}
	held := flock.New(filepath.Join(scratch, ".stacklock"))
	locked, err := held.TryLock()
	if err != nil || !locked {
		t.Fatalf("hold lock: locked=%v err=%v", locked, err)
	}
	t.Cleanup(func() { _ = held.Unlock() })

	start := time.Now()
	_, stderr, err = helpers.RunCLI(dir, []string{"apply", "terraform", "null", "--lock-timeout=300ms"}, env)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected failure while the stack lock is held, command succeeded")
	}
	if !strings.Contains(string(stderr), "stack lock") || !strings.Contains(string(stderr), "is held by") {
		t.Errorf("expected a stack-lock busy error, got: %s", stderr)
	}
	if elapsed < 250*time.Millisecond {
		t.Errorf("expected --lock-timeout=300ms to retry for ~300ms before failing, took %v", elapsed)
	}
}
