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

func TestConfigureNetwork_FailsWhenNotTrusted(t *testing.T) {
	t.Parallel()
	dir, env := helpers.CopyFixtureOnly(t, "default")
	env = append(env, "WINDSOR_CONTEXT=default")
	_, stderr, err := helpers.RunCLI(dir, []string{"configure", "network"}, env)
	if err == nil {
		t.Fatal("expected configure network to fail when not in trusted dir")
	}
	if !strings.Contains(string(stderr), "trusted") {
		t.Errorf("expected stderr to contain 'trusted', got: %s", stderr)
	}
}

func TestConfigureNetwork_SucceedsAfterInit(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "default")
	env = append(env, "WINDSOR_CONTEXT=default")
	_, stderr, err := helpers.RunCLI(dir, []string{"configure", "network"}, env)
	if err != nil {
		t.Fatalf("configure network: %v\nstderr: %s", err, stderr)
	}
}

func TestConfigureNetwork_SucceedsWithDnsAddressFlag(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "default")
	env = append(env, "WINDSOR_CONTEXT=default")
	_, stderr, err := helpers.RunCLI(dir, []string{"configure", "network", "--dns-address=10.5.0.2"}, env)
	if err != nil {
		t.Fatalf("configure network --dns-address: %v\nstderr: %s", err, stderr)
	}
}

// TestConfigureNetwork_FailsBeforeWorkstationProvisioned exercises the new precondition added
// alongside --dry-run and --revert: if the workstation.yaml state file that 'windsor up' would
// write hasn't been created yet for this context, configure network refuses with an operator-
// facing message pointing at 'windsor up'.
func TestConfigureNetwork_FailsBeforeWorkstationProvisioned(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "default")
	// Delete the workstation.yaml that 'init' would otherwise leave behind so the precondition
	// fires. The init command writes .windsor/context = "local" (the default), so we target
	// that context.
	if err := os.Remove(filepath.Join(dir, ".windsor", "contexts", "local", "workstation.yaml")); err != nil {
		t.Fatalf("setup: removing workstation.yaml: %v", err)
	}
	_, stderr, err := helpers.RunCLI(dir, []string{"configure", "network"}, env)
	if err == nil {
		t.Fatal("expected configure network to fail when workstation.yaml is missing")
	}
	if !strings.Contains(string(stderr), "has not been provisioned yet") {
		t.Errorf("expected stderr to mention precondition message, got: %s", stderr)
	}
	if !strings.Contains(string(stderr), "windsor up") {
		t.Errorf("expected stderr to point at 'windsor up', got: %s", stderr)
	}
}

// TestConfigureNetwork_DryRunPrintsPlanAndDoesNotConfigure exercises the new --dry-run flag.
// In the integration environment nothing is actually installed and the cluster runtime is the
// default (docker-desktop on macOS), so the plan reports "nothing pending" — verifying both
// the flag's wiring and that no privileged work runs.
func TestConfigureNetwork_DryRunPrintsPlanAndDoesNotConfigure(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "default")
	stdout, stderr, err := helpers.RunCLI(dir, []string{"configure", "network", "--dry-run"}, env)
	if err != nil {
		t.Fatalf("configure network --dry-run: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(string(stdout), "nothing pending") {
		t.Errorf("expected stdout to report 'nothing pending' (no install in test env), got stdout=%q stderr=%q", stdout, stderr)
	}
}

// Note: a direct integration test of `configure network --revert` would require the test
// environment to handle the sudo prompts that revert needs (the per-OS RevertHostRoute /
// RevertDNS still invoke ExecSudo even when the underlying resource is absent — only the
// EXIT outcome is tolerant, not the elevation step). That's correct behavior for the
// operator's explicit-revert path but not testable without an askpass helper in CI. The flag's
// wiring is exercised indirectly by TestConfigureNetwork_DryRunTakesPrecedenceOverRevert
// below; the actual revert logic is covered by unit tests in pkg/workstation/network/ and
// pkg/workstation/workstation_test.go.

// TestConfigureNetwork_DryRunTakesPrecedenceOverRevert documents the ordering in the RunE
// branch: when both --dry-run and --revert are passed, dry-run wins (it short-circuits before
// the revert branch). Operators relying on this composition should see the plan, not have
// state modified.
func TestConfigureNetwork_DryRunTakesPrecedenceOverRevert(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "default")
	stdout, stderr, err := helpers.RunCLI(dir, []string{"configure", "network", "--dry-run", "--revert"}, env)
	if err != nil {
		t.Fatalf("configure network --dry-run --revert: %v\nstderr: %s", err, stderr)
	}
	// "nothing pending" is what --dry-run prints; --revert would print "network: skipped..."
	// or "dns: reverted" status lines via showStatus=true.
	if !strings.Contains(string(stdout), "nothing pending") {
		t.Errorf("expected dry-run output ('nothing pending') when both flags are passed, got stdout=%q stderr=%q", stdout, stderr)
	}
}
