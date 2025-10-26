package cmd

import (
	"context"
	"fmt"
	"testing"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/pipelines"
)

// setupExecMocks sets up mocks for exec command tests
func setupExecMocks(t *testing.T, opts ...*SetupOptions) *Mocks {
	t.Helper()

	// Get base mocks (includes trusted directory setup)
	baseMocks := setupMocks(t, opts...)

	// Register mock exec pipeline in injector
	mockExecPipeline := pipelines.NewMockBasePipeline()
	mockExecPipeline.InitializeFunc = func(injector di.Injector, ctx context.Context) error { return nil }
	mockExecPipeline.ExecuteFunc = func(ctx context.Context) error { return nil }
	baseMocks.Injector.Register("execPipeline", mockExecPipeline)

	return baseMocks
}

func TestExecCmd(t *testing.T) {
	createTestCmd := func() *cobra.Command {
		return &cobra.Command{
			Use:          "exec -- [command]",
			Short:        "Execute a shell command with environment variables",
			Long:         "Execute a shell command with environment variables set for the application.",
			Args:         cobra.MinimumNArgs(1),
			SilenceUsage: true,
			RunE:         execCmd.RunE,
		}
	}

	t.Run("Success", func(t *testing.T) {
		// Given proper mock setup
		mocks := setupExecMocks(t)
		cmd := createTestCmd()
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		cmd.SetContext(ctx)

		args := []string{"go", "version"}
		cmd.SetArgs(args)

		// When executing the command
		err := cmd.Execute()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("NoCommandProvided", func(t *testing.T) {
		// Given proper mock setup
		mocks := setupExecMocks(t)
		cmd := createTestCmd()
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		cmd.SetContext(ctx)

		args := []string{}
		cmd.SetArgs(args)

		// When executing the command
		err := cmd.Execute()

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}

		expectedError := "requires at least 1 arg(s), only received 0"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	// Note: EnvPipelineExecutionError test removed - env pipeline no longer used

	t.Run("ExecPipelineExecutionError", func(t *testing.T) {
		// Given proper mock setup with failing exec pipeline
		mocks := setupExecMocks(t)
		mockExecPipeline := pipelines.NewMockBasePipeline()
		mockExecPipeline.InitializeFunc = func(injector di.Injector, ctx context.Context) error { return nil }
		mockExecPipeline.ExecuteFunc = func(context.Context) error {
			return fmt.Errorf("exec pipeline execution failed")
		}
		mocks.Injector.Register("execPipeline", mockExecPipeline)

		cmd := createTestCmd()
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		cmd.SetContext(ctx)

		args := []string{"go", "version"}
		cmd.SetArgs(args)

		// When executing the command
		err := cmd.Execute()

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error, got nil")
		}

		expectedError := "failed to execute command: exec pipeline execution failed"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ContextValuesPassedCorrectly", func(t *testing.T) {
		// Given proper mock setup
		mocks := setupExecMocks(t)

		// Capture context values passed to exec pipeline
		var execContext context.Context
		mockExecPipeline := pipelines.NewMockBasePipeline()
		mockExecPipeline.InitializeFunc = func(injector di.Injector, ctx context.Context) error { return nil }
		mockExecPipeline.ExecuteFunc = func(ctx context.Context) error {
			execContext = ctx
			return nil
		}
		mocks.Injector.Register("execPipeline", mockExecPipeline)

		cmd := createTestCmd()
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		cmd.SetContext(ctx)

		args := []string{"test-command", "arg1", "arg2"}
		cmd.SetArgs(args)

		// When executing the command
		err := cmd.Execute()
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then exec pipeline should receive correct context values
		if execContext.Value("command") != "test-command" {
			t.Errorf("Expected exec pipeline to receive command='test-command', got %v", execContext.Value("command"))
		}
		ctxArgs := execContext.Value("args")
		if ctxArgs == nil {
			t.Error("Expected exec pipeline to receive args")
		} else {
			argsSlice := ctxArgs.([]string)
			if len(argsSlice) != 2 || argsSlice[0] != "arg1" || argsSlice[1] != "arg2" {
				t.Errorf("Expected exec pipeline to receive args=['arg1', 'arg2'], got %v", argsSlice)
			}
		}
	})

	t.Run("PipelineCreationAndRegistration", func(t *testing.T) {
		// Given proper mock setup
		mocks := setupExecMocks(t)

		// Verify exec pipeline is registered
		execPipeline := mocks.Injector.Resolve("execPipeline")
		if execPipeline == nil {
			t.Error("Expected exec pipeline to be registered")
		}

		cmd := createTestCmd()
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		cmd.SetContext(ctx)

		args := []string{"go", "version"}
		cmd.SetArgs(args)

		// When executing the command
		err := cmd.Execute()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And exec pipeline should still be registered
		execPipeline = mocks.Injector.Resolve("execPipeline")
		if execPipeline == nil {
			t.Error("Expected exec pipeline to be registered")
		}
	})

	t.Run("SingleArgumentCommand", func(t *testing.T) {
		// Given proper mock setup
		mocks := setupExecMocks(t)

		// Capture context values passed to exec pipeline
		var execContext context.Context
		mockExecPipeline := pipelines.NewMockBasePipeline()
		mockExecPipeline.InitializeFunc = func(injector di.Injector, ctx context.Context) error { return nil }
		mockExecPipeline.ExecuteFunc = func(ctx context.Context) error {
			execContext = ctx
			return nil
		}
		mocks.Injector.Register("execPipeline", mockExecPipeline)

		cmd := createTestCmd()
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		cmd.SetContext(ctx)

		args := []string{"single-command"}
		cmd.SetArgs(args)

		// When executing the command
		err := cmd.Execute()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And command should be set correctly
		command := execContext.Value("command")
		if command != "single-command" {
			t.Errorf("Expected command to be 'single-command', got %v", command)
		}

		// And args context value should not be set for single command
		ctxArgs := execContext.Value("args")
		if ctxArgs != nil {
			t.Errorf("Expected args to be nil for single command, got %v", ctxArgs)
		}
	})

	t.Run("PipelineReuseWhenAlreadyRegistered", func(t *testing.T) {
		// Given proper mock setup
		mocks := setupExecMocks(t)

		// Pre-register exec pipeline
		originalExecPipeline := pipelines.NewMockBasePipeline()
		originalExecPipeline.InitializeFunc = func(injector di.Injector, ctx context.Context) error { return nil }
		originalExecPipeline.ExecuteFunc = func(ctx context.Context) error { return nil }
		mocks.Injector.Register("execPipeline", originalExecPipeline)

		cmd := createTestCmd()
		ctx := context.WithValue(context.Background(), injectorKey, mocks.Injector)
		cmd.SetContext(ctx)

		args := []string{"go", "version"}
		cmd.SetArgs(args)

		// When executing the command
		err := cmd.Execute()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And same exec pipeline instance should be reused
		execPipeline := mocks.Injector.Resolve("execPipeline")
		if execPipeline != originalExecPipeline {
			t.Error("Expected to reuse existing exec pipeline")
		}
	})
}
