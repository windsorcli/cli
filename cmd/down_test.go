package cmd

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/pipelines"
	"github.com/windsorcli/cli/pkg/shell"
)

// =============================================================================
// Types
// =============================================================================

// DownMocks contains mock dependencies for down command tests
type DownMocks struct {
	Injector      di.Injector
	ConfigHandler config.ConfigHandler
	Shell         *shell.MockShell
	Shims         *Shims
}

// =============================================================================
// Helpers
// =============================================================================

func setupDownMocks(t *testing.T, opts ...*SetupOptions) *DownMocks {
	t.Helper()

	// Get base mocks (includes trusted directory setup)
	baseMocks := setupMocks(t, opts...)

	// Note: env pipeline is no longer used - environment setup is handled by runtime

	// Register mock init pipeline in injector (needed since down runs init pipeline second)
	mockInitPipeline := pipelines.NewMockBasePipeline()
	mockInitPipeline.InitializeFunc = func(injector di.Injector, ctx context.Context) error { return nil }
	mockInitPipeline.ExecuteFunc = func(ctx context.Context) error { return nil }
	baseMocks.Injector.Register("initPipeline", mockInitPipeline)

	// Register mock down pipeline in injector
	mockDownPipeline := pipelines.NewMockBasePipeline()
	mockDownPipeline.InitializeFunc = func(injector di.Injector, ctx context.Context) error { return nil }
	mockDownPipeline.ExecuteFunc = func(ctx context.Context) error { return nil }
	baseMocks.Injector.Register("downPipeline", mockDownPipeline)

	return &DownMocks{
		Injector:      baseMocks.Injector,
		ConfigHandler: baseMocks.ConfigHandler,
		Shell:         baseMocks.Shell,
		Shims:         baseMocks.Shims,
	}
}

// =============================================================================
// Test Cases
// =============================================================================

func TestDownCmd(t *testing.T) {
	createTestDownCmd := func() *cobra.Command {
		// Create a new command with the same RunE as downCmd
		cmd := &cobra.Command{
			Use:   "down",
			Short: "Tear down the Windsor environment",
			RunE:  downCmd.RunE,
		}

		// Copy all flags from downCmd to ensure they're available
		downCmd.Flags().VisitAll(func(flag *pflag.Flag) {
			cmd.Flags().AddFlag(flag)
		})

		// Disable help text printing
		cmd.SetHelpFunc(func(*cobra.Command, []string) {})

		return cmd
	}

	t.Run("Success", func(t *testing.T) {
		// Given a down command with mocks
		mocks := setupDownMocks(t)
		cmd := createTestDownCmd()

		// When executing the command
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	// Note: ErrorSettingUpEnvironment test removed - runtime is self-healing and creates missing dependencies

	// Note: ErrorExecutingEnvPipeline test removed - env pipeline no longer used

	t.Run("ErrorExecutingInitPipeline", func(t *testing.T) {
		// Given a down command with failing init pipeline execution
		mocks := setupDownMocks(t)
		mockInitPipeline := pipelines.NewMockBasePipeline()
		mockInitPipeline.InitializeFunc = func(injector di.Injector, ctx context.Context) error { return nil }
		mockInitPipeline.ExecuteFunc = func(ctx context.Context) error {
			return fmt.Errorf("init pipeline execution failed")
		}
		mocks.Injector.Register("initPipeline", mockInitPipeline)
		cmd := createTestDownCmd()

		// When executing the command
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to initialize environment") {
			t.Errorf("Expected error about init pipeline execution, got: %v", err)
		}
	})

	t.Run("ErrorExecutingDownPipeline", func(t *testing.T) {
		// Given a down command with failing down pipeline execution
		mocks := setupDownMocks(t)
		mockDownPipeline := pipelines.NewMockBasePipeline()
		mockDownPipeline.InitializeFunc = func(injector di.Injector, ctx context.Context) error { return nil }
		mockDownPipeline.ExecuteFunc = func(ctx context.Context) error {
			return fmt.Errorf("down pipeline execution failed")
		}
		mocks.Injector.Register("downPipeline", mockDownPipeline)
		cmd := createTestDownCmd()

		// When executing the command
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "Error executing down pipeline") {
			t.Errorf("Expected error about down pipeline execution, got: %v", err)
		}
	})

	t.Run("FlagsPassedToContext", func(t *testing.T) {
		// Given a down command with mocks
		mocks := setupDownMocks(t)
		var executedContext context.Context
		mockDownPipeline := pipelines.NewMockBasePipeline()
		mockDownPipeline.InitializeFunc = func(injector di.Injector, ctx context.Context) error { return nil }
		mockDownPipeline.ExecuteFunc = func(ctx context.Context) error {
			executedContext = ctx
			return nil
		}
		mocks.Injector.Register("downPipeline", mockDownPipeline)
		cmd := createTestDownCmd()

		// When executing the command with flags
		cmd.SetArgs([]string{"--clean", "--skip-k8s", "--skip-tf"})
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And flags should be passed to context
		if executedContext == nil {
			t.Fatal("Expected context to be passed to pipeline")
		}
		if cleanValue := executedContext.Value("clean"); cleanValue != true {
			t.Errorf("Expected clean flag to be true, got %v", cleanValue)
		}
		if skipK8sValue := executedContext.Value("skipK8s"); skipK8sValue != true {
			t.Errorf("Expected skipK8s flag to be true, got %v", skipK8sValue)
		}
		if skipTerraformValue := executedContext.Value("skipTerraform"); skipTerraformValue != true {
			t.Errorf("Expected skipTerraform flag to be true, got %v", skipTerraformValue)
		}
	})

	// Note: EnvPipelineContextFlags test removed - env pipeline no longer used

	t.Run("PipelineOrchestrationOrder", func(t *testing.T) {
		// Given a down command with mocks
		mocks := setupDownMocks(t)

		// And pipelines that track execution order
		executionOrder := []string{}

		// Note: env pipeline no longer used - environment setup handled by runtime

		mockInitPipeline := pipelines.NewMockBasePipeline()
		mockInitPipeline.InitializeFunc = func(injector di.Injector, ctx context.Context) error { return nil }
		mockInitPipeline.ExecuteFunc = func(ctx context.Context) error {
			executionOrder = append(executionOrder, "init")
			return nil
		}
		mocks.Injector.Register("initPipeline", mockInitPipeline)

		mockDownPipeline := pipelines.NewMockBasePipeline()
		mockDownPipeline.InitializeFunc = func(injector di.Injector, ctx context.Context) error { return nil }
		mockDownPipeline.ExecuteFunc = func(ctx context.Context) error {
			executionOrder = append(executionOrder, "down")
			return nil
		}
		mocks.Injector.Register("downPipeline", mockDownPipeline)

		// When executing the down command
		cmd := createTestDownCmd()
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then no error should occur and pipelines should execute in correct order
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		expectedOrder := []string{"init", "down"}
		if len(executionOrder) != len(expectedOrder) {
			t.Errorf("Expected %d pipelines to execute, got %d", len(expectedOrder), len(executionOrder))
		}

		for i, expected := range expectedOrder {
			if i >= len(executionOrder) || executionOrder[i] != expected {
				t.Errorf("Expected pipeline %d to be %s, got %v", i, expected, executionOrder)
			}
		}
	})

}
