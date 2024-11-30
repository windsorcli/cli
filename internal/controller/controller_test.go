package controller

import (
	"fmt"
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

func TestController_Initialize(t *testing.T) {
	t.Run("Initialize", func(t *testing.T) {
		injector := di.NewInjector()
		controller := &BaseController{}
		err := controller.Initialize(injector)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestController_ResolveInjector(t *testing.T) {
	t.Run("ResolveInjector", func(t *testing.T) {
		injector := di.NewInjector()
		controller := &BaseController{injector: injector}
		resolvedInjector := controller.ResolveInjector()
		if resolvedInjector != injector {
			t.Fatalf("expected %v, got %v", injector, resolvedInjector)
		}
	})
}

func TestController_ResolveConfigHandler(t *testing.T) {
	t.Run("ResolveConfigHandler", func(t *testing.T) {
		injector := di.NewInjector()
		expectedConfigHandler := &config.MockConfigHandler{}
		injector.Register("configHandler", expectedConfigHandler)
		controller := &BaseController{injector: injector}
		configHandler, err := controller.ResolveConfigHandler()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if configHandler != expectedConfigHandler {
			t.Fatalf("expected %v, got %v", expectedConfigHandler, configHandler)
		}
	})

	t.Run("ErrorResolvingConfigHandler", func(t *testing.T) {
		injector := di.NewInjector()
		controller := &BaseController{injector: injector}
		_, err := controller.ResolveConfigHandler()
		if err == nil {
			t.Fatalf("expected an error, got nil")
		} else {
			t.Logf("expected error received: %v", err)
		}
	})

	t.Run("ErrorCastingConfigHandler", func(t *testing.T) {
		injector := di.NewInjector()
		injector.Register("configHandler", "invalidConfigHandler")
		controller := &BaseController{injector: injector}
		_, err := controller.ResolveConfigHandler()
		if err == nil {
			t.Fatalf("expected an error, got nil")
		} else {
			t.Logf("expected error received: %v", err)
		}
	})
}

func TestController_ResolveContextHandler(t *testing.T) {
	t.Run("ResolveContextHandler", func(t *testing.T) {
		injector := di.NewInjector()
		expectedContextHandler := context.NewMockContext()
		injector.Register("contextHandler", expectedContextHandler)
		controller := &BaseController{injector: injector}
		contextHandler, err := controller.ResolveContextHandler()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if contextHandler != expectedContextHandler {
			t.Fatalf("expected %v, got %v", expectedContextHandler, contextHandler)
		}
	})

	t.Run("ErrorResolvingContextHandler", func(t *testing.T) {
		injector := di.NewInjector()
		controller := &BaseController{injector: injector}
		_, err := controller.ResolveContextHandler()
		if err == nil {
			t.Fatalf("expected an error, got nil")
		} else {
			t.Logf("expected error received: %v", err)
		}
	})

	t.Run("ErrorCastingContextHandler", func(t *testing.T) {
		injector := di.NewInjector()
		injector.Register("contextHandler", "invalidContextHandler")
		controller := &BaseController{injector: injector}
		_, err := controller.ResolveContextHandler()
		if err == nil {
			t.Fatalf("expected an error, got nil")
		} else {
			t.Logf("expected error received: %v", err)
		}
	})
}

func TestController_ResolveEnvPrinter(t *testing.T) {
	t.Run("ResolveEnvPrinter", func(t *testing.T) {
		injector := di.NewInjector()
		expectedEnvPrinter := &env.MockEnvPrinter{}
		injector.Register("envPrinter", expectedEnvPrinter)
		controller := &BaseController{injector: injector}
		envPrinter, err := controller.ResolveEnvPrinter("envPrinter")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if envPrinter != expectedEnvPrinter {
			t.Fatalf("expected %v, got %v", expectedEnvPrinter, envPrinter)
		}
	})

	t.Run("ErrorResolvingEnvPrinter", func(t *testing.T) {
		injector := di.NewInjector()
		controller := &BaseController{injector: injector}
		_, err := controller.ResolveEnvPrinter("envPrinter")
		if err == nil {
			t.Fatalf("expected an error, got nil")
		} else {
			t.Logf("expected error received: %v", err)
		}
	})

	t.Run("ErrorCastingEnvPrinter", func(t *testing.T) {
		injector := di.NewInjector()
		injector.Register("envPrinter", "invalidEnvPrinter")
		controller := &BaseController{injector: injector}
		_, err := controller.ResolveEnvPrinter("envPrinter")
		if err == nil {
			t.Fatalf("expected an error, got nil")
		} else {
			t.Logf("expected error received: %v", err)
		}
	})
}

func TestController_ResolveAllEnvPrinters(t *testing.T) {
	t.Run("ResolveAllEnvPrinters", func(t *testing.T) {
		injector := di.NewInjector()
		expectedEnvPrinters := []env.EnvPrinter{&env.MockEnvPrinter{}, &env.MockEnvPrinter{}}
		for i, printer := range expectedEnvPrinters {
			injector.Register(fmt.Sprintf("envPrinter%d", i+1), printer)
		}
		controller := &BaseController{injector: injector}
		envPrinters, err := controller.ResolveAllEnvPrinters()
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

	t.Run("ErrorResolvingEnvPrinters", func(t *testing.T) {
		injector := di.NewInjector()
		controller := &BaseController{injector: injector}
		_, err := controller.ResolveAllEnvPrinters()
		if err == nil {
			t.Fatalf("expected an error, got nil")
		} else {
			t.Logf("expected error received: %v", err)
		}
	})
}

func TestController_ResolveShell(t *testing.T) {
	t.Run("ResolveShell", func(t *testing.T) {
		injector := di.NewInjector()
		expectedShell := &shell.MockShell{}
		injector.Register("shell", expectedShell)
		controller := &BaseController{injector: injector}
		shellInstance, err := controller.ResolveShell()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if shellInstance != expectedShell {
			t.Fatalf("expected %v, got %v", expectedShell, shellInstance)
		}
	})

	t.Run("ErrorResolvingShell", func(t *testing.T) {
		injector := di.NewInjector()
		controller := &BaseController{injector: injector}
		_, err := controller.ResolveShell()
		if err == nil {
			t.Fatalf("expected an error, got nil")
		} else {
			t.Logf("expected error received: %v", err)
		}
	})

	t.Run("ErrorCastingShell", func(t *testing.T) {
		injector := di.NewInjector()
		injector.Register("shell", "invalidShell")
		controller := &BaseController{injector: injector}
		_, err := controller.ResolveShell()
		if err == nil {
			t.Fatalf("expected an error, got nil")
		} else {
			t.Logf("expected error received: %v", err)
		}
	})
}

func TestController_ResolveNetworkManager(t *testing.T) {
	t.Run("ResolveNetworkManager", func(t *testing.T) {
		injector := di.NewInjector()
		expectedNetworkManager := &network.MockNetworkManager{}
		injector.Register("networkManager", expectedNetworkManager)
		controller := &BaseController{injector: injector}
		networkManager, err := controller.ResolveNetworkManager()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if networkManager != expectedNetworkManager {
			t.Fatalf("expected %v, got %v", expectedNetworkManager, networkManager)
		}
	})

	t.Run("ErrorResolvingNetworkManager", func(t *testing.T) {
		injector := di.NewInjector()
		controller := &BaseController{injector: injector}
		_, err := controller.ResolveNetworkManager()
		if err == nil {
			t.Fatalf("expected an error, got nil")
		} else {
			t.Logf("expected error received: %v", err)
		}
	})

	t.Run("ErrorCastingNetworkManager", func(t *testing.T) {
		injector := di.NewInjector()
		injector.Register("networkManager", "invalidNetworkManager")
		controller := &BaseController{injector: injector}
		_, err := controller.ResolveNetworkManager()
		if err == nil {
			t.Fatalf("expected an error, got nil")
		} else {
			t.Logf("expected error received: %v", err)
		}
	})
}

func TestController_ResolveService(t *testing.T) {
	t.Run("ResolveService", func(t *testing.T) {
		injector := di.NewInjector()
		expectedService := &services.MockService{}
		injector.Register("service", expectedService)
		controller := &BaseController{injector: injector}
		service, err := controller.ResolveService("service")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if service != expectedService {
			t.Fatalf("expected %v, got %v", expectedService, service)
		}
	})

	t.Run("ErrorResolvingService", func(t *testing.T) {
		injector := di.NewInjector()
		controller := &BaseController{injector: injector}
		_, err := controller.ResolveService("service")
		if err == nil {
			t.Fatalf("expected an error, got nil")
		} else {
			t.Logf("expected error received: %v", err)
		}
	})

	t.Run("ErrorCastingService", func(t *testing.T) {
		injector := di.NewInjector()
		injector.Register("service", "invalidService")
		controller := &BaseController{injector: injector}
		_, err := controller.ResolveService("service")
		if err == nil {
			t.Fatalf("expected an error, got nil")
		} else {
			t.Logf("expected error received: %v", err)
		}
	})
}

func TestController_ResolveAllServices(t *testing.T) {
	t.Run("ResolveAllServices", func(t *testing.T) {
		injector := di.NewInjector()
		expectedService1 := &services.MockService{}
		expectedService2 := &services.MockService{}
		injector.Register("service1", expectedService1)
		injector.Register("service2", expectedService2)
		controller := &BaseController{injector: injector}
		services, err := controller.ResolveAllServices()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if len(services) != 2 {
			t.Fatalf("expected %v, got %v", 2, len(services))
		}
		if services[0] != expectedService1 && services[1] != expectedService2 {
			t.Fatalf("expected services to match registered services")
		}
	})

	t.Run("ErrorResolvingAllServices", func(t *testing.T) {
		injector := di.NewInjector()
		controller := &BaseController{injector: injector}
		_, err := controller.ResolveAllServices()
		if err == nil {
			t.Fatalf("expected an error, got nil")
		} else {
			t.Logf("expected error received: %v", err)
		}
	})

	t.Run("ErrorCastingService", func(t *testing.T) {
		injector := di.NewInjector()
		injector.Register("service1", "invalidService")
		controller := &BaseController{injector: injector}
		_, err := controller.ResolveAllServices()
		if err == nil {
			t.Fatalf("expected an error, got nil")
		} else {
			t.Logf("expected error received: %v", err)
		}
	})
}

func TestController_ResolveVirtualMachine(t *testing.T) {
	t.Run("ResolveVirtualMachine", func(t *testing.T) {
		injector := di.NewInjector()
		expectedVirtualMachine := &virt.MockVirt{}
		injector.Register("virtualMachine", expectedVirtualMachine)
		controller := &BaseController{injector: injector}
		virtualMachine, err := controller.ResolveVirtualMachine()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if virtualMachine != expectedVirtualMachine {
			t.Fatalf("expected %v, got %v", expectedVirtualMachine, virtualMachine)
		}
	})

	t.Run("ErrorResolvingVirtualMachine", func(t *testing.T) {
		injector := di.NewInjector()
		controller := &BaseController{injector: injector}
		_, err := controller.ResolveVirtualMachine()
		if err == nil {
			t.Fatalf("expected an error, got nil")
		} else {
			t.Logf("expected error received: %v", err)
		}
	})

	t.Run("ErrorCastingVirtualMachine", func(t *testing.T) {
		injector := di.NewInjector()
		injector.Register("virtualMachine", "invalidVirtualMachine")
		controller := &BaseController{injector: injector}
		_, err := controller.ResolveVirtualMachine()
		if err == nil {
			t.Fatalf("expected an error, got nil")
		} else {
			t.Logf("expected error received: %v", err)
		}
	})
}

func TestController_ResolveContainerRuntime(t *testing.T) {
	t.Run("ResolveContainerRuntime", func(t *testing.T) {
		injector := di.NewInjector()
		expectedContainerRuntime := &virt.MockVirt{}
		injector.Register("containerRuntime", expectedContainerRuntime)
		controller := &BaseController{injector: injector}
		containerRuntime, err := controller.ResolveContainerRuntime()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if containerRuntime != expectedContainerRuntime {
			t.Fatalf("expected %v, got %v", expectedContainerRuntime, containerRuntime)
		}
	})

	t.Run("ErrorResolvingContainerRuntime", func(t *testing.T) {
		injector := di.NewInjector()
		controller := &BaseController{injector: injector}
		_, err := controller.ResolveContainerRuntime()
		if err == nil {
			t.Fatalf("expected an error, got nil")
		} else {
			t.Logf("expected error received: %v", err)
		}
	})

	t.Run("ErrorCastingContainerRuntime", func(t *testing.T) {
		injector := di.NewInjector()
		injector.Register("containerRuntime", "invalidContainerRuntime")
		controller := &BaseController{injector: injector}
		_, err := controller.ResolveContainerRuntime()
		if err == nil {
			t.Fatalf("expected an error, got nil")
		} else {
			t.Logf("expected error received: %v", err)
		}
	})
}
