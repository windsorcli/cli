package controller

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/blueprint"
	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/env"
	"github.com/windsorcli/cli/pkg/generators"
	"github.com/windsorcli/cli/pkg/network"
	"github.com/windsorcli/cli/pkg/secrets"
	"github.com/windsorcli/cli/pkg/services"
	"github.com/windsorcli/cli/pkg/shell"
	"github.com/windsorcli/cli/pkg/stack"
	"github.com/windsorcli/cli/pkg/tools"
	"github.com/windsorcli/cli/pkg/virt"
)

// =============================================================================
// Test Setup
// =============================================================================

type Mocks struct {
	Injector          di.Injector
	ConfigHandler     config.ConfigHandler
	SecretsProvider   *secrets.MockSecretsProvider
	EnvPrinter        *env.MockEnvPrinter
	WindsorEnvPrinter *env.MockEnvPrinter
	Shell             *shell.MockShell
	SecureShell       *shell.MockShell
	ToolsManager      *tools.MockToolsManager
	NetworkManager    *network.MockNetworkManager
	Service           *services.MockService
	VirtualMachine    *virt.MockVirt
	ContainerRuntime  *virt.MockVirt
	BlueprintHandler  *blueprint.MockBlueprintHandler
	Stack             *stack.MockStack
	Generator         *generators.MockGenerator
}

type SetupOptions struct {
	Injector      di.Injector
	ConfigHandler config.ConfigHandler
	ConfigStr     string
}

func setupMocks(t *testing.T, opts ...*SetupOptions) *Mocks {
	t.Helper()

	// Store original working directory
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}

	// Create temp dir using testing.TempDir()
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to change to temp directory: %v", err)
	}

	os.Setenv("WINDSOR_PROJECT_ROOT", tmpDir)

	options := &SetupOptions{}
	if len(opts) > 0 && opts[0] != nil {
		options = opts[0]
	}

	var injector di.Injector
	if options.Injector == nil {
		injector = di.NewMockInjector()
	} else {
		injector = options.Injector
	}

	var configHandler config.ConfigHandler
	if options.ConfigHandler == nil {
		configHandler = config.NewYamlConfigHandler(injector)
	} else {
		configHandler = options.ConfigHandler
	}

	// Create mock components
	mockSecretsProvider := secrets.NewMockSecretsProvider(injector)
	mockEnvPrinter := env.NewMockEnvPrinter()
	mockWindsorEnvPrinter := env.NewMockEnvPrinter()
	mockShell := shell.NewMockShell()
	mockSecureShell := shell.NewMockShell()
	mockToolsManager := tools.NewMockToolsManager()
	mockNetworkManager := network.NewMockNetworkManager()
	mockService := services.NewMockService()
	mockVirtualMachine := virt.NewMockVirt()
	mockContainerRuntime := virt.NewMockVirt()
	mockBlueprintHandler := blueprint.NewMockBlueprintHandler(injector)
	mockGenerator := generators.NewMockGenerator()
	mockStack := stack.NewMockStack(injector)

	// Register all mocks in the injector
	injector.Register("configHandler", configHandler)
	injector.Register("secretsProvider", mockSecretsProvider)
	injector.Register("envPrinter1", mockEnvPrinter)
	injector.Register("envPrinter2", mockEnvPrinter)
	injector.Register("windsorEnv", mockWindsorEnvPrinter)
	injector.Register("shell", mockShell)
	injector.Register("secureShell", mockSecureShell)
	injector.Register("toolsManager", mockToolsManager)
	injector.Register("networkManager", mockNetworkManager)
	injector.Register("blueprintHandler", mockBlueprintHandler)
	injector.Register("service1", mockService)
	injector.Register("service2", mockService)
	injector.Register("virtualMachine", mockVirtualMachine)
	injector.Register("containerRuntime", mockContainerRuntime)
	injector.Register("generator", mockGenerator)
	injector.Register("stack", mockStack)

	// Initialize and configure config handler
	configHandler.Initialize()
	configHandler.SetContext("mock-context")

	defaultConfigStr := `
version: v1alpha1
toolsManager: default
contexts:
  mock-context:
    projectName: mock-project
    environment:
      MOCK_ENV: "true"
    
    # Core service configuration
    docker:
      enabled: true
      registryUrl: mock.registry.com
    
    cluster:
      enabled: true
      workers:
        enabled: true
    
    vm:
      enabled: true
      driver: colima
    
    # Network configuration
    dns:
      enabled: true
      domain: mock.domain.com
    
    network:
      enabled: true
      cidrBlock: 192.168.1.0/24
    
    # Tools and secrets
    terraform:
      enabled: true
      backend:
        type: local
    
    secrets:
      provider: mock
      enabled: true
`

	if err := configHandler.LoadConfigString(defaultConfigStr); err != nil {
		t.Fatalf("Failed to load default config string: %v", err)
	}
	if options.ConfigStr != "" {
		if err := configHandler.LoadConfigString(options.ConfigStr); err != nil {
			t.Fatalf("Failed to load config string: %v", err)
		}
	}

	// Set up default mock behaviors
	mockWindsorEnvPrinter.GetEnvVarsFunc = func() (map[string]string, error) {
		return map[string]string{
			"WINDSOR_CONTEXT":       "mock-context",
			"WINDSOR_PROJECT_ROOT":  tmpDir,
			"WINDSOR_SESSION_TOKEN": "mock-token",
		}, nil
	}

	mockShell.GetProjectRootFunc = func() (string, error) {
		return tmpDir, nil
	}

	mockShell.GetSessionTokenFunc = func() (string, error) {
		return "mock-token", nil
	}

	mockShell.WriteResetTokenFunc = func() (string, error) {
		return filepath.Join(tmpDir, ".windsor", ".session.mock-token"), nil
	}

	// Initialize all components that need initialization
	mockSecretsProvider.InitializeFunc = func() error { return nil }
	mockEnvPrinter.InitializeFunc = func() error { return nil }
	mockWindsorEnvPrinter.InitializeFunc = func() error { return nil }
	mockShell.InitializeFunc = func() error { return nil }
	mockSecureShell.InitializeFunc = func() error { return nil }
	mockToolsManager.InitializeFunc = func() error { return nil }
	mockNetworkManager.InitializeFunc = func() error { return nil }
	mockService.InitializeFunc = func() error { return nil }
	mockVirtualMachine.InitializeFunc = func() error { return nil }
	mockContainerRuntime.InitializeFunc = func() error { return nil }
	mockBlueprintHandler.InitializeFunc = func() error { return nil }
	mockGenerator.InitializeFunc = func() error { return nil }
	mockStack.InitializeFunc = func() error { return nil }

	// Set up blueprint handler defaults
	mockBlueprintHandler.LoadConfigFunc = func(path ...string) error { return nil }
	mockBlueprintHandler.WriteConfigFunc = func(path ...string) error { return nil }

	// Set up tools manager defaults
	mockToolsManager.WriteManifestFunc = func() error { return nil }

	// Set up service defaults
	mockService.WriteConfigFunc = func() error { return nil }

	// Set up virtual machine defaults
	mockVirtualMachine.WriteConfigFunc = func() error { return nil }

	// Set up container runtime defaults
	mockContainerRuntime.WriteConfigFunc = func() error { return nil }

	// Set up generator defaults
	mockGenerator.WriteFunc = func() error { return nil }

	t.Cleanup(func() {
		os.Unsetenv("WINDSOR_PROJECT_ROOT")

		if err := os.Chdir(origDir); err != nil {
			t.Logf("Warning: Failed to change back to original directory: %v", err)
		}
	})

	return &Mocks{
		Injector:          injector,
		ConfigHandler:     configHandler,
		SecretsProvider:   mockSecretsProvider,
		EnvPrinter:        mockEnvPrinter,
		WindsorEnvPrinter: mockWindsorEnvPrinter,
		Shell:             mockShell,
		SecureShell:       mockSecureShell,
		ToolsManager:      mockToolsManager,
		NetworkManager:    mockNetworkManager,
		Service:           mockService,
		VirtualMachine:    mockVirtualMachine,
		ContainerRuntime:  mockContainerRuntime,
		BlueprintHandler:  mockBlueprintHandler,
		Stack:             mockStack,
		Generator:         mockGenerator,
	}
}

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewController(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new test setup
		mocks := setupMocks(t)

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

// =============================================================================
// Test Public Methods
// =============================================================================

func TestController_InitializeComponents(t *testing.T) {
	setup := func(t *testing.T) (Controller, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		controller := NewController(mocks.Injector)
		err := controller.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize controller: %v", err)
		}
		return controller, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a new controller
		controller, _ := setup(t)

		// When initializing the components
		err := controller.InitializeComponents()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("ErrorInitializingShell", func(t *testing.T) {
		// Given a mock shell that returns an error
		controller, mocks := setup(t)
		mocks.Shell.InitializeFunc = func() error {
			return fmt.Errorf("error initializing shell")
		}

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
		// Given a mock secure shell that returns an error
		controller, mocks := setup(t)
		mocks.SecureShell.InitializeFunc = func() error {
			return fmt.Errorf("error initializing secure shell")
		}

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
		// Given a mock env printer that returns an error
		controller, mocks := setup(t)
		mocks.EnvPrinter.InitializeFunc = func() error {
			return fmt.Errorf("error initializing env printer")
		}

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
		// Given a mock tools manager that returns an error
		controller, mocks := setup(t)
		mocks.ToolsManager.InitializeFunc = func() error {
			return fmt.Errorf("error initializing tools manager")
		}

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
		// Given a mock network manager that returns an error
		controller, mocks := setup(t)
		mocks.NetworkManager.InitializeFunc = func() error {
			return fmt.Errorf("error initializing network manager")
		}

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
		// Given a mock service that returns an error
		controller, mocks := setup(t)
		mocks.Service.InitializeFunc = func() error {
			return fmt.Errorf("error initializing service")
		}

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
		// Given a mock virtual machine that returns an error
		controller, mocks := setup(t)
		mocks.VirtualMachine.InitializeFunc = func() error {
			return fmt.Errorf("error initializing virtual machine")
		}

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
		// Given a mock container runtime that returns an error
		controller, mocks := setup(t)
		mocks.ContainerRuntime.InitializeFunc = func() error {
			return fmt.Errorf("error initializing container runtime")
		}

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
		// Given a mock blueprint handler that returns an error
		controller, mocks := setup(t)
		mocks.BlueprintHandler.InitializeFunc = func() error {
			return fmt.Errorf("error initializing blueprint handler")
		}

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
		// Given a mock blueprint handler that returns an error on load config
		controller, mocks := setup(t)
		mocks.BlueprintHandler.LoadConfigFunc = func(path ...string) error {
			return fmt.Errorf("error loading blueprint config")
		}

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
		// Given a mock generator that returns an error
		controller, mocks := setup(t)
		mocks.Generator.InitializeFunc = func() error {
			return fmt.Errorf("error initializing generator")
		}

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
		// Given a mock stack that returns an error
		controller, mocks := setup(t)
		mocks.Stack.InitializeFunc = func() error {
			return fmt.Errorf("error initializing stack")
		}

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

	t.Run("ErrorInitializingSecretsProvider", func(t *testing.T) {
		// Given a mock secrets provider that returns an error
		controller, mocks := setup(t)
		mocks.SecretsProvider.InitializeFunc = func() error {
			return fmt.Errorf("error initializing secrets provider")
		}

		// When initializing the components
		err := controller.InitializeComponents()

		// Then there should be an error
		if err == nil {
			t.Fatalf("expected an error, got nil")
		} else if !strings.Contains(err.Error(), "error initializing secrets provider") {
			t.Fatalf("expected error to contain 'error initializing secrets provider', got %v", err)
		} else {
			t.Logf("expected error received: %v", err)
		}
	})
}

func TestController_CreateCommonComponents(t *testing.T) {
	setup := func(t *testing.T) (Controller, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		controller := NewController(mocks.Injector)
		err := controller.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize controller: %v", err)
		}
		return controller, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a new controller
		controller, _ := setup(t)

		// When creating common components
		err := controller.CreateCommonComponents()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestController_CreateSecretsProviders(t *testing.T) {
	setup := func(t *testing.T) (Controller, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		controller := NewController(mocks.Injector)
		err := controller.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize controller: %v", err)
		}
		return controller, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a new controller
		controller, _ := setup(t)

		// When creating secrets provider
		err := controller.CreateSecretsProviders()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestController_CreateProjectComponents(t *testing.T) {
	setup := func(t *testing.T) (Controller, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		controller := NewController(mocks.Injector)
		err := controller.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize controller: %v", err)
		}
		return controller, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a new controller
		controller, _ := setup(t)

		// When creating project components
		err := controller.CreateProjectComponents()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestController_CreateEnvComponents(t *testing.T) {
	setup := func(t *testing.T) (Controller, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		controller := NewController(mocks.Injector)
		err := controller.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize controller: %v", err)
		}
		return controller, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a new controller
		controller, _ := setup(t)

		// When creating env components
		err := controller.CreateEnvComponents()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestController_CreateServiceComponents(t *testing.T) {
	setup := func(t *testing.T) (Controller, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		controller := NewController(mocks.Injector)
		err := controller.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize controller: %v", err)
		}
		return controller, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a new controller
		controller, _ := setup(t)

		// When creating service components
		err := controller.CreateServiceComponents()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestController_CreateVirtualizationComponents(t *testing.T) {
	setup := func(t *testing.T) (Controller, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		controller := NewController(mocks.Injector)
		err := controller.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize controller: %v", err)
		}
		return controller, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a new controller
		controller, _ := setup(t)

		// When creating virtualization components
		err := controller.CreateVirtualizationComponents()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestController_CreateStackComponents(t *testing.T) {
	setup := func(t *testing.T) (Controller, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		controller := NewController(mocks.Injector)
		err := controller.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize controller: %v", err)
		}
		return controller, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a new controller
		controller, _ := setup(t)

		// When creating stack components
		err := controller.CreateStackComponents()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestController_WriteConfigurationFiles(t *testing.T) {
	setup := func(t *testing.T) (Controller, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		controller := NewController(mocks.Injector)
		err := controller.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize controller: %v", err)
		}
		return controller, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a new controller
		controller, _ := setup(t)

		// When writing configuration files
		err := controller.WriteConfigurationFiles()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("ErrorWritingToolsManifest", func(t *testing.T) {
		// Given a mock tools manager that returns an error
		controller, mocks := setup(t)
		mocks.ToolsManager.WriteManifestFunc = func() error {
			return fmt.Errorf("error writing tools manifest")
		}

		// When writing configuration files
		err := controller.WriteConfigurationFiles()

		// Then there should be an error
		if err == nil {
			t.Fatalf("expected an error, got nil")
		} else if !strings.Contains(err.Error(), "error writing tools manifest") {
			t.Fatalf("expected error to contain 'error writing tools manifest', got %v", err)
		} else {
			t.Logf("expected error received: %v", err)
		}
	})

	t.Run("ErrorWritingBlueprintConfig", func(t *testing.T) {
		// Given a mock blueprint handler that returns an error
		controller, mocks := setup(t)
		mocks.BlueprintHandler.WriteConfigFunc = func(path ...string) error {
			return fmt.Errorf("error writing blueprint config")
		}

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
		// Given a mock service that returns an error
		controller, mocks := setup(t)
		mocks.Service.WriteConfigFunc = func() error {
			return fmt.Errorf("error writing service config")
		}

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
		// Given a mock virtual machine that returns an error
		controller, mocks := setup(t)
		mocks.VirtualMachine.WriteConfigFunc = func() error {
			return fmt.Errorf("error writing virtual machine config")
		}

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
		// Given a mock container runtime that returns an error
		controller, mocks := setup(t)
		mocks.ContainerRuntime.WriteConfigFunc = func() error {
			return fmt.Errorf("error writing container runtime config")
		}

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

	t.Run("ErrorWritingGeneratorConfig", func(t *testing.T) {
		// Given a mock generator that returns an error
		controller, mocks := setup(t)
		mocks.Generator.WriteFunc = func() error {
			return fmt.Errorf("error writing generator config")
		}

		// When writing configuration files
		err := controller.WriteConfigurationFiles()

		// Then there should be an error
		if err == nil {
			t.Fatalf("expected an error, got nil")
		} else if !strings.Contains(err.Error(), "error writing generator config") {
			t.Fatalf("expected error to contain 'error writing generator config', got %v", err)
		} else {
			t.Logf("expected error received: %v", err)
		}
	})
}

func TestController_ResolveInjector(t *testing.T) {
	setup := func(t *testing.T) (Controller, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		controller := NewController(mocks.Injector)
		err := controller.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize controller: %v", err)
		}
		return controller, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a new controller and injector
		controller, mocks := setup(t)

		// When resolving the injector
		resolvedInjector := controller.ResolveInjector()

		// Then the resolved injector should match the original injector
		if resolvedInjector != mocks.Injector {
			t.Fatalf("expected %v, got %v", mocks.Injector, resolvedInjector)
		}
	})
}

func TestController_ResolveConfigHandler(t *testing.T) {
	setup := func(t *testing.T) (Controller, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		controller := NewController(mocks.Injector)
		err := controller.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize controller: %v", err)
		}
		return controller, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a new controller and injector
		controller, mocks := setup(t)

		// When resolving the config handler
		configHandler := controller.ResolveConfigHandler()

		// And the resolved config handler should match the expected config handler
		if configHandler != mocks.ConfigHandler {
			t.Fatalf("expected %v, got %v", mocks.ConfigHandler, configHandler)
		}
	})
}

func TestController_ResolveAllSecretsProviders(t *testing.T) {
	setup := func(t *testing.T) (Controller, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		controller := NewController(mocks.Injector)
		err := controller.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize controller: %v", err)
		}
		return controller, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a new controller and injector
		controller, mocks := setup(t)

		// When resolving the secrets provider
		secretsProviders := controller.ResolveAllSecretsProviders()

		// Then there should be no error
		if secretsProviders == nil {
			t.Fatalf("expected no error, got nil")
		}

		// And the resolved secrets provider should match the expected secrets provider
		if len(secretsProviders) != 1 {
			t.Fatalf("expected %v, got %v", 1, len(secretsProviders))
		}
		if secretsProviders[0] != mocks.SecretsProvider {
			t.Fatalf("expected %v, got %v", mocks.SecretsProvider, secretsProviders[0])
		}
	})
}

func TestController_ResolveEnvPrinter(t *testing.T) {
	setup := func(t *testing.T) (Controller, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		controller := NewController(mocks.Injector)
		err := controller.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize controller: %v", err)
		}
		return controller, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a new controller and injector
		controller, mocks := setup(t)

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
	setup := func(t *testing.T) (Controller, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		controller := NewController(mocks.Injector)
		err := controller.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize controller: %v", err)
		}
		return controller, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a new controller with multiple envPrinters
		controller, _ := setup(t)

		// When resolving all envPrinters
		envPrinters := controller.ResolveAllEnvPrinters()

		// Then all envPrinters should be returned
		if len(envPrinters) < 3 {
			t.Fatalf("expected at least 3 envPrinters, got %d", len(envPrinters))
		}
	})

	t.Run("WindsorEnvIsLastPrinter", func(t *testing.T) {
		// Given a new controller with multiple envPrinters
		controller, _ := setup(t)

		// When resolving all envPrinters
		envPrinters := controller.ResolveAllEnvPrinters()

		// Then the last envPrinter should be the WindsorEnvPrinter
		if len(envPrinters) < 1 {
			t.Fatalf("expected at least 1 envPrinter, got %d", len(envPrinters))
		}

		// Get the last printer
		lastPrinter := envPrinters[len(envPrinters)-1]

		// Since we're using a MockEnvPrinter instead of WindsorEnvPrinter for tests
		_, isMockEnv := lastPrinter.(*env.MockEnvPrinter)
		if !isMockEnv {
			t.Errorf("expected last printer to be *env.MockEnvPrinter, got %T", lastPrinter)
		}
	})

	t.Run("WindsorEnvPrinterTypeAssertion", func(t *testing.T) {
		// Given a new controller
		controller, mocks := setup(t)

		// And a WindsorEnvPrinter is registered
		windsorEnvPrinter := env.NewWindsorEnvPrinter(mocks.Injector)
		mocks.Injector.Register("windsorEnv", windsorEnvPrinter)

		// When resolving all envPrinters
		envPrinters := controller.ResolveAllEnvPrinters()

		// Then the WindsorEnvPrinter should be in the list
		found := false
		for _, printer := range envPrinters {
			if _, ok := printer.(*env.WindsorEnvPrinter); ok {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected to find WindsorEnvPrinter in the list of envPrinters")
		}
	})

	t.Run("WindsorEnvPrinterIsAppended", func(t *testing.T) {
		// Given a new controller
		controller, mocks := setup(t)

		// And a WindsorEnvPrinter is registered
		windsorEnvPrinter := env.NewWindsorEnvPrinter(mocks.Injector)
		mocks.Injector.Register("windsorEnv", windsorEnvPrinter)

		// When resolving all envPrinters
		envPrinters := controller.ResolveAllEnvPrinters()

		// Then the WindsorEnvPrinter should be the last printer in the list
		if len(envPrinters) < 1 {
			t.Fatalf("expected at least 1 envPrinter, got %d", len(envPrinters))
		}

		lastPrinter := envPrinters[len(envPrinters)-1]
		if _, ok := lastPrinter.(*env.WindsorEnvPrinter); !ok {
			t.Errorf("expected last printer to be *env.WindsorEnvPrinter, got %T", lastPrinter)
		}
	})
}

func TestController_ResolveShell(t *testing.T) {
	setup := func(t *testing.T) (Controller, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		controller := NewController(mocks.Injector)
		err := controller.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize controller: %v", err)
		}
		return controller, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a new controller and injector
		controller, mocks := setup(t)

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
	setup := func(t *testing.T) (Controller, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		controller := NewController(mocks.Injector)
		err := controller.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize controller: %v", err)
		}
		return controller, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a new controller and injector
		controller, _ := setup(t)

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
	setup := func(t *testing.T) (Controller, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		controller := NewController(mocks.Injector)
		err := controller.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize controller: %v", err)
		}
		return controller, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a new controller and injector
		controller, mocks := setup(t)

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
	setup := func(t *testing.T) (Controller, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		controller := NewController(mocks.Injector)
		err := controller.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize controller: %v", err)
		}
		return controller, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a new controller and injector
		controller, mocks := setup(t)

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
	setup := func(t *testing.T) (Controller, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		controller := NewController(mocks.Injector)
		err := controller.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize controller: %v", err)
		}
		return controller, mocks
	}

	t.Run("ResolveService", func(t *testing.T) {
		// Given a new controller and injector
		controller, mocks := setup(t)

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
	setup := func(t *testing.T) (Controller, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		controller := NewController(mocks.Injector)
		err := controller.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize controller: %v", err)
		}
		return controller, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a new controller and injector
		controller, mocks := setup(t)

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
	setup := func(t *testing.T) (Controller, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		controller := NewController(mocks.Injector)
		err := controller.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize controller: %v", err)
		}
		return controller, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a new controller and injector
		controller, mocks := setup(t)

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
	setup := func(t *testing.T) (Controller, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		controller := NewController(mocks.Injector)
		err := controller.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize controller: %v", err)
		}
		return controller, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a new controller and injector
		controller, mocks := setup(t)

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

func TestController_SetEnvironmentVariables(t *testing.T) {
	setup := func(t *testing.T) (Controller, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		controller := NewController(mocks.Injector)
		err := controller.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize controller: %v", err)
		}
		return controller, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a new controller
		controller, _ := setup(t)

		// When setting environment variables
		err := controller.SetEnvironmentVariables()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("ErrorGettingEnvVars", func(t *testing.T) {
		// Given a mock env printer that returns an error
		controller, mocks := setup(t)
		mocks.EnvPrinter.GetEnvVarsFunc = func() (map[string]string, error) {
			return nil, fmt.Errorf("error getting environment variables")
		}

		// When setting environment variables
		err := controller.SetEnvironmentVariables()

		// Then there should be an error
		if err == nil {
			t.Fatalf("expected an error, got nil")
		} else if !strings.Contains(err.Error(), "error getting environment variables") {
			t.Fatalf("expected error to contain 'error getting environment variables', got %v", err)
		} else {
			t.Logf("expected error received: %v", err)
		}
	})

	t.Run("ErrorSettingEnvVars", func(t *testing.T) {
		// Given a mock env printer that returns environment variables
		controller, mocks := setup(t)
		mocks.EnvPrinter.GetEnvVarsFunc = func() (map[string]string, error) {
			return map[string]string{"TEST_VAR": "test_value"}, nil
		}

		// And a mock os.Setenv that returns an error
		originalSetenv := osSetenv
		defer func() { osSetenv = originalSetenv }()
		osSetenv = func(key, value string) error {
			return fmt.Errorf("error setting environment variable")
		}

		// When setting environment variables
		err := controller.SetEnvironmentVariables()

		// Then there should be an error
		if err == nil {
			t.Fatalf("expected an error, got nil")
		} else if !strings.Contains(err.Error(), "error setting environment variable") {
			t.Fatalf("expected error to contain 'error setting environment variable', got %v", err)
		} else {
			t.Logf("expected error received: %v", err)
		}
	})
}
