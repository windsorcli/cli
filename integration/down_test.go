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

// TestDown_NoOpWhenWorkstationDisabled verifies that windsor down exits successfully
// and prints a hint when no workstation is configured.
func TestDown_NoOpWhenWorkstationDisabled(t *testing.T) {
	t.Parallel()
	dir, env := helpers.CopyFixtureOnly(t, "up")
	helpers.MarkAsGitRepo(t, dir)
	_, stderr, err := helpers.RunCLI(dir, []string{"init", "staging"}, env)
	if err != nil {
		t.Fatalf("init staging: %v\nstderr: %s", err, stderr)
	}
	env = append(env, "WINDSOR_CONTEXT=staging")
	_, stderr, err = helpers.RunCLI(dir, []string{"down"}, env)
	if err != nil {
		t.Fatalf("expected no-op success when workstation is disabled, got error: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(string(stderr), "windsor destroy") {
		t.Errorf("expected stderr to name 'windsor destroy', got: %s", stderr)
	}
}

// TestDown_DestroyFailsWhenWorkstationDisabled verifies that windsor down --destroy
// refuses a context with no workstation and points at windsor destroy.
func TestDown_DestroyFailsWhenWorkstationDisabled(t *testing.T) {
	t.Parallel()
	dir, env := helpers.CopyFixtureOnly(t, "up")
	helpers.MarkAsGitRepo(t, dir)
	_, stderr, err := helpers.RunCLI(dir, []string{"init", "staging"}, env)
	if err != nil {
		t.Fatalf("init staging: %v\nstderr: %s", err, stderr)
	}
	env = append(env, "WINDSOR_CONTEXT=staging")
	_, stderr, err = helpers.RunCLI(dir, []string{"down", "--destroy", "--confirm=staging"}, env)
	if err == nil {
		t.Fatal("expected failure when workstation is disabled, got success")
	}
	if !strings.Contains(string(stderr), "Run 'windsor destroy'") {
		t.Errorf("expected stderr to name 'windsor destroy', got: %s", stderr)
	}
}

// TestDown_RefusesDestroyFlagsWithoutDestroy verifies that windsor down refuses
// --confirm and --continue when --destroy is absent.
func TestDown_RefusesDestroyFlagsWithoutDestroy(t *testing.T) {
	t.Parallel()
	for _, arg := range []string{"--confirm=local", "--continue"} {
		t.Run(arg, func(t *testing.T) {
			t.Parallel()
			dir, env := helpers.CopyFixtureOnly(t, "up")
			_, stderr, err := helpers.RunCLI(dir, []string{"down", arg}, env)
			if err == nil {
				t.Fatal("expected failure, got success")
			}
			if !strings.Contains(string(stderr), "--destroy") {
				t.Errorf("expected stderr to name --destroy, got: %s", stderr)
			}
		})
	}
}
