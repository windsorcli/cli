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

// Helper function to set up the container with mock handlers
func setupMockEnvContainer(mockConfigHandler config.ConfigHandler, mockShell shell.Shell, mockHelpers ...helpers.Helper) di.ContainerInterface {
	container := di.NewContainer()
	container.Register("cliConfigHandler", mockConfigHandler)
	container.Register("shell", mockShell)
	for i, helper := range mockHelpers {
		container.Register(fmt.Sprintf("mockHelper%d", i), helper)
	}
	Initialize(container)
	return container
}

func TestEnvCmd(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given: a valid config handler, shell, and helper
		mockConfigHandler := config.NewMockConfigHandler(nil, nil, nil, nil, nil, nil)
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
		setupMockEnvContainer(mockConfigHandler, mockShell, mockHelper)

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
		// Given: a container that returns an error when resolving helpers
		mockConfigHandler := config.NewMockConfigHandler(nil, nil, nil, nil, nil, nil)
		mockShell, err := shell.NewMockShell("cmd")
		if err != nil {
			t.Fatalf("NewMockShell() error = %v", err)
		}
		mockContainer := di.NewMockContainer()
		mockContainer.SetResolveAllError(errors.New("resolve helpers error"))
		mockContainer.Register("cliConfigHandler", mockConfigHandler)
		mockContainer.Register("shell", mockShell)
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
		// Given: a container that returns an error when resolving the shell
		mockConfigHandler := config.NewMockConfigHandler(nil, nil, nil, nil, nil, nil)
		mockHelper := helpers.NewMockHelper(
			func() (map[string]string, error) {
				return map[string]string{
					"VAR1": "value1",
					"VAR2": "value2",
				}, nil
			},
			nil,
		)
		mockContainer := di.NewMockContainer()
		mockContainer.SetResolveError("shell", errors.New("resolve shell error"))
		mockContainer.Register("cliConfigHandler", mockConfigHandler)
		mockContainer.Register("mockHelper0", mockHelper)
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
	})

	t.Run("GetEnvVarsError", func(t *testing.T) {
		// Given: a helper that returns an error when getting environment variables
		mockConfigHandler := config.NewMockConfigHandler(nil, nil, nil, nil, nil, nil)
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
		setupMockEnvContainer(mockConfigHandler, mockShell, mockHelper)

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
}
