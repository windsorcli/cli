package cmd

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/windsorcli/cli/pkg/project"
	"github.com/windsorcli/cli/pkg/runtime/config"
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

	proj, err := project.NewProject("", &project.Project{Runtime: baseMocks.Runtime})
	if err != nil {
		t.Fatalf("Failed to create project: %v", err)
	}

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
		// Given a properly configured down command
		mocks := setupDownTest(t)

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
		mocks.Project, _ = project.NewProject("", &project.Project{Runtime: mocks.Runtime.Runtime})

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

	t.Run("SkipK8sFlag", func(t *testing.T) {
		// Given a down command with skip-k8s flag
		mocks := setupDownTest(t)

		// When executing the down command with skip-k8s flag
		cmd := createTestDownCmd()
		cmd.SetArgs([]string{"--skip-k8s"})
		ctx := context.WithValue(context.Background(), projectOverridesKey, mocks.Project)
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("SkipTerraformFlag", func(t *testing.T) {
		// Given a down command with skip-tf flag
		mocks := setupDownTest(t)

		// When executing the down command with skip-tf flag
		cmd := createTestDownCmd()
		cmd.SetArgs([]string{"--skip-tf"})
		ctx := context.WithValue(context.Background(), projectOverridesKey, mocks.Project)
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("SkipDockerFlag", func(t *testing.T) {
		// Given a down command with skip-docker flag
		mocks := setupDownTest(t)

		// When executing the down command with skip-docker flag
		cmd := createTestDownCmd()
		cmd.SetArgs([]string{"--skip-docker"})
		ctx := context.WithValue(context.Background(), projectOverridesKey, mocks.Project)
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("DevModeWithoutWorkstation", func(t *testing.T) {
		// Given a down command in non-dev mode
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.IsDevModeFunc = func(contextName string) bool { return false }

		opts := &SetupOptions{
			ConfigHandler: mockConfigHandler,
		}
		mocks := setupDownTest(t, opts)
		mocks.Runtime.Runtime.ConfigHandler = mockConfigHandler
		mocks.Project, _ = project.NewProject("", &project.Project{Runtime: mocks.Runtime.Runtime})

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
}
