//go:build integration
// +build integration

package integration

import (
	"os"
	"path/filepath"
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

// TestDestroy_TolerantOfMisorderedBackend is the regression test for the catch-22 where
// blueprint.ValidateComposedBlueprint blocked windsor destroy on a deployed-but-misordered
// blueprint. We init against the correctly-ordered backend-first fixture (so init passes
// validation and lays modules + composed blueprint to disk), then mutate the composed
// blueprint.yaml to swap component order — simulating "operator deployed cleanly, then a
// later edit moved backend off position 0." Destroy must still succeed: the prepareProject
// path opts out of structural validation via SetSkipValidation(true), and the destroy
// lifecycle iterates components by GetID() rather than blueprint position so order does
// not affect correctness for teardown.
func TestDestroy_TolerantOfMisorderedBackend(t *testing.T) {
	t.Parallel()
	dir, env := helpers.CopyFixtureOnly(t, "backend-first")
	if _, stderr, err := helpers.RunCLI(dir, []string{"init", "local"}, env); err != nil {
		t.Fatalf("init local: %v\nstderr: %s", err, stderr)
	}
	env = append(env, "WINDSOR_CONTEXT=local")

	// Mutate the composed blueprint to put backend after null. Mirror what an operator
	// would see if they hand-edited blueprint.yaml after deployment.
	composedPath := filepath.Join(dir, "contexts", "local", "blueprint.yaml")
	misordered := []byte(`kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: backend-first
  description: Misordered for regression test.
terraform:
  - path: "null"
  - path: "backend"
`)
	if err := os.WriteFile(composedPath, misordered, 0o644); err != nil {
		t.Fatalf("rewrite composed blueprint: %v", err)
	}

	stdout, stderr, err := helpers.RunCLI(dir, []string{"destroy", "--confirm=local", "terraform"}, env)
	if err != nil {
		t.Fatalf("destroy must tolerate misordered backend, got: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	combined := string(stdout) + string(stderr)
	// Prove validation did NOT fire — its prose would mention "first item in
	// terraformComponents" or "currently at position".
	if strings.Contains(combined, "first item in terraformComponents") || strings.Contains(combined, "currently at position") {
		t.Errorf("destroy must skip structural validation; saw validator prose in output:\n%s", combined)
	}
}
