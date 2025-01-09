package cmd

import (
	"fmt"
	"os"
	"strings"
	"testing"

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
	controller = mockController

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
			err := Execute(controller)
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
