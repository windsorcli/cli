package controller

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/windsorcli/cli/internal/blueprint"
	"github.com/windsorcli/cli/internal/config"
	"github.com/windsorcli/cli/internal/context"
	"github.com/windsorcli/cli/internal/di"
	"github.com/windsorcli/cli/internal/env"
	"github.com/windsorcli/cli/internal/network"
	"github.com/windsorcli/cli/internal/services"
	"github.com/windsorcli/cli/internal/shell"
	"github.com/windsorcli/cli/internal/virt"
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
	BlueprintHandler *blueprint.MockBlueprintHandler
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
	mockBlueprintHandler := &blueprint.MockBlueprintHandler{}

	// Register mocks in the injector
	injector.Register("configHandler", mockConfigHandler)
	injector.Register("contextHandler", mockContextHandler)
	injector.Register("envPrinter1", mockEnvPrinter1)
	injector.Register("envPrinter2", mockEnvPrinter2)
	injector.Register("shell", mockShell)
	injector.Register("secureShell", mockSecureShell)
	injector.Register("networkManager", mockNetworkManager)
	injector.Register("blueprintHandler", mockBlueprintHandler)
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
		BlueprintHandler: mockBlueprintHandler,
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

	t.Run("ErrorInitializingShell", func(t *testing.T) {
		// Given a new controller with a mock injector
		mocks := setSafeControllerMocks()
		mockShell := shell.NewMockShell()
		mockShell.InitializeFunc = func() error {
			return fmt.Errorf("error initializing shell")
		}
		mocks.Injector.Register("shell", mockShell)
		controller := NewController(mocks.Injector)

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

	t.Run("ErrorInitializingSecureShell", func(t *testing.T) {
		// Given a new controller with a mock injector
		mocks := setSafeControllerMocks()
		mockSecureShell := shell.NewMockShell()
		mockSecureShell.InitializeFunc = func() error {
			return fmt.Errorf("error initializing secure shell")
		}
		mocks.Injector.Register("secureShell", mockSecureShell)
		controller := NewController(mocks.Injector)

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

func TestController_CreateBlueprintComponents(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new controller
		mocks := setSafeControllerMocks()
		controller := NewController(mocks.Injector)

		// When creating blueprint components
		err := controller.CreateBlueprintComponents()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestController_WriteConfigurationFiles(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new controller
		mocks := setSafeControllerMocks()
		controller := NewController(mocks.Injector)
		controller.Initialize()

		// When writing configuration files
		err := controller.WriteConfigurationFiles()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("ErrorWritingBlueprintConfig", func(t *testing.T) {
		// Given a new controller with a mock injector
		mocks := setSafeControllerMocks()
		mockBlueprintHandler := &blueprint.MockBlueprintHandler{}
		mockBlueprintHandler.WriteConfigFunc = func(path ...string) error {
			return fmt.Errorf("error writing blueprint config")
		}
		mocks.Injector.Register("blueprintHandler", mockBlueprintHandler)
		controller := NewController(mocks.Injector)
		controller.Initialize()

		// When writing configuration files
		err := controller.WriteConfigurationFiles()

		// Then there should be an error
		if err == nil {
			t.Fatalf("expected an error, got nil")
		} else if !strings.Contains(err.Error(), "error writing blueprint config") {
			t.Fatalf("expected error to contain 'error writing blueprint config', got %v", err)
		} else {
			t.Logf("expected error received: %v", err)
		}
	})

	t.Run("ErrorWritingConfigurationFiles", func(t *testing.T) {
		// Given a new controller with a mock injector
		mocks := setSafeControllerMocks()
		mockService := &services.MockService{}
		mockService.WriteConfigFunc = func() error {
			return fmt.Errorf("error writing service config")
		}
		mocks.Injector.Register("service1", mockService)
		controller := NewController(mocks.Injector)
		controller.Initialize()

		// When writing configuration files
		err := controller.WriteConfigurationFiles()

		// Then there should be an error
		if err == nil {
			t.Fatalf("expected an error, got nil")
		} else if !strings.Contains(err.Error(), "error writing service config") {
			t.Fatalf("expected error to contain 'error writing service config', got %v", err)
		} else {
			t.Logf("expected error received: %v", err)
		}
	})

	t.Run("ErrorWritingVirtualMachineConfig", func(t *testing.T) {
		// Given a new controller with a mock injector
		mocks := setSafeControllerMocks()
		mockVirtualMachine := virt.NewMockVirt()
		mockVirtualMachine.WriteConfigFunc = func() error {
			return fmt.Errorf("error writing virtual machine config")
		}
		mocks.Injector.Register("virtualMachine", mockVirtualMachine)
		controller := NewController(mocks.Injector)
		controller.Initialize()

		// When writing configuration files
		err := controller.WriteConfigurationFiles()

		// Then there should be an error
		if err == nil {
			t.Fatalf("expected an error, got nil")
		} else if !strings.Contains(err.Error(), "error writing virtual machine config") {
			t.Fatalf("expected error to contain 'error writing virtual machine config', got %v", err)
		} else {
			t.Logf("expected error received: %v", err)
		}
	})

	t.Run("ErrorWritingContainerRuntimeConfig", func(t *testing.T) {
		// Given a new controller with a mock injector
		mocks := setSafeControllerMocks()
		mockContainerRuntime := virt.NewMockVirt()
		mockContainerRuntime.WriteConfigFunc = func() error {
			return fmt.Errorf("error writing container runtime config")
		}
		mocks.Injector.Register("containerRuntime", mockContainerRuntime)
		controller := NewController(mocks.Injector)
		controller.Initialize()

		// When writing configuration files
		err := controller.WriteConfigurationFiles()

		// Then there should be an error
		if err == nil {
			t.Fatalf("expected an error, got nil")
		} else if !strings.Contains(err.Error(), "error writing container runtime config") {
			t.Fatalf("expected error to contain 'error writing container runtime config', got %v", err)
		} else {
			t.Logf("expected error received: %v", err)
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
		configHandler := controller.ResolveConfigHandler()

		// And the resolved config handler should match the expected config handler
		if configHandler != mocks.ConfigHandler {
			t.Fatalf("expected %v, got %v", mocks.ConfigHandler, configHandler)
		}
	})
}

func TestController_ResolveContextHandler(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new controller and injector
		mocks := setSafeControllerMocks()
		controller := NewController(mocks.Injector)

		// When resolving the context handler
		contextHandler := controller.ResolveContextHandler()

		// Then there should be no error
		if contextHandler == nil {
			t.Fatalf("expected no error, got nil")
		}

		// And the resolved context handler should match the expected context handler
		if contextHandler != mocks.ContextHandler {
			t.Fatalf("expected %v, got %v", mocks.ContextHandler, contextHandler)
		}
	})
}

func TestController_ResolveEnvPrinter(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new controller and injector
		mocks := setSafeControllerMocks()
		controller := NewController(mocks.Injector)

		// When resolving the env printer
		envPrinter := controller.ResolveEnvPrinter("envPrinter1")

		// Then there should be no error
		if envPrinter == nil {
			t.Fatalf("expected no error, got nil")
		}

		// And the resolved env printer should match the expected env printer
		if envPrinter != mocks.EnvPrinter {
			t.Fatalf("expected %v, got %v", mocks.EnvPrinter, envPrinter)
		}
	})
}

func TestController_ResolveAllEnvPrinters(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new controller and injector
		mocks := setSafeControllerMocks()
		controller := NewController(mocks.Injector)

		// When resolving all env printers
		envPrinters := controller.ResolveAllEnvPrinters()

		// Then there should be no error
		if envPrinters == nil {
			t.Fatalf("expected no error, got nil")
		}

		// And the number of resolved env printers should match the expected number
		if len(envPrinters) != 2 {
			t.Fatalf("expected %d env printers, got %d", 2, len(envPrinters))
		}

		// And each resolved env printer should match the expected env printer
		expectedPrinters := make(map[*env.MockEnvPrinter]bool)
		envPrinter1 := mocks.Injector.Resolve("envPrinter1")
		envPrinter2 := mocks.Injector.Resolve("envPrinter2")
		expectedPrinters[envPrinter1.(*env.MockEnvPrinter)] = true
		expectedPrinters[envPrinter2.(*env.MockEnvPrinter)] = true

		for _, printer := range envPrinters {
			if _, exists := expectedPrinters[printer.(*env.MockEnvPrinter)]; !exists {
				t.Fatalf("unexpected printer: got %v", printer)
			}
		}
	})
}

func TestController_ResolveShell(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new controller and injector
		mocks := setSafeControllerMocks()
		controller := NewController(mocks.Injector)

		// When resolving the shell
		shellInstance := controller.ResolveShell()

		// Then there should be no error
		if shellInstance == nil {
			t.Fatalf("expected no error, got nil")
		}

		// And the resolved shell should match the expected shell
		if shellInstance != mocks.Shell {
			t.Fatalf("expected %v, got %v", mocks.Shell, shellInstance)
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
		secureShell := controller.ResolveSecureShell()

		// Then there should be no error
		if secureShell == nil {
			t.Fatalf("expected no error, got nil")
		}

		// And the resolved secure shell should not be nil
		if secureShell == nil {
			t.Fatalf("expected a valid secure shell, got nil")
		}
	})
}

func TestController_ResolveBlueprintHandler(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new controller and injector
		mockInjector := di.NewMockInjector()
		mocks := setSafeControllerMocks(mockInjector)
		controller := NewController(mocks.Injector)

		// When resolving the blueprint handler
		blueprintHandler := controller.ResolveBlueprintHandler()

		// Then a blueprint handler should be returned
		if blueprintHandler == nil {
			t.Fatalf("expected a blueprint handler, got nil")
		}

		// And the resolved blueprint handler should match the expected blueprint handler
		if blueprintHandler != mocks.BlueprintHandler {
			t.Fatalf("expected %v, got %v", mocks.BlueprintHandler, blueprintHandler)
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
		networkManager := controller.ResolveNetworkManager()

		// Then there should be no error
		if networkManager == nil {
			t.Fatalf("expected no error, got nil")
		}

		// And the resolved network manager should match the expected network manager
		if networkManager != mocks.NetworkManager {
			t.Fatalf("expected %v, got %v", mocks.NetworkManager, networkManager)
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
		service := controller.ResolveService("service1")

		// Then there should be no error
		if service == nil {
			t.Fatalf("expected no error, got nil")
		}

		// And the resolved service should match the expected service
		if service != mocks.Service {
			t.Fatalf("expected %v, got %v", mocks.Service, service)
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
		resolvedServices := controller.ResolveAllServices()

		// Then there should be no error
		if resolvedServices == nil {
			t.Fatalf("expected no error, got nil")
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
}

func TestController_ResolveVirtualMachine(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new controller and injector
		mocks := setSafeControllerMocks()
		controller := NewController(mocks.Injector)

		// When resolving the virtual machine
		virtualMachine := controller.ResolveVirtualMachine()

		// Then there should be no error
		if virtualMachine == nil {
			t.Fatalf("expected no error, got nil")
		}

		// And the resolved virtual machine should match the expected virtual machine
		if virtualMachine != mocks.VirtualMachine {
			t.Fatalf("expected %v, got %v", mocks.VirtualMachine, virtualMachine)
		}
	})
}

func TestController_ResolveContainerRuntime(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new controller and injector
		mockInjector := di.NewMockInjector()
		mocks := setSafeControllerMocks(mockInjector)
		controller := NewController(mocks.Injector)
		// When resolving the container runtime
		containerRuntime := controller.ResolveContainerRuntime()

		// Then there should be no error
		if containerRuntime == nil {
			t.Fatalf("expected no error, got nil")
		}

		// And the resolved container runtime should match the expected container runtime
		if containerRuntime != mocks.ContainerRuntime {
			t.Fatalf("expected %v, got %v", mocks.ContainerRuntime, containerRuntime)
		}
	})
}

func TestController_getCLIConfigPath(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given the CLI config path is set
		os.Setenv("WINDSORCONFIG", "testdata/config.yaml")

		// When getting the CLI config path
		configPath, err := getCLIConfigPath()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the config path should match the expected path
		expectedPath := "testdata/config.yaml"
		if configPath != expectedPath {
			t.Fatalf("expected %v, got %v", expectedPath, configPath)
		}
	})

	t.Run("SuccessWithNoEnvVar", func(t *testing.T) {
		// Given the CLI config path is not set
		os.Unsetenv("WINDSORCONFIG")

		// When getting the CLI config path
		configPath, err := getCLIConfigPath()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the config path should match the default path
		home, _ := os.UserHomeDir()
		expectedPath := filepath.Join(home, ".config", "windsor", "config.yaml")
		if configPath != expectedPath {
			t.Fatalf("expected %v, got %v", expectedPath, configPath)
		}
	})

	t.Run("UserHomeDirError", func(t *testing.T) {
		// Given the CLI config path is not set
		os.Unsetenv("WINDSORCONFIG")

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
