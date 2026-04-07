package cmd

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/composer"
	"github.com/windsorcli/cli/pkg/composer/blueprint"
	"github.com/windsorcli/cli/pkg/project"
	"github.com/windsorcli/cli/pkg/provisioner"
	fluxinfra "github.com/windsorcli/cli/pkg/provisioner/flux"
	terraforminfra "github.com/windsorcli/cli/pkg/provisioner/terraform"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// =============================================================================
// Test Setup
// =============================================================================

type PlanMocks struct {
	ConfigHandler    config.ConfigHandler
	Shell            *shell.MockShell
	BlueprintHandler *blueprint.MockBlueprintHandler
	TerraformStack   *terraforminfra.MockStack
	Runtime          *runtime.Runtime
	TmpDir           string
}

func setupPlanTest(t *testing.T, opts ...*SetupOptions) *PlanMocks {
	t.Helper()

	mockConfigHandler := config.NewMockConfigHandler()
	mockConfigHandler.GetContextFunc = func() string { return "test-context" }
	mockConfigHandler.IsDevModeFunc = func(contextName string) bool { return false }
	mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return ""
	}
	mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
		switch key {
		case "terraform.enabled":
			return true
		default:
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return false
		}
	}
	mockConfigHandler.IsLoadedFunc = func() bool { return true }
	mockConfigHandler.LoadConfigFunc = func() error { return nil }
	mockConfigHandler.SaveConfigFunc = func(hasSetFlags ...bool) error { return nil }
	mockConfigHandler.GenerateContextIDFunc = func() error { return nil }

	testOpts := &SetupOptions{}
	if len(opts) > 0 && opts[0] != nil {
		testOpts = opts[0]
	}
	testOpts.ConfigHandler = mockConfigHandler
	baseMocks := setupMocks(t, testOpts)
	tmpDir := baseMocks.TmpDir
	mockConfigHandler.GetConfigRootFunc = func() (string, error) { return tmpDir + "/contexts/test-context", nil }

	baseMocks.Shell.CheckTrustedDirectoryFunc = func() error { return nil }

	mockBlueprintHandler := blueprint.NewMockBlueprintHandler()
	mockBlueprintHandler.LoadBlueprintFunc = func(...string) error { return nil }
	mockBlueprintHandler.WriteFunc = func(overwrite ...bool) error { return nil }
	testBlueprint := &blueprintv1alpha1.Blueprint{
		Metadata: blueprintv1alpha1.Metadata{Name: "test"},
	}
	mockBlueprintHandler.GenerateFunc = func() *blueprintv1alpha1.Blueprint { return testBlueprint }

	mockTerraformStack := terraforminfra.NewMockStack()
	mockTerraformStack.PlanFunc = func(blueprint *blueprintv1alpha1.Blueprint, componentID string) error { return nil }

	rt := runtime.NewRuntime(&runtime.Runtime{
		Shell:         baseMocks.Shell,
		ConfigHandler: baseMocks.ConfigHandler,
		ProjectRoot:   tmpDir,
		ToolsManager:  baseMocks.ToolsManager,
	})
	if rt == nil {
		t.Fatal("Failed to create runtime")
	}

	return &PlanMocks{
		ConfigHandler:    baseMocks.ConfigHandler,
		Shell:            baseMocks.Shell,
		BlueprintHandler: mockBlueprintHandler,
		TerraformStack:   mockTerraformStack,
		Runtime:          rt,
		TmpDir:           tmpDir,
	}
}

// newPlanProject is a helper that wires mocks into a project for plan tests.
func newPlanProject(mocks *PlanMocks) *project.Project {
	comp := composer.NewComposer(mocks.Runtime)
	comp.BlueprintHandler = mocks.BlueprintHandler
	mockProvisioner := provisioner.NewProvisioner(mocks.Runtime, comp.BlueprintHandler, &provisioner.Provisioner{
		TerraformStack: mocks.TerraformStack,
	})
	return project.NewProject("", &project.Project{
		Runtime:     mocks.Runtime,
		Composer:    comp,
		Provisioner: mockProvisioner,
	})
}

// newKustomizePlanProject wires a FluxStack mock into a project for kustomize plan tests.
func newKustomizePlanProject(mocks *PlanMocks, fluxStack *fluxinfra.MockStack) *project.Project {
	comp := composer.NewComposer(mocks.Runtime)
	comp.BlueprintHandler = mocks.BlueprintHandler
	mockProvisioner := provisioner.NewProvisioner(mocks.Runtime, comp.BlueprintHandler, &provisioner.Provisioner{
		FluxStack: fluxStack,
	})
	return project.NewProject("", &project.Project{
		Runtime:     mocks.Runtime,
		Composer:    comp,
		Provisioner: mockProvisioner,
	})
}

// =============================================================================
// Test Cases
// =============================================================================

func TestPlanTerraformCmd(t *testing.T) {
	createTestPlanTerraformCmd := func() *cobra.Command {
		cmd := &cobra.Command{
			Use:  "terraform",
			RunE: planTerraformCmd.RunE,
		}
		planTerraformCmd.Flags().VisitAll(func(flag *pflag.Flag) {
			cmd.Flags().AddFlag(flag)
		})
		cmd.Args = planTerraformCmd.Args
		cmd.SilenceUsage = true
		cmd.SilenceErrors = true
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)
		return cmd
	}

	suppressProcessStdout(t)
	suppressProcessStderr(t)

	t.Run("Success", func(t *testing.T) {
		// Given a properly configured plan terraform command
		mocks := setupPlanTest(t)
		proj := newPlanProject(mocks)

		// When executing the plan terraform command with a component ID
		cmd := createTestPlanTerraformCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetArgs([]string{"cluster"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("SuccessNoArgStreamsAllComponents", func(t *testing.T) {
		// Given a plan terraform command with no arguments
		mocks := setupPlanTest(t)
		planAllCalled := false
		mocks.TerraformStack.PlanAllFunc = func(bp *blueprintv1alpha1.Blueprint) error {
			planAllCalled = true
			return nil
		}
		proj := newPlanProject(mocks)

		// When executing without a component ID
		cmd := createTestPlanTerraformCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then PlanAll is called (streaming), not PlanSummary
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if !planAllCalled {
			t.Error("expected PlanAll to be called for no-arg non-JSON path")
		}
	})

	t.Run("SuccessSummaryFlag", func(t *testing.T) {
		// Given a plan terraform command with --summary
		mocks := setupPlanTest(t)
		summaryCalled := false
		mocks.TerraformStack.PlanSummaryFunc = func(bp *blueprintv1alpha1.Blueprint) []terraforminfra.TerraformComponentPlan {
			summaryCalled = true
			return nil
		}
		proj := newPlanProject(mocks)

		// When executing with --summary
		cmd := createTestPlanTerraformCmd()
		planSummary = true
		t.Cleanup(func() { planSummary = false })
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then PlanSummary is called, not PlanAll
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if !summaryCalled {
			t.Error("expected PlanSummary to be called with --summary flag")
		}
	})

	t.Run("ErrorCheckingTrustedDirectory", func(t *testing.T) {
		// Given a plan terraform command with an untrusted directory
		mocks := setupPlanTest(t)
		mocks.Shell.CheckTrustedDirectoryFunc = func() error {
			return fmt.Errorf("not in trusted directory")
		}
		proj := newPlanProject(mocks)

		// When executing the plan terraform command
		cmd := createTestPlanTerraformCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetArgs([]string{"cluster"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "not in a trusted directory") {
			t.Errorf("Expected trusted directory error, got: %v", err)
		}
	})

	t.Run("ErrorPlanFails", func(t *testing.T) {
		// Given a plan terraform command where the plan operation fails
		mocks := setupPlanTest(t)
		mocks.TerraformStack.PlanFunc = func(blueprint *blueprintv1alpha1.Blueprint, componentID string) error {
			return fmt.Errorf("component %q not found", componentID)
		}
		proj := newPlanProject(mocks)

		// When executing the plan terraform command
		cmd := createTestPlanTerraformCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetArgs([]string{"nonexistent"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error planning terraform") {
			t.Errorf("Expected plan error, got: %v", err)
		}
	})

	t.Run("ErrorLoadingConfig", func(t *testing.T) {
		// Given a plan terraform command with config load failure
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.LoadConfigFunc = func() error {
			return fmt.Errorf("config load failed")
		}

		opts := &SetupOptions{ConfigHandler: mockConfigHandler}
		mocks := setupPlanTest(t, opts)
		mocks.Runtime.ConfigHandler = mockConfigHandler
		proj := project.NewProject("", &project.Project{Runtime: mocks.Runtime})

		// When executing the plan terraform command
		cmd := createTestPlanTerraformCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetArgs([]string{"cluster"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}
	})
}

func TestPlanKustomizeCmd(t *testing.T) {
	createTestCmd := func(use string) *cobra.Command {
		src := planKustomizeCmd
		cmd := &cobra.Command{
			Use:     use,
			Aliases: src.Aliases,
			RunE:    src.RunE,
		}
		src.Flags().VisitAll(func(flag *pflag.Flag) { cmd.Flags().AddFlag(flag) })
		cmd.Args = src.Args
		cmd.SilenceUsage = true
		cmd.SilenceErrors = true
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)
		return cmd
	}

	suppressProcessStdout(t)
	suppressProcessStderr(t)

	t.Run("Success", func(t *testing.T) {
		// Given a properly configured plan kustomize command
		mocks := setupPlanTest(t)
		fluxStack := fluxinfra.NewMockStack()
		proj := newKustomizePlanProject(mocks, fluxStack)

		// When executing with a component ID
		cmd := createTestCmd("kustomize")
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetArgs([]string{"my-app"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then no error is returned
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("Success_AllAlias", func(t *testing.T) {
		// Given a plan kustomize command with "all" as the component ID
		mocks := setupPlanTest(t)
		fluxStack := fluxinfra.NewMockStack()
		proj := newKustomizePlanProject(mocks, fluxStack)

		// When executing with "all"
		cmd := createTestCmd("kustomize")
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetArgs([]string{"all"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then no error is returned
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("Success_K8sAlias", func(t *testing.T) {
		// Given the command is invoked via the k8s alias
		mocks := setupPlanTest(t)
		fluxStack := fluxinfra.NewMockStack()
		proj := newKustomizePlanProject(mocks, fluxStack)

		// When executing as "k8s"
		cmd := createTestCmd("k8s")
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetArgs([]string{"my-app"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then no error is returned
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("SuccessNoArgStreamsAllKustomizations", func(t *testing.T) {
		// Given a plan kustomize command with no arguments
		mocks := setupPlanTest(t)
		fluxStack := fluxinfra.NewMockStack()
		planAllCalled := false
		fluxStack.PlanAllFunc = func(bp *blueprintv1alpha1.Blueprint) error {
			planAllCalled = true
			return nil
		}
		proj := newKustomizePlanProject(mocks, fluxStack)

		// When executing without a component ID
		cmd := createTestCmd("kustomize")
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then PlanAll is called (streaming), not PlanSummary
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if !planAllCalled {
			t.Error("expected PlanAll to be called for no-arg non-summary path")
		}
	})

	t.Run("SuccessSummaryFlag", func(t *testing.T) {
		// Given a plan kustomize command with --summary
		mocks := setupPlanTest(t)
		fluxStack := fluxinfra.NewMockStack()
		summaryCalled := false
		fluxStack.PlanSummaryFunc = func(bp *blueprintv1alpha1.Blueprint) ([]fluxinfra.KustomizePlan, []string) {
			summaryCalled = true
			return nil, nil
		}
		proj := newKustomizePlanProject(mocks, fluxStack)

		// When executing with --summary
		cmd := createTestCmd("kustomize")
		planSummary = true
		t.Cleanup(func() { planSummary = false })
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then PlanSummary is called, not Plan("all")
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if !summaryCalled {
			t.Error("expected PlanSummary to be called with --summary flag")
		}
	})

	t.Run("ErrorCheckingTrustedDirectory", func(t *testing.T) {
		// Given an untrusted working directory
		mocks := setupPlanTest(t)
		mocks.Shell.CheckTrustedDirectoryFunc = func() error {
			return fmt.Errorf("not in trusted directory")
		}
		fluxStack := fluxinfra.NewMockStack()
		proj := newKustomizePlanProject(mocks, fluxStack)

		// When executing the command
		cmd := createTestCmd("kustomize")
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetArgs([]string{"my-app"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then a trusted directory error is returned
		if err == nil {
			t.Error("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "not in a trusted directory") {
			t.Errorf("expected trusted directory error, got: %v", err)
		}
	})

	t.Run("ErrorPlanFails", func(t *testing.T) {
		// Given a flux stack that returns an error
		mocks := setupPlanTest(t)
		fluxStack := fluxinfra.NewMockStack()
		fluxStack.PlanFunc = func(bp *blueprintv1alpha1.Blueprint, componentID string) error {
			return fmt.Errorf("flux diff failed")
		}
		proj := newKustomizePlanProject(mocks, fluxStack)

		// When executing the command
		cmd := createTestCmd("kustomize")
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetArgs([]string{"my-app"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then the error is propagated
		if err == nil {
			t.Error("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error planning kustomize") {
			t.Errorf("expected planning error, got: %v", err)
		}
	})

	t.Run("ErrorLoadingConfig", func(t *testing.T) {
		// Given a config handler that fails to load
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.LoadConfigFunc = func() error {
			return fmt.Errorf("config load failed")
		}
		opts := &SetupOptions{ConfigHandler: mockConfigHandler}
		mocks := setupPlanTest(t, opts)
		mocks.Runtime.ConfigHandler = mockConfigHandler
		proj := project.NewProject("", &project.Project{Runtime: mocks.Runtime})

		// When executing the command
		cmd := createTestCmd("kustomize")
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetArgs([]string{"my-app"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then an error is returned
		if err == nil {
			t.Error("expected error, got nil")
		}
	})
}
