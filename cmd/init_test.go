package cmd

import (
	"errors"
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

	t.Run("SetConfigValueError", func(t *testing.T) {
		// Given: a config handler that returns an error on SetConfigValue
		mockHandler := config.NewMockConfigHandler()
		mockHandler.SetConfigValueFunc = func(key string, value interface{}) error { return errors.New("set config value error") }
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

	t.Run("SetBackendConfigError", func(t *testing.T) {
		// Given: a config handler that returns an error on setting backend config value
		mockHandler := config.NewMockConfigHandler()
		mockHandler.SetConfigValueFunc = func(key string, value interface{}) error {
			if key == "contexts.test-context.terraform.backend" {
				return errors.New("set backend config error")
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
		expectedOutput := "Error setting backend value: set backend config error"
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

	t.Run("SetAwsConfigError", func(t *testing.T) {
		// Given: a config handler that returns an error on setting AWS config values
		mockHandler := config.NewMockConfigHandler()
		mockHandler.SetConfigValueFunc = func(key string, value interface{}) error {
			if key == "contexts.test-context.aws.aws_endpoint_url" || key == "contexts.test-context.aws.aws_profile" {
				return errors.New("set aws config error")
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
		expectedOutput := "error setting aws_endpoint_url: set aws config error"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("SetAwsProfileError", func(t *testing.T) {
		// Given: a config handler that returns an error on setting AWS profile
		mockHandler := config.NewMockConfigHandler()
		mockHandler.SetConfigValueFunc = func(key string, value interface{}) error {
			if key == "contexts.test-context.aws.aws_profile" {
				return errors.New("set aws profile error")
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
		expectedOutput := "error setting aws_profile: set aws profile error"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("SetColimaTypeError", func(t *testing.T) {
		// Given: a config handler that returns an error on setting Colima driver
		mockHandler := config.NewMockConfigHandler()
		mockHandler.SetConfigValueFunc = func(key string, value interface{}) error {
			if key == "contexts.test-context.vm.driver" {
				return errors.New("set driver error")
			}
			return nil
		}
		mockShell, err := shell.NewMockShell("cmd")
		if err != nil {
			t.Fatalf("NewMockShell() error = %v", err)
		}
		mockHelper := &helpers.MockHelper{}
		setupContainer(mockHandler, mockHandler, mockShell, mockHelper, mockHelper, mockHelper, dockerHelper)

		// When: the init command is executed with vm-driver flag and context
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"init", "test-context", "--vm-driver", "test-driver"})
			err := rootCmd.Execute()
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then: the output should indicate the error
		expectedOutput := "error setting vm driver: set driver error"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("SetColimaCpuError", func(t *testing.T) {
		// Given: a config handler that returns an error on setting Colima CPU
		mockHandler := config.NewMockConfigHandler()
		mockHandler.SetConfigValueFunc = func(key string, value interface{}) error {
			if key == "contexts.test-context.vm.cpu" {
				return errors.New("set cpu error")
			}
			return nil
		}
		mockShell, err := shell.NewMockShell("cmd")
		if err != nil {
			t.Fatalf("NewMockShell() error = %v", err)
		}
		mockHelper := &helpers.MockHelper{}
		setupContainer(mockHandler, mockHandler, mockShell, mockHelper, mockHelper, mockHelper, dockerHelper)

		// When: the init command is executed with vm-cpu flag and context
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"init", "test-context", "--vm-cpu", "test-cpu"})
			err := rootCmd.Execute()
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then: the output should indicate the error
		expectedOutput := "error setting vm cpu: set cpu error"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("SetColimaDiskError", func(t *testing.T) {
		// Given: a config handler that returns an error on setting Colima disk
		mockHandler := config.NewMockConfigHandler()
		mockHandler.SetConfigValueFunc = func(key string, value interface{}) error {
			if key == "contexts.test-context.vm.disk" {
				return errors.New("set disk error")
			}
			return nil
		}
		mockShell, err := shell.NewMockShell("cmd")
		if err != nil {
			t.Fatalf("NewMockShell() error = %v", err)
		}
		mockHelper := &helpers.MockHelper{}
		setupContainer(mockHandler, mockHandler, mockShell, mockHelper, mockHelper, mockHelper, dockerHelper)

		// When: the init command is executed with vm-disk flag and context
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"init", "test-context", "--vm-disk", "test-disk"})
			err := rootCmd.Execute()
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then: the output should indicate the error
		expectedOutput := "error setting vm disk: set disk error"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("SetColimaMemoryError", func(t *testing.T) {
		// Given: a config handler that returns an error on setting Colima memory
		mockHandler := config.NewMockConfigHandler()
		mockHandler.SetConfigValueFunc = func(key string, value interface{}) error {
			if key == "contexts.test-context.vm.memory" {
				return errors.New("set memory error")
			}
			return nil
		}
		mockShell, err := shell.NewMockShell("cmd")
		if err != nil {
			t.Fatalf("NewMockShell() error = %v", err)
		}
		mockHelper := &helpers.MockHelper{}
		setupContainer(mockHandler, mockHandler, mockShell, mockHelper, mockHelper, mockHelper, dockerHelper)

		// When: the init command is executed with vm-memory flag and context
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"init", "test-context", "--vm-memory", "test-memory"})
			err := rootCmd.Execute()
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then: the output should indicate the error
		expectedOutput := "error setting vm memory: set memory error"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("SetColimaArchError", func(t *testing.T) {
		// Given: a config handler that returns an error on setting Colima arch
		mockHandler := config.NewMockConfigHandler()
		mockHandler.SetConfigValueFunc = func(key string, value interface{}) error {
			if key == "contexts.test-context.vm.arch" {
				return errors.New("set arch error")
			}
			return nil
		}
		mockShell, err := shell.NewMockShell("cmd")
		if err != nil {
			t.Fatalf("NewMockShell() error = %v", err)
		}
		mockHelper := &helpers.MockHelper{}
		setupContainer(mockHandler, mockHandler, mockShell, mockHelper, mockHelper, mockHelper, dockerHelper)

		// When: the init command is executed with vm-arch flag and context
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"init", "test-context", "--vm-arch", "test-arch"})
			err := rootCmd.Execute()
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then: the output should indicate the error
		expectedOutput := "error setting vm arch: set arch error"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("SetDockerConfigError", func(t *testing.T) {
		// Given: a config handler that returns an error on setting Docker config
		mockHandler := config.NewMockConfigHandler()
		mockHandler.SetConfigValueFunc = func(key string, value interface{}) error {
			if key == "contexts.test-context.docker.enabled" {
				return errors.New("set docker config error")
			}
			return nil
		}
		mockHandler.SaveConfigFunc = func(path string) error { return nil }
		mockHandler.GetConfigValueFunc = func(key string) (string, error) { return "value", nil }

		mockShell, err := shell.NewMockShell("cmd")
		if err != nil {
			t.Fatalf("NewMockShell() error = %v", err)
		}
		mockHelper := &helpers.MockHelper{}
		setupContainer(mockHandler, mockHandler, mockShell, mockHelper, mockHelper, mockHelper, mockHelper)

		// When: the init command is executed with the docker flag
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"init", "test-context", "--docker"})
			err := rootCmd.Execute()
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then: the output should indicate the error
		expectedOutput := "error setting Docker configuration: set docker config error"
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
}
