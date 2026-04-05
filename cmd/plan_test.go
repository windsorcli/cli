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

	t.Run("SuccessNoArgPlansSummary", func(t *testing.T) {
		// Given a plan terraform command with no arguments
		mocks := setupPlanTest(t)
		mocks.TerraformStack.PlanSummaryFunc = func(bp *blueprintv1alpha1.Blueprint) []terraforminfra.TerraformComponentPlan {
			return []terraforminfra.TerraformComponentPlan{{ComponentID: "cluster", Add: 2}}
		}
		proj := newPlanProject(mocks)

		// When executing without a component ID
		cmd := createTestPlanTerraformCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then no error occurs and summary is printed
		if err != nil {
			t.Errorf("expected no error when no arg given, got %v", err)
		}
	})

	t.Run("SuccessJSONWithSpecificComponent", func(t *testing.T) {
		// Given a plan terraform command with --json and a specific component
		mocks := setupPlanTest(t)
		mocks.TerraformStack.PlanSummaryFunc = func(bp *blueprintv1alpha1.Blueprint) []terraforminfra.TerraformComponentPlan {
			return []terraforminfra.TerraformComponentPlan{
				{ComponentID: "cluster", Add: 3},
				{ComponentID: "networking", NoChanges: true},
			}
		}
		proj := newPlanProject(mocks)

		// When executing with --json and a specific component
		cmd := createTestPlanTerraformCmd()
		planJSON = true
		t.Cleanup(func() { planJSON = false })
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetArgs([]string{"cluster"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then no error occurs and only the named component appears in output
		if err != nil {
			t.Errorf("expected no error, got %v", err)
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

func TestPlanCmd(t *testing.T) {
	createTestCmd := func() *cobra.Command {
		cmd := &cobra.Command{
			Use:          "plan",
			RunE:         planCmd.RunE,
			SilenceUsage: true,
			SilenceErrors: true,
		}
		planCmd.Flags().VisitAll(func(flag *pflag.Flag) { cmd.Flags().AddFlag(flag) })
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)
		return cmd
	}

	suppressProcessStdout(t)
	suppressProcessStderr(t)

	t.Run("Success", func(t *testing.T) {
		// Given a properly configured project with terraform and flux stacks
		mocks := setupPlanTest(t)
		fluxStack := fluxinfra.NewMockStack()
		comp := composer.NewComposer(mocks.Runtime)
		comp.BlueprintHandler = mocks.BlueprintHandler
		mockProvisioner := provisioner.NewProvisioner(mocks.Runtime, comp.BlueprintHandler, &provisioner.Provisioner{
			TerraformStack: mocks.TerraformStack,
			FluxStack:      fluxStack,
		})
		proj := project.NewProject("", &project.Project{
			Runtime:     mocks.Runtime,
			Composer:    comp,
			Provisioner: mockProvisioner,
		})

		// When the plan command is executed without subcommand
		cmd := createTestCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then no error is returned
		if err != nil {
			t.Errorf("expected no error, got %v", err)
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

		// When the plan command is executed
		cmd := createTestCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
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

		// When the plan command is executed
		cmd := createTestCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then an error is returned
		if err == nil {
			t.Error("expected error, got nil")
		}
	})

	t.Run("SuccessComponentInTerraformOnly", func(t *testing.T) {
		// Given a blueprint with a terraform component named "compute"
		mocks := setupPlanTest(t)
		mocks.TerraformStack.PlanFunc = func(bp *blueprintv1alpha1.Blueprint, id string) error { return nil }
		bp := &blueprintv1alpha1.Blueprint{
			Metadata:            blueprintv1alpha1.Metadata{Name: "test"},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{{Path: "compute"}},
		}
		mocks.BlueprintHandler.GenerateFunc = func() *blueprintv1alpha1.Blueprint { return bp }
		fluxStack := fluxinfra.NewMockStack()
		comp := composer.NewComposer(mocks.Runtime)
		comp.BlueprintHandler = mocks.BlueprintHandler
		mockProvisioner := provisioner.NewProvisioner(mocks.Runtime, comp.BlueprintHandler, &provisioner.Provisioner{
			TerraformStack: mocks.TerraformStack,
			FluxStack:      fluxStack,
		})
		proj := project.NewProject("", &project.Project{Runtime: mocks.Runtime, Composer: comp, Provisioner: mockProvisioner})

		// When the plan command is executed with a component name
		cmd := createTestCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetArgs([]string{"compute"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then no error is returned
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("SuccessComponentInKustomizeOnly", func(t *testing.T) {
		// Given a blueprint with a kustomization named "csi"
		mocks := setupPlanTest(t)
		bp := &blueprintv1alpha1.Blueprint{
			Metadata:       blueprintv1alpha1.Metadata{Name: "test"},
			Kustomizations: []blueprintv1alpha1.Kustomization{{Name: "csi"}},
		}
		mocks.BlueprintHandler.GenerateFunc = func() *blueprintv1alpha1.Blueprint { return bp }
		fluxStack := fluxinfra.NewMockStack()
		fluxStack.PlanFunc = func(b *blueprintv1alpha1.Blueprint, id string) error { return nil }
		comp := composer.NewComposer(mocks.Runtime)
		comp.BlueprintHandler = mocks.BlueprintHandler
		mockProvisioner := provisioner.NewProvisioner(mocks.Runtime, comp.BlueprintHandler, &provisioner.Provisioner{
			TerraformStack: mocks.TerraformStack,
			FluxStack:      fluxStack,
		})
		proj := project.NewProject("", &project.Project{Runtime: mocks.Runtime, Composer: comp, Provisioner: mockProvisioner})

		// When the plan command is executed with a kustomization name
		cmd := createTestCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetArgs([]string{"csi"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then no error is returned
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("ErrorComponentNotFound", func(t *testing.T) {
		// Given a blueprint with no component named "nonexistent"
		mocks := setupPlanTest(t)
		bp := &blueprintv1alpha1.Blueprint{Metadata: blueprintv1alpha1.Metadata{Name: "test"}}
		mocks.BlueprintHandler.GenerateFunc = func() *blueprintv1alpha1.Blueprint { return bp }
		fluxStack := fluxinfra.NewMockStack()
		comp := composer.NewComposer(mocks.Runtime)
		comp.BlueprintHandler = mocks.BlueprintHandler
		mockProvisioner := provisioner.NewProvisioner(mocks.Runtime, comp.BlueprintHandler, &provisioner.Provisioner{
			TerraformStack: mocks.TerraformStack,
			FluxStack:      fluxStack,
		})
		proj := project.NewProject("", &project.Project{Runtime: mocks.Runtime, Composer: comp, Provisioner: mockProvisioner})

		// When the plan command is executed with an unknown component
		cmd := createTestCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetArgs([]string{"nonexistent"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then an error mentioning the component name is returned
		if err == nil {
			t.Error("expected error for unknown component, got nil")
		}
		if !strings.Contains(err.Error(), "nonexistent") {
			t.Errorf("expected component name in error, got: %v", err)
		}
	})
}

func TestPrintPlanSummary(t *testing.T) {
	t.Run("PrintsTerraformAndKustomizeRows", func(t *testing.T) {
		// Given terraform and kustomize results
		tfPlans := []terraforminfra.TerraformComponentPlan{
			{ComponentID: "cluster", Add: 5, Change: 1, Destroy: 0},
			{ComponentID: "networking", NoChanges: true},
		}
		k8sPlans := []fluxinfra.KustomizePlan{
			{Name: "flux-system", Added: 12, IsNew: true},
			{Name: "monitoring", Added: 3, Removed: 1},
		}
		var buf strings.Builder

		// When printPlanSummary is called
		printPlanSummary(&buf, tfPlans, k8sPlans, nil, false)

		output := buf.String()

		// Then key content appears in output
		if !strings.Contains(output, "Terraform") {
			t.Errorf("expected Terraform section header, got: %s", output)
		}
		if !strings.Contains(output, "Kustomize") {
			t.Errorf("expected Kustomize section header, got: %s", output)
		}
		if !strings.Contains(output, "cluster") {
			t.Errorf("expected cluster row, got: %s", output)
		}
		if !strings.Contains(output, "flux-system") {
			t.Errorf("expected flux-system row, got: %s", output)
		}
		if !strings.Contains(output, "(no changes)") {
			t.Errorf("expected (no changes) for networking, got: %s", output)
		}
		if !strings.Contains(output, "(new)") {
			t.Errorf("expected (new) label for flux-system, got: %s", output)
		}
	})

	t.Run("PrintsNoBlueprintMessageForEmptySlices", func(t *testing.T) {
		// Given empty slices
		var buf strings.Builder

		// When printPlanSummary is called with no components
		printPlanSummary(&buf, nil, nil, nil, false)

		// Then a helpful message is printed
		if !strings.Contains(buf.String(), "no components") {
			t.Errorf("expected 'no components' message, got: %s", buf.String())
		}
	})

	t.Run("ShowsErrorRowForFailedComponents", func(t *testing.T) {
		// Given a terraform component with an error
		tfPlans := []terraforminfra.TerraformComponentPlan{
			{ComponentID: "broken", Err: fmt.Errorf("backend unavailable")},
		}
		var buf strings.Builder

		// When printPlanSummary is called
		printPlanSummary(&buf, tfPlans, nil, nil, false)

		// Then the error text appears in the output
		if !strings.Contains(buf.String(), "backend unavailable") {
			t.Errorf("expected error text, got: %s", buf.String())
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

	t.Run("SuccessNoArgPlansSummary", func(t *testing.T) {
		// Given a plan kustomize command with no arguments
		mocks := setupPlanTest(t)
		fluxStack := fluxinfra.NewMockStack()
		fluxStack.PlanSummaryFunc = func(bp *blueprintv1alpha1.Blueprint) ([]fluxinfra.KustomizePlan, []string) {
			return []fluxinfra.KustomizePlan{{Name: "flux-system", IsNew: true, Added: 3}}, nil
		}
		proj := newKustomizePlanProject(mocks, fluxStack)

		// When executing without a component ID
		cmd := createTestCmd("kustomize")
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then no error occurs and summary is printed
		if err != nil {
			t.Errorf("expected no error when no arg given, got %v", err)
		}
	})

	t.Run("SuccessJSONWithSpecificComponent", func(t *testing.T) {
		// Given a plan kustomize command with --json and a specific component
		mocks := setupPlanTest(t)
		fluxStack := fluxinfra.NewMockStack()
		fluxStack.PlanSummaryFunc = func(bp *blueprintv1alpha1.Blueprint) ([]fluxinfra.KustomizePlan, []string) {
			return []fluxinfra.KustomizePlan{
				{Name: "flux-system", IsNew: true, Added: 5},
				{Name: "monitoring", Added: 2, Removed: 1},
			}, nil
		}
		proj := newKustomizePlanProject(mocks, fluxStack)

		// When executing with --json and a specific component
		cmd := createTestCmd("kustomize")
		planJSON = true
		t.Cleanup(func() { planJSON = false })
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetArgs([]string{"flux-system"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then no error occurs and only the named component appears in output
		if err != nil {
			t.Errorf("expected no error, got %v", err)
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

// =============================================================================
// Test Helpers
// =============================================================================

func TestBlueprintHasTerraformComponent(t *testing.T) {
	falseVal := false

	t.Run("ReturnsFalseForNilBlueprint", func(t *testing.T) {
		if blueprintHasTerraformComponent(nil, "foo") {
			t.Error("expected false for nil blueprint")
		}
	})

	t.Run("ReturnsTrueForEnabledComponent", func(t *testing.T) {
		bp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Name: "cluster"},
			},
		}
		if !blueprintHasTerraformComponent(bp, "cluster") {
			t.Error("expected true for enabled component")
		}
	})

	t.Run("ReturnsFalseForDisabledComponent", func(t *testing.T) {
		bp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Name: "cluster", Enabled: &blueprintv1alpha1.BoolExpression{Value: &falseVal}},
			},
		}
		if blueprintHasTerraformComponent(bp, "cluster") {
			t.Error("expected false for explicitly disabled component")
		}
	})

	t.Run("ReturnsFalseForMissingComponent", func(t *testing.T) {
		bp := &blueprintv1alpha1.Blueprint{
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Name: "cluster"},
			},
		}
		if blueprintHasTerraformComponent(bp, "nonexistent") {
			t.Error("expected false for missing component")
		}
	})
}
