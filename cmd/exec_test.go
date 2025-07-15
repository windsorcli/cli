package cmd

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/pipelines"
	"github.com/windsorcli/cli/pkg/shell"
)

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
		tmpDir := t.TempDir()
		originalDir, _ := os.Getwd()
		defer func() {
			os.Chdir(originalDir)
		}()
		os.Chdir(tmpDir)

		injector := di.NewInjector()

		// Register mock shell
		mockShell := shell.NewMockShell()
		mockShell.CheckTrustedDirectoryFunc = func() error { return nil }
		injector.Register("shell", mockShell)

		// Register mock base pipeline
		mockBasePipeline := pipelines.NewMockBasePipeline()
		injector.Register("basePipeline", mockBasePipeline)

		mockEnvPipeline := pipelines.NewMockBasePipeline()
		mockExecPipeline := pipelines.NewMockBasePipeline()

		injector.Register("envPipeline", mockEnvPipeline)
		injector.Register("execPipeline", mockExecPipeline)

		cmd := createTestCmd()
		ctx := context.WithValue(context.Background(), injectorKey, injector)
		cmd.SetContext(ctx)

		args := []string{"go", "version"}
		cmd.SetArgs(args)

		err := cmd.Execute()

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("UntrustedDirectory", func(t *testing.T) {
		tmpDir := t.TempDir()
		originalDir, _ := os.Getwd()
		defer func() {
			os.Chdir(originalDir)
		}()
		os.Chdir(tmpDir)

		injector := di.NewInjector()

		// Register mock shell that fails trust check
		mockShell := shell.NewMockShell()
		mockShell.CheckTrustedDirectoryFunc = func() error {
			return fmt.Errorf("directory not trusted")
		}
		injector.Register("shell", mockShell)

		// Register mock base pipeline
		mockBasePipeline := pipelines.NewMockBasePipeline()
		injector.Register("basePipeline", mockBasePipeline)

		cmd := createTestCmd()
		ctx := context.WithValue(context.Background(), injectorKey, injector)
		cmd.SetContext(ctx)

		args := []string{"go", "version"}
		cmd.SetArgs(args)

		err := cmd.Execute()

		if err == nil {
			t.Error("Expected error for untrusted directory, got nil")
		}
		expectedMsg := "not in a trusted directory. If you are in a Windsor project, run 'windsor init' to approve"
		if fmt.Sprintf("%v", err) != expectedMsg {
			t.Errorf("Expected error message '%s', got '%v'", expectedMsg, err)
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

		// Register mock shell
		mockShell := shell.NewMockShell()
		mockShell.CheckTrustedDirectoryFunc = func() error { return nil }
		injector.Register("shell", mockShell)

		// Register mock base pipeline
		mockBasePipeline := pipelines.NewMockBasePipeline()
		injector.Register("basePipeline", mockBasePipeline)

		cmd := createTestCmd()
		ctx := context.WithValue(context.Background(), injectorKey, injector)
		cmd.SetContext(ctx)

		args := []string{}
		cmd.SetArgs(args)

		err := cmd.Execute()

		if err == nil {
			t.Error("Expected error, got nil")
		}

		expectedError := "requires at least 1 arg(s), only received 0"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("EnvPipelineExecutionError", func(t *testing.T) {
		tmpDir := t.TempDir()
		originalDir, _ := os.Getwd()
		defer func() {
			os.Chdir(originalDir)
		}()
		os.Chdir(tmpDir)

		injector := di.NewInjector()

		// Register mock shell
		mockShell := shell.NewMockShell()
		mockShell.CheckTrustedDirectoryFunc = func() error { return nil }
		injector.Register("shell", mockShell)

		// Register mock base pipeline
		mockBasePipeline := pipelines.NewMockBasePipeline()
		injector.Register("basePipeline", mockBasePipeline)

		mockEnvPipeline := pipelines.NewMockBasePipeline()
		mockEnvPipeline.ExecuteFunc = func(context.Context) error {
			return fmt.Errorf("env pipeline execution failed")
		}
		mockExecPipeline := pipelines.NewMockBasePipeline()

		injector.Register("envPipeline", mockEnvPipeline)
		injector.Register("execPipeline", mockExecPipeline)

		cmd := createTestCmd()
		ctx := context.WithValue(context.Background(), injectorKey, injector)
		cmd.SetContext(ctx)

		args := []string{"go", "version"}
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

	t.Run("ExecPipelineExecutionError", func(t *testing.T) {
		tmpDir := t.TempDir()
		originalDir, _ := os.Getwd()
		defer func() {
			os.Chdir(originalDir)
		}()
		os.Chdir(tmpDir)

		injector := di.NewInjector()

		// Register mock shell
		mockShell := shell.NewMockShell()
		mockShell.CheckTrustedDirectoryFunc = func() error { return nil }
		injector.Register("shell", mockShell)

		// Register mock base pipeline
		mockBasePipeline := pipelines.NewMockBasePipeline()
		injector.Register("basePipeline", mockBasePipeline)

		mockEnvPipeline := pipelines.NewMockBasePipeline()
		mockExecPipeline := pipelines.NewMockBasePipeline()
		mockExecPipeline.ExecuteFunc = func(context.Context) error {
			return fmt.Errorf("exec pipeline execution failed")
		}

		injector.Register("envPipeline", mockEnvPipeline)
		injector.Register("execPipeline", mockExecPipeline)

		cmd := createTestCmd()
		ctx := context.WithValue(context.Background(), injectorKey, injector)
		cmd.SetContext(ctx)

		args := []string{"go", "version"}
		cmd.SetArgs(args)

		err := cmd.Execute()

		if err == nil {
			t.Error("Expected error, got nil")
		}

		expectedError := "failed to execute command: exec pipeline execution failed"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ContextValuesPassedCorrectly", func(t *testing.T) {
		tmpDir := t.TempDir()
		originalDir, _ := os.Getwd()
		defer func() {
			os.Chdir(originalDir)
		}()
		os.Chdir(tmpDir)

		injector := di.NewInjector()

		// Register mock shell
		mockShell := shell.NewMockShell()
		mockShell.CheckTrustedDirectoryFunc = func() error { return nil }
		injector.Register("shell", mockShell)

		// Register mock base pipeline
		mockBasePipeline := pipelines.NewMockBasePipeline()
		injector.Register("basePipeline", mockBasePipeline)

		// Capture context values passed to pipelines
		var envContext, execContext context.Context

		mockEnvPipeline := pipelines.NewMockBasePipeline()
		mockEnvPipeline.ExecuteFunc = func(ctx context.Context) error {
			envContext = ctx
			return nil
		}

		mockExecPipeline := pipelines.NewMockBasePipeline()
		mockExecPipeline.ExecuteFunc = func(ctx context.Context) error {
			execContext = ctx
			return nil
		}

		injector.Register("envPipeline", mockEnvPipeline)
		injector.Register("execPipeline", mockExecPipeline)

		cmd := createTestCmd()
		ctx := context.WithValue(context.Background(), injectorKey, injector)
		cmd.SetContext(ctx)

		args := []string{"test-command", "arg1", "arg2"}
		cmd.SetArgs(args)

		err := cmd.Execute()
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Verify env pipeline context
		if envContext.Value("quiet") != true {
			t.Error("Expected env pipeline to receive quiet=true")
		}
		if envContext.Value("decrypt") != true {
			t.Error("Expected env pipeline to receive decrypt=true")
		}

		// Verify exec pipeline context
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
		tmpDir := t.TempDir()
		originalDir, _ := os.Getwd()
		defer func() {
			os.Chdir(originalDir)
		}()
		os.Chdir(tmpDir)

		// Create injector with only shell and base pipeline initially
		injector := di.NewInjector()

		// Register mock shell and base pipeline (required for exec command)
		mockShell := shell.NewMockShell()
		mockShell.CheckTrustedDirectoryFunc = func() error { return nil }
		injector.Register("shell", mockShell)

		mockBasePipeline := pipelines.NewMockBasePipeline()
		injector.Register("basePipeline", mockBasePipeline)

		// Verify pipelines don't exist initially
		if injector.Resolve("envPipeline") != nil {
			t.Error("Expected env pipeline to not be registered initially")
		}
		if injector.Resolve("execPipeline") != nil {
			t.Error("Expected exec pipeline to not be registered initially")
		}

		// Pre-register the pipelines as mocks to simulate successful creation
		mockEnvPipeline := pipelines.NewMockBasePipeline()
		mockExecPipeline := pipelines.NewMockBasePipeline()
		injector.Register("envPipeline", mockEnvPipeline)
		injector.Register("execPipeline", mockExecPipeline)

		cmd := createTestCmd()
		ctx := context.WithValue(context.Background(), injectorKey, injector)
		cmd.SetContext(ctx)

		args := []string{"go", "version"}
		cmd.SetArgs(args)

		err := cmd.Execute()

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// Verify both pipelines are still registered (reused from injector)
		envPipeline := injector.Resolve("envPipeline")
		if envPipeline == nil {
			t.Error("Expected env pipeline to be registered")
		}

		execPipeline := injector.Resolve("execPipeline")
		if execPipeline == nil {
			t.Error("Expected exec pipeline to be registered")
		}
	})

	t.Run("SingleArgumentCommand", func(t *testing.T) {
		tmpDir := t.TempDir()
		originalDir, _ := os.Getwd()
		defer func() {
			os.Chdir(originalDir)
		}()
		os.Chdir(tmpDir)

		injector := di.NewInjector()

		// Register mock shell
		mockShell := shell.NewMockShell()
		mockShell.CheckTrustedDirectoryFunc = func() error { return nil }
		injector.Register("shell", mockShell)

		// Register mock base pipeline
		mockBasePipeline := pipelines.NewMockBasePipeline()
		injector.Register("basePipeline", mockBasePipeline)

		var execContext context.Context
		mockEnvPipeline := pipelines.NewMockBasePipeline()
		mockExecPipeline := pipelines.NewMockBasePipeline()
		mockExecPipeline.ExecuteFunc = func(ctx context.Context) error {
			execContext = ctx
			return nil
		}

		injector.Register("envPipeline", mockEnvPipeline)
		injector.Register("execPipeline", mockExecPipeline)

		cmd := createTestCmd()
		ctx := context.WithValue(context.Background(), injectorKey, injector)
		cmd.SetContext(ctx)

		args := []string{"single-command"}
		cmd.SetArgs(args)

		err := cmd.Execute()

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// Verify command is set correctly
		command := execContext.Value("command")
		if command != "single-command" {
			t.Errorf("Expected command to be 'single-command', got %v", command)
		}

		// Verify args context value is not set for single command
		ctxArgs := execContext.Value("args")
		if ctxArgs != nil {
			t.Errorf("Expected args to be nil for single command, got %v", ctxArgs)
		}
	})

	t.Run("PipelineReuseWhenAlreadyRegistered", func(t *testing.T) {
		tmpDir := t.TempDir()
		originalDir, _ := os.Getwd()
		defer func() {
			os.Chdir(originalDir)
		}()
		os.Chdir(tmpDir)

		injector := di.NewInjector()

		// Register mock shell
		mockShell := shell.NewMockShell()
		mockShell.CheckTrustedDirectoryFunc = func() error { return nil }
		injector.Register("shell", mockShell)

		// Register mock base pipeline
		mockBasePipeline := pipelines.NewMockBasePipeline()
		injector.Register("basePipeline", mockBasePipeline)

		// Pre-register pipelines
		originalEnvPipeline := pipelines.NewMockBasePipeline()
		originalExecPipeline := pipelines.NewMockBasePipeline()

		injector.Register("envPipeline", originalEnvPipeline)
		injector.Register("execPipeline", originalExecPipeline)

		cmd := createTestCmd()
		ctx := context.WithValue(context.Background(), injectorKey, injector)
		cmd.SetContext(ctx)

		args := []string{"go", "version"}
		cmd.SetArgs(args)

		err := cmd.Execute()

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// Verify same pipeline instances are reused
		envPipeline := injector.Resolve("envPipeline")
		if envPipeline != originalEnvPipeline {
			t.Error("Expected to reuse existing env pipeline")
		}

		execPipeline := injector.Resolve("execPipeline")
		if execPipeline != originalExecPipeline {
			t.Error("Expected to reuse existing exec pipeline")
		}
	})
}
