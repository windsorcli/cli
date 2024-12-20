package cmd

import (
	"fmt"
	"strings"
	"testing"

	"github.com/windsorcli/cli/internal/config"
	"github.com/windsorcli/cli/internal/context"
	"github.com/windsorcli/cli/internal/network"

	ctrl "github.com/windsorcli/cli/internal/controller"
	"github.com/windsorcli/cli/internal/di"
	"github.com/windsorcli/cli/internal/shell"
	"github.com/windsorcli/cli/internal/virt"
)

type MockSafeDownCmdComponents struct {
	Injector             di.Injector
	MockController       *ctrl.MockController
	MockContextHandler   *context.MockContext
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
		MockContextHandler:   mockContextHandler,
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
		output := captureStdout(func() {
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
		// Allows for reaching the second call of the function
		callCount := 0
		mocks.MockController.ResolveConfigHandlerFunc = func() config.ConfigHandler {
			callCount++
			if callCount == 2 {
				return nil
			}
			return config.NewMockConfigHandler()
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
			mockCR := virt.NewMockVirt()
			mockCR.DownFunc = func() error {
				return fmt.Errorf("Error running container runtime Down command: %w", fmt.Errorf("error running container runtime down"))
			}
			return mockCR
		}

		// Given a mock container runtime that returns an error when running the Down command
		rootCmd.SetArgs([]string{"down"})
		err := Execute(mocks.MockController)
		// Then the error should contain the expected message
		if err == nil || !strings.Contains(err.Error(), "Error running container runtime Down command: error running container runtime down") {
			t.Fatalf("Expected error containing 'Error running container runtime Down command: error running container runtime down', got %v", err)
		}
	})

	t.Run("ErrorCleaningContextArtifacts", func(t *testing.T) {
		mocks := setupSafeDownCmdMocks()
		mocks.MockController.ResolveContextHandlerFunc = func() context.ContextHandler {
			mockContextHandler := context.NewMockContext()
			mockContextHandler.CleanFunc = func() error {
				return fmt.Errorf("Error cleaning up context specific artifacts: %w", fmt.Errorf("error cleaning context artifacts"))
			}
			return mockContextHandler
		}

		// Given a mock context handler that returns an error when cleaning context specific artifacts
		rootCmd.SetArgs([]string{"down", "--clean"})
		err := Execute(mocks.MockController)
		// Then the error should contain the expected message
		if err == nil || !strings.Contains(err.Error(), "Error cleaning up context specific artifacts: error cleaning context artifacts") {
			t.Fatalf("Expected error containing 'Error cleaning up context specific artifacts: error cleaning context artifacts', got %v", err)
		}
	})
}
