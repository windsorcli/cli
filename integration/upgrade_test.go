//go:build integration
// +build integration

package integration

import (
	"strings"
	"testing"

	"github.com/windsorcli/cli/integration/helpers"
)

// =============================================================================
// Integration Tests — upgrade (blueprint)
// =============================================================================

func TestUpgrade_FailsWhenNotInTrustedDirectory(t *testing.T) {
	t.Parallel()
	dir, env := helpers.CopyFixtureOnly(t, "plan")
	// Bare `upgrade` now runs the blueprint flow, so it must enforce the trusted-directory
	// gate. A no-op parent would have printed help and exited 0 instead.
	_, stderr, err := helpers.RunCLI(dir, []string{"upgrade"}, env)
	if err == nil {
		t.Fatal("expected failure but command succeeded")
	}
	if !strings.Contains(string(stderr), "trusted") {
		t.Errorf("expected stderr to mention 'trusted', got: %s", stderr)
	}
}

func TestUpgrade_RunsBlueprintFlowInLocalContext(t *testing.T) {
	t.Parallel()
	dir, env := helpers.CopyFixtureOnly(t, "plan")
	helpers.MarkAsGitRepo(t, dir)
	if _, stderr, err := helpers.RunCLI(dir, []string{"init", "local"}, env); err != nil {
		t.Fatalf("init local: %v\nstderr: %s", err, stderr)
	}
	env = append(env, "WINDSOR_CONTEXT=local")

	// Bare `upgrade` drives terraform + the blueprint install/wait/prune flow. With no live
	// cluster the install/wait phase fails, which proves the command reaches real
	// provisioning rather than dispatching to the talos subcommands or printing help.
	_, stderr, err := helpers.RunCLI(dir, []string{"upgrade"}, env)
	if err == nil {
		t.Fatal("expected upgrade to fail without a live cluster, got success")
	}
	out := string(stderr)
	if strings.Contains(out, "unknown command") || strings.Contains(out, "Available Commands") {
		t.Errorf("expected bare upgrade to run the blueprint flow, not print help, got: %s", out)
	}
}

func TestUpgrade_SourceRejectsUndeclaredSource(t *testing.T) {
	t.Parallel()
	dir, env := helpers.CopyFixtureOnly(t, "plan")
	helpers.MarkAsGitRepo(t, dir)
	if _, stderr, err := helpers.RunCLI(dir, []string{"init", "local"}, env); err != nil {
		t.Fatalf("init local: %v\nstderr: %s", err, stderr)
	}
	env = append(env, "WINDSOR_CONTEXT=local")

	// --source must refuse a source the blueprint does not declare, before any cluster work —
	// adding a source is a structural edit to blueprint.yaml, not a retarget.
	_, stderr, err := helpers.RunCLI(dir, []string{"upgrade", "--source", "bogus=oci://ghcr.io/windsorcli/bogus:v1.0.0"}, env)
	if err == nil {
		t.Fatal("expected upgrade to reject an undeclared --source, got success")
	}
	if !strings.Contains(string(stderr), "not declared") {
		t.Errorf("expected an undeclared-source error, got: %s", stderr)
	}
}

func TestUpgrade_AcceptsYesFlag(t *testing.T) {
	t.Parallel()
	dir, env := helpers.CopyFixtureOnly(t, "plan")
	helpers.MarkAsGitRepo(t, dir)
	if _, stderr, err := helpers.RunCLI(dir, []string{"init", "local"}, env); err != nil {
		t.Fatalf("init local: %v\nstderr: %s", err, stderr)
	}
	env = append(env, "WINDSOR_CONTEXT=local")
	_, stderr, err := helpers.RunCLI(dir, []string{"upgrade", "--yes"}, env)
	if strings.Contains(string(stderr), "unknown flag") {
		t.Errorf("--yes should be a recognised flag on upgrade, got: %s", stderr)
	}
	_ = err // fails without a live cluster; the flag must be accepted
}

// =============================================================================
// Integration Tests — upgrade cluster
// =============================================================================

func TestUpgradeCluster_FailsWhenNotInTrustedDirectory(t *testing.T) {
	t.Parallel()
	dir, env := helpers.CopyFixtureOnly(t, "plan")
	_, stderr, err := helpers.RunCLI(dir, []string{"upgrade", "cluster", "--nodes", "10.0.0.1", "--image", "ghcr.io/siderolabs/talos:v1.9.0"}, env)
	if err == nil {
		t.Fatal("expected failure but command succeeded")
	}
	if !strings.Contains(string(stderr), "trusted") {
		t.Errorf("expected stderr to mention 'trusted', got: %s", stderr)
	}
}

func TestUpgradeCluster_FailsWithMissingNodesFlag(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "plan")
	_, _, err := helpers.RunCLI(dir, []string{"upgrade", "cluster", "--image", "ghcr.io/siderolabs/talos:v1.9.0"}, env)
	if err == nil {
		t.Fatal("expected failure for missing --nodes flag, got success")
	}
}

func TestUpgradeCluster_FailsWithMissingImageFlag(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "plan")
	_, _, err := helpers.RunCLI(dir, []string{"upgrade", "cluster", "--nodes", "10.0.0.1"}, env)
	if err == nil {
		t.Fatal("expected failure for missing --image flag, got success")
	}
}

func TestUpgradeCluster_FailsWhenConfigNotLoaded(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "plan")
	env = append(env, "WINDSOR_CONTEXT=nonexistent")
	_, stderr, err := helpers.RunCLI(dir, []string{"upgrade", "cluster", "--nodes", "10.0.0.1", "--image", "ghcr.io/siderolabs/talos:v1.9.0"}, env)
	if err == nil {
		t.Fatal("expected failure but command succeeded")
	}
	if !strings.Contains(string(stderr), "upgrade") {
		t.Errorf("expected stderr to mention 'upgrade', got: %s", stderr)
	}
}

// =============================================================================
// Integration Tests — upgrade node
// =============================================================================

func TestUpgradeNode_FailsWhenNotInTrustedDirectory(t *testing.T) {
	t.Parallel()
	dir, env := helpers.CopyFixtureOnly(t, "plan")
	_, stderr, err := helpers.RunCLI(dir, []string{"upgrade", "node", "--node", "10.0.0.1", "--image", "ghcr.io/siderolabs/talos:v1.9.0"}, env)
	if err == nil {
		t.Fatal("expected failure but command succeeded")
	}
	if !strings.Contains(string(stderr), "trusted") {
		t.Errorf("expected stderr to mention 'trusted', got: %s", stderr)
	}
}

func TestUpgradeNode_FailsWithMissingNodeFlag(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "plan")
	_, _, err := helpers.RunCLI(dir, []string{"upgrade", "node", "--image", "ghcr.io/siderolabs/talos:v1.9.0"}, env)
	if err == nil {
		t.Fatal("expected failure for missing --node flag, got success")
	}
}

func TestUpgradeNode_FailsWithMissingImageFlag(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "plan")
	_, _, err := helpers.RunCLI(dir, []string{"upgrade", "node", "--node", "10.0.0.1"}, env)
	if err == nil {
		t.Fatal("expected failure for missing --image flag, got success")
	}
}

func TestUpgradeNode_FailsWhenConfigNotLoaded(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "plan")
	env = append(env, "WINDSOR_CONTEXT=nonexistent")
	_, stderr, err := helpers.RunCLI(dir, []string{"upgrade", "node", "--node", "10.0.0.1", "--image", "ghcr.io/siderolabs/talos:v1.9.0"}, env)
	if err == nil {
		t.Fatal("expected failure but command succeeded")
	}
	if !strings.Contains(string(stderr), "upgrade") {
		t.Errorf("expected stderr to mention 'upgrade', got: %s", stderr)
	}
}
