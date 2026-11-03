package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/windsorcli/cli/pkg/runtime/config"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestConfigHandler_LoadConfigForContext(t *testing.T) {
	t.Run("LoadsConfigForSpecifiedContext", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := config.NewConfigHandler(mocks.Shell)

		tmpDir, _ := mocks.Shell.GetProjectRoot()
		rootConfig := `version: v1alpha1
contexts:
  test-context:
    provider: local
    dns:
      domain: example.com
`
		os.WriteFile(filepath.Join(tmpDir, "windsor.yaml"), []byte(rootConfig), 0644)

		err := handler.LoadConfigForContext("test-context")

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		platform := handler.GetString("platform")
		if platform != "local" {
			t.Errorf("Expected platform='local', got '%s'", platform)
		}

		domain := handler.GetString("dns.domain")
		if domain != "example.com" {
			t.Errorf("Expected dns.domain='example.com', got '%s'", domain)
		}
	})

	t.Run("DoesNotChangeCurrentContext", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := config.NewConfigHandler(mocks.Shell)
		handler.SetContext("original-context")

		tmpDir, _ := mocks.Shell.GetProjectRoot()
		rootConfig := `version: v1alpha1
contexts:
  other-context:
    provider: aws
`
		os.WriteFile(filepath.Join(tmpDir, "windsor.yaml"), []byte(rootConfig), 0644)

		err := handler.LoadConfigForContext("other-context")
		if err != nil {
			t.Fatalf("LoadConfigForContext failed: %v", err)
		}

		currentContext := handler.GetContext()
		if currentContext != "original-context" {
			t.Errorf("Expected current context to remain 'original-context', got '%s'", currentContext)
		}
	})

	t.Run("LoadsContextSpecificFiles", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := config.NewConfigHandler(mocks.Shell)

		tmpDir, _ := mocks.Shell.GetProjectRoot()
		contextDir := filepath.Join(tmpDir, "contexts", "test-context")
		os.MkdirAll(contextDir, 0755)
		contextConfig := `provider: docker
cluster:
  enabled: true
  driver: talos
`
		os.WriteFile(filepath.Join(contextDir, "windsor.yaml"), []byte(contextConfig), 0644)

		err := handler.LoadConfigForContext("test-context")

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		platform := handler.GetString("platform")
		if platform != "docker" {
			t.Errorf("Expected platform='docker', got '%s'", platform)
		}

		driver := handler.GetString("cluster.driver")
		if driver != "talos" {
			t.Errorf("Expected cluster.driver='talos', got '%s'", driver)
		}
	})

	t.Run("LoadsValuesYamlForContext", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := config.NewConfigHandler(mocks.Shell)

		tmpDir, _ := mocks.Shell.GetProjectRoot()
		contextDir := filepath.Join(tmpDir, "contexts", "test-context")
		os.MkdirAll(contextDir, 0755)
		valuesContent := `dev: true
custom_key: custom_value
`
		os.WriteFile(filepath.Join(contextDir, "values.yaml"), []byte(valuesContent), 0644)

		err := handler.LoadConfigForContext("test-context")

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		dev := handler.GetBool("dev")
		if !dev {
			t.Error("Expected dev=true")
		}

		customKey := handler.GetString("custom_key")
		if customKey != "custom_value" {
			t.Errorf("Expected custom_key='custom_value', got '%s'", customKey)
		}
	})

	t.Run("PanicsWhenShellNotInitialized", func(t *testing.T) {
		// When NewConfigHandler is called with nil shell
		// Then it should panic
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected panic when shell is nil")
			}
		}()
		_ = config.NewConfigHandler(nil)
	})
}
