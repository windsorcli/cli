package terraform

// The StackTest provides comprehensive test coverage for the Stack interface implementation.
// It provides validation of stack initialization, component management, and infrastructure operations,
// The StackTest ensures proper dependency injection and component lifecycle management,
// verifying error handling, mock interactions, and infrastructure state management.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/resources/blueprint"
	"github.com/windsorcli/cli/pkg/shell"
)

// =============================================================================
// Test Setup
// =============================================================================

type Mocks struct {
	Injector      di.Injector
	ConfigHandler config.ConfigHandler
	Shell         *shell.MockShell
	Blueprint     *blueprint.MockBlueprintHandler
	Shims         *Shims
}

type SetupOptions struct {
	Injector      di.Injector
	ConfigHandler config.ConfigHandler
	ConfigStr     string
}

// setupMocks creates mock components for testing the stack
func setupMocks(t *testing.T, opts ...*SetupOptions) *Mocks {
	t.Helper()

	// Store original directory and create temp dir
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}

	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to change to temp directory: %v", err)
	}

	// Set project root environment variable
	os.Setenv("WINDSOR_PROJECT_ROOT", tmpDir)

	// Process options with defaults
	options := &SetupOptions{}
	if len(opts) > 0 && opts[0] != nil {
		options = opts[0]
	}

	// Create injector
	var injector di.Injector
	if options.Injector == nil {
		injector = di.NewMockInjector()
	} else {
		injector = options.Injector
	}

	// Create mock shell
	mockShell := shell.NewMockShell()

	// Create mock blueprint handler
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

	// Register dependencies
	injector.Register("shell", mockShell)
	injector.Register("blueprintHandler", mockBlueprint)

	// Create config handler
	var configHandler config.ConfigHandler
	if options.ConfigHandler == nil {
		configHandler = config.NewConfigHandler(injector)
	} else {
		configHandler = options.ConfigHandler
	}

	// Initialize config handler
	if err := configHandler.Initialize(); err != nil {
		t.Fatalf("Failed to initialize config handler: %v", err)
	}
	if err := configHandler.SetContext("mock-context"); err != nil {
		t.Fatalf("Failed to set context: %v", err)
	}

	// Load default config string
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

	// Register config handler
	injector.Register("configHandler", configHandler)

	// Mock system calls
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

	// Register cleanup to restore original state
	t.Cleanup(func() {
		os.Unsetenv("WINDSOR_PROJECT_ROOT")
		if err := os.Chdir(origDir); err != nil {
			t.Logf("Warning: Failed to change back to original directory: %v", err)
		}
	})

	return &Mocks{
		Injector:      injector,
		ConfigHandler: configHandler,
		Shell:         mockShell,
		Blueprint:     mockBlueprint,
		Shims:         shims,
	}
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestStack_NewStack(t *testing.T) {
	setup := func(t *testing.T) (*BaseStack, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		stack := NewBaseStack(mocks.Injector)
		stack.shims = mocks.Shims
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

func TestStack_Initialize(t *testing.T) {
	setup := func(t *testing.T) (*BaseStack, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		stack := NewBaseStack(mocks.Injector)
		stack.shims = mocks.Shims
		return stack, mocks
	}

	t.Run("Success", func(t *testing.T) {
		stack, _ := setup(t)

		// When a new BaseStack is initialized
		if err := stack.Initialize(); err != nil {
			// Then no error should occur
			t.Errorf("Expected Initialize to return nil, got %v", err)
		}
	})

	t.Run("ErrorResolvingShell", func(t *testing.T) {
		// Given safe mock components
		mocks := setupMocks(t)

		// And the shell is unregistered to simulate an error
		mocks.Injector.Register("shell", nil)

		// When a new BaseStack is initialized
		stack := NewBaseStack(mocks.Injector)
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
		// Given safe mock components
		mocks := setupMocks(t)

		// And the blueprintHandler is unregistered to simulate an error
		mocks.Injector.Register("blueprintHandler", nil)

		// When a new BaseStack is initialized
		stack := NewBaseStack(mocks.Injector)

		// Then an error should occur
		if err := stack.Initialize(); err == nil {
			t.Errorf("Expected Initialize to return an error")
		}
	})
}

func TestStack_Up(t *testing.T) {
	setup := func(t *testing.T) (*BaseStack, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		stack := NewBaseStack(mocks.Injector)
		stack.shims = mocks.Shims
		return stack, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given safe mock components
		stack, _ := setup(t)

		// When a new BaseStack is created and initialized
		if err := stack.Initialize(); err != nil {
			t.Fatalf("Expected no error during initialization, got %v", err)
		}

		// And when Up is called
		if err := stack.Up(); err != nil {
			// Then no error should occur
			t.Errorf("Expected Up to return nil, got %v", err)
		}
	})

	t.Run("UninitializedStack", func(t *testing.T) {
		// Given a new BaseStack without initialization
		stack, _ := setup(t)

		// When Up is called without initializing
		if err := stack.Up(); err != nil {
			// Then no error should occur since base implementation is empty
			t.Errorf("Expected Up to return nil even without initialization, got %v", err)
		}
	})

	t.Run("NilInjector", func(t *testing.T) {
		// Given a BaseStack with nil injector
		stack := NewBaseStack(nil)

		// When Up is called
		if err := stack.Up(); err != nil {
			// Then no error should occur since base implementation is empty
			t.Errorf("Expected Up to return nil even with nil injector, got %v", err)
		}
	})
}

func TestStack_Down(t *testing.T) {
	setup := func(t *testing.T) (*BaseStack, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		stack := NewBaseStack(mocks.Injector)
		stack.shims = mocks.Shims
		return stack, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given safe mock components
		stack, _ := setup(t)

		// When a new BaseStack is created and initialized
		if err := stack.Initialize(); err != nil {
			t.Fatalf("Expected no error during initialization, got %v", err)
		}

		// And when Down is called
		if err := stack.Down(); err != nil {
			// Then no error should occur
			t.Errorf("Expected Down to return nil, got %v", err)
		}
	})

	t.Run("UninitializedStack", func(t *testing.T) {
		// Given a new BaseStack without initialization
		stack, _ := setup(t)

		// When Down is called without initializing
		if err := stack.Down(); err != nil {
			// Then no error should occur since base implementation is empty
			t.Errorf("Expected Down to return nil even without initialization, got %v", err)
		}
	})

	t.Run("NilInjector", func(t *testing.T) {
		// Given a BaseStack with nil injector
		stack := NewBaseStack(nil)

		// When Down is called
		if err := stack.Down(); err != nil {
			// Then no error should occur since base implementation is empty
			t.Errorf("Expected Down to return nil even with nil injector, got %v", err)
		}
	})
}

func TestStack_Interface(t *testing.T) {
	t.Run("BaseStackImplementsStack", func(t *testing.T) {
		// Given a type assertion for Stack interface
		var _ Stack = (*BaseStack)(nil)

		// Then the code should compile, indicating BaseStack implements Stack
	})
}
