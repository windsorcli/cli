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
	t.Run("ResolveConfigHandler", func(t *testing.T) {
		expectedConfigHandler := config.NewMockConfigHandler()
		injector := di.NewMockInjector()
		mockCtrl := NewMockController(injector)
		mockCtrl.ResolveConfigHandlerFunc = func() (config.ConfigHandler, error) {
			return expectedConfigHandler, nil
		}
		configHandler, err := mockCtrl.ResolveConfigHandler()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if configHandler != expectedConfigHandler {
			t.Fatalf("expected %v, got %v", expectedConfigHandler, configHandler)
		}
	})

	t.Run("NoResolveConfigHandlerFunc", func(t *testing.T) {
		injector := di.NewMockInjector()
		mockCtrl := NewMockController(injector)
		if _, err := mockCtrl.ResolveConfigHandler(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestMockController_ResolveContextHandler(t *testing.T) {
	t.Run("ResolveContextHandler", func(t *testing.T) {
		expectedContextHandler := context.NewMockContext()
		injector := di.NewMockInjector()
		mockCtrl := NewMockController(injector)
		mockCtrl.ResolveContextHandlerFunc = func() (context.ContextHandler, error) {
			return expectedContextHandler, nil
		}
		contextHandler, err := mockCtrl.ResolveContextHandler()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if contextHandler != expectedContextHandler {
			t.Fatalf("expected %v, got %v", expectedContextHandler, contextHandler)
		}
	})

	t.Run("NoResolveContextHandlerFunc", func(t *testing.T) {
		injector := di.NewMockInjector()
		mockCtrl := NewMockController(injector)
		if _, err := mockCtrl.ResolveContextHandler(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestMockController_ResolveEnvPrinter(t *testing.T) {
	t.Run("ResolveEnvPrinter", func(t *testing.T) {
		expectedEnvPrinter := &env.MockEnvPrinter{}
		injector := di.NewMockInjector()
		mockCtrl := NewMockController(injector)
		mockCtrl.ResolveEnvPrinterFunc = func(name string) (env.EnvPrinter, error) {
			return expectedEnvPrinter, nil
		}
		envPrinter, err := mockCtrl.ResolveEnvPrinter("envPrinter")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if envPrinter != expectedEnvPrinter {
			t.Fatalf("expected %v, got %v", expectedEnvPrinter, envPrinter)
		}
	})

	t.Run("NoResolveEnvPrinterFunc", func(t *testing.T) {
		injector := di.NewMockInjector()
		mockCtrl := NewMockController(injector)
		if _, err := mockCtrl.ResolveEnvPrinter("envPrinter"); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestMockController_ResolveAllEnvPrinters(t *testing.T) {
	t.Run("ResolveAllEnvPrinters", func(t *testing.T) {
		expectedEnvPrinters := []env.EnvPrinter{&env.MockEnvPrinter{}, &env.MockEnvPrinter{}}
		injector := di.NewMockInjector()
		mockCtrl := NewMockController(injector)
		mockCtrl.ResolveAllEnvPrintersFunc = func() ([]env.EnvPrinter, error) {
			return expectedEnvPrinters, nil
		}
		envPrinters, err := mockCtrl.ResolveAllEnvPrinters()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
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
		if _, err := mockCtrl.ResolveAllEnvPrinters(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestMockController_ResolveShell(t *testing.T) {
	t.Run("ResolveShell", func(t *testing.T) {
		expectedShell := &shell.MockShell{}
		injector := di.NewMockInjector()
		mockCtrl := NewMockController(injector)
		mockCtrl.ResolveShellFunc = func() (shell.Shell, error) {
			return expectedShell, nil
		}
		shellInstance, err := mockCtrl.ResolveShell()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if shellInstance != expectedShell {
			t.Fatalf("expected %v, got %v", expectedShell, shellInstance)
		}
	})

	t.Run("NoResolveShellFunc", func(t *testing.T) {
		injector := di.NewMockInjector()
		mockCtrl := NewMockController(injector)
		if _, err := mockCtrl.ResolveShell(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestMockController_ResolveSecureShell(t *testing.T) {
	t.Run("ResolveSecureShell", func(t *testing.T) {
		expectedSecureShell := &shell.MockShell{}
		injector := di.NewMockInjector()
		mockCtrl := NewMockController(injector)
		mockCtrl.ResolveSecureShellFunc = func() (shell.Shell, error) {
			return expectedSecureShell, nil
		}
		secureShell, err := mockCtrl.ResolveSecureShell()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if secureShell != expectedSecureShell {
			t.Fatalf("expected %v, got %v", expectedSecureShell, secureShell)
		}
	})

	t.Run("NoResolveSecureShellFunc", func(t *testing.T) {
		injector := di.NewMockInjector()
		mockCtrl := NewMockController(injector)
		if _, err := mockCtrl.ResolveSecureShell(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestMockController_ResolveNetworkManager(t *testing.T) {
	t.Run("ResolveNetworkManager", func(t *testing.T) {
		expectedNetworkManager := &network.MockNetworkManager{}
		injector := di.NewMockInjector()
		mockCtrl := NewMockController(injector)
		mockCtrl.ResolveNetworkManagerFunc = func() (network.NetworkManager, error) {
			return expectedNetworkManager, nil
		}
		networkManager, err := mockCtrl.ResolveNetworkManager()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if networkManager != expectedNetworkManager {
			t.Fatalf("expected %v, got %v", expectedNetworkManager, networkManager)
		}
	})

	t.Run("NoResolveNetworkManagerFunc", func(t *testing.T) {
		injector := di.NewMockInjector()
		mockCtrl := NewMockController(injector)
		if _, err := mockCtrl.ResolveNetworkManager(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestMockController_ResolveService(t *testing.T) {
	t.Run("ResolveService", func(t *testing.T) {
		expectedService := &services.MockService{}
		injector := di.NewMockInjector()
		mockCtrl := NewMockController(injector)
		mockCtrl.ResolveServiceFunc = func(name string) (services.Service, error) {
			return expectedService, nil
		}
		service, err := mockCtrl.ResolveService("service")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if service != expectedService {
			t.Fatalf("expected %v, got %v", expectedService, service)
		}
	})

	t.Run("NoResolveServiceFunc", func(t *testing.T) {
		injector := di.NewMockInjector()
		mockCtrl := NewMockController(injector)
		if _, err := mockCtrl.ResolveService("service"); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestMockController_ResolveAllServices(t *testing.T) {
	t.Run("ResolveAllServices", func(t *testing.T) {
		expectedServices := []services.Service{&services.MockService{}, &services.MockService{}}
		injector := di.NewMockInjector()
		mockCtrl := NewMockController(injector)
		mockCtrl.ResolveAllServicesFunc = func() ([]services.Service, error) {
			return expectedServices, nil
		}
		services, err := mockCtrl.ResolveAllServices()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
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
		if _, err := mockCtrl.ResolveAllServices(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestMockController_ResolveVirtualMachine(t *testing.T) {
	t.Run("ResolveVirtualMachine", func(t *testing.T) {
		expectedVirtualMachine := &virt.MockVirt{}
		injector := di.NewMockInjector()
		mockCtrl := NewMockController(injector)
		mockCtrl.ResolveVirtualMachineFunc = func() (virt.VirtualMachine, error) {
			return expectedVirtualMachine, nil
		}
		virtualMachine, err := mockCtrl.ResolveVirtualMachine()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if virtualMachine != expectedVirtualMachine {
			t.Fatalf("expected %v, got %v", expectedVirtualMachine, virtualMachine)
		}
	})

	t.Run("NoResolveVirtualMachineFunc", func(t *testing.T) {
		injector := di.NewMockInjector()
		mockCtrl := NewMockController(injector)
		if _, err := mockCtrl.ResolveVirtualMachine(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestMockController_ResolveContainerRuntime(t *testing.T) {
	t.Run("ResolveContainerRuntime", func(t *testing.T) {
		expectedContainerRuntime := &virt.MockVirt{}
		injector := di.NewMockInjector()
		mockCtrl := NewMockController(injector)
		mockCtrl.ResolveContainerRuntimeFunc = func() (virt.ContainerRuntime, error) {
			return expectedContainerRuntime, nil
		}
		containerRuntime, err := mockCtrl.ResolveContainerRuntime()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if containerRuntime != expectedContainerRuntime {
			t.Fatalf("expected %v, got %v", expectedContainerRuntime, containerRuntime)
		}
	})

	t.Run("NoResolveContainerRuntimeFunc", func(t *testing.T) {
		injector := di.NewMockInjector()
		mockCtrl := NewMockController(injector)
		if _, err := mockCtrl.ResolveContainerRuntime(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}
