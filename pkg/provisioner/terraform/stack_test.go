package terraform

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/internal/presenter"
	"github.com/windsorcli/cli/internal/werror"
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

// warningPresenter writes each warning message to w as one line.
type warningPresenter struct{ w io.Writer }

func (p warningPresenter) Emit(_ context.Context, e presenter.Event) {
	if e.Kind == presenter.KindMessage && e.Level >= slog.LevelWarn {
		fmt.Fprintln(p.w, e.Message)
	}
}

// withWarnings returns a context whose presenter writes warning messages to w.
func withWarnings(w io.Writer) context.Context {
	return presenter.NewContext(context.Background(), warningPresenter{w})
}

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
	shims.Remove = func(_ string) error {
		return nil
	}
	shims.ReadFile = func(_ string) ([]byte, error) {
		return nil, os.ErrNotExist
	}
	shims.TimeSleep = func(time.Duration) {}
	shims.Getenv = func(_ string) string {
		return ""
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

	// Default `terraform show -json` to a valid empty-state response so Up/Apply's
	// pre-plan hasStateResources check can parse the output and short-circuit refresh
	// for tests that don't override (the §4.4 unified refresh policy added this call).
	// Tests asserting the refresh-fires path override this to return non-empty state.
	mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
		if command == "terraform" && len(args) >= 3 && args[1] == "show" && args[2] == "-json" {
			return `{"values":{"root_module":{"resources":[]}}}`, nil
		}
		return "", nil
	}

	return mocks
}

// eventRecorder records every presenter event it receives.
type eventRecorder struct {
	events []presenter.Event
}

func (r *eventRecorder) Emit(_ context.Context, e presenter.Event) {
	r.events = append(r.events, e)
}

// =============================================================================
// Test Constructor
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

// =============================================================================
// Test Public Methods
// =============================================================================

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

		if _, err := stack.Up(context.Background(), blueprint); err != nil {
			t.Errorf("Expected Up to return nil, got %v", err)
		}
	})

	t.Run("InvokesOnApplyHooksAfterEachComponentWithComponentID", func(t *testing.T) {
		stack, _ := setup(t)
		blueprint := createTestBlueprint()
		var invokedIDs []string
		hook := func(ctx context.Context, id string) (bool, error) {
			invokedIDs = append(invokedIDs, id)
			return false, nil
		}

		_, err := stack.Up(context.Background(), blueprint, hook)

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
		hook := func(ctx context.Context, id string) (bool, error) {
			return false, hookErr
		}

		_, err := stack.Up(context.Background(), blueprint, hook)

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
		_, err := stack.Up(context.Background(), blueprint)
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
		_, err := stack.Up(context.Background(), blueprint)
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
		_, err := stack.Up(context.Background(), blueprint)
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
			if command == "terraform" && len(args) >= 3 && args[1] == "show" && args[2] == "-json" {
				return `{"values":{"root_module":{"resources":[]}}}`, nil
			}
			return "", nil
		}

		blueprint := createTestBlueprint()
		_, err := stack.Up(context.Background(), blueprint)
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
		_, err := stack.Up(context.Background(), blueprint)
		expectedError := "error running terraform apply for"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("PassesNoColorToInitAndApply", func(t *testing.T) {
		stack, mocks := setup(t)
		var initArgs, applyArgs []string
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" && len(args) > 1 && args[1] == "init" {
				initArgs = args
			}
			if command == "terraform" && len(args) >= 3 && args[1] == "show" && args[2] == "-json" {
				return `{"values":{"root_module":{"resources":[]}}}`, nil
			}
			return "", nil
		}
		mocks.Shell.ExecProgressWithEnvFunc = func(message string, command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" && len(args) > 1 && args[1] == "apply" {
				applyArgs = args
			}
			return "", nil
		}

		blueprint := createTestBlueprint()
		if _, err := stack.Up(context.Background(), blueprint); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !slices.Contains(initArgs, "-no-color") {
			t.Errorf("Expected init args to contain -no-color, got %v", initArgs)
		}
		if !slices.Contains(applyArgs, "-no-color") {
			t.Errorf("Expected apply args to contain -no-color, got %v", applyArgs)
		}
	})

	t.Run("NilBlueprint", func(t *testing.T) {
		stack, _ := setup(t)
		_, err := stack.Up(context.Background(), nil)
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
		_, err := stack.Up(context.Background(), blueprint)
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
		_, err := stack.Up(context.Background(), blueprint)

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

		if _, err := stack.Up(context.Background(), blueprint); err != nil {
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

		if _, err := stack.Up(context.Background(), blueprint); err != nil {
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
			if command == "terraform" && len(args) >= 3 && args[1] == "show" && args[2] == "-json" {
				return `{"values":{"root_module":{"resources":[]}}}`, nil
			}
			return "", nil
		}

		_, _ = stack.Up(context.Background(), blueprint)

		// Then TF_VAR_operation is "apply"
		if capturedEnv == nil {
			t.Fatal("Expected plan to be invoked")
		}
		if capturedEnv["TF_VAR_operation"] != "apply" {
			t.Errorf("Expected TF_VAR_operation to be %q, got %q", "apply", capturedEnv["TF_VAR_operation"])
		}
	})

	t.Run("PassesRefreshFalseToPlanSinceRefreshFiresExplicitlyFirst", func(t *testing.T) {
		// Given a stack and a blueprint
		stack, mocks := setup(t)
		blueprint := createTestBlueprint()

		// When Up is called, capture the plan args
		var capturedPlanArgs []string
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" && len(args) > 1 && args[1] == "plan" {
				capturedPlanArgs = args
			}
			if command == "terraform" && len(args) >= 3 && args[1] == "show" && args[2] == "-json" {
				return `{"values":{"root_module":{"resources":[]}}}`, nil
			}
			return "", nil
		}

		_, _ = stack.Up(context.Background(), blueprint)

		// Then plan was called with -refresh=false (refresh ran explicitly before)
		var sawRefreshFalse bool
		for _, a := range capturedPlanArgs {
			if a == "-refresh=false" {
				sawRefreshFalse = true
				break
			}
		}
		if !sawRefreshFalse {
			t.Errorf("Expected plan args to include -refresh=false, got %v", capturedPlanArgs)
		}
	})

	t.Run("SkipsRefreshWhenStateIsEmpty", func(t *testing.T) {
		// Given a stack whose state-show returns empty state
		stack, mocks := setup(t)
		blueprint := createTestBlueprint()

		var sawRefreshExec bool
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" && len(args) > 1 && args[1] == "refresh" {
				sawRefreshExec = true
			}
			if command == "terraform" && len(args) >= 3 && args[1] == "show" && args[2] == "-json" {
				return `{"values":{"root_module":{"resources":[]}}}`, nil
			}
			return "", nil
		}

		// When Up runs
		_, _ = stack.Up(context.Background(), blueprint)

		// Then refresh was never invoked — empty state means nothing to reconcile
		if sawRefreshExec {
			t.Error("Expected refresh to be skipped when state is empty, but it was invoked")
		}
	})

	t.Run("RefreshFiresWhenStateIsNonEmpty", func(t *testing.T) {
		// Given a stack whose state-show returns a populated state
		stack, mocks := setup(t)
		blueprint := createTestBlueprint()

		var sawRefreshExec bool
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" && len(args) > 1 && args[1] == "refresh" {
				sawRefreshExec = true
			}
			if command == "terraform" && len(args) >= 3 && args[1] == "show" && args[2] == "-json" {
				return `{"values":{"root_module":{"resources":[{"address":"aws_s3_bucket.example"}]}}}`, nil
			}
			return "", nil
		}

		// When Up runs
		_, _ = stack.Up(context.Background(), blueprint)

		// Then refresh was invoked because state is non-empty
		if !sawRefreshExec {
			t.Error("Expected refresh to be invoked when state is non-empty, but it was not")
		}
	})

	t.Run("WarnsAndContinuesWhenRefreshFailsOnNonEmptyState", func(t *testing.T) {
		// Given a stack with non-empty state and a refresh that fails
		stack, mocks := setup(t)
		warnings := &bytes.Buffer{}
		ctx := withWarnings(warnings)

		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" && len(args) > 1 && args[1] == "refresh" {
				return "", fmt.Errorf("simulated refresh failure")
			}
			if command == "terraform" && len(args) >= 3 && args[1] == "show" && args[2] == "-json" {
				return `{"values":{"root_module":{"resources":[{"address":"aws_s3_bucket.example"}]}}}`, nil
			}
			return "", nil
		}
		blueprint := createTestBlueprint()

		// When Up runs
		_, err := stack.Up(ctx, blueprint)

		// Then no error surfaces (warn-but-continue policy) and the warning names the failure
		if err != nil {
			t.Fatalf("Expected refresh failure on non-empty state to be tolerated, got %v", err)
		}
		if !strings.Contains(warnings.String(), "terraform refresh failed") {
			t.Errorf("Expected stderr warning to mention refresh failure, got: %s", warnings.String())
		}
	})

	t.Run("PropagatesStateCheckErrorBeforeRunningPlan", func(t *testing.T) {
		// Given a stack whose hasStateResources fails (terraform show -json errors)
		stack, mocks := setup(t)
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" && len(args) >= 3 && args[1] == "show" && args[2] == "-json" {
				return "", fmt.Errorf("simulated show -json failure")
			}
			return "", nil
		}
		blueprint := createTestBlueprint()

		// When Up runs
		_, err := stack.Up(context.Background(), blueprint)

		// Then the state-check error propagates
		if err == nil {
			t.Fatal("Expected state-check error to propagate, got nil")
		}
		if !strings.Contains(err.Error(), "error reading terraform state JSON") {
			t.Errorf("Expected error to mention terraform state JSON, got %v", err)
		}
	})

	t.Run("DoesNotPassForceCopyFlag", func(t *testing.T) {
		stack, mocks := setup(t)
		blueprint := createTestBlueprint()

		var forceCopySeen bool
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if len(args) >= 2 && args[1] == "init" {
				for _, a := range args {
					if a == "-force-copy" {
						forceCopySeen = true
					}
				}
			}
			if command == "terraform" && len(args) >= 3 && args[1] == "show" && args[2] == "-json" {
				return `{"values":{"root_module":{"resources":[]}}}`, nil
			}
			return "", nil
		}

		// When Up runs
		if _, err := stack.Up(context.Background(), blueprint); err != nil {
			t.Fatalf("Expected Up to succeed, got %v", err)
		}

		// Then no init invocation carried -force-copy
		if forceCopySeen {
			t.Error("Expected Up's init not to include -force-copy; the flag belongs only on the explicit state-migration path")
		}
	})

	t.Run("PassesInputFalseFlag", func(t *testing.T) {
		stack, mocks := setup(t)
		blueprint := createTestBlueprint()

		var inputFalseSeen bool
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if len(args) >= 2 && args[1] == "init" {
				for _, a := range args {
					if a == "-input=false" {
						inputFalseSeen = true
					}
				}
			}
			if command == "terraform" && len(args) >= 3 && args[1] == "show" && args[2] == "-json" {
				return `{"values":{"root_module":{"resources":[]}}}`, nil
			}
			return "", nil
		}

		// When Up runs
		if _, err := stack.Up(context.Background(), blueprint); err != nil {
			t.Fatalf("Expected Up to succeed, got %v", err)
		}

		// Then init ran non-interactively so a backend-migration prompt after destroy
		// fails fast with a clear error instead of reading EOF from a closed stdin
		if !inputFalseSeen {
			t.Error("Expected Up's init to include -input=false so terraform never blocks on an interactive prompt")
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
		err := stack.Apply(context.Background(), blueprint, "local/path")

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected Apply to return nil, got %v", err)
		}
	})

	t.Run("NilBlueprint", func(t *testing.T) {
		// Given a stack with a nil blueprint
		stack, _ := setup(t)

		// When applying with nil blueprint
		err := stack.Apply(context.Background(), nil, "local/path")

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
		err := stack.Apply(context.Background(), blueprint, "")

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
		err := stack.Apply(context.Background(), blueprint, "does/not/exist")

		// Then an error should occur naming the missing component
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), `"does/not/exist" not found`) {
			t.Errorf("Expected not found error, got: %v", err)
		}
		if we := werror.Find(err); we == nil || we.Remediation != componentNotFoundHint {
			t.Errorf("Expected the component-not-found remediation, got %v", err)
		}
	})

	t.Run("EmptyProjectRoot", func(t *testing.T) {
		// Given a stack with an empty project root
		stack, mocks := setup(t)
		mocks.Runtime.ProjectRoot = ""
		blueprint := createTestBlueprint()

		// When applying
		err := stack.Apply(context.Background(), blueprint, "local/path")

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "project root is empty") {
			t.Errorf("Expected project root error, got: %v", err)
		}
	})

	t.Run("TerraformScopedDotEnvValueReachesSubprocessExec", func(t *testing.T) {
		// Given a contexts/<ctx>/terraform/.env file declaring a provider credential
		stack, mocks := setup(t)
		configRoot, err := mocks.ConfigHandler.GetConfigRoot()
		if err != nil {
			t.Fatalf("Failed to get config root: %v", err)
		}
		dotEnvDir := filepath.Join(configRoot, "terraform")
		if err := os.MkdirAll(dotEnvDir, 0750); err != nil {
			t.Fatalf("Failed to create terraform/.env directory: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dotEnvDir, ".env"), []byte("HYPERV_HOST=hyperv.local\n"), 0600); err != nil {
			t.Fatalf("Failed to write terraform/.env fixture: %v", err)
		}
		blueprint := createTestBlueprint()

		var capturedEnvs []map[string]string
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			capturedEnvs = append(capturedEnvs, env)
			if command == "terraform" && len(args) >= 3 && args[1] == "show" && args[2] == "-json" {
				return `{"values":{"root_module":{"resources":[]}}}`, nil
			}
			return "", nil
		}
		mocks.Shell.ExecProgressWithEnvFunc = func(message string, command string, env map[string]string, args ...string) (string, error) {
			capturedEnvs = append(capturedEnvs, env)
			return "", nil
		}

		// When applying the local component
		if err := stack.Apply(context.Background(), blueprint, "local/path"); err != nil {
			t.Fatalf("Expected Apply to return nil, got %v", err)
		}

		// Then every actual terraform subprocess invocation should have received the
		// terraform/.env value — not just the in-process terraformVars map
		if len(capturedEnvs) == 0 {
			t.Fatal("Expected at least one captured subprocess env")
		}
		for i, env := range capturedEnvs {
			if env["HYPERV_HOST"] != "hyperv.local" {
				t.Errorf("Exec call %d: expected HYPERV_HOST=hyperv.local in subprocess env, got %v", i, env["HYPERV_HOST"])
			}
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
		err := stack.Apply(context.Background(), blueprint, "local/path")

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
		err := stack.Apply(context.Background(), blueprint, "local/path")

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
		err := stack.Apply(context.Background(), blueprint, "local/path")

		// Then an error should occur
		if !strings.Contains(err.Error(), "error running terraform init for") {
			t.Fatalf("Expected error to contain %q, got %q", "error running terraform init for", err.Error())
		}
	})

	t.Run("HintsAtStaleCacheOnStaleProviderLockForScratchComponent", func(t *testing.T) {
		// Given a scratch (Source-set) component whose init fails with Terraform's
		// stale-provider-lock signature
		stack, mocks := setup(t)
		staleLockErr := "Error: Failed to query available provider packages\n\nCould not retrieve the list of available versions for provider hashicorp/aws: locked provider registry.terraform.io/hashicorp/aws 6.47.0 does not match configured version constraint 6.57.1; must use terraform init -upgrade to allow selection of new versions"
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" && len(args) > 1 && args[1] == "init" {
				return "", fmt.Errorf("%s", staleLockErr)
			}
			return "", nil
		}
		blueprint := createTestBlueprint()

		// When applying the scratch component
		err := stack.Apply(context.Background(), blueprint, "remote/path")

		// Then the remediation names the scratch cache directory to clear, and the
		// message keeps Terraform's own error text
		if err == nil {
			t.Fatal("Expected an error, got nil")
		}
		if we := werror.Find(err); we == nil || !strings.Contains(we.Remediation, "local terraform cache for this component is stale") {
			t.Errorf("Expected a scratch-cache remediation, got %v", err)
		}
		if !strings.Contains(err.Error(), staleLockErr) {
			t.Errorf("Expected the underlying terraform error preserved, got %v", err)
		}
	})

	t.Run("HintsAtLockFileOnStaleProviderLockForNonScratchComponent", func(t *testing.T) {
		// Given a plain local (no Name, no Source) component whose init fails with the same
		// stale-provider-lock signature — its .terraform.lock.hcl may be an intentional,
		// git-tracked pin, so the hint must not presume to say "delete this directory"
		stack, mocks := setup(t)
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" && len(args) > 1 && args[1] == "init" {
				return "", fmt.Errorf("Error: Failed to query available provider packages\n\nCould not retrieve the list of available versions for provider hashicorp/aws: locked provider registry.terraform.io/hashicorp/aws 6.47.0 does not match configured version constraint 6.57.1; must use terraform init -upgrade to allow selection of new versions")
			}
			return "", nil
		}
		blueprint := createTestBlueprint()

		// When applying the non-scratch component
		err := stack.Apply(context.Background(), blueprint, "local/path")

		// Then the hint points at re-running init with -upgrade in that directory, not at
		// deleting anything
		if err == nil {
			t.Fatal("Expected an error, got nil")
		}
		if !strings.Contains(err.Error(), "terraform init -upgrade") {
			t.Errorf("Expected a lock-file hint pointing at terraform init -upgrade, got %v", err)
		}
	})

	t.Run("UnrelatedInitFailureGetsNoHint", func(t *testing.T) {
		// Given an init failure that is not the stale-provider-lock signature
		stack, mocks := setup(t)
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" && len(args) > 1 && args[1] == "init" {
				return "", fmt.Errorf("mock error running terraform init")
			}
			return "", nil
		}
		blueprint := createTestBlueprint()

		// When applying
		err := stack.Apply(context.Background(), blueprint, "local/path")

		// Then no stale-lock hint is added
		if err == nil {
			t.Fatal("Expected an error, got nil")
		}
		if strings.Contains(err.Error(), "provider lock") || strings.Contains(err.Error(), "terraform init -upgrade") {
			t.Errorf("Expected no stale-lock hint on an unrelated failure, got %v", err)
		}
	})

	t.Run("ErrorRunningTerraformPlan", func(t *testing.T) {
		// Given a stack whose shell fails on terraform plan
		stack, mocks := setup(t)
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" && len(args) > 1 && args[1] == "plan" {
				return "", fmt.Errorf("mock error running terraform plan")
			}
			if command == "terraform" && len(args) >= 3 && args[1] == "show" && args[2] == "-json" {
				return `{"values":{"root_module":{"resources":[]}}}`, nil
			}
			return "", nil
		}
		blueprint := createTestBlueprint()

		// When applying
		err := stack.Apply(context.Background(), blueprint, "local/path")

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
		err := stack.Apply(context.Background(), blueprint, "local/path")

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
		err := stack.Apply(context.Background(), blueprint, "cluster")

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
		err := stack.Apply(context.Background(), blueprint, "local/path")

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
			if command == "terraform" && len(args) >= 3 && args[1] == "show" && args[2] == "-json" {
				return `{"values":{"root_module":{"resources":[]}}}`, nil
			}
			return "", nil
		}

		_ = stack.Apply(context.Background(), blueprint, "local/path")

		// Then TF_VAR_operation is "apply"
		if capturedEnv == nil {
			t.Fatal("Expected plan to be invoked")
		}
		if capturedEnv["TF_VAR_operation"] != "apply" {
			t.Errorf("Expected TF_VAR_operation to be %q, got %q", "apply", capturedEnv["TF_VAR_operation"])
		}
	})

	t.Run("PassesRefreshFalseToPlanSinceRefreshFiresExplicitlyFirst", func(t *testing.T) {
		// Given a stack and a single component
		stack, mocks := setup(t)
		blueprint := createTestBlueprint()

		// When Apply is called, capture the plan args
		var capturedPlanArgs []string
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" && len(args) > 1 && args[1] == "plan" {
				capturedPlanArgs = args
			}
			if command == "terraform" && len(args) >= 3 && args[1] == "show" && args[2] == "-json" {
				return `{"values":{"root_module":{"resources":[]}}}`, nil
			}
			return "", nil
		}

		_ = stack.Apply(context.Background(), blueprint, "local/path")

		// Then plan was called with -refresh=false (refresh ran explicitly before)
		var sawRefreshFalse bool
		for _, a := range capturedPlanArgs {
			if a == "-refresh=false" {
				sawRefreshFalse = true
				break
			}
		}
		if !sawRefreshFalse {
			t.Errorf("Expected plan args to include -refresh=false, got %v", capturedPlanArgs)
		}
	})

	t.Run("RefreshFiresWhenStateIsNonEmpty", func(t *testing.T) {
		// Given a stack whose state-show returns a populated state
		stack, mocks := setup(t)
		blueprint := createTestBlueprint()

		var sawRefreshExec bool
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" && len(args) > 1 && args[1] == "refresh" {
				sawRefreshExec = true
			}
			if command == "terraform" && len(args) >= 3 && args[1] == "show" && args[2] == "-json" {
				return `{"values":{"root_module":{"resources":[{"address":"aws_s3_bucket.example"}]}}}`, nil
			}
			return "", nil
		}

		// When Apply runs
		_ = stack.Apply(context.Background(), blueprint, "local/path")

		// Then refresh was invoked because state is non-empty
		if !sawRefreshExec {
			t.Error("Expected refresh to be invoked when state is non-empty, but it was not")
		}
	})

	t.Run("WarnsAndContinuesWhenRefreshFailsOnNonEmptyState", func(t *testing.T) {
		// Given a stack with non-empty state and a refresh that fails
		stack, mocks := setup(t)
		warnings := &bytes.Buffer{}
		ctx := withWarnings(warnings)

		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" && len(args) > 1 && args[1] == "refresh" {
				return "", fmt.Errorf("simulated refresh failure")
			}
			if command == "terraform" && len(args) >= 3 && args[1] == "show" && args[2] == "-json" {
				return `{"values":{"root_module":{"resources":[{"address":"aws_s3_bucket.example"}]}}}`, nil
			}
			return "", nil
		}
		blueprint := createTestBlueprint()

		// When Apply runs
		err := stack.Apply(ctx, blueprint, "local/path")

		// Then no error surfaces (warn-but-continue) and the warning names the failure
		if err != nil {
			t.Fatalf("Expected refresh failure on non-empty state to be tolerated, got %v", err)
		}
		if !strings.Contains(warnings.String(), "terraform refresh failed") {
			t.Errorf("Expected stderr warning to mention refresh failure, got: %s", warnings.String())
		}
	})

	t.Run("PropagatesStateCheckErrorBeforeRunningPlan", func(t *testing.T) {
		// Given a stack whose hasStateResources fails (terraform show -json errors)
		stack, mocks := setup(t)
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" && len(args) >= 3 && args[1] == "show" && args[2] == "-json" {
				return "", fmt.Errorf("simulated show -json failure")
			}
			return "", nil
		}
		blueprint := createTestBlueprint()

		// When Apply runs
		err := stack.Apply(context.Background(), blueprint, "local/path")

		// Then the state-check error propagates — Apply must not silently fall through
		// to plan against an unknown state
		if err == nil {
			t.Fatal("Expected state-check error to propagate, got nil")
		}
		if !strings.Contains(err.Error(), "error reading terraform state JSON") {
			t.Errorf("Expected error to mention terraform state JSON, got %v", err)
		}
	})
}

func TestStack_InitCache(t *testing.T) {
	setup := func(t *testing.T) (*TerraformStack, *TerraformTestMocks) {
		t.Helper()
		mocks := setupWindsorStackMocks(t)
		stack := NewStack(mocks.Runtime).(*TerraformStack)
		stack.shims = mocks.Shims
		return stack, mocks
	}

	t.Run("RepeatedInitsForSameKeyShortCircuit", func(t *testing.T) {
		// Given two back-to-back HasRemoteState calls for the same component
		// (same configured backend, no -migrate-state on either), the second
		// call must reuse the cached init result and not invoke terraform a
		// second time. terraform init is the expensive operation here — even
		// on a warm provider cache it costs a process spawn and lockfile
		// verification — so the cache halves overhead on probe-then-Up paths.
		stack, mocks := setup(t)
		bp := createTestBlueprint()

		var initCalls int
		var showCalls int
		mocks.Shell.ExecSilentWithEnvFunc = func(_ string, _ map[string]string, args ...string) (string, error) {
			if len(args) >= 2 && strings.HasPrefix(args[0], "-chdir=") {
				switch args[1] {
				case "init":
					initCalls++
				case "show":
					showCalls++
					return `{"values":{"root_module":{"resources":[{"address":"foo"}]}}}`, nil
				}
			}
			return "", nil
		}

		if _, err := stack.HasRemoteState(bp, "remote/path"); err != nil {
			t.Fatalf("first HasRemoteState: %v", err)
		}
		if _, err := stack.HasRemoteState(bp, "remote/path"); err != nil {
			t.Fatalf("second HasRemoteState: %v", err)
		}

		if initCalls != 1 {
			t.Errorf("expected exactly 1 terraform init invocation across two HasRemoteState calls, got %d", initCalls)
		}
		if showCalls != 2 {
			t.Errorf("expected 2 terraform show invocations (cache only memoizes init, not state queries), got %d", showCalls)
		}
	})

	t.Run("DifferentComponentsDoNotShareCache", func(t *testing.T) {
		// Cache key includes componentID, so probing two different components
		// runs init twice — once per component. Verifies the cache doesn't
		// over-collapse across distinct components.
		stack, mocks := setup(t)
		bp := createTestBlueprint()

		var initCalls int
		mocks.Shell.ExecSilentWithEnvFunc = func(_ string, _ map[string]string, args ...string) (string, error) {
			if len(args) >= 2 && strings.HasPrefix(args[0], "-chdir=") {
				switch args[1] {
				case "init":
					initCalls++
				case "show":
					return `{"values":{"root_module":{"resources":[]}}}`, nil
				}
			}
			return "", nil
		}

		if _, err := stack.HasRemoteState(bp, "remote/path"); err != nil {
			t.Fatalf("HasRemoteState remote/path: %v", err)
		}
		if _, err := stack.HasRemoteState(bp, "local/path"); err != nil {
			t.Fatalf("HasRemoteState local/path: %v", err)
		}

		if initCalls != 2 {
			t.Errorf("expected 2 inits across two distinct components, got %d", initCalls)
		}
	})

	t.Run("FailedInitIsNotCached", func(t *testing.T) {
		// A transient init failure must not poison the cache — subsequent
		// retries need to actually re-invoke terraform. The cache only
		// records successful inits.
		stack, mocks := setup(t)
		bp := createTestBlueprint()

		var initCalls int
		failNext := true
		mocks.Shell.ExecSilentWithEnvFunc = func(_ string, _ map[string]string, args ...string) (string, error) {
			if len(args) >= 2 && strings.HasPrefix(args[0], "-chdir=") {
				switch args[1] {
				case "init":
					initCalls++
					if failNext {
						failNext = false
						return "", fmt.Errorf("transient init failure")
					}
				case "show":
					return `{"values":{"root_module":{"resources":[]}}}`, nil
				}
			}
			return "", nil
		}

		if _, err := stack.HasRemoteState(bp, "remote/path"); err == nil {
			t.Fatal("expected first HasRemoteState to fail on init")
		}
		if _, err := stack.HasRemoteState(bp, "remote/path"); err != nil {
			t.Fatalf("expected retry to succeed after transient failure: %v", err)
		}

		if initCalls != 2 {
			t.Errorf("expected 2 inits (failure not cached), got %d", initCalls)
		}
	})
}

func TestStack_Progress(t *testing.T) {
	setup := func(t *testing.T) (*TerraformStack, *TerraformTestMocks) {
		t.Helper()
		mocks := setupWindsorStackMocks(t)
		stack := NewStack(mocks.Runtime).(*TerraformStack)
		stack.shims = mocks.Shims
		return stack, mocks
	}

	t.Run("UpTracksEachComponentUnderTheCallerOperation", func(t *testing.T) {
		// Given a stack and a caller operation with a recording presenter
		stack, _ := setup(t)
		rec := &eventRecorder{}
		ctx, end := presenter.Begin(presenter.NewContext(context.Background(), rec), "terraform", "Applying terraform")
		parentID := rec.events[0].ID

		// When the stack applies the blueprint
		_, err := stack.Up(ctx, createTestBlueprint())
		end(err)

		// Then each component is one applied operation under the caller operation
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		var applied []string
		for _, e := range rec.events {
			if e.Kind == presenter.KindApplied && e.ParentID == parentID {
				if !strings.HasPrefix(e.Subject, "terraform:") {
					t.Errorf("expected a terraform subject, got %q", e.Subject)
				}
				applied = append(applied, e.Message)
			}
		}
		if strings.Join(applied, ",") != "Applying remote/path,Applying local/path" {
			t.Errorf("unexpected applied operations %v", applied)
		}
	})

	t.Run("UpEmitsFailedForTheFailingComponent", func(t *testing.T) {
		// Given a stack whose terraform apply fails
		stack, mocks := setup(t)
		mocks.Shell.ExecProgressWithEnvFunc = func(message string, command string, env map[string]string, args ...string) (string, error) {
			return "", fmt.Errorf("apply failed")
		}
		rec := &eventRecorder{}

		// When the stack applies the blueprint
		_, err := stack.Up(presenter.NewContext(context.Background(), rec), createTestBlueprint())

		// Then the first component ends as failed with the error, and no later component starts
		if err == nil {
			t.Fatal("expected an error")
		}
		last := rec.events[len(rec.events)-1]
		if last.Kind != presenter.KindFailed || last.Message != "Applying remote/path" || last.Err == nil {
			t.Errorf("expected a failed event for remote/path, got %+v", last)
		}
		if len(rec.events) != 2 {
			t.Errorf("expected only the first component's events, got %d", len(rec.events))
		}
	})

	t.Run("DestroyTracksTheComponent", func(t *testing.T) {
		// Given a stack and a recording presenter
		stack, _ := setup(t)
		rec := &eventRecorder{}
		blueprint := createTestBlueprint()

		// When one component is destroyed
		_, err := stack.Destroy(presenter.NewContext(context.Background(), rec), blueprint, blueprint.TerraformComponents[1].GetID())

		// Then the destroy is one operation that starts and ends
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if len(rec.events) != 2 || rec.events[0].Kind != presenter.KindApplying || rec.events[1].Kind != presenter.KindApplied || rec.events[0].Message != "Destroying local/path" {
			t.Errorf("unexpected events %+v", rec.events)
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

		terraformVars, _, terraformArgs, err := stack.setupTerraformEnvironment(component)
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

		_, _, _, err := stack.setupTerraformEnvironment(component)
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

		_, _, _, err := stack.setupTerraformEnvironment(component)
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
		}, false, nil)

		if selected["TF_DATA_DIR"] != "/tmp/data" {
			t.Error("Expected TF_DATA_DIR to be included")
		}
		if _, ok := selected["TF_VAR_context_path"]; ok {
			t.Error("Expected TF_VAR_context_path to be omitted when includeTFVars is false")
		}
		if _, ok := selected["OTHER"]; ok {
			t.Error("Expected keys not named in scopedKeys to be omitted")
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
		}, true, nil)
		if selected["TF_VAR_context_path"] == "" {
			t.Error("Expected TF_VAR_context_path to be included when includeTFVars is true")
		}
	})

	t.Run("SelectCommandEnvPassesThroughScopedKeysRegardlessOfIncludeTFVars", func(t *testing.T) {
		// Given a terraform-scoped key (e.g. from contexts/<ctx>/terraform/.env) and an
		// unrelated key that was never declared as scoped
		terraformVars := map[string]string{
			"TF_DATA_DIR": "/tmp/data",
			"HYPERV_HOST": "hyperv.local",
			"UNRELATED":   "should-not-leak",
		}

		// When selecting the command env with includeTFVars false (e.g. init/apply)
		selected := selectTerraformCommandEnv(terraformVars, false, []string{"HYPERV_HOST"})

		// Then the scoped key passes through regardless of includeTFVars
		if selected["HYPERV_HOST"] != "hyperv.local" {
			t.Error("Expected HYPERV_HOST to pass through as a declared scoped key")
		}
		// And an unrelated key not named in scopedKeys is still dropped
		if _, ok := selected["UNRELATED"]; ok {
			t.Error("Expected UNRELATED to be omitted since it isn't in scopedKeys")
		}
	})

	t.Run("EffectiveEnvValuePrefersCommandEnvOverProcessEnv", func(t *testing.T) {
		// Given a command env that overrides a key also set in the process env
		stack, mocks := setup(t)
		mocks.Shims.Getenv = func(key string) string { return "azurecli" }
		commandEnv := map[string]string{"TF_VAR_kubelogin_mode": "workloadidentity"}

		// When resolving the effective value
		value := stack.effectiveTerraformEnvValue(commandEnv, "TF_VAR_kubelogin_mode")

		// Then the command env's value wins
		if value != "workloadidentity" {
			t.Errorf("Expected workloadidentity, got %q", value)
		}
	})

	t.Run("EffectiveEnvValueFallsBackToProcessEnv", func(t *testing.T) {
		// Given a command env that does not carry the key
		stack, mocks := setup(t)
		mocks.Shims.Getenv = func(key string) string {
			if key == "TF_VAR_kubelogin_mode" {
				return "azurecli"
			}
			return ""
		}
		commandEnv := map[string]string{}

		// When resolving the effective value
		value := stack.effectiveTerraformEnvValue(commandEnv, "TF_VAR_kubelogin_mode")

		// Then the inherited process env value is returned
		if value != "azurecli" {
			t.Errorf("Expected azurecli, got %q", value)
		}
	})
}

func TestTerraformStack_clearStaleBackendPointer(t *testing.T) {
	setup := func(t *testing.T, configuredBackendType string) (*TerraformStack, *TerraformTestMocks) {
		t.Helper()
		mocks := setupWindsorStackMocks(t, &SetupOptions{ConfigStr: fmt.Sprintf(`
contexts:
  mock-context:
    terraform:
      backend:
        type: %s`, configuredBackendType)})
		stack := NewStack(mocks.Runtime).(*TerraformStack)
		stack.shims = mocks.Shims
		return stack, mocks
	}

	t.Run("RemovesPointerWhenBackendTypeMismatches", func(t *testing.T) {
		// Repo carries a committed backend_override.tf for `terraform test` runs,
		// so a raw `terraform init` lands a "current backend = local" pointer
		// inside TF_DATA_DIR. Windsor's configured backend is now s3 — the
		// pointer must be removed so the next init sets up s3 fresh, not
		// triggering terraform's "Prior to changing backends" migration prompt
		// against an s3 bucket that doesn't exist yet.
		stack, mocks := setup(t, "s3")
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			return []byte(`{"backend":{"type":"local"}}`), nil
		}
		var removed []string
		mocks.Shims.Remove = func(path string) error {
			removed = append(removed, filepath.ToSlash(path))
			return nil
		}

		stack.clearStaleBackendPointer(map[string]string{"TF_DATA_DIR": "/tmp/scratch/.terraform/cluster"})

		if len(removed) != 1 || !strings.HasSuffix(removed[0], "/terraform.tfstate") {
			t.Errorf("expected pointer file removal, got %v", removed)
		}
	})

	t.Run("LeavesPointerInPlaceWhenTypesMatch", func(t *testing.T) {
		// Pointer says s3, configured is s3 — no migration would be triggered.
		// Leave the pointer alone so terraform's normal "I already initialized
		// against this backend" fast path runs.
		stack, mocks := setup(t, "s3")
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			return []byte(`{"backend":{"type":"s3"}}`), nil
		}
		var removed []string
		mocks.Shims.Remove = func(path string) error {
			removed = append(removed, filepath.ToSlash(path))
			return nil
		}

		stack.clearStaleBackendPointer(map[string]string{"TF_DATA_DIR": "/tmp/scratch/.terraform/cluster"})

		if len(removed) != 0 {
			t.Errorf("expected no removal when types match, got %v", removed)
		}
	})

	t.Run("NoOpWhenPointerMissing", func(t *testing.T) {
		// Fresh init scenario: no pointer file exists yet. Function must
		// silently no-op so terraform init runs normally.
		stack, mocks := setup(t, "s3")
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			return nil, os.ErrNotExist
		}
		var removed []string
		mocks.Shims.Remove = func(path string) error {
			removed = append(removed, filepath.ToSlash(path))
			return nil
		}

		stack.clearStaleBackendPointer(map[string]string{"TF_DATA_DIR": "/tmp/scratch/.terraform/cluster"})

		if len(removed) != 0 {
			t.Errorf("expected no removal when pointer missing, got %v", removed)
		}
	})

	t.Run("RemovesUnparseablePointer", func(t *testing.T) {
		// A corrupted/truncated pointer file would block init either way.
		// Removing it lets init regenerate cleanly.
		stack, mocks := setup(t, "s3")
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			return []byte("not json"), nil
		}
		var removed []string
		mocks.Shims.Remove = func(path string) error {
			removed = append(removed, filepath.ToSlash(path))
			return nil
		}

		stack.clearStaleBackendPointer(map[string]string{"TF_DATA_DIR": "/tmp/scratch/.terraform/cluster"})

		if len(removed) != 1 {
			t.Errorf("expected unparseable pointer to be removed, got %v", removed)
		}
	})

	t.Run("NoOpWhenTFDataDirEmpty", func(t *testing.T) {
		// Belt-and-suspenders: if TF_DATA_DIR isn't set we have no path to
		// resolve, must not attempt any I/O.
		stack, mocks := setup(t, "s3")
		var readCalled, removeCalled bool
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			readCalled = true
			return nil, nil
		}
		mocks.Shims.Remove = func(path string) error {
			removeCalled = true
			return nil
		}

		stack.clearStaleBackendPointer(map[string]string{})

		if readCalled || removeCalled {
			t.Errorf("expected no I/O when TF_DATA_DIR empty, got read=%v remove=%v", readCalled, removeCalled)
		}
	})
}

func TestTerraformStack_refreshProviderLock(t *testing.T) {
	setup := func(t *testing.T) (*TerraformStack, *TerraformTestMocks) {
		t.Helper()
		mocks := setupWindsorStackMocks(t)
		stack := NewStack(mocks.Runtime).(*TerraformStack)
		stack.shims = mocks.Shims
		return stack, mocks
	}

	component := &blueprintv1alpha1.TerraformComponent{
		Path:     "backend/azurerm",
		FullPath: "/tmp/scratch/terraform/backend",
	}

	t.Run("RunsUpgradeWithBackendDisabled", func(t *testing.T) {
		// -backend=false skips backend configuration entirely, so this can never
		// touch state or contend for a state lock — only providers and modules
		// are resolved and the lock file is brought current.
		stack, mocks := setup(t)

		var args []string
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, a ...string) (string, error) {
			args = a
			return "", nil
		}

		if err := stack.refreshProviderLock(component, nil, nil); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if !slices.Contains(args, "init") {
			t.Errorf("expected an init invocation, got %v", args)
		}
		for _, want := range []string{"-upgrade", "-backend=false", "-input=false", "-chdir=/tmp/scratch/terraform/backend"} {
			if !slices.Contains(args, want) {
				t.Errorf("expected args to contain %q, got %v", want, args)
			}
		}
		if slices.Contains(args, "-migrate-state") {
			t.Error("expected no -migrate-state flag; this call must never touch state")
		}
	})

	t.Run("WrapsTheUnderlyingError", func(t *testing.T) {
		stack, mocks := setup(t)
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			return "", fmt.Errorf("registry unreachable")
		}

		err := stack.refreshProviderLock(component, nil, nil)
		if err == nil {
			t.Fatal("expected an error")
		}
		if !strings.Contains(err.Error(), "backend/azurerm") || !strings.Contains(err.Error(), "registry unreachable") {
			t.Errorf("expected error to name the component and wrap the cause, got: %v", err)
		}
	})
}

// =============================================================================
// Test Helpers
// =============================================================================

func TestIsScratchComponent(t *testing.T) {
	t.Run("TrueWhenNameSet", func(t *testing.T) {
		c := &blueprintv1alpha1.TerraformComponent{Name: "cluster"}
		if !isScratchComponent(c) {
			t.Error("Expected true when Name is set")
		}
	})

	t.Run("TrueWhenSourceSet", func(t *testing.T) {
		c := &blueprintv1alpha1.TerraformComponent{Source: "core"}
		if !isScratchComponent(c) {
			t.Error("Expected true when Source is set")
		}
	})

	t.Run("FalseWhenNeitherSet", func(t *testing.T) {
		c := &blueprintv1alpha1.TerraformComponent{Path: "local/path"}
		if isScratchComponent(c) {
			t.Error("Expected false when neither Name nor Source is set")
		}
	})
}

func TestMissingComponentDirError(t *testing.T) {
	t.Run("ScratchComponentHintsAtWindsorInit", func(t *testing.T) {
		// Given a component with a name
		c := &blueprintv1alpha1.TerraformComponent{Name: "cluster", Path: "cluster", FullPath: "/scratch/cluster"}

		// When the error is built
		err := missingComponentDirError(c)

		// Then the message names the directory and the remediation names windsor init
		if !strings.Contains(err.Error(), "/scratch/cluster") {
			t.Errorf("Expected the directory in the message, got %v", err)
		}
		if we := werror.Find(err); we == nil || !strings.Contains(we.Remediation, "windsor init") {
			t.Errorf("Expected a windsor init remediation, got %v", err)
		}
	})

	t.Run("LocalComponentHintsAtComponentPath", func(t *testing.T) {
		// Given a local component with no name or source
		c := &blueprintv1alpha1.TerraformComponent{Path: "network", FullPath: "/project/terraform/network"}

		// When the error is built
		err := missingComponentDirError(c)

		// Then the remediation names the terraform directory
		if we := werror.Find(err); we == nil || !strings.Contains(we.Remediation, "under terraform/") {
			t.Errorf("Expected a component path remediation, got %v", err)
		}
	})
}

func TestIsStaleProviderLockError(t *testing.T) {
	t.Run("NilError", func(t *testing.T) {
		if isStaleProviderLockError(nil) {
			t.Error("Expected false for nil error")
		}
	})

	t.Run("MatchesTerraformsStaleLockSignature", func(t *testing.T) {
		err := fmt.Errorf("Error: Failed to query available provider packages\n\nCould not retrieve the list of available versions for provider hashicorp/aws: locked provider registry.terraform.io/hashicorp/aws 6.47.0 does not match configured version constraint 6.57.1; must use terraform init -upgrade to allow selection of new versions")
		if !isStaleProviderLockError(err) {
			t.Error("Expected true for terraform's stale-provider-lock error text")
		}
	})

	t.Run("UnrelatedErrorDoesNotMatch", func(t *testing.T) {
		err := fmt.Errorf("mock error running terraform init")
		if isStaleProviderLockError(err) {
			t.Error("Expected false for an unrelated init failure")
		}
	})

	t.Run("PartialMarkerMatchDoesNotMatch", func(t *testing.T) {
		// Only one of the two markers present — must not false-positive.
		err := fmt.Errorf("does not match configured version constraint, but no upgrade hint here")
		if isStaleProviderLockError(err) {
			t.Error("Expected false when only one marker is present")
		}
	})

	t.Run("MatchesWhenTerraformWrapsTheMarkerAcrossLines", func(t *testing.T) {
		// Terraform's diagnostic renderer word-wraps at a fixed column width; here that
		// wrap lands between "terraform" and "init -upgrade", splitting the second marker.
		err := fmt.Errorf("Error: Failed to query available provider packages\n\nCould not retrieve the list of available versions for provider\nhashicorp/azurerm: locked provider registry.terraform.io/hashicorp/azurerm\n5.0.1 does not match configured version constraint 5.4.0; must use terraform\ninit -upgrade to allow selection of new versions")
		if !isStaleProviderLockError(err) {
			t.Error("Expected true when the marker is split across a wrapped line")
		}
	})
}

func TestNoColorArgs(t *testing.T) {
	t.Run("ReturnsNoColorFlagUnconditionally", func(t *testing.T) {
		if got := noColorArgs(); !slices.Equal(got, []string{"-no-color"}) {
			t.Errorf("Expected [-no-color], got %v", got)
		}
	})
}
