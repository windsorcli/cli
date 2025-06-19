package stack

// The WindsorStackTest provides comprehensive test coverage for the WindsorStack implementation.
// It provides validation of stack initialization, component management, and infrastructure operations,
// The WindsorStackTest ensures proper dependency injection and component lifecycle management,
// verifying error handling, mock interactions, and infrastructure state management.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/env"
)

// =============================================================================
// Test Setup
// =============================================================================

// setupWindsorStackMocks creates mock components for testing the WindsorStack
func setupWindsorStackMocks(t *testing.T, opts ...*SetupOptions) *Mocks {
	t.Helper()
	mocks := setupMocks(t, opts...)

	// Create necessary directories for tests
	projectRoot := os.Getenv("WINDSOR_PROJECT_ROOT")
	tfModulesDir := filepath.Join(projectRoot, ".windsor", ".tf_modules", "remote", "path")
	if err := os.MkdirAll(tfModulesDir, 0755); err != nil {
		t.Fatalf("Failed to create tf modules directory: %v", err)
	}

	localDir := filepath.Join(projectRoot, "terraform", "local", "path")
	if err := os.MkdirAll(localDir, 0755); err != nil {
		t.Fatalf("Failed to create local directory: %v", err)
	}

	// Update shims to handle Windsor-specific paths
	mocks.Shims.Stat = func(path string) (os.FileInfo, error) {
		// Return success for both directories
		if path == tfModulesDir || path == localDir {
			return os.Stat(path)
		}
		return nil, nil
	}

	return mocks
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestWindsorStack_NewWindsorStack(t *testing.T) {
	setup := func(t *testing.T) (*WindsorStack, *Mocks) {
		t.Helper()
		mocks := setupWindsorStackMocks(t)
		stack := NewWindsorStack(mocks.Injector)
		return stack, mocks
	}

	t.Run("Success", func(t *testing.T) {
		stack, _ := setup(t)

		// Then the stack should be non-nil
		if stack == nil {
			t.Errorf("Expected stack to be non-nil")
		}
	})
}

func TestWindsorStack_Initialize(t *testing.T) {
	setup := func(t *testing.T) (*WindsorStack, *Mocks) {
		t.Helper()
		mocks := setupWindsorStackMocks(t)
		stack := NewWindsorStack(mocks.Injector)
		return stack, mocks
	}

	t.Run("Success", func(t *testing.T) {
		stack, _ := setup(t)

		// When a new WindsorStack is initialized
		if err := stack.Initialize(); err != nil {
			// Then no error should occur
			t.Errorf("Expected Initialize to return nil, got %v", err)
		}
	})

	t.Run("ErrorResolvingShell", func(t *testing.T) {
		stack, mocks := setup(t)

		// And the shell is unregistered to simulate an error
		mocks.Injector.Register("shell", nil)

		// When a new WindsorStack is initialized
		err := stack.Initialize()

		// Then an error should occur
		if err == nil {
			t.Errorf("Expected Initialize to return an error")
		} else {
			expectedError := "error resolving shell"
			if !strings.Contains(err.Error(), expectedError) {
				t.Errorf("Expected error to contain %q, got %q", expectedError, err.Error())
			}
		}
	})

	t.Run("ErrorResolvingBlueprintHandler", func(t *testing.T) {
		stack, mocks := setup(t)

		// And the blueprintHandler is unregistered to simulate an error
		mocks.Injector.Register("blueprintHandler", nil)

		// Then an error should occur
		if err := stack.Initialize(); err == nil {
			t.Errorf("Expected Initialize to return an error")
		}
	})

	t.Run("ErrorResolvingEnvPrinters", func(t *testing.T) {
		// Given safe mock components with a resolve all error
		mockInjector := di.NewMockInjector()
		mockInjector.SetResolveAllError((*env.EnvPrinter)(nil), fmt.Errorf("mock error resolving envPrinters"))
		opts := &SetupOptions{
			Injector: mockInjector,
		}
		mocks := setupWindsorStackMocks(t, opts)
		stack := NewWindsorStack(mocks.Injector)

		// When a new WindsorStack is initialized
		err := stack.Initialize()

		// Then an error should occur
		if err == nil {
			t.Errorf("Expected Initialize to return an error")
		} else {
			expectedError := "error resolving envPrinters"
			if !strings.Contains(err.Error(), expectedError) {
				t.Errorf("Expected error to contain %q, got %q", expectedError, err.Error())
			}
		}
	})
}

func TestWindsorStack_Up(t *testing.T) {
	setup := func(t *testing.T) (*WindsorStack, *Mocks) {
		t.Helper()
		mocks := setupWindsorStackMocks(t)
		stack := NewWindsorStack(mocks.Injector)
		stack.shims = mocks.Shims
		if err := stack.Initialize(); err != nil {
			t.Fatalf("Expected no error during initialization, got %v", err)
		}
		return stack, mocks
	}

	t.Run("Success", func(t *testing.T) {
		stack, _ := setup(t)

		// And when the stack is brought up
		if err := stack.Up(); err != nil {
			// Then no error should occur
			t.Errorf("Expected Up to return nil, got %v", err)
		}
	})

	t.Run("ErrorGettingCurrentDirectory", func(t *testing.T) {
		stack, mocks := setup(t)
		mocks.Shims.Getwd = func() (string, error) {
			return "", fmt.Errorf("mock error getting current directory")
		}

		// And when Up is called
		err := stack.Up()
		// Then the expected error is contained in err
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

		// And when Up is called
		err := stack.Up()
		if err == nil {
			t.Fatalf("Expected an error, but got nil")
		}

		// Then the expected error is contained in err
		expectedError := "directory"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorChangingDirectory", func(t *testing.T) {
		stack, mocks := setup(t)
		mocks.Shims.Chdir = func(_ string) error {
			return fmt.Errorf("mock error changing directory")
		}

		// And when Up is called
		err := stack.Up()
		// Then the expected error is contained in err
		expectedError := "error changing to directory"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorGettingEnvVars", func(t *testing.T) {
		stack, mocks := setup(t)
		mocks.EnvPrinter.GetEnvVarsFunc = func() (map[string]string, error) {
			return nil, fmt.Errorf("mock error getting environment variables")
		}

		// And when Up is called
		err := stack.Up()
		// Then the expected error is contained in err
		expectedError := "error getting environment variables"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorSettingEnvVars", func(t *testing.T) {
		stack, mocks := setup(t)
		mocks.Shims.Setenv = func(_ string, _ string) error {
			return fmt.Errorf("mock error setting environment variable")
		}

		// And when Up is called
		err := stack.Up()
		// Then the expected error is contained in err
		expectedError := "error setting environment variable"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorRunningPostEnvHook", func(t *testing.T) {
		stack, mocks := setup(t)
		mocks.EnvPrinter.PostEnvHookFunc = func() error {
			return fmt.Errorf("mock error running post environment hook")
		}

		// And when Up is called
		err := stack.Up()
		// Then the expected error is contained in err
		expectedError := "error running post environment hook"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorRunningTerraformInit", func(t *testing.T) {
		stack, mocks := setup(t)
		mocks.Shell.ExecProgressFunc = func(message string, command string, args ...string) (string, error) {
			if command == "terraform" && len(args) > 0 && args[0] == "init" {
				return "", fmt.Errorf("mock error running terraform init")
			}
			return "", nil
		}

		// And when Up is called
		err := stack.Up()
		// Then the expected error is contained in err
		expectedError := "error initializing Terraform"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorRunningTerraformPlan", func(t *testing.T) {
		stack, mocks := setup(t)
		mocks.Shell.ExecProgressFunc = func(message string, command string, args ...string) (string, error) {
			if command == "terraform" && len(args) > 0 && args[0] == "plan" {
				return "", fmt.Errorf("mock error running terraform plan")
			}
			return "", nil
		}

		// And when Up is called
		err := stack.Up()
		// Then the expected error is contained in err
		expectedError := "error planning Terraform changes"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorRunningTerraformApply", func(t *testing.T) {
		stack, mocks := setup(t)
		mocks.Shell.ExecProgressFunc = func(message string, command string, args ...string) (string, error) {
			if command == "terraform" && len(args) > 0 && args[0] == "apply" {
				return "", fmt.Errorf("mock error running terraform apply")
			}
			return "", nil
		}

		// And when Up is called
		err := stack.Up()
		// Then the expected error is contained in err
		expectedError := "error applying Terraform changes"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorRemovingBackendOverride", func(t *testing.T) {
		stack, mocks := setup(t)
		mocks.Shims.Remove = func(_ string) error {
			return fmt.Errorf("mock error removing backend override")
		}

		// And when Up is called
		err := stack.Up()
		// Then the expected error is contained in err
		expectedError := "error removing backend_override.tf"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("SuccessWithParallelism", func(t *testing.T) {
		stack, mocks := setup(t)

		// Set up components with parallelism
		mocks.Blueprint.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
			return []blueprintv1alpha1.TerraformComponent{
				{
					Source:      "source1",
					Path:        "module/path1",
					FullPath:    filepath.Join(os.Getenv("WINDSOR_PROJECT_ROOT"), ".windsor", ".tf_modules", "remote", "path"),
					Parallelism: ptrInt(5),
				},
			}
		}

		// Track terraform commands to verify parallelism flag
		var capturedCommands [][]string
		mocks.Shell.ExecProgressFunc = func(message string, command string, args ...string) (string, error) {
			if command == "terraform" {
				capturedCommands = append(capturedCommands, append([]string{command}, args...))
			}
			return "", nil
		}

		// When the stack is brought up
		if err := stack.Up(); err != nil {
			t.Errorf("Expected Up to return nil, got %v", err)
		}

		// Then terraform apply should be called with parallelism flag
		foundApplyWithParallelism := false
		for _, cmd := range capturedCommands {
			if len(cmd) >= 3 && cmd[1] == "apply" && cmd[2] == "-parallelism=5" {
				foundApplyWithParallelism = true
				break
			}
		}
		if !foundApplyWithParallelism {
			t.Errorf("Expected terraform apply command with -parallelism=5, but it was not found in captured commands: %v", capturedCommands)
		}
	})
}

func TestWindsorStack_Down(t *testing.T) {
	setup := func(t *testing.T) (*WindsorStack, *Mocks) {
		t.Helper()
		mocks := setupWindsorStackMocks(t)
		stack := NewWindsorStack(mocks.Injector)
		stack.shims = mocks.Shims
		if err := stack.Initialize(); err != nil {
			t.Fatalf("Expected no error during initialization, got %v", err)
		}

		// Set up default components for the blueprint handler
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

		// And when the stack is brought down
		if err := stack.Down(); err != nil {
			// Then no error should occur
			t.Errorf("Expected Down to return nil, got %v", err)
		}
	})

	t.Run("ErrorGettingCurrentDirectory", func(t *testing.T) {
		stack, mocks := setup(t)
		mocks.Shims.Getwd = func() (string, error) {
			return "", fmt.Errorf("mock error getting current directory")
		}

		// And when Down is called
		err := stack.Down()
		// Then the expected error is contained in err
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

		// And when Down is called
		err := stack.Down()
		if err == nil {
			t.Fatalf("Expected an error, but got nil")
		}

		// Then the expected error is contained in err
		expectedError := "directory"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorChangingDirectory", func(t *testing.T) {
		stack, mocks := setup(t)
		mocks.Shims.Chdir = func(_ string) error {
			return fmt.Errorf("mock error changing directory")
		}

		// And when Down is called
		err := stack.Down()
		// Then the expected error is contained in err
		expectedError := "error changing to directory"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorGettingEnvVars", func(t *testing.T) {
		stack, mocks := setup(t)
		mocks.EnvPrinter.GetEnvVarsFunc = func() (map[string]string, error) {
			return nil, fmt.Errorf("mock error getting environment variables")
		}

		// And when Down is called
		err := stack.Down()
		// Then the expected error is contained in err
		expectedError := "error getting environment variables"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorSettingEnvVars", func(t *testing.T) {
		stack, mocks := setup(t)
		mocks.Shims.Setenv = func(_ string, _ string) error {
			return fmt.Errorf("mock error setting environment variable")
		}

		// And when Down is called
		err := stack.Down()
		// Then the expected error is contained in err
		expectedError := "error setting environment variable"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorRunningPostEnvHook", func(t *testing.T) {
		stack, mocks := setup(t)
		mocks.EnvPrinter.PostEnvHookFunc = func() error {
			return fmt.Errorf("mock error running post environment hook")
		}

		// And when Down is called
		err := stack.Down()
		// Then the expected error is contained in err
		expectedError := "error running post environment hook"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorRunningTerraformInit", func(t *testing.T) {
		stack, mocks := setup(t)
		mocks.Shell.ExecProgressFunc = func(message string, command string, args ...string) (string, error) {
			if command == "terraform" && len(args) > 0 && args[0] == "init" {
				return "", fmt.Errorf("mock error running terraform init")
			}
			return "", nil
		}

		// And when Down is called
		err := stack.Down()
		// Then the expected error is contained in err
		expectedError := "error initializing Terraform"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorRunningTerraformPlanDestroy", func(t *testing.T) {
		stack, mocks := setup(t)
		mocks.Shell.ExecProgressFunc = func(message string, command string, args ...string) (string, error) {
			if command == "terraform" && len(args) > 0 && args[0] == "plan" && len(args) > 1 && args[1] == "-destroy" {
				return "", fmt.Errorf("mock error running terraform plan -destroy")
			}
			return "", nil
		}

		// And when Down is called
		err := stack.Down()
		// Then the expected error is contained in err
		expectedError := "error planning Terraform destruction"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorRunningTerraformDestroy", func(t *testing.T) {
		stack, mocks := setup(t)
		mocks.Shell.ExecProgressFunc = func(message string, command string, args ...string) (string, error) {
			if command == "terraform" && len(args) > 0 && args[0] == "destroy" {
				return "", fmt.Errorf("mock error running terraform destroy")
			}
			return "", nil
		}

		// And when Down is called
		err := stack.Down()
		// Then the expected error is contained in err
		expectedError := "error destroying Terraform resources"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorRemovingBackendOverride", func(t *testing.T) {
		stack, mocks := setup(t)
		mocks.Shims.Remove = func(_ string) error {
			return fmt.Errorf("mock error removing backend override")
		}

		// And when Down is called
		err := stack.Down()
		// Then the expected error is contained in err
		expectedError := "error removing backend_override.tf"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("SuccessWithParallelism", func(t *testing.T) {
		stack, mocks := setup(t)

		// Set up components with parallelism
		mocks.Blueprint.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
			return []blueprintv1alpha1.TerraformComponent{
				{
					Source:      "source1",
					Path:        "module/path1",
					FullPath:    filepath.Join(os.Getenv("WINDSOR_PROJECT_ROOT"), ".windsor", ".tf_modules", "remote", "path"),
					Parallelism: ptrInt(3),
				},
			}
		}

		// Track terraform commands to verify parallelism flag
		var capturedCommands [][]string
		mocks.Shell.ExecProgressFunc = func(message string, command string, args ...string) (string, error) {
			if command == "terraform" {
				capturedCommands = append(capturedCommands, append([]string{command}, args...))
			}
			return "", nil
		}

		// When the stack is brought down
		if err := stack.Down(); err != nil {
			t.Errorf("Expected Down to return nil, got %v", err)
		}

		// Then terraform destroy should be called with parallelism flag
		foundDestroyWithParallelism := false
		for _, cmd := range capturedCommands {
			if len(cmd) >= 4 && cmd[1] == "destroy" && cmd[2] == "-auto-approve" && cmd[3] == "-parallelism=3" {
				foundDestroyWithParallelism = true
				break
			}
		}
		if !foundDestroyWithParallelism {
			t.Errorf("Expected terraform destroy command with -parallelism=3, but it was not found in captured commands: %v", capturedCommands)
		}
	})

	t.Run("SkipComponentWithDestroyFalse", func(t *testing.T) {
		stack, mocks := setup(t)
		mocks.Blueprint.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
			return []blueprintv1alpha1.TerraformComponent{
				{
					Source:   "source1",
					Path:     "module/path1",
					FullPath: filepath.Join(os.Getenv("WINDSOR_PROJECT_ROOT"), ".windsor", ".tf_modules", "remote", "path"),
					Destroy:  ptrBool(false),
				},
			}
		}

		// And when Down is called
		err := stack.Down()
		// Then no error should occur
		if err != nil {
			t.Errorf("Expected Down to return nil, got %v", err)
		}
	})
}

// Helper functions to create pointers for basic types
func ptrBool(b bool) *bool {
	return &b
}

func ptrInt(i int) *int {
	return &i
}
