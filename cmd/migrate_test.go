package cmd_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	cliCmd "github.com/windsorcli/cli/cmd"
)

// =============================================================================
// Test Setup
// =============================================================================

// setupMigrateProject writes the files under a temp dir and points the mock shell at it.
func setupMigrateProject(t *testing.T, files map[string]string, args ...string) (*bytes.Buffer, *bytes.Buffer, string) {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	stdout, stderr := captureOutput(t)
	cliCmd.RootCmd().SetOut(stdout)
	cliCmd.RootCmd().SetErr(stderr)
	mocks := setupMocks(t)
	mocks.Shell.GetProjectRootFunc = func() (string, error) { return dir, nil }
	ctx := context.WithValue(context.Background(), cliCmd.RuntimeOverridesKey, mocks.Runtime)
	cliCmd.RootCmd().SetContext(ctx)
	cliCmd.RootCmd().SetArgs(args)
	t.Cleanup(func() { cliCmd.RootCmd().SetArgs(nil) })
	return stdout, stderr, dir
}

const migrateV1Root = "version: v1alpha1\ncontexts:\n  local:\n    provider: docker\n"

// =============================================================================
// Test Public Methods
// =============================================================================

func TestMigrateCmd(t *testing.T) {
	t.Run("ConvertsProject", func(t *testing.T) {
		// Given a v1alpha1 project
		stdout, _, dir := setupMigrateProject(t, map[string]string{"windsor.yaml": migrateV1Root}, "migrate")

		// When running migrate
		err := cliCmd.Execute()

		// Then values.yaml holds platform and the root is v1alpha2
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		values, _ := os.ReadFile(filepath.Join(dir, "contexts", "local", "values.yaml"))
		if !strings.Contains(string(values), "platform: docker") {
			t.Errorf("unexpected values.yaml:\n%s", values)
		}
		if !strings.Contains(stdout.String(), "write") {
			t.Errorf("expected the change list, got %q", stdout.String())
		}
	})

	t.Run("DryRunWritesNothing", func(t *testing.T) {
		// Given a v1alpha1 project
		stdout, _, dir := setupMigrateProject(t, map[string]string{"windsor.yaml": migrateV1Root}, "migrate", "--dry-run")
		t.Cleanup(func() { cliCmd.RootCmd().SetArgs([]string{"migrate", "--dry-run=false"}) })

		// When running migrate --dry-run
		err := cliCmd.Execute()

		// Then no file changes
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(stdout.String(), "Dry run") {
			t.Errorf("expected dry run text, got %q", stdout.String())
		}
		if got, _ := os.ReadFile(filepath.Join(dir, "windsor.yaml")); string(got) != migrateV1Root {
			t.Errorf("root file changed:\n%s", got)
		}
	})

	t.Run("ConflictFails", func(t *testing.T) {
		// Given a conflict between the root entry and values.yaml
		_, stderr, dir := setupMigrateProject(t, map[string]string{
			"windsor.yaml":               migrateV1Root,
			"contexts/local/values.yaml": "platform: aws\n",
		}, "migrate", "--dry-run=false")

		// When running migrate
		err := cliCmd.Execute()

		// Then it fails, lists the conflict, and writes nothing
		if err == nil {
			t.Fatal("expected an error")
		}
		for _, want := range []string{"platform", "docker", "aws", "values.yaml"} {
			if !strings.Contains(stderr.String(), want) {
				t.Errorf("expected stderr to contain %q, got %q", want, stderr.String())
			}
		}
		if got, _ := os.ReadFile(filepath.Join(dir, "windsor.yaml")); string(got) != migrateV1Root {
			t.Errorf("root file changed:\n%s", got)
		}
	})

	t.Run("NothingToMigrate", func(t *testing.T) {
		// Given a v1alpha2 project
		stdout, _, _ := setupMigrateProject(t, map[string]string{"windsor.yaml": "version: v1alpha2\n"}, "migrate", "--dry-run=false")

		// When running migrate
		err := cliCmd.Execute()

		// Then it exits 0 and says so
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(stdout.String(), "Nothing to migrate") {
			t.Errorf("unexpected output %q", stdout.String())
		}
	})
}

func TestMigrationNotice(t *testing.T) {
	t.Run("PrintsForV1alpha1Project", func(t *testing.T) {
		// Given a v1alpha1 project and a command that loads it
		_, stderr, dir := setupMigrateProject(t, map[string]string{"windsor.yaml": migrateV1Root}, "context", "get")

		// When running the command
		_ = cliCmd.Execute()

		// Then stderr names the file and the command
		for _, want := range []string{filepath.Join(dir, "windsor.yaml"), "windsor migrate"} {
			if !strings.Contains(stderr.String(), want) {
				t.Errorf("expected %q in %q", want, stderr.String())
			}
		}
	})

	t.Run("SilentForHook", func(t *testing.T) {
		// Given a v1alpha1 project
		stdout, stderr, _ := setupMigrateProject(t, map[string]string{"windsor.yaml": migrateV1Root}, "env", "--hook")

		// When running the hook path
		_ = cliCmd.Execute()

		// Then no notice appears on either stream
		if strings.Contains(stderr.String(), "windsor migrate") || strings.Contains(stdout.String(), "windsor migrate") {
			t.Errorf("unexpected notice: stdout %q stderr %q", stdout.String(), stderr.String())
		}
	})

	t.Run("SilentForV1alpha2Project", func(t *testing.T) {
		// Given a v1alpha2 project
		_, stderr, _ := setupMigrateProject(t, map[string]string{"windsor.yaml": "version: v1alpha2\n"}, "context", "get")

		// When running the command
		_ = cliCmd.Execute()

		// Then no notice appears
		if strings.Contains(stderr.String(), "windsor migrate") {
			t.Errorf("unexpected notice %q", stderr.String())
		}
	})
}
