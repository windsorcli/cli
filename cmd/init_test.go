package cmd

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	ctrl "github.com/windsor-hotel/cli/internal/controller"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/services"
	"github.com/windsor-hotel/cli/internal/virt"
)

// setupSafeInitCmdMocks sets up mock objects for the init command
func setupSafeInitCmdMocks() *MockObjects {
	injector := di.NewInjector()
	mockController := ctrl.NewMockController(injector)

	mockConfigHandler := &config.MockConfigHandler{}
	mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
		switch key {
		case "vm.driver":
			return "colima"
		default:
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}
	}
	mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
		switch key {
		case "docker.enabled":
			return true
		default:
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return false
		}
	}
	mockController.ResolveConfigHandlerFunc = func() (config.ConfigHandler, error) {
		return mockConfigHandler, nil
	}

	mockContext := context.NewMockContext()
	mockController.ResolveContextHandlerFunc = func() (context.ContextHandler, error) {
		return mockContext, nil
	}

	return &MockObjects{
		Controller:     mockController,
		ConfigHandler:  mockConfigHandler,
		ContextHandler: mockContext,
	}
}

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
		mocks := setupSafeInitCmdMocks()

		// Execute the init command and capture output
		output := captureStdout(func() {
			rootCmd.SetArgs([]string{"init", "test-context"})
			if err := Execute(mocks.Controller); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
		})

		// Validate the output
		expectedOutput := "Initialization successful\n"
		if output != expectedOutput {
			t.Errorf("Expected output %q, got %q", expectedOutput, output)
		}
	})

	t.Run("AllFlagsSet", func(t *testing.T) {
		// Given a valid config handler
		mocks := setupSafeInitCmdMocks()

		// When the init command is executed with all flags set
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
			err := Execute(mocks.Controller)
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
		})

		// Then the output should indicate success
		expectedOutput := "Initialization successful\n"
		if output != expectedOutput {
			t.Errorf("Expected output %q, got %q", expectedOutput, output)
		}
	})

	t.Run("ErrorInitializingController", func(t *testing.T) {
		// Given a mock controller with InitializeFunc set to fail
		mocks := setupSafeInitCmdMocks()
		mocks.Controller.InitializeFunc = func() error {
			return fmt.Errorf("mocked error initializing controller")
		}

		// When the init command is executed
		rootCmd.SetArgs([]string{"init", "test-context"})
		err := Execute(mocks.Controller)
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		// Then the error should indicate an error initializing the controller
		expectedError := "Error initializing controller: mocked error initializing controller"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorCreatingCommonComponents", func(t *testing.T) {
		// Given a mock controller with CreateCommonComponentsFunc set to fail
		mocks := setupSafeInitCmdMocks()
		mocks.Controller.CreateCommonComponentsFunc = func() error {
			return fmt.Errorf("mocked error creating common components")
		}

		// When the init command is executed
		rootCmd.SetArgs([]string{"init", "test-context"})
		err := Execute(mocks.Controller)
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		// Then the error should indicate an error creating common components
		expectedError := "Error creating common components: mocked error creating common components"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorResolvingContextHandler", func(t *testing.T) {
		// Given a mock controller with ResolveContextHandlerFunc set to fail
		// Given a valid config handler
		mocks := setupSafeInitCmdMocks()
		mocks.Controller.ResolveContextHandlerFunc = func() (context.ContextHandler, error) {
			return nil, fmt.Errorf("mocked error resolving context handler")
		}

		// When the init command is executed
		rootCmd.SetArgs([]string{"init", "test-context"})
		err := Execute(mocks.Controller)
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		// Then the error should indicate an error resolving the context handler
		expectedError := "Error getting context handler: mocked error resolving context handler"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("NoContextProvided", func(t *testing.T) {
		// Given a valid config handler
		mocks := setupSafeInitCmdMocks()

		// When the init command is executed without a context
		output := captureStdout(func() {
			rootCmd.SetArgs([]string{"init"})
			err := Execute(mocks.Controller)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
		})

		// Then the output should indicate success
		expectedOutput := "Initialization successful\n"
		if output != expectedOutput {
			t.Errorf("Expected output %q, got %q", expectedOutput, output)
		}
	})

	t.Run("ContextNameProvided", func(t *testing.T) {
		// Given a valid config handler
		mocks := setupSafeInitCmdMocks()

		// When the init command is executed with a context name provided
		output := captureStdout(func() {
			rootCmd.SetArgs([]string{"init", "test-context"})
			err := Execute(mocks.Controller)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
		})

		// Then the output should indicate success
		expectedOutput := "Initialization successful\n"
		if output != expectedOutput {
			t.Errorf("Expected output %q, got %q", expectedOutput, output)
		}
	})

	t.Run("GetContextError", func(t *testing.T) {
		// Given a valid config handler
		mocks := setupSafeInitCmdMocks()

		mocks.ContextHandler.GetContextFunc = func() (string, error) {
			return "", fmt.Errorf("mocked error getting context")
		}

		// When the init command is executed
		rootCmd.SetArgs([]string{"init"})
		err := Execute(mocks.Controller)
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		// Then the error should be present
		expectedError := "mocked error getting context"
		if !strings.Contains(err.Error(), expectedError) {
			t.Errorf("Expected error to contain %q, but got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorResolvingConfigHandler", func(t *testing.T) {
		// Given a mock controller with ResolveConfigHandlerFunc set to fail
		injector := di.NewInjector()
		mockController := ctrl.NewMockController(injector)
		mockController.ResolveConfigHandlerFunc = func() (config.ConfigHandler, error) {
			return nil, fmt.Errorf("mocked error resolving config handler")
		}

		// When the init command is executed
		rootCmd.SetArgs([]string{"init", "test-context"})
		err := Execute(mockController)
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		// Then the error should be present
		expectedError := "mocked error resolving config handler"
		if !strings.Contains(err.Error(), expectedError) {
			t.Errorf("Expected error to contain %q, but got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorLoadingConfig", func(t *testing.T) {
		// Given a mock controller with LoadConfigFunc set to fail
		injector := di.NewInjector()
		mockController := ctrl.NewMockController(injector)
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.LoadConfigFunc = func(path string) error {
			return fmt.Errorf("mocked error loading config")
		}
		mockController.ResolveConfigHandlerFunc = func() (config.ConfigHandler, error) {
			return mockConfigHandler, nil
		}

		// When the init command is executed
		rootCmd.SetArgs([]string{"init", "test-context"})
		err := Execute(mockController)
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		// Then the error should be present
		expectedError := "mocked error loading config"
		if !strings.Contains(err.Error(), expectedError) {
			t.Errorf("Expected error to contain %q, but got %q", expectedError, err.Error())
		}
	})

	t.Run("SetContextError", func(t *testing.T) {
		// Given a valid config handler
		mocks := setupSafeInitCmdMocks()

		// Mock SetContext to return an error
		mocks.ContextHandler.SetContextFunc = func(contextName string) error {
			return fmt.Errorf("mocked error setting context")
		}

		// When the init command is executed
		rootCmd.SetArgs([]string{"init", "test-context"})
		err := Execute(mocks.Controller)
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		// Then the error should be present
		expectedError := "mocked error setting context"
		if !strings.Contains(err.Error(), expectedError) {
			t.Errorf("Expected error to contain %q, but got %q", expectedError, err.Error())
		}
	})

	t.Run("HomeDirError", func(t *testing.T) {
		// Given a mocked error when retrieving the user home directory
		mocks := setupSafeInitCmdMocks()

		// Mock osUserHomeDir to simulate an error
		originalUserHomeDir := osUserHomeDir
		osUserHomeDir = func() (string, error) {
			return "", fmt.Errorf("mocked error retrieving home directory")
		}
		defer func() { osUserHomeDir = originalUserHomeDir }()

		// When the init command is executed
		rootCmd.SetArgs([]string{"init", "test-context"})
		err := Execute(mocks.Controller)

		// Then check for presence of error using contains
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		expectedError := "mocked error retrieving home directory"
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain %q, got %v", expectedError, err)
		}
	})

	t.Run("SetConfigValueError", func(t *testing.T) {
		// Given a config handler that returns an error on SetConfigValue
		mocks := setupSafeInitCmdMocks()
		mocks.ConfigHandler.SetFunc = func(key string, value interface{}) error { return fmt.Errorf("set config value error") }

		// When the init command is executed
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"init", "test-context"})
			err := Execute(mocks.Controller)
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then the output should indicate the error
		expectedOutput := "set config value error"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("SaveConfigError", func(t *testing.T) {
		// Given a config handler that returns an error on SaveConfig
		mocks := setupSafeInitCmdMocks()
		mocks.ConfigHandler.SaveConfigFunc = func(path string) error { return fmt.Errorf("save config error") }

		// When the init command is executed
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"init", "test-context"})
			err := Execute(mocks.Controller)
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then the output should indicate the error
		expectedOutput := "Error saving config file: save config error"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("ErrorCreatingServiceComponents", func(t *testing.T) {
		// Given a mock controller with CreateServiceComponents set to fail
		mocks := setupSafeInitCmdMocks()
		mocks.Controller.CreateServiceComponentsFunc = func() error { return fmt.Errorf("create service components error") }

		// When the init command is executed
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"init", "test-context"})
			err := Execute(mocks.Controller)
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then the output should indicate the error
		expectedOutput := "Error creating service components: create service components error"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("ErrorInitializingComponents", func(t *testing.T) {
		// Given a mock controller with InitializeComponents set to fail
		mocks := setupSafeInitCmdMocks()
		mocks.Controller.InitializeComponentsFunc = func() error {
			return fmt.Errorf("initialize components error")
		}

		// Temporarily set the global controller to mockController
		originalController := controller
		controller = mocks.Controller
		defer func() { controller = originalController }()

		// When the init command is executed
		rootCmd.SetArgs([]string{"init", "test-context"})
		err := Execute(mocks.Controller)
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		// Then the output should indicate the error
		expectedOutput := "Error initializing components: initialize components error"
		if err.Error() != expectedOutput {
			t.Errorf("Expected error %q, got %q", expectedOutput, err.Error())
		}
	})

	t.Run("ErrorResolvingServices", func(t *testing.T) {
		// Given a mock controller with ResolveAllServices set to fail
		mocks := setupSafeInitCmdMocks()
		mocks.Controller.ResolveAllServicesFunc = func() ([]services.Service, error) {
			return nil, fmt.Errorf("resolve services error")
		}

		// When the init command is executed
		rootCmd.SetArgs([]string{"init", "test-context"})
		err := Execute(mocks.Controller)
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		// Then the error should indicate an error resolving services
		expectedError := "Error resolving services: resolve services error"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorWritingServiceConfig", func(t *testing.T) {
		// Given a mock controller with WriteConfig set to fail for a service
		mocks := setupSafeInitCmdMocks()
		mockService := services.NewMockService()
		mockService.WriteConfigFunc = func() error {
			return fmt.Errorf("write service config error")
		}
		mocks.Controller.ResolveAllServicesFunc = func() ([]services.Service, error) {
			return []services.Service{mockService}, nil
		}

		// When the init command is executed
		rootCmd.SetArgs([]string{"init", "test-context"})
		err := Execute(mocks.Controller)
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		// Then the error should indicate an error writing service config
		expectedError := "error writing service config: write service config error"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorResolvingVirtualMachine", func(t *testing.T) {
		// Given a mock controller with ResolveVirtualMachine set to fail
		mocks := setupSafeInitCmdMocks()
		mocks.Controller.ResolveVirtualMachineFunc = func() (virt.VirtualMachine, error) {
			return nil, fmt.Errorf("resolve virtual machine error")
		}

		// When the init command is executed
		rootCmd.SetArgs([]string{"init", "test-context"})
		err := Execute(mocks.Controller)
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		// Then the error should indicate an error resolving the virtual machine
		expectedError := "Error resolving virtual machine: resolve virtual machine error"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorWritingVirtualMachineConfig", func(t *testing.T) {
		// Given a mock controller with WriteConfig set to fail for the virtual machine
		mocks := setupSafeInitCmdMocks()
		mockVirtualMachine := virt.NewMockVirt()
		mockVirtualMachine.WriteConfigFunc = func() error {
			return fmt.Errorf("write virtual machine config error")
		}
		mocks.Controller.ResolveVirtualMachineFunc = func() (virt.VirtualMachine, error) {
			return mockVirtualMachine, nil
		}

		// When the init command is executed
		rootCmd.SetArgs([]string{"init", "test-context"})
		err := Execute(mocks.Controller)
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		// Then the error should indicate an error writing virtual machine config
		expectedError := "error writing virtual machine config: write virtual machine config error"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorResolvingContainerRuntime", func(t *testing.T) {
		// Given a mock controller with ResolveContainerRuntime set to fail
		mocks := setupSafeInitCmdMocks()
		mocks.Controller.ResolveContainerRuntimeFunc = func() (virt.ContainerRuntime, error) {
			return nil, fmt.Errorf("resolve container runtime error")
		}

		// When the init command is executed
		rootCmd.SetArgs([]string{"init", "test-context"})
		err := Execute(mocks.Controller)
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		// Then the error should indicate an error resolving the container runtime
		expectedError := "Error resolving container runtime: resolve container runtime error"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorWritingContainerRuntimeConfig", func(t *testing.T) {
		// Given a mock controller with WriteConfig set to fail for the container runtime
		mocks := setupSafeInitCmdMocks()
		mockContainerRuntime := virt.NewMockVirt()
		mockContainerRuntime.WriteConfigFunc = func() error {
			return fmt.Errorf("write container runtime config error")
		}
		mocks.Controller.ResolveContainerRuntimeFunc = func() (virt.ContainerRuntime, error) {
			return mockContainerRuntime, nil
		}

		// When the init command is executed
		rootCmd.SetArgs([]string{"init", "test-context"})
		err := Execute(mocks.Controller)
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		// Then the error should indicate an error writing container runtime config
		expectedError := "error writing container runtime config: write container runtime config error"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("CLIConfigSaveError", func(t *testing.T) {
		// Given a config handler that returns an error on SaveConfig
		mocks := setupSafeInitCmdMocks()
		mocks.ConfigHandler.SaveConfigFunc = func(path string) error { return fmt.Errorf("save cli config error") }

		// When the init command is executed
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"init", "test-context"})
			err := Execute(mocks.Controller)
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then the output should indicate the error
		expectedOutput := "Error saving config file: save cli config error"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("LocalContextSetsDefault", func(t *testing.T) {
		// Arrange: Create a mock config handler and set the SetDefaultFunc to check the parameters
		mocks := setupSafeInitCmdMocks()
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
			err := Execute(mocks.Controller)
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
		mocks := setupSafeInitCmdMocks()
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
		err := Execute(mocks.Controller)
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
		mocks := setupSafeInitCmdMocks()
		calledKeys := make(map[string]bool)
		mocks.ConfigHandler.SetFunc = func(key string, value interface{}) error {
			calledKeys[key] = true
			return nil
		}
		rootCmd.SetArgs([]string{
			"init", "test-context",
			"--docker",
		})
		err := Execute(mocks.Controller)
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
		mocks := setupSafeInitCmdMocks()
		calledKeys := make(map[string]bool)
		mocks.ConfigHandler.SetFunc = func(key string, value interface{}) error {
			calledKeys[key] = true
			return nil
		}

		rootCmd.SetArgs([]string{
			"init", "test-context",
			"--git-livereload",
		})
		err := Execute(mocks.Controller)
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
		mocks := setupSafeInitCmdMocks()
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
		err := Execute(mocks.Controller)
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		expectedError := "mock set error"
		if !strings.Contains(err.Error(), expectedError) {
			t.Errorf("Expected error to contain %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("TerraformConfiguration", func(t *testing.T) {
		mocks := setupSafeInitCmdMocks()
		calledKeys := make(map[string]bool)
		mocks.ConfigHandler.SetFunc = func(key string, value interface{}) error {
			calledKeys[key] = true
			return nil
		}

		rootCmd.SetArgs([]string{
			"init", "test-context",
			"--backend", "s3",
		})
		err := Execute(mocks.Controller)
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
		mocks := setupSafeInitCmdMocks()
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
		err := Execute(mocks.Controller)
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
		mocks := setupSafeInitCmdMocks()
		mocks.ConfigHandler.SetFunc = func(key string, value interface{}) error {
			if key == "aws.aws_endpoint_url" {
				return fmt.Errorf("error setting AWS endpoint URL")
			}
			return nil
		}

		rootCmd.SetArgs([]string{
			"init", "test-context",
			"--aws-endpoint-url", "http://localhost:4566",
		})
		err := Execute(mocks.Controller)
		if err == nil || err.Error() != "Error setting AWS endpoint URL: error setting AWS endpoint URL" {
			t.Fatalf("Expected error setting AWS endpoint URL, got %v", err)
		}
	})

	t.Run("ErrorSettingAWSProfile", func(t *testing.T) {
		mocks := setupSafeInitCmdMocks()
		mocks.ConfigHandler.SetFunc = func(key string, value interface{}) error {
			if key == "aws.aws_profile" {
				return fmt.Errorf("error setting AWS profile")
			}
			return nil
		}

		rootCmd.SetArgs([]string{
			"init", "test-context",
			"--aws-profile", "default",
		})
		err := Execute(mocks.Controller)
		if err == nil || err.Error() != "Error setting AWS profile: error setting AWS profile" {
			t.Fatalf("Expected error setting AWS profile, got %v", err)
		}
	})

	t.Run("ErrorSettingDockerConfiguration", func(t *testing.T) {
		mocks := setupSafeInitCmdMocks()
		mocks.ConfigHandler.SetFunc = func(key string, value interface{}) error {
			if key == "docker.enabled" {
				return fmt.Errorf("error setting Docker enabled")
			}
			return nil
		}
		rootCmd.SetArgs([]string{
			"init", "test-context",
			"--docker",
		})
		err := Execute(mocks.Controller)
		if err == nil || err.Error() != "Error setting Docker enabled: error setting Docker enabled" {
			t.Fatalf("Expected error setting Docker enabled, got %v", err)
		}
	})

	t.Run("ErrorSettingTerraformConfiguration", func(t *testing.T) {
		mocks := setupSafeInitCmdMocks()
		mocks.ConfigHandler.SetFunc = func(key string, value interface{}) error {
			if key == "terraform.backend" {
				return fmt.Errorf("error setting Terraform backend")
			}
			return nil
		}

		rootCmd.SetArgs([]string{
			"init", "test-context",
			"--backend", "s3",
		})
		err := Execute(mocks.Controller)
		if err == nil || err.Error() != "Error setting Terraform backend: error setting Terraform backend" {
			t.Fatalf("Expected error setting Terraform backend, got %v", err)
		}
	})

	t.Run("ErrorSettingVMConfigurationArch", func(t *testing.T) {
		mocks := setupSafeInitCmdMocks()
		mocks.ConfigHandler.SetFunc = func(key string, value interface{}) error {
			if key == "vm.arch" {
				return fmt.Errorf("error setting VM architecture")
			}
			return nil
		}

		rootCmd.SetArgs([]string{
			"init", "test-context",
			"--vm-arch", "x86_64",
		})
		err := Execute(mocks.Controller)
		if err == nil || err.Error() != "Error setting VM architecture: error setting VM architecture" {
			t.Fatalf("Expected error setting VM architecture, got %v", err)
		}
	})

	t.Run("ErrorSettingVMConfigurationDriver", func(t *testing.T) {
		mocks := setupSafeInitCmdMocks()
		mocks.ConfigHandler.SetFunc = func(key string, value interface{}) error {
			if key == "vm.driver" {
				return fmt.Errorf("error setting VM driver")
			}
			return nil
		}

		rootCmd.SetArgs([]string{
			"init", "test-context",
			"--vm-driver", "colima",
		})
		err := Execute(mocks.Controller)
		if err == nil || err.Error() != "Error setting VM driver: error setting VM driver" {
			t.Fatalf("Expected error setting VM driver, got %v", err)
		}
	})
	t.Run("ErrorSettingVMConfigurationCPU", func(t *testing.T) {
		mocks := setupSafeInitCmdMocks()
		mocks.ConfigHandler.SetFunc = func(key string, value interface{}) error {
			if key == "vm.cpu" {
				return fmt.Errorf("error setting VM CPU")
			}
			return nil
		}

		rootCmd.SetArgs([]string{
			"init", "test-context",
			"--vm-cpu", "2",
		})
		err := Execute(mocks.Controller)
		if err == nil || err.Error() != "Error setting VM CPU: error setting VM CPU" {
			t.Fatalf("Expected error setting VM CPU, got %v", err)
		}
	})

	t.Run("ErrorSettingVMConfigurationDisk", func(t *testing.T) {
		mocks := setupSafeInitCmdMocks()
		mocks.ConfigHandler.SetFunc = func(key string, value interface{}) error {
			if key == "vm.disk" {
				return fmt.Errorf("error setting VM disk")
			}
			return nil
		}

		rootCmd.SetArgs([]string{
			"init", "test-context",
			"--vm-disk", "20",
		})
		err := Execute(mocks.Controller)
		if err == nil || err.Error() != "Error setting VM disk: error setting VM disk" {
			t.Fatalf("Expected error setting VM disk, got %v", err)
		}
	})

	t.Run("ErrorSettingVMConfigurationMemory", func(t *testing.T) {
		mocks := setupSafeInitCmdMocks()
		mocks.ConfigHandler.SetFunc = func(key string, value interface{}) error {
			if key == "vm.memory" {
				return fmt.Errorf("error setting VM memory")
			}
			return nil
		}

		rootCmd.SetArgs([]string{
			"init", "test-context",
			"--vm-memory", "4096",
		})
		err := Execute(mocks.Controller)
		if err == nil || err.Error() != "Error setting VM memory: error setting VM memory" {
			t.Fatalf("Expected error setting VM memory, got %v", err)
		}
	})

	t.Run("SetDefaultLocalConfigError", func(t *testing.T) {
		// Given a config handler that returns an error on SetDefault for local config
		mocks := setupSafeInitCmdMocks()
		mocks.ConfigHandler.SetDefaultFunc = func(context config.Context) error {
			expectedValue := config.DefaultLocalConfig
			if !reflect.DeepEqual(context, expectedValue) {
				return fmt.Errorf("Expected value %v, got %v", expectedValue, context)
			}
			return fmt.Errorf("error setting default local config")
		}

		// When the init command is executed with a local context
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"init", "local"})
			err := Execute(mocks.Controller)
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then the output should indicate the error
		expectedOutput := "Error setting default local config: error setting default local config"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("SetDefaultConfigError", func(t *testing.T) {
		// Given a config handler that returns an error on SetDefault for default config
		mocks := setupSafeInitCmdMocks()
		mocks.ConfigHandler.SetDefaultFunc = func(context config.Context) error {
			if reflect.DeepEqual(context, config.DefaultConfig) {
				return fmt.Errorf("error setting default config")
			}
			return nil
		}

		// When the init command is executed with a non-local context
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"init", "test-context"})
			err := Execute(mocks.Controller)
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then the output should indicate the error
		expectedOutput := "Error setting default config: error setting default config"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})
}
