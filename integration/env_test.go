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

// globalModeEnv returns a pristine workDir (no windsor.yaml above it) and env
// with HOME redirected to a separate temp dir, so the subprocess's global-mode
// fallback lands in an isolated location instead of the developer's real home.
func globalModeEnv(t *testing.T) (workDir string, env []string, homeDir string) {
	t.Helper()
	workDir = t.TempDir()
	homeDir = t.TempDir()
	env = []string{
		"HOME=" + homeDir,
		"USERPROFILE=" + homeDir,
		"PATH=" + os.Getenv("PATH"),
	}
	return workDir, env, homeDir
}

// TestEnv_HookIsSilentInGlobalMode verifies that `windsor env --hook` returns
// silently (no output, no error) when run from a directory with no ancestral
// windsor.yaml. Shell hook integrations rely on this silent no-op so prompts
// don't emit stray exports in non-Windsor directories.
func TestEnv_HookIsSilentInGlobalMode(t *testing.T) {
	t.Parallel()
	workDir, env, _ := globalModeEnv(t)

	stdout, stderr, err := helpers.RunCLI(workDir, []string{"env", "--hook"}, env)
	if err != nil {
		t.Fatalf("expected silent success from env --hook in global mode, got error: %v\nstderr: %s", err, stderr)
	}
	if len(stdout) != 0 {
		t.Errorf("expected empty stdout in global mode, got: %q", stdout)
	}
}

// TestEnv_CreatesGlobalRootOnDemand verifies that running a windsor command in
// a directory with no project automatically creates $HOME/.config/windsor so
// subsequent state (context file, contexts/, etc.) has a place to live.
func TestEnv_CreatesGlobalRootOnDemand(t *testing.T) {
	t.Parallel()
	workDir, env, homeDir := globalModeEnv(t)

	globalRoot := filepath.Join(homeDir, ".config", "windsor")
	if _, err := os.Stat(globalRoot); err == nil {
		t.Fatalf("global root %s should not exist before running the CLI", globalRoot)
	}

	_, stderr, err := helpers.RunCLI(workDir, []string{"env", "--hook"}, env)
	if err != nil {
		t.Fatalf("env --hook: %v\nstderr: %s", err, stderr)
	}

	if _, err := os.Stat(globalRoot); err != nil {
		t.Fatalf("expected %s to be created by global-mode fallback, got %v", globalRoot, err)
	}
}

// TestEnv_SucceedsInGlobalModeWithoutTrust verifies that `windsor env` works in
// a fresh directory without a trust file, because the global fallback
// implicitly trusts $HOME/.config/windsor. Prior to global mode this call
// would have failed the trusted-directory check.
func TestEnv_SucceedsInGlobalModeWithoutTrust(t *testing.T) {
	t.Parallel()
	workDir, env, _ := globalModeEnv(t)

	_, stderr, err := helpers.RunCLI(workDir, []string{"env"}, env)
	if err != nil {
		t.Fatalf("expected env to succeed in global mode, got error: %v\nstderr: %s", err, stderr)
	}
	if strings.Contains(string(stderr), "not in a trusted directory") {
		t.Errorf("expected trust check to be bypassed in global mode, got: %s", stderr)
	}
}
