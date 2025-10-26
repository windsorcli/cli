package cmd

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/pipelines"
)

// =============================================================================
// Test Setup
// =============================================================================

type InstallMocks struct {
	*Mocks
}

func setupInstallTest(t *testing.T, opts ...*SetupOptions) *InstallMocks {
	t.Helper()

	// Setup base mocks
	baseMocks := setupMocks(t, opts...)

	// Note: envPipeline no longer used - install now uses runtime.LoadEnvVars

	// Register mock install pipeline in injector
	mockInstallPipeline := pipelines.NewMockBasePipeline()
	mockInstallPipeline.InitializeFunc = func(injector di.Injector, ctx context.Context) error { return nil }
	mockInstallPipeline.ExecuteFunc = func(ctx context.Context) error { return nil }
	baseMocks.Injector.Register("installPipeline", mockInstallPipeline)

	return &InstallMocks{
		Mocks: baseMocks,
	}
}

// =============================================================================
// Test Cases
// =============================================================================

func TestInstallCmd(t *testing.T) {
	createTestInstallCmd := func() *cobra.Command {
		// Create a new command with the same RunE as installCmd
		cmd := &cobra.Command{
			Use:   "install",
			Short: "Install the blueprint's cluster-level services",
			RunE:  installCmd.RunE,
		}

		// Copy all flags from installCmd to ensure they're available
		installCmd.Flags().VisitAll(func(flag *pflag.Flag) {
			cmd.Flags().AddFlag(flag)
		})

		// Disable help text printing
		cmd.SilenceUsage = true
		cmd.SilenceErrors = true

		return cmd
	}

	t.Run("Success", func(t *testing.T) {
		// Given a temporary directory with mocked dependencies
		mocks := setupInstallTest(t)

		// When executing the install command
		cmd := createTestInstallCmd()
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		cmd.SetArgs([]string{})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
	})

	t.Run("SuccessWithWaitFlag", func(t *testing.T) {
		// Given a temporary directory with mocked dependencies
		mocks := setupInstallTest(t)

		// When executing the install command with wait flag
		cmd := createTestInstallCmd()
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		cmd.SetArgs([]string{"--wait"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
	})

	t.Run("SuccessWithVerboseContext", func(t *testing.T) {
		// Given a temporary directory with mocked dependencies
		mocks := setupInstallTest(t)

		// When executing the install command with verbose context
		cmd := createTestInstallCmd()
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		ctx = context.WithValue(ctx, "verbose", true)
		cmd.SetArgs([]string{})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
	})

	// Note: ReturnsErrorWhenEnvPipelineSetupFails test removed - env pipeline no longer used

	t.Run("ReturnsErrorWhenInstallPipelineSetupFails", func(t *testing.T) {
		// Given a temporary directory with mocked dependencies
		mocks := setupInstallTest(t)

		// Override install pipeline to return error during execution
		mockInstallPipeline := pipelines.NewMockBasePipeline()
		mockInstallPipeline.InitializeFunc = func(injector di.Injector, ctx context.Context) error { return nil }
		mockInstallPipeline.ExecuteFunc = func(ctx context.Context) error {
			return fmt.Errorf("install pipeline execution failed")
		}
		mocks.Injector.Register("installPipeline", mockInstallPipeline)

		// When executing the install command
		cmd := createTestInstallCmd()
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		cmd.SetArgs([]string{})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "Error executing install pipeline") {
			t.Errorf("Expected install pipeline execution error, got %q", err.Error())
		}
	})
}
