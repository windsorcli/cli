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

		// Initialize mocks and set the injector
		mocks := mocks.CreateSuperMocks()

		// Create a mock WindsorEnv
		mockEnv := env.NewMockEnvPrinter()
		mockEnv.PrintFunc = func() error {
			fmt.Println("export VAR=value")
			return nil
		}
		mocks.Injector.Register("windsorEnv", mockEnv)

		Initialize(mocks.Injector)

		// Capture the output using captureStdout
		output := captureStdout(func() {
			rootCmd.SetArgs([]string{"env"})
			err := rootCmd.Execute()
			if err != nil {
				t.Fatalf("Expected no error, got %v", err)
			}
		})

		// Verify the output
		expectedOutput := "export VAR=value\n"
		if output != expectedOutput {
			t.Errorf("Expected output %q, got %q", expectedOutput, output)
		}
	})

	t.Run("ResolveEnvError", func(t *testing.T) {
		defer resetRootCmd()
		defer recoverPanic(t)

		// Given a local injector that returns an error when resolving env
		mockInjector := di.NewMockInjector()
		mockInjector.SetResolveAllError(fmt.Errorf("resolve env error"))
		mocks := mocks.CreateSuperMocks(mockInjector)

		Initialize(mocks.Injector)

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

		// Given a local injector that returns an error when resolving env
		mockInjector := di.NewMockInjector()
		mockInjector.SetResolveAllError(fmt.Errorf("resolve env error"))
		mocks := mocks.CreateSuperMocks(mockInjector)

		Initialize(mocks.Injector)

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

	t.Run("GetEnvVarsErrorWithoutVerbose", func(t *testing.T) {
		defer resetRootCmd()
		defer recoverPanic(t)

		// Given a mock environment that returns an error when getting environment variables
		mocks := mocks.CreateSuperMocks()
		mockEnv := mocks.WindsorEnv
		mockEnv.GetEnvVarsFunc = func() (map[string]string, error) {
			return nil, fmt.Errorf("get env vars error")
		}
		mockEnv.PostEnvHookFunc = func() error {
			return nil
		}

		Initialize(mocks.Injector)

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

	t.Run("ErrorPrinting", func(t *testing.T) {
		defer resetRootCmd()
		defer recoverPanic(t)

		// Given an env that returns an error when executing Print
		mocks := mocks.CreateSuperMocks()
		mockEnv := mocks.WindsorEnv
		mockEnv.PrintFunc = func() error {
			return fmt.Errorf("print error")
		}
		mockEnv.PostEnvHookFunc = func() error {
			return nil
		}

		Initialize(mocks.Injector)

		// When the env command is executed with verbose flag
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"env", "--verbose"})
			err := rootCmd.Execute()
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then the output should indicate the error
		expectedOutput := "Error executing Print: print error"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("ErrorPrintingWithoutVerbose", func(t *testing.T) {
		defer resetRootCmd()
		defer recoverPanic(t)

		// Given an env that returns an error when executing Print
		mocks := mocks.CreateSuperMocks()
		mockEnv := mocks.WindsorEnv
		mockEnv.PrintFunc = func() error {
			return fmt.Errorf("print error")
		}
		mockEnv.PostEnvHookFunc = func() error {
			return nil
		}

		Initialize(mocks.Injector)

		// Capture the output
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"env"})
			err := rootCmd.Execute()
			if err != nil {
				t.Fatalf("Expected no error, got %v", err)
			}
		})

		// Then the output should not indicate the error
		unexpectedOutput := "Error executing Print: print error"
		if strings.Contains(output, unexpectedOutput) {
			t.Errorf("Did not expect output to contain %q, got %q", unexpectedOutput, output)
		}
	})

	t.Run("PostEnvHookError", func(t *testing.T) {
		defer resetRootCmd()
		defer recoverPanic(t)

		// Given an env that returns an error when executing PostEnvHook
		mocks := mocks.CreateSuperMocks()
		mockEnv := mocks.WindsorEnv
		mockEnv.PrintFunc = func() error {
			return nil
		}
		mockEnv.PostEnvHookFunc = func() error {
			return fmt.Errorf("post env hook error")
		}

		Initialize(mocks.Injector)

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
		mockEnv.PrintFunc = func() error {
			return nil
		}
		mockEnv.PostEnvHookFunc = func() error {
			return fmt.Errorf("post env hook error")
		}

		Initialize(mocks.Injector)

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
