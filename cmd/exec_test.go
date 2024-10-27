package cmd

import (
	"bytes"
	"errors"
	"runtime"
	"strings"
	"testing"

	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/mocks"
	"github.com/windsor-hotel/cli/internal/shell"
)

func TestExecCmd(t *testing.T) {
	originalExitFunc := exitFunc
	exitFunc = func(code int) {
		mockExit(code, "")
	}
	t.Cleanup(func() {
		exitFunc = originalExitFunc
	})

	t.Run("Success", func(t *testing.T) {
		defer resetRootCmd()
		defer recoverPanic(t)

		// Setup mock components
		mockShell, _ := shell.NewMockShell("unix")
		mockShell.ExecFunc = func(verbose bool, message string, command string, args ...string) (string, error) {
			return "hello\n", nil
		}
		mocks.DockerHelper.GetEnvVarsFunc = func() (map[string]string, error) {
			return map[string]string{
				"VAR1": "value1",
				"VAR2": "value2",
			}, nil
		}
		mocks.TerraformHelper.GetEnvVarsFunc = func() (map[string]string, error) {
			return map[string]string{
				"TF_VAR1": "tf_value1",
				"TF_VAR2": "tf_value2",
			}, nil
		}
		Initialize(mocks.Container)

		// Capture stdout using a buffer
		output := captureStdout(func() {
			rootCmd.SetArgs([]string{"exec", "echo", "hello"})
			err := rootCmd.Execute()
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
		})

		// Verify the output
		expectedOutput := "hello\n\n"
		if output != expectedOutput {
			t.Errorf("Expected output %q, got %q", expectedOutput, output)
		}
	})

	t.Run("NoCommandProvided", func(t *testing.T) {
		defer resetRootCmd()
		defer recoverPanic(t)

		// Setup mock components using SuperMocks
		mocks := mocks.CreateSuperMocks()
		Initialize(mocks.Container)

		// Capture stderr
		var buf bytes.Buffer
		rootCmd.SetErr(&buf)
		rootCmd.SetArgs([]string{"exec"})
		err := rootCmd.Execute()
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		output := buf.String()

		// Verify output
		expectedOutput := "no command provided"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("ResolveHelpersError", func(t *testing.T) {
		defer resetRootCmd()
		defer recoverPanic(t)

		// Setup mock container
		mockContainer := di.NewMockContainer()
		mockContainer.SetResolveAllError(errors.New("resolve helpers error"))

		// Setup mock components using SuperMocks with the mock container
		mocks := mocks.CreateSuperMocks(mockContainer)
		Initialize(mocks.Container)

		// Capture stderr
		var buf bytes.Buffer
		rootCmd.SetErr(&buf)

		// When the exec command is executed
		rootCmd.SetArgs([]string{"exec", "echo", "hello"})
		err := rootCmd.Execute()
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		output := buf.String()

		// Then the output should indicate the error
		expectedOutput := "Error resolving helpers: resolve helpers error"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("ResolveHelpersErrorWithoutVerbose", func(t *testing.T) {
		defer resetRootCmd()
		defer recoverPanic(t)

		// Given a container that returns an error when resolving helpers
		mockContainer := di.NewMockContainer()
		mockContainer.SetResolveAllError(errors.New("resolve helpers error"))
		mockContainer.Register("shell", mockShell)
		Initialize(mockContainer)
		container = mockContainer

		// Capture stderr
		var buf bytes.Buffer
		rootCmd.SetErr(&buf)

		// When the exec command is executed without verbose flag
		rootCmd.SetArgs([]string{"exec", "echo", "hello"})
		err := rootCmd.Execute()
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		// Then there should be no output
		if buf.Len() != 0 {
			t.Fatalf("Expected no output, got %s", buf.String())
		}
	})

	t.Run("GetEnvVarsError", func(t *testing.T) {
		defer resetRootCmd()
		defer recoverPanic(t)

		// Given a helper that returns an error when getting environment variables
		mocks := mocks.CreateSuperMocks()
		mocks.DockerHelper.GetEnvVarsFunc = func() (map[string]string, error) {
			return nil, errors.New("get env vars error")
		}
		Initialize(mocks.Container)

		// Capture stderr
		var buf bytes.Buffer
		rootCmd.SetErr(&buf)

		// When the exec command is executed with verbose flag
		rootCmd.SetArgs([]string{"exec", "--verbose", "echo", "hello"})
		err := rootCmd.Execute()
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		output := buf.String()

		// Then the output should indicate the error
		expectedOutput := "Error getting environment variables: get env vars error"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("GetEnvVarsErrorWithoutVerbose", func(t *testing.T) {
		defer resetRootCmd()
		defer recoverPanic(t)

		// Given a helper that returns an error when getting environment variables
		mocks := mocks.CreateSuperMocks()
		mocks.DockerHelper.GetEnvVarsFunc = func() (map[string]string, error) {
			return nil, errors.New("get env vars error")
		}
		Initialize(mocks.Container)

		// Capture stderr
		var buf bytes.Buffer
		rootCmd.SetErr(&buf)

		// When the exec command is executed without verbose flag
		rootCmd.SetArgs([]string{"exec", "echo", "hello"})
		err := rootCmd.Execute()
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		output := buf.String()

		// Then the output should indicate the error
		expectedOutput := "Error getting environment variables: get env vars error"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("SetEnvError", func(t *testing.T) {
		defer resetRootCmd()
		defer recoverPanic(t)

		// Given a helper that returns environment variables
		mocks := mocks.CreateSuperMocks()
		mocks.DockerHelper.GetEnvVarsFunc = func() (map[string]string, error) {
			return map[string]string{
				"VAR1": "value1",
			}, nil
		}
		Initialize(mocks.Container)

		// Mock os.Setenv to return an error
		setenvError := func(key, value string) error {
			return errors.New("set env error")
		}
		originalSetenv := osSetenv
		defer func() { osSetenv = originalSetenv }()
		osSetenv = setenvError

		// Execute the command
		rootCmd.SetArgs([]string{"exec", "echo", "hello"})
		err := rootCmd.Execute()
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		// Then the error should indicate the set environment variable error
		expectedError := "Error setting environment variable VAR1: set env error"
		if err.Error() != expectedError {
			t.Errorf("Expected error to be %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("CommandExecutionError", func(t *testing.T) {
		defer resetRootCmd()
		defer recoverPanic(t)

		// Given a shell that returns an error when executing the command
		mockShell, _ := shell.NewMockShell("unix")
		mockShell.ExecFunc = func(verbose bool, message string, command string, args ...string) (string, error) {
			return "", errors.New("command execution error")
		}
		mocks.DockerHelper.GetEnvVarsFunc = func() (map[string]string, error) {
			return map[string]string{
				"VAR1": "value1",
			}, nil
		}
		Initialize(mocks.Container)

		// Execute the command
		rootCmd.SetArgs([]string{"exec", "--verbose", "echo", "hello"})
		err := rootCmd.Execute()
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		// Then the error should indicate the command execution error
		expectedError := "command execution failed: command execution error"
		if err.Error() != expectedError {
			t.Errorf("Expected error to be %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("CommandExecutionErrorWithoutVerbose", func(t *testing.T) {
		defer resetRootCmd()
		defer recoverPanic(t)

		// Given a shell that returns an error when executing the command
		mockShell, _ := shell.NewMockShell("unix")
		mockShell.ExecFunc = func(verbose bool, message string, command string, args ...string) (string, error) {
			if runtime.GOOS == "windows" {
				return "", errors.New("mock stderr output")
			}
			return "", errors.New("command execution error")
		}
		Initialize(mocks.Container)

		// Capture output
		var buf bytes.Buffer
		rootCmd.SetOut(&buf)
		rootCmd.SetErr(&buf)

		// When the exec command is executed without verbose flag
		rootCmd.SetArgs([]string{"exec", "echo", "hello"})
		err := rootCmd.Execute()
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		// Then check that the output contains the error message without usage info
		output := buf.String()
		expectedOutput := "Error: command execution failed: mock stderr output\n"
		if runtime.GOOS != "windows" {
			expectedOutput = "Error: command execution failed: command execution error\n"
		}
		if output != expectedOutput {
			t.Errorf("Expected output %q, got %q", expectedOutput, output)
		}
	})
}
