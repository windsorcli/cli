package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/project"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// newConfigurePreconditionTestProject builds a minimal project rooted at projectRoot with the
// given context name. Used only by ensureWorkstationProvisioned's unit tests; the helper does
// not depend on a NetworkManager so no workstation is attached.
func newConfigurePreconditionTestProject(t *testing.T, projectRoot, contextName string) *project.Project {
	t.Helper()
	cfg := config.NewMockConfigHandler()
	cfg.GetContextFunc = func() string { return contextName }
	sh := shell.NewMockShell()
	sh.GetProjectRootFunc = func() (string, error) { return projectRoot, nil }
	rt := runtime.NewRuntime(&runtime.Runtime{Shell: sh, ConfigHandler: cfg, ProjectRoot: projectRoot})
	return project.NewProject(contextName, &project.Project{Runtime: rt})
}

func TestEnsureWorkstationProvisioned(t *testing.T) {
	t.Run("ErrorsWhenWorkstationYAMLMissing", func(t *testing.T) {
		// Given a project root with no .windsor/contexts/<context>/workstation.yaml
		tmpDir := t.TempDir()
		proj := newConfigurePreconditionTestProject(t, tmpDir, "test-context")

		// When checking the precondition
		err := ensureWorkstationProvisioned(proj)

		// Then an operator-facing error points the operator at 'windsor up'
		if err == nil {
			t.Fatalf("expected error when workstation.yaml is missing, got nil")
		}
		want := `workstation has not been provisioned yet for context "test-context"`
		if !strings.Contains(err.Error(), want) {
			t.Errorf("expected error to contain %q, got %q", want, err.Error())
		}
		if !strings.Contains(err.Error(), "Run 'windsor up' first") {
			t.Errorf("expected error to suggest 'windsor up' remediation, got %q", err.Error())
		}
	})

	t.Run("PassesWhenWorkstationYAMLExists", func(t *testing.T) {
		// Given a project root with the workstation state file present
		tmpDir := t.TempDir()
		contextDir := filepath.Join(tmpDir, ".windsor", "contexts", "test-context")
		if err := os.MkdirAll(contextDir, 0755); err != nil {
			t.Fatalf("setup mkdir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(contextDir, "workstation.yaml"), []byte("dns:\n  address: 10.5.0.2\n"), 0644); err != nil {
			t.Fatalf("setup write workstation.yaml: %v", err)
		}
		proj := newConfigurePreconditionTestProject(t, tmpDir, "test-context")

		// When checking the precondition
		err := ensureWorkstationProvisioned(proj)

		// Then no error
		if err != nil {
			t.Errorf("expected nil error when workstation.yaml exists, got %v", err)
		}
	})
}
