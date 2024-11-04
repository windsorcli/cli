package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/helpers"
	"github.com/windsor-hotel/cli/internal/mocks"
	"github.com/windsor-hotel/cli/internal/shell"
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

		// Create a mock TerraformHelper
		mockTerraformHelper := helpers.NewMockHelper()
		mockTerraformHelper.GetEnvVarsFunc = func() (map[string]string, error) {
			return map[string]string{
				"TF_DATA_DIR": "/mock/terraform/data/dir",
			}, nil
		}

		// Mock the PrintEnvVars function
		mockShell := shell.NewMockShell()
		mockShell.PrintEnvVarsFunc = func(envVars map[string]string) error {
			for k, v := range envVars {
				fmt.Printf("%s=%s\n", k, v)
			}
			return nil
		}

		// Initialize mocks and set the container
		mocks := mocks.CreateSuperMocks()
		mocks.Container.Register("terraformHelper", mockTerraformHelper)
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

	t.Run("ResolveHelpersError", func(t *testing.T) {
		defer resetRootCmd()
		defer recoverPanic(t)

		// Given a local container that returns an error when resolving helpers
		mockContainer := di.NewMockContainer()
		mockContainer.SetResolveAllError(errors.New("resolve helpers error")) // Simulate error
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
		expectedOutput := "Error resolving helpers: resolve helpers error"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("ResolveHelpersErrorWithoutVerbose", func(t *testing.T) {
		defer resetRootCmd()
		defer recoverPanic(t)

		// Given a local container that returns an error when resolving helpers
		mockContainer := di.NewMockContainer()
		mockContainer.SetResolveAllError(errors.New("resolve helpers error")) // Simulate error
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
		mockContainer.SetResolveError("shell", errors.New("resolve shell error")) // Simulate error
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

		// Given a helper that returns an error when getting environment variables
		mocks := mocks.CreateSuperMocks()
		mocks.TerraformHelper.GetEnvVarsFunc = func() (map[string]string, error) {
			return nil, errors.New("get env vars error")
		}
		mocks.TerraformHelper.SetPostEnvExecFunc(func() error {
			return nil
		})

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
		mocks.TerraformHelper.GetEnvVarsFunc = func() (map[string]string, error) {
			return nil, errors.New("get env vars error")
		}
		mocks.TerraformHelper.SetPostEnvExecFunc(func() error {
			return nil
		})

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

	t.Run("PostEnvExecError", func(t *testing.T) {
		defer resetRootCmd()
		defer recoverPanic(t)

		// Given a helper that returns an error when executing PostEnvExec
		mocks := mocks.CreateSuperMocks()
		mocks.TerraformHelper.GetEnvVarsFunc = func() (map[string]string, error) {
			return map[string]string{
				"VAR1": "value1",
				"VAR2": "value2",
			}, nil
		}
		mocks.TerraformHelper.SetPostEnvExecFunc(func() error {
			return errors.New("post env exec error")
		})

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
		expectedOutput := "Error executing PostEnvExec: post env exec error"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("PostEnvExecErrorWithoutVerbose", func(t *testing.T) {
		defer resetRootCmd()
		defer recoverPanic(t)

		// Given a helper that returns an error when executing PostEnvExec
		mocks := mocks.CreateSuperMocks()
		mocks.TerraformHelper.GetEnvVarsFunc = func() (map[string]string, error) {
			return map[string]string{
				"VAR1": "value1",
				"VAR2": "value2",
			}, nil
		}
		mocks.TerraformHelper.SetPostEnvExecFunc(func() error {
			return errors.New("post env exec error")
		})

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
