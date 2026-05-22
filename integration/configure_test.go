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
// A clean exit proves no privileged work ran (sudo would hang or fail non-interactively in CI).
// The plan body is host-dependent — a fresh macOS host has no /etc/resolver entry so the DNS
// resolver shows pending, whereas a Linux host whose resolv.conf isn't a systemd-resolved stub
// reports nothing pending — so the test asserts the output is a plan, not a specific host's plan.
func TestConfigureNetwork_DryRunPrintsPlanAndDoesNotConfigure(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "default")
	stdout, stderr, err := helpers.RunCLI(dir, []string{"configure", "network", "--dry-run"}, env)
	if err != nil {
		t.Fatalf("configure network --dry-run: %v\nstderr: %s", err, stderr)
	}
	if !isDryRunPlan(string(stdout)) {
		t.Errorf("expected stdout to be a dry-run plan, got stdout=%q stderr=%q", stdout, stderr)
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
	// dry-run prints a plan; --revert would print "network: skipped..." or "dns: reverted"
	// status lines via showStatus=true. Seeing a plan confirms dry-run short-circuited first.
	if !isDryRunPlan(string(stdout)) {
		t.Errorf("expected dry-run plan (not revert status) when both flags are passed, got stdout=%q stderr=%q", stdout, stderr)
	}
}

// isDryRunPlan reports whether out is a 'configure network --dry-run' plan: the "nothing pending"
// sentinel or a tabwriter row keyed by a known change kind. Used to assert dry-run behavior
// without depending on which host the test runs on.
func isDryRunPlan(out string) bool {
	if strings.Contains(out, "nothing pending") {
		return true
	}
	for _, kind := range []string{"host-route", "vm-forward", "dns-resolver"} {
		if strings.Contains(out, kind) {
			return true
		}
	}
	return false
}
