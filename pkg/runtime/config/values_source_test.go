package config

import (
	"os"
	"path/filepath"
	"testing"
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
