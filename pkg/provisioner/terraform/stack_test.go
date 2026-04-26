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

	t.Run("ReturnsPartialSkippedListAlongsideError", func(t *testing.T) {
		// Given a two-component blueprint where the first component's directory is missing
		// (will be appended to skipped) and the second component's terraform init fails.
		// MigrateState must return BOTH: the skipped list so far AND the error, so a
		// caller (e.g. bootstrap) can render "skipped A, then B failed" in a single
		// diagnostic rather than losing the skip signal on the error path.
		stack, mocks := setup(t)
		blueprint := createTestBlueprint()

		statCallsByPath := make(map[string]int)
		originalStat := mocks.Shims.Stat
		firstSeen := ""
		mocks.Shims.Stat = func(path string) (os.FileInfo, error) {
			// The MigrateState loop calls Stat once on component.FullPath per component.
			// Treat the first unique path seen as the "missing" one to get a skip; every
			// other path falls through to the default behavior so init can run against it.
			if strings.HasSuffix(path, "backend_override.tf") {
				return originalStat(path)
			}
			statCallsByPath[path]++
			if firstSeen == "" {
				firstSeen = path
			}
			if path == firstSeen {
				return nil, os.ErrNotExist
			}
			return originalStat(path)
		}

		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if len(args) >= 2 && args[1] == "init" {
				return "", fmt.Errorf("backend not reachable")
			}
			return "", nil
		}

		skipped, err := stack.MigrateState(blueprint)

		// Then the error from the second component surfaces AND the skip from the first
		// survives — callers that only looked at err previously would have silently lost
		// the fact that something upstream was already missing
		if err == nil {
			t.Fatal("Expected MigrateState to return an error")
		}
		if len(skipped) == 0 {
			t.Error("Expected skipped list to include the first component whose directory was missing")
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

func TestStack_MigrateComponentState(t *testing.T) {
	setup := func(t *testing.T) (*TerraformStack, *TerraformTestMocks) {
		t.Helper()
		mocks := setupWindsorStackMocks(t)
		stack := NewStack(mocks.Runtime).(*TerraformStack)
		stack.shims = mocks.Shims
		return stack, mocks
	}

	t.Run("ReturnsErrorForNilBlueprint", func(t *testing.T) {
		stack, _ := setup(t)
		if err := stack.MigrateComponentState(nil, "backend"); err == nil {
			t.Fatal("Expected error for nil blueprint")
		}
	})

	t.Run("ReturnsErrorForEmptyComponentID", func(t *testing.T) {
		stack, _ := setup(t)
		if err := stack.MigrateComponentState(createTestBlueprint(), ""); err == nil {
			t.Fatal("Expected error for empty component ID")
		}
	})

	t.Run("ReturnsErrorWhenComponentNotFound", func(t *testing.T) {
		// Given a blueprint that does not declare the named component, the per-
		// component variant must fail loudly rather than silently no-op — the caller
		// (bootstrap) explicitly asked for a specific component to be migrated.
		stack, _ := setup(t)
		err := stack.MigrateComponentState(createTestBlueprint(), "nonexistent")
		if err == nil {
			t.Fatal("Expected error when component is not in the blueprint")
		}
		if !strings.Contains(err.Error(), "nonexistent") {
			t.Errorf("Expected error to name the missing component, got: %v", err)
		}
	})

	t.Run("RunsInitWithMigrateStateForOneComponent", func(t *testing.T) {
		// Given a two-component blueprint and a request to migrate only the second
		// component, exactly one terraform init -migrate-state must fire and only
		// against that component's directory.
		stack, mocks := setup(t)
		blueprint := createTestBlueprint()
		targetID := blueprint.TerraformComponents[1].GetID()

		var migrateStateInits int
		var initChdirs []string
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if len(args) >= 2 && args[1] == "init" {
				for _, a := range args {
					if a == "-migrate-state" {
						migrateStateInits++
						break
					}
				}
				if strings.HasPrefix(args[0], "-chdir=") {
					initChdirs = append(initChdirs, strings.TrimPrefix(args[0], "-chdir="))
				}
			}
			return "", nil
		}

		if err := stack.MigrateComponentState(blueprint, targetID); err != nil {
			t.Fatalf("Expected MigrateComponentState to succeed, got %v", err)
		}
		if migrateStateInits != 1 {
			t.Errorf("Expected 1 migrate-state init for the targeted component, got %d", migrateStateInits)
		}
		// targetID is the blueprint Path (forward slashes) but FullPath runs through
		// filepath.FromSlash, so initChdirs[0] is OS-native (backslashes on Windows).
		// Normalize before substring matching so the assertion is portable.
		if len(initChdirs) != 1 || !strings.Contains(filepath.ToSlash(initChdirs[0]), targetID) {
			t.Errorf("Expected init to run against the targeted component dir, got %v", initChdirs)
		}
	})

	t.Run("ReturnsErrorWhenComponentDirectoryMissing", func(t *testing.T) {
		// Given the targeted component's directory is missing on disk, the per-
		// component variant treats this as an error (unlike MigrateState's bulk
		// loop, which collects skipped IDs). The single-component caller asked for
		// a specific migration and got nothing — silent skip would leave the caller
		// believing the migration succeeded.
		stack, mocks := setup(t)
		blueprint := createTestBlueprint()
		targetID := blueprint.TerraformComponents[0].GetID()

		mocks.Shims.Stat = func(path string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		err := stack.MigrateComponentState(blueprint, targetID)
		if err == nil {
			t.Fatal("Expected error when component directory is missing")
		}
		if !strings.Contains(err.Error(), "does not exist") {
			t.Errorf("Expected error to mention missing directory, got %v", err)
		}
	})

	t.Run("ReturnsErrorWhenInitFails", func(t *testing.T) {
		stack, mocks := setup(t)
		blueprint := createTestBlueprint()
		targetID := blueprint.TerraformComponents[0].GetID()

		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if len(args) >= 2 && args[1] == "init" {
				return "", fmt.Errorf("backend not reachable")
			}
			return "", nil
		}

		err := stack.MigrateComponentState(blueprint, targetID)
		if err == nil {
			t.Fatal("Expected error when terraform init fails")
		}
		if !strings.Contains(err.Error(), "backend not reachable") {
			t.Errorf("Expected error to wrap the underlying cause, got %v", err)
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

		// Default `terraform show -json` to return state JSON containing one
		// resource AND a plan JSON containing an "update" action, so the full
		// flow runs: classifyDestroyAction sees non-empty state, then a plan
		// with updates, routes to PrepThenDestroy. Tests that want to verify
		// the Noop or DestroyOnly branches override this themselves.
		// Distinguish state-show vs plan-show by len(args): `show -json` is 3
		// args, `show -json <planpath>` is 4.
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" && len(args) >= 3 && args[1] == "show" && args[2] == "-json" {
				if len(args) == 3 {
					return `{"values":{"root_module":{"resources":[{"address":"aws_s3_bucket.example"}]}}}`, nil
				}
				return `{"resource_changes":[{"change":{"actions":["update"]}}]}`, nil
			}
			return "", nil
		}

		return stack, mocks
	}

	t.Run("Success", func(t *testing.T) {
		stack, _ := setup(t)
		blueprint := createTestBlueprint()

		if _, err := stack.DestroyAll(blueprint); err != nil {
			t.Errorf("Expected Down to return nil, got %v", err)
		}
	})

	t.Run("ErrorGettingCurrentDirectory", func(t *testing.T) {
		stack, mocks := setup(t)
		mocks.Shims.Getwd = func() (string, error) {
			return "", fmt.Errorf("mock error getting current directory")
		}

		blueprint := createTestBlueprint()
		_, err := stack.DestroyAll(blueprint)
		expectedError := "error getting current directory"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ReportsMissingDirectoriesAsSkipped", func(t *testing.T) {
		// Given a stat shim that reports every component directory as missing
		stack, mocks := setup(t)
		mocks.Shims.Stat = func(path string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		// When DestroyAll runs against a blueprint with two components
		blueprint := createTestBlueprint()
		skipped, err := stack.DestroyAll(blueprint)

		// Then no error is returned and both components appear in the skipped list,
		// matching MigrateState's contract for missing-on-disk components.
		if err != nil {
			t.Fatalf("Expected no error when directory doesn't exist, got %v", err)
		}
		if len(skipped) != len(blueprint.TerraformComponents) {
			t.Fatalf("Expected %d skipped components, got %d: %v", len(blueprint.TerraformComponents), len(skipped), skipped)
		}
	})

	t.Run("SkipsComponentsListedInExcludeIDs", func(t *testing.T) {
		// Given a blueprint with multiple components and DestroyAll called with one
		// of their IDs in excludeIDs, the named component must not run terraform
		// init/destroy at all — symmetric-destroy peels the backend off the bulk
		// pass at the cmd layer and destroys it last after migrating its state.
		stack, mocks := setup(t)
		mocks.Runtime.TerraformProvider.ClearCache()

		projectRoot := os.Getenv("WINDSOR_PROJECT_ROOT")
		contextName := mocks.Runtime.ContextName
		blueprint := createTestBlueprint()
		blueprint.TerraformComponents = []blueprintv1alpha1.TerraformComponent{
			{Source: "source1", Path: "backend"},
			{Source: "source1", Path: "cluster"},
		}

		for _, p := range []string{"backend", "cluster"} {
			dir := filepath.Join(projectRoot, ".windsor", "contexts", contextName, "terraform", p)
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatalf("Failed to create directory %s: %v", dir, err)
			}
		}

		var terraformCommands []string
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" {
				terraformCommands = append(terraformCommands, strings.Join(args, " "))
			}
			if command == "terraform" && len(args) >= 3 && args[1] == "show" && args[2] == "-json" {
				return `{"values":{"root_module":{"resources":[{"address":"aws_s3_bucket.example"}]}}}`, nil
			}
			return "", nil
		}

		if _, err := stack.DestroyAll(blueprint, "backend"); err != nil {
			t.Errorf("Expected DestroyAll to return nil, got %v", err)
		}

		for _, c := range terraformCommands {
			if strings.Contains(c, "backend") && !strings.Contains(c, "cluster") {
				t.Errorf("Expected no terraform commands for excluded \"backend\" component, but found: %q", c)
			}
		}
		if len(terraformCommands) == 0 {
			t.Errorf("Expected terraform commands for non-excluded \"cluster\" component, got none")
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
			if command == "terraform" && len(args) >= 3 && args[1] == "show" && args[2] == "-json" {
				if len(args) == 3 {
					return `{"values":{"root_module":{"resources":[{"address":"aws_s3_bucket.example"}]}}}`, nil
				}
				return `{"resource_changes":[{"change":{"actions":["update"]}}]}`, nil
			}
			return "", nil
		}

		if _, err := stack.DestroyAll(blueprint); err != nil {
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
			if command == "terraform" && len(args) >= 3 && args[1] == "show" && args[2] == "-json" {
				if len(args) == 3 {
					return `{"values":{"root_module":{"resources":[{"address":"aws_s3_bucket.example"}]}}}`, nil
				}
				return `{"resource_changes":[{"change":{"actions":["update"]}}]}`, nil
			}
			return "", nil
		}

		if _, err := stack.DestroyAll(blueprint); err != nil {
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
		// "Destroying <path>" progress span is the only label the user sees. state list
		// is seeded non-empty so the idempotency short-circuit does not fire. The mock
		// returns a recognizable terraform diagnostic alongside the error to verify it
		// is surfaced via warningWriter rather than discarded.
		stack, mocks := setup(t)
		var captured strings.Builder
		stack.warningWriter = &captured

		const tfDiagnostic = "Error: deleting S3 Bucket (terraform-state-x): operation error S3: BucketNotEmpty"
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" && len(args) >= 3 && args[1] == "show" && args[2] == "-json" {
				if len(args) == 3 {
					return `{"values":{"root_module":{"resources":[{"address":"aws_s3_bucket.example"}]}}}`, nil
				}
				return `{"resource_changes":[{"change":{"actions":["update"]}}]}`, nil
			}
			if command == "terraform" && len(args) > 0 && strings.HasPrefix(args[0], "-chdir=") && len(args) > 1 && args[1] == "destroy" {
				return tfDiagnostic, fmt.Errorf("mock error running terraform destroy")
			}
			return "", nil
		}

		blueprint := createTestBlueprint()
		_, err := stack.DestroyAll(blueprint)
		expectedError := "error running terraform destroy for"
		if err == nil {
			t.Fatalf("Expected destroy error, got nil")
		}
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
		warningOutput := captured.String()
		if !strings.Contains(warningOutput, tfDiagnostic) {
			t.Errorf("Expected terraform diagnostic %q to be surfaced via warningWriter, got %q", tfDiagnostic, warningOutput)
		}
	})

	t.Run("NilBlueprint", func(t *testing.T) {
		stack, _ := setup(t)
		_, err := stack.DestroyAll(nil)
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
		_, err := stack.DestroyAll(blueprint)
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
		_, err := stack.DestroyAll(blueprint)

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

		if _, err := stack.DestroyAll(blueprint); err != nil {
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

		if _, err := stack.DestroyAll(blueprint); err != nil {
			t.Errorf("Expected Down to succeed with named component with source, got %v", err)
		}
	})

	t.Run("SetsTFVarOperationToDestroy", func(t *testing.T) {
		// Given a stack and a blueprint; state list is seeded non-empty so the
		// idempotency short-circuit does not fire.
		stack, mocks := setup(t)
		blueprint := createTestBlueprint()

		// When DestroyAll is called, capture the env passed to terraform destroy.
		// All destroy-phase subcommands run via ExecSilentWithEnv now.
		var capturedEnv map[string]string
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" && len(args) >= 3 && args[1] == "show" && args[2] == "-json" {
				if len(args) == 3 {
					return `{"values":{"root_module":{"resources":[{"address":"aws_s3_bucket.example"}]}}}`, nil
				}
				return `{"resource_changes":[{"change":{"actions":["update"]}}]}`, nil
			}
			if len(args) > 1 && args[1] == "destroy" {
				capturedEnv = env
			}
			return "", nil
		}

		_, _ = stack.DestroyAll(blueprint)

		// Then TF_VAR_operation is "destroy"
		if capturedEnv == nil {
			t.Fatal("Expected destroy to be invoked")
		}
		if capturedEnv["TF_VAR_operation"] != "destroy" {
			t.Errorf("Expected TF_VAR_operation to be %q, got %q", "destroy", capturedEnv["TF_VAR_operation"])
		}
	})

	t.Run("RunsRefreshGatedDestroyPerComponentInReverseOrder", func(t *testing.T) {
		// Given a stack that records the per-component terraform steps in order, and
		// state JSON reports a resource so the idempotency short-circuit does not fire.
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
			// Normalize OS-native separators to forward slashes before matching: FullPath
			// goes through filepath.FromSlash, so on Windows args[0] is "-chdir=...\remote\path"
			// and the literal substring "remote/path" never matches.
			normalized := filepath.ToSlash(args[0])
			switch {
			case strings.Contains(normalized, "remote/path"):
				return "remote/path"
			case strings.Contains(normalized, "local/path"):
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
			if command == "terraform" && len(args) >= 3 && args[1] == "show" && args[2] == "-json" && len(args) == 3 {
				return `{"values":{"root_module":{"resources":[{"address":"aws_s3_bucket.example"}]}}}`, nil
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
		if _, err := stack.DestroyAll(blueprint); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then components are walked in reverse dependency order, and each component is
		// taken through the idempotent destroy flow end-to-end before the next one begins:
		// init → show (pre-refresh state JSON) → refresh → show (post-refresh state JSON) →
		// destroy. The pre-refresh show-json is load-bearing for the empty-state skip; refresh
		// reconciles state with cloud reality for partial-destroy cases; the post-refresh
		// show-json drives the second skip check. TF_VAR_operation=destroy is forwarded to
		// commands that include TF_VAR_* in their env (show, refresh, destroy); init does
		// not forward TF_VAR_* and relies on the process env set by windsor env.
		type ordered struct {
			component  string
			subcommand string
		}
		expected := []ordered{
			{component: "local/path", subcommand: "init"},
			{component: "local/path", subcommand: "show"},
			{component: "local/path", subcommand: "refresh"},
			{component: "local/path", subcommand: "show"},
			{component: "local/path", subcommand: "destroy"},
			{component: "remote/path", subcommand: "init"},
			{component: "remote/path", subcommand: "show"},
			{component: "remote/path", subcommand: "refresh"},
			{component: "remote/path", subcommand: "show"},
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
			case "refresh", "show", "destroy":
				if sequence[i].operation != "destroy" {
					t.Errorf("step %d (%s %s): TF_VAR_operation = %q, want %q", i, sequence[i].component, sequence[i].subcommand, sequence[i].operation, "destroy")
				}
			}
		}
	})

	t.Run("RefreshFailureFallsThroughToDestroyWithRefreshTrue", func(t *testing.T) {
		// Given a non-empty-state component in a bulk destroy where refresh fails — same
		// fallback contract as single-component Destroy. A transient refresh failure on one
		// component must not abort the whole bulk pass; we fall through to `terraform destroy
		// -refresh=true` for that component and continue iterating the rest in reverse order.
		stack, mocks := setup(t)
		blueprint := createTestBlueprint()

		// Capture warnings via the injected writer rather than redirecting os.Stderr
		// (the TUI spinner shares os.Stderr; pipe-based redirect deadlocks on Windows).
		var captured strings.Builder
		stack.warningWriter = &captured

		var destroyArgsByComponent = map[string][]string{}
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			componentOf := func() string {
				if len(args) == 0 {
					return ""
				}
				// Normalize OS-native separators (Windows uses \) before matching.
				normalized := filepath.ToSlash(args[0])
				switch {
				case strings.Contains(normalized, "remote/path"):
					return "remote/path"
				case strings.Contains(normalized, "local/path"):
					return "local/path"
				}
				return ""
			}
			if command == "terraform" && len(args) >= 3 && args[1] == "show" && args[2] == "-json" && len(args) == 3 {
				return `{"values":{"root_module":{"resources":[{"address":"aws_s3_bucket.example"}]}}}`, nil
			}
			if command == "terraform" && len(args) > 1 && args[1] == "refresh" {
				if componentOf() == "local/path" {
					return "", fmt.Errorf("mock refresh failure for local/path")
				}
				return "", nil
			}
			if command == "terraform" && len(args) > 1 && args[1] == "destroy" {
				destroyArgsByComponent[componentOf()] = append([]string(nil), args...)
				return "", nil
			}
			return "", nil
		}

		skipped, err := stack.DestroyAll(blueprint)
		warningOutput := captured.String()

		if err != nil {
			t.Fatalf("Expected DestroyAll to tolerate refresh failure on one component, got %v", err)
		}
		if len(skipped) != 0 {
			t.Errorf("Expected no skipped components (state was non-empty), got %v", skipped)
		}

		// local/path (refresh failed) must have used -refresh=true on destroy.
		localArgs, ok := destroyArgsByComponent["local/path"]
		if !ok {
			t.Fatal("Expected destroy to be called for local/path despite refresh failure")
		}
		hasRefreshTrue := false
		for _, a := range localArgs {
			if a == "-refresh=true" {
				hasRefreshTrue = true
			}
			if a == "-refresh=false" {
				t.Errorf("local/path: expected -refresh=true on refresh-fallback path, got -refresh=false in %v", localArgs)
			}
		}
		if !hasRefreshTrue {
			t.Errorf("local/path: expected destroy args to include -refresh=true, got %v", localArgs)
		}

		// remote/path (refresh succeeded) must have used -refresh=false on destroy.
		remoteArgs, ok := destroyArgsByComponent["remote/path"]
		if !ok {
			t.Fatal("Expected destroy to be called for remote/path")
		}
		hasRefreshFalse := false
		for _, a := range remoteArgs {
			if a == "-refresh=false" {
				hasRefreshFalse = true
			}
		}
		if !hasRefreshFalse {
			t.Errorf("remote/path: expected -refresh=false on happy path, got %v", remoteArgs)
		}

		// Warning must surface for the failed component only — silent fallback would hide
		// a recurring credential or connectivity issue across a bulk destroy.
		if !strings.Contains(warningOutput, "warning: terraform refresh failed for local/path") {
			t.Errorf("Expected warning for failed component, got: %q", warningOutput)
		}
		if !strings.Contains(warningOutput, "mock refresh failure for local/path") {
			t.Errorf("Expected warning to include underlying refresh error, got: %q", warningOutput)
		}
		if strings.Contains(warningOutput, "warning: terraform refresh failed for remote/path") {
			t.Errorf("No warning expected for remote/path (refresh succeeded), got: %q", warningOutput)
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

		// Default `terraform show -json` to return state JSON containing one
		// resource AND a plan JSON containing an "update" action, so the full
		// flow runs: classifyDestroyAction sees non-empty state, then a plan
		// with updates, routes to PrepThenDestroy. Tests that want to verify
		// the Noop or DestroyOnly branches override this themselves.
		// Distinguish state-show vs plan-show by len(args): `show -json` is 3
		// args, `show -json <planpath>` is 4.
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" && len(args) >= 3 && args[1] == "show" && args[2] == "-json" {
				if len(args) == 3 {
					return `{"values":{"root_module":{"resources":[{"address":"aws_s3_bucket.example"}]}}}`, nil
				}
				return `{"resource_changes":[{"change":{"actions":["update"]}}]}`, nil
			}
			return "", nil
		}
		return stack, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a stack and a blueprint with a local component
		stack, _ := setup(t)
		blueprint := createTestBlueprint()

		// When destroying the local component by ID
		_, err := stack.Destroy(blueprint, "local/path")

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected Destroy to return nil, got %v", err)
		}
	})

	t.Run("NilBlueprint", func(t *testing.T) {
		// Given a stack with a nil blueprint
		stack, _ := setup(t)

		// When destroying with nil blueprint
		_, err := stack.Destroy(nil, "local/path")

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
		_, err := stack.Destroy(blueprint, "")

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
		_, err := stack.Destroy(blueprint, "does/not/exist")

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
		_, err := stack.Destroy(blueprint, "local/path")

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
		_, err := stack.Destroy(blueprint, "local/path")

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
		_, err := stack.Destroy(blueprint, "local/path")

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
		_, err := stack.Destroy(blueprint, "local/path")

		// Then an error should occur
		if !strings.Contains(err.Error(), "error running terraform init for") {
			t.Fatalf("Expected error to contain %q, got %q", "error running terraform init for", err.Error())
		}
	})

	t.Run("ErrorRunningTerraformDestroy", func(t *testing.T) {
		// Given a stack whose shell fails on terraform destroy. The single Destroy
		// method uses ExecSilentWithEnv inside a tui.WithProgress wrapper to mirror
		// the bulk DestroyAll loop body. The mock returns a recognizable terraform
		// diagnostic alongside the error to verify it is surfaced via warningWriter.
		stack, mocks := setup(t)
		var captured strings.Builder
		stack.warningWriter = &captured

		const tfDiagnostic = "Error: deleting EC2 Instance (i-abc123): InvalidInstanceID.NotFound"
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" && len(args) > 1 && args[1] == "destroy" {
				return tfDiagnostic, fmt.Errorf("mock error running terraform destroy")
			}
			if command == "terraform" && len(args) > 2 && args[1] == "show" && args[2] == "-json" {
				return `{"values":{"root_module":{"resources":[{"address":"aws_s3_bucket.example"}]}}}`, nil
			}
			return "", nil
		}
		blueprint := createTestBlueprint()

		// When destroying
		_, err := stack.Destroy(blueprint, "local/path")

		// Then an error should occur
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error running terraform destroy for") {
			t.Fatalf("Expected error to contain %q, got %q", "error running terraform destroy for", err.Error())
		}
		warningOutput := captured.String()
		if !strings.Contains(warningOutput, tfDiagnostic) {
			t.Errorf("Expected terraform diagnostic %q to be surfaced via warningWriter, got %q", tfDiagnostic, warningOutput)
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
			if command == "terraform" && len(args) >= 3 && args[1] == "show" && args[2] == "-json" {
				if len(args) == 3 {
					return `{"values":{"root_module":{"resources":[{"address":"aws_s3_bucket.example"}]}}}`, nil
				}
				return `{"resource_changes":[{"change":{"actions":["update"]}}]}`, nil
			}
			if command == "terraform" && len(args) > 1 && args[1] == "refresh" {
				refreshArgs = args
			}
			return "", nil
		}
		blueprint := createTestBlueprint()

		// When destroying
		_, err := stack.Destroy(blueprint, "local/path")

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
		// Given a stack and a blueprint with a local component. The single Destroy
		// method now uses ExecSilentWithEnv inside a tui.WithProgress wrapper to
		// match the bulk DestroyAll loop body, so destroy invocations and the
		// state-show JSON both flow through ExecSilentWithEnvFunc.
		stack, mocks := setup(t)
		blueprint := createTestBlueprint()

		var capturedEnv map[string]string
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" && len(args) >= 3 && args[1] == "show" && args[2] == "-json" {
				return `{"values":{"root_module":{"resources":[{"address":"aws_s3_bucket.example"}]}}}`, nil
			}
			if len(args) > 1 && args[1] == "destroy" {
				capturedEnv = env
			}
			return "", nil
		}

		_, _ = stack.Destroy(blueprint, "local/path")

		// Then TF_VAR_operation is "destroy"
		if capturedEnv == nil {
			t.Fatal("Expected destroy to be invoked")
		}
		if capturedEnv["TF_VAR_operation"] != "destroy" {
			t.Errorf("Expected TF_VAR_operation to be %q, got %q", "destroy", capturedEnv["TF_VAR_operation"])
		}
	})

	t.Run("RunsRefreshGatedDestroy", func(t *testing.T) {
		// Given a stack that records the ordered sequence of terraform subcommands with
		// their TF_VAR_operation; state JSON reports a resource so the idempotency
		// short-circuit does not fire.
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
			if command == "terraform" && len(args) >= 3 && args[1] == "show" && args[2] == "-json" {
				if len(args) == 3 {
					return `{"values":{"root_module":{"resources":[{"address":"aws_s3_bucket.example"}]}}}`, nil
				}
				return `{"resource_changes":[{"change":{"actions":["update"]}}]}`, nil
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
		if _, err := stack.Destroy(blueprint, "local/path"); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then the sequence is the idempotent destroy flow end-to-end: init, show
		// (pre-refresh state JSON), refresh, show (post-refresh state JSON), destroy. The
		// pre-refresh show-json drives the empty-state skip; refresh reconciles state with
		// reality for partial-destroy reconciliation; the post-refresh show-json drives the
		// second skip check. No intermediate plan or apply runs; terraform destroy plans
		// internally. TF_VAR_operation=destroy is forwarded to commands that include
		// TF_VAR_* in their env (show, refresh, destroy); init does not forward TF_VAR_*
		// and relies on the process env.
		expected := []string{"init", "show", "refresh", "show", "destroy"}
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
			case "refresh", "show", "destroy":
				if s.operation != "destroy" {
					t.Errorf("step %d (%s): TF_VAR_operation = %q, want %q", i, s.subcommand, s.operation, "destroy")
				}
			}
		}
	})

	t.Run("DetectsResourcesInChildModules", func(t *testing.T) {
		// Given state JSON where all resources live under root_module.child_modules
		// instead of root_module.resources — windsor's canonical shape, because every
		// windsor blueprint wraps its resources in a `module "main"` block. An earlier
		// version only checked root_module.resources and so collapsed every windsor
		// destroy into a no-op (the real-world bug that motivated this test). The fix
		// walks the module tree recursively in tfStateModule.hasResources.
		stack, mocks := setup(t)
		blueprint := createTestBlueprint()

		destroyCalled := false
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" && len(args) >= 3 && args[1] == "show" && args[2] == "-json" && len(args) == 3 {
				// State JSON with resources only in a child module — no root-level
				// resources at all. This is what windsor's terraform actually emits.
				return `{"values":{"root_module":{"child_modules":[{"address":"module.main","resources":[{"address":"module.main.aws_s3_bucket.this"}]}]}}}`, nil
			}
			if command == "terraform" && len(args) > 1 && args[1] == "destroy" {
				destroyCalled = true
			}
			return "", nil
		}

		if _, err := stack.Destroy(blueprint, "local/path"); err != nil {
			t.Fatalf("Expected destroy to succeed against child-module state, got %v", err)
		}
		if !destroyCalled {
			t.Error("Expected destroy to run when child-module state has resources — the root-only check would miss them and short-circuit to noop")
		}
	})

	t.Run("DetectsResourcesInDeeplyNestedModules", func(t *testing.T) {
		// Given state JSON where resources live two levels deep (module "main" → module
		// "nested") — the recursion must cover arbitrary depth, not just windsor's
		// canonical one-level convention, so blueprint compositions that nest modules
		// still classify correctly.
		stack, mocks := setup(t)
		blueprint := createTestBlueprint()

		destroyCalled := false
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" && len(args) >= 3 && args[1] == "show" && args[2] == "-json" {
				if len(args) == 3 {
					return `{"values":{"root_module":{"child_modules":[{"address":"module.main","child_modules":[{"address":"module.main.module.nested","resources":[{"address":"module.main.module.nested.aws_s3_bucket.x"}]}]}]}}}`, nil
				}
				return `{"resource_changes":[{"change":{"actions":["update"]}}]}`, nil
			}
			if command == "terraform" && len(args) > 1 && args[1] == "destroy" {
				destroyCalled = true
			}
			return "", nil
		}

		if _, err := stack.Destroy(blueprint, "local/path"); err != nil {
			t.Fatalf("Expected destroy to succeed against deeply-nested state, got %v", err)
		}
		if !destroyCalled {
			t.Error("Expected destroy to run when nested-module state has resources")
		}
	})

	t.Run("ShortCircuitsWhenStateJSONHasNoResources", func(t *testing.T) {
		// Given a component whose `terraform show -json` returns state with an empty
		// root_module.resources array — the component has already been destroyed or was
		// never applied, and there is nothing left to tear down. This is the idempotency
		// case: re-running destroy must not fail just because the previous destroy
		// already did the job.
		stack, mocks := setup(t)
		blueprint := createTestBlueprint()

		planCalled := false
		applyCalled := false
		destroyCalled := false
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" && len(args) >= 3 && args[1] == "show" && args[2] == "-json" {
				// State JSON with an empty resources array — the whole point.
				return `{"values":{"root_module":{"resources":[]}}}`, nil
			}
			if command == "terraform" && len(args) > 1 {
				switch args[1] {
				case "plan":
					planCalled = true
				case "apply":
					applyCalled = true
				}
			}
			return "", nil
		}
		mocks.Shell.ExecProgressWithEnvFunc = func(message string, command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" && len(args) > 1 && args[1] == "destroy" {
				destroyCalled = true
			}
			return "", nil
		}

		if _, err := stack.Destroy(blueprint, "local/path"); err != nil {
			t.Fatalf("Expected destroy against empty state to succeed (idempotent), got %v", err)
		}

		// Then the plan/apply/destroy steps are all skipped — classifyDestroyAction
		// returned Noop and we returned immediately.
		if planCalled {
			t.Error("Expected classification plan to be skipped when state is empty")
		}
		if applyCalled {
			t.Error("Expected prep-apply to be skipped when state is empty")
		}
		if destroyCalled {
			t.Error("Expected destroy to be skipped when state is empty")
		}
	})

	t.Run("TreatsMissingValuesNodeAsEmpty", func(t *testing.T) {
		// Given a component whose `terraform show -json` returns state JSON where the
		// top-level "values" field is absent — this is what terraform emits when there
		// is literally no state file at all (never-applied component). The JSON is valid
		// but sparse; the typed decode against tfState should collapse this into the
		// Noop path, not a parse error.
		stack, mocks := setup(t)
		blueprint := createTestBlueprint()

		destroyCalled := false
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" && len(args) >= 3 && args[1] == "show" && args[2] == "-json" {
				return `{"format_version":"1.0"}`, nil
			}
			return "", nil
		}
		mocks.Shell.ExecProgressWithEnvFunc = func(message string, command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" && len(args) > 1 && args[1] == "destroy" {
				destroyCalled = true
			}
			return "", nil
		}

		if _, err := stack.Destroy(blueprint, "local/path"); err != nil {
			t.Fatalf("Expected never-applied state to be treated as empty, got %v", err)
		}
		if destroyCalled {
			t.Error("Expected destroy to be skipped when state values node is absent")
		}
	})

	t.Run("PassesRefreshFalseToDestroyToAvoidDoubleRefresh", func(t *testing.T) {
		// Given a normal destroy path (state non-empty). refreshComponentState just ran
		// before the destroy call, so terraform destroy's internal refresh would be
		// redundant — twice the provider API load and twice the "Refreshing state..."
		// output per component. We pass -refresh=false to skip terraform's internal
		// refresh; state is already current.
		stack, mocks := setup(t)
		blueprint := createTestBlueprint()

		var destroyArgs []string
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" && len(args) >= 3 && args[1] == "show" && args[2] == "-json" {
				return `{"values":{"root_module":{"resources":[{"address":"aws_s3_bucket.example"}]}}}`, nil
			}
			if command == "terraform" && len(args) > 1 && args[1] == "destroy" {
				destroyArgs = args
			}
			return "", nil
		}

		if _, err := stack.Destroy(blueprint, "local/path"); err != nil {
			t.Fatalf("Expected destroy to succeed, got %v", err)
		}
		if destroyArgs == nil {
			t.Fatal("Expected destroy to be invoked")
		}
		found := false
		for _, a := range destroyArgs {
			if a == "-refresh=false" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected destroy invocation to include -refresh=false to skip the redundant internal refresh, got args: %v", destroyArgs)
		}
	})

	t.Run("NeverRunsApplyDuringDestroy", func(t *testing.T) {
		// Given a normal destroy path (state has resources) — apply must never run
		// during destroy. Previous designs ran a prep-apply; that was unsafe because
		// a regular plan after refresh can include "create" actions for resources
		// dropped from state, which prep-apply would recreate right before destroy.
		// Destroy must go straight from state-check to terraform destroy; no apply.
		stack, mocks := setup(t)
		blueprint := createTestBlueprint()

		applyCalled := false
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" && len(args) >= 3 && args[1] == "show" && args[2] == "-json" && len(args) == 3 {
				return `{"values":{"root_module":{"resources":[{"address":"aws_s3_bucket.example"}]}}}`, nil
			}
			if command == "terraform" && len(args) > 1 && args[1] == "apply" {
				applyCalled = true
			}
			return "", nil
		}

		if _, err := stack.Destroy(blueprint, "local/path"); err != nil {
			t.Fatalf("Expected destroy to succeed, got %v", err)
		}
		if applyCalled {
			t.Error("Expected apply never to run during destroy — prep-apply is unsafe because it can recreate resources dropped by refresh")
		}
	})

	t.Run("RefreshFailureFallsThroughToDestroyWithRefreshTrue", func(t *testing.T) {
		// Given a component with non-empty state where terraform refresh fails (transient
		// network blip, credential rotation, provider API hiccup), refresh failure must not
		// make a live component undestroyable. The pre-refresh check confirmed state is
		// non-empty, so we know there is something to destroy. Destroy must fall through to
		// `terraform destroy -refresh=true` so terraform's own refresh has a second shot.
		// The post-refresh state-show must be skipped (its result would be from the same
		// pre-refresh snapshot). Persistent refresh problems will then surface from destroy
		// itself, which yields a more actionable error than surfacing refresh's. A stderr
		// warning must also fire so the operator can correlate a later destroy failure with
		// the upstream refresh hiccup — silent fallback would hide a recurring credential
		// or connectivity issue until destroy itself errored.
		stack, mocks := setup(t)
		blueprint := createTestBlueprint()

		// Capture warnings via the injected writer rather than redirecting os.Stderr;
		// the TUI spinner shares os.Stderr and a pipe-based redirect deadlocks on Windows
		// where the spinner goroutine cannot be reliably synchronized with w.Close().
		var captured strings.Builder
		stack.warningWriter = &captured

		showCalls := 0
		var destroyArgs []string
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" && len(args) >= 3 && args[1] == "show" && args[2] == "-json" && len(args) == 3 {
				showCalls++
				return `{"values":{"root_module":{"resources":[{"address":"aws_s3_bucket.example"}]}}}`, nil
			}
			if command == "terraform" && len(args) > 1 && args[1] == "refresh" {
				return "", fmt.Errorf("mock error refreshing state")
			}
			if command == "terraform" && len(args) > 1 && args[1] == "destroy" {
				destroyArgs = append([]string(nil), args...)
				return "", nil
			}
			return "", nil
		}

		skipped, err := stack.Destroy(blueprint, "local/path")
		warningOutput := captured.String()

		if err != nil {
			t.Fatalf("Expected destroy to fall through despite refresh error, got %v", err)
		}
		if skipped {
			t.Error("Destroy must not report skipped when refresh fails on a non-empty-state component")
		}
		if showCalls != 1 {
			t.Errorf("Expected exactly one state-show call (pre-refresh only); post-refresh check must be skipped on refresh failure, got %d show calls", showCalls)
		}
		if destroyArgs == nil {
			t.Fatal("Expected terraform destroy to be called when refresh fails on non-empty state")
		}
		hasRefreshTrue := false
		for _, a := range destroyArgs {
			if a == "-refresh=true" {
				hasRefreshTrue = true
			}
			if a == "-refresh=false" {
				t.Errorf("Expected destroy to use -refresh=true on refresh fallback path, got -refresh=false in args %v", destroyArgs)
			}
		}
		if !hasRefreshTrue {
			t.Errorf("Expected destroy args to include -refresh=true, got %v", destroyArgs)
		}
		if !strings.Contains(warningOutput, "warning: terraform refresh failed for local/path") {
			t.Errorf("Expected warning naming the component, got: %q", warningOutput)
		}
		if !strings.Contains(warningOutput, "mock error refreshing state") {
			t.Errorf("Expected warning to include underlying refresh error for diagnostics, got: %q", warningOutput)
		}
	})

	t.Run("SkipsRefreshAndDestroyOnEmptyPreRefreshState", func(t *testing.T) {
		// Given state is already empty going into destroy — the bug case from the field
		// where an upstream component (e.g. a VPC) was destroyed first, then a downstream
		// module's refresh tried to read its `data "aws_vpc"` and failed because the cloud
		// object was gone. With the pre-refresh empty-state check, the entire flow
		// (refresh + destroy) must be skipped: refresh adds no signal when state is empty
		// going in (it can only drop resources, not add), and skipping it dodges any data
		// source that depends on already-torn-down infra. Destroy must report skipped=true.
		stack, mocks := setup(t)
		blueprint := createTestBlueprint()

		refreshCalled := false
		destroyCalled := false
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" && len(args) >= 3 && args[1] == "show" && args[2] == "-json" && len(args) == 3 {
				// Empty state — no resources at any module level.
				return `{"values":{"root_module":{}}}`, nil
			}
			if command == "terraform" && len(args) > 1 && args[1] == "refresh" {
				refreshCalled = true
				return "", fmt.Errorf("refresh must not run on empty pre-refresh state")
			}
			return "", nil
		}
		mocks.Shell.ExecProgressWithEnvFunc = func(message string, command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" && len(args) > 1 && args[1] == "destroy" {
				destroyCalled = true
			}
			return "", nil
		}

		skipped, err := stack.Destroy(blueprint, "local/path")
		if err != nil {
			t.Fatalf("Expected no error on empty-state skip, got %v", err)
		}
		if !skipped {
			t.Error("Destroy must report skipped=true when state is empty going in")
		}
		if refreshCalled {
			t.Error("refresh must not run when pre-refresh state is empty")
		}
		if destroyCalled {
			t.Error("destroy must not run when pre-refresh state is empty")
		}
	})

	t.Run("SurfacesStateShowJSONError", func(t *testing.T) {
		// Given a component where `terraform show -json` fails reading state — e.g. a
		// corrupted state file or a backend that rejects the call. Silently treating
		// this as "empty" would make destroy skip a component whose state is actually
		// intact, leaving resources orphaned.
		stack, mocks := setup(t)
		blueprint := createTestBlueprint()

		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" && len(args) >= 3 && args[1] == "show" && args[2] == "-json" && len(args) == 3 {
				return "", fmt.Errorf("Error: Failed to load state: corrupted state file")
			}
			return "", nil
		}

		_, err := stack.Destroy(blueprint, "local/path")
		if err == nil {
			t.Fatal("Expected state-show error to be surfaced, got nil")
		}
		if !strings.Contains(err.Error(), "error reading terraform state JSON") {
			t.Errorf("Expected state-read error text, got %q", err.Error())
		}
	})

	t.Run("SurfacesMalformedStateJSON", func(t *testing.T) {
		// Given state JSON that terraform produced but which doesn't decode (garbage
		// from a broken plugin, truncated stream, etc.) — the parse error must surface
		// rather than collapsing into the Noop path. Same rationale as
		// SurfacesStateShowJSONError: a parse failure could otherwise mask real state.
		stack, mocks := setup(t)
		blueprint := createTestBlueprint()

		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" && len(args) >= 3 && args[1] == "show" && args[2] == "-json" && len(args) == 3 {
				return `not valid json {`, nil
			}
			return "", nil
		}

		_, err := stack.Destroy(blueprint, "local/path")
		if err == nil {
			t.Fatal("Expected parse error to be surfaced, got nil")
		}
		if !strings.Contains(err.Error(), "error parsing terraform state JSON") {
			t.Errorf("Expected state-parse error text, got %q", err.Error())
		}
	})

	t.Run("HandlesPartialDestructionViaRefreshReconciliation", func(t *testing.T) {
		// Given a scenario where some resources have been manually deleted between the
		// last apply and this destroy — the canonical idempotency scenario this flow
		// must handle. The pre-refresh check sees the pre-deletion state (non-empty,
		// so we proceed), refresh drops the missing resources, and the post-refresh
		// check sees only the survivors. The flow must then destroy those survivors,
		// NOT recreate the missing ones — no apply runs. The critical properties are:
		// refresh ran between the two state reads, destroy ran against the survivors,
		// and apply did not run.
		stack, mocks := setup(t)
		blueprint := createTestBlueprint()

		// Both state reads return a non-empty view: pre-refresh shows the original 5
		// (test only cares that it is non-empty so the pre-check does not skip), post-
		// refresh shows the 3 survivors. Mock returns the same payload to both — fine
		// because the property under test is "refresh runs and destroy runs" not the
		// exact pre/post payload differential.
		stateJSON := `{"values":{"root_module":{"resources":[
			{"address":"aws_s3_bucket.surviving_a"},
			{"address":"aws_rds_cluster.surviving_b"},
			{"address":"aws_ecr_repository.surviving_c"}
		]}}}`

		refreshCalled := false
		applyCalled := false
		destroyCalled := false
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" && len(args) > 1 && args[1] == "refresh" {
				refreshCalled = true
				return "", nil
			}
			if command == "terraform" && len(args) >= 3 && args[1] == "show" && args[2] == "-json" && len(args) == 3 {
				return stateJSON, nil
			}
			if command == "terraform" && len(args) > 1 && args[1] == "apply" {
				applyCalled = true
			}
			if command == "terraform" && len(args) > 1 && args[1] == "destroy" {
				destroyCalled = true
			}
			return "", nil
		}

		if _, err := stack.Destroy(blueprint, "local/path"); err != nil {
			t.Fatalf("Expected partial-destruction destroy to succeed, got %v", err)
		}

		// Then refresh ran first (reconciled state), the state check saw survivors, and
		// destroy tore them down. Apply must NOT have run — running apply against the
		// post-refresh state would try to reconcile config→state and recreate the
		// resources refresh just dropped, which is the exact bug the no-prep design
		// prevents.
		if !refreshCalled {
			t.Error("Expected refresh to run before the state check")
		}
		if applyCalled {
			t.Error("Expected apply never to run — it would recreate resources dropped by refresh")
		}
		if !destroyCalled {
			t.Error("Expected destroy to run against survivors")
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

