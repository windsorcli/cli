package terraform

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/internal/werror"
)

// =============================================================================
// Test Public Methods
// =============================================================================

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

	t.Run("SurfacesTheRealDiagnosticOnPlanFailure", func(t *testing.T) {
		// Given a plan that fails with a real provider error
		stack, mocks := setup(t)
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if len(args) > 1 && args[1] == "plan" {
				output := `{"type":"diagnostic","diagnostic":{"severity":"error","summary":"Error 403: Permission denied","detail":"googleapi: Error 403"}}` + "\n"
				return output, fmt.Errorf("exit status 1")
			}
			return "", nil
		}

		// When PlanJSON is called
		err := stack.PlanJSON(createTestBlueprint(), "local/path")

		// Then the real diagnostic reaches the caller, not just "exit status 1"
		if err == nil {
			t.Fatal("expected an error, got nil")
		}
		if !strings.Contains(err.Error(), "Error 403: Permission denied") {
			t.Errorf("expected the real diagnostic in the error, got: %v", err)
		}
	})
}

func TestStack_PlanAllJSON(t *testing.T) {
	t.Run("SurfacesTheRealDiagnosticOnPlanFailure", func(t *testing.T) {
		// Given a plan that fails with a real provider error
		mocks := setupWindsorStackMocks(t)
		stack := NewStack(mocks.Runtime).(*TerraformStack)
		stack.shims = mocks.Shims
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if len(args) > 1 && args[1] == "plan" {
				output := `{"type":"diagnostic","diagnostic":{"severity":"error","summary":"Error 403: Permission denied","detail":"googleapi: Error 403"}}` + "\n"
				return output, fmt.Errorf("exit status 1")
			}
			return "", nil
		}

		// When PlanAllJSON is called
		err := stack.PlanAllJSON(createTestBlueprint())

		// Then the real diagnostic reaches the caller, not just "exit status 1"
		if err == nil {
			t.Fatal("expected an error, got nil")
		}
		if !strings.Contains(err.Error(), "Error 403: Permission denied") {
			t.Errorf("expected the real diagnostic in the error, got: %v", err)
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
			if len(args) > 2 && args[1] == "show" && args[2] == "-json" {
				return `{"values":{"root_module":{"resources":[{"address":"aws_s3_bucket.example"}]}}}`, nil
			}
			if len(args) > 1 && args[1] == "plan" {
				return strings.Join([]string{
					`{"type":"planned_change","change":{"resource":{"addr":"module.main.aws_s3_bucket.logs"},"action":"create"}}`,
					`{"type":"change_summary","changes":{"add":5,"change":3,"remove":1}}`,
					``,
				}, "\n"), nil
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
		if len(r.Resources) == 0 {
			t.Errorf("expected resources to be populated")
		}
	})

	t.Run("SetsNoChangesWhenTerraformReportsNoDiff", func(t *testing.T) {
		// Given a shell that returns a "No changes." line
		stack, mocks := setup(t)
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if len(args) > 2 && args[1] == "show" && args[2] == "-json" {
				return `{"values":{"root_module":{"resources":[{"address":"aws_s3_bucket.example"}]}}}`, nil
			}
			if len(args) > 1 && args[1] == "plan" {
				return `{"type":"change_summary","changes":{"add":0,"change":0,"remove":0}}` + "\n", nil
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
			if len(args) > 2 && args[1] == "show" && args[2] == "-json" {
				return `{"values":{"root_module":{"resources":[{"address":"aws_s3_bucket.example"}]}}}`, nil
			}
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
			if len(args) > 2 && args[1] == "show" && args[2] == "-json" {
				return `{"values":{"root_module":{"resources":[{"address":"aws_s3_bucket.example"}]}}}`, nil
			}
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

	t.Run("MarksComponentNewWhenStateIsEmpty", func(t *testing.T) {
		// `terraform show -json` returns "{}" when the configured backend has
		// no state object for this component. With no orphaned local
		// terraform.tfstate either, the component has never been applied —
		// IsNew is the truth and running plan would either show a misleading
		// "all creates" or fail reading dependent layers via terraform_remote_state.
		stack, mocks := setup(t)
		// Shims default Stat returns (nil, nil) for unknown paths, but the local-
		// state probe treats that as "doesn't exist" because FileInfo is nil.
		var planCalled bool
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if len(args) > 2 && args[1] == "show" && args[2] == "-json" {
				return "{}", nil
			}
			if len(args) > 1 && args[1] == "plan" {
				planCalled = true
			}
			return "", nil
		}

		results := stack.PlanSummary(createTestBlueprint())

		if len(results) == 0 {
			t.Fatal("expected results")
		}
		for _, r := range results {
			if !r.IsNew {
				t.Errorf("expected IsNew=true for %q, got false", r.ComponentID)
			}
			if r.Err != nil {
				t.Errorf("expected no error for %q, got %v", r.ComponentID, r.Err)
			}
		}
		if planCalled {
			t.Error("plan must not be invoked when state is empty")
		}
	})

	t.Run("PropagatesInitError", func(t *testing.T) {
		// Init failure is a real error to surface — distinct from a fresh
		// component with no state.
		stack, mocks := setup(t)
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if len(args) > 1 && args[1] == "init" {
				return "", fmt.Errorf("init failed: invalid HCL")
			}
			return "", nil
		}

		results := stack.PlanSummary(createTestBlueprint())

		if len(results) == 0 {
			t.Fatal("expected results")
		}
		for _, r := range results {
			if r.Err == nil {
				t.Errorf("expected init error for %q, got nil", r.ComponentID)
			}
			if r.IsNew {
				t.Errorf("expected IsNew=false when init fails, got true for %q", r.ComponentID)
			}
		}
	})

	t.Run("PropagatesStateProbeError", func(t *testing.T) {
		// `terraform show -json` failures (e.g., transient backend connectivity
		// or invalid credentials) are real errors — silently classifying as
		// IsNew would hide them.
		stack, mocks := setup(t)
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if len(args) > 2 && args[1] == "show" && args[2] == "-json" {
				return "", fmt.Errorf("backend timeout")
			}
			return "", nil
		}

		results := stack.PlanSummary(createTestBlueprint())

		if len(results) == 0 {
			t.Fatal("expected results")
		}
		for _, r := range results {
			if r.Err == nil {
				t.Errorf("expected state-probe error for %q, got nil", r.ComponentID)
			}
			if r.IsNew {
				t.Errorf("expected IsNew=false for %q on state-probe failure, got true", r.ComponentID)
			}
		}
	})

	t.Run("PopulatesPathFromComponent", func(t *testing.T) {
		// The blueprint Path (e.g., "cluster/aws-eks") locates the underlying
		// module and is more informative than the short ComponentID alias for
		// renderers. PlanSummary copies it into each result so callers don't
		// need to re-resolve components — verify on the non-empty-state path,
		// where plan actually runs.
		stack, mocks := setup(t)
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if len(args) > 2 && args[1] == "show" && args[2] == "-json" {
				return `{"values":{"root_module":{"resources":[{"address":"aws_s3_bucket.example"}]}}}`, nil
			}
			if len(args) > 1 && args[1] == "plan" {
				return strings.Join([]string{
					`{"type":"planned_change","change":{"resource":{"addr":"aws_s3_bucket.foo"},"action":"create"}}`,
					`{"type":"change_summary","changes":{"add":1,"change":0,"remove":0}}`,
					``,
				}, "\n"), nil
			}
			return "", nil
		}

		results := stack.PlanSummary(createTestBlueprint())

		if len(results) == 0 {
			t.Fatal("expected results")
		}
		for _, r := range results {
			if r.Path == "" {
				t.Errorf("expected Path to be populated for %q, got empty", r.ComponentID)
			}
		}
	})

	t.Run("SurfacesTheRealDiagnosticOnPlanFailure", func(t *testing.T) {
		// Given a plan that fails with a real provider error
		stack, mocks := setup(t)
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if len(args) > 2 && args[1] == "show" && args[2] == "-json" {
				return `{"values":{"root_module":{"resources":[{"address":"google_container_cluster.main"}]}}}`, nil
			}
			if len(args) > 1 && args[1] == "plan" {
				output := `{"type":"diagnostic","diagnostic":{"severity":"error","summary":"Error 403: Permission denied","detail":"googleapi: Error 403: Required 'compute.instances.create' permission"}}` + "\n"
				return output, fmt.Errorf("exit status 1")
			}
			return "", nil
		}
		bp := &blueprintv1alpha1.Blueprint{
			Metadata:            blueprintv1alpha1.Metadata{Name: "t"},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{{Path: "cluster"}},
		}

		// When PlanSummary runs
		results := stack.PlanSummary(bp)

		// Then the result's error carries the real diagnostic, not just "exit status 1"
		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d: %#v", len(results), results)
		}
		if results[0].Err == nil {
			t.Fatal("expected an error, got nil")
		}
		if !strings.Contains(results[0].Err.Error(), "Error 403: Permission denied") || !strings.Contains(results[0].Err.Error(), "compute.instances.create") {
			t.Errorf("expected the real diagnostic in the error, got: %v", results[0].Err)
		}
	})
}

func TestStack_PlanDestroySummary(t *testing.T) {
	setup := func(t *testing.T) (*TerraformStack, *TerraformTestMocks) {
		t.Helper()
		mocks := setupWindsorStackMocks(t)
		stack := NewStack(mocks.Runtime).(*TerraformStack)
		stack.shims = mocks.Shims
		return stack, mocks
	}

	t.Run("ReturnsNilForNilBlueprint", func(t *testing.T) {
		stack, _ := setup(t)
		if got := stack.PlanDestroySummary(nil); got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})

	t.Run("InvokesPlanInDestroyModeAndCapturesResources", func(t *testing.T) {
		// Given a shell that records the plan args and returns a destroy event
		// stream (every planned_change action="delete")
		stack, mocks := setup(t)
		var capturedArgs []string
		var capturedEnv map[string]string
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if len(args) > 2 && args[1] == "show" && args[2] == "-json" {
				return `{"values":{"root_module":{"resources":[{"address":"aws_s3_bucket.example"}]}}}`, nil
			}
			// Only the destroy plan (carrying -destroy) is captured here; the
			// drift-check's own regular plan runs afterward and is exercised by
			// TestStack_destroyBehaviorDrift instead.
			if capturedArgs == nil && len(args) > 1 && args[1] == "plan" && slices.Contains(args, "-destroy") {
				capturedArgs = append([]string{}, args...)
				capturedEnv = env
				return strings.Join([]string{
					`{"type":"planned_change","change":{"resource":{"addr":"aws_s3_bucket.logs"},"action":"delete"}}`,
					`{"type":"planned_change","change":{"resource":{"addr":"aws_iam_role.eks"},"action":"delete"}}`,
					`{"type":"change_summary","changes":{"add":0,"change":0,"remove":2}}`,
					``,
				}, "\n"), nil
			}
			return "", nil
		}

		// When PlanDestroySummary runs
		results := stack.PlanDestroySummary(createTestBlueprint())

		// Then the plan command included -destroy and used destroy-mode env
		var sawDestroyFlag, sawAutoApprove bool
		for _, a := range capturedArgs {
			if a == "-destroy" {
				sawDestroyFlag = true
			}
			if a == "-auto-approve" {
				sawAutoApprove = true
			}
		}
		if !sawDestroyFlag {
			t.Errorf("expected -destroy flag in plan args, got %v", capturedArgs)
		}
		// terraform plan rejects -auto-approve (apply/destroy flag); leaking it in
		// from DestroyArgs caused every destroy plan to fail with exit status 1.
		if sawAutoApprove {
			t.Errorf("plan -destroy must not include -auto-approve, got args %v", capturedArgs)
		}
		if capturedEnv["TF_VAR_operation"] != "destroy" {
			t.Errorf("expected TF_VAR_operation=destroy, got %q", capturedEnv["TF_VAR_operation"])
		}
		// And resources came back tagged Delete
		if len(results) == 0 {
			t.Fatal("expected at least one result")
		}
		r := results[0]
		if r.Destroy != 2 {
			t.Errorf("expected destroy=2, got %d", r.Destroy)
		}
		if len(r.Resources) != 2 {
			t.Fatalf("expected 2 resources, got %#v", r.Resources)
		}
		for _, rc := range r.Resources {
			if rc.Action != ActionDelete {
				t.Errorf("expected ActionDelete, got %v for %q", rc.Action, rc.Address)
			}
		}
	})

	t.Run("AttachesDestroyBehaviorDriftForAResourceBeingDestroyed", func(t *testing.T) {
		// Given a destroy plan targeting aws_s3_bucket.logs, and a drift-check plan
		// reporting a curated attribute drifted on that same resource (cli#3328)
		stack, mocks := setup(t)
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if len(args) > 3 && args[1] == "show" && args[2] == "-json" {
				return `{"resource_changes":[{"address":"module.main.aws_s3_bucket.logs","change":{"actions":["update"],"before":{"force_destroy":false},"after":{"force_destroy":true}}}]}`, nil
			}
			if len(args) > 2 && args[1] == "show" && args[2] == "-json" {
				return `{"values":{"root_module":{"resources":[{"address":"aws_s3_bucket.logs"}]}}}`, nil
			}
			if len(args) > 1 && args[1] == "plan" && slices.Contains(args, "-destroy") {
				return `{"type":"planned_change","change":{"resource":{"addr":"aws_s3_bucket.logs"},"action":"delete"}}` + "\n" +
					`{"type":"change_summary","changes":{"add":0,"change":0,"remove":1}}` + "\n", nil
			}
			return "", nil
		}

		// When PlanDestroySummary runs
		results := stack.PlanDestroySummary(createTestBlueprint())

		// Then the drift is attached to the result naming the resource being destroyed
		if len(results) == 0 {
			t.Fatal("expected at least one result")
		}
		r := results[0]
		if len(r.DriftedAttributes) != 1 {
			t.Fatalf("expected 1 drifted attribute, got %#v", r.DriftedAttributes)
		}
		d := r.DriftedAttributes[0]
		if d.Address != "aws_s3_bucket.logs" || d.Attribute != "force_destroy" {
			t.Errorf("expected drift on aws_s3_bucket.logs/force_destroy, got %#v", d)
		}
	})

	t.Run("OmitsDriftForAResourceNotBeingDestroyed", func(t *testing.T) {
		// Given the drift-check plan reports drift on a resource absent from the
		// destroy plan's own resource list
		stack, mocks := setup(t)
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if len(args) > 3 && args[1] == "show" && args[2] == "-json" {
				return `{"resource_changes":[{"address":"module.main.aws_db_instance.untouched","change":{"actions":["update"],"before":{"deletion_protection":true},"after":{"deletion_protection":false}}}]}`, nil
			}
			if len(args) > 2 && args[1] == "show" && args[2] == "-json" {
				return `{"values":{"root_module":{"resources":[{"address":"aws_s3_bucket.logs"}]}}}`, nil
			}
			if len(args) > 1 && args[1] == "plan" && slices.Contains(args, "-destroy") {
				return `{"type":"planned_change","change":{"resource":{"addr":"aws_s3_bucket.logs"},"action":"delete"}}` + "\n" +
					`{"type":"change_summary","changes":{"add":0,"change":0,"remove":1}}` + "\n", nil
			}
			return "", nil
		}

		results := stack.PlanDestroySummary(createTestBlueprint())

		if len(results) == 0 {
			t.Fatal("expected at least one result")
		}
		if len(results[0].DriftedAttributes) != 0 {
			t.Errorf("expected no drift reported for a resource this destroy leaves untouched, got %#v", results[0].DriftedAttributes)
		}
	})

	t.Run("MarksComponentNewWhenStateIsEmpty", func(t *testing.T) {
		// Given empty state (renderer will show "(no state)")
		stack, mocks := setup(t)
		var planCalled bool
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if len(args) > 2 && args[1] == "show" && args[2] == "-json" {
				return "{}", nil
			}
			if len(args) > 1 && args[1] == "plan" {
				planCalled = true
			}
			return "", nil
		}

		results := stack.PlanDestroySummary(createTestBlueprint())

		if len(results) == 0 {
			t.Fatal("expected results")
		}
		for _, r := range results {
			if !r.IsNew {
				t.Errorf("expected IsNew=true (no state to destroy) for %q, got false", r.ComponentID)
			}
		}
		if planCalled {
			t.Error("plan must not be invoked when state is empty")
		}
	})

	t.Run("FiltersComponentsPinnedWithDestroyFalse", func(t *testing.T) {
		// Given a blueprint with one component pinned destroy=false
		stack, mocks := setup(t)
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if len(args) > 2 && args[1] == "show" && args[2] == "-json" {
				return `{"values":{"root_module":{"resources":[{"address":"x.y"}]}}}`, nil
			}
			if len(args) > 1 && args[1] == "plan" {
				return `{"type":"change_summary","changes":{"add":0,"change":0,"remove":1}}` + "\n", nil
			}
			return "", nil
		}
		falseVal := false
		falseBool := &blueprintv1alpha1.BoolExpression{Value: &falseVal}
		bp := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "t"},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Path: "a", Destroy: falseBool},
				{Path: "b"},
			},
		}

		results := stack.PlanDestroySummary(bp)

		// Then only the unpinned component shows up
		if len(results) != 1 {
			t.Fatalf("expected 1 result (b only), got %d: %#v", len(results), results)
		}
		if results[0].Path != "b" {
			t.Errorf("expected path=b, got %q", results[0].Path)
		}
	})

	t.Run("SurfacesTheRealDiagnosticOnPlanFailure", func(t *testing.T) {
		// Given a plan -destroy that fails with a real provider error, not a
		// prevent_destroy diagnostic
		stack, mocks := setup(t)
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if len(args) > 2 && args[1] == "show" && args[2] == "-json" {
				return `{"values":{"root_module":{"resources":[{"address":"google_container_cluster.main"}]}}}`, nil
			}
			if len(args) > 1 && args[1] == "plan" {
				output := `{"type":"diagnostic","diagnostic":{"severity":"error","summary":"Error 403: Permission denied","detail":"googleapi: Error 403: Required 'compute.instances.delete' permission"}}` + "\n"
				return output, fmt.Errorf("exit status 1")
			}
			return "", nil
		}
		bp := &blueprintv1alpha1.Blueprint{
			Metadata:            blueprintv1alpha1.Metadata{Name: "t"},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{{Path: "cluster"}},
		}

		// When PlanDestroySummary runs
		results := stack.PlanDestroySummary(bp)

		// Then the result's error carries the real diagnostic, not just "exit status 1"
		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d: %#v", len(results), results)
		}
		if results[0].Err == nil {
			t.Fatal("expected an error, got nil")
		}
		if !strings.Contains(results[0].Err.Error(), "Error 403: Permission denied") || !strings.Contains(results[0].Err.Error(), "compute.instances.delete") {
			t.Errorf("expected the real diagnostic in the error, got: %v", results[0].Err)
		}
	})

	t.Run("SurfacesARealErrorAlongsideAProtectedResource", func(t *testing.T) {
		// Given a plan -destroy that fails with both a prevent_destroy diagnostic
		// for one resource and an unrelated real error for another — the
		// protected-resource warning must never mask the real error
		stack, mocks := setup(t)
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if len(args) > 2 && args[1] == "show" && args[2] == "-json" {
				return `{"values":{"root_module":{"resources":[{"address":"x.y"}]}}}`, nil
			}
			if len(args) > 1 && args[1] == "plan" {
				output := strings.Join([]string{
					`{"type":"diagnostic","diagnostic":{"severity":"error","summary":"Instance cannot be destroyed","address":"null_resource.guarded"}}`,
					`{"type":"diagnostic","diagnostic":{"severity":"error","summary":"Error 403: Permission denied","detail":"googleapi: Error 403"}}`,
					``,
				}, "\n")
				return output, fmt.Errorf("exit status 1")
			}
			return "", nil
		}
		bp := &blueprintv1alpha1.Blueprint{
			Metadata:            blueprintv1alpha1.Metadata{Name: "t"},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{{Path: "cluster"}},
		}

		// When PlanDestroySummary runs
		results := stack.PlanDestroySummary(bp)

		// Then both the protected resource and the real error are reported
		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d: %#v", len(results), results)
		}
		r := results[0]
		if len(r.Protected) != 1 || r.Protected[0] != "null_resource.guarded" {
			t.Errorf("expected the protected resource reported, got: %#v", r.Protected)
		}
		if r.Err == nil {
			t.Fatal("expected the unrelated error to still be reported, got nil")
		}
		if !strings.Contains(r.Err.Error(), "Error 403: Permission denied") {
			t.Errorf("expected the real diagnostic in the error, got: %v", r.Err)
		}
	})
}

func TestStack_PlanDestroyComponentSummary(t *testing.T) {
	setup := func(t *testing.T) (*TerraformStack, *TerraformTestMocks) {
		t.Helper()
		mocks := setupWindsorStackMocks(t)
		stack := NewStack(mocks.Runtime).(*TerraformStack)
		stack.shims = mocks.Shims
		return stack, mocks
	}

	t.Run("ReturnsErrorForNilBlueprint", func(t *testing.T) {
		stack, _ := setup(t)
		if got := stack.PlanDestroyComponentSummary(nil, "x"); got.Err == nil {
			t.Error("expected error for nil blueprint, got nil")
		}
	})

	t.Run("ReturnsErrorForMissingComponent", func(t *testing.T) {
		stack, _ := setup(t)
		got := stack.PlanDestroyComponentSummary(createTestBlueprint(), "nonexistent")
		if got.Err == nil || !strings.Contains(got.Err.Error(), "not found") {
			t.Errorf("expected not-found error, got %v", got.Err)
		}
	})

	t.Run("ReturnsErrorForDestroyFalseComponent", func(t *testing.T) {
		// A pinned-destroy=false component returns an error rather than running
		// plan — Teardown would skip it, so producing a destroy plan would lie.
		stack, _ := setup(t)
		falseVal := false
		falseBool := &blueprintv1alpha1.BoolExpression{Value: &falseVal}
		bp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Name: "pinned", Path: "a", Destroy: falseBool},
			},
		}
		got := stack.PlanDestroyComponentSummary(bp, "pinned")
		if got.Err == nil || !strings.Contains(got.Err.Error(), "destroy=false") {
			t.Errorf("expected destroy=false error, got %v", got.Err)
		}
		if we := werror.Find(got.Err); we == nil || !strings.Contains(we.Remediation, "Set destroy to true") {
			t.Errorf("expected a destroy remediation, got %v", got.Err)
		}
	})

	t.Run("RunsForNamedComponent", func(t *testing.T) {
		stack, mocks := setup(t)
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if len(args) > 2 && args[1] == "show" && args[2] == "-json" {
				return `{"values":{"root_module":{"resources":[{"address":"x.y"}]}}}`, nil
			}
			if len(args) > 1 && args[1] == "plan" {
				return strings.Join([]string{
					`{"type":"planned_change","change":{"resource":{"addr":"aws_s3.bucket"},"action":"delete"}}`,
					`{"type":"change_summary","changes":{"add":0,"change":0,"remove":1}}`,
					``,
				}, "\n"), nil
			}
			return "", nil
		}
		bp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Name: "mycomp", Path: "local/path"},
			},
		}

		got := stack.PlanDestroyComponentSummary(bp, "mycomp")

		if got.Err != nil {
			t.Fatalf("unexpected error: %v", got.Err)
		}
		if got.Destroy != 1 {
			t.Errorf("expected destroy=1, got %d", got.Destroy)
		}
		if len(got.Resources) != 1 || got.Resources[0].Action != ActionDelete {
			t.Errorf("expected one Delete resource, got %#v", got.Resources)
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
			if len(args) > 2 && args[1] == "show" && args[2] == "-json" {
				return `{"values":{"root_module":{"resources":[{"address":"aws_s3_bucket.example"}]}}}`, nil
			}
			if len(args) > 1 && args[1] == "plan" {
				return strings.Join([]string{
					`{"type":"planned_change","change":{"resource":{"addr":"aws_s3_bucket.foo"},"action":"create"}}`,
					`{"type":"planned_change","change":{"resource":{"addr":"aws_s3_bucket.bar"},"action":"create"}}`,
					`{"type":"planned_change","change":{"resource":{"addr":"aws_iam_role.app"},"action":"update"}}`,
					`{"type":"change_summary","changes":{"add":2,"change":1,"remove":0}}`,
					``,
				}, "\n"), nil
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

		// Then counts are parsed from plan output and resources are populated
		if result.Err != nil {
			t.Fatalf("unexpected error: %v", result.Err)
		}
		if result.Add != 2 || result.Change != 1 || result.Destroy != 0 {
			t.Errorf("expected add=2 change=1 destroy=0, got add=%d change=%d destroy=%d", result.Add, result.Change, result.Destroy)
		}
		if len(result.Resources) != 3 {
			t.Errorf("expected 3 resources, got %d", len(result.Resources))
		}
	})

}

// =============================================================================
// Test Helpers
// =============================================================================

func TestParseTerraformPlanJSON(t *testing.T) {
	t.Run("ParsesCountsAndResourcesFromEventStream", func(t *testing.T) {
		// Given a terraform plan -json event stream with summary + planned changes
		output := strings.Join([]string{
			`{"type":"planned_change","change":{"resource":{"addr":"module.main.aws_s3_bucket.logs[0]"},"action":"create"}}`,
			`{"type":"planned_change","change":{"resource":{"addr":"module.main.aws_iam_role.eks"},"action":"update"}}`,
			`{"type":"planned_change","change":{"resource":{"addr":"module.main.aws_db_instance.primary"},"action":"replace"}}`,
			`{"type":"planned_change","change":{"resource":{"addr":"module.main.aws_security_group.legacy"},"action":"delete"}}`,
			`{"type":"planned_change","change":{"resource":{"addr":"module.main.data.aws_caller_identity.current"},"action":"read"}}`,
			`{"type":"change_summary","changes":{"add":2,"change":1,"remove":2}}`,
			``,
		}, "\n")

		// When parsed
		add, change, destroy, noChanges, resources := parseTerraformPlanJSON(output)

		// Then counts come from change_summary and noChanges is false
		if add != 2 || change != 1 || destroy != 2 {
			t.Errorf("expected +2 ~1 -2, got +%d ~%d -%d", add, change, destroy)
		}
		if noChanges {
			t.Error("expected noChanges=false when counts are non-zero")
		}
		// And resources exclude the read action and strip the module.main. prefix
		want := []ResourceChange{
			{Address: "aws_s3_bucket.logs[0]", Action: ActionCreate},
			{Address: "aws_iam_role.eks", Action: ActionUpdate},
			{Address: "aws_db_instance.primary", Action: ActionReplace},
			{Address: "aws_security_group.legacy", Action: ActionDelete},
		}
		if len(resources) != len(want) {
			t.Fatalf("expected %d resources, got %d: %#v", len(want), len(resources), resources)
		}
		for i, r := range resources {
			if r != want[i] {
				t.Errorf("resource[%d]: expected %#v, got %#v", i, want[i], r)
			}
		}
	})

	t.Run("SetsNoChangesWhenSummaryReportsZero", func(t *testing.T) {
		// Given a stream with only an all-zero change_summary
		output := `{"type":"change_summary","changes":{"add":0,"change":0,"remove":0}}` + "\n"

		// When parsed
		add, change, destroy, noChanges, resources := parseTerraformPlanJSON(output)

		// Then noChanges is true and the resource list is empty
		if !noChanges {
			t.Error("expected noChanges=true")
		}
		if add != 0 || change != 0 || destroy != 0 {
			t.Errorf("expected zero counts, got +%d ~%d -%d", add, change, destroy)
		}
		if len(resources) != 0 {
			t.Errorf("expected empty resources, got %#v", resources)
		}
	})

	t.Run("DropsNoopAndUnknownActions", func(t *testing.T) {
		// Given planned changes with noop and a made-up action
		output := strings.Join([]string{
			`{"type":"planned_change","change":{"resource":{"addr":"aws_s3_bucket.foo"},"action":"noop"}}`,
			`{"type":"planned_change","change":{"resource":{"addr":"aws_s3_bucket.bar"},"action":"frobnicate"}}`,
			``,
		}, "\n")

		// When parsed
		_, _, _, _, resources := parseTerraformPlanJSON(output)

		// Then both are dropped
		if len(resources) != 0 {
			t.Errorf("expected no resources, got %#v", resources)
		}
	})

	t.Run("IgnoresNonJSONLines", func(t *testing.T) {
		// Given mixed text and JSON
		output := "Initialising...\n" +
			`{"type":"change_summary","changes":{"add":1,"change":0,"remove":0}}` + "\n" +
			"trailing junk\n"

		// When parsed
		add, _, _, noChanges, _ := parseTerraformPlanJSON(output)

		// Then the summary is still extracted and counts are non-zero
		if add != 1 {
			t.Errorf("expected add=1, got %d", add)
		}
		if noChanges {
			t.Error("expected noChanges=false when add>0")
		}
	})

	t.Run("LeavesNoChangesFalseForEmptyOutput", func(t *testing.T) {
		// Given empty output (no events at all)
		_, _, _, noChanges, _ := parseTerraformPlanJSON("")

		// Then noChanges remains false — distinguishes "no output" from "no diff"
		if noChanges {
			t.Error("expected noChanges=false for empty output")
		}
	})
}

func TestExtractPlanErrorDiagnostics(t *testing.T) {
	t.Run("ReturnsNilForEmptyOrNonDiagnosticOutput", func(t *testing.T) {
		// Given an event stream with only planned_change and change_summary events
		output := strings.Join([]string{
			`{"type":"planned_change","change":{"resource":{"addr":"module.main.aws_s3_bucket.x"},"action":"delete"}}`,
			`{"type":"change_summary","changes":{"add":0,"change":0,"remove":1}}`,
			``,
		}, "\n")

		// When extracting error diagnostics
		got := extractPlanErrorDiagnostics(output)

		// Then none are reported
		if got != nil {
			t.Errorf("expected nil, got %#v", got)
		}
		if extractPlanErrorDiagnostics("") != nil {
			t.Error("expected nil for empty output")
		}
	})

	t.Run("ExtractsSummaryAndDetailForEveryErrorDiagnostic", func(t *testing.T) {
		// Given a provider error diagnostic like a real destroy-plan failure would emit
		output := strings.Join([]string{
			`Initialising backend...`,
			`{"type":"diagnostic","diagnostic":{"severity":"error","summary":"Error 403: Permission denied","detail":"googleapi: Error 403: Required 'compute.instances.delete' permission, forbidden"}}`,
			`trailing junk`,
			``,
		}, "\n")

		// When extracting
		got := extractPlanErrorDiagnostics(output)

		// Then the summary and detail both reach the caller
		if len(got) != 1 {
			t.Fatalf("expected 1 diagnostic, got %d: %#v", len(got), got)
		}
		if !strings.Contains(got[0], "Error 403: Permission denied") || !strings.Contains(got[0], "compute.instances.delete") {
			t.Errorf("expected summary and detail both present, got %q", got[0])
		}
	})

	t.Run("OmitsDetailWhenAbsent", func(t *testing.T) {
		// Given a diagnostic with only a summary
		output := `{"type":"diagnostic","diagnostic":{"severity":"error","summary":"Provider produced inconsistent result"}}`

		// When extracting
		got := extractPlanErrorDiagnostics(output)

		// Then the bare summary is reported with no trailing separator
		if len(got) != 1 || got[0] != "Provider produced inconsistent result" {
			t.Errorf("expected bare summary, got %#v", got)
		}
	})

	t.Run("IgnoresNonErrorSeverityAndNonDiagnosticEvents", func(t *testing.T) {
		// Given a warning and a differently-typed event carrying a diagnostic field
		output := strings.Join([]string{
			`{"type":"diagnostic","diagnostic":{"severity":"warning","summary":"deprecated argument"}}`,
			`{"type":"some_other_event","diagnostic":{"severity":"error","summary":"should not surface"}}`,
			``,
		}, "\n")

		// When extracting
		got := extractPlanErrorDiagnostics(output)

		// Then neither qualifies
		if got != nil {
			t.Errorf("expected nil, got %#v", got)
		}
	})

	t.Run("CollapsesDuplicateMessages", func(t *testing.T) {
		// Given the same diagnostic repeated, e.g. once per affected resource instance
		output := strings.Join([]string{
			`{"type":"diagnostic","diagnostic":{"severity":"error","summary":"Error 500","detail":"internal error"}}`,
			`{"type":"diagnostic","diagnostic":{"severity":"error","summary":"Error 500","detail":"internal error"}}`,
			``,
		}, "\n")

		// When extracting
		got := extractPlanErrorDiagnostics(output)

		// Then it is reported once
		if len(got) != 1 {
			t.Errorf("expected duplicates collapsed to 1, got %d: %#v", len(got), got)
		}
	})

	t.Run("FlattensEmbeddedNewlinesToOneLine", func(t *testing.T) {
		// Given a detail that spans multiple lines — callers render each returned
		// line as its own bullet, so an unflattened detail would look like
		// several distinct diagnostics
		output := `{"type":"diagnostic","diagnostic":{"severity":"error","summary":"Error 403","detail":"googleapi: Error 403:\nRequired permission denied\nfor project x"}}`

		// When extracting
		got := extractPlanErrorDiagnostics(output)

		// Then it is reported as a single, newline-free line
		if len(got) != 1 {
			t.Fatalf("expected 1 diagnostic, got %d: %#v", len(got), got)
		}
		if strings.Contains(got[0], "\n") {
			t.Errorf("expected no embedded newlines, got %q", got[0])
		}
	})
}
