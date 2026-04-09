//go:build integration
// +build integration

package integration

import (
	"strings"
	"testing"

	"github.com/windsorcli/cli/integration/helpers"
)

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
