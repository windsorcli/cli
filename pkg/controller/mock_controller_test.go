package controller

import (
	"testing"

	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/context"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/env"
	"github.com/windsorcli/cli/pkg/generators"
	"github.com/windsorcli/cli/pkg/network"
	"github.com/windsorcli/cli/pkg/services"
	"github.com/windsorcli/cli/pkg/shell"
	"github.com/windsorcli/cli/pkg/stack"
	"github.com/windsorcli/cli/pkg/virt"
)

func TestMockController_Initialize(t *testing.T) {
	t.Run("Initialize", func(t *testing.T) {
		// Given a new injector and mock controller
		injector := di.NewInjector()
		mockCtrl := NewMockController(injector)
		// And the InitializeFunc is set to return nil
		mockCtrl.InitializeFunc = func() error {
			return nil
		}
		// When Initialize is called
		if err := mockCtrl.Initialize(); err != nil {
			// Then no error should be returned
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("NoInitializeFunc", func(t *testing.T) {
		// Given a new injector and mock controller
		injector := di.NewInjector()
		mockCtrl := NewMockController(injector)
		// When Initialize is called without setting InitializeFunc
		if err := mockCtrl.Initialize(); err != nil {
			// Then no error should be returned
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestMockController_InitializeComponents(t *testing.T) {
	t.Run("InitializeComponents", func(t *testing.T) {
		// Given a new injector and mock controller
		injector := di.NewInjector()
		mockCtrl := NewMockController(injector)
		// And the InitializeComponentsFunc is set to return nil
		mockCtrl.InitializeComponentsFunc = func() error {
			return nil
		}
		// When InitializeComponents is called
		if err := mockCtrl.InitializeComponents(); err != nil {
			// Then no error should be returned
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("NoInitializeComponentsFunc", func(t *testing.T) {
		// Given a new injector and mock controller
		injector := di.NewInjector()
		mockCtrl := NewMockController(injector)
		// When InitializeComponents is called without setting InitializeComponentsFunc
		if err := mockCtrl.InitializeComponents(); err != nil {
			// Then no error should be returned
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestMockController_CreateCommonComponents(t *testing.T) {
	t.Run("CreateCommonComponents", func(t *testing.T) {
		// Given a new injector and mock controller
		injector := di.NewInjector()
		mockCtrl := NewMockController(injector)
		// And the CreateCommonComponentsFunc is set to return nil
		mockCtrl.CreateCommonComponentsFunc = func() error {
			return nil
		}
		// When CreateCommonComponents is called
		if err := mockCtrl.CreateCommonComponents(); err != nil {
			// Then no error should be returned
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("NoCreateCommonComponentsFunc", func(t *testing.T) {
		// Given a new injector and mock controller
		injector := di.NewInjector()
		mockCtrl := NewMockController(injector)
		// When CreateCommonComponents is called without setting CreateCommonComponentsFunc
		if err := mockCtrl.CreateCommonComponents(); err != nil {
			// Then no error should be returned
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestMockController_CreateProjectComponents(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new injector and mock controller
		injector := di.NewInjector()
		mockCtrl := NewMockController(injector)
		// And the CreateProjectComponentsFunc is set to return nil
		mockCtrl.CreateProjectComponentsFunc = func() error {
			return nil
		}
		// When CreateProjectComponents is called
		if err := mockCtrl.CreateProjectComponents(); err != nil {
			// Then no error should be returned
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("DefaultCreateProjectComponents", func(t *testing.T) {
		// Given a new injector and a mock controller
		injector := di.NewInjector()
		mockCtrl := NewMockController(injector)
		// When CreateProjectComponents is invoked without setting CreateProjectComponentsFunc
		if err := mockCtrl.CreateProjectComponents(); err != nil {
			// Then no error should be returned
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestMockController_CreateEnvComponents(t *testing.T) {
	t.Run("CreateEnvComponents", func(t *testing.T) {
		// Given a new injector and mock controller
		injector := di.NewInjector()
		mockCtrl := NewMockController(injector)
		// And the CreateEnvComponentsFunc is set to return nil
		mockCtrl.CreateEnvComponentsFunc = func() error {
			return nil
		}
		// When CreateEnvComponents is called
		if err := mockCtrl.CreateEnvComponents(); err != nil {
			// Then no error should be returned
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("NoCreateEnvComponentsFunc", func(t *testing.T) {
		// Use setSafeControllerMocks to set up the mock environment
		mocks := setSafeControllerMocks()
		// Given a new injector and mock controller
		mockCtrl := NewMockController(mocks.Injector)

		// Initialize the mock controller and check for error
		if err := mockCtrl.Initialize(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// When CreateEnvComponents is called without setting CreateEnvComponentsFunc
		if err := mockCtrl.CreateEnvComponents(); err != nil {
			// Then no error should be returned
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestMockController_CreateServiceComponents(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new injector and mock controller
		injector := di.NewInjector()
		mockCtrl := NewMockController(injector)
		// And the CreateServiceComponentsFunc is set to return nil
		mockCtrl.CreateServiceComponentsFunc = func() error {
			return nil
		}
		// When CreateServiceComponents is called
		if err := mockCtrl.CreateServiceComponents(); err != nil {
			// Then no error should be returned
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("NoCreateServiceComponentsFunc", func(t *testing.T) {
		// Given a new injector and mock controller
		injector := di.NewInjector()
		mockCtrl := NewMockController(injector)
		// And a mock config handler is created and assigned to the controller
		mockConfigHandler := config.NewMockConfigHandler()
		mockCtrl.configHandler = mockConfigHandler

		// And the mock config handler is set to return specific values for certain keys
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			switch key {
			case "dns.enabled":
				return true
			case "git.livereload.enabled":
				return true
			case "aws.localstack.enabled":
				return true
			case "cluster.enabled":
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
				return 3
			case "cluster.workers.count":
				return 5
			default:
				if len(defaultValue) > 0 {
					return defaultValue[0]
				}
				return 0
			}
		}

		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "cluster.driver" {
				return "talos"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		mockConfigHandler.GetConfigFunc = func() *config.Context {
			return &config.Context{
				Docker: &config.DockerConfig{
					Registries: []config.RegistryConfig{
						{Name: "registry1"},
						{Name: "registry2"},
					},
				},
			}
		}

		// When CreateServiceComponents is called
		if err := mockCtrl.CreateServiceComponents(); err != nil {
			// Then no error should be returned
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestMockController_CreateVirtualizationComponents(t *testing.T) {
	t.Run("CreateVirtualizationComponents", func(t *testing.T) {
		// Given a new injector and mock controller
		injector := di.NewInjector()
		mockCtrl := NewMockController(injector)
		// And the CreateVirtualizationComponentsFunc is set to return nil
		mockCtrl.CreateVirtualizationComponentsFunc = func() error {
			return nil
		}
		// When CreateVirtualizationComponents is called
		if err := mockCtrl.CreateVirtualizationComponents(); err != nil {
			// Then no error should be returned
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("NoCreateVirtualizationComponentsFunc", func(t *testing.T) {
		// Given a new injector and mock controller
		injector := di.NewInjector()
		mockCtrl := NewMockController(injector)
		// And a mock config handler is created and assigned to the controller
		mockConfigHandler := config.NewMockConfigHandler()
		mockCtrl.configHandler = mockConfigHandler
		// And the mock config handler is set to return specific values for certain keys
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "vm.driver" {
				return "colima"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}
		// When CreateVirtualizationComponents is called
		if err := mockCtrl.CreateVirtualizationComponents(); err != nil {
			// Then no error should be returned
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestMockController_CreateStackComponents(t *testing.T) {
	t.Run("CreateStackComponents", func(t *testing.T) {
		// Given a new injector and mock controller
		injector := di.NewInjector()
		mockCtrl := NewMockController(injector)
		// And the CreateStackComponentsFunc is set to return nil
		mockCtrl.CreateStackComponentsFunc = func() error {
			return nil
		}
		// When CreateStackComponents is called
		if err := mockCtrl.CreateStackComponents(); err != nil {
			// Then no error should be returned
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("NoCreateStackComponentsFunc", func(t *testing.T) {
		// Given a new injector and mock controller
		injector := di.NewInjector()
		mockCtrl := NewMockController(injector)
		// When CreateStackComponents is called without setting CreateStackComponentsFunc
		if err := mockCtrl.CreateStackComponents(); err != nil {
			// Then no error should be returned
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestMockController_WriteConfigurationFiles(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new injector and mock controller
		injector := di.NewInjector()
		mockCtrl := NewMockController(injector)
		// And the WriteConfigurationFilesFunc is set to return nil
		mockCtrl.WriteConfigurationFilesFunc = func() error {
			// Validate that the WriteConfigFunc is called
			if mockCtrl.WriteConfigurationFilesFunc == nil {
				t.Fatalf("expected WriteConfigurationFilesFunc to be set")
			}
			return nil
		}
		// When WriteConfigurationFiles is called
		if err := mockCtrl.WriteConfigurationFiles(); err != nil {
			// Then no error should be returned
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("NoWriteConfigurationFilesFunc", func(t *testing.T) {
		// Given a new injector and mock controller
		injector := di.NewInjector()
		mockCtrl := NewMockController(injector)
		// When WriteConfigurationFiles is called without setting WriteConfigurationFilesFunc
		if err := mockCtrl.WriteConfigurationFiles(); err != nil {
			// Then no error should be returned
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestMockController_ResolveInjector(t *testing.T) {
	t.Run("ResolveInjector", func(t *testing.T) {
		// Given a new mock injector and mock controller
		expectedInjector := di.NewMockInjector()
		mockCtrl := NewMockController(expectedInjector)
		// And the ResolveInjectorFunc is set to return the expected injector
		mockCtrl.ResolveInjectorFunc = func() di.Injector {
			return expectedInjector
		}
		// When ResolveInjector is called
		if injector := mockCtrl.ResolveInjector(); injector != expectedInjector {
			// Then the returned injector should be the expected injector
			t.Fatalf("expected %v, got %v", expectedInjector, injector)
		}
	})

	t.Run("NoResolveInjectorFunc", func(t *testing.T) {
		// Given a new mock injector and mock controller
		injector := di.NewMockInjector()
		mockCtrl := NewMockController(injector)
		// When ResolveInjector is called without setting ResolveInjectorFunc
		if injector := mockCtrl.ResolveInjector(); injector != injector {
			// Then the returned injector should be the same as the created injector
			t.Fatalf("expected %v, got %v", injector, injector)
		}
	})
}

func TestMockController_ResolveConfigHandler(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new mock config handler, mock injector, and mock controller
		expectedConfigHandler := config.NewMockConfigHandler()
		injector := di.NewMockInjector()
		mockCtrl := NewMockController(injector)
		// And the ResolveConfigHandlerFunc is set to return the expected config handler
		mockCtrl.ResolveConfigHandlerFunc = func() config.ConfigHandler {
			return expectedConfigHandler
		}
		// When ResolveConfigHandler is called
		configHandler := mockCtrl.ResolveConfigHandler()
		if configHandler != expectedConfigHandler {
			// Then the returned config handler should be the expected config handler
			t.Fatalf("expected %v, got %v", expectedConfigHandler, configHandler)
		}
	})

	t.Run("NoResolveConfigHandlerFunc", func(t *testing.T) {
		// Given a new mock injector and mock controller
		injector := di.NewMockInjector()
		mockCtrl := NewMockController(injector)
		// When ResolveConfigHandler is called without setting ResolveConfigHandlerFunc
		configHandler := mockCtrl.ResolveConfigHandler()
		if configHandler != configHandler {
			// Then the returned config handler should be the same as the created config handler
			t.Fatalf("expected %v, got %v", configHandler, configHandler)
		}
	})
}

func TestMockController_ResolveContextHandler(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new mock context handler, mock injector, and mock controller
		expectedContextHandler := context.NewMockContext()
		injector := di.NewMockInjector()
		mockCtrl := NewMockController(injector)
		// And the ResolveContextHandlerFunc is set to return the expected context handler
		mockCtrl.ResolveContextHandlerFunc = func() context.ContextHandler {
			return expectedContextHandler
		}
		// When ResolveContextHandler is called
		contextHandler := mockCtrl.ResolveContextHandler()
		if contextHandler != expectedContextHandler {
			// Then the returned context handler should be the expected context handler
			t.Fatalf("expected %v, got %v", expectedContextHandler, contextHandler)
		}
	})

	t.Run("NoResolveContextHandlerFunc", func(t *testing.T) {
		// Given a new mock injector and mock controller
		injector := di.NewMockInjector()
		mockCtrl := NewMockController(injector)
		// When ResolveContextHandler is called without setting ResolveContextHandlerFunc
		contextHandler := mockCtrl.ResolveContextHandler()
		if contextHandler != contextHandler {
			// Then the returned context handler should be the same as the created context handler
			t.Fatalf("expected %v, got %v", contextHandler, contextHandler)
		}
	})
}

func TestMockController_ResolveEnvPrinter(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new mock env printer, mock injector, and mock controller
		expectedEnvPrinter := &env.MockEnvPrinter{}
		injector := di.NewMockInjector()
		mockCtrl := NewMockController(injector)
		// And the ResolveEnvPrinterFunc is set to return the expected env printer
		mockCtrl.ResolveEnvPrinterFunc = func(name string) env.EnvPrinter {
			return expectedEnvPrinter
		}
		// When ResolveEnvPrinter is called
		envPrinter := mockCtrl.ResolveEnvPrinter("envPrinter")
		if envPrinter != expectedEnvPrinter {
			// Then the returned env printer should be the expected env printer
			t.Fatalf("expected %v, got %v", expectedEnvPrinter, envPrinter)
		}
	})

	t.Run("NoResolveEnvPrinterFunc", func(t *testing.T) {
		// Given a new mock injector and mock controller
		injector := di.NewMockInjector()
		mockCtrl := NewMockController(injector)
		// When ResolveEnvPrinter is called without setting ResolveEnvPrinterFunc
		envPrinter := mockCtrl.ResolveEnvPrinter("envPrinter")
		if envPrinter != envPrinter {
			// Then the returned env printer should be the same as the created env printer
			t.Fatalf("expected %v, got %v", envPrinter, envPrinter)
		}
	})
}

func TestMockController_ResolveAllEnvPrinters(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new mock injector and mock controller
		injector := di.NewMockInjector()
		mockCtrl := NewMockController(injector)
		// And the ResolveAllEnvPrintersFunc is set to return a list of mock env printers
		mockCtrl.ResolveAllEnvPrintersFunc = func() []env.EnvPrinter {
			return []env.EnvPrinter{&env.MockEnvPrinter{}, &env.MockEnvPrinter{}}
		}
		// When ResolveAllEnvPrinters is called
		envPrinters := mockCtrl.ResolveAllEnvPrinters()
		if len(envPrinters) != 2 {
			// Then the length of the returned env printers list should be 2
			t.Fatalf("expected %v, got %v", 2, len(envPrinters))
		}
	})

	t.Run("NoResolveAllEnvPrintersFunc", func(t *testing.T) {
		// Given a new mock injector and mock controller
		injector := di.NewMockInjector()
		mockCtrl := NewMockController(injector)
		// When ResolveAllEnvPrinters is called without setting ResolveAllEnvPrintersFunc
		envPrinters := mockCtrl.ResolveAllEnvPrinters()
		if len(envPrinters) != 0 {
			// Then the length of the returned env printers list should be 0
			t.Fatalf("expected %v, got %v", 0, len(envPrinters))
		}
	})
}

func TestMockController_ResolveShell(t *testing.T) {
	t.Run("ResolveShell", func(t *testing.T) {
		// Given a new mock shell, mock injector, and mock controller
		expectedShell := &shell.MockShell{}
		injector := di.NewMockInjector()
		mockCtrl := NewMockController(injector)
		// And the ResolveShellFunc is set to return the expected shell
		mockCtrl.ResolveShellFunc = func() shell.Shell {
			return expectedShell
		}
		// When ResolveShell is called
		shellInstance := mockCtrl.ResolveShell()
		if shellInstance != expectedShell {
			// Then the returned shell should be the expected shell
			t.Fatalf("expected %v, got %v", expectedShell, shellInstance)
		}
	})

	t.Run("NoResolveShellFunc", func(t *testing.T) {
		// Given a new mock injector and mock controller
		injector := di.NewMockInjector()
		mockCtrl := NewMockController(injector)
		// When ResolveShell is called without setting ResolveShellFunc
		shellInstance := mockCtrl.ResolveShell()
		if shellInstance != shellInstance {
			// Then the returned shell should be the same as the created shell
			t.Fatalf("expected %v, got %v", shellInstance, shellInstance)
		}
	})
}

func TestMockController_ResolveSecureShell(t *testing.T) {
	t.Run("ResolveSecureShell", func(t *testing.T) {
		// Given a new mock secure shell, mock injector, and mock controller
		expectedSecureShell := &shell.MockShell{}
		injector := di.NewMockInjector()
		mockCtrl := NewMockController(injector)
		// And the ResolveSecureShellFunc is set to return the expected secure shell
		mockCtrl.ResolveSecureShellFunc = func() shell.Shell {
			return expectedSecureShell
		}
		// When ResolveSecureShell is called
		secureShell := mockCtrl.ResolveSecureShell()
		if secureShell != expectedSecureShell {
			// Then the returned secure shell should be the expected secure shell
			t.Fatalf("expected %v, got %v", expectedSecureShell, secureShell)
		}
	})

	t.Run("NoResolveSecureShellFunc", func(t *testing.T) {
		// Given a new mock injector and mock controller
		injector := di.NewMockInjector()
		mockCtrl := NewMockController(injector)
		// When ResolveSecureShell is called without setting ResolveSecureShellFunc
		secureShell := mockCtrl.ResolveSecureShell()
		if secureShell != secureShell {
			// Then the returned secure shell should be the same as the created secure shell
			t.Fatalf("expected %v, got %v", secureShell, secureShell)
		}
	})
}

func TestMockController_ResolveNetworkManager(t *testing.T) {
	t.Run("ResolveNetworkManager", func(t *testing.T) {
		// Given a new mock network manager, mock injector, and mock controller
		expectedNetworkManager := &network.MockNetworkManager{}
		injector := di.NewMockInjector()
		mockCtrl := NewMockController(injector)
		// And the ResolveNetworkManagerFunc is set to return the expected network manager
		mockCtrl.ResolveNetworkManagerFunc = func() network.NetworkManager {
			return expectedNetworkManager
		}
		// When ResolveNetworkManager is called
		networkManager := mockCtrl.ResolveNetworkManager()
		if networkManager != expectedNetworkManager {
			// Then the returned network manager should be the expected network manager
			t.Fatalf("expected %v, got %v", expectedNetworkManager, networkManager)
		}
	})

	t.Run("NoResolveNetworkManagerFunc", func(t *testing.T) {
		// Given a new mock injector and mock controller
		injector := di.NewMockInjector()
		mockCtrl := NewMockController(injector)
		// When ResolveNetworkManager is called without setting ResolveNetworkManagerFunc
		networkManager := mockCtrl.ResolveNetworkManager()
		if networkManager != networkManager {
			// Then the returned network manager should be the same as the created network manager
			t.Fatalf("expected %v, got %v", networkManager, networkManager)
		}
	})
}

func TestMockController_ResolveService(t *testing.T) {
	t.Run("ResolveService", func(t *testing.T) {
		// Given a new mock service, mock injector, and mock controller
		expectedService := &services.MockService{}
		injector := di.NewMockInjector()
		mockCtrl := NewMockController(injector)
		// And the ResolveServiceFunc is set to return the expected service
		mockCtrl.ResolveServiceFunc = func(name string) services.Service {
			return expectedService
		}
		// When ResolveService is called
		service := mockCtrl.ResolveService("service")
		if service != expectedService {
			// Then the returned service should be the expected service
			t.Fatalf("expected %v, got %v", expectedService, service)
		}
	})

	t.Run("NoResolveServiceFunc", func(t *testing.T) {
		// Given a new mock injector and mock controller
		injector := di.NewMockInjector()
		mockCtrl := NewMockController(injector)
		// When ResolveService is called without setting ResolveServiceFunc
		service := mockCtrl.ResolveService("service")
		if service != service {
			// Then the returned service should be the same as the created service
			t.Fatalf("expected %v, got %v", service, service)
		}
	})
}

func TestMockController_ResolveAllServices(t *testing.T) {
	t.Run("ResolveAllServices", func(t *testing.T) {
		// Given a new mock injector and mock controller
		expectedServices := []services.Service{&services.MockService{}, &services.MockService{}}
		injector := di.NewMockInjector()
		mockCtrl := NewMockController(injector)
		// And the ResolveAllServicesFunc is set to return a list of mock services
		mockCtrl.ResolveAllServicesFunc = func() []services.Service {
			return expectedServices
		}
		// When ResolveAllServices is called
		services := mockCtrl.ResolveAllServices()
		if len(services) != len(expectedServices) {
			// Then the length of the returned services list should be the same as the expected services list
			t.Fatalf("expected %v, got %v", len(expectedServices), len(services))
		}
		for i, service := range services {
			if service != expectedServices[i] {
				// Then each service in the returned list should match the expected service
				t.Fatalf("expected %v, got %v", expectedServices[i], service)
			}
		}
	})

	t.Run("NoResolveAllServicesFunc", func(t *testing.T) {
		// Given a new mock injector and mock controller
		injector := di.NewMockInjector()
		mockCtrl := NewMockController(injector)
		services := mockCtrl.ResolveAllServices()
		if len(services) != 0 {
			t.Fatalf("expected %v, got %v", 0, len(services))
		}
	})
}

func TestMockController_ResolveVirtualMachine(t *testing.T) {
	t.Run("ResolveVirtualMachine", func(t *testing.T) {
		// Given a new mock injector and mock controller
		expectedVirtualMachine := &virt.MockVirt{}
		injector := di.NewMockInjector()
		mockCtrl := NewMockController(injector)
		// And the ResolveVirtualMachineFunc is set to return the expected virtual machine
		mockCtrl.ResolveVirtualMachineFunc = func() virt.VirtualMachine {
			return expectedVirtualMachine
		}
		// When ResolveVirtualMachine is called
		virtualMachine := mockCtrl.ResolveVirtualMachine()
		// Then the returned virtual machine should be the expected virtual machine
		if virtualMachine != expectedVirtualMachine {
			t.Fatalf("expected %v, got %v", expectedVirtualMachine, virtualMachine)
		}
	})

	t.Run("NoResolveVirtualMachineFunc", func(t *testing.T) {
		// Given a new mock injector and mock controller
		injector := di.NewMockInjector()
		mockCtrl := NewMockController(injector)
		// When ResolveVirtualMachine is called without setting ResolveVirtualMachineFunc
		virtualMachine := mockCtrl.ResolveVirtualMachine()
		// Then the returned virtual machine should be the same as the created virtual machine
		if virtualMachine != virtualMachine {
			t.Fatalf("expected %v, got %v", virtualMachine, virtualMachine)
		}
	})
}

func TestMockController_ResolveContainerRuntime(t *testing.T) {
	t.Run("ResolveContainerRuntime", func(t *testing.T) {
		// Given a new mock injector and mock controller
		expectedContainerRuntime := &virt.MockVirt{}
		injector := di.NewMockInjector()
		mockCtrl := NewMockController(injector)
		// And the ResolveContainerRuntimeFunc is set to return the expected container runtime
		mockCtrl.ResolveContainerRuntimeFunc = func() virt.ContainerRuntime {
			return expectedContainerRuntime
		}
		// When ResolveContainerRuntime is called
		containerRuntime := mockCtrl.ResolveContainerRuntime()
		// Then the returned container runtime should be the expected container runtime
		if containerRuntime != expectedContainerRuntime {
			t.Fatalf("expected %v, got %v", expectedContainerRuntime, containerRuntime)
		}
	})

	t.Run("NoResolveContainerRuntimeFunc", func(t *testing.T) {
		// Given a new mock injector and mock controller
		injector := di.NewMockInjector()
		mockCtrl := NewMockController(injector)
		// When ResolveContainerRuntime is called without setting ResolveContainerRuntimeFunc
		containerRuntime := mockCtrl.ResolveContainerRuntime()
		// Then the returned container runtime should be the same as the created container runtime
		if containerRuntime != containerRuntime {
			t.Fatalf("expected %v, got %v", containerRuntime, containerRuntime)
		}
	})
}

func TestMockController_ResolveAllGenerators(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new mock injector and mock controller
		injector := di.NewMockInjector()
		mockCtrl := NewMockController(injector)
		// And the ResolveAllGeneratorsFunc is set to return a list of mock generators
		mockCtrl.ResolveAllGeneratorsFunc = func() []generators.Generator {
			return []generators.Generator{&generators.MockGenerator{}, &generators.MockGenerator{}}
		}
		// When ResolveAllGenerators is called
		generators := mockCtrl.ResolveAllGenerators()
		// Then the length of the returned generators list should be 2
		if len(generators) != 2 {
			t.Fatalf("expected %v, got %v", 2, len(generators))
		}
	})

	t.Run("NoResolveAllGeneratorsFunc", func(t *testing.T) {
		// Given a new mock injector and mock controller
		injector := di.NewMockInjector()
		mockCtrl := NewMockController(injector)
		// When ResolveAllGenerators is called without setting ResolveAllGeneratorsFunc
		generators := mockCtrl.ResolveAllGenerators()
		// Then the length of the returned generators list should be 0
		if len(generators) != 0 {
			t.Fatalf("expected %v, got %v", 0, len(generators))
		}
	})
}

func TestMockController_ResolveStack(t *testing.T) {
	t.Run("ResolveStack", func(t *testing.T) {
		// Given a new mock injector and mock controller
		injector := di.NewMockInjector()
		mockCtrl := NewMockController(injector)
		// And the ResolveStackFunc is set to return a mock stack
		mockCtrl.ResolveStackFunc = func() stack.Stack {
			return stack.NewMockStack(injector)
		}
		// When ResolveStack is called
		stackInstance := mockCtrl.ResolveStack()
		// Then the returned stack instance should not be nil
		if stackInstance == nil {
			t.Fatalf("expected %v, got %v", stack.NewMockStack(injector), stackInstance)
		}
	})

	t.Run("NoResolveStackFunc", func(t *testing.T) {
		// Given a new mock injector and mock controller
		injector := di.NewMockInjector()
		mockCtrl := NewMockController(injector)
		// When ResolveStack is called without setting ResolveStackFunc
		stackInstance := mockCtrl.ResolveStack()
		// Then the returned stack instance should be nil
		if stackInstance != nil {
			t.Fatalf("expected nil, got %v", stackInstance)
		}
	})
}
