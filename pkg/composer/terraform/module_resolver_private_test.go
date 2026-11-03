package terraform

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/composer/blueprint"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/evaluator"
	"github.com/windsorcli/cli/pkg/runtime/shell"
	"github.com/windsorcli/cli/pkg/runtime/tools"
)

// =============================================================================
// Test Setup
// =============================================================================

// TerraformTestMocks contains all the mock dependencies for testing terraform resolvers
type TerraformTestMocks struct {
	Shell            *shell.MockShell
	ConfigHandler    config.ConfigHandler
	BlueprintHandler *blueprint.MockBlueprintHandler
	Shims            *Shims
	Runtime          *runtime.Runtime
}

// setupTerraformMocks creates mock components for testing terraform resolvers with optional overrides
func setupTerraformMocks(t *testing.T, opts ...func(*TerraformTestMocks)) *TerraformTestMocks {
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

	t.Setenv("WINDSOR_PROJECT_ROOT", tmpDir)

	t.Cleanup(func() {
		os.Unsetenv("WINDSOR_PROJECT_ROOT")
		if err := os.Chdir(origDir); err != nil {
			t.Logf("Warning: Failed to change back to original directory: %v", err)
		}
	})

	mockShell := shell.NewMockShell()
	mockShell.GetProjectRootFunc = func() (string, error) {
		return tmpDir, nil
	}
	mockShell.ExecProgressFunc = func(msg, cmd string, args ...string) (string, error) {
		if cmd == "terraform" && len(args) > 0 && args[0] == "init" {
			return `{"@level":"info","@message":"Initializing modules...","@module":"terraform.ui","@timestamp":"2025-01-09T16:25:03Z","type":"log","message":"- main in /path/to/module"}`, nil
		}
		return "", nil
	}

	configHandler := config.NewConfigHandler(mockShell)
	configHandler.SetContext("mock-context")

	defaultConfigStr := `
contexts:
  mock-context:
    terraform:
      enabled: true
`
	if err := configHandler.LoadConfigString(defaultConfigStr); err != nil {
		t.Fatalf("Failed to load default config string: %v", err)
	}

	mockBlueprintHandler := blueprint.NewMockBlueprintHandler()
	mockBlueprintHandler.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
		return []blueprintv1alpha1.TerraformComponent{
			{
				Path:     "test-module",
				Source:   "git::https://github.com/test/module.git",
				FullPath: filepath.Join(tmpDir, "terraform", "test-module"),
				Inputs: map[string]any{
					"cluster_name": "test-cluster",
				},
			},
		}
	}

	shims := setupDefaultShims()

	// Create mock tools manager
	mockToolsManager := tools.NewMockToolsManager()
	mockToolsManager.GetTerraformCommandFunc = func() string {
		return "terraform"
	}

	// Create evaluator
	evaluator := evaluator.NewExpressionEvaluator(configHandler, tmpDir, filepath.Join(tmpDir, "contexts", "_template"))

	// Create runtime
	rt := &runtime.Runtime{
		ConfigHandler:      configHandler,
		ToolsManager:       mockToolsManager,
		Shell:              mockShell,
		Evaluator:          evaluator,
		ProjectRoot:        tmpDir,
		ContextName:        "local",
		WindsorScratchPath: filepath.Join(tmpDir, ".windsor", "contexts", "local"),
	}

	mocks := &TerraformTestMocks{
		Shell:            mockShell,
		ConfigHandler:    configHandler,
		BlueprintHandler: mockBlueprintHandler,
		Shims:            shims,
		Runtime:          rt,
	}

	// Apply any overrides
	for _, opt := range opts {
		opt(mocks)
	}

	return mocks
}

// setupDefaultShims configures safe default implementations for all shims operations
// This eliminates the need for repetitive mocking in individual test cases
func setupDefaultShims() *Shims {
	shims := NewShims()

	// Safe defaults for file operations
	shims.ReadFile = func(path string) ([]byte, error) {
		if strings.HasSuffix(path, "variables.tf") {
			return []byte(`variable "foo" { 
				type = string 
				description = "Test variable"
				default = "test"
				sensitive = true
			}`), nil
		}
		if strings.HasSuffix(path, "outputs.tf") {
			return []byte(`output "foo" { 
				value = "test"
				description = "Test output"
			}`), nil
		}
		return []byte{}, nil
	}

	shims.WriteFile = func(path string, data []byte, perm os.FileMode) error {
		return nil
	}

	shims.Stat = func(path string) (os.FileInfo, error) {
		if strings.HasSuffix(path, "variables.tf") || strings.HasSuffix(path, "outputs.tf") {
			return nil, nil
		}
		return nil, os.ErrNotExist
	}

	shims.MkdirAll = func(path string, perm os.FileMode) error {
		return nil
	}

	shims.Chdir = func(path string) error {
		return nil
	}

	shims.Setenv = func(key, value string) error {
		return nil
	}

	shims.FilepathRel = func(basepath, targpath string) (string, error) {
		return "./relative/path", nil
	}

	shims.JsonUnmarshal = func(data []byte, v any) error {
		return nil
	}

	return shims
}
