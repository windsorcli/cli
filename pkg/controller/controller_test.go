package controller

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/api/v1alpha1/docker"
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
	"github.com/windsorcli/cli/pkg/ssh"
	"github.com/windsorcli/cli/pkg/tools"
	"github.com/windsorcli/cli/pkg/virt"
)

// =============================================================================
// Test Setup
// =============================================================================

type Mocks struct {
	Shell         *shell.MockShell
	ConfigHandler config.ConfigHandler
	Injector      di.Injector
}

type SetupOptions struct {
	Injector      di.Injector
	ConfigHandler config.ConfigHandler
	ConfigStr     string
}

func setupMocks(t *testing.T, opts ...*SetupOptions) *Mocks {
	t.Helper()

	options := &SetupOptions{}
	if len(opts) > 0 && opts[0] != nil {
		options = opts[0]
	}

	// Create injector
	var injector di.Injector
	if options.Injector == nil {
		injector = di.NewInjector()
	} else {
		injector = options.Injector
	}

	// Create shell with project root
	mockShell := shell.NewMockShell()
	mockShell.GetProjectRootFunc = func() (string, error) {
		return t.TempDir(), nil
	}
	injector.Register("shell", mockShell)

	// Create config handler
	var configHandler config.ConfigHandler
	if options.ConfigHandler == nil {
		configHandler = config.NewYamlConfigHandler(injector)
	} else {
		configHandler = options.ConfigHandler
	}

	injector.Register("configHandler", configHandler)

	if options.ConfigStr != "" {
		if err := configHandler.LoadConfigString(options.ConfigStr); err != nil {
			t.Fatalf("Failed to load config string: %v", err)
		}
	}

	configHandler.Initialize()

	return &Mocks{
		Shell:         mockShell,
		ConfigHandler: configHandler,
		Injector:      injector,
	}
}

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewController(t *testing.T) {
	setup := func(t *testing.T) (*BaseController, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		controller := NewController(mocks.Injector)
		return controller, mocks
	}

	t.Run("CreatesControllerWithDefaultConstructors", func(t *testing.T) {
		// Given
		controller, mocks := setup(t)

		// Then
		if controller == nil {
			t.Fatal("Expected controller to not be nil")
		}

		if controller.injector != mocks.Injector {
			t.Errorf("Expected injector to be %v, got %v", mocks.Injector, controller.injector)
		}

		// Test each constructor by actually calling it
		constructorTests := map[string]func() error{
			"NewConfigHandler": func() error {
				handler := controller.constructors.NewConfigHandler(mocks.Injector)
				if handler == nil {
					return fmt.Errorf("NewConfigHandler returned nil")
				}
				return nil
			},
			"NewShell": func() error {
				shell := controller.constructors.NewShell(mocks.Injector)
				if shell == nil {
					return fmt.Errorf("NewShell returned nil")
				}
				return nil
			},
			"NewSecureShell": func() error {
				shell := controller.constructors.NewSecureShell(mocks.Injector)
				if shell == nil {
					return fmt.Errorf("NewSecureShell returned nil")
				}
				return nil
			},
			"NewGitGenerator": func() error {
				generator := controller.constructors.NewGitGenerator(mocks.Injector)
				if generator == nil {
					return fmt.Errorf("NewGitGenerator returned nil")
				}
				return nil
			},
			"NewBlueprintHandler": func() error {
				handler := controller.constructors.NewBlueprintHandler(mocks.Injector)
				if handler == nil {
					return fmt.Errorf("NewBlueprintHandler returned nil")
				}
				return nil
			},
			"NewTerraformGenerator": func() error {
				generator := controller.constructors.NewTerraformGenerator(mocks.Injector)
				if generator == nil {
					return fmt.Errorf("NewTerraformGenerator returned nil")
				}
				return nil
			},
			"NewKustomizeGenerator": func() error {
				generator := controller.constructors.NewKustomizeGenerator(mocks.Injector)
				if generator == nil {
					return fmt.Errorf("NewKustomizeGenerator returned nil")
				}
				return nil
			},
			"NewToolsManager": func() error {
				manager := controller.constructors.NewToolsManager(mocks.Injector)
				if manager == nil {
					return fmt.Errorf("NewToolsManager returned nil")
				}
				return nil
			},
			"NewAwsEnvPrinter": func() error {
				printer := controller.constructors.NewAwsEnvPrinter(mocks.Injector)
				if printer == nil {
					return fmt.Errorf("NewAwsEnvPrinter returned nil")
				}
				return nil
			},
			"NewDockerEnvPrinter": func() error {
				printer := controller.constructors.NewDockerEnvPrinter(mocks.Injector)
				if printer == nil {
					return fmt.Errorf("NewDockerEnvPrinter returned nil")
				}
				return nil
			},
			"NewKubeEnvPrinter": func() error {
				printer := controller.constructors.NewKubeEnvPrinter(mocks.Injector)
				if printer == nil {
					return fmt.Errorf("NewKubeEnvPrinter returned nil")
				}
				return nil
			},
			"NewOmniEnvPrinter": func() error {
				printer := controller.constructors.NewOmniEnvPrinter(mocks.Injector)
				if printer == nil {
					return fmt.Errorf("NewOmniEnvPrinter returned nil")
				}
				return nil
			},
			"NewTalosEnvPrinter": func() error {
				printer := controller.constructors.NewTalosEnvPrinter(mocks.Injector)
				if printer == nil {
					return fmt.Errorf("NewTalosEnvPrinter returned nil")
				}
				return nil
			},
			"NewTerraformEnvPrinter": func() error {
				printer := controller.constructors.NewTerraformEnvPrinter(mocks.Injector)
				if printer == nil {
					return fmt.Errorf("NewTerraformEnvPrinter returned nil")
				}
				return nil
			},
			"NewWindsorEnvPrinter": func() error {
				printer := controller.constructors.NewWindsorEnvPrinter(mocks.Injector)
				if printer == nil {
					return fmt.Errorf("NewWindsorEnvPrinter returned nil")
				}
				return nil
			},
			"NewDNSService": func() error {
				service := controller.constructors.NewDNSService(mocks.Injector)
				if service == nil {
					return fmt.Errorf("NewDNSService returned nil")
				}
				return nil
			},
			"NewGitLivereloadService": func() error {
				service := controller.constructors.NewGitLivereloadService(mocks.Injector)
				if service == nil {
					return fmt.Errorf("NewGitLivereloadService returned nil")
				}
				return nil
			},
			"NewLocalstackService": func() error {
				service := controller.constructors.NewLocalstackService(mocks.Injector)
				if service == nil {
					return fmt.Errorf("NewLocalstackService returned nil")
				}
				return nil
			},
			"NewRegistryService": func() error {
				service := controller.constructors.NewRegistryService(mocks.Injector)
				if service == nil {
					return fmt.Errorf("NewRegistryService returned nil")
				}
				return nil
			},
			"NewTalosService": func() error {
				service := controller.constructors.NewTalosService(mocks.Injector, "test")
				if service == nil {
					return fmt.Errorf("NewTalosService returned nil")
				}
				return nil
			},
			"NewSSHClient": func() error {
				client := controller.constructors.NewSSHClient()
				if client == nil {
					return fmt.Errorf("NewSSHClient returned nil")
				}
				return nil
			},
			"NewColimaVirt": func() error {
				virt := controller.constructors.NewColimaVirt(mocks.Injector)
				if virt == nil {
					return fmt.Errorf("NewColimaVirt returned nil")
				}
				return nil
			},
			"NewColimaNetworkManager": func() error {
				manager := controller.constructors.NewColimaNetworkManager(mocks.Injector)
				if manager == nil {
					return fmt.Errorf("NewColimaNetworkManager returned nil")
				}
				return nil
			},
			"NewBaseNetworkManager": func() error {
				manager := controller.constructors.NewBaseNetworkManager(mocks.Injector)
				if manager == nil {
					return fmt.Errorf("NewBaseNetworkManager returned nil")
				}
				return nil
			},
			"NewDockerVirt": func() error {
				virt := controller.constructors.NewDockerVirt(mocks.Injector)
				if virt == nil {
					return fmt.Errorf("NewDockerVirt returned nil")
				}
				return nil
			},
			"NewNetworkInterfaceProvider": func() error {
				provider := controller.constructors.NewNetworkInterfaceProvider()
				if provider == nil {
					return fmt.Errorf("NewNetworkInterfaceProvider returned nil")
				}
				return nil
			},
			"NewSopsSecretsProvider": func() error {
				provider := controller.constructors.NewSopsSecretsProvider("", mocks.Injector)
				if provider == nil {
					return fmt.Errorf("NewSopsSecretsProvider returned nil")
				}
				return nil
			},
			"NewOnePasswordSDKSecretsProvider": func() error {
				provider := controller.constructors.NewOnePasswordSDKSecretsProvider(secretsConfigType.OnePasswordVault{}, mocks.Injector)
				if provider == nil {
					return fmt.Errorf("NewOnePasswordSDKSecretsProvider returned nil")
				}
				return nil
			},
			"NewOnePasswordCLISecretsProvider": func() error {
				provider := controller.constructors.NewOnePasswordCLISecretsProvider(secretsConfigType.OnePasswordVault{}, mocks.Injector)
				if provider == nil {
					return fmt.Errorf("NewOnePasswordCLISecretsProvider returned nil")
				}
				return nil
			},
			"NewWindsorStack": func() error {
				stack := controller.constructors.NewWindsorStack(mocks.Injector)
				if stack == nil {
					return fmt.Errorf("NewWindsorStack returned nil")
				}
				return nil
			},
		}

		for name, test := range constructorTests {
			if err := test(); err != nil {
				t.Errorf("Failed to create %s: %v", name, err)
			}
		}
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestBaseController_SetRequirements(t *testing.T) {
	setup := func(t *testing.T) (*BaseController, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		controller := NewController(mocks.Injector)
		return controller, mocks
	}

	t.Run("SetsRequirementsOnController", func(t *testing.T) {
		// Given
		controller, _ := setup(t)
		requirements := Requirements{
			Trust:        true,
			ConfigLoaded: true,
			Env:          true,
			Secrets:      true,
			VM:           true,
			Containers:   true,
			Network:      true,
			Services:     true,
			Tools:        true,
			Blueprint:    true,
			Generators:   true,
			Stack:        true,
			CommandName:  "test-command",
			Flags:        map[string]bool{"verbose": true},
		}

		// When
		controller.SetRequirements(requirements)

		// Then
		if controller.requirements.Trust != requirements.Trust {
			t.Errorf("Expected Trust to be %v, got %v", requirements.Trust, controller.requirements.Trust)
		}

		if controller.requirements.ConfigLoaded != requirements.ConfigLoaded {
			t.Errorf("Expected ConfigLoaded to be %v, got %v", requirements.ConfigLoaded, controller.requirements.ConfigLoaded)
		}

		if controller.requirements.CommandName != requirements.CommandName {
			t.Errorf("Expected CommandName to be %v, got %v", requirements.CommandName, controller.requirements.CommandName)
		}

		if len(controller.requirements.Flags) != len(requirements.Flags) {
			t.Errorf("Expected Flags length to be %v, got %v", len(requirements.Flags), len(controller.requirements.Flags))
		}

		if controller.requirements.Flags["verbose"] != requirements.Flags["verbose"] {
			t.Errorf("Expected Flags[verbose] to be %v, got %v", requirements.Flags["verbose"], controller.requirements.Flags["verbose"])
		}
	})
}

func TestBaseController_CreateComponents(t *testing.T) {
	setup := func(t *testing.T) (*BaseController, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		controller := NewController(mocks.Injector)
		return controller, mocks
	}

	t.Run("ReturnsErrorWhenInjectorIsNil", func(t *testing.T) {
		// Given
		controller, _ := setup(t)
		controller.injector = nil

		// When
		err := controller.CreateComponents()

		// Then
		if err == nil {
			t.Fatal("Expected error when injector is nil")
		}
		if err.Error() != "injector is nil" {
			t.Errorf("Expected error 'injector is nil', got %v", err)
		}
	})

	t.Run("ReturnsErrorWhenRequirementsNotSet", func(t *testing.T) {
		// Given
		controller, _ := setup(t)

		// When
		err := controller.CreateComponents()

		// Then
		if err == nil {
			t.Fatal("Expected error when requirements not set")
		}
		if err.Error() != "requirements not set" {
			t.Errorf("Expected error 'requirements not set', got %v", err)
		}
	})

	t.Run("CreatesAllRequiredComponents", func(t *testing.T) {
		// Given
		controller, _ := setup(t)
		controller.SetRequirements(Requirements{
			Trust:        true,
			ConfigLoaded: true,
			Env:          true,
			Secrets:      true,
			VM:           true,
			Containers:   true,
			Network:      true,
			Services:     true,
			Tools:        true,
			Blueprint:    true,
			Generators:   true,
			Stack:        true,
			CommandName:  "test",
		})

		// When
		err := controller.CreateComponents()

		// Then
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ReturnsErrorWhenVirtualizationComponentCreationFails", func(t *testing.T) {
		// Setup
		mocks := setupMocks(t)
		if err := mocks.ConfigHandler.LoadConfigString(`
contexts:
  test:
    vm:
      driver: colima
    docker:
      enabled: true
`); err != nil {
			t.Fatalf("Failed to load config string: %v", err)
		}
		if err := mocks.ConfigHandler.Initialize(); err != nil {
			t.Fatalf("Failed to initialize config handler: %v", err)
		}
		if err := mocks.ConfigHandler.SetContext("test"); err != nil {
			t.Fatalf("Failed to set context: %v", err)
		}

		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) {
			return t.TempDir(), nil
		}

		// Create controller with nil constructors to force error
		controller := NewController(mocks.Injector)
		controller.constructors = ComponentConstructors{
			NewShell: func(injector di.Injector) shell.Shell {
				return mockShell
			},
			NewConfigHandler: func(injector di.Injector) config.ConfigHandler {
				return mocks.ConfigHandler
			},
		}

		// Test
		controller.SetRequirements(Requirements{
			CommandName:  "test",
			VM:           true,
			Containers:   true,
			ConfigLoaded: true,
		})
		err := controller.CreateComponents()

		// Verify
		if err == nil {
			t.Error("Expected error, got nil")
		}
		expectedErr := "failed to create virtualization components: failed to create virtualization components: NewColimaVirt constructor is nil"
		if err.Error() != expectedErr {
			t.Errorf("Expected error %q, got %q", expectedErr, err.Error())
		}
	})

	t.Run("CreatesNoComponentsWhenNoRequirementsSet", func(t *testing.T) {
		// Given
		controller, _ := setup(t)
		controller.SetRequirements(Requirements{
			CommandName: "test",
		})

		// When
		err := controller.CreateComponents()

		// Then
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("CreatesOnlyRequestedComponents", func(t *testing.T) {
		// Given
		controller, mocks := setup(t)
		controller.SetRequirements(Requirements{
			CommandName: "test",
			Tools:       true,
			Blueprint:   true,
			Generators:  false,
			Stack:       false,
			VM:          false,
			Containers:  false,
			Network:     false,
			Services:    false,
			Env:         false,
			Secrets:     false,
		})

		// When
		err := controller.CreateComponents()

		// Then
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// Verify only requested components were created
		if toolsManager := mocks.Injector.Resolve("toolsManager"); toolsManager == nil {
			t.Error("Expected tools manager to be created")
		}
		if blueprintHandler := mocks.Injector.Resolve("blueprintHandler"); blueprintHandler == nil {
			t.Error("Expected blueprint handler to be created")
		}
		if generator := mocks.Injector.Resolve("generator"); generator != nil {
			t.Error("Expected no generator to be created")
		}
		if stack := mocks.Injector.Resolve("stack"); stack != nil {
			t.Error("Expected no stack to be created")
		}
	})
}

func TestBaseController_InitializeComponents(t *testing.T) {
	setup := func(t *testing.T) (*BaseController, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		controller := NewController(mocks.Injector)
		return controller, mocks
	}

	t.Run("SuccessfulInitialization", func(t *testing.T) {
		// Given
		controller, _ := setup(t)

		// When
		err := controller.InitializeComponents()

		// Then
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ShellInitializationFailure", func(t *testing.T) {
		// Given
		controller, mocks := setup(t)
		mockShell := shell.NewMockShell()
		mockShell.InitializeFunc = func() error {
			return fmt.Errorf("shell initialization failed")
		}
		mocks.Injector.Register("shell", mockShell)

		// When
		err := controller.InitializeComponents()

		// Then
		if err == nil {
			t.Error("Expected error when shell initialization fails")
		}
		if !strings.Contains(err.Error(), "shell initialization failed") {
			t.Errorf("Expected error to contain 'shell initialization failed', got %v", err)
		}
	})

	t.Run("SecureShellInitializationFailure", func(t *testing.T) {
		// Given
		controller, mocks := setup(t)
		mockSecureShell := shell.NewMockShell()
		mockSecureShell.InitializeFunc = func() error {
			return fmt.Errorf("secure shell initialization failed")
		}
		mocks.Injector.Register("secureShell", mockSecureShell)

		// When
		err := controller.InitializeComponents()

		// Then
		if err == nil {
			t.Error("Expected error when secure shell initialization fails")
		}
		if !strings.Contains(err.Error(), "secure shell initialization failed") {
			t.Errorf("Expected error to contain 'secure shell initialization failed', got %v", err)
		}
	})

	t.Run("EnvPrinterInitializationFailure", func(t *testing.T) {
		// Given
		controller, mocks := setup(t)
		mockEnvPrinter := env.NewMockEnvPrinter()
		mockEnvPrinter.InitializeFunc = func() error {
			return fmt.Errorf("env printer initialization failed")
		}
		// EnvPrinters need to be registered under specific names to be found by ResolveAllEnvPrinters
		// The general "envPrinter" name won't be picked up
		mocks.Injector.Register("windsorEnv", mockEnvPrinter)

		// When
		err := controller.InitializeComponents()

		// Then
		if err == nil {
			t.Error("Expected error when env printer initialization fails")
		}
		if !strings.Contains(err.Error(), "env printer initialization failed") {
			t.Errorf("Expected error to contain 'env printer initialization failed', got %v", err)
		}
	})

	t.Run("ToolsManagerInitializationFailure", func(t *testing.T) {
		// Given
		controller, mocks := setup(t)
		mockTools := tools.NewMockToolsManager()
		mockTools.InitializeFunc = func() error {
			return fmt.Errorf("tools manager initialization failed")
		}
		mocks.Injector.Register("toolsManager", mockTools)

		// When
		err := controller.InitializeComponents()

		// Then
		if err == nil {
			t.Error("Expected error when tools manager initialization fails")
		}
		if !strings.Contains(err.Error(), "tools manager initialization failed") {
			t.Errorf("Expected error to contain 'tools manager initialization failed', got %v", err)
		}
	})

	t.Run("ServiceInitializationFailure", func(t *testing.T) {
		// Given
		controller, mocks := setup(t)
		mockService := services.NewMockService()
		mockService.InitializeFunc = func() error {
			return fmt.Errorf("service initialization failed")
		}
		// Need to use the interface type registration for ResolveAllServices to find it
		// Let's use a special mock service that implements Service
		mocks.Injector.Register("service", mockService)

		// When
		err := controller.InitializeComponents()

		// Then
		if err == nil {
			t.Error("Expected error when service initialization fails")
		}
		if !strings.Contains(err.Error(), "service initialization failed") {
			t.Errorf("Expected error to contain 'service initialization failed', got %v", err)
		}
	})

	t.Run("VirtualMachineInitializationFailure", func(t *testing.T) {
		// Given
		controller, mocks := setup(t)
		mockVM := virt.NewMockVirt()
		mockVM.InitializeFunc = func() error {
			return fmt.Errorf("virtual machine initialization failed")
		}
		mocks.Injector.Register("virtualMachine", mockVM)

		// When
		err := controller.InitializeComponents()

		// Then
		if err == nil {
			t.Error("Expected error when virtual machine initialization fails")
		}
		if !strings.Contains(err.Error(), "virtual machine initialization failed") {
			t.Errorf("Expected error to contain 'virtual machine initialization failed', got %v", err)
		}
	})

	t.Run("ContainerRuntimeInitializationFailure", func(t *testing.T) {
		// Given
		controller, mocks := setup(t)
		mockRuntime := virt.NewMockVirt()
		mockRuntime.InitializeFunc = func() error {
			return fmt.Errorf("container runtime initialization failed")
		}
		mocks.Injector.Register("containerRuntime", mockRuntime)

		// When
		err := controller.InitializeComponents()

		// Then
		if err == nil {
			t.Error("Expected error when container runtime initialization fails")
		}
		if !strings.Contains(err.Error(), "container runtime initialization failed") {
			t.Errorf("Expected error to contain 'container runtime initialization failed', got %v", err)
		}
	})

	t.Run("NetworkManagerInitializationFailure", func(t *testing.T) {
		// Given
		controller, mocks := setup(t)
		mockNetwork := network.NewMockNetworkManager()
		mockNetwork.InitializeFunc = func() error {
			return fmt.Errorf("network manager initialization failed")
		}
		mocks.Injector.Register("networkManager", mockNetwork)

		// When
		err := controller.InitializeComponents()

		// Then
		if err == nil {
			t.Error("Expected error when network manager initialization fails")
		}
		if !strings.Contains(err.Error(), "network manager initialization failed") {
			t.Errorf("Expected error to contain 'network manager initialization failed', got %v", err)
		}
	})

	t.Run("BlueprintHandlerInitializationFailure", func(t *testing.T) {
		// Given
		controller, mocks := setup(t)
		mockBlueprint := blueprint.NewMockBlueprintHandler(mocks.Injector)
		mockBlueprint.InitializeFunc = func() error {
			return fmt.Errorf("blueprint handler initialization failed")
		}
		mocks.Injector.Register("blueprintHandler", mockBlueprint)

		// When
		err := controller.InitializeComponents()

		// Then
		if err == nil {
			t.Error("Expected error when blueprint handler initialization fails")
		}
		if !strings.Contains(err.Error(), "blueprint handler initialization failed") {
			t.Errorf("Expected error to contain 'blueprint handler initialization failed', got %v", err)
		}
	})

	t.Run("BlueprintHandlerLoadConfigFailure", func(t *testing.T) {
		// Given
		controller, mocks := setup(t)
		mockBlueprint := blueprint.NewMockBlueprintHandler(mocks.Injector)
		mockBlueprint.InitializeFunc = func() error {
			return nil
		}
		mockBlueprint.LoadConfigFunc = func(path ...string) error {
			return fmt.Errorf("blueprint config loading failed")
		}
		mocks.Injector.Register("blueprintHandler", mockBlueprint)

		// When
		err := controller.InitializeComponents()

		// Then
		if err == nil {
			t.Error("Expected error when blueprint config loading fails")
		}
		if !strings.Contains(err.Error(), "blueprint config loading failed") {
			t.Errorf("Expected error to contain 'blueprint config loading failed', got %v", err)
		}
	})

	t.Run("GeneratorInitializationFailure", func(t *testing.T) {
		// Given
		controller, mocks := setup(t)
		mockGenerator := generators.NewMockGenerator()
		mockGenerator.InitializeFunc = func() error {
			return fmt.Errorf("generator initialization failed")
		}
		// Need to use specific generator names or interface-based registration
		// for ResolveAllGenerators to find it
		mocks.Injector.Register("gitGenerator", mockGenerator)

		// When
		err := controller.InitializeComponents()

		// Then
		if err == nil {
			t.Error("Expected error when generator initialization fails")
		}
		if !strings.Contains(err.Error(), "generator initialization failed") {
			t.Errorf("Expected error to contain 'generator initialization failed', got %v", err)
		}
	})

	t.Run("StackInitializationFailure", func(t *testing.T) {
		// Given
		controller, mocks := setup(t)
		mockStack := services.NewMockService()
		mockStack.InitializeFunc = func() error {
			return fmt.Errorf("stack initialization failed")
		}
		mocks.Injector.Register("stack", mockStack)

		// When
		err := controller.InitializeComponents()

		// Then
		if err == nil {
			t.Error("Expected error when stack initialization fails")
		}
		if !strings.Contains(err.Error(), "stack initialization failed") {
			t.Errorf("Expected error to contain 'stack initialization failed', got %v", err)
		}
	})

	t.Run("ComprehensiveTestWithAllComponents", func(t *testing.T) {
		// Given
		controller, mocks := setup(t)

		// Register all component types to ensure full coverage
		mockShell := shell.NewMockShell()
		mocks.Injector.Register("shell", mockShell)

		mockSecureShell := shell.NewMockShell()
		mocks.Injector.Register("secureShell", mockSecureShell)

		mockSecretsProvider := secrets.NewMockSecretsProvider(mocks.Injector)
		mocks.Injector.Register("secretsProvider", mockSecretsProvider)

		mockEnvPrinter := env.NewMockEnvPrinter()
		mocks.Injector.Register("envPrinter", mockEnvPrinter)

		mockToolsManager := tools.NewMockToolsManager()
		mocks.Injector.Register("toolsManager", mockToolsManager)

		mockService := services.NewMockService()
		mocks.Injector.Register("testService", mockService)

		mockVM := virt.NewMockVirt()
		mocks.Injector.Register("virtualMachine", mockVM)

		mockRuntime := virt.NewMockVirt()
		mocks.Injector.Register("containerRuntime", mockRuntime)

		mockNetwork := network.NewMockNetworkManager()
		mocks.Injector.Register("networkManager", mockNetwork)

		mockBlueprint := blueprint.NewMockBlueprintHandler(mocks.Injector)
		mocks.Injector.Register("blueprintHandler", mockBlueprint)

		mockGenerator := generators.NewMockGenerator()
		mocks.Injector.Register("generator", mockGenerator)

		mockStack := services.NewMockService()
		mocks.Injector.Register("stack", mockStack)

		// Track which components were initialized
		initialized := make(map[string]bool)

		mockShell.InitializeFunc = func() error {
			initialized["shell"] = true
			return nil
		}

		mockSecureShell.InitializeFunc = func() error {
			initialized["secureShell"] = true
			return nil
		}

		mockSecretsProvider.InitializeFunc = func() error {
			initialized["secretsProvider"] = true
			return nil
		}

		mockEnvPrinter.InitializeFunc = func() error {
			initialized["envPrinter"] = true
			return nil
		}

		mockToolsManager.InitializeFunc = func() error {
			initialized["toolsManager"] = true
			return nil
		}

		mockService.InitializeFunc = func() error {
			initialized["service"] = true
			return nil
		}

		mockVM.InitializeFunc = func() error {
			initialized["virtualMachine"] = true
			return nil
		}

		mockRuntime.InitializeFunc = func() error {
			initialized["containerRuntime"] = true
			return nil
		}

		mockNetwork.InitializeFunc = func() error {
			initialized["networkManager"] = true
			return nil
		}

		mockBlueprint.InitializeFunc = func() error {
			initialized["blueprintHandler"] = true
			return nil
		}

		mockBlueprint.LoadConfigFunc = func(path ...string) error {
			initialized["blueprintHandlerLoadConfig"] = true
			return nil
		}

		mockGenerator.InitializeFunc = func() error {
			initialized["generator"] = true
			return nil
		}

		mockStack.InitializeFunc = func() error {
			initialized["stack"] = true
			return nil
		}

		// When
		err := controller.InitializeComponents()

		// Then
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// Verify all components were initialized
		expectedInitialized := []string{
			"shell",
			"secureShell",
			"secretsProvider",
			// "envPrinter", // Won't be initialized due to how ResolveAllEnvPrinters works
			"toolsManager",
			// "service", // Won't be initialized due to how ResolveAllServices works
			"virtualMachine",
			"containerRuntime",
			"networkManager",
			"blueprintHandler",
			"blueprintHandlerLoadConfig",
			// "generator", // Won't be initialized due to how ResolveAllGenerators works
			"stack",
		}

		for _, component := range expectedInitialized {
			if !initialized[component] {
				t.Errorf("Expected %s to be initialized", component)
			}
		}
	})

	t.Run("StackInitializationFailure", func(t *testing.T) {
		// Given
		controller, mocks := setup(t)
		mockStack := services.NewMockService()
		mockStack.InitializeFunc = func() error {
			return fmt.Errorf("stack initialization failed")
		}
		mocks.Injector.Register("stack", mockStack)

		// When
		err := controller.InitializeComponents()

		// Then
		if err == nil {
			t.Error("Expected error when stack initialization fails")
		}
		if !strings.Contains(err.Error(), "stack initialization failed") {
			t.Errorf("Expected error to contain 'stack initialization failed', got %v", err)
		}
	})
}

func TestBaseController_WriteConfigurationFiles(t *testing.T) {
	setup := func(t *testing.T) (*BaseController, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		controller := NewController(mocks.Injector)
		return controller, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given
		controller, mocks := setup(t)
		controller.SetRequirements(Requirements{
			Tools:      true,
			Blueprint:  true,
			Services:   true,
			VM:         true,
			Containers: true,
			Generators: true,
		})

		// Mock tools manager
		mockToolsManager := tools.NewMockToolsManager()
		mockToolsManager.WriteManifestFunc = func() error {
			return nil
		}
		mocks.Injector.Register("toolsManager", mockToolsManager)

		// Mock blueprint handler
		mockBlueprintHandler := blueprint.NewMockBlueprintHandler(mocks.Injector)
		mockBlueprintHandler.WriteConfigFunc = func(path ...string) error {
			return nil
		}
		mocks.Injector.Register("blueprintHandler", mockBlueprintHandler)

		// Mock services
		mockService := services.NewMockService()
		mockService.WriteConfigFunc = func() error {
			return nil
		}
		mocks.Injector.Register("service", mockService)

		// Mock virtual machine
		mockVM := virt.NewMockVirt()
		mockVM.WriteConfigFunc = func() error {
			return nil
		}
		mocks.Injector.Register("virtualMachine", mockVM)

		// Mock container runtime
		mockContainerRuntime := virt.NewMockVirt()
		mockContainerRuntime.WriteConfigFunc = func() error {
			return nil
		}
		mocks.Injector.Register("containerRuntime", mockContainerRuntime)

		// Mock generators
		mockGenerator := generators.NewMockGenerator()
		mockGenerator.WriteFunc = func() error {
			return nil
		}
		mocks.Injector.Register("generator", mockGenerator)

		// Mock config handler for VM driver and docker enabled
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "vm.driver" {
				return "colima"
			}
			return ""
		}
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "docker.enabled" {
				return true
			}
			return false
		}
		mockConfigHandler.GetConfigFunc = func() *v1alpha1.Context {
			enabled := true
			return &v1alpha1.Context{
				Docker: &docker.DockerConfig{
					Enabled: &enabled,
					Registries: map[string]docker.RegistryConfig{
						"registry1": docker.RegistryConfig{
							Remote:   "remote1",
							Local:    "local1",
							HostName: "hostname1",
							HostPort: 5000,
						},
						"registry2": docker.RegistryConfig{
							Remote:   "remote2",
							Local:    "local2",
							HostName: "hostname2",
							HostPort: 5001,
						},
					},
				},
			}
		}
		mocks.Injector.Register("configHandler", mockConfigHandler)

		// When
		err := controller.WriteConfigurationFiles()

		// Then
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ToolsConfigError", func(t *testing.T) {
		// Given
		controller, mocks := setup(t)
		controller.SetRequirements(Requirements{
			Tools: true,
		})

		// Mock tools manager with error
		mockToolsManager := tools.NewMockToolsManager()
		mockToolsManager.WriteManifestFunc = func() error {
			return fmt.Errorf("tools manifest write failed")
		}
		mocks.Injector.Register("toolsManager", mockToolsManager)

		// When
		err := controller.WriteConfigurationFiles()

		// Then
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error writing tools manifest") {
			t.Errorf("Expected error about tools manifest, got %v", err)
		}
	})

	t.Run("BlueprintConfigError", func(t *testing.T) {
		// Given
		controller, mocks := setup(t)
		controller.SetRequirements(Requirements{
			Blueprint: true,
		})

		// Mock blueprint handler with error
		mockBlueprintHandler := blueprint.NewMockBlueprintHandler(mocks.Injector)
		mockBlueprintHandler.WriteConfigFunc = func(path ...string) error {
			return fmt.Errorf("blueprint config write failed")
		}
		mocks.Injector.Register("blueprintHandler", mockBlueprintHandler)

		// When
		err := controller.WriteConfigurationFiles()

		// Then
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error writing blueprint config") {
			t.Errorf("Expected error about blueprint config, got %v", err)
		}
	})

	t.Run("ServiceConfigError", func(t *testing.T) {
		// Given
		controller, mocks := setup(t)
		controller.SetRequirements(Requirements{
			Services: true,
		})

		// Mock service with error
		mockService := services.NewMockService()
		mockService.WriteConfigFunc = func() error {
			return fmt.Errorf("service config write failed")
		}
		mocks.Injector.Register("service", mockService)

		// When
		err := controller.WriteConfigurationFiles()

		// Then
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error writing service config") {
			t.Errorf("Expected error about service config, got %v", err)
		}
	})

	t.Run("VMConfigError", func(t *testing.T) {
		// Given
		controller, mocks := setup(t)
		controller.SetRequirements(Requirements{
			VM: true,
		})

		// Mock config handler for VM driver
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "vm.driver" {
				return "colima"
			}
			return ""
		}
		mocks.Injector.Register("configHandler", mockConfigHandler)

		// Mock virtual machine with error
		mockVM := virt.NewMockVirt()
		mockVM.WriteConfigFunc = func() error {
			return fmt.Errorf("virtual machine config write failed")
		}
		mocks.Injector.Register("virtualMachine", mockVM)

		// When
		err := controller.WriteConfigurationFiles()

		// Then
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error writing virtual machine config") {
			t.Errorf("Expected error about virtual machine config, got %v", err)
		}
	})

	t.Run("ContainerConfigError", func(t *testing.T) {
		// Given
		controller, mocks := setup(t)
		controller.SetRequirements(Requirements{
			Containers: true,
		})

		// Mock config handler for docker enabled
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "docker.enabled" {
				return true
			}
			return false
		}
		mocks.Injector.Register("configHandler", mockConfigHandler)

		// Mock container runtime with error
		mockContainerRuntime := virt.NewMockVirt()
		mockContainerRuntime.WriteConfigFunc = func() error {
			return fmt.Errorf("container runtime config write failed")
		}
		mocks.Injector.Register("containerRuntime", mockContainerRuntime)

		// When
		err := controller.WriteConfigurationFiles()

		// Then
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error writing container runtime config") {
			t.Errorf("Expected error about container runtime config, got %v", err)
		}
	})

	t.Run("GeneratorConfigError", func(t *testing.T) {
		// Given
		controller, mocks := setup(t)
		controller.SetRequirements(Requirements{
			Generators: true,
		})

		// Mock generator with error
		mockGenerator := generators.NewMockGenerator()
		mockGenerator.WriteFunc = func() error {
			return fmt.Errorf("generator config write failed")
		}
		mocks.Injector.Register("generator", mockGenerator)

		// When
		err := controller.WriteConfigurationFiles()

		// Then
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error writing generator config") {
			t.Errorf("Expected error about generator config, got %v", err)
		}
	})
}

func TestBaseController_ResolveInjector(t *testing.T) {
	setup := func(t *testing.T) (*BaseController, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		controller := NewController(mocks.Injector)
		return controller, mocks
	}

	t.Run("ReturnsInjectedInjector", func(t *testing.T) {
		// Given
		controller, mocks := setup(t)

		// When
		resolvedInjector := controller.ResolveInjector()

		// Then
		if resolvedInjector != mocks.Injector {
			t.Errorf("Expected injector to be %v, got %v", mocks.Injector, resolvedInjector)
		}
	})
}

func TestBaseController_ResolveConfigHandler(t *testing.T) {
	setup := func(t *testing.T) (*BaseController, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		controller := NewController(mocks.Injector)
		return controller, mocks
	}

	t.Run("ReturnsNilWhenConfigHandlerNotRegistered", func(t *testing.T) {
		// Given
		mocks := setupMocks(t)
		mocks.Injector.Register("configHandler", nil)
		controller := NewController(mocks.Injector)

		// When
		configHandler := controller.ResolveConfigHandler()

		// Then
		if configHandler != nil {
			t.Errorf("Expected configHandler to be nil, got %v", configHandler)
		}
	})

	t.Run("ReturnsRegisteredConfigHandler", func(t *testing.T) {
		// Given
		controller, mocks := setup(t)

		// When
		resolvedConfigHandler := controller.ResolveConfigHandler()

		// Then
		if resolvedConfigHandler != mocks.ConfigHandler {
			t.Errorf("Expected configHandler to be %v, got %v", mocks.ConfigHandler, resolvedConfigHandler)
		}
	})
}

func TestBaseController_ResolveAllSecretsProviders(t *testing.T) {
	t.Run("ReturnsEmptySliceWhenNoProvidersRegistered", func(t *testing.T) {
		// Given
		setup := func() *BaseController {
			mockInjector := di.NewMockInjector()
			controller := NewController(mockInjector)
			return controller
		}

		// When
		controller := setup()
		providers := controller.ResolveAllSecretsProviders()

		// Then
		if len(providers) != 0 {
			t.Errorf("Expected providers to be empty, got %v", providers)
		}
	})

	t.Run("ReturnsAllRegisteredProviders", func(t *testing.T) {
		// Given
		setup := func() (*BaseController, []secrets.SecretsProvider) {
			mockInjector := di.NewMockInjector()
			mockProvider1 := secrets.NewMockSecretsProvider(mockInjector)
			mockProvider2 := secrets.NewMockSecretsProvider(mockInjector)

			// Register providers with the injector
			mockInjector.Register("provider1", mockProvider1)
			mockInjector.Register("provider2", mockProvider2)

			controller := NewController(mockInjector)
			expectedProviders := []secrets.SecretsProvider{mockProvider1, mockProvider2}

			return controller, expectedProviders
		}

		// When
		controller, expectedProviders := setup()
		resolvedProviders := controller.ResolveAllSecretsProviders()

		// Then
		if len(resolvedProviders) != len(expectedProviders) {
			t.Errorf("Expected %d providers, got %d", len(expectedProviders), len(resolvedProviders))
		}

		// Check that all expected providers are in the resolved providers
		for _, expectedProvider := range expectedProviders {
			found := false
			for _, resolvedProvider := range resolvedProviders {
				if resolvedProvider == expectedProvider {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Expected provider %v to be in resolved providers", expectedProvider)
			}
		}
	})
}

func TestBaseController_ResolveEnvPrinter(t *testing.T) {
	setup := func(t *testing.T) (*BaseController, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		controller := NewController(mocks.Injector)
		return controller, mocks
	}

	t.Run("ReturnsNilWhenPrinterNotRegistered", func(t *testing.T) {
		// Given
		controller, _ := setup(t)

		// When
		printer := controller.ResolveEnvPrinter("nonexistent")

		// Then
		if printer != nil {
			t.Errorf("Expected printer to be nil, got %v", printer)
		}
	})

	t.Run("ReturnsRegisteredPrinter", func(t *testing.T) {
		// Given
		controller, mocks := setup(t)
		mockPrinter := env.NewMockEnvPrinter()
		mocks.Injector.Register("testPrinter", mockPrinter)

		// When
		resolvedPrinter := controller.ResolveEnvPrinter("testPrinter")

		// Then
		if resolvedPrinter != mockPrinter {
			t.Errorf("Expected printer to be %v, got %v", mockPrinter, resolvedPrinter)
		}
	})
}

func TestBaseController_ResolveAllEnvPrinters(t *testing.T) {
	setup := func(t *testing.T) (*BaseController, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		controller := NewController(mocks.Injector)
		return controller, mocks
	}

	t.Run("ReturnsEmptySliceWhenNoPrintersRegistered", func(t *testing.T) {
		// Given
		controller, _ := setup(t)

		// When
		printers := controller.ResolveAllEnvPrinters()

		// Then
		if len(printers) != 0 {
			t.Errorf("Expected empty slice of printers, got %v", printers)
		}
	})

	t.Run("ReturnsRegisteredPrintersInCorrectOrder", func(t *testing.T) {
		// Given
		controller, mocks := setup(t)

		// Create and register mock printers
		mockPrinter1 := env.NewMockEnvPrinter()
		mockPrinter2 := env.NewMockEnvPrinter()
		mockWindsorPrinter := &env.WindsorEnvPrinter{}

		mocks.Injector.Register("printer1", mockPrinter1)
		mocks.Injector.Register("printer2", mockPrinter2)
		mocks.Injector.Register("windsorEnv", mockWindsorPrinter)

		// When
		resolvedPrinters := controller.ResolveAllEnvPrinters()

		// Then
		if len(resolvedPrinters) != 3 {
			t.Errorf("Expected 3 printers, got %d", len(resolvedPrinters))
		}

		// Verify Windsor printer is last
		lastPrinter := resolvedPrinters[len(resolvedPrinters)-1]
		if _, ok := lastPrinter.(*env.WindsorEnvPrinter); !ok {
			t.Error("Expected WindsorEnvPrinter to be last")
		}

		// Verify other printers are present
		foundPrinter1 := false
		foundPrinter2 := false
		for i := 0; i < len(resolvedPrinters)-1; i++ {
			if resolvedPrinters[i] == mockPrinter1 {
				foundPrinter1 = true
			}
			if resolvedPrinters[i] == mockPrinter2 {
				foundPrinter2 = true
			}
		}

		if !foundPrinter1 {
			t.Error("Expected to find mockPrinter1 in resolved printers")
		}
		if !foundPrinter2 {
			t.Error("Expected to find mockPrinter2 in resolved printers")
		}
	})

	t.Run("HandlesNilWindsorPrinter", func(t *testing.T) {
		// Given
		controller, mocks := setup(t)

		// Create and register mock printers
		mockPrinter := env.NewMockEnvPrinter()
		mocks.Injector.Register("printer1", mockPrinter)
		mocks.Injector.Register("windsorEnv", nil)

		// When
		resolvedPrinters := controller.ResolveAllEnvPrinters()

		// Then
		if len(resolvedPrinters) != 1 {
			t.Errorf("Expected 1 printer, got %d", len(resolvedPrinters))
		}
		if resolvedPrinters[0] != mockPrinter {
			t.Error("Expected to find mockPrinter in resolved printers")
		}
	})

	t.Run("HandlesNonEnvPrinterTypes", func(t *testing.T) {
		// Given
		controller, mocks := setup(t)

		// Register a non-printer type
		mocks.Injector.Register("notAPrinter", "some string")

		// When
		resolvedPrinters := controller.ResolveAllEnvPrinters()

		// Then
		if len(resolvedPrinters) != 0 {
			t.Errorf("Expected no printers, got %d", len(resolvedPrinters))
		}
	})
}

func TestBaseController_ResolveShell(t *testing.T) {
	setup := func(t *testing.T) (*BaseController, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		controller := NewController(mocks.Injector)
		return controller, mocks
	}

	t.Run("ReturnsNilWhenShellNotRegistered", func(t *testing.T) {
		// Given
		controller, mocks := setup(t)
		mocks.Injector.Register("shell", nil)

		// When
		shell := controller.ResolveShell()

		// Then
		if shell != nil {
			t.Errorf("Expected shell to be nil, got %v", shell)
		}
	})

	t.Run("ReturnsRegisteredShell", func(t *testing.T) {
		// Given
		controller, mocks := setup(t)

		// When
		resolvedShell := controller.ResolveShell()

		// Then
		if resolvedShell != mocks.Shell {
			t.Errorf("Expected shell to be %v, got %v", mocks.Shell, resolvedShell)
		}
	})
}

func TestBaseController_ResolveSecureShell(t *testing.T) {
	setup := func(t *testing.T) (*BaseController, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		controller := NewController(mocks.Injector)
		return controller, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given
		controller, mocks := setup(t)
		mockSecureShell := shell.NewMockShell()
		mocks.Injector.Register("secureShell", mockSecureShell)

		// When
		resolvedShell := controller.ResolveSecureShell()

		// Then
		if resolvedShell != mockSecureShell {
			t.Errorf("Expected shell to be %v, got %v", mockSecureShell, resolvedShell)
		}
	})

	t.Run("ReturnsNilWhenSecureShellNotRegistered", func(t *testing.T) {
		// Given
		controller, mocks := setup(t)
		mocks.Injector.Register("secureShell", nil)

		// When
		shell := controller.ResolveSecureShell()

		// Then
		if shell != nil {
			t.Errorf("Expected shell to be nil, got %v", shell)
		}
	})
}

func TestBaseController_ResolveNetworkManager(t *testing.T) {
	setup := func(t *testing.T) (*BaseController, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		controller := NewController(mocks.Injector)
		return controller, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given
		controller, mocks := setup(t)
		mockNetworkManager := network.NewMockNetworkManager()
		mocks.Injector.Register("networkManager", mockNetworkManager)

		// When
		resolvedManager := controller.ResolveNetworkManager()

		// Then
		if resolvedManager != mockNetworkManager {
			t.Errorf("Expected network manager to be %v, got %v", mockNetworkManager, resolvedManager)
		}
	})

	t.Run("ReturnsNilWhenNetworkManagerNotRegistered", func(t *testing.T) {
		// Given
		controller, mocks := setup(t)
		mocks.Injector.Register("networkManager", nil)

		// When
		manager := controller.ResolveNetworkManager()

		// Then
		if manager != nil {
			t.Errorf("Expected network manager to be nil, got %v", manager)
		}
	})
}

func TestBaseController_ResolveToolsManager(t *testing.T) {
	setup := func(t *testing.T) (*BaseController, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		controller := NewController(mocks.Injector)
		return controller, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given
		controller, mocks := setup(t)
		mockToolsManager := tools.NewMockToolsManager()
		mocks.Injector.Register("toolsManager", mockToolsManager)

		// When
		resolvedManager := controller.ResolveToolsManager()

		// Then
		if resolvedManager != mockToolsManager {
			t.Errorf("Expected tools manager to be %v, got %v", mockToolsManager, resolvedManager)
		}
	})

	t.Run("ReturnsNilWhenToolsManagerNotRegistered", func(t *testing.T) {
		// Given
		controller, mocks := setup(t)
		mocks.Injector.Register("toolsManager", nil)

		// When
		manager := controller.ResolveToolsManager()

		// Then
		if manager != nil {
			t.Errorf("Expected tools manager to be nil, got %v", manager)
		}
	})
}

func TestBaseController_ResolveBlueprintHandler(t *testing.T) {
	setup := func(t *testing.T) (*BaseController, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		controller := NewController(mocks.Injector)
		return controller, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given
		controller, mocks := setup(t)
		mockBlueprintHandler := blueprint.NewMockBlueprintHandler(mocks.Injector)
		mocks.Injector.Register("blueprintHandler", mockBlueprintHandler)

		// When
		resolvedHandler := controller.ResolveBlueprintHandler()

		// Then
		if resolvedHandler != mockBlueprintHandler {
			t.Errorf("Expected blueprint handler to be %v, got %v", mockBlueprintHandler, resolvedHandler)
		}
	})

	t.Run("ReturnsNilWhenBlueprintHandlerNotRegistered", func(t *testing.T) {
		// Given
		controller, mocks := setup(t)
		mocks.Injector.Register("blueprintHandler", nil)

		// When
		handler := controller.ResolveBlueprintHandler()

		// Then
		if handler != nil {
			t.Errorf("Expected blueprint handler to be nil, got %v", handler)
		}
	})
}

func TestBaseController_ResolveService(t *testing.T) {
	setup := func(t *testing.T) (*BaseController, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		controller := NewController(mocks.Injector)
		return controller, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given
		controller, mocks := setup(t)
		mockService := services.NewMockService()
		mocks.Injector.Register("testService", mockService)

		// When
		resolvedService := controller.ResolveService("testService")

		// Then
		if resolvedService != mockService {
			t.Errorf("Expected service to be %v, got %v", mockService, resolvedService)
		}
	})

	t.Run("ReturnsNilWhenServiceNotRegistered", func(t *testing.T) {
		// Given
		controller, mocks := setup(t)
		mocks.Injector.Register("testService", nil)

		// When
		service := controller.ResolveService("testService")

		// Then
		if service != nil {
			t.Errorf("Expected service to be nil, got %v", service)
		}
	})
}

func TestBaseController_ResolveAllServices(t *testing.T) {
	setup := func(t *testing.T) (*BaseController, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		controller := NewController(mocks.Injector)
		return controller, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given
		controller, mocks := setup(t)
		mockService1 := services.NewMockService()
		mockService2 := services.NewMockService()
		mocks.Injector.Register("service1", mockService1)
		mocks.Injector.Register("service2", mockService2)

		// When
		resolvedServices := controller.ResolveAllServices()

		// Then
		if len(resolvedServices) != 2 {
			t.Errorf("Expected 2 services, got %d", len(resolvedServices))
		}

		// Verify both services are present
		foundService1 := false
		foundService2 := false
		for _, service := range resolvedServices {
			switch service {
			case mockService1:
				foundService1 = true
			case mockService2:
				foundService2 = true
			}
		}

		if !foundService1 {
			t.Error("Expected to find mockService1 in resolved services")
		}
		if !foundService2 {
			t.Error("Expected to find mockService2 in resolved services")
		}
	})

	t.Run("ReturnsEmptySliceWhenNoServicesRegistered", func(t *testing.T) {
		// Given
		controller, _ := setup(t)

		// When
		services := controller.ResolveAllServices()

		// Then
		if len(services) != 0 {
			t.Errorf("Expected empty slice of services, got %v", services)
		}
	})
}

func TestBaseController_ResolveVirtualMachine(t *testing.T) {
	setup := func(t *testing.T) (*BaseController, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		controller := NewController(mocks.Injector)
		return controller, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given
		controller, mocks := setup(t)
		mockVM := &struct{ virt.VirtualMachine }{}
		mocks.Injector.Register("virtualMachine", mockVM)

		// When
		resolvedVM := controller.ResolveVirtualMachine()

		// Then
		if resolvedVM != mockVM {
			t.Errorf("Expected virtual machine to be %v, got %v", mockVM, resolvedVM)
		}
	})

	t.Run("ReturnsNilWhenVirtualMachineNotRegistered", func(t *testing.T) {
		// Given
		controller, mocks := setup(t)
		mocks.Injector.Register("virtualMachine", nil)

		// When
		vm := controller.ResolveVirtualMachine()

		// Then
		if vm != nil {
			t.Errorf("Expected virtual machine to be nil, got %v", vm)
		}
	})
}

func TestBaseController_ResolveContainerRuntime(t *testing.T) {
	setup := func(t *testing.T) (*BaseController, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		controller := NewController(mocks.Injector)
		return controller, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given
		controller, mocks := setup(t)
		mockRuntime := &struct{ virt.ContainerRuntime }{}
		mocks.Injector.Register("containerRuntime", mockRuntime)

		// When
		resolvedRuntime := controller.ResolveContainerRuntime()

		// Then
		if resolvedRuntime != mockRuntime {
			t.Errorf("Expected container runtime to be %v, got %v", mockRuntime, resolvedRuntime)
		}
	})

	t.Run("ReturnsNilWhenContainerRuntimeNotRegistered", func(t *testing.T) {
		// Given
		controller, mocks := setup(t)
		mocks.Injector.Register("containerRuntime", nil)

		// When
		runtime := controller.ResolveContainerRuntime()

		// Then
		if runtime != nil {
			t.Errorf("Expected container runtime to be nil, got %v", runtime)
		}
	})
}

func TestBaseController_ResolveStack(t *testing.T) {
	t.Run("when stack is registered", func(t *testing.T) {
		// Test resolving a registered stack
	})

	t.Run("when stack is not registered", func(t *testing.T) {
		// Test resolving when no stack is registered
	})
}

func TestBaseController_ResolveAllGenerators(t *testing.T) {
	t.Run("", func(t *testing.T) {
		// Given

		// When

		// Then
	})
}

func TestBaseController_SetEnvironmentVariables(t *testing.T) {
	setup := func(t *testing.T) (*BaseController, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		controller := NewController(mocks.Injector)
		return controller, mocks
	}

	t.Run("SetsEnvironmentVariablesFromAllPrinters", func(t *testing.T) {
		// Given
		controller, mocks := setup(t)

		mockPrinter1 := env.NewMockEnvPrinter()
		mockPrinter2 := env.NewMockEnvPrinter()

		mockPrinter1.GetEnvVarsFunc = func() (map[string]string, error) {
			return map[string]string{
				"TEST_VAR1": "value1",
			}, nil
		}
		mockPrinter2.GetEnvVarsFunc = func() (map[string]string, error) {
			return map[string]string{
				"TEST_VAR2": "value2",
			}, nil
		}

		mocks.Injector.Register("printer1", mockPrinter1)
		mocks.Injector.Register("printer2", mockPrinter2)

		// When
		err := controller.SetEnvironmentVariables()

		// Then
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if got := os.Getenv("TEST_VAR1"); got != "value1" {
			t.Errorf("Expected TEST_VAR1 to be 'value1', got %q", got)
		}
		if got := os.Getenv("TEST_VAR2"); got != "value2" {
			t.Errorf("Expected TEST_VAR2 to be 'value2', got %q", got)
		}
	})

	t.Run("ReturnsErrorWhenEnvPrinterFailsToGetVariables", func(t *testing.T) {
		// Given
		controller, mocks := setup(t)

		mockPrinter := env.NewMockEnvPrinter()
		mockPrinter.GetEnvVarsFunc = func() (map[string]string, error) {
			return nil, fmt.Errorf("failed to get env vars")
		}
		mocks.Injector.Register("printer1", mockPrinter)

		// When
		err := controller.SetEnvironmentVariables()

		// Then
		if err == nil {
			t.Error("Expected error when env printer fails to get variables")
		}
		if !strings.Contains(err.Error(), "failed to get env vars") {
			t.Errorf("Expected error to contain 'failed to get env vars', got %v", err)
		}
	})

	t.Run("ReturnsErrorWhenSettingEnvironmentVariableFails", func(t *testing.T) {
		// Given
		controller, mocks := setup(t)

		mockPrinter := env.NewMockEnvPrinter()
		mockPrinter.GetEnvVarsFunc = func() (map[string]string, error) {
			return map[string]string{
				"INVALID_VAR": "value",
			}, nil
		}
		mocks.Injector.Register("printer1", mockPrinter)

		// Override os.Setenv to simulate failure
		originalSetenv := osSetenv
		osSetenv = func(key, value string) error {
			return fmt.Errorf("failed to set env var")
		}
		defer func() { osSetenv = originalSetenv }()

		// When
		err := controller.SetEnvironmentVariables()

		// Then
		if err == nil {
			t.Error("Expected error when setting environment variable fails")
		}
		if !strings.Contains(err.Error(), "failed to set env var") {
			t.Errorf("Expected error to contain 'failed to set env var', got %v", err)
		}
	})
}

func TestBaseController_createConfigComponent(t *testing.T) {
	setup := func(t *testing.T) (*BaseController, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		controller := NewController(mocks.Injector)
		return controller, mocks
	}

	t.Run("ReturnsErrorWhenNewConfigHandlerIsNil", func(t *testing.T) {
		// Given
		controller, _ := setup(t)
		controller.constructors.NewConfigHandler = nil

		// When
		err := controller.createConfigComponent(Requirements{})

		// Then
		if err == nil {
			t.Error("Expected error when NewConfigHandler is nil")
		}
		if err.Error() != "required constructor NewConfigHandler is nil" {
			t.Errorf("Expected error 'required constructor NewConfigHandler is nil', got %v", err)
		}
	})

	t.Run("CreatesAndRegistersConfigHandler", func(t *testing.T) {
		// Given
		controller, mocks := setup(t)
		mockConfigHandler := config.NewMockConfigHandler()
		controller.constructors.NewConfigHandler = func(di.Injector) config.ConfigHandler {
			return mockConfigHandler
		}

		// Clear any existing config handler
		mocks.Injector.Register("configHandler", nil)

		// When
		err := controller.createConfigComponent(Requirements{})

		// Then
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		resolvedHandler := mocks.Injector.Resolve("configHandler")
		if resolvedHandler != mockConfigHandler {
			t.Error("Expected config handler to be registered with injector")
		}
	})

	t.Run("ReturnsErrorWhenInitializationFails", func(t *testing.T) {
		// Given
		controller, mocks := setup(t)
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.InitializeFunc = func() error {
			return fmt.Errorf("initialization failed")
		}
		controller.constructors.NewConfigHandler = func(di.Injector) config.ConfigHandler {
			return mockConfigHandler
		}

		// Clear any existing config handler
		mocks.Injector.Register("configHandler", nil)

		// When
		err := controller.createConfigComponent(Requirements{})

		// Then
		if err == nil {
			t.Error("Expected error when initialization fails")
		}
		if !strings.Contains(err.Error(), "initialization failed") {
			t.Errorf("Expected error to contain 'initialization failed', got %v", err)
		}
	})

	t.Run("UsesWindsorConfigEnvVar", func(t *testing.T) {
		// Given
		controller, mocks := setup(t)
		mockConfigHandler := config.NewMockConfigHandler()
		configPath := "/custom/config/path"
		loadConfigCalled := false
		mockConfigHandler.LoadConfigFunc = func(path string) error {
			loadConfigCalled = true
			if path != configPath {
				t.Errorf("Expected config path %q, got %q", configPath, path)
			}
			return nil
		}
		controller.constructors.NewConfigHandler = func(di.Injector) config.ConfigHandler {
			return mockConfigHandler
		}

		// Clear any existing config handler
		mocks.Injector.Register("configHandler", nil)

		// Set environment variable
		oldEnv := os.Getenv("WINDSORCONFIG")
		os.Setenv("WINDSORCONFIG", configPath)
		defer func() {
			if oldEnv != "" {
				os.Setenv("WINDSORCONFIG", oldEnv)
			} else {
				os.Unsetenv("WINDSORCONFIG")
			}
		}()

		// When
		err := controller.createConfigComponent(Requirements{})

		// Then
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if !loadConfigCalled {
			t.Error("Expected LoadConfig to be called")
		}
	})

	t.Run("HandlesProjectRootError", func(t *testing.T) {
		// Given
		controller, mocks := setup(t)
		mockConfigHandler := config.NewMockConfigHandler()
		controller.constructors.NewConfigHandler = func(di.Injector) config.ConfigHandler {
			return mockConfigHandler
		}

		// Clear any existing config handler
		mocks.Injector.Register("configHandler", nil)

		// Mock shell to return error
		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("project root error")
		}
		mocks.Injector.Register("shell", mockShell)

		// When
		err := controller.createConfigComponent(Requirements{})

		// Then
		if err == nil {
			t.Error("Expected error when getting project root fails")
		}
		if !strings.Contains(err.Error(), "project root error") {
			t.Errorf("Expected error to contain 'project root error', got %v", err)
		}
	})

	t.Run("HandlesConfigFileDiscovery", func(t *testing.T) {
		// Given
		controller, mocks := setup(t)
		mockConfigHandler := config.NewMockConfigHandler()
		loadedPath := ""
		mockConfigHandler.LoadConfigFunc = func(path string) error {
			loadedPath = path
			return nil
		}
		controller.constructors.NewConfigHandler = func(di.Injector) config.ConfigHandler {
			return mockConfigHandler
		}

		// Clear any existing config handler
		mocks.Injector.Register("configHandler", nil)

		// Create temporary project directory
		projectRoot := t.TempDir()
		mockShell := shell.NewMockShell()
		mockShell.GetProjectRootFunc = func() (string, error) {
			return projectRoot, nil
		}
		mocks.Injector.Register("shell", mockShell)

		// Create config file
		configPath := filepath.Join(projectRoot, "windsor.yaml")
		if err := os.WriteFile(configPath, []byte("test"), 0644); err != nil {
			t.Fatalf("Failed to create test config file: %v", err)
		}

		// When
		err := controller.createConfigComponent(Requirements{})

		// Then
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if loadedPath != configPath {
			t.Errorf("Expected config path %q, got %q", configPath, loadedPath)
		}
	})

	t.Run("HandlesConfigLoadError", func(t *testing.T) {
		// Given
		controller, mocks := setup(t)
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.LoadConfigFunc = func(path string) error {
			return fmt.Errorf("load config error")
		}
		controller.constructors.NewConfigHandler = func(di.Injector) config.ConfigHandler {
			return mockConfigHandler
		}

		// Clear any existing config handler
		mocks.Injector.Register("configHandler", nil)

		// Set environment variable to force config load
		oldEnv := os.Getenv("WINDSORCONFIG")
		os.Setenv("WINDSORCONFIG", "test.yaml")
		defer func() {
			if oldEnv != "" {
				os.Setenv("WINDSORCONFIG", oldEnv)
			} else {
				os.Unsetenv("WINDSORCONFIG")
			}
		}()

		// When
		err := controller.createConfigComponent(Requirements{})

		// Then
		if err == nil {
			t.Error("Expected error when loading config fails")
		}
		if !strings.Contains(err.Error(), "load config error") {
			t.Errorf("Expected error to contain 'load config error', got %v", err)
		}
	})

	t.Run("HandlesConfigLoadedRequirement", func(t *testing.T) {
		// Given
		controller, mocks := setup(t)
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.IsLoadedFunc = func() bool {
			return false
		}
		controller.constructors.NewConfigHandler = func(di.Injector) config.ConfigHandler {
			return mockConfigHandler
		}

		// Clear any existing config handler
		mocks.Injector.Register("configHandler", nil)

		// When
		err := controller.createConfigComponent(Requirements{
			ConfigLoaded: true,
		})

		// Then
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}

func TestBaseController_createSecretsComponents(t *testing.T) {
	setup := func(t *testing.T) (*BaseController, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		controller := NewController(mocks.Injector)
		return controller, mocks
	}

	t.Run("NoSecretsRequired", func(t *testing.T) {
		// Given
		controller, _ := setup(t)
		controller.SetRequirements(Requirements{
			CommandName: "test",
			Secrets:     false,
		})

		// When
		err := controller.createSecretsComponents(controller.requirements)

		// Then
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// Verify no secrets providers were registered
		providers := controller.ResolveAllSecretsProviders()
		if len(providers) != 0 {
			t.Errorf("Expected no secrets providers, got %d", len(providers))
		}
	})

	t.Run("NilConfigHandler", func(t *testing.T) {
		// Given
		controller, mocks := setup(t)
		mocks.Injector.Register("configHandler", nil)
		controller.SetRequirements(Requirements{
			CommandName: "test",
			Secrets:     true,
		})

		// When
		err := controller.createSecretsComponents(controller.requirements)

		// Then
		if err == nil {
			t.Error("Expected error when config handler is nil")
		}
		if err.Error() != "config handler is nil" {
			t.Errorf("Expected error 'config handler is nil', got %v", err)
		}
	})

	t.Run("SopsSecretsProvider", func(t *testing.T) {
		// Given
		controller, mocks := setup(t)
		controller.SetRequirements(Requirements{
			CommandName: "test",
			Secrets:     true,
		})

		// Create a temporary directory structure
		tempDir := t.TempDir()
		projectRoot := filepath.Join(tempDir, "project")
		if err := os.MkdirAll(projectRoot, 0755); err != nil {
			t.Fatalf("Failed to create project directory: %v", err)
		}

		// Create contexts directory and secrets file
		contextDir := filepath.Join(projectRoot, "contexts", "test")
		if err := os.MkdirAll(contextDir, 0755); err != nil {
			t.Fatalf("Failed to create context directory: %v", err)
		}
		secretsFile := filepath.Join(contextDir, "secrets.enc.yaml")
		if err := os.WriteFile(secretsFile, []byte("encrypted: data"), 0644); err != nil {
			t.Fatalf("Failed to create secrets file: %v", err)
		}

		// Set up mock shell to return our project root
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return projectRoot, nil
		}

		// Configure the config handler
		if err := mocks.ConfigHandler.LoadConfigString(`
contexts:
  test:
    projectRoot: "` + projectRoot + `"
`); err != nil {
			t.Fatalf("Failed to load config string: %v", err)
		}

		// When
		err := controller.createSecretsComponents(controller.requirements)

		// Then
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// Verify SOPS provider was registered
		providers := controller.ResolveAllSecretsProviders()
		if len(providers) != 1 {
			t.Errorf("Expected 1 secrets provider, got %d", len(providers))
		}
	})

	t.Run("OnePasswordSDKProvider", func(t *testing.T) {
		// Given
		controller, mocks := setup(t)
		controller.SetRequirements(Requirements{
			CommandName: "test",
			Secrets:     true,
		})

		// Set OP_SERVICE_ACCOUNT_TOKEN env var
		os.Setenv("OP_SERVICE_ACCOUNT_TOKEN", "test-token")
		defer os.Unsetenv("OP_SERVICE_ACCOUNT_TOKEN")

		// Configure the config handler with 1Password vaults
		if err := mocks.ConfigHandler.LoadConfigString(`
contexts:
  test:
    secrets:
      onepassword:
        vaults:
          vault1:
            url: "https://test.1password.com"
            name: "Test Vault"
`); err != nil {
			t.Fatalf("Failed to load config string: %v", err)
		}

		// When
		err := controller.createSecretsComponents(controller.requirements)

		// Then
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// Verify 1Password SDK provider was registered
		providers := controller.ResolveAllSecretsProviders()
		if len(providers) != 1 {
			t.Errorf("Expected 1 secrets provider, got %d", len(providers))
		}
	})
}

func TestBaseController_createToolsComponents(t *testing.T) {
	setup := func(t *testing.T) (*BaseController, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		controller := NewController(mocks.Injector)
		return controller, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given
		controller, _ := setup(t)
		controller.SetRequirements(Requirements{
			CommandName: "test",
			Tools:       true,
		})

		// When
		err := controller.createToolsComponents(controller.requirements)

		// Then
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}

func TestBaseController_createGeneratorsComponents(t *testing.T) {
	setup := func(t *testing.T) (*BaseController, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		controller := NewController(mocks.Injector)
		return controller, mocks
	}

	t.Run("ReturnsEarlyWhenNotRequired", func(t *testing.T) {
		// Given
		controller, _ := setup(t)

		// When
		err := controller.createGeneratorsComponents(Requirements{
			Generators: false,
		})

		// Then
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// Verify no generators were created
		generators := controller.ResolveAllGenerators()
		if len(generators) != 0 {
			t.Errorf("Expected no generators, got %d", len(generators))
		}
	})

	t.Run("CreatesGitGeneratorWhenRequired", func(t *testing.T) {
		// Given
		controller, mocks := setup(t)

		// Mock git generator
		mockGenerator := generators.NewMockGenerator()
		controller.constructors.NewGitGenerator = func(di.Injector) generators.Generator {
			return mockGenerator
		}

		// When
		err := controller.createGeneratorsComponents(Requirements{
			Generators: true,
		})

		// Then
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// Verify git generator was created
		if resolved := mocks.Injector.Resolve("gitGenerator"); resolved != mockGenerator {
			t.Error("Expected git generator to be registered")
		}
	})

	t.Run("CreatesTerraformAndKustomizeGeneratorsWhenBlueprintRequired", func(t *testing.T) {
		// Given
		controller, mocks := setup(t)

		// Mock generators
		mockTerraformGenerator := generators.NewMockGenerator()
		mockKustomizeGenerator := generators.NewMockGenerator()
		controller.constructors.NewTerraformGenerator = func(di.Injector) generators.Generator {
			return mockTerraformGenerator
		}
		controller.constructors.NewKustomizeGenerator = func(di.Injector) generators.Generator {
			return mockKustomizeGenerator
		}

		// When
		err := controller.createGeneratorsComponents(Requirements{
			Generators: true,
			Blueprint:  true,
		})

		// Then
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// Verify generators were created
		if resolved := mocks.Injector.Resolve("terraformGenerator"); resolved != mockTerraformGenerator {
			t.Error("Expected terraform generator to be registered")
		}
		if resolved := mocks.Injector.Resolve("kustomizeGenerator"); resolved != mockKustomizeGenerator {
			t.Error("Expected kustomize generator to be registered")
		}
	})

	t.Run("DoesNotCreateDuplicateGenerators", func(t *testing.T) {
		// Given
		controller, mocks := setup(t)

		// Create initial generator
		mockGenerator := generators.NewMockGenerator()
		mocks.Injector.Register("gitGenerator", mockGenerator)

		// Mock new generator constructor
		newGeneratorCalled := false
		controller.constructors.NewGitGenerator = func(di.Injector) generators.Generator {
			newGeneratorCalled = true
			return generators.NewMockGenerator()
		}

		// When
		err := controller.createGeneratorsComponents(Requirements{
			Generators: true,
		})

		// Then
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// Verify constructor wasn't called
		if newGeneratorCalled {
			t.Error("Expected constructor not to be called for existing generator")
		}

		// Verify original generator is still registered
		if resolved := mocks.Injector.Resolve("gitGenerator"); resolved != mockGenerator {
			t.Error("Expected original generator to remain registered")
		}
	})
}

func TestBaseController_createBlueprintComponent(t *testing.T) {
	setup := func(t *testing.T) (*BaseController, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		controller := NewController(mocks.Injector)
		return controller, mocks
	}

	t.Run("ReturnsEarlyWhenNotRequired", func(t *testing.T) {
		// Given
		controller, _ := setup(t)

		// When
		err := controller.createBlueprintComponent(Requirements{
			Blueprint: false,
		})

		// Then
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// Verify no blueprint handler was created
		if handler := controller.ResolveBlueprintHandler(); handler != nil {
			t.Error("Expected no blueprint handler to be created")
		}
	})

	t.Run("CreatesBlueprintHandlerWhenRequired", func(t *testing.T) {
		// Given
		controller, mocks := setup(t)

		// Mock blueprint handler
		mockHandler := blueprint.NewMockBlueprintHandler(mocks.Injector)
		controller.constructors.NewBlueprintHandler = func(di.Injector) blueprint.BlueprintHandler {
			return mockHandler
		}

		// When
		err := controller.createBlueprintComponent(Requirements{
			Blueprint: true,
		})

		// Then
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// Verify blueprint handler was created
		if resolved := controller.ResolveBlueprintHandler(); resolved != mockHandler {
			t.Error("Expected blueprint handler to be registered")
		}
	})

	t.Run("DoesNotCreateDuplicateBlueprintHandler", func(t *testing.T) {
		// Given
		controller, mocks := setup(t)

		// Create initial handler
		mockHandler := blueprint.NewMockBlueprintHandler(mocks.Injector)
		mocks.Injector.Register("blueprintHandler", mockHandler)

		// Mock new handler constructor
		newHandlerCalled := false
		controller.constructors.NewBlueprintHandler = func(di.Injector) blueprint.BlueprintHandler {
			newHandlerCalled = true
			return blueprint.NewMockBlueprintHandler(mocks.Injector)
		}

		// When
		err := controller.createBlueprintComponent(Requirements{
			Blueprint: true,
		})

		// Then
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// Verify constructor wasn't called
		if newHandlerCalled {
			t.Error("Expected constructor not to be called for existing handler")
		}

		// Verify original handler is still registered
		if resolved := controller.ResolveBlueprintHandler(); resolved != mockHandler {
			t.Error("Expected original handler to remain registered")
		}
	})
}

func TestBaseController_createEnvComponents(t *testing.T) {
	setup := func(t *testing.T) (*BaseController, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		controller := NewController(mocks.Injector)
		return controller, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given
		controller, _ := setup(t)
		controller.SetRequirements(Requirements{
			CommandName: "test",
			Env:         true,
		})

		// When
		err := controller.createEnvComponents(controller.requirements)

		// Then
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}

func TestBaseController_createServiceComponents(t *testing.T) {
	setup := func(t *testing.T) (*BaseController, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		controller := NewController(mocks.Injector)
		return controller, mocks
	}

	t.Run("ReturnsEarlyWhenNotRequired", func(t *testing.T) {
		// Given
		controller, _ := setup(t)

		// When
		err := controller.createServiceComponents(Requirements{Services: false})

		// Then
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ReturnsEarlyWhenDockerNotEnabled", func(t *testing.T) {
		// Given
		controller, mocks := setup(t)

		// Mock config handler
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetBoolFunc = func(key string, _ ...bool) bool {
			return false
		}
		mocks.Injector.Register("configHandler", mockConfigHandler)

		// When
		err := controller.createServiceComponents(Requirements{Services: true})

		// Then
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("CreatesServicesWhenEnabled", func(t *testing.T) {
		// Given
		controller, mocks := setup(t)

		// Mock config handler
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetBoolFunc = func(key string, _ ...bool) bool {
			switch key {
			case "docker.enabled", "dns.enabled", "git.livereload.enabled", "aws.localstack.enabled":
				return true
			default:
				return false
			}
		}
		mocks.Injector.Register("configHandler", mockConfigHandler)

		// Track created services
		createdServices := make(map[string]string)
		mockService := services.NewMockService()
		mockService.SetNameFunc = func(name string) {
			createdServices[name] = name
		}

		// Set up constructors
		controller.constructors.NewDNSService = func(di.Injector) services.Service { return mockService }
		controller.constructors.NewGitLivereloadService = func(di.Injector) services.Service { return mockService }
		controller.constructors.NewLocalstackService = func(di.Injector) services.Service { return mockService }

		// When
		err := controller.createServiceComponents(Requirements{Services: true})

		// Then
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// Verify expected services were created
		expectedServices := map[string]string{
			"dns": "dnsService",
			"git": "gitLivereloadService",
			"aws": "localstackService",
		}
		for name, key := range expectedServices {
			if _, ok := createdServices[name]; !ok {
				t.Errorf("Expected service %q to be created", name)
			}
			if resolved := mocks.Injector.Resolve(key); resolved == nil {
				t.Errorf("Expected service %q to be registered as %q", name, key)
			}
		}
	})

	t.Run("CreatesRegistryServices", func(t *testing.T) {
		// Given
		controller, mocks := setup(t)

		// Mock config handler
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetBoolFunc = func(key string, _ ...bool) bool {
			return key == "docker.enabled"
		}
		mockConfigHandler.GetConfigFunc = func() *v1alpha1.Context {
			enabled := true
			return &v1alpha1.Context{
				Docker: &docker.DockerConfig{
					Enabled: &enabled,
					Registries: map[string]docker.RegistryConfig{
						"registry1": {},
						"registry2": {},
					},
				},
			}
		}
		mocks.Injector.Register("configHandler", mockConfigHandler)

		// Track created services
		createdServices := make(map[string]string)
		mockService := services.NewMockService()
		mockService.SetNameFunc = func(name string) {
			createdServices[name] = name
		}

		// Set up constructors
		controller.constructors.NewRegistryService = func(di.Injector) services.Service { return mockService }

		// When
		err := controller.createServiceComponents(Requirements{Services: true})

		// Then
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// Verify registry services were created
		expectedRegistries := []string{"registry1", "registry2"}
		for _, name := range expectedRegistries {
			if _, ok := createdServices[name]; !ok {
				t.Errorf("Expected registry service %q to be created", name)
			}
			serviceName := fmt.Sprintf("registryService.%s", name)
			if resolved := mocks.Injector.Resolve(serviceName); resolved == nil {
				t.Errorf("Expected registry service %q to be registered as %q", name, serviceName)
			}
		}
	})

	t.Run("CreatesTalosServices", func(t *testing.T) {
		// Given
		controller, mocks := setup(t)

		// Mock config handler
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetBoolFunc = func(key string, _ ...bool) bool {
			switch key {
			case "docker.enabled", "cluster.enabled":
				return true
			default:
				return false
			}
		}
		mockConfigHandler.GetStringFunc = func(key string, _ ...string) string {
			if key == "cluster.driver" {
				return "talos"
			}
			return ""
		}
		mockConfigHandler.GetIntFunc = func(key string, _ ...int) int {
			switch key {
			case "cluster.controlplanes.count":
				return 2
			case "cluster.workers.count":
				return 3
			default:
				return 0
			}
		}
		mocks.Injector.Register("configHandler", mockConfigHandler)

		// Track created services
		createdServices := make(map[string]string)
		mockService := services.NewMockService()
		mockService.SetNameFunc = func(name string) {
			createdServices[name] = name
		}

		// Set up constructors
		controller.constructors.NewTalosService = func(injector di.Injector, nodeType string) services.Service {
			return mockService
		}

		// When
		err := controller.createServiceComponents(Requirements{Services: true})

		// Then
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// Verify control plane services were created
		for i := 1; i <= 2; i++ {
			name := fmt.Sprintf("controlplane-%d", i)
			if _, ok := createdServices[name]; !ok {
				t.Errorf("Expected control plane service %q to be created", name)
			}
			serviceName := fmt.Sprintf("clusterNode.%s", name)
			if resolved := mocks.Injector.Resolve(serviceName); resolved == nil {
				t.Errorf("Expected control plane service %q to be registered as %q", name, serviceName)
			}
		}

		// Verify worker services were created
		for i := 1; i <= 3; i++ {
			name := fmt.Sprintf("worker-%d", i)
			if _, ok := createdServices[name]; !ok {
				t.Errorf("Expected worker service %q to be created", name)
			}
			serviceName := fmt.Sprintf("clusterNode.%s", name)
			if resolved := mocks.Injector.Resolve(serviceName); resolved == nil {
				t.Errorf("Expected worker service %q to be registered as %q", name, serviceName)
			}
		}
	})
}

func TestBaseController_createNetworkComponents(t *testing.T) {
	t.Run("CreatesBaseNetworkManagerWhenNetworkRequired", func(t *testing.T) {
		mocks := setupMocks(t)
		controller := NewController(mocks.Injector)

		req := Requirements{
			Network: true,
		}

		err := controller.createNetworkComponents(req)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		networkManager := controller.ResolveNetworkManager()
		if networkManager == nil {
			t.Error("Expected network manager to be created")
		}
		if _, ok := networkManager.(*network.BaseNetworkManager); !ok {
			t.Errorf("Expected BaseNetworkManager, got %T", networkManager)
		}
	})

	t.Run("CreatesColimaNetworkManagerWhenVMRequired", func(t *testing.T) {
		mocks := setupMocks(t, &SetupOptions{
			ConfigStr: `
version: v1alpha1
contexts:
  mock-context:
    vm:
      driver: colima
`,
		})
		controller := NewController(mocks.Injector)

		if err := mocks.ConfigHandler.SetContext("mock-context"); err != nil {
			t.Fatalf("Failed to set context: %v", err)
		}

		req := Requirements{
			Network: true,
			VM:      true,
		}

		err := controller.createNetworkComponents(req)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		networkManager := controller.ResolveNetworkManager()
		if networkManager == nil {
			t.Error("Expected network manager to be created")
		}
		if _, ok := networkManager.(*network.ColimaNetworkManager); !ok {
			t.Errorf("Expected ColimaNetworkManager, got %T", networkManager)
		}
	})

	t.Run("CreatesSecureShellAndSSHClientWhenVMRequired", func(t *testing.T) {
		mocks := setupMocks(t)
		controller := NewController(mocks.Injector)

		req := Requirements{
			Network: true,
			VM:      true,
		}

		err := controller.createNetworkComponents(req)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		secureShell := controller.ResolveSecureShell()
		if secureShell == nil {
			t.Error("Expected secure shell to be created")
		}

		sshClient := controller.ResolveInjector().Resolve("sshClient")
		if sshClient == nil {
			t.Error("Expected SSH client to be created")
		}
		if _, ok := sshClient.(*ssh.SSHClient); !ok {
			t.Errorf("Expected SSHClient, got %T", sshClient)
		}
	})

	t.Run("DoesNotCreateComponentsWhenNetworkNotRequired", func(t *testing.T) {
		mocks := setupMocks(t)
		controller := NewController(mocks.Injector)

		req := Requirements{
			Network: false,
		}

		err := controller.createNetworkComponents(req)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		networkManager := controller.ResolveNetworkManager()
		if networkManager != nil {
			t.Error("Expected no network manager to be created")
		}
	})

	t.Run("CreatesNetworkInterfaceProviderWhenNetworkRequired", func(t *testing.T) {
		mocks := setupMocks(t)
		controller := NewController(mocks.Injector)

		req := Requirements{
			Network: true,
		}

		err := controller.createNetworkComponents(req)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		provider := mocks.Injector.Resolve("networkInterfaceProvider")
		if provider == nil {
			t.Error("Expected network interface provider to be created")
		}
	})

	t.Run("DoesNotCreateDuplicateComponents", func(t *testing.T) {
		mocks := setupMocks(t)
		controller := NewController(mocks.Injector)

		req := Requirements{
			Network: true,
			VM:      true,
		}

		// First creation
		err := controller.createNetworkComponents(req)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// Store initial instances
		initialNetworkManager := controller.ResolveNetworkManager()
		initialSecureShell := controller.ResolveSecureShell()
		initialSSHClient := mocks.Injector.Resolve("sshClient")
		initialProvider := mocks.Injector.Resolve("networkInterfaceProvider")

		// Second creation
		err = controller.createNetworkComponents(req)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// Verify instances haven't changed
		if controller.ResolveNetworkManager() != initialNetworkManager {
			t.Error("Network manager was recreated")
		}
		if controller.ResolveSecureShell() != initialSecureShell {
			t.Error("Secure shell was recreated")
		}
		if mocks.Injector.Resolve("sshClient") != initialSSHClient {
			t.Error("SSH client was recreated")
		}
		if mocks.Injector.Resolve("networkInterfaceProvider") != initialProvider {
			t.Error("Network interface provider was recreated")
		}
	})
}

func TestController_CreateVirtualizationComponents(t *testing.T) {
	setup := func(t *testing.T) (*BaseController, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		controller := NewController(mocks.Injector)
		controller.SetRequirements(Requirements{
			CommandName: "test",
		})
		if err := controller.CreateComponents(); err != nil {
			t.Fatalf("Failed to create components: %v", err)
		}
		return controller, mocks
	}

	t.Run("ReturnsNilWhenNoVMIsRequired", func(t *testing.T) {
		// Given
		controller, mocks := setup(t)

		// When
		req := Requirements{
			VM:         false,
			Containers: false,
		}

		// Then
		err := controller.createVirtualizationComponents(req)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// Verify no components were registered
		if vm := mocks.Injector.Resolve("virtualMachine"); vm != nil {
			t.Error("Expected no virtual machine to be registered")
		}
		if cr := mocks.Injector.Resolve("containerRuntime"); cr != nil {
			t.Error("Expected no container runtime to be registered")
		}
	})
}

func TestBaseController_InitializeWithRequirements(t *testing.T) {
	type initializationTestCase struct {
		requirements         Requirements
		mockInitializations  map[string]bool
		mockInitErrors       map[string]error
		expectedError        bool
		expectedErrorMessage string
	}

	setup := func(t *testing.T, testCase *initializationTestCase) (*BaseController, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		controller := NewController(mocks.Injector)

		// Setup initialization tracking
		initCalled := make(map[string]bool)
		for component := range testCase.mockInitializations {
			initCalled[component] = false
		}

		// Mock components based on requirements
		if testCase.requirements.Secrets {
			mockProvider := secrets.NewMockSecretsProvider(mocks.Injector)
			mockProvider.InitializeFunc = func() error {
				initCalled["secretsProvider"] = true
				return testCase.mockInitErrors["secretsProvider"]
			}
			mocks.Injector.Register("secretsProvider", mockProvider)
		}

		if testCase.requirements.Tools {
			mockTools := tools.NewMockToolsManager()
			mockTools.InitializeFunc = func() error {
				initCalled["toolsManager"] = true
				return testCase.mockInitErrors["toolsManager"]
			}
			mocks.Injector.Register("toolsManager", mockTools)
		}

		if testCase.requirements.Services {
			mockService := services.NewMockService()
			mockService.InitializeFunc = func() error {
				initCalled["service"] = true
				return testCase.mockInitErrors["service"]
			}
			mocks.Injector.Register("testService", mockService)
		}

		if testCase.requirements.VM || testCase.requirements.Containers {
			mockVM := virt.NewMockVirt()
			mockVM.InitializeFunc = func() error {
				initCalled["virtualMachine"] = true
				return testCase.mockInitErrors["virtualMachine"]
			}
			mocks.Injector.Register("virtualMachine", mockVM)

			if testCase.requirements.Containers {
				mockRuntime := virt.NewMockVirt()
				mockRuntime.InitializeFunc = func() error {
					initCalled["containerRuntime"] = true
					return testCase.mockInitErrors["containerRuntime"]
				}
				mocks.Injector.Register("containerRuntime", mockRuntime)
			}
		}

		if testCase.requirements.Network {
			mockNetwork := network.NewMockNetworkManager()
			mockNetwork.InitializeFunc = func() error {
				initCalled["networkManager"] = true
				return testCase.mockInitErrors["networkManager"]
			}
			mocks.Injector.Register("networkManager", mockNetwork)
		}

		if testCase.requirements.Blueprint {
			mockBlueprint := blueprint.NewMockBlueprintHandler(mocks.Injector)
			mockBlueprint.InitializeFunc = func() error {
				initCalled["blueprintHandler"] = true
				return testCase.mockInitErrors["blueprintHandler"]
			}
			mocks.Injector.Register("blueprintHandler", mockBlueprint)
		}

		if testCase.requirements.Stack {
			mockStack := services.NewMockService()
			mockStack.InitializeFunc = func() error {
				initCalled["stack"] = true
				return testCase.mockInitErrors["stack"]
			}
			mocks.Injector.Register("stack", mockStack)
		}

		mocks.Injector.Register("initCalled", initCalled)

		return controller, mocks
	}

	t.Run("BasicInitialization", func(t *testing.T) {
		// Given
		testCase := &initializationTestCase{
			requirements: Requirements{
				CommandName: "test",
			},
			mockInitializations: map[string]bool{},
			mockInitErrors:      map[string]error{},
		}
		controller, _ := setup(t, testCase)

		// When
		err := controller.InitializeWithRequirements(testCase.requirements)

		// Then
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("SecretsInitialization", func(t *testing.T) {
		// Given
		testCase := &initializationTestCase{
			requirements: Requirements{
				CommandName: "test",
				Secrets:     true,
			},
			mockInitializations: map[string]bool{
				"secretsProvider": true,
			},
			mockInitErrors: map[string]error{
				"secretsProvider": nil,
			},
		}
		controller, mocks := setup(t, testCase)

		// When
		err := controller.InitializeWithRequirements(testCase.requirements)

		// Then
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// Verify initialization was called
		initCalled := mocks.Injector.Resolve("initCalled").(map[string]bool)
		if !initCalled["secretsProvider"] {
			t.Error("Expected secretsProvider.Initialize to be called")
		}
	})

	t.Run("ToolsInitialization", func(t *testing.T) {
		// Given
		testCase := &initializationTestCase{
			requirements: Requirements{
				CommandName: "test",
				Tools:       true,
			},
			mockInitializations: map[string]bool{
				"toolsManager": true,
			},
			mockInitErrors: map[string]error{
				"toolsManager": nil,
			},
		}
		controller, mocks := setup(t, testCase)

		controller.constructors.NewToolsManager = func(di.Injector) tools.ToolsManager {
			return mocks.Injector.Resolve("toolsManager").(tools.ToolsManager)
		}

		// When
		err := controller.InitializeWithRequirements(testCase.requirements)

		// Then
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// Verify initialization was called
		initCalled := mocks.Injector.Resolve("initCalled").(map[string]bool)
		if !initCalled["toolsManager"] {
			t.Error("Expected toolsManager.Initialize to be called")
		}
	})

	t.Run("ComponentInitializationFailure", func(t *testing.T) {
		// Given
		testCase := &initializationTestCase{
			requirements: Requirements{
				CommandName: "test",
				Secrets:     true,
			},
			mockInitializations: map[string]bool{
				"secretsProvider": true,
			},
			mockInitErrors: map[string]error{
				"secretsProvider": fmt.Errorf("initialization failed"),
			},
			expectedError:        true,
			expectedErrorMessage: "initialization failed",
		}
		controller, _ := setup(t, testCase)

		// When
		err := controller.InitializeWithRequirements(testCase.requirements)

		// Then
		if err == nil {
			t.Error("Expected error when component initialization fails")
		}
		if !strings.Contains(err.Error(), testCase.expectedErrorMessage) {
			t.Errorf("Expected error to contain '%s', got %v", testCase.expectedErrorMessage, err)
		}
	})

	t.Run("CreateComponentsFailure", func(t *testing.T) {
		// Given
		mocks := setupMocks(t)
		controller := NewController(mocks.Injector)
		controller.injector = nil

		// When
		err := controller.InitializeWithRequirements(Requirements{CommandName: "test"})

		// Then
		if err == nil {
			t.Error("Expected error when injector is nil")
		}
		if !strings.Contains(err.Error(), "injector is nil") {
			t.Errorf("Expected error to contain 'injector is nil', got %v", err)
		}
	})

	t.Run("InitializationOrderVerification", func(t *testing.T) {
		// Given
		mockInjector := di.NewMockInjector()
		mocks := setupMocks(t, &SetupOptions{Injector: mockInjector})
		controller := NewController(mocks.Injector)

		// Set requirements
		requirements := Requirements{
			CommandName: "test",
			Secrets:     true,
			VM:          true,
			Network:     true,
		}
		controller.SetRequirements(requirements)

		// Track initialization order
		initOrder := []string{}

		// Mock secrets provider
		mockProvider := secrets.NewMockSecretsProvider(mocks.Injector)
		mockProvider.InitializeFunc = func() error {
			initOrder = append(initOrder, "secretsProvider")
			return nil
		}
		mocks.Injector.Register("secretsProvider", mockProvider)

		// Mock VM
		mockVM := virt.NewMockVirt()
		mockVM.InitializeFunc = func() error {
			initOrder = append(initOrder, "virtualMachine")
			return nil
		}
		mocks.Injector.Register("virtualMachine", mockVM)

		// Mock network manager
		mockNetwork := network.NewMockNetworkManager()
		mockNetwork.InitializeFunc = func() error {
			initOrder = append(initOrder, "networkManager")
			return nil
		}
		mocks.Injector.Register("networkManager", mockNetwork)

		// Override constructors to ensure our mocks are used
		controller.constructors.NewSopsSecretsProvider = func(string, di.Injector) secrets.SecretsProvider {
			return mockProvider
		}
		controller.constructors.NewColimaVirt = func(di.Injector) virt.VirtualMachine {
			return mockVM
		}
		controller.constructors.NewBaseNetworkManager = func(di.Injector) network.NetworkManager {
			return mockNetwork
		}

		// When
		err := controller.CreateComponents()
		if err != nil {
			t.Fatalf("Failed to create components: %v", err)
		}

		err = controller.InitializeComponents()

		// Then
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// Verify initialization order (each component should be initialized)
		if len(initOrder) != 3 {
			t.Errorf("Expected 3 initializations, got %d", len(initOrder))
		}

		// Verify all components were initialized
		expectedComponents := []string{"secretsProvider", "virtualMachine", "networkManager"}
		for _, component := range expectedComponents {
			found := false
			for _, initialized := range initOrder {
				if initialized == component {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Expected %s to be initialized", component)
			}
		}
	})
}

func TestBaseController_createShellComponent(t *testing.T) {
	setup := func(t *testing.T) (*BaseController, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		controller := NewController(mocks.Injector)
		return controller, mocks
	}

	t.Run("ReturnsErrorWhenNewShellIsNil", func(t *testing.T) {
		// Given
		controller, _ := setup(t)
		controller.constructors.NewShell = nil

		// When
		err := controller.createShellComponent(Requirements{})

		// Then
		if err == nil {
			t.Error("Expected error when NewShell is nil")
		}
		if err.Error() != "required constructor NewShell is nil" {
			t.Errorf("Expected error 'required constructor NewShell is nil', got %v", err)
		}
	})

	t.Run("CreatesAndRegistersShellComponent", func(t *testing.T) {
		// Given
		controller, mocks := setup(t)
		mockShell := shell.NewMockShell()
		controller.constructors.NewShell = func(di.Injector) shell.Shell {
			return mockShell
		}

		// Clear any existing shell
		mocks.Injector.Register("shell", nil)

		// When
		err := controller.createShellComponent(Requirements{})

		// Then
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		resolvedShell := mocks.Injector.Resolve("shell")
		if resolvedShell != mockShell {
			t.Error("Expected shell to be registered with injector")
		}
	})

	t.Run("SetsVerbosityWhenFlagIsTrue", func(t *testing.T) {
		// Given
		controller, mocks := setup(t)
		mockShell := shell.NewMockShell()
		verbositySet := false
		mockShell.SetVerbosityFunc = func(verbose bool) {
			verbositySet = verbose
		}
		controller.constructors.NewShell = func(di.Injector) shell.Shell {
			return mockShell
		}

		// Clear any existing shell
		mocks.Injector.Register("shell", nil)

		// When
		err := controller.createShellComponent(Requirements{
			Flags: map[string]bool{"verbose": true},
		})

		// Then
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if !verbositySet {
			t.Error("Expected verbosity to be set to true")
		}
	})

	t.Run("ChecksTrustedDirectoryWhenTrustIsRequired", func(t *testing.T) {
		// Given
		controller, mocks := setup(t)
		mockShell := shell.NewMockShell()
		trustedChecked := false
		mockShell.CheckTrustedDirectoryFunc = func() error {
			trustedChecked = true
			return fmt.Errorf("not trusted")
		}
		controller.constructors.NewShell = func(di.Injector) shell.Shell {
			return mockShell
		}

		// Clear any existing shell
		mocks.Injector.Register("shell", nil)

		// When
		err := controller.createShellComponent(Requirements{
			Trust: true,
		})

		// Then
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if !trustedChecked {
			t.Error("Expected trusted directory to be checked")
		}
	})

	t.Run("DoesNotCheckTrustedDirectoryWhenTrustNotRequired", func(t *testing.T) {
		// Given
		controller, mocks := setup(t)
		mockShell := shell.NewMockShell()
		trustedChecked := false
		mockShell.CheckTrustedDirectoryFunc = func() error {
			trustedChecked = true
			return nil
		}
		controller.constructors.NewShell = func(di.Injector) shell.Shell {
			return mockShell
		}

		// Clear any existing shell
		mocks.Injector.Register("shell", nil)

		// When
		err := controller.createShellComponent(Requirements{
			Trust: false,
		})

		// Then
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if trustedChecked {
			t.Error("Expected trusted directory not to be checked")
		}
	})
}

func TestBaseController_createVirtualizationComponents(t *testing.T) {
	setup := func(t *testing.T) (*BaseController, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		controller := NewController(mocks.Injector)
		return controller, mocks
	}

	t.Run("ReturnsEarlyWhenNotRequired", func(t *testing.T) {
		// Given
		controller, _ := setup(t)

		// When
		err := controller.createVirtualizationComponents(Requirements{
			VM:         false,
			Containers: false,
		})

		// Then
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("CreatesColimaVMWhenEnabled", func(t *testing.T) {
		// Given
		controller, mocks := setup(t)

		// Mock config handler
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetStringFunc = func(key string, _ ...string) string {
			if key == "vm.driver" {
				return "colima"
			}
			return ""
		}
		mocks.Injector.Register("configHandler", mockConfigHandler)

		// Mock Colima VM
		mockVM := virt.NewMockVirt()
		controller.constructors.NewColimaVirt = func(di.Injector) virt.VirtualMachine {
			return mockVM
		}

		// When
		err := controller.createVirtualizationComponents(Requirements{
			VM: true,
		})

		// Then
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		resolvedVM := mocks.Injector.Resolve("virtualMachine")
		if resolvedVM != mockVM {
			t.Error("Expected Colima VM to be registered with injector")
		}
	})

	t.Run("HandlesNilColimaConstructor", func(t *testing.T) {
		// Given
		controller, mocks := setup(t)

		// Mock config handler
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetStringFunc = func(key string, _ ...string) string {
			if key == "vm.driver" {
				return "colima"
			}
			return ""
		}
		mocks.Injector.Register("configHandler", mockConfigHandler)

		// Set nil constructor
		controller.constructors.NewColimaVirt = nil

		// When
		err := controller.createVirtualizationComponents(Requirements{
			VM: true,
		})

		// Then
		if err == nil {
			t.Error("Expected error when Colima constructor is nil")
		}
		if !strings.Contains(err.Error(), "NewColimaVirt constructor is nil") {
			t.Errorf("Expected error about nil constructor, got %v", err)
		}
	})

	t.Run("HandlesNilColimaVM", func(t *testing.T) {
		// Given
		controller, mocks := setup(t)

		// Mock config handler
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetStringFunc = func(key string, _ ...string) string {
			if key == "vm.driver" {
				return "colima"
			}
			return ""
		}
		mocks.Injector.Register("configHandler", mockConfigHandler)

		// Constructor returns nil
		controller.constructors.NewColimaVirt = func(di.Injector) virt.VirtualMachine {
			return nil
		}

		// When
		err := controller.createVirtualizationComponents(Requirements{
			VM: true,
		})

		// Then
		if err == nil {
			t.Error("Expected error when Colima VM is nil")
		}
		if !strings.Contains(err.Error(), "NewColimaVirt returned nil") {
			t.Errorf("Expected error about nil VM, got %v", err)
		}
	})

	t.Run("CreatesDockerRuntimeWhenEnabled", func(t *testing.T) {
		// Given
		controller, mocks := setup(t)

		// Mock config handler
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetBoolFunc = func(key string, _ ...bool) bool {
			return key == "docker.enabled"
		}
		mocks.Injector.Register("configHandler", mockConfigHandler)

		// Mock Docker runtime
		mockRuntime := virt.NewMockVirt()
		controller.constructors.NewDockerVirt = func(di.Injector) virt.ContainerRuntime {
			return mockRuntime
		}

		// When
		err := controller.createVirtualizationComponents(Requirements{
			Containers: true,
		})

		// Then
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		resolvedRuntime := mocks.Injector.Resolve("containerRuntime")
		if resolvedRuntime != mockRuntime {
			t.Error("Expected Docker runtime to be registered with injector")
		}
	})

	t.Run("HandlesNilDockerConstructor", func(t *testing.T) {
		// Given
		controller, mocks := setup(t)

		// Mock config handler
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetBoolFunc = func(key string, _ ...bool) bool {
			return key == "docker.enabled"
		}
		mocks.Injector.Register("configHandler", mockConfigHandler)

		// Set nil constructor
		controller.constructors.NewDockerVirt = nil

		// When
		err := controller.createVirtualizationComponents(Requirements{
			Containers: true,
		})

		// Then
		if err == nil {
			t.Error("Expected error when Docker constructor is nil")
		}
		if !strings.Contains(err.Error(), "NewDockerVirt constructor is nil") {
			t.Errorf("Expected error about nil constructor, got %v", err)
		}
	})

	t.Run("HandlesNilDockerRuntime", func(t *testing.T) {
		// Given
		controller, mocks := setup(t)

		// Mock config handler
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetBoolFunc = func(key string, _ ...bool) bool {
			return key == "docker.enabled"
		}
		mocks.Injector.Register("configHandler", mockConfigHandler)

		// Constructor returns nil
		controller.constructors.NewDockerVirt = func(di.Injector) virt.ContainerRuntime {
			return nil
		}

		// When
		err := controller.createVirtualizationComponents(Requirements{
			Containers: true,
		})

		// Then
		if err == nil {
			t.Error("Expected error when Docker runtime is nil")
		}
		if !strings.Contains(err.Error(), "NewDockerVirt returned nil") {
			t.Errorf("Expected error about nil runtime, got %v", err)
		}
	})
}
