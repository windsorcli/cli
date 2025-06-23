package generators

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/blueprint"
	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
)

// =============================================================================
// Test Setup
// =============================================================================

// Mocks holds all mock dependencies for testing
type Mocks struct {
	Injector         di.Injector
	ConfigHandler    config.ConfigHandler
	BlueprintHandler blueprint.MockBlueprintHandler
	Shell            *shell.MockShell
	Shims            *Shims
}

// SetupOptions configures test setup behavior
type SetupOptions struct {
	Injector      di.Injector
	ConfigHandler config.ConfigHandler
	ConfigStr     string
}

// =============================================================================
// Test Setup Functions
// =============================================================================

// setupMocks creates mock dependencies for testing
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

	options := &SetupOptions{}
	if len(opts) > 0 && opts[0] != nil {
		options = opts[0]
	}

	// Create a new injector
	var injector di.Injector
	if options.Injector == nil {
		injector = di.NewMockInjector()
	} else {
		injector = options.Injector
	}

	// Create a new config handler
	var configHandler config.ConfigHandler
	if options.ConfigHandler == nil {
		configHandler = config.NewMockConfigHandler()
		configHandler.(*config.MockConfigHandler).GetConfigRootFunc = func() (string, error) {
			return filepath.Join(tmpDir, "contexts", "default"), nil
		}
	} else {
		configHandler = options.ConfigHandler
	}
	injector.Register("configHandler", configHandler)

	// Create a new mock shell
	mockShell := shell.NewMockShell()
	mockShell.GetProjectRootFunc = func() (string, error) {
		return tmpDir, nil
	}
	mockShell.ExecProgressFunc = func(msg string, cmd string, args ...string) (string, error) {
		if cmd == "terraform" && len(args) > 0 && args[0] == "init" {
			return `{"@level":"info","@message":"Initializing modules...","@module":"terraform.ui","@timestamp":"2025-05-09T16:25:03Z","message_code":"initializing_modules_message","type":"init_output"}
{"@level":"info","@message":"- main in /path/to/module","@module":"terraform.ui","@timestamp":"2025-05-09T12:25:04.557548-04:00","type":"log"}`, nil
		}
		return "", nil
	}
	injector.Register("shell", mockShell)

	// Create a new mock blueprint handler
	mockBlueprintHandler := blueprint.NewMockBlueprintHandler(injector)
	injector.Register("blueprintHandler", mockBlueprintHandler)

	// Mock the GetTerraformComponents method
	mockBlueprintHandler.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
		// Common components setup
		remoteComponent := blueprintv1alpha1.TerraformComponent{
			Source:   "git::https://github.com/terraform-aws-modules/terraform-aws-vpc.git//terraform/remote/path@v1.0.0",
			Path:     "remote/path",
			FullPath: filepath.Join(tmpDir, ".windsor", ".tf_modules", "remote/path"),
			Values: map[string]any{
				"remote_variable1": "default_value",
			},
		}

		localComponent := blueprintv1alpha1.TerraformComponent{
			Source:   "local/path",
			Path:     "local/path",
			FullPath: filepath.Join(tmpDir, ".windsor", ".tf_modules", "local/path"),
			Values: map[string]any{
				"local_variable1": "default_value",
			},
		}

		return []blueprintv1alpha1.TerraformComponent{remoteComponent, localComponent}
	}

	// Set project root environment variable
	os.Setenv("WINDSOR_PROJECT_ROOT", tmpDir)

	// Register cleanup to restore original state
	t.Cleanup(func() {
		os.Unsetenv("WINDSOR_PROJECT_ROOT")
		if err := os.Chdir(origDir); err != nil {
			t.Logf("Warning: Failed to change back to original directory: %v", err)
		}
	})

	// Create shims with mock implementations
	shims := NewShims()
	shims.WriteFile = func(path string, data []byte, perm fs.FileMode) error {
		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
		return os.WriteFile(path, data, perm)
	}
	shims.MkdirAll = func(path string, perm fs.FileMode) error {
		return os.MkdirAll(path, perm)
	}
	shims.TempDir = func(_, _ string) (string, error) {
		return t.TempDir(), nil
	}
	shims.RemoveAll = func(path string) error {
		return os.RemoveAll(path)
	}
	shims.Chdir = func(path string) error {
		return os.Chdir(path)
	}
	shims.Stat = func(path string) (fs.FileInfo, error) {
		return os.Stat(path)
	}
	shims.Setenv = func(key, value string) error {
		return os.Setenv(key, value)
	}
	shims.ReadFile = func(path string) ([]byte, error) {
		// Handle variables.tf
		if strings.HasSuffix(path, "variables.tf") {
			return []byte(`variable "remote_variable1" {
  description = "Remote variable 1"
  type        = string
  default     = "default_value"
}

variable "local_variable1" {
  description = "Local variable 1"
  type        = string
  default     = "default_value"
}`), nil
		}

		// Handle tfvars files
		if strings.HasSuffix(path, ".tfvars") {
			return []byte(`# Managed by Windsor CLI
remote_variable1 = "default_value"
local_variable1 = "default_value"`), nil
		}

		// Handle outputs.tf
		if strings.HasSuffix(path, "outputs.tf") {
			return []byte(`output "remote_output1" {
  value       = "remote_value1"
  description = "Remote output 1"
}

output "local_output1" {
  value       = "local_value1"
  description = "Local output 1"
}`), nil
		}

		return []byte{}, nil
	}
	shims.JsonUnmarshal = func(data []byte, v any) error {
		return json.Unmarshal(data, v)
	}
	shims.FilepathRel = func(basepath, targpath string) (string, error) {
		return filepath.Rel(basepath, targpath)
	}

	configHandler.Initialize()

	// Create base mocks
	mocks := &Mocks{
		Injector:         injector,
		ConfigHandler:    configHandler,
		BlueprintHandler: *mockBlueprintHandler,
		Shell:            mockShell,
		Shims:            shims,
	}

	return mocks
}

// =============================================================================
// Test Constructor
// =============================================================================

func TestGenerator_NewGenerator(t *testing.T) {
	mocks := setupMocks(t)
	generator := NewGenerator(mocks.Injector)

	if generator == nil {
		t.Errorf("Expected generator to be non-nil")
	}
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestGenerator_Initialize(t *testing.T) {
	setup := func(t *testing.T) (*BaseGenerator, *Mocks) {
		mocks := setupMocks(t)
		generator := NewGenerator(mocks.Injector)
		generator.shims = mocks.Shims

		return generator, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a set of safe mocks
		generator, _ := setup(t)

		// And the BaseGenerator is initialized
		err := generator.Initialize()

		// Then the initialization should succeed
		if err != nil {
			t.Errorf("Expected Initialize to succeed, but got error: %v", err)
		}
	})

	t.Run("ErrorResolvingBlueprintHandler", func(t *testing.T) {
		// Given a set of safe mocks
		generator, mocks := setup(t)

		// And a mock injector with a nil blueprint handler
		mocks.Injector.Register("blueprintHandler", nil)

		// And the BaseGenerator is initialized
		err := generator.Initialize()

		// Then the initialization should fail
		if err == nil {
			t.Errorf("Expected Initialize to fail, but it succeeded")
		}
	})

	t.Run("ErrorResolvingShell", func(t *testing.T) {
		// Given a set of safe mocks
		generator, mocks := setup(t)

		// And a mock injector with a nil shell
		mocks.Injector.Register("shell", nil)

		// When the BaseGenerator is initialized
		err := generator.Initialize()

		// Then the initialization should fail
		if err == nil {
			t.Errorf("Expected Initialize to fail, but it succeeded")
		}
	})

	t.Run("ErrorResolvingConfigHandler", func(t *testing.T) {
		// Given a set of mocks
		generator, mocks := setup(t)

		// And a mock injector with a nil config handler
		mocks.Injector.Register("configHandler", nil)

		// When the BaseGenerator is initialized
		err := generator.Initialize()

		// Then the initialization should fail
		if err == nil {
			t.Errorf("Expected Initialize to fail, but it succeeded")
		}

		// And the error should match the expected error
		expectedError := "failed to resolve config handler"
		if err.Error() != expectedError {
			t.Errorf("expected error %s, got %s", expectedError, err.Error())
		}
	})
}

func TestGenerator_Write(t *testing.T) {
	setup := func(t *testing.T) (*BaseGenerator, *Mocks) {
		mocks := setupMocks(t)
		generator := NewGenerator(mocks.Injector)
		generator.shims = mocks.Shims
		generator.Initialize()

		return generator, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a set of safe mocks
		generator, _ := setup(t)

		// When the Write method is called
		err := generator.Write()

		// Then the Write method should succeed
		if err != nil {
			t.Errorf("Expected Write to succeed, but got error: %v", err)
		}
	})
}
