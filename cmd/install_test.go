package cmd

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/composer/blueprint"
	"github.com/windsorcli/cli/pkg/context/config"
	terraforminfra "github.com/windsorcli/cli/pkg/provisioner/terraform"
	"github.com/windsorcli/cli/pkg/workstation/virt"
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

		mockBlueprintHandler := blueprint.NewMockBlueprintHandler(mocks.Injector)
		mockBlueprintHandler.GenerateFunc = func() *blueprintv1alpha1.Blueprint {
			return &blueprintv1alpha1.Blueprint{}
		}
		mocks.Injector.Register("blueprintHandler", mockBlueprintHandler)

		mockStack := &terraforminfra.MockStack{}
		mockStack.InitializeFunc = func() error { return nil }
		mocks.Injector.Register("stack", mockStack)

		mockContainerRuntime := &virt.MockVirt{}
		mockContainerRuntime.InitializeFunc = func() error { return nil }
		mocks.Injector.Register("containerRuntime", mockContainerRuntime)

		cmd := createTestInstallCmd()
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		cmd.SetArgs([]string{})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
	})

	t.Run("SuccessWithWaitFlag", func(t *testing.T) {
		mocks := setupMocks(t)

		mockBlueprintHandler := blueprint.NewMockBlueprintHandler(mocks.Injector)
		mockBlueprintHandler.GenerateFunc = func() *blueprintv1alpha1.Blueprint {
			return &blueprintv1alpha1.Blueprint{}
		}
		mocks.Injector.Register("blueprintHandler", mockBlueprintHandler)

		mockStack := &terraforminfra.MockStack{}
		mockStack.InitializeFunc = func() error { return nil }
		mocks.Injector.Register("stack", mockStack)

		mockContainerRuntime := &virt.MockVirt{}
		mockContainerRuntime.InitializeFunc = func() error { return nil }
		mocks.Injector.Register("containerRuntime", mockContainerRuntime)

		cmd := createTestInstallCmd()
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		cmd.SetArgs([]string{"--wait"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
	})

	t.Run("ErrorCheckingTrustedDirectory", func(t *testing.T) {
		mocks := setupMocks(t)

		mockShell := mocks.Shell
		mockShell.CheckTrustedDirectoryFunc = func() error {
			return fmt.Errorf("not in trusted directory")
		}

		cmd := createTestInstallCmd()
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		cmd.SetContext(ctx)
		err := cmd.Execute()

		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "not in a trusted directory") {
			t.Errorf("Expected trusted directory error, got: %v", err)
		}
	})

	t.Run("ErrorLoadingConfig", func(t *testing.T) {
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.LoadConfigFunc = func() error {
			return fmt.Errorf("config load failed")
		}

		opts := &SetupOptions{
			ConfigHandler: mockConfigHandler,
		}
		mocks := setupMocks(t, opts)

		cmd := createTestInstallCmd()
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		cmd.SetContext(ctx)
		err := cmd.Execute()

		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to load config") {
			t.Errorf("Expected config load error, got: %v", err)
		}
	})
}
