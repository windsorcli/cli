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

// TestDotEnv_InjectsValueIntoWindsorEnv verifies that a plain KEY=VALUE line in
// contexts/<ctx>/.env is exported by `windsor env`. Bare `windsor init` (used by
// PrepareFixture) provisions the "local" context.
func TestDotEnv_InjectsValueIntoWindsorEnv(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "default")

	dotEnvPath := filepath.Join(dir, "contexts", "local", ".env")
	if err := os.WriteFile(dotEnvPath, []byte("HYPERV_HOST=hyperv.local\n"), 0600); err != nil {
		t.Fatalf("write .env fixture: %v", err)
	}

	stdout, stderr, err := helpers.RunCLI(dir, []string{"env"}, env)
	if err != nil {
		t.Fatalf("env: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(string(stdout), "HYPERV_HOST=hyperv.local") {
		t.Errorf("expected HYPERV_HOST=hyperv.local in output, got:\n%s", stdout)
	}
}

// TestDotEnv_AbsentFileDoesNotBreakWindsorEnv verifies that `windsor env` succeeds
// normally when no contexts/<ctx>/.env file is present.
func TestDotEnv_AbsentFileDoesNotBreakWindsorEnv(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "default")

	_, stderr, err := helpers.RunCLI(dir, []string{"env"}, env)
	if err != nil {
		t.Fatalf("expected env to succeed with no .env file, got error: %v\nstderr: %s", err, stderr)
	}
}

// TestDotEnv_ContextEnvironmentKeyOverridesDotEnv verifies that the declarative
// contexts.<ctx>.environment key in windsor.yaml takes precedence over the same
// key supplied via contexts/<ctx>/.env, matching the documented precedence where
// .env is the lowest-precedence base layer.
func TestDotEnv_ContextEnvironmentKeyOverridesDotEnv(t *testing.T) {
	t.Parallel()
	dir, env := helpers.CopyFixtureOnly(t, "default")

	windsorYamlPath := filepath.Join(dir, "windsor.yaml")
	windsorYaml := `version: v1alpha1
contexts:
  local:
    environment:
      SHARED_VAR: from-windsor-yaml
`
	if err := os.WriteFile(windsorYamlPath, []byte(windsorYaml), 0644); err != nil {
		t.Fatalf("write windsor.yaml: %v", err)
	}
	helpers.MarkAsGitRepo(t, dir)
	if _, stderr, err := helpers.RunCLI(dir, []string{"init"}, env); err != nil {
		t.Fatalf("windsor init: %v\nstderr: %s", err, stderr)
	}

	dotEnvPath := filepath.Join(dir, "contexts", "local", ".env")
	if err := os.WriteFile(dotEnvPath, []byte("SHARED_VAR=from-dotenv\n"), 0600); err != nil {
		t.Fatalf("write .env fixture: %v", err)
	}

	stdout, stderr, err := helpers.RunCLI(dir, []string{"env"}, env)
	if err != nil {
		t.Fatalf("env: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(string(stdout), "SHARED_VAR=from-windsor-yaml") {
		t.Errorf("expected the windsor.yaml environment key to win over .env, got:\n%s", stdout)
	}
	if strings.Contains(string(stdout), "from-dotenv") {
		t.Errorf("expected .env value to be overridden, got:\n%s", stdout)
	}
}

// TestHetznerEnv_InjectsHcloudTokenFromConfig verifies that a hetzner.token set in
// windsor.yaml is exported by `windsor env` as HCLOUD_TOKEN, the credential the
// hcloud Terraform provider and CLI read — the Hetzner analog of the aws/azure
// provider env printers.
func TestHetznerEnv_InjectsHcloudTokenFromConfig(t *testing.T) {
	t.Parallel()
	dir, env := helpers.CopyFixtureOnly(t, "default")

	windsorYamlPath := filepath.Join(dir, "windsor.yaml")
	windsorYaml := `version: v1alpha1
contexts:
  local:
    hetzner:
      token: test-hcloud-token
`
	if err := os.WriteFile(windsorYamlPath, []byte(windsorYaml), 0644); err != nil {
		t.Fatalf("write windsor.yaml: %v", err)
	}
	helpers.MarkAsGitRepo(t, dir)
	if _, stderr, err := helpers.RunCLI(dir, []string{"init"}, env); err != nil {
		t.Fatalf("windsor init: %v\nstderr: %s", err, stderr)
	}

	stdout, stderr, err := helpers.RunCLI(dir, []string{"env"}, env)
	if err != nil {
		t.Fatalf("env: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(string(stdout), "HCLOUD_TOKEN=test-hcloud-token") {
		t.Errorf("expected HCLOUD_TOKEN=test-hcloud-token in output, got:\n%s", stdout)
	}
}

// TestHetznerEnv_OmitsHcloudTokenWhenUnset verifies that `windsor env` does not emit
// HCLOUD_TOKEN when no hetzner.token is configured, so the operator's ambient
// credential applies unchanged.
func TestHetznerEnv_OmitsHcloudTokenWhenUnset(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "default")

	stdout, stderr, err := helpers.RunCLI(dir, []string{"env"}, env)
	if err != nil {
		t.Fatalf("env: %v\nstderr: %s", err, stderr)
	}
	if strings.Contains(string(stdout), "HCLOUD_TOKEN") {
		t.Errorf("expected no HCLOUD_TOKEN in output when unset, got:\n%s", stdout)
	}
}

// TestDotEnv_ContextGitignoreCoversDotEnvFile verifies that `windsor init` writes
// contexts/<ctx>/.gitignore covering .env so the file can never be committed.
func TestDotEnv_ContextGitignoreCoversDotEnvFile(t *testing.T) {
	t.Parallel()
	dir, _ := helpers.PrepareFixture(t, "default")

	gitignorePath := filepath.Join(dir, "contexts", "local", ".gitignore")
	content, err := os.ReadFile(gitignorePath)
	if err != nil {
		t.Fatalf("expected contexts/local/.gitignore to exist: %v", err)
	}
	if !strings.Contains(string(content), ".env") {
		t.Errorf("expected .gitignore to cover .env, got:\n%s", content)
	}
}

// TestTerraformDotEnv_InjectedWhenCdIntoTerraformDirectory verifies that a value in
// contexts/<ctx>/terraform/.env reaches `windsor env` when the operator is cd'd into
// a Terraform component directory — the same TerraformProvider.GetEnvVars call that
// `windsor up`/`apply`/`plan`/`destroy` use per component.
func TestTerraformDotEnv_InjectedWhenCdIntoTerraformDirectory(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "terraform-dotenv")

	dotEnvPath := filepath.Join(dir, "contexts", "local", "terraform", ".env")
	if err := os.MkdirAll(filepath.Dir(dotEnvPath), 0750); err != nil {
		t.Fatalf("mkdir terraform dir: %v", err)
	}
	if err := os.WriteFile(dotEnvPath, []byte("HYPERV_HOST=hyperv.local\n"), 0600); err != nil {
		t.Fatalf("write terraform/.env fixture: %v", err)
	}

	componentDir := filepath.Join(dir, "terraform", "null")
	stdout, stderr, err := helpers.RunCLI(componentDir, []string{"env"}, env)
	if err != nil {
		t.Fatalf("env: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(string(stdout), "HYPERV_HOST=hyperv.local") {
		t.Errorf("expected HYPERV_HOST=hyperv.local in output, got:\n%s", stdout)
	}
}

// TestTerraformDotEnv_AbsentFromHookOutsideTerraformDirectory verifies that a value in
// contexts/<ctx>/terraform/.env does NOT reach `windsor env` when the operator is not
// cd'd into a Terraform directory — the core performance property: content that's only
// relevant to Terraform work never touches the general interactive hook path.
func TestTerraformDotEnv_AbsentFromHookOutsideTerraformDirectory(t *testing.T) {
	t.Parallel()
	dir, env := helpers.PrepareFixture(t, "terraform-dotenv")

	dotEnvPath := filepath.Join(dir, "contexts", "local", "terraform", ".env")
	if err := os.MkdirAll(filepath.Dir(dotEnvPath), 0750); err != nil {
		t.Fatalf("mkdir terraform dir: %v", err)
	}
	if err := os.WriteFile(dotEnvPath, []byte("HYPERV_HOST=hyperv.local\n"), 0600); err != nil {
		t.Fatalf("write terraform/.env fixture: %v", err)
	}

	stdout, stderr, err := helpers.RunCLI(dir, []string{"env"}, env)
	if err != nil {
		t.Fatalf("env: %v\nstderr: %s", err, stderr)
	}
	if strings.Contains(string(stdout), "hyperv.local") {
		t.Errorf("expected HYPERV_HOST to be absent outside a Terraform directory, got:\n%s", stdout)
	}
}
