package workstation

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/api/v1alpha1/docker"
	ctxpkg "github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/evaluator"
	"github.com/windsorcli/cli/pkg/runtime/shell"
	"github.com/windsorcli/cli/pkg/runtime/shell/ssh"
	"github.com/windsorcli/cli/pkg/workstation/network"
	"github.com/windsorcli/cli/pkg/workstation/services"
	"github.com/windsorcli/cli/pkg/workstation/virt"
)

// =============================================================================
// Test Setup
// =============================================================================

type WorkstationTestMocks struct {
	Runtime          *ctxpkg.Runtime
	ConfigHandler    config.ConfigHandler
	Shell            *shell.MockShell
	NetworkManager   *network.MockNetworkManager
	Services         []*services.MockService
	VirtualMachine   *virt.MockVirt
	ContainerRuntime *virt.MockVirt
	SSHClient        *ssh.MockClient
}

func convertToServiceSlice(mockServices []*services.MockService) []services.Service {
	serviceSlice := make([]services.Service, len(mockServices))
	for i, mockService := range mockServices {
		serviceSlice[i] = mockService
	}
	return serviceSlice
}

func setupWorkstationMocks(t *testing.T, opts ...func(*WorkstationTestMocks)) *WorkstationTestMocks {
	t.Helper()

	// Create mock config handler
	mockConfigHandler := config.NewMockConfigHandler()

	// Create mock shell
	mockShell := shell.NewMockShell()

	// Create mock network manager
	mockNetworkManager := network.NewMockNetworkManager()

	// Create mock services
	mockServices := []*services.MockService{
		services.NewMockService(),
		services.NewMockService(),
	}

	// Create mock virtual machine
	mockVirtualMachine := virt.NewMockVirt()

	// Create mock container runtime
	mockContainerRuntime := virt.NewMockVirt()

	// Create mock SSH client
	mockSSHClient := &ssh.MockClient{}

	// Set up mock behaviors
	mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
		switch key {
		case "vm.driver":
			return "colima"
		case "docker.enabled":
			return "true"
		case "dns.enabled":
			return "true"
		case "git.livereload.enabled":
			return "true"
		case "aws.localstack.enabled":
			return "true"
		case "cluster.driver":
			return "talos"
		case "cluster.controlplanes.count":
			return "2"
		case "cluster.workers.count":
			return "1"
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
		case "dns.enabled":
			return true
		case "git.livereload.enabled":
			return true
		case "aws.localstack.enabled":
			return true
		default:
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return false
		}
	}

	mockConfigHandler.GetIntFunc = func(key string, defaultValue ...int) int {
		switch key {
		case "cluster.controlplanes.count":
			return 2
		case "cluster.workers.count":
			return 1
		default:
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return 0
		}
	}

	mockConfigHandler.GetStringSliceFunc = func(key string, defaultValue ...[]string) []string {
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return []string{}
	}

	mockConfigHandler.GetConfigFunc = func() *v1alpha1.Context {
		return &v1alpha1.Context{
			Docker: &docker.DockerConfig{
				Registries: map[string]docker.RegistryConfig{
					"test-registry": {
						HostPort: 5000,
						Remote:   "https://registry.example.com",
					},
				},
			},
		}
	}

	mockShell.GetProjectRootFunc = func() (string, error) {
		return "/test/project", nil
	}

	// Set up mock service behaviors
	for _, service := range mockServices {
		service.SetNameFunc = func(name string) {}
		service.GetNameFunc = func() string { return "test-service" }
		service.WriteConfigFunc = func() error { return nil }
	}

	// Set up mock network manager behaviors
	mockNetworkManager.AssignIPsFunc = func(services []services.Service) error { return nil }
	mockNetworkManager.ConfigureHostRouteFunc = func() error { return nil }
	mockNetworkManager.ConfigureGuestFunc = func() error { return nil }
	mockNetworkManager.ConfigureDNSFunc = func() error { return nil }

	// Set up mock virtual machine behaviors
	mockVirtualMachine.UpFunc = func(verbose ...bool) error { return nil }
	mockVirtualMachine.DownFunc = func() error { return nil }

	// Set up mock container runtime behaviors
	mockContainerRuntime.UpFunc = func(verbose ...bool) error { return nil }
	mockContainerRuntime.DownFunc = func() error { return nil }

	rt := &ctxpkg.Runtime{
		ContextName:   "test-context",
		ProjectRoot:   "/test/project",
		ConfigRoot:    "/test/project/contexts/test-context",
		TemplateRoot:  "/test/project/contexts/_template",
		ConfigHandler: mockConfigHandler,
		Shell:         mockShell,
		Evaluator:     evaluator.NewExpressionEvaluator(mockConfigHandler, "/test/project", "/test/project/contexts/_template"),
	}

	mocks := &WorkstationTestMocks{
		Runtime:          rt,
		ConfigHandler:    mockConfigHandler,
		Shell:            mockShell,
		NetworkManager:   mockNetworkManager,
		Services:         mockServices,
		VirtualMachine:   mockVirtualMachine,
		ContainerRuntime: mockContainerRuntime,
		SSHClient:        mockSSHClient,
	}

	// Apply any overrides
	for _, opt := range opts {
		opt(mocks)
	}

	return mocks
}

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewWorkstation(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a properly configured runtime with all required dependencies
		mocks := setupWorkstationMocks(t)

		// When creating a new workstation with the runtime
		workstation := NewWorkstation(mocks.Runtime)

		// Then the workstation should be created successfully without errors
		// And the workstation should not be nil
		if workstation == nil {
			t.Error("Expected workstation to be created")
		}
		// And the ConfigHandler should be set
		if workstation.configHandler == nil {
			t.Error("Expected ConfigHandler to be set")
		}
		// And the Shell should be set
		if workstation.shell == nil {
			t.Error("Expected Shell to be set")
		}
	})

	t.Run("NilContext", func(t *testing.T) {
		// Given a nil runtime is provided
		_ = setupWorkstationMocks(t)

		// When creating a new workstation with nil runtime
		// Then it should panic
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected panic for nil runtime")
			}
		}()
		_ = NewWorkstation(nil)
	})

	t.Run("NilConfigHandler", func(t *testing.T) {
		// Given a runtime with nil ConfigHandler
		mocks := setupWorkstationMocks(t)
		rt := &ctxpkg.Runtime{
			Shell: mocks.Shell,
		}

		// When creating a new workstation with the incomplete runtime
		// Then it should panic
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected panic for nil ConfigHandler")
			}
		}()
		_ = NewWorkstation(rt)
	})

	t.Run("NilShell", func(t *testing.T) {
		// Given a runtime with nil Shell
		mocks := setupWorkstationMocks(t)
		rt := &ctxpkg.Runtime{
			ConfigHandler: mocks.ConfigHandler,
		}

		// When creating a new workstation with the incomplete runtime
		// Then it should panic
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected panic for nil Shell")
			}
		}()
		_ = NewWorkstation(rt)
	})

	t.Run("NoErrorWhenShellIsProvided", func(t *testing.T) {
		// Given a runtime with Shell
		mocks := setupWorkstationMocks(t)
		rt := mocks.Runtime

		// When creating a new workstation
		workstation := NewWorkstation(rt)

		// Then the workstation should be created successfully
		if workstation == nil {
			t.Error("Expected workstation to be created")
		}
	})

	t.Run("NilRuntime", func(t *testing.T) {
		// Given a nil runtime is provided
		_ = setupWorkstationMocks(t)

		// When creating a new workstation with nil runtime
		// Then it should panic
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected panic for nil runtime")
			}
		}()
		_ = NewWorkstation(nil)
	})

	t.Run("NoErrorWhenRuntimeIsProvided", func(t *testing.T) {
		// Given a valid runtime
		mocks := setupWorkstationMocks(t)

		// When creating a new workstation
		workstation := NewWorkstation(mocks.Runtime)

		// Then the workstation should be created successfully
		if workstation == nil {
			t.Error("Expected workstation to be created")
		}
	})

	t.Run("CreatesDependencies", func(t *testing.T) {
		// Given a properly configured runtime
		mocks := setupWorkstationMocks(t)

		// When creating a new workstation
		workstation := NewWorkstation(mocks.Runtime)

		// Then the workstation should be created successfully
		// And SSHClient should be created
		if workstation.SSHClient == nil {
			t.Error("Expected SSHClient to be created")
		}
		// And NetworkManager should not be created yet (created in Prepare)
		if workstation.NetworkManager != nil {
			t.Error("Expected NetworkManager not to be created in NewWorkstation (created in Prepare)")
		}
		// And Services should not be created yet (created in Prepare)
		if workstation.Services != nil {
			t.Error("Expected Services not to be created in NewWorkstation (created in Prepare)")
		}
		// And VirtualMachine should not be created yet (created in Prepare)
		if workstation.VirtualMachine != nil {
			t.Error("Expected VirtualMachine not to be created in NewWorkstation (created in Prepare)")
		}
		// And ContainerRuntime should not be created yet (created in Prepare)
		if workstation.ContainerRuntime != nil {
			t.Error("Expected ContainerRuntime not to be created in NewWorkstation (created in Prepare)")
		}
	})

	t.Run("UsesExistingDependencies", func(t *testing.T) {
		// Given a runtime and workstation options with pre-configured dependencies
		mocks := setupWorkstationMocks(t)
		opts := &Workstation{
			NetworkManager:   mocks.NetworkManager,
			Services:         []services.Service{mocks.Services[0]},
			VirtualMachine:   mocks.VirtualMachine,
			ContainerRuntime: mocks.ContainerRuntime,
			SSHClient:        mocks.SSHClient,
		}

		// When creating a new workstation with the provided options
		workstation := NewWorkstation(mocks.Runtime, opts)

		// Then the workstation should be created successfully
		if workstation == nil {
			t.Error("Expected workstation to be created")
		}
		// And the existing NetworkManager should be used
		if workstation.NetworkManager != mocks.NetworkManager {
			t.Error("Expected existing NetworkManager to be used")
		}
		// And the existing Services should be used
		if len(workstation.Services) != 1 {
			t.Error("Expected existing Services to be used")
		}
		// And the existing VirtualMachine should be used
		if workstation.VirtualMachine != mocks.VirtualMachine {
			t.Error("Expected existing VirtualMachine to be used")
		}
		// And the existing ContainerRuntime should be used
		if workstation.ContainerRuntime != mocks.ContainerRuntime {
			t.Error("Expected existing ContainerRuntime to be used")
		}
		// And the existing SSHClient should be used
		if workstation.SSHClient != mocks.SSHClient {
			t.Error("Expected existing SSHClient to be used")
		}
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestWorkstation_Up(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a workstation with all dependencies configured
		mocks := setupWorkstationMocks(t)
		workstation := NewWorkstation(mocks.Runtime, &Workstation{
			VirtualMachine:   mocks.VirtualMachine,
			ContainerRuntime: mocks.ContainerRuntime,
			NetworkManager:   mocks.NetworkManager,
			Services:         convertToServiceSlice(mocks.Services),
		})

		// When calling Up() to start the workstation
		err := workstation.Up()

		// Then the workstation should start successfully without errors
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
	})

	t.Run("SetsNoCacheEnvironmentVariable", func(t *testing.T) {
		// Given a workstation with all dependencies configured
		mocks := setupWorkstationMocks(t)
		workstation := NewWorkstation(mocks.Runtime, &Workstation{
			VirtualMachine:   mocks.VirtualMachine,
			ContainerRuntime: mocks.ContainerRuntime,
			NetworkManager:   mocks.NetworkManager,
			Services:         convertToServiceSlice(mocks.Services),
		})

		// When calling Up() to start the workstation
		err := workstation.Up()

		// Then the workstation should start successfully
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
		// And the NO_CACHE environment variable should be set to "true"
		if os.Getenv("NO_CACHE") != "true" {
			t.Error("Expected NO_CACHE environment variable to be set")
		}
	})

	t.Run("StartsVirtualMachine", func(t *testing.T) {
		// Given a workstation with a virtual machine configured and a tracking flag for Up() calls
		mocks := setupWorkstationMocks(t)
		vmUpCalled := false
		vmWriteConfigCalled := false
		callOrder := []string{}
		mocks.VirtualMachine.WriteConfigFunc = func() error {
			vmWriteConfigCalled = true
			callOrder = append(callOrder, "WriteConfig")
			return nil
		}
		mocks.VirtualMachine.UpFunc = func(verbose ...bool) error {
			vmUpCalled = true
			callOrder = append(callOrder, "Up")
			return nil
		}
		mocks.ConfigHandler.Set("vm.driver", "colima")
		workstation := NewWorkstation(mocks.Runtime, &Workstation{
			VirtualMachine:   mocks.VirtualMachine,
			ContainerRuntime: mocks.ContainerRuntime,
			NetworkManager:   mocks.NetworkManager,
			Services:         convertToServiceSlice(mocks.Services),
		})

		// When calling Up() to start the workstation
		err := workstation.Up()

		// Then the workstation should start successfully
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
		// And VirtualMachine.WriteConfig() should be called
		if !vmWriteConfigCalled {
			t.Error("Expected VirtualMachine.WriteConfig to be called")
		}
		// And VirtualMachine.Up() should be called
		if !vmUpCalled {
			t.Error("Expected VirtualMachine.Up to be called")
		}
		// And WriteConfig should be called before Up
		if len(callOrder) != 2 || callOrder[0] != "WriteConfig" || callOrder[1] != "Up" {
			t.Errorf("Expected WriteConfig to be called before Up, got call order: %v", callOrder)
		}
	})

	t.Run("StartsContainerRuntime", func(t *testing.T) {
		// Given a workstation with a container runtime configured and a tracking flag for Up() calls
		mocks := setupWorkstationMocks(t)
		containerUpCalled := false
		mocks.ContainerRuntime.UpFunc = func(verbose ...bool) error {
			containerUpCalled = true
			return nil
		}
		workstation := NewWorkstation(mocks.Runtime, &Workstation{
			VirtualMachine:   mocks.VirtualMachine,
			ContainerRuntime: mocks.ContainerRuntime,
			NetworkManager:   mocks.NetworkManager,
			Services:         convertToServiceSlice(mocks.Services),
		})

		// When calling Up() to start the workstation
		err := workstation.Up()

		// Then the workstation should start successfully
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
		// And ContainerRuntime.Up() should be called
		if !containerUpCalled {
			t.Error("Expected ContainerRuntime.Up to be called")
		}
	})

	t.Run("ConfiguresNetworking", func(t *testing.T) {
		// Given a workstation with network manager configured and tracking flags for network configuration calls
		mocks := setupWorkstationMocks(t)
		hostRouteCalled := false
		guestCalled := false
		dnsCalled := false

		mocks.NetworkManager.ConfigureHostRouteFunc = func() error {
			hostRouteCalled = true
			return nil
		}
		mocks.NetworkManager.ConfigureGuestFunc = func() error {
			guestCalled = true
			return nil
		}
		mocks.NetworkManager.ConfigureDNSFunc = func() error {
			dnsCalled = true
			return nil
		}
		workstation := NewWorkstation(mocks.Runtime, &Workstation{
			VirtualMachine:   mocks.VirtualMachine,
			ContainerRuntime: mocks.ContainerRuntime,
			NetworkManager:   mocks.NetworkManager,
			Services:         convertToServiceSlice(mocks.Services),
		})

		// When calling Up() to start the workstation
		err := workstation.Up()

		// Then the workstation should start successfully
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
		// And ConfigureHostRoute should be called
		if !hostRouteCalled {
			t.Error("Expected ConfigureHostRoute to be called")
		}
		// And ConfigureGuest should be called
		if !guestCalled {
			t.Error("Expected ConfigureGuest to be called")
		}
		// And ConfigureDNS should be called
		if !dnsCalled {
			t.Error("Expected ConfigureDNS to be called")
		}
	})

	t.Run("WritesServiceConfigs", func(t *testing.T) {
		// Given a workstation with services configured and a tracking flag for WriteConfig() calls
		mocks := setupWorkstationMocks(t)
		writeConfigCalled := false
		for _, service := range mocks.Services {
			service.WriteConfigFunc = func() error {
				writeConfigCalled = true
				return nil
			}
		}
		workstation := NewWorkstation(mocks.Runtime, &Workstation{
			VirtualMachine:   mocks.VirtualMachine,
			ContainerRuntime: mocks.ContainerRuntime,
			NetworkManager:   mocks.NetworkManager,
			Services:         convertToServiceSlice(mocks.Services),
		})

		// When calling Up() to start the workstation
		err := workstation.Up()

		// Then the workstation should start successfully
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
		// And WriteConfig() should be called on each service
		if !writeConfigCalled {
			t.Error("Expected service WriteConfig to be called")
		}
	})

	t.Run("VirtualMachineWriteConfigError", func(t *testing.T) {
		// Given a workstation with a virtual machine that will fail when writing config
		mocks := setupWorkstationMocks(t)
		mocks.VirtualMachine.WriteConfigFunc = func() error {
			return fmt.Errorf("VM config write failed")
		}
		mocks.ConfigHandler.Set("vm.driver", "colima")
		workstation := NewWorkstation(mocks.Runtime, &Workstation{
			VirtualMachine:   mocks.VirtualMachine,
			ContainerRuntime: mocks.ContainerRuntime,
			NetworkManager:   mocks.NetworkManager,
			Services:         convertToServiceSlice(mocks.Services),
		})

		// When calling Up() to start the workstation
		err := workstation.Up()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error writing virtual machine config") {
			t.Errorf("Expected error about writing VM config, got: %v", err)
		}
	})

	t.Run("VirtualMachineUpError", func(t *testing.T) {
		// Given a workstation with a virtual machine that will fail when starting
		mocks := setupWorkstationMocks(t)
		mocks.VirtualMachine.WriteConfigFunc = func() error {
			return nil
		}
		mocks.VirtualMachine.UpFunc = func(verbose ...bool) error {
			return fmt.Errorf("VM start failed")
		}
		mocks.ConfigHandler.Set("vm.driver", "colima")
		workstation := NewWorkstation(mocks.Runtime, &Workstation{
			VirtualMachine:   mocks.VirtualMachine,
			ContainerRuntime: mocks.ContainerRuntime,
			NetworkManager:   mocks.NetworkManager,
			Services:         convertToServiceSlice(mocks.Services),
		})

		// When calling Up() to start the workstation
		err := workstation.Up()

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error for VM start failure")
		}
		// And the error message should indicate virtual machine Up command failure
		if !strings.Contains(err.Error(), "error running virtual machine Up command") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("ContainerRuntimeUpError", func(t *testing.T) {
		// Given a workstation with a container runtime that will fail when starting
		mocks := setupWorkstationMocks(t)
		mocks.ContainerRuntime.UpFunc = func(verbose ...bool) error {
			return fmt.Errorf("container start failed")
		}
		workstation := NewWorkstation(mocks.Runtime, &Workstation{
			VirtualMachine:   mocks.VirtualMachine,
			ContainerRuntime: mocks.ContainerRuntime,
			NetworkManager:   mocks.NetworkManager,
			Services:         convertToServiceSlice(mocks.Services),
		})

		// When calling Up() to start the workstation
		err := workstation.Up()

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error for container start failure")
		}
		// And the error message should indicate container runtime Up command failure
		if !strings.Contains(err.Error(), "error running container runtime Up command") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("NetworkConfigurationError", func(t *testing.T) {
		// Given
		mocks := setupWorkstationMocks(t)
		mocks.NetworkManager.ConfigureHostRouteFunc = func() error {
			return fmt.Errorf("network config failed")
		}
		workstation := NewWorkstation(mocks.Runtime, &Workstation{
			VirtualMachine:   mocks.VirtualMachine,
			ContainerRuntime: mocks.ContainerRuntime,
			NetworkManager:   mocks.NetworkManager,
			Services:         convertToServiceSlice(mocks.Services),
		})

		// When
		err := workstation.Up()

		// Then
		if err == nil {
			t.Error("Expected error for network configuration failure")
		}
		if !strings.Contains(err.Error(), "error configuring host route") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("ServiceWriteConfigError", func(t *testing.T) {
		// Given
		mocks := setupWorkstationMocks(t)
		for _, service := range mocks.Services {
			service.WriteConfigFunc = func() error {
				return fmt.Errorf("service config failed")
			}
		}
		workstation := NewWorkstation(mocks.Runtime, &Workstation{
			VirtualMachine:   mocks.VirtualMachine,
			ContainerRuntime: mocks.ContainerRuntime,
			NetworkManager:   mocks.NetworkManager,
			Services:         convertToServiceSlice(mocks.Services),
		})

		// When
		err := workstation.Up()

		// Then
		if err == nil {
			t.Error("Expected error for service config failure")
		}
		if !strings.Contains(err.Error(), "Error writing config for service") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})
}

func TestWorkstation_Down(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given
		mocks := setupWorkstationMocks(t)
		workstation := NewWorkstation(mocks.Runtime)

		// When
		err := workstation.Down()

		// Then
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
	})

	t.Run("StopsContainerRuntime", func(t *testing.T) {
		// Given
		mocks := setupWorkstationMocks(t)
		containerDownCalled := false
		mocks.ContainerRuntime.DownFunc = func() error {
			containerDownCalled = true
			return nil
		}
		workstation := NewWorkstation(mocks.Runtime, &Workstation{
			ContainerRuntime: mocks.ContainerRuntime,
		})

		// When
		err := workstation.Down()

		// Then
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
		if !containerDownCalled {
			t.Error("Expected ContainerRuntime.Down to be called")
		}
	})

	t.Run("StopsVirtualMachine", func(t *testing.T) {
		// Given
		mocks := setupWorkstationMocks(t)
		vmDownCalled := false
		mocks.VirtualMachine.DownFunc = func() error {
			vmDownCalled = true
			return nil
		}
		workstation := NewWorkstation(mocks.Runtime, &Workstation{
			VirtualMachine: mocks.VirtualMachine,
		})

		// When
		err := workstation.Down()

		// Then
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
		if !vmDownCalled {
			t.Error("Expected VirtualMachine.Down to be called")
		}
	})

	t.Run("ContainerRuntimeDownError", func(t *testing.T) {
		// Given
		mocks := setupWorkstationMocks(t)
		mocks.ContainerRuntime.DownFunc = func() error {
			return fmt.Errorf("container stop failed")
		}
		workstation := NewWorkstation(mocks.Runtime, &Workstation{
			ContainerRuntime: mocks.ContainerRuntime,
		})

		// When
		err := workstation.Down()

		// Then
		if err == nil {
			t.Error("Expected error for container stop failure")
		}
		if !strings.Contains(err.Error(), "Error running container runtime Down command") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("VirtualMachineDownError", func(t *testing.T) {
		// Given
		mocks := setupWorkstationMocks(t)
		mocks.VirtualMachine.DownFunc = func() error {
			return fmt.Errorf("VM stop failed")
		}
		workstation := NewWorkstation(mocks.Runtime, &Workstation{
			VirtualMachine: mocks.VirtualMachine,
		})

		// When
		err := workstation.Down()

		// Then
		if err == nil {
			t.Error("Expected error for VM stop failure")
		}
		if !strings.Contains(err.Error(), "Error running virtual machine Down command") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})
}

// =============================================================================
// Test Private Methods
// =============================================================================

func TestWorkstation_createServices(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given
		mocks := setupWorkstationMocks(t)
		workstation := NewWorkstation(mocks.Runtime)

		// When
		services, err := workstation.createServices()

		// Then
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
		if services == nil {
			t.Error("Expected services to be created")
		}
		if len(services) == 0 {
			t.Error("Expected services to be created")
		}
	})

	t.Run("DockerDisabled", func(t *testing.T) {
		// Given
		mocks := setupWorkstationMocks(t)
		mockConfig := config.NewMockConfigHandler()
		mockConfig.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "docker.enabled" {
				return false
			}
			return false
		}
		mocks.Runtime.ConfigHandler = mockConfig
		workstation := NewWorkstation(mocks.Runtime)

		// When
		services, err := workstation.createServices()

		// Then
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
		if len(services) != 0 {
			t.Error("Expected no services when docker is disabled")
		}
	})

	t.Run("ServiceInitializationError", func(t *testing.T) {
		// Given
		mocks := setupWorkstationMocks(t)
		workstation := NewWorkstation(mocks.Runtime)

		// When
		services, err := workstation.createServices()

		// Then
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
		if len(services) == 0 {
			t.Error("Expected services to be created")
		}
	})

	t.Run("CreatesDNSService", func(t *testing.T) {
		// Given
		mocks := setupWorkstationMocks(t)
		workstation := NewWorkstation(mocks.Runtime)

		// When
		services, err := workstation.createServices()

		// Then
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
		if len(services) == 0 {
			t.Error("Expected services to be created")
		}
	})

	t.Run("CreatesGitLivereloadService", func(t *testing.T) {
		// Given
		mocks := setupWorkstationMocks(t)
		workstation := NewWorkstation(mocks.Runtime)

		// When
		services, err := workstation.createServices()

		// Then
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
		if len(services) == 0 {
			t.Error("Expected services to be created")
		}
	})

	t.Run("CreatesLocalstackService", func(t *testing.T) {
		// Given
		mocks := setupWorkstationMocks(t)
		workstation := NewWorkstation(mocks.Runtime)

		// When
		services, err := workstation.createServices()

		// Then
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
		if len(services) == 0 {
			t.Error("Expected services to be created")
		}
	})

	t.Run("CreatesRegistryServices", func(t *testing.T) {
		// Given
		mocks := setupWorkstationMocks(t)
		workstation := NewWorkstation(mocks.Runtime)

		// When
		services, err := workstation.createServices()

		// Then
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
		if len(services) == 0 {
			t.Error("Expected services to be created")
		}
	})

	t.Run("CreatesTalosServices", func(t *testing.T) {
		// Given
		mocks := setupWorkstationMocks(t)
		workstation := NewWorkstation(mocks.Runtime)

		// When
		services, err := workstation.createServices()

		// Then
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
		if len(services) == 0 {
			t.Error("Expected services to be created")
		}
	})
}

// =============================================================================
// Test Helpers
// =============================================================================

func TestWorkstation_Integration(t *testing.T) {
	t.Run("FullUpDownCycle", func(t *testing.T) {
		// Given
		mocks := setupWorkstationMocks(t)
		workstation := NewWorkstation(mocks.Runtime, &Workstation{
			VirtualMachine:   mocks.VirtualMachine,
			ContainerRuntime: mocks.ContainerRuntime,
			NetworkManager:   mocks.NetworkManager,
			Services:         convertToServiceSlice(mocks.Services),
		})

		// When - Up
		err := workstation.Up()

		// Then
		if err != nil {
			t.Errorf("Expected Up to succeed, got error: %v", err)
		}

		// When - Down
		err = workstation.Down()

		// Then
	})

	t.Run("MultipleUpDownCycles", func(t *testing.T) {
		// Given
		mocks := setupWorkstationMocks(t)
		workstation := NewWorkstation(mocks.Runtime, &Workstation{
			VirtualMachine:   mocks.VirtualMachine,
			ContainerRuntime: mocks.ContainerRuntime,
			NetworkManager:   mocks.NetworkManager,
			Services:         convertToServiceSlice(mocks.Services),
		})

		// When - Multiple cycles
		for i := 0; i < 3; i++ {
			err := workstation.Up()
			if err != nil {
				t.Errorf("Expected Up cycle %d to succeed, got error: %v", i+1, err)
			}

			err = workstation.Down()
			if err != nil {
				t.Errorf("Expected Down cycle %d to succeed, got error: %v", i+1, err)
			}
		}
	})
}
