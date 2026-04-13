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
	"github.com/windsorcli/cli/pkg/provisioner/kubernetes"
	terraforminfra "github.com/windsorcli/cli/pkg/provisioner/terraform"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// =============================================================================
// Test Setup
// =============================================================================

type ApplyMocks struct {
	ConfigHandler     config.ConfigHandler
	Shell             *shell.MockShell
	BlueprintHandler  *blueprint.MockBlueprintHandler
	TerraformStack    *terraforminfra.MockStack
	KubernetesManager *kubernetes.MockKubernetesManager
	Runtime           *runtime.Runtime
	TmpDir            string
}

func setupApplyTest(t *testing.T, opts ...*SetupOptions) *ApplyMocks {
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
	mockTerraformStack.ApplyFunc = func(blueprint *blueprintv1alpha1.Blueprint, componentID string) error { return nil }

	mockKubernetesManager := kubernetes.NewMockKubernetesManager()
	mockKubernetesManager.ApplyBlueprintFunc = func(bp *blueprintv1alpha1.Blueprint, namespace string) error { return nil }
	mockKubernetesManager.WaitForKustomizationsFunc = func(message string, blueprint *blueprintv1alpha1.Blueprint) error { return nil }

	rt := runtime.NewRuntime(&runtime.Runtime{
		Shell:         baseMocks.Shell,
		ConfigHandler: baseMocks.ConfigHandler,
		ProjectRoot:   tmpDir,
		ToolsManager:  baseMocks.ToolsManager,
	})
	if rt == nil {
		t.Fatal("Failed to create runtime")
	}

	return &ApplyMocks{
		ConfigHandler:     baseMocks.ConfigHandler,
		Shell:             baseMocks.Shell,
		BlueprintHandler:  mockBlueprintHandler,
		TerraformStack:    mockTerraformStack,
		KubernetesManager: mockKubernetesManager,
		Runtime:           rt,
		TmpDir:            tmpDir,
	}
}

// newApplyProjectWith wires mocks into a project using the given provisioner overrides.
func newApplyProjectWith(mocks *ApplyMocks, overrides *provisioner.Provisioner) *project.Project {
	comp := composer.NewComposer(mocks.Runtime)
	comp.BlueprintHandler = mocks.BlueprintHandler
	return project.NewProject("", &project.Project{
		Runtime:     mocks.Runtime,
		Composer:    comp,
		Provisioner: provisioner.NewProvisioner(mocks.Runtime, comp.BlueprintHandler, overrides),
	})
}

func newApplyProject(mocks *ApplyMocks) *project.Project {
	return newApplyProjectWith(mocks, &provisioner.Provisioner{TerraformStack: mocks.TerraformStack})
}

func newApplyKustomizeProject(mocks *ApplyMocks) *project.Project {
	return newApplyProjectWith(mocks, &provisioner.Provisioner{KubernetesManager: mocks.KubernetesManager})
}

func newApplyAllProject(mocks *ApplyMocks) *project.Project {
	return newApplyProjectWith(mocks, &provisioner.Provisioner{TerraformStack: mocks.TerraformStack, KubernetesManager: mocks.KubernetesManager})
}

// makeApplyTestCmd clones source into a fresh isolated command for use in unit tests.
func makeApplyTestCmd(source *cobra.Command) *cobra.Command {
	cmd := &cobra.Command{Use: source.Use, RunE: source.RunE, Args: source.Args}
	source.Flags().VisitAll(func(flag *pflag.Flag) { cmd.Flags().AddFlag(flag) })
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	return cmd
}

// =============================================================================
// Test Cases
// =============================================================================

func TestApplyCmd(t *testing.T) {
	createTestApplyCmd := func() *cobra.Command { return makeApplyTestCmd(applyCmd) }

	suppressProcessStdout(t)
	suppressProcessStderr(t)

	t.Run("Success", func(t *testing.T) {
		// Given a properly configured apply command
		mocks := setupApplyTest(t)
		proj := newApplyAllProject(mocks)

		// When executing the bare apply command
		cmd := createTestApplyCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("SuccessWithWait", func(t *testing.T) {
		t.Cleanup(func() { applyWaitFlag = false })
		// Given a properly configured apply command with --wait
		mocks := setupApplyTest(t)
		proj := newApplyAllProject(mocks)

		// When executing the bare apply command with --wait
		cmd := createTestApplyCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetArgs([]string{"--wait"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ErrorNilBlueprint", func(t *testing.T) {
		// Given a blueprint handler that returns nil
		mocks := setupApplyTest(t)
		mocks.BlueprintHandler.GenerateFunc = func() *blueprintv1alpha1.Blueprint { return nil }
		proj := newApplyAllProject(mocks)

		// When executing the bare apply command
		cmd := createTestApplyCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
			return
		}
		if !strings.Contains(err.Error(), "blueprint is not available") {
			t.Errorf("Expected blueprint error, got: %v", err)
		}
	})

	t.Run("ErrorTerraformFails", func(t *testing.T) {
		// Given a terraform stack whose Up fails
		mocks := setupApplyTest(t)
		mocks.TerraformStack.UpFunc = func(bp *blueprintv1alpha1.Blueprint, onApply ...func(id string) error) error {
			return fmt.Errorf("terraform up failed")
		}
		proj := newApplyAllProject(mocks)

		// When executing the bare apply command
		cmd := createTestApplyCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
			return
		}
		if !strings.Contains(err.Error(), "error applying terraform") {
			t.Errorf("Expected terraform error, got: %v", err)
		}
	})

	t.Run("ErrorKustomizeFails", func(t *testing.T) {
		// Given a kubernetes manager whose ApplyBlueprint fails
		mocks := setupApplyTest(t)
		mocks.KubernetesManager.ApplyBlueprintFunc = func(bp *blueprintv1alpha1.Blueprint, namespace string) error {
			return fmt.Errorf("kustomize apply failed")
		}
		proj := newApplyAllProject(mocks)

		// When executing the bare apply command
		cmd := createTestApplyCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
			return
		}
		if !strings.Contains(err.Error(), "error applying kustomize") {
			t.Errorf("Expected kustomize error, got: %v", err)
		}
	})

	t.Run("ErrorWaitFails", func(t *testing.T) {
		t.Cleanup(func() { applyWaitFlag = false })
		// Given a kubernetes manager whose WaitForKustomizations fails
		mocks := setupApplyTest(t)
		mocks.KubernetesManager.WaitForKustomizationsFunc = func(message string, bp *blueprintv1alpha1.Blueprint) error {
			return fmt.Errorf("wait failed")
		}
		proj := newApplyAllProject(mocks)

		// When executing the bare apply command with --wait
		cmd := createTestApplyCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetArgs([]string{"--wait"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
			return
		}
		if !strings.Contains(err.Error(), "error waiting for kustomizations") {
			t.Errorf("Expected wait error, got: %v", err)
		}
	})
}

func TestApplyTerraformCmd(t *testing.T) {
	createTestApplyTerraformCmd := func() *cobra.Command { return makeApplyTestCmd(applyTerraformCmd) }

	suppressProcessStdout(t)
	suppressProcessStderr(t)

	t.Run("Success", func(t *testing.T) {
		// Given a properly configured apply terraform command
		mocks := setupApplyTest(t)
		proj := newApplyProject(mocks)

		// When executing the apply terraform command with a component ID
		cmd := createTestApplyTerraformCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetArgs([]string{"cluster"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ErrorMissingArgument", func(t *testing.T) {
		// Given an apply terraform command with no arguments
		mocks := setupApplyTest(t)
		proj := newApplyProject(mocks)

		// When executing without a component ID
		cmd := createTestApplyTerraformCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then an error should occur
		if err == nil {
			t.Error("Expected error for missing argument, got nil")
		}
	})

	t.Run("ErrorCheckingTrustedDirectory", func(t *testing.T) {
		// Given an apply terraform command with an untrusted directory
		mocks := setupApplyTest(t)
		mocks.Shell.CheckTrustedDirectoryFunc = func() error {
			return fmt.Errorf("not in trusted directory")
		}
		proj := newApplyProject(mocks)

		// When executing the apply terraform command
		cmd := createTestApplyTerraformCmd()
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

	t.Run("ErrorApplyFails", func(t *testing.T) {
		// Given an apply terraform command where the apply operation fails
		mocks := setupApplyTest(t)
		mocks.TerraformStack.ApplyFunc = func(blueprint *blueprintv1alpha1.Blueprint, componentID string) error {
			return fmt.Errorf("component %q not found", componentID)
		}
		proj := newApplyProject(mocks)

		// When executing the apply terraform command
		cmd := createTestApplyTerraformCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetArgs([]string{"nonexistent"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error applying terraform") {
			t.Errorf("Expected apply error, got: %v", err)
		}
	})

	t.Run("ErrorLoadingConfig", func(t *testing.T) {
		// Given an apply terraform command with config load failure
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.LoadConfigFunc = func() error {
			return fmt.Errorf("config load failed")
		}

		opts := &SetupOptions{ConfigHandler: mockConfigHandler}
		mocks := setupApplyTest(t, opts)
		mocks.Runtime.ConfigHandler = mockConfigHandler
		proj := project.NewProject("", &project.Project{Runtime: mocks.Runtime})

		// When executing the apply terraform command
		cmd := createTestApplyTerraformCmd()
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

func TestApplyKustomizeCmd(t *testing.T) {
	createTestApplyKustomizeCmd := func() *cobra.Command { return makeApplyTestCmd(applyKustomizeCmd) }

	suppressProcessStdout(t)
	suppressProcessStderr(t)

	t.Run("SuccessAll", func(t *testing.T) {
		// Given a properly configured apply kustomize command with no argument
		mocks := setupApplyTest(t)
		testBlueprint := &blueprintv1alpha1.Blueprint{
			Metadata:       blueprintv1alpha1.Metadata{Name: "test"},
			Kustomizations: []blueprintv1alpha1.Kustomization{{Name: "my-app"}},
		}
		mocks.BlueprintHandler.GenerateFunc = func() *blueprintv1alpha1.Blueprint { return testBlueprint }
		proj := newApplyKustomizeProject(mocks)

		// When executing the apply kustomize command with no argument
		cmd := createTestApplyKustomizeCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("Success", func(t *testing.T) {
		// Given a properly configured apply kustomize command with a matching kustomization
		mocks := setupApplyTest(t)
		testBlueprint := &blueprintv1alpha1.Blueprint{
			Metadata:       blueprintv1alpha1.Metadata{Name: "test"},
			Kustomizations: []blueprintv1alpha1.Kustomization{{Name: "my-app"}},
		}
		mocks.BlueprintHandler.GenerateFunc = func() *blueprintv1alpha1.Blueprint { return testBlueprint }
		proj := newApplyKustomizeProject(mocks)

		// When executing the apply kustomize command with a kustomization name
		cmd := createTestApplyKustomizeCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetArgs([]string{"my-app"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ErrorCheckingTrustedDirectory", func(t *testing.T) {
		// Given an apply kustomize command with an untrusted directory
		mocks := setupApplyTest(t)
		mocks.Shell.CheckTrustedDirectoryFunc = func() error {
			return fmt.Errorf("not in trusted directory")
		}
		proj := newApplyKustomizeProject(mocks)

		// When executing the apply kustomize command
		cmd := createTestApplyKustomizeCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetArgs([]string{"my-app"})
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

	t.Run("ErrorApplyFails", func(t *testing.T) {
		// Given an apply kustomize command where the kustomization is not in the blueprint
		mocks := setupApplyTest(t)
		testBlueprint := &blueprintv1alpha1.Blueprint{
			Metadata:       blueprintv1alpha1.Metadata{Name: "test"},
			Kustomizations: []blueprintv1alpha1.Kustomization{},
		}
		mocks.BlueprintHandler.GenerateFunc = func() *blueprintv1alpha1.Blueprint { return testBlueprint }
		proj := newApplyKustomizeProject(mocks)

		// When executing the apply kustomize command with a nonexistent name
		cmd := createTestApplyKustomizeCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetArgs([]string{"nonexistent"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error applying kustomize") {
			t.Errorf("Expected apply error, got: %v", err)
		}
	})

	t.Run("SuccessWithWaitAll", func(t *testing.T) {
		t.Cleanup(func() { applyWaitFlag = false })
		// Given a blueprint with multiple kustomizations and --wait but no name argument
		mocks := setupApplyTest(t)
		testBlueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test"},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "app-a"},
				{Name: "app-b"},
			},
		}
		mocks.BlueprintHandler.GenerateFunc = func() *blueprintv1alpha1.Blueprint { return testBlueprint }
		var waitedBlueprint *blueprintv1alpha1.Blueprint
		mocks.KubernetesManager.WaitForKustomizationsFunc = func(message string, bp *blueprintv1alpha1.Blueprint) error {
			waitedBlueprint = bp
			return nil
		}
		proj := newApplyKustomizeProject(mocks)

		// When executing apply kustomize --wait with no name
		cmd := createTestApplyKustomizeCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetArgs([]string{"--wait"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then no error should occur and the full blueprint is waited on
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if len(waitedBlueprint.Kustomizations) != 2 {
			t.Errorf("Expected wait on full blueprint (2 kustomizations), got %d", len(waitedBlueprint.Kustomizations))
		}
	})

	t.Run("SuccessWithWaitSingle", func(t *testing.T) {
		t.Cleanup(func() { applyWaitFlag = false })
		// Given a blueprint with multiple kustomizations and --wait with a specific name
		mocks := setupApplyTest(t)
		testBlueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test"},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "app-a"},
				{Name: "app-b"},
			},
		}
		mocks.BlueprintHandler.GenerateFunc = func() *blueprintv1alpha1.Blueprint { return testBlueprint }
		var waitedBlueprint *blueprintv1alpha1.Blueprint
		mocks.KubernetesManager.WaitForKustomizationsFunc = func(message string, bp *blueprintv1alpha1.Blueprint) error {
			waitedBlueprint = bp
			return nil
		}
		proj := newApplyKustomizeProject(mocks)

		// When executing apply kustomize app-a --wait
		cmd := createTestApplyKustomizeCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetArgs([]string{"app-a", "--wait"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then no error should occur and only the named kustomization is waited on
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if len(waitedBlueprint.Kustomizations) != 1 {
			t.Errorf("Expected wait on 1 kustomization, got %d", len(waitedBlueprint.Kustomizations))
		}
		if waitedBlueprint.Kustomizations[0].Name != "app-a" {
			t.Errorf("Expected wait on app-a, got %s", waitedBlueprint.Kustomizations[0].Name)
		}
	})

	t.Run("ErrorWaitFails", func(t *testing.T) {
		t.Cleanup(func() { applyWaitFlag = false })
		// Given a kubernetes manager whose WaitForKustomizations fails
		mocks := setupApplyTest(t)
		mocks.KubernetesManager.WaitForKustomizationsFunc = func(message string, blueprint *blueprintv1alpha1.Blueprint) error {
			return fmt.Errorf("wait for kustomizations failed")
		}
		testBlueprint := &blueprintv1alpha1.Blueprint{
			Metadata:       blueprintv1alpha1.Metadata{Name: "test"},
			Kustomizations: []blueprintv1alpha1.Kustomization{{Name: "my-app"}},
		}
		mocks.BlueprintHandler.GenerateFunc = func() *blueprintv1alpha1.Blueprint { return testBlueprint }
		proj := newApplyKustomizeProject(mocks)

		// When executing the apply kustomize command with --wait
		cmd := createTestApplyKustomizeCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetArgs([]string{"--wait"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
			return
		}
		if !strings.Contains(err.Error(), "error waiting for kustomizations") {
			t.Errorf("Expected wait error, got: %v", err)
		}
	})

	t.Run("ErrorWaitFailsSingle", func(t *testing.T) {
		t.Cleanup(func() { applyWaitFlag = false })
		// Given a kubernetes manager whose WaitForKustomizations fails on a named kustomization
		mocks := setupApplyTest(t)
		mocks.KubernetesManager.WaitForKustomizationsFunc = func(message string, blueprint *blueprintv1alpha1.Blueprint) error {
			return fmt.Errorf("wait for kustomizations failed")
		}
		testBlueprint := &blueprintv1alpha1.Blueprint{
			Metadata:       blueprintv1alpha1.Metadata{Name: "test"},
			Kustomizations: []blueprintv1alpha1.Kustomization{{Name: "my-app"}},
		}
		mocks.BlueprintHandler.GenerateFunc = func() *blueprintv1alpha1.Blueprint { return testBlueprint }
		proj := newApplyKustomizeProject(mocks)

		// When executing the apply kustomize command with a name and --wait
		cmd := createTestApplyKustomizeCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetArgs([]string{"my-app", "--wait"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
			return
		}
		if !strings.Contains(err.Error(), "error waiting for kustomizations") {
			t.Errorf("Expected wait error, got: %v", err)
		}
	})

	t.Run("ErrorNilBlueprint", func(t *testing.T) {
		// Given a blueprint handler that returns nil
		mocks := setupApplyTest(t)
		mocks.BlueprintHandler.GenerateFunc = func() *blueprintv1alpha1.Blueprint { return nil }
		proj := newApplyKustomizeProject(mocks)

		// When executing the apply kustomize command
		cmd := createTestApplyKustomizeCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
			return
		}
		if !strings.Contains(err.Error(), "blueprint is not available") {
			t.Errorf("Expected blueprint error, got: %v", err)
		}
	})
}
