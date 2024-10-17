package cmd

import (
	"errors"
	"strings"
	"testing"

	"github.com/windsor-hotel/cli/internal/config"
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
		mockHelper := &helpers.MockHelper{
			SetConfigFunc: func(key, value string) error { return nil },
		}
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
		mockHelper := &helpers.MockHelper{
			SetConfigFunc: func(key, value string) error { return nil },
		}
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
		mockHelper := &helpers.MockHelper{
			SetConfigFunc: func(key, value string) error { return nil },
		}
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
		mockHelper := &helpers.MockHelper{
			SetConfigFunc: func(key, value string) error { return nil },
		}
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

	t.Run("TerraformHelperSetConfigError", func(t *testing.T) {
		// Given: a terraform helper that returns an error on SetConfig
		originalTerraformHelper := terraformHelper
		defer func() { terraformHelper = originalTerraformHelper }()
		terraformHelper = &helpers.MockHelper{
			SetConfigFunc: func(key, value string) error {
				if key == "backend" {
					return errors.New("set backend error")
				}
				return nil
			},
		}

		mockHandler := config.NewMockConfigHandler()
		mockShell, err := shell.NewMockShell("cmd")
		if err != nil {
			t.Fatalf("NewMockShell() error = %v", err)
		}
		setupContainer(mockHandler, mockHandler, mockShell, terraformHelper, nil, nil, dockerHelper)

		// When: the init command is executed with a backend flag
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"init", "test-context", "--backend", "test-backend"})
			err := rootCmd.Execute()
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then: the output should indicate the error
		expectedOutput := "Error setting backend value: set backend error"
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
		mockHelper := &helpers.MockHelper{
			SetConfigFunc: func(key, value string) error { return nil },
		}

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

	t.Run("SetAwsEndpointURLError", func(t *testing.T) {
		// Given: a config handler that returns an error on setting aws_endpoint_url
		mockHandler := config.NewMockConfigHandler()
		mockShell, err := shell.NewMockShell("cmd")
		if err != nil {
			t.Fatalf("NewMockShell() error = %v", err)
		}
		mockHelper := &helpers.MockHelper{
			SetConfigFunc: func(key, value string) error {
				if key == "aws_endpoint_url" {
					return errors.New("set aws_endpoint_url error")
				}
				return nil
			},
		}
		setupContainer(mockHandler, mockHandler, mockShell, mockHelper, mockHelper, nil, dockerHelper)

		// When: the init command is executed with aws-endpoint-url flag and context
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"init", "test-context", "--aws-endpoint-url", "http://example.com"})
			err := rootCmd.Execute()
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then: the output should indicate the error
		expectedOutput := "error setting AWS configuration: set aws_endpoint_url error"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("SetAwsProfileError", func(t *testing.T) {
		// Given: a config handler that returns an error on setting aws_profile
		mockHandler := config.NewMockConfigHandler()
		mockShell, err := shell.NewMockShell("cmd")
		if err != nil {
			t.Fatalf("NewMockShell() error = %v", err)
		}
		mockHelper := &helpers.MockHelper{
			SetConfigFunc: func(key, value string) error {
				if key == "aws_profile" {
					return errors.New("set aws_profile error")
				}
				return nil
			},
		}
		setupContainer(mockHandler, mockHandler, mockShell, mockHelper, mockHelper, nil, dockerHelper)

		// When: the init command is executed with aws-profile flag and context
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"init", "test-context", "--aws-profile", "test-profile"})
			err := rootCmd.Execute()
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then: the output should indicate the error
		expectedOutput := "error setting AWS configuration: set aws_profile error"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("SetColimaTypeError", func(t *testing.T) {
		// Given: a colima helper that returns an error on setting type
		mockHandler := config.NewMockConfigHandler()
		mockShell, err := shell.NewMockShell("cmd")
		if err != nil {
			t.Fatalf("NewMockShell() error = %v", err)
		}
		mockHelper := &helpers.MockHelper{
			SetConfigFunc: func(key, value string) error {
				if key == "driver" {
					return errors.New("set driver error")
				}
				return nil
			},
		}
		// Pass mockHelper as the Colima helper
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
		expectedOutput := "error setting Colima configuration: set driver error"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("SetColimaCpuError", func(t *testing.T) {
		// Given: a colima helper that returns an error on setting cpu
		mockHandler := config.NewMockConfigHandler()
		mockShell, err := shell.NewMockShell("cmd")
		if err != nil {
			t.Fatalf("NewMockShell() error = %v", err)
		}
		mockHelper := &helpers.MockHelper{
			SetConfigFunc: func(key, value string) error {
				if key == "cpu" {
					return errors.New("set cpu error")
				}
				return nil
			},
		}
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
		expectedOutput := "error setting Colima configuration: set cpu error"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("SetColimaDiskError", func(t *testing.T) {
		// Given: a colima helper that returns an error on setting disk
		mockHandler := config.NewMockConfigHandler()
		mockShell, err := shell.NewMockShell("cmd")
		if err != nil {
			t.Fatalf("NewMockShell() error = %v", err)
		}
		mockHelper := &helpers.MockHelper{
			SetConfigFunc: func(key, value string) error {
				if key == "disk" {
					return errors.New("set disk error")
				}
				return nil
			},
		}
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
		expectedOutput := "error setting Colima configuration: set disk error"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})
	t.Run("SetColimaMemoryError", func(t *testing.T) {
		// Given: a colima helper that returns an error on setting memory
		mockHandler := config.NewMockConfigHandler()
		mockShell, err := shell.NewMockShell("cmd")
		if err != nil {
			t.Fatalf("NewMockShell() error = %v", err)
		}
		mockHelper := &helpers.MockHelper{
			SetConfigFunc: func(key, value string) error {
				if key == "memory" {
					return errors.New("set memory error")
				}
				return nil
			},
		}
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
		expectedOutput := "error setting Colima configuration: set memory error"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("SetColimaArchError", func(t *testing.T) {
		// Given: a colima helper that returns an error on setting arch
		mockHandler := config.NewMockConfigHandler()
		mockShell, err := shell.NewMockShell("cmd")
		if err != nil {
			t.Fatalf("NewMockShell() error = %v", err)
		}
		mockHelper := &helpers.MockHelper{
			SetConfigFunc: func(key, value string) error {
				if key == "arch" {
					return errors.New("set arch error")
				}
				return nil
			},
		}
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
		expectedOutput := "error setting Colima configuration: set arch error"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("SetDockerConfigError", func(t *testing.T) {
		// Given: a docker helper that returns an error on setting config
		mockHandler := config.NewMockConfigHandler()
		mockHandler.SetConfigValueFunc = func(key string, value interface{}) error { return nil }
		mockHandler.SaveConfigFunc = func(path string) error { return nil }
		mockHandler.GetConfigValueFunc = func(key string) (string, error) { return "value", nil }

		mockShell, err := shell.NewMockShell("cmd")
		if err != nil {
			t.Fatalf("NewMockShell() error = %v", err)
		}
		mockHelper := &helpers.MockHelper{
			SetConfigFunc: func(key, value string) error {
				if key == "enabled" {
					return errors.New("set docker config error")
				}
				return nil
			},
		}
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

	t.Run("SetDockerConfigError", func(t *testing.T) {
		// Given: a docker helper that returns an error on setting config
		mockHandler := config.NewMockConfigHandler()
		mockHandler.SetConfigValueFunc = func(key string, value interface{}) error { return nil }
		mockHandler.SaveConfigFunc = func(path string) error { return nil }
		mockHandler.GetConfigValueFunc = func(key string) (string, error) { return "value", nil }

		mockShell, err := shell.NewMockShell("cmd")
		if err != nil {
			t.Fatalf("NewMockShell() error = %v", err)
		}
		mockHelper := &helpers.MockHelper{
			SetConfigFunc: func(key, value string) error {
				if key == "enabled" {
					return errors.New("set docker config error")
				}
				return nil
			},
		}
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
}
