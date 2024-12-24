package cmd

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/context"
	ctrl "github.com/windsorcli/cli/pkg/controller"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
)

// setupSafeInitCmdMocks returns a mock controller with safe mocks for the init command
func setupSafeInitCmdMocks(existingControllers ...ctrl.Controller) *ctrl.MockController {
	var mockController *ctrl.MockController
	var injector di.Injector

	if len(existingControllers) > 0 {
		// Use the passed controller and its injector
		mockController = existingControllers[0].(*ctrl.MockController)
		injector = mockController.ResolveInjector()
	} else {
		// Create a new injector and mock controller
		injector = di.NewInjector()
		mockController = ctrl.NewMockController(injector)
	}

	// Manually override and set up components
	mockController.CreateCommonComponentsFunc = func() error {
		return nil
	}

	// Setup mock context handler
	mockContextHandler := context.NewMockContext()
	mockContextHandler.GetContextFunc = func() string {
		return "test-context"
	}
	injector.Register("contextHandler", mockContextHandler)

	// Setup mock config handler
	mockConfigHandler := config.NewMockConfigHandler()
	mockConfigHandler.SetFunc = func(key string, value interface{}) error {
		return nil
	}
	injector.Register("configHandler", mockConfigHandler)

	// Setup mock shell
	mockShell := shell.NewMockShell()
	injector.Register("shell", mockShell)

	return mockController
}

// TestInitCmd tests the init command
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
		// Setup mocks
		controller := setupSafeInitCmdMocks()

		// Execute the init command and capture output
		output := captureStdout(func() {
			rootCmd.SetArgs([]string{"init", "test-context"})
			if err := Execute(controller); err != nil {
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
		controller := setupSafeInitCmdMocks()

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
			err := Execute(controller)
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

	t.Run("NoContextProvided", func(t *testing.T) {
		// Given a valid config handler
		controller := setupSafeInitCmdMocks()

		// When the init command is executed without a context
		output := captureStdout(func() {
			rootCmd.SetArgs([]string{"init"})
			err := Execute(controller)
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

	t.Run("EmptyContextName", func(t *testing.T) {
		// Given a valid config handler
		controller := setupSafeInitCmdMocks()

		// When the init command is executed with an empty context name
		output := captureStdout(func() {
			rootCmd.SetArgs([]string{"init", ""})
			err := Execute(controller)
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
		controller := setupSafeInitCmdMocks()

		// When the init command is executed with a context name provided
		output := captureStdout(func() {
			rootCmd.SetArgs([]string{"init", "test-context"})
			err := Execute(controller)
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

	t.Run("ResolveContextHandlerError", func(t *testing.T) {
		// Given a mock controller with ResolveContextHandlerFunc set to fail
		injector := di.NewInjector()
		controller := ctrl.NewMockController(injector)
		controller.ResolveContextHandlerFunc = func() context.ContextHandler {
			return nil
		}

		// When the init command is executed
		rootCmd.SetArgs([]string{"init"})
		err := Execute(controller)
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		// Then the error should be present
		expectedError := "No context handler found"
		if !strings.Contains(err.Error(), expectedError) {
			t.Errorf("Expected error to contain %q, but got %q", expectedError, err.Error())
		}
	})

	t.Run("SetContextError", func(t *testing.T) {
		// Given a valid config handler
		injector := di.NewInjector()
		controller := ctrl.NewMockController(injector)
		setupSafeInitCmdMocks(controller)

		// Mock SetContext to return an error
		contextHandler := context.NewMockContext()
		contextHandler.SetContextFunc = func(contextName string) error {
			return fmt.Errorf("mocked error setting context")
		}
		injector.Register("contextHandler", contextHandler)

		// When the init command is executed
		rootCmd.SetArgs([]string{"init", "test-context"})
		err := Execute(controller)
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		// Then the error should be present
		expectedError := "mocked error setting context"
		if !strings.Contains(err.Error(), expectedError) {
			t.Errorf("Expected error to contain %q, but got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorSettingDefaultLocalConfig", func(t *testing.T) {
		// Given a mock config handler with SetDefaultFunc set to fail
		injector := di.NewInjector()
		controller := ctrl.NewMockController(injector)
		setupSafeInitCmdMocks(controller)
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.SetDefaultFunc = func(context config.Context) error {
			return fmt.Errorf("set default local config error")
		}
		injector.Register("configHandler", mockConfigHandler)

		// When the init command is executed
		rootCmd.SetArgs([]string{"init", "local"})
		err := Execute(controller)
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		// Then the error should indicate an error setting the default config
		expectedError := "set default local config error"
		if !strings.Contains(err.Error(), expectedError) {
			t.Errorf("Expected error to contain %q, but got %q", expectedError, err.Error())
		}
	})

	t.Run("NonLocalContextSetsDefault", func(t *testing.T) {
		// Given a mock config handler with SetDefaultFunc set to succeed
		injector := di.NewInjector()
		controller := ctrl.NewMockController(injector)
		setupSafeInitCmdMocks(controller)
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.SetDefaultFunc = func(context config.Context) error {
			expectedValue := config.DefaultConfig
			if !reflect.DeepEqual(context, expectedValue) {
				t.Errorf("Expected value %v, got %v", expectedValue, context)
			}
			return nil
		}
		injector.Register("configHandler", mockConfigHandler)

		// When the init command is executed with a non-local context
		output := captureStdout(func() {
			rootCmd.SetArgs([]string{"init", "test-context"})
			err := Execute(controller)
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

	t.Run("SetDefaultConfigError", func(t *testing.T) {
		// Given a mock config handler with SetDefaultFunc set to fail
		injector := di.NewInjector()
		controller := ctrl.NewMockController(injector)
		setupSafeInitCmdMocks(controller)
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.SetDefaultFunc = func(context config.Context) error {
			return fmt.Errorf("set default config error")
		}
		injector.Register("configHandler", mockConfigHandler)

		// When the init command is executed
		rootCmd.SetArgs([]string{"init", "test-context"})
		err := Execute(controller)
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		// Then the error should indicate an error setting the default config
		expectedError := "set default config error"
		if !strings.Contains(err.Error(), expectedError) {
			t.Errorf("Expected error to contain %q, but got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorSettingConfig", func(t *testing.T) {
		// Given a mock config handler with SetFunc set to fail for specific configs
		injector := di.NewInjector()
		controller := ctrl.NewMockController(injector)
		setupSafeInitCmdMocks(controller)
		mockConfigHandler := config.NewMockConfigHandler()

		// Define the flags and expected errors
		flags := []struct {
			arg           string
			key           string
			value         interface{}
			expectedError string
			isSingleParam bool
		}{
			{"--aws-endpoint-url", "aws.aws_endpoint_url", "value", "Error setting aws-endpoint-url configuration: set aws.aws_endpoint_url config error", true},
			{"--aws-profile", "aws.aws_profile", "value", "Error setting aws-profile configuration: set aws.aws_profile config error", true},
			{"--docker", "docker.enabled", true, "Error setting docker configuration: set docker.enabled config error", false},
			{"--backend", "terraform.backend", "value", "Error setting backend configuration: set terraform.backend config error", true},
			{"--vm-driver", "vm.driver", "value", "Error setting vm-driver configuration: set vm.driver config error", true},
			{"--vm-cpu", "vm.cpu", 1, "Error setting vm-cpu configuration: set vm.cpu config error", true},
			{"--vm-disk", "vm.disk", 1, "Error setting vm-disk configuration: set vm.disk config error", true},
			{"--vm-memory", "vm.memory", 1, "Error setting vm-memory configuration: set vm.memory config error", true},
			{"--vm-arch", "vm.arch", "value", "Error setting vm-arch configuration: set vm.arch config error", true},
			{"--git-livereload", "git.livereload.enabled", true, "Error setting git-livereload configuration: set git.livereload.enabled config error", false},
		}

		// Loop through each flag and check for errors
		for _, flag := range flags {
			mockConfigHandler.SetContextValueFunc = func(key string, value interface{}) error {
				if key == flag.key {
					return fmt.Errorf("set %s config error", key)
				}
				return nil
			}
			injector.Register("configHandler", mockConfigHandler)

			if flag.isSingleParam {
				rootCmd.SetArgs([]string{"init", "test-context", flag.arg, fmt.Sprintf("%v", flag.value)})
			} else {
				rootCmd.SetArgs([]string{"init", "test-context", flag.arg})
			}
			err := Execute(controller)
			if err == nil {
				t.Fatalf("Expected error for flag %q, got nil", flag.arg)
			}

			if err.Error() != flag.expectedError {
				t.Errorf("For flag %q, expected error %q, got %q", flag.arg, flag.expectedError, err.Error())
			}
		}
	})

	t.Run("ErrorGettingCliConfigPath", func(t *testing.T) { // Given a mock controller with GetCliConfigPath set to fail
		injector := di.NewInjector()
		controller := ctrl.NewMockController(injector)
		setupSafeInitCmdMocks(controller)

		// Override getCliConfigPath to return an error on the second call
		originalGetCliConfigPath := getCliConfigPath
		callCount := 0
		getCliConfigPath = func() (string, error) {
			callCount++
			if callCount == 2 {
				return "", fmt.Errorf("get cli config path error")
			}
			return "/mock/path/to/config.yaml", nil
		}
		defer func() { getCliConfigPath = originalGetCliConfigPath }()

		// When the init command is executed twice
		rootCmd.SetArgs([]string{"init", "test-context"})
		err := Execute(controller)
		if err == nil {
			t.Fatalf("Expected error on first execution, got nil")
		}

		// Then the error should be as expected
		expectedError := "Error getting cli configuration path: get cli config path error"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("SaveConfigError", func(t *testing.T) {
		// Given a config handler that returns an error on SaveConfig
		injector := di.NewInjector()
		controller := ctrl.NewMockController(injector)
		setupSafeInitCmdMocks(controller)
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.SaveConfigFunc = func(path string) error { return fmt.Errorf("save config error") }
		injector.Register("configHandler", mockConfigHandler)

		// When the init command is executed
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"init", "test-context"})
			err := Execute(controller)
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

	t.Run("ErrorCreatingProjectComponents", func(t *testing.T) {
		// Given a mock controller with CreateProjectComponents set to fail
		injector := di.NewInjector()
		controller := ctrl.NewMockController(injector)
		setupSafeInitCmdMocks(controller)
		controller.CreateProjectComponentsFunc = func() error { return fmt.Errorf("create project components error") }

		// When the init command is executed
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"init", "test-context"})
			err := Execute(controller)
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then the output should indicate the error
		expectedOutput := "Error creating project components: create project components error"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("ErrorCreatingServiceComponents", func(t *testing.T) {
		// Given a mock controller with CreateServiceComponents set to fail
		injector := di.NewInjector()
		controller := ctrl.NewMockController(injector)
		setupSafeInitCmdMocks(controller)
		controller.CreateServiceComponentsFunc = func() error { return fmt.Errorf("create service components error") }

		// When the init command is executed
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"init", "test-context"})
			err := Execute(controller)
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

	t.Run("ErrorCreatingVirtualizationComponents", func(t *testing.T) {
		// Given a mock controller with CreateVirtualizationComponents set to fail
		injector := di.NewInjector()
		controller := ctrl.NewMockController(injector)
		setupSafeInitCmdMocks(controller)
		controller.CreateVirtualizationComponentsFunc = func() error { return fmt.Errorf("create virtualization components error") }

		// When the init command is executed
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"init", "test-context"})
			err := Execute(controller)
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then the output should indicate the error
		expectedOutput := "Error creating virtualization components: create virtualization components error"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("ErrorCreatingStackComponents", func(t *testing.T) {
		// Given a mock controller with CreateStackComponents set to fail
		injector := di.NewInjector()
		controller := ctrl.NewMockController(injector)
		setupSafeInitCmdMocks(controller)
		controller.CreateStackComponentsFunc = func() error { return fmt.Errorf("create stack components error") }

		// When the init command is executed
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"init", "test-context"})
			err := Execute(controller)
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then the output should indicate the error
		expectedOutput := "Error creating stack components: create stack components error"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("ErrorInitializingComponents", func(t *testing.T) {
		// Given a mock controller with InitializeComponents set to fail
		injector := di.NewInjector()
		controller := ctrl.NewMockController(injector)
		setupSafeInitCmdMocks(controller)
		controller.InitializeComponentsFunc = func() error {
			return fmt.Errorf("initialize components error")
		}

		// When the init command is executed
		rootCmd.SetArgs([]string{"init", "test-context"})
		err := Execute(controller)
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		// Then the output should indicate the error
		expectedOutput := "Error initializing components: initialize components error"
		if err.Error() != expectedOutput {
			t.Errorf("Expected error %q, got %q", expectedOutput, err.Error())
		}
	})

	t.Run("ErrorWritingConfigurationFiles", func(t *testing.T) {
		// Given a mock controller with WriteConfigurationFiles set to fail
		injector := di.NewInjector()
		controller := ctrl.NewMockController(injector)
		setupSafeInitCmdMocks(controller)
		controller.WriteConfigurationFilesFunc = func() error {
			return fmt.Errorf("write configuration files error")
		}

		// When the init command is executed
		rootCmd.SetArgs([]string{"init", "test-context"})
		err := Execute(controller)
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		// Then the output should indicate the error
		expectedOutput := "Error writing configuration files: write configuration files error"
		if err.Error() != expectedOutput {
			t.Errorf("Expected error %q, got %q", expectedOutput, err.Error())
		}
	})
}
