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

func TestCheckNodeHealth_ContextFlagUsesContextKubeconfig(t *testing.T) {
	t.Parallel()
	dir, env := helpers.CopyFixtureOnly(t, "default")
	helpers.MarkAsGitRepo(t, dir)
	if _, stderr, err := helpers.RunCLI(dir, []string{"init", "test", "--set", "cluster.driver=talos"}, env); err != nil {
		t.Fatalf("init test: %v\nstderr: %s", err, stderr)
	}
	if _, stderr, err := helpers.RunCLI(dir, []string{"init", "default"}, env); err != nil {
		t.Fatalf("init default: %v\nstderr: %s", err, stderr)
	}
	helpers.WriteStaleKubeconfig(t, dir, "test")
	otherKubeconfig := filepath.Join(t.TempDir(), "config")
	other := []byte(`apiVersion: v1
kind: Config
clusters:
  - name: other
    cluster:
      server: https://127.0.0.1:2
contexts:
  - name: other
    context:
      cluster: other
current-context: other
`)
	if err := os.WriteFile(otherKubeconfig, other, 0o644); err != nil {
		t.Fatalf("write other kubeconfig: %v", err)
	}
	env = append(env, "KUBECONFIG="+otherKubeconfig)

	_, stderr, err := helpers.RunCLI(dir, []string{"check", "node-health", "-c", "test", "--k8s-endpoint", "--timeout", "2s"}, env)
	if err == nil {
		t.Fatal("expected the health check to fail against an unreachable API server")
	}
	if !strings.Contains(string(stderr), "127.0.0.1:1") {
		t.Errorf("expected the check to probe the server in the test context kubeconfig, got: %s", stderr)
	}
	if strings.Contains(string(stderr), "127.0.0.1:2") {
		t.Errorf("expected the check to ignore the inherited KUBECONFIG, got: %s", stderr)
	}
}
