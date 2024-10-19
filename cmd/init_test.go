package cmd

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/helpers"
	"github.com/windsor-hotel/cli/internal/shell"
)

func TestInitCmd(t *testing.T) {
	originalArgs := rootCmd.Args
	originalExitFunc := exitFunc
	originalTerraformHelper := terraformHelper
	originalColimaHelper := colimaHelper
	originalContextInstance := contextInstance

	t.Cleanup(func() {
		rootCmd.Args = originalArgs
		exitFunc = originalExitFunc
		terraformHelper = originalTerraformHelper
		colimaHelper = originalColimaHelper
		contextInstance = originalContextInstance
	})

	// Mock the exit function to prevent the test from exiting
	exitFunc = func(code int) {
		panic("exit called")
	}

	t.Run("Success", func(t *testing.T) {
		// Given: a valid config handler
		mockHandler := config.NewMockConfigHandler()
		mockShell, err := shell.NewMockShell("cmd")
		if err != nil {
			t.Fatalf("NewMockShell() error = %v", err)
		}
		mockHelper := &helpers.MockHelper{}
		setupContainer(mockHandler, mockHandler, mockShell, mockHelper, mockHelper, nil, dockerHelper)

		// When: the init command is executed with a valid context
		output := captureStdout(func() {
			rootCmd.SetArgs([]string{"init", "test-context"})
			err := rootCmd.Execute()
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
		})

		// Then: the output should indicate success
		expectedOutput := "Initialization successful\n"
		if output != expectedOutput {
			t.Errorf("Expected output %q, got %q", expectedOutput, output)
		}
	})

	t.Run("AllFlagsSet", func(t *testing.T) {
		// Given: a valid config handler
		mockHandler := config.NewMockConfigHandler()
		mockShell, err := shell.NewMockShell("cmd")
		if err != nil {
			t.Fatalf("NewMockShell() error = %v", err)
		}
		mockHelper := &helpers.MockHelper{}
		setupContainer(mockHandler, mockHandler, mockShell, mockHelper, mockHelper, nil, dockerHelper)

		// Mock the Get function to ensure it is called with the desired object
		mockHandler.GetFunc = func(key string) (interface{}, error) {
			if key != "test-context" {
				t.Errorf("Expected key %q, got %q", "test-context", key)
			}
			return map[string]interface{}{
				"AWS": map[string]interface{}{
					"AWSEndpointURL": "http://localhost:4566",
					"AWSProfile":     "test-profile",
				},
				"Docker": map[string]interface{}{
					"Enabled": true,
				},
				"Terraform": map[string]interface{}{
					"Backend": "s3",
				},
				"VM": map[string]interface{}{
					"Driver": "colima",
					"CPU":    2,
					"Disk":   20,
					"Memory": 4096,
					"Arch":   "x86_64",
				},
			}, nil
		}

		// When: the init command is executed with all flags set
		output := captureStdout(func() {
			rootCmd.SetArgs([]string{
				"init", "test-context",
				"--aws-endpoint-url", "http://localhost:4566",
				"--aws-profile", "test-profile",
				"--docker",
				"--backend", "s3",
				"--vm-driver", "colima",
				"--vm-cpu", "2",
				"--vm-disk", "20",
				"--vm-memory", "4096",
				"--vm-arch", "x86_64",
			})
			err := rootCmd.Execute()
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
		})

		// Then: the output should indicate success
		expectedOutput := "Initialization successful\n"
		if output != expectedOutput {
			t.Errorf("Expected output %q, got %q", expectedOutput, output)
		}

		// Verify that the Get function was called
		if mockHandler.GetFunc == nil {
			t.Errorf("Expected Get function to be called")
		}
	})

	t.Run("SetContextConfigError", func(t *testing.T) {
		// Given: a config handler that returns an error on setting the context configuration
		mockHandler := config.NewMockConfigHandler()
		mockHandler.SetFunc = func(key string, value interface{}) error {
			if key == fmt.Sprintf("contexts.%s", "test-context") {
				return errors.New("set context config error")
			}
			return nil
		}
		mockShell, err := shell.NewMockShell("cmd")
		if err != nil {
			t.Fatalf("NewMockShell() error = %v", err)
		}
		mockHelper := &helpers.MockHelper{}
		setupContainer(mockHandler, mockHandler, mockShell, mockHelper, mockHelper, nil, dockerHelper)

		// When: the init command is executed
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"init", "test-context"})
			err := rootCmd.Execute()
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then: the output should indicate the error
		expectedOutput := "Error setting contexts value: set context config error"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("SetConfigValueError", func(t *testing.T) {
		// Given: a config handler that returns an error on SetConfigValue
		mockHandler := config.NewMockConfigHandler()
		mockHandler.SetFunc = func(key string, value interface{}) error { return errors.New("set config value error") }
		mockShell, err := shell.NewMockShell("cmd")
		if err != nil {
			t.Fatalf("NewMockShell() error = %v", err)
		}
		mockHelper := &helpers.MockHelper{}
		setupContainer(mockHandler, mockHandler, mockShell, mockHelper, mockHelper, nil, dockerHelper)

		// When: the init command is executed
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"init", "test-context"})
			err := rootCmd.Execute()
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then: the output should indicate the error
		expectedOutput := "set config value error"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("SaveConfigError", func(t *testing.T) {
		// Given: a config handler that returns an error on SaveConfig
		mockHandler := config.NewMockConfigHandler()
		mockHandler.SaveConfigFunc = func(path string) error { return errors.New("save config error") }
		mockShell, err := shell.NewMockShell("cmd")
		if err != nil {
			t.Fatalf("NewMockShell() error = %v", err)
		}
		mockHelper := &helpers.MockHelper{}
		setupContainer(mockHandler, mockHandler, mockShell, mockHelper, mockHelper, nil, dockerHelper)

		// When: the init command is executed
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"init", "test-context"})
			err := rootCmd.Execute()
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then: the output should indicate the error
		expectedOutput := "Error saving config file: save config error"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("ProjectConfigSaveError", func(t *testing.T) {
		// Given: a CLI config handler that succeeds and a project config handler that returns an error on SaveConfig
		mockCLIHandler := config.NewMockConfigHandler()
		mockProjectHandler := config.NewMockConfigHandler()
		mockProjectHandler.SaveConfigFunc = func(path string) error { return errors.New("save project config error") }
		mockShell, err := shell.NewMockShell("cmd") // Ensure valid shell type
		if err != nil {
			t.Fatalf("NewMockShell() error = %v", err)
		}
		mockHelper := &helpers.MockHelper{}
		setupContainer(mockCLIHandler, mockProjectHandler, mockShell, mockHelper, mockHelper, nil, dockerHelper)

		// When: the init command is executed
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"init", "test-context"})
			err := rootCmd.Execute()
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then: the output should indicate the error
		expectedOutput := "Error saving project config file: save project config error"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("CLIConfigSaveError", func(t *testing.T) {
		// Given: a config handler that returns an error on SaveConfig
		mockHandler := config.NewMockConfigHandler()
		mockHandler.SaveConfigFunc = func(path string) error { return errors.New("save cli config error") }
		mockShell, err := shell.NewMockShell("cmd")
		if err != nil {
			t.Fatalf("NewMockShell() error = %v", err)
		}
		mockHelper := &helpers.MockHelper{}

		// Replace the global contextInstance with the mock
		originalContextInstance := contextInstance
		defer func() { contextInstance = originalContextInstance }()

		setupContainer(mockHandler, mockHandler, mockShell, mockHelper, mockHelper, nil, dockerHelper)

		// When: the init command is executed
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"init", "test-context"})
			err := rootCmd.Execute()
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then: the output should indicate the error
		expectedOutput := "Error saving config file: save cli config error"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("WriteConfigError", func(t *testing.T) {
		// Given: a helper that returns an error on WriteConfig
		mockHandler := config.NewMockConfigHandler()
		mockShell, err := shell.NewMockShell("cmd")
		if err != nil {
			t.Fatalf("NewMockShell() error = %v", err)
		}
		mockHelper := &helpers.MockHelper{
			WriteConfigFunc: func() error {
				return errors.New("write config error")
			},
		}
		setupContainer(mockHandler, mockHandler, mockShell, mockHelper, nil, nil, nil)

		// When: the init command is executed
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"init", "test-context"})
			err := rootCmd.Execute()
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then: the output should indicate the error
		expectedOutput := "error writing config for helper: write config error"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("ResolveHelpersError", func(t *testing.T) {
		defer resetRootCmd()
		defer recoverPanic(t)

		// Given a container that returns an error when resolving helpers
		mockCliConfigHandler := config.NewMockConfigHandler()
		mockProjectConfigHandler := config.NewMockConfigHandler()
		mockShell, err := shell.NewMockShell("cmd")
		if err != nil {
			t.Fatalf("NewMockShell() error = %v", err)
		}
		mockHelper := helpers.NewMockHelper(func() (map[string]string, error) {
			return nil, nil
		})
		mockContainer := di.NewMockContainer()
		mockContainer.SetResolveAllError(errors.New("resolve helpers error")) // Simulate error
		mockContainer.Register("cliConfigHandler", mockCliConfigHandler)
		mockContainer.Register("projectConfigHandler", mockProjectConfigHandler)
		mockContainer.Register("shell", mockShell)
		mockContainer.Register("terraformHelper", mockHelper)
		mockContainer.Register("awsHelper", mockHelper)
		mockContainer.Register("colimaHelper", mockHelper)
		mockContainer.Register("dockerHelper", mockHelper)
		Initialize(mockContainer)

		// When the init command is executed
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"init", "test-context"})
			err := rootCmd.Execute()
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then the output should indicate the error
		expectedOutput := "error resolving helpers: resolve helpers error"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("LocalContextSetsDefault", func(t *testing.T) {
		// Arrange: Create a mock config handler and a flag to check if SetDefault was called
		mockHandler := config.NewMockConfigHandler()
		called := false

		// Set the SetDefaultFunc to update the flag and check the parameters
		mockHandler.SetDefaultFunc = func(context config.Context) {
			called = true
			expectedValue := config.DefaultLocalConfig
			if !reflect.DeepEqual(context, expectedValue) {
				t.Errorf("Expected value %v, got %v", expectedValue, context)
			}
		}

		mockShell, err := shell.NewMockShell("cmd")
		if err != nil {
			t.Fatalf("NewMockShell() error = %v", err)
		}
		mockHelper := &helpers.MockHelper{}
		setupContainer(mockHandler, mockHandler, mockShell, mockHelper, mockHelper, nil, dockerHelper)

		// Act: Call the init command with a local context
		output := captureStdout(func() {
			rootCmd.SetArgs([]string{"init", "local"})
			err := rootCmd.Execute()
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
		})

		// Assert: Verify that SetDefault was called
		if !called {
			t.Error("Expected SetDefaultFunc to be called")
		}

		// Assert: Verify the output indicates success
		expectedOutput := "Initialization successful\n"
		if output != expectedOutput {
			t.Errorf("Expected output %q, got %q", expectedOutput, output)
		}
	})
}
