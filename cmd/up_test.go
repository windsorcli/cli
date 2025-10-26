package cmd

import (
	"context"
	"fmt"
	"os"
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
// Test Setup
// =============================================================================

type UpMocks struct {
	Injector      di.Injector
	ConfigHandler config.ConfigHandler
	Shell         *shell.MockShell
	Shims         *Shims
}

func setupUpTest(t *testing.T, opts ...*SetupOptions) *UpMocks {
	t.Helper()

	// Set up temporary directory and change to it
	tmpDir := t.TempDir()
	oldDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	t.Cleanup(func() { os.Chdir(oldDir) })

	// Get base mocks
	baseMocks := setupMocks(t, opts...)

	// Note: envPipeline no longer used - up now uses runtime.LoadEnvVars

	// Register mock init pipeline in injector (needed since up runs init pipeline second)
	mockInitPipeline := pipelines.NewMockBasePipeline()
	mockInitPipeline.InitializeFunc = func(injector di.Injector, ctx context.Context) error { return nil }
	mockInitPipeline.ExecuteFunc = func(ctx context.Context) error { return nil }
	baseMocks.Injector.Register("initPipeline", mockInitPipeline)

	// Register mock up pipeline in injector
	mockUpPipeline := pipelines.NewMockBasePipeline()
	mockUpPipeline.InitializeFunc = func(injector di.Injector, ctx context.Context) error { return nil }
	mockUpPipeline.ExecuteFunc = func(ctx context.Context) error { return nil }
	baseMocks.Injector.Register("upPipeline", mockUpPipeline)

	// Register mock install pipeline in injector (needed since up conditionally runs install pipeline)
	mockInstallPipeline := pipelines.NewMockBasePipeline()
	mockInstallPipeline.InitializeFunc = func(injector di.Injector, ctx context.Context) error { return nil }
	mockInstallPipeline.ExecuteFunc = func(ctx context.Context) error { return nil }
	baseMocks.Injector.Register("installPipeline", mockInstallPipeline)

	return &UpMocks{
		Injector:      baseMocks.Injector,
		ConfigHandler: baseMocks.ConfigHandler,
		Shell:         baseMocks.Shell,
		Shims:         baseMocks.Shims,
	}
}

// =============================================================================
// Test Cases
// =============================================================================

func TestUpCmd(t *testing.T) {
	createTestUpCmd := func() *cobra.Command {
		// Create a new command with the same RunE as upCmd
		cmd := &cobra.Command{
			Use:   "up",
			Short: "Set up the Windsor environment",
			RunE:  upCmd.RunE,
		}

		// Copy all flags from upCmd to ensure they're available
		upCmd.Flags().VisitAll(func(flag *pflag.Flag) {
			cmd.Flags().AddFlag(flag)
		})

		// Disable help text printing
		cmd.SilenceUsage = true
		cmd.SilenceErrors = true

		return cmd
	}

	t.Run("Success", func(t *testing.T) {
		// Given a temporary directory with mocked dependencies
		mocks := setupUpTest(t)

		// When executing the up command
		cmd := createTestUpCmd()
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		cmd.SetArgs([]string{})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
	})

	t.Run("SuccessWithInstallFlag", func(t *testing.T) {
		// Given a temporary directory with mocked dependencies
		mocks := setupUpTest(t)

		// When executing the up command with install flag
		cmd := createTestUpCmd()
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		cmd.SetArgs([]string{"--install"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
	})

	t.Run("SuccessWithWaitFlag", func(t *testing.T) {
		// Given a temporary directory with mocked dependencies
		mocks := setupUpTest(t)

		// When executing the up command with wait flag
		cmd := createTestUpCmd()
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		cmd.SetArgs([]string{"--wait"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
	})

	t.Run("SuccessWithInstallAndWaitFlags", func(t *testing.T) {
		// Given a temporary directory with mocked dependencies
		mocks := setupUpTest(t)

		// When executing the up command with both install and wait flags
		cmd := createTestUpCmd()
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		cmd.SetArgs([]string{"--install", "--wait"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
	})

	t.Run("SuccessWithVerboseContext", func(t *testing.T) {
		// Given a temporary directory with mocked dependencies
		mocks := setupUpTest(t)

		// When executing the up command with verbose context
		cmd := createTestUpCmd()
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

	// Note: EnvPipelineExecutionError test removed - env pipeline no longer used

	t.Run("InitPipelineExecutionError", func(t *testing.T) {
		// Given a temporary directory with mocked dependencies
		mocks := setupUpTest(t)

		// And an init pipeline that fails to execute
		mockInitPipeline := pipelines.NewMockBasePipeline()
		mockInitPipeline.InitializeFunc = func(injector di.Injector, ctx context.Context) error { return nil }
		mockInitPipeline.ExecuteFunc = func(ctx context.Context) error {
			return fmt.Errorf("init pipeline execution failed")
		}
		mocks.Injector.Register("initPipeline", mockInitPipeline)

		// When executing the up command
		cmd := createTestUpCmd()
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		cmd.SetArgs([]string{})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to initialize environment") {
			t.Errorf("Expected init pipeline execution error, got: %v", err)
		}
	})

	t.Run("UpPipelineExecutionError", func(t *testing.T) {
		// Given a temporary directory with mocked dependencies
		mocks := setupUpTest(t)

		// And an up pipeline that fails to execute
		mockUpPipeline := pipelines.NewMockBasePipeline()
		mockUpPipeline.InitializeFunc = func(injector di.Injector, ctx context.Context) error { return nil }
		mockUpPipeline.ExecuteFunc = func(ctx context.Context) error {
			return fmt.Errorf("up pipeline execution failed")
		}
		mocks.Injector.Register("upPipeline", mockUpPipeline)

		// When executing the up command
		cmd := createTestUpCmd()
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		cmd.SetArgs([]string{})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "Error executing up pipeline") {
			t.Errorf("Expected up pipeline execution error, got: %v", err)
		}
	})

	t.Run("ContextPropagation", func(t *testing.T) {
		// Given a temporary directory with mocked dependencies
		mocks := setupUpTest(t)

		// And an install pipeline that validates context values
		installPipelineCalled := false
		waitContextPassed := false
		mockInstallPipeline := pipelines.NewMockBasePipeline()
		mockInstallPipeline.InitializeFunc = func(injector di.Injector, ctx context.Context) error { return nil }
		mockInstallPipeline.ExecuteFunc = func(ctx context.Context) error {
			installPipelineCalled = true
			if ctx.Value("wait") == true {
				waitContextPassed = true
			}
			return nil
		}
		mocks.Injector.Register("installPipeline", mockInstallPipeline)

		// When executing the up command with install and wait flags
		cmd := createTestUpCmd()
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		cmd.SetArgs([]string{"--install", "--wait"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then no error should occur and install pipeline should be called with wait context
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
		if !installPipelineCalled {
			t.Error("Expected install pipeline to be called when --install flag is set")
		}
		if !waitContextPassed {
			t.Error("Expected wait context to be passed to install pipeline")
		}
	})

	t.Run("InstallPipelineExecutionError", func(t *testing.T) {
		// Given a temporary directory with mocked dependencies
		mocks := setupUpTest(t)

		// And an install pipeline that fails to execute
		mockInstallPipeline := pipelines.NewMockBasePipeline()
		mockInstallPipeline.InitializeFunc = func(injector di.Injector, ctx context.Context) error { return nil }
		mockInstallPipeline.ExecuteFunc = func(ctx context.Context) error {
			return fmt.Errorf("install pipeline execution failed")
		}
		mocks.Injector.Register("installPipeline", mockInstallPipeline)

		// When executing the up command with install flag
		cmd := createTestUpCmd()
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		cmd.SetArgs([]string{"--install"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "Error executing install pipeline") {
			t.Errorf("Expected install pipeline execution error, got: %v", err)
		}
	})

	t.Run("VerboseContextPropagation", func(t *testing.T) {
		// Given a temporary directory with mocked dependencies
		mocks := setupUpTest(t)

		// And an up pipeline that validates verbose context
		verboseValidated := false
		mockUpPipeline := pipelines.NewMockBasePipeline()
		mockUpPipeline.InitializeFunc = func(injector di.Injector, ctx context.Context) error { return nil }
		mockUpPipeline.ExecuteFunc = func(ctx context.Context) error {
			// Verify that verbose flag is properly propagated
			if ctx.Value("verbose") == true {
				verboseValidated = true
			}
			return nil
		}
		mocks.Injector.Register("upPipeline", mockUpPipeline)

		// When executing the up command with verbose flag set in context
		cmd := createTestUpCmd()
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		ctx = context.WithValue(ctx, "verbose", true)
		cmd.SetArgs([]string{})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then no error should occur and verbose context should be validated
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
		if !verboseValidated {
			t.Error("Expected verbose context value to be properly propagated to up pipeline")
		}
	})

	// Note: EnvPipelineContextValues test removed - env pipeline no longer used

	t.Run("MultipleFlagsCombination", func(t *testing.T) {
		// Given a temporary directory with mocked dependencies
		mocks := setupUpTest(t)

		// When executing the up command with multiple flags
		cmd := createTestUpCmd()
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		ctx = context.WithValue(ctx, "verbose", true)
		cmd.SetArgs([]string{"--install", "--wait"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
	})

	t.Run("PipelineOrchestrationOrder", func(t *testing.T) {
		// Given a temporary directory with mocked dependencies
		mocks := setupUpTest(t)

		// And pipelines that track execution order
		executionOrder := []string{}

		// Note: env pipeline no longer used - environment setup is handled by runtime

		mockInitPipeline := pipelines.NewMockBasePipeline()
		mockInitPipeline.InitializeFunc = func(injector di.Injector, ctx context.Context) error { return nil }
		mockInitPipeline.ExecuteFunc = func(ctx context.Context) error {
			executionOrder = append(executionOrder, "init")
			return nil
		}
		mocks.Injector.Register("initPipeline", mockInitPipeline)

		mockUpPipeline := pipelines.NewMockBasePipeline()
		mockUpPipeline.InitializeFunc = func(injector di.Injector, ctx context.Context) error { return nil }
		mockUpPipeline.ExecuteFunc = func(ctx context.Context) error {
			executionOrder = append(executionOrder, "up")
			return nil
		}
		mocks.Injector.Register("upPipeline", mockUpPipeline)

		mockInstallPipeline := pipelines.NewMockBasePipeline()
		mockInstallPipeline.InitializeFunc = func(injector di.Injector, ctx context.Context) error { return nil }
		mockInstallPipeline.ExecuteFunc = func(ctx context.Context) error {
			executionOrder = append(executionOrder, "install")
			return nil
		}
		mocks.Injector.Register("installPipeline", mockInstallPipeline)

		// When executing the up command with install flag
		cmd := createTestUpCmd()
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		cmd.SetArgs([]string{"--install"})
		cmd.SetContext(ctx)
		err := cmd.Execute()

		// Then no error should occur and pipelines should execute in correct order
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
		expectedOrder := []string{"init", "up", "install"}
		if len(executionOrder) != len(expectedOrder) {
			t.Errorf("Expected %d pipeline executions, got %d", len(expectedOrder), len(executionOrder))
		}
		for i, expected := range expectedOrder {
			if i >= len(executionOrder) || executionOrder[i] != expected {
				t.Errorf("Expected pipeline execution order %v, got %v", expectedOrder, executionOrder)
				break
			}
		}
	})

}
