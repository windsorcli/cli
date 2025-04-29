package cmd

import (
	"fmt"
	"testing"

	blueprintpkg "github.com/windsorcli/cli/pkg/blueprint"
	"github.com/windsorcli/cli/pkg/controller"
	"github.com/windsorcli/cli/pkg/network"
	"github.com/windsorcli/cli/pkg/stack"
	"github.com/windsorcli/cli/pkg/tools"
	"github.com/windsorcli/cli/pkg/virt"
)

// =============================================================================
// Types
// =============================================================================

// Extend Mocks with additional fields needed for up command tests
type UpMocks struct {
	*Mocks
	ToolsManager     *tools.MockToolsManager
	VirtualMachine   *virt.MockVirt
	ContainerRuntime *virt.MockVirt
	NetworkManager   *network.MockNetworkManager
	Stack            *stack.MockStack
}

// =============================================================================
// Helpers
// =============================================================================

func setupUpMocks(t *testing.T) *UpMocks {
	t.Helper()
	opts := &SetupOptions{
		ConfigStr: `
contexts:
  default:
    tools:
      enabled: true
    docker:
      enabled: true`,
	}
	mocks := setupMocks(t, opts)

	toolsManager := tools.NewMockToolsManager()
	toolsManager.CheckFunc = func() error { return nil }
	toolsManager.InstallFunc = func() error { return nil }
	mocks.Injector.Register("toolsManager", toolsManager)

	virtualMachine := virt.NewMockVirt()
	virtualMachine.UpFunc = func(verbose ...bool) error { return nil }
	mocks.Injector.Register("virtualMachine", virtualMachine)

	containerRuntime := virt.NewMockVirt()
	containerRuntime.UpFunc = func(verbose ...bool) error { return nil }
	mocks.Injector.Register("containerRuntime", containerRuntime)

	networkManager := network.NewMockNetworkManager()
	networkManager.ConfigureGuestFunc = func() error { return nil }
	networkManager.ConfigureHostRouteFunc = func() error { return nil }
	networkManager.ConfigureDNSFunc = func() error { return nil }
	mocks.Injector.Register("networkManager", networkManager)

	stack := stack.NewMockStack(mocks.Injector)
	stack.UpFunc = func() error { return nil }
	mocks.Injector.Register("stack", stack)

	mocks.Controller.SetEnvironmentVariablesFunc = func() error { return nil }
	mocks.Controller.InitializeWithRequirementsFunc = func(req controller.Requirements) error { return nil }
	mocks.Controller.WriteConfigurationFilesFunc = func() error { return nil }

	return &UpMocks{
		Mocks:            mocks,
		ToolsManager:     toolsManager,
		VirtualMachine:   virtualMachine,
		ContainerRuntime: containerRuntime,
		NetworkManager:   networkManager,
		Stack:            stack,
	}
}

// =============================================================================
// Tests
// =============================================================================

func TestUpCmd(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupUpMocks(t)
		mocks.Controller.SetEnvironmentVariablesFunc = func() error {
			return nil
		}
		mocks.Controller.InitializeWithRequirementsFunc = func(req controller.Requirements) error {
			return nil
		}
		mocks.Controller.WriteConfigurationFilesFunc = func() error {
			return nil
		}
		mocks.SecretsProvider.LoadSecretsFunc = func() error {
			return nil
		}
		mocks.BlueprintHandler.InstallFunc = func() error {
			return nil
		}

		rootCmd.SetArgs([]string{"up"})
		err := Execute(mocks.Controller)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ErrorSettingEnvironmentVariables", func(t *testing.T) {
		mocks := setupUpMocks(t)
		mocks.Controller.SetEnvironmentVariablesFunc = func() error {
			return fmt.Errorf("test error")
		}

		rootCmd.SetArgs([]string{"up"})
		err := Execute(mocks.Controller)
		if err == nil {
			t.Error("Expected error, got nil")
		}
	})

	t.Run("ErrorInitializingWithRequirements", func(t *testing.T) {
		mocks := setupUpMocks(t)
		mocks.Controller.InitializeWithRequirementsFunc = func(req controller.Requirements) error {
			return fmt.Errorf("failed to initialize with requirements")
		}

		rootCmd.SetArgs([]string{"up"})
		err := Execute(mocks.Controller)
		if err == nil {
			t.Error("Expected error, got nil")
		}
	})

	t.Run("ErrorWritingConfigurationFiles", func(t *testing.T) {
		mocks := setupUpMocks(t)
		mocks.Controller.WriteConfigurationFilesFunc = func() error {
			return fmt.Errorf("test error")
		}

		rootCmd.SetArgs([]string{"up"})
		err := Execute(mocks.Controller)
		if err == nil {
			t.Error("Expected error, got nil")
		}
	})

	t.Run("ErrorLoadingSecrets", func(t *testing.T) {
		mocks := setupUpMocks(t)
		mocks.SecretsProvider.LoadSecretsFunc = func() error {
			return fmt.Errorf("test error")
		}

		rootCmd.SetArgs([]string{"up"})
		err := Execute(mocks.Controller)
		if err == nil {
			t.Error("Expected error, got nil")
		}
	})

	t.Run("ErrorInstallingBlueprint", func(t *testing.T) {
		mocks := setupUpMocks(t)
		mocks.BlueprintHandler.InstallFunc = func() error {
			return fmt.Errorf("test error")
		}

		rootCmd.SetArgs([]string{"up", "--install"})
		err := Execute(mocks.Controller)
		if err == nil {
			t.Error("Expected error, got nil")
		}
	})

	t.Run("ErrorCheckingTools", func(t *testing.T) {
		mocks := setupUpMocks(t)
		mocks.ToolsManager.CheckFunc = func() error {
			return fmt.Errorf("test error")
		}

		rootCmd.SetArgs([]string{"up"})
		err := Execute(mocks.Controller)
		if err == nil {
			t.Error("Expected error, got nil")
		}
	})

	t.Run("ErrorInstallingTools", func(t *testing.T) {
		mocks := setupUpMocks(t)
		mocks.ToolsManager.InstallFunc = func() error {
			return fmt.Errorf("test error")
		}

		rootCmd.SetArgs([]string{"up"})
		err := Execute(mocks.Controller)
		if err == nil {
			t.Error("Expected error, got nil")
		}
	})

	t.Run("ErrorStartingVirtualMachine", func(t *testing.T) {
		mocks := setupUpMocks(t)
		vmDriver = "colima"
		defer func() { vmDriver = "" }()
		mocks.VirtualMachine.UpFunc = func(verbose ...bool) error {
			return fmt.Errorf("test error")
		}

		rootCmd.SetArgs([]string{"up"})
		err := Execute(mocks.Controller)
		if err == nil {
			t.Error("Expected error, got nil")
		}
	})

	t.Run("ErrorStartingContainerRuntime", func(t *testing.T) {
		mocks := setupUpMocks(t)
		mocks.ContainerRuntime.UpFunc = func(verbose ...bool) error {
			return fmt.Errorf("test error")
		}

		rootCmd.SetArgs([]string{"up"})
		err := Execute(mocks.Controller)
		if err == nil {
			t.Error("Expected error, got nil")
		}
	})

	t.Run("ErrorConfiguringGuestNetwork", func(t *testing.T) {
		mocks := setupUpMocks(t)
		vmDriver = "colima"
		defer func() { vmDriver = "" }()
		mocks.NetworkManager.ConfigureGuestFunc = func() error {
			return fmt.Errorf("test error")
		}

		rootCmd.SetArgs([]string{"up"})
		err := Execute(mocks.Controller)
		if err == nil {
			t.Error("Expected error, got nil")
		}
	})

	t.Run("ErrorConfiguringHostNetwork", func(t *testing.T) {
		mocks := setupUpMocks(t)
		vmDriver = "colima"
		defer func() { vmDriver = "" }()
		mocks.NetworkManager.ConfigureHostRouteFunc = func() error {
			return fmt.Errorf("test error")
		}

		rootCmd.SetArgs([]string{"up"})
		err := Execute(mocks.Controller)
		if err == nil {
			t.Error("Expected error, got nil")
		}
	})

	t.Run("ErrorConfiguringDNS", func(t *testing.T) {
		mocks := setupUpMocks(t)
		mocks.NetworkManager.ConfigureDNSFunc = func() error {
			return fmt.Errorf("test error")
		}

		rootCmd.SetArgs([]string{"up"})
		err := Execute(mocks.Controller)
		if err == nil {
			t.Error("Expected error, got nil")
		}
	})

	t.Run("ErrorStartingStack", func(t *testing.T) {
		mocks := setupUpMocks(t)
		mocks.Stack.UpFunc = func() error {
			return fmt.Errorf("test error")
		}

		rootCmd.SetArgs([]string{"up"})
		err := Execute(mocks.Controller)
		if err == nil {
			t.Error("Expected error, got nil")
		}
	})

	t.Run("ErrorNilVirtualMachine", func(t *testing.T) {
		mocks := setupUpMocks(t)
		vmDriver = "colima"
		defer func() { vmDriver = "" }()
		mocks.Controller.ResolveVirtualMachineFunc = func() virt.VirtualMachine {
			return nil
		}

		rootCmd.SetArgs([]string{"up"})
		err := Execute(mocks.Controller)
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if err.Error() != "No virtual machine found" {
			t.Errorf("Expected 'No virtual machine found', got '%v'", err)
		}
	})

	t.Run("ErrorNilContainerRuntime", func(t *testing.T) {
		mocks := setupUpMocks(t)
		mocks.Controller.ResolveContainerRuntimeFunc = func() virt.ContainerRuntime {
			return nil
		}

		rootCmd.SetArgs([]string{"up"})
		err := Execute(mocks.Controller)
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if err.Error() != "No container runtime found" {
			t.Errorf("Expected 'No container runtime found', got '%v'", err)
		}
	})

	t.Run("ErrorNilNetworkManager", func(t *testing.T) {
		mocks := setupUpMocks(t)
		vmDriver = "colima"
		defer func() { vmDriver = "" }()
		mocks.Controller.ResolveNetworkManagerFunc = func() network.NetworkManager {
			return nil
		}

		rootCmd.SetArgs([]string{"up"})
		err := Execute(mocks.Controller)
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if err.Error() != "No network manager found" {
			t.Errorf("Expected 'No network manager found', got '%v'", err)
		}
	})

	t.Run("ErrorNilStack", func(t *testing.T) {
		mocks := setupUpMocks(t)
		mocks.Controller.ResolveStackFunc = func() stack.Stack {
			return nil
		}

		rootCmd.SetArgs([]string{"up"})
		err := Execute(mocks.Controller)
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if err.Error() != "No stack found" {
			t.Errorf("Expected 'No stack found', got '%v'", err)
		}
	})

	t.Run("ErrorNilBlueprintHandler", func(t *testing.T) {
		mocks := setupUpMocks(t)
		mocks.Controller.ResolveBlueprintHandlerFunc = func() blueprintpkg.BlueprintHandler {
			return nil
		}

		rootCmd.SetArgs([]string{"up", "--install"})
		err := Execute(mocks.Controller)
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if err.Error() != "No blueprint handler found" {
			t.Errorf("Expected 'No blueprint handler found', got '%v'", err)
		}
	})
}
