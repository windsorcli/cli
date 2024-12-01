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
		mockCtrl := NewMockController()
		mockCtrl.InitializeFunc = func() error {
			return nil
		}
		if err := mockCtrl.Initialize(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("NoInitializeFunc", func(t *testing.T) {
		mockCtrl := NewMockController()
		if err := mockCtrl.Initialize(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestMockController_InitializeComponents(t *testing.T) {
	t.Run("InitializeComponents", func(t *testing.T) {
		mockCtrl := NewMockController()
		mockCtrl.InitializeComponentsFunc = func() error {
			return nil
		}
		if err := mockCtrl.InitializeComponents(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("NoInitializeComponentsFunc", func(t *testing.T) {
		mockCtrl := NewMockController()
		if err := mockCtrl.InitializeComponents(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestMockController_CreateCommonComponents(t *testing.T) {
	t.Run("CreateCommonComponents", func(t *testing.T) {
		mockCtrl := NewMockController()
		mockCtrl.CreateCommonComponentsFunc = func() error {
			return nil
		}
		if err := mockCtrl.CreateCommonComponents(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("NoCreateCommonComponentsFunc", func(t *testing.T) {
		mockCtrl := NewMockController()
		if err := mockCtrl.CreateCommonComponents(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestMockController_CreateEnvComponents(t *testing.T) {
	t.Run("CreateEnvComponents", func(t *testing.T) {
		mockCtrl := NewMockController()
		mockCtrl.CreateEnvComponentsFunc = func() error {
			return nil
		}
		if err := mockCtrl.CreateEnvComponents(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("NoCreateEnvComponentsFunc", func(t *testing.T) {
		mockCtrl := NewMockController()
		if err := mockCtrl.CreateEnvComponents(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestMockController_CreateServiceComponents(t *testing.T) {
	t.Run("CreateServiceComponents", func(t *testing.T) {
		mockCtrl := NewMockController()
		mockCtrl.CreateServiceComponentsFunc = func() error {
			return nil
		}
		if err := mockCtrl.CreateServiceComponents(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("NoCreateServiceComponentsFunc", func(t *testing.T) {
		mockCtrl := NewMockController()
		if err := mockCtrl.CreateServiceComponents(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestMockController_CreateVirtualizationComponents(t *testing.T) {
	t.Run("CreateVirtualizationComponents", func(t *testing.T) {
		mockCtrl := NewMockController()
		mockCtrl.CreateVirtualizationComponentsFunc = func() error {
			return nil
		}
		if err := mockCtrl.CreateVirtualizationComponents(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("NoCreateVirtualizationComponentsFunc", func(t *testing.T) {
		mockCtrl := NewMockController()
		if err := mockCtrl.CreateVirtualizationComponents(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestMockController_ResolveInjector(t *testing.T) {
	t.Run("ResolveInjector", func(t *testing.T) {
		expectedInjector := di.NewInjector()
		mockCtrl := NewMockController()
		mockCtrl.ResolveInjectorFunc = func() di.Injector {
			return expectedInjector
		}
		if injector := mockCtrl.ResolveInjector(); injector != expectedInjector {
			t.Fatalf("expected %v, got %v", expectedInjector, injector)
		}
	})

	t.Run("NoResolveInjectorFunc", func(t *testing.T) {
		mockCtrl := NewMockController()
		if injector := mockCtrl.ResolveInjector(); injector != nil {
			t.Fatalf("expected nil, got %v", injector)
		}
	})
}

func TestMockController_ResolveConfigHandler(t *testing.T) {
	t.Run("ResolveConfigHandler", func(t *testing.T) {
		expectedConfigHandler := config.NewMockConfigHandler()
		mockCtrl := NewMockController()
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
		mockCtrl := NewMockController()
		if _, err := mockCtrl.ResolveConfigHandler(); err == nil {
			t.Fatalf("expected an error, got nil")
		}
	})
}

func TestMockController_ResolveContextHandler(t *testing.T) {
	t.Run("ResolveContextHandler", func(t *testing.T) {
		expectedContextHandler := context.NewMockContext()
		mockCtrl := NewMockController()
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
		mockCtrl := NewMockController()
		if _, err := mockCtrl.ResolveContextHandler(); err == nil {
			t.Fatalf("expected an error, got nil")
		}
	})
}

func TestMockController_ResolveEnvPrinter(t *testing.T) {
	t.Run("ResolveEnvPrinter", func(t *testing.T) {
		expectedEnvPrinter := &env.MockEnvPrinter{}
		mockCtrl := NewMockController()
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
		mockCtrl := NewMockController()
		if _, err := mockCtrl.ResolveEnvPrinter("envPrinter"); err == nil {
			t.Fatalf("expected an error, got nil")
		}
	})
}

func TestMockController_ResolveAllEnvPrinters(t *testing.T) {
	t.Run("ResolveAllEnvPrinters", func(t *testing.T) {
		expectedEnvPrinters := []env.EnvPrinter{&env.MockEnvPrinter{}, &env.MockEnvPrinter{}}
		mockCtrl := NewMockController()
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
		mockCtrl := NewMockController()
		if _, err := mockCtrl.ResolveAllEnvPrinters(); err == nil {
			t.Fatalf("expected an error, got nil")
		}
	})
}

func TestMockController_ResolveShell(t *testing.T) {
	t.Run("ResolveShell", func(t *testing.T) {
		expectedShell := &shell.MockShell{}
		mockCtrl := NewMockController()
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
		mockCtrl := NewMockController()
		if _, err := mockCtrl.ResolveShell(); err == nil {
			t.Fatalf("expected an error, got nil")
		}
	})
}

func TestMockController_ResolveSecureShell(t *testing.T) {
	t.Run("ResolveSecureShell", func(t *testing.T) {
		expectedSecureShell := &shell.MockShell{}
		mockCtrl := NewMockController()
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
		mockCtrl := NewMockController()
		if _, err := mockCtrl.ResolveSecureShell(); err == nil {
			t.Fatalf("expected an error, got nil")
		}
	})
}

func TestMockController_ResolveNetworkManager(t *testing.T) {
	t.Run("ResolveNetworkManager", func(t *testing.T) {
		expectedNetworkManager := &network.MockNetworkManager{}
		mockCtrl := NewMockController()
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
		mockCtrl := NewMockController()
		if _, err := mockCtrl.ResolveNetworkManager(); err == nil {
			t.Fatalf("expected an error, got nil")
		}
	})
}

func TestMockController_ResolveService(t *testing.T) {
	t.Run("ResolveService", func(t *testing.T) {
		expectedService := &services.MockService{}
		mockCtrl := NewMockController()
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
		mockCtrl := NewMockController()
		if _, err := mockCtrl.ResolveService("service"); err == nil {
			t.Fatalf("expected an error, got nil")
		}
	})
}

func TestMockController_ResolveAllServices(t *testing.T) {
	t.Run("ResolveAllServices", func(t *testing.T) {
		expectedServices := []services.Service{&services.MockService{}, &services.MockService{}}
		mockCtrl := NewMockController()
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
		mockCtrl := NewMockController()
		if _, err := mockCtrl.ResolveAllServices(); err == nil {
			t.Fatalf("expected an error, got nil")
		}
	})
}

func TestMockController_ResolveVirtualMachine(t *testing.T) {
	t.Run("ResolveVirtualMachine", func(t *testing.T) {
		expectedVirtualMachine := &virt.MockVirt{}
		mockCtrl := NewMockController()
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
		mockCtrl := NewMockController()
		if _, err := mockCtrl.ResolveVirtualMachine(); err == nil {
			t.Fatalf("expected an error, got nil")
		}
	})
}

func TestMockController_ResolveContainerRuntime(t *testing.T) {
	t.Run("ResolveContainerRuntime", func(t *testing.T) {
		expectedContainerRuntime := &virt.MockVirt{}
		mockCtrl := NewMockController()
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
		mockCtrl := NewMockController()
		if _, err := mockCtrl.ResolveContainerRuntime(); err == nil {
			t.Fatalf("expected an error, got nil")
		}
	})
}
