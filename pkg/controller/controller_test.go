package controller

import (
	"fmt"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/blueprint"
	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/env"
	"github.com/windsorcli/cli/pkg/generators"
	"github.com/windsorcli/cli/pkg/network"
	"github.com/windsorcli/cli/pkg/services"
	"github.com/windsorcli/cli/pkg/shell"
	"github.com/windsorcli/cli/pkg/stack"
	"github.com/windsorcli/cli/pkg/tools"
	"github.com/windsorcli/cli/pkg/virt"
)

type MockResources struct {
	Shell            shell.Shell
	SecureShell      shell.Shell
	BlueprintHandler blueprint.BlueprintHandler
	NetworkManager   network.NetworkManager
	Service1         services.Service
	Service2         services.Service
	ToolsManager     tools.ToolsManager
	VirtualMachine   virt.VirtualMachine
	ContainerRuntime virt.ContainerRuntime
	Stack            stack.Stack
	ConfigHandler    config.ConfigHandler
	EnvPrinter       env.EnvPrinter
	Generator        generators.Generator
}

func registerMockResources(injector di.Injector) MockResources {
	mockResources := MockResources{
		Shell:            shell.NewMockShell(),
		SecureShell:      shell.NewMockShell(),
		BlueprintHandler: blueprint.NewMockBlueprintHandler(injector),
		NetworkManager:   network.NewMockNetworkManager(),
		Service1:         services.NewMockService(),
		Service2:         services.NewMockService(),
		ToolsManager:     tools.NewMockToolsManager(),
		VirtualMachine:   virt.NewMockVirt(),
		ContainerRuntime: virt.NewMockVirt(),
		Stack:            stack.NewMockStack(injector),
		ConfigHandler:    config.NewMockConfigHandler(),
		EnvPrinter:       env.NewMockEnvPrinter(),
		Generator:        generators.NewMockGenerator(),
	}

	injector.Register("shell", mockResources.Shell)
	injector.Register("secureShell", mockResources.SecureShell)
	injector.Register("blueprintHandler", mockResources.BlueprintHandler)
	injector.Register("networkManager", mockResources.NetworkManager)
	injector.Register("service1", mockResources.Service1)
	injector.Register("service2", mockResources.Service2)
	injector.Register("toolsManager", mockResources.ToolsManager)
	injector.Register("virtualMachine", mockResources.VirtualMachine)
	injector.Register("containerRuntime", mockResources.ContainerRuntime)
	injector.Register("stack", mockResources.Stack)
	injector.Register("configHandler", mockResources.ConfigHandler)
	injector.Register("envPrinter", mockResources.EnvPrinter)
	injector.Register("generator", mockResources.Generator)

	return mockResources
}

func TestNewController(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new injector
		injector := di.NewInjector()

		// Given a new controller
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
	t.Run("Success", func(t *testing.T) {
		// Given a new controller
		injector := di.NewInjector()
		controller := NewController(injector)

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
		injector := di.NewInjector()
		controller := NewController(injector)
		controller.Initialize()

		// When initializing the components
		err := controller.InitializeComponents()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("ErrorInitializingShell", func(t *testing.T) {
		// Given a new controller with a mock injector
		injector := di.NewInjector()
		mockShell := shell.NewMockShell()
		mockShell.InitializeFunc = func() error {
			return fmt.Errorf("error initializing shell")
		}
		injector.Register("shell", mockShell)
		controller := NewController(injector)
		controller.Initialize()

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
		injector := di.NewInjector()
		mockSecureShell := shell.NewMockShell()
		mockSecureShell.InitializeFunc = func() error {
			return fmt.Errorf("error initializing secure shell")
		}
		injector.Register("secureShell", mockSecureShell)
		controller := NewController(injector)
		controller.Initialize()

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
		injector := di.NewInjector()
		mockEnvPrinter := env.NewMockEnvPrinter()
		mockEnvPrinter.InitializeFunc = func() error {
			return fmt.Errorf("error initializing env printer")
		}
		injector.Register("envPrinter1", mockEnvPrinter)
		controller := NewController(injector)
		controller.Initialize()

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

	t.Run("ErrorInitializingToolsManager", func(t *testing.T) {
		// Given a new controller with a mock injector
		injector := di.NewInjector()
		mockToolsManager := tools.NewMockToolsManager()
		mockToolsManager.InitializeFunc = func() error {
			return fmt.Errorf("error initializing tools manager")
		}
		injector.Register("toolsManager", mockToolsManager)
		controller := NewController(injector)
		controller.Initialize()

		// When initializing the components
		err := controller.InitializeComponents()

		// Then there should be an error
		if err == nil {
			t.Fatalf("expected an error, got nil")
		} else if !strings.Contains(err.Error(), "error initializing tools manager") {
			t.Fatalf("expected error to contain 'error initializing tools manager', got %v", err)
		} else {
			t.Logf("expected error received: %v", err)
		}
	})

	t.Run("ErrorInitializingNetworkManager", func(t *testing.T) {
		// Given a new controller with a mock injector
		injector := di.NewInjector()
		controller := NewController(injector)
		controller.Initialize()
		mockNetworkManager := network.NewMockNetworkManager()
		mockNetworkManager.InitializeFunc = func() error {
			return fmt.Errorf("error initializing network manager")
		}
		injector.Register("networkManager", mockNetworkManager)

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
		injector := di.NewInjector()
		controller := NewController(injector)
		controller.Initialize()
		mockService := services.NewMockService()
		mockService.InitializeFunc = func() error {
			return fmt.Errorf("error initializing service")
		}
		injector.Register("service1", mockService)

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
		injector := di.NewInjector()
		controller := NewController(injector)
		controller.Initialize()
		mockVirtualMachine := &virt.MockVirt{}
		mockVirtualMachine.InitializeFunc = func() error {
			return fmt.Errorf("error initializing virtual machine")
		}
		injector.Register("virtualMachine", mockVirtualMachine)

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
		injector := di.NewInjector()
		controller := NewController(injector)
		controller.Initialize()
		mockContainerRuntime := &virt.MockVirt{}
		mockContainerRuntime.InitializeFunc = func() error {
			return fmt.Errorf("error initializing container runtime")
		}
		injector.Register("containerRuntime", mockContainerRuntime)

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

	t.Run("ErrorInitializingBlueprintHandler", func(t *testing.T) {
		// Given a new controller with a mock injector
		injector := di.NewInjector()
		controller := NewController(injector)
		controller.Initialize()
		mockBlueprintHandler := &blueprint.MockBlueprintHandler{}
		mockBlueprintHandler.InitializeFunc = func() error {
			return fmt.Errorf("error initializing blueprint handler")
		}
		injector.Register("blueprintHandler", mockBlueprintHandler)

		// When initializing the components
		err := controller.InitializeComponents()

		// Then there should be an error
		if err == nil {
			t.Fatalf("expected an error, got nil")
		} else if !strings.Contains(err.Error(), "error initializing blueprint handler") {
			t.Fatalf("expected error to contain 'error initializing blueprint handler', got %v", err)
		} else {
			t.Logf("expected error received: %v", err)
		}
	})

	t.Run("ErrorLoadingBlueprintConfig", func(t *testing.T) {
		// Given a new controller with a mock injector
		injector := di.NewInjector()
		controller := NewController(injector)
		controller.Initialize()
		mockBlueprintHandler := &blueprint.MockBlueprintHandler{}
		mockBlueprintHandler.LoadConfigFunc = func(path ...string) error {
			return fmt.Errorf("error loading blueprint config")
		}
		injector.Register("blueprintHandler", mockBlueprintHandler)

		// When initializing the components
		err := controller.InitializeComponents()

		// Then there should be an error
		if err == nil {
			t.Fatalf("expected an error, got nil")
		} else if !strings.Contains(err.Error(), "error loading blueprint config") {
			t.Fatalf("expected error to contain 'error loading blueprint config', got %v", err)
		} else {
			t.Logf("expected error received: %v", err)
		}
	})

	t.Run("ErrorInitializingGenerators", func(t *testing.T) {
		// Given a new controller with a mock injector
		injector := di.NewInjector()
		controller := NewController(injector)
		controller.Initialize()
		mockGenerator := generators.NewMockGenerator()
		mockGenerator.InitializeFunc = func() error {
			return fmt.Errorf("error initializing generator")
		}
		injector.Register("generator", mockGenerator)

		// When initializing the components
		err := controller.InitializeComponents()

		// Then there should be an error
		if err == nil {
			t.Fatalf("expected an error, got nil")
		} else if !strings.Contains(err.Error(), "error initializing generator") {
			t.Fatalf("expected error to contain 'error initializing generator', got %v", err)
		} else {
			t.Logf("expected error received: %v", err)
		}
	})

	t.Run("ErrorInitializingStack", func(t *testing.T) {
		// Given a new controller with a mock injector
		injector := di.NewInjector()
		controller := NewController(injector)
		controller.Initialize()
		mockStack := stack.NewMockStack(injector)
		mockStack.InitializeFunc = func() error {
			return fmt.Errorf("error initializing stack")
		}
		injector.Register("stack", mockStack)

		// When initializing the components
		err := controller.InitializeComponents()

		// Then there should be an error
		if err == nil {
			t.Fatalf("expected an error, got nil")
		} else if !strings.Contains(err.Error(), "error initializing stack") {
			t.Fatalf("expected error to contain 'error initializing stack', got %v", err)
		} else {
			t.Logf("expected error received: %v", err)
		}
	})
}

func TestController_CreateCommonComponents(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new controller
		injector := di.NewInjector()
		controller := NewController(injector)
		controller.Initialize()

		// When creating common components
		err := controller.CreateCommonComponents()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestController_CreateProjectComponents(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new controller
		injector := di.NewInjector()
		controller := NewController(injector)
		controller.Initialize()

		// When creating project components
		err := controller.CreateProjectComponents()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestController_CreateEnvComponents(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new controller
		injector := di.NewInjector()
		controller := NewController(injector)
		controller.Initialize()

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
		injector := di.NewInjector()
		controller := NewController(injector)
		controller.Initialize()

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
		injector := di.NewInjector()
		controller := NewController(injector)
		controller.Initialize()

		// When creating virtualization components
		err := controller.CreateVirtualizationComponents()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestController_CreateStackComponents(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new controller
		injector := di.NewInjector()
		controller := NewController(injector)
		controller.Initialize()

		// When creating stack components
		err := controller.CreateStackComponents()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestController_WriteConfigurationFiles(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new controller
		injector := di.NewInjector()
		registerMockResources(injector)
		controller := NewController(injector)
		controller.Initialize()

		// Mock write methods for validation
		mockService1 := injector.Resolve("service1").(*services.MockService)
		mockService2 := injector.Resolve("service2").(*services.MockService)
		mockVirtualMachine := injector.Resolve("virtualMachine").(*virt.MockVirt)
		mockContainerRuntime := injector.Resolve("containerRuntime").(*virt.MockVirt)
		mockToolsManager := injector.Resolve("toolsManager").(*tools.MockToolsManager)
		mockBlueprintHandler := injector.Resolve("blueprintHandler").(*blueprint.MockBlueprintHandler)
		mockGenerator := injector.Resolve("generator").(*generators.MockGenerator)

		// Set up a map to track calls
		callTracker := map[string]bool{
			"service1":         false,
			"service2":         false,
			"virtualMachine":   false,
			"containerRuntime": false,
			"toolsManager":     false,
			"blueprintHandler": false,
			"generator":        false,
		}

		mockService1.WriteConfigFunc = func() error { callTracker["service1"] = true; return nil }
		mockService2.WriteConfigFunc = func() error { callTracker["service2"] = true; return nil }
		mockVirtualMachine.WriteConfigFunc = func() error { callTracker["virtualMachine"] = true; return nil }
		mockContainerRuntime.WriteConfigFunc = func() error { callTracker["containerRuntime"] = true; return nil }
		mockToolsManager.WriteManifestFunc = func() error { callTracker["toolsManager"] = true; return nil }
		mockBlueprintHandler.WriteConfigFunc = func(path ...string) error { callTracker["blueprintHandler"] = true; return nil }
		mockGenerator.WriteFunc = func() error { callTracker["generator"] = true; return nil }

		// When writing configuration files
		err := controller.WriteConfigurationFiles()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Validate successful writing
		for component, called := range callTracker {
			if !called {
				t.Fatalf("expected WriteConfig to be called on %s", component)
			}
		}
	})

	t.Run("ErrorWritingToolsManifest", func(t *testing.T) {
		// Given a new controller
		injector := di.NewInjector()
		registerMockResources(injector)
		controller := NewController(injector)
		controller.Initialize()

		// Mock an error on WriteManifestFunc
		mockToolsManager := injector.Resolve("toolsManager").(*tools.MockToolsManager)
		mockToolsManager.WriteManifestFunc = func() error {
			return fmt.Errorf("error writing tools manifest")
		}

		// When writing configuration files
		err := controller.WriteConfigurationFiles()

		// Then there should be an error
		if err == nil || !strings.Contains(err.Error(), "error writing tools manifest") {
			t.Fatalf("expected error writing tools manifest, got %v", err)
		}
	})

	t.Run("ErrorWritingBlueprintConfig", func(t *testing.T) {
		// Given a new controller
		injector := di.NewInjector()
		registerMockResources(injector)
		controller := NewController(injector)
		controller.Initialize()

		// Mock an error on WriteConfigFunc for blueprintHandler
		mockBlueprintHandler := injector.Resolve("blueprintHandler").(*blueprint.MockBlueprintHandler)
		mockBlueprintHandler.WriteConfigFunc = func(path ...string) error {
			return fmt.Errorf("error writing blueprint config")
		}

		// When writing configuration files
		err := controller.WriteConfigurationFiles()

		// Then there should be an error
		if err == nil || !strings.Contains(err.Error(), "error writing blueprint config") {
			t.Fatalf("expected error writing blueprint config, got %v", err)
		}
	})

	t.Run("ErrorWritingServiceConfig", func(t *testing.T) {
		// Given a new controller
		injector := di.NewInjector()
		registerMockResources(injector)
		controller := NewController(injector)
		controller.Initialize()

		// Mock an error on WriteConfigFunc for service1
		mockService1 := injector.Resolve("service1").(*services.MockService)
		mockService1.WriteConfigFunc = func() error {
			return fmt.Errorf("error writing service config")
		}

		// When writing configuration files
		err := controller.WriteConfigurationFiles()

		// Then there should be an error
		if err == nil || !strings.Contains(err.Error(), "error writing service config") {
			t.Fatalf("expected error writing service config, got %v", err)
		}
	})

	t.Run("ErrorWritingVirtualMachineConfig", func(t *testing.T) {
		// Given a new controller
		injector := di.NewInjector()
		registerMockResources(injector)
		controller := NewController(injector)
		controller.Initialize()

		// Mock an error on WriteConfigFunc for virtualMachine
		mockVirtualMachine := injector.Resolve("virtualMachine").(*virt.MockVirt)
		mockVirtualMachine.WriteConfigFunc = func() error {
			return fmt.Errorf("error writing virtual machine config")
		}

		// When writing configuration files
		err := controller.WriteConfigurationFiles()

		// Then there should be an error
		if err == nil || !strings.Contains(err.Error(), "error writing virtual machine config") {
			t.Fatalf("expected error writing virtual machine config, got %v", err)
		}
	})

	t.Run("ErrorWritingContainerRuntimeConfig", func(t *testing.T) {
		// Given a new controller
		injector := di.NewInjector()
		registerMockResources(injector)
		controller := NewController(injector)
		controller.Initialize()

		// Mock an error on WriteConfigFunc for containerRuntime
		mockContainerRuntime := injector.Resolve("containerRuntime").(*virt.MockVirt)
		mockContainerRuntime.WriteConfigFunc = func() error {
			return fmt.Errorf("error writing container runtime config")
		}

		// When writing configuration files
		err := controller.WriteConfigurationFiles()

		// Then there should be an error
		if err == nil || !strings.Contains(err.Error(), "error writing container runtime config") {
			t.Fatalf("expected error writing container runtime config, got %v", err)
		}
	})

	t.Run("ErrorWritingGeneratorConfig", func(t *testing.T) {
		// Given a new controller
		injector := di.NewInjector()
		registerMockResources(injector)
		controller := NewController(injector)
		controller.Initialize()

		// Mock an error on WriteFunc for generator
		mockGenerator := injector.Resolve("generator").(*generators.MockGenerator)
		mockGenerator.WriteFunc = func() error {
			return fmt.Errorf("error writing generator config")
		}

		// When writing configuration files
		err := controller.WriteConfigurationFiles()

		// Then there should be an error
		if err == nil || !strings.Contains(err.Error(), "error writing generator config") {
			t.Fatalf("expected error writing generator config, got %v", err)
		}
	})
}

func TestController_ResolveInjector(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new controller and injector
		injector := di.NewInjector()
		controller := NewController(injector)
		controller.Initialize()

		// When resolving the injector
		resolvedInjector := controller.ResolveInjector()

		// Then the resolved injector should match the original injector
		if resolvedInjector != injector {
			t.Fatalf("expected %v, got %v", injector, resolvedInjector)
		}
	})
}

func TestController_ResolveConfigHandler(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new controller and injector
		injector := di.NewInjector()
		controller := NewController(injector)
		controller.Initialize()

		// When resolving the config handler
		configHandler := controller.ResolveConfigHandler()

		// And the resolved config handler should match the expected config handler
		if configHandler != controller.configHandler {
			t.Fatalf("expected %v, got %v", controller.configHandler, configHandler)
		}
	})
}

func TestController_ResolveEnvPrinter(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new controller and injector
		injector := di.NewInjector()
		injector.Register("envPrinter1", nil) // Register a nil env printer as envPrinter1
		controller := NewController(injector)
		controller.Initialize()

		// When resolving the env printer
		envPrinter := controller.ResolveEnvPrinter("envPrinter1")

		// Then the resolved env printer should be nil
		if envPrinter != nil {
			t.Fatalf("expected nil, got %v", envPrinter)
		}
	})
}

func TestController_ResolveAllEnvPrinters(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new controller and injector
		injector := di.NewInjector()
		mockEnvPrinter1 := env.NewMockEnvPrinter()
		mockEnvPrinter2 := env.NewMockEnvPrinter()
		injector.Register("envPrinter1", mockEnvPrinter1)
		injector.Register("envPrinter2", mockEnvPrinter2)
		controller := NewController(injector)
		controller.Initialize()

		// When resolving all env printers
		envPrinters := controller.ResolveAllEnvPrinters()

		// Then the number of resolved env printers should match the expected number
		if len(envPrinters) != 2 {
			t.Fatalf("expected %d env printers, got %d", 2, len(envPrinters))
		}

		// And each resolved env printer should not be nil
		for _, printer := range envPrinters {
			if printer == nil {
				t.Fatalf("expected non-nil printer, got nil")
			}
		}
	})
}

func TestController_ResolveShell(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new controller and injector
		injector := di.NewInjector()
		injector.Register("shell", nil) // Register a nil shell
		controller := NewController(injector)
		controller.Initialize()

		// When resolving the shell
		shellInstance := controller.ResolveShell()

		// Then the resolved shell should be nil
		if shellInstance != nil {
			t.Fatalf("expected nil, got %v", shellInstance)
		}
	})
}

func TestController_ResolveSecureShell(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new controller and injector
		injector := di.NewInjector()
		mockSecureShell := shell.NewMockShell() // Create a mock secure shell
		injector.Register("secureShell", mockSecureShell)
		controller := NewController(injector)
		controller.Initialize()

		// When resolving the secure shell
		secureShell := controller.ResolveSecureShell()

		// Then the resolved secure shell should match the mock
		if secureShell != mockSecureShell {
			t.Fatalf("expected %v, got %v", mockSecureShell, secureShell)
		}
	})
}

func TestController_ResolveBlueprintHandler(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new controller and injector
		injector := di.NewInjector()
		mockBlueprintHandler := blueprint.NewMockBlueprintHandler(injector) // Create a mock blueprint handler
		injector.Register("blueprintHandler", mockBlueprintHandler)
		controller := NewController(injector)
		controller.Initialize()

		// When resolving the blueprint handler
		blueprintHandler := controller.ResolveBlueprintHandler()

		// Then the resolved blueprint handler should match the mock
		if blueprintHandler != mockBlueprintHandler {
			t.Fatalf("expected %v, got %v", mockBlueprintHandler, blueprintHandler)
		}
	})
}

func TestController_ResolveNetworkManager(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new controller and injector
		injector := di.NewInjector()
		mockNetworkManager := network.NewMockNetworkManager() // Create a mock network manager
		injector.Register("networkManager", mockNetworkManager)
		controller := NewController(injector)
		controller.Initialize()

		// When resolving the network manager
		networkManager := controller.ResolveNetworkManager()

		// Then the resolved network manager should match the mock
		if networkManager != mockNetworkManager {
			t.Fatalf("expected %v, got %v", mockNetworkManager, networkManager)
		}
	})
}

func TestController_ResolveService(t *testing.T) {
	t.Run("ResolveService", func(t *testing.T) {
		// Given a new controller and injector
		injector := di.NewInjector()
		mockService := services.NewMockService() // Create a mock service
		injector.Register("service1", mockService)
		controller := NewController(injector)
		controller.Initialize()

		// When resolving the service
		service := controller.ResolveService("service1")

		// Then the resolved service should match the mock
		if service != mockService {
			t.Fatalf("expected %v, got %v", mockService, service)
		}
	})
}

func TestController_ResolveAllServices(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new controller and injector
		injector := di.NewInjector()
		mockResources := registerMockResources(injector) // Use registerMockResources to register mocks
		controller := NewController(injector)
		controller.Initialize()

		// When resolving all services
		resolvedServices := controller.ResolveAllServices()

		// Then the number of resolved services should match the expected number
		if len(resolvedServices) != 2 {
			t.Fatalf("expected %v, got %v", 2, len(resolvedServices))
		}

		// And each resolved service should match the mock
		if resolvedServices[0] != mockResources.Service1 || resolvedServices[1] != mockResources.Service2 {
			t.Fatalf("expected services to match mocks, got %v and %v", resolvedServices[0], resolvedServices[1])
		}
	})
}

func TestController_ResolveVirtualMachine(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new controller and injector
		injector := di.NewInjector()
		mockVirtualMachine := virt.NewMockVirt() // Create a mock virtual machine
		injector.Register("virtualMachine", mockVirtualMachine)
		controller := NewController(injector)
		controller.Initialize()

		// When resolving the virtual machine
		virtualMachine := controller.ResolveVirtualMachine()

		// Then the resolved virtual machine should match the mock
		if virtualMachine != mockVirtualMachine {
			t.Fatalf("expected %v, got %v", mockVirtualMachine, virtualMachine)
		}
	})
}

func TestController_ResolveContainerRuntime(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new controller and injector
		injector := di.NewInjector()
		mockContainerRuntime := virt.NewMockVirt() // Create a mock container runtime
		injector.Register("containerRuntime", mockContainerRuntime)
		controller := NewController(injector)
		controller.Initialize()

		// When resolving the container runtime
		containerRuntime := controller.ResolveContainerRuntime()

		// Then the resolved container runtime should match the mock
		if containerRuntime != mockContainerRuntime {
			t.Fatalf("expected %v, got %v", mockContainerRuntime, containerRuntime)
		}
	})
}
