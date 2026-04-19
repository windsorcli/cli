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

// TestUp_NoOpWhenWorkstationDisabled verifies that windsor up exits successfully
// and prints a descriptive message when no workstation is configured.
func TestUp_NoOpWhenWorkstationDisabled(t *testing.T) {
	t.Parallel()
	dir, env := helpers.CopyFixtureOnly(t, "up")
	_, stderr, err := helpers.RunCLI(dir, []string{"init", "staging"}, env)
	if err != nil {
		t.Fatalf("init staging: %v\nstderr: %s", err, stderr)
	}
	env = append(env, "WINDSOR_CONTEXT=staging")
	_, stderr, err = helpers.RunCLI(dir, []string{"up"}, env)
	if err != nil {
		t.Fatalf("expected no-op success when workstation is disabled, got error: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(string(stderr), "workstation") {
		t.Errorf("expected stderr to mention 'workstation', got: %s", stderr)
	}
}

// TestUp_FailsWhenNotInTrustedDirectory verifies that windsor up rejects runs
// outside a trusted directory.
func TestUp_FailsWhenNotInTrustedDirectory(t *testing.T) {
	t.Parallel()
	dir, env := helpers.CopyFixtureOnly(t, "up")
	_, stderr, err := helpers.RunCLI(dir, []string{"up"}, env)
	if err == nil {
		t.Fatal("expected failure but command succeeded")
	}
	if !strings.Contains(string(stderr), "trusted") {
		t.Errorf("expected stderr to mention 'trusted', got: %s", stderr)
	}
}

// TestUp_AcceptsWaitFlag verifies that --wait is a recognised flag and does not
// cause an "unknown flag" error in the no-op path.
func TestUp_AcceptsWaitFlag(t *testing.T) {
	t.Parallel()
	dir, env := helpers.CopyFixtureOnly(t, "up")
	_, stderr, err := helpers.RunCLI(dir, []string{"init", "staging"}, env)
	if err != nil {
		t.Fatalf("init staging: %v\nstderr: %s", err, stderr)
	}
	env = append(env, "WINDSOR_CONTEXT=staging")
	_, stderr, err = helpers.RunCLI(dir, []string{"up", "--wait"}, env)
	if err != nil {
		t.Fatalf("expected --wait to be accepted, got error: %v\nstderr: %s", err, stderr)
	}
}

// TestUp_AcceptsSetFlag verifies the new --set flag on up parses and persists
// arbitrary key=value overrides, giving windsor up feature parity with
// windsor init for non-workstation config changes.
func TestUp_AcceptsSetFlag(t *testing.T) {
	t.Parallel()
	dir, env := helpers.CopyFixtureOnly(t, "up")
	_, stderr, err := helpers.RunCLI(dir, []string{"init", "staging"}, env)
	if err != nil {
		t.Fatalf("init staging: %v\nstderr: %s", err, stderr)
	}

	env = append(env, "WINDSOR_CONTEXT=staging")
	if _, stderr, err = helpers.RunCLI(dir, []string{"up", "--set", "dns.enabled=false", "--set", "custom.key=hello"}, env); err != nil {
		t.Fatalf("up --set: %v\nstderr: %s", err, stderr)
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

// TestUp_FailsOnMalformedSetFlag verifies that up returns a clear error when a
// --set entry is missing the equals sign, matching up's strict --set handling.
func TestUp_FailsOnMalformedSetFlag(t *testing.T) {
	t.Parallel()
	dir, env := helpers.CopyFixtureOnly(t, "up")
	_, stderr, err := helpers.RunCLI(dir, []string{"init", "staging"}, env)
	if err != nil {
		t.Fatalf("init staging: %v\nstderr: %s", err, stderr)
	}

	env = append(env, "WINDSOR_CONTEXT=staging")
	_, stderr, err = helpers.RunCLI(dir, []string{"up", "--set", "no-equals-here"}, env)
	if err == nil {
		t.Fatal("expected failure for malformed --set, got success")
	}
	if !strings.Contains(string(stderr), "invalid --set format") {
		t.Errorf("expected stderr to mention 'invalid --set format', got: %s", stderr)
	}
}

// TestUp_RecognisesVmDriverAndBlueprintFlags verifies that --vm-driver and
// --blueprint are parsed flags (no "unknown flag" error when included alongside
// --help). These flags ultimately trigger workstation startup, which requires
// real infrastructure, so we verify recognition via --help rather than
// end-to-end execution. Persistence semantics are covered by unit tests and by
// init integration tests.
func TestUp_RecognisesVmDriverAndBlueprintFlags(t *testing.T) {
	t.Parallel()
	dir, env := helpers.CopyFixtureOnly(t, "up")
	stdout, stderr, err := helpers.RunCLI(dir, []string{"up", "--help"}, env)
	if err != nil {
		t.Fatalf("up --help: %v\nstderr: %s", err, stderr)
	}
	for _, flag := range []string{"--vm-driver", "--platform", "--blueprint", "--set"} {
		if !strings.Contains(string(stdout), flag) {
			t.Errorf("expected up --help to advertise %s flag, got:\n%s", flag, stdout)
		}
	}
}
