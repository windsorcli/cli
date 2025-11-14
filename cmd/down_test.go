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
	terraforminfra "github.com/windsorcli/cli/pkg/provisioner/terraform"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/workstation/virt"
)

func TestDownCmd(t *testing.T) {
	createTestDownCmd := func() *cobra.Command {
		cmd := &cobra.Command{
			Use:   "down",
			Short: "Tear down the Windsor environment",
			RunE:  downCmd.RunE,
		}

		downCmd.Flags().VisitAll(func(flag *pflag.Flag) {
			cmd.Flags().AddFlag(flag)
		})

		cmd.SetHelpFunc(func(*cobra.Command, []string) {})

		return cmd
	}

	t.Run("Success", func(t *testing.T) {
		mocks := setupMocks(t)

		mockBlueprintHandler := blueprint.NewMockBlueprintHandler(mocks.Injector)
		mockBlueprintHandler.DownFunc = func() error { return nil }
		mockBlueprintHandler.GenerateFunc = func() *blueprintv1alpha1.Blueprint {
			return &blueprintv1alpha1.Blueprint{}
		}
		mocks.Injector.Register("blueprintHandler", mockBlueprintHandler)

		mockStack := &terraforminfra.MockStack{}
		mockStack.InitializeFunc = func() error { return nil }
		mockStack.DownFunc = func(blueprint *blueprintv1alpha1.Blueprint) error { return nil }
		mocks.Injector.Register("stack", mockStack)

		mockContainerRuntime := &virt.MockVirt{}
		mockContainerRuntime.InitializeFunc = func() error { return nil }
		mockContainerRuntime.DownFunc = func() error { return nil }
		mocks.Injector.Register("containerRuntime", mockContainerRuntime)

		cmd := createTestDownCmd()
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		cmd.SetContext(ctx)
		err := cmd.Execute()

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ErrorCheckingTrustedDirectory", func(t *testing.T) {
		mocks := setupMocks(t)

		mockShell := mocks.Shell
		mockShell.CheckTrustedDirectoryFunc = func() error {
			return fmt.Errorf("not in trusted directory")
		}

		cmd := createTestDownCmd()
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

		cmd := createTestDownCmd()
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

	t.Run("SkipK8sFlag", func(t *testing.T) {
		mocks := setupMocks(t)

		blueprintDownCalled := false
		mockBlueprintHandler := blueprint.NewMockBlueprintHandler(mocks.Injector)
		mockBlueprintHandler.DownFunc = func() error {
			blueprintDownCalled = true
			return nil
		}
		mockBlueprintHandler.GenerateFunc = func() *blueprintv1alpha1.Blueprint {
			return &blueprintv1alpha1.Blueprint{}
		}
		mocks.Injector.Register("blueprintHandler", mockBlueprintHandler)

		mockStack := &terraforminfra.MockStack{}
		mockStack.InitializeFunc = func() error { return nil }
		mockStack.DownFunc = func(blueprint *blueprintv1alpha1.Blueprint) error { return nil }
		mocks.Injector.Register("stack", mockStack)

		mockContainerRuntime := &virt.MockVirt{}
		mockContainerRuntime.InitializeFunc = func() error { return nil }
		mockContainerRuntime.DownFunc = func() error { return nil }
		mocks.Injector.Register("containerRuntime", mockContainerRuntime)

		cmd := createTestDownCmd()
		cmd.SetArgs([]string{"--skip-k8s"})
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		cmd.SetContext(ctx)
		err := cmd.Execute()

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if blueprintDownCalled {
			t.Error("Expected blueprint Down to be skipped, but it was called")
		}
	})

	t.Run("SkipTerraformFlag", func(t *testing.T) {
		mocks := setupMocks(t)

		mockBlueprintHandler := blueprint.NewMockBlueprintHandler(mocks.Injector)
		mockBlueprintHandler.DownFunc = func() error { return nil }
		mockBlueprintHandler.GenerateFunc = func() *blueprintv1alpha1.Blueprint {
			return &blueprintv1alpha1.Blueprint{}
		}
		mocks.Injector.Register("blueprintHandler", mockBlueprintHandler)

		stackDownCalled := false
		mockStack := &terraforminfra.MockStack{}
		mockStack.InitializeFunc = func() error { return nil }
		mockStack.DownFunc = func(blueprint *blueprintv1alpha1.Blueprint) error {
			stackDownCalled = true
			return nil
		}
		mocks.Injector.Register("stack", mockStack)

		mockContainerRuntime := &virt.MockVirt{}
		mockContainerRuntime.InitializeFunc = func() error { return nil }
		mockContainerRuntime.DownFunc = func() error { return nil }
		mocks.Injector.Register("containerRuntime", mockContainerRuntime)

		cmd := createTestDownCmd()
		cmd.SetArgs([]string{"--skip-tf"})
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		cmd.SetContext(ctx)
		err := cmd.Execute()

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if stackDownCalled {
			t.Error("Expected stack Down to be skipped, but it was called")
		}
	})

	t.Run("SkipDockerFlag", func(t *testing.T) {
		mocks := setupMocks(t)

		mockBlueprintHandler := blueprint.NewMockBlueprintHandler(mocks.Injector)
		mockBlueprintHandler.DownFunc = func() error { return nil }
		mockBlueprintHandler.GenerateFunc = func() *blueprintv1alpha1.Blueprint {
			return &blueprintv1alpha1.Blueprint{}
		}
		mocks.Injector.Register("blueprintHandler", mockBlueprintHandler)

		mockStack := &terraforminfra.MockStack{}
		mockStack.InitializeFunc = func() error { return nil }
		mockStack.DownFunc = func(blueprint *blueprintv1alpha1.Blueprint) error { return nil }
		mocks.Injector.Register("stack", mockStack)

		containerDownCalled := false
		mockContainerRuntime := &virt.MockVirt{}
		mockContainerRuntime.InitializeFunc = func() error { return nil }
		mockContainerRuntime.DownFunc = func() error {
			containerDownCalled = true
			return nil
		}
		mocks.Injector.Register("containerRuntime", mockContainerRuntime)

		cmd := createTestDownCmd()
		cmd.SetArgs([]string{"--skip-docker"})
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		cmd.SetContext(ctx)
		err := cmd.Execute()

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if containerDownCalled {
			t.Error("Expected container runtime Down to be skipped, but it was called")
		}
	})

	t.Run("DevModeWithoutWorkstation", func(t *testing.T) {
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.IsDevModeFunc = func(contextName string) bool { return false }

		opts := &SetupOptions{
			ConfigHandler: mockConfigHandler,
		}
		mocks := setupMocks(t, opts)

		mockBlueprintHandler := blueprint.NewMockBlueprintHandler(mocks.Injector)
		mockBlueprintHandler.DownFunc = func() error { return nil }
		mockBlueprintHandler.GenerateFunc = func() *blueprintv1alpha1.Blueprint {
			return &blueprintv1alpha1.Blueprint{}
		}
		mocks.Injector.Register("blueprintHandler", mockBlueprintHandler)

		mockStack := &terraforminfra.MockStack{}
		mockStack.InitializeFunc = func() error { return nil }
		mockStack.DownFunc = func(blueprint *blueprintv1alpha1.Blueprint) error { return nil }
		mocks.Injector.Register("stack", mockStack)

		cmd := createTestDownCmd()
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		cmd.SetContext(ctx)
		err := cmd.Execute()

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}
