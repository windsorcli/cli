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

		if err := source.Save(projectRoot, contextName, data, true, persistencePolicyInput{IsDevMode: true}); err != nil {
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

		if err := source.Save(projectRoot, contextName, map[string]any{"cluster": map[string]any{"driver": "talos"}}, false, persistencePolicyInput{IsDevMode: true}); err != nil {
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
}
