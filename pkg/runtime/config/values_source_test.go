package config

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// =============================================================================
// Test Values Source
// =============================================================================

func TestValuesSource_Load(t *testing.T) {
	t.Run("ReturnsNotFoundWhenValuesYamlMissing", func(t *testing.T) {
		source := newValuesSource(NewShims(), nil, newPersistencePolicy())
		projectRoot := t.TempDir()

		values, found, err := source.Load(projectRoot, "missing-context")
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if found {
			t.Fatal("Expected values.yaml to be reported missing")
		}
		if values != nil {
			t.Errorf("Expected nil values map, got %v", values)
		}
	})

	t.Run("LoadsValuesYamlWhenPresent", func(t *testing.T) {
		source := newValuesSource(NewShims(), nil, newPersistencePolicy())
		projectRoot := t.TempDir()
		contextName := "local"
		contextDir := filepath.Join(projectRoot, "contexts", contextName)
		if err := os.MkdirAll(contextDir, 0755); err != nil {
			t.Fatalf("Expected no error creating context dir, got %v", err)
		}
		if err := os.WriteFile(filepath.Join(contextDir, "values.yaml"), []byte("provider: docker\n"), 0644); err != nil {
			t.Fatalf("Expected no error writing values.yaml, got %v", err)
		}

		values, found, err := source.Load(projectRoot, contextName)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !found {
			t.Fatal("Expected values.yaml to be found")
		}
		if values["provider"] != "docker" {
			t.Errorf("Expected provider=docker, got %v", values["provider"])
		}
	})

	t.Run("WarnsWithFormattedErrorsAndMarksReportedWhenSchemaInvalid", func(t *testing.T) {
		validator := NewSchemaValidator(shell.NewMockShell())
		validator.Schema = map[string]any{
			"$schema":              "https://json-schema.org/draft/2020-12/schema",
			"type":                 "object",
			"properties":           map[string]any{"provider": map[string]any{"type": "string"}},
			"required":             []any{"provider"},
			"additionalProperties": false,
		}
		source := newValuesSource(NewShims(), validator, newPersistencePolicy())
		projectRoot := t.TempDir()
		contextName := "local"
		contextDir := filepath.Join(projectRoot, "contexts", contextName)
		if err := os.MkdirAll(contextDir, 0755); err != nil {
			t.Fatalf("Expected no error creating context dir, got %v", err)
		}
		// Missing required "provider" and carrying an undeclared "oidc" property.
		if err := os.WriteFile(filepath.Join(contextDir, "values.yaml"), []byte("oidc: not-allowed\n"), 0644); err != nil {
			t.Fatalf("Expected no error writing values.yaml, got %v", err)
		}

		oldStderr := os.Stderr
		r, w, _ := os.Pipe()
		os.Stderr = w
		values, found, err := source.Load(projectRoot, contextName)
		w.Close()
		var buf strings.Builder
		_, _ = io.Copy(&buf, r)
		os.Stderr = oldStderr

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !found {
			t.Fatal("Expected values.yaml to be found")
		}
		if values["oidc"] != "not-allowed" {
			t.Errorf("Expected values to still be returned despite validation failure, got %v", values)
		}

		output := buf.String()
		if !strings.HasPrefix(output, "Warning: values.yaml validation failed (config still loaded):\n  - ") {
			t.Errorf("Expected an indented multi-line warning, got %q", output)
		}
		if !strings.Contains(output, "required") || !strings.Contains(output, "additionalProperties") {
			t.Errorf("Expected the warning to mention both violations, got %q", output)
		}

		// And the reported error set is now recognized by ErrorsAlreadyReported, so a later
		// ValidateContextValues call against the same invalid data won't re-print it.
		result, err := validator.Validate(values)
		if err != nil {
			t.Fatalf("Expected no error re-validating, got %v", err)
		}
		if !validator.ErrorsAlreadyReported(result.Errors) {
			t.Error("Expected the warned error set to be marked as already reported")
		}
	})
}

func TestValuesSource_Save(t *testing.T) {
	t.Run("WritesOnlyValuesPartitionForDevInput", func(t *testing.T) {
		source := newValuesSource(NewShims(), nil, newPersistencePolicy())
		projectRoot := t.TempDir()
		contextName := "local"
		data := map[string]any{
			"provider":    "docker",
			"platform":    "docker",
			"workstation": map[string]any{"runtime": "colima"},
		}

		if err := source.Save(projectRoot, contextName, data, nil, nil, true, persistencePolicyInput{IsDevMode: true}); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		valuesPath := filepath.Join(projectRoot, "contexts", contextName, "values.yaml")
		content, err := os.ReadFile(valuesPath)
		if err != nil {
			t.Fatalf("Expected values.yaml to be written, got %v", err)
		}
		valuesStr := string(content)
		if contains(valuesStr, "provider:") {
			t.Errorf("Expected provider to be excluded from values.yaml, got %s", valuesStr)
		}
		if contains(valuesStr, "platform:") {
			t.Errorf("Expected platform to be excluded from values.yaml in dev input, got %s", valuesStr)
		}
		if contains(valuesStr, "workstation:") {
			t.Errorf("Expected workstation to be excluded from values.yaml, got %s", valuesStr)
		}
	})

	t.Run("LeavesExistingValuesUntouchedWhenNotOverwrite", func(t *testing.T) {
		source := newValuesSource(NewShims(), nil, newPersistencePolicy())
		projectRoot := t.TempDir()
		contextName := "local"
		contextDir := filepath.Join(projectRoot, "contexts", contextName)
		if err := os.MkdirAll(contextDir, 0755); err != nil {
			t.Fatalf("Expected no error creating context dir, got %v", err)
		}
		initial := "provider: docker\nplatform: docker\nworkstation:\n    runtime: colima\n"
		if err := os.WriteFile(filepath.Join(contextDir, "values.yaml"), []byte(initial), 0644); err != nil {
			t.Fatalf("Expected no error writing initial values file, got %v", err)
		}

		data := map[string]any{"cluster": map[string]any{"driver": "talos"}}
		if err := source.Save(projectRoot, contextName, data, data, nil, false, persistencePolicyInput{IsDevMode: true}); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		valuesPath := filepath.Join(contextDir, "values.yaml")
		content, err := os.ReadFile(valuesPath)
		if err != nil {
			t.Fatalf("Expected values.yaml to be readable, got %v", err)
		}
		if string(content) != initial {
			t.Errorf("Expected values.yaml to be unchanged, got %s", string(content))
		}
	})

	t.Run("PatchesOnlyTheOverriddenPathLeavingSiblingsIntact", func(t *testing.T) {
		// backend.type must patch in without disturbing backend.bucket or component.
		source := newValuesSource(NewShims(), nil, newPersistencePolicy())
		projectRoot := t.TempDir()
		contextName := "local"
		contextDir := filepath.Join(projectRoot, "contexts", contextName)
		if err := os.MkdirAll(contextDir, 0755); err != nil {
			t.Fatalf("Expected no error creating context dir, got %v", err)
		}
		initial := "id: abc123\nterraform:\n    component: cluster\n    backend:\n        type: s3\n        bucket: mybucket\n"
		valuesPath := filepath.Join(contextDir, "values.yaml")
		if err := os.WriteFile(valuesPath, []byte(initial), 0644); err != nil {
			t.Fatalf("Expected no error writing initial values file, got %v", err)
		}

		overrides := map[string]any{"terraform": map[string]any{"backend": map[string]any{"type": "gcs"}}}
		if err := source.Save(projectRoot, contextName, overrides, overrides, nil, true, persistencePolicyInput{}); err != nil {
			t.Fatalf("Expected no error patching a nested override, got %v", err)
		}

		content, err := os.ReadFile(valuesPath)
		if err != nil {
			t.Fatalf("Expected values.yaml to be readable, got %v", err)
		}
		values := map[string]any{}
		if err := NewShims().YamlUnmarshal(content, &values); err != nil {
			t.Fatalf("Expected the patched file to still be valid YAML, got %v: %s", err, content)
		}
		if got := getPathValue(t, values, "terraform", "backend", "type"); got != "gcs" {
			t.Errorf("Expected terraform.backend.type=gcs, got %v", got)
		}
		if got := getPathValue(t, values, "terraform", "backend", "bucket"); got != "mybucket" {
			t.Errorf("Expected terraform.backend.bucket to survive the patch untouched, got %v", got)
		}
		if got := getPathValue(t, values, "terraform", "component"); got != "cluster" {
			t.Errorf("Expected terraform.component to survive the patch untouched, got %v", got)
		}
	})

	t.Run("PatchPreservesKeyOrderAndComments", func(t *testing.T) {
		source := newValuesSource(NewShims(), nil, newPersistencePolicy())
		projectRoot := t.TempDir()
		contextName := "local"
		contextDir := filepath.Join(projectRoot, "contexts", contextName)
		if err := os.MkdirAll(contextDir, 0755); err != nil {
			t.Fatalf("Expected no error creating context dir, got %v", err)
		}
		initial := "zed: last\nid: abc123\n# a note about terraform\nterraform:\n    backend:\n        type: s3\nabc: first\n"
		valuesPath := filepath.Join(contextDir, "values.yaml")
		if err := os.WriteFile(valuesPath, []byte(initial), 0644); err != nil {
			t.Fatalf("Expected no error writing initial values file, got %v", err)
		}

		overrides := map[string]any{"terraform": map[string]any{"backend": map[string]any{"type": "gcs"}}}
		if err := source.Save(projectRoot, contextName, overrides, overrides, nil, true, persistencePolicyInput{}); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		content, err := os.ReadFile(valuesPath)
		if err != nil {
			t.Fatalf("Expected values.yaml to be readable, got %v", err)
		}
		expected := "zed: last\nid: abc123\n# a note about terraform\nterraform:\n    backend:\n        type: gcs\nabc: first\n"
		if string(content) != expected {
			t.Errorf("Expected order and comments preserved with only type changed,\nwant: %q\ngot:  %q", expected, string(content))
		}
	})

	t.Run("PatchAppendsANewTopLevelKey", func(t *testing.T) {
		source := newValuesSource(NewShims(), nil, newPersistencePolicy())
		projectRoot := t.TempDir()
		contextName := "local"
		contextDir := filepath.Join(projectRoot, "contexts", contextName)
		if err := os.MkdirAll(contextDir, 0755); err != nil {
			t.Fatalf("Expected no error creating context dir, got %v", err)
		}
		initial := "id: abc123\n"
		valuesPath := filepath.Join(contextDir, "values.yaml")
		if err := os.WriteFile(valuesPath, []byte(initial), 0644); err != nil {
			t.Fatalf("Expected no error writing initial values file, got %v", err)
		}

		overrides := map[string]any{"gcp": map[string]any{"project_id": "new-project"}}
		if err := source.Save(projectRoot, contextName, overrides, overrides, nil, true, persistencePolicyInput{}); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		content, err := os.ReadFile(valuesPath)
		if err != nil {
			t.Fatalf("Expected values.yaml to be readable, got %v", err)
		}
		if !contains(string(content), "id: abc123") || !contains(string(content), "gcp:") {
			t.Errorf("Expected both the prior id and the new gcp key present, got %s", string(content))
		}
	})

	t.Run("PatchRoutesPlatformAwayWhileStillWritingOtherOverrides", func(t *testing.T) {
		// platform must not land in values.yaml even alongside a real override.
		source := newValuesSource(NewShims(), nil, newPersistencePolicy())
		projectRoot := t.TempDir()
		contextName := "local"
		contextDir := filepath.Join(projectRoot, "contexts", contextName)
		if err := os.MkdirAll(contextDir, 0755); err != nil {
			t.Fatalf("Expected no error creating context dir, got %v", err)
		}
		initial := "id: abc123\n"
		valuesPath := filepath.Join(contextDir, "values.yaml")
		if err := os.WriteFile(valuesPath, []byte(initial), 0644); err != nil {
			t.Fatalf("Expected no error writing initial values file, got %v", err)
		}

		overrides := map[string]any{"platform": "docker", "dns": map[string]any{"enabled": false}}
		if err := source.Save(projectRoot, contextName, overrides, overrides, nil, true, persistencePolicyInput{IsDevMode: true}); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		content, err := os.ReadFile(valuesPath)
		if err != nil {
			t.Fatalf("Expected values.yaml to be readable, got %v", err)
		}
		if contains(string(content), "platform:") {
			t.Errorf("Expected platform to be routed away from values.yaml, got %s", string(content))
		}
		if !contains(string(content), "dns:") {
			t.Errorf("Expected dns to still be written, got %s", string(content))
		}
	})

	t.Run("PatchRemovesADeletedTopLevelKey", func(t *testing.T) {
		// A merge alone can't remove a key; deletes must.
		source := newValuesSource(NewShims(), nil, newPersistencePolicy())
		projectRoot := t.TempDir()
		contextName := "local"
		contextDir := filepath.Join(projectRoot, "contexts", contextName)
		if err := os.MkdirAll(contextDir, 0755); err != nil {
			t.Fatalf("Expected no error creating context dir, got %v", err)
		}
		initial := "provider: docker\nid: abc123\n"
		valuesPath := filepath.Join(contextDir, "values.yaml")
		if err := os.WriteFile(valuesPath, []byte(initial), 0644); err != nil {
			t.Fatalf("Expected no error writing initial values file, got %v", err)
		}

		overrides := map[string]any{"platform": "docker"}
		if err := source.Save(projectRoot, contextName, overrides, overrides, []string{"provider"}, true, persistencePolicyInput{}); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		content, err := os.ReadFile(valuesPath)
		if err != nil {
			t.Fatalf("Expected values.yaml to be readable, got %v", err)
		}
		if contains(string(content), "provider:") {
			t.Errorf("Expected provider to be removed, got %s", string(content))
		}
		if !contains(string(content), "id: abc123") || !contains(string(content), "platform: docker") {
			t.Errorf("Expected id and the new platform to remain, got %s", string(content))
		}
	})

	t.Run("SkipsTheWriteWhenNothingIsOverriddenOrDeleted", func(t *testing.T) {
		source := newValuesSource(NewShims(), nil, newPersistencePolicy())
		projectRoot := t.TempDir()
		contextName := "local"
		contextDir := filepath.Join(projectRoot, "contexts", contextName)
		if err := os.MkdirAll(contextDir, 0755); err != nil {
			t.Fatalf("Expected no error creating context dir, got %v", err)
		}
		initial := "id: abc123\n"
		valuesPath := filepath.Join(contextDir, "values.yaml")
		if err := os.WriteFile(valuesPath, []byte(initial), 0644); err != nil {
			t.Fatalf("Expected no error writing initial values file, got %v", err)
		}

		data := map[string]any{"id": "abc123"}
		if err := source.Save(projectRoot, contextName, data, nil, nil, true, persistencePolicyInput{}); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		content, err := os.ReadFile(valuesPath)
		if err != nil {
			t.Fatalf("Expected values.yaml to be readable, got %v", err)
		}
		if string(content) != initial {
			t.Errorf("Expected values.yaml to be untouched, got %s", string(content))
		}
	})

	t.Run("FallsBackToAFullRewriteWhenTheExistingFileIsUnparseable", func(t *testing.T) {
		source := newValuesSource(NewShims(), nil, newPersistencePolicy())
		projectRoot := t.TempDir()
		contextName := "local"
		contextDir := filepath.Join(projectRoot, "contexts", contextName)
		if err := os.MkdirAll(contextDir, 0755); err != nil {
			t.Fatalf("Expected no error creating context dir, got %v", err)
		}
		valuesPath := filepath.Join(contextDir, "values.yaml")
		if err := os.WriteFile(valuesPath, []byte("not: valid: yaml: [\n"), 0644); err != nil {
			t.Fatalf("Expected no error writing initial values file, got %v", err)
		}

		data := map[string]any{"id": "abc123", "gcp": map[string]any{"project_id": "p"}}
		overrides := map[string]any{"id": "abc123"}
		if err := source.Save(projectRoot, contextName, data, overrides, nil, true, persistencePolicyInput{}); err != nil {
			t.Fatalf("Expected the write to fall back to the full config, got %v", err)
		}

		content, err := os.ReadFile(valuesPath)
		if err != nil {
			t.Fatalf("Expected values.yaml to be readable, got %v", err)
		}
		if !contains(string(content), "id:") || !contains(string(content), "gcp:") {
			t.Errorf("Expected the full config written as the fallback, got %s", string(content))
		}
	})
}

// getPathValue walks a decoded YAML map through keys, failing the test if any step is missing
// or not itself a map.
func getPathValue(t *testing.T, values map[string]any, keys ...string) any {
	t.Helper()
	var current any = values
	for _, key := range keys {
		m, ok := current.(map[string]any)
		if !ok {
			t.Fatalf("Expected a map while walking to %v, got %T at %q", keys, current, key)
		}
		current, ok = m[key]
		if !ok {
			t.Fatalf("Expected key %q present while walking %v", key, keys)
		}
	}
	return current
}
