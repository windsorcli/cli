package terraform_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/evaluator"
	"github.com/windsorcli/cli/pkg/runtime/shell"
	"github.com/windsorcli/cli/pkg/runtime/terraform"
	"github.com/windsorcli/cli/pkg/runtime/tools"
)

// =============================================================================
// Test Setup
// =============================================================================

type Mocks struct {
	Provider      *terraform.DefaultTerraformProvider
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

	provider := terraform.NewTerraformProvider(configHandler, mockShell, toolsManager, testEvaluator)
	concreteProvider := provider.(*terraform.DefaultTerraformProvider)

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
