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
