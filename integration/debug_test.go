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

// TestDebug_FlagEnablesDebugLogging verifies that --debug turns on the
// "[debug]" diagnostic lines windsor writes to stderr while resolving
// environment variables.
func TestDebug_FlagEnablesDebugLogging(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "default")

	_, stderr, err := helpers.RunCLI(dir, []string{"env", "--debug"}, env)
	if err != nil {
		t.Fatalf("env --debug: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(string(stderr), "[debug]") {
		t.Errorf("expected stderr to contain debug output, got:\n%s", stderr)
	}
}

// TestDebug_DefaultIsSilent verifies that windsor writes no "[debug]" lines
// when --debug is not passed and WINDSOR_DEBUG is not set.
func TestDebug_DefaultIsSilent(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "default")

	_, stderr, err := helpers.RunCLI(dir, []string{"env"}, env)
	if err != nil {
		t.Fatalf("env: %v\nstderr: %s", err, stderr)
	}
	if strings.Contains(string(stderr), "[debug]") {
		t.Errorf("expected no debug output without --debug, got:\n%s", stderr)
	}
}

// TestDebug_EnvVarEnablesDebugLogging verifies that WINDSOR_DEBUG=true enables
// the same debug output as --debug, for runs that cannot pass a flag.
func TestDebug_EnvVarEnablesDebugLogging(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "default")
	env = append(env, "WINDSOR_DEBUG=true")

	_, stderr, err := helpers.RunCLI(dir, []string{"env"}, env)
	if err != nil {
		t.Fatalf("env: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(string(stderr), "[debug]") {
		t.Errorf("expected stderr to contain debug output, got:\n%s", stderr)
	}
}
