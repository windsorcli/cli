package controller

import (
	"fmt"
	"strings"
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

func TestNewController(t *testing.T) {
	t.Run("NewController", func(t *testing.T) {
		// Given a new injector
		injector := di.NewInjector()

		// When creating a new controller
		controller := NewController(injector)

		// Then the controller should not be nil
		if controller == nil {
			t.Fatalf("expected controller, got nil")
		} else {
			t.Logf("Success: controller created")
		}
	})
}

func TestController_Initialize(t *testing.T) {
	t.Run("Initialize", func(t *testing.T) {
		// Given a new injector
		injector := di.NewInjector()

		// And a new controller
		controller := NewController(injector)

		// When initializing the controller
		err := controller.Initialize()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestController_ResolveInjector(t *testing.T) {
	t.Run("ResolveInjector", func(t *testing.T) {
		// Given a new injector
		injector := di.NewInjector()

		// And a new controller
		controller := NewController(injector)

		// When resolving the injector
		resolvedInjector := controller.ResolveInjector()

		// Then the resolved injector should match the original injector
		if resolvedInjector != injector {
			t.Fatalf("expected %v, got %v", injector, resolvedInjector)
		}
	})
}

func TestController_ResolveConfigHandler(t *testing.T) {
	t.Run("ResolveConfigHandler", func(t *testing.T) {
		// Given a new injector
		injector := di.NewInjector()

		// And a mock config handler registered
		expectedConfigHandler := config.NewMockConfigHandler()
		injector.Register("configHandler", expectedConfigHandler)

		// And a new controller
		controller := NewController(injector)

		// When resolving the config handler
		configHandler, err := controller.ResolveConfigHandler()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the resolved config handler should match the expected config handler
		if configHandler != expectedConfigHandler {
			t.Fatalf("expected %v, got %v", expectedConfigHandler, configHandler)
		}
	})

	t.Run("ErrorResolvingConfigHandler", func(t *testing.T) {
		// Given a new injector
		injector := di.NewInjector()

		// And a new controller
		controller := NewController(injector)

		// When resolving the config handler
		_, err := controller.ResolveConfigHandler()

		// Then there should be an error
		if err == nil {
			t.Fatalf("expected an error, got nil")
		} else {
			t.Logf("expected error received: %v", err)
		}
	})

	t.Run("ErrorCastingConfigHandler", func(t *testing.T) {
		// Given a new injector
		injector := di.NewInjector()

		// And an invalid config handler registered
		injector.Register("configHandler", "invalidConfigHandler")

		// And a new controller
		controller := NewController(injector)

		// When resolving the config handler
		_, err := controller.ResolveConfigHandler()

		// Then there should be an error
		if err == nil {
			t.Fatalf("expected an error, got nil")
		} else {
			t.Logf("expected error received: %v", err)
		}
	})
}

func TestController_ResolveContextHandler(t *testing.T) {
	t.Run("ResolveContextHandler", func(t *testing.T) {
		// Given a new injector
		injector := di.NewInjector()

		// And a mock context handler registered
		expectedContextHandler := context.NewMockContext()
		injector.Register("contextHandler", expectedContextHandler)

		// And a new controller
		controller := NewController(injector)

		// When resolving the context handler
		contextHandler, err := controller.ResolveContextHandler()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the resolved context handler should match the expected context handler
		if contextHandler != expectedContextHandler {
			t.Fatalf("expected %v, got %v", expectedContextHandler, contextHandler)
		}
	})

	t.Run("ErrorResolvingContextHandler", func(t *testing.T) {
		// Given a new injector
		injector := di.NewInjector()

		// And a new controller
		controller := NewController(injector)

		// When resolving the context handler
		_, err := controller.ResolveContextHandler()

		// Then there should be an error
		if err == nil {
			t.Fatalf("expected an error, got nil")
		} else {
			t.Logf("expected error received: %v", err)
		}
	})

	t.Run("ErrorCastingContextHandler", func(t *testing.T) {
		// Given a new injector
		injector := di.NewInjector()

		// And an invalid context handler registered
		injector.Register("contextHandler", "invalidContextHandler")

		// And a new controller
		controller := NewController(injector)

		// When resolving the context handler
		_, err := controller.ResolveContextHandler()

		// Then there should be an error
		if err == nil {
			t.Fatalf("expected an error, got nil")
		} else {
			t.Logf("expected error received: %v", err)
		}
	})
}

func TestController_ResolveEnvPrinter(t *testing.T) {
	t.Run("ResolveEnvPrinter", func(t *testing.T) {
		// Given a new injector
		injector := di.NewInjector()

		// And a mock env printer registered
		expectedEnvPrinter := &env.MockEnvPrinter{}
		injector.Register("envPrinter", expectedEnvPrinter)

		// And a new controller
		controller := NewController(injector)

		// When resolving the env printer
		envPrinter, err := controller.ResolveEnvPrinter("envPrinter")

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the resolved env printer should match the expected env printer
		if envPrinter != expectedEnvPrinter {
			t.Fatalf("expected %v, got %v", expectedEnvPrinter, envPrinter)
		}
	})

	t.Run("ErrorResolvingEnvPrinter", func(t *testing.T) {
		// Given a new injector
		injector := di.NewInjector()

		// And a new controller
		controller := NewController(injector)

		// When resolving the env printer
		_, err := controller.ResolveEnvPrinter("envPrinter")

		// Then there should be an error
		if err == nil {
			t.Fatalf("expected an error, got nil")
		} else {
			t.Logf("expected error received: %v", err)
		}
	})

	t.Run("ErrorCastingEnvPrinter", func(t *testing.T) {
		// Given a new injector
		injector := di.NewInjector()

		// And an invalid env printer registered
		injector.Register("envPrinter", "invalidEnvPrinter")

		// And a new controller
		controller := NewController(injector)

		// When resolving the env printer
		_, err := controller.ResolveEnvPrinter("envPrinter")

		// Then there should be an error
		if err == nil {
			t.Fatalf("expected an error, got nil")
		} else {
			t.Logf("expected error received: %v", err)
		}
	})
}

func TestController_ResolveAllEnvPrinters(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new injector
		injector := di.NewInjector()

		// And multiple mock env printers registered
		expectedEnvPrinters := []env.EnvPrinter{&env.MockEnvPrinter{}, &env.MockEnvPrinter{}}
		for i, printer := range expectedEnvPrinters {
			injector.Register(fmt.Sprintf("envPrinter%d", i+1), printer)
		}

		// And a new controller
		controller := NewController(injector)

		// When resolving all env printers
		envPrinters, err := controller.ResolveAllEnvPrinters()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the number of resolved env printers should match the expected number
		if len(envPrinters) != len(expectedEnvPrinters) {
			t.Fatalf("expected %d env printers, got %d", len(expectedEnvPrinters), len(envPrinters))
		}

		// And each resolved env printer should match the expected env printer
		for i, printer := range envPrinters {
			if fmt.Sprintf("%p", printer) != fmt.Sprintf("%p", expectedEnvPrinters[i]) {
				t.Fatalf("expected printer at index %d to be %v, got %v", i, expectedEnvPrinters[i], printer)
			}
		}
	})

	t.Run("ErrorResolvingEnvPrinters", func(t *testing.T) {
		// Given a new injector
		injector := di.NewInjector()

		// And a new controller
		controller := NewController(injector)

		// When resolving all env printers
		_, err := controller.ResolveAllEnvPrinters()

		// Then there should be an error
		if err == nil {
			t.Fatalf("expected an error, got nil")
		} else {
			t.Logf("expected error received: %v", err)
		}
	})
}

func TestController_ResolveShell(t *testing.T) {
	t.Run("ResolveShell", func(t *testing.T) {
		// Given a new injector
		injector := di.NewInjector()

		// And a mock shell registered
		expectedShell := &shell.MockShell{}
		injector.Register("shell", expectedShell)

		// And a new controller
		controller := NewController(injector)

		// When resolving the shell
		shellInstance, err := controller.ResolveShell()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the resolved shell should match the expected shell
		if shellInstance != expectedShell {
			t.Fatalf("expected %v, got %v", expectedShell, shellInstance)
		}
	})

	t.Run("ErrorResolvingShell", func(t *testing.T) {
		// Given a new injector
		injector := di.NewInjector()

		// And a new controller
		controller := NewController(injector)

		// When resolving the shell
		_, err := controller.ResolveShell()

		// Then there should be an error
		if err == nil {
			t.Fatalf("expected an error, got nil")
		} else {
			t.Logf("expected error received: %v", err)
		}
	})

	t.Run("ErrorCastingShell", func(t *testing.T) {
		// Given a new injector
		injector := di.NewInjector()

		// And an invalid shell registered
		injector.Register("shell", "invalidShell")

		// And a new controller
		controller := NewController(injector)

		// When resolving the shell
		_, err := controller.ResolveShell()

		// Then there should be an error
		if err == nil {
			t.Fatalf("expected an error, got nil")
		} else {
			t.Logf("expected error received: %v", err)
		}
	})
}

func TestController_ResolveSecureShell(t *testing.T) {
	t.Run("ResolveSecureShell", func(t *testing.T) {
		// Given a new injector
		injector := di.NewInjector()

		// And a mock secure shell registered
		expectedSecureShell := &shell.MockShell{}
		injector.Register("secureShell", expectedSecureShell)

		// And a new controller
		controller := NewController(injector)

		// When resolving the secure shell
		secureShell, err := controller.ResolveSecureShell()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the resolved secure shell should match the expected secure shell
		if secureShell != expectedSecureShell {
			t.Fatalf("expected %v, got %v", expectedSecureShell, secureShell)
		}
	})

	t.Run("ErrorResolvingSecureShell", func(t *testing.T) {
		// Given a new injector
		injector := di.NewInjector()

		// And a new controller
		controller := NewController(injector)

		// When resolving the secure shell
		_, err := controller.ResolveSecureShell()

		// Then there should be an error
		if err == nil {
			t.Fatalf("expected an error, got nil")
		} else {
			t.Logf("expected error received: %v", err)
		}
	})

	t.Run("ErrorCastingSecureShell", func(t *testing.T) {
		// Given a new injector
		injector := di.NewInjector()

		// And an invalid secure shell registered
		injector.Register("secureShell", "invalidSecureShell")

		// And a new controller
		controller := NewController(injector)

		// When resolving the secure shell
		_, err := controller.ResolveSecureShell()

		// Then there should be an error
		if err == nil {
			t.Fatalf("expected an error, got nil")
		} else {
			t.Logf("expected error received: %v", err)
		}
	})
}

func TestController_ResolveNetworkManager(t *testing.T) {
	t.Run("ResolveNetworkManager", func(t *testing.T) {
		// Given a new injector
		injector := di.NewInjector()

		// And a mock network manager registered
		expectedNetworkManager := &network.MockNetworkManager{}
		injector.Register("networkManager", expectedNetworkManager)

		// And a new controller
		controller := NewController(injector)

		// When resolving the network manager
		networkManager, err := controller.ResolveNetworkManager()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the resolved network manager should match the expected network manager
		if networkManager != expectedNetworkManager {
			t.Fatalf("expected %v, got %v", expectedNetworkManager, networkManager)
		}
	})

	t.Run("ErrorResolvingNetworkManager", func(t *testing.T) {
		// Given a new injector
		injector := di.NewInjector()

		// And a new controller
		controller := NewController(injector)

		// When resolving the network manager
		_, err := controller.ResolveNetworkManager()

		// Then there should be an error
		if err == nil {
			t.Fatalf("expected an error, got nil")
		} else {
			t.Logf("expected error received: %v", err)
		}
	})

	t.Run("ErrorCastingNetworkManager", func(t *testing.T) {
		// Given a new injector
		injector := di.NewInjector()

		// And an invalid network manager registered
		injector.Register("networkManager", "invalidNetworkManager")

		// And a new controller
		controller := NewController(injector)

		// When resolving the network manager
		_, err := controller.ResolveNetworkManager()

		// Then there should be an error
		if err == nil {
			t.Fatalf("expected an error, got nil")
		} else {
			t.Logf("expected error received: %v", err)
		}
	})
}

func TestController_ResolveService(t *testing.T) {
	t.Run("ResolveService", func(t *testing.T) {
		// Given a new injector
		injector := di.NewInjector()

		// And a mock service registered
		expectedService := &services.MockService{}
		injector.Register("service", expectedService)

		// And a new controller
		controller := NewController(injector)

		// When resolving the service
		service, err := controller.ResolveService("service")

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the resolved service should match the expected service
		if service != expectedService {
			t.Fatalf("expected %v, got %v", expectedService, service)
		}
	})

	t.Run("ErrorResolvingService", func(t *testing.T) {
		// Given a new injector
		injector := di.NewInjector()

		// And a new controller
		controller := NewController(injector)

		// When resolving the service
		_, err := controller.ResolveService("service")

		// Then there should be an error
		if err == nil {
			t.Fatalf("expected an error, got nil")
		} else {
			t.Logf("expected error received: %v", err)
		}
	})

	t.Run("ErrorCastingService", func(t *testing.T) {
		// Given a new injector
		injector := di.NewInjector()

		// And an invalid service registered
		injector.Register("service", "invalidService")

		// And a new controller
		controller := NewController(injector)

		// When resolving the service
		_, err := controller.ResolveService("service")

		// Then there should be an error
		if err == nil {
			t.Fatalf("expected an error, got nil")
		} else {
			t.Logf("expected error received: %v", err)
		}
	})
}

func TestController_ResolveAllServices(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new injector
		injector := di.NewInjector()

		// And multiple mock services registered
		expectedService1 := &services.MockService{}
		expectedService2 := &services.MockService{}
		injector.Register("service1", expectedService1)
		injector.Register("service2", expectedService2)

		// And a new controller
		controller := NewController(injector)

		// When resolving all services
		resolvedServices, err := controller.ResolveAllServices()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the number of resolved services should match the expected number
		if len(resolvedServices) != 2 {
			t.Fatalf("expected %v, got %v", 2, len(resolvedServices))
		}

		// And each resolved service should match one of the expected services
		expectedServices := map[*services.MockService]bool{
			expectedService1: false,
			expectedService2: false,
		}

		for _, service := range resolvedServices {
			if mockService, ok := service.(*services.MockService); ok {
				if _, exists := expectedServices[mockService]; exists {
					expectedServices[mockService] = true
				}
			} else {
				t.Fatalf("service is not of type *services.MockService")
			}
		}

		for service, found := range expectedServices {
			if !found {
				t.Fatalf("expected service %v not found", service)
			}
		}
	})

	t.Run("ErrorResolvingAllServices", func(t *testing.T) {
		// Given a new injector
		mockInjector := di.NewMockInjector()
		mockInjector.SetResolveAllError(fmt.Errorf("resolve all error"))

		// And a new controller
		controller := NewController(mockInjector)

		// When resolving all services
		_, err := controller.ResolveAllServices()

		// Then there should be an error
		if err == nil {
			t.Fatalf("expected an error, got nil")
		} else if !strings.Contains(err.Error(), "resolve all error") {
			t.Fatalf("expected error to contain 'resolve all error', got %v", err)
		} else {
			t.Logf("expected error received: %v", err)
		}
	})

	t.Run("ErrorCastingService", func(t *testing.T) {
		// Given a new injector
		injector := di.NewInjector()

		// And an invalid service registered
		injector.Register("service1", "invalidService")

		// And a new controller
		controller := NewController(injector)

		// When resolving all services
		_, err := controller.ResolveAllServices()

		// Then there should be an error
		if err == nil {
			t.Fatalf("expected an error, got nil")
		} else {
			t.Logf("expected error received: %v", err)
		}
	})
}

func TestController_ResolveVirtualMachine(t *testing.T) {
	t.Run("ResolveVirtualMachine", func(t *testing.T) {
		// Given a new injector
		injector := di.NewInjector()

		// And a mock virtual machine registered
		expectedVirtualMachine := &virt.MockVirt{}
		injector.Register("virtualMachine", expectedVirtualMachine)

		// And a new controller
		controller := NewController(injector)

		// When resolving the virtual machine
		virtualMachine, err := controller.ResolveVirtualMachine()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the resolved virtual machine should match the expected virtual machine
		if virtualMachine != expectedVirtualMachine {
			t.Fatalf("expected %v, got %v", expectedVirtualMachine, virtualMachine)
		}
	})

	t.Run("ErrorResolvingVirtualMachine", func(t *testing.T) {
		// Given a new injector
		injector := di.NewInjector()

		// And a new controller
		controller := NewController(injector)

		// When resolving the virtual machine
		_, err := controller.ResolveVirtualMachine()

		// Then there should be an error
		if err == nil {
			t.Fatalf("expected an error, got nil")
		} else {
			t.Logf("expected error received: %v", err)
		}
	})

	t.Run("ErrorCastingVirtualMachine", func(t *testing.T) {
		// Given a new injector
		injector := di.NewInjector()

		// And an invalid virtual machine registered
		injector.Register("virtualMachine", "invalidVirtualMachine")

		// And a new controller
		controller := NewController(injector)

		// When resolving the virtual machine
		_, err := controller.ResolveVirtualMachine()

		// Then there should be an error
		if err == nil {
			t.Fatalf("expected an error, got nil")
		} else {
			t.Logf("expected error received: %v", err)
		}
	})
}

func TestController_ResolveContainerRuntime(t *testing.T) {
	t.Run("ResolveContainerRuntime", func(t *testing.T) {
		// Given a new injector
		injector := di.NewInjector()

		// And a mock container runtime registered
		expectedContainerRuntime := &virt.MockVirt{}
		injector.Register("containerRuntime", expectedContainerRuntime)

		// And a new controller
		controller := NewController(injector)

		// When resolving the container runtime
		containerRuntime, err := controller.ResolveContainerRuntime()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the resolved container runtime should match the expected container runtime
		if containerRuntime != expectedContainerRuntime {
			t.Fatalf("expected %v, got %v", expectedContainerRuntime, containerRuntime)
		}
	})

	t.Run("ErrorResolvingContainerRuntime", func(t *testing.T) {
		// Given a new injector
		injector := di.NewInjector()

		// And a new controller
		controller := NewController(injector)

		// When resolving the container runtime
		_, err := controller.ResolveContainerRuntime()

		// Then there should be an error
		if err == nil {
			t.Fatalf("expected an error, got nil")
		} else {
			t.Logf("expected error received: %v", err)
		}
	})

	t.Run("ErrorCastingContainerRuntime", func(t *testing.T) {
		// Given a new injector
		injector := di.NewInjector()

		// And an invalid container runtime registered
		injector.Register("containerRuntime", "invalidContainerRuntime")

		// And a new controller
		controller := NewController(injector)

		// When resolving the container runtime
		_, err := controller.ResolveContainerRuntime()

		// Then there should be an error
		if err == nil {
			t.Fatalf("expected an error, got nil")
		} else {
			t.Logf("expected error received: %v", err)
		}
	})
}
