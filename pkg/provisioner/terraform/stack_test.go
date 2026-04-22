package terraform

// The StackTest provides comprehensive test coverage for the Stack implementation.
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
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/config"
	envvars "github.com/windsorcli/cli/pkg/runtime/env"
	"github.com/windsorcli/cli/pkg/runtime/evaluator"
	"github.com/windsorcli/cli/pkg/runtime/shell"
	terraformRuntime "github.com/windsorcli/cli/pkg/runtime/terraform"
	"github.com/windsorcli/cli/pkg/runtime/tools"
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

type TerraformTestMocks struct {
	ConfigHandler    config.ConfigHandler
	Shell            *shell.MockShell
	Blueprint        *blueprint.MockBlueprintHandler
	Shims            *Shims
	Runtime          *runtime.Runtime
	BlueprintHandler blueprint.BlueprintHandler
}

type SetupOptions struct {
	ConfigHandler config.ConfigHandler
	ConfigStr     string
}

// setupTerraformMocks creates mock components for testing the stack
func setupTerraformMocks(t *testing.T, opts ...*SetupOptions) *TerraformTestMocks {
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

	mockShell := shell.NewMockShell()

	mockBlueprint := blueprint.NewMockBlueprintHandler()
	mockBlueprint.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
		return []blueprintv1alpha1.TerraformComponent{
			{
				Source:   "git::https://github.com/terraform-aws-modules/terraform-aws-vpc.git//terraform/remote/path@v1.0.0",
				Path:     "remote/path",
				FullPath: filepath.Join(tmpDir, ".windsor", "contexts", "local", "terraform", "remote", "path"),
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

	evaluator := evaluator.NewExpressionEvaluator(configHandler, tmpDir, filepath.Join(tmpDir, "contexts", "_template"))

	rt := &runtime.Runtime{
		ProjectRoot:   tmpDir,
		ConfigRoot:    tmpDir,
		TemplateRoot:  filepath.Join(tmpDir, "contexts", "_template"),
		ConfigHandler: configHandler,
		Shell:         mockShell,
		Evaluator:     evaluator,
	}

	return &TerraformTestMocks{
		ConfigHandler:    configHandler,
		Shell:            mockShell,
		Blueprint:        mockBlueprint,
		Shims:            shims,
		Runtime:          rt,
		BlueprintHandler: mockBlueprint,
	}
}

// setupWindsorStackMocks creates mock components for testing the Stack
func setupWindsorStackMocks(t *testing.T, opts ...*SetupOptions) *TerraformTestMocks {
	t.Helper()
	mocks := setupTerraformMocks(t, opts...)

	projectRoot := os.Getenv("WINDSOR_PROJECT_ROOT")
	tfModulesDir := filepath.Join(projectRoot, ".windsor", "contexts", "local", "terraform", "remote", "path")
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

	mockToolsManager := tools.NewMockToolsManager()
	mockToolsManager.GetTerraformCommandFunc = func() string {
		return "terraform"
	}
	mocks.Runtime.ToolsManager = mockToolsManager
	mockTerraformProvider := terraformRuntime.NewTerraformProvider(mocks.ConfigHandler, mocks.Shell, mockToolsManager, mocks.Runtime.Evaluator)
	mocks.Runtime.TerraformProvider = mockTerraformProvider
	terraformEnv := envvars.NewTerraformEnvPrinter(mocks.Shell, mocks.ConfigHandler, mockToolsManager, mockTerraformProvider)
	mocks.Runtime.EnvPrinters.TerraformEnv = terraformEnv

	return mocks
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestStack_NewStack(t *testing.T) {
	setup := func(t *testing.T) (*TerraformStack, *TerraformTestMocks) {
		t.Helper()
		mocks := setupWindsorStackMocks(t)
		stack := NewStack(mocks.Runtime).(*TerraformStack)
		return stack, mocks
	}

	t.Run("Success", func(t *testing.T) {
		stack, _ := setup(t)

		if stack == nil {
			t.Errorf("Expected stack to be non-nil")
		}
	})
}

func TestStack_Up(t *testing.T) {
	setup := func(t *testing.T) (*TerraformStack, *TerraformTestMocks) {
		t.Helper()
		mocks := setupWindsorStackMocks(t)
		stack := NewStack(mocks.Runtime).(*TerraformStack)
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

	t.Run("InvokesOnApplyHooksAfterEachComponentWithComponentID", func(t *testing.T) {
		stack, _ := setup(t)
		blueprint := createTestBlueprint()
		var invokedIDs []string
		hook := func(id string) error {
			invokedIDs = append(invokedIDs, id)
			return nil
		}

		err := stack.Up(blueprint, hook)

		if err != nil {
			t.Errorf("Expected Up to return nil, got %v", err)
		}
		expected := []string{"remote/path", "local/path"}
		if len(invokedIDs) != len(expected) {
			t.Errorf("Expected hook to be invoked %d times, got %d: %v", len(expected), len(invokedIDs), invokedIDs)
		}
		for i, id := range expected {
			if i >= len(invokedIDs) || invokedIDs[i] != id {
				t.Errorf("Expected hook invocation %d to have id %q, got %v", i, id, invokedIDs)
			}
		}
	})

	t.Run("ReturnsErrorWhenOnApplyHookFails", func(t *testing.T) {
		stack, _ := setup(t)
		blueprint := createTestBlueprint()
		hookErr := fmt.Errorf("hook failed")
		hook := func(id string) error {
			return hookErr
		}

		err := stack.Up(blueprint, hook)

		if err == nil {
			t.Fatal("Expected Up to return error when hook fails")
		}
		if !strings.Contains(err.Error(), "post-apply hook") {
			t.Errorf("Expected error to mention post-apply hook, got %q", err.Error())
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

	t.Run("ErrorRunningTerraformInit", func(t *testing.T) {
		stack, mocks := setup(t)
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
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
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
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
		mocks.Shell.ExecProgressWithEnvFunc = func(message string, command string, env map[string]string, args ...string) (string, error) {
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

	t.Run("NilBlueprint", func(t *testing.T) {
		stack, _ := setup(t)
		err := stack.Up(nil)
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "blueprint not provided") {
			t.Errorf("Expected blueprint not provided error, got: %v", err)
		}
	})

	t.Run("EmptyProjectRoot", func(t *testing.T) {
		stack, mocks := setup(t)
		mocks.Runtime.ProjectRoot = ""
		blueprint := createTestBlueprint()
		err := stack.Up(blueprint)
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "project root is empty") {
			t.Errorf("Expected project root error, got: %v", err)
		}
	})

	t.Run("IgnoresErrorRemovingBackendOverride", func(t *testing.T) {
		// Given a stack with Remove that fails
		stack, mocks := setup(t)
		projectRoot := os.Getenv("WINDSOR_PROJECT_ROOT")
		backendOverridePath := filepath.Join(projectRoot, ".windsor", "contexts", "local", "remote", "path", "backend_override.tf")
		if err := os.MkdirAll(filepath.Dir(backendOverridePath), 0755); err != nil {
			t.Fatalf("Failed to create directory: %v", err)
		}
		if err := os.WriteFile(backendOverridePath, []byte("test"), 0644); err != nil {
			t.Fatalf("Failed to create backend override file: %v", err)
		}
		mocks.Shims.Remove = func(path string) error {
			return fmt.Errorf("remove error")
		}
		blueprint := createTestBlueprint()

		// When running Up
		err := stack.Up(blueprint)

		// Then it should succeed (cleanup errors are ignored)
		if err != nil {
			t.Errorf("Expected no error (cleanup is best-effort), got: %v", err)
		}
	})

	t.Run("HandlesNamedComponent", func(t *testing.T) {
		stack, _ := setup(t)
		projectRoot := os.Getenv("WINDSOR_PROJECT_ROOT")

		namedComponentDir := filepath.Join(projectRoot, ".windsor", "contexts", "local", "terraform", "cluster")
		if err := os.MkdirAll(namedComponentDir, 0755); err != nil {
			t.Fatalf("Failed to create named component directory: %v", err)
		}

		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{
					Name:   "cluster",
					Path:   "terraform/cluster",
					Source: "",
					Inputs: map[string]any{
						"node_count": 3,
					},
				},
			},
		}

		if err := stack.Up(blueprint); err != nil {
			t.Errorf("Expected Up to succeed with named component, got %v", err)
		}
	})

	t.Run("HandlesNamedComponentWithSource", func(t *testing.T) {
		stack, _ := setup(t)
		projectRoot := os.Getenv("WINDSOR_PROJECT_ROOT")

		namedComponentDir := filepath.Join(projectRoot, ".windsor", "contexts", "local", "terraform", "network")
		if err := os.MkdirAll(namedComponentDir, 0755); err != nil {
			t.Fatalf("Failed to create named component directory: %v", err)
		}

		blueprint := &blueprintv1alpha1.Blueprint{
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
					Name:   "network",
					Path:   "terraform/network",
					Source: "source1",
					Inputs: map[string]any{
						"cidr": "10.0.0.0/16",
					},
				},
			},
		}

		if err := stack.Up(blueprint); err != nil {
			t.Errorf("Expected Up to succeed with named component with source, got %v", err)
		}
	})

	t.Run("SetsTFVarOperationToApply", func(t *testing.T) {
		// Given a stack and a blueprint
		stack, mocks := setup(t)
		blueprint := createTestBlueprint()

		// When Up is called, capture the env passed to terraform plan
		// (TF_VAR_* are included in the plan step, not apply, since apply uses a plan file)
		var capturedEnv map[string]string
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if len(args) > 1 && args[1] == "plan" {
				capturedEnv = env
			}
			return "", nil
		}

		_ = stack.Up(blueprint)

		// Then TF_VAR_operation is "apply"
		if capturedEnv == nil {
			t.Fatal("Expected plan to be invoked")
		}
		if capturedEnv["TF_VAR_operation"] != "apply" {
			t.Errorf("Expected TF_VAR_operation to be %q, got %q", "apply", capturedEnv["TF_VAR_operation"])
		}
	})

}

func TestStack_MigrateState(t *testing.T) {
	setup := func(t *testing.T) (*TerraformStack, *TerraformTestMocks) {
		t.Helper()
		mocks := setupWindsorStackMocks(t)
		stack := NewStack(mocks.Runtime).(*TerraformStack)
		stack.shims = mocks.Shims
		return stack, mocks
	}

	t.Run("ReturnsErrorForNilBlueprint", func(t *testing.T) {
		stack, _ := setup(t)
		if _, err := stack.MigrateState(nil); err == nil {
			t.Fatal("Expected error for nil blueprint")
		}
	})

	t.Run("RunsInitWithMigrateStateForEachComponent", func(t *testing.T) {
		// Given a stack and a two-component blueprint
		stack, mocks := setup(t)
		blueprint := createTestBlueprint()

		migrateStateInits := 0
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if len(args) >= 2 && args[1] == "init" {
				for _, a := range args {
					if a == "-migrate-state" {
						migrateStateInits++
						break
					}
				}
			}
			return "", nil
		}

		// When MigrateState runs
		skipped, err := stack.MigrateState(blueprint)
		if err != nil {
			t.Fatalf("Expected MigrateState to succeed, got %v", err)
		}
		if len(skipped) != 0 {
			t.Errorf("Expected no skipped components when all dirs exist, got %v", skipped)
		}

		// Then terraform init -migrate-state fired once per component and no plan/apply ran.
		if migrateStateInits != 2 {
			t.Errorf("Expected 2 migrate-state inits (one per component), got %d", migrateStateInits)
		}
	})

	t.Run("DoesNotPassUpgradeFlag", func(t *testing.T) {
		// MigrateState must not include -upgrade: its contract is moving state, not
		// reinstalling providers or rewriting .terraform.lock.hcl. Execution flags are
		// added per operation at each call site, and MigrateState's call site passes
		// only -migrate-state + -force-copy.
		stack, mocks := setup(t)
		blueprint := createTestBlueprint()

		var upgradeSeen bool
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if len(args) >= 2 && args[1] == "init" {
				for _, a := range args {
					if a == "-upgrade" {
						upgradeSeen = true
					}
				}
			}
			return "", nil
		}

		// When MigrateState runs
		if _, err := stack.MigrateState(blueprint); err != nil {
			t.Fatalf("Expected MigrateState to succeed, got %v", err)
		}

		// Then no init invocation carried -upgrade
		if upgradeSeen {
			t.Error("Expected MigrateState's init not to include -upgrade; state migration must not reinstall providers")
		}
	})

	t.Run("ReturnsErrorWhenMigrationInitFails", func(t *testing.T) {
		stack, mocks := setup(t)
		blueprint := createTestBlueprint()

		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if len(args) >= 2 && args[1] == "init" {
				return "", fmt.Errorf("backend not reachable")
			}
			return "", nil
		}

		_, err := stack.MigrateState(blueprint)
		if err == nil {
			t.Fatal("Expected MigrateState to return an error when init fails")
		}
		if !strings.Contains(err.Error(), "error running terraform init") {
			t.Errorf("Expected error to mention terraform init failure, got: %v", err)
		}
		if !strings.Contains(err.Error(), "backend not reachable") {
			t.Errorf("Expected error to wrap the underlying cause, got: %v", err)
		}
	})

	t.Run("SkipsComponentsWithMissingDirectoriesAndReportsThem", func(t *testing.T) {
		// Given a stat shim that reports every component directory as missing — the
		// blueprint may list components that were never applied (or were already
		// torn down manually), and MigrateState is called before destroy precisely
		// when some of that state may be absent. The skipped components must be
		// returned to the caller so bootstrap (which calls MigrateState after Up
		// has materialized all dirs) can treat a non-empty skip as an anomaly,
		// while pre-destroy migration can discard the list and proceed.
		stack, mocks := setup(t)
		blueprint := createTestBlueprint()

		mocks.Shims.Stat = func(path string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		var initsSeen int
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if len(args) >= 2 && args[1] == "init" {
				initsSeen++
			}
			return "", nil
		}

		// When MigrateState runs
		skipped, err := stack.MigrateState(blueprint)
		if err != nil {
			t.Fatalf("Expected no error when every component dir is missing, got %v", err)
		}

		// Then no terraform init was invoked — the missing components were skipped —
		// and both component IDs appear in the returned skip list.
		if initsSeen != 0 {
			t.Errorf("Expected 0 init invocations when all dirs are missing, got %d", initsSeen)
		}
		if len(skipped) != 2 {
			t.Fatalf("Expected 2 skipped component IDs, got %d: %v", len(skipped), skipped)
		}
	})
}

func TestStack_DestroyAll(t *testing.T) {
	setup := func(t *testing.T) (*TerraformStack, *TerraformTestMocks) {
		t.Helper()
		mocks := setupWindsorStackMocks(t)
		stack := NewStack(mocks.Runtime).(*TerraformStack)
		stack.shims = mocks.Shims

		mocks.Blueprint.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
			return []blueprintv1alpha1.TerraformComponent{
				{
					Source:   "source1",
					Path:     "module/path1",
					FullPath: filepath.Join(os.Getenv("WINDSOR_PROJECT_ROOT"), ".windsor", "contexts", "local", "terraform", "remote", "path"),
				},
			}
		}

		return stack, mocks
	}

	t.Run("Success", func(t *testing.T) {
		stack, _ := setup(t)
		blueprint := createTestBlueprint()

		if err := stack.DestroyAll(blueprint); err != nil {
			t.Errorf("Expected Down to return nil, got %v", err)
		}
	})

	t.Run("ErrorGettingCurrentDirectory", func(t *testing.T) {
		stack, mocks := setup(t)
		mocks.Shims.Getwd = func() (string, error) {
			return "", fmt.Errorf("mock error getting current directory")
		}

		blueprint := createTestBlueprint()
		err := stack.DestroyAll(blueprint)
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
		err := stack.DestroyAll(blueprint)
		if err != nil {
			t.Fatalf("Expected no error when directory doesn't exist, got %v", err)
		}
	})

	t.Run("ErrorRunningTerraformPlan", func(t *testing.T) {
		// Given a stack whose shell fails on the destroy-phase terraform plan.
		// Both the prep-apply plan and the destroy-phase plan run via ExecSilentWithEnv;
		// this test matches the second occurrence (plan -destroy, identified by
		// PlanDestroyArgs being appended), so the first prep-apply plan succeeds and we
		// still exercise the destroy-phase error path.
		stack, mocks := setup(t)
		planCalls := 0
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" && len(args) > 0 && strings.HasPrefix(args[0], "-chdir=") && len(args) > 1 && args[1] == "plan" {
				planCalls++
				if planCalls >= 2 {
					return "", fmt.Errorf("mock error running terraform plan")
				}
			}
			return "", nil
		}

		blueprint := createTestBlueprint()
		err := stack.DestroyAll(blueprint)
		expectedError := "error running terraform plan destroy for"
		if err == nil {
			t.Fatalf("Expected plan-destroy error, got nil")
		}
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ExcludesComponentsWithEnabledFalse", func(t *testing.T) {
		stack, mocks := setup(t)

		mocks.Runtime.TerraformProvider.ClearCache()

		projectRoot := os.Getenv("WINDSOR_PROJECT_ROOT")
		contextName := mocks.Runtime.ContextName
		enabledFalse := false
		blueprint := createTestBlueprint()
		blueprint.Sources = append(blueprint.Sources, blueprintv1alpha1.Source{
			Name: "source2",
			Url:  "https://github.com/example/example2.git",
			Ref:  blueprintv1alpha1.Reference{Branch: "main"},
		})
		blueprint.TerraformComponents = []blueprintv1alpha1.TerraformComponent{
			{
				Source: "source1",
				Path:   "module/path1",
			},
			{
				Source:  "source2",
				Path:    "module/path2",
				Enabled: &blueprintv1alpha1.BoolExpression{Value: &enabledFalse, IsExpr: false},
			},
		}

		path1Dir := filepath.Join(projectRoot, ".windsor", "contexts", contextName, "terraform", "module/path1")
		if err := os.MkdirAll(path1Dir, 0755); err != nil {
			t.Fatalf("Failed to create directory: %v", err)
		}

		var terraformCommands []string
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" {
				terraformCommands = append(terraformCommands, strings.Join(args, " "))
			}
			return "", nil
		}

		if err := stack.DestroyAll(blueprint); err != nil {
			t.Errorf("Expected Down to return nil, got %v", err)
		}

		for _, cmd := range terraformCommands {
			if strings.Contains(cmd, "path2") {
				t.Errorf("Expected no terraform commands for path2 (enabled: false), but found: %v", terraformCommands)
				break
			}
		}
		if len(terraformCommands) == 0 {
			t.Errorf("Expected terraform commands for path1 (enabled), but got none: %v", terraformCommands)
		}
	})

	t.Run("SkipComponentsWithDestroyFalse", func(t *testing.T) {
		stack, mocks := setup(t)

		mocks.Runtime.TerraformProvider.ClearCache()

		projectRoot := os.Getenv("WINDSOR_PROJECT_ROOT")
		destroyFalse := false
		blueprint := createTestBlueprint()
		blueprint.Sources = append(blueprint.Sources, blueprintv1alpha1.Source{
			Name: "source2",
			Url:  "https://github.com/example/example2.git",
			Ref:  blueprintv1alpha1.Reference{Branch: "main"},
		})
		blueprint.TerraformComponents = []blueprintv1alpha1.TerraformComponent{
			{
				Source:   "source1",
				Path:     "module/path1",
				FullPath: filepath.Join(projectRoot, ".windsor", "contexts", "local", "terraform", "module/path1"),
				Destroy:  &blueprintv1alpha1.BoolExpression{Value: &destroyFalse, IsExpr: false},
			},
			{
				Source:   "source2",
				Path:     "module/path2",
				FullPath: filepath.Join(projectRoot, ".windsor", "contexts", "local", "terraform", "module/path2"),
			},
		}

		if err := os.MkdirAll(blueprint.TerraformComponents[0].FullPath, 0755); err != nil {
			t.Fatalf("Failed to create directory: %v", err)
		}
		if err := os.MkdirAll(blueprint.TerraformComponents[1].FullPath, 0755); err != nil {
			t.Fatalf("Failed to create directory: %v", err)
		}

		var terraformCommands []string
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" {
				cmdStr := strings.Join(args, " ")
				terraformCommands = append(terraformCommands, cmdStr)
			}
			return "", nil
		}

		if err := stack.DestroyAll(blueprint); err != nil {
			t.Errorf("Expected Down to return nil, got %v", err)
		}

		foundPath1Commands := false
		foundPath2Commands := false

		for _, cmd := range terraformCommands {
			if strings.Contains(cmd, "module/path1") || strings.Contains(cmd, "/path1") || (strings.Contains(cmd, "path1") && !strings.Contains(cmd, "path2")) {
				foundPath1Commands = true
			}
			if strings.Contains(cmd, "module/path2") || strings.Contains(cmd, "/path2") || (strings.Contains(cmd, "path2") && !strings.Contains(cmd, "path1")) {
				foundPath2Commands = true
			}
		}

		if foundPath1Commands {
			t.Errorf("Expected no terraform commands for path1 (destroy: false), but found commands: %v", terraformCommands)
		}
		if !foundPath2Commands {
			t.Errorf("Expected terraform commands for path2 (destroy: true), but found none. Commands executed: %v", terraformCommands)
		}
	})

	t.Run("ErrorRunningTerraformDestroy", func(t *testing.T) {
		// DestroyAll runs every terraform subcommand via ExecSilentWithEnv so the outer
		// "Destroying <path>" progress span is the only label the user sees.
		stack, mocks := setup(t)
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" && len(args) > 0 && strings.HasPrefix(args[0], "-chdir=") && len(args) > 1 && args[1] == "destroy" {
				return "", fmt.Errorf("mock error running terraform destroy")
			}
			return "", nil
		}

		blueprint := createTestBlueprint()
		err := stack.DestroyAll(blueprint)
		expectedError := "error running terraform destroy for"
		if err == nil {
			t.Fatalf("Expected destroy error, got nil")
		}
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("NilBlueprint", func(t *testing.T) {
		stack, _ := setup(t)
		err := stack.DestroyAll(nil)
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "blueprint not provided") {
			t.Errorf("Expected blueprint not provided error, got: %v", err)
		}
	})

	t.Run("EmptyProjectRoot", func(t *testing.T) {
		stack, mocks := setup(t)
		mocks.Runtime.ProjectRoot = ""
		blueprint := createTestBlueprint()
		err := stack.DestroyAll(blueprint)
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "project root is empty") {
			t.Errorf("Expected project root error, got: %v", err)
		}
	})

	t.Run("IgnoresErrorRemovingBackendOverride", func(t *testing.T) {
		// Given a stack with Remove that fails
		stack, mocks := setup(t)
		projectRoot := os.Getenv("WINDSOR_PROJECT_ROOT")
		backendOverridePath := filepath.Join(projectRoot, ".windsor", "contexts", "local", "remote", "path", "backend_override.tf")
		if err := os.MkdirAll(filepath.Dir(backendOverridePath), 0755); err != nil {
			t.Fatalf("Failed to create directory: %v", err)
		}
		if err := os.WriteFile(backendOverridePath, []byte("test"), 0644); err != nil {
			t.Fatalf("Failed to create backend override file: %v", err)
		}
		mocks.Shims.Remove = func(path string) error {
			return fmt.Errorf("remove error")
		}
		blueprint := createTestBlueprint()

		// When running Down
		err := stack.DestroyAll(blueprint)

		// Then it should succeed (cleanup errors are ignored)
		if err != nil {
			t.Errorf("Expected no error (cleanup is best-effort), got: %v", err)
		}
	})

	t.Run("HandlesNamedComponent", func(t *testing.T) {
		stack, _ := setup(t)
		projectRoot := os.Getenv("WINDSOR_PROJECT_ROOT")

		namedComponentDir := filepath.Join(projectRoot, ".windsor", "contexts", "local", "terraform", "cluster")
		if err := os.MkdirAll(namedComponentDir, 0755); err != nil {
			t.Fatalf("Failed to create named component directory: %v", err)
		}

		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{
					Name:   "cluster",
					Path:   "terraform/cluster",
					Source: "",
					Inputs: map[string]any{
						"node_count": 3,
					},
				},
			},
		}

		if err := stack.DestroyAll(blueprint); err != nil {
			t.Errorf("Expected Down to succeed with named component, got %v", err)
		}
	})

	t.Run("HandlesNamedComponentWithSource", func(t *testing.T) {
		stack, _ := setup(t)
		projectRoot := os.Getenv("WINDSOR_PROJECT_ROOT")

		namedComponentDir := filepath.Join(projectRoot, ".windsor", "contexts", "local", "terraform", "network")
		if err := os.MkdirAll(namedComponentDir, 0755); err != nil {
			t.Fatalf("Failed to create named component directory: %v", err)
		}

		blueprint := &blueprintv1alpha1.Blueprint{
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
					Name:   "network",
					Path:   "terraform/network",
					Source: "source1",
					Inputs: map[string]any{
						"cidr": "10.0.0.0/16",
					},
				},
			},
		}

		if err := stack.DestroyAll(blueprint); err != nil {
			t.Errorf("Expected Down to succeed with named component with source, got %v", err)
		}
	})

	t.Run("SetsTFVarOperationToDestroy", func(t *testing.T) {
		// Given a stack and a blueprint
		stack, mocks := setup(t)
		blueprint := createTestBlueprint()

		// When DestroyAll is called, capture the env passed to terraform destroy.
		// All destroy-phase subcommands run via ExecSilentWithEnv now.
		var capturedEnv map[string]string
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if len(args) > 1 && args[1] == "destroy" {
				capturedEnv = env
			}
			return "", nil
		}

		_ = stack.DestroyAll(blueprint)

		// Then TF_VAR_operation is "destroy"
		if capturedEnv == nil {
			t.Fatal("Expected destroy to be invoked")
		}
		if capturedEnv["TF_VAR_operation"] != "destroy" {
			t.Errorf("Expected TF_VAR_operation to be %q, got %q", "destroy", capturedEnv["TF_VAR_operation"])
		}
	})

	t.Run("RunsPrepThenDestroyPerComponentInReverseOrder", func(t *testing.T) {
		// Given a stack that records the per-component terraform steps in order
		stack, mocks := setup(t)
		blueprint := createTestBlueprint()

		type step struct {
			component  string
			subcommand string
			operation  string
		}
		var sequence []step
		componentOf := func(args []string) string {
			if len(args) == 0 {
				return ""
			}
			switch {
			case strings.Contains(args[0], "remote/path"):
				return "remote/path"
			case strings.Contains(args[0], "local/path"):
				return "local/path"
			}
			return ""
		}
		record := func(env map[string]string, args []string) {
			if len(args) <= 1 {
				return
			}
			sequence = append(sequence, step{
				component:  componentOf(args),
				subcommand: args[1],
				operation:  env["TF_VAR_operation"],
			})
		}
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" {
				record(env, args)
			}
			return "", nil
		}
		mocks.Shell.ExecProgressWithEnvFunc = func(message string, command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" {
				record(env, args)
			}
			return "", nil
		}

		// When destroying all components
		if err := stack.DestroyAll(blueprint); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then components are walked in reverse dependency order, and each component is
		// taken fully through prep-apply then destroy before the next one begins. This
		// matches terraform destroy semantics end-to-end: a dependent is torn down
		// (including its prep) before its dependency is touched. TF_VAR_operation=destroy
		// is forwarded to the commands that include TF_VAR_* in their env (plan, refresh,
		// destroy); init and apply intentionally don't forward TF_VAR_* and rely on the
		// process env set by windsor env.
		type ordered struct {
			component  string
			subcommand string
		}
		expected := []ordered{
			{component: "local/path", subcommand: "init"},
			{component: "local/path", subcommand: "plan"},
			{component: "local/path", subcommand: "apply"},
			{component: "local/path", subcommand: "refresh"},
			{component: "local/path", subcommand: "plan"},
			{component: "local/path", subcommand: "destroy"},
			{component: "remote/path", subcommand: "init"},
			{component: "remote/path", subcommand: "plan"},
			{component: "remote/path", subcommand: "apply"},
			{component: "remote/path", subcommand: "refresh"},
			{component: "remote/path", subcommand: "plan"},
			{component: "remote/path", subcommand: "destroy"},
		}
		if len(sequence) != len(expected) {
			t.Fatalf("Expected %d steps, got %d: %+v", len(expected), len(sequence), sequence)
		}
		for i, want := range expected {
			got := ordered{component: sequence[i].component, subcommand: sequence[i].subcommand}
			if got != want {
				t.Errorf("step %d: got %+v, want %+v", i, got, want)
			}
			switch sequence[i].subcommand {
			case "plan", "refresh", "destroy":
				if sequence[i].operation != "destroy" {
					t.Errorf("step %d (%s %s): TF_VAR_operation = %q, want %q", i, sequence[i].component, sequence[i].subcommand, sequence[i].operation, "destroy")
				}
			}
		}
	})

}

func TestNewShims(t *testing.T) {
	t.Run("InitializesAllFields", func(t *testing.T) {
		shims := NewShims()
		if shims.Stat == nil {
			t.Error("Expected Stat to be initialized")
		}
		if shims.Chdir == nil {
			t.Error("Expected Chdir to be initialized")
		}
		if shims.Getwd == nil {
			t.Error("Expected Getwd to be initialized")
		}
		if shims.Setenv == nil {
			t.Error("Expected Setenv to be initialized")
		}
		if shims.Unsetenv == nil {
			t.Error("Expected Unsetenv to be initialized")
		}
		if shims.Remove == nil {
			t.Error("Expected Remove to be initialized")
		}
	})
}


func TestTerraformStack_PlanComponentSummary(t *testing.T) {
	setup := func(t *testing.T) (*TerraformStack, *TerraformTestMocks) {
		t.Helper()
		mocks := setupWindsorStackMocks(t)
		stack := NewStack(mocks.Runtime).(*TerraformStack)
		stack.shims = mocks.Shims
		return stack, mocks
	}

	t.Run("ReturnsErrorForNilBlueprint", func(t *testing.T) {
		// Given a stack with a valid runtime
		stack, _ := setup(t)

		// When PlanComponentSummary is called with nil blueprint
		result := stack.PlanComponentSummary(nil, "cluster")

		// Then an error is set on the result
		if result.Err == nil {
			t.Error("expected error for nil blueprint, got nil")
		}
	})

	t.Run("ReturnsErrorForMissingComponent", func(t *testing.T) {
		// Given a stack with a valid blueprint
		stack, _ := setup(t)

		// When PlanComponentSummary is called with an unknown componentID
		result := stack.PlanComponentSummary(createTestBlueprint(), "nonexistent")

		// Then an error is set on the result indicating not found
		if result.Err == nil {
			t.Error("expected not-found error, got nil")
		}
		if !strings.Contains(result.Err.Error(), "not found") {
			t.Errorf("expected 'not found' in error, got: %v", result.Err)
		}
	})

	t.Run("ParsesPlanCountsForNamedComponent", func(t *testing.T) {
		// Given a shell that returns a plan output with counts
		stack, mocks := setup(t)
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if len(args) > 1 && args[1] == "plan" {
				return "Plan: 2 to add, 1 to change, 0 to destroy.\n", nil
			}
			return "", nil
		}
		bp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Name: "mycomp", Path: "local/path"},
			},
		}

		// When PlanComponentSummary is called for the named component
		result := stack.PlanComponentSummary(bp, "mycomp")

		// Then counts are parsed from plan output
		if result.Err != nil {
			t.Fatalf("unexpected error: %v", result.Err)
		}
		if result.Add != 2 || result.Change != 1 || result.Destroy != 0 {
			t.Errorf("expected add=2 change=1 destroy=0, got add=%d change=%d destroy=%d", result.Add, result.Change, result.Destroy)
		}
	})
}

// =============================================================================
// Test Private Methods
// =============================================================================

func TestTerraformStack_setupTerraformEnvironment(t *testing.T) {
	setup := func(t *testing.T) (*TerraformStack, *TerraformTestMocks) {
		t.Helper()
		mocks := setupWindsorStackMocks(t)
		stack := NewStack(mocks.Runtime).(*TerraformStack)
		stack.shims = mocks.Shims
		return stack, mocks
	}

	t.Run("Success", func(t *testing.T) {
		stack, _ := setup(t)
		component := blueprintv1alpha1.TerraformComponent{
			Path:     "test/path",
			FullPath: filepath.Join(os.Getenv("WINDSOR_PROJECT_ROOT"), "terraform", "test", "path"),
		}
		_ = os.Setenv("TF_CLI_ARGS", "-var-file=/tmp/ambient.tfvars")
		_ = os.Setenv("TF_CLI_ARGS_init", "-upgrade")
		_ = os.Setenv("TF_CLI_ARGS_plan", "-lock=false")
		_ = os.Setenv("TF_CLI_ARGS_apply", "-auto-approve")
		_ = os.Setenv("TF_CLI_ARGS_destroy", "-auto-approve")
		_ = os.Setenv("TF_CLI_ARGS_import", "-input=false")
		_ = os.Setenv("TF_DATA_DIR", "/tmp/ambient")
		_ = os.Setenv("TF_VAR_talos_version", "1.0.0")
		defer func() {
			_ = os.Unsetenv("TF_CLI_ARGS")
			_ = os.Unsetenv("TF_CLI_ARGS_init")
			_ = os.Unsetenv("TF_CLI_ARGS_plan")
			_ = os.Unsetenv("TF_CLI_ARGS_apply")
			_ = os.Unsetenv("TF_CLI_ARGS_destroy")
			_ = os.Unsetenv("TF_CLI_ARGS_import")
			_ = os.Unsetenv("TF_DATA_DIR")
			_ = os.Unsetenv("TF_VAR_talos_version")
		}()

		terraformVars, terraformArgs, err := stack.setupTerraformEnvironment(component)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if terraformVars == nil {
			t.Fatal("Expected terraformVars to be non-nil")
		}

		if terraformArgs == nil {
			t.Error("Expected terraformArgs to be non-nil")
		}
		if terraformVars["TF_DATA_DIR"] == "" {
			t.Error("Expected TF_DATA_DIR to be populated")
		}
		if terraformVars["TF_VAR_context_path"] == "" {
			t.Error("Expected TF_VAR_context_path to be populated")
		}
	})

	t.Run("ErrorWhenTerraformEnvIsNil", func(t *testing.T) {
		stack, _ := setup(t)
		stack.terraformEnv = nil
		stack.runtime.EnvPrinters.TerraformEnv = nil

		component := blueprintv1alpha1.TerraformComponent{
			Path:     "test/path",
			FullPath: filepath.Join(os.Getenv("WINDSOR_PROJECT_ROOT"), "terraform", "test", "path"),
		}

		_, _, err := stack.setupTerraformEnvironment(component)
		if err == nil {
			t.Fatal("Expected error when terraformEnv is nil")
		}

		expectedError := "terraform environment printer not available"
		if !strings.Contains(err.Error(), expectedError) {
			t.Errorf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorWhenGenerateTerraformArgsFails", func(t *testing.T) {
		stack, mocks := setup(t)
		component := blueprintv1alpha1.TerraformComponent{
			Path:     "test/path",
			FullPath: filepath.Join(os.Getenv("WINDSOR_PROJECT_ROOT"), "terraform", "test", "path"),
		}

		mocks.ConfigHandler.Set("terraform.backend.type", "unsupported")

		_, _, err := stack.setupTerraformEnvironment(component)
		if err == nil {
			t.Fatal("Expected error when GenerateTerraformArgs fails")
		}

		expectedError := "error generating terraform args"
		if !strings.Contains(err.Error(), expectedError) {
			t.Errorf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("SelectCommandEnvWithoutTFVars", func(t *testing.T) {
		selected := selectTerraformCommandEnv(map[string]string{
			"TF_DATA_DIR":         "/tmp/data",
			"TF_VAR_context_path": "/tmp/context",
			"OTHER":               "ignored",
		}, false)

		if selected["TF_DATA_DIR"] != "/tmp/data" {
			t.Error("Expected TF_DATA_DIR to be included")
		}
		if _, ok := selected["TF_VAR_context_path"]; ok {
			t.Error("Expected TF_VAR_context_path to be omitted when includeTFVars is false")
		}
		if _, ok := selected["OTHER"]; ok {
			t.Error("Expected non-TF keys to be omitted")
		}
		if val, ok := selected["TF_CLI_ARGS_apply"]; !ok || val != "" {
			t.Error("Expected TF_CLI_ARGS_apply to be explicitly cleared")
		}
		if val, ok := selected["TF_CLI_ARGS_plan"]; !ok || val != "" {
			t.Error("Expected TF_CLI_ARGS_plan to be explicitly cleared")
		}
	})

	t.Run("SelectCommandEnvWithTFVars", func(t *testing.T) {
		selected := selectTerraformCommandEnv(map[string]string{
			"TF_DATA_DIR":         "/tmp/data",
			"TF_VAR_context_path": "/tmp/context",
		}, true)
		if selected["TF_VAR_context_path"] == "" {
			t.Error("Expected TF_VAR_context_path to be included when includeTFVars is true")
		}
	})
}

func TestStack_Plan(t *testing.T) {
	setup := func(t *testing.T) (*TerraformStack, *TerraformTestMocks) {
		t.Helper()
		mocks := setupWindsorStackMocks(t)
		stack := NewStack(mocks.Runtime).(*TerraformStack)
		stack.shims = mocks.Shims
		return stack, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a stack and a blueprint with a local component
		stack, _ := setup(t)
		blueprint := createTestBlueprint()

		// When planning the local component by ID
		err := stack.Plan(blueprint, "local/path")

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected Plan to return nil, got %v", err)
		}
	})

	t.Run("NilBlueprint", func(t *testing.T) {
		// Given a stack with a nil blueprint
		stack, _ := setup(t)

		// When planning with nil blueprint
		err := stack.Plan(nil, "local/path")

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "blueprint not provided") {
			t.Errorf("Expected blueprint not provided error, got: %v", err)
		}
	})

	t.Run("EmptyComponentID", func(t *testing.T) {
		// Given a stack with an empty component ID
		stack, _ := setup(t)
		blueprint := createTestBlueprint()

		// When planning with an empty component ID
		err := stack.Plan(blueprint, "")

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "component ID not provided") {
			t.Errorf("Expected component ID error, got: %v", err)
		}
	})

	t.Run("ComponentNotFound", func(t *testing.T) {
		// Given a stack and a blueprint with known components
		stack, _ := setup(t)
		blueprint := createTestBlueprint()

		// When planning a component that does not exist in the blueprint
		err := stack.Plan(blueprint, "does/not/exist")

		// Then an error should occur naming the missing component
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), `"does/not/exist" not found`) {
			t.Errorf("Expected not found error, got: %v", err)
		}
	})

	t.Run("EmptyProjectRoot", func(t *testing.T) {
		// Given a stack with an empty project root
		stack, mocks := setup(t)
		mocks.Runtime.ProjectRoot = ""
		blueprint := createTestBlueprint()

		// When planning
		err := stack.Plan(blueprint, "local/path")

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "project root is empty") {
			t.Errorf("Expected project root error, got: %v", err)
		}
	})

	t.Run("ErrorGettingCurrentDirectory", func(t *testing.T) {
		// Given a stack whose Getwd returns an error
		stack, mocks := setup(t)
		mocks.Shims.Getwd = func() (string, error) {
			return "", fmt.Errorf("mock error getting current directory")
		}
		blueprint := createTestBlueprint()

		// When planning
		err := stack.Plan(blueprint, "local/path")

		// Then an error should occur
		if !strings.Contains(err.Error(), "error getting current directory") {
			t.Fatalf("Expected error to contain %q, got %q", "error getting current directory", err.Error())
		}
	})

	t.Run("ErrorDirectoryDoesNotExist", func(t *testing.T) {
		// Given a stack whose Stat reports the component directory is missing
		stack, mocks := setup(t)
		mocks.Shims.Stat = func(path string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}
		blueprint := createTestBlueprint()

		// When planning
		err := stack.Plan(blueprint, "local/path")

		// Then an error should occur mentioning directory
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "directory") {
			t.Fatalf("Expected error to contain %q, got %q", "directory", err.Error())
		}
	})

	t.Run("ErrorRunningTerraformInit", func(t *testing.T) {
		// Given a stack whose shell fails on terraform init
		stack, mocks := setup(t)
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" && len(args) > 1 && args[1] == "init" {
				return "", fmt.Errorf("mock error running terraform init")
			}
			return "", nil
		}
		blueprint := createTestBlueprint()

		// When planning
		err := stack.Plan(blueprint, "local/path")

		// Then an error should occur
		if !strings.Contains(err.Error(), "error running terraform init for") {
			t.Fatalf("Expected error to contain %q, got %q", "error running terraform init for", err.Error())
		}
	})

	t.Run("ErrorRunningTerraformPlan", func(t *testing.T) {
		// Given a stack whose shell fails on terraform plan
		stack, mocks := setup(t)
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" && len(args) > 1 && args[1] == "plan" {
				return "", fmt.Errorf("mock error running terraform plan")
			}
			return "", nil
		}
		blueprint := createTestBlueprint()

		// When planning
		err := stack.Plan(blueprint, "local/path")

		// Then an error should occur
		if !strings.Contains(err.Error(), "error running terraform plan for") {
			t.Fatalf("Expected error to contain %q, got %q", "error running terraform plan for", err.Error())
		}
	})

	t.Run("SuccessWithNamedComponent", func(t *testing.T) {
		// Given a stack and a blueprint with a named component
		stack, _ := setup(t)
		projectRoot := os.Getenv("WINDSOR_PROJECT_ROOT")
		namedDir := filepath.Join(projectRoot, ".windsor", "contexts", "local", "terraform", "cluster")
		if err := os.MkdirAll(namedDir, 0755); err != nil {
			t.Fatalf("Failed to create named component directory: %v", err)
		}
		blueprint := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Name: "cluster", Path: "terraform/cluster"},
			},
		}

		// When planning by name
		err := stack.Plan(blueprint, "cluster")

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected Plan to return nil, got %v", err)
		}
	})

	t.Run("IgnoresErrorRemovingBackendOverride", func(t *testing.T) {
		// Given a stack with a backend_override.tf that fails to remove
		stack, mocks := setup(t)
		projectRoot := os.Getenv("WINDSOR_PROJECT_ROOT")
		localDir := filepath.Join(projectRoot, "terraform", "local", "path")
		backendOverridePath := filepath.Join(localDir, "backend_override.tf")
		if err := os.MkdirAll(localDir, 0755); err != nil {
			t.Fatalf("Failed to create directory: %v", err)
		}
		if err := os.WriteFile(backendOverridePath, []byte("test"), 0644); err != nil {
			t.Fatalf("Failed to create backend override file: %v", err)
		}
		mocks.Shims.Remove = func(path string) error {
			return fmt.Errorf("remove error")
		}
		blueprint := createTestBlueprint()

		// When planning
		err := stack.Plan(blueprint, "local/path")

		// Then it should succeed — cleanup errors are ignored
		if err != nil {
			t.Errorf("Expected no error (cleanup is best-effort), got: %v", err)
		}
	})

	t.Run("SetsTFVarOperationToApply", func(t *testing.T) {
		// Given a stack and a blueprint with a local component
		stack, mocks := setup(t)
		blueprint := createTestBlueprint()

		// When Plan is called, capture the env passed to terraform plan
		var capturedEnv map[string]string
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if len(args) > 1 && args[1] == "plan" {
				capturedEnv = env
			}
			return "", nil
		}

		_ = stack.Plan(blueprint, "local/path")

		// Then TF_VAR_operation is "apply"
		if capturedEnv == nil {
			t.Fatal("Expected plan to be invoked")
		}
		if capturedEnv["TF_VAR_operation"] != "apply" {
			t.Errorf("Expected TF_VAR_operation to be %q, got %q", "apply", capturedEnv["TF_VAR_operation"])
		}
	})

}

func TestStack_PlanAll(t *testing.T) {
	setup := func(t *testing.T) (*TerraformStack, *TerraformTestMocks) {
		t.Helper()
		mocks := setupWindsorStackMocks(t)
		stack := NewStack(mocks.Runtime).(*TerraformStack)
		stack.shims = mocks.Shims
		return stack, mocks
	}

	t.Run("ReturnsErrorForNilBlueprint", func(t *testing.T) {
		// Given a stack with a nil blueprint
		stack, _ := setup(t)

		// When planning all with nil blueprint
		err := stack.PlanAll(nil)

		// Then an error is returned
		if err == nil {
			t.Error("expected error for nil blueprint, got nil")
		}
	})

	t.Run("StreamsEveryComponent", func(t *testing.T) {
		// Given a stack with multiple components in the blueprint
		stack, mocks := setup(t)
		var planCalls int
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" && len(args) > 1 && args[1] == "plan" {
				planCalls++
			}
			return "", nil
		}
		bp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Name: "alpha", Path: "local/alpha"},
				{Name: "beta", Path: "local/beta"},
			},
		}

		// When planning all
		err := stack.PlanAll(bp)

		// Then both components are planned
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if planCalls != 2 {
			t.Errorf("expected 2 plan calls, got %d", planCalls)
		}
	})

	t.Run("SetsTFVarOperationToApply", func(t *testing.T) {
		// Given a stack and a blueprint with a local component
		stack, mocks := setup(t)
		blueprint := createTestBlueprint()

		// When PlanAll is called, capture the env passed to terraform plan
		var capturedEnv map[string]string
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if len(args) > 1 && args[1] == "plan" {
				capturedEnv = env
			}
			return "", nil
		}

		_ = stack.PlanAll(blueprint)

		// Then TF_VAR_operation is "apply"
		if capturedEnv == nil {
			t.Fatal("Expected plan to be invoked")
		}
		if capturedEnv["TF_VAR_operation"] != "apply" {
			t.Errorf("Expected TF_VAR_operation to be %q, got %q", "apply", capturedEnv["TF_VAR_operation"])
		}
	})
}

func TestStack_PlanJSON(t *testing.T) {
	setup := func(t *testing.T) (*TerraformStack, *TerraformTestMocks) {
		t.Helper()
		mocks := setupWindsorStackMocks(t)
		stack := NewStack(mocks.Runtime).(*TerraformStack)
		stack.shims = mocks.Shims
		return stack, mocks
	}

	t.Run("SetsTFVarOperationToApply", func(t *testing.T) {
		// Given a stack and a blueprint with a local component
		stack, mocks := setup(t)
		blueprint := createTestBlueprint()

		// When PlanJSON is called, capture the env passed to terraform plan
		var capturedEnv map[string]string
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if len(args) > 1 && args[1] == "plan" {
				capturedEnv = env
			}
			return "", nil
		}

		_ = stack.PlanJSON(blueprint, "local/path")

		// Then TF_VAR_operation is "apply"
		if capturedEnv == nil {
			t.Fatal("Expected plan to be invoked")
		}
		if capturedEnv["TF_VAR_operation"] != "apply" {
			t.Errorf("Expected TF_VAR_operation to be %q, got %q", "apply", capturedEnv["TF_VAR_operation"])
		}
	})
}

func TestStack_Apply(t *testing.T) {
	setup := func(t *testing.T) (*TerraformStack, *TerraformTestMocks) {
		t.Helper()
		mocks := setupWindsorStackMocks(t)
		stack := NewStack(mocks.Runtime).(*TerraformStack)
		stack.shims = mocks.Shims
		return stack, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a stack and a blueprint with a local component
		stack, _ := setup(t)
		blueprint := createTestBlueprint()

		// When applying the local component by ID
		err := stack.Apply(blueprint, "local/path")

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected Apply to return nil, got %v", err)
		}
	})

	t.Run("NilBlueprint", func(t *testing.T) {
		// Given a stack with a nil blueprint
		stack, _ := setup(t)

		// When applying with nil blueprint
		err := stack.Apply(nil, "local/path")

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "blueprint not provided") {
			t.Errorf("Expected blueprint not provided error, got: %v", err)
		}
	})

	t.Run("EmptyComponentID", func(t *testing.T) {
		// Given a stack with an empty component ID
		stack, _ := setup(t)
		blueprint := createTestBlueprint()

		// When applying with an empty component ID
		err := stack.Apply(blueprint, "")

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "component ID not provided") {
			t.Errorf("Expected component ID error, got: %v", err)
		}
	})

	t.Run("ComponentNotFound", func(t *testing.T) {
		// Given a stack and a blueprint with known components
		stack, _ := setup(t)
		blueprint := createTestBlueprint()

		// When applying a component that does not exist in the blueprint
		err := stack.Apply(blueprint, "does/not/exist")

		// Then an error should occur naming the missing component
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), `"does/not/exist" not found`) {
			t.Errorf("Expected not found error, got: %v", err)
		}
	})

	t.Run("EmptyProjectRoot", func(t *testing.T) {
		// Given a stack with an empty project root
		stack, mocks := setup(t)
		mocks.Runtime.ProjectRoot = ""
		blueprint := createTestBlueprint()

		// When applying
		err := stack.Apply(blueprint, "local/path")

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "project root is empty") {
			t.Errorf("Expected project root error, got: %v", err)
		}
	})

	t.Run("ErrorGettingCurrentDirectory", func(t *testing.T) {
		// Given a stack whose Getwd returns an error
		stack, mocks := setup(t)
		mocks.Shims.Getwd = func() (string, error) {
			return "", fmt.Errorf("mock error getting current directory")
		}
		blueprint := createTestBlueprint()

		// When applying
		err := stack.Apply(blueprint, "local/path")

		// Then an error should occur
		if !strings.Contains(err.Error(), "error getting current directory") {
			t.Fatalf("Expected error to contain %q, got %q", "error getting current directory", err.Error())
		}
	})

	t.Run("ErrorDirectoryDoesNotExist", func(t *testing.T) {
		// Given a stack whose Stat reports the component directory is missing
		stack, mocks := setup(t)
		mocks.Shims.Stat = func(path string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}
		blueprint := createTestBlueprint()

		// When applying
		err := stack.Apply(blueprint, "local/path")

		// Then an error should occur mentioning directory
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "directory") {
			t.Fatalf("Expected error to contain %q, got %q", "directory", err.Error())
		}
	})

	t.Run("ErrorRunningTerraformInit", func(t *testing.T) {
		// Given a stack whose shell fails on terraform init
		stack, mocks := setup(t)
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" && len(args) > 1 && args[1] == "init" {
				return "", fmt.Errorf("mock error running terraform init")
			}
			return "", nil
		}
		blueprint := createTestBlueprint()

		// When applying
		err := stack.Apply(blueprint, "local/path")

		// Then an error should occur
		if !strings.Contains(err.Error(), "error running terraform init for") {
			t.Fatalf("Expected error to contain %q, got %q", "error running terraform init for", err.Error())
		}
	})

	t.Run("ErrorRunningTerraformPlan", func(t *testing.T) {
		// Given a stack whose shell fails on terraform plan
		stack, mocks := setup(t)
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" && len(args) > 1 && args[1] == "plan" {
				return "", fmt.Errorf("mock error running terraform plan")
			}
			return "", nil
		}
		blueprint := createTestBlueprint()

		// When applying
		err := stack.Apply(blueprint, "local/path")

		// Then an error should occur
		if !strings.Contains(err.Error(), "error running terraform plan for") {
			t.Fatalf("Expected error to contain %q, got %q", "error running terraform plan for", err.Error())
		}
	})

	t.Run("ErrorRunningTerraformApply", func(t *testing.T) {
		// Given a stack whose shell fails on terraform apply
		stack, mocks := setup(t)
		mocks.Shell.ExecProgressWithEnvFunc = func(message string, command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" && len(args) > 1 && args[1] == "apply" {
				return "", fmt.Errorf("mock error running terraform apply")
			}
			return "", nil
		}
		blueprint := createTestBlueprint()

		// When applying
		err := stack.Apply(blueprint, "local/path")

		// Then an error should occur
		if !strings.Contains(err.Error(), "error running terraform apply for") {
			t.Fatalf("Expected error to contain %q, got %q", "error running terraform apply for", err.Error())
		}
	})

	t.Run("SuccessWithNamedComponent", func(t *testing.T) {
		// Given a stack and a blueprint with a named component
		stack, _ := setup(t)
		projectRoot := os.Getenv("WINDSOR_PROJECT_ROOT")
		namedDir := filepath.Join(projectRoot, ".windsor", "contexts", "local", "terraform", "cluster")
		if err := os.MkdirAll(namedDir, 0755); err != nil {
			t.Fatalf("Failed to create named component directory: %v", err)
		}
		blueprint := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Name: "cluster", Path: "terraform/cluster"},
			},
		}

		// When applying by name
		err := stack.Apply(blueprint, "cluster")

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected Apply to return nil, got %v", err)
		}
	})

	t.Run("IgnoresErrorRemovingBackendOverride", func(t *testing.T) {
		// Given a stack with a backend_override.tf that fails to remove
		stack, mocks := setup(t)
		projectRoot := os.Getenv("WINDSOR_PROJECT_ROOT")
		localDir := filepath.Join(projectRoot, "terraform", "local", "path")
		backendOverridePath := filepath.Join(localDir, "backend_override.tf")
		if err := os.MkdirAll(localDir, 0755); err != nil {
			t.Fatalf("Failed to create directory: %v", err)
		}
		if err := os.WriteFile(backendOverridePath, []byte("test"), 0644); err != nil {
			t.Fatalf("Failed to create backend override file: %v", err)
		}
		mocks.Shims.Remove = func(path string) error {
			return fmt.Errorf("remove error")
		}
		blueprint := createTestBlueprint()

		// When applying
		err := stack.Apply(blueprint, "local/path")

		// Then it should succeed — cleanup errors are ignored
		if err != nil {
			t.Errorf("Expected no error (cleanup is best-effort), got: %v", err)
		}
	})

	t.Run("SetsTFVarOperationToApply", func(t *testing.T) {
		// Given a stack and a blueprint with a local component
		stack, mocks := setup(t)
		blueprint := createTestBlueprint()

		// When Apply is called, capture the env passed to terraform plan
		// (TF_VAR_* are included in the plan step, not apply, since apply uses a plan file)
		var capturedEnv map[string]string
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if len(args) > 1 && args[1] == "plan" {
				capturedEnv = env
			}
			return "", nil
		}

		_ = stack.Apply(blueprint, "local/path")

		// Then TF_VAR_operation is "apply"
		if capturedEnv == nil {
			t.Fatal("Expected plan to be invoked")
		}
		if capturedEnv["TF_VAR_operation"] != "apply" {
			t.Errorf("Expected TF_VAR_operation to be %q, got %q", "apply", capturedEnv["TF_VAR_operation"])
		}
	})
}

func TestStack_Destroy(t *testing.T) {
	setup := func(t *testing.T) (*TerraformStack, *TerraformTestMocks) {
		t.Helper()
		mocks := setupWindsorStackMocks(t)
		stack := NewStack(mocks.Runtime).(*TerraformStack)
		stack.shims = mocks.Shims
		return stack, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a stack and a blueprint with a local component
		stack, _ := setup(t)
		blueprint := createTestBlueprint()

		// When destroying the local component by ID
		err := stack.Destroy(blueprint, "local/path")

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected Destroy to return nil, got %v", err)
		}
	})

	t.Run("NilBlueprint", func(t *testing.T) {
		// Given a stack with a nil blueprint
		stack, _ := setup(t)

		// When destroying with nil blueprint
		err := stack.Destroy(nil, "local/path")

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "blueprint not provided") {
			t.Errorf("Expected blueprint not provided error, got: %v", err)
		}
	})

	t.Run("EmptyComponentID", func(t *testing.T) {
		// Given a stack with an empty component ID
		stack, _ := setup(t)
		blueprint := createTestBlueprint()

		// When destroying with an empty component ID
		err := stack.Destroy(blueprint, "")

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "component ID not provided") {
			t.Errorf("Expected component ID error, got: %v", err)
		}
	})

	t.Run("ComponentNotFound", func(t *testing.T) {
		// Given a stack and a blueprint with known components
		stack, _ := setup(t)
		blueprint := createTestBlueprint()

		// When destroying a component that does not exist in the blueprint
		err := stack.Destroy(blueprint, "does/not/exist")

		// Then an error should occur naming the missing component
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), `"does/not/exist" not found`) {
			t.Errorf("Expected not found error, got: %v", err)
		}
	})

	t.Run("EmptyProjectRoot", func(t *testing.T) {
		// Given a stack with an empty project root
		stack, mocks := setup(t)
		mocks.Runtime.ProjectRoot = ""
		blueprint := createTestBlueprint()

		// When destroying
		err := stack.Destroy(blueprint, "local/path")

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "project root is empty") {
			t.Errorf("Expected project root error, got: %v", err)
		}
	})

	t.Run("ErrorGettingCurrentDirectory", func(t *testing.T) {
		// Given a stack whose Getwd returns an error
		stack, mocks := setup(t)
		mocks.Shims.Getwd = func() (string, error) {
			return "", fmt.Errorf("mock error getting current directory")
		}
		blueprint := createTestBlueprint()

		// When destroying
		err := stack.Destroy(blueprint, "local/path")

		// Then an error should occur
		if !strings.Contains(err.Error(), "error getting current directory") {
			t.Fatalf("Expected error to contain %q, got %q", "error getting current directory", err.Error())
		}
	})

	t.Run("ErrorDirectoryDoesNotExist", func(t *testing.T) {
		// Given a stack whose Stat reports the component directory is missing
		stack, mocks := setup(t)
		mocks.Shims.Stat = func(path string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}
		blueprint := createTestBlueprint()

		// When destroying
		err := stack.Destroy(blueprint, "local/path")

		// Then an error should occur mentioning directory
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "directory") {
			t.Fatalf("Expected error to contain %q, got %q", "directory", err.Error())
		}
	})

	t.Run("ErrorRunningTerraformInit", func(t *testing.T) {
		// Given a stack whose shell fails on terraform init
		stack, mocks := setup(t)
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" && len(args) > 1 && args[1] == "init" {
				return "", fmt.Errorf("mock error running terraform init")
			}
			return "", nil
		}
		blueprint := createTestBlueprint()

		// When destroying
		err := stack.Destroy(blueprint, "local/path")

		// Then an error should occur
		if !strings.Contains(err.Error(), "error running terraform init for") {
			t.Fatalf("Expected error to contain %q, got %q", "error running terraform init for", err.Error())
		}
	})

	t.Run("ErrorRunningDestroyPrepPlan", func(t *testing.T) {
		// Given a stack whose shell fails on the first terraform plan (the destroy-prep plan)
		stack, mocks := setup(t)
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" && len(args) > 1 && args[1] == "plan" {
				return "", fmt.Errorf("mock error running destroy-prep plan")
			}
			return "", nil
		}
		blueprint := createTestBlueprint()

		// When destroying
		err := stack.Destroy(blueprint, "local/path")

		// Then the prep-plan error path should be reported
		if !strings.Contains(err.Error(), "error running destroy-prep plan for") {
			t.Fatalf("Expected error to contain %q, got %q", "error running destroy-prep plan for", err.Error())
		}
	})

	t.Run("ErrorRunningTerraformPlanDestroy", func(t *testing.T) {
		// Given a stack whose shell fails on the second terraform plan (the destroy-pass plan -destroy)
		stack, mocks := setup(t)
		planCalls := 0
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" && len(args) > 1 && args[1] == "plan" {
				planCalls++
				if planCalls >= 2 {
					return "", fmt.Errorf("mock error running terraform plan destroy")
				}
			}
			return "", nil
		}
		blueprint := createTestBlueprint()

		// When destroying
		err := stack.Destroy(blueprint, "local/path")

		// Then the destroy-pass plan error path should be reported
		if !strings.Contains(err.Error(), "error running terraform plan destroy for") {
			t.Fatalf("Expected error to contain %q, got %q", "error running terraform plan destroy for", err.Error())
		}
	})

	t.Run("ErrorRunningTerraformDestroy", func(t *testing.T) {
		// Given a stack whose shell fails on terraform destroy
		stack, mocks := setup(t)
		mocks.Shell.ExecProgressWithEnvFunc = func(message string, command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" && len(args) > 1 && args[1] == "destroy" {
				return "", fmt.Errorf("mock error running terraform destroy")
			}
			return "", nil
		}
		blueprint := createTestBlueprint()

		// When destroying
		err := stack.Destroy(blueprint, "local/path")

		// Then an error should occur
		if !strings.Contains(err.Error(), "error running terraform destroy for") {
			t.Fatalf("Expected error to contain %q, got %q", "error running terraform destroy for", err.Error())
		}
	})

	t.Run("PassesRefreshArgsToRefreshCommand", func(t *testing.T) {
		// Given a stack whose provider returns RefreshArgs containing a var-file flag
		stack, mocks := setup(t)
		mocks.Runtime.TerraformProvider = &terraformRuntime.MockTerraformProvider{
			GetEnvVarsFunc: func(componentID string, interactive bool) (map[string]string, *terraformRuntime.TerraformArgs, error) {
				return map[string]string{}, &terraformRuntime.TerraformArgs{
					RefreshArgs: []string{"-var-file=secrets.tfvars"},
				}, nil
			},
		}
		var refreshArgs []string
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" && len(args) > 1 && args[1] == "refresh" {
				refreshArgs = args
			}
			return "", nil
		}
		blueprint := createTestBlueprint()

		// When destroying
		err := stack.Destroy(blueprint, "local/path")

		// Then no error should occur and RefreshArgs should be forwarded
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		found := false
		for _, arg := range refreshArgs {
			if arg == "-var-file=secrets.tfvars" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected refresh command to include -var-file=secrets.tfvars, got args: %v", refreshArgs)
		}
	})

	t.Run("SetsTFVarOperationToDestroy", func(t *testing.T) {
		// Given a stack and a blueprint with a local component
		stack, mocks := setup(t)
		blueprint := createTestBlueprint()

		// When Destroy is called, capture the env passed to terraform destroy
		var capturedEnv map[string]string
		mocks.Shell.ExecProgressWithEnvFunc = func(message string, command string, env map[string]string, args ...string) (string, error) {
			if len(args) > 1 && args[1] == "destroy" {
				capturedEnv = env
			}
			return "", nil
		}

		_ = stack.Destroy(blueprint, "local/path")

		// Then TF_VAR_operation is "destroy"
		if capturedEnv == nil {
			t.Fatal("Expected destroy to be invoked")
		}
		if capturedEnv["TF_VAR_operation"] != "destroy" {
			t.Errorf("Expected TF_VAR_operation to be %q, got %q", "destroy", capturedEnv["TF_VAR_operation"])
		}
	})

	t.Run("RunsPrepApplyBeforeDestroy", func(t *testing.T) {
		// Given a stack that records the ordered sequence of terraform subcommands with their TF_VAR_operation
		stack, mocks := setup(t)
		blueprint := createTestBlueprint()

		type step struct {
			subcommand string
			operation  string
		}
		var sequence []step
		record := func(env map[string]string, args []string) {
			if len(args) <= 1 {
				return
			}
			sequence = append(sequence, step{subcommand: args[1], operation: env["TF_VAR_operation"]})
		}
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" {
				record(env, args)
			}
			return "", nil
		}
		mocks.Shell.ExecProgressWithEnvFunc = func(message string, command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" {
				record(env, args)
			}
			return "", nil
		}

		// When destroying a single component
		if err := stack.Destroy(blueprint, "local/path"); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then the sequence starts with a full apply pass (init, plan, apply) followed by the
		// destroy pass (refresh, plan, destroy). TF_VAR_operation=destroy is forwarded to the
		// commands that include TF_VAR_* in their env (plan, refresh, destroy); init and apply
		// intentionally don't forward TF_VAR_* and rely on the process env set by windsor env.
		expected := []string{"init", "plan", "apply", "refresh", "plan", "destroy"}
		if len(sequence) < len(expected) {
			t.Fatalf("Expected at least %d terraform steps, got %d: %+v", len(expected), len(sequence), sequence)
		}
		for i, want := range expected {
			if sequence[i].subcommand != want {
				t.Errorf("step %d: subcommand = %q, want %q (full sequence: %+v)", i, sequence[i].subcommand, want, sequence)
			}
		}
		for i, s := range sequence[:len(expected)] {
			switch s.subcommand {
			case "plan", "refresh", "destroy":
				if s.operation != "destroy" {
					t.Errorf("step %d (%s): TF_VAR_operation = %q, want %q", i, s.subcommand, s.operation, "destroy")
				}
			}
		}
	})
}

func TestStack_PlanSummary(t *testing.T) {
	setup := func(t *testing.T) (*TerraformStack, *TerraformTestMocks) {
		t.Helper()
		mocks := setupWindsorStackMocks(t)
		stack := NewStack(mocks.Runtime).(*TerraformStack)
		stack.shims = mocks.Shims
		return stack, mocks
	}

	t.Run("ReturnsNilForNilBlueprint", func(t *testing.T) {
		// Given a stack with a valid runtime
		stack, _ := setup(t)

		// When PlanSummary is called with nil blueprint
		result := stack.PlanSummary(nil)

		// Then nil is returned
		if result != nil {
			t.Errorf("expected nil, got %v", result)
		}
	})

	t.Run("ParsesAddChangeDestroyFromPlanOutput", func(t *testing.T) {
		// Given a shell that returns a terraform plan line with counts
		stack, mocks := setup(t)
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if len(args) > 1 && args[1] == "plan" {
				return "Plan: 5 to add, 3 to change, 1 to destroy.\n", nil
			}
			return "", nil
		}

		// When PlanSummary is called
		results := stack.PlanSummary(createTestBlueprint())

		// Then two entries are returned with parsed counts for the plan-output component
		if len(results) != 2 {
			t.Fatalf("expected 2 results, got %d", len(results))
		}
		r := results[0]
		if r.Err != nil {
			t.Fatalf("expected no error, got %v", r.Err)
		}
		if r.Add != 5 || r.Change != 3 || r.Destroy != 1 {
			t.Errorf("expected +5 ~3 -1, got +%d ~%d -%d", r.Add, r.Change, r.Destroy)
		}
		if r.NoChanges {
			t.Errorf("expected NoChanges=false")
		}
	})

	t.Run("SetsNoChangesWhenTerraformReportsNoDiff", func(t *testing.T) {
		// Given a shell that returns a "No changes." line
		stack, mocks := setup(t)
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if len(args) > 1 && args[1] == "plan" {
				return "No changes. Infrastructure is up-to-date.\n", nil
			}
			return "", nil
		}

		// When PlanSummary is called
		results := stack.PlanSummary(createTestBlueprint())

		// Then the first result has NoChanges=true and zero counts
		if len(results) == 0 {
			t.Fatal("expected at least one result")
		}
		r := results[0]
		if r.Err != nil {
			t.Fatalf("expected no error, got %v", r.Err)
		}
		if !r.NoChanges {
			t.Errorf("expected NoChanges=true")
		}
		if r.Add != 0 || r.Change != 0 || r.Destroy != 0 {
			t.Errorf("expected zero counts, got +%d ~%d -%d", r.Add, r.Change, r.Destroy)
		}
	})

	t.Run("RecordsErrorWhenDirectoryMissing", func(t *testing.T) {
		// Given a stat shim that reports the component directory does not exist
		stack, mocks := setup(t)
		mocks.Shims.Stat = func(path string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		// When PlanSummary is called
		results := stack.PlanSummary(createTestBlueprint())

		// Then each result carries an error
		if len(results) == 0 {
			t.Fatal("expected results even on error")
		}
		for _, r := range results {
			if r.Err == nil {
				t.Errorf("expected error for component %q, got nil", r.ComponentID)
			}
		}
	})

	t.Run("RecordsErrorWhenTerraformInitFails", func(t *testing.T) {
		// Given a shell that returns an error on terraform init
		stack, mocks := setup(t)
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if len(args) > 1 && args[1] == "init" {
				return "", fmt.Errorf("init failed")
			}
			return "", nil
		}

		// When PlanSummary is called
		results := stack.PlanSummary(createTestBlueprint())

		// Then an error is recorded for the failing component
		if len(results) == 0 {
			t.Fatal("expected results")
		}
		if results[0].Err == nil {
			t.Errorf("expected init error on first component, got nil")
		}
	})

	t.Run("RecordsErrorWhenTerraformPlanFails", func(t *testing.T) {
		// Given a shell that returns an error on terraform plan
		stack, mocks := setup(t)
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if len(args) > 1 && args[1] == "plan" {
				return "", fmt.Errorf("plan failed")
			}
			return "", nil
		}

		// When PlanSummary is called
		results := stack.PlanSummary(createTestBlueprint())

		// Then errors are recorded and the summary still contains entries for all components
		if len(results) != 2 {
			t.Fatalf("expected 2 results, got %d", len(results))
		}
		for _, r := range results {
			if r.Err == nil {
				t.Errorf("expected plan error for %q, got nil", r.ComponentID)
			}
		}
	})

	t.Run("SetsTFVarOperationToApply", func(t *testing.T) {
		// Given a stack and a blueprint with a local component
		stack, mocks := setup(t)
		blueprint := createTestBlueprint()

		// When PlanSummary is called, capture the env passed to terraform plan
		var capturedEnv map[string]string
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if len(args) > 1 && args[1] == "plan" {
				capturedEnv = env
			}
			return "", nil
		}

		_ = stack.PlanSummary(blueprint)

		// Then TF_VAR_operation is "apply"
		if capturedEnv == nil {
			t.Fatal("Expected plan to be invoked")
		}
		if capturedEnv["TF_VAR_operation"] != "apply" {
			t.Errorf("Expected TF_VAR_operation to be %q, got %q", "apply", capturedEnv["TF_VAR_operation"])
		}
	})
}

func TestParseTerraformPlanCounts(t *testing.T) {
	t.Run("ParsesAllThreeCounts", func(t *testing.T) {
		// Given standard terraform plan output
		output := "Plan: 5 to add, 3 to change, 1 to destroy.\n"

		// When parsed
		add, change, destroy, noChanges := parseTerraformPlanCounts(output)

		// Then counts are set correctly
		if add != 5 || change != 3 || destroy != 1 {
			t.Errorf("expected +5 ~3 -1, got +%d ~%d -%d", add, change, destroy)
		}
		if noChanges {
			t.Error("expected noChanges=false")
		}
	})

	t.Run("SetsNoChangesForNoChangesLine", func(t *testing.T) {
		// Given "No changes." output
		output := "No changes. Infrastructure is up-to-date.\n"

		// When parsed
		_, _, _, noChanges := parseTerraformPlanCounts(output)

		// Then noChanges is true
		if !noChanges {
			t.Error("expected noChanges=true")
		}
	})

	t.Run("LeavesCountsAtZeroForUnrecognisedOutput", func(t *testing.T) {
		// Given unrecognised output
		output := "Something unexpected happened.\n"

		// When parsed
		add, change, destroy, noChanges := parseTerraformPlanCounts(output)

		// Then all values are zero/false
		if add != 0 || change != 0 || destroy != 0 || noChanges {
			t.Errorf("expected all zero/false, got add=%d change=%d destroy=%d noChanges=%v", add, change, destroy, noChanges)
		}
	})
}

