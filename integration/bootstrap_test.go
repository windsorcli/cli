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

// TestBootstrap_RecognisesFlags verifies bootstrap's minimal flag surface: it
// advertises --platform, --blueprint, and --set, and explicitly does NOT
// advertise flags dropped from the bootstrap design (--wait, --vm-driver,
// --backend, --aws-profile). This guards against accidental flag reintroduction.
func TestBootstrap_RecognisesFlags(t *testing.T) {
	t.Parallel()
	dir, env := helpers.CopyFixtureOnly(t, "bootstrap")
	stdout, stderr, err := helpers.RunCLI(dir, []string{"bootstrap", "--help"}, env)
	if err != nil {
		t.Fatalf("bootstrap --help: %v\nstderr: %s", err, stderr)
	}
	help := string(stdout)
	for _, flag := range []string{"--platform", "--blueprint", "--set"} {
		if !strings.Contains(help, flag) {
			t.Errorf("expected bootstrap --help to advertise %s, got:\n%s", flag, help)
		}
	}
	for _, flag := range []string{"--wait", "--vm-driver", "--backend", "--aws-profile", "--aws-endpoint-url"} {
		if strings.Contains(help, flag) {
			t.Errorf("expected bootstrap --help NOT to advertise %s (dropped from flag surface), got:\n%s", flag, help)
		}
	}
}

// TestBootstrap_FailsOnMalformedSetFlag verifies bootstrap rejects --set entries
// missing the = separator with a clear error, matching up's strict --set handling.
func TestBootstrap_FailsOnMalformedSetFlag(t *testing.T) {
	t.Parallel()
	dir, env := helpers.CopyFixtureOnly(t, "bootstrap")
	_, stderr, err := helpers.RunCLI(dir, []string{"bootstrap", "--set", "no-equals-here"}, env)
	if err == nil {
		t.Fatal("expected failure for malformed --set, got success")
	}
	if !strings.Contains(string(stderr), "invalid --set format") {
		t.Errorf("expected stderr to mention 'invalid --set format', got: %s", stderr)
	}
}

// TestBootstrap_PersistsSetValues exercises the configuration pipeline end-to-end
// up through SaveConfig. Bootstrap will ultimately fail at the kubernetes Install
// step because the integration harness has no cluster, but --set values must have
// been persisted to values.yaml before that failure. This guards the contract
// that bootstrap persists user overrides regardless of downstream failures.
func TestBootstrap_PersistsSetValues(t *testing.T) {
	t.Parallel()
	dir, env := helpers.CopyFixtureOnly(t, "bootstrap")
	// Run bootstrap; failure at the install step is expected and tolerated. Capture the
	// exit so an early/unexpected failure (missing binary, panic, etc.) surfaces in test
	// output alongside the later "values.yaml not found" assertion rather than hiding it.
	stdout, stderr, err := helpers.RunCLI(dir, []string{"bootstrap", "staging", "--set", "dns.enabled=false", "--set", "custom.key=hello"}, env)
	if err != nil {
		t.Logf("bootstrap exited: %v (tolerated)\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	valuesPath := filepath.Join(dir, "contexts", "staging", "values.yaml")
	data, err := os.ReadFile(valuesPath)
	if err != nil {
		t.Fatalf("expected values.yaml at %s, got %v", valuesPath, err)
	}
	body := string(data)
	if !strings.Contains(body, "enabled: false") {
		t.Errorf("expected dns.enabled=false persisted, got:\n%s", body)
	}
	if !strings.Contains(body, "key: hello") {
		t.Errorf("expected custom.key=hello persisted, got:\n%s", body)
	}
}

// TestBootstrap_GlobalModeExitsCleanlyWhenPlanDeclined verifies the global-mode
// plan-confirm prompt: bootstrap run from a directory without a windsor.yaml falls
// back to global mode, prints a plan, and prompts to apply. Empty stdin (non-
// interactive without --yes) is treated as "no" and exits cleanly — the context
// has already been configured by this point, so declining the apply is a valid
// no-op rather than a failure exit. aws.region is required by the aws platform
// facets (validator added in core after this test was written); supply it via
// --set so the plan can render without tripping the required-values gate.
func TestBootstrap_GlobalModeExitsCleanlyWhenPlanDeclined(t *testing.T) {
	t.Parallel()
	workDir := t.TempDir()
	homeDir := t.TempDir()
	env := []string{
		"HOME=" + homeDir,
		"USERPROFILE=" + homeDir,
		"PATH=" + os.Getenv("PATH"),
	}
	stdout, stderr, err := helpers.RunCLI(workDir, []string{"bootstrap", "--platform", "aws", "--set", "aws.region=us-east-1"}, env)
	if err != nil {
		t.Fatalf("expected clean exit when plan-confirm is declined, got %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	if !strings.Contains(string(stderr), "Apply skipped") {
		t.Errorf("expected stderr to mention apply was skipped, got: %s", stderr)
	}
	// Side effect: the global config dir must have been materialized as part of
	// falling back to global mode, confirming the global-mode codepath was hit
	// and the context was configured before the prompt fired.
	globalDir := filepath.Join(homeDir, ".config", "windsor")
	if _, statErr := os.Stat(globalDir); os.IsNotExist(statErr) {
		t.Errorf("expected global config dir at %s, not found (global-mode fallback never triggered)", globalDir)
	}
}

// TestBootstrap_DanceIsScopedToTier exercises the always-on tier pivot end to end
// against the backend-first fixture with a remote backend configured. Stage 1 must
// apply only the tier (backend) against local state, Stage 2 must attempt to
// migrate state to the configured remote, and Stage 3 (non-tier — here, the "null"
// module) must NOT run once migration fails. The assertion proves the dance is
// scoped to the tier rather than the legacy "apply everything local then migrate
// everything" path.
func TestBootstrap_DanceIsScopedToTier(t *testing.T) {
	t.Parallel()
	dir, env := helpers.CopyFixtureOnly(t, "backend-first")
	helpers.MarkAsGitRepo(t, dir)
	if _, stderr, err := helpers.RunCLI(dir, []string{"init", "local", "--set", "terraform.backend.type=s3"}, env); err != nil {
		t.Fatalf("init local: %v\nstderr: %s", err, stderr)
	}
	env = append(env, "WINDSOR_CONTEXT=local")

	stdout, stderr, err := helpers.RunCLI(dir, []string{"bootstrap", "--yes"}, env)
	if err == nil {
		t.Fatalf("expected bootstrap to fail at migrate (no real s3), got success\nstdout: %s\nstderr: %s", stdout, stderr)
	}
	combined := string(stdout) + string(stderr)

	// Stage 1: only the tier was applied locally. The "null" module is non-tier
	// and must not appear in Stage 1's apply output.
	if !strings.Contains(combined, "Applying backend") {
		t.Errorf("expected tier apply line for backend, got:\n%s", combined)
	}
	// Stage 2 was reached.
	if !strings.Contains(combined, "Migrating terraform state") {
		t.Errorf("expected migrate stage to be reached, got:\n%s", combined)
	}
	// Stage 3 was NOT reached because migrate failed. "Applying null" is the
	// signature of a non-tier component apply; its presence here would mean
	// the dance ran against the full stack instead of just the tier.
	if strings.Contains(combined, "Applying null") {
		t.Errorf("Stage 3 must not run after migrate failure; saw 'Applying null':\n%s", combined)
	}
}

// TestBootstrap_WritesContextFileOnFirstRun verifies the positional context arg
// persists to .windsor/context even when bootstrap later fails at the install
// step. Users on other machines need this file to resolve the same context,
// so context persistence must happen early in the bootstrap sequence.
func TestBootstrap_WritesContextFileOnFirstRun(t *testing.T) {
	t.Parallel()
	dir, env := helpers.CopyFixtureOnly(t, "bootstrap")
	// Run bootstrap; failure at the install step is expected and tolerated. Capture the
	// exit so an early/unexpected failure (missing binary, panic, etc.) surfaces in test
	// output alongside the later ".windsor/context not found" assertion rather than hiding it.
	stdout, stderr, err := helpers.RunCLI(dir, []string{"bootstrap", "staging"}, env)
	if err != nil {
		t.Logf("bootstrap exited: %v (tolerated)\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	contextPath := filepath.Join(dir, ".windsor", "context")
	data, err := os.ReadFile(contextPath)
	if err != nil {
		t.Fatalf("expected .windsor/context at %s, got %v", contextPath, err)
	}
	if strings.TrimSpace(string(data)) != "staging" {
		t.Errorf("expected .windsor/context to contain 'staging', got: %q", data)
	}
}
