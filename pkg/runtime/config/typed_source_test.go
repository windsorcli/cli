package config

import (
	"os"
	"path/filepath"
	"testing"
)

// =============================================================================
// Test Typed Source
// =============================================================================

func TestTypedSource_EnsureRoot(t *testing.T) {
	t.Run("CreatesRootWindsorYamlWhenMissing", func(t *testing.T) {
		source := newTypedSource(NewShims(), nil)
		projectRoot := t.TempDir()

		if err := source.EnsureRoot(projectRoot); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		rootPath := filepath.Join(projectRoot, "windsor.yaml")
		content, err := os.ReadFile(rootPath)
		if err != nil {
			t.Fatalf("Expected windsor.yaml to exist, got %v", err)
		}
		if !contains(string(content), "version: v1alpha1") {
			t.Errorf("Expected v1alpha1 root config, got %s", string(content))
		}
	})
}

func TestTypedSource_LoadRoot(t *testing.T) {
	t.Run("ReturnsContextMapFromRootContextsBlock", func(t *testing.T) {
		source := newTypedSource(NewShims(), nil)
		projectRoot := t.TempDir()
		rootConfig := `version: v1alpha1
contexts:
  local:
    provider: docker
    cluster:
      workers:
        count: 2
`
		if err := os.WriteFile(filepath.Join(projectRoot, "windsor.yaml"), []byte(rootConfig), 0644); err != nil {
			t.Fatalf("Expected no error writing root config, got %v", err)
		}

		contextMap, found, err := source.LoadRoot(projectRoot, "local", func([]byte) error { return nil })
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !found {
			t.Fatal("Expected root config to be found")
		}
		if contextMap["provider"] != "docker" {
			t.Errorf("Expected provider=docker, got %v", contextMap["provider"])
		}
	})
}

func TestTypedSource_LoadContext(t *testing.T) {
	t.Run("LoadsLegacyContextWindsorYaml", func(t *testing.T) {
		source := newTypedSource(NewShims(), nil)
		projectRoot := t.TempDir()
		contextName := "local"
		contextDir := filepath.Join(projectRoot, "contexts", contextName)
		if err := os.MkdirAll(contextDir, 0755); err != nil {
			t.Fatalf("Expected no error creating context dir, got %v", err)
		}
		if err := os.WriteFile(filepath.Join(contextDir, "windsor.yaml"), []byte("provider: aws\n"), 0644); err != nil {
			t.Fatalf("Expected no error writing context windsor.yaml, got %v", err)
		}

		contextMap, found, err := source.LoadContext(projectRoot, contextName)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !found {
			t.Fatal("Expected legacy context config to be found")
		}
		if contextMap["provider"] != "aws" {
			t.Errorf("Expected provider=aws, got %v", contextMap["provider"])
		}
	})

	t.Run("LoadsLegacyContextWindsorYmlFallback", func(t *testing.T) {
		source := newTypedSource(NewShims(), nil)
		projectRoot := t.TempDir()
		contextName := "local"
		contextDir := filepath.Join(projectRoot, "contexts", contextName)
		if err := os.MkdirAll(contextDir, 0755); err != nil {
			t.Fatalf("Expected no error creating context dir, got %v", err)
		}
		if err := os.WriteFile(filepath.Join(contextDir, "windsor.yml"), []byte("provider: gcp\n"), 0644); err != nil {
			t.Fatalf("Expected no error writing context windsor.yml, got %v", err)
		}

		contextMap, found, err := source.LoadContext(projectRoot, contextName)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !found {
			t.Fatal("Expected legacy context config fallback to be found")
		}
		if contextMap["provider"] != "gcp" {
			t.Errorf("Expected provider=gcp, got %v", contextMap["provider"])
		}
	})
}
