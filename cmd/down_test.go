package cmd

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/windsorcli/cli/pkg/project"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/config"
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

		rt, err := runtime.NewRuntime(&runtime.Runtime{
			Shell:         mocks.Shell,
			ConfigHandler: mocks.ConfigHandler,
			ProjectRoot:   mocks.TmpDir,
			ToolsManager:  mocks.ToolsManager,
		})
		if err != nil {
			t.Fatalf("Failed to create runtime: %v", err)
		}

		proj, err := project.NewProject("", &project.Project{Runtime: rt})
		if err != nil {
			t.Fatalf("Failed to create project: %v", err)
		}

		cmd := createTestDownCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetContext(ctx)
		err = cmd.Execute()

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ErrorCheckingTrustedDirectory", func(t *testing.T) {
		mocks := setupMocks(t)

		mocks.Shell.CheckTrustedDirectoryFunc = func() error {
			return fmt.Errorf("not in trusted directory")
		}

		rt, err := runtime.NewRuntime(&runtime.Runtime{
			Shell:         mocks.Shell,
			ConfigHandler: mocks.ConfigHandler,
			ProjectRoot:   mocks.TmpDir,
			ToolsManager:  mocks.ToolsManager,
		})
		if err != nil {
			t.Fatalf("Failed to create runtime: %v", err)
		}

		proj, err := project.NewProject("", &project.Project{Runtime: rt})
		if err != nil {
			t.Fatalf("Failed to create project: %v", err)
		}

		cmd := createTestDownCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetContext(ctx)
		err = cmd.Execute()

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

		rt, err := runtime.NewRuntime(&runtime.Runtime{
			Shell:         mocks.Shell,
			ConfigHandler: mockConfigHandler,
			ProjectRoot:   mocks.TmpDir,
			ToolsManager:  mocks.ToolsManager,
		})
		if err != nil {
			t.Fatalf("Failed to create runtime: %v", err)
		}

		proj, err := project.NewProject("", &project.Project{Runtime: rt})
		if err != nil {
			t.Fatalf("Failed to create project: %v", err)
		}

		cmd := createTestDownCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetContext(ctx)
		err = cmd.Execute()

		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to load config") {
			t.Errorf("Expected config load error, got: %v", err)
		}
	})

	t.Run("SkipK8sFlag", func(t *testing.T) {
		mocks := setupMocks(t)

		rt, err := runtime.NewRuntime(&runtime.Runtime{
			Shell:         mocks.Shell,
			ConfigHandler: mocks.ConfigHandler,
			ProjectRoot:   mocks.TmpDir,
			ToolsManager:  mocks.ToolsManager,
		})
		if err != nil {
			t.Fatalf("Failed to create runtime: %v", err)
		}

		proj, err := project.NewProject("", &project.Project{Runtime: rt})
		if err != nil {
			t.Fatalf("Failed to create project: %v", err)
		}

		cmd := createTestDownCmd()
		cmd.SetArgs([]string{"--skip-k8s"})
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetContext(ctx)
		err = cmd.Execute()

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("SkipTerraformFlag", func(t *testing.T) {
		mocks := setupMocks(t)

		rt, err := runtime.NewRuntime(&runtime.Runtime{
			Shell:         mocks.Shell,
			ConfigHandler: mocks.ConfigHandler,
			ProjectRoot:   mocks.TmpDir,
			ToolsManager:  mocks.ToolsManager,
		})
		if err != nil {
			t.Fatalf("Failed to create runtime: %v", err)
		}

		proj, err := project.NewProject("", &project.Project{Runtime: rt})
		if err != nil {
			t.Fatalf("Failed to create project: %v", err)
		}

		cmd := createTestDownCmd()
		cmd.SetArgs([]string{"--skip-tf"})
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetContext(ctx)
		err = cmd.Execute()

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("SkipDockerFlag", func(t *testing.T) {
		mocks := setupMocks(t)

		rt, err := runtime.NewRuntime(&runtime.Runtime{
			Shell:         mocks.Shell,
			ConfigHandler: mocks.ConfigHandler,
			ProjectRoot:   mocks.TmpDir,
			ToolsManager:  mocks.ToolsManager,
		})
		if err != nil {
			t.Fatalf("Failed to create runtime: %v", err)
		}

		proj, err := project.NewProject("", &project.Project{Runtime: rt})
		if err != nil {
			t.Fatalf("Failed to create project: %v", err)
		}

		cmd := createTestDownCmd()
		cmd.SetArgs([]string{"--skip-docker"})
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetContext(ctx)
		err = cmd.Execute()

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("DevModeWithoutWorkstation", func(t *testing.T) {
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.IsDevModeFunc = func(contextName string) bool { return false }

		opts := &SetupOptions{
			ConfigHandler: mockConfigHandler,
		}
		mocks := setupMocks(t, opts)

		rt, err := runtime.NewRuntime(&runtime.Runtime{
			Shell:         mocks.Shell,
			ConfigHandler: mockConfigHandler,
			ProjectRoot:   mocks.TmpDir,
			ToolsManager:  mocks.ToolsManager,
		})
		if err != nil {
			t.Fatalf("Failed to create runtime: %v", err)
		}

		proj, err := project.NewProject("", &project.Project{Runtime: rt})
		if err != nil {
			t.Fatalf("Failed to create project: %v", err)
		}

		cmd := createTestDownCmd()
		ctx := context.WithValue(context.Background(), projectOverridesKey, proj)
		cmd.SetContext(ctx)
		err = cmd.Execute()

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}
