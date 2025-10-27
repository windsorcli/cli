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
	"github.com/windsorcli/cli/pkg/environment/envvars"
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

	// Register and initialize terraform env printer by default
	terraformEnv := envvars.NewTerraformEnvPrinter(mocks.Injector)
	if err := terraformEnv.Initialize(); err != nil {
		t.Fatalf("Failed to initialize terraform env printer: %v", err)
	}
	mocks.Injector.Register("terraformEnv", terraformEnv)

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

		// And the terraform env should be resolved
		if stack.terraformEnv == nil {
			t.Errorf("Expected terraformEnv to be resolved")
		}
	})

	t.Run("ErrorTerraformEnvNotFound", func(t *testing.T) {
		stack, mocks := setup(t)

		// And the terraformEnv is unregistered
		mocks.Injector.Register("terraformEnv", nil)

		// When a new WindsorStack is initialized
		err := stack.Initialize()

		// Then an error should occur
		if err == nil {
			t.Errorf("Expected Initialize to return an error")
		} else {
			expectedError := "terraformEnv not found in dependency injector"
			if !strings.Contains(err.Error(), expectedError) {
				t.Errorf("Expected error to contain %q, got %q", expectedError, err.Error())
			}
		}
	})

	t.Run("ErrorResolvingTerraformEnv", func(t *testing.T) {
		stack, mocks := setup(t)

		// And a non-terraform env printer is registered with terraformEnv key
		mocks.Injector.Register("terraformEnv", "not-a-terraform-env")

		// When a new WindsorStack is initialized
		err := stack.Initialize()

		// Then an error should occur
		if err == nil {
			t.Errorf("Expected Initialize to return an error")
		} else {
			expectedError := "error resolving terraformEnv"
			if !strings.Contains(err.Error(), expectedError) {
				t.Errorf("Expected error to contain %q, got %q", expectedError, err.Error())
			}
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

	t.Run("ErrorGeneratingTerraformArgs", func(t *testing.T) {
		stack, mocks := setup(t)
		mocks.ConfigHandler.Set("terraform.backend.type", "unsupported")

		// And when Up is called
		err := stack.Up()
		// Then the expected error is contained in err
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

		// And when Up is called
		err := stack.Up()
		// Then the expected error is contained in err
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

		// And when Up is called
		err := stack.Up()
		// Then the expected error is contained in err
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

		// And when Up is called
		err := stack.Up()
		// Then the expected error is contained in err
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
		// Then no error should occur since Down continues when directory doesn't exist
		if err != nil {
			t.Fatalf("Expected no error when directory doesn't exist, got %v", err)
		}
	})

	t.Run("ErrorGeneratingTerraformArgs", func(t *testing.T) {
		stack, mocks := setup(t)
		mocks.ConfigHandler.Set("terraform.backend.type", "unsupported")

		// And when Down is called
		err := stack.Down()
		// Then the expected error is contained in err
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

		// And when Down is called
		err := stack.Down()
		// Then the expected error is contained in err
		expectedError := "error running terraform plan destroy for"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("SkipComponentsWithDestroyFalse", func(t *testing.T) {
		stack, mocks := setup(t)

		// Set up components with one having destroy: false
		destroyFalse := false
		mocks.Blueprint.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
			return []blueprintv1alpha1.TerraformComponent{
				{
					Source:   "source1",
					Path:     "module/path1",
					FullPath: filepath.Join(os.Getenv("WINDSOR_PROJECT_ROOT"), ".windsor", ".tf_modules", "remote", "path1"),
					Destroy:  &destroyFalse, // This component should be skipped
				},
				{
					Source:   "source2",
					Path:     "module/path2",
					FullPath: filepath.Join(os.Getenv("WINDSOR_PROJECT_ROOT"), ".windsor", ".tf_modules", "remote", "path2"),
					// Destroy defaults to true, so this should be destroyed
				},
			}
		}

		// Track terraform commands executed
		var terraformCommands []string
		mocks.Shell.ExecProgressFunc = func(message string, command string, args ...string) (string, error) {
			if command == "terraform" && len(args) > 1 {
				terraformCommands = append(terraformCommands, fmt.Sprintf("%s %s", args[0], args[1]))
			}
			return "", nil
		}

		// When Down is called
		if err := stack.Down(); err != nil {
			t.Errorf("Expected Down to return nil, got %v", err)
		}

		// Then only the component without destroy: false should be destroyed
		// We should see terraform commands for path2 but not path1
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

		// And when Down is called
		err := stack.Down()
		// Then the expected error is contained in err
		expectedError := "error running terraform destroy for"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
	})
}
