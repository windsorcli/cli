package cmd

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/config"
	ctrl "github.com/windsorcli/cli/pkg/controller"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
)

func setupSafeInitCmdMocks(existingInjectors ...di.Injector) *initMockObjects {
	var injector di.Injector
	if len(existingInjectors) > 0 {
		injector = existingInjectors[0]
	} else {
		injector = di.NewInjector()
	}

	mockController := ctrl.NewMockController(injector)

	mockController.CreateCommonComponentsFunc = func() error { return nil }
	mockController.InitializeComponentsFunc = func() error { return nil }

	mockConfigHandler := config.NewMockConfigHandler()
	mockConfigHandler.SetFunc = func(key string, value interface{}) error { return nil }
	mockConfigHandler.GetContextFunc = func() string { return "test-context" }
	mockConfigHandler.SetContextFunc = func(contextName string) error { return nil }
	mockConfigHandler.InitializeFunc = func() error { return nil }
	mockConfigHandler.LoadConfigFunc = func(path string) error { return nil }
	injector.Register("configHandler", mockConfigHandler)

	mockShell := shell.NewMockShell()
	mockShell.GetProjectRootFunc = func() (string, error) { return "/mock/project/root", nil }
	injector.Register("shell", mockShell)

	osStat = func(_ string) (os.FileInfo, error) { return nil, nil }

	mockController.ResolveConfigHandlerFunc = func() config.ConfigHandler { return mockConfigHandler }
	mockController.ResolveShellFunc = func() shell.Shell { return mockShell }

	return &initMockObjects{
		Controller:    mockController,
		Injector:      injector,
		ConfigHandler: mockConfigHandler,
		Shell:         mockShell,
	}
}

// initMockObjects encapsulates all mock objects used in the init command tests
type initMockObjects struct {
	Controller    *ctrl.MockController
	Injector      di.Injector
	ConfigHandler *config.MockConfigHandler
	Shell         *shell.MockShell
}

// TestInitCmd tests the init command
func TestInitCmd(t *testing.T) {
	originalArgs := rootCmd.Args
	originalExitFunc := exitFunc

	t.Cleanup(func() {
		rootCmd.Args = originalArgs
		exitFunc = originalExitFunc
		resetRootCmd()
	})

	// Mock the exit function to prevent the test from exiting
	exitFunc = func(code int) {
		panic("exit called")
	}

	t.Run("Success", func(t *testing.T) {

		// Setup mocks
		mocks := setupSafeInitCmdMocks()

		// Execute the init command and capture output
		output := captureStderr(func() {
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
		output := captureStderr(func() {
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

	t.Run("VMDriverDockerDesktop", func(t *testing.T) {

		// Given a valid config handler
		mocks := setupSafeInitCmdMocks()

		// Mock the GetString method to return "docker-desktop" for vm.driver
		mocks.ConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "vm.driver" {
				return "docker-desktop"
			}
			return ""
		}

		// Track if SetDefault is called with the correct config
		setDefaultCalled := false
		mocks.ConfigHandler.SetDefaultFunc = func(contextConfig v1alpha1.Context) error {
			if contextConfig.VM != nil && contextConfig.VM.Driver != nil && *contextConfig.VM.Driver == "docker-desktop" {
				setDefaultCalled = true
			}
			return nil
		}

		// When the init command is executed with vm.driver set to "docker-desktop"
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"init", "test-context", "--vm-driver", "docker-desktop"})
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

		// Check if SetDefault was called correctly
		if !setDefaultCalled {
			t.Errorf("Expected SetDefault to be called with vm.driver 'docker-desktop'")
		}
	})

	t.Run("ErrorSettingDefaultContainerizedConfig", func(t *testing.T) {

		// Given a mock config handler that returns an error when setting default containerized config
		mocks := setupSafeInitCmdMocks()
		mocks.ConfigHandler.SetDefaultFunc = func(contextConfig v1alpha1.Context) error {
			if contextConfig.VM != nil && *contextConfig.VM.Driver == "docker-desktop" {
				return fmt.Errorf("error setting default containerized config")
			}
			return nil
		}

		// Mock the GetString method to return "docker-desktop" for vm.driver
		mocks.ConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "vm.driver" {
				return "docker-desktop"
			}
			return ""
		}

		// When the init command is executed with vm.driver set to "docker-desktop"
		rootCmd.SetArgs([]string{"init", "test-context", "--vm-driver", "docker-desktop"})
		err := Execute(mocks.Controller)

		// Then an error should be returned
		if err == nil || !strings.Contains(err.Error(), "error setting default containerized config") {
			t.Fatalf("Expected error setting default containerized config, got %v", err)
		}
	})

	t.Run("VMDriverColima", func(t *testing.T) {
		// Given a valid config handler
		mocks := setupSafeInitCmdMocks()

		// Mock the GetString method to return "colima" for vm.driver
		mocks.ConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "vm.driver" {
				return "colima"
			}
			return ""
		}

		// Track if SetDefault is called with the correct config
		setDefaultCalled := false
		mocks.ConfigHandler.SetDefaultFunc = func(contextConfig v1alpha1.Context) error {
			if contextConfig.VM != nil && *contextConfig.VM.Driver == "colima" {
				setDefaultCalled = true
			}
			return nil
		}

		// When the init command is executed with vm.driver set to "colima"
		rootCmd.SetArgs([]string{"init", "test-context", "--vm-driver", "colima"})
		output := captureStderr(func() {
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

		// Validate that SetDefault was called with the correct configuration
		if !setDefaultCalled {
			t.Error("Expected SetDefault to be called with DefaultConfig_FullVM, but it was not")
		}
	})

	t.Run("ErrorSettingDefaultFullVMConfig", func(t *testing.T) {

		// Given a mock config handler that returns an error when setting default full VM config
		mocks := setupSafeInitCmdMocks()
		mocks.ConfigHandler.SetDefaultFunc = func(contextConfig v1alpha1.Context) error {
			if contextConfig.VM != nil && *contextConfig.VM.Driver == "colima" {
				return fmt.Errorf("error setting default full VM config")
			}
			return nil
		}

		// Mock the GetString method to return "colima" for vm.driver
		mocks.ConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "vm.driver" {
				return "colima"
			}
			return ""
		}

		// When the init command is executed with vm.driver set to "colima"
		rootCmd.SetArgs([]string{"init", "test-context", "--vm-driver", "colima"})
		err := Execute(mocks.Controller)

		// Then an error should be returned
		if err == nil || !strings.Contains(err.Error(), "error setting default full VM config") {
			t.Fatalf("Expected error setting default full VM config, got %v", err)
		}
	})

	t.Run("ErrorAddingCurrentDirToTrustedFile", func(t *testing.T) {

		// Given a mock shell that returns an error when adding current directory to trusted file
		injector := di.NewInjector()
		mockShell := shell.NewMockShell(injector)
		mockShell.AddCurrentDirToTrustedFileFunc = func() error {
			return fmt.Errorf("error adding current directory to trusted file")
		}

		// Set the shell in the controller to the mock shell
		mocks := setupSafeInitCmdMocks()
		mocks.Controller.ResolveShellFunc = func() shell.Shell {
			return mockShell
		}

		// When the init command is executed
		rootCmd.SetArgs([]string{"init", "test-context"})
		err := Execute(mocks.Controller)

		// Then check the error contents
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
		expectedError := "Error adding current directory to trusted file: error adding current directory to trusted file"
		if err.Error() != expectedError {
			t.Fatalf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("NoContextProvided", func(t *testing.T) {
		// Given a valid config handler
		mocks := setupSafeInitCmdMocks()

		// When the init command is executed without a context
		output := captureStderr(func() {
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

	t.Run("EmptyContextName", func(t *testing.T) {
		// Given a valid config handler
		mocks := setupSafeInitCmdMocks()

		// When the init command is executed with an empty context name
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"init", ""})
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
		output := captureStderr(func() {
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

	t.Run("SetContextError", func(t *testing.T) {
		// Given a valid config handler
		mocks := setupSafeInitCmdMocks()

		// Mock SetContext to return an error
		mocks.ConfigHandler.SetContextFunc = func(contextName string) error {
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

	t.Run("ErrorGettingProjectRoot", func(t *testing.T) {
		// Given a valid config handler
		mocks := setupSafeInitCmdMocks()

		// Counter to track the number of times GetProjectRootFunc is called
		callCount := 0

		// Mock GetProjectRoot to return an error only on the second call
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			callCount++
			if callCount == 2 {
				return "", fmt.Errorf("mocked error retrieving project root")
			}
			return "/mock/project/root", nil
		}

		// When the init command is executed
		err := Execute(mocks.Controller)
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		// Then the error should be present
		expectedError := "error retrieving project root: mocked error retrieving project root"
		if !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(expectedError)) {
			t.Errorf("Expected error to contain %q, but got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorSettingFlagConfig", func(t *testing.T) {
		// Given a valid config handler
		mocks := setupSafeInitCmdMocks()

		// Mock SetContextValue to return an error for a specific flag
		mocks.ConfigHandler.SetContextValueFunc = func(configPath string, value interface{}) error {
			if configPath == "aws.aws_endpoint_url" {
				return fmt.Errorf("mocked error setting aws-endpoint-url configuration")
			}
			return nil
		}

		// When the init command is executed with the aws-endpoint-url flag set
		rootCmd.SetArgs([]string{"init", "test-context", "--aws-endpoint-url", "http://mock-url"})
		err := Execute(mocks.Controller)
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		// Then the error should be present
		expectedError := "mocked error setting aws-endpoint-url configuration"
		if !strings.Contains(err.Error(), expectedError) {
			t.Errorf("Expected error to contain %q, but got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorRetrievingProjectRoot", func(t *testing.T) {
		// Given a valid config handler
		mocks := setupSafeInitCmdMocks()

		// Mock GetProjectRoot to return an error
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("mocked error retrieving project root")
		}

		// When the init command is executed
		err := Execute(mocks.Controller)
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		// Then the error should be present
		expectedError := "error retrieving project root: mocked error retrieving project root"
		if !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(expectedError)) {
			t.Errorf("Expected error to contain %q, but got %q", expectedError, err.Error())
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

	t.Run("ErrorCreatingProjectComponents", func(t *testing.T) {
		// Given a mock controller with CreateProjectComponents set to fail
		mocks := setupSafeInitCmdMocks()
		mocks.Controller.CreateProjectComponentsFunc = func() error { return fmt.Errorf("create project components error") }

		// When the init command is executed
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"init", "test-context"})
			err := Execute(mocks.Controller)
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

	t.Run("ErrorCreatingVirtualizationComponents", func(t *testing.T) {
		// Given a mock controller with CreateVirtualizationComponents set to fail
		mocks := setupSafeInitCmdMocks()
		mocks.Controller.CreateVirtualizationComponentsFunc = func() error { return fmt.Errorf("create virtualization components error") }

		// When the init command is executed
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"init", "test-context"})
			err := Execute(mocks.Controller)
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
		mocks := setupSafeInitCmdMocks()
		mocks.Controller.CreateStackComponentsFunc = func() error { return fmt.Errorf("create stack components error") }

		// When the init command is executed
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"init", "test-context"})
			err := Execute(mocks.Controller)
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
		mocks := setupSafeInitCmdMocks()
		mocks.Controller.InitializeComponentsFunc = func() error {
			return fmt.Errorf("initialize components error")
		}

		// When the init command is executed
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"init", "test-context"})
			err := Execute(mocks.Controller)
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then the output should indicate the error
		expectedOutput := "Error initializing components: initialize components error"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("ErrorWritingConfigurationFiles", func(t *testing.T) {
		// Given a mock controller with WriteConfigurationFiles set to fail
		mocks := setupSafeInitCmdMocks()
		mocks.Controller.WriteConfigurationFilesFunc = func() error {
			return fmt.Errorf("write configuration files error")
		}

		// When the init command is executed
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"init", "test-context"})
			err := Execute(mocks.Controller)
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then the output should indicate the error
		expectedOutput := "Error writing configuration files: write configuration files error"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})
}
