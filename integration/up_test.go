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
