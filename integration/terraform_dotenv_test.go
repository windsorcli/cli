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
