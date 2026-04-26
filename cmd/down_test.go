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
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/workstation"
	"github.com/windsorcli/cli/pkg/workstation/virt"
)

// =============================================================================
// Test Setup
// =============================================================================

type DownMocks struct {
	ConfigHandler config.ConfigHandler
	Runtime       *Mocks
	Project       *project.Project
}

func setupDownTest(t *testing.T, opts ...*SetupOptions) *DownMocks {
	t.Helper()

	baseMocks := setupMocks(t, opts...)

	mockBlueprintHandler := blueprint.NewMockBlueprintHandler()
	mockBlueprintHandler.GenerateFunc = func() *blueprintv1alpha1.Blueprint {
		return &blueprintv1alpha1.Blueprint{}
	}

	mockKubernetesManager := kubernetes.NewMockKubernetesManager()
	mockKubernetesManager.DeleteBlueprintFunc = func(blueprint *blueprintv1alpha1.Blueprint, namespace string) error {
		return nil
	}

	mockTerraformStack := terraforminfra.NewMockStack()
	mockTerraformStack.DestroyFunc = func(blueprint *blueprintv1alpha1.Blueprint, componentID string) (bool, error) {
		return false, nil
	}

	comp := composer.NewComposer(baseMocks.Runtime)
	comp.BlueprintHandler = mockBlueprintHandler
	mockProvisioner := provisioner.NewProvisioner(baseMocks.Runtime, comp.BlueprintHandler, &provisioner.Provisioner{
		TerraformStack:    mockTerraformStack,
		KubernetesManager: mockKubernetesManager,
	})

	proj := project.NewProject("", &project.Project{
		Runtime:     baseMocks.Runtime,
		Composer:    comp,
		Provisioner: mockProvisioner,
	})

	return &DownMocks{
		ConfigHandler: baseMocks.ConfigHandler,
		Runtime:       baseMocks,
		Project:       proj,
	}
}

// =============================================================================
// Test Cases
// =============================================================================

func TestDownCmd(t *testing.T) {
	createTestDownCmd := func() *cobra.Command {
		cmd := &cobra.Command{
			Use:   "down",
			Short: "Stop the local workstation environment",
			RunE:  downCmd.RunE,
		}

		downCmd.Flags().VisitAll(func(flag *pflag.Flag) {
			cmd.Flags().AddFlag(flag)
		})

		cmd.SetHelpFunc(func(*cobra.Command, []string) {})
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)

		return cmd
	}

	suppressProcessStdout(t)
	suppressProcessStderr(t)

	t.Run("NoOpWhenWorkstationDisabled", func(t *testing.T) {
		// Given a down command with no workstation configured
		mocks := setupDownTest(t)

		var stderrBuf strings.Builder
		cmd := createTestDownCmd()
		cmd.SetErr(&stderrBuf)
		ctx := context.WithValue(context.Background(), projectOverridesKey, mocks.Project)
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then no error should occur and a descriptive message is printed
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ErrorCheckingTrustedDirectory", func(t *testing.T) {
		// Given a down command with untrusted directory
		mocks := setupDownTest(t)
		mocks.Runtime.Shell.CheckTrustedDirectoryFunc = func() error {
			return fmt.Errorf("not in trusted directory")
		}

		// When executing the down command
		cmd := createTestDownCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, mocks.Project)
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}
		// And error should contain trusted directory message
		if !strings.Contains(err.Error(), "not in a trusted directory") {
			t.Errorf("Expected trusted directory error, got: %v", err)
		}
	})

	t.Run("ErrorLoadingConfig", func(t *testing.T) {
		// Given a down command with config load failure
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.LoadConfigFunc = func() error {
			return fmt.Errorf("config load failed")
		}

		opts := &SetupOptions{
			ConfigHandler: mockConfigHandler,
		}
		mocks := setupDownTest(t, opts)
		mocks.Runtime.Runtime.ConfigHandler = mockConfigHandler
		mocks.Project = project.NewProject("", &project.Project{Runtime: mocks.Runtime.Runtime})

		// When executing the down command
		cmd := createTestDownCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, mocks.Project)
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}
		// And error should contain config load message
		if !strings.Contains(err.Error(), "failed to load config") {
			t.Errorf("Expected config load error, got: %v", err)
		}
	})

	t.Run("SuccessWithWorkstation", func(t *testing.T) {
		// Given a down command with a workstation configured (no terraform layer, no VM)
		mocks := setupDownTest(t)
		ws := workstation.NewWorkstation(mocks.Runtime.Runtime)
		mocks.Project.Workstation = ws

		// When executing the down command
		cmd := createTestDownCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, mocks.Project)
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ErrorTearingDownWorkstationVM", func(t *testing.T) {
		// Given a workstation whose VM teardown fails
		mocks := setupDownTest(t)

		mockVirt := virt.NewMockVirt()
		mockVirt.DownFunc = func() error { return fmt.Errorf("vm down failed") }

		ws := workstation.NewWorkstation(mocks.Runtime.Runtime, &workstation.Workstation{
			VirtualMachine: mockVirt,
		})
		mocks.Project.Workstation = ws

		// When executing the down command
		cmd := createTestDownCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, mocks.Project)
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error tearing down workstation") {
			t.Errorf("Expected workstation error, got: %v", err)
		}
	})

	t.Run("ErrorPerformingCleanup", func(t *testing.T) {
		// Given a workstation where PerformCleanup fails
		mocks := setupDownTest(t)
		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.CleanFunc = func() error {
			return fmt.Errorf("cleanup failed")
		}
		ws := workstation.NewWorkstation(mocks.Runtime.Runtime)
		mocks.Project.Workstation = ws

		// When executing the down command
		cmd := createTestDownCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, mocks.Project)
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error performing cleanup") {
			t.Errorf("Expected cleanup error, got: %v", err)
		}
	})
}
