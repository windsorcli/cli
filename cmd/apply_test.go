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

// newApplyProject wires mocks into a project for apply tests.
func newApplyProject(mocks *ApplyMocks) *project.Project {
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

// newApplyKustomizeProject wires mocks into a project for apply kustomize tests.
func newApplyKustomizeProject(mocks *ApplyMocks) *project.Project {
	comp := composer.NewComposer(mocks.Runtime)
	comp.BlueprintHandler = mocks.BlueprintHandler
	mockProvisioner := provisioner.NewProvisioner(mocks.Runtime, comp.BlueprintHandler, &provisioner.Provisioner{
		KubernetesManager: mocks.KubernetesManager,
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

func TestApplyTerraformCmd(t *testing.T) {
	createTestApplyTerraformCmd := func() *cobra.Command {
		cmd := &cobra.Command{
			Use:  "terraform",
			RunE: applyTerraformCmd.RunE,
		}
		applyTerraformCmd.Flags().VisitAll(func(flag *pflag.Flag) {
			cmd.Flags().AddFlag(flag)
		})
		cmd.Args = applyTerraformCmd.Args
		cmd.SilenceUsage = true
		cmd.SilenceErrors = true
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)
		return cmd
	}

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
	createTestApplyKustomizeCmd := func() *cobra.Command {
		cmd := &cobra.Command{
			Use:  "kustomize",
			RunE: applyKustomizeCmd.RunE,
		}
		applyKustomizeCmd.Flags().VisitAll(func(flag *pflag.Flag) {
			cmd.Flags().AddFlag(flag)
		})
		cmd.Args = applyKustomizeCmd.Args
		cmd.SilenceUsage = true
		cmd.SilenceErrors = true
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)
		return cmd
	}

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
}
