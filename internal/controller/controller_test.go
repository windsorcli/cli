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

type MockObjects struct {
	Injector         di.Injector
	ConfigHandler    *config.MockConfigHandler
	ContextHandler   *context.MockContext
	EnvPrinter       *env.MockEnvPrinter
	Shell            *shell.MockShell
	SecureShell      *shell.MockShell
	NetworkManager   *network.MockNetworkManager
	Service          *services.MockService
	VirtualMachine   *virt.MockVirt
	ContainerRuntime *virt.MockVirt
}

func setSafeControllerMocks(customInjector ...di.Injector) *MockObjects {
	var injector di.Injector
	if len(customInjector) > 0 {
		injector = customInjector[0]
	} else {
		injector = di.NewMockInjector()
	}

	// Create necessary mocks
	mockConfigHandler := config.NewMockConfigHandler()
	mockContextHandler := context.NewMockContext()
	mockEnvPrinter1 := &env.MockEnvPrinter{}
	mockEnvPrinter2 := &env.MockEnvPrinter{}
	mockShell := &shell.MockShell{}
	mockSecureShell := &shell.MockShell{}
	mockNetworkManager := &network.MockNetworkManager{}
	mockService1 := &services.MockService{}
	mockService2 := &services.MockService{}
	mockVirtualMachine := &virt.MockVirt{}
	mockContainerRuntime := &virt.MockVirt{}

	// Register mocks in the injector
	injector.Register("configHandler", mockConfigHandler)
	injector.Register("contextHandler", mockContextHandler)
	injector.Register("envPrinter1", mockEnvPrinter1)
	injector.Register("envPrinter2", mockEnvPrinter2)
	injector.Register("shell", mockShell)
	injector.Register("secureShell", mockSecureShell)
	injector.Register("networkManager", mockNetworkManager)
	injector.Register("service1", mockService1)
	injector.Register("service2", mockService2)
	injector.Register("virtualMachine", mockVirtualMachine)
	injector.Register("containerRuntime", mockContainerRuntime)

	return &MockObjects{
		Injector:         injector,
		ConfigHandler:    mockConfigHandler,
		ContextHandler:   mockContextHandler,
		EnvPrinter:       mockEnvPrinter1, // Assuming the first envPrinter is the primary one
		Shell:            mockShell,
		SecureShell:      mockSecureShell,
		NetworkManager:   mockNetworkManager,
		Service:          mockService1, // Assuming the first service is the primary one
		VirtualMachine:   mockVirtualMachine,
		ContainerRuntime: mockContainerRuntime,
	}
}

func TestNewController(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setSafeControllerMocks()

		// Given a new controller
		controller := NewController(mocks.Injector)

		// Then the controller should not be nil
		if controller == nil {
			t.Fatalf("expected controller, got nil")
		} else {
			t.Logf("Success: controller created")
		}
	})
}

func TestController_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new controller
		mocks := setSafeControllerMocks()
		controller := NewController(mocks.Injector)

		// When initializing the controller
		err := controller.Initialize()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("ErrorResolvingConfigHandler", func(t *testing.T) {
		// Given a new controller with a mock injector
		mockInjector := di.NewMockInjector()
		mocks := setSafeControllerMocks(mockInjector)
		mockInjector.SetResolveError("configHandler", fmt.Errorf("resolve error"))
		controller := NewController(mocks.Injector)

		// When initializing the controller
		err := controller.Initialize()

		// Then there should be an error
		if err == nil {
			t.Fatalf("expected an error, got nil")
		} else {
			t.Logf("expected error received: %v", err)
		}
	})

	t.Run("ErrorGettingCLIConfigPath", func(t *testing.T) {
		// Given a new controller with a mock injector
		mocks := setSafeControllerMocks()
		controller := NewController(mocks.Injector)

		// Override the getCLIConfigPath function to simulate an error
		originalGetCLIConfigPath := getCLIConfigPath
		getCLIConfigPath = func() (string, error) {
			return "", fmt.Errorf("error getting CLI config path")
		}
		defer func() {
			// Restore the original function after the test
			getCLIConfigPath = originalGetCLIConfigPath
		}()

		// When initializing the controller
		err := controller.Initialize()

		// Then there should be an error
		if err == nil {
			t.Fatalf("expected an error, got nil")
		} else if !strings.Contains(err.Error(), "error getting CLI config path") {
			t.Fatalf("expected error to contain 'error getting CLI config path', got %v", err)
		} else {
			t.Logf("expected error received: %v", err)
		}
	})

	t.Run("ErrorLoadingCLIConfig", func(t *testing.T) {
		// Given a new controller with a mock config handler
		mocks := setSafeControllerMocks()
		mocks.ConfigHandler.LoadConfigFunc = func(path string) error {
			return fmt.Errorf("error loading CLI config")
		}
		controller := NewController(mocks.Injector)

		// When initializing the controller
		err := controller.Initialize()

		// Then there should be an error
		if err == nil {
			t.Fatalf("expected an error, got nil")
		} else if !strings.Contains(err.Error(), "error loading CLI config") {
			t.Fatalf("expected error to contain 'error loading CLI config', got %v", err)
		} else {
			t.Logf("expected error received: %v", err)
		}
	})
}

func TestController_InitializeComponents(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new controller
		mocks := setSafeControllerMocks()
		controller := NewController(mocks.Injector)

		// When initializing the components
		err := controller.InitializeComponents()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("ErrorResolvingContextHandler", func(t *testing.T) {
		// Given a new controller with a mock injector
		mocks := setSafeControllerMocks()
		mocks.Injector.Register("contextHandler", "invalid")
		controller := NewController(mocks.Injector)

		// When initializing the components
		err := controller.InitializeComponents()

		// Then there should be an error
		if err == nil {
			t.Fatalf("expected an error, got nil")
		} else if !strings.Contains(err.Error(), "error initializing context handler") {
			t.Fatalf("expected error to contain 'error initializing context handler', got %v", err)
		}
	})

	t.Run("ErrorInitializingContextHandler", func(t *testing.T) {
		// Given a new controller with a mock injector
		mocks := setSafeControllerMocks()
		mockContextHandler := context.NewMockContext()
		mockContextHandler.InitializeFunc = func() error {
			return fmt.Errorf("error initializing context handler")
		}
		mocks.Injector.Register("contextHandler", mockContextHandler)
		controller := NewController(mocks.Injector)

		// When initializing the components
		err := controller.InitializeComponents()

		// Then there should be an error
		if err == nil {
			t.Fatalf("expected an error, got nil")
		} else if !strings.Contains(err.Error(), "error initializing context handler") {
			t.Fatalf("expected error to contain 'error initializing context handler', got %v", err)
		} else {
			t.Logf("expected error received: %v", err)
		}
	})

	t.Run("ErrorResolvingEnvPrinters", func(t *testing.T) {
		mockInjector := di.NewMockInjector()
		// Given a new controller with a mock injector
		mocks := setSafeControllerMocks(mockInjector)
		mockInjector.SetResolveAllError((*env.EnvPrinter)(nil), fmt.Errorf("error resolving env printers"))
		controller := NewController(mocks.Injector)

		// When initializing the components
		err := controller.InitializeComponents()

		// Then there should be an error
		if err == nil {
			t.Fatalf("expected an error, got nil")
		} else if !strings.Contains(err.Error(), "error resolving env printers") {
			t.Fatalf("expected error to contain 'error resolving env printers', got %v", err)
		} else {
			t.Logf("expected error received: %v", err)
		}
	})

	t.Run("ErrorInitializingEnvPrinters", func(t *testing.T) {
		// Given a new controller with a mock injector
		mocks := setSafeControllerMocks()
		mockEnvPrinter := env.NewMockEnvPrinter()
		mockEnvPrinter.InitializeFunc = func() error {
			return fmt.Errorf("error initializing env printer")
		}
		mocks.Injector.Register("envPrinter1", mockEnvPrinter)
		controller := NewController(mocks.Injector)

		// When initializing the components
		err := controller.InitializeComponents()

		// Then there should be an error
		if err == nil {
			t.Fatalf("expected an error, got nil")
		} else if !strings.Contains(err.Error(), "error initializing env printer") {
			t.Fatalf("expected error to contain 'error initializing env printer', got %v", err)
		} else {
			t.Logf("expected error received: %v", err)
		}
	})

	t.Run("ErrorResolvingShell", func(t *testing.T) {
		// Given a new controller with a mock injector
		mocks := setSafeControllerMocks()
		controller := NewController(mocks.Injector)
		mocks.Injector.Register("shell", "invalid")

		// When initializing the components
		err := controller.InitializeComponents()

		// Then there should be an error
		if err == nil {
			t.Fatalf("expected an error, got nil")
		} else if !strings.Contains(err.Error(), "error initializing shell") {
			t.Fatalf("expected error to contain 'error initializing shell', got %v", err)
		} else {
			t.Logf("expected error received: %v", err)
		}
	})

	t.Run("ErrorInitializingShell", func(t *testing.T) {
		// Given a new controller with a mock injector
		mocks := setSafeControllerMocks()
		controller := NewController(mocks.Injector)
		mockShell := shell.NewMockShell()
		mockShell.InitializeFunc = func() error {
			return fmt.Errorf("error initializing shell")
		}
		mocks.Injector.Register("shell", mockShell)

		// When initializing the components
		err := controller.InitializeComponents()

		// Then there should be an error
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}
	})

	t.Run("ErrorResolvingSecureShell", func(t *testing.T) {
		// Given a new controller with a mock injector
		mocks := setSafeControllerMocks()
		controller := NewController(mocks.Injector)
		mocks.Injector.Register("secureShell", "invalid")

		// When initializing the components
		err := controller.InitializeComponents()

		// Then there should be an error
		if err == nil {
			t.Fatalf("expected an error, got nil")
		} else if !strings.Contains(err.Error(), "error initializing secure shell") {
			t.Fatalf("expected error to contain 'error initializing secure shell', got %v", err)
		} else {
			t.Logf("expected error received: %v", err)
		}
	})

	t.Run("ErrorInitializingSecureShell", func(t *testing.T) {
		// Given a new controller with a mock injector
		mocks := setSafeControllerMocks()
		controller := NewController(mocks.Injector)
		mockSecureShell := shell.NewMockShell()
		mockSecureShell.InitializeFunc = func() error {
			return fmt.Errorf("error initializing secure shell")
		}
		mocks.Injector.Register("secureShell", mockSecureShell)

		// When initializing the components
		err := controller.InitializeComponents()

		// Then there should be an error
		if err == nil {
			t.Fatalf("expected an error, got nil")
		} else if !strings.Contains(err.Error(), "error initializing secure shell") {
			t.Fatalf("expected error to contain 'error initializing secure shell', got %v", err)
		} else {
			t.Logf("expected error received: %v", err)
		}
	})

	t.Run("ErrorResolvingNetworkManager", func(t *testing.T) {
		// Given a new controller with a mock injector
		mocks := setSafeControllerMocks()
		controller := NewController(mocks.Injector)
		mocks.Injector.Register("networkManager", "invalid")

		// When initializing the components
		err := controller.InitializeComponents()

		// Then there should be an error
		if err == nil {
			t.Fatalf("expected an error, got nil")
		} else if !strings.Contains(err.Error(), "error initializing network manager") {
			t.Fatalf("expected error to contain 'error initializing network manager', got %v", err)
		} else {
			t.Logf("expected error received: %v", err)
		}
	})

	t.Run("ErrorInitializingNetworkManager", func(t *testing.T) {
		// Given a new controller with a mock injector
		mocks := setSafeControllerMocks()
		controller := NewController(mocks.Injector)
		mockNetworkManager := network.NewMockNetworkManager()
		mockNetworkManager.InitializeFunc = func() error {
			return fmt.Errorf("error initializing network manager")
		}
		mocks.Injector.Register("networkManager", mockNetworkManager)

		// When initializing the components
		err := controller.InitializeComponents()

		// Then there should be an error
		if err == nil {
			t.Fatalf("expected an error, got nil")
		} else if !strings.Contains(err.Error(), "error initializing network manager") {
			t.Fatalf("expected error to contain 'error initializing network manager', got %v", err)
		} else {
			t.Logf("expected error received: %v", err)
		}
	})

	t.Run("ErrorResolvingServices", func(t *testing.T) {
		mockInjector := di.NewMockInjector()

		// Given a new controller with a mock injector
		mocks := setSafeControllerMocks(mockInjector)
		mockInjector.SetResolveAllError(new(services.Service), fmt.Errorf("error resolving services"))
		controller := NewController(mocks.Injector)

		// When initializing the components
		err := controller.InitializeComponents()

		// Then there should be an error
		if err == nil {
			t.Fatalf("expected an error, got nil")
		} else if !strings.Contains(err.Error(), "error initializing service") {
			t.Fatalf("expected error to contain 'error initializing service', got %v", err)
		} else {
			t.Logf("expected error received: %v", err)
		}
	})

	t.Run("ErrorInitializingServices", func(t *testing.T) {
		// Given a new controller with a mock injector
		mocks := setSafeControllerMocks()
		controller := NewController(mocks.Injector)
		mockService := services.NewMockService()
		mockService.InitializeFunc = func() error {
			return fmt.Errorf("error initializing service")
		}
		mocks.Injector.Register("service1", mockService)

		// When initializing the components
		err := controller.InitializeComponents()

		// Then there should be an error
		if err == nil {
			t.Fatalf("expected an error, got nil")
		} else if !strings.Contains(err.Error(), "error initializing service") {
			t.Fatalf("expected error to contain 'error initializing service', got %v", err)
		} else {
			t.Logf("expected error received: %v", err)
		}
	})

	t.Run("ErrorResolvingVirtualMachine", func(t *testing.T) {
		// Given a new controller with a mock injector
		mocks := setSafeControllerMocks()
		controller := NewController(mocks.Injector)
		mocks.Injector.Register("virtualMachine", "invalid")

		// When initializing the components
		err := controller.InitializeComponents()

		// Then there should be an error
		if err == nil {
			t.Fatalf("expected an error, got nil")
		} else if !strings.Contains(err.Error(), "error initializing virtual machine") {
			t.Fatalf("expected error to contain 'error initializing virtual machine', got %v", err)
		} else {
			t.Logf("expected error received: %v", err)
		}
	})

	t.Run("ErrorInitializingVirtualMachine", func(t *testing.T) {
		// Given a new controller with a mock injector
		mocks := setSafeControllerMocks()
		controller := NewController(mocks.Injector)
		mockVirtualMachine := &virt.MockVirt{}
		mockVirtualMachine.InitializeFunc = func() error {
			return fmt.Errorf("error initializing virtual machine")
		}
		mocks.Injector.Register("virtualMachine", mockVirtualMachine)

		// When initializing the components
		err := controller.InitializeComponents()

		// Then there should be an error
		if err == nil {
			t.Fatalf("expected an error, got nil")
		} else if !strings.Contains(err.Error(), "error initializing virtual machine") {
			t.Fatalf("expected error to contain 'error initializing virtual machine', got %v", err)
		} else {
			t.Logf("expected error received: %v", err)
		}
	})

	t.Run("ErrorResolvingContainerRuntime", func(t *testing.T) {
		// Given a new controller with a mock injector
		mocks := setSafeControllerMocks()
		controller := NewController(mocks.Injector)
		mocks.Injector.Register("containerRuntime", "invalid")

		// When initializing the components
		err := controller.InitializeComponents()

		// Then there should be an error
		if err == nil {
			t.Fatalf("expected an error, got nil")
		} else if !strings.Contains(err.Error(), "error initializing container runtime") {
			t.Fatalf("expected error to contain 'error initializing container runtime', got %v", err)
		} else {
			t.Logf("expected error received: %v", err)
		}
	})

	t.Run("ErrorInitializingContainerRuntime", func(t *testing.T) {
		// Given a new controller with a mock injector
		mocks := setSafeControllerMocks()
		controller := NewController(mocks.Injector)
		mockContainerRuntime := &virt.MockVirt{}
		mockContainerRuntime.InitializeFunc = func() error {
			return fmt.Errorf("error initializing container runtime")
		}
		mocks.Injector.Register("containerRuntime", mockContainerRuntime)

		// When initializing the components
		err := controller.InitializeComponents()

		// Then there should be an error
		if err == nil {
			t.Fatalf("expected an error, got nil")
		} else if !strings.Contains(err.Error(), "error initializing container runtime") {
			t.Fatalf("expected error to contain 'error initializing container runtime', got %v", err)
		} else {
			t.Logf("expected error received: %v", err)
		}
	})
}

func TestController_CreateCommonComponents(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new controller
		mocks := setSafeControllerMocks()
		controller := NewController(mocks.Injector)

		// When creating common components
		err := controller.CreateCommonComponents()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestController_CreateEnvComponents(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new controller
		mocks := setSafeControllerMocks()
		controller := NewController(mocks.Injector)

		// When creating env components
		err := controller.CreateEnvComponents()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestController_CreateServiceComponents(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new controller
		mocks := setSafeControllerMocks()
		controller := NewController(mocks.Injector)

		// When creating service components
		err := controller.CreateServiceComponents()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestController_CreateVirtualizationComponents(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new controller
		mocks := setSafeControllerMocks()
		controller := NewController(mocks.Injector)

		// When creating virtualization components
		err := controller.CreateVirtualizationComponents()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestController_ResolveInjector(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new controller and injector
		mocks := setSafeControllerMocks()
		controller := NewController(mocks.Injector)

		// When resolving the injector
		resolvedInjector := controller.ResolveInjector()

		// Then the resolved injector should match the original injector
		if resolvedInjector != mocks.Injector {
			t.Fatalf("expected %v, got %v", mocks.Injector, resolvedInjector)
		}
	})
}

func TestController_ResolveConfigHandler(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new controller and injector
		mocks := setSafeControllerMocks()
		controller := NewController(mocks.Injector)

		// When resolving the config handler
		configHandler, err := controller.ResolveConfigHandler()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the resolved config handler should match the expected config handler
		if configHandler != mocks.ConfigHandler {
			t.Fatalf("expected %v, got %v", mocks.ConfigHandler, configHandler)
		}
	})

	t.Run("ErrorResolvingConfigHandler", func(t *testing.T) {
		mockInjector := di.NewMockInjector()

		// Given a new controller with a mock injector
		mocks := setSafeControllerMocks(mockInjector)
		controller := NewController(mocks.Injector)
		mockInjector.SetResolveError("configHandler", fmt.Errorf("resolve error"))

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
		// Given a new controller and injector
		mocks := setSafeControllerMocks()
		controller := NewController(mocks.Injector)

		// Register an invalid type for the config handler
		mocks.Injector.Register("configHandler", "invalid")

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
	t.Run("Success", func(t *testing.T) {
		// Given a new controller and injector
		mocks := setSafeControllerMocks()
		controller := NewController(mocks.Injector)

		// When resolving the context handler
		contextHandler, err := controller.ResolveContextHandler()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the resolved context handler should match the expected context handler
		if contextHandler != mocks.ContextHandler {
			t.Fatalf("expected %v, got %v", mocks.ContextHandler, contextHandler)
		}
	})

	t.Run("ErrorResolvingContextHandler", func(t *testing.T) {
		// Given a new controller with a mock injector
		mockInjector := di.NewMockInjector()
		mocks := setSafeControllerMocks(mockInjector)
		mockInjector.SetResolveError("contextHandler", fmt.Errorf("resolve error"))
		controller := NewController(mocks.Injector)

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
		// Given a new controller and injector
		mocks := setSafeControllerMocks()
		mocks.Injector.Register("contextHandler", "invalidContextHandler")
		controller := NewController(mocks.Injector)

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
	t.Run("Success", func(t *testing.T) {
		// Given a new controller and injector
		mocks := setSafeControllerMocks()
		controller := NewController(mocks.Injector)

		// When resolving the env printer
		envPrinter, err := controller.ResolveEnvPrinter("envPrinter1")

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the resolved env printer should match the expected env printer
		if envPrinter != mocks.EnvPrinter {
			t.Fatalf("expected %v, got %v", mocks.EnvPrinter, envPrinter)
		}
	})

	t.Run("ErrorResolvingEnvPrinter", func(t *testing.T) {
		// Given a new controller with a mock injector
		mockInjector := di.NewMockInjector()
		mocks := setSafeControllerMocks(mockInjector)
		mockInjector.SetResolveError("envPrinter", fmt.Errorf("resolve error"))
		controller := NewController(mocks.Injector)

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
		// Given a new controller and injector
		mocks := setSafeControllerMocks()
		mocks.Injector.Register("envPrinter", "invalidEnvPrinter")
		controller := NewController(mocks.Injector)

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
		// Given a new controller and injector
		mocks := setSafeControllerMocks()
		controller := NewController(mocks.Injector)

		// When resolving all env printers
		envPrinters, err := controller.ResolveAllEnvPrinters()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the number of resolved env printers should match the expected number
		if len(envPrinters) != 2 {
			t.Fatalf("expected %d env printers, got %d", 2, len(envPrinters))
		}

		// And each resolved env printer should match the expected env printer
		expectedPrinters := make(map[*env.MockEnvPrinter]bool)
		envPrinter1, err := mocks.Injector.Resolve("envPrinter1")
		if err != nil {
			t.Fatalf("failed to resolve envPrinter1: %v", err)
		}
		envPrinter2, err := mocks.Injector.Resolve("envPrinter2")
		if err != nil {
			t.Fatalf("failed to resolve envPrinter2: %v", err)
		}
		expectedPrinters[envPrinter1.(*env.MockEnvPrinter)] = true
		expectedPrinters[envPrinter2.(*env.MockEnvPrinter)] = true

		for _, printer := range envPrinters {
			if _, exists := expectedPrinters[printer.(*env.MockEnvPrinter)]; !exists {
				t.Fatalf("unexpected printer: got %v", printer)
			}
		}
	})

	t.Run("ErrorResolvingEnvPrinters", func(t *testing.T) {
		// Given a new controller with a mock injector
		mockInjector := di.NewMockInjector()
		mocks := setSafeControllerMocks(mockInjector)
		mockInjector.SetResolveAllError((*env.EnvPrinter)(nil), fmt.Errorf("resolve error"))
		controller := NewController(mocks.Injector)

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
	t.Run("Success", func(t *testing.T) {
		// Given a new controller and injector
		mocks := setSafeControllerMocks()
		controller := NewController(mocks.Injector)

		// When resolving the shell
		shellInstance, err := controller.ResolveShell()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the resolved shell should match the expected shell
		if shellInstance != mocks.Shell {
			t.Fatalf("expected %v, got %v", mocks.Shell, shellInstance)
		}
	})

	t.Run("ErrorResolvingShell", func(t *testing.T) {
		// Given a new controller with a mock injector
		mockInjector := di.NewMockInjector()
		mocks := setSafeControllerMocks(mockInjector)
		mockInjector.SetResolveError("shell", fmt.Errorf("resolve error"))
		controller := NewController(mocks.Injector)

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
		// Given a new controller and injector
		mocks := setSafeControllerMocks()
		mocks.Injector.Register("shell", "invalidShell")
		controller := NewController(mocks.Injector)

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
	t.Run("Success", func(t *testing.T) {
		// Given a new controller and injector
		mockInjector := di.NewMockInjector()
		mocks := setSafeControllerMocks(mockInjector)
		controller := NewController(mocks.Injector)

		// When resolving the secure shell
		secureShell, err := controller.ResolveSecureShell()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the resolved secure shell should not be nil
		if secureShell == nil {
			t.Fatalf("expected a valid secure shell, got nil")
		}
	})

	t.Run("ErrorResolvingSecureShell", func(t *testing.T) {
		// Given a new controller with a mock injector
		mockInjector := di.NewMockInjector()
		mocks := setSafeControllerMocks(mockInjector)
		mockInjector.SetResolveError("secureShell", fmt.Errorf("resolve error"))
		controller := NewController(mocks.Injector)

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
		// Given a new controller and injector
		mocks := setSafeControllerMocks()
		mocks.Injector.Register("secureShell", "invalidSecureShell")
		controller := NewController(mocks.Injector)

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
	t.Run("Success", func(t *testing.T) {
		// Given a new controller and injector
		mockInjector := di.NewMockInjector()
		mocks := setSafeControllerMocks(mockInjector)
		controller := NewController(mocks.Injector)

		// When resolving the network manager
		networkManager, err := controller.ResolveNetworkManager()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the resolved network manager should match the expected network manager
		if networkManager != mocks.NetworkManager {
			t.Fatalf("expected %v, got %v", mocks.NetworkManager, networkManager)
		}
	})

	t.Run("ErrorResolvingNetworkManager", func(t *testing.T) {
		// Given a new controller with a mock injector
		mockInjector := di.NewMockInjector()
		mocks := setSafeControllerMocks(mockInjector)
		mockInjector.SetResolveError("networkManager", fmt.Errorf("resolve error"))
		controller := NewController(mocks.Injector)

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
		// Given a new controller and injector
		mocks := setSafeControllerMocks()
		mocks.Injector.Register("networkManager", "invalidNetworkManager")
		controller := NewController(mocks.Injector)

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
		// Given a new controller and injector
		mockInjector := di.NewMockInjector()
		mocks := setSafeControllerMocks(mockInjector)
		controller := NewController(mocks.Injector)

		// When resolving the service
		service, err := controller.ResolveService("service1")

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the resolved service should match the expected service
		if service != mocks.Service {
			t.Fatalf("expected %v, got %v", mocks.Service, service)
		}
	})

	t.Run("ErrorResolvingService", func(t *testing.T) {
		// Given a new controller with a mock injector
		mockInjector := di.NewMockInjector()
		mocks := setSafeControllerMocks(mockInjector)
		mockInjector.SetResolveError("service1", fmt.Errorf("resolve error"))
		controller := NewController(mocks.Injector)

		// When resolving the service
		_, err := controller.ResolveService("service1")

		// Then there should be an error
		if err == nil {
			t.Fatalf("expected an error, got nil")
		} else {
			t.Logf("expected error received: %v", err)
		}
	})

	t.Run("ErrorCastingService", func(t *testing.T) {
		// Given a new controller and injector
		mocks := setSafeControllerMocks()
		mocks.Injector.Register("service1", "invalidService")
		controller := NewController(mocks.Injector)

		// When resolving the service
		_, err := controller.ResolveService("service1")

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
		// Given a new controller and injector
		mockInjector := di.NewMockInjector()
		mocks := setSafeControllerMocks(mockInjector)
		controller := NewController(mocks.Injector)

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
			mocks.Service: false,
			mocks.Service: false,
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
		mockInjector := di.NewMockInjector()

		// Given a new controller with a mock injector
		mocks := setSafeControllerMocks(mockInjector)
		mockInjector.SetResolveAllError((*services.Service)(nil), fmt.Errorf("resolve all error"))
		controller := NewController(mocks.Injector)

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
}

func TestController_ResolveVirtualMachine(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new controller and injector
		mocks := setSafeControllerMocks()
		controller := NewController(mocks.Injector)

		// When resolving the virtual machine
		virtualMachine, err := controller.ResolveVirtualMachine()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the resolved virtual machine should match the expected virtual machine
		if virtualMachine != mocks.VirtualMachine {
			t.Fatalf("expected %v, got %v", mocks.VirtualMachine, virtualMachine)
		}
	})

	t.Run("ErrorResolvingVirtualMachine", func(t *testing.T) {
		// Given a new controller with a mock injector
		mockInjector := di.NewMockInjector()
		mocks := setSafeControllerMocks(mockInjector)
		mockInjector.SetResolveError("virtualMachine", fmt.Errorf("resolve error"))
		controller := NewController(mocks.Injector)

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
		// Given a new controller and injector
		mocks := setSafeControllerMocks()
		mocks.Injector.Register("virtualMachine", "invalidVirtualMachine")
		controller := NewController(mocks.Injector)

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
		// Given a new controller and injector
		mockInjector := di.NewMockInjector()
		mocks := setSafeControllerMocks(mockInjector)
		controller := NewController(mocks.Injector)
		// When resolving the container runtime
		containerRuntime, err := controller.ResolveContainerRuntime()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the resolved container runtime should match the expected container runtime
		if containerRuntime != mocks.ContainerRuntime {
			t.Fatalf("expected %v, got %v", mocks.ContainerRuntime, containerRuntime)
		}
	})

	t.Run("ErrorResolvingContainerRuntime", func(t *testing.T) {
		// Given a new controller with a mock injector
		mockInjector := di.NewMockInjector()
		mocks := setSafeControllerMocks(mockInjector)
		mockInjector.SetResolveError("containerRuntime", fmt.Errorf("resolve error"))
		controller := NewController(mocks.Injector)

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
		// Given a new controller and injector
		mocks := setSafeControllerMocks()
		mocks.Injector.Register("containerRuntime", "invalidContainerRuntime")
		controller := NewController(mocks.Injector)

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

func TestController_getCLIConfigPath(t *testing.T) {
	t.Run("UserHomeDirError", func(t *testing.T) {
		// Given osUserHomeDir is mocked to return an error
		originalUserHomeDir := osUserHomeDir
		osUserHomeDir = func() (string, error) {
			return "", fmt.Errorf("mock error")
		}
		defer func() { osUserHomeDir = originalUserHomeDir }()

		// Execute the function
		_, err := getCLIConfigPath()

		// Verify the error
		expectedError := "error retrieving user home directory: mock error"
		if err == nil || err.Error() != expectedError {
			t.Errorf("expected error %q, got %v", expectedError, err)
		}
	})
}
