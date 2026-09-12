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

func TestContextFlag_OverridesActiveContextForOneInvocation(t *testing.T) {
	t.Parallel()
	dir, env := helpers.CopyFixtureOnly(t, "default")
	helpers.MarkAsGitRepo(t, dir)
	_, stderr, err := helpers.RunCLI(dir, []string{"init", "default"}, env)
	if err != nil {
		t.Fatalf("init default: %v\nstderr: %s", err, stderr)
	}

	stdout, stderr, err := helpers.RunCLI(dir, []string{"get", "context"}, env)
	if err != nil {
		t.Fatalf("get context: %v\nstderr: %s", err, stderr)
	}
	if got := strings.TrimSpace(string(stdout)); got != "default" {
		t.Fatalf("expected baseline context 'default', got %q", got)
	}

	stdout, stderr, err = helpers.RunCLI(dir, []string{"get", "context", "--context", "test"}, env)
	if err != nil {
		t.Fatalf("get context --context test: %v\nstderr: %s", err, stderr)
	}
	if got := strings.TrimSpace(string(stdout)); got != "test" {
		t.Errorf("expected --context to override to 'test', got %q", got)
	}

	stdout, stderr, err = helpers.RunCLI(dir, []string{"get", "context"}, env)
	if err != nil {
		t.Fatalf("get context (after override): %v\nstderr: %s", err, stderr)
	}
	if got := strings.TrimSpace(string(stdout)); got != "default" {
		t.Errorf("expected --context override not to persist to .windsor/context, got %q", got)
	}
}

func TestContextFlag_DoesNotBypassTrustedDirectoryCheck(t *testing.T) {
	t.Parallel()
	dir, env := helpers.CopyFixtureOnly(t, "default")
	_, stderr, err := helpers.RunCLI(dir, []string{"plan", "terraform", "cluster", "--context", "test"}, env)
	if err == nil {
		t.Fatal("expected failure but command succeeded")
	}
	if !strings.Contains(string(stderr), "trusted") {
		t.Errorf("expected stderr to mention 'trusted', got: %s", stderr)
	}
}
