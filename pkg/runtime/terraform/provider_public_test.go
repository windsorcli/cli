package terraform

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/evaluator"
	"github.com/windsorcli/cli/pkg/runtime/shell"
	"github.com/windsorcli/cli/pkg/runtime/tools"
)

// =============================================================================
// Test Setup
// =============================================================================

type Mocks struct {
	Provider      *terraformProvider
	ConfigHandler *config.MockConfigHandler
	Shell         *shell.MockShell
	ToolsManager  *tools.MockToolsManager
	Evaluator     evaluator.ExpressionEvaluator
}

type SetupOptions struct {
	BlueprintYAML string
	BackendType   string
	Evaluator     evaluator.ExpressionEvaluator
}

func setupMocks(t *testing.T, opts ...*SetupOptions) *Mocks {
	t.Helper()

	options := &SetupOptions{
		BackendType: "none",
	}
	if len(opts) > 0 && opts[0] != nil {
		options = opts[0]
	}

	configHandler := config.NewMockConfigHandler()
	mockShell := shell.NewMockShell()
	toolsManager := tools.NewMockToolsManager()

	mockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
		return "", errors.New("ExecSilent not mocked in test")
	}

	configRoot := "/test/config"
	configHandler.GetConfigRootFunc = func() (string, error) {
		return configRoot, nil
	}

	mockShell.GetProjectRootFunc = func() (string, error) {
		return "/test/project", nil
	}

	configHandler.GetWindsorScratchPathFunc = func() (string, error) {
		return "/test/scratch", nil
	}

	configHandler.GetStringFunc = func(key string, defaultValue ...string) string {
		if key == "terraform.backend.type" {
			return options.BackendType
		}
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return ""
	}

	configHandler.GetContextFunc = func() string {
		return "default"
	}

	toolsManager.GetTerraformCommandFunc = func() string {
		return "terraform"
	}

	provider := NewTerraformProvider(configHandler, mockShell, toolsManager, options.Evaluator)
	concreteProvider := provider.(*terraformProvider)

	if options.BlueprintYAML != "" {
		concreteProvider.Shims.ReadFile = func(path string) ([]byte, error) {
			if path == filepath.Join(configRoot, "blueprint.yaml") {
				return []byte(options.BlueprintYAML), nil
			}
			return nil, os.ErrNotExist
		}
	}

	concreteProvider.Shims.Getenv = func(key string) string {
		return ""
	}

	concreteProvider.Shims.Setenv = func(key, value string) error {
		return nil
	}

	concreteProvider.Shims.Stat = func(path string) (os.FileInfo, error) {
		return nil, os.ErrNotExist
	}

	concreteProvider.Shims.Remove = func(path string) error {
		return nil
	}

	concreteProvider.Shims.WriteFile = func(path string, data []byte, perm os.FileMode) error {
		return nil
	}

	return &Mocks{
		Provider:      concreteProvider,
		ConfigHandler: configHandler,
		Shell:         mockShell,
		ToolsManager:  toolsManager,
		Evaluator:     options.Evaluator,
	}
}

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewTerraformProvider(t *testing.T) {
	t.Run("CreatesProviderWithEmptyCache", func(t *testing.T) {
		mocks := setupMocks(t)

		if mocks.Provider == nil {
			t.Fatal("Expected provider to be created")
		}

		if mocks.Provider.cache == nil {
			t.Error("Expected cache to be initialized")
		}

		if len(mocks.Provider.cache) != 0 {
			t.Errorf("Expected empty cache, got %d entries", len(mocks.Provider.cache))
		}

		if mocks.Provider.Shims == nil {
			t.Error("Expected shims to be initialized")
		}
	})

	t.Run("CallsRegisterWhenEvaluatorProvided", func(t *testing.T) {
		configHandler := config.NewMockConfigHandler()
		mockShell := shell.NewMockShell()
		toolsManager := tools.NewMockToolsManager()
		mockEvaluator := evaluator.NewMockExpressionEvaluator()
		registerCalled := false
		registerName := ""
		mockEvaluator.RegisterFunc = func(name string, helper func(params ...any) (any, error), signature any) {
			registerCalled = true
			registerName = name
		}

		provider := NewTerraformProvider(configHandler, mockShell, toolsManager, mockEvaluator)

		if provider == nil {
			t.Fatal("Expected provider to be created")
		}

		if !registerCalled {
			t.Error("Expected Register to be called on evaluator")
		}

		if registerName != "terraform_output" {
			t.Errorf("Expected Register to be called with name 'terraform_output', got %s", registerName)
		}
	})

	t.Run("RegistersHelperWhenEvaluatorProvided", func(t *testing.T) {
		configHandler := config.NewMockConfigHandler()
		mockShell := shell.NewMockShell()
		toolsManager := tools.NewMockToolsManager()
		testEvaluator := evaluator.NewExpressionEvaluator(configHandler, "/test/project", "/test/template")

		provider := NewTerraformProvider(configHandler, mockShell, toolsManager, testEvaluator)
		concreteProvider := provider.(*terraformProvider)

		if provider == nil {
			t.Fatal("Expected provider to be created")
		}

		concreteProvider.mu.Lock()
		concreteProvider.cache["test-component"] = map[string]any{"test-key": "test-value"}
		concreteProvider.mu.Unlock()

		result, err := testEvaluator.Evaluate(`terraform_output("test-component", "test-key")`, map[string]any{}, "")

		if err != nil {
			t.Fatalf("Expected helper to be registered and callable, got error: %v", err)
		}

		if result != "test-value" {
			t.Errorf("Expected helper to return 'test-value', got %v", result)
		}
	})

	t.Run("DoesNotRegisterHelperWhenEvaluatorIsNil", func(t *testing.T) {
		mocks := setupMocks(t)

		if mocks.Provider == nil {
			t.Fatal("Expected provider to be created")
		}
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestTerraformProvider_ClearCache(t *testing.T) {
	t.Run("ClearsAllCachedOutputsAndComponents", func(t *testing.T) {
		mocks := setupMocks(t)

		mocks.Provider.mu.Lock()
		mocks.Provider.cache["component1"] = map[string]any{"output1": "value1"}
		mocks.Provider.cache["component2"] = map[string]any{"output2": "value2"}
		mocks.Provider.components = []blueprintv1alpha1.TerraformComponent{{Path: "test"}}
		mocks.Provider.mu.Unlock()

		mocks.Provider.ClearCache()

		if len(mocks.Provider.cache) != 0 {
			t.Errorf("Expected cache to be empty after ClearCache, got %d entries", len(mocks.Provider.cache))
		}

		if mocks.Provider.components != nil {
			t.Error("Expected components to be cleared after ClearCache")
		}
	})
}

func TestTerraformProvider_GetTerraformComponents(t *testing.T) {
	t.Run("ReturnsCachedComponents", func(t *testing.T) {
		mocks := setupMocks(t)

		expectedComponents := []blueprintv1alpha1.TerraformComponent{
			{Path: "test/path"},
		}

		mocks.Provider.mu.Lock()
		mocks.Provider.components = expectedComponents
		mocks.Provider.mu.Unlock()

		components := mocks.Provider.GetTerraformComponents()

		if len(components) != len(expectedComponents) {
			t.Errorf("Expected %d components, got %d", len(expectedComponents), len(components))
		}

		if components[0].Path != expectedComponents[0].Path {
			t.Errorf("Expected component path to be 'test/path', got %s", components[0].Path)
		}
	})

	t.Run("LoadsComponentsFromBlueprint", func(t *testing.T) {
		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
metadata:
  name: test
terraform:
  - path: test/path
    name: test-component`

		mocks := setupMocks(t, &SetupOptions{BlueprintYAML: blueprintYAML})

		components := mocks.Provider.GetTerraformComponents()

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
		mocks := setupMocks(t)

		mocks.ConfigHandler.GetConfigRootFunc = func() (string, error) {
			return "", errors.New("config root not found")
		}

		components := mocks.Provider.GetTerraformComponents()

		if len(components) != 0 {
			t.Errorf("Expected empty components on error, got %d", len(components))
		}
	})

	t.Run("HandlesMissingBlueprintFile", func(t *testing.T) {
		mocks := setupMocks(t)

		mocks.Provider.Shims.ReadFile = func(path string) ([]byte, error) {
			return nil, os.ErrNotExist
		}

		components := mocks.Provider.GetTerraformComponents()

		if len(components) != 0 {
			t.Errorf("Expected empty components on file error, got %d", len(components))
		}
	})

	t.Run("HandlesInvalidYAML", func(t *testing.T) {
		mocks := setupMocks(t)

		configRoot := "/test/config"
		mocks.Provider.Shims.ReadFile = func(path string) ([]byte, error) {
			if path == filepath.Join(configRoot, "blueprint.yaml") {
				return []byte("invalid: yaml: [unclosed"), nil
			}
			return nil, os.ErrNotExist
		}

		components := mocks.Provider.GetTerraformComponents()

		if len(components) != 0 {
			t.Errorf("Expected empty components on YAML error, got %d", len(components))
		}
	})

	t.Run("SetsFullPathForSourcedComponents", func(t *testing.T) {
		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
metadata:
  name: test
terraform:
  - path: test/path
    source: config`

		mocks := setupMocks(t, &SetupOptions{BlueprintYAML: blueprintYAML})

		components := mocks.Provider.GetTerraformComponents()

		expectedPath := filepath.Join("/test/project", ".windsor", "contexts", "default", "terraform", "test/path")
		if components[0].FullPath != expectedPath {
			t.Errorf("Expected FullPath to be %s, got %s", expectedPath, components[0].FullPath)
		}
	})

	t.Run("SetsFullPathForLocalComponents", func(t *testing.T) {
		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
metadata:
  name: test
terraform:
  - path: test/path`

		mocks := setupMocks(t, &SetupOptions{BlueprintYAML: blueprintYAML})

		components := mocks.Provider.GetTerraformComponents()

		expectedPath := filepath.Join("/test/project", "terraform", "test/path")
		if components[0].FullPath != expectedPath {
			t.Errorf("Expected FullPath to be %s, got %s", expectedPath, components[0].FullPath)
		}
	})
}

func TestTerraformProvider_GenerateBackendOverride(t *testing.T) {
	t.Run("CreatesLocalBackendOverride", func(t *testing.T) {
		mocks := setupMocks(t, &SetupOptions{BackendType: "local"})

		var writtenPath string
		var writtenData []byte
		mocks.Provider.Shims.WriteFile = func(path string, data []byte, perm os.FileMode) error {
			writtenPath = path
			writtenData = data
			return nil
		}

		err := mocks.Provider.GenerateBackendOverride("/test/dir")

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
		mocks := setupMocks(t, &SetupOptions{BackendType: "s3"})

		mocks.Provider.Shims.WriteFile = func(path string, data []byte, perm os.FileMode) error {
			return nil
		}

		err := mocks.Provider.GenerateBackendOverride("/test/dir")

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
	})

	t.Run("CreatesKubernetesBackendOverride", func(t *testing.T) {
		mocks := setupMocks(t, &SetupOptions{BackendType: "kubernetes"})

		mocks.Provider.Shims.WriteFile = func(path string, data []byte, perm os.FileMode) error {
			return nil
		}

		err := mocks.Provider.GenerateBackendOverride("/test/dir")

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
	})

	t.Run("CreatesAzurermBackendOverride", func(t *testing.T) {
		mocks := setupMocks(t, &SetupOptions{BackendType: "azurerm"})

		mocks.Provider.Shims.WriteFile = func(path string, data []byte, perm os.FileMode) error {
			return nil
		}

		err := mocks.Provider.GenerateBackendOverride("/test/dir")

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
	})

	t.Run("RemovesBackendOverrideForNone", func(t *testing.T) {
		mocks := setupMocks(t, &SetupOptions{BackendType: "none"})

		var removedPath string
		mocks.Provider.Shims.Stat = func(path string) (os.FileInfo, error) {
			return nil, nil
		}
		mocks.Provider.Shims.Remove = func(path string) error {
			removedPath = path
			return nil
		}

		err := mocks.Provider.GenerateBackendOverride("/test/dir")

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		expectedPath := filepath.Join("/test/dir", "backend_override.tf")
		if removedPath != expectedPath {
			t.Errorf("Expected to remove %s, got %s", expectedPath, removedPath)
		}
	})

	t.Run("HandlesUnsupportedBackend", func(t *testing.T) {
		mocks := setupMocks(t)

		mocks.ConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "unsupported"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		err := mocks.Provider.GenerateBackendOverride("/test/dir")

		if err == nil {
			t.Fatal("Expected error for unsupported backend")
		}

		if err.Error() != "unsupported backend: unsupported" {
			t.Errorf("Expected unsupported backend error, got %v", err)
		}
	})

	t.Run("HandlesWriteFileError", func(t *testing.T) {
		mocks := setupMocks(t, &SetupOptions{BackendType: "local"})

		mocks.Provider.Shims.WriteFile = func(path string, data []byte, perm os.FileMode) error {
			return errors.New("write failed")
		}

		err := mocks.Provider.GenerateBackendOverride("/test/dir")

		if err == nil {
			t.Fatal("Expected error on write failure")
		}
	})

	t.Run("HandlesRemoveError", func(t *testing.T) {
		mocks := setupMocks(t, &SetupOptions{BackendType: "none"})

		mocks.Provider.Shims.Stat = func(path string) (os.FileInfo, error) {
			return nil, nil
		}
		mocks.Provider.Shims.Remove = func(path string) error {
			return errors.New("remove failed")
		}

		err := mocks.Provider.GenerateBackendOverride("/test/dir")

		if err == nil {
			t.Fatal("Expected error on remove failure")
		}
	})
}

func TestTerraformProvider_GetOutputs(t *testing.T) {
	t.Run("ReturnsCachedOutputs", func(t *testing.T) {
		mocks := setupMocks(t)

		expectedOutputs := map[string]any{
			"output1": "value1",
			"output2": 42,
		}

		mocks.Provider.mu.Lock()
		mocks.Provider.cache["test-component"] = expectedOutputs
		mocks.Provider.mu.Unlock()

		output1, err := mocks.Provider.getOutput("test-component", "output1")
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if output1 != expectedOutputs["output1"] {
			t.Errorf("Expected output1 to be 'value1', got %v", output1)
		}

		output2, err := mocks.Provider.getOutput("test-component", "output2")
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if output2 != expectedOutputs["output2"] {
			t.Errorf("Expected output2 to be 42, got %v", output2)
		}
	})

	t.Run("CachesOutputsAfterFirstCall", func(t *testing.T) {
		mocks := setupMocks(t)

		expectedOutputs := map[string]any{
			"output1": "value1",
		}

		mocks.Provider.mu.Lock()
		mocks.Provider.cache["test-component"] = expectedOutputs
		mocks.Provider.mu.Unlock()

		output1, err1 := mocks.Provider.getOutput("test-component", "output1")
		if err1 != nil {
			t.Fatalf("Expected no error on first call, got %v", err1)
		}

		output2, err2 := mocks.Provider.getOutput("test-component", "output1")
		if err2 != nil {
			t.Fatalf("Expected no error on second call, got %v", err2)
		}

		if output1 != output2 {
			t.Error("Expected both calls to return same cached value")
		}

		if len(mocks.Provider.cache) != 1 {
			t.Errorf("Expected cache to have 1 entry, got %d", len(mocks.Provider.cache))
		}
	})

	t.Run("ReturnsEmptyMapWhenComponentNotFound", func(t *testing.T) {
		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
metadata:
  name: test
terraform:
  - path: other/path`

		mocks := setupMocks(t, &SetupOptions{BlueprintYAML: blueprintYAML})

		output, err := mocks.Provider.getOutput("nonexistent-component", "any-key")

		if err != nil {
			t.Errorf("Expected no error for nonexistent component, got %v", err)
		}
		if _, isDeferred := output.(evaluator.DeferredValue); !isDeferred {
			t.Errorf("Expected DeferredValue for nonexistent component, got %v", output)
		}
	})

	t.Run("CapturesOutputsFromTerraform", func(t *testing.T) {
		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
metadata:
  name: test
terraform:
  - path: test/path
    name: test-component`

		mocks := setupMocks(t, &SetupOptions{BlueprintYAML: blueprintYAML, BackendType: "local"})

		execCallCount := 0
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			execCallCount++
			if command == "terraform" && len(args) >= 2 {
				if args[1] == "output" {
					return `{"output1": {"value": "val1"}, "output2": {"value": 42}}`, nil
				}
				if args[1] == "init" {
					return "", nil
				}
			}
			return "", nil
		}

		components := mocks.Provider.GetTerraformComponents()
		if len(components) == 0 {
			t.Fatal("Expected components to be loaded from blueprint")
		}

		output1, err := mocks.Provider.getOutput("test-component", "output1")
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		output2, err := mocks.Provider.getOutput("test-component", "output2")
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if execCallCount == 0 {
			t.Errorf("Expected ExecSilent to be called (components found: %d)", len(components))
		}

		if output1 != "val1" {
			t.Errorf("Expected output1 to be 'val1', got %v", output1)
		}

		if output2 != float64(42) {
			t.Errorf("Expected output2 to be 42, got %v", output2)
		}
	})

	t.Run("HandlesEmptyTerraformOutput", func(t *testing.T) {
		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
metadata:
  name: test
terraform:
  - path: test/path
    name: test-component`

		mocks := setupMocks(t, &SetupOptions{BlueprintYAML: blueprintYAML, BackendType: "local"})

		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if len(args) > 1 && args[1] == "output" {
				return "{}", nil
			}
			return "", nil
		}

		output, err := mocks.Provider.getOutput("test-component", "any-key")

		if err != nil {
			t.Errorf("Expected no error for empty outputs, got %v", err)
		}
		if _, isDeferred := output.(evaluator.DeferredValue); !isDeferred {
			t.Errorf("Expected DeferredValue for missing key, got %v", output)
		}
	})

	t.Run("HandlesTerraformInitFallback", func(t *testing.T) {
		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
metadata:
  name: test
terraform:
  - path: test/path
    name: test-component`

		mocks := setupMocks(t, &SetupOptions{BlueprintYAML: blueprintYAML, BackendType: "local"})

		outputCallCount := 0
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
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

		output, err := mocks.Provider.getOutput("test-component", "output1")
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if output != "val1" {
			t.Errorf("Expected output to be 'val1', got %v (outputCallCount: %d)", output, outputCallCount)
		}
	})

	t.Run("HandlesSetenvError", func(t *testing.T) {
		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
metadata:
  name: test
terraform:
  - path: test/path
    name: test-component`

		mocks := setupMocks(t, &SetupOptions{BlueprintYAML: blueprintYAML, BackendType: "local"})

		mocks.Provider.Shims.Setenv = func(key, value string) error {
			return errors.New("setenv failed")
		}

		output, err := mocks.Provider.getOutput("test-component", "any-key")

		if err != nil {
			t.Errorf("Expected no error when setenv fails (errors are swallowed), got %v", err)
		}
		if _, isDeferred := output.(evaluator.DeferredValue); !isDeferred {
			t.Errorf("Expected DeferredValue when setenv fails, got %v", output)
		}
	})

	t.Run("HandlesBackendOverrideError", func(t *testing.T) {
		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
metadata:
  name: test
terraform:
  - path: test/path
    name: test-component`

		mocks := setupMocks(t, &SetupOptions{BlueprintYAML: blueprintYAML, BackendType: "unsupported"})

		output, err := mocks.Provider.getOutput("test-component", "any-key")

		if err != nil {
			t.Errorf("Expected no error when backend override fails (errors are swallowed), got %v", err)
		}
		if _, isDeferred := output.(evaluator.DeferredValue); !isDeferred {
			t.Errorf("Expected DeferredValue when backend override fails, got %v", output)
		}
	})

	t.Run("HandlesCacheRaceCondition", func(t *testing.T) {
		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
metadata:
  name: test
terraform:
  - path: test/path
    name: test-component`

		mocks := setupMocks(t, &SetupOptions{BlueprintYAML: blueprintYAML, BackendType: "local"})

		callCount := 0
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			callCount++
			if callCount == 1 {
				mocks.Provider.mu.Lock()
				mocks.Provider.cache["test-component"] = map[string]any{"output1": "cached-value"}
				mocks.Provider.mu.Unlock()
			}
			if command == "terraform" && len(args) >= 2 && args[1] == "output" {
				return `{"output1": {"value": "val1"}}`, nil
			}
			if command == "terraform" && len(args) >= 2 && args[1] == "init" {
				return "", nil
			}
			return "", nil
		}

		output, err := mocks.Provider.getOutput("test-component", "output1")
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if output != "cached-value" {
			t.Errorf("Expected cached value, got %v", output)
		}
	})

	t.Run("HandlesJsonUnmarshalError", func(t *testing.T) {
		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
metadata:
  name: test
terraform:
  - path: test/path
    name: test-component`

		mocks := setupMocks(t, &SetupOptions{BlueprintYAML: blueprintYAML})

		mocks.Provider.Shims.JsonUnmarshal = func(data []byte, v any) error {
			return errors.New("json unmarshal error")
		}

		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "terraform" && len(args) >= 2 && args[1] == "output" {
				return `{"output1": {"value": "val1"}}`, nil
			}
			return "", nil
		}

		output, err := mocks.Provider.getOutput("test-component", "output1")
		if err != nil {
			t.Errorf("Expected no error when JsonUnmarshal fails (errors are swallowed), got %v", err)
		}
		if _, isDeferred := output.(evaluator.DeferredValue); !isDeferred {
			t.Errorf("Expected DeferredValue when JsonUnmarshal fails, got %v", output)
		}
	})

	t.Run("HandlesOutputsWithoutValueField", func(t *testing.T) {
		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
metadata:
  name: test
terraform:
  - path: test/path
    name: test-component`

		mocks := setupMocks(t, &SetupOptions{BlueprintYAML: blueprintYAML})

		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "terraform" && len(args) >= 2 && args[1] == "output" {
				return `{"output1": {"sensitive": true}}`, nil
			}
			return "", nil
		}

		output, err := mocks.Provider.getOutput("test-component", "output1")
		if err != nil {
			t.Errorf("Expected no error when output has no value field, got %v", err)
		}
		if _, isDeferred := output.(evaluator.DeferredValue); !isDeferred {
			t.Errorf("Expected DeferredValue when output has no value field, got %v", output)
		}
	})

	t.Run("HandlesOutputsWithNonMapValue", func(t *testing.T) {
		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
metadata:
  name: test
terraform:
  - path: test/path
    name: test-component`

		mocks := setupMocks(t, &SetupOptions{BlueprintYAML: blueprintYAML})

		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "terraform" && len(args) >= 2 && args[1] == "output" {
				return `{"output1": "not-a-map"}`, nil
			}
			return "", nil
		}

		output, err := mocks.Provider.getOutput("test-component", "output1")
		if err != nil {
			t.Errorf("Expected no error when output value is not a map, got %v", err)
		}
		if _, isDeferred := output.(evaluator.DeferredValue); !isDeferred {
			t.Errorf("Expected DeferredValue when output value is not a map, got %v", output)
		}
	})
}

func TestTerraformProvider_GetTerraformComponent(t *testing.T) {
	t.Run("FindsComponentByPath", func(t *testing.T) {
		mocks := setupMocks(t)

		// Use mocks.ConfigHandler directly
		// Use mocks.Shell directly

		configRoot := "/test/config"
		mocks.ConfigHandler.GetConfigRootFunc = func() (string, error) {
			return configRoot, nil
		}

		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
metadata:
  name: test
terraform:
  - path: test/path`

		mocks.Provider.Shims.ReadFile = func(path string) ([]byte, error) {
			if path == filepath.Join(configRoot, "blueprint.yaml") {
				return []byte(blueprintYAML), nil
			}
			return nil, os.ErrNotExist
		}

		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "/test/project", nil
		}

		mocks.ConfigHandler.GetContextFunc = func() string {
			return "default"
		}

		component := mocks.Provider.GetTerraformComponent("test/path")

		if component == nil {
			t.Fatal("Expected component to be found")
		}

		if component.Path != "test/path" {
			t.Errorf("Expected component path to be 'test/path', got %s", component.Path)
		}
	})

	t.Run("FindsComponentByName", func(t *testing.T) {
		mocks := setupMocks(t)

		// Use mocks.ConfigHandler directly
		// Use mocks.Shell directly

		configRoot := "/test/config"
		mocks.ConfigHandler.GetConfigRootFunc = func() (string, error) {
			return configRoot, nil
		}

		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
metadata:
  name: test
terraform:
  - path: test/path
    name: test-component`

		mocks.Provider.Shims.ReadFile = func(path string) ([]byte, error) {
			if path == filepath.Join(configRoot, "blueprint.yaml") {
				return []byte(blueprintYAML), nil
			}
			return nil, os.ErrNotExist
		}

		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "/test/project", nil
		}

		mocks.ConfigHandler.GetContextFunc = func() string {
			return "default"
		}

		component := mocks.Provider.GetTerraformComponent("test-component")

		if component == nil {
			t.Fatal("Expected component to be found")
		}

		if component.Name != "test-component" {
			t.Errorf("Expected component name to be 'test-component', got %s", component.Name)
		}
	})

	t.Run("ReturnsNilWhenComponentNotFound", func(t *testing.T) {
		mocks := setupMocks(t)

		// Use mocks.ConfigHandler directly
		// Use mocks.Shell directly

		configRoot := "/test/config"
		mocks.ConfigHandler.GetConfigRootFunc = func() (string, error) {
			return configRoot, nil
		}

		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
metadata:
  name: test
terraform:
  - path: test/path`

		mocks.Provider.Shims.ReadFile = func(path string) ([]byte, error) {
			if path == filepath.Join(configRoot, "blueprint.yaml") {
				return []byte(blueprintYAML), nil
			}
			return nil, os.ErrNotExist
		}

		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "/test/project", nil
		}

		mocks.ConfigHandler.GetContextFunc = func() string {
			return "default"
		}

		component := mocks.Provider.GetTerraformComponent("nonexistent")

		if component != nil {
			t.Error("Expected component to be nil when not found")
		}
	})
}

func TestTerraformProvider_ResolveModulePath(t *testing.T) {
	t.Run("ResolvesPathForComponentWithName", func(t *testing.T) {
		mocks := setupMocks(t)

		// Use mocks.ConfigHandler directly

		mocks.ConfigHandler.GetWindsorScratchPathFunc = func() (string, error) {
			return "/test/scratch", nil
		}

		component := &blueprintv1alpha1.TerraformComponent{
			Name: "test-component",
		}

		path, err := mocks.Provider.resolveModulePath(component)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		expected := filepath.Join("/test/scratch", "terraform", "test-component")
		if path != expected {
			t.Errorf("Expected path %s, got %s", expected, path)
		}
	})

	t.Run("ResolvesPathForComponentWithSource", func(t *testing.T) {
		mocks := setupMocks(t)

		// Use mocks.ConfigHandler directly

		mocks.ConfigHandler.GetWindsorScratchPathFunc = func() (string, error) {
			return "/test/scratch", nil
		}

		component := &blueprintv1alpha1.TerraformComponent{
			Path:   "test/path",
			Source: "config",
		}

		path, err := mocks.Provider.resolveModulePath(component)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		expected := filepath.Join("/test/scratch", "terraform", "test/path")
		if path != expected {
			t.Errorf("Expected path %s, got %s", expected, path)
		}
	})

	t.Run("ResolvesPathForLocalComponent", func(t *testing.T) {
		mocks := setupMocks(t)

		// Use mocks.Shell directly

		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "/test/project", nil
		}

		component := &blueprintv1alpha1.TerraformComponent{
			Path: "test/path",
		}

		path, err := mocks.Provider.resolveModulePath(component)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		expected := filepath.Join("/test/project", "terraform", "test/path")
		if path != expected {
			t.Errorf("Expected path %s, got %s", expected, path)
		}
	})

	t.Run("ReturnsErrorWhenScratchPathFails", func(t *testing.T) {
		mocks := setupMocks(t)

		// Use mocks.ConfigHandler directly

		mocks.ConfigHandler.GetWindsorScratchPathFunc = func() (string, error) {
			return "", errors.New("scratch path error")
		}

		component := &blueprintv1alpha1.TerraformComponent{
			Name: "test-component",
		}

		_, err := mocks.Provider.resolveModulePath(component)

		if err == nil {
			t.Fatal("Expected error when scratch path fails")
		}
	})

	t.Run("ReturnsErrorWhenProjectRootFails", func(t *testing.T) {
		mocks := setupMocks(t)

		// Use mocks.Shell directly

		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "", errors.New("project root error")
		}

		component := &blueprintv1alpha1.TerraformComponent{
			Path: "test/path",
		}

		_, err := mocks.Provider.resolveModulePath(component)

		if err == nil {
			t.Fatal("Expected error when project root fails")
		}
	})
}

func TestTerraformProvider_FindRelativeProjectPath(t *testing.T) {
	t.Run("FindsProjectPathInTerraformDirectory", func(t *testing.T) {
		mocks := setupMocks(t)

		testPath := filepath.Join("/test", "project", "terraform", "component", "subdir")
		mocks.Provider.Shims.Getwd = func() (string, error) {
			return testPath, nil
		}

		mocks.Provider.Shims.Glob = func(pattern string) ([]string, error) {
			if pattern == filepath.Join(testPath, "*.tf") {
				return []string{filepath.Join(testPath, "main.tf")}, nil
			}
			return nil, nil
		}

		path, err := mocks.Provider.FindRelativeProjectPath()

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		expected := "component/subdir"
		if path != expected {
			t.Errorf("Expected path %s, got %s", expected, path)
		}
	})

	t.Run("FindsProjectPathInContextsDirectory", func(t *testing.T) {
		mocks := setupMocks(t)

		testPath := filepath.Join("/test", "project", ".windsor", "contexts", "local", "terraform", "component")
		mocks.Provider.Shims.Getwd = func() (string, error) {
			return testPath, nil
		}

		mocks.Provider.Shims.Glob = func(pattern string) ([]string, error) {
			if pattern == filepath.Join(testPath, "*.tf") {
				return []string{filepath.Join(testPath, "main.tf")}, nil
			}
			return nil, nil
		}

		path, err := mocks.Provider.FindRelativeProjectPath()

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		expected := "component"
		if path != expected {
			t.Errorf("Expected path %s, got %s", expected, path)
		}
	})

	t.Run("ReturnsEmptyWhenNoTerraformFiles", func(t *testing.T) {
		mocks := setupMocks(t)

		testPath := "/test/path"
		mocks.Provider.Shims.Getwd = func() (string, error) {
			return testPath, nil
		}

		mocks.Provider.Shims.Glob = func(pattern string) ([]string, error) {
			return nil, nil
		}

		path, err := mocks.Provider.FindRelativeProjectPath()

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if path != "" {
			t.Errorf("Expected empty path, got %s", path)
		}
	})

	t.Run("ReturnsErrorWhenGetwdFails", func(t *testing.T) {
		mocks := setupMocks(t)

		mocks.Provider.Shims.Getwd = func() (string, error) {
			return "", errors.New("getwd failed")
		}

		_, err := mocks.Provider.FindRelativeProjectPath()

		if err == nil {
			t.Fatal("Expected error when Getwd fails")
		}
	})

	t.Run("ReturnsErrorWhenGlobFails", func(t *testing.T) {
		mocks := setupMocks(t)

		testPath := "/test/path"
		mocks.Provider.Shims.Getwd = func() (string, error) {
			return testPath, nil
		}

		mocks.Provider.Shims.Glob = func(pattern string) ([]string, error) {
			return nil, errors.New("glob failed")
		}

		_, err := mocks.Provider.FindRelativeProjectPath()

		if err == nil {
			t.Fatal("Expected error when Glob fails")
		}
	})

	t.Run("AcceptsDirectoryParameter", func(t *testing.T) {
		mocks := setupMocks(t)

		testPath := filepath.Join("/test", "project", "terraform", "component")
		mocks.Provider.Shims.Glob = func(pattern string) ([]string, error) {
			if pattern == filepath.Join(testPath, "*.tf") {
				return []string{filepath.Join(testPath, "main.tf")}, nil
			}
			return nil, nil
		}

		path, err := mocks.Provider.FindRelativeProjectPath(testPath)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		expected := "component"
		if path != expected {
			t.Errorf("Expected path %s, got %s", expected, path)
		}
	})

	t.Run("ReturnsEmptyWhenNoTerraformOrContextsDirectory", func(t *testing.T) {
		mocks := setupMocks(t)

		testPath := filepath.Join("/test", "random", "path", "with", "tf", "files")
		mocks.Provider.Shims.Getwd = func() (string, error) {
			return testPath, nil
		}

		mocks.Provider.Shims.Glob = func(pattern string) ([]string, error) {
			if pattern == filepath.Join(testPath, "*.tf") {
				return []string{filepath.Join(testPath, "main.tf")}, nil
			}
			return nil, nil
		}

		path, err := mocks.Provider.FindRelativeProjectPath()

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if path != "" {
			t.Errorf("Expected empty path when no terraform/contexts directory found, got %s", path)
		}
	})
}

func TestTerraformProvider_GenerateTerraformArgs(t *testing.T) {
	t.Run("GeneratesArgsSuccessfully", func(t *testing.T) {
		mocks := setupMocks(t)

		// Use mocks.ConfigHandler directly
		// Use mocks.Shell directly

		configRoot := "/test/config"
		mocks.ConfigHandler.GetConfigRootFunc = func() (string, error) {
			return configRoot, nil
		}

		windsorScratchPath := "/test/scratch"
		mocks.ConfigHandler.GetWindsorScratchPathFunc = func() (string, error) {
			return windsorScratchPath, nil
		}

		mocks.ConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "local"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		mocks.ConfigHandler.GetContextFunc = func() string {
			return "default"
		}

		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "/test/project", nil
		}

		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
metadata:
  name: test
terraform:
  - path: test/path`

		mocks.Provider.Shims.ReadFile = func(path string) ([]byte, error) {
			if path == filepath.Join(configRoot, "blueprint.yaml") {
				return []byte(blueprintYAML), nil
			}
			return nil, os.ErrNotExist
		}

		mocks.Provider.Shims.Stat = func(path string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		modulePath := filepath.Join("/test/project", "terraform", "test/path")
		args, err := mocks.Provider.GenerateTerraformArgs("test/path", modulePath, true)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		expectedTFDataDir := filepath.ToSlash(filepath.Join(windsorScratchPath, ".terraform", "test/path"))
		if args.TFDataDir != expectedTFDataDir {
			t.Errorf("Expected TFDataDir %s, got %s", expectedTFDataDir, args.TFDataDir)
		}

		if args.ModulePath != modulePath {
			t.Errorf("Expected ModulePath %s, got %s", modulePath, args.ModulePath)
		}

		if len(args.InitArgs) == 0 {
			t.Error("Expected InitArgs to be populated")
		}

		expectedPlanPath := filepath.ToSlash(filepath.Join(expectedTFDataDir, "terraform.tfplan"))
		if len(args.PlanArgs) == 0 || !strings.Contains(args.PlanArgs[0], expectedPlanPath) {
			t.Errorf("Expected PlanArgs to contain plan path, got %v", args.PlanArgs)
		}

		if len(args.ApplyArgs) == 0 || args.ApplyArgs[len(args.ApplyArgs)-1] != expectedPlanPath {
			t.Errorf("Expected ApplyArgs to end with plan path, got %v", args.ApplyArgs)
		}
	})

	t.Run("IncludesVarFileArgs", func(t *testing.T) {
		mocks := setupMocks(t)

		// Use mocks.ConfigHandler directly
		// Use mocks.Shell directly

		configRoot := "/test/config"
		mocks.ConfigHandler.GetConfigRootFunc = func() (string, error) {
			return configRoot, nil
		}

		windsorScratchPath := "/test/scratch"
		mocks.ConfigHandler.GetWindsorScratchPathFunc = func() (string, error) {
			return windsorScratchPath, nil
		}

		mocks.ConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "local"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		mocks.ConfigHandler.GetContextFunc = func() string {
			return "default"
		}

		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "/test/project", nil
		}

		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
metadata:
  name: test
terraform:
  - path: test/path`

		mocks.Provider.Shims.ReadFile = func(path string) ([]byte, error) {
			if path == filepath.Join(configRoot, "blueprint.yaml") {
				return []byte(blueprintYAML), nil
			}
			return nil, os.ErrNotExist
		}

		tfvarsPath := filepath.Join(configRoot, "terraform", "test/path.tfvars")
		mocks.Provider.Shims.Stat = func(path string) (os.FileInfo, error) {
			if filepath.ToSlash(path) == filepath.ToSlash(tfvarsPath) {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		modulePath := filepath.Join("/test/project", "terraform", "test/path")
		args, err := mocks.Provider.GenerateTerraformArgs("test/path", modulePath, true)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		foundVarFile := false
		for _, arg := range args.PlanArgs {
			if strings.Contains(arg, "test/path.tfvars") {
				foundVarFile = true
				break
			}
		}
		if !foundVarFile {
			t.Errorf("Expected PlanArgs to contain var-file for test/path.tfvars, got %v", args.PlanArgs)
		}
	})

	t.Run("IncludesParallelismWhenComponentHasIt", func(t *testing.T) {
		mocks := setupMocks(t)

		// Use mocks.ConfigHandler directly
		// Use mocks.Shell directly

		configRoot := "/test/config"
		mocks.ConfigHandler.GetConfigRootFunc = func() (string, error) {
			return configRoot, nil
		}

		windsorScratchPath := "/test/scratch"
		mocks.ConfigHandler.GetWindsorScratchPathFunc = func() (string, error) {
			return windsorScratchPath, nil
		}

		mocks.ConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "local"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		mocks.ConfigHandler.GetContextFunc = func() string {
			return "default"
		}

		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "/test/project", nil
		}

		parallelism := 5
		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
metadata:
  name: test
terraform:
  - path: test/path
    parallelism: 5`

		mocks.Provider.Shims.ReadFile = func(path string) ([]byte, error) {
			if path == filepath.Join(configRoot, "blueprint.yaml") {
				return []byte(blueprintYAML), nil
			}
			return nil, os.ErrNotExist
		}

		mocks.Provider.Shims.Stat = func(path string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		modulePath := filepath.Join("/test/project", "terraform", "test/path")
		args, err := mocks.Provider.GenerateTerraformArgs("test/path", modulePath, true)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		foundParallelismInApply := false
		for _, arg := range args.ApplyArgs {
			if arg == fmt.Sprintf("-parallelism=%d", parallelism) {
				foundParallelismInApply = true
				break
			}
		}
		if !foundParallelismInApply {
			t.Errorf("Expected ApplyArgs to contain -parallelism=%d, got %v", parallelism, args.ApplyArgs)
		}

		foundParallelismInDestroy := false
		for _, arg := range args.DestroyArgs {
			if arg == fmt.Sprintf("-parallelism=%d", parallelism) {
				foundParallelismInDestroy = true
				break
			}
		}
		if !foundParallelismInDestroy {
			t.Errorf("Expected DestroyArgs to contain -parallelism=%d, got %v", parallelism, args.DestroyArgs)
		}
	})

	t.Run("IncludesAutoApproveForNonInteractive", func(t *testing.T) {
		mocks := setupMocks(t)

		// Use mocks.ConfigHandler directly
		// Use mocks.Shell directly

		configRoot := "/test/config"
		mocks.ConfigHandler.GetConfigRootFunc = func() (string, error) {
			return configRoot, nil
		}

		windsorScratchPath := "/test/scratch"
		mocks.ConfigHandler.GetWindsorScratchPathFunc = func() (string, error) {
			return windsorScratchPath, nil
		}

		mocks.ConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "local"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		mocks.ConfigHandler.GetContextFunc = func() string {
			return "default"
		}

		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "/test/project", nil
		}

		mocks.Provider.Shims.Stat = func(path string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		args, err := mocks.Provider.GenerateTerraformArgs("test/path", "test/module", false)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if len(args.DestroyArgs) == 0 || args.DestroyArgs[0] != "-auto-approve" {
			t.Errorf("Expected DestroyArgs to start with -auto-approve for non-interactive, got %v", args.DestroyArgs)
		}
	})

	t.Run("ReturnsErrorWhenConfigRootFails", func(t *testing.T) {
		mocks := setupMocks(t)

		// Use mocks.ConfigHandler directly

		mocks.ConfigHandler.GetConfigRootFunc = func() (string, error) {
			return "", fmt.Errorf("config root error")
		}

		_, err := mocks.Provider.GenerateTerraformArgs("test/path", "test/module", true)

		if err == nil {
			t.Error("Expected error when GetConfigRoot fails")
		}
		if !strings.Contains(err.Error(), "config root") {
			t.Errorf("Expected error about config root, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenWindsorScratchPathFails", func(t *testing.T) {
		mocks := setupMocks(t)

		// Use mocks.ConfigHandler directly

		configRoot := "/test/config"
		mocks.ConfigHandler.GetConfigRootFunc = func() (string, error) {
			return configRoot, nil
		}

		mocks.ConfigHandler.GetWindsorScratchPathFunc = func() (string, error) {
			return "", fmt.Errorf("windsor scratch path error")
		}

		_, err := mocks.Provider.GenerateTerraformArgs("test/path", "test/module", true)

		if err == nil {
			t.Error("Expected error when GetWindsorScratchPath fails")
		}
		if !strings.Contains(err.Error(), "windsor scratch path") {
			t.Errorf("Expected error about windsor scratch path, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenStatFails", func(t *testing.T) {
		mocks := setupMocks(t)

		// Use mocks.ConfigHandler directly
		// Use mocks.Shell directly

		configRoot := "/test/config"
		mocks.ConfigHandler.GetConfigRootFunc = func() (string, error) {
			return configRoot, nil
		}

		windsorScratchPath := "/test/scratch"
		mocks.ConfigHandler.GetWindsorScratchPathFunc = func() (string, error) {
			return windsorScratchPath, nil
		}

		mocks.ConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "local"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		mocks.ConfigHandler.GetContextFunc = func() string {
			return "default"
		}

		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "/test/project", nil
		}

		mocks.Provider.Shims.Stat = func(path string) (os.FileInfo, error) {
			return nil, fmt.Errorf("stat error")
		}

		_, err := mocks.Provider.GenerateTerraformArgs("test/path", "test/module", true)

		if err == nil {
			t.Error("Expected error when Stat fails")
		}
		if !strings.Contains(err.Error(), "error checking file") {
			t.Errorf("Expected error about checking file, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenGetTFDataDirFails", func(t *testing.T) {
		mocks := setupMocks(t)

		// Use mocks.ConfigHandler directly
		// Use mocks.Shell directly

		configRoot := "/test/config"
		mocks.ConfigHandler.GetConfigRootFunc = func() (string, error) {
			return configRoot, nil
		}

		mocks.ConfigHandler.GetWindsorScratchPathFunc = func() (string, error) {
			return "", fmt.Errorf("scratch path error")
		}

		mocks.ConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "local"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		mocks.ConfigHandler.GetContextFunc = func() string {
			return "default"
		}

		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "/test/project", nil
		}

		mocks.Provider.Shims.Stat = func(path string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		_, err := mocks.Provider.GenerateTerraformArgs("test/path", "test/module", true)

		if err == nil {
			t.Error("Expected error when GetTFDataDir fails")
		}
		if !strings.Contains(err.Error(), "TF_DATA_DIR") && !strings.Contains(err.Error(), "windsor scratch path") {
			t.Errorf("Expected error about TF_DATA_DIR or windsor scratch path, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenGenerateBackendConfigArgsFails", func(t *testing.T) {
		mocks := setupMocks(t)

		// Use mocks.ConfigHandler directly
		// Use mocks.Shell directly

		configRoot := "/test/config"
		mocks.ConfigHandler.GetConfigRootFunc = func() (string, error) {
			return configRoot, nil
		}

		windsorScratchPath := "/test/scratch"
		mocks.ConfigHandler.GetWindsorScratchPathFunc = func() (string, error) {
			return windsorScratchPath, nil
		}

		mocks.ConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "terraform.backend.type" {
				return "invalid"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		mocks.ConfigHandler.GetContextFunc = func() string {
			return "default"
		}

		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "/test/project", nil
		}

		mocks.Provider.Shims.Stat = func(path string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		_, err := mocks.Provider.GenerateTerraformArgs("test/path", "test/module", true)

		if err == nil {
			t.Error("Expected error when GenerateBackendConfigArgs fails")
		}
		if !strings.Contains(err.Error(), "backend") {
			t.Errorf("Expected error about backend, got: %v", err)
		}
	})
}

func TestTerraformProvider_FormatArgsForEnv(t *testing.T) {
	t.Run("FormatsVarFileArgs", func(t *testing.T) {
		mocks := setupMocks(t)

		args := []string{"-var-file=/path/to/file.tfvars"}
		result := mocks.Provider.FormatArgsForEnv(args)

		expected := `-var-file="/path/to/file.tfvars"`
		if result != expected {
			t.Errorf("Expected %s, got %s", expected, result)
		}
	})

	t.Run("FormatsOutArgs", func(t *testing.T) {
		mocks := setupMocks(t)

		args := []string{"-out=plan.tfplan"}
		result := mocks.Provider.FormatArgsForEnv(args)

		expected := `-out="plan.tfplan"`
		if result != expected {
			t.Errorf("Expected %s, got %s", expected, result)
		}
	})

	t.Run("FormatsBackendConfigArgs", func(t *testing.T) {
		mocks := setupMocks(t)

		args := []string{"-backend-config=key=value"}
		result := mocks.Provider.FormatArgsForEnv(args)

		expected := `-backend-config="key=value"`
		if result != expected {
			t.Errorf("Expected %s, got %s", expected, result)
		}
	})

	t.Run("PreservesAlreadyQuotedBackendConfig", func(t *testing.T) {
		mocks := setupMocks(t)

		args := []string{`-backend-config="key=value"`}
		result := mocks.Provider.FormatArgsForEnv(args)

		expected := `-backend-config="key=value"`
		if result != expected {
			t.Errorf("Expected %s, got %s", expected, result)
		}
	})

	t.Run("FormatsAbsolutePaths", func(t *testing.T) {
		mocks := setupMocks(t)

		args := []string{"/absolute/path"}
		result := mocks.Provider.FormatArgsForEnv(args)

		expected := `"/absolute/path"`
		if result != expected {
			t.Errorf("Expected %s, got %s", expected, result)
		}
	})

	t.Run("FormatsRelativePaths", func(t *testing.T) {
		mocks := setupMocks(t)

		args := []string{"./relative/path"}
		result := mocks.Provider.FormatArgsForEnv(args)

		expected := `"./relative/path"`
		if result != expected {
			t.Errorf("Expected %s, got %s", expected, result)
		}
	})

	t.Run("FormatsWindowsDrivePaths", func(t *testing.T) {
		mocks := setupMocks(t)

		args := []string{"C:/windows/path", "D:\\windows\\path"}
		result := mocks.Provider.FormatArgsForEnv(args)

		if !strings.Contains(result, `"C:/windows/path"`) {
			t.Errorf("Expected result to contain quoted C:/windows/path, got %s", result)
		}
		if !strings.Contains(result, `"D:\windows\path"`) {
			t.Errorf("Expected result to contain quoted D:\\windows\\path, got %s", result)
		}
	})

	t.Run("PreservesFlags", func(t *testing.T) {
		mocks := setupMocks(t)

		args := []string{"-flag", "--long-flag", "-backend=true"}
		result := mocks.Provider.FormatArgsForEnv(args)

		expected := `-flag --long-flag -backend=true`
		if result != expected {
			t.Errorf("Expected %s, got %s", expected, result)
		}
	})

	t.Run("HandlesMultipleArgs", func(t *testing.T) {
		mocks := setupMocks(t)

		args := []string{"-var-file=file.tfvars", "-out=plan.tfplan", "/path/to/file"}
		result := mocks.Provider.FormatArgsForEnv(args)

		if !strings.Contains(result, `-var-file="file.tfvars"`) {
			t.Errorf("Expected result to contain formatted var-file, got %s", result)
		}
		if !strings.Contains(result, `-out="plan.tfplan"`) {
			t.Errorf("Expected result to contain formatted out, got %s", result)
		}
		if !strings.Contains(result, `"/path/to/file"`) {
			t.Errorf("Expected result to contain quoted path, got %s", result)
		}
	})

	t.Run("HandlesEmptyArgs", func(t *testing.T) {
		mocks := setupMocks(t)

		args := []string{}
		result := mocks.Provider.FormatArgsForEnv(args)

		if result != "" {
			t.Errorf("Expected empty string, got %s", result)
		}
	})
}

func TestTerraformProvider_GetTFDataDir(t *testing.T) {
	t.Run("ReturnsTFDataDirForComponentID", func(t *testing.T) {
		mocks := setupMocks(t)

		windsorScratchPath := "/test/scratch"
		mocks.ConfigHandler.GetWindsorScratchPathFunc = func() (string, error) {
			return windsorScratchPath, nil
		}

		tfDataDir, err := mocks.Provider.GetTFDataDir("test-component")
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		expected := filepath.ToSlash(filepath.Join(windsorScratchPath, ".terraform", "test-component"))
		if tfDataDir != expected {
			t.Errorf("Expected TFDataDir %s, got %s", expected, tfDataDir)
		}
	})

	t.Run("UsesComponentGetIDWhenComponentExists", func(t *testing.T) {
		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
metadata:
  name: test
terraform:
  - path: cluster/path
    name: cluster`

		mocks := setupMocks(t, &SetupOptions{BlueprintYAML: blueprintYAML})

		windsorScratchPath := "/test/scratch"
		mocks.ConfigHandler.GetWindsorScratchPathFunc = func() (string, error) {
			return windsorScratchPath, nil
		}

		tfDataDir, err := mocks.Provider.GetTFDataDir("cluster")
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		expected := filepath.ToSlash(filepath.Join(windsorScratchPath, ".terraform", "cluster"))
		if tfDataDir != expected {
			t.Errorf("Expected TFDataDir %s, got %s", expected, tfDataDir)
		}
	})

	t.Run("ReturnsErrorWhenScratchPathFails", func(t *testing.T) {
		mocks := setupMocks(t)

		mocks.ConfigHandler.GetWindsorScratchPathFunc = func() (string, error) {
			return "", fmt.Errorf("scratch path error")
		}

		_, err := mocks.Provider.GetTFDataDir("test-component")
		if err == nil {
			t.Error("Expected error when GetWindsorScratchPath fails")
		}
		if !strings.Contains(err.Error(), "windsor scratch path") {
			t.Errorf("Expected error about windsor scratch path, got: %v", err)
		}
	})
}

func TestTerraformProvider_GetTerraformOutputs(t *testing.T) {
	t.Run("ReturnsOutputsForComponent", func(t *testing.T) {
		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
metadata:
  name: test
terraform:
  - path: cluster
    name: cluster`

		mocks := setupMocks(t, &SetupOptions{BlueprintYAML: blueprintYAML, BackendType: "local"})

		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "terraform" && len(args) >= 2 {
				if args[1] == "output" {
					return `{"cluster_name": {"value": "my-cluster"}, "region": {"value": "us-west-2"}}`, nil
				}
			}
			return "", nil
		}

		outputs, err := mocks.Provider.GetTerraformOutputs("cluster")
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if outputs["cluster_name"] != "my-cluster" {
			t.Errorf("Expected cluster_name to be my-cluster, got: %v", outputs["cluster_name"])
		}

		if outputs["region"] != "us-west-2" {
			t.Errorf("Expected region to be us-west-2, got: %v", outputs["region"])
		}
	})

	t.Run("ReturnsEmptyMapWhenComponentNotFound", func(t *testing.T) {
		mocks := setupMocks(t)

		outputs, err := mocks.Provider.GetTerraformOutputs("nonexistent")
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if len(outputs) != 0 {
			t.Errorf("Expected empty map, got: %v", outputs)
		}
	})

	t.Run("HandlesEmptyOutput", func(t *testing.T) {
		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
metadata:
  name: test
terraform:
  - path: cluster
    name: cluster`

		mocks := setupMocks(t, &SetupOptions{BlueprintYAML: blueprintYAML, BackendType: "local"})

		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "terraform" && len(args) >= 2 {
				if args[1] == "output" {
					return "{}", nil
				}
			}
			return "", nil
		}

		outputs, err := mocks.Provider.GetTerraformOutputs("cluster")
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if len(outputs) != 0 {
			t.Errorf("Expected empty map, got: %v", outputs)
		}
	})

	t.Run("InitializesModuleWhenOutputFails", func(t *testing.T) {
		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
metadata:
  name: test
terraform:
  - path: cluster
    name: cluster`

		mocks := setupMocks(t, &SetupOptions{BlueprintYAML: blueprintYAML, BackendType: "local"})

		callCount := 0
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "terraform" && len(args) >= 2 {
				if args[1] == "output" {
					callCount++
					if callCount == 1 {
						return "", fmt.Errorf("not initialized")
					}
					return `{"cluster_name": {"value": "my-cluster"}}`, nil
				}
				if args[1] == "init" {
					return "", nil
				}
			}
			return "", nil
		}

		outputs, err := mocks.Provider.GetTerraformOutputs("cluster")
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if outputs["cluster_name"] != "my-cluster" {
			t.Errorf("Expected cluster_name to be my-cluster, got: %v", outputs["cluster_name"])
		}
	})

	t.Run("IncludesBackendConfigArgsInFallbackInit", func(t *testing.T) {
		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
metadata:
  name: test
terraform:
  - path: cluster
    name: cluster`

		mocks := setupMocks(t, &SetupOptions{BlueprintYAML: blueprintYAML, BackendType: "local"})

		callCount := 0
		var initArgs []string
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "terraform" && len(args) >= 2 {
				if args[1] == "output" {
					callCount++
					if callCount == 1 {
						return "", fmt.Errorf("not initialized")
					}
					return `{"cluster_name": {"value": "my-cluster"}}`, nil
				}
				if args[1] == "init" {
					initArgs = args
					return "", nil
				}
			}
			return "", nil
		}

		outputs, err := mocks.Provider.GetTerraformOutputs("cluster")
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if outputs["cluster_name"] != "my-cluster" {
			t.Errorf("Expected cluster_name to be my-cluster, got: %v", outputs["cluster_name"])
		}

		hasBackendConfig := false
		for _, arg := range initArgs {
			if strings.HasPrefix(arg, "-backend-config") {
				hasBackendConfig = true
				break
			}
		}

		if !hasBackendConfig {
			t.Errorf("Expected init command to include -backend-config arguments, got init args: %v", initArgs)
		}
	})
}

func TestTerraformProvider_GetEnvVars_OnlyIncludesRequestedOutputs(t *testing.T) {
	t.Run("OnlyIncludesOutputsAccessedViaTerraformOutput", func(t *testing.T) {
		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
metadata:
  name: test
terraform:
  - path: network
    name: network
  - path: cluster
    name: cluster
    inputs:
      vpc_id: '${terraform_output("network", "vpc_id")}'`

		mocks := setupMocks(t, &SetupOptions{BlueprintYAML: blueprintYAML, BackendType: "local"})

		mocks.ConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "id" {
				return "test-context"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "terraform" && len(args) >= 2 {
				if args[1] == "output" {
					return `{"vpc_id": {"value": "vpc-123"}, "isolated_subnet_ids": {"value": ["subnet-1"]}, "private_subnet_ids": {"value": ["subnet-2"]}, "public_subnet_ids": {"value": ["subnet-3"]}}`, nil
				}
				if args[1] == "init" {
					return "", nil
				}
			}
			return "", nil
		}

		envVars, _, err := mocks.Provider.GetEnvVars("cluster", false)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if envVars["TF_VAR_vpc_id"] != "vpc-123" {
			t.Errorf("Expected TF_VAR_vpc_id to be 'vpc-123', got: %s", envVars["TF_VAR_vpc_id"])
		}

		unexpectedVars := []string{
			"TF_VAR_isolated_subnet_ids",
			"TF_VAR_private_subnet_ids",
			"TF_VAR_public_subnet_ids",
		}

		for _, unexpectedVar := range unexpectedVars {
			if _, exists := envVars[unexpectedVar]; exists {
				t.Errorf("Expected %s to NOT be included (not accessed via terraform_output)", unexpectedVar)
			}
		}

		mocks.Provider.mu.RLock()
		networkCache := mocks.Provider.cache["network"]
		mocks.Provider.mu.RUnlock()

		if networkCache == nil {
			t.Error("Expected network component outputs to be cached")
		} else {
			if len(networkCache) != 1 {
				t.Errorf("Expected cache to contain only 1 output (vpc_id), got %d: %v", len(networkCache), networkCache)
			}
			if networkCache["vpc_id"] != "vpc-123" {
				t.Errorf("Expected cached vpc_id to be 'vpc-123', got: %v", networkCache["vpc_id"])
			}
			if _, exists := networkCache["isolated_subnet_ids"]; exists {
				t.Error("Expected isolated_subnet_ids to NOT be cached (not accessed via terraform_output)")
			}
		}
	})

	t.Run("AccumulatesMultipleRequestedOutputs", func(t *testing.T) {
		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
metadata:
  name: test
terraform:
  - path: network
    name: network
  - path: cluster
    name: cluster
    inputs:
      vpc_id: '${terraform_output("network", "vpc_id")}'
      subnet_ids: '${terraform_output("network", "private_subnet_ids")}'`

		mocks := setupMocks(t, &SetupOptions{BlueprintYAML: blueprintYAML, BackendType: "local"})

		mocks.ConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "id" {
				return "test-context"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "terraform" && len(args) >= 2 {
				if args[1] == "output" {
					return `{"vpc_id": {"value": "vpc-123"}, "private_subnet_ids": {"value": ["subnet-1", "subnet-2"]}, "public_subnet_ids": {"value": ["subnet-3"]}, "isolated_subnet_ids": {"value": ["subnet-4"]}}`, nil
				}
				if args[1] == "init" {
					return "", nil
				}
			}
			return "", nil
		}

		envVars, _, err := mocks.Provider.GetEnvVars("cluster", false)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if envVars["TF_VAR_vpc_id"] != "vpc-123" {
			t.Errorf("Expected TF_VAR_vpc_id to be 'vpc-123', got: %s", envVars["TF_VAR_vpc_id"])
		}

		expectedSubnetIds := `["subnet-1","subnet-2"]`
		if envVars["TF_VAR_private_subnet_ids"] != expectedSubnetIds {
			t.Errorf("Expected TF_VAR_private_subnet_ids to be %s, got: %s", expectedSubnetIds, envVars["TF_VAR_private_subnet_ids"])
		}

		unexpectedVars := []string{
			"TF_VAR_public_subnet_ids",
			"TF_VAR_isolated_subnet_ids",
		}

		for _, unexpectedVar := range unexpectedVars {
			if _, exists := envVars[unexpectedVar]; exists {
				t.Errorf("Expected %s to NOT be included (not accessed via terraform_output)", unexpectedVar)
			}
		}

		mocks.Provider.mu.RLock()
		networkCache := mocks.Provider.cache["network"]
		mocks.Provider.mu.RUnlock()

		if networkCache == nil {
			t.Error("Expected network component outputs to be cached")
		} else {
			if len(networkCache) != 2 {
				t.Errorf("Expected cache to contain 2 outputs (vpc_id, private_subnet_ids), got %d: %v", len(networkCache), networkCache)
			}
			if networkCache["vpc_id"] != "vpc-123" {
				t.Errorf("Expected cached vpc_id to be 'vpc-123', got: %v", networkCache["vpc_id"])
			}
		}
	})
}
