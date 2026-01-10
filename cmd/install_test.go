package cmd

import (
	"context"
	"fmt"
	"os"
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
	"github.com/windsorcli/cli/pkg/runtime/config"
)

func TestInstallCmd(t *testing.T) {
	createTestInstallCmd := func() *cobra.Command {
		cmd := &cobra.Command{
			Use:   "install",
			Short: "Install the blueprint's cluster-level services",
			RunE:  installCmd.RunE,
		}

		installCmd.Flags().VisitAll(func(flag *pflag.Flag) {
			cmd.Flags().AddFlag(flag)
		})

		cmd.SilenceUsage = true
		cmd.SilenceErrors = true

		return cmd
	}

	t.Run("Success", func(t *testing.T) {
		mocks := setupMocks(t)
		tmpDir := mocks.TmpDir

		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetContextFunc = func() string { return "test-context" }
		mockConfigHandler.IsLoadedFunc = func() bool { return true }
		mockConfigHandler.LoadConfigFunc = func() error { return nil }
		mockConfigHandler.GetConfigRootFunc = func() (string, error) { return tmpDir + "/contexts/test-context", nil }

		mockBlueprintHandler := blueprint.NewMockBlueprintHandler()
		mockBlueprintHandler.GenerateFunc = func() *blueprintv1alpha1.Blueprint {
			return &blueprintv1alpha1.Blueprint{}
		}

		mockKubernetesManager := kubernetes.NewMockKubernetesManager()
		mockKubernetesManager.ApplyBlueprintFunc = func(blueprint *blueprintv1alpha1.Blueprint, namespace string) error { return nil }

		// Override ConfigHandler and ProjectRoot in runtime
		mocks.Runtime.ConfigHandler = mockConfigHandler
		mocks.Runtime.ProjectRoot = tmpDir

		comp := composer.NewComposer(mocks.Runtime)
		comp.BlueprintHandler = mockBlueprintHandler
		mockProvisioner := provisioner.NewProvisioner(mocks.Runtime, comp.BlueprintHandler, &provisioner.Provisioner{
			KubernetesManager: mockKubernetesManager,
		})

		proj := project.NewProject("", &project.Project{
			Runtime:     mocks.Runtime,
			Composer:    comp,
			Provisioner: mockProvisioner,
		})

		cmd := createTestInstallCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetArgs([]string{})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
	})

	t.Run("SuccessWithWaitFlag", func(t *testing.T) {
		mocks := setupMocks(t)
		tmpDir := mocks.TmpDir

		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetContextFunc = func() string { return "test-context" }
		mockConfigHandler.IsLoadedFunc = func() bool { return true }
		mockConfigHandler.LoadConfigFunc = func() error { return nil }
		mockConfigHandler.GetConfigRootFunc = func() (string, error) { return tmpDir + "/contexts/test-context", nil }

		mockBlueprintHandler := blueprint.NewMockBlueprintHandler()
		mockBlueprintHandler.GenerateFunc = func() *blueprintv1alpha1.Blueprint {
			return &blueprintv1alpha1.Blueprint{}
		}

		mockKubernetesManager := kubernetes.NewMockKubernetesManager()
		mockKubernetesManager.ApplyBlueprintFunc = func(blueprint *blueprintv1alpha1.Blueprint, namespace string) error { return nil }
		mockKubernetesManager.WaitForKustomizationsFunc = func(message string, blueprint *blueprintv1alpha1.Blueprint) error { return nil }

		// Override ConfigHandler and ProjectRoot in runtime
		mocks.Runtime.ConfigHandler = mockConfigHandler
		mocks.Runtime.ProjectRoot = tmpDir

		comp := composer.NewComposer(mocks.Runtime)
		comp.BlueprintHandler = mockBlueprintHandler
		mockProvisioner := provisioner.NewProvisioner(mocks.Runtime, comp.BlueprintHandler, &provisioner.Provisioner{
			KubernetesManager: mockKubernetesManager,
		})

		proj := project.NewProject("", &project.Project{
			Runtime:     mocks.Runtime,
			Composer:    comp,
			Provisioner: mockProvisioner,
		})

		cmd := createTestInstallCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetArgs([]string{"--wait"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
	})

	t.Run("ErrorCheckingTrustedDirectory", func(t *testing.T) {
		mocks := setupMocks(t)
		tmpDir := mocks.TmpDir
		mocks.Shell.CheckTrustedDirectoryFunc = func() error {
			return fmt.Errorf("not in trusted directory")
		}

		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetContextFunc = func() string { return "test-context" }
		mockConfigHandler.IsLoadedFunc = func() bool { return true }
		mockConfigHandler.LoadConfigFunc = func() error { return nil }

		// Override ConfigHandler and ProjectRoot in runtime
		mocks.Runtime.ConfigHandler = mockConfigHandler
		mocks.Runtime.ProjectRoot = tmpDir

		comp := composer.NewComposer(mocks.Runtime)
		mockProvisioner := provisioner.NewProvisioner(mocks.Runtime, comp.BlueprintHandler, nil)

		proj := project.NewProject("", &project.Project{
			Runtime:     mocks.Runtime,
			Composer:    comp,
			Provisioner: mockProvisioner,
		})

		cmd := createTestInstallCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetContext(ctx)
		err := cmd.Execute()

		if err == nil {
			t.Error("Expected error, got nil")
			return
		}
		if !strings.Contains(err.Error(), "not in a trusted directory") {
			t.Errorf("Expected trusted directory error, got: %v", err)
		}
	})

	t.Run("ErrorLoadingConfig", func(t *testing.T) {
		tmpDir := t.TempDir()
		oldDir, _ := os.Getwd()
		os.Chdir(tmpDir)
		t.Cleanup(func() { os.Chdir(oldDir) })

		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.LoadConfigFunc = func() error {
			return fmt.Errorf("config load failed")
		}
		mockConfigHandler.GetContextFunc = func() string { return "test-context" }
		mockConfigHandler.IsLoadedFunc = func() bool { return false }

		mocks := setupMocks(t, &SetupOptions{
			ConfigHandler: mockConfigHandler,
		})

		// Override ConfigHandler and ProjectRoot in runtime
		mocks.Runtime.ConfigHandler = mockConfigHandler
		mocks.Runtime.ProjectRoot = tmpDir

		comp := composer.NewComposer(mocks.Runtime)
		mockProvisioner := provisioner.NewProvisioner(mocks.Runtime, comp.BlueprintHandler, nil)

		proj := project.NewProject("", &project.Project{
			Runtime:     mocks.Runtime,
			Composer:    comp,
			Provisioner: mockProvisioner,
		})

		cmd := createTestInstallCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetContext(ctx)
		err := cmd.Execute()

		if err == nil {
			t.Error("Expected error, got nil")
			return
		}
		if !strings.Contains(err.Error(), "failed to load config") {
			t.Errorf("Expected config load error, got: %v", err)
		}
	})
}
