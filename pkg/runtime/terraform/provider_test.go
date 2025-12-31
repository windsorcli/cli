package terraform

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/shell"
	"github.com/windsorcli/cli/pkg/runtime/tools"
)

// =============================================================================
// Test Setup
// =============================================================================

func setupProvider(t *testing.T) *terraformProvider {
	t.Helper()

	configHandler := config.NewMockConfigHandler()
	mockShell := shell.NewMockShell()
	toolsManager := tools.NewMockToolsManager()

	mockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
		return "", errors.New("ExecSilent not mocked in test")
	}

	provider := NewTerraformProvider(configHandler, mockShell, toolsManager)
	return provider.(*terraformProvider)
}

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewTerraformProvider(t *testing.T) {
	t.Run("CreatesProviderWithEmptyCache", func(t *testing.T) {
		provider := setupProvider(t)

		if provider == nil {
			t.Fatal("Expected provider to be created")
		}

		if provider.cache == nil {
			t.Error("Expected cache to be initialized")
		}

		if len(provider.cache) != 0 {
			t.Errorf("Expected empty cache, got %d entries", len(provider.cache))
		}

		if provider.Shims == nil {
			t.Error("Expected shims to be initialized")
		}
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestTerraformProvider_ClearCache(t *testing.T) {
	t.Run("ClearsAllCachedOutputsAndComponents", func(t *testing.T) {
		provider := setupProvider(t)

		provider.mu.Lock()
		provider.cache["component1"] = map[string]any{"output1": "value1"}
		provider.cache["component2"] = map[string]any{"output2": "value2"}
		provider.components = []blueprintv1alpha1.TerraformComponent{{Path: "test"}}
		provider.mu.Unlock()

		provider.ClearCache()

		if len(provider.cache) != 0 {
			t.Errorf("Expected cache to be empty after ClearCache, got %d entries", len(provider.cache))
		}

		if provider.components != nil {
			t.Error("Expected components to be cleared after ClearCache")
		}
	})
}

func TestTerraformProvider_GetTerraformComponents(t *testing.T) {
	t.Run("ReturnsCachedComponents", func(t *testing.T) {
		provider := setupProvider(t)

		expectedComponents := []blueprintv1alpha1.TerraformComponent{
			{Path: "test/path"},
		}

		provider.mu.Lock()
		provider.components = expectedComponents
		provider.mu.Unlock()

		components := provider.GetTerraformComponents()

		if len(components) != len(expectedComponents) {
			t.Errorf("Expected %d components, got %d", len(expectedComponents), len(components))
		}

		if components[0].Path != expectedComponents[0].Path {
			t.Errorf("Expected component path to be 'test/path', got %s", components[0].Path)
		}
	})

	t.Run("LoadsComponentsFromBlueprint", func(t *testing.T) {
		provider := setupProvider(t)
		mockConfig := provider.configHandler.(*config.MockConfigHandler)
		mockShell := provider.shell.(*shell.MockShell)

		configRoot := "/test/config"
		mockConfig.GetConfigRootFunc = func() (string, error) {
			return configRoot, nil
		}

		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
metadata:
  name: test
terraform:
  - path: test/path
    name: test-component`

		provider.Shims.ReadFile = func(path string) ([]byte, error) {
			if path == filepath.Join(configRoot, "blueprint.yaml") {
				return []byte(blueprintYAML), nil
			}
			return nil, os.ErrNotExist
		}

		mockShell.GetProjectRootFunc = func() (string, error) {
			return "/test/project", nil
		}

		mockConfig.GetContextFunc = func() string {
			return "default"
		}

		components := provider.GetTerraformComponents()

		if len(components) != 1 {
			t.Fatalf("Expected 1 component, got %d", len(components))
		}

		if components[0].Path != "test/path" {
			t.Errorf("Expected component path to be 'test/path', got %s", components[0].Path)
		}

		if components[0].Name != "test-component" {
			t.Errorf("Expected component name to be 'test-component', got %s", components[0].Name)
		}
	})

	t.Run("HandlesMissingConfigRoot", func(t *testing.T) {
		provider := setupProvider(t)
		mockConfig := provider.configHandler.(*config.MockConfigHandler)

		mockConfig.GetConfigRootFunc = func() (string, error) {
			return "", errors.New("config root not found")
		}

		components := provider.GetTerraformComponents()

		if len(components) != 0 {
			t.Errorf("Expected empty components on error, got %d", len(components))
		}
	})

	t.Run("HandlesMissingBlueprintFile", func(t *testing.T) {
		provider := setupProvider(t)
		mockConfig := provider.configHandler.(*config.MockConfigHandler)

		configRoot := "/test/config"
		mockConfig.GetConfigRootFunc = func() (string, error) {
			return configRoot, nil
		}

		provider.Shims.ReadFile = func(path string) ([]byte, error) {
			return nil, os.ErrNotExist
		}

		components := provider.GetTerraformComponents()

		if len(components) != 0 {
			t.Errorf("Expected empty components on file error, got %d", len(components))
		}
	})

	t.Run("HandlesInvalidYAML", func(t *testing.T) {
		provider := setupProvider(t)
		mockConfig := provider.configHandler.(*config.MockConfigHandler)

		configRoot := "/test/config"
		mockConfig.GetConfigRootFunc = func() (string, error) {
			return configRoot, nil
		}

		provider.Shims.ReadFile = func(path string) ([]byte, error) {
			if path == filepath.Join(configRoot, "blueprint.yaml") {
				return []byte("invalid: yaml: [unclosed"), nil
			}
			return nil, os.ErrNotExist
		}

		components := provider.GetTerraformComponents()

		if len(components) != 0 {
			t.Errorf("Expected empty components on YAML error, got %d", len(components))
		}
	})

	t.Run("SetsFullPathForSourcedComponents", func(t *testing.T) {
		provider := setupProvider(t)
		mockConfig := provider.configHandler.(*config.MockConfigHandler)
		mockShell := provider.shell.(*shell.MockShell)

		configRoot := "/test/config"
		mockConfig.GetConfigRootFunc = func() (string, error) {
			return configRoot, nil
		}

		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
metadata:
  name: test
terraform:
  - path: test/path
    source: config`

		provider.Shims.ReadFile = func(path string) ([]byte, error) {
			if path == filepath.Join(configRoot, "blueprint.yaml") {
				return []byte(blueprintYAML), nil
			}
			return nil, os.ErrNotExist
		}

		mockShell.GetProjectRootFunc = func() (string, error) {
			return "/test/project", nil
		}

		mockConfig.GetContextFunc = func() string {
			return "default"
		}

		components := provider.GetTerraformComponents()

		expectedPath := filepath.Join("/test/project", ".windsor", "contexts", "default", "terraform", "test/path")
		if components[0].FullPath != expectedPath {
			t.Errorf("Expected FullPath to be %s, got %s", expectedPath, components[0].FullPath)
		}
	})

	t.Run("SetsFullPathForLocalComponents", func(t *testing.T) {
		provider := setupProvider(t)
		mockConfig := provider.configHandler.(*config.MockConfigHandler)
		mockShell := provider.shell.(*shell.MockShell)

		configRoot := "/test/config"
		mockConfig.GetConfigRootFunc = func() (string, error) {
			return configRoot, nil
		}

		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
metadata:
  name: test
terraform:
  - path: test/path`

		provider.Shims.ReadFile = func(path string) ([]byte, error) {
			if path == filepath.Join(configRoot, "blueprint.yaml") {
				return []byte(blueprintYAML), nil
			}
			return nil, os.ErrNotExist
		}

		mockShell.GetProjectRootFunc = func() (string, error) {
			return "/test/project", nil
		}

		components := provider.GetTerraformComponents()

		expectedPath := filepath.Join("/test/project", "terraform", "test/path")
		if components[0].FullPath != expectedPath {
			t.Errorf("Expected FullPath to be %s, got %s", expectedPath, components[0].FullPath)
		}
	})
}

func TestTerraformProvider_GenerateBackendOverride(t *testing.T) {
	t.Run("CreatesLocalBackendOverride", func(t *testing.T) {
		provider := setupProvider(t)
		mockConfig := provider.configHandler.(*config.MockConfigHandler)

		mockConfig.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "local"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		var writtenPath string
		var writtenData []byte
		provider.Shims.WriteFile = func(path string, data []byte, perm os.FileMode) error {
			writtenPath = path
			writtenData = data
			return nil
		}

		err := provider.GenerateBackendOverride("/test/dir")

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		expectedPath := filepath.Join("/test/dir", "backend_override.tf")
		if writtenPath != expectedPath {
			t.Errorf("Expected to write to %s, got %s", expectedPath, writtenPath)
		}

		expectedConfig := "terraform {\n  backend \"local\" {}\n}"
		if string(writtenData) != expectedConfig {
			t.Errorf("Expected backend config to be %q, got %q", expectedConfig, string(writtenData))
		}
	})

	t.Run("CreatesS3BackendOverride", func(t *testing.T) {
		provider := setupProvider(t)
		mockConfig := provider.configHandler.(*config.MockConfigHandler)

		mockConfig.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "s3"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		provider.Shims.WriteFile = func(path string, data []byte, perm os.FileMode) error {
			return nil
		}

		err := provider.GenerateBackendOverride("/test/dir")

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
	})

	t.Run("CreatesKubernetesBackendOverride", func(t *testing.T) {
		provider := setupProvider(t)
		mockConfig := provider.configHandler.(*config.MockConfigHandler)

		mockConfig.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "kubernetes"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		provider.Shims.WriteFile = func(path string, data []byte, perm os.FileMode) error {
			return nil
		}

		err := provider.GenerateBackendOverride("/test/dir")

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
	})

	t.Run("CreatesAzurermBackendOverride", func(t *testing.T) {
		provider := setupProvider(t)
		mockConfig := provider.configHandler.(*config.MockConfigHandler)

		mockConfig.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "azurerm"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		provider.Shims.WriteFile = func(path string, data []byte, perm os.FileMode) error {
			return nil
		}

		err := provider.GenerateBackendOverride("/test/dir")

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
	})

	t.Run("RemovesBackendOverrideForNone", func(t *testing.T) {
		provider := setupProvider(t)
		mockConfig := provider.configHandler.(*config.MockConfigHandler)

		mockConfig.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "none"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		var removedPath string
		provider.Shims.Stat = func(path string) (os.FileInfo, error) {
			return nil, nil
		}
		provider.Shims.Remove = func(path string) error {
			removedPath = path
			return nil
		}

		err := provider.GenerateBackendOverride("/test/dir")

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		expectedPath := filepath.Join("/test/dir", "backend_override.tf")
		if removedPath != expectedPath {
			t.Errorf("Expected to remove %s, got %s", expectedPath, removedPath)
		}
	})

	t.Run("HandlesUnsupportedBackend", func(t *testing.T) {
		provider := setupProvider(t)
		mockConfig := provider.configHandler.(*config.MockConfigHandler)

		mockConfig.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "unsupported"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		err := provider.GenerateBackendOverride("/test/dir")

		if err == nil {
			t.Fatal("Expected error for unsupported backend")
		}

		if err.Error() != "unsupported backend: unsupported" {
			t.Errorf("Expected unsupported backend error, got %v", err)
		}
	})

	t.Run("HandlesWriteFileError", func(t *testing.T) {
		provider := setupProvider(t)
		mockConfig := provider.configHandler.(*config.MockConfigHandler)

		mockConfig.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "local"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		provider.Shims.WriteFile = func(path string, data []byte, perm os.FileMode) error {
			return errors.New("write failed")
		}

		err := provider.GenerateBackendOverride("/test/dir")

		if err == nil {
			t.Fatal("Expected error on write failure")
		}
	})

	t.Run("HandlesRemoveError", func(t *testing.T) {
		provider := setupProvider(t)
		mockConfig := provider.configHandler.(*config.MockConfigHandler)

		mockConfig.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "none"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		provider.Shims.Stat = func(path string) (os.FileInfo, error) {
			return nil, nil
		}
		provider.Shims.Remove = func(path string) error {
			return errors.New("remove failed")
		}

		err := provider.GenerateBackendOverride("/test/dir")

		if err == nil {
			t.Fatal("Expected error on remove failure")
		}
	})
}

func TestTerraformProvider_GetOutputs(t *testing.T) {
	t.Run("ReturnsCachedOutputs", func(t *testing.T) {
		provider := setupProvider(t)

		expectedOutputs := map[string]any{
			"output1": "value1",
			"output2": 42,
		}

		provider.mu.Lock()
		provider.cache["test-component"] = expectedOutputs
		provider.mu.Unlock()

		outputs, err := provider.GetOutputs("test-component")

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if len(outputs) != len(expectedOutputs) {
			t.Errorf("Expected %d outputs, got %d", len(expectedOutputs), len(outputs))
		}

		if outputs["output1"] != expectedOutputs["output1"] {
			t.Errorf("Expected output1 to be 'value1', got %v", outputs["output1"])
		}

		if outputs["output2"] != expectedOutputs["output2"] {
			t.Errorf("Expected output2 to be 42, got %v", outputs["output2"])
		}
	})

	t.Run("CachesOutputsAfterFirstCall", func(t *testing.T) {
		provider := setupProvider(t)

		expectedOutputs := map[string]any{
			"output1": "value1",
		}

		provider.mu.Lock()
		provider.cache["test-component"] = expectedOutputs
		provider.mu.Unlock()

		outputs1, err1 := provider.GetOutputs("test-component")
		if err1 != nil {
			t.Fatalf("Expected no error on first call, got %v", err1)
		}

		outputs2, err2 := provider.GetOutputs("test-component")
		if err2 != nil {
			t.Fatalf("Expected no error on second call, got %v", err2)
		}

		if outputs1["output1"] != outputs2["output1"] {
			t.Error("Expected both calls to return same cached value")
		}

		if len(provider.cache) != 1 {
			t.Errorf("Expected cache to have 1 entry, got %d", len(provider.cache))
		}
	})

	t.Run("ReturnsEmptyMapWhenComponentNotFound", func(t *testing.T) {
		provider := setupProvider(t)
		mockConfig := provider.configHandler.(*config.MockConfigHandler)

		configRoot := "/test/config"
		mockConfig.GetConfigRootFunc = func() (string, error) {
			return configRoot, nil
		}

		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
metadata:
  name: test
terraform:
  - path: other/path`

		provider.Shims.ReadFile = func(path string) ([]byte, error) {
			if path == filepath.Join(configRoot, "blueprint.yaml") {
				return []byte(blueprintYAML), nil
			}
			return nil, os.ErrNotExist
		}

		mockShell := provider.shell.(*shell.MockShell)
		mockShell.GetProjectRootFunc = func() (string, error) {
			return "/test/project", nil
		}

		mockConfig.GetContextFunc = func() string {
			return "default"
		}

		outputs, err := provider.GetOutputs("nonexistent-component")

		if err != nil {
			t.Fatalf("Expected no error for nonexistent component, got %v", err)
		}

		if len(outputs) != 0 {
			t.Errorf("Expected empty outputs for nonexistent component, got %d", len(outputs))
		}
	})

	t.Run("CapturesOutputsFromTerraform", func(t *testing.T) {
		provider := setupProvider(t)
		mockConfig := provider.configHandler.(*config.MockConfigHandler)
		mockShell := provider.shell.(*shell.MockShell)

		configRoot := "/test/config"
		mockConfig.GetConfigRootFunc = func() (string, error) {
			return configRoot, nil
		}

		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
metadata:
  name: test
terraform:
  - path: test/path
    name: test-component`

		provider.Shims.ReadFile = func(path string) ([]byte, error) {
			if path == filepath.Join(configRoot, "blueprint.yaml") {
				return []byte(blueprintYAML), nil
			}
			return nil, os.ErrNotExist
		}

		mockShell.GetProjectRootFunc = func() (string, error) {
			return "/test/project", nil
		}

		mockConfig.GetWindsorScratchPathFunc = func() (string, error) {
			return "/test/scratch", nil
		}

		mockConfig.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "none"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		mockConfig.GetContextFunc = func() string {
			return "default"
		}

		provider.Shims.Getenv = func(key string) string {
			return ""
		}

		provider.Shims.Setenv = func(key, value string) error {
			return nil
		}

		provider.Shims.Stat = func(path string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		provider.Shims.Remove = func(path string) error {
			return nil
		}

		provider.Shims.WriteFile = func(path string, data []byte, perm os.FileMode) error {
			return nil
		}

		mockToolsManager := provider.toolsManager.(*tools.MockToolsManager)
		mockToolsManager.GetTerraformCommandFunc = func() string {
			return "terraform"
		}

		execCallCount := 0
		mockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			execCallCount++
			if command == "terraform" && len(args) >= 2 && args[1] == "output" {
				return `{"output1": {"value": "val1"}, "output2": {"value": 42}}`, nil
			}
			if command == "terraform" && len(args) >= 2 && args[1] == "init" {
				return "", nil
			}
			return "", errors.New("unexpected command")
		}

		components := provider.GetTerraformComponents()
		if len(components) == 0 {
			t.Fatal("Expected components to be loaded from blueprint")
		}

		outputs, err := provider.GetOutputs("test-component")

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if execCallCount == 0 {
			t.Errorf("Expected ExecSilent to be called (components found: %d)", len(components))
		}

		if len(outputs) != 2 {
			t.Errorf("Expected 2 outputs, got %d (execCallCount: %d, components: %d)", len(outputs), execCallCount, len(components))
		}

		if outputs["output1"] != "val1" {
			t.Errorf("Expected output1 to be 'val1', got %v", outputs["output1"])
		}

		if outputs["output2"] != float64(42) {
			t.Errorf("Expected output2 to be 42, got %v", outputs["output2"])
		}
	})

	t.Run("HandlesEmptyTerraformOutput", func(t *testing.T) {
		provider := setupProvider(t)
		mockConfig := provider.configHandler.(*config.MockConfigHandler)
		mockShell := provider.shell.(*shell.MockShell)

		configRoot := "/test/config"
		mockConfig.GetConfigRootFunc = func() (string, error) {
			return configRoot, nil
		}

		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
metadata:
  name: test
terraform:
  - path: test/path
    name: test-component`

		provider.Shims.ReadFile = func(path string) ([]byte, error) {
			if path == filepath.Join(configRoot, "blueprint.yaml") {
				return []byte(blueprintYAML), nil
			}
			return nil, os.ErrNotExist
		}

		mockShell.GetProjectRootFunc = func() (string, error) {
			return "/test/project", nil
		}

		mockConfig.GetWindsorScratchPathFunc = func() (string, error) {
			return "/test/scratch", nil
		}

		mockConfig.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "local"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		mockConfig.GetContextFunc = func() string {
			return "default"
		}

		provider.Shims.Getenv = func(key string) string {
			return ""
		}

		provider.Shims.Setenv = func(key, value string) error {
			return nil
		}

		provider.Shims.Stat = func(path string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		mockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if len(args) > 1 && args[1] == "output" {
				return "{}", nil
			}
			return "", nil
		}

		mockToolsManager := provider.toolsManager.(*tools.MockToolsManager)
		mockToolsManager.GetTerraformCommandFunc = func() string {
			return "terraform"
		}

		outputs, err := provider.GetOutputs("test-component")

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if len(outputs) != 0 {
			t.Errorf("Expected empty outputs, got %d", len(outputs))
		}
	})

	t.Run("HandlesTerraformInitFallback", func(t *testing.T) {
		provider := setupProvider(t)
		mockConfig := provider.configHandler.(*config.MockConfigHandler)
		mockShell := provider.shell.(*shell.MockShell)

		configRoot := "/test/config"
		mockConfig.GetConfigRootFunc = func() (string, error) {
			return configRoot, nil
		}

		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
metadata:
  name: test
terraform:
  - path: test/path
    name: test-component`

		provider.Shims.ReadFile = func(path string) ([]byte, error) {
			if path == filepath.Join(configRoot, "blueprint.yaml") {
				return []byte(blueprintYAML), nil
			}
			return nil, os.ErrNotExist
		}

		mockShell.GetProjectRootFunc = func() (string, error) {
			return "/test/project", nil
		}

		mockConfig.GetWindsorScratchPathFunc = func() (string, error) {
			return "/test/scratch", nil
		}

		mockConfig.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "local"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		mockConfig.GetContextFunc = func() string {
			return "default"
		}

		provider.Shims.Getenv = func(key string) string {
			return ""
		}

		provider.Shims.Setenv = func(key, value string) error {
			return nil
		}

		provider.Shims.Stat = func(path string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		provider.Shims.WriteFile = func(path string, data []byte, perm os.FileMode) error {
			return nil
		}

		mockToolsManager := provider.toolsManager.(*tools.MockToolsManager)
		mockToolsManager.GetTerraformCommandFunc = func() string {
			return "terraform"
		}

		outputCallCount := 0
		mockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "terraform" && len(args) >= 2 && args[1] == "output" {
				outputCallCount++
				if outputCallCount == 1 {
					return "", errors.New("output failed")
				}
				return `{"output1": {"value": "val1"}}`, nil
			}
			if command == "terraform" && len(args) >= 2 && args[1] == "init" {
				return "", nil
			}
			return "", nil
		}

		outputs, err := provider.GetOutputs("test-component")

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if len(outputs) != 1 {
			t.Errorf("Expected 1 output, got %d (outputCallCount: %d)", len(outputs), outputCallCount)
		}
	})

	t.Run("HandlesSetenvError", func(t *testing.T) {
		provider := setupProvider(t)
		mockConfig := provider.configHandler.(*config.MockConfigHandler)
		mockShell := provider.shell.(*shell.MockShell)

		configRoot := "/test/config"
		mockConfig.GetConfigRootFunc = func() (string, error) {
			return configRoot, nil
		}

		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
metadata:
  name: test
terraform:
  - path: test/path
    name: test-component`

		provider.Shims.ReadFile = func(path string) ([]byte, error) {
			if path == filepath.Join(configRoot, "blueprint.yaml") {
				return []byte(blueprintYAML), nil
			}
			return nil, os.ErrNotExist
		}

		mockShell.GetProjectRootFunc = func() (string, error) {
			return "/test/project", nil
		}

		mockConfig.GetWindsorScratchPathFunc = func() (string, error) {
			return "/test/scratch", nil
		}

		mockConfig.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "local"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		mockConfig.GetContextFunc = func() string {
			return "default"
		}

		provider.Shims.Getenv = func(key string) string {
			return ""
		}

		provider.Shims.Setenv = func(key, value string) error {
			return errors.New("setenv failed")
		}

		outputs, err := provider.GetOutputs("test-component")

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if len(outputs) != 0 {
			t.Errorf("Expected empty outputs when setenv fails, got %d", len(outputs))
		}
	})

	t.Run("HandlesBackendOverrideError", func(t *testing.T) {
		provider := setupProvider(t)
		mockConfig := provider.configHandler.(*config.MockConfigHandler)
		mockShell := provider.shell.(*shell.MockShell)

		configRoot := "/test/config"
		mockConfig.GetConfigRootFunc = func() (string, error) {
			return configRoot, nil
		}

		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
metadata:
  name: test
terraform:
  - path: test/path
    name: test-component`

		provider.Shims.ReadFile = func(path string) ([]byte, error) {
			if path == filepath.Join(configRoot, "blueprint.yaml") {
				return []byte(blueprintYAML), nil
			}
			return nil, os.ErrNotExist
		}

		mockShell.GetProjectRootFunc = func() (string, error) {
			return "/test/project", nil
		}

		mockConfig.GetWindsorScratchPathFunc = func() (string, error) {
			return "/test/scratch", nil
		}

		mockConfig.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "unsupported"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		mockConfig.GetContextFunc = func() string {
			return "default"
		}

		provider.Shims.Getenv = func(key string) string {
			return ""
		}

		provider.Shims.Setenv = func(key, value string) error {
			return nil
		}

		outputs, err := provider.GetOutputs("test-component")

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if len(outputs) != 0 {
			t.Errorf("Expected empty outputs when backend override fails, got %d", len(outputs))
		}
	})
}

// =============================================================================
// Test Private Methods (via public methods that call them)
// =============================================================================

func TestTerraformProvider_GetTerraformComponent(t *testing.T) {
	t.Run("FindsComponentByPath", func(t *testing.T) {
		provider := setupProvider(t)
		mockConfig := provider.configHandler.(*config.MockConfigHandler)
		mockShell := provider.shell.(*shell.MockShell)

		configRoot := "/test/config"
		mockConfig.GetConfigRootFunc = func() (string, error) {
			return configRoot, nil
		}

		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
metadata:
  name: test
terraform:
  - path: test/path`

		provider.Shims.ReadFile = func(path string) ([]byte, error) {
			if path == filepath.Join(configRoot, "blueprint.yaml") {
				return []byte(blueprintYAML), nil
			}
			return nil, os.ErrNotExist
		}

		mockShell.GetProjectRootFunc = func() (string, error) {
			return "/test/project", nil
		}

		mockConfig.GetContextFunc = func() string {
			return "default"
		}

		component := provider.GetTerraformComponent("test/path")

		if component == nil {
			t.Fatal("Expected component to be found")
		}

		if component.Path != "test/path" {
			t.Errorf("Expected component path to be 'test/path', got %s", component.Path)
		}
	})

	t.Run("FindsComponentByName", func(t *testing.T) {
		provider := setupProvider(t)
		mockConfig := provider.configHandler.(*config.MockConfigHandler)
		mockShell := provider.shell.(*shell.MockShell)

		configRoot := "/test/config"
		mockConfig.GetConfigRootFunc = func() (string, error) {
			return configRoot, nil
		}

		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
metadata:
  name: test
terraform:
  - path: test/path
    name: test-component`

		provider.Shims.ReadFile = func(path string) ([]byte, error) {
			if path == filepath.Join(configRoot, "blueprint.yaml") {
				return []byte(blueprintYAML), nil
			}
			return nil, os.ErrNotExist
		}

		mockShell.GetProjectRootFunc = func() (string, error) {
			return "/test/project", nil
		}

		mockConfig.GetContextFunc = func() string {
			return "default"
		}

		component := provider.GetTerraformComponent("test-component")

		if component == nil {
			t.Fatal("Expected component to be found")
		}

		if component.Name != "test-component" {
			t.Errorf("Expected component name to be 'test-component', got %s", component.Name)
		}
	})

	t.Run("ReturnsNilWhenComponentNotFound", func(t *testing.T) {
		provider := setupProvider(t)
		mockConfig := provider.configHandler.(*config.MockConfigHandler)
		mockShell := provider.shell.(*shell.MockShell)

		configRoot := "/test/config"
		mockConfig.GetConfigRootFunc = func() (string, error) {
			return configRoot, nil
		}

		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
metadata:
  name: test
terraform:
  - path: test/path`

		provider.Shims.ReadFile = func(path string) ([]byte, error) {
			if path == filepath.Join(configRoot, "blueprint.yaml") {
				return []byte(blueprintYAML), nil
			}
			return nil, os.ErrNotExist
		}

		mockShell.GetProjectRootFunc = func() (string, error) {
			return "/test/project", nil
		}

		mockConfig.GetContextFunc = func() string {
			return "default"
		}

		component := provider.GetTerraformComponent("nonexistent")

		if component != nil {
			t.Error("Expected component to be nil when not found")
		}
	})
}

func TestTerraformProvider_ResolveModulePath(t *testing.T) {
	t.Run("ResolvesPathForComponentWithName", func(t *testing.T) {
		provider := setupProvider(t)
		mockConfig := provider.configHandler.(*config.MockConfigHandler)

		mockConfig.GetWindsorScratchPathFunc = func() (string, error) {
			return "/test/scratch", nil
		}

		component := &blueprintv1alpha1.TerraformComponent{
			Name: "test-component",
		}

		path, err := provider.resolveModulePath(component)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		expected := filepath.Join("/test/scratch", "terraform", "test-component")
		if path != expected {
			t.Errorf("Expected path %s, got %s", expected, path)
		}
	})

	t.Run("ResolvesPathForComponentWithSource", func(t *testing.T) {
		provider := setupProvider(t)
		mockConfig := provider.configHandler.(*config.MockConfigHandler)

		mockConfig.GetWindsorScratchPathFunc = func() (string, error) {
			return "/test/scratch", nil
		}

		component := &blueprintv1alpha1.TerraformComponent{
			Path:   "test/path",
			Source: "config",
		}

		path, err := provider.resolveModulePath(component)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		expected := filepath.Join("/test/scratch", "terraform", "test/path")
		if path != expected {
			t.Errorf("Expected path %s, got %s", expected, path)
		}
	})

	t.Run("ResolvesPathForLocalComponent", func(t *testing.T) {
		provider := setupProvider(t)
		mockShell := provider.shell.(*shell.MockShell)

		mockShell.GetProjectRootFunc = func() (string, error) {
			return "/test/project", nil
		}

		component := &blueprintv1alpha1.TerraformComponent{
			Path: "test/path",
		}

		path, err := provider.resolveModulePath(component)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		expected := filepath.Join("/test/project", "terraform", "test/path")
		if path != expected {
			t.Errorf("Expected path %s, got %s", expected, path)
		}
	})

	t.Run("ReturnsErrorWhenScratchPathFails", func(t *testing.T) {
		provider := setupProvider(t)
		mockConfig := provider.configHandler.(*config.MockConfigHandler)

		mockConfig.GetWindsorScratchPathFunc = func() (string, error) {
			return "", errors.New("scratch path error")
		}

		component := &blueprintv1alpha1.TerraformComponent{
			Name: "test-component",
		}

		_, err := provider.resolveModulePath(component)

		if err == nil {
			t.Fatal("Expected error when scratch path fails")
		}
	})

	t.Run("ReturnsErrorWhenProjectRootFails", func(t *testing.T) {
		provider := setupProvider(t)
		mockShell := provider.shell.(*shell.MockShell)

		mockShell.GetProjectRootFunc = func() (string, error) {
			return "", errors.New("project root error")
		}

		component := &blueprintv1alpha1.TerraformComponent{
			Path: "test/path",
		}

		_, err := provider.resolveModulePath(component)

		if err == nil {
			t.Fatal("Expected error when project root fails")
		}
	})
}

func TestTerraformProvider_FindRelativeProjectPath(t *testing.T) {
	t.Run("FindsProjectPathInTerraformDirectory", func(t *testing.T) {
		provider := setupProvider(t)

		testPath := filepath.Join("/test", "project", "terraform", "component", "subdir")
		provider.Shims.Getwd = func() (string, error) {
			return testPath, nil
		}

		provider.Shims.Glob = func(pattern string) ([]string, error) {
			if pattern == filepath.Join(testPath, "*.tf") {
				return []string{filepath.Join(testPath, "main.tf")}, nil
			}
			return nil, nil
		}

		path, err := provider.FindRelativeProjectPath()

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		expected := "component/subdir"
		if path != expected {
			t.Errorf("Expected path %s, got %s", expected, path)
		}
	})

	t.Run("FindsProjectPathInContextsDirectory", func(t *testing.T) {
		provider := setupProvider(t)

		testPath := filepath.Join("/test", "project", ".windsor", "contexts", "local", "terraform", "component")
		provider.Shims.Getwd = func() (string, error) {
			return testPath, nil
		}

		provider.Shims.Glob = func(pattern string) ([]string, error) {
			if pattern == filepath.Join(testPath, "*.tf") {
				return []string{filepath.Join(testPath, "main.tf")}, nil
			}
			return nil, nil
		}

		path, err := provider.FindRelativeProjectPath()

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		expected := "component"
		if path != expected {
			t.Errorf("Expected path %s, got %s", expected, path)
		}
	})

	t.Run("ReturnsEmptyWhenNoTerraformFiles", func(t *testing.T) {
		provider := setupProvider(t)

		testPath := "/test/path"
		provider.Shims.Getwd = func() (string, error) {
			return testPath, nil
		}

		provider.Shims.Glob = func(pattern string) ([]string, error) {
			return nil, nil
		}

		path, err := provider.FindRelativeProjectPath()

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if path != "" {
			t.Errorf("Expected empty path, got %s", path)
		}
	})

	t.Run("ReturnsErrorWhenGetwdFails", func(t *testing.T) {
		provider := setupProvider(t)

		provider.Shims.Getwd = func() (string, error) {
			return "", errors.New("getwd failed")
		}

		_, err := provider.FindRelativeProjectPath()

		if err == nil {
			t.Fatal("Expected error when Getwd fails")
		}
	})

	t.Run("ReturnsErrorWhenGlobFails", func(t *testing.T) {
		provider := setupProvider(t)

		testPath := "/test/path"
		provider.Shims.Getwd = func() (string, error) {
			return testPath, nil
		}

		provider.Shims.Glob = func(pattern string) ([]string, error) {
			return nil, errors.New("glob failed")
		}

		_, err := provider.FindRelativeProjectPath()

		if err == nil {
			t.Fatal("Expected error when Glob fails")
		}
	})

	t.Run("AcceptsDirectoryParameter", func(t *testing.T) {
		provider := setupProvider(t)

		testPath := filepath.Join("/test", "project", "terraform", "component")
		provider.Shims.Glob = func(pattern string) ([]string, error) {
			if pattern == filepath.Join(testPath, "*.tf") {
				return []string{filepath.Join(testPath, "main.tf")}, nil
			}
			return nil, nil
		}

		path, err := provider.FindRelativeProjectPath(testPath)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		expected := "component"
		if path != expected {
			t.Errorf("Expected path %s, got %s", expected, path)
		}
	})

	t.Run("ReturnsEmptyWhenNoTerraformOrContextsDirectory", func(t *testing.T) {
		provider := setupProvider(t)

		testPath := filepath.Join("/test", "random", "path", "with", "tf", "files")
		provider.Shims.Getwd = func() (string, error) {
			return testPath, nil
		}

		provider.Shims.Glob = func(pattern string) ([]string, error) {
			if pattern == filepath.Join(testPath, "*.tf") {
				return []string{filepath.Join(testPath, "main.tf")}, nil
			}
			return nil, nil
		}

		path, err := provider.FindRelativeProjectPath()

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if path != "" {
			t.Errorf("Expected empty path when no terraform/contexts directory found, got %s", path)
		}
	})
}

func TestTerraformProvider_GenerateTerraformArgs(t *testing.T) {
	t.Run("GeneratesArgsForComponent", func(t *testing.T) {
		provider := setupProvider(t)
		mockConfig := provider.configHandler.(*config.MockConfigHandler)

		mockConfig.GetWindsorScratchPathFunc = func() (string, error) {
			return "/test/scratch", nil
		}

		mockConfig.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "local"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		configRoot := "/test/config"
		mockConfig.GetConfigRootFunc = func() (string, error) {
			return configRoot, nil
		}

		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
metadata:
  name: test
terraform:
  - path: test/path
    name: test-component`

		provider.Shims.ReadFile = func(path string) ([]byte, error) {
			if path == filepath.Join(configRoot, "blueprint.yaml") {
				return []byte(blueprintYAML), nil
			}
			return nil, os.ErrNotExist
		}

		mockShell := provider.shell.(*shell.MockShell)
		mockShell.GetProjectRootFunc = func() (string, error) {
			return "/test/project", nil
		}

		mockConfig.GetContextFunc = func() string {
			return "default"
		}

		args, err := provider.generateTerraformArgs("test-component")

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if args == nil {
			t.Fatal("Expected args to be generated")
		}

		expectedTFDataDir := filepath.Join("/test/scratch", "terraform", "test-component", ".terraform")
		if args.TFDataDir != expectedTFDataDir {
			t.Errorf("Expected TFDataDir %s, got %s", expectedTFDataDir, args.TFDataDir)
		}

		if len(args.InitArgs) == 0 {
			t.Error("Expected InitArgs to include backend config")
		}
	})

	t.Run("ReturnsErrorWhenScratchPathFails", func(t *testing.T) {
		provider := setupProvider(t)
		mockConfig := provider.configHandler.(*config.MockConfigHandler)

		mockConfig.GetWindsorScratchPathFunc = func() (string, error) {
			return "", errors.New("scratch path error")
		}

		_, err := provider.generateTerraformArgs("test-component")

		if err == nil {
			t.Fatal("Expected error when scratch path fails")
		}
	})

	t.Run("HandlesNoneBackend", func(t *testing.T) {
		provider := setupProvider(t)
		mockConfig := provider.configHandler.(*config.MockConfigHandler)

		mockConfig.GetWindsorScratchPathFunc = func() (string, error) {
			return "/test/scratch", nil
		}

		mockConfig.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "none"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		configRoot := "/test/config"
		mockConfig.GetConfigRootFunc = func() (string, error) {
			return configRoot, nil
		}

		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
metadata:
  name: test
terraform:
  - path: test/path`

		provider.Shims.ReadFile = func(path string) ([]byte, error) {
			if path == filepath.Join(configRoot, "blueprint.yaml") {
				return []byte(blueprintYAML), nil
			}
			return nil, os.ErrNotExist
		}

		mockShell := provider.shell.(*shell.MockShell)
		mockShell.GetProjectRootFunc = func() (string, error) {
			return "/test/project", nil
		}

		mockConfig.GetContextFunc = func() string {
			return "default"
		}

		args, err := provider.generateTerraformArgs("test/path")

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if len(args.InitArgs) != 0 {
			t.Errorf("Expected no InitArgs for none backend, got %d", len(args.InitArgs))
		}
	})
}

func TestTerraformProvider_RestoreEnvVar(t *testing.T) {
	t.Run("RestoresEnvVarWithValue", func(t *testing.T) {
		provider := setupProvider(t)

		var setKey, setValue string
		provider.Shims.Setenv = func(key, value string) error {
			setKey = key
			setValue = value
			return nil
		}

		provider.restoreEnvVar("TEST_VAR", "original-value")

		if setKey != "TEST_VAR" {
			t.Errorf("Expected to set TEST_VAR, got %s", setKey)
		}

		if setValue != "original-value" {
			t.Errorf("Expected to set original-value, got %s", setValue)
		}
	})

	t.Run("UnsetsEnvVarWhenEmpty", func(t *testing.T) {
		provider := setupProvider(t)

		var unsetKey string
		provider.Shims.Unsetenv = func(key string) error {
			unsetKey = key
			return nil
		}

		provider.restoreEnvVar("TEST_VAR", "")

		if unsetKey != "TEST_VAR" {
			t.Errorf("Expected to unset TEST_VAR, got %s", unsetKey)
		}
	})
}
