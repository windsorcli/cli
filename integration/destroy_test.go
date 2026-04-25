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

func TestDestroyTerraform_SucceedsWithMinimalLocalConfig(t *testing.T) {
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
	_, stderr, err = helpers.RunCLI(dir, []string{"destroy", "--confirm=null", "terraform", "null"}, env)
	if err != nil {
		t.Fatalf("destroy terraform null: %v\nstderr: %s", err, stderr)
	}
}

func TestDestroyTerraform_SucceedsAllWithMinimalLocalConfig(t *testing.T) {
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
	_, stderr, err = helpers.RunCLI(dir, []string{"destroy", "--confirm=local", "terraform"}, env)
	if err != nil {
		t.Fatalf("destroy terraform: %v\nstderr: %s", err, stderr)
	}
}

func TestDestroyTerraform_FailsWhenNotInTrustedDirectory(t *testing.T) {
	t.Parallel()
	dir, env := helpers.CopyFixtureOnly(t, "plan")
	_, stderr, err := helpers.RunCLI(dir, []string{"destroy", "terraform", "null"}, env)
	if err == nil {
		t.Fatal("expected failure but command succeeded")
	}
	if !strings.Contains(string(stderr), "trusted") {
		t.Errorf("expected stderr to mention 'trusted', got: %s", stderr)
	}
}

func TestDestroy_FailsWhenConfirmFlagDoesNotMatch(t *testing.T) {
	t.Parallel()
	dir, env := helpers.CopyFixtureOnly(t, "plan")
	_, stderr, err := helpers.RunCLI(dir, []string{"init", "local"}, env)
	if err != nil {
		t.Fatalf("init local: %v\nstderr: %s", err, stderr)
	}
	env = append(env, "WINDSOR_CONTEXT=local")
	_, stderr, err = helpers.RunCLI(dir, []string{"destroy", "--confirm=wrong", "terraform"}, env)
	if err == nil {
		t.Fatal("expected failure but command succeeded")
	}
	if !strings.Contains(string(stderr), "confirmation failed") {
		t.Errorf("expected stderr to mention 'confirmation failed', got: %s", stderr)
	}
}

func TestDestroyTerraform_FailsForNonexistentComponent(t *testing.T) {
	t.Parallel()
	dir, env := helpers.CopyFixtureOnly(t, "plan")
	_, stderr, err := helpers.RunCLI(dir, []string{"init", "local"}, env)
	if err != nil {
		t.Fatalf("init local: %v\nstderr: %s", err, stderr)
	}
	env = append(env, "WINDSOR_CONTEXT=local")
	_, stderr, err = helpers.RunCLI(dir, []string{"destroy", "--confirm=nonexistent", "terraform", "nonexistent"}, env)
	if err == nil {
		t.Fatal("expected failure but command succeeded")
	}
	if !strings.Contains(string(stderr), "nonexistent") && !strings.Contains(string(stderr), "not found") && !strings.Contains(string(stderr), "error") {
		t.Errorf("expected stderr to mention the component or an error, got: %s", stderr)
	}
}

// TestDestroyTerraform_SkipsComponentWithEmptyState is the regression test for the field bug
// where a component with empty state (never applied, fully torn down already, or upstream
// destroy collapsed its cloud objects out from under it) would fail at refresh because module
// data sources couldn't read the missing cloud objects. The fix runs `terraform show -json`
// pre-refresh and short-circuits the entire flow when state is empty going in. We exercise
// this with the plan fixture's `null` component immediately after init: nothing has been
// applied, so its state is empty, and destroy must report "skipped" rather than running
// terraform refresh/destroy and possibly failing.
func TestDestroyTerraform_SkipsComponentWithEmptyState(t *testing.T) {
	t.Parallel()
	dir, env := helpers.CopyFixtureOnly(t, "plan")
	_, stderr, err := helpers.RunCLI(dir, []string{"init", "local"}, env)
	if err != nil {
		t.Fatalf("init local: %v\nstderr: %s", err, stderr)
	}
	env = append(env, "WINDSOR_CONTEXT=local")
	// Destroy without ever applying — state is empty for every component in the blueprint.
	_, stderr, err = helpers.RunCLI(dir, []string{"destroy", "--confirm=local", "terraform"}, env)
	if err != nil {
		t.Fatalf("destroy terraform on never-applied context: %v\nstderr: %s", err, stderr)
	}
	combined := string(stderr)
	if !strings.Contains(combined, "Skipped (empty state, nothing to destroy)") {
		t.Errorf("expected stderr to mention skipped components, got: %s", combined)
	}
	if !strings.Contains(combined, "null") {
		t.Errorf("expected the null component to appear in the skip list, got: %s", combined)
	}
}

// TestDestroy_FailsAtCredentialPreflightWhenAwsPlatformAndNoCreds is the regression test for
// the bug where a `windsor destroy` against an AWS context with expired/missing credentials
// would init every component and migrate state across several minutes before failing at the
// AWS provider. CheckAuth must fire up front so the failure surfaces before any terraform
// work runs.
func TestDestroy_FailsAtCredentialPreflightWhenAwsPlatformAndNoCreds(t *testing.T) {
	t.Parallel()
	dir, env := helpers.CopyFixtureOnly(t, "plan")
	_, stderr, err := helpers.RunCLI(dir, []string{"init", "aws-test", "--platform", "aws"}, env)
	if err != nil {
		t.Fatalf("init aws-test --platform aws: %v\nstderr: %s", err, stderr)
	}
	// Force CheckAuth into a known-failing state regardless of the host environment: clear
	// every ambient-credentials env var so the SDK chain has nothing to fall back to, and
	// blank AWS_PROFILE so the context-scoped profile is the only candidate (which doesn't
	// exist on a freshly-init'd context). Whether the host has the aws CLI installed or not,
	// the preflight must fail; the test only asserts that failure surfaces before terraform.
	env = append(env,
		"WINDSOR_CONTEXT=aws-test",
		"AWS_ACCESS_KEY_ID=",
		"AWS_SECRET_ACCESS_KEY=",
		"AWS_SESSION_TOKEN=",
		"AWS_WEB_IDENTITY_TOKEN_FILE=",
		"AWS_CONTAINER_CREDENTIALS_RELATIVE_URI=",
		"AWS_CONTAINER_CREDENTIALS_FULL_URI=",
		"AWS_PROFILE=",
	)
	stdout, stderr, err := helpers.RunCLI(dir, []string{"destroy", "--confirm=aws-test"}, env)
	if err == nil {
		t.Fatal("expected destroy to fail at credential preflight, got success")
	}
	combined := string(stderr) + string(stdout)
	// The preflight error surfaces the per-platform hint directly without an extra "error
	// validating credentials" wrapper — the hint already names the action ("aws sso login"
	// / "aws configure"), and stacking a generic prefix on top reads as panic. So we
	// assert on the actionable signal: an aws sso/configure remediation command appears.
	if !strings.Contains(combined, "aws sso login") && !strings.Contains(combined, "aws configure") {
		t.Errorf("expected stderr to include an aws sso/configure remediation hint, got: %s", combined)
	}
	// Terraform init must NOT have run — the whole point of the preflight is to fail before
	// the long init/migrate sequence the bug report described.
	if strings.Contains(combined, "Terraform has been successfully initialized") {
		t.Errorf("preflight must fail before terraform init runs, but init output appeared: %s", combined)
	}
	if strings.Contains(combined, "Migrating terraform state") {
		t.Errorf("preflight must fail before state migration, but migration output appeared: %s", combined)
	}
}

// TestDestroyTerraform_LocalBackendSkipsMigrationDance confirms that when a
// blueprint declares a backend terraform component but the configured backend is
// "local" (the default for fixtures without a cloud platform), the destroy flow
// takes the fast path: a single DestroyAllTerraform pass with no state
// migration. The two backend-aware components must both be reported in the
// empty-state skip list (never applied), proving the bulk destroy iterated
// them rather than getting peeled off by the migration dance. The "Migrating
// terraform state" progress line is the negative signal: it appears when the
// cmd layer activates the migrate-and-destroy-backend phase, which must NOT
// fire for a local backend.
func TestDestroyTerraform_LocalBackendSkipsMigrationDance(t *testing.T) {
	t.Parallel()
	dir, env := helpers.CopyFixtureOnly(t, "backend-first")
	_, stderr, err := helpers.RunCLI(dir, []string{"init", "local"}, env)
	if err != nil {
		t.Fatalf("init local: %v\nstderr: %s", err, stderr)
	}
	env = append(env, "WINDSOR_CONTEXT=local")
	stdout, stderr, err := helpers.RunCLI(dir, []string{"destroy", "--confirm=local", "terraform"}, env)
	if err != nil {
		t.Fatalf("destroy terraform: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	combined := string(stdout) + string(stderr)
	if strings.Contains(combined, "Migrating terraform state") {
		t.Errorf("expected no state migration for local-backend destroy, but migration progress line appeared:\n%s", combined)
	}
	if !strings.Contains(combined, "Skipped (empty state, nothing to destroy)") {
		t.Errorf("expected stderr to mention skipped components, got:\n%s", combined)
	}
	if !strings.Contains(combined, "backend") {
		t.Errorf("expected backend component in skip list, got:\n%s", combined)
	}
	if !strings.Contains(combined, "null") {
		t.Errorf("expected null component in skip list, got:\n%s", combined)
	}
}
