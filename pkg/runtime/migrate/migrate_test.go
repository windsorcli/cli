package migrate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
)

// =============================================================================
// Test Setup
// =============================================================================

// writeFiles writes each file under dir, creating parent directories.
func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
}

// readValues reads a YAML file into a map.
func readValues(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var out map[string]any
	if err := yaml.Unmarshal(data, &out); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return out
}

// exists reports whether the path exists.
func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

const v1Root = `version: v1alpha1
terraform:
  driver: tofu
contexts:
  staging:
    provider: aws
    aws:
      region: us-east-1
`

// =============================================================================
// Test Public Methods
// =============================================================================

func TestMigrator_Plan(t *testing.T) {
	t.Run("MergesSourcesAndMapsProvider", func(t *testing.T) {
		// Given a root entry, a context file that overrides it, and a root driver
		dir := t.TempDir()
		writeFiles(t, dir, map[string]string{
			"windsor.yaml":                  v1Root,
			"contexts/staging/windsor.yaml": "aws:\n  region: us-west-2\ndns:\n  domain: example.com\n",
		})
		m := NewMigrator(nil)

		// When planning and applying
		plan, err := m.Plan(dir)
		if err != nil {
			t.Fatalf("Plan: %v", err)
		}
		if len(plan.Conflicts) != 0 {
			t.Fatalf("unexpected conflicts: %+v", plan.Conflicts)
		}
		if err := m.Apply(plan); err != nil {
			t.Fatalf("Apply: %v", err)
		}

		// Then values.yaml holds the merged, mapped keys and the sources are gone
		values := readValues(t, filepath.Join(dir, "contexts", "staging", "values.yaml"))
		if values["platform"] != "aws" || values["provider"] != nil {
			t.Errorf("expected platform aws and no provider, got %v", values)
		}
		if got := values["aws"].(map[string]any)["region"]; got != "us-west-2" {
			t.Errorf("expected the context file to win, got %v", got)
		}
		if got := values["terraform"].(map[string]any)["driver"]; got != "tofu" {
			t.Errorf("expected the root driver to move, got %v", got)
		}
		if exists(filepath.Join(dir, "contexts", "staging", "windsor.yaml")) {
			t.Error("expected the context windsor.yaml to be deleted")
		}
		root := readValues(t, filepath.Join(dir, "windsor.yaml"))
		if root["version"] != "v1alpha2" || root["contexts"] != nil || root["terraform"] != nil {
			t.Errorf("expected a v1alpha2 root with no contexts, got %v", root)
		}
	})

	t.Run("WritesValuesInHeaderOrder", func(t *testing.T) {
		// Given a source that lists keys out of header order
		dir := t.TempDir()
		writeFiles(t, dir, map[string]string{
			"windsor.yaml": "version: v1alpha1\ncontexts:\n  local:\n    zzz: 1\n    platform: docker\n    id: abc\n",
		})
		m := NewMigrator(nil)

		// When applying
		plan, err := m.Plan(dir)
		if err != nil {
			t.Fatalf("Plan: %v", err)
		}
		if err := m.Apply(plan); err != nil {
			t.Fatalf("Apply: %v", err)
		}

		// Then platform comes before the non-header key
		data, _ := os.ReadFile(filepath.Join(dir, "contexts", "local", "values.yaml"))
		text := string(data)
		if strings.Index(text, "platform") > strings.Index(text, "zzz") {
			t.Errorf("expected header order, got:\n%s", text)
		}
	})

	t.Run("PlatformWinsOverProvider", func(t *testing.T) {
		// Given a source with both keys
		dir := t.TempDir()
		writeFiles(t, dir, map[string]string{
			"windsor.yaml": "version: v1alpha1\ncontexts:\n  local:\n    provider: aws\n    platform: docker\n",
		})
		m := NewMigrator(nil)

		// When applying
		plan, _ := m.Plan(dir)
		if err := m.Apply(plan); err != nil {
			t.Fatalf("Apply: %v", err)
		}

		// Then platform keeps its value
		values := readValues(t, filepath.Join(dir, "contexts", "local", "values.yaml"))
		if values["platform"] != "docker" || values["provider"] != nil {
			t.Errorf("expected platform docker, got %v", values)
		}
	})

	t.Run("MapsProviderInExistingValues", func(t *testing.T) {
		// Given a v1alpha2 project whose values.yaml still holds provider
		dir := t.TempDir()
		writeFiles(t, dir, map[string]string{
			"windsor.yaml":               "version: v1alpha2\n",
			"contexts/local/values.yaml": "provider: aws\n",
		})
		m := NewMigrator(nil)

		// When applying
		plan, err := m.Plan(dir)
		if err != nil {
			t.Fatalf("Plan: %v", err)
		}
		if err := m.Apply(plan); err != nil {
			t.Fatalf("Apply: %v", err)
		}

		// Then values.yaml holds platform
		values := readValues(t, filepath.Join(dir, "contexts", "local", "values.yaml"))
		if values["platform"] != "aws" || values["provider"] != nil {
			t.Errorf("expected platform aws, got %v", values)
		}
	})

	t.Run("ConflictWritesNothing", func(t *testing.T) {
		// Given a values.yaml that disagrees with the root entry
		dir := t.TempDir()
		writeFiles(t, dir, map[string]string{
			"windsor.yaml":                  v1Root,
			"contexts/staging/windsor.yaml": "dns:\n  domain: example.com\n",
			"contexts/staging/values.yaml":  "aws:\n  region: eu-west-1\n",
		})
		m := NewMigrator(nil)

		// When planning
		plan, err := m.Plan(dir)
		if err != nil {
			t.Fatalf("Plan: %v", err)
		}

		// Then the plan reports the conflict with both values and both files
		if len(plan.Conflicts) != 1 {
			t.Fatalf("expected 1 conflict, got %+v", plan.Conflicts)
		}
		c := plan.Conflicts[0]
		if c.KeyPath != "aws.region" || c.SourceValue != "us-east-1" || c.ValuesValue != "eu-west-1" {
			t.Errorf("unexpected conflict: %+v", c)
		}
		if filepath.Base(c.SourceFile) != "windsor.yaml" || filepath.Base(c.ValuesFile) != "values.yaml" {
			t.Errorf("unexpected file names: %+v", c)
		}
		text := plan.Format(false)
		for _, want := range []string{"aws.region", "us-east-1", "eu-west-1", "windsor.yaml", "values.yaml"} {
			if !strings.Contains(text, want) {
				t.Errorf("expected output to contain %q, got:\n%s", want, text)
			}
		}

		// And Apply refuses and leaves every file in place
		if err := m.Apply(plan); err == nil {
			t.Error("expected Apply to fail with conflicts")
		}
		if !exists(filepath.Join(dir, "contexts", "staging", "windsor.yaml")) {
			t.Error("expected the context windsor.yaml to remain")
		}
		if got, _ := os.ReadFile(filepath.Join(dir, "windsor.yaml")); string(got) != v1Root {
			t.Errorf("expected the root file to stay, got:\n%s", got)
		}
	})

	t.Run("EqualValuesAreNoConflict", func(t *testing.T) {
		// Given a values.yaml that already holds the same value
		dir := t.TempDir()
		writeFiles(t, dir, map[string]string{
			"windsor.yaml":               "version: v1alpha1\ncontexts:\n  local:\n    aws:\n      region: us-east-1\n",
			"contexts/local/values.yaml": "aws:\n  region: us-east-1\n",
		})
		m := NewMigrator(nil)

		// When planning
		plan, err := m.Plan(dir)

		// Then no conflict appears and the values file stays unwritten
		if err != nil || len(plan.Conflicts) != 0 {
			t.Fatalf("unexpected result: %v %+v", err, plan.Conflicts)
		}
		for _, w := range plan.Writes {
			if filepath.Base(w.Path) == "values.yaml" {
				t.Errorf("expected no values.yaml write, got %s", w.Path)
			}
		}
	})

	t.Run("DriverConflictsWithValues", func(t *testing.T) {
		// Given a values.yaml with another driver
		dir := t.TempDir()
		writeFiles(t, dir, map[string]string{
			"windsor.yaml":               "version: v1alpha1\nterraform:\n  driver: tofu\n",
			"contexts/local/values.yaml": "terraform:\n  driver: terraform\n",
		})

		// When planning
		plan, err := NewMigrator(nil).Plan(dir)

		// Then the driver is a conflict
		if err != nil || len(plan.Conflicts) != 1 || plan.Conflicts[0].KeyPath != "terraform.driver" {
			t.Fatalf("expected a driver conflict, got %v %+v", err, plan.Conflicts)
		}
	})

	t.Run("RejectsUnknownRootKey", func(t *testing.T) {
		// Given a root key that the migration cannot place
		dir := t.TempDir()
		writeFiles(t, dir, map[string]string{"windsor.yaml": "version: v1alpha1\nfoo: bar\n"})

		// When planning
		_, err := NewMigrator(nil).Plan(dir)

		// Then the plan fails and names the key
		if err == nil || !strings.Contains(err.Error(), "foo") {
			t.Errorf("expected an error naming foo, got %v", err)
		}
	})

	t.Run("KeepsEnvironmentAndPathsInRoot", func(t *testing.T) {
		// Given a root file with environment beside contexts
		dir := t.TempDir()
		writeFiles(t, dir, map[string]string{
			"windsor.yaml": "version: v1alpha2\nenvironment:\n  A: b\ncontexts:\n  local:\n    platform: docker\n",
		})
		m := NewMigrator(nil)

		// When applying
		plan, _ := m.Plan(dir)
		if err := m.Apply(plan); err != nil {
			t.Fatalf("Apply: %v", err)
		}

		// Then the root keeps environment and drops contexts
		root := readValues(t, filepath.Join(dir, "windsor.yaml"))
		if root["environment"] == nil || root["contexts"] != nil {
			t.Errorf("unexpected root: %v", root)
		}
	})

	t.Run("IsIdempotent", func(t *testing.T) {
		// Given a migrated project
		dir := t.TempDir()
		writeFiles(t, dir, map[string]string{
			"windsor.yaml":                  v1Root,
			"contexts/staging/windsor.yaml": "dns:\n  domain: example.com\n",
		})
		m := NewMigrator(nil)
		plan, _ := m.Plan(dir)
		if err := m.Apply(plan); err != nil {
			t.Fatalf("Apply: %v", err)
		}

		// When planning again
		again, err := m.Plan(dir)

		// Then nothing remains
		if err != nil || !again.Empty() {
			t.Fatalf("expected an empty plan, got %v %+v", err, again)
		}
		if got := again.Format(false); got != "Nothing to migrate.\n" {
			t.Errorf("unexpected text: %q", got)
		}
	})

	t.Run("DryRunPlanWritesNothing", func(t *testing.T) {
		// Given a v1alpha1 project
		dir := t.TempDir()
		writeFiles(t, dir, map[string]string{"windsor.yaml": v1Root})

		// When only planning
		plan, err := NewMigrator(nil).Plan(dir)

		// Then the plan lists the changes and no file changed
		if err != nil || plan.Empty() {
			t.Fatalf("expected a plan, got %v %+v", err, plan)
		}
		if !strings.Contains(plan.Format(true), "Dry run") {
			t.Errorf("expected dry run wording, got %q", plan.Format(true))
		}
		if got, _ := os.ReadFile(filepath.Join(dir, "windsor.yaml")); string(got) != v1Root {
			t.Error("expected the root file to stay")
		}
		if exists(filepath.Join(dir, "contexts")) {
			t.Error("expected no contexts directory")
		}
	})
}

func TestMigrator_LegacyFiles(t *testing.T) {
	t.Run("ListsRootAndContextFiles", func(t *testing.T) {
		// Given a v1alpha1 root and one context file
		dir := t.TempDir()
		writeFiles(t, dir, map[string]string{
			"windsor.yaml":                  v1Root,
			"contexts/staging/windsor.yaml": "a: b\n",
		})

		// When listing
		files := NewMigrator(nil).LegacyFiles(dir)

		// Then both appear
		if len(files) != 2 {
			t.Fatalf("expected 2 files, got %v", files)
		}
	})

	t.Run("EmptyForV1alpha2", func(t *testing.T) {
		// Given a v1alpha2 project
		dir := t.TempDir()
		writeFiles(t, dir, map[string]string{"windsor.yaml": "version: v1alpha2\n"})

		// When listing
		files := NewMigrator(nil).LegacyFiles(dir)

		// Then nothing appears
		if len(files) != 0 {
			t.Errorf("expected none, got %v", files)
		}
	})
}
