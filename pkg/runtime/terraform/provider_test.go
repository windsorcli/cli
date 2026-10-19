package terraform

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/evaluator"
	"github.com/windsorcli/cli/pkg/runtime/shell"
	"github.com/windsorcli/cli/pkg/runtime/tools"
)

// =============================================================================
// Test Setup
// =============================================================================

// fakeFileInfo is a minimal os.FileInfo with a caller-controlled Mode, used where a
// Stat mock must return a real FileInfo (callers that call .Mode()) rather than nil.
type fakeFileInfo struct {
	mode os.FileMode
}

func (f fakeFileInfo) Name() string       { return ".env" }
func (f fakeFileInfo) Size() int64        { return 0 }
func (f fakeFileInfo) Mode() os.FileMode  { return f.mode }
func (f fakeFileInfo) ModTime() time.Time { return time.Time{} }
func (f fakeFileInfo) IsDir() bool        { return false }
func (f fakeFileInfo) Sys() any           { return nil }

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
		mockEvaluator.RegisterFunc = func(name string, helper func(params []any, deferred bool) (any, error), signatures ...any) {
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

func TestTerraformProvider_GetTerraformComponentForPath(t *testing.T) {
	setup := func(t *testing.T) *Mocks {
		t.Helper()
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

		return mocks
	}

	t.Run("FindsComponentByFullPath", func(t *testing.T) {
		// Given a blueprint with a component whose FullPath resolves under the project root
		mocks := setup(t)
		wantPath := filepath.Join("/test/project", "terraform", "test/path")

		// When resolving that exact directory
		component := mocks.Provider.GetTerraformComponentForPath(wantPath)

		// Then the matching component is returned
		if component == nil {
			t.Fatal("Expected component to be found")
		}
		if component.Path != "test/path" {
			t.Errorf("Expected component path to be 'test/path', got %s", component.Path)
		}
	})

	t.Run("ReturnsNilWhenDirectoryNotInBlueprint", func(t *testing.T) {
		// Given the same blueprint, and a directory none of its components resolve to — e.g. a
		// terraform module directory shared on disk with a different context's blueprint
		mocks := setup(t)

		// When resolving an unrelated directory
		component := mocks.Provider.GetTerraformComponentForPath("/test/project/terraform/cluster/gcp-gke")

		// Then no component is returned
		if component != nil {
			t.Errorf("Expected nil for a directory outside the active blueprint, got %+v", component)
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

// =============================================================================
// Test Private Methods
// =============================================================================

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
