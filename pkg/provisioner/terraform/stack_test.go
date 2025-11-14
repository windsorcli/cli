package terraform

// The StackTest provides comprehensive test coverage for the Stack interface implementation.
// It provides validation of stack initialization, component management, and infrastructure operations,
// The StackTest ensures proper dependency injection and component lifecycle management,
// verifying error handling, mock interactions, and infrastructure state management.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/composer/blueprint"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/config"
	envvars "github.com/windsorcli/cli/pkg/runtime/env"
	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// =============================================================================
// Test Setup
// =============================================================================

// createTestBlueprint creates a test blueprint with terraform components
func createTestBlueprint() *blueprintv1alpha1.Blueprint {
	return &blueprintv1alpha1.Blueprint{
		Metadata: blueprintv1alpha1.Metadata{
			Name: "test-blueprint",
		},
		Sources: []blueprintv1alpha1.Source{
			{
				Name: "source1",
				Url:  "https://github.com/example/example.git",
				Ref:  blueprintv1alpha1.Reference{Branch: "main"},
			},
		},
		TerraformComponents: []blueprintv1alpha1.TerraformComponent{
			{
				Source: "source1",
				Path:   "remote/path",
				Inputs: map[string]any{
					"remote_variable1": "default_value",
				},
			},
			{
				Source: "",
				Path:   "local/path",
				Inputs: map[string]any{
					"local_variable1": "default_value",
				},
			},
		},
	}
}

type Mocks struct {
	Injector         di.Injector
	ConfigHandler    config.ConfigHandler
	Shell            *shell.MockShell
	Blueprint        *blueprint.MockBlueprintHandler
	Shims            *Shims
	Runtime          *runtime.Runtime
	BlueprintHandler blueprint.BlueprintHandler
}

type SetupOptions struct {
	Injector      di.Injector
	ConfigHandler config.ConfigHandler
	ConfigStr     string
}

// setupMocks creates mock components for testing the stack
func setupMocks(t *testing.T, opts ...*SetupOptions) *Mocks {
	t.Helper()

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}

	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to change to temp directory: %v", err)
	}

	os.Setenv("WINDSOR_PROJECT_ROOT", tmpDir)

	options := &SetupOptions{}
	if len(opts) > 0 && opts[0] != nil {
		options = opts[0]
	}

	var injector di.Injector
	if options.Injector == nil {
		injector = di.NewMockInjector()
	} else {
		injector = options.Injector
	}

	mockShell := shell.NewMockShell()

	mockBlueprint := blueprint.NewMockBlueprintHandler(injector)
	mockBlueprint.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
		return []blueprintv1alpha1.TerraformComponent{
			{
				Source:   "git::https://github.com/terraform-aws-modules/terraform-aws-vpc.git//terraform/remote/path@v1.0.0",
				Path:     "remote/path",
				FullPath: filepath.Join(tmpDir, ".windsor", ".tf_modules", "remote", "path"),
				Inputs: map[string]any{
					"remote_variable1": "default_value",
				},
			},
			{
				Source:   "",
				Path:     "local/path",
				FullPath: filepath.Join(tmpDir, "terraform", "local", "path"),
				Inputs: map[string]any{
					"local_variable1": "default_value",
				},
			},
		}
	}

	injector.Register("shell", mockShell)
	injector.Register("blueprintHandler", mockBlueprint)

	var configHandler config.ConfigHandler
	if options.ConfigHandler == nil {
		configHandler = config.NewConfigHandler(mockShell)
	} else {
		configHandler = options.ConfigHandler
	}

	if err := configHandler.SetContext("mock-context"); err != nil {
		t.Fatalf("Failed to set context: %v", err)
	}

	defaultConfigStr := `
contexts:
  mock-context:
    dns:
      domain: mock.domain.com`

	if err := configHandler.LoadConfigString(defaultConfigStr); err != nil {
		t.Fatalf("Failed to load default config string: %v", err)
	}
	if options.ConfigStr != "" {
		if err := configHandler.LoadConfigString(options.ConfigStr); err != nil {
			t.Fatalf("Failed to load config string: %v", err)
		}
	}

	injector.Register("configHandler", configHandler)

	shims := &Shims{}

	shims.Stat = func(path string) (os.FileInfo, error) {
		return nil, nil
	}
	shims.Chdir = func(_ string) error {
		return nil
	}
	shims.Getwd = func() (string, error) {
		return tmpDir, nil
	}
	shims.Setenv = func(key, value string) error {
		return os.Setenv(key, value)
	}
	shims.Unsetenv = func(key string) error {
		return os.Unsetenv(key)
	}
	shims.Remove = func(_ string) error {
		return nil
	}

	t.Cleanup(func() {
		os.Unsetenv("WINDSOR_PROJECT_ROOT")
		if err := os.Chdir(origDir); err != nil {
			t.Logf("Warning: Failed to change back to original directory: %v", err)
		}
	})

	rt := &runtime.Runtime{
		ProjectRoot:   tmpDir,
		ConfigRoot:    tmpDir,
		TemplateRoot:  filepath.Join(tmpDir, "contexts", "_template"),
		Injector:      injector,
		ConfigHandler: configHandler,
		Shell:         mockShell,
	}

	return &Mocks{
		Injector:         injector,
		ConfigHandler:    configHandler,
		Shell:            mockShell,
		Blueprint:        mockBlueprint,
		Shims:            shims,
		Runtime:          rt,
		BlueprintHandler: mockBlueprint,
	}
}

// setupWindsorStackMocks creates mock components for testing the WindsorStack
func setupWindsorStackMocks(t *testing.T, opts ...*SetupOptions) *Mocks {
	t.Helper()
	mocks := setupMocks(t, opts...)

	projectRoot := os.Getenv("WINDSOR_PROJECT_ROOT")
	tfModulesDir := filepath.Join(projectRoot, ".windsor", ".tf_modules", "remote", "path")
	if err := os.MkdirAll(tfModulesDir, 0755); err != nil {
		t.Fatalf("Failed to create tf modules directory: %v", err)
	}

	localDir := filepath.Join(projectRoot, "terraform", "local", "path")
	if err := os.MkdirAll(localDir, 0755); err != nil {
		t.Fatalf("Failed to create local directory: %v", err)
	}

	mocks.Shims.Stat = func(path string) (os.FileInfo, error) {
		if path == tfModulesDir || path == localDir {
			return os.Stat(path)
		}
		return nil, nil
	}

	terraformEnv := envvars.NewTerraformEnvPrinter(mocks.Shell, mocks.ConfigHandler)
	mocks.Runtime.EnvPrinters.TerraformEnv = terraformEnv

	return mocks
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestStack_NewStack(t *testing.T) {
	setup := func(t *testing.T) (*BaseStack, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		stack := NewBaseStack(mocks.Runtime, mocks.BlueprintHandler)
		stack.shims = mocks.Shims
		return stack, mocks
	}

	t.Run("Success", func(t *testing.T) {
		stack, _ := setup(t)

		if stack == nil {
			t.Errorf("Expected stack to be non-nil")
		}
	})
}

func TestStack_Initialize(t *testing.T) {
	setup := func(t *testing.T) (*BaseStack, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		stack := NewBaseStack(mocks.Runtime, mocks.BlueprintHandler)
		stack.shims = mocks.Shims
		return stack, mocks
	}

	t.Run("Success", func(t *testing.T) {
		stack, _ := setup(t)

		if stack == nil {
			t.Error("Expected stack to be created")
		}
	})
}

func TestStack_Up(t *testing.T) {
	setup := func(t *testing.T) (*BaseStack, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		stack := NewBaseStack(mocks.Runtime, mocks.BlueprintHandler)
		stack.shims = mocks.Shims
		return stack, mocks
	}

	t.Run("Success", func(t *testing.T) {
		stack, _ := setup(t)

		blueprint := createTestBlueprint()
		if err := stack.Up(blueprint); err != nil {
			t.Errorf("Expected Up to return nil, got %v", err)
		}
	})

	t.Run("UninitializedStack", func(t *testing.T) {
		stack, _ := setup(t)

		blueprint := createTestBlueprint()
		if err := stack.Up(blueprint); err != nil {
			t.Errorf("Expected Up to return nil even without initialization, got %v", err)
		}
	})

	t.Run("NilInjector", func(t *testing.T) {
		mocks := setupMocks(t)
		stack := NewBaseStack(mocks.Runtime, mocks.BlueprintHandler)

		blueprint := createTestBlueprint()
		if err := stack.Up(blueprint); err != nil {
			t.Errorf("Expected Up to return nil even with nil injector, got %v", err)
		}
	})
}

func TestStack_Down(t *testing.T) {
	setup := func(t *testing.T) (*BaseStack, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		stack := NewBaseStack(mocks.Runtime, mocks.BlueprintHandler)
		stack.shims = mocks.Shims
		return stack, mocks
	}

	t.Run("Success", func(t *testing.T) {
		stack, _ := setup(t)

		blueprint := createTestBlueprint()
		if err := stack.Down(blueprint); err != nil {
			t.Errorf("Expected Down to return nil, got %v", err)
		}
	})

	t.Run("UninitializedStack", func(t *testing.T) {
		stack, _ := setup(t)

		blueprint := createTestBlueprint()
		if err := stack.Down(blueprint); err != nil {
			t.Errorf("Expected Down to return nil even without initialization, got %v", err)
		}
	})

	t.Run("NilInjector", func(t *testing.T) {
		mocks := setupMocks(t)
		stack := NewBaseStack(mocks.Runtime, mocks.BlueprintHandler)

		blueprint := createTestBlueprint()
		if err := stack.Down(blueprint); err != nil {
			t.Errorf("Expected Down to return nil even with nil injector, got %v", err)
		}
	})
}

func TestStack_Interface(t *testing.T) {
	t.Run("BaseStackImplementsStack", func(t *testing.T) {
		var _ Stack = (*BaseStack)(nil)
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestWindsorStack_NewWindsorStack(t *testing.T) {
	setup := func(t *testing.T) (*WindsorStack, *Mocks) {
		t.Helper()
		mocks := setupWindsorStackMocks(t)
		stack := NewWindsorStack(mocks.Runtime, mocks.BlueprintHandler)
		return stack, mocks
	}

	t.Run("Success", func(t *testing.T) {
		stack, _ := setup(t)

		if stack == nil {
			t.Errorf("Expected stack to be non-nil")
		}
	})
}

func TestWindsorStack_Up(t *testing.T) {
	setup := func(t *testing.T) (*WindsorStack, *Mocks) {
		t.Helper()
		mocks := setupWindsorStackMocks(t)
		stack := NewWindsorStack(mocks.Runtime, mocks.BlueprintHandler)
		stack.shims = mocks.Shims
		return stack, mocks
	}

	t.Run("Success", func(t *testing.T) {
		stack, _ := setup(t)
		blueprint := createTestBlueprint()

		if err := stack.Up(blueprint); err != nil {
			t.Errorf("Expected Up to return nil, got %v", err)
		}
	})

	t.Run("ErrorGettingCurrentDirectory", func(t *testing.T) {
		stack, mocks := setup(t)
		mocks.Shims.Getwd = func() (string, error) {
			return "", fmt.Errorf("mock error getting current directory")
		}

		blueprint := createTestBlueprint()
		err := stack.Up(blueprint)
		expectedError := "error getting current directory"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorCheckingDirectoryExists", func(t *testing.T) {
		stack, mocks := setup(t)
		mocks.Shims.Stat = func(path string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		blueprint := createTestBlueprint()
		err := stack.Up(blueprint)
		if err == nil {
			t.Fatalf("Expected an error, but got nil")
		}

		expectedError := "directory"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorGeneratingTerraformArgs", func(t *testing.T) {
		stack, mocks := setup(t)
		mocks.ConfigHandler.Set("terraform.backend.type", "unsupported")

		blueprint := createTestBlueprint()
		err := stack.Up(blueprint)
		expectedError := "error generating terraform args"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorRunningTerraformInit", func(t *testing.T) {
		stack, mocks := setup(t)
		mocks.Shell.ExecProgressFunc = func(message string, command string, args ...string) (string, error) {
			if command == "terraform" && len(args) > 0 && strings.HasPrefix(args[0], "-chdir=") && len(args) > 1 && args[1] == "init" {
				return "", fmt.Errorf("mock error running terraform init")
			}
			return "", nil
		}

		blueprint := createTestBlueprint()
		err := stack.Up(blueprint)
		expectedError := "error running terraform init for"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorRunningTerraformPlan", func(t *testing.T) {
		stack, mocks := setup(t)
		mocks.Shell.ExecProgressFunc = func(message string, command string, args ...string) (string, error) {
			if command == "terraform" && len(args) > 0 && strings.HasPrefix(args[0], "-chdir=") && len(args) > 1 && args[1] == "plan" {
				return "", fmt.Errorf("mock error running terraform plan")
			}
			return "", nil
		}

		blueprint := createTestBlueprint()
		err := stack.Up(blueprint)
		expectedError := "error running terraform plan for"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorRunningTerraformApply", func(t *testing.T) {
		stack, mocks := setup(t)
		mocks.Shell.ExecProgressFunc = func(message string, command string, args ...string) (string, error) {
			if command == "terraform" && len(args) > 0 && strings.HasPrefix(args[0], "-chdir=") && len(args) > 1 && args[1] == "apply" {
				return "", fmt.Errorf("mock error running terraform apply")
			}
			return "", nil
		}

		blueprint := createTestBlueprint()
		err := stack.Up(blueprint)
		expectedError := "error running terraform apply for"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
	})
}

func TestWindsorStack_Down(t *testing.T) {
	setup := func(t *testing.T) (*WindsorStack, *Mocks) {
		t.Helper()
		mocks := setupWindsorStackMocks(t)
		stack := NewWindsorStack(mocks.Runtime, mocks.BlueprintHandler)
		stack.shims = mocks.Shims

		mocks.Blueprint.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
			return []blueprintv1alpha1.TerraformComponent{
				{
					Source:   "source1",
					Path:     "module/path1",
					FullPath: filepath.Join(os.Getenv("WINDSOR_PROJECT_ROOT"), ".windsor", ".tf_modules", "remote", "path"),
				},
			}
		}

		return stack, mocks
	}

	t.Run("Success", func(t *testing.T) {
		stack, _ := setup(t)
		blueprint := createTestBlueprint()

		if err := stack.Down(blueprint); err != nil {
			t.Errorf("Expected Down to return nil, got %v", err)
		}
	})

	t.Run("ErrorGettingCurrentDirectory", func(t *testing.T) {
		stack, mocks := setup(t)
		mocks.Shims.Getwd = func() (string, error) {
			return "", fmt.Errorf("mock error getting current directory")
		}

		blueprint := createTestBlueprint()
		err := stack.Down(blueprint)
		expectedError := "error getting current directory"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorCheckingDirectoryExists", func(t *testing.T) {
		stack, mocks := setup(t)
		mocks.Shims.Stat = func(path string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		blueprint := createTestBlueprint()
		err := stack.Down(blueprint)
		if err != nil {
			t.Fatalf("Expected no error when directory doesn't exist, got %v", err)
		}
	})

	t.Run("ErrorGeneratingTerraformArgs", func(t *testing.T) {
		stack, mocks := setup(t)
		mocks.ConfigHandler.Set("terraform.backend.type", "unsupported")

		blueprint := createTestBlueprint()
		err := stack.Down(blueprint)
		expectedError := "error generating terraform args"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorRunningTerraformPlan", func(t *testing.T) {
		stack, mocks := setup(t)
		mocks.Shell.ExecProgressFunc = func(message string, command string, args ...string) (string, error) {
			if command == "terraform" && len(args) > 0 && strings.HasPrefix(args[0], "-chdir=") && len(args) > 1 && args[1] == "plan" {
				return "", fmt.Errorf("mock error running terraform plan")
			}
			return "", nil
		}

		blueprint := createTestBlueprint()
		err := stack.Down(blueprint)
		expectedError := "error running terraform plan destroy for"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("SkipComponentsWithDestroyFalse", func(t *testing.T) {
		stack, mocks := setup(t)

		projectRoot := os.Getenv("WINDSOR_PROJECT_ROOT")
		destroyFalse := false
		blueprint := createTestBlueprint()
		blueprint.TerraformComponents = []blueprintv1alpha1.TerraformComponent{
			{
				Source:   "source1",
				Path:     "module/path1",
				FullPath: filepath.Join(projectRoot, ".windsor", ".tf_modules", "remote", "path1"),
				Destroy:  &destroyFalse,
			},
			{
				Source:   "source2",
				Path:     "module/path2",
				FullPath: filepath.Join(projectRoot, ".windsor", ".tf_modules", "remote", "path2"),
			},
		}

		if err := os.MkdirAll(blueprint.TerraformComponents[0].FullPath, 0755); err != nil {
			t.Fatalf("Failed to create directory: %v", err)
		}
		if err := os.MkdirAll(blueprint.TerraformComponents[1].FullPath, 0755); err != nil {
			t.Fatalf("Failed to create directory: %v", err)
		}

		var terraformCommands []string
		mocks.Shell.ExecProgressFunc = func(message string, command string, args ...string) (string, error) {
			if command == "terraform" && len(args) > 1 {
				terraformCommands = append(terraformCommands, fmt.Sprintf("%s %s", args[0], args[1]))
			}
			return "", nil
		}

		if err := stack.Down(blueprint); err != nil {
			t.Errorf("Expected Down to return nil, got %v", err)
		}

		foundPath1Commands := false
		foundPath2Commands := false

		for _, cmd := range terraformCommands {
			if strings.Contains(cmd, "path1") {
				foundPath1Commands = true
			}
			if strings.Contains(cmd, "path2") {
				foundPath2Commands = true
			}
		}

		if foundPath1Commands {
			t.Errorf("Expected no terraform commands for path1 (destroy: false), but found commands")
		}
		if !foundPath2Commands {
			t.Errorf("Expected terraform commands for path2 (destroy: true), but found none")
		}
	})

	t.Run("ErrorRunningTerraformDestroy", func(t *testing.T) {
		stack, mocks := setup(t)
		mocks.Shell.ExecProgressFunc = func(message string, command string, args ...string) (string, error) {
			if command == "terraform" && len(args) > 0 && strings.HasPrefix(args[0], "-chdir=") && len(args) > 1 && args[1] == "destroy" {
				return "", fmt.Errorf("mock error running terraform destroy")
			}
			return "", nil
		}

		blueprint := createTestBlueprint()
		err := stack.Down(blueprint)
		expectedError := "error running terraform destroy for"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
	})
}
