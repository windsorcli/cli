package controller

import (
	"testing"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/env"
	"github.com/windsor-hotel/cli/internal/network"
	"github.com/windsor-hotel/cli/internal/services"
	"github.com/windsor-hotel/cli/internal/shell"
	"github.com/windsor-hotel/cli/internal/virt"
)

func TestMockController_Initialize(t *testing.T) {
	t.Run("Initialize", func(t *testing.T) {
		injector := di.NewInjector()
		mockCtrl := NewMockController(injector)
		mockCtrl.InitializeFunc = func() error {
			return nil
		}
		if err := mockCtrl.Initialize(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("NoInitializeFunc", func(t *testing.T) {
		injector := di.NewInjector()
		mockCtrl := NewMockController(injector)
		if err := mockCtrl.Initialize(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestMockController_InitializeComponents(t *testing.T) {
	t.Run("InitializeComponents", func(t *testing.T) {
		injector := di.NewInjector()
		mockCtrl := NewMockController(injector)
		mockCtrl.InitializeComponentsFunc = func() error {
			return nil
		}
		if err := mockCtrl.InitializeComponents(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("NoInitializeComponentsFunc", func(t *testing.T) {
		injector := di.NewInjector()
		mockCtrl := NewMockController(injector)
		if err := mockCtrl.InitializeComponents(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestMockController_CreateCommonComponents(t *testing.T) {
	t.Run("CreateCommonComponents", func(t *testing.T) {
		injector := di.NewInjector()
		mockCtrl := NewMockController(injector)
		mockCtrl.CreateCommonComponentsFunc = func() error {
			return nil
		}
		if err := mockCtrl.CreateCommonComponents(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("NoCreateCommonComponentsFunc", func(t *testing.T) {
		injector := di.NewInjector()
		mockCtrl := NewMockController(injector)
		if err := mockCtrl.CreateCommonComponents(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestMockController_CreateEnvComponents(t *testing.T) {
	t.Run("CreateEnvComponents", func(t *testing.T) {
		injector := di.NewInjector()
		mockCtrl := NewMockController(injector)
		mockCtrl.CreateEnvComponentsFunc = func() error {
			return nil
		}
		if err := mockCtrl.CreateEnvComponents(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("NoCreateEnvComponentsFunc", func(t *testing.T) {
		injector := di.NewInjector()
		mockCtrl := NewMockController(injector)
		if err := mockCtrl.CreateEnvComponents(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestMockController_CreateServiceComponents(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		injector := di.NewInjector()
		mockCtrl := NewMockController(injector)
		mockCtrl.CreateServiceComponentsFunc = func() error {
			return nil
		}
		if err := mockCtrl.CreateServiceComponents(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("NoCreateServiceComponentsFunc", func(t *testing.T) {
		injector := di.NewInjector()
		mockCtrl := NewMockController(injector)
		mockConfigHandler := config.NewMockConfigHandler()
		mockCtrl.configHandler = mockConfigHandler

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
					Registries: []config.Registry{
						{Name: "registry1"},
						{Name: "registry2"},
					},
				},
			}
		}

		if err := mockCtrl.CreateServiceComponents(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestMockController_CreateVirtualizationComponents(t *testing.T) {
	t.Run("CreateVirtualizationComponents", func(t *testing.T) {
		injector := di.NewInjector()
		mockCtrl := NewMockController(injector)
		mockCtrl.CreateVirtualizationComponentsFunc = func() error {
			return nil
		}
		if err := mockCtrl.CreateVirtualizationComponents(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("NoCreateVirtualizationComponentsFunc", func(t *testing.T) {
		injector := di.NewInjector()
		mockCtrl := NewMockController(injector)
		mockConfigHandler := config.NewMockConfigHandler()
		mockCtrl.configHandler = mockConfigHandler
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "vm.driver" {
				return "colima"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}
		if err := mockCtrl.CreateVirtualizationComponents(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestMockController_CreateBlueprintComponents(t *testing.T) {
	t.Run("CreateBlueprintComponents", func(t *testing.T) {
		injector := di.NewInjector()
		mockCtrl := NewMockController(injector)
		mockCtrl.CreateBlueprintComponentsFunc = func() error {
			return nil
		}
		if err := mockCtrl.CreateBlueprintComponents(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("NoCreateBlueprintComponentsFunc", func(t *testing.T) {
		injector := di.NewInjector()
		mockCtrl := NewMockController(injector)
		if err := mockCtrl.CreateBlueprintComponents(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestMockController_WriteConfigurationFiles(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		injector := di.NewInjector()
		mockCtrl := NewMockController(injector)
		mockCtrl.WriteConfigurationFilesFunc = func() error {
			// Validate that the WriteConfigFunc is called
			if mockCtrl.WriteConfigurationFilesFunc == nil {
				t.Fatalf("expected WriteConfigurationFilesFunc to be set")
			}
			return nil
		}
		if err := mockCtrl.WriteConfigurationFiles(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("NoWriteConfigurationFilesFunc", func(t *testing.T) {
		injector := di.NewInjector()
		mockCtrl := NewMockController(injector)
		if err := mockCtrl.WriteConfigurationFiles(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestMockController_ResolveInjector(t *testing.T) {
	t.Run("ResolveInjector", func(t *testing.T) {
		expectedInjector := di.NewMockInjector()
		mockCtrl := NewMockController(expectedInjector)
		mockCtrl.ResolveInjectorFunc = func() di.Injector {
			return expectedInjector
		}
		if injector := mockCtrl.ResolveInjector(); injector != expectedInjector {
			t.Fatalf("expected %v, got %v", expectedInjector, injector)
		}
	})

	t.Run("NoResolveInjectorFunc", func(t *testing.T) {
		injector := di.NewMockInjector()
		mockCtrl := NewMockController(injector)
		if injector := mockCtrl.ResolveInjector(); injector != injector {
			t.Fatalf("expected %v, got %v", injector, injector)
		}
	})
}

func TestMockController_ResolveConfigHandler(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		expectedConfigHandler := config.NewMockConfigHandler()
		injector := di.NewMockInjector()
		mockCtrl := NewMockController(injector)
		mockCtrl.ResolveConfigHandlerFunc = func() config.ConfigHandler {
			return expectedConfigHandler
		}
		configHandler := mockCtrl.ResolveConfigHandler()
		if configHandler != expectedConfigHandler {
			t.Fatalf("expected %v, got %v", expectedConfigHandler, configHandler)
		}
	})

	t.Run("NoResolveConfigHandlerFunc", func(t *testing.T) {
		injector := di.NewMockInjector()
		mockCtrl := NewMockController(injector)
		configHandler := mockCtrl.ResolveConfigHandler()
		if configHandler != configHandler {
			t.Fatalf("expected %v, got %v", configHandler, configHandler)
		}
	})
}

func TestMockController_ResolveContextHandler(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		expectedContextHandler := context.NewMockContext()
		injector := di.NewMockInjector()
		mockCtrl := NewMockController(injector)
		mockCtrl.ResolveContextHandlerFunc = func() context.ContextHandler {
			return expectedContextHandler
		}
		contextHandler := mockCtrl.ResolveContextHandler()
		if contextHandler != expectedContextHandler {
			t.Fatalf("expected %v, got %v", expectedContextHandler, contextHandler)
		}
	})

	t.Run("NoResolveContextHandlerFunc", func(t *testing.T) {
		injector := di.NewMockInjector()
		mockCtrl := NewMockController(injector)
		contextHandler := mockCtrl.ResolveContextHandler()
		if contextHandler != contextHandler {
			t.Fatalf("expected %v, got %v", contextHandler, contextHandler)
		}
	})
}

func TestMockController_ResolveEnvPrinter(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		expectedEnvPrinter := &env.MockEnvPrinter{}
		injector := di.NewMockInjector()
		mockCtrl := NewMockController(injector)
		mockCtrl.ResolveEnvPrinterFunc = func(name string) env.EnvPrinter {
			return expectedEnvPrinter
		}
		envPrinter := mockCtrl.ResolveEnvPrinter("envPrinter")
		if envPrinter != expectedEnvPrinter {
			t.Fatalf("expected %v, got %v", expectedEnvPrinter, envPrinter)
		}
	})

	t.Run("NoResolveEnvPrinterFunc", func(t *testing.T) {
		injector := di.NewMockInjector()
		mockCtrl := NewMockController(injector)
		envPrinter := mockCtrl.ResolveEnvPrinter("envPrinter")
		if envPrinter != envPrinter {
			t.Fatalf("expected %v, got %v", envPrinter, envPrinter)
		}
	})
}

func TestMockController_ResolveAllEnvPrinters(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		expectedEnvPrinters := []env.EnvPrinter{&env.MockEnvPrinter{}, &env.MockEnvPrinter{}}
		injector := di.NewMockInjector()
		mockCtrl := NewMockController(injector)
		mockCtrl.ResolveAllEnvPrintersFunc = func() []env.EnvPrinter {
			return expectedEnvPrinters
		}
		envPrinters := mockCtrl.ResolveAllEnvPrinters()
		if len(envPrinters) != len(expectedEnvPrinters) {
			t.Fatalf("expected %v, got %v", len(expectedEnvPrinters), len(envPrinters))
		}
		for i, printer := range envPrinters {
			if printer != expectedEnvPrinters[i] {
				t.Fatalf("expected %v, got %v", expectedEnvPrinters[i], printer)
			}
		}
	})

	t.Run("NoResolveAllEnvPrintersFunc", func(t *testing.T) {
		injector := di.NewMockInjector()
		mockCtrl := NewMockController(injector)
		envPrinters := mockCtrl.ResolveAllEnvPrinters()
		if len(envPrinters) != 0 {
			t.Fatalf("expected %v, got %v", 0, len(envPrinters))
		}
	})
}

func TestMockController_ResolveShell(t *testing.T) {
	t.Run("ResolveShell", func(t *testing.T) {
		expectedShell := &shell.MockShell{}
		injector := di.NewMockInjector()
		mockCtrl := NewMockController(injector)
		mockCtrl.ResolveShellFunc = func() shell.Shell {
			return expectedShell
		}
		shellInstance := mockCtrl.ResolveShell()
		if shellInstance != expectedShell {
			t.Fatalf("expected %v, got %v", expectedShell, shellInstance)
		}
	})

	t.Run("NoResolveShellFunc", func(t *testing.T) {
		injector := di.NewMockInjector()
		mockCtrl := NewMockController(injector)
		shellInstance := mockCtrl.ResolveShell()
		if shellInstance != shellInstance {
			t.Fatalf("expected %v, got %v", shellInstance, shellInstance)
		}
	})
}

func TestMockController_ResolveSecureShell(t *testing.T) {
	t.Run("ResolveSecureShell", func(t *testing.T) {
		expectedSecureShell := &shell.MockShell{}
		injector := di.NewMockInjector()
		mockCtrl := NewMockController(injector)
		mockCtrl.ResolveSecureShellFunc = func() shell.Shell {
			return expectedSecureShell
		}
		secureShell := mockCtrl.ResolveSecureShell()
		if secureShell != expectedSecureShell {
			t.Fatalf("expected %v, got %v", expectedSecureShell, secureShell)
		}
	})

	t.Run("NoResolveSecureShellFunc", func(t *testing.T) {
		injector := di.NewMockInjector()
		mockCtrl := NewMockController(injector)
		secureShell := mockCtrl.ResolveSecureShell()
		if secureShell != secureShell {
			t.Fatalf("expected %v, got %v", secureShell, secureShell)
		}
	})
}

func TestMockController_ResolveNetworkManager(t *testing.T) {
	t.Run("ResolveNetworkManager", func(t *testing.T) {
		expectedNetworkManager := &network.MockNetworkManager{}
		injector := di.NewMockInjector()
		mockCtrl := NewMockController(injector)
		mockCtrl.ResolveNetworkManagerFunc = func() network.NetworkManager {
			return expectedNetworkManager
		}
		networkManager := mockCtrl.ResolveNetworkManager()
		if networkManager != expectedNetworkManager {
			t.Fatalf("expected %v, got %v", expectedNetworkManager, networkManager)
		}
	})

	t.Run("NoResolveNetworkManagerFunc", func(t *testing.T) {
		injector := di.NewMockInjector()
		mockCtrl := NewMockController(injector)
		networkManager := mockCtrl.ResolveNetworkManager()
		if networkManager != networkManager {
			t.Fatalf("expected %v, got %v", networkManager, networkManager)
		}
	})
}

func TestMockController_ResolveService(t *testing.T) {
	t.Run("ResolveService", func(t *testing.T) {
		expectedService := &services.MockService{}
		injector := di.NewMockInjector()
		mockCtrl := NewMockController(injector)
		mockCtrl.ResolveServiceFunc = func(name string) services.Service {
			return expectedService
		}
		service := mockCtrl.ResolveService("service")
		if service != expectedService {
			t.Fatalf("expected %v, got %v", expectedService, service)
		}
	})

	t.Run("NoResolveServiceFunc", func(t *testing.T) {
		injector := di.NewMockInjector()
		mockCtrl := NewMockController(injector)
		service := mockCtrl.ResolveService("service")
		if service != service {
			t.Fatalf("expected %v, got %v", service, service)
		}
	})
}

func TestMockController_ResolveAllServices(t *testing.T) {
	t.Run("ResolveAllServices", func(t *testing.T) {
		expectedServices := []services.Service{&services.MockService{}, &services.MockService{}}
		injector := di.NewMockInjector()
		mockCtrl := NewMockController(injector)
		mockCtrl.ResolveAllServicesFunc = func() []services.Service {
			return expectedServices
		}
		services := mockCtrl.ResolveAllServices()
		if len(services) != len(expectedServices) {
			t.Fatalf("expected %v, got %v", len(expectedServices), len(services))
		}
		for i, service := range services {
			if service != expectedServices[i] {
				t.Fatalf("expected %v, got %v", expectedServices[i], service)
			}
		}
	})

	t.Run("NoResolveAllServicesFunc", func(t *testing.T) {
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
		expectedVirtualMachine := &virt.MockVirt{}
		injector := di.NewMockInjector()
		mockCtrl := NewMockController(injector)
		mockCtrl.ResolveVirtualMachineFunc = func() virt.VirtualMachine {
			return expectedVirtualMachine
		}
		virtualMachine := mockCtrl.ResolveVirtualMachine()
		if virtualMachine != expectedVirtualMachine {
			t.Fatalf("expected %v, got %v", expectedVirtualMachine, virtualMachine)
		}
	})

	t.Run("NoResolveVirtualMachineFunc", func(t *testing.T) {
		injector := di.NewMockInjector()
		mockCtrl := NewMockController(injector)
		virtualMachine := mockCtrl.ResolveVirtualMachine()
		if virtualMachine != virtualMachine {
			t.Fatalf("expected %v, got %v", virtualMachine, virtualMachine)
		}
	})
}

func TestMockController_ResolveContainerRuntime(t *testing.T) {
	t.Run("ResolveContainerRuntime", func(t *testing.T) {
		expectedContainerRuntime := &virt.MockVirt{}
		injector := di.NewMockInjector()
		mockCtrl := NewMockController(injector)
		mockCtrl.ResolveContainerRuntimeFunc = func() virt.ContainerRuntime {
			return expectedContainerRuntime
		}
		containerRuntime := mockCtrl.ResolveContainerRuntime()
		if containerRuntime != expectedContainerRuntime {
			t.Fatalf("expected %v, got %v", expectedContainerRuntime, containerRuntime)
		}
	})

	t.Run("NoResolveContainerRuntimeFunc", func(t *testing.T) {
		injector := di.NewMockInjector()
		mockCtrl := NewMockController(injector)
		containerRuntime := mockCtrl.ResolveContainerRuntime()
		if containerRuntime != containerRuntime {
			t.Fatalf("expected %v, got %v", containerRuntime, containerRuntime)
		}
	})
}
