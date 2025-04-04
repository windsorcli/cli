package cmd

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/network"

	ctrl "github.com/windsorcli/cli/pkg/controller"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
	"github.com/windsorcli/cli/pkg/virt"
)

type MockSafeDownCmdComponents struct {
	Injector             di.Injector
	MockController       *ctrl.MockController
	MockConfigHandler    *config.MockConfigHandler
	MockShell            *shell.MockShell
	MockNetworkManager   *network.MockNetworkManager
	MockVirtualMachine   *virt.MockVirt
	MockContainerRuntime *virt.MockVirt
}

// setupSafeDownCmdMocks creates mock components for testing the down command
func setupSafeDownCmdMocks(optionalInjector ...di.Injector) MockSafeDownCmdComponents {
	var mockController *ctrl.MockController
	var injector di.Injector

	// Use the provided injector if passed, otherwise create a new one
	if len(optionalInjector) > 0 {
		injector = optionalInjector[0]
	} else {
		injector = di.NewInjector()
	}

	// Use the injector to create a mock controller
	mockController = ctrl.NewMockController(injector)

	// Manually override and set up components
	mockController.CreateCommonComponentsFunc = func() error {
		return nil
	}

	// Setup mock config handler
	mockConfigHandler := config.NewMockConfigHandler()
	mockConfigHandler.SetFunc = func(key string, value interface{}) error {
		return nil
	}
	mockConfigHandler.GetContextFunc = func() string {
		return "test-context"
	}
	mockConfigHandler.IsLoadedFunc = func() bool {
		return true
	}
	injector.Register("configHandler", mockConfigHandler)

	// Setup mock shell
	mockShell := shell.NewMockShell()
	injector.Register("shell", mockShell)

	// Setup mock network manager
	mockNetworkManager := network.NewMockNetworkManager()
	injector.Register("networkManager", mockNetworkManager)

	// Setup mock virtual machine
	mockVirtualMachine := virt.NewMockVirt()
	injector.Register("virtualMachine", mockVirtualMachine)

	// Setup mock container runtime
	mockContainerRuntime := virt.NewMockVirt()
	injector.Register("containerRuntime", mockContainerRuntime)

	return MockSafeDownCmdComponents{
		Injector:             injector,
		MockController:       mockController,
		MockConfigHandler:    mockConfigHandler,
		MockShell:            mockShell,
		MockNetworkManager:   mockNetworkManager,
		MockVirtualMachine:   mockVirtualMachine,
		MockContainerRuntime: mockContainerRuntime,
	}
}

func TestDownCmd(t *testing.T) {
	originalExitFunc := exitFunc
	exitFunc = mockExit
	t.Cleanup(func() {
		exitFunc = originalExitFunc
	})

	t.Run("Success", func(t *testing.T) {
		// Given a set of mock components
		mocks := setupSafeDownCmdMocks()

		// When the down command is executed
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"down"})
			if err := Execute(mocks.MockController); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
		})

		// Then the output should indicate success
		expectedOutput := "Windsor environment torn down successfully.\n"
		if output != expectedOutput {
			t.Errorf("Expected output %q, got %q", expectedOutput, output)
		}
	})

	t.Run("ErrorCreatingVirtualizationComponents", func(t *testing.T) {
		mocks := setupSafeDownCmdMocks()
		mocks.MockController.CreateVirtualizationComponentsFunc = func() error {
			return fmt.Errorf("Error creating virtualization components: %w", fmt.Errorf("error creating virtualization components"))
		}

		// Given a mock controller that returns an error when creating virtualization components
		rootCmd.SetArgs([]string{"down"})
		err := Execute(mocks.MockController)
		// Then the error should contain the expected message
		if err == nil || !strings.Contains(err.Error(), "Error creating virtualization components: error creating virtualization components") {
			t.Fatalf("Expected error containing 'Error creating virtualization components: error creating virtualization components', got %v", err)
		}
	})

	t.Run("ErrorInitializingComponents", func(t *testing.T) {
		mocks := setupSafeDownCmdMocks()
		mocks.MockController.InitializeComponentsFunc = func() error {
			return fmt.Errorf("Error initializing components: %w", fmt.Errorf("error initializing components"))
		}

		// Given a mock controller that returns an error when initializing components
		rootCmd.SetArgs([]string{"down"})
		err := Execute(mocks.MockController)
		// Then the error should contain the expected message
		if err == nil || !strings.Contains(err.Error(), "Error initializing components: error initializing components") {
			t.Fatalf("Expected error containing 'Error initializing components: error initializing components', got %v", err)
		}
	})

	t.Run("ErrorResolvingConfigHandler", func(t *testing.T) {
		mocks := setupSafeDownCmdMocks()
		// Allows for reaching the third call of the function
		callCount := 0
		mocks.MockController.ResolveConfigHandlerFunc = func() config.ConfigHandler {
			callCount++
			if callCount == 2 {
				return nil
			}
			return mocks.MockConfigHandler
		}

		// Given a mock controller that returns nil on the second call to ResolveConfigHandler
		rootCmd.SetArgs([]string{"down"})
		err := Execute(mocks.MockController)
		// Then the error should contain the expected message
		if err == nil || !strings.Contains(err.Error(), "No config handler found") {
			t.Fatalf("Expected error containing 'No config handler found', got %v", err)
		}
	})

	t.Run("ErrorResolvingContainerRuntime", func(t *testing.T) {
		mocks := setupSafeDownCmdMocks()
		mocks.MockController.ResolveContainerRuntimeFunc = func() virt.ContainerRuntime {
			return nil
		}

		// Given a mock controller that returns nil when resolving the container runtime
		rootCmd.SetArgs([]string{"down"})
		err := Execute(mocks.MockController)
		// Then the error should contain the expected message
		if err == nil || !strings.Contains(err.Error(), "No container runtime found") {
			t.Fatalf("Expected error containing 'No container runtime found', got %v", err)
		}
	})

	t.Run("ErrorRunningContainerRuntimeDown", func(t *testing.T) {
		mocks := setupSafeDownCmdMocks()
		mocks.MockController.ResolveContainerRuntimeFunc = func() virt.ContainerRuntime {
			mocks.MockContainerRuntime.DownFunc = func() error {
				return fmt.Errorf("Error running container runtime Down command: %w", fmt.Errorf("error running container runtime down"))
			}
			return mocks.MockContainerRuntime
		}

		// Given a mock container runtime that returns an error when running the Down command
		rootCmd.SetArgs([]string{"down"})
		err := Execute(mocks.MockController)
		// Then the error should contain the expected message
		if err == nil || !strings.Contains(err.Error(), "Error running container runtime Down command: error running container runtime down") {
			t.Fatalf("Expected error containing 'Error running container runtime Down command: error running container runtime down', got %v", err)
		}
	})

	t.Run("ErrorCleaningConfigArtifacts", func(t *testing.T) {
		mocks := setupSafeDownCmdMocks()
		mocks.MockConfigHandler.CleanFunc = func() error {
			return fmt.Errorf("Error cleaning up context specific artifacts: %w", fmt.Errorf("error cleaning context artifacts"))
		}

		// Given a mock context handler that returns an error when cleaning context specific artifacts
		rootCmd.SetArgs([]string{"down", "--clean"})
		err := Execute(mocks.MockController)
		// Then the error should contain the expected message
		if err == nil || !strings.Contains(err.Error(), "Error cleaning up context specific artifacts: error cleaning context artifacts") {
			t.Fatalf("Expected error containing 'Error cleaning up context specific artifacts: error cleaning context artifacts', got %v", err)
		}
	})

	t.Run("ErrorDeletingVolumes", func(t *testing.T) {
		mocks := setupSafeDownCmdMocks()
		mocks.MockShell.GetProjectRootFunc = func() (string, error) {
			return filepath.Join("mock", "project", "root"), nil
		}

		// Mock the osRemoveAll function to simulate an error when attempting to delete the .volumes folder
		originalOsRemoveAll := osRemoveAll
		defer func() { osRemoveAll = originalOsRemoveAll }()
		osRemoveAll = func(path string) error {
			if path == filepath.Join("mock", "project", "root", ".volumes") {
				return fmt.Errorf("Error deleting .volumes folder")
			}
			return nil
		}

		// Given a mock osRemoveAll that returns an error when deleting the .volumes folder
		rootCmd.SetArgs([]string{"down", "--clean"})
		err := Execute(mocks.MockController)
		// Then the error should contain the expected message
		if err == nil || !strings.Contains(err.Error(), "Error deleting .volumes folder") {
			t.Fatalf("Expected error containing 'Error deleting .volumes folder', got %v", err)
		}
	})

	t.Run("SuccessDeletingVolumes", func(t *testing.T) {
		mocks := setupSafeDownCmdMocks()
		mocks.MockShell.GetProjectRootFunc = func() (string, error) {
			return filepath.Join("mock", "project", "root"), nil
		}

		// Mock the shell's Exec function to simulate successful deletion of the .volumes folder
		mocks.MockShell.ExecFunc = func(command string, args ...string) (string, error) {
			if command == "cmd" && len(args) > 0 && args[0] == "/C" && args[1] == "rmdir" && args[2] == "/S" && args[3] == "/Q" && args[4] == filepath.Join("mock", "project", "root", ".volumes") {
				return "", nil
			}
			return "", fmt.Errorf("Unexpected command: %s %v", command, args)
		}

		// Given a mock shell that successfully deletes the .volumes folder
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"down", "--clean"})
			if err := Execute(mocks.MockController); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
		})

		// Then the output should indicate success
		expectedOutput := "Windsor environment torn down successfully.\n"
		if output != expectedOutput {
			t.Errorf("Expected output %q, got %q", expectedOutput, output)
		}
	})

	t.Run("ErrorGettingProjectRoot", func(t *testing.T) {
		mocks := setupSafeDownCmdMocks()
		callCount := 0
		mocks.MockShell.GetProjectRootFunc = func() (string, error) {
			callCount++
			if callCount == 2 {
				return "", fmt.Errorf("Error retrieving project root")
			}
			return filepath.Join("mock", "project", "root"), nil
		}

		rootCmd.SetArgs([]string{"down", "--clean"})
		err := Execute(mocks.MockController)
		if err == nil || !strings.Contains(err.Error(), "Error retrieving project root") {
			t.Fatalf("Expected error containing 'Error retrieving project root', got %v", err)
		}
	})

	t.Run("ErrorConfigNotLoaded", func(t *testing.T) {
		// Create mock components
		mocks := setupSafeDownCmdMocks()
		mocks.MockConfigHandler.IsLoadedFunc = func() bool { return false }

		// When: the down command is executed
		err := Execute(mocks.MockController)

		// Then: it should return an error indicating the configuration is not loaded
		if err == nil || !strings.Contains(err.Error(), "No configuration is loaded. Is there a project to tear down?") {
			t.Errorf("Expected error about configuration not loaded, got %v", err)
		}
	})

	t.Run("ErrorSettingEnvironmentVariables", func(t *testing.T) {
		// Given a mock controller that returns an error when setting environment variables
		mocks := setupSafeDownCmdMocks()
		mocks.MockController.SetEnvironmentVariablesFunc = func() error {
			return fmt.Errorf("mock environment variables error")
		}

		// When the down command is executed
		rootCmd.SetArgs([]string{"down"})
		err := Execute(mocks.MockController)

		// Then the error should contain the expected message
		if err == nil || !strings.Contains(err.Error(), "Error setting environment variables: mock environment variables error") {
			t.Fatalf("Expected error containing 'Error setting environment variables: mock environment variables error', got %v", err)
		}
	})

	t.Run("SuccessSettingEnvironmentVariables", func(t *testing.T) {
		// Given a mock controller where SetEnvironmentVariables is successful
		mocks := setupSafeDownCmdMocks()
		setEnvVarsCalled := false
		mocks.MockController.SetEnvironmentVariablesFunc = func() error {
			setEnvVarsCalled = true
			return nil
		}

		// When the down command is executed
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"down"})
			if err := Execute(mocks.MockController); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
		})

		// Then SetEnvironmentVariables should have been called
		if !setEnvVarsCalled {
			t.Fatal("Expected SetEnvironmentVariables to be called, but it wasn't")
		}

		// And the output should indicate success
		expectedOutput := "Windsor environment torn down successfully.\n"
		if output != expectedOutput {
			t.Errorf("Expected output %q, got %q", expectedOutput, output)
		}
	})
}
