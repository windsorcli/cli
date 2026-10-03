package cmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/shell"
)

func TestListCmd(t *testing.T) {
	runWithContexts := func(t *testing.T, args []string) string {
		t.Helper()
		os.Unsetenv("WINDSOR_CONTEXT")
		tmpDir := t.TempDir()
		t.Setenv("HOME", tmpDir)

		for name, config := range map[string]string{
			"local":   "provider: generic\nterraform:\n  backend:\n    type: local\n",
			"staging": "provider: aws\nterraform:\n  backend:\n    type: s3\n",
		} {
			dir := filepath.Join(tmpDir, "contexts", name)
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatalf("Failed to create context directory: %v", err)
			}
			if err := os.WriteFile(filepath.Join(dir, "windsor.yaml"), []byte(config), 0644); err != nil {
				t.Fatalf("Failed to write context config: %v", err)
			}
		}
		if err := os.WriteFile(filepath.Join(tmpDir, "windsor.yaml"), []byte("version: v1alpha1\n"), 0644); err != nil {
			t.Fatalf("Failed to write windsor.yaml: %v", err)
		}

		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) { return tmpDir, nil }
		rt := runtime.NewRuntime(&runtime.Runtime{Shell: mockShell, ProjectRoot: tmpDir})
		rootCmd.SetContext(context.WithValue(context.Background(), runtimeOverridesKey, rt))

		stdout, stderr := captureOutput(t)
		rootCmd.SetOut(stdout)
		rootCmd.SetErr(stderr)
		rootCmd.SetArgs(args)
		if err := Execute(); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		return stdout.String()
	}

	t.Run("PrintsSameOutputAsGetContexts", func(t *testing.T) {
		want := runWithContexts(t, []string{"get", "contexts"})
		got := runWithContexts(t, []string{"list"})

		if got != want {
			t.Errorf("Expected list output to match get contexts.\nlist:\n%s\nget contexts:\n%s", got, want)
		}
		if want == "" {
			t.Error("Expected non-empty output")
		}
	})
}
