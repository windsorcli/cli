package cmd

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/env"
	"github.com/windsor-hotel/cli/internal/mocks"
)

func TestEnvCmd(t *testing.T) {
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

		// Initialize mocks and set the container
		mocks := mocks.CreateSuperMocks()

		// Create a mock EnvInterface
		mockEnv := env.NewMockEnv(mocks.Container)
		mockEnv.PrintFunc = func(envVars map[string]string) error {
			envVars["TF_DATA_DIR"] = "/mock/terraform/data/dir"
			fmt.Printf("TF_DATA_DIR=%s\n", envVars["TF_DATA_DIR"])
			return nil
		}

		// Mock the Print function using the correct shell package
		mockShell := mocks.Shell
		mockShell.PrintEnvVarsFunc = func(envVars map[string]string) error {
			for k, v := range envVars {
				fmt.Printf("%s=%s\n", k, v)
			}
			return nil
		}

		mocks.Container.Register("windsorEnv", mockEnv)
		mocks.Container.Register("shell", mockShell)

		Initialize(mocks.Container)

		// Capture stdout using the captureStdout function
		output := captureStdout(func() {
			// When the env command is executed
			rootCmd.SetArgs([]string{"env"})
			err := rootCmd.Execute()
			if err != nil {
				t.Fatalf("Expected no error, got %v", err)
			}
		})

		// Then the output should contain the expected environment variables
		expectedOutput := "TF_DATA_DIR=/mock/terraform/data/dir"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("ResolveEnvError", func(t *testing.T) {
		defer resetRootCmd()
		defer recoverPanic(t)

		// Given a local container that returns an error when resolving env
		mockContainer := di.NewMockContainer()
		mockContainer.SetResolveAllError(fmt.Errorf("resolve env error"))
		mocks := mocks.CreateSuperMocks(mockContainer)

		Initialize(mocks.Container)

		// Capture stderr
		var buf bytes.Buffer
		rootCmd.SetErr(&buf)

		// When the env command is executed with verbose flag
		rootCmd.SetArgs([]string{"env", "--verbose"})
		err := rootCmd.Execute()
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
		defer recoverPanic(t)

		// Given a local container that returns an error when resolving env
		mockContainer := di.NewMockContainer()
		mockContainer.SetResolveAllError(fmt.Errorf("resolve env error"))
		mocks := mocks.CreateSuperMocks(mockContainer)

		Initialize(mocks.Container)

		// Capture stderr
		var buf bytes.Buffer
		rootCmd.SetErr(&buf)

		// When the env command is executed without verbose flag
		rootCmd.SetArgs([]string{"env"})
		err := rootCmd.Execute()
		// Then the error should be nil and no output should be produced
		if err != nil {
			t.Fatalf("Expected error nil, got %v", err)
		}

		// Then there should be no output
		if buf.Len() != 0 {
			t.Fatalf("Expected no output, got %s", buf.String())
		}
	})

	t.Run("ResolveShellError", func(t *testing.T) {
		defer resetRootCmd()
		defer recoverPanic(t)
		// Given a local container that returns an error when resolving the shell
		mockContainer := di.NewMockContainer()
		mockContainer.SetResolveError("shell", fmt.Errorf("resolve shell error"))
		mocks := mocks.CreateSuperMocks(mockContainer)

		Initialize(mocks.Container)

		// When the env command is executed with verbose flag
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"env", "--verbose"})
			err := rootCmd.Execute()
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then the output should indicate the error
		expectedOutput := "Error resolving shell: resolve shell error"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}

		// Assert that exitFunc was called with the correct code and message
		if exitCode != 1 {
			t.Errorf("Expected exit code 1, got %d", exitCode)
		}
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected exit message to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("GetEnvVarsError", func(t *testing.T) {
		defer resetRootCmd()
		defer recoverPanic(t)

		// Given a mock environment that returns an error when getting environment variables
		mocks := mocks.CreateSuperMocks()
		mockEnv := mocks.WindsorEnv
		mockEnv.PrintFunc = func(envVars map[string]string) error {
			return fmt.Errorf("get env vars error")
		}
		mockEnv.PostEnvHookFunc = func() error {
			return nil
		}

		Initialize(mocks.Container)

		// When the env command is executed with verbose flag
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"env", "--verbose"})
			err := rootCmd.Execute()
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then the output should indicate the error
		if !strings.Contains(output, "get env vars error") {
			t.Errorf("Expected output to contain error message, got %q", output)
		}
	})

	t.Run("GetEnvVarsErrorWithoutVerbose", func(t *testing.T) {
		defer resetRootCmd()
		defer recoverPanic(t)

		// Given a mock environment that returns an error when getting environment variables
		mocks := mocks.CreateSuperMocks()
		mockEnv := mocks.WindsorEnv
		mockEnv.PrintFunc = func(envVars map[string]string) error {
			return fmt.Errorf("get env vars error")
		}
		mockEnv.PostEnvHookFunc = func() error {
			return nil
		}

		Initialize(mocks.Container)

		// Capture the output
		var buf bytes.Buffer
		rootCmd.SetOut(&buf)
		rootCmd.SetErr(&buf)

		// When the env command is executed without verbose flag
		rootCmd.SetArgs([]string{"env"})
		err := rootCmd.Execute()

		// Then the error should be nil and no output should be produced
		if err != nil {
			t.Fatalf("Expected error nil, got %v", err)
		}
		if buf.Len() != 0 {
			t.Fatalf("Expected no output, got %s", buf.String())
		}
	})

	t.Run("PostEnvHookError", func(t *testing.T) {
		defer resetRootCmd()
		defer recoverPanic(t)

		// Given an env that returns an error when executing PostEnvHook
		mocks := mocks.CreateSuperMocks()
		mockEnv := mocks.WindsorEnv
		mockEnv.PrintFunc = func(envVars map[string]string) error {
			envVars["VAR1"] = "value1"
			envVars["VAR2"] = "value2"
			return nil
		}
		mockEnv.PostEnvHookFunc = func() error {
			return fmt.Errorf("post env hook error")
		}

		Initialize(mocks.Container)

		// When the env command is executed with verbose flag
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"env", "--verbose"})
			err := rootCmd.Execute()
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then the output should indicate the error
		expectedOutput := "Error executing PostEnvHook: post env hook error"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("PostEnvHookErrorWithoutVerbose", func(t *testing.T) {
		defer resetRootCmd()
		defer recoverPanic(t)

		// Given an env that returns an error when executing PostEnvHook
		mocks := mocks.CreateSuperMocks()
		mockEnv := mocks.WindsorEnv
		mockEnv.PrintFunc = func(envVars map[string]string) error {
			envVars["VAR1"] = "value1"
			envVars["VAR2"] = "value2"
			return nil
		}
		mockEnv.PostEnvHookFunc = func() error {
			return fmt.Errorf("post env hook error")
		}

		Initialize(mocks.Container)

		// Capture the output
		var buf bytes.Buffer
		rootCmd.SetOut(&buf)
		rootCmd.SetErr(&buf)

		// When the env command is executed without verbose flag
		rootCmd.SetArgs([]string{"env"})
		err := rootCmd.Execute()

		// Then the error should be nil and no output should be produced
		if err != nil {
			t.Fatalf("Expected error nil, got %v", err)
		}
		if buf.Len() != 0 {
			t.Fatalf("Expected no output, got %s", buf.String())
		}
	})
}

// resetRootCmd resets the root command to its initial state.
func resetRootCmd() {
	rootCmd.SetArgs([]string{})
	rootCmd.SetOut(nil)
	rootCmd.SetErr(nil)
	verbose = false // Reset the verbose flag
}

// recoverPanic recovers from a panic and checks for the expected exit code.
func recoverPanic(t *testing.T) {
	if r := recover(); r != nil {
		if r != "exit code: 1" {
			t.Fatalf("unexpected panic: %v", r)
		}
	}
}
