package config

import (
	"os"
	"path/filepath"
	"testing"
)

// =============================================================================
// Test Workstation Source
// =============================================================================

func TestWorkstationSource_Load(t *testing.T) {
	t.Run("ReturnsNotFoundWhenWorkstationYamlMissing", func(t *testing.T) {
		source := newWorkstationSource(NewShims(), newPersistencePolicy())
		projectRoot := t.TempDir()

		values, found, err := source.Load(projectRoot, "local")
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if found {
			t.Fatal("Expected workstation.yaml to be reported missing")
		}
		if values != nil {
			t.Errorf("Expected nil map, got %v", values)
		}
	})

	t.Run("LoadsWorkstationYamlWhenPresent", func(t *testing.T) {
		source := newWorkstationSource(NewShims(), newPersistencePolicy())
		projectRoot := t.TempDir()
		contextName := "local"
		statePath := source.StatePath(projectRoot, contextName)
		if err := os.MkdirAll(filepath.Dir(statePath), 0755); err != nil {
			t.Fatalf("Expected no error creating state dir, got %v", err)
		}
		if err := os.WriteFile(statePath, []byte("workstation:\n    runtime: colima\n"), 0644); err != nil {
			t.Fatalf("Expected no error writing workstation.yaml, got %v", err)
		}

		values, found, err := source.Load(projectRoot, contextName)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !found {
			t.Fatal("Expected workstation.yaml to be found")
		}
		workstation, ok := values["workstation"].(map[string]any)
		if !ok {
			t.Fatalf("Expected workstation map, got %T", values["workstation"])
		}
		if workstation["runtime"] != "colima" {
			t.Errorf("Expected workstation.runtime=colima, got %v", workstation["runtime"])
		}
	})
}

func TestWorkstationSource_Save(t *testing.T) {
	t.Run("WritesWorkstationPartitionForDevInput", func(t *testing.T) {
		source := newWorkstationSource(NewShims(), newPersistencePolicy())
		projectRoot := t.TempDir()
		contextName := "local"
		data := map[string]any{
			"provider":    "docker",
			"platform":    "docker",
			"workstation": map[string]any{"runtime": "colima"},
		}

		if err := source.Save(projectRoot, contextName, data, persistencePolicyInput{IsDevMode: true}); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		statePath := source.StatePath(projectRoot, contextName)
		content, err := os.ReadFile(statePath)
		if err != nil {
			t.Fatalf("Expected workstation.yaml to be written, got %v", err)
		}
		stateStr := string(content)
		if !contains(stateStr, "workstation:") {
			t.Errorf("Expected workstation section, got %s", stateStr)
		}
		if !contains(stateStr, "runtime: colima") {
			t.Errorf("Expected runtime field, got %s", stateStr)
		}
		if !contains(stateStr, "platform: docker") {
			t.Errorf("Expected platform in workstation file for dev input, got %s", stateStr)
		}
		if contains(stateStr, "provider:") {
			t.Errorf("Expected provider excluded from workstation file, got %s", stateStr)
		}
	})

	t.Run("DeletesStateFileWhenNoWorkstationPartition", func(t *testing.T) {
		source := newWorkstationSource(NewShims(), newPersistencePolicy())
		projectRoot := t.TempDir()
		contextName := "default"
		statePath := source.StatePath(projectRoot, contextName)
		if err := os.MkdirAll(filepath.Dir(statePath), 0755); err != nil {
			t.Fatalf("Expected no error creating state dir, got %v", err)
		}
		if err := os.WriteFile(statePath, []byte("workstation:\n    runtime: colima\n"), 0644); err != nil {
			t.Fatalf("Expected no error writing initial workstation file, got %v", err)
		}

		if err := source.Save(projectRoot, contextName, map[string]any{"provider": "aws"}, persistencePolicyInput{}); err != nil {
			t.Fatalf("Expected no error when saving empty workstation partition, got %v", err)
		}
		if _, err := os.Stat(statePath); !os.IsNotExist(err) {
			t.Fatalf("Expected workstation state file to be removed, stat err=%v", err)
		}
	})
}
