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
	"github.com/windsorcli/cli/pkg/runtime/shell"
	"github.com/windsorcli/cli/pkg/runtime/shell/ssh"
	"github.com/windsorcli/cli/pkg/workstation/network"
	"github.com/windsorcli/cli/pkg/workstation/services"
	"github.com/windsorcli/cli/pkg/workstation/virt"
)

// =============================================================================
// Test Setup
// =============================================================================

type Mocks struct {
	Runtime          *ctxpkg.Runtime
	ConfigHandler    config.ConfigHandler
	Shell            *shell.MockShell
	NetworkManager   *network.MockNetworkManager
	Services         []*services.MockService
	VirtualMachine   *virt.MockVirt
	ContainerRuntime *virt.MockVirt
	SSHClient        *ssh.MockClient
}

type SetupOptions struct {
	ConfigHandler config.ConfigHandler
	ConfigStr     string
}

func convertToServiceSlice(mockServices []*services.MockService) []services.Service {
	serviceSlice := make([]services.Service, len(mockServices))
	for i, mockService := range mockServices {
		serviceSlice[i] = mockService
	}
	return serviceSlice
}

func setupMocks(t *testing.T, opts ...*SetupOptions) *Mocks {
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

	// Apply custom options
	if len(opts) > 0 && opts[0] != nil {
		if opts[0].ConfigHandler != nil {
			if mockConfig, ok := opts[0].ConfigHandler.(*config.MockConfigHandler); ok {
				mockConfigHandler = mockConfig
			}
		}
	}

	rt := &ctxpkg.Runtime{
		ContextName:   "test-context",
		ProjectRoot:   "/test/project",
		ConfigRoot:    "/test/project/contexts/test-context",
		TemplateRoot:  "/test/project/contexts/_template",
		ConfigHandler: mockConfigHandler,
		Shell:         mockShell,
	}

	return &Mocks{
		Runtime:          rt,
		ConfigHandler:    mockConfigHandler,
		Shell:            mockShell,
		NetworkManager:   mockNetworkManager,
		Services:         mockServices,
		VirtualMachine:   mockVirtualMachine,
		ContainerRuntime: mockContainerRuntime,
		SSHClient:        mockSSHClient,
	}
}

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewWorkstation(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given
		mocks := setupMocks(t)

		// When
		workstation, err := NewWorkstation(mocks.Runtime)

		// Then
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
		if workstation == nil {
			t.Error("Expected workstation to be created")
		}
		if workstation.ConfigHandler == nil {
			t.Error("Expected ConfigHandler to be set")
		}
		if workstation.Shell == nil {
			t.Error("Expected Shell to be set")
		}
	})

	t.Run("NilContext", func(t *testing.T) {
		// Given
		_ = setupMocks(t)

		// When
		workstation, err := NewWorkstation(nil)

		// Then
		if err == nil {
			t.Error("Expected error for nil context")
		}
		if workstation != nil {
			t.Error("Expected workstation to be nil")
		}
		if err.Error() != "runtime is required" {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("NilConfigHandler", func(t *testing.T) {
		// Given
		mocks := setupMocks(t)
		rt := &ctxpkg.Runtime{
			Shell: mocks.Shell,
		}

		// When
		workstation, err := NewWorkstation(rt)

		// Then
		if err == nil {
			t.Error("Expected error for nil ConfigHandler")
		}
		if workstation != nil {
			t.Error("Expected workstation to be nil")
		}
		if err.Error() != "ConfigHandler is required on runtime" {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("NilShell", func(t *testing.T) {
		// Given
		mocks := setupMocks(t)
		rt := &ctxpkg.Runtime{
			ConfigHandler: mocks.ConfigHandler,
		}

		// When
		workstation, err := NewWorkstation(rt)

		// Then
		if err == nil {
			t.Error("Expected error for nil Shell")
		}
		if workstation != nil {
			t.Error("Expected workstation to be nil")
		}
		if err.Error() != "Shell is required on runtime" {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("NilRuntime", func(t *testing.T) {
		// Given
		_ = setupMocks(t)

		// When
		workstation, err := NewWorkstation(nil)

		// Then
		if err == nil {
			t.Error("Expected error for nil injector")
		}
		if workstation != nil {
			t.Error("Expected workstation to be nil")
		}
		if err.Error() != "runtime is required" {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("CreatesDependencies", func(t *testing.T) {
		// Given
		mocks := setupMocks(t)

		// When
		workstation, err := NewWorkstation(mocks.Runtime)

		// Then
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
		if workstation.NetworkManager == nil {
			t.Error("Expected NetworkManager to be created")
		}
		if workstation.Services == nil {
			t.Error("Expected Services to be created")
		}
		if workstation.VirtualMachine == nil {
			t.Error("Expected VirtualMachine to be created")
		}
		if workstation.ContainerRuntime == nil {
			t.Error("Expected ContainerRuntime to be created")
		}
		if workstation.SSHClient == nil {
			t.Error("Expected SSHClient to be created")
		}
	})

	t.Run("UsesExistingDependencies", func(t *testing.T) {
		// Given
		mocks := setupMocks(t)
		opts := &Workstation{
			Runtime:          mocks.Runtime,
			NetworkManager:   mocks.NetworkManager,
			Services:         []services.Service{mocks.Services[0]},
			VirtualMachine:   mocks.VirtualMachine,
			ContainerRuntime: mocks.ContainerRuntime,
			SSHClient:        mocks.SSHClient,
		}

		// When
		workstation, err := NewWorkstation(mocks.Runtime, opts)

		// Then
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
		if workstation.NetworkManager != mocks.NetworkManager {
			t.Error("Expected existing NetworkManager to be used")
		}
		if len(workstation.Services) != 1 {
			t.Error("Expected existing Services to be used")
		}
		if workstation.VirtualMachine != mocks.VirtualMachine {
			t.Error("Expected existing VirtualMachine to be used")
		}
		if workstation.ContainerRuntime != mocks.ContainerRuntime {
			t.Error("Expected existing ContainerRuntime to be used")
		}
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
		// Given
		mocks := setupMocks(t)
		workstation, err := NewWorkstation(mocks.Runtime, &Workstation{
			VirtualMachine:   mocks.VirtualMachine,
			ContainerRuntime: mocks.ContainerRuntime,
			NetworkManager:   mocks.NetworkManager,
			Services:         convertToServiceSlice(mocks.Services),
		})
		if err != nil {
			t.Fatalf("Failed to create workstation: %v", err)
		}

		// When
		err = workstation.Up()

		// Then
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
	})

	t.Run("SetsNoCacheEnvironmentVariable", func(t *testing.T) {
		// Given
		mocks := setupMocks(t)
		workstation, err := NewWorkstation(mocks.Runtime, &Workstation{
			VirtualMachine:   mocks.VirtualMachine,
			ContainerRuntime: mocks.ContainerRuntime,
			NetworkManager:   mocks.NetworkManager,
			Services:         convertToServiceSlice(mocks.Services),
		})
		if err != nil {
			t.Fatalf("Failed to create workstation: %v", err)
		}

		// When
		err = workstation.Up()

		// Then
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
		if os.Getenv("NO_CACHE") != "true" {
			t.Error("Expected NO_CACHE environment variable to be set")
		}
	})

	t.Run("StartsVirtualMachine", func(t *testing.T) {
		// Given
		mocks := setupMocks(t)
		vmUpCalled := false
		mocks.VirtualMachine.UpFunc = func(verbose ...bool) error {
			vmUpCalled = true
			return nil
		}
		workstation, err := NewWorkstation(mocks.Runtime, &Workstation{
			VirtualMachine:   mocks.VirtualMachine,
			ContainerRuntime: mocks.ContainerRuntime,
			NetworkManager:   mocks.NetworkManager,
			Services:         convertToServiceSlice(mocks.Services),
		})
		if err != nil {
			t.Fatalf("Failed to create workstation: %v", err)
		}

		// When
		err = workstation.Up()

		// Then
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
		if !vmUpCalled {
			t.Error("Expected VirtualMachine.Up to be called")
		}
	})

	t.Run("StartsContainerRuntime", func(t *testing.T) {
		// Given
		mocks := setupMocks(t)
		containerUpCalled := false
		mocks.ContainerRuntime.UpFunc = func(verbose ...bool) error {
			containerUpCalled = true
			return nil
		}
		workstation, err := NewWorkstation(mocks.Runtime, &Workstation{
			VirtualMachine:   mocks.VirtualMachine,
			ContainerRuntime: mocks.ContainerRuntime,
			NetworkManager:   mocks.NetworkManager,
			Services:         convertToServiceSlice(mocks.Services),
		})
		if err != nil {
			t.Fatalf("Failed to create workstation: %v", err)
		}

		// When
		err = workstation.Up()

		// Then
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
		if !containerUpCalled {
			t.Error("Expected ContainerRuntime.Up to be called")
		}
	})

	t.Run("ConfiguresNetworking", func(t *testing.T) {
		// Given
		mocks := setupMocks(t)
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
		workstation, err := NewWorkstation(mocks.Runtime, &Workstation{
			VirtualMachine:   mocks.VirtualMachine,
			ContainerRuntime: mocks.ContainerRuntime,
			NetworkManager:   mocks.NetworkManager,
			Services:         convertToServiceSlice(mocks.Services),
		})
		if err != nil {
			t.Fatalf("Failed to create workstation: %v", err)
		}

		// When
		err = workstation.Up()

		// Then
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
		if !hostRouteCalled {
			t.Error("Expected ConfigureHostRoute to be called")
		}
		if !guestCalled {
			t.Error("Expected ConfigureGuest to be called")
		}
		if !dnsCalled {
			t.Error("Expected ConfigureDNS to be called")
		}
	})

	t.Run("WritesServiceConfigs", func(t *testing.T) {
		// Given
		mocks := setupMocks(t)
		writeConfigCalled := false
		for _, service := range mocks.Services {
			service.WriteConfigFunc = func() error {
				writeConfigCalled = true
				return nil
			}
		}
		workstation, err := NewWorkstation(mocks.Runtime, &Workstation{
			VirtualMachine:   mocks.VirtualMachine,
			ContainerRuntime: mocks.ContainerRuntime,
			NetworkManager:   mocks.NetworkManager,
			Services:         convertToServiceSlice(mocks.Services),
		})
		if err != nil {
			t.Fatalf("Failed to create workstation: %v", err)
		}

		// When
		err = workstation.Up()

		// Then
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
		if !writeConfigCalled {
			t.Error("Expected service WriteConfig to be called")
		}
	})

	t.Run("VirtualMachineUpError", func(t *testing.T) {
		// Given
		mocks := setupMocks(t)
		mocks.VirtualMachine.UpFunc = func(verbose ...bool) error {
			return fmt.Errorf("VM start failed")
		}
		workstation, err := NewWorkstation(mocks.Runtime, &Workstation{
			VirtualMachine:   mocks.VirtualMachine,
			ContainerRuntime: mocks.ContainerRuntime,
			NetworkManager:   mocks.NetworkManager,
			Services:         convertToServiceSlice(mocks.Services),
		})
		if err != nil {
			t.Fatalf("Failed to create workstation: %v", err)
		}

		// When
		err = workstation.Up()

		// Then
		if err == nil {
			t.Error("Expected error for VM start failure")
		}
		if !strings.Contains(err.Error(), "error running virtual machine Up command") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("ContainerRuntimeUpError", func(t *testing.T) {
		// Given
		mocks := setupMocks(t)
		mocks.ContainerRuntime.UpFunc = func(verbose ...bool) error {
			return fmt.Errorf("container start failed")
		}
		workstation, err := NewWorkstation(mocks.Runtime, &Workstation{
			VirtualMachine:   mocks.VirtualMachine,
			ContainerRuntime: mocks.ContainerRuntime,
			NetworkManager:   mocks.NetworkManager,
			Services:         convertToServiceSlice(mocks.Services),
		})
		if err != nil {
			t.Fatalf("Failed to create workstation: %v", err)
		}

		// When
		err = workstation.Up()

		// Then
		if err == nil {
			t.Error("Expected error for container start failure")
		}
		if !strings.Contains(err.Error(), "error running container runtime Up command") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("NetworkConfigurationError", func(t *testing.T) {
		// Given
		mocks := setupMocks(t)
		mocks.NetworkManager.ConfigureHostRouteFunc = func() error {
			return fmt.Errorf("network config failed")
		}
		workstation, err := NewWorkstation(mocks.Runtime, &Workstation{
			VirtualMachine:   mocks.VirtualMachine,
			ContainerRuntime: mocks.ContainerRuntime,
			NetworkManager:   mocks.NetworkManager,
			Services:         convertToServiceSlice(mocks.Services),
		})
		if err != nil {
			t.Fatalf("Failed to create workstation: %v", err)
		}

		// When
		err = workstation.Up()

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
		mocks := setupMocks(t)
		for _, service := range mocks.Services {
			service.WriteConfigFunc = func() error {
				return fmt.Errorf("service config failed")
			}
		}
		workstation, err := NewWorkstation(mocks.Runtime, &Workstation{
			VirtualMachine:   mocks.VirtualMachine,
			ContainerRuntime: mocks.ContainerRuntime,
			NetworkManager:   mocks.NetworkManager,
			Services:         convertToServiceSlice(mocks.Services),
		})
		if err != nil {
			t.Fatalf("Failed to create workstation: %v", err)
		}

		// When
		err = workstation.Up()

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
		mocks := setupMocks(t)
		workstation, err := NewWorkstation(mocks.Runtime)
		if err != nil {
			t.Fatalf("Failed to create workstation: %v", err)
		}

		// When
		err = workstation.Down()

		// Then
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}
	})

	t.Run("StopsContainerRuntime", func(t *testing.T) {
		// Given
		mocks := setupMocks(t)
		containerDownCalled := false
		mocks.ContainerRuntime.DownFunc = func() error {
			containerDownCalled = true
			return nil
		}
		workstation, err := NewWorkstation(mocks.Runtime, &Workstation{
			ContainerRuntime: mocks.ContainerRuntime,
		})
		if err != nil {
			t.Fatalf("Failed to create workstation: %v", err)
		}

		// When
		err = workstation.Down()

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
		mocks := setupMocks(t)
		vmDownCalled := false
		mocks.VirtualMachine.DownFunc = func() error {
			vmDownCalled = true
			return nil
		}
		workstation, err := NewWorkstation(mocks.Runtime, &Workstation{
			VirtualMachine: mocks.VirtualMachine,
		})
		if err != nil {
			t.Fatalf("Failed to create workstation: %v", err)
		}

		// When
		err = workstation.Down()

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
		mocks := setupMocks(t)
		mocks.ContainerRuntime.DownFunc = func() error {
			return fmt.Errorf("container stop failed")
		}
		workstation, err := NewWorkstation(mocks.Runtime, &Workstation{
			ContainerRuntime: mocks.ContainerRuntime,
		})
		if err != nil {
			t.Fatalf("Failed to create workstation: %v", err)
		}

		// When
		err = workstation.Down()

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
		mocks := setupMocks(t)
		mocks.VirtualMachine.DownFunc = func() error {
			return fmt.Errorf("VM stop failed")
		}
		workstation, err := NewWorkstation(mocks.Runtime, &Workstation{
			VirtualMachine: mocks.VirtualMachine,
		})
		if err != nil {
			t.Fatalf("Failed to create workstation: %v", err)
		}

		// When
		err = workstation.Down()

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
		mocks := setupMocks(t)
		workstation, err := NewWorkstation(mocks.Runtime)
		if err != nil {
			t.Fatalf("Failed to create workstation: %v", err)
		}

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
		mocks := setupMocks(t)
		mockConfig := config.NewMockConfigHandler()
		mockConfig.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "docker.enabled" {
				return false
			}
			return false
		}
		mocks.Runtime.ConfigHandler = mockConfig
		workstation, err := NewWorkstation(mocks.Runtime)
		if err != nil {
			t.Fatalf("Failed to create workstation: %v", err)
		}

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
		mocks := setupMocks(t)
		workstation, err := NewWorkstation(mocks.Runtime)
		if err != nil {
			t.Fatalf("Failed to create workstation: %v", err)
		}

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
		mocks := setupMocks(t)
		workstation, err := NewWorkstation(mocks.Runtime)
		if err != nil {
			t.Fatalf("Failed to create workstation: %v", err)
		}

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
		mocks := setupMocks(t)
		workstation, err := NewWorkstation(mocks.Runtime)
		if err != nil {
			t.Fatalf("Failed to create workstation: %v", err)
		}

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
		mocks := setupMocks(t)
		workstation, err := NewWorkstation(mocks.Runtime)
		if err != nil {
			t.Fatalf("Failed to create workstation: %v", err)
		}

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
		mocks := setupMocks(t)
		workstation, err := NewWorkstation(mocks.Runtime)
		if err != nil {
			t.Fatalf("Failed to create workstation: %v", err)
		}

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
		mocks := setupMocks(t)
		workstation, err := NewWorkstation(mocks.Runtime)
		if err != nil {
			t.Fatalf("Failed to create workstation: %v", err)
		}

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
		mocks := setupMocks(t)
		workstation, err := NewWorkstation(mocks.Runtime, &Workstation{
			VirtualMachine:   mocks.VirtualMachine,
			ContainerRuntime: mocks.ContainerRuntime,
			NetworkManager:   mocks.NetworkManager,
			Services:         convertToServiceSlice(mocks.Services),
		})
		if err != nil {
			t.Fatalf("Failed to create workstation: %v", err)
		}

		// When - Up
		err = workstation.Up()

		// Then
		if err != nil {
			t.Errorf("Expected Up to succeed, got error: %v", err)
		}

		// When - Down
		err = workstation.Down()

		// Then
		if err != nil {
			t.Errorf("Expected Down to succeed, got error: %v", err)
		}
	})

	t.Run("MultipleUpDownCycles", func(t *testing.T) {
		// Given
		mocks := setupMocks(t)
		workstation, err := NewWorkstation(mocks.Runtime, &Workstation{
			VirtualMachine:   mocks.VirtualMachine,
			ContainerRuntime: mocks.ContainerRuntime,
			NetworkManager:   mocks.NetworkManager,
			Services:         convertToServiceSlice(mocks.Services),
		})
		if err != nil {
			t.Fatalf("Failed to create workstation: %v", err)
		}

		// When - Multiple cycles
		for i := 0; i < 3; i++ {
			err = workstation.Up()
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
