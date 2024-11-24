package cmd

import (
	"bytes"
	"errors"
	"runtime"
	"strings"
	"testing"

	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/mocks"
)

func TestExecCmd(t *testing.T) {
	originalExitFunc := exitFunc
	exitFunc = mockExit
	t.Cleanup(func() {
		exitFunc = originalExitFunc
	})

	t.Run("Success", func(t *testing.T) {
		defer resetRootCmd()

		// Setup mock components using SuperMocks
		mocks := mocks.CreateSuperMocks()
		mocks.Shell.ExecFunc = func(verbose bool, message string, command string, args ...string) (string, error) {
			return "hello\n", nil
		}

		// Capture stdout using a buffer
		output := captureStdout(func() {
			rootCmd.SetArgs([]string{"exec", "echo", "hello"})
			err := Execute(mocks.Injector)
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

		// Setup mock components using SuperMocks
		mocks := mocks.CreateSuperMocks()

		// Capture stderr
		var buf bytes.Buffer
		rootCmd.SetErr(&buf)
		rootCmd.SetArgs([]string{"exec"})
		err := Execute(mocks.Injector)
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

	t.Run("ResolveEnvError", func(t *testing.T) {
		defer resetRootCmd()

		// Setup mock injector
		mockInjector := di.NewMockInjector()
		mockInjector.SetResolveAllError(errors.New("resolve env error"))

		// Setup mock components using SuperMocks with the mock injector
		mocks := mocks.CreateSuperMocks(mockInjector)

		// Capture stderr
		var buf bytes.Buffer
		rootCmd.SetErr(&buf)

		// When the exec command is executed
		rootCmd.SetArgs([]string{"exec", "echo", "hello"})
		err := Execute(mocks.Injector)
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		output := buf.String()

		// Then the output should indicate the error
		expectedOutput := "Error resolving environments: resolve env error"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("ResolveEnvErrorWithoutVerbose", func(t *testing.T) {
		defer resetRootCmd()

		// Given a injector that returns an error when resolving environments
		mockInjector := di.NewMockInjector()
		mockInjector.SetResolveAllError(errors.New("resolve env error")) // Simulate error
		mocks := mocks.CreateSuperMocks(mockInjector)

		// Capture stderr
		var buf bytes.Buffer
		rootCmd.SetErr(&buf)

		// When the exec command is executed without verbose flag
		rootCmd.SetArgs([]string{"exec", "echo", "hello"})
		err := Execute(mocks.Injector)
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		// Then the output should indicate the error
		expectedOutput := "Error resolving environments: resolve env error"
		if !strings.Contains(buf.String(), expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, buf.String())
		}
	})

	t.Run("ErrorInitializing", func(t *testing.T) {
		defer resetRootCmd()

		// Given an environment that returns an error when initializing
		mocks := mocks.CreateSuperMocks()
		mocks.WindsorEnv.InitializeFunc = func() error {
			return errors.New("initialize error")
		}

		// Capture stderr
		var buf bytes.Buffer
		rootCmd.SetErr(&buf)

		// When the exec command is executed with verbose flag
		rootCmd.SetArgs([]string{"exec", "--verbose", "echo", "hello"})
		err := Execute(mocks.Injector)
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		output := buf.String()

		// Then the output should indicate the error
		expectedOutput := "Error initializing environment: initialize error"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("GetEnvVarsError", func(t *testing.T) {
		defer resetRootCmd()

		// Given an environment that returns an error when getting environment variables
		mocks := mocks.CreateSuperMocks()
		mocks.WindsorEnv.GetEnvVarsFunc = func() (map[string]string, error) {
			return nil, errors.New("get env vars error")
		}

		// Capture stderr
		var buf bytes.Buffer
		rootCmd.SetErr(&buf)

		// When the exec command is executed with verbose flag
		rootCmd.SetArgs([]string{"exec", "--verbose", "echo", "hello"})
		err := Execute(mocks.Injector)
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

		// Given an environment that returns an error when getting environment variables
		mocks := mocks.CreateSuperMocks()
		mocks.WindsorEnv.GetEnvVarsFunc = func() (map[string]string, error) {
			return nil, errors.New("get env vars error")
		}

		// Capture stderr
		var buf bytes.Buffer
		rootCmd.SetErr(&buf)

		// When the exec command is executed without verbose flag
		rootCmd.SetArgs([]string{"exec", "echo", "hello"})
		err := Execute(mocks.Injector)
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

		// Given an environment that returns environment variables
		mocks := mocks.CreateSuperMocks()
		mocks.WindsorEnv.GetEnvVarsFunc = func() (map[string]string, error) {
			return map[string]string{
				"VAR1": "value1",
			}, nil
		}
		// Mock os.Setenv to return an error
		setenvError := func(key, value string) error {
			return errors.New("set env error")
		}
		originalSetenv := osSetenv
		defer func() { osSetenv = originalSetenv }()
		osSetenv = setenvError

		// Execute the command
		rootCmd.SetArgs([]string{"exec", "echo", "hello"})
		err := Execute(mocks.Injector)
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		// Then the error should indicate the set environment variable error
		expectedError := "Error setting environment variable VAR1: set env error"
		if err.Error() != expectedError {
			t.Errorf("Expected error to be %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorResolvingShell", func(t *testing.T) {
		defer resetRootCmd()

		// Given an injector that returns an error when resolving the shell
		mockInjector := di.NewMockInjector()
		mockInjector.SetResolveError("shell", errors.New("resolve shell error"))
		mocks := mocks.CreateSuperMocks(mockInjector)

		// When the exec command is executed
		rootCmd.SetArgs([]string{"exec", "echo", "hello"})
		err := Execute(mocks.Injector)
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		// Then the error should indicate the shell resolution error
		expectedError := "Error resolving shell instance: resolve shell error"
		if err.Error() != expectedError {
			t.Errorf("Expected error to be %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorCastingShell", func(t *testing.T) {
		defer resetRootCmd()

		// Given a shell that returns an error when casting
		mocks := mocks.CreateSuperMocks()
		mocks.Injector.Register("shell", "invalid")

		// When the exec command is executed
		rootCmd.SetArgs([]string{"exec", "echo", "hello"})
		err := Execute(mocks.Injector)
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		// Then the error should indicate the casting error
		expectedError := "Resolved instance is not of type shell.Shell"
		if err.Error() != expectedError {
			t.Errorf("Expected error to be %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorInitializingShell", func(t *testing.T) {
		defer resetRootCmd()

		// Given a shell that returns an error when initializing
		mocks := mocks.CreateSuperMocks()
		mocks.Shell.InitializeFunc = func() error {
			return errors.New("initialize shell error")
		}

		// When the exec command is executed
		rootCmd.SetArgs([]string{"exec", "echo", "hello"})
		err := Execute(mocks.Injector)
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		// Then the error should indicate the shell initialization error
		expectedError := "Error initializing shell: initialize shell error"
		if err.Error() != expectedError {
			t.Errorf("Expected error to be %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("CommandExecutionError", func(t *testing.T) {
		defer resetRootCmd()

		// Given a shell that returns an error when executing the command
		mocks := mocks.CreateSuperMocks()
		mocks.Shell.ExecFunc = func(verbose bool, message string, command string, args ...string) (string, error) {
			return "", errors.New("command execution error")
		}
		mocks.WindsorEnv.GetEnvVarsFunc = func() (map[string]string, error) {
			return map[string]string{
				"VAR1": "value1",
			}, nil
		}

		// Execute the command
		rootCmd.SetArgs([]string{"exec", "--verbose", "echo", "hello"})
		err := Execute(mocks.Injector)
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

		// Given a shell that returns an error when executing the command
		mocks := mocks.CreateSuperMocks()
		mocks.Shell.ExecFunc = func(verbose bool, message string, command string, args ...string) (string, error) {
			if runtime.GOOS == "windows" {
				return "", errors.New("mock stderr output")
			}
			return "", errors.New("command execution error")
		}

		// Capture output
		var buf bytes.Buffer
		rootCmd.SetOut(&buf)
		rootCmd.SetErr(&buf)

		// When the exec command is executed without verbose flag
		rootCmd.SetArgs([]string{"exec", "echo", "hello"})
		err := Execute(mocks.Injector)
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
