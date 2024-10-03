package cmd

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/helpers"
	"github.com/windsor-hotel/cli/internal/shell"
)

func TestEnvCmd(t *testing.T) {
	originalExitFunc := exitFunc
	var exitCode int
	exitFunc = func(code int) {
		exitCode = code
		panic(fmt.Sprintf("exit code: %d", code)) // Simulate exit
	}
	t.Cleanup(func() {
		exitFunc = originalExitFunc
	})

	t.Run("Success", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				if r != "exit code: 1" {
					t.Fatalf("unexpected panic: %v", r)
				}
			}
		}()

		// Given: a valid config handler, shell, and helper
		mockCliConfigHandler := config.NewMockConfigHandler(nil, nil, nil, nil, nil, nil)
		mockProjectConfigHandler := config.NewMockConfigHandler(nil, nil, nil, nil, nil, nil)
		mockShell, err := shell.NewMockShell("cmd")
		if err != nil {
			t.Fatalf("NewMockShell() error = %v", err)
		}
		mockHelper := helpers.NewMockHelper(
			func() (map[string]string, error) {
				return map[string]string{
					"VAR1": "value1",
					"VAR2": "value2",
				}, nil
			},
			mockShell,
		)
		mockHelper.SetPostEnvExecFunc(func() error {
			return nil
		})
		setupContainer(mockCliConfigHandler, mockProjectConfigHandler, mockShell, mockHelper, nil, nil)

		// When: the env command is executed
		output := captureStdout(func() {
			rootCmd.SetArgs([]string{"env"})
			err := rootCmd.Execute()
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
		})

		// Then: the output should contain the environment variables
		expectedOutput := "VAR1=value1\nVAR2=value2\n"
		if output != expectedOutput {
			t.Errorf("Expected output %q, got %q", expectedOutput, output)
		}
	})

	t.Run("ResolveHelpersError", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				if r != "exit code: 1" {
					t.Fatalf("unexpected panic: %v", r)
				}
			}
		}()

		// Given: a container that returns an error when resolving helpers
		mockCliConfigHandler := config.NewMockConfigHandler(nil, nil, nil, nil, nil, nil)
		mockProjectConfigHandler := config.NewMockConfigHandler(nil, nil, nil, nil, nil, nil)
		mockShell, err := shell.NewMockShell("cmd")
		if err != nil {
			t.Fatalf("NewMockShell() error = %v", err)
		}
		mockHelper := helpers.NewMockHelper(nil, mockShell)
		mockContainer := di.NewMockContainer()
		mockContainer.SetResolveAllError(errors.New("resolve helpers error")) // Simulate error
		mockContainer.Register("cliConfigHandler", mockCliConfigHandler)
		mockContainer.Register("projectConfigHandler", mockProjectConfigHandler)
		mockContainer.Register("shell", mockShell)
		mockContainer.Register("terraformHelper", mockHelper)
		mockContainer.Register("awsHelper", mockHelper)
		mockContainer.Register("colimaHelper", mockHelper) // Register ColimaHelper
		Initialize(mockContainer)

		// When: the env command is executed
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"env"})
			err := rootCmd.Execute()
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then: the output should indicate the error
		expectedOutput := "Error resolving helpers: resolve helpers error"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("ResolveShellError", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				if r != "exit code: 1" {
					t.Fatalf("unexpected panic: %v", r)
				}
			}
		}()

		// Given: a container that returns an error when resolving the shell
		mockCliConfigHandler := config.NewMockConfigHandler(nil, nil, nil, nil, nil, nil)
		mockProjectConfigHandler := config.NewMockConfigHandler(nil, nil, nil, nil, nil, nil)
		mockContainer := di.NewMockContainer()
		mockContainer.SetResolveError("shell", errors.New("resolve shell error")) // Simulate error
		mockContainer.Register("cliConfigHandler", mockCliConfigHandler)
		mockContainer.Register("projectConfigHandler", mockProjectConfigHandler)
		Initialize(mockContainer)

		// When: the env command is executed
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"env"})
			err := rootCmd.Execute()
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then: the output should indicate the error
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
		defer func() {
			if r := recover(); r != nil {
				if r != "exit code: 1" {
					t.Fatalf("unexpected panic: %v", r)
				}
			}
		}()

		// Given: a helper that returns an error when getting environment variables
		mockCliConfigHandler := config.NewMockConfigHandler(nil, nil, nil, nil, nil, nil)
		mockProjectConfigHandler := config.NewMockConfigHandler(nil, nil, nil, nil, nil, nil)
		mockShell, err := shell.NewMockShell("cmd")
		if err != nil {
			t.Fatalf("NewMockShell() error = %v", err)
		}
		mockHelper := helpers.NewMockHelper(
			func() (map[string]string, error) {
				return nil, errors.New("get env vars error")
			},
			mockShell,
		)
		mockHelper.SetPostEnvExecFunc(func() error {
			return nil
		})
		setupContainer(mockCliConfigHandler, mockProjectConfigHandler, mockShell, mockHelper, nil, nil)

		// When: the env command is executed
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"env"})
			err := rootCmd.Execute()
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then: the output should indicate the error
		expectedOutput := "Error getting environment variables: get env vars error"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("PostEnvExecError", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				if r != "exit code: 1" {
					t.Fatalf("unexpected panic: %v", r)
				}
			}
		}()

		// Given: a helper that returns an error when executing PostEnvExec
		mockCliConfigHandler := config.NewMockConfigHandler(nil, nil, nil, nil, nil, nil)
		mockProjectConfigHandler := config.NewMockConfigHandler(nil, nil, nil, nil, nil, nil)
		mockShell, err := shell.NewMockShell("cmd")
		if err != nil {
			t.Fatalf("NewMockShell() error = %v", err)
		}
		mockHelper := helpers.NewMockHelper(
			func() (map[string]string, error) {
				return map[string]string{
					"VAR1": "value1",
					"VAR2": "value2",
				}, nil
			},
			mockShell,
		)
		mockHelper.SetPostEnvExecFunc(func() error {
			return errors.New("post env exec error")
		})
		setupContainer(mockCliConfigHandler, mockProjectConfigHandler, mockShell, mockHelper, nil, nil)

		// When: the env command is executed
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"env"})
			err := rootCmd.Execute()
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then: the output should indicate the error
		expectedOutput := "Error executing PostEnvExec: post env exec error"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})
}
