package cmd

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/pipelines"
)

func TestExecCmd(t *testing.T) {
	createTestCmd := func() *cobra.Command {
		return &cobra.Command{
			Use:          "exec -- [command]",
			Short:        "Execute a shell command with environment variables",
			Long:         "Execute a shell command with environment variables set for the application.",
			SilenceUsage: true,
			RunE:         execCmd.RunE,
		}
	}

	t.Run("Success", func(t *testing.T) {
		tmpDir := t.TempDir()
		originalDir, _ := os.Getwd()
		defer func() {
			os.Chdir(originalDir)
		}()
		os.Chdir(tmpDir)

		injector := di.NewInjector()
		mockPipeline := pipelines.NewMockExecPipeline()

		injector.Register("execPipeline", mockPipeline)

		cmd := createTestCmd()
		ctx := context.WithValue(context.Background(), injectorKey, injector)
		cmd.SetContext(ctx)

		args := []string{"echo", "hello"}
		cmd.SetArgs(args)

		err := cmd.Execute()

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("NoCommandProvided", func(t *testing.T) {
		tmpDir := t.TempDir()
		originalDir, _ := os.Getwd()
		defer func() {
			os.Chdir(originalDir)
		}()
		os.Chdir(tmpDir)

		injector := di.NewInjector()
		mockPipeline := pipelines.NewMockExecPipeline()

		injector.Register("execPipeline", mockPipeline)

		cmd := createTestCmd()
		ctx := context.WithValue(context.Background(), injectorKey, injector)
		cmd.SetContext(ctx)

		args := []string{}
		cmd.SetArgs(args)

		err := cmd.Execute()

		if err == nil {
			t.Error("Expected error, got nil")
		}

		expectedError := "no command provided"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("PipelineExecutionError", func(t *testing.T) {
		tmpDir := t.TempDir()
		originalDir, _ := os.Getwd()
		defer func() {
			os.Chdir(originalDir)
		}()
		os.Chdir(tmpDir)

		injector := di.NewInjector()
		mockPipeline := pipelines.NewMockExecPipeline()
		mockPipeline.ExecuteFunc = func(context.Context) error {
			return fmt.Errorf("pipeline execution failed")
		}

		injector.Register("execPipeline", mockPipeline)

		cmd := createTestCmd()
		ctx := context.WithValue(context.Background(), injectorKey, injector)
		cmd.SetContext(ctx)

		args := []string{"echo", "hello"}
		cmd.SetArgs(args)

		err := cmd.Execute()

		if err == nil {
			t.Error("Expected error, got nil")
		}

		expectedError := "pipeline execution failed"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("WithArguments", func(t *testing.T) {
		tmpDir := t.TempDir()
		originalDir, _ := os.Getwd()
		defer func() {
			os.Chdir(originalDir)
		}()
		os.Chdir(tmpDir)

		injector := di.NewInjector()
		mockPipeline := pipelines.NewMockExecPipeline()
		var capturedContext context.Context
		mockPipeline.ExecuteFunc = func(ctx context.Context) error {
			capturedContext = ctx
			return nil
		}

		injector.Register("execPipeline", mockPipeline)

		cmd := createTestCmd()
		ctx := context.WithValue(context.Background(), injectorKey, injector)
		cmd.SetContext(ctx)

		args := []string{"echo", "hello", "world"}
		cmd.SetArgs(args)

		err := cmd.Execute()

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		command := capturedContext.Value("command").(string)
		if command != "echo" {
			t.Errorf("Expected command 'echo', got %q", command)
		}

		cmdArgs := capturedContext.Value("args").([]string)
		if len(cmdArgs) != 2 || cmdArgs[0] != "hello" || cmdArgs[1] != "world" {
			t.Errorf("Expected args ['hello', 'world'], got %v", cmdArgs)
		}
	})
}
