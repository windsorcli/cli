package terraform

// Test coverage for stack_show.go: PlanResourceChangesJSON, PlanAllResourceChangesJSON,
// and the shared planResourceChangesJSON helper.

import (
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestStack_PlanResourceChangesJSON(t *testing.T) {
	setup := func(t *testing.T) (*TerraformStack, *TerraformTestMocks) {
		t.Helper()
		mocks := setupWindsorStackMocks(t)
		stack := NewStack(mocks.Runtime).(*TerraformStack)
		stack.shims = mocks.Shims
		return stack, mocks
	}

	t.Run("PrintsShowOutputToStdout", func(t *testing.T) {
		// Given a stack whose show -json returns a fixed plan document
		stack, mocks := setup(t)
		blueprint := createTestBlueprint()
		mocks.Shell.ExecCaptureWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if len(args) > 1 && args[1] == "show" {
				return `{"format_version":"1.2","resource_changes":[]}`, nil
			}
			return "", nil
		}

		r, w, pipeErr := os.Pipe()
		if pipeErr != nil {
			t.Fatalf("Pipe failed: %v", pipeErr)
		}
		origStdout := os.Stdout
		os.Stdout = w
		defer func() { os.Stdout = origStdout }()

		// When PlanResourceChangesJSON is called
		err := stack.PlanResourceChangesJSON(blueprint, "local/path")

		w.Close()
		stdoutBytes, _ := io.ReadAll(r)

		// Then the plan document is printed to stdout, compacted onto one line
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := string(stdoutBytes); got != "{\"format_version\":\"1.2\",\"resource_changes\":[]}\n" {
			t.Errorf("unexpected stdout: %q", got)
		}
	})

	t.Run("RunsPlanBeforeShowOnThePlanFile", func(t *testing.T) {
		// Given a stack that records the order and args of plan/show invocations
		stack, mocks := setup(t)
		blueprint := createTestBlueprint()
		var planArgs, showArgs []string
		mocks.Shell.ExecCaptureWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if len(args) > 1 && args[1] == "plan" {
				planArgs = args
				return "", nil
			}
			if len(args) > 1 && args[1] == "show" {
				showArgs = args
				return "{}", nil
			}
			return "", nil
		}

		// When PlanResourceChangesJSON is called
		err := stack.PlanResourceChangesJSON(blueprint, "local/path")

		// Then plan writes a plan file, and show reads that same plan file as JSON
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if planArgs == nil {
			t.Fatal("expected terraform plan to be invoked")
		}
		var outArg string
		for _, a := range planArgs {
			if strings.HasPrefix(a, "-out=") {
				outArg = strings.TrimPrefix(a, "-out=")
			}
		}
		if outArg == "" {
			t.Fatal("expected plan args to include -out=<planfile>")
		}
		if showArgs == nil {
			t.Fatal("expected terraform show to be invoked")
		}
		if showArgs[len(showArgs)-1] != outArg {
			t.Errorf("expected show to read the plan file %q, got args %v", outArg, showArgs)
		}
		if showArgs[1] != "show" || showArgs[2] != "-json" {
			t.Errorf("expected show args to be [show -json <planfile>], got %v", showArgs)
		}
	})

	t.Run("ReturnsErrorForNilBlueprint", func(t *testing.T) {
		// Given a stack with a nil blueprint
		stack, _ := setup(t)

		// When PlanResourceChangesJSON is called
		err := stack.PlanResourceChangesJSON(nil, "local/path")

		// Then an error is returned
		if err == nil {
			t.Error("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "blueprint not provided") {
			t.Errorf("expected blueprint not provided error, got: %v", err)
		}
	})

	t.Run("ReturnsErrorForEmptyComponentID", func(t *testing.T) {
		// Given a stack with a blueprint
		stack, _ := setup(t)
		blueprint := createTestBlueprint()

		// When PlanResourceChangesJSON is called with an empty component ID
		err := stack.PlanResourceChangesJSON(blueprint, "")

		// Then an error is returned
		if err == nil {
			t.Error("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "component ID not provided") {
			t.Errorf("expected component ID error, got: %v", err)
		}
	})

	t.Run("ReturnsErrorForUnknownComponent", func(t *testing.T) {
		// Given a stack and a blueprint with known components
		stack, _ := setup(t)
		blueprint := createTestBlueprint()

		// When PlanResourceChangesJSON is called for a component that does not exist
		err := stack.PlanResourceChangesJSON(blueprint, "does/not/exist")

		// Then an error names the missing component
		if err == nil {
			t.Error("expected error, got nil")
		}
		if !strings.Contains(err.Error(), `"does/not/exist" not found`) {
			t.Errorf("expected not found error, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenPlanFails", func(t *testing.T) {
		// Given a stack whose terraform plan fails
		stack, mocks := setup(t)
		blueprint := createTestBlueprint()
		mocks.Shell.ExecCaptureWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if len(args) > 1 && args[1] == "plan" {
				return "", fmt.Errorf("mock error running terraform plan")
			}
			return "", nil
		}

		// When PlanResourceChangesJSON is called
		err := stack.PlanResourceChangesJSON(blueprint, "local/path")

		// Then the plan failure is surfaced
		if err == nil {
			t.Error("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error running terraform plan for") {
			t.Errorf("expected plan error, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenShowFails", func(t *testing.T) {
		// Given a stack whose terraform show -json fails
		stack, mocks := setup(t)
		blueprint := createTestBlueprint()
		mocks.Shell.ExecCaptureWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if len(args) > 1 && args[1] == "show" {
				return "", fmt.Errorf("mock error")
			}
			return "", nil
		}

		// When PlanResourceChangesJSON is called
		err := stack.PlanResourceChangesJSON(blueprint, "local/path")

		// Then the show failure is surfaced
		if err == nil {
			t.Error("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error reading terraform plan JSON for") {
			t.Errorf("expected show error, got: %v", err)
		}
	})

	t.Run("ReturnsErrorWhenShowOutputIsNotValidJSON", func(t *testing.T) {
		// Given a stack whose terraform show -json returns malformed output
		stack, mocks := setup(t)
		blueprint := createTestBlueprint()
		mocks.Shell.ExecCaptureWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if len(args) > 1 && args[1] == "show" {
				return "not json", nil
			}
			return "", nil
		}

		// When PlanResourceChangesJSON is called
		err := stack.PlanResourceChangesJSON(blueprint, "local/path")

		// Then a parse error is surfaced
		if err == nil {
			t.Error("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error parsing terraform plan JSON for") {
			t.Errorf("expected parse error, got: %v", err)
		}
	})

	t.Run("NeverStreamsPlanOrShowThroughTheVerboseEchoPath", func(t *testing.T) {
		// Given a stack whose ExecSilentWithEnv fails the test if called for plan or show:
		// both steps can carry sensitive plaintext, and ExecSilentWithEnv echoes its
		// captured output to the terminal under --verbose, unlike ExecCaptureWithEnv. Init
		// output is not sensitive plan data and is exempt.
		stack, mocks := setup(t)
		blueprint := createTestBlueprint()
		mocks.Shell.ExecSilentWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if len(args) > 1 && (args[1] == "plan" || args[1] == "show") {
				t.Fatalf("unexpected ExecSilentWithEnv call with args %v; plan and show must use ExecCaptureWithEnv", args)
			}
			return "", nil
		}
		mocks.Shell.ExecCaptureWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			return "{}", nil
		}

		// When PlanResourceChangesJSON is called
		err := stack.PlanResourceChangesJSON(blueprint, "local/path")

		// Then no error occurs and ExecSilentWithEnv was never reached for plan/show
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestStack_PlanAllResourceChangesJSON(t *testing.T) {
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

		// When PlanAllResourceChangesJSON is called
		err := stack.PlanAllResourceChangesJSON(nil)

		// Then an error is returned
		if err == nil {
			t.Error("expected error, got nil")
		}
	})

	t.Run("ReturnsErrorForEmptyProjectRoot", func(t *testing.T) {
		// Given a stack with an empty project root
		stack, mocks := setup(t)
		mocks.Runtime.ProjectRoot = ""
		blueprint := createTestBlueprint()

		// When PlanAllResourceChangesJSON is called
		err := stack.PlanAllResourceChangesJSON(blueprint)

		// Then an error is returned
		if err == nil {
			t.Error("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "project root is empty") {
			t.Errorf("expected project root error, got: %v", err)
		}
	})

	t.Run("RunsPlanAndShowForEveryComponent", func(t *testing.T) {
		// Given a stack with multiple components in the blueprint
		stack, mocks := setup(t)
		var planCalls, showCalls int
		mocks.Shell.ExecCaptureWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" && len(args) > 1 && args[1] == "plan" {
				planCalls++
			}
			if command == "terraform" && len(args) > 1 && args[1] == "show" {
				showCalls++
			}
			return "{}", nil
		}
		bp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Name: "alpha", Path: "local/alpha"},
				{Name: "beta", Path: "local/beta"},
			},
		}

		// When PlanAllResourceChangesJSON is called
		err := stack.PlanAllResourceChangesJSON(bp)

		// Then both components are planned and shown
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if planCalls != 2 {
			t.Errorf("expected 2 plan calls, got %d", planCalls)
		}
		if showCalls != 2 {
			t.Errorf("expected 2 show calls, got %d", showCalls)
		}
	})

	t.Run("StopsOnFirstError", func(t *testing.T) {
		// Given a stack whose show -json fails for the first component
		stack, mocks := setup(t)
		var showCalls int
		mocks.Shell.ExecCaptureWithEnvFunc = func(command string, env map[string]string, args ...string) (string, error) {
			if command == "terraform" && len(args) > 1 && args[1] == "show" {
				showCalls++
				return "", fmt.Errorf("mock error")
			}
			return "", nil
		}
		bp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Name: "alpha", Path: "local/alpha"},
				{Name: "beta", Path: "local/beta"},
			},
		}

		// When PlanAllResourceChangesJSON is called
		err := stack.PlanAllResourceChangesJSON(bp)

		// Then the first failure stops the loop before the second component runs
		if err == nil {
			t.Error("expected error, got nil")
		}
		if showCalls != 1 {
			t.Errorf("expected 1 show call before stopping, got %d", showCalls)
		}
	})
}
