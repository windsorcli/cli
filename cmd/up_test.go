package cmd

import (
	"fmt"
	"strings"
	"testing"

	"github.com/windsorcli/cli/internal/config"
	"github.com/windsorcli/cli/internal/context"
	"github.com/windsorcli/cli/internal/network"
	"github.com/windsorcli/cli/internal/stack"

	ctrl "github.com/windsorcli/cli/internal/controller"
	"github.com/windsorcli/cli/internal/di"
	"github.com/windsorcli/cli/internal/shell"
	"github.com/windsorcli/cli/internal/virt"
)

type MockSafeUpCmdComponents struct {
	Injector             di.Injector
	MockController       *ctrl.MockController
	MockContextHandler   *context.MockContext
	MockConfigHandler    *config.MockConfigHandler
	MockShell            *shell.MockShell
	MockNetworkManager   *network.MockNetworkManager
	MockVirtualMachine   *virt.MockVirt
	MockContainerRuntime *virt.MockVirt
}

// setupSafeUpCmdMocks creates mock components for testing the up command
func setupSafeUpCmdMocks(optionalInjector ...di.Injector) MockSafeUpCmdComponents {
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

	return MockSafeUpCmdComponents{
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

func TestUpCmd(t *testing.T) {
	originalExitFunc := exitFunc
	exitFunc = mockExit
	t.Cleanup(func() {
		exitFunc = originalExitFunc
	})

	t.Run("Success", func(t *testing.T) {
		// Given a set of mock components
		mocks := setupSafeUpCmdMocks()

		// When the up command is executed
		output := captureStdout(func() {
			rootCmd.SetArgs([]string{"up"})
			if err := Execute(mocks.MockController); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
		})

		// Then the output should indicate success
		expectedOutput := "Windsor environment set up successfully.\n"
		if output != expectedOutput {
			t.Errorf("Expected output %q, got %q", expectedOutput, output)
		}
	})

	t.Run("ErrorCreatingEnvComponents", func(t *testing.T) {
		mocks := setupSafeUpCmdMocks()
		mocks.MockController.CreateEnvComponentsFunc = func() error {
			return fmt.Errorf("Error creating environment components: %w", fmt.Errorf("error creating environment components"))
		}

		// Given a mock controller that returns an error when creating environment components
		rootCmd.SetArgs([]string{"up"})
		err := Execute(mocks.MockController)
		// Then the error should contain the expected message
		if err == nil || !strings.Contains(err.Error(), "Error creating environment components: error creating environment components") {
			t.Fatalf("Expected error containing 'Error creating environment components: error creating environment components', got %v", err)
		}
	})

	t.Run("ErrorCreatingServiceComponents", func(t *testing.T) {
		mocks := setupSafeUpCmdMocks()
		mocks.MockController.CreateServiceComponentsFunc = func() error {
			return fmt.Errorf("Error creating service components: %w", fmt.Errorf("error creating service components"))
		}

		// Given a mock controller that returns an error when creating service components
		rootCmd.SetArgs([]string{"up"})
		err := Execute(mocks.MockController)
		// Then the error should contain the expected message
		if err == nil || !strings.Contains(err.Error(), "Error creating service components: error creating service components") {
			t.Fatalf("Expected error containing 'Error creating service components: error creating service components', got %v", err)
		}
	})

	t.Run("ErrorCreatingVirtualizationComponents", func(t *testing.T) {
		mocks := setupSafeUpCmdMocks()
		mocks.MockController.CreateVirtualizationComponentsFunc = func() error {
			return fmt.Errorf("Error creating virtualization components: %w", fmt.Errorf("error creating virtualization components"))
		}

		// Given a mock controller that returns an error when creating virtualization components
		rootCmd.SetArgs([]string{"up"})
		err := Execute(mocks.MockController)
		// Then the error should contain the expected message
		if err == nil || !strings.Contains(err.Error(), "Error creating virtualization components: error creating virtualization components") {
			t.Fatalf("Expected error containing 'Error creating virtualization components: error creating virtualization components', got %v", err)
		}
	})

	t.Run("ErrorCreatingStackComponents", func(t *testing.T) {
		mocks := setupSafeUpCmdMocks()
		mocks.MockController.CreateStackComponentsFunc = func() error {
			return fmt.Errorf("Error creating stack components: %w", fmt.Errorf("error creating stack components"))
		}

		// Given a mock controller that returns an error when creating stack components
		rootCmd.SetArgs([]string{"up"})
		err := Execute(mocks.MockController)
		// Then the error should contain the expected message
		if err == nil || !strings.Contains(err.Error(), "Error creating stack components: error creating stack components") {
			t.Fatalf("Expected error containing 'Error creating stack components: error creating stack components', got %v", err)
		}
	})

	t.Run("ErrorInitializingComponents", func(t *testing.T) {
		mocks := setupSafeUpCmdMocks()
		mocks.MockController.InitializeComponentsFunc = func() error {
			return fmt.Errorf("Error initializing components: %w", fmt.Errorf("error initializing components"))
		}

		// Given a mock controller that returns an error when initializing components
		rootCmd.SetArgs([]string{"up"})
		err := Execute(mocks.MockController)
		// Then the error should contain the expected message
		if err == nil || !strings.Contains(err.Error(), "Error initializing components: error initializing components") {
			t.Fatalf("Expected error containing 'Error initializing components: error initializing components', got %v", err)
		}
	})

	t.Run("ErrorWritingConfigurationFiles", func(t *testing.T) {
		mocks := setupSafeUpCmdMocks()
		mocks.MockController.WriteConfigurationFilesFunc = func() error {
			return fmt.Errorf("Error writing configuration files: %w", fmt.Errorf("error writing configuration files"))
		}

		// Given a mock controller that returns an error when writing configuration files
		rootCmd.SetArgs([]string{"up"})
		err := Execute(mocks.MockController)
		// Then the error should contain the expected message
		if err == nil || !strings.Contains(err.Error(), "Error writing configuration files: error writing configuration files") {
			t.Fatalf("Expected error containing 'Error writing configuration files: error writing configuration files', got %v", err)
		}
	})

	t.Run("ErrorResolvingConfigHandler", func(t *testing.T) {
		mocks := setupSafeUpCmdMocks()
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
		rootCmd.SetArgs([]string{"up"})
		err := Execute(mocks.MockController)
		// Then the error should contain the expected message
		if err == nil || !strings.Contains(err.Error(), "No config handler found") {
			t.Fatalf("Expected error containing 'No config handler found', got %v", err)
		}
	})

	t.Run("ErrorResolvingVirtualMachine", func(t *testing.T) {
		mocks := setupSafeUpCmdMocks()
		mocks.MockController.ResolveVirtualMachineFunc = func() virt.VirtualMachine {
			return nil
		}

		// Given a mock controller that returns nil when resolving the virtual machine
		rootCmd.SetArgs([]string{"up"})
		err := Execute(mocks.MockController)
		// Then the error should contain the expected message
		if err == nil || !strings.Contains(err.Error(), "No virtual machine found") {
			t.Fatalf("Expected error containing 'No virtual machine found', got %v", err)
		}
	})

	t.Run("ErrorRunningVirtualMachineUp", func(t *testing.T) {
		mocks := setupSafeUpCmdMocks()
		mocks.MockController.ResolveVirtualMachineFunc = func() virt.VirtualMachine {
			mockVM := virt.NewMockVirt()
			mockVM.UpFunc = func(verbose ...bool) error {
				return fmt.Errorf("Error running virtual machine Up command: %w", fmt.Errorf("error running VM up"))
			}
			return mockVM
		}

		// Given a mock virtual machine that returns an error when running the Up command
		rootCmd.SetArgs([]string{"up"})
		err := Execute(mocks.MockController)
		// Then the error should contain the expected message
		if err == nil || !strings.Contains(err.Error(), "Error running virtual machine Up command: error running VM up") {
			t.Fatalf("Expected error containing 'Error running virtual machine Up command: error running VM up', got %v", err)
		}
	})

	t.Run("ErrorResolvingContainerRuntime", func(t *testing.T) {
		mocks := setupSafeUpCmdMocks()
		mocks.MockController.ResolveContainerRuntimeFunc = func() virt.ContainerRuntime {
			return nil
		}

		// Given a mock controller that returns nil when resolving the container runtime
		rootCmd.SetArgs([]string{"up"})
		err := Execute(mocks.MockController)
		// Then the error should contain the expected message
		if err == nil || !strings.Contains(err.Error(), "No container runtime found") {
			t.Fatalf("Expected error containing 'No container runtime found', got %v", err)
		}
	})

	t.Run("ErrorRunningContainerRuntimeUp", func(t *testing.T) {
		mocks := setupSafeUpCmdMocks()
		mocks.MockController.ResolveContainerRuntimeFunc = func() virt.ContainerRuntime {
			mockCR := virt.NewMockVirt()
			mockCR.UpFunc = func(verbose ...bool) error {
				return fmt.Errorf("Error running container runtime Up command: %w", fmt.Errorf("error running container runtime up"))
			}
			return mockCR
		}

		// Given a mock container runtime that returns an error when running the Up command
		rootCmd.SetArgs([]string{"up"})
		err := Execute(mocks.MockController)
		// Then the error should contain the expected message
		if err == nil || !strings.Contains(err.Error(), "Error running container runtime Up command: error running container runtime up") {
			t.Fatalf("Expected error containing 'Error running container runtime Up command: error running container runtime up', got %v", err)
		}
	})

	t.Run("ErrorResolvingNetworkManager", func(t *testing.T) {
		mocks := setupSafeUpCmdMocks()
		mocks.MockController.ResolveNetworkManagerFunc = func() network.NetworkManager {
			return nil
		}

		// Given a mock controller that returns nil when resolving the network manager
		rootCmd.SetArgs([]string{"up"})
		err := Execute(mocks.MockController)
		// Then the error should contain the expected message
		if err == nil || !strings.Contains(err.Error(), "No network manager found") {
			t.Fatalf("Expected error containing 'No network manager found', got %v", err)
		}
	})

	t.Run("ErrorConfiguringGuestNetwork", func(t *testing.T) {
		mocks := setupSafeUpCmdMocks()
		mocks.MockController.ResolveNetworkManagerFunc = func() network.NetworkManager {
			mockNM := network.NewMockNetworkManager()
			mockNM.ConfigureGuestFunc = func() error {
				return fmt.Errorf("Error configuring guest network: %w", fmt.Errorf("error configuring guest network"))
			}
			return mockNM
		}

		// Given a mock network manager that returns an error when configuring the guest network
		rootCmd.SetArgs([]string{"up"})
		err := Execute(mocks.MockController)
		// Then the error should contain the expected message
		if err == nil || !strings.Contains(err.Error(), "Error configuring guest network") {
			t.Fatalf("Expected error containing 'Error configuring guest network', got %v", err)
		}
	})

	t.Run("ErrorConfiguringHostRoute", func(t *testing.T) {
		mocks := setupSafeUpCmdMocks()
		mocks.MockController.ResolveNetworkManagerFunc = func() network.NetworkManager {
			mockNM := network.NewMockNetworkManager()
			mockNM.ConfigureHostRouteFunc = func() error {
				return fmt.Errorf("Error configuring host network: %w", fmt.Errorf("error configuring host route"))
			}
			return mockNM
		}

		// Given a mock network manager that returns an error when configuring the host route
		rootCmd.SetArgs([]string{"up"})
		err := Execute(mocks.MockController)
		// Then the error should contain the expected message
		if err == nil || !strings.Contains(err.Error(), "Error configuring host network: error configuring host route") {
			t.Fatalf("Expected error containing 'Error configuring host network: error configuring host route', got %v", err)
		}
	})

	t.Run("ErrorConfiguringDNS", func(t *testing.T) {
		mocks := setupSafeUpCmdMocks()
		mocks.MockController.ResolveNetworkManagerFunc = func() network.NetworkManager {
			mockNM := network.NewMockNetworkManager()
			mockNM.ConfigureDNSFunc = func() error {
				return fmt.Errorf("Error configuring DNS: %w", fmt.Errorf("error configuring DNS"))
			}
			return mockNM
		}

		// Given a mock network manager that returns an error when configuring DNS
		rootCmd.SetArgs([]string{"up"})
		err := Execute(mocks.MockController)
		// Then the error should contain the expected message
		if err == nil || !strings.Contains(err.Error(), "Error configuring DNS") {
			t.Fatalf("Expected error containing 'Error configuring DNS', got %v", err)
		}
	})

	t.Run("NoStackFound", func(t *testing.T) {
		mocks := setupSafeUpCmdMocks()
		mocks.MockController.ResolveStackFunc = func() stack.Stack {
			return nil
		}

		// Given a mock controller that returns nil when resolving the stack
		rootCmd.SetArgs([]string{"up"})
		err := Execute(mocks.MockController)
		// Then the error should contain the expected message
		if err == nil || !strings.Contains(err.Error(), "No stack found") {
			t.Fatalf("Expected error containing 'No stack found', got %v", err)
		}
	})

	t.Run("ErrorRunningStackUp", func(t *testing.T) {
		mocks := setupSafeUpCmdMocks()
		injector := mocks.Injector
		mocks.MockController.ResolveStackFunc = func() stack.Stack {
			mockStack := stack.NewMockStack(injector)
			mockStack.UpFunc = func() error {
				return fmt.Errorf("Error running stack Up command: %w", fmt.Errorf("error running stack up"))
			}
			return mockStack
		}

		// Given a mock stack that returns an error when running the Up command
		rootCmd.SetArgs([]string{"up"})
		err := Execute(mocks.MockController)
		// Then the error should contain the expected message
		if err == nil || !strings.Contains(err.Error(), "Error running stack Up command: error running stack up") {
			t.Fatalf("Expected error containing 'Error running stack Up command: error running stack up', got %v", err)
		}
	})
}
