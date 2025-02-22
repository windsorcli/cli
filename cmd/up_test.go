package cmd

import (
	"fmt"
	"strings"
	"testing"

	bp "github.com/windsorcli/cli/pkg/blueprint"
	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/network"
	"github.com/windsorcli/cli/pkg/secrets"
	"github.com/windsorcli/cli/pkg/stack"
	"github.com/windsorcli/cli/pkg/tools"

	ctrl "github.com/windsorcli/cli/pkg/controller"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
	"github.com/windsorcli/cli/pkg/virt"
)

type SafeUpCmdComponents struct {
	Injector         di.Injector
	Controller       *ctrl.MockController
	ConfigHandler    *config.MockConfigHandler
	Shell            *shell.MockShell
	NetworkManager   *network.MockNetworkManager
	ToolsManager     *tools.MockToolsManager
	VirtualMachine   *virt.MockVirt
	ContainerRuntime *virt.MockVirt
}

// setupSafeUpCmdMocks creates mock components for testing the up command
func setupSafeUpCmdMocks(optionalInjector ...di.Injector) SafeUpCmdComponents {
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
	mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
		if key == "projectName" {
			return "mockProjectName"
		}
		if key == "vm.driver" {
			return "colima"
		}
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return ""
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

	// Setup mock tools manager
	mockToolsManager := tools.NewMockToolsManager()
	injector.Register("toolsManager", mockToolsManager)

	return SafeUpCmdComponents{
		Injector:         injector,
		Controller:       mockController,
		ConfigHandler:    mockConfigHandler,
		Shell:            mockShell,
		NetworkManager:   mockNetworkManager,
		ToolsManager:     mockToolsManager,
		VirtualMachine:   mockVirtualMachine,
		ContainerRuntime: mockContainerRuntime,
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
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"up"})
			if err := Execute(mocks.Controller); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
		})

		// Then the output should indicate success
		expectedOutput := "Windsor environment set up successfully.\n"
		if output != expectedOutput {
			t.Errorf("Expected output %q, got %q", expectedOutput, output)
		}
	})

	t.Run("NoProjectNameSet", func(t *testing.T) {
		// Given a mock controller that returns an empty projectName
		mocks := setupSafeUpCmdMocks()
		mocks.Controller.ResolveConfigHandlerFunc = func() config.ConfigHandler {
			mockConfigHandler := config.NewMockConfigHandler()
			mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
				return "" // empty
			}
			return mockConfigHandler
		}

		// When the "up" command is executed
		output := captureStdout(func() {
			rootCmd.SetArgs([]string{"up"})
			_ = Execute(mocks.Controller)
		})

		// Then the output should contain the new message
		expected := "Cannot set up environment. Please run `windsor init` to set up your project first."
		if !strings.Contains(output, expected) {
			t.Errorf("Expected to contain %q, got %q", expected, output)
		}
	})

	t.Run("ErrorCreatingProjectComponents", func(t *testing.T) {
		mocks := setupSafeUpCmdMocks()
		mocks.Controller.CreateProjectComponentsFunc = func() error {
			return fmt.Errorf("error creating project components")
		}

		// Given a mock controller that returns an error when creating project components
		rootCmd.SetArgs([]string{"up"})
		err := Execute(mocks.Controller)
		// Then the error should contain the expected message
		if err == nil || !strings.Contains(err.Error(), "error creating project components") {
			t.Fatalf("Expected error containing 'error creating project components', got %v", err)
		}
	})

	t.Run("ErrorCreatingEnvComponents", func(t *testing.T) {
		mocks := setupSafeUpCmdMocks()
		mocks.Controller.CreateEnvComponentsFunc = func() error {
			return fmt.Errorf("Error creating environment components: %w", fmt.Errorf("error creating environment components"))
		}

		// Given a mock controller that returns an error when creating environment components
		rootCmd.SetArgs([]string{"up"})
		err := Execute(mocks.Controller)
		// Then the error should contain the expected message
		if err == nil || !strings.Contains(err.Error(), "Error creating environment components: error creating environment components") {
			t.Fatalf("Expected error containing 'Error creating environment components: error creating environment components', got %v", err)
		}
	})

	t.Run("ErrorCreatingServiceComponents", func(t *testing.T) {
		mocks := setupSafeUpCmdMocks()
		mocks.Controller.CreateServiceComponentsFunc = func() error {
			return fmt.Errorf("Error creating service components: %w", fmt.Errorf("error creating service components"))
		}

		// Given a mock controller that returns an error when creating service components
		rootCmd.SetArgs([]string{"up"})
		err := Execute(mocks.Controller)
		// Then the error should contain the expected message
		if err == nil || !strings.Contains(err.Error(), "Error creating service components: error creating service components") {
			t.Fatalf("Expected error containing 'Error creating service components: error creating service components', got %v", err)
		}
	})

	t.Run("ErrorCreatingVirtualizationComponents", func(t *testing.T) {
		mocks := setupSafeUpCmdMocks()
		mocks.Controller.CreateVirtualizationComponentsFunc = func() error {
			return fmt.Errorf("Error creating virtualization components: %w", fmt.Errorf("error creating virtualization components"))
		}

		// Given a mock controller that returns an error when creating virtualization components
		rootCmd.SetArgs([]string{"up"})
		err := Execute(mocks.Controller)
		// Then the error should contain the expected message
		if err == nil || !strings.Contains(err.Error(), "Error creating virtualization components: error creating virtualization components") {
			t.Fatalf("Expected error containing 'Error creating virtualization components: error creating virtualization components', got %v", err)
		}
	})

	t.Run("ErrorCreatingStackComponents", func(t *testing.T) {
		mocks := setupSafeUpCmdMocks()
		mocks.Controller.CreateStackComponentsFunc = func() error {
			return fmt.Errorf("Error creating stack components: %w", fmt.Errorf("error creating stack components"))
		}

		// Given a mock controller that returns an error when creating stack components
		rootCmd.SetArgs([]string{"up"})
		err := Execute(mocks.Controller)
		// Then the error should contain the expected message
		if err == nil || !strings.Contains(err.Error(), "Error creating stack components: error creating stack components") {
			t.Fatalf("Expected error containing 'Error creating stack components: error creating stack components', got %v", err)
		}
	})

	t.Run("ErrorInitializingComponents", func(t *testing.T) {
		mocks := setupSafeUpCmdMocks()
		mocks.Controller.InitializeComponentsFunc = func() error {
			return fmt.Errorf("Error initializing components: %w", fmt.Errorf("error initializing components"))
		}

		// Given a mock controller that returns an error when initializing components
		rootCmd.SetArgs([]string{"up"})
		err := Execute(mocks.Controller)
		// Then the error should contain the expected message
		if err == nil || !strings.Contains(err.Error(), "Error initializing components: error initializing components") {
			t.Fatalf("Expected error containing 'Error initializing components: error initializing components', got %v", err)
		}
	})

	t.Run("ErrorWritingConfigurationFiles", func(t *testing.T) {
		mocks := setupSafeUpCmdMocks()
		mocks.Controller.WriteConfigurationFilesFunc = func() error {
			return fmt.Errorf("Error writing configuration files: %w", fmt.Errorf("error writing configuration files"))
		}

		// Given a mock controller that returns an error when writing configuration files
		rootCmd.SetArgs([]string{"up"})
		err := Execute(mocks.Controller)
		// Then the error should contain the expected message
		if err == nil || !strings.Contains(err.Error(), "Error writing configuration files: error writing configuration files") {
			t.Fatalf("Expected error containing 'Error writing configuration files: error writing configuration files', got %v", err)
		}
	})

	t.Run("ErrorResolvingConfigHandler", func(t *testing.T) {
		mocks := setupSafeUpCmdMocks()
		// Allows for reaching the second call of the function
		callCount := 0
		mocks.Controller.ResolveConfigHandlerFunc = func() config.ConfigHandler {
			callCount++
			if callCount == 2 {
				return nil
			}
			return mocks.ConfigHandler
		}

		// Given a mock controller that returns nil on the second call to ResolveConfigHandler
		rootCmd.SetArgs([]string{"up"})
		err := Execute(mocks.Controller)
		// Then the error should contain the expected message
		if err == nil || !strings.Contains(err.Error(), "No config handler found") {
			t.Fatalf("Expected error containing 'No config handler found', got %v", err)
		}
	})

	t.Run("ErrorInstallingTools", func(t *testing.T) {
		mocks := setupSafeUpCmdMocks()
		mocks.Controller.ResolveToolsManagerFunc = func() tools.ToolsManager {
			return &tools.MockToolsManager{
				InstallFunc: func() error {
					return fmt.Errorf("error installing tools")
				},
			}
		}

		// Given a mock controller that returns a mock tools manager with an error when installing tools
		rootCmd.SetArgs([]string{"up"})
		err := Execute(mocks.Controller)
		// Then the error should contain the expected message
		if err == nil || !strings.Contains(err.Error(), "Error installing tools: error installing tools") {
			t.Fatalf("Expected error containing 'Error installing tools: error installing tools', got %v", err)
		}
	})

	t.Run("ErrorResolvingContainerRuntime", func(t *testing.T) {
		mocks := setupSafeUpCmdMocks()
		mocks.Controller.ResolveContainerRuntimeFunc = func() virt.ContainerRuntime {
			return nil
		}

		// Given a mock controller that returns nil when resolving the container runtime
		rootCmd.SetArgs([]string{"up"})
		err := Execute(mocks.Controller)
		// Then the error should contain the expected message
		if err == nil || !strings.Contains(err.Error(), "No container runtime found") {
			t.Fatalf("Expected error containing 'No container runtime found', got %v", err)
		}
	})

	t.Run("ErrorRunningContainerRuntimeUp", func(t *testing.T) {
		mocks := setupSafeUpCmdMocks()
		mocks.Controller.ResolveContainerRuntimeFunc = func() virt.ContainerRuntime {
			mockCR := virt.NewMockVirt()
			mockCR.UpFunc = func(verbose ...bool) error {
				return fmt.Errorf("Error running container runtime Up command: %w", fmt.Errorf("error running container runtime up"))
			}
			return mockCR
		}

		// Given a mock container runtime that returns an error when running the Up command
		rootCmd.SetArgs([]string{"up"})
		err := Execute(mocks.Controller)
		// Then the error should contain the expected message
		if err == nil || !strings.Contains(err.Error(), "Error running container runtime Up command: error running container runtime up") {
			t.Fatalf("Expected error containing 'Error running container runtime Up command: error running container runtime up', got %v", err)
		}
	})

	t.Run("ErrorResolvingNetworkManager", func(t *testing.T) {
		mocks := setupSafeUpCmdMocks()
		mocks.Controller.ResolveNetworkManagerFunc = func() network.NetworkManager {
			return nil
		}

		// Given a mock controller that returns nil when resolving the network manager
		rootCmd.SetArgs([]string{"up"})
		err := Execute(mocks.Controller)
		// Then the error should contain the expected message
		if err == nil || !strings.Contains(err.Error(), "No network manager found") {
			t.Fatalf("Expected error containing 'No network manager found', got %v", err)
		}
	})

	t.Run("ErrorConfiguringDNS", func(t *testing.T) {
		mocks := setupSafeUpCmdMocks()
		mocks.Controller.ResolveNetworkManagerFunc = func() network.NetworkManager {
			mockNM := network.NewMockNetworkManager()
			mockNM.ConfigureDNSFunc = func() error {
				return fmt.Errorf("Error configuring DNS: %w", fmt.Errorf("error configuring DNS"))
			}
			return mockNM
		}

		// Given a mock network manager that returns an error when configuring DNS
		rootCmd.SetArgs([]string{"up"})
		err := Execute(mocks.Controller)
		// Then the error should contain the expected message
		if err == nil || !strings.Contains(err.Error(), "Error configuring DNS") {
			t.Fatalf("Expected error containing 'Error configuring DNS', got %v", err)
		}
	})

	t.Run("NoStackFound", func(t *testing.T) {
		mocks := setupSafeUpCmdMocks()
		mocks.Controller.ResolveStackFunc = func() stack.Stack {
			return nil
		}

		// Given a mock controller that returns nil when resolving the stack
		rootCmd.SetArgs([]string{"up"})
		err := Execute(mocks.Controller)
		// Then the error should contain the expected message
		if err == nil || !strings.Contains(err.Error(), "No stack found") {
			t.Fatalf("Expected error containing 'No stack found', got %v", err)
		}
	})

	t.Run("ErrorRunningStackUp", func(t *testing.T) {
		mocks := setupSafeUpCmdMocks()
		injector := mocks.Injector
		mocks.Controller.ResolveStackFunc = func() stack.Stack {
			mockStack := stack.NewMockStack(injector)
			mockStack.UpFunc = func() error {
				return fmt.Errorf("Error running stack Up command: %w", fmt.Errorf("error running stack up"))
			}
			return mockStack
		}

		// Given a mock stack that returns an error when running the Up command
		rootCmd.SetArgs([]string{"up"})
		err := Execute(mocks.Controller)
		// Then the error should contain the expected message
		if err == nil || !strings.Contains(err.Error(), "Error running stack Up command: error running stack up") {
			t.Fatalf("Expected error containing 'Error running stack Up command: error running stack up', got %v", err)
		}
	})

	t.Run("ErrorInstallingBlueprint", func(t *testing.T) {
		mocks := setupSafeUpCmdMocks()
		injector := mocks.Injector
		mocks.Controller.ResolveBlueprintHandlerFunc = func() bp.BlueprintHandler {
			mockBlueprintHandler := bp.NewMockBlueprintHandler(injector)
			mockBlueprintHandler.InstallFunc = func() error {
				return fmt.Errorf("Error installing blueprint: %w", fmt.Errorf("installation failed"))
			}
			return mockBlueprintHandler
		}

		// Given a mock blueprint handler that returns an error when installing
		rootCmd.SetArgs([]string{"up", "--install"})
		err := Execute(mocks.Controller)
		// Then the error should contain the expected message
		if err == nil || !strings.Contains(err.Error(), "Error installing blueprint: installation failed") {
			t.Fatalf("Expected error containing 'Error installing blueprint: installation failed', got %v", err)
		}
	})

	t.Run("ErrorResolvingBlueprintHandler", func(t *testing.T) {
		mocks := setupSafeUpCmdMocks()
		mocks.Controller.ResolveBlueprintHandlerFunc = func() bp.BlueprintHandler {
			return nil
		}

		// Given a mock controller that returns nil when resolving the blueprint handler
		rootCmd.SetArgs([]string{"up", "--install"})
		err := Execute(mocks.Controller)
		// Then the error should contain the expected message
		if err == nil || !strings.Contains(err.Error(), "No blueprint handler found") {
			t.Fatalf("Expected error containing 'No blueprint handler found', got %v", err)
		}
	})

	t.Run("ColimaDriverSuccess", func(t *testing.T) {
		mocks := setupSafeUpCmdMocks()

		// Simulate successful virtual machine setup process
		mocks.VirtualMachine.UpFunc = func(verbose ...bool) error {
			return nil
		}
		mocks.NetworkManager.ConfigureGuestFunc = func() error {
			return nil
		}
		mocks.NetworkManager.ConfigureHostRouteFunc = func() error {
			return nil
		}

		// Given a mock controller with Colima driver success
		rootCmd.SetArgs([]string{"up"})
		err := Execute(mocks.Controller)
		// Then there should be no error
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
	})

	t.Run("NoVirtualMachineFound", func(t *testing.T) {
		mocks := setupSafeUpCmdMocks()
		mocks.Controller.ResolveVirtualMachineFunc = func() virt.VirtualMachine {
			return nil
		}

		// Given a mock controller that returns nil when resolving the virtual machine
		rootCmd.SetArgs([]string{"up"})
		err := Execute(mocks.Controller)
		// Then the error should contain the expected message
		if err == nil || !strings.Contains(err.Error(), "No virtual machine found") {
			t.Fatalf("Expected error containing 'No virtual machine found', got %v", err)
		}
	})

	t.Run("ErrorRunningVirtualMachineUp", func(t *testing.T) {
		mocks := setupSafeUpCmdMocks()
		mocks.VirtualMachine.UpFunc = func(verbose ...bool) error {
			return fmt.Errorf("Error running virtual machine Up command: %w", fmt.Errorf("error running virtual machine up"))
		}
		mocks.Controller.ResolveVirtualMachineFunc = func() virt.VirtualMachine {
			return mocks.VirtualMachine
		}

		// Given a mock controller with Colima driver and virtual machine up error
		rootCmd.SetArgs([]string{"up"})
		err := Execute(mocks.Controller)
		// Then the error should contain the expected message
		if err == nil || !strings.Contains(err.Error(), "Error running virtual machine Up command") {
			t.Fatalf("Expected error containing 'Error running virtual machine Up command', got %v", err)
		}
	})

	t.Run("ErrorConfiguringGuestNetwork", func(t *testing.T) {
		mocks := setupSafeUpCmdMocks()

		// Simulate an error when configuring the guest network
		mocks.NetworkManager.ConfigureGuestFunc = func() error {
			return fmt.Errorf("Error configuring guest network: %w", fmt.Errorf("network configuration failed"))
		}

		// Resolve the mocked network manager
		mocks.Controller.ResolveNetworkManagerFunc = func() network.NetworkManager {
			return mocks.NetworkManager
		}

		// Given a mock network manager that returns an error when configuring the guest network
		rootCmd.SetArgs([]string{"up"})
		err := Execute(mocks.Controller)
		// Then the error should contain the expected message
		if err == nil || !strings.Contains(err.Error(), "Error configuring guest network: network configuration failed") {
			t.Fatalf("Expected error containing 'Error configuring guest network: network configuration failed', got %v", err)
		}
	})

	t.Run("ErrorConfiguringHostRoute", func(t *testing.T) {
		mocks := setupSafeUpCmdMocks()

		// Simulate an error when configuring the host route
		mocks.NetworkManager.ConfigureHostRouteFunc = func() error {
			return fmt.Errorf("Error configuring host network: %w", fmt.Errorf("host route configuration failed"))
		}

		// Resolve the mocked network manager
		mocks.Controller.ResolveNetworkManagerFunc = func() network.NetworkManager {
			return mocks.NetworkManager
		}

		// Given a mock network manager that returns an error when configuring the host route
		rootCmd.SetArgs([]string{"up"})
		err := Execute(mocks.Controller)
		// Then the error should contain the expected message
		if err == nil || !strings.Contains(err.Error(), "Error configuring host network: host route configuration failed") {
			t.Fatalf("Expected error containing 'Error configuring host network: host route configuration failed', got %v", err)
		}
	})

	t.Run("ErrorCheckingTools", func(t *testing.T) {
		mocks := setupSafeUpCmdMocks()
		mocks.Controller.ResolveToolsManagerFunc = func() tools.ToolsManager {
			return &tools.MockToolsManager{
				CheckFunc: func() error {
					return fmt.Errorf("mock error checking tools")
				},
			}
		}

		// Given a mock tools manager that returns an error when checking tools
		rootCmd.SetArgs([]string{"up"})
		err := Execute(mocks.Controller)
		// Then the error should contain the expected message
		if err == nil || !strings.Contains(err.Error(), "Error checking tools: mock error checking tools") {
			t.Fatalf("Expected error containing 'Error checking tools: mock error checking tools', got %v", err)
		}
	})

	t.Run("ErrorLoadingSecrets", func(t *testing.T) {
		mocks := setupSafeUpCmdMocks()
		mocks.Controller.ResolveAllSecretsProvidersFunc = func() []secrets.SecretsProvider {
			return []secrets.SecretsProvider{
				&secrets.MockSecretsProvider{
					LoadSecretsFunc: func() error {
						return fmt.Errorf("Error loading secrets: %w", fmt.Errorf("mock error loading secrets"))
					},
				},
			}
		}

		// Given a mock secrets provider that returns an error when loading secrets
		rootCmd.SetArgs([]string{"up"})
		err := Execute(mocks.Controller)
		// Then the error should contain the expected message
		if err == nil || !strings.Contains(err.Error(), "Error loading secrets: mock error loading secrets") {
			t.Fatalf("Expected error containing 'Error loading secrets: mock error loading secrets', got %v", err)
		}
	})

	t.Run("LoadCalledAsExpected", func(t *testing.T) {
		mocks := setupSafeUpCmdMocks()
		var loadCalled bool
		mockSecretsProvider := &secrets.MockSecretsProvider{
			LoadSecretsFunc: func() error {
				loadCalled = true
				return nil
			},
		}
		mocks.Controller.ResolveAllSecretsProvidersFunc = func() []secrets.SecretsProvider {
			return []secrets.SecretsProvider{mockSecretsProvider}
		}

		// Given a mock secrets provider
		rootCmd.SetArgs([]string{"up"})
		_ = Execute(mocks.Controller)

		// Check if the Load function was called
		if !loadCalled {
			t.Fatalf("Expected Load to be called, but it was not")
		}
	})

	t.Run("ErrorSettingNoCache", func(t *testing.T) {
		mocks := setupSafeUpCmdMocks()

		// Mock osSetenv to return an error
		originalOsSetenv := osSetenv
		osSetenv = func(key, value string) error {
			if key == "NO_CACHE" {
				return fmt.Errorf("mock error setting NO_CACHE")
			}
			return nil
		}
		defer func() {
			// Restore the original osSetenv function after the test
			osSetenv = originalOsSetenv
		}()

		// Given the up command is executed
		rootCmd.SetArgs([]string{"up"})
		err := Execute(mocks.Controller)

		// Then the error should contain the expected message
		if err == nil || !strings.Contains(err.Error(), "Error setting NO_CACHE environment variable: mock error setting NO_CACHE") {
			t.Fatalf("Expected error containing 'Error setting NO_CACHE environment variable: mock error setting NO_CACHE', got %v", err)
		}
	})

}
