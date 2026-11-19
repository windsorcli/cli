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

// migrateProject writes a v1alpha1 project into a temp dir and returns it with the CLI env.
func migrateProject(t *testing.T, files map[string]string) (string, []string) {
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
	helpers.MarkAsGitRepo(t, dir)
	_, env := helpers.CopyFixtureOnly(t, "default")
	return dir, env
}

const migrateRoot = "version: v1alpha1\nterraform:\n  driver: tofu\ncontexts:\n  staging:\n    provider: aws\n    aws:\n      region: us-east-1\n"

// TestMigrate_ConvertsProject verifies that `windsor migrate` writes values.yaml, deletes the
// context windsor.yaml, and rewrites the root file, and that a second run reports nothing to migrate.
func TestMigrate_ConvertsProject(t *testing.T) {
	t.Parallel()
	dir, env := migrateProject(t, map[string]string{
		"windsor.yaml":                  migrateRoot,
		"contexts/staging/windsor.yaml": "dns:\n  domain: example.com\n",
	})

	stdout, stderr, err := helpers.RunCLI(dir, []string{"migrate"}, env)
	if err != nil {
		t.Fatalf("migrate: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(string(stdout), "values.yaml") {
		t.Errorf("expected the change list, got %s", stdout)
	}

	values := readYAMLFile(t, filepath.Join(dir, "contexts", "staging", "values.yaml"))
	if values["platform"] != "aws" {
		t.Errorf("expected platform aws, got %v", values["platform"])
	}
	if _, ok := values["provider"]; ok {
		t.Error("expected no provider key")
	}
	if got := values["terraform"].(map[string]any)["driver"]; got != "tofu" {
		t.Errorf("expected the root driver in values.yaml, got %v", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "contexts", "staging", "windsor.yaml")); err == nil {
		t.Error("expected the context windsor.yaml to be deleted")
	}
	root := readYAMLFile(t, filepath.Join(dir, "windsor.yaml"))
	if root["version"] != "v1alpha2" {
		t.Errorf("expected version v1alpha2, got %v", root["version"])
	}
	if _, ok := root["contexts"]; ok {
		t.Error("expected no contexts key in the root file")
	}

	stdout, stderr, err = helpers.RunCLI(dir, []string{"migrate"}, env)
	if err != nil {
		t.Fatalf("second migrate: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(string(stdout), "Nothing to migrate") {
		t.Errorf("expected 'Nothing to migrate', got %s", stdout)
	}
}

// TestMigrate_DryRunWritesNothing verifies that --dry-run lists the changes and leaves every file as it was.
func TestMigrate_DryRunWritesNothing(t *testing.T) {
	t.Parallel()
	dir, env := migrateProject(t, map[string]string{
		"windsor.yaml":                  migrateRoot,
		"contexts/staging/windsor.yaml": "dns:\n  domain: example.com\n",
	})

	stdout, stderr, err := helpers.RunCLI(dir, []string{"migrate", "--dry-run"}, env)
	if err != nil {
		t.Fatalf("migrate --dry-run: %v\nstderr: %s", err, stderr)
	}
	for _, want := range []string{"Dry run", "values.yaml", filepath.Join("staging", "windsor.yaml")} {
		if !strings.Contains(string(stdout), want) {
			t.Errorf("expected stdout to contain %q, got %s", want, stdout)
		}
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "windsor.yaml")); string(got) != migrateRoot {
		t.Errorf("expected the root file to stay, got %s", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "contexts", "staging", "values.yaml")); err == nil {
		t.Error("expected no values.yaml")
	}
}

// TestMigrate_ConflictWritesNothing verifies that a conflict exits non-zero, lists the key path,
// both values, and both files, and changes no file.
func TestMigrate_ConflictWritesNothing(t *testing.T) {
	t.Parallel()
	dir, env := migrateProject(t, map[string]string{
		"windsor.yaml":                 migrateRoot,
		"contexts/staging/values.yaml": "aws:\n  region: eu-west-1\n",
	})

	_, stderr, err := helpers.RunCLI(dir, []string{"migrate"}, env)
	if err == nil {
		t.Fatal("expected migrate to fail on a conflict")
	}
	for _, want := range []string{"aws.region", "us-east-1", "eu-west-1", "windsor.yaml", "values.yaml"} {
		if !strings.Contains(string(stderr), want) {
			t.Errorf("expected stderr to contain %q, got %s", want, stderr)
		}
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "windsor.yaml")); string(got) != migrateRoot {
		t.Errorf("expected the root file to stay, got %s", got)
	}
}

// TestMigrate_NoticeAndInitStop verifies that a command prints the v1alpha1 notice, that init stops
// before it writes anything and lists the files, and that init works after migrate.
func TestMigrate_NoticeAndInitStop(t *testing.T) {
	t.Parallel()
	dir, env := migrateProject(t, map[string]string{
		"windsor.yaml":                  migrateRoot,
		"contexts/staging/windsor.yaml": "dns:\n  domain: example.com\n",
	})

	_, stderr, _ := helpers.RunCLI(dir, []string{"env"}, env)
	for _, want := range []string{"windsor migrate", filepath.Join("staging", "windsor.yaml")} {
		if !strings.Contains(string(stderr), want) {
			t.Errorf("expected the notice to contain %q, got %s", want, stderr)
		}
	}

	_, stderr, err := helpers.RunCLI(dir, []string{"init", "staging"}, env)
	if err == nil {
		t.Fatal("expected init to stop on a v1alpha1 project")
	}
	for _, want := range []string{"windsor migrate", filepath.Join(dir, "windsor.yaml"), filepath.Join("staging", "windsor.yaml")} {
		if !strings.Contains(string(stderr), want) {
			t.Errorf("expected stderr to contain %q, got %s", want, stderr)
		}
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "windsor.yaml")); string(got) != migrateRoot {
		t.Errorf("expected init to leave the root file, got %s", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "contexts", "staging", "values.yaml")); err == nil {
		t.Error("expected init to write no values.yaml")
	}
}

// TestMigrate_HookIsSilent verifies that `env --hook` writes nothing to stdout or stderr
// in a v1alpha1 project, in a fresh session and in a repeat run.
func TestMigrate_HookIsSilent(t *testing.T) {
	t.Parallel()
	dir, env := migrateProject(t, map[string]string{"windsor.yaml": migrateRoot})
	env = append(env, "WINDSOR_SESSION_TOKEN=abc1234")

	for run := 1; run <= 2; run++ {
		stdout, stderr, _ := helpers.RunCLI(dir, []string{"env", "--hook"}, env)
		if strings.Contains(string(stdout), "windsor migrate") || strings.Contains(string(stderr), "windsor migrate") {
			t.Errorf("run %d: expected no notice, got stdout %q stderr %q", run, stdout, stderr)
		}
	}
}
