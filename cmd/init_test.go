package cmd

import (
	"context"
	"fmt"
	"testing"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/pipelines"
)

func TestInitCmd(t *testing.T) {
	createTestCmd := func() *cobra.Command {
		return &cobra.Command{
			Use:          "init [context]",
			Short:        "Initialize the application",
			Long:         "Initialize the application by setting up necessary configurations and environment",
			SilenceUsage: true,
			RunE:         initCmd.RunE,
		}
	}

	t.Run("Success", func(t *testing.T) {
		injector := di.NewInjector()
		mockEnvPipeline := pipelines.NewMockBasePipeline()
		mockInitPipeline := pipelines.NewMockBasePipeline()

		injector.Register("envPipeline", mockEnvPipeline)
		injector.Register("initPipeline", mockInitPipeline)

		cmd := createTestCmd()
		ctx := context.WithValue(context.Background(), injectorKey, injector)
		cmd.SetContext(ctx)

		args := []string{}
		cmd.SetArgs(args)

		err := cmd.Execute()

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("EnvPipelineExecutionError", func(t *testing.T) {
		injector := di.NewInjector()
		mockEnvPipeline := pipelines.NewMockBasePipeline()
		mockEnvPipeline.ExecuteFunc = func(context.Context) error {
			return fmt.Errorf("env pipeline execution failed")
		}
		mockInitPipeline := pipelines.NewMockBasePipeline()

		injector.Register("envPipeline", mockEnvPipeline)
		injector.Register("initPipeline", mockInitPipeline)

		cmd := createTestCmd()
		ctx := context.WithValue(context.Background(), injectorKey, injector)
		cmd.SetContext(ctx)

		args := []string{}
		cmd.SetArgs(args)

		err := cmd.Execute()

		if err == nil {
			t.Error("Expected error, got nil")
		}

		expectedError := "failed to set up environment: env pipeline execution failed"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("InitPipelineExecutionError", func(t *testing.T) {
		injector := di.NewInjector()
		mockEnvPipeline := pipelines.NewMockBasePipeline()
		mockInitPipeline := pipelines.NewMockBasePipeline()
		mockInitPipeline.ExecuteFunc = func(context.Context) error {
			return fmt.Errorf("init pipeline execution failed")
		}

		injector.Register("envPipeline", mockEnvPipeline)
		injector.Register("initPipeline", mockInitPipeline)

		cmd := createTestCmd()
		ctx := context.WithValue(context.Background(), injectorKey, injector)
		cmd.SetContext(ctx)

		args := []string{}
		cmd.SetArgs(args)

		err := cmd.Execute()

		if err == nil {
			t.Error("Expected error, got nil")
		}

		expectedError := "Error executing init pipeline: init pipeline execution failed"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ContextValuesPassedCorrectly", func(t *testing.T) {
		injector := di.NewInjector()
		var envContext, initContext context.Context

		mockEnvPipeline := pipelines.NewMockBasePipeline()
		mockEnvPipeline.ExecuteFunc = func(ctx context.Context) error {
			envContext = ctx
			return nil
		}

		mockInitPipeline := pipelines.NewMockBasePipeline()
		mockInitPipeline.ExecuteFunc = func(ctx context.Context) error {
			initContext = ctx
			return nil
		}

		injector.Register("envPipeline", mockEnvPipeline)
		injector.Register("initPipeline", mockInitPipeline)

		cmd := createTestCmd()
		ctx := context.WithValue(context.Background(), injectorKey, injector)
		cmd.SetContext(ctx)

		args := []string{"test-context"}
		cmd.SetArgs(args)

		err := cmd.Execute()
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Check that context values are passed
		if envContext == nil || initContext == nil {
			t.Error("Expected both env and init pipeline contexts to be set")
		}
	})
}
