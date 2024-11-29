package cmd

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/mocks"
)

func TestInitCmd(t *testing.T) {
	originalArgs := rootCmd.Args
	originalExitFunc := exitFunc

	t.Cleanup(func() {
		rootCmd.Args = originalArgs
		exitFunc = originalExitFunc
	})

	// Mock the exit function to prevent the test from exiting
	exitFunc = func(code int) {
		panic("exit called")
	}

	t.Run("Success", func(t *testing.T) {
		// Given: a valid config handler that returns a normal config object
		mocks := mocks.CreateSuperMocks()
		mocks.ContextHandler.GetConfigRootFunc = func() (string, error) {
			return "test-context", nil
		}
		// Mock the GetConfig function to ensure it is called with the desired object
		mocks.ConfigHandler.GetConfigFunc = func() *config.Context {
			return &config.Context{
				Docker: &config.DockerConfig{
					Enabled: ptrBool(true),
				},
				VM: &config.VMConfig{
					Driver: ptrString("colima"),
				},
			}
		}

		// When: the init command is executed with a valid context
		output := captureStdout(func() {
			rootCmd.SetArgs([]string{"init", "test-context"})
			err := Execute(mocks.Injector)
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

		// Mock the GetConfig function to ensure it is called with the desired object
		mocks.ConfigHandler.GetConfigFunc = func() *config.Context {
			return &config.Context{}
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
			err := Execute(mocks.Injector)
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

	t.Run("GetContextHandlerError", func(t *testing.T) {
		// Given: a valid config handler
		mocks := mocks.CreateSuperMocks()

		// Override getContextHandler to return an error
		originalGetContextHandler := getContextHandler
		getContextHandler = func() (context.ContextHandler, error) {
			return nil, fmt.Errorf("mocked error getting context")
		}
		defer func() { getContextHandler = originalGetContextHandler }()

		// When: the init command is executed
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"init", "test-context"})
			err := Execute(mocks.Injector)
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then: the output should indicate the mocked error
		expectedError := "Error getting context handler: mocked error getting context"
		if !strings.Contains(output, expectedError) {
			t.Errorf("Expected error %q, got %q", expectedError, output)
		}
	})

	t.Run("NoContextProvided", func(t *testing.T) {
		// Given: a valid config handler
		mocks := mocks.CreateSuperMocks()

		// When: the init command is executed without a context
		output := captureStdout(func() {
			rootCmd.SetArgs([]string{"init"})
			err := Execute(mocks.Injector)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
		})

		// Then: the output should indicate success
		expectedOutput := "Initialization successful\n"
		if output != expectedOutput {
			t.Errorf("Expected output %q, got %q", expectedOutput, output)
		}
	})

	t.Run("ContextNameProvided", func(t *testing.T) {
		// Given: a valid config handler
		mocks := mocks.CreateSuperMocks()

		// When: the init command is executed with a context name provided
		output := captureStdout(func() {
			rootCmd.SetArgs([]string{"init", "test-context"})
			err := Execute(mocks.Injector)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
		})

		// Then: the output should indicate success
		expectedOutput := "Initialization successful\n"
		if output != expectedOutput {
			t.Errorf("Expected output %q, got %q", expectedOutput, output)
		}
	})

	t.Run("GetContextError", func(t *testing.T) {
		// Given: a valid config handler
		mocks := mocks.CreateSuperMocks()

		mocks.ContextHandler.GetContextFunc = func() (string, error) {
			return "", fmt.Errorf("mocked error getting context")
		}

		// When: the init command is executed
		rootCmd.SetArgs([]string{"init"})
		err := Execute(mocks.Injector)
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		// Then: the error should be present
		expectedError := "mocked error getting context"
		if !strings.Contains(err.Error(), expectedError) {
			t.Errorf("Expected error to contain %q, but got %q", expectedError, err.Error())
		}
	})

	t.Run("SetContextError", func(t *testing.T) {
		// Given: a valid config handler
		mocks := mocks.CreateSuperMocks()

		// Mock SetContext to return an error
		mocks.ContextHandler.SetContextFunc = func(contextName string) error {
			return fmt.Errorf("mocked error setting context")
		}

		// When: the init command is executed
		rootCmd.SetArgs([]string{"init", "test-context"})
		err := Execute(mocks.Injector)
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		// Then: the error should be present
		expectedError := "mocked error setting context"
		if !strings.Contains(err.Error(), expectedError) {
			t.Errorf("Expected error to contain %q, but got %q", expectedError, err.Error())
		}
	})

	t.Run("HomeDirError", func(t *testing.T) {
		// Given: a mocked error when retrieving the user home directory
		mocks := mocks.CreateSuperMocks()

		// Mock osUserHomeDir to simulate an error
		originalUserHomeDir := osUserHomeDir
		osUserHomeDir = func() (string, error) {
			return "", fmt.Errorf("mocked error retrieving home directory")
		}
		defer func() { osUserHomeDir = originalUserHomeDir }()

		// When: the init command is executed
		rootCmd.SetArgs([]string{"init", "test-context"})
		err := Execute(mocks.Injector)

		// Then: check for presence of error using contains
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		expectedError := "mocked error retrieving home directory"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %v", expectedError, err)
		}
	})

	t.Run("SetConfigValueError", func(t *testing.T) {
		// Given: a config handler that returns an error on SetConfigValue
		mocks := mocks.CreateSuperMocks()
		mocks.ConfigHandler.SetFunc = func(key string, value interface{}) error { return errors.New("set config value error") }

		// When: the init command is executed
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"init", "test-context"})
			err := Execute(mocks.Injector)
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
		mocks.ConfigHandler.SaveConfigFunc = func(path string) error { return errors.New("save config error") }

		// When: the init command is executed
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"init", "test-context"})
			err := Execute(mocks.Injector)
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
		mocks.ConfigHandler.SaveConfigFunc = func(path string) error { return errors.New("save cli config error") }

		// When: the init command is executed
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"init", "test-context"})
			err := Execute(mocks.Injector)
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
		mocks.ConfigHandler.SetDefaultFunc = func(context config.Context) error {
			expectedValue := config.DefaultLocalConfig
			if !reflect.DeepEqual(context, expectedValue) {
				return fmt.Errorf("Expected value %v, got %v", expectedValue, context)
			}
			return nil
		}

		// Act: Call the init command with a local context
		output := captureStdout(func() {
			rootCmd.SetArgs([]string{"init", "local"})
			err := Execute(mocks.Injector)
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
		mocks.ConfigHandler.SetFunc = func(key string, value interface{}) error {
			calledKeys[key] = true
			return nil
		}

		rootCmd.SetArgs([]string{
			"init", "test-context",
			"--aws-endpoint-url", "http://localhost:4566",
			"--aws-profile", "test-profile",
		})
		err := Execute(mocks.Injector)
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		expectedKeys := map[string]bool{
			"aws.aws_endpoint_url": true,
			"aws.aws_profile":      true,
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
		mocks.ConfigHandler.SetFunc = func(key string, value interface{}) error {
			calledKeys[key] = true
			return nil
		}
		rootCmd.SetArgs([]string{
			"init", "test-context",
			"--docker",
		})
		err := Execute(mocks.Injector)
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		expectedKeys := map[string]bool{
			"docker.enabled": true,
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
		mocks.ConfigHandler.SetFunc = func(key string, value interface{}) error {
			calledKeys[key] = true
			return nil
		}

		rootCmd.SetArgs([]string{
			"init", "test-context",
			"--git-livereload",
		})
		err := Execute(mocks.Injector)
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		expectedKeys := map[string]bool{
			"git.livereload.enabled": true,
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
		mocks.ConfigHandler.SetFunc = func(key string, value interface{}) error {
			calledKeys[key] = true
			if key == "git.livereload.enabled" {
				return fmt.Errorf("mock set error")
			}
			return nil
		}

		rootCmd.SetArgs([]string{
			"init", "test-context",
			"--git-livereload",
		})
		err := Execute(mocks.Injector)
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
		mocks.ConfigHandler.SetFunc = func(key string, value interface{}) error {
			calledKeys[key] = true
			return nil
		}

		rootCmd.SetArgs([]string{
			"init", "test-context",
			"--backend", "s3",
		})
		err := Execute(mocks.Injector)
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		expectedKeys := map[string]bool{
			"terraform.backend": true,
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
		mocks.ConfigHandler.SetFunc = func(key string, value interface{}) error {
			calledKeys[key] = true
			return nil
		}

		rootCmd.SetArgs([]string{
			"init", "test-context",
			"--vm-driver", "colima",
			"--vm-cpu", "2",
			"--vm-disk", "20",
			"--vm-memory", "4096",
			"--vm-arch", "x86_64",
		})
		err := Execute(mocks.Injector)
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		expectedKeys := map[string]bool{
			"vm.driver": true,
			"vm.cpu":    true,
			"vm.disk":   true,
			"vm.memory": true,
			"vm.arch":   true,
		}
		for key := range expectedKeys {
			if !calledKeys[key] {
				t.Errorf("Expected key %q to be set", key)
			}
		}
	})

	t.Run("ErrorSettingAWSEndpointURL", func(t *testing.T) {
		mocks := mocks.CreateSuperMocks()
		mocks.ConfigHandler.SetFunc = func(key string, value interface{}) error {
			if key == "aws.aws_endpoint_url" {
				return errors.New("error setting AWS endpoint URL")
			}
			return nil
		}

		rootCmd.SetArgs([]string{
			"init", "test-context",
			"--aws-endpoint-url", "http://localhost:4566",
		})
		err := Execute(mocks.Injector)
		if err == nil || err.Error() != "Error setting AWS endpoint URL: error setting AWS endpoint URL" {
			t.Fatalf("Expected error setting AWS endpoint URL, got %v", err)
		}
	})

	t.Run("ErrorSettingAWSProfile", func(t *testing.T) {
		mocks := mocks.CreateSuperMocks()
		mocks.ConfigHandler.SetFunc = func(key string, value interface{}) error {
			if key == "aws.aws_profile" {
				return errors.New("error setting AWS profile")
			}
			return nil
		}

		rootCmd.SetArgs([]string{
			"init", "test-context",
			"--aws-profile", "default",
		})
		err := Execute(mocks.Injector)
		if err == nil || err.Error() != "Error setting AWS profile: error setting AWS profile" {
			t.Fatalf("Expected error setting AWS profile, got %v", err)
		}
	})

	t.Run("ErrorSettingDockerConfiguration", func(t *testing.T) {
		mocks := mocks.CreateSuperMocks()
		mocks.ConfigHandler.SetFunc = func(key string, value interface{}) error {
			if key == "docker.enabled" {
				return errors.New("error setting Docker enabled")
			}
			return nil
		}
		rootCmd.SetArgs([]string{
			"init", "test-context",
			"--docker",
		})
		err := Execute(mocks.Injector)
		if err == nil || err.Error() != "Error setting Docker enabled: error setting Docker enabled" {
			t.Fatalf("Expected error setting Docker enabled, got %v", err)
		}
	})

	t.Run("ErrorSettingTerraformConfiguration", func(t *testing.T) {
		mocks := mocks.CreateSuperMocks()
		mocks.ConfigHandler.SetFunc = func(key string, value interface{}) error {
			if key == "terraform.backend" {
				return errors.New("error setting Terraform backend")
			}
			return nil
		}

		rootCmd.SetArgs([]string{
			"init", "test-context",
			"--backend", "s3",
		})
		err := Execute(mocks.Injector)
		if err == nil || err.Error() != "Error setting Terraform backend: error setting Terraform backend" {
			t.Fatalf("Expected error setting Terraform backend, got %v", err)
		}
	})

	t.Run("ErrorSettingVMConfigurationArch", func(t *testing.T) {
		mocks := mocks.CreateSuperMocks()
		mocks.ConfigHandler.SetFunc = func(key string, value interface{}) error {
			if key == "vm.arch" {
				return errors.New("error setting VM architecture")
			}
			return nil
		}

		rootCmd.SetArgs([]string{
			"init", "test-context",
			"--vm-arch", "x86_64",
		})
		err := Execute(mocks.Injector)
		if err == nil || err.Error() != "Error setting VM architecture: error setting VM architecture" {
			t.Fatalf("Expected error setting VM architecture, got %v", err)
		}
	})

	t.Run("ErrorSettingVMConfigurationDriver", func(t *testing.T) {
		mocks := mocks.CreateSuperMocks()
		mocks.ConfigHandler.SetFunc = func(key string, value interface{}) error {
			if key == "vm.driver" {
				return errors.New("error setting VM driver")
			}
			return nil
		}

		rootCmd.SetArgs([]string{
			"init", "test-context",
			"--vm-driver", "colima",
		})
		err := Execute(mocks.Injector)
		if err == nil || err.Error() != "Error setting VM driver: error setting VM driver" {
			t.Fatalf("Expected error setting VM driver, got %v", err)
		}
	})
	t.Run("ErrorSettingVMConfigurationCPU", func(t *testing.T) {
		mocks := mocks.CreateSuperMocks()
		mocks.ConfigHandler.SetFunc = func(key string, value interface{}) error {
			if key == "vm.cpu" {
				return errors.New("error setting VM CPU")
			}
			return nil
		}

		rootCmd.SetArgs([]string{
			"init", "test-context",
			"--vm-cpu", "2",
		})
		err := Execute(mocks.Injector)
		if err == nil || err.Error() != "Error setting VM CPU: error setting VM CPU" {
			t.Fatalf("Expected error setting VM CPU, got %v", err)
		}
	})

	t.Run("ErrorSettingVMConfigurationDisk", func(t *testing.T) {
		mocks := mocks.CreateSuperMocks()
		mocks.ConfigHandler.SetFunc = func(key string, value interface{}) error {
			if key == "vm.disk" {
				return errors.New("error setting VM disk")
			}
			return nil
		}

		rootCmd.SetArgs([]string{
			"init", "test-context",
			"--vm-disk", "20",
		})
		err := Execute(mocks.Injector)
		if err == nil || err.Error() != "Error setting VM disk: error setting VM disk" {
			t.Fatalf("Expected error setting VM disk, got %v", err)
		}
	})

	t.Run("ErrorSettingVMConfigurationMemory", func(t *testing.T) {
		mocks := mocks.CreateSuperMocks()
		mocks.ConfigHandler.SetFunc = func(key string, value interface{}) error {
			if key == "vm.memory" {
				return errors.New("error setting VM memory")
			}
			return nil
		}

		rootCmd.SetArgs([]string{
			"init", "test-context",
			"--vm-memory", "4096",
		})
		err := Execute(mocks.Injector)
		if err == nil || err.Error() != "Error setting VM memory: error setting VM memory" {
			t.Fatalf("Expected error setting VM memory, got %v", err)
		}
	})

	t.Run("SetDefaultLocalConfigError", func(t *testing.T) {
		// Given: a config handler that returns an error on SetDefault for local config
		mocks := mocks.CreateSuperMocks()
		mocks.ConfigHandler.SetDefaultFunc = func(context config.Context) error {
			expectedValue := config.DefaultLocalConfig
			if !reflect.DeepEqual(context, expectedValue) {
				return fmt.Errorf("Expected value %v, got %v", expectedValue, context)
			}
			return errors.New("error setting default local config")
		}

		// When: the init command is executed with a local context
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"init", "local"})
			err := Execute(mocks.Injector)
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
		mocks.ConfigHandler.SetDefaultFunc = func(context config.Context) error {
			if reflect.DeepEqual(context, config.DefaultConfig) {
				return errors.New("error setting default config")
			}
			return nil
		}

		// When: the init command is executed with a non-local context
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"init", "test-context"})
			err := Execute(mocks.Injector)
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

	t.Run("ErrorResolvingColimaVirt", func(t *testing.T) {
		mockInjector := di.NewMockInjector()

		// Given: an injector that returns an error when resolving colimaVirt
		mocks := mocks.CreateSuperMocks(mockInjector)
		mockInjector.SetResolveError("colimaVirt", errors.New("error resolving colimaVirt"))

		// Mock the configHandler to return "colima" for vm.driver
		mocks.ConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "vm.driver" {
				return "colima"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		// When: the init command is executed with a context that enables Colima
		rootCmd.SetArgs([]string{"init", "test-context", "--vm-driver", "colima"})
		err := Execute(mocks.Injector)
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		// Then: the error message should indicate the error
		expectedError := "Error resolving colimaVirt: error resolving colimaVirt"
		if !strings.Contains(err.Error(), expectedError) {
			t.Errorf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorCastingColimaVirt", func(t *testing.T) {
		// Given: an injector that returns an invalid type for colimaVirt
		mocks := mocks.CreateSuperMocks()
		mocks.Injector.Register("colimaVirt", "invalid")

		// Mock the configHandler to return "colima" for vm.driver
		mocks.ConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "vm.driver" {
				return "colima"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		// When: the init command is executed with a context that enables Colima
		rootCmd.SetArgs([]string{"init", "test-context", "--vm-driver", "colima"})
		err := Execute(mocks.Injector)
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		// Then: the error message should indicate the error
		expectedError := "Resolved instance is not of type virt.VirtualMachine"
		if !strings.Contains(err.Error(), expectedError) {
			t.Errorf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorInitializingColima", func(t *testing.T) {
		// Given: an injector that returns a valid colimaVirt but initialization fails
		mocks := mocks.CreateSuperMocks()
		colimaVirtMock := mocks.ColimaVirt
		colimaVirtMock.InitializeFunc = func() error {
			return fmt.Errorf("initialization failed")
		}
		mocks.Injector.Register("colimaVirt", colimaVirtMock)

		// Mock the configHandler to return "colima" for vm.driver
		mocks.ConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "vm.driver" {
				return "colima"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		// When: the init command is executed with a context that enables Colima
		rootCmd.SetArgs([]string{"init", "test-context", "--vm-driver", "colima"})
		err := Execute(mocks.Injector)
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		// Then: the error message should indicate the error
		expectedError := "Error initializing colimaVirt: initialization failed"
		if !strings.Contains(err.Error(), expectedError) {
			t.Errorf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorResolvingDockerVirt", func(t *testing.T) {
		mockInjector := di.NewMockInjector()

		// Given: an injector that returns an error when resolving dockerVirt
		mocks := mocks.CreateSuperMocks(mockInjector)
		mockInjector.SetResolveError("dockerVirt", errors.New("error resolving dockerVirt"))

		// When: the init command is executed
		rootCmd.SetArgs([]string{"init", "test-context"})
		err := Execute(mocks.Injector)
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		// Then: the error message should indicate the error
		expectedError := "Error resolving dockerVirt: error resolving dockerVirt"
		if !strings.Contains(err.Error(), expectedError) {
			t.Errorf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorCastingDockerVirt", func(t *testing.T) {
		mocks := mocks.CreateSuperMocks()
		mocks.Injector.Register("dockerVirt", "invalid")

		// When: the init command is executed
		rootCmd.SetArgs([]string{"init", "test-context"})
		err := Execute(mocks.Injector)
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		// Then: the error message should indicate the error
		expectedError := "Resolved instance is not of type virt.ContainerRuntime"
		if !strings.Contains(err.Error(), expectedError) {
			t.Errorf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorInitializingDocker", func(t *testing.T) {
		// Given: a config handler that returns a valid config with Docker enabled
		mocks := mocks.CreateSuperMocks()
		mocks.ConfigHandler.GetConfigFunc = func() *config.Context {
			return &config.Context{
				Docker: &config.DockerConfig{
					Enabled: ptrBool(true),
				},
			}
		}
		// Mock DockerVirt to return an error on Initialize
		mocks.DockerVirt.InitializeFunc = func() error {
			return errors.New("error initializing Docker")
		}

		// When: the init command is executed
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"init", "test-context"})
			err := Execute(mocks.Injector)
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then: the output should indicate the error
		expectedOutput := "Error: Error initializing dockerVirt: error initializing Docker"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})
}
