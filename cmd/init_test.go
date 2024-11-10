package cmd

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/mocks"
)

func TestInitCmd(t *testing.T) {
	originalArgs := rootCmd.Args
	originalExitFunc := exitFunc
	originalContextInstance := contextHandler

	t.Cleanup(func() {
		rootCmd.Args = originalArgs
		exitFunc = originalExitFunc
		contextHandler = originalContextInstance
	})

	// Mock the exit function to prevent the test from exiting
	exitFunc = func(code int) {
		panic("exit called")
	}

	t.Run("Success", func(t *testing.T) {
		// Given: a valid config handler that returns a normal config object
		mocks := mocks.CreateSuperMocks()
		mocks.ContextInstance.GetConfigRootFunc = func() (string, error) {
			return "test-context", nil
		}
		// Mock the GetConfig function to ensure it is called with the desired object
		mocks.CLIConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Docker: &config.DockerConfig{
					Enabled: ptrBool(true),
				},
				VM: &config.VMConfig{
					Driver: ptrString("colima"),
				},
			}, nil
		}

		Initialize(mocks.Container)

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
		mocks := mocks.CreateSuperMocks()
		Initialize(mocks.Injector)

		// Mock the GetConfig function to ensure it is called with the desired object
		mocks.CLIConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{}, nil
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
	})

	t.Run("HomeDirError", func(t *testing.T) {
		// Mock configHandler
		mocks := mocks.CreateSuperMocks()
		Initialize(mocks.Injector)

		// Mock os.UserHomeDir to simulate an error
		originalUserHomeDir := osUserHomeDir
		osUserHomeDir = func() (string, error) {
			return "", fmt.Errorf("mocked error retrieving home directory")
		}
		defer func() { osUserHomeDir = originalUserHomeDir }()

		// Mock the exit function to prevent the test from exiting
		exitCalled := false
		exitFunc = func(code int) {
			exitCalled = true
		}

		rootCmd.SetArgs([]string{"init", "test-context"})
		err := rootCmd.Execute()

		// Check that the error is as expected
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		expectedError := "error retrieving home directory: mocked error retrieving home directory"
		if err.Error() != expectedError {
			t.Fatalf("Execute() error = %v, expected '%s'", err, expectedError)
		}

		// Check that exit was called
		if !exitCalled {
			t.Fatalf("Expected exit to be called, but it was not")
		}
	})
	t.Run("SetConfigValueError", func(t *testing.T) {
		// Given: a config handler that returns an error on SetConfigValue
		mocks := mocks.CreateSuperMocks()
		mocks.CLIConfigHandler.SetFunc = func(key string, value interface{}) error { return errors.New("set config value error") }
		Initialize(mocks.Injector)

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
		mocks := mocks.CreateSuperMocks()
		mocks.CLIConfigHandler.SaveConfigFunc = func(path string) error { return errors.New("save config error") }
		Initialize(mocks.Injector)

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
	t.Run("CLIConfigSaveError", func(t *testing.T) {
		// Given: a config handler that returns an error on SaveConfig
		mocks := mocks.CreateSuperMocks()
		mocks.CLIConfigHandler.SaveConfigFunc = func(path string) error { return errors.New("save cli config error") }
		Initialize(mocks.Injector)

		// Replace the global contextHandler with the mock
		originalContextInstance := contextHandler
		defer func() { contextHandler = originalContextInstance }()

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

	t.Run("LocalContextSetsDefault", func(t *testing.T) {
		// Arrange: Create a mock config handler and set the SetDefaultFunc to check the parameters
		mocks := mocks.CreateSuperMocks()
		mocks.CLIConfigHandler.SetDefaultFunc = func(context config.Context) error {
			expectedValue := config.DefaultLocalConfig
			if !reflect.DeepEqual(context, expectedValue) {
				return fmt.Errorf("Expected value %v, got %v", expectedValue, context)
			}
			return nil
		}
		Initialize(mocks.Injector)

		// Act: Call the init command with a local context
		output := captureStdout(func() {
			rootCmd.SetArgs([]string{"init", "local"})
			err := rootCmd.Execute()
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
		})

		// Assert: Verify the output indicates success
		expectedOutput := "Initialization successful\n"
		if output != expectedOutput {
			t.Errorf("Expected output %q, got %q", expectedOutput, output)
		}
	})

	t.Run("AWSConfiguration", func(t *testing.T) {
		mocks := mocks.CreateSuperMocks()
		calledKeys := make(map[string]bool)
		mocks.CLIConfigHandler.SetFunc = func(key string, value interface{}) error {
			calledKeys[key] = true
			return nil
		}
		Initialize(mocks.Injector)

		rootCmd.SetArgs([]string{
			"init", "test-context",
			"--aws-endpoint-url", "http://localhost:4566",
			"--aws-profile", "test-profile",
		})
		err := rootCmd.Execute()
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		expectedKeys := map[string]bool{
			"contexts.test-context.aws.aws_endpoint_url": true,
			"contexts.test-context.aws.aws_profile":      true,
		}
		for key := range expectedKeys {
			if !calledKeys[key] {
				t.Errorf("Expected key %q to be set", key)
			}
		}
	})

	t.Run("DockerConfiguration", func(t *testing.T) {
		mocks := mocks.CreateSuperMocks()
		calledKeys := make(map[string]bool)
		mocks.CLIConfigHandler.SetFunc = func(key string, value interface{}) error {
			calledKeys[key] = true
			return nil
		}
		Initialize(mocks.Injector)

		rootCmd.SetArgs([]string{
			"init", "test-context",
			"--docker",
		})
		err := rootCmd.Execute()
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		expectedKeys := map[string]bool{
			"contexts.test-context.docker.enabled": true,
		}
		for key := range expectedKeys {
			if !calledKeys[key] {
				t.Errorf("Expected key %q to be set", key)
			}
		}
	})

	t.Run("GitLivereloadConfiguration", func(t *testing.T) {
		mocks := mocks.CreateSuperMocks()
		calledKeys := make(map[string]bool)
		mocks.CLIConfigHandler.SetFunc = func(key string, value interface{}) error {
			calledKeys[key] = true
			return nil
		}
		Initialize(mocks.Injector)

		rootCmd.SetArgs([]string{
			"init", "test-context",
			"--git-livereload",
		})
		err := rootCmd.Execute()
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		expectedKeys := map[string]bool{
			"contexts.test-context.git.livereload.enabled": true,
		}
		for key := range expectedKeys {
			if !calledKeys[key] {
				t.Errorf("Expected key %q to be set", key)
			}
		}
	})

	t.Run("GitLivereloadConfigurationError", func(t *testing.T) {
		mocks := mocks.CreateSuperMocks()
		calledKeys := make(map[string]bool)
		mocks.CLIConfigHandler.SetFunc = func(key string, value interface{}) error {
			calledKeys[key] = true
			if key == "contexts.test-context.git.livereload.enabled" {
				return fmt.Errorf("mock set error")
			}
			return nil
		}
		Initialize(mocks.Injector)

		rootCmd.SetArgs([]string{
			"init", "test-context",
			"--git-livereload",
		})
		err := rootCmd.Execute()
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		expectedError := "mock set error"
		if !strings.Contains(err.Error(), expectedError) {
			t.Errorf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("TerraformConfiguration", func(t *testing.T) {
		mocks := mocks.CreateSuperMocks()
		calledKeys := make(map[string]bool)
		mocks.CLIConfigHandler.SetFunc = func(key string, value interface{}) error {
			calledKeys[key] = true
			return nil
		}
		Initialize(mocks.Injector)

		rootCmd.SetArgs([]string{
			"init", "test-context",
			"--backend", "s3",
		})
		err := rootCmd.Execute()
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		expectedKeys := map[string]bool{
			"contexts.test-context.terraform.backend": true,
		}
		for key := range expectedKeys {
			if !calledKeys[key] {
				t.Errorf("Expected key %q to be set", key)
			}
		}
	})
	t.Run("VMConfiguration", func(t *testing.T) {
		mocks := mocks.CreateSuperMocks()
		calledKeys := make(map[string]bool)
		mocks.CLIConfigHandler.SetFunc = func(key string, value interface{}) error {
			calledKeys[key] = true
			return nil
		}
		Initialize(mocks.Injector)

		rootCmd.SetArgs([]string{
			"init", "test-context",
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

		expectedKeys := map[string]bool{
			"contexts.test-context.vm.driver": true,
			"contexts.test-context.vm.cpu":    true,
			"contexts.test-context.vm.disk":   true,
			"contexts.test-context.vm.memory": true,
			"contexts.test-context.vm.arch":   true,
		}
		for key := range expectedKeys {
			if !calledKeys[key] {
				t.Errorf("Expected key %q to be set", key)
			}
		}
	})

	t.Run("ErrorSettingAWSEndpointURL", func(t *testing.T) {
		mocks := mocks.CreateSuperMocks()
		mocks.CLIConfigHandler.SetFunc = func(key string, value interface{}) error {
			if key == "contexts.test-context.aws.aws_endpoint_url" {
				return errors.New("error setting AWS endpoint URL")
			}
			return nil
		}
		Initialize(mocks.Injector)

		rootCmd.SetArgs([]string{
			"init", "test-context",
			"--aws-endpoint-url", "http://localhost:4566",
		})
		err := rootCmd.Execute()
		if err == nil || err.Error() != "Error setting AWS endpoint URL: error setting AWS endpoint URL" {
			t.Fatalf("Expected error setting AWS endpoint URL, got %v", err)
		}
	})

	t.Run("ErrorSettingAWSProfile", func(t *testing.T) {
		mocks := mocks.CreateSuperMocks()
		mocks.CLIConfigHandler.SetFunc = func(key string, value interface{}) error {
			if key == "contexts.test-context.aws.aws_profile" {
				return errors.New("error setting AWS profile")
			}
			return nil
		}
		Initialize(mocks.Injector)

		rootCmd.SetArgs([]string{
			"init", "test-context",
			"--aws-profile", "default",
		})
		err := rootCmd.Execute()
		if err == nil || err.Error() != "Error setting AWS profile: error setting AWS profile" {
			t.Fatalf("Expected error setting AWS profile, got %v", err)
		}
	})

	t.Run("ErrorSettingDockerConfiguration", func(t *testing.T) {
		mocks := mocks.CreateSuperMocks()
		mocks.CLIConfigHandler.SetFunc = func(key string, value interface{}) error {
			if key == "contexts.test-context.docker.enabled" {
				return errors.New("error setting Docker enabled")
			}
			return nil
		}
		Initialize(mocks.Injector)

		rootCmd.SetArgs([]string{
			"init", "test-context",
			"--docker",
		})
		err := rootCmd.Execute()
		if err == nil || err.Error() != "Error setting Docker enabled: error setting Docker enabled" {
			t.Fatalf("Expected error setting Docker enabled, got %v", err)
		}
	})

	t.Run("ErrorSettingTerraformConfiguration", func(t *testing.T) {
		mocks := mocks.CreateSuperMocks()
		mocks.CLIConfigHandler.SetFunc = func(key string, value interface{}) error {
			if key == "contexts.test-context.terraform.backend" {
				return errors.New("error setting Terraform backend")
			}
			return nil
		}
		Initialize(mocks.Injector)

		rootCmd.SetArgs([]string{
			"init", "test-context",
			"--backend", "s3",
		})
		err := rootCmd.Execute()
		if err == nil || err.Error() != "Error setting Terraform backend: error setting Terraform backend" {
			t.Fatalf("Expected error setting Terraform backend, got %v", err)
		}
	})

	t.Run("ErrorSettingVMConfigurationArch", func(t *testing.T) {
		mocks := mocks.CreateSuperMocks()
		mocks.CLIConfigHandler.SetFunc = func(key string, value interface{}) error {
			if key == "contexts.test-context.vm.arch" {
				return errors.New("error setting VM architecture")
			}
			return nil
		}
		Initialize(mocks.Injector)

		rootCmd.SetArgs([]string{
			"init", "test-context",
			"--vm-arch", "x86_64",
		})
		err := rootCmd.Execute()
		if err == nil || err.Error() != "Error setting VM architecture: error setting VM architecture" {
			t.Fatalf("Expected error setting VM architecture, got %v", err)
		}
	})

	t.Run("ErrorSettingVMConfigurationDriver", func(t *testing.T) {
		mocks := mocks.CreateSuperMocks()
		mocks.CLIConfigHandler.SetFunc = func(key string, value interface{}) error {
			if key == "contexts.test-context.vm.driver" {
				return errors.New("error setting VM driver")
			}
			return nil
		}
		Initialize(mocks.Injector)

		rootCmd.SetArgs([]string{
			"init", "test-context",
			"--vm-driver", "colima",
		})
		err := rootCmd.Execute()
		if err == nil || err.Error() != "Error setting VM driver: error setting VM driver" {
			t.Fatalf("Expected error setting VM driver, got %v", err)
		}
	})
	t.Run("ErrorSettingVMConfigurationCPU", func(t *testing.T) {
		mocks := mocks.CreateSuperMocks()
		mocks.CLIConfigHandler.SetFunc = func(key string, value interface{}) error {
			if key == "contexts.test-context.vm.cpu" {
				return errors.New("error setting VM CPU")
			}
			return nil
		}
		Initialize(mocks.Injector)

		rootCmd.SetArgs([]string{
			"init", "test-context",
			"--vm-cpu", "2",
		})
		err := rootCmd.Execute()
		if err == nil || err.Error() != "Error setting VM CPU: error setting VM CPU" {
			t.Fatalf("Expected error setting VM CPU, got %v", err)
		}
	})

	t.Run("ErrorSettingVMConfigurationDisk", func(t *testing.T) {
		mocks := mocks.CreateSuperMocks()
		mocks.CLIConfigHandler.SetFunc = func(key string, value interface{}) error {
			if key == "contexts.test-context.vm.disk" {
				return errors.New("error setting VM disk")
			}
			return nil
		}
		Initialize(mocks.Injector)

		rootCmd.SetArgs([]string{
			"init", "test-context",
			"--vm-disk", "20",
		})
		err := rootCmd.Execute()
		if err == nil || err.Error() != "Error setting VM disk: error setting VM disk" {
			t.Fatalf("Expected error setting VM disk, got %v", err)
		}
	})

	t.Run("ErrorSettingVMConfigurationMemory", func(t *testing.T) {
		mocks := mocks.CreateSuperMocks()
		mocks.CLIConfigHandler.SetFunc = func(key string, value interface{}) error {
			if key == "contexts.test-context.vm.memory" {
				return errors.New("error setting VM memory")
			}
			return nil
		}
		Initialize(mocks.Injector)

		rootCmd.SetArgs([]string{
			"init", "test-context",
			"--vm-memory", "4096",
		})
		err := rootCmd.Execute()
		if err == nil || err.Error() != "Error setting VM memory: error setting VM memory" {
			t.Fatalf("Expected error setting VM memory, got %v", err)
		}
	})

	t.Run("SetDefaultLocalConfigError", func(t *testing.T) {
		// Given: a config handler that returns an error on SetDefault for local config
		mocks := mocks.CreateSuperMocks()
		mocks.CLIConfigHandler.SetDefaultFunc = func(context config.Context) error {
			expectedValue := config.DefaultLocalConfig
			if !reflect.DeepEqual(context, expectedValue) {
				return fmt.Errorf("Expected value %v, got %v", expectedValue, context)
			}
			return errors.New("error setting default local config")
		}
		Initialize(mocks.Injector)

		// When: the init command is executed with a local context
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"init", "local"})
			err := rootCmd.Execute()
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then: the output should indicate the error
		expectedOutput := "Error setting default local config: error setting default local config"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("SetDefaultConfigError", func(t *testing.T) {
		// Given: a config handler that returns an error on SetDefault for default config
		mocks := mocks.CreateSuperMocks()
		mocks.CLIConfigHandler.SetDefaultFunc = func(context config.Context) error {
			if reflect.DeepEqual(context, config.DefaultConfig) {
				return errors.New("error setting default config")
			}
			return nil
		}
		Initialize(mocks.Injector)

		// When: the init command is executed with a non-local context
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"init", "test-context"})
			err := rootCmd.Execute()
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then: the output should indicate the error
		expectedOutput := "Error setting default config: error setting default config"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("ErrorGettingConfig", func(t *testing.T) {
		// Given: a config handler that returns an error on GetConfig
		mocks := mocks.CreateSuperMocks()
		mocks.CLIConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return nil, errors.New("error getting config")
		}
		// Mock ColimaVirt to return an error on WriteConfig
		mocks.ColimaVirt.WriteConfigFunc = func() error {
			return errors.New("error writing Colima config")
		}
		Initialize(mocks.Injector)

		// When: the init command is executed
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"init", "test-context"})
			err := rootCmd.Execute()
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then: the output should indicate the error
		expectedOutput := "error retrieving context configuration: error getting config"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("ErrorWritingColimaConfig", func(t *testing.T) {
		// Given: a config handler that returns a valid config with Colima driver
		mocks := mocks.CreateSuperMocks()
		mocks.CLIConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				VM: &config.VMConfig{
					Driver: ptrString("colima"),
				},
			}, nil
		}
		// Mock ColimaVirt to return an error on WriteConfig
		mocks.ColimaVirt.WriteConfigFunc = func() error {
			return errors.New("error writing Colima config")
		}
		Initialize(mocks.Container)

		// When: the init command is executed
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"init", "test-context"})
			err := rootCmd.Execute()
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then: the output should indicate the error
		expectedOutput := "error writing Colima config: error writing Colima config"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("ErrorWritingDockerConfig", func(t *testing.T) {
		// Given: a config handler that returns a valid config with Docker enabled
		mocks := mocks.CreateSuperMocks()
		mocks.CLIConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Docker: &config.DockerConfig{
					Enabled: ptrBool(true),
				},
			}, nil
		}
		// Mock DockerVirt to return an error on WriteConfig
		mocks.DockerVirt.WriteConfigFunc = func() error {
			return errors.New("error writing Docker config")
		}
		Initialize(mocks.Container)

		// When: the init command is executed
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"init", "test-context"})
			err := rootCmd.Execute()
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then: the output should indicate the error
		expectedOutput := "error writing Docker config: error writing Docker config"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})
}
