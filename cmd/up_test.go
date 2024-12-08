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

// setupSafeUpCmdMocks returns a mock controller with safe mocks for the up command
func setupSafeUpCmdMocks(existingControllers ...ctrl.Controller) *ctrl.MockController {
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

	// Setup mock network manager
	mockNetworkManager := network.NewMockNetworkManager()
	injector.Register("networkManager", mockNetworkManager)

	// Setup mock virtual machine
	mockVirtualMachine := virt.NewMockVirt()
	injector.Register("virtualMachine", mockVirtualMachine)

	// Setup mock container runtime
	mockContainerRuntime := virt.NewMockVirt()
	injector.Register("containerRuntime", mockContainerRuntime)

	return mockController
}

func TestUpCmd(t *testing.T) {
	originalExitFunc := exitFunc
	exitFunc = mockExit
	t.Cleanup(func() {
		exitFunc = originalExitFunc
	})

	t.Run("Success", func(t *testing.T) {
		mockController := setupSafeUpCmdMocks()

		// Execute the up command and capture output
		output := captureStdout(func() {
			rootCmd.SetArgs([]string{"up"})
			if err := Execute(mockController); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
		})

		// Validate the output
		expectedOutput := "Windsor environment set up successfully.\n"
		if output != expectedOutput {
			t.Errorf("Expected output %q, got %q", expectedOutput, output)
		}
	})

	t.Run("ErrorCreatingServiceComponents", func(t *testing.T) {
		mockController := setupSafeUpCmdMocks()
		mockController.CreateServiceComponentsFunc = func() error {
			return fmt.Errorf("Error creating service components: %w", fmt.Errorf("error creating service components"))
		}

		rootCmd.SetArgs([]string{"up"})
		err := Execute(mockController)
		if err == nil || !strings.Contains(err.Error(), "Error creating service components: error creating service components") {
			t.Fatalf("Expected error containing 'Error creating service components: error creating service components', got %v", err)
		}
	})

	t.Run("ErrorCreatingCommonComponents", func(t *testing.T) {
		mockController := setupSafeUpCmdMocks()
		mockController.CreateCommonComponentsFunc = func() error {
			return fmt.Errorf("Error creating services components: %w", fmt.Errorf("error creating common components"))
		}

		rootCmd.SetArgs([]string{"up"})
		err := Execute(mockController)
		if err == nil || !strings.Contains(err.Error(), "Error creating services components: error creating common components") {
			t.Fatalf("Expected error containing 'Error creating services components: error creating common components', got %v", err)
		}
	})

	t.Run("ErrorCreatingVirtualizationComponents", func(t *testing.T) {
		mockController := setupSafeUpCmdMocks()
		mockController.CreateVirtualizationComponentsFunc = func() error {
			return fmt.Errorf("Error creating virtualization components: %w", fmt.Errorf("error creating virtualization components"))
		}

		rootCmd.SetArgs([]string{"up"})
		err := Execute(mockController)
		if err == nil || !strings.Contains(err.Error(), "Error creating virtualization components: error creating virtualization components") {
			t.Fatalf("Expected error containing 'Error creating virtualization components: error creating virtualization components', got %v", err)
		}
	})

	t.Run("ErrorInitializingComponents", func(t *testing.T) {
		mockController := setupSafeUpCmdMocks()
		mockController.InitializeComponentsFunc = func() error {
			return fmt.Errorf("Error initializing components: %w", fmt.Errorf("error initializing components"))
		}

		rootCmd.SetArgs([]string{"up"})
		err := Execute(mockController)
		if err == nil || !strings.Contains(err.Error(), "Error initializing components: error initializing components") {
			t.Fatalf("Expected error containing 'Error initializing components: error initializing components', got %v", err)
		}
	})

	t.Run("ErrorWritingConfigurationFiles", func(t *testing.T) {
		mockController := setupSafeUpCmdMocks()
		mockController.WriteConfigurationFilesFunc = func() error {
			return fmt.Errorf("Error writing configuration files: %w", fmt.Errorf("error writing configuration files"))
		}

		rootCmd.SetArgs([]string{"up"})
		err := Execute(mockController)
		if err == nil || !strings.Contains(err.Error(), "Error writing configuration files: error writing configuration files") {
			t.Fatalf("Expected error containing 'Error writing configuration files: error writing configuration files', got %v", err)
		}
	})

	t.Run("ErrorResolvingConfigHandler", func(t *testing.T) {
		mockController := setupSafeUpCmdMocks()
		// Allows for reaching the second call of the function
		callCount := 0
		mockController.ResolveConfigHandlerFunc = func() config.ConfigHandler {
			callCount++
			if callCount == 2 {
				return nil
			}
			return config.NewMockConfigHandler()
		}

		rootCmd.SetArgs([]string{"up"})
		err := Execute(mockController)
		if err == nil || !strings.Contains(err.Error(), "No config handler found") {
			t.Fatalf("Expected error containing 'No config handler found', got %v", err)
		}
	})

	t.Run("ErrorResolvingVirtualMachine", func(t *testing.T) {
		mockController := setupSafeUpCmdMocks()
		mockController.ResolveVirtualMachineFunc = func() virt.VirtualMachine {
			return nil
		}

		rootCmd.SetArgs([]string{"up"})
		err := Execute(mockController)
		if err == nil || !strings.Contains(err.Error(), "No virtual machine found") {
			t.Fatalf("Expected error containing 'No virtual machine found', got %v", err)
		}
	})

	t.Run("ErrorRunningVirtualMachineUp", func(t *testing.T) {
		mockController := setupSafeUpCmdMocks()
		mockController.ResolveVirtualMachineFunc = func() virt.VirtualMachine {
			mockVM := virt.NewMockVirt()
			mockVM.UpFunc = func(verbose ...bool) error {
				return fmt.Errorf("Error running virtual machine Up command: %w", fmt.Errorf("error running VM up"))
			}
			return mockVM
		}

		rootCmd.SetArgs([]string{"up"})
		err := Execute(mockController)
		if err == nil || !strings.Contains(err.Error(), "Error running virtual machine Up command: error running VM up") {
			t.Fatalf("Expected error containing 'Error running virtual machine Up command: error running VM up', got %v", err)
		}
	})

	t.Run("ErrorResolvingContainerRuntime", func(t *testing.T) {
		mockController := setupSafeUpCmdMocks()
		mockController.ResolveContainerRuntimeFunc = func() virt.ContainerRuntime {
			return nil
		}

		rootCmd.SetArgs([]string{"up"})
		err := Execute(mockController)
		if err == nil || !strings.Contains(err.Error(), "No container runtime found") {
			t.Fatalf("Expected error containing 'No container runtime found', got %v", err)
		}
	})

	t.Run("ErrorRunningContainerRuntimeUp", func(t *testing.T) {
		mockController := setupSafeUpCmdMocks()
		mockController.ResolveContainerRuntimeFunc = func() virt.ContainerRuntime {
			mockCR := virt.NewMockVirt()
			mockCR.UpFunc = func(verbose ...bool) error {
				return fmt.Errorf("Error running container runtime Up command: %w", fmt.Errorf("error running container runtime up"))
			}
			return mockCR
		}

		rootCmd.SetArgs([]string{"up"})
		err := Execute(mockController)
		if err == nil || !strings.Contains(err.Error(), "Error running container runtime Up command: error running container runtime up") {
			t.Fatalf("Expected error containing 'Error running container runtime Up command: error running container runtime up', got %v", err)
		}
	})

	t.Run("ErrorResolvingNetworkManager", func(t *testing.T) {
		mockController := setupSafeUpCmdMocks()
		mockController.ResolveNetworkManagerFunc = func() network.NetworkManager {
			return nil
		}

		rootCmd.SetArgs([]string{"up"})
		err := Execute(mockController)
		if err == nil || !strings.Contains(err.Error(), "No network manager found") {
			t.Fatalf("Expected error containing 'No network manager found', got %v", err)
		}
	})

	t.Run("ErrorConfiguringGuestNetwork", func(t *testing.T) {
		mockController := setupSafeUpCmdMocks()
		mockController.ResolveNetworkManagerFunc = func() network.NetworkManager {
			mockNM := network.NewMockNetworkManager()
			mockNM.ConfigureGuestFunc = func() error {
				return fmt.Errorf("Error configuring guest network: %w", fmt.Errorf("error configuring guest network"))
			}
			return mockNM
		}

		rootCmd.SetArgs([]string{"up"})
		err := Execute(mockController)
		if err == nil || !strings.Contains(err.Error(), "Error configuring guest network") {
			t.Fatalf("Expected error containing 'Error configuring guest network', got %v", err)
		}
	})

	t.Run("ErrorConfiguringHostRoute", func(t *testing.T) {
		mockController := setupSafeUpCmdMocks()
		mockController.ResolveNetworkManagerFunc = func() network.NetworkManager {
			mockNM := network.NewMockNetworkManager()
			mockNM.ConfigureHostRouteFunc = func() error {
				return fmt.Errorf("Error configuring host network: %w", fmt.Errorf("error configuring host route"))
			}
			return mockNM
		}

		rootCmd.SetArgs([]string{"up"})
		err := Execute(mockController)
		if err == nil || !strings.Contains(err.Error(), "Error configuring host network: error configuring host route") {
			t.Fatalf("Expected error containing 'Error configuring host network: error configuring host route', got %v", err)
		}
	})

	t.Run("ErrorConfiguringDNS", func(t *testing.T) {
		mockController := setupSafeUpCmdMocks()
		mockController.ResolveNetworkManagerFunc = func() network.NetworkManager {
			mockNM := network.NewMockNetworkManager()
			mockNM.ConfigureDNSFunc = func() error {
				return fmt.Errorf("Error configuring DNS: %w", fmt.Errorf("error configuring DNS"))
			}
			return mockNM
		}

		rootCmd.SetArgs([]string{"up"})
		err := Execute(mockController)
		if err == nil || !strings.Contains(err.Error(), "Error configuring DNS") {
			t.Fatalf("Expected error containing 'Error configuring DNS', got %v", err)
		}
	})
}
