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

	testEvaluator := options.Evaluator
	if testEvaluator == nil {
		testEvaluator = evaluator.NewExpressionEvaluator(configHandler, "/test/project", "/test/template")
	}

	provider := NewTerraformProvider(configHandler, mockShell, toolsManager, testEvaluator)
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
		Evaluator:     testEvaluator,
	}
}

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewTerraformProvider(t *testing.T) {
	t.Run("CreatesProviderWithEmptyCache", func(t *testing.T) {
		// Given setup mocks
		mocks := setupMocks(t)

		// When creating a provider
		// Then it should have empty cache and initialized shims
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
		// Given a mock evaluator that tracks Register calls
		configHandler := config.NewMockConfigHandler()
		mockShell := shell.NewMockShell()
		toolsManager := tools.NewMockToolsManager()
		mockEvaluator := evaluator.NewMockExpressionEvaluator()
		registerCalled := false
		registerName := ""
		mockEvaluator.RegisterFunc = func(name string, helper func(params []any, deferred bool) (any, error), signature any) {
			registerCalled = true
			registerName = name
		}

		// When creating a provider with evaluator
		provider := NewTerraformProvider(configHandler, mockShell, toolsManager, mockEvaluator)

		// Then Register should be called with terraform_output
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
		// Given an evaluator and provider setup
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

		// When evaluating terraform_output expression
		result, err := testEvaluator.Evaluate(`${terraform_output("test-component", "test-key")}`, "", nil, true)

		// Then helper should be callable and return cached value
		if err != nil {
			t.Fatalf("Expected helper to be registered and callable, got error: %v", err)
		}

		if result != "test-value" {
			t.Errorf("Expected helper to return 'test-value', got %v", result)
		}
	})

}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestTerraformProvider_ClearCache(t *testing.T) {
	t.Run("ClearsAllCachedOutputsAndComponents", func(t *testing.T) {
		// Given a provider with cached outputs and components
		mocks := setupMocks(t)

		mocks.Provider.mu.Lock()
		mocks.Provider.cache["component1"] = map[string]any{"output1": "value1"}
		mocks.Provider.cache["component2"] = map[string]any{"output2": "value2"}
		mocks.Provider.components = []blueprintv1alpha1.TerraformComponent{{Path: "test"}}
		mocks.Provider.mu.Unlock()

		// When clearing the cache
		mocks.Provider.ClearCache()

		// Then cache and components should be cleared
		if len(mocks.Provider.cache) != 0 {
			t.Errorf("Expected cache to be empty after ClearCache, got %d entries", len(mocks.Provider.cache))
		}

		if mocks.Provider.components != nil {
			t.Error("Expected components to be cleared after ClearCache")
		}
	})
}

func TestTerraformProvider_CacheOutputs(t *testing.T) {
	t.Run("CachesOutputsForComponent", func(t *testing.T) {
		mocks := setupMocks(t)
		mocks.Provider.mu.Lock()
		mocks.Provider.components = []blueprintv1alpha1.TerraformComponent{
			{Path: "test-component"},
		}
		mocks.Provider.mu.Unlock()

		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "terraform" && len(args) >= 2 && args[1] == "output" {
				return `{"output1": {"value": "cached-value"}, "output2": {"value": 42}}`, nil
			}
			return "", nil
		}

		err := mocks.Provider.CacheOutputs("test-component")
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		mocks.Provider.mu.RLock()
		cached := mocks.Provider.cache["test-component"]
		mocks.Provider.mu.RUnlock()

		if cached == nil {
			t.Fatal("Expected outputs to be cached")
		}
		if cached["output1"] != "cached-value" {
			t.Errorf("Expected output1 to be 'cached-value', got %v", cached["output1"])
		}
		if cached["output2"] != float64(42) {
			t.Errorf("Expected output2 to be 42, got %v", cached["output2"])
		}
	})

	t.Run("ReturnsNoErrorWhenComponentNotFound", func(t *testing.T) {
		mocks := setupMocks(t)

		err := mocks.Provider.CacheOutputs("nonexistent-component")

		if err != nil {
			t.Errorf("Expected no error for nonexistent component (graceful fallback), got: %v", err)
		}
	})
}

func TestTerraformProvider_GetTerraformComponents(t *testing.T) {
	t.Run("ReturnsCachedComponents", func(t *testing.T) {
		// Given a provider with cached components
		mocks := setupMocks(t)

		expectedComponents := []blueprintv1alpha1.TerraformComponent{
			{Path: "test/path"},
		}

		mocks.Provider.mu.Lock()
		mocks.Provider.components = expectedComponents
		mocks.Provider.mu.Unlock()

		// When getting terraform components
		components := mocks.Provider.GetTerraformComponents()

		// Then cached components should be returned
		if len(components) != len(expectedComponents) {
			t.Errorf("Expected %d components, got %d", len(expectedComponents), len(components))
		}

		if components[0].Path != expectedComponents[0].Path {
			t.Errorf("Expected component path to be 'test/path', got %s", components[0].Path)
		}
	})

	t.Run("LoadsComponentsFromBlueprint", func(t *testing.T) {
		// Given a blueprint with terraform components
		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
metadata:
  name: test
terraform:
  - path: test/path
    name: test-component`

		mocks := setupMocks(t, &SetupOptions{BlueprintYAML: blueprintYAML})

		// When getting terraform components
		components := mocks.Provider.GetTerraformComponents()

		// Then components should be loaded from blueprint
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
		// Given a provider with config handler that returns error for GetConfigRoot
		mocks := setupMocks(t)

		mocks.ConfigHandler.GetConfigRootFunc = func() (string, error) {
			return "", errors.New("config root not found")
		}

		// When getting terraform components
		components := mocks.Provider.GetTerraformComponents()

		// Then it should return empty components
		if len(components) != 0 {
			t.Errorf("Expected empty components on error, got %d", len(components))
		}
	})

	t.Run("HandlesMissingBlueprintFile", func(t *testing.T) {
		// Given a provider with ReadFile that returns file not found
		mocks := setupMocks(t)

		mocks.Provider.Shims.ReadFile = func(path string) ([]byte, error) {
			return nil, os.ErrNotExist
		}

		// When getting terraform components
		components := mocks.Provider.GetTerraformComponents()

		// Then it should return empty components
		if len(components) != 0 {
			t.Errorf("Expected empty components on file error, got %d", len(components))
		}
	})

	t.Run("HandlesInvalidYAML", func(t *testing.T) {
		// Given a provider with invalid YAML in blueprint file
		mocks := setupMocks(t)

		configRoot := "/test/config"
		mocks.Provider.Shims.ReadFile = func(path string) ([]byte, error) {
			if path == filepath.Join(configRoot, "blueprint.yaml") {
				return []byte("invalid: yaml: [unclosed"), nil
			}
			return nil, os.ErrNotExist
		}

		// When getting terraform components
		components := mocks.Provider.GetTerraformComponents()

		// Then it should return empty components
		if len(components) != 0 {
			t.Errorf("Expected empty components on YAML error, got %d", len(components))
		}
	})

	t.Run("SetsFullPathForSourcedComponents", func(t *testing.T) {
		// Given a blueprint with sourced component
		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
metadata:
  name: test
terraform:
  - path: test/path
    source: config`

		mocks := setupMocks(t, &SetupOptions{BlueprintYAML: blueprintYAML})

		// When getting terraform components
		components := mocks.Provider.GetTerraformComponents()

		// Then FullPath should be set to context path
		expectedPath := filepath.Join("/test/project", ".windsor", "contexts", "default", "terraform", "test/path")
		if components[0].FullPath != expectedPath {
			t.Errorf("Expected FullPath to be %s, got %s", expectedPath, components[0].FullPath)
		}
	})

	t.Run("SetsFullPathForLocalComponents", func(t *testing.T) {
		// Given a blueprint with local component
		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
metadata:
  name: test
terraform:
  - path: test/path`

		mocks := setupMocks(t, &SetupOptions{BlueprintYAML: blueprintYAML})

		// When getting terraform components
		components := mocks.Provider.GetTerraformComponents()

		// Then FullPath should be set to project terraform path
		expectedPath := filepath.Join("/test/project", "terraform", "test/path")
		if components[0].FullPath != expectedPath {
			t.Errorf("Expected FullPath to be %s, got %s", expectedPath, components[0].FullPath)
		}
	})
}

func TestTerraformProvider_GenerateBackendOverride(t *testing.T) {
	t.Run("CreatesLocalBackendOverride", func(t *testing.T) {
		// Given a provider with local backend type
		mocks := setupMocks(t, &SetupOptions{BackendType: "local"})

		var writtenPath string
		var writtenData []byte
		mocks.Provider.Shims.WriteFile = func(path string, data []byte, perm os.FileMode) error {
			writtenPath = path
			writtenData = data
			return nil
		}

		// When generating backend override
		err := mocks.Provider.GenerateBackendOverride("/test/dir")

		// Then it should write local backend config
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
		// Given a provider with s3 backend type
		mocks := setupMocks(t, &SetupOptions{BackendType: "s3"})

		mocks.Provider.Shims.WriteFile = func(path string, data []byte, perm os.FileMode) error {
			return nil
		}

		// When generating backend override
		err := mocks.Provider.GenerateBackendOverride("/test/dir")

		// Then it should succeed
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
	})

	t.Run("CreatesKubernetesBackendOverride", func(t *testing.T) {
		// Given a provider with kubernetes backend type
		mocks := setupMocks(t, &SetupOptions{BackendType: "kubernetes"})

		mocks.Provider.Shims.WriteFile = func(path string, data []byte, perm os.FileMode) error {
			return nil
		}

		// When generating backend override
		err := mocks.Provider.GenerateBackendOverride("/test/dir")

		// Then it should succeed
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
	})

	t.Run("CreatesAzurermBackendOverride", func(t *testing.T) {
		// Given a provider with azurerm backend type
		mocks := setupMocks(t, &SetupOptions{BackendType: "azurerm"})

		mocks.Provider.Shims.WriteFile = func(path string, data []byte, perm os.FileMode) error {
			return nil
		}

		// When generating backend override
		err := mocks.Provider.GenerateBackendOverride("/test/dir")

		// Then it should succeed
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
	})

	t.Run("RemovesBackendOverrideForNone", func(t *testing.T) {
		// Given a provider with none backend type
		mocks := setupMocks(t, &SetupOptions{BackendType: "none"})

		var removedPath string
		mocks.Provider.Shims.Stat = func(path string) (os.FileInfo, error) {
			return nil, nil
		}
		mocks.Provider.Shims.Remove = func(path string) error {
			removedPath = path
			return nil
		}

		// When generating backend override
		err := mocks.Provider.GenerateBackendOverride("/test/dir")

		// Then backend override file should be removed
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		expectedPath := filepath.Join("/test/dir", "backend_override.tf")
		if removedPath != expectedPath {
			t.Errorf("Expected to remove %s, got %s", expectedPath, removedPath)
		}
	})

	t.Run("HandlesUnsupportedBackend", func(t *testing.T) {
		// Given a provider with unsupported backend type
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

		// When generating backend override
		err := mocks.Provider.GenerateBackendOverride("/test/dir")

		// Then it should return an error
		if err == nil {
			t.Fatal("Expected error for unsupported backend")
		}

		if err.Error() != "unsupported backend: unsupported" {
			t.Errorf("Expected unsupported backend error, got %v", err)
		}
	})

	t.Run("HandlesWriteFileError", func(t *testing.T) {
		// Given a provider with WriteFile that fails
		mocks := setupMocks(t, &SetupOptions{BackendType: "local"})

		mocks.Provider.Shims.WriteFile = func(path string, data []byte, perm os.FileMode) error {
			return errors.New("write failed")
		}

		// When generating backend override
		err := mocks.Provider.GenerateBackendOverride("/test/dir")

		// Then it should return an error
		if err == nil {
			t.Fatal("Expected error on write failure")
		}
	})

	t.Run("HandlesRemoveError", func(t *testing.T) {
		// Given a provider with Remove that fails
		mocks := setupMocks(t, &SetupOptions{BackendType: "none"})

		mocks.Provider.Shims.Stat = func(path string) (os.FileInfo, error) {
			return nil, nil
		}
		mocks.Provider.Shims.Remove = func(path string) error {
			return errors.New("remove failed")
		}

		// When generating backend override
		err := mocks.Provider.GenerateBackendOverride("/test/dir")

		// Then it should return an error
		if err == nil {
			t.Fatal("Expected error on remove failure")
		}
	})
}

func TestTerraformProvider_GetOutputs(t *testing.T) {
	t.Run("ReturnsCachedOutputs", func(t *testing.T) {
		// Given a provider with cached outputs
		mocks := setupMocks(t)

		expectedOutputs := map[string]any{
			"output1": "value1",
			"output2": 42,
		}

		mocks.Provider.mu.Lock()
		mocks.Provider.cache["test-component"] = expectedOutputs
		mocks.Provider.mu.Unlock()

		// When getting outputs
		output1, err := mocks.Provider.getOutput("test-component", "output1", `terraform_output("test-component", "output1")`, true)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if output1 != expectedOutputs["output1"] {
			t.Errorf("Expected output1 to be 'value1', got %v", output1)
		}

		output2, err := mocks.Provider.getOutput("test-component", "output2", `terraform_output("test-component", "output2")`, true)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		// Then cached outputs should be returned
		if output2 != expectedOutputs["output2"] {
			t.Errorf("Expected output2 to be 42, got %v", output2)
		}
	})

	t.Run("ReturnsEmptyMapWhenComponentNotFound", func(t *testing.T) {
		// Given a blueprint without the requested component
		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
metadata:
  name: test
terraform:
  - path: other/path`

		mocks := setupMocks(t, &SetupOptions{BlueprintYAML: blueprintYAML})

		// When getting output for nonexistent component
		output, err := mocks.Provider.getOutput("nonexistent-component", "any-key", `terraform_output("nonexistent-component", "any-key")`, false)

		// Then it should return DeferredError
		var deferredErr *evaluator.DeferredError
		if err == nil || !errors.As(err, &deferredErr) {
			t.Errorf("Expected DeferredError for nonexistent component, got %v", err)
		}
		if output != nil {
			t.Errorf("Expected nil output for deferred error, got %v", output)
		}
	})

	t.Run("CapturesOutputsFromTerraform", func(t *testing.T) {
		// Given a provider with terraform component and mock shell
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

		// When getting outputs
		output1, err := mocks.Provider.getOutput("test-component", "output1", `terraform_output("test-component", "output1")`, true)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		output2, err := mocks.Provider.getOutput("test-component", "output2", `terraform_output("test-component", "output2")`, true)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then outputs should be captured from terraform
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
		// Given a provider with terraform that returns empty outputs
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

		// When getting output for missing key with deferred=true
		output, err := mocks.Provider.getOutput("test-component", "any-key", `terraform_output("test-component", "any-key")`, true)

		// Then it should return nil without error (enables ?? fallback in expressions)
		if err != nil {
			t.Errorf("Expected no error for graceful fallback, got: %v", err)
		}
		if output != nil {
			t.Errorf("Expected nil output for missing key, got %v", output)
		}
	})

	t.Run("HandlesTerraformInitFallback", func(t *testing.T) {
		// Given a provider with terraform output that fails initially
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

		// When getting output after init fallback
		output, err := mocks.Provider.getOutput("test-component", "output1", `terraform_output("test-component", "output1")`, true)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then output should be retrieved after init
		if output != "val1" {
			t.Errorf("Expected output to be 'val1', got %v (outputCallCount: %d)", output, outputCallCount)
		}
	})

	t.Run("HandlesSetenvError", func(t *testing.T) {
		// Given a provider with Setenv that fails
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

		// When getting output
		output, err := mocks.Provider.getOutput("test-component", "any-key", `terraform_output("test-component", "any-key")`, true)

		// Then it should return nil without error (graceful fallback)
		if err != nil {
			t.Errorf("Expected no error for graceful fallback, got: %v", err)
		}
		if output != nil {
			t.Errorf("Expected nil output when context prep fails, got %v", output)
		}
	})

	t.Run("HandlesBackendOverrideError", func(t *testing.T) {
		// Given a provider with unsupported backend type
		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
metadata:
  name: test
terraform:
  - path: test/path
    name: test-component`

		mocks := setupMocks(t, &SetupOptions{BlueprintYAML: blueprintYAML, BackendType: "unsupported"})

		// When getting output
		output, err := mocks.Provider.getOutput("test-component", "any-key", `terraform_output("test-component", "any-key")`, true)

		// Then it should return nil without error (graceful fallback)
		if err != nil {
			t.Errorf("Expected no error for graceful fallback, got: %v", err)
		}
		if output != nil {
			t.Errorf("Expected nil output when context prep fails, got %v", output)
		}
	})

	t.Run("HandlesCacheRaceCondition", func(t *testing.T) {
		// Given a provider where cache is populated during execution
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

		// When getting output
		output, err := mocks.Provider.getOutput("test-component", "output1", `terraform_output("test-component", "output1")`, true)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then cached value should be returned
		if output != "cached-value" {
			t.Errorf("Expected cached value, got %v", output)
		}
	})

	t.Run("HandlesJsonUnmarshalError", func(t *testing.T) {
		// Given a provider with JsonUnmarshal that fails
		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
metadata:
  name: test
terraform:
  - path: test/path
    name: test-component`

		mocks := setupMocks(t, &SetupOptions{BlueprintYAML: blueprintYAML, BackendType: "local"})

		mocks.Provider.Shims.JsonUnmarshal = func(data []byte, v any) error {
			return errors.New("json unmarshal error")
		}

		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "terraform" && len(args) >= 2 && args[1] == "output" {
				return `{"output1": {"value": "val1"}}`, nil
			}
			return "", nil
		}

		// When getting output
		output, err := mocks.Provider.getOutput("test-component", "output1", `terraform_output("test-component", "output1")`, true)
		// Then it should return an error with the JSON parse error in the chain
		if err == nil {
			t.Error("Expected error when JsonUnmarshal fails, got nil")
		}
		if output != nil {
			t.Errorf("Expected nil output when error occurs, got %v", output)
		}
		if !strings.Contains(err.Error(), "failed to parse terraform output JSON") {
			t.Errorf("Expected error message about JSON parse failure, got: %v", err)
		}
	})

	t.Run("HandlesOutputsWithoutValueField", func(t *testing.T) {
		// Given a provider with terraform output that has no value field
		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
metadata:
  name: test
terraform:
  - path: test/path
    name: test-component`

		mocks := setupMocks(t, &SetupOptions{BlueprintYAML: blueprintYAML, BackendType: "local"})

		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "terraform" && len(args) >= 2 && args[1] == "output" {
				return `{"output1": {"sensitive": true}}`, nil
			}
			return "", nil
		}

		// When getting output
		output, err := mocks.Provider.getOutput("test-component", "output1", `terraform_output("test-component", "output1")`, true)
		// Then it should return nil without error (enables ?? fallback)
		if err != nil {
			t.Errorf("Expected no error for graceful fallback, got: %v", err)
		}
		if output != nil {
			t.Errorf("Expected nil output for missing key, got %v", output)
		}
	})

	t.Run("HandlesOutputsWithNonMapValue", func(t *testing.T) {
		// Given a provider with terraform output that is not a map
		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
metadata:
  name: test
terraform:
  - path: test/path
    name: test-component`

		mocks := setupMocks(t, &SetupOptions{BlueprintYAML: blueprintYAML, BackendType: "local"})

		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "terraform" && len(args) >= 2 && args[1] == "output" {
				return `{"output1": "not-a-map"}`, nil
			}
			return "", nil
		}

		// When getting output
		output, err := mocks.Provider.getOutput("test-component", "output1", `terraform_output("test-component", "output1")`, true)
		// Then it should return nil without error (enables ?? fallback)
		if err != nil {
			t.Errorf("Expected no error for graceful fallback, got: %v", err)
		}
		if output != nil {
			t.Errorf("Expected nil output for malformed output, got %v", output)
		}
	})
}

func TestTerraformProvider_GetTerraformComponent(t *testing.T) {
	t.Run("FindsComponentByPath", func(t *testing.T) {
		// Given a blueprint with a component path
		mocks := setupMocks(t)

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

		// When getting component by path
		component := mocks.Provider.GetTerraformComponent("test/path")

		// Then component should be found
		if component == nil {
			t.Fatal("Expected component to be found")
		}

		if component.Path != "test/path" {
			t.Errorf("Expected component path to be 'test/path', got %s", component.Path)
		}
	})

	t.Run("FindsComponentByName", func(t *testing.T) {
		// Given a blueprint with a named component
		mocks := setupMocks(t)

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

		// When getting component by name
		component := mocks.Provider.GetTerraformComponent("test-component")

		// Then component should be found
		if component == nil {
			t.Fatal("Expected component to be found")
		}

		if component.Name != "test-component" {
			t.Errorf("Expected component name to be 'test-component', got %s", component.Name)
		}
	})

	t.Run("ReturnsNilWhenComponentNotFound", func(t *testing.T) {
		// Given a blueprint without the requested component
		mocks := setupMocks(t)

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

		// When getting nonexistent component
		component := mocks.Provider.GetTerraformComponent("nonexistent")

		// Then it should return nil
		if component != nil {
			t.Error("Expected component to be nil when not found")
		}
	})
}

func TestTerraformProvider_ResolveModulePath(t *testing.T) {
	t.Run("ResolvesPathForComponentWithName", func(t *testing.T) {
		// Given a component with name
		mocks := setupMocks(t)

		mocks.ConfigHandler.GetWindsorScratchPathFunc = func() (string, error) {
			return "/test/scratch", nil
		}

		component := &blueprintv1alpha1.TerraformComponent{
			Name: "test-component",
		}

		// When resolving module path
		path, err := mocks.Provider.resolveModulePath(component)

		// Then it should resolve to scratch path
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		expected := filepath.Join("/test/scratch", "terraform", "test-component")
		if path != expected {
			t.Errorf("Expected path %s, got %s", expected, path)
		}
	})

	t.Run("ResolvesPathForComponentWithSource", func(t *testing.T) {
		// Given a component with source
		mocks := setupMocks(t)

		mocks.ConfigHandler.GetWindsorScratchPathFunc = func() (string, error) {
			return "/test/scratch", nil
		}

		component := &blueprintv1alpha1.TerraformComponent{
			Path:   "test/path",
			Source: "config",
		}

		// When resolving module path
		path, err := mocks.Provider.resolveModulePath(component)

		// Then it should resolve to scratch path
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		expected := filepath.Join("/test/scratch", "terraform", "test/path")
		if path != expected {
			t.Errorf("Expected path %s, got %s", expected, path)
		}
	})

	t.Run("ResolvesPathForLocalComponent", func(t *testing.T) {
		// Given a local component without source
		mocks := setupMocks(t)

		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "/test/project", nil
		}

		component := &blueprintv1alpha1.TerraformComponent{
			Path: "test/path",
		}

		// When resolving module path
		path, err := mocks.Provider.resolveModulePath(component)

		// Then it should resolve to project terraform path
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		expected := filepath.Join("/test/project", "terraform", "test/path")
		if path != expected {
			t.Errorf("Expected path %s, got %s", expected, path)
		}
	})

	t.Run("ReturnsErrorWhenScratchPathFails", func(t *testing.T) {
		// Given a component requiring scratch path that fails
		mocks := setupMocks(t)

		mocks.ConfigHandler.GetWindsorScratchPathFunc = func() (string, error) {
			return "", errors.New("scratch path error")
		}

		component := &blueprintv1alpha1.TerraformComponent{
			Name: "test-component",
		}

		// When resolving module path
		_, err := mocks.Provider.resolveModulePath(component)

		// Then it should return an error
		if err == nil {
			t.Fatal("Expected error when scratch path fails")
		}
	})

	t.Run("ReturnsErrorWhenProjectRootFails", func(t *testing.T) {
		// Given a local component with project root that fails
		mocks := setupMocks(t)

		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "", errors.New("project root error")
		}

		component := &blueprintv1alpha1.TerraformComponent{
			Path: "test/path",
		}

		// When resolving module path
		_, err := mocks.Provider.resolveModulePath(component)

		// Then it should return an error
		if err == nil {
			t.Fatal("Expected error when project root fails")
		}
	})
}

func TestTerraformProvider_IsInTerraformProject(t *testing.T) {
	t.Run("ReturnsTrueWhenInTerraformProject", func(t *testing.T) {
		// Given a provider in a terraform project directory
		mocks := setupMocks(t)
		mocks.Provider.Shims.Getwd = func() (string, error) {
			return filepath.Join("/test", "project", "terraform", "component"), nil
		}
		mocks.Provider.Shims.Glob = func(pattern string) ([]string, error) {
			return []string{filepath.Join("/test", "project", "terraform", "component", "main.tf")}, nil
		}

		// When checking if in terraform project
		result := mocks.Provider.IsInTerraformProject()

		// Then it should return true
		if !result {
			t.Error("Expected IsInTerraformProject to return true")
		}
	})

	t.Run("ReturnsFalseWhenNotInTerraformProject", func(t *testing.T) {
		// Given a provider not in a terraform project directory
		mocks := setupMocks(t)
		mocks.Provider.Shims.Getwd = func() (string, error) {
			return "/test/project", nil
		}
		mocks.Provider.Shims.Glob = func(pattern string) ([]string, error) {
			return nil, nil
		}

		// When checking if in terraform project
		result := mocks.Provider.IsInTerraformProject()

		// Then it should return false
		if result {
			t.Error("Expected IsInTerraformProject to return false")
		}
	})

	t.Run("ReturnsFalseWhenFindRelativeProjectPathErrors", func(t *testing.T) {
		// Given a provider with Getwd that fails
		mocks := setupMocks(t)
		mocks.Provider.Shims.Getwd = func() (string, error) {
			return "", errors.New("getwd error")
		}

		// When checking if in terraform project
		result := mocks.Provider.IsInTerraformProject()

		// Then it should return false
		if result {
			t.Error("Expected IsInTerraformProject to return false on error")
		}
	})
}

func TestTerraformProvider_SetTerraformComponents(t *testing.T) {
	t.Run("SetsComponentsAndReturnsThem", func(t *testing.T) {
		// Given a provider and components to set
		mocks := setupMocks(t)
		components := []blueprintv1alpha1.TerraformComponent{
			{Path: "vpc", Inputs: map[string]any{"name": "test-vpc"}},
			{Path: "rds", Inputs: map[string]any{"name": "test-rds"}},
		}

		// When setting components
		mocks.Provider.SetTerraformComponents(components)

		// Then they should be returned
		result := mocks.Provider.GetTerraformComponents()

		if len(result) != 2 {
			t.Errorf("Expected 2 components, got %d", len(result))
		}

		if result[0].Path != "vpc" {
			t.Errorf("Expected first component path to be 'vpc', got '%s'", result[0].Path)
		}

		if result[1].Path != "rds" {
			t.Errorf("Expected second component path to be 'rds', got '%s'", result[1].Path)
		}
	})

	t.Run("OverridesPreviouslyLoadedComponents", func(t *testing.T) {
		// Given a provider with components loaded from blueprint
		mocks := setupMocks(t, &SetupOptions{
			BlueprintYAML: `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
terraform:
  - path: original
`,
		})

		originalComponents := mocks.Provider.GetTerraformComponents()
		if len(originalComponents) == 0 {
			t.Fatal("Expected original components to be loaded")
		}

		newComponents := []blueprintv1alpha1.TerraformComponent{
			{Path: "new", Inputs: map[string]any{"name": "new-component"}},
		}

		// When setting new components
		mocks.Provider.SetTerraformComponents(newComponents)

		// Then they should override the original components
		result := mocks.Provider.GetTerraformComponents()

		if len(result) != 1 {
			t.Errorf("Expected 1 component after override, got %d", len(result))
		}

		if result[0].Path != "new" {
			t.Errorf("Expected component path to be 'new', got '%s'", result[0].Path)
		}
	})
}

func TestTerraformProvider_FindRelativeProjectPath(t *testing.T) {
	t.Run("FindsProjectPathInTerraformDirectory", func(t *testing.T) {
		// Given a provider in terraform directory
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

		// When finding relative project path
		path, err := mocks.Provider.FindRelativeProjectPath()

		// Then it should return relative path
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		expected := "component/subdir"
		if path != expected {
			t.Errorf("Expected path %s, got %s", expected, path)
		}
	})

	t.Run("FindsProjectPathInContextsDirectory", func(t *testing.T) {
		// Given a provider in contexts directory
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

		// When finding relative project path
		path, err := mocks.Provider.FindRelativeProjectPath()

		// Then it should return relative path
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		expected := "component"
		if path != expected {
			t.Errorf("Expected path %s, got %s", expected, path)
		}
	})

	t.Run("ReturnsEmptyWhenNoTerraformFiles", func(t *testing.T) {
		// Given a provider in directory without terraform files
		mocks := setupMocks(t)

		testPath := "/test/path"
		mocks.Provider.Shims.Getwd = func() (string, error) {
			return testPath, nil
		}

		mocks.Provider.Shims.Glob = func(pattern string) ([]string, error) {
			return nil, nil
		}

		// When finding relative project path
		path, err := mocks.Provider.FindRelativeProjectPath()

		// Then it should return empty path
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if path != "" {
			t.Errorf("Expected empty path, got %s", path)
		}
	})

	t.Run("ReturnsErrorWhenGetwdFails", func(t *testing.T) {
		// Given a provider with Getwd that fails
		mocks := setupMocks(t)

		mocks.Provider.Shims.Getwd = func() (string, error) {
			return "", errors.New("getwd failed")
		}

		// When finding relative project path
		_, err := mocks.Provider.FindRelativeProjectPath()

		// Then it should return an error
		if err == nil {
			t.Fatal("Expected error when Getwd fails")
		}
	})

	t.Run("UsesProvidedDirectory", func(t *testing.T) {
		// Given a provider and a directory path
		mocks := setupMocks(t)

		testPath := filepath.Join("/test", "project", "terraform", "component")
		mocks.Provider.Shims.Glob = func(pattern string) ([]string, error) {
			if pattern == filepath.Join(testPath, "*.tf") {
				return []string{filepath.Join(testPath, "main.tf")}, nil
			}
			return nil, nil
		}

		// When finding relative project path with provided directory
		path, err := mocks.Provider.FindRelativeProjectPath(testPath)

		// Then it should use the provided directory
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		expected := "component"
		if path != expected {
			t.Errorf("Expected path %s, got %s", expected, path)
		}
	})

	t.Run("ReturnsErrorWhenGlobFails", func(t *testing.T) {
		// Given a provider with Glob that fails
		mocks := setupMocks(t)

		testPath := "/test/path"
		mocks.Provider.Shims.Getwd = func() (string, error) {
			return testPath, nil
		}

		mocks.Provider.Shims.Glob = func(pattern string) ([]string, error) {
			return nil, errors.New("glob failed")
		}

		// When finding relative project path
		_, err := mocks.Provider.FindRelativeProjectPath()

		// Then it should return an error
		if err == nil {
			t.Fatal("Expected error when Glob fails")
		}
	})

	t.Run("ReturnsEmptyWhenNoTerraformOrContextsDirectory", func(t *testing.T) {
		// Given a provider in directory outside terraform/contexts
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

		// When finding relative project path
		path, err := mocks.Provider.FindRelativeProjectPath()

		// Then it should return empty path
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
		// Given a provider with proper configuration
		mocks := setupMocks(t)

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
		// When generating terraform args
		args, err := mocks.Provider.GenerateTerraformArgs("test/path", modulePath, true)

		// Then args should be generated successfully
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		expectedTFDataDir := filepath.ToSlash(filepath.Join(windsorScratchPath, ".terraform", "test/path"))
		if args.TFDataDir != expectedTFDataDir {
			t.Errorf("Expected TFDataDir %s, got %s", expectedTFDataDir, args.TFDataDir)
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
		// Given a provider with tfvars file
		mocks := setupMocks(t)

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
		// When generating terraform args
		args, err := mocks.Provider.GenerateTerraformArgs("test/path", modulePath, true)

		// Then var-file args should be included
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
		// Given a provider with component that has parallelism
		mocks := setupMocks(t)

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
		// When generating terraform args
		args, err := mocks.Provider.GenerateTerraformArgs("test/path", modulePath, true)

		// Then parallelism should be included in apply and destroy args
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

	t.Run("HandlesIntExpressionParallelism", func(t *testing.T) {
		mocks := setupMocks(t)

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

		parallelismValue := 10
		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
metadata:
  name: test
terraform:
  - path: test/path
    parallelism: 10`

		mocks.Provider.Shims.ReadFile = func(path string) ([]byte, error) {
			if path == filepath.Join(configRoot, "blueprint.yaml") {
				return []byte(blueprintYAML), nil
			}
			return nil, os.ErrNotExist
		}

		mocks.Provider.Shims.Stat = func(path string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		component := mocks.Provider.GetTerraformComponent("test/path")
		if component == nil {
			t.Fatal("Expected component to be loaded")
		}

		if component.Parallelism == nil {
			t.Fatal("Expected parallelism to be set")
		}

		parallelismInt := component.Parallelism.ToInt()
		if parallelismInt == nil {
			t.Fatal("Expected ToInt() to return non-nil value")
		}
		if *parallelismInt != parallelismValue {
			t.Errorf("Expected parallelism to be %d, got %d", parallelismValue, *parallelismInt)
		}

		modulePath := filepath.Join("/test/project", "terraform", "test/path")
		args, err := mocks.Provider.GenerateTerraformArgs("test/path", modulePath, true)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		foundParallelismInApply := false
		for _, arg := range args.ApplyArgs {
			if arg == fmt.Sprintf("-parallelism=%d", parallelismValue) {
				foundParallelismInApply = true
				break
			}
		}
		if !foundParallelismInApply {
			t.Errorf("Expected ApplyArgs to contain -parallelism=%d, got %v", parallelismValue, args.ApplyArgs)
		}

		foundParallelismInDestroy := false
		for _, arg := range args.DestroyArgs {
			if arg == fmt.Sprintf("-parallelism=%d", parallelismValue) {
				foundParallelismInDestroy = true
				break
			}
		}
		if !foundParallelismInDestroy {
			t.Errorf("Expected DestroyArgs to contain -parallelism=%d, got %v", parallelismValue, args.DestroyArgs)
		}
	})

	t.Run("HandlesNilParallelism", func(t *testing.T) {
		mocks := setupMocks(t)

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

		for _, arg := range args.ApplyArgs {
			if strings.Contains(arg, "-parallelism=") {
				t.Errorf("Expected ApplyArgs to not contain parallelism arg, got %v", args.ApplyArgs)
			}
		}

		for _, arg := range args.DestroyArgs {
			if strings.Contains(arg, "-parallelism=") {
				t.Errorf("Expected DestroyArgs to not contain parallelism arg, got %v", args.DestroyArgs)
			}
		}
	})

	t.Run("IncludesAutoApproveForNonInteractive", func(t *testing.T) {
		// Given a provider with non-interactive mode
		mocks := setupMocks(t)

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

		// When generating terraform args with non-interactive mode
		args, err := mocks.Provider.GenerateTerraformArgs("test/path", "test/module", false)

		// Then auto-approve should be included
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if len(args.DestroyArgs) == 0 || args.DestroyArgs[0] != "-auto-approve" {
			t.Errorf("Expected DestroyArgs to start with -auto-approve for non-interactive, got %v", args.DestroyArgs)
		}
	})

	t.Run("ReturnsErrorWhenConfigRootFails", func(t *testing.T) {
		// Given a provider with GetConfigRoot that fails
		mocks := setupMocks(t)

		mocks.ConfigHandler.GetConfigRootFunc = func() (string, error) {
			return "", fmt.Errorf("config root error")
		}

		// When generating terraform args
		_, err := mocks.Provider.GenerateTerraformArgs("test/path", "test/module", true)

		// Then it should return an error
		if err == nil {
			t.Error("Expected error when GetConfigRoot fails")
		}
		if !strings.Contains(err.Error(), "config root") {
			t.Errorf("Expected error about config root, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenWindsorScratchPathFails", func(t *testing.T) {
		// Given a provider with GetWindsorScratchPath that fails
		mocks := setupMocks(t)

		configRoot := "/test/config"
		mocks.ConfigHandler.GetConfigRootFunc = func() (string, error) {
			return configRoot, nil
		}

		mocks.ConfigHandler.GetWindsorScratchPathFunc = func() (string, error) {
			return "", fmt.Errorf("windsor scratch path error")
		}

		// When generating terraform args
		_, err := mocks.Provider.GenerateTerraformArgs("test/path", "test/module", true)

		// Then it should return an error
		if err == nil {
			t.Error("Expected error when GetWindsorScratchPath fails")
		}
		if !strings.Contains(err.Error(), "windsor scratch path") {
			t.Errorf("Expected error about windsor scratch path, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenStatFails", func(t *testing.T) {
		// Given a provider with Stat that fails
		mocks := setupMocks(t)

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

		// When generating terraform args
		_, err := mocks.Provider.GenerateTerraformArgs("test/path", "test/module", true)

		// Then it should return an error
		if err == nil {
			t.Error("Expected error when Stat fails")
		}
		if !strings.Contains(err.Error(), "error checking file") {
			t.Errorf("Expected error about checking file, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenGenerateBackendConfigArgsFails", func(t *testing.T) {
		// Given a provider with invalid backend type
		mocks := setupMocks(t)

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

		// When generating terraform args
		_, err := mocks.Provider.GenerateTerraformArgs("test/path", "test/module", true)

		// Then it should return an error
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
		// Given args with var-file
		mocks := setupMocks(t)

		args := []string{"-var-file=/path/to/file.tfvars"}
		// When formatting args for env
		result := mocks.Provider.FormatArgsForEnv(args)

		// Then var-file should be quoted
		expected := `-var-file="/path/to/file.tfvars"`
		if result != expected {
			t.Errorf("Expected %s, got %s", expected, result)
		}
	})

	t.Run("FormatsOutArgs", func(t *testing.T) {
		// Given args with out
		mocks := setupMocks(t)

		args := []string{"-out=plan.tfplan"}
		// When formatting args for env
		result := mocks.Provider.FormatArgsForEnv(args)

		// Then out should be quoted
		expected := `-out="plan.tfplan"`
		if result != expected {
			t.Errorf("Expected %s, got %s", expected, result)
		}
	})

	t.Run("FormatsBackendConfigArgs", func(t *testing.T) {
		// Given args with backend-config
		mocks := setupMocks(t)

		args := []string{"-backend-config=key=value"}
		// When formatting args for env
		result := mocks.Provider.FormatArgsForEnv(args)

		// Then backend-config should be quoted
		expected := `-backend-config="key=value"`
		if result != expected {
			t.Errorf("Expected %s, got %s", expected, result)
		}
	})

	t.Run("PreservesAlreadyQuotedBackendConfig", func(t *testing.T) {
		// Given args with already quoted backend-config
		mocks := setupMocks(t)

		args := []string{`-backend-config="key=value"`}
		// When formatting args for env
		result := mocks.Provider.FormatArgsForEnv(args)

		// Then quotes should be preserved
		expected := `-backend-config="key=value"`
		if result != expected {
			t.Errorf("Expected %s, got %s", expected, result)
		}
	})

	t.Run("FormatsRelativePaths", func(t *testing.T) {
		// Given args with relative path
		mocks := setupMocks(t)

		args := []string{"./relative/path"}
		// When formatting args for env
		result := mocks.Provider.FormatArgsForEnv(args)

		// Then relative path should be quoted
		expected := `"./relative/path"`
		if result != expected {
			t.Errorf("Expected %s, got %s", expected, result)
		}
	})

	t.Run("FormatsWindowsDrivePaths", func(t *testing.T) {
		// Given args with Windows drive paths
		mocks := setupMocks(t)

		args := []string{"C:/windows/path", "D:\\windows\\path"}
		// When formatting args for env
		result := mocks.Provider.FormatArgsForEnv(args)

		// Then Windows paths should be quoted
		if !strings.Contains(result, `"C:/windows/path"`) {
			t.Errorf("Expected result to contain quoted C:/windows/path, got %s", result)
		}
		if !strings.Contains(result, `"D:\windows\path"`) {
			t.Errorf("Expected result to contain quoted D:\\windows\\path, got %s", result)
		}
	})

	t.Run("PreservesFlags", func(t *testing.T) {
		// Given args with flags
		mocks := setupMocks(t)

		args := []string{"-flag", "--long-flag", "-backend=true"}
		// When formatting args for env
		result := mocks.Provider.FormatArgsForEnv(args)

		// Then flags should be preserved without quotes
		expected := `-flag --long-flag -backend=true`
		if result != expected {
			t.Errorf("Expected %s, got %s", expected, result)
		}
	})

	t.Run("HandlesMultipleArgs", func(t *testing.T) {
		// Given args with multiple types
		mocks := setupMocks(t)

		args := []string{"-var-file=file.tfvars", "-out=plan.tfplan", "/path/to/file"}
		// When formatting args for env
		result := mocks.Provider.FormatArgsForEnv(args)

		// Then all args should be formatted correctly
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
		// Given empty args
		mocks := setupMocks(t)

		args := []string{}
		// When formatting args for env
		result := mocks.Provider.FormatArgsForEnv(args)

		// Then it should return empty string
		if result != "" {
			t.Errorf("Expected empty string, got %s", result)
		}
	})
}

func TestTerraformProvider_GetTFDataDir(t *testing.T) {
	t.Run("ReturnsTFDataDirForComponentID", func(t *testing.T) {
		// Given a provider with component ID
		mocks := setupMocks(t)

		windsorScratchPath := "/test/scratch"
		mocks.ConfigHandler.GetWindsorScratchPathFunc = func() (string, error) {
			return windsorScratchPath, nil
		}

		// When getting TF data dir
		tfDataDir, err := mocks.Provider.GetTFDataDir("test-component")
		// Then it should return correct path
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		expected := filepath.ToSlash(filepath.Join(windsorScratchPath, ".terraform", "test-component"))
		if tfDataDir != expected {
			t.Errorf("Expected TFDataDir %s, got %s", expected, tfDataDir)
		}
	})

	t.Run("UsesComponentGetIDWhenComponentExists", func(t *testing.T) {
		// Given a provider with component that exists in blueprint
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

		// When getting TF data dir
		tfDataDir, err := mocks.Provider.GetTFDataDir("cluster")
		// Then it should use component GetID
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		expected := filepath.ToSlash(filepath.Join(windsorScratchPath, ".terraform", "cluster"))
		if tfDataDir != expected {
			t.Errorf("Expected TFDataDir %s, got %s", expected, tfDataDir)
		}
	})

	t.Run("ReturnsErrorWhenScratchPathFails", func(t *testing.T) {
		// Given a provider with GetWindsorScratchPath that fails
		mocks := setupMocks(t)

		mocks.ConfigHandler.GetWindsorScratchPathFunc = func() (string, error) {
			return "", fmt.Errorf("scratch path error")
		}

		// When getting TF data dir
		_, err := mocks.Provider.GetTFDataDir("test-component")
		// Then it should return an error
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
		// Given a provider with terraform component
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

		// When getting terraform outputs
		outputs, err := mocks.Provider.GetTerraformOutputs("cluster")
		// Then it should return outputs
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
		// Given a provider with nonexistent component
		mocks := setupMocks(t)

		// When getting terraform outputs for nonexistent component
		outputs, err := mocks.Provider.GetTerraformOutputs("nonexistent")
		// Then it should return empty map without error (enables ?? fallback)
		if err != nil {
			t.Fatalf("Expected no error for graceful fallback, got: %v", err)
		}

		if len(outputs) != 0 {
			t.Errorf("Expected empty map, got: %v", outputs)
		}
	})

	t.Run("HandlesEmptyOutput", func(t *testing.T) {
		// Given a provider with terraform that returns empty output
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

		// When getting terraform outputs
		outputs, err := mocks.Provider.GetTerraformOutputs("cluster")
		// Then it should return empty map
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if len(outputs) != 0 {
			t.Errorf("Expected empty map, got: %v", outputs)
		}
	})

	t.Run("InitializesModuleWhenOutputFails", func(t *testing.T) {
		// Given a provider with terraform output that fails initially
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

		// When getting terraform outputs
		outputs, err := mocks.Provider.GetTerraformOutputs("cluster")
		// Then it should initialize and retry
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if outputs["cluster_name"] != "my-cluster" {
			t.Errorf("Expected cluster_name to be my-cluster, got: %v", outputs["cluster_name"])
		}
	})

}

func TestTerraformProvider_GetEnvVars(t *testing.T) {
	t.Run("OnlyIncludesOutputsAccessedViaTerraformOutput", func(t *testing.T) {
		// Given a blueprint with component inputs using terraform_output
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

		configHandler := config.NewMockConfigHandler()
		testEvaluator := evaluator.NewExpressionEvaluator(configHandler, "/test/project", "/test/template")
		mocks := setupMocks(t, &SetupOptions{BlueprintYAML: blueprintYAML, BackendType: "local", Evaluator: testEvaluator})
		mocks.Provider.evaluator = testEvaluator

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

		// When getting env vars
		envVars, _, err := mocks.Provider.GetEnvVars("cluster", false)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Then only outputs accessed via terraform_output should be included
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
			if networkCache["vpc_id"] != "vpc-123" {
				t.Errorf("Expected cached vpc_id to be 'vpc-123', got: %v", networkCache["vpc_id"])
			}
			// All outputs are cached when fetched, even if not accessed via terraform_output()
			if isolatedSubnets, exists := networkCache["isolated_subnet_ids"]; !exists {
				t.Error("Expected isolated_subnet_ids to be cached (all outputs are cached when fetched)")
			} else {
				// Verify it's the actual value, not a deferred expression
				expected := []any{"subnet-1"}
				if fmt.Sprintf("%v", isolatedSubnets) != fmt.Sprintf("%v", expected) {
					t.Errorf("Expected isolated_subnet_ids to be cached as actual value, got: %v", isolatedSubnets)
				}
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

		configHandler := config.NewMockConfigHandler()
		testEvaluator := evaluator.NewExpressionEvaluator(configHandler, "/test/project", "/test/template")
		mocks := setupMocks(t, &SetupOptions{BlueprintYAML: blueprintYAML, BackendType: "local", Evaluator: testEvaluator})
		mocks.Provider.evaluator = testEvaluator

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
		if envVars["TF_VAR_subnet_ids"] != expectedSubnetIds {
			t.Errorf("Expected TF_VAR_subnet_ids to be %s, got: %s", expectedSubnetIds, envVars["TF_VAR_subnet_ids"])
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
			if networkCache["vpc_id"] != "vpc-123" {
				t.Errorf("Expected cached vpc_id to be 'vpc-123', got: %v", networkCache["vpc_id"])
			}
			if str, isString := networkCache["private_subnet_ids"].(string); isString && strings.HasPrefix(str, "terraform_output(") {
				t.Errorf("Expected private_subnet_ids to be accessed (not deferred), got: %v", networkCache["private_subnet_ids"])
			}
		}
	})

	t.Run("EvaluatesInputsWithContainsExpression", func(t *testing.T) {
		// Given a blueprint with inputs containing expressions
		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
terraform:
  - path: cluster
    inputs:
      cluster_name: "test-cluster"
      vpc_id: "${terraform_output('vpc', 'id')}"`

		mocks := setupMocks(t, &SetupOptions{BlueprintYAML: blueprintYAML, BackendType: "none"})
		mockEvaluator := evaluator.NewMockExpressionEvaluator()
		mockEvaluator.EvaluateMapFunc = func(values map[string]any, featurePath string, scope map[string]any, evaluateDeferred bool) (map[string]any, error) {
			result := make(map[string]any)
			for key, value := range values {
				if strVal, ok := value.(string); ok && strings.Contains(strVal, "${") {
					result[key] = "evaluated-vpc-id"
				} else {
					result[key] = value
				}
			}
			return result, nil
		}
		mocks.Provider.evaluator = mockEvaluator

		// When getting env vars
		envVars, _, err := mocks.Provider.GetEnvVars("cluster", false)

		// Then inputs with expressions should be evaluated
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if envVars["TF_VAR_vpc_id"] != "evaluated-vpc-id" {
			t.Errorf("Expected TF_VAR_vpc_id to be 'evaluated-vpc-id', got '%s'", envVars["TF_VAR_vpc_id"])
		}
	})

	t.Run("SkipsInputsWithoutContainsExpression", func(t *testing.T) {
		// Given a blueprint with inputs without expressions
		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
terraform:
  - path: cluster
    inputs:
      cluster_name: "test-cluster"
      plain_value: "plain"`

		mocks := setupMocks(t, &SetupOptions{BlueprintYAML: blueprintYAML, BackendType: "none"})

		// When getting env vars
		envVars, _, err := mocks.Provider.GetEnvVars("cluster", false)

		// Then inputs without expressions should be skipped
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if _, exists := envVars["TF_VAR_cluster_name"]; exists {
			t.Error("Expected TF_VAR_cluster_name to not be set when ContainsExpression returns false")
		}

		if _, exists := envVars["TF_VAR_plain_value"]; exists {
			t.Error("Expected TF_VAR_plain_value to not be set when ContainsExpression returns false")
		}
	})

	t.Run("HandlesComponentNotFoundWithProjectRoot", func(t *testing.T) {
		// Given a provider with project root and nonexistent component
		mocks := setupMocks(t, &SetupOptions{BackendType: "none"})

		// When getting env vars for nonexistent component
		envVars, _, err := mocks.Provider.GetEnvVars("nonexistent", false)

		// Then it should return empty env vars without error
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if envVars == nil {
			t.Error("Expected envVars to be non-nil")
		}
	})

	t.Run("HandlesComponentNotFoundWithoutProjectRoot", func(t *testing.T) {
		// Given a provider without project root and nonexistent component
		mocks := setupMocks(t, &SetupOptions{BackendType: "none"})
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "", errors.New("project root error")
		}

		// When getting env vars for nonexistent component
		_, _, err := mocks.Provider.GetEnvVars("nonexistent", false)

		// Then it should return an error
		if err == nil {
			t.Fatal("Expected error when component not found and project root unavailable")
		}
	})

	t.Run("HandlesEvaluateMapError", func(t *testing.T) {
		// Given a provider with evaluator that fails
		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
terraform:
  - path: cluster
    inputs:
      vpc_id: "${terraform_output('vpc', 'id')}"`

		mocks := setupMocks(t, &SetupOptions{BlueprintYAML: blueprintYAML, BackendType: "none"})
		mockEvaluator := evaluator.NewMockExpressionEvaluator()
		mockEvaluator.EvaluateMapFunc = func(values map[string]any, featurePath string, scope map[string]any, evaluateDeferred bool) (map[string]any, error) {
			return nil, errors.New("evaluation error")
		}
		mocks.Provider.evaluator = mockEvaluator

		// When getting env vars
		_, _, err := mocks.Provider.GetEnvVars("cluster", false)

		// Then it should return an error
		if err == nil {
			t.Fatal("Expected error when EvaluateMap fails")
		}
	})

	t.Run("HandlesNonStringEvaluatedValue", func(t *testing.T) {
		// Given a provider with evaluator that returns non-string value
		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
terraform:
  - path: cluster
    inputs:
      vpc_id: "${terraform_output('vpc', 'id')}"`

		mocks := setupMocks(t, &SetupOptions{BlueprintYAML: blueprintYAML, BackendType: "none"})
		mockEvaluator := evaluator.NewMockExpressionEvaluator()
		mockEvaluator.EvaluateMapFunc = func(values map[string]any, featurePath string, scope map[string]any, evaluateDeferred bool) (map[string]any, error) {
			return map[string]any{
				"vpc_id": 42,
			}, nil
		}
		mocks.Provider.evaluator = mockEvaluator
		mocks.Provider.Shims.JsonMarshal = func(v any) ([]byte, error) {
			return []byte(`"42"`), nil
		}

		// When getting env vars
		envVars, _, err := mocks.Provider.GetEnvVars("cluster", false)

		// Then non-string values should be JSON marshaled
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if envVars["TF_VAR_vpc_id"] != `"42"` {
			t.Errorf("Expected TF_VAR_vpc_id to be JSON marshaled, got '%s'", envVars["TF_VAR_vpc_id"])
		}
	})

	t.Run("HandlesJsonMarshalError", func(t *testing.T) {
		// Given a provider with JsonMarshal that fails
		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
terraform:
  - path: cluster
    inputs:
      vpc_id: "${terraform_output('vpc', 'id')}"`

		mocks := setupMocks(t, &SetupOptions{BlueprintYAML: blueprintYAML, BackendType: "none"})
		mockEvaluator := evaluator.NewMockExpressionEvaluator()
		mockEvaluator.EvaluateMapFunc = func(values map[string]any, featurePath string, scope map[string]any, evaluateDeferred bool) (map[string]any, error) {
			return map[string]any{
				"vpc_id": make(chan int),
			}, nil
		}
		mocks.Provider.evaluator = mockEvaluator
		mocks.Provider.Shims.JsonMarshal = func(v any) ([]byte, error) {
			return nil, errors.New("marshal error")
		}

		// When getting env vars
		envVars, _, err := mocks.Provider.GetEnvVars("cluster", false)

		// Then marshal error should be skipped and var not set
		if err != nil {
			t.Fatalf("Expected no error (marshal error is skipped), got: %v", err)
		}

		if _, exists := envVars["TF_VAR_vpc_id"]; exists {
			t.Error("Expected TF_VAR_vpc_id to not be set when JSON marshal fails")
		}
	})

	t.Run("HandlesResolveModulePathErrorForDirectComponent", func(t *testing.T) {
		mocks := setupMocks(t, &SetupOptions{BackendType: "none"})

		component := blueprintv1alpha1.TerraformComponent{
			Name:   "cluster",
			Source: "./modules/cluster",
		}
		mocks.Provider.SetTerraformComponents([]blueprintv1alpha1.TerraformComponent{component})

		mocks.ConfigHandler.GetWindsorScratchPathFunc = func() (string, error) {
			return "", errors.New("scratch path error")
		}

		_, _, err := mocks.Provider.GetEnvVars("cluster", false)

		if err == nil {
			t.Fatal("Expected error when resolveModulePath fails")
		}
		if !strings.Contains(err.Error(), "error resolving module path") {
			t.Errorf("Expected error to mention resolving module path, got: %v", err)
		}
	})

	t.Run("HandlesGenerateTerraformArgsError", func(t *testing.T) {
		mocks := setupMocks(t, &SetupOptions{BackendType: "none"})

		component := blueprintv1alpha1.TerraformComponent{
			Path:     "cluster",
			FullPath: "/test/project/terraform/cluster",
		}
		mocks.Provider.SetTerraformComponents([]blueprintv1alpha1.TerraformComponent{component})

		mocks.ConfigHandler.GetWindsorScratchPathFunc = func() (string, error) {
			return "", errors.New("scratch path error")
		}

		_, _, err := mocks.Provider.GetEnvVars("cluster", false)

		if err == nil {
			t.Fatal("Expected error when GenerateTerraformArgs fails")
		}
		if !strings.Contains(err.Error(), "error generating terraform args") {
			t.Errorf("Expected error to mention generating terraform args, got: %v", err)
		}
	})

	t.Run("HandlesGetBaseEnvVarsForComponentError", func(t *testing.T) {
		mocks := setupMocks(t, &SetupOptions{BackendType: "none"})

		component := blueprintv1alpha1.TerraformComponent{
			Path:     "cluster",
			FullPath: "/test/project/terraform/cluster",
		}
		mocks.Provider.SetTerraformComponents([]blueprintv1alpha1.TerraformComponent{component})

		callCount := 0
		mocks.ConfigHandler.GetConfigRootFunc = func() (string, error) {
			callCount++
			if callCount <= 1 {
				return "/test/config", nil
			}
			return "", errors.New("config root error")
		}

		_, _, err := mocks.Provider.GetEnvVars("cluster", false)

		if err == nil {
			t.Fatal("Expected error when getBaseEnvVarsForComponent fails")
		}
		if !strings.Contains(err.Error(), "config root") {
			t.Errorf("Expected error to mention config root, got: %v", err)
		}
	})

	t.Run("HandlesEvaluatedKeyNotExist", func(t *testing.T) {
		blueprintYAML := `apiVersion: blueprints.windsorcli.dev/v1alpha1
kind: Blueprint
terraform:
  - path: cluster
    inputs:
      vpc_id: "${terraform_output('vpc', 'id')}"`

		mocks := setupMocks(t, &SetupOptions{BlueprintYAML: blueprintYAML, BackendType: "none"})
		mockEvaluator := evaluator.NewMockExpressionEvaluator()
		mockEvaluator.EvaluateMapFunc = func(values map[string]any, featurePath string, scope map[string]any, evaluateDeferred bool) (map[string]any, error) {
			return map[string]any{}, nil
		}
		mocks.Provider.evaluator = mockEvaluator

		envVars, _, err := mocks.Provider.GetEnvVars("cluster", false)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if _, exists := envVars["TF_VAR_vpc_id"]; exists {
			t.Error("Expected TF_VAR_vpc_id to not be set when key doesn't exist in evaluated result")
		}
	})
}
