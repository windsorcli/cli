package controller

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	secretsConfigType "github.com/windsorcli/cli/api/v1alpha1/secrets"
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

// Package level variable for mocking
var checkExistingToolsManager = tools.CheckExistingToolsManager

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
	t.Run("WithDefaultConstructors", func(t *testing.T) {
		// Given a new injector
		injector := di.NewInjector()

		// When creating a new controller without custom constructors
		controller := NewController(injector)

		// Then the controller should not be nil
		if controller == nil {
			t.Fatal("expected controller, got nil")
		}

		// And it should use default constructors
		if controller.constructors.NewYamlConfigHandler == nil {
			t.Error("expected NewYamlConfigHandler constructor, got nil")
		}
		if controller.constructors.NewDefaultShell == nil {
			t.Error("expected NewDefaultShell constructor, got nil")
		}
	})

	t.Run("WithCustomConstructors", func(t *testing.T) {
		// Given a new injector and custom constructors
		injector := di.NewInjector()
		customConstructors := ComponentConstructors{
			NewYamlConfigHandler: func(di.Injector) config.ConfigHandler {
				return config.NewMockConfigHandler()
			},
			NewDefaultShell: func(di.Injector) shell.Shell {
				return shell.NewMockShell()
			},
		}

		// When creating a new controller with custom constructors
		controller := NewController(injector, customConstructors)

		// Then the controller should not be nil
		if controller == nil {
			t.Fatal("expected controller, got nil")
		}

		// And it should use the custom constructors
		configHandler := controller.constructors.NewYamlConfigHandler(injector)
		if _, ok := configHandler.(*config.MockConfigHandler); !ok {
			t.Errorf("expected *config.MockConfigHandler, got %T", configHandler)
		}

		shellInstance := controller.constructors.NewDefaultShell(injector)
		if _, ok := shellInstance.(*shell.MockShell); !ok {
			t.Errorf("expected *shell.MockShell, got %T", shellInstance)
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

	t.Run("WithNilInjector", func(t *testing.T) {
		// Given a controller with nil injector
		controller, _ := setup(t)
		controller.(*BaseController).injector = nil

		// When creating common components
		err := controller.CreateCommonComponents()

		// Then there should be an error
		if err == nil {
			t.Fatal("expected error with nil injector, got nil")
		}
		if !strings.Contains(err.Error(), "injector is nil") {
			t.Errorf("expected error to contain 'injector is nil', got %v", err)
		}
	})

	t.Run("WithNilConfigHandler", func(t *testing.T) {
		// Given a controller with a mock config handler that fails to initialize
		controller, _ := setup(t)
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.InitializeFunc = func() error {
			return fmt.Errorf("config handler initialization error")
		}
		controller.(*BaseController).constructors.NewYamlConfigHandler = func(di.Injector) config.ConfigHandler {
			return mockConfigHandler
		}

		// When creating common components
		err := controller.CreateCommonComponents()

		// Then there should be an error
		if err == nil {
			t.Fatal("expected error with config handler initialization error, got nil")
		}
		if !strings.Contains(err.Error(), "error initializing config handler") {
			t.Errorf("expected error to contain 'error initializing config handler', got %v", err)
		}
	})

	t.Run("WithNilConstructors", func(t *testing.T) {
		// Given a controller with nil constructors
		controller, _ := setup(t)
		controller.(*BaseController).constructors = ComponentConstructors{}

		// When creating common components
		err := controller.CreateCommonComponents()

		// Then there should be an error
		if err == nil {
			t.Fatal("expected error with nil constructors, got nil")
		}
		if !strings.Contains(err.Error(), "required constructors are nil") {
			t.Errorf("expected error to contain 'required constructors are nil', got %v", err)
		}
	})

	t.Run("WithShellInitializationError", func(t *testing.T) {
		// Given a controller with a mock shell that fails to initialize
		controller, _ := setup(t)
		mockShell := shell.NewMockShell()
		mockShell.InitializeFunc = func() error {
			return fmt.Errorf("shell initialization error")
		}
		controller.(*BaseController).constructors.NewDefaultShell = func(di.Injector) shell.Shell {
			return mockShell
		}

		// When creating common components
		err := controller.CreateCommonComponents()

		// Then there should be an error
		if err == nil {
			t.Fatal("expected error with shell initialization error, got nil")
		}
		if !strings.Contains(err.Error(), "error initializing shell") {
			t.Errorf("expected error to contain 'error initializing shell', got %v", err)
		}
	})
}

func TestController_CreateSecretsProviders(t *testing.T) {
	setup := func(t *testing.T, opts *SetupOptions) (Controller, *Mocks) {
		t.Helper()
		mocks := setupMocks(t, opts)
		controller := NewController(mocks.Injector)
		err := controller.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize controller: %v", err)
		}
		return controller, mocks
	}

	t.Run("NoSecretsEnabled", func(t *testing.T) {
		// Given a controller with no secrets configured
		controller, _ := setup(t, &SetupOptions{ConfigStr: `
contexts:
  mock-context:
    secrets:
      enabled: false`})

		// When creating secrets providers
		err := controller.CreateSecretsProviders()

		// Then no error should occur
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("SOPSProviderEnabled", func(t *testing.T) {
		// Given a controller with SOPS secrets file
		tmpDir := t.TempDir()
		secretsFile := filepath.Join(tmpDir, "secrets.enc.yaml")
		if err := os.WriteFile(secretsFile, []byte("test"), 0644); err != nil {
			t.Fatalf("Failed to create test secrets file: %v", err)
		}

		// Create a mock config handler
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigRootFunc = func() (string, error) {
			return tmpDir, nil
		}

		controller, _ := setup(t, &SetupOptions{
			ConfigStr: `
contexts:
  mock-context:
    secrets:
      enabled: true`,
			ConfigHandler: mockConfigHandler,
		})

		// When creating secrets providers
		err := controller.CreateSecretsProviders()

		// Then no error should occur
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("OnePasswordProviderWithSDK", func(t *testing.T) {
		// Given a controller with 1Password vault configured
		controller, _ := setup(t, &SetupOptions{ConfigStr: `
contexts:
  mock-context:
    secrets:
      enabled: true
      onepassword:
        vaults:
          dev:
            title: Development
            url: https://dev.1password.com`})

		// And 1Password SDK token is set
		os.Setenv("OP_SERVICE_ACCOUNT_TOKEN", "test-token")
		defer os.Unsetenv("OP_SERVICE_ACCOUNT_TOKEN")

		// When creating secrets providers
		err := controller.CreateSecretsProviders()

		// Then no error should occur
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("OnePasswordProviderWithCLI", func(t *testing.T) {
		// Given a controller with 1Password vault configured
		controller, _ := setup(t, &SetupOptions{ConfigStr: `
contexts:
  mock-context:
    secrets:
      enabled: true
      onepassword:
        vaults:
          dev:
            title: Development
            url: https://dev.1password.com`})

		// And 1Password SDK token is not set (forcing CLI usage)
		os.Unsetenv("OP_SERVICE_ACCOUNT_TOKEN")

		// When creating secrets providers
		err := controller.CreateSecretsProviders()

		// Then no error should occur
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("ConfigRootError", func(t *testing.T) {
		// Given a controller with a failing config handler
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigRootFunc = func() (string, error) {
			return "", fmt.Errorf("config root error")
		}

		controller, _ := setup(t, &SetupOptions{
			ConfigStr: `
contexts:
  mock-context:
    secrets:
      enabled: true`,
			ConfigHandler: mockConfigHandler,
		})

		// When creating secrets providers
		err := controller.CreateSecretsProviders()

		// Then an error should occur
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "config root error") {
			t.Fatalf("expected error to contain 'config root error', got %v", err)
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

	t.Run("CreatesDefaultComponents", func(t *testing.T) {
		// Given a new controller
		controller, _ := setup(t)

		// When creating project components
		err := controller.CreateProjectComponents()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("WithNilInjector", func(t *testing.T) {
		// Given a controller with nil injector
		controller, _ := setup(t)
		controller.(*BaseController).injector = nil

		// When creating project components
		err := controller.CreateProjectComponents()

		// Then there should be an error
		if err == nil {
			t.Fatal("expected error with nil injector, got nil")
		}
		if !strings.Contains(err.Error(), "injector is nil") {
			t.Errorf("expected error to contain 'injector is nil', got %v", err)
		}
	})

	t.Run("WithNilConfigHandler", func(t *testing.T) {
		// Given a controller with nil config handler
		controller, _ := setup(t)
		controller.(*BaseController).configHandler = nil

		// When creating project components
		err := controller.CreateProjectComponents()

		// Then there should be an error
		if err == nil {
			t.Fatal("expected error with nil config handler, got nil")
		}
		if !strings.Contains(err.Error(), "config handler is nil") {
			t.Errorf("expected error to contain 'config handler is nil', got %v", err)
		}
	})

	t.Run("WithCustomToolsManagerType", func(t *testing.T) {
		// Given a controller with a custom tools manager type
		controller, _ := setup(t)
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "toolsManager" {
				return "custom"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}
		controller.(*BaseController).configHandler = mockConfigHandler

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

	t.Run("CreatesEnvironmentComponents", func(t *testing.T) {
		// Given a new controller
		controller, _ := setup(t)

		// When creating env components
		err := controller.CreateEnvComponents()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("WithNilInjector", func(t *testing.T) {
		// Given a controller with nil injector
		controller, _ := setup(t)
		controller.(*BaseController).injector = nil

		// When creating env components
		err := controller.CreateEnvComponents()

		// Then there should be an error
		if err == nil {
			t.Fatal("expected error with nil injector, got nil")
		}
		if !strings.Contains(err.Error(), "injector is nil") {
			t.Errorf("expected error to contain 'injector is nil', got %v", err)
		}
	})

	t.Run("WithNilConfigHandler", func(t *testing.T) {
		// Given a controller with nil config handler
		controller, _ := setup(t)
		controller.(*BaseController).configHandler = nil

		// When creating env components
		err := controller.CreateEnvComponents()

		// Then there should be an error
		if err == nil {
			t.Fatal("expected error with nil config handler, got nil")
		}
		if !strings.Contains(err.Error(), "config handler is nil") {
			t.Errorf("expected error to contain 'config handler is nil', got %v", err)
		}
	})

	t.Run("WithAwsEnabled", func(t *testing.T) {
		// Given a controller with AWS enabled
		controller, mocks := setup(t)
		mocks.ConfigHandler.Set("contexts.mock-context.aws.enabled", true)

		// When creating env components
		err := controller.CreateEnvComponents()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("WithDockerEnabled", func(t *testing.T) {
		// Given a controller with Docker enabled
		controller, mocks := setup(t)
		mocks.ConfigHandler.Set("contexts.mock-context.docker.enabled", true)

		// When creating env components
		err := controller.CreateEnvComponents()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("WithBothAwsAndDockerEnabled", func(t *testing.T) {
		// Given a controller with both AWS and Docker enabled
		controller, mocks := setup(t)
		mocks.ConfigHandler.Set("contexts.mock-context.aws.enabled", true)
		mocks.ConfigHandler.Set("contexts.mock-context.docker.enabled", true)

		// When creating env components
		err := controller.CreateEnvComponents()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestController_CreateServiceComponents(t *testing.T) {
	setup := func(t *testing.T, configStr string) (Controller, *Mocks) {
		t.Helper()
		mocks := setupMocks(t, &SetupOptions{ConfigStr: configStr})
		controller := NewController(mocks.Injector)
		err := controller.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize controller: %v", err)
		}
		return controller, mocks
	}

	t.Run("NoServicesEnabled", func(t *testing.T) {
		// Given a controller with no services enabled
		controller, _ := setup(t, `
contexts:
  mock-context:
    docker:
      enabled: false`)

		// When creating service components
		err := controller.CreateServiceComponents()

		// Then no error should occur
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("DNSServiceEnabled", func(t *testing.T) {
		// Given a controller with DNS service enabled
		controller, _ := setup(t, `
contexts:
  mock-context:
    docker:
      enabled: true
    dns:
      enabled: true`)

		// When creating service components
		err := controller.CreateServiceComponents()

		// Then no error should occur
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("GitLivereloadEnabled", func(t *testing.T) {
		// Given a controller with Git livereload enabled
		controller, _ := setup(t, `
contexts:
  mock-context:
    docker:
      enabled: true
    git:
      livereload:
        enabled: true`)

		// When creating service components
		err := controller.CreateServiceComponents()

		// Then no error should occur
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("LocalstackEnabled", func(t *testing.T) {
		// Given a controller with Localstack enabled
		controller, _ := setup(t, `
contexts:
  mock-context:
    docker:
      enabled: true
    aws:
      localstack:
        enabled: true`)

		// When creating service components
		err := controller.CreateServiceComponents()

		// Then no error should occur
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("DockerRegistriesEnabled", func(t *testing.T) {
		// Given a controller with Docker registries configured
		controller, _ := setup(t, `
contexts:
  mock-context:
    docker:
      enabled: true
      registries:
        registry1:
          remote: registry1.example.com
        registry2:
          remote: registry2.example.com`)

		// When creating service components
		err := controller.CreateServiceComponents()

		// Then no error should occur
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("ClusterEnabled", func(t *testing.T) {
		// Given a controller with cluster enabled
		controller, _ := setup(t, `
contexts:
  mock-context:
    docker:
      enabled: true
    cluster:
      enabled: true
      driver: talos
      controlplanes:
        count: 3
      workers:
        count: 5`)

		// When creating service components
		err := controller.CreateServiceComponents()

		// Then no error should occur
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

	t.Run("CreatesVirtualizationComponents", func(t *testing.T) {
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

	t.Run("CreatesStackComponents", func(t *testing.T) {
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

	t.Run("WritesConfigurationFiles", func(t *testing.T) {
		// Given a new controller
		controller, _ := setup(t)

		// When writing configuration files
		err := controller.WriteConfigurationFiles()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("FailsWritingToolsManifest", func(t *testing.T) {
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

func TestDefaultConstructors(t *testing.T) {
	t.Run("ReturnsAllRequiredConstructors", func(t *testing.T) {
		// Given a new set of default constructors
		constructors := DefaultConstructors()

		// When checking each constructor field
		// Then each constructor should be non-nil
		if constructors.NewYamlConfigHandler == nil {
			t.Error("NewYamlConfigHandler constructor is nil")
		}
		if constructors.NewDefaultShell == nil {
			t.Error("NewDefaultShell constructor is nil")
		}
		if constructors.NewSecureShell == nil {
			t.Error("NewSecureShell constructor is nil")
		}
		if constructors.NewGitGenerator == nil {
			t.Error("NewGitGenerator constructor is nil")
		}
		if constructors.NewBlueprintHandler == nil {
			t.Error("NewBlueprintHandler constructor is nil")
		}
		if constructors.NewTerraformGenerator == nil {
			t.Error("NewTerraformGenerator constructor is nil")
		}
		if constructors.NewKustomizeGenerator == nil {
			t.Error("NewKustomizeGenerator constructor is nil")
		}
		if constructors.NewToolsManager == nil {
			t.Error("NewToolsManager constructor is nil")
		}
		if constructors.NewDNSService == nil {
			t.Error("NewDNSService constructor is nil")
		}
		if constructors.NewGitLivereloadService == nil {
			t.Error("NewGitLivereloadService constructor is nil")
		}
		if constructors.NewLocalstackService == nil {
			t.Error("NewLocalstackService constructor is nil")
		}
		if constructors.NewRegistryService == nil {
			t.Error("NewRegistryService constructor is nil")
		}
		if constructors.NewTalosService == nil {
			t.Error("NewTalosService constructor is nil")
		}
		if constructors.NewSSHClient == nil {
			t.Error("NewSSHClient constructor is nil")
		}
		if constructors.NewColimaVirt == nil {
			t.Error("NewColimaVirt constructor is nil")
		}
		if constructors.NewColimaNetworkManager == nil {
			t.Error("NewColimaNetworkManager constructor is nil")
		}
		if constructors.NewBaseNetworkManager == nil {
			t.Error("NewBaseNetworkManager constructor is nil")
		}
		if constructors.NewDockerVirt == nil {
			t.Error("NewDockerVirt constructor is nil")
		}
		if constructors.NewNetworkInterfaceProvider == nil {
			t.Error("NewNetworkInterfaceProvider constructor is nil")
		}
		if constructors.NewSopsSecretsProvider == nil {
			t.Error("NewSopsSecretsProvider constructor is nil")
		}
		if constructors.NewOnePasswordSDKSecretsProvider == nil {
			t.Error("NewOnePasswordSDKSecretsProvider constructor is nil")
		}
		if constructors.NewOnePasswordCLISecretsProvider == nil {
			t.Error("NewOnePasswordCLISecretsProvider constructor is nil")
		}
		if constructors.NewWindsorStack == nil {
			t.Error("NewWindsorStack constructor is nil")
		}
	})

	t.Run("CreatesCorrectConcreteTypes", func(t *testing.T) {
		// Given a new injector and constructors
		injector := di.NewInjector()
		constructors := DefaultConstructors()

		// When creating components
		configHandler := constructors.NewYamlConfigHandler(injector)
		defaultShell := constructors.NewDefaultShell(injector)
		secureShell := constructors.NewSecureShell(injector)
		gitGenerator := constructors.NewGitGenerator(injector)
		blueprintHandler := constructors.NewBlueprintHandler(injector)
		terraformGenerator := constructors.NewTerraformGenerator(injector)
		kustomizeGenerator := constructors.NewKustomizeGenerator(injector)
		toolsManager := constructors.NewToolsManager(injector)
		dnsService := constructors.NewDNSService(injector)
		gitLivereloadService := constructors.NewGitLivereloadService(injector)
		localstackService := constructors.NewLocalstackService(injector)
		registryService := constructors.NewRegistryService(injector)
		talosService := constructors.NewTalosService(injector, "controlplane")
		sshClient := constructors.NewSSHClient()
		colimaVirt := constructors.NewColimaVirt(injector)
		colimaNetworkManager := constructors.NewColimaNetworkManager(injector)
		baseNetworkManager := constructors.NewBaseNetworkManager(injector)
		dockerVirt := constructors.NewDockerVirt(injector)
		networkInterfaceProvider := constructors.NewNetworkInterfaceProvider()
		sopsSecretsProvider := constructors.NewSopsSecretsProvider("test.yaml", injector)
		onePasswordSDKSecretsProvider := constructors.NewOnePasswordSDKSecretsProvider(secretsConfigType.OnePasswordVault{}, injector)
		onePasswordCLISecretsProvider := constructors.NewOnePasswordCLISecretsProvider(secretsConfigType.OnePasswordVault{}, injector)
		windsorStack := constructors.NewWindsorStack(injector)

		// Then they should be of the correct concrete type
		if _, ok := configHandler.(*config.YamlConfigHandler); !ok {
			t.Error("NewYamlConfigHandler did not create YamlConfigHandler")
		}
		if _, ok := defaultShell.(*shell.DefaultShell); !ok {
			t.Error("NewDefaultShell did not create DefaultShell")
		}
		if _, ok := secureShell.(*shell.SecureShell); !ok {
			t.Error("NewSecureShell did not create SecureShell")
		}
		if _, ok := gitGenerator.(*generators.GitGenerator); !ok {
			t.Error("NewGitGenerator did not create GitGenerator")
		}
		if _, ok := blueprintHandler.(blueprint.BlueprintHandler); !ok {
			t.Error("NewBlueprintHandler did not create BlueprintHandler")
		}
		if _, ok := terraformGenerator.(*generators.TerraformGenerator); !ok {
			t.Error("NewTerraformGenerator did not create TerraformGenerator")
		}
		if _, ok := kustomizeGenerator.(*generators.KustomizeGenerator); !ok {
			t.Error("NewKustomizeGenerator did not create KustomizeGenerator")
		}
		if _, ok := toolsManager.(tools.ToolsManager); !ok {
			t.Error("NewToolsManager did not create ToolsManager")
		}
		if _, ok := dnsService.(*services.DNSService); !ok {
			t.Error("NewDNSService did not create DNSService")
		}
		if _, ok := gitLivereloadService.(*services.GitLivereloadService); !ok {
			t.Error("NewGitLivereloadService did not create GitLivereloadService")
		}
		if _, ok := localstackService.(*services.LocalstackService); !ok {
			t.Error("NewLocalstackService did not create LocalstackService")
		}
		if _, ok := registryService.(*services.RegistryService); !ok {
			t.Error("NewRegistryService did not create RegistryService")
		}
		if _, ok := talosService.(*services.TalosService); !ok {
			t.Error("NewTalosService did not create TalosService")
		}
		if sshClient == nil {
			t.Error("NewSSHClient did not create SSHClient")
		}
		if _, ok := colimaVirt.(*virt.ColimaVirt); !ok {
			t.Error("NewColimaVirt did not create ColimaVirt")
		}
		if _, ok := colimaNetworkManager.(*network.ColimaNetworkManager); !ok {
			t.Error("NewColimaNetworkManager did not create ColimaNetworkManager")
		}
		if _, ok := baseNetworkManager.(*network.BaseNetworkManager); !ok {
			t.Error("NewBaseNetworkManager did not create BaseNetworkManager")
		}
		if _, ok := dockerVirt.(*virt.DockerVirt); !ok {
			t.Error("NewDockerVirt did not create DockerVirt")
		}
		if _, ok := networkInterfaceProvider.(network.NetworkInterfaceProvider); !ok {
			t.Error("NewNetworkInterfaceProvider did not create NetworkInterfaceProvider")
		}
		if _, ok := sopsSecretsProvider.(*secrets.SopsSecretsProvider); !ok {
			t.Error("NewSopsSecretsProvider did not create SopsSecretsProvider")
		}
		if _, ok := onePasswordSDKSecretsProvider.(*secrets.OnePasswordSDKSecretsProvider); !ok {
			t.Error("NewOnePasswordSDKSecretsProvider did not create OnePasswordSDKSecretsProvider")
		}
		if _, ok := onePasswordCLISecretsProvider.(*secrets.OnePasswordCLISecretsProvider); !ok {
			t.Error("NewOnePasswordCLISecretsProvider did not create OnePasswordCLISecretsProvider")
		}
		if _, ok := windsorStack.(*stack.WindsorStack); !ok {
			t.Error("NewWindsorStack did not create WindsorStack")
		}
	})
}

func TestMockConstructors(t *testing.T) {
	t.Run("ReturnsAllRequiredMockConstructors", func(t *testing.T) {
		// Given a new set of mock constructors
		constructors := MockConstructors()

		// When checking each constructor field
		// Then each constructor should be non-nil
		if constructors.NewYamlConfigHandler == nil {
			t.Error("NewYamlConfigHandler mock constructor is nil")
		}
		if constructors.NewDefaultShell == nil {
			t.Error("NewDefaultShell mock constructor is nil")
		}
		if constructors.NewSecureShell == nil {
			t.Error("NewSecureShell mock constructor is nil")
		}
		if constructors.NewGitGenerator == nil {
			t.Error("NewGitGenerator mock constructor is nil")
		}
		if constructors.NewBlueprintHandler == nil {
			t.Error("NewBlueprintHandler mock constructor is nil")
		}
		if constructors.NewTerraformGenerator == nil {
			t.Error("NewTerraformGenerator mock constructor is nil")
		}
		if constructors.NewKustomizeGenerator == nil {
			t.Error("NewKustomizeGenerator mock constructor is nil")
		}
		if constructors.NewToolsManager == nil {
			t.Error("NewToolsManager mock constructor is nil")
		}
		if constructors.NewAwsEnvPrinter == nil {
			t.Error("NewAwsEnvPrinter mock constructor is nil")
		}
		if constructors.NewDockerEnvPrinter == nil {
			t.Error("NewDockerEnvPrinter mock constructor is nil")
		}
		if constructors.NewKubeEnvPrinter == nil {
			t.Error("NewKubeEnvPrinter mock constructor is nil")
		}
		if constructors.NewOmniEnvPrinter == nil {
			t.Error("NewOmniEnvPrinter mock constructor is nil")
		}
		if constructors.NewTalosEnvPrinter == nil {
			t.Error("NewTalosEnvPrinter mock constructor is nil")
		}
		if constructors.NewTerraformEnvPrinter == nil {
			t.Error("NewTerraformEnvPrinter mock constructor is nil")
		}
		if constructors.NewWindsorEnvPrinter == nil {
			t.Error("NewWindsorEnvPrinter mock constructor is nil")
		}
		if constructors.NewDNSService == nil {
			t.Error("NewDNSService mock constructor is nil")
		}
		if constructors.NewGitLivereloadService == nil {
			t.Error("NewGitLivereloadService mock constructor is nil")
		}
		if constructors.NewLocalstackService == nil {
			t.Error("NewLocalstackService mock constructor is nil")
		}
		if constructors.NewRegistryService == nil {
			t.Error("NewRegistryService mock constructor is nil")
		}
		if constructors.NewTalosService == nil {
			t.Error("NewTalosService mock constructor is nil")
		}
		if constructors.NewSSHClient == nil {
			t.Error("NewSSHClient mock constructor is nil")
		}
		if constructors.NewColimaVirt == nil {
			t.Error("NewColimaVirt mock constructor is nil")
		}
		if constructors.NewColimaNetworkManager == nil {
			t.Error("NewColimaNetworkManager mock constructor is nil")
		}
		if constructors.NewBaseNetworkManager == nil {
			t.Error("NewBaseNetworkManager mock constructor is nil")
		}
		if constructors.NewDockerVirt == nil {
			t.Error("NewDockerVirt mock constructor is nil")
		}
		if constructors.NewNetworkInterfaceProvider == nil {
			t.Error("NewNetworkInterfaceProvider mock constructor is nil")
		}
		if constructors.NewSopsSecretsProvider == nil {
			t.Error("NewSopsSecretsProvider mock constructor is nil")
		}
		if constructors.NewOnePasswordSDKSecretsProvider == nil {
			t.Error("NewOnePasswordSDKSecretsProvider mock constructor is nil")
		}
		if constructors.NewOnePasswordCLISecretsProvider == nil {
			t.Error("NewOnePasswordCLISecretsProvider mock constructor is nil")
		}
		if constructors.NewWindsorStack == nil {
			t.Error("NewWindsorStack mock constructor is nil")
		}
	})

	t.Run("CreatesCorrectMockTypes", func(t *testing.T) {
		// Given a new injector and mock constructors
		injector := di.NewInjector()
		constructors := MockConstructors()

		// When creating components
		configHandler := constructors.NewYamlConfigHandler(injector)
		defaultShell := constructors.NewDefaultShell(injector)
		secureShell := constructors.NewSecureShell(injector)
		gitGenerator := constructors.NewGitGenerator(injector)
		blueprintHandler := constructors.NewBlueprintHandler(injector)
		terraformGenerator := constructors.NewTerraformGenerator(injector)
		kustomizeGenerator := constructors.NewKustomizeGenerator(injector)
		toolsManager := constructors.NewToolsManager(injector)
		awsEnvPrinter := constructors.NewAwsEnvPrinter(injector)
		dockerEnvPrinter := constructors.NewDockerEnvPrinter(injector)
		kubeEnvPrinter := constructors.NewKubeEnvPrinter(injector)
		omniEnvPrinter := constructors.NewOmniEnvPrinter(injector)
		talosEnvPrinter := constructors.NewTalosEnvPrinter(injector)
		terraformEnvPrinter := constructors.NewTerraformEnvPrinter(injector)
		windsorEnvPrinter := constructors.NewWindsorEnvPrinter(injector)
		dnsService := constructors.NewDNSService(injector)
		gitLivereloadService := constructors.NewGitLivereloadService(injector)
		localstackService := constructors.NewLocalstackService(injector)
		registryService := constructors.NewRegistryService(injector)
		talosService := constructors.NewTalosService(injector, "controlplane")
		sshClient := constructors.NewSSHClient()
		colimaVirt := constructors.NewColimaVirt(injector)
		colimaNetworkManager := constructors.NewColimaNetworkManager(injector)
		baseNetworkManager := constructors.NewBaseNetworkManager(injector)
		dockerVirt := constructors.NewDockerVirt(injector)
		networkInterfaceProvider := constructors.NewNetworkInterfaceProvider()
		sopsSecretsProvider := constructors.NewSopsSecretsProvider("test.yaml", injector)
		onePasswordSDKSecretsProvider := constructors.NewOnePasswordSDKSecretsProvider(secretsConfigType.OnePasswordVault{}, injector)
		onePasswordCLISecretsProvider := constructors.NewOnePasswordCLISecretsProvider(secretsConfigType.OnePasswordVault{}, injector)
		windsorStack := constructors.NewWindsorStack(injector)

		// Then they should be of the correct mock type
		if _, ok := configHandler.(*config.MockConfigHandler); !ok {
			t.Error("NewYamlConfigHandler did not create MockConfigHandler")
		}
		if _, ok := defaultShell.(*shell.MockShell); !ok {
			t.Error("NewDefaultShell did not create MockShell")
		}
		if _, ok := secureShell.(*shell.MockShell); !ok {
			t.Error("NewSecureShell did not create MockShell")
		}
		if _, ok := gitGenerator.(*generators.MockGenerator); !ok {
			t.Error("NewGitGenerator did not create MockGenerator")
		}
		if _, ok := blueprintHandler.(*blueprint.MockBlueprintHandler); !ok {
			t.Error("NewBlueprintHandler did not create MockBlueprintHandler")
		}
		if _, ok := terraformGenerator.(*generators.MockGenerator); !ok {
			t.Error("NewTerraformGenerator did not create MockGenerator")
		}
		if _, ok := kustomizeGenerator.(*generators.MockGenerator); !ok {
			t.Error("NewKustomizeGenerator did not create MockGenerator")
		}
		if _, ok := toolsManager.(*tools.MockToolsManager); !ok {
			t.Error("NewToolsManager did not create MockToolsManager")
		}
		if _, ok := awsEnvPrinter.(*env.MockEnvPrinter); !ok {
			t.Error("NewAwsEnvPrinter did not create MockEnvPrinter")
		}
		if _, ok := dockerEnvPrinter.(*env.MockEnvPrinter); !ok {
			t.Error("NewDockerEnvPrinter did not create MockEnvPrinter")
		}
		if _, ok := kubeEnvPrinter.(*env.MockEnvPrinter); !ok {
			t.Error("NewKubeEnvPrinter did not create MockEnvPrinter")
		}
		if _, ok := omniEnvPrinter.(*env.MockEnvPrinter); !ok {
			t.Error("NewOmniEnvPrinter did not create MockEnvPrinter")
		}
		if _, ok := talosEnvPrinter.(*env.MockEnvPrinter); !ok {
			t.Error("NewTalosEnvPrinter did not create MockEnvPrinter")
		}
		if _, ok := terraformEnvPrinter.(*env.MockEnvPrinter); !ok {
			t.Error("NewTerraformEnvPrinter did not create MockEnvPrinter")
		}
		if _, ok := windsorEnvPrinter.(*env.MockEnvPrinter); !ok {
			t.Error("NewWindsorEnvPrinter did not create MockEnvPrinter")
		}
		if _, ok := dnsService.(*services.MockService); !ok {
			t.Error("NewDNSService did not create MockService")
		}
		if _, ok := gitLivereloadService.(*services.MockService); !ok {
			t.Error("NewGitLivereloadService did not create MockService")
		}
		if _, ok := localstackService.(*services.MockService); !ok {
			t.Error("NewLocalstackService did not create MockService")
		}
		if _, ok := registryService.(*services.MockService); !ok {
			t.Error("NewRegistryService did not create MockService")
		}
		if _, ok := talosService.(*services.MockService); !ok {
			t.Error("NewTalosService did not create MockService")
		}
		if sshClient == nil {
			t.Error("NewSSHClient did not create SSHClient")
		}
		if _, ok := colimaVirt.(*virt.MockVirt); !ok {
			t.Error("NewColimaVirt did not create MockVirt")
		}
		if _, ok := colimaNetworkManager.(*network.MockNetworkManager); !ok {
			t.Error("NewColimaNetworkManager did not create MockNetworkManager")
		}
		if _, ok := baseNetworkManager.(*network.MockNetworkManager); !ok {
			t.Error("NewBaseNetworkManager did not create MockNetworkManager")
		}
		if _, ok := dockerVirt.(*virt.MockVirt); !ok {
			t.Error("NewDockerVirt did not create MockVirt")
		}
		if _, ok := networkInterfaceProvider.(*network.MockNetworkInterfaceProvider); !ok {
			t.Error("NewNetworkInterfaceProvider did not create MockNetworkInterfaceProvider")
		}
		if _, ok := sopsSecretsProvider.(*secrets.MockSecretsProvider); !ok {
			t.Error("NewSopsSecretsProvider did not create MockSecretsProvider")
		}
		if _, ok := onePasswordSDKSecretsProvider.(*secrets.MockSecretsProvider); !ok {
			t.Error("NewOnePasswordSDKSecretsProvider did not create MockSecretsProvider")
		}
		if _, ok := onePasswordCLISecretsProvider.(*secrets.MockSecretsProvider); !ok {
			t.Error("NewOnePasswordCLISecretsProvider did not create MockSecretsProvider")
		}
		if _, ok := windsorStack.(*stack.MockStack); !ok {
			t.Error("NewWindsorStack did not create MockStack")
		}
	})
}

func TestMockConstructors_EnvPrinters(t *testing.T) {
	t.Run("AwsEnvPrinter", func(t *testing.T) {
		// Given a new injector and mock constructors
		injector := di.NewInjector()
		constructors := MockConstructors()

		// When creating an AWS environment printer
		awsEnvPrinter := constructors.NewAwsEnvPrinter(injector)

		// Then it should not be nil
		if awsEnvPrinter == nil {
			t.Fatal("expected non-nil AwsEnvPrinter")
		}

		// And it should be of the correct mock type
		if _, ok := awsEnvPrinter.(*env.MockEnvPrinter); !ok {
			t.Errorf("expected *env.MockEnvPrinter, got %T", awsEnvPrinter)
		}

		// And it should implement the EnvPrinter interface
		var _ env.EnvPrinter = awsEnvPrinter
	})

	t.Run("DockerEnvPrinter", func(t *testing.T) {
		// Given a new injector and mock constructors
		injector := di.NewInjector()
		constructors := MockConstructors()

		// When creating a Docker environment printer
		dockerEnvPrinter := constructors.NewDockerEnvPrinter(injector)

		// Then it should not be nil
		if dockerEnvPrinter == nil {
			t.Fatal("expected non-nil DockerEnvPrinter")
		}

		// And it should be of the correct mock type
		if _, ok := dockerEnvPrinter.(*env.MockEnvPrinter); !ok {
			t.Errorf("expected *env.MockEnvPrinter, got %T", dockerEnvPrinter)
		}

		// And it should implement the EnvPrinter interface
		var _ env.EnvPrinter = dockerEnvPrinter
	})

	t.Run("KubeEnvPrinter", func(t *testing.T) {
		// Given a new injector and mock constructors
		injector := di.NewInjector()
		constructors := MockConstructors()

		// When creating a Kubernetes environment printer
		kubeEnvPrinter := constructors.NewKubeEnvPrinter(injector)

		// Then it should not be nil
		if kubeEnvPrinter == nil {
			t.Fatal("expected non-nil KubeEnvPrinter")
		}

		// And it should be of the correct mock type
		if _, ok := kubeEnvPrinter.(*env.MockEnvPrinter); !ok {
			t.Errorf("expected *env.MockEnvPrinter, got %T", kubeEnvPrinter)
		}

		// And it should implement the EnvPrinter interface
		var _ env.EnvPrinter = kubeEnvPrinter
	})

	t.Run("OmniEnvPrinter", func(t *testing.T) {
		// Given a new injector and mock constructors
		injector := di.NewInjector()
		constructors := MockConstructors()

		// When creating an Omni environment printer
		omniEnvPrinter := constructors.NewOmniEnvPrinter(injector)

		// Then it should not be nil
		if omniEnvPrinter == nil {
			t.Fatal("expected non-nil OmniEnvPrinter")
		}

		// And it should be of the correct mock type
		if _, ok := omniEnvPrinter.(*env.MockEnvPrinter); !ok {
			t.Errorf("expected *env.MockEnvPrinter, got %T", omniEnvPrinter)
		}

		// And it should implement the EnvPrinter interface
		var _ env.EnvPrinter = omniEnvPrinter
	})

	t.Run("TalosEnvPrinter", func(t *testing.T) {
		// Given a new injector and mock constructors
		injector := di.NewInjector()
		constructors := MockConstructors()

		// When creating a Talos environment printer
		talosEnvPrinter := constructors.NewTalosEnvPrinter(injector)

		// Then it should not be nil
		if talosEnvPrinter == nil {
			t.Fatal("expected non-nil TalosEnvPrinter")
		}

		// And it should be of the correct mock type
		if _, ok := talosEnvPrinter.(*env.MockEnvPrinter); !ok {
			t.Errorf("expected *env.MockEnvPrinter, got %T", talosEnvPrinter)
		}

		// And it should implement the EnvPrinter interface
		var _ env.EnvPrinter = talosEnvPrinter
	})

	t.Run("TerraformEnvPrinter", func(t *testing.T) {
		// Given a new injector and mock constructors
		injector := di.NewInjector()
		constructors := MockConstructors()

		// When creating a Terraform environment printer
		terraformEnvPrinter := constructors.NewTerraformEnvPrinter(injector)

		// Then it should not be nil
		if terraformEnvPrinter == nil {
			t.Fatal("expected non-nil TerraformEnvPrinter")
		}

		// And it should be of the correct mock type
		if _, ok := terraformEnvPrinter.(*env.MockEnvPrinter); !ok {
			t.Errorf("expected *env.MockEnvPrinter, got %T", terraformEnvPrinter)
		}

		// And it should implement the EnvPrinter interface
		var _ env.EnvPrinter = terraformEnvPrinter
	})

	t.Run("WindsorEnvPrinter", func(t *testing.T) {
		// Given a new injector and mock constructors
		injector := di.NewInjector()
		constructors := MockConstructors()

		// When creating a Windsor environment printer
		windsorEnvPrinter := constructors.NewWindsorEnvPrinter(injector)

		// Then it should not be nil
		if windsorEnvPrinter == nil {
			t.Fatal("expected non-nil WindsorEnvPrinter")
		}

		// And it should be of the correct mock type
		if _, ok := windsorEnvPrinter.(*env.MockEnvPrinter); !ok {
			t.Errorf("expected *env.MockEnvPrinter, got %T", windsorEnvPrinter)
		}

		// And it should implement the EnvPrinter interface
		var _ env.EnvPrinter = windsorEnvPrinter
	})
}
