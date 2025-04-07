package controller

import (
	"testing"

	"github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/api/v1alpha1/docker"
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

func TestMockController_Initialize(t *testing.T) {
	t.Run("Initialize", func(t *testing.T) {
		// Given a new injector and mock controller
		mocks := setSafeControllerMocks()
		mockCtrl := NewMockController(mocks.Injector)
		// And the InitializeFunc is set to return nil
		mockCtrl.InitializeFunc = func() error {
			return nil
		}
		// When Initialize is called
		if err := mockCtrl.Initialize(); err != nil {
			// Then no error should be returned
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("NoInitializeFunc", func(t *testing.T) {
		// Given a new injector and mock controller
		mocks := setSafeControllerMocks()
		mockCtrl := NewMockController(mocks.Injector)
		// When Initialize is called without setting InitializeFunc
		if err := mockCtrl.Initialize(); err != nil {
			// Then no error should be returned
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestMockController_InitializeComponents(t *testing.T) {
	t.Run("InitializeComponents", func(t *testing.T) {
		// Given a new injector and mock controller
		mocks := setSafeControllerMocks()
		mockCtrl := NewMockController(mocks.Injector)

		// Initialize the controller
		mockCtrl.Initialize()

		// And the InitializeComponentsFunc is set to return nil
		mockCtrl.InitializeComponentsFunc = func() error {
			return nil
		}
		// When InitializeComponents is called
		if err := mockCtrl.InitializeComponents(); err != nil {
			// Then no error should be returned
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestMockController_CreateCommonComponents(t *testing.T) {
	t.Run("CreateCommonComponents", func(t *testing.T) {
		// Given a new injector and mock controller
		mocks := setSafeControllerMocks()
		mockCtrl := NewMockController(mocks.Injector)
		// And the CreateCommonComponentsFunc is set to return nil
		mockCtrl.CreateCommonComponentsFunc = func() error {
			return nil
		}
		// When CreateCommonComponents is called
		if err := mockCtrl.CreateCommonComponents(); err != nil {
			// Then no error should be returned
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("NoCreateCommonComponentsFunc", func(t *testing.T) {
		// Given a new injector and mock controller
		mocks := setSafeControllerMocks()
		mockCtrl := NewMockController(mocks.Injector)
		// When CreateCommonComponents is called without setting CreateCommonComponentsFunc
		if err := mockCtrl.CreateCommonComponents(); err != nil {
			// Then no error should be returned
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestMockController_CreateSecretsProviders(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new injector and mock controller
		mocks := setSafeControllerMocks()
		mockCtrl := NewMockController(mocks.Injector)
		// And the CreateSecretsProvidersFunc is set to return nil
		mockCtrl.CreateSecretsProvidersFunc = func() error {
			return nil
		}
		// When CreateSecretsProviders is called
		if err := mockCtrl.CreateSecretsProviders(); err != nil {
			// Then no error should be returned
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("NoCreateSecretsProvidersFunc", func(t *testing.T) {
		// Given a new injector and mock controller
		mocks := setSafeControllerMocks()
		mockCtrl := NewMockController(mocks.Injector)
		// When CreateSecretsProviders is called without setting CreateSecretsProvidersFunc
		if err := mockCtrl.CreateSecretsProviders(); err != nil {
			// Then no error should be returned
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestMockController_CreateProjectComponents(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new injector and mock controller
		mocks := setSafeControllerMocks()
		mockCtrl := NewMockController(mocks.Injector)
		// And the CreateProjectComponentsFunc is set to return nil
		mockCtrl.CreateProjectComponentsFunc = func() error {
			return nil
		}
		// When CreateProjectComponents is called
		if err := mockCtrl.CreateProjectComponents(); err != nil {
			// Then no error should be returned
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("DefaultCreateProjectComponents", func(t *testing.T) {
		// Given a new injector and a mock controller
		mocks := setSafeControllerMocks()
		mockCtrl := NewMockController(mocks.Injector)
		// When CreateProjectComponents is invoked without setting CreateProjectComponentsFunc
		if err := mockCtrl.CreateProjectComponents(); err != nil {
			// Then no error should be returned
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestMockController_CreateEnvComponents(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new injector and mock controller
		mocks := setSafeControllerMocks()
		mockCtrl := NewMockController(mocks.Injector)
		// And the CreateEnvComponentsFunc is set to return nil
		mockCtrl.CreateEnvComponentsFunc = func() error {
			return nil
		}
		// When CreateEnvComponents is called
		if err := mockCtrl.CreateEnvComponents(); err != nil {
			// Then no error should be returned
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("NoCreateEnvComponentsFunc", func(t *testing.T) {
		// Given a new injector and mock controller
		mocks := setSafeControllerMocks()
		mockCtrl := NewMockController(mocks.Injector)
		mockCtrl.CreateCommonComponents()

		// When CreateEnvComponents is called without setting CreateEnvComponentsFunc
		if err := mockCtrl.CreateEnvComponents(); err != nil {
			// Then no error should be returned
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestMockController_CreateServiceComponents(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new injector and mock controller
		mocks := setSafeControllerMocks()
		mockCtrl := NewMockController(mocks.Injector)
		// And the CreateServiceComponentsFunc is set to return nil
		mockCtrl.CreateServiceComponentsFunc = func() error {
			return nil
		}
		// When CreateServiceComponents is called
		if err := mockCtrl.CreateServiceComponents(); err != nil {
			// Then no error should be returned
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("NoCreateServiceComponentsFunc", func(t *testing.T) {
		// Given a new injector and mock controller
		mocks := setSafeControllerMocks()
		mockCtrl := NewMockController(mocks.Injector)
		// And a mock config handler is created and assigned to the controller
		mockConfigHandler := config.NewMockConfigHandler()
		mockCtrl.configHandler = mockConfigHandler

		// And the mock config handler is set to return specific values for certain keys
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

		mockConfigHandler.GetConfigFunc = func() *v1alpha1.Context {
			return &v1alpha1.Context{
				Docker: &docker.DockerConfig{
					Registries: map[string]docker.RegistryConfig{
						"registry1": {Remote: "registry1"},
						"registry2": {Remote: "registry2"},
					},
				},
			}
		}

		// When CreateServiceComponents is called
		if err := mockCtrl.CreateServiceComponents(); err != nil {
			// Then no error should be returned
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestMockController_CreateVirtualizationComponents(t *testing.T) {
	t.Run("CreateVirtualizationComponents", func(t *testing.T) {
		// Given a new injector and mock controller
		mocks := setSafeControllerMocks()
		mockCtrl := NewMockController(mocks.Injector)
		// And the CreateVirtualizationComponentsFunc is set to return nil
		mockCtrl.CreateVirtualizationComponentsFunc = func() error {
			return nil
		}
		// When CreateVirtualizationComponents is called
		if err := mockCtrl.CreateVirtualizationComponents(); err != nil {
			// Then no error should be returned
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("NoCreateVirtualizationComponentsFunc", func(t *testing.T) {
		// Given a new injector and mock controller
		mocks := setSafeControllerMocks()
		mockCtrl := NewMockController(mocks.Injector)
		// And a mock config handler is created and assigned to the controller
		mockConfigHandler := config.NewMockConfigHandler()
		mockCtrl.configHandler = mockConfigHandler
		// And the mock config handler is set to return specific values for certain keys
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "vm.driver" {
				return "colima"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}
		// When CreateVirtualizationComponents is called
		if err := mockCtrl.CreateVirtualizationComponents(); err != nil {
			// Then no error should be returned
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestMockController_CreateStackComponents(t *testing.T) {
	t.Run("CreateStackComponents", func(t *testing.T) {
		// Given a new injector and mock controller
		mocks := setSafeControllerMocks()
		mockCtrl := NewMockController(mocks.Injector)
		// And the CreateStackComponentsFunc is set to return nil
		mockCtrl.CreateStackComponentsFunc = func() error {
			return nil
		}
		// When CreateStackComponents is called
		if err := mockCtrl.CreateStackComponents(); err != nil {
			// Then no error should be returned
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("NoCreateStackComponentsFunc", func(t *testing.T) {
		// Given a new injector and mock controller
		mocks := setSafeControllerMocks()
		mockCtrl := NewMockController(mocks.Injector)
		// When CreateStackComponents is called without setting CreateStackComponentsFunc
		if err := mockCtrl.CreateStackComponents(); err != nil {
			// Then no error should be returned
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestMockController_WriteConfigurationFiles(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new injector and mock controller
		mocks := setSafeControllerMocks()
		mockCtrl := NewMockController(mocks.Injector)
		// And the WriteConfigurationFilesFunc is set to return nil
		mockCtrl.WriteConfigurationFilesFunc = func() error {
			// Validate that the WriteConfigFunc is called
			if mockCtrl.WriteConfigurationFilesFunc == nil {
				t.Fatalf("expected WriteConfigurationFilesFunc to be set")
			}
			return nil
		}
		// When WriteConfigurationFiles is called
		if err := mockCtrl.WriteConfigurationFiles(); err != nil {
			// Then no error should be returned
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("NoWriteConfigurationFilesFunc", func(t *testing.T) {
		// Given a new injector and mock controller
		mocks := setSafeControllerMocks()
		mockCtrl := NewMockController(mocks.Injector)
		// When WriteConfigurationFiles is called without setting WriteConfigurationFilesFunc
		if err := mockCtrl.WriteConfigurationFiles(); err != nil {
			// Then no error should be returned
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestMockController_ResolveInjector(t *testing.T) {
	t.Run("ResolveInjector", func(t *testing.T) {
		// Given a new mock injector and mock controller
		mocks := setSafeControllerMocks()
		mockCtrl := NewMockController(mocks.Injector)
		// And the ResolveInjectorFunc is set to return the expected injector
		mockCtrl.ResolveInjectorFunc = func() di.Injector {
			return mocks.Injector
		}
		// When ResolveInjector is called
		if injector := mockCtrl.ResolveInjector(); injector != mocks.Injector {
			// Then the returned injector should be the expected injector
			t.Fatalf("expected %v, got %v", mocks.Injector, injector)
		}
	})

	t.Run("NoResolveInjectorFunc", func(t *testing.T) {
		// Given a new mock injector and mock controller
		mocks := setSafeControllerMocks()
		mockCtrl := NewMockController(mocks.Injector)
		// When ResolveInjector is called without setting ResolveInjectorFunc
		if injector := mockCtrl.ResolveInjector(); injector != mocks.Injector {
			// Then the returned injector should be the same as the created injector
			t.Fatalf("expected %v, got %v", mocks.Injector, injector)
		}
	})
}

func TestMockController_ResolveConfigHandler(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new mock config handler, mock injector, and mock controller
		mocks := setSafeControllerMocks()
		mockCtrl := NewMockController(mocks.Injector)
		// And the ResolveConfigHandlerFunc is set to return the expected config handler
		mockCtrl.ResolveConfigHandlerFunc = func() config.ConfigHandler {
			return mocks.ConfigHandler
		}
		// When ResolveConfigHandler is called
		configHandler := mockCtrl.ResolveConfigHandler()
		if configHandler != mocks.ConfigHandler {
			// Then the returned config handler should be the expected config handler
			t.Fatalf("expected %v, got %v", mocks.ConfigHandler, configHandler)
		}
	})

	t.Run("NoResolveConfigHandlerFunc", func(t *testing.T) {
		// Given a new mock injector and mock controller
		mocks := setSafeControllerMocks()
		mockCtrl := NewMockController(mocks.Injector)
		// When ResolveConfigHandler is called without setting ResolveConfigHandlerFunc
		configHandler := mockCtrl.ResolveConfigHandler()
		if configHandler != mocks.ConfigHandler {
			// Then the returned config handler should be the same as the created config handler
			t.Fatalf("expected %v, got %v", mocks.ConfigHandler, configHandler)
		}
	})
}

func TestMockController_ResolveAllSecretsProviders(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new mock secrets provider, mock injector, and mock controller
		mocks := setSafeControllerMocks()
		mockCtrl := NewMockController(mocks.Injector)
		// And the ResolveAllSecretsProvidersFunc is set to return the expected secrets provider
		mockCtrl.ResolveAllSecretsProvidersFunc = func() []secrets.SecretsProvider {
			return []secrets.SecretsProvider{mocks.SecretsProvider}
		}
		// When ResolveAllSecretsProviders is called
		secretsProviders := mockCtrl.ResolveAllSecretsProviders()
		if len(secretsProviders) != 1 {
			// Then the returned secrets provider should be the expected secrets provider
			t.Fatalf("expected %v, got %v", 1, len(secretsProviders))
		}
		if secretsProviders[0] != mocks.SecretsProvider {
			// Then the returned secrets provider should be the expected secrets provider
			t.Fatalf("expected %v, got %v", mocks.SecretsProvider, secretsProviders[0])
		}
	})

	t.Run("NoResolveAllSecretsProvidersFunc", func(t *testing.T) {
		// Given a new mock injector and mock controller
		mocks := setSafeControllerMocks()
		mockCtrl := NewMockController(mocks.Injector)
		// When ResolveAllSecretsProviders is called without setting ResolveAllSecretsProvidersFunc
		secretsProviders := mockCtrl.ResolveAllSecretsProviders()
		if len(secretsProviders) != 1 {
			// Then the returned secrets provider should be the same as the created secrets provider
			t.Fatalf("expected %v, got %v", 1, len(secretsProviders))
		}
	})
}

func TestMockController_ResolveEnvPrinter(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new mock env printer, mock injector, and mock controller
		mocks := setSafeControllerMocks()
		mockCtrl := NewMockController(mocks.Injector)
		// And the ResolveEnvPrinterFunc is set to return the expected env printer
		mockCtrl.ResolveEnvPrinterFunc = func(name string) env.EnvPrinter {
			return mocks.EnvPrinter
		}
		// When ResolveEnvPrinter is called
		envPrinter := mockCtrl.ResolveEnvPrinter("envPrinter1")
		if envPrinter != mocks.EnvPrinter {
			// Then the returned env printer should be the expected env printer
			t.Fatalf("expected %v, got %v", mocks.EnvPrinter, envPrinter)
		}
	})

	t.Run("NoResolveEnvPrinterFunc", func(t *testing.T) {
		// Given a new mock injector and mock controller
		mocks := setSafeControllerMocks()
		mockCtrl := NewMockController(mocks.Injector)
		// When ResolveEnvPrinter is called without setting ResolveEnvPrinterFunc
		envPrinter := mockCtrl.ResolveEnvPrinter("envPrinter1")
		if envPrinter != mocks.EnvPrinter {
			// Then the returned env printer should be the same as the created env printer
			t.Fatalf("expected %v, got %v", mocks.EnvPrinter, envPrinter)
		}
	})
}

func TestMockController_ResolveAllEnvPrinters(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new mock injector and mock controller
		mocks := setSafeControllerMocks()
		mockCtrl := NewMockController(mocks.Injector)
		// And the ResolveAllEnvPrintersFunc is set to return a list of mock env printers
		mockCtrl.ResolveAllEnvPrintersFunc = func() []env.EnvPrinter {
			return []env.EnvPrinter{mocks.EnvPrinter, mocks.EnvPrinter} // Use the same mock EnvPrinter
		}
		// When ResolveAllEnvPrinters is called
		envPrinters := mockCtrl.ResolveAllEnvPrinters()
		if len(envPrinters) != 2 {
			// Then the length of the returned env printers list should be 2
			t.Fatalf("expected %v, got %v", 2, len(envPrinters))
		}
	})

	t.Run("NoResolveAllEnvPrintersFunc", func(t *testing.T) {
		// Given a new mock injector and mock controller
		mocks := setSafeControllerMocks()
		mockCtrl := NewMockController(mocks.Injector)
		// When ResolveAllEnvPrinters is called without setting ResolveAllEnvPrintersFunc
		envPrinters := mockCtrl.ResolveAllEnvPrinters()
		if len(envPrinters) != 3 {
			// Then the length of the returned env printers list should be 0
			t.Fatalf("expected %v, got %v", 0, len(envPrinters))
		}
	})
}

func TestMockController_ResolveShell(t *testing.T) {
	t.Run("ResolveShell", func(t *testing.T) {
		// Given a new mock shell, mock injector, and mock controller
		mocks := setSafeControllerMocks()
		mockCtrl := NewMockController(mocks.Injector)
		// And the ResolveShellFunc is set to return the expected shell
		mockCtrl.ResolveShellFunc = func() shell.Shell {
			return mocks.Shell
		}
		// When ResolveShell is called
		shellInstance := mockCtrl.ResolveShell()
		if shellInstance != mocks.Shell {
			// Then the returned shell should be the expected shell
			t.Fatalf("expected %v, got %v", mocks.Shell, shellInstance)
		}
	})

	t.Run("NoResolveShellFunc", func(t *testing.T) {
		// Given a new mock injector and mock controller
		mocks := setSafeControllerMocks()
		mockCtrl := NewMockController(mocks.Injector)
		// When ResolveShell is called without setting ResolveShellFunc
		shellInstance := mockCtrl.ResolveShell()
		if shellInstance != mocks.Shell {
			// Then the returned shell should be the same as the created shell
			t.Fatalf("expected %v, got %v", mocks.Shell, shellInstance)
		}
	})
}

func TestMockController_ResolveSecureShell(t *testing.T) {
	t.Run("ResolveSecureShell", func(t *testing.T) {
		// Given a new mock secure shell, mock injector, and mock controller
		mocks := setSafeControllerMocks()
		mockCtrl := NewMockController(mocks.Injector)
		// And the ResolveSecureShellFunc is set to return the expected secure shell
		mockCtrl.ResolveSecureShellFunc = func() shell.Shell {
			return mocks.SecureShell
		}
		// When ResolveSecureShell is called
		secureShell := mockCtrl.ResolveSecureShell()
		if secureShell != mocks.SecureShell {
			// Then the returned secure shell should be the expected secure shell
			t.Fatalf("expected %v, got %v", mocks.SecureShell, secureShell)
		}
	})

	t.Run("NoResolveSecureShellFunc", func(t *testing.T) {
		// Given a new mock injector and mock controller
		mocks := setSafeControllerMocks()
		mockCtrl := NewMockController(mocks.Injector)
		// When ResolveSecureShell is called without setting ResolveSecureShellFunc
		secureShell := mockCtrl.ResolveSecureShell()
		if secureShell != mocks.SecureShell {
			// Then the returned secure shell should be the same as the created secure shell
			t.Fatalf("expected %v, got %v", mocks.SecureShell, secureShell)
		}
	})
}

func TestMockController_ResolveToolsManager(t *testing.T) {
	t.Run("ResolveToolsManager", func(t *testing.T) {
		// Given a new mock tools manager, mock injector, and mock controller
		mocks := setSafeControllerMocks()
		mockCtrl := NewMockController(mocks.Injector)
		// And the ResolveToolsManagerFunc is set to return the expected tools manager
		mockCtrl.ResolveToolsManagerFunc = func() tools.ToolsManager {
			return mocks.ToolsManager
		}
		// When ResolveToolsManager is called
		toolsManager := mockCtrl.ResolveToolsManager()
		if toolsManager != mocks.ToolsManager {
			// Then the returned tools manager should be the expected tools manager
			t.Fatalf("expected %v, got %v", mocks.ToolsManager, toolsManager)
		}
	})

	t.Run("NoResolveToolsManagerFunc", func(t *testing.T) {
		// Given a new mock injector and mock controller
		mocks := setSafeControllerMocks()
		mockCtrl := NewMockController(mocks.Injector)
		// When ResolveToolsManager is called without setting ResolveToolsManagerFunc
		toolsManager := mockCtrl.ResolveToolsManager()
		if toolsManager != mocks.ToolsManager {
			// Then the returned tools manager should be the same as the created tools manager
			t.Fatalf("expected %v, got %v", mocks.ToolsManager, toolsManager)
		}
	})
}

func TestMockController_ResolveNetworkManager(t *testing.T) {
	t.Run("ResolveNetworkManager", func(t *testing.T) {
		// Given a new mock network manager, mock injector, and mock controller
		mocks := setSafeControllerMocks()
		mockCtrl := NewMockController(mocks.Injector)
		// And the ResolveNetworkManagerFunc is set to return the expected network manager
		mockCtrl.ResolveNetworkManagerFunc = func() network.NetworkManager {
			return mocks.NetworkManager
		}
		// When ResolveNetworkManager is called
		networkManager := mockCtrl.ResolveNetworkManager()
		if networkManager != mocks.NetworkManager {
			// Then the returned network manager should be the expected network manager
			t.Fatalf("expected %v, got %v", mocks.NetworkManager, networkManager)
		}
	})

	t.Run("NoResolveNetworkManagerFunc", func(t *testing.T) {
		// Given a new mock injector and mock controller
		mocks := setSafeControllerMocks()
		mockCtrl := NewMockController(mocks.Injector)
		// When ResolveNetworkManager is called without setting ResolveNetworkManagerFunc
		networkManager := mockCtrl.ResolveNetworkManager()
		if networkManager != mocks.NetworkManager {
			// Then the returned network manager should be the same as the created network manager
			t.Fatalf("expected %v, got %v", mocks.NetworkManager, networkManager)
		}
	})
}

func TestMockController_ResolveService(t *testing.T) {
	t.Run("ResolveService", func(t *testing.T) {
		// Given a new mock service, mock injector, and mock controller
		mocks := setSafeControllerMocks()
		mockCtrl := NewMockController(mocks.Injector)
		// And the ResolveServiceFunc is set to return the expected service
		mockCtrl.ResolveServiceFunc = func(name string) services.Service {
			return mocks.Service
		}
		// When ResolveService is called
		service := mockCtrl.ResolveService("service")
		if service != mocks.Service {
			// Then the returned service should be the expected service
			t.Fatalf("expected %v, got %v", mocks.Service, service)
		}
	})

	t.Run("NoResolveServiceFunc", func(t *testing.T) {
		// Given a new mock injector and mock controller
		mocks := setSafeControllerMocks()
		mockCtrl := NewMockController(mocks.Injector)
		// When ResolveService is called without setting ResolveServiceFunc
		service := mockCtrl.ResolveService("service1")
		// Then the returned service should be the one resolved by the base controller
		expectedService := mockCtrl.BaseController.ResolveService("service1")
		if service != expectedService {
			t.Fatalf("expected %v, got %v", expectedService, service)
		}
	})
}

func TestMockController_ResolveAllServices(t *testing.T) {
	t.Run("ResolveAllServices", func(t *testing.T) {
		// Given a new mock injector and mock controller
		mocks := setSafeControllerMocks()
		mockCtrl := NewMockController(mocks.Injector)
		// And the ResolveAllServicesFunc is set to return a list of mock services
		mockCtrl.ResolveAllServicesFunc = func() []services.Service {
			return []services.Service{mocks.Service, mocks.Service} // Use the same mock Service
		}
		// When ResolveAllServices is called
		services := mockCtrl.ResolveAllServices()
		if len(services) != 2 {
			// Then the length of the returned services list should be the same as the expected services list
			t.Fatalf("expected %v, got %v", 2, len(services))
		}
		for _, service := range services {
			if service != mocks.Service {
				// Then each service in the returned list should match the expected service
				t.Fatalf("expected %v, got %v", mocks.Service, service)
			}
		}
	})

	t.Run("NoResolveAllServicesFunc", func(t *testing.T) {
		// Given a new mock injector and mock controller
		mocks := setSafeControllerMocks()
		mockCtrl := NewMockController(mocks.Injector)
		services := mockCtrl.ResolveAllServices()
		if len(services) != 2 {
			t.Fatalf("expected %v, got %v", 0, len(services))
		}
	})
}

func TestMockController_ResolveVirtualMachine(t *testing.T) {
	t.Run("ResolveVirtualMachine", func(t *testing.T) {
		// Given a new mock injector and mock controller
		mocks := setSafeControllerMocks()
		mockCtrl := NewMockController(mocks.Injector)
		// And the ResolveVirtualMachineFunc is set to return the expected virtual machine
		mockCtrl.ResolveVirtualMachineFunc = func() virt.VirtualMachine {
			return mocks.VirtualMachine
		}
		// When ResolveVirtualMachine is called
		virtualMachine := mockCtrl.ResolveVirtualMachine()
		// Then the returned virtual machine should be the expected virtual machine
		if virtualMachine != mocks.VirtualMachine {
			t.Fatalf("expected %v, got %v", mocks.VirtualMachine, virtualMachine)
		}
	})

	t.Run("NoResolveVirtualMachineFunc", func(t *testing.T) {
		// Given a new mock injector and mock controller
		mocks := setSafeControllerMocks()
		mockCtrl := NewMockController(mocks.Injector)
		// When ResolveVirtualMachine is called without setting ResolveVirtualMachineFunc
		virtualMachine := mockCtrl.ResolveVirtualMachine()
		// Then the returned virtual machine should be the same as the created virtual machine
		if virtualMachine != mocks.VirtualMachine {
			t.Fatalf("expected %v, got %v", mocks.VirtualMachine, virtualMachine)
		}
	})
}

func TestMockController_ResolveContainerRuntime(t *testing.T) {
	t.Run("ResolveContainerRuntime", func(t *testing.T) {
		// Given a new mock injector and mock controller
		mocks := setSafeControllerMocks()
		mockCtrl := NewMockController(mocks.Injector)
		// And the ResolveContainerRuntimeFunc is set to return the expected container runtime
		mockCtrl.ResolveContainerRuntimeFunc = func() virt.ContainerRuntime {
			return mocks.ContainerRuntime
		}
		// When ResolveContainerRuntime is called
		containerRuntime := mockCtrl.ResolveContainerRuntime()
		// Then the returned container runtime should be the expected container runtime
		if containerRuntime != mocks.ContainerRuntime {
			t.Fatalf("expected %v, got %v", mocks.ContainerRuntime, containerRuntime)
		}
	})

	t.Run("NoResolveContainerRuntimeFunc", func(t *testing.T) {
		// Given a new mock injector and mock controller
		mocks := setSafeControllerMocks()
		mockCtrl := NewMockController(mocks.Injector)
		// When ResolveContainerRuntime is called without setting ResolveContainerRuntimeFunc
		containerRuntime := mockCtrl.ResolveContainerRuntime()
		// Then the returned container runtime should be the same as the created container runtime
		if containerRuntime != mocks.ContainerRuntime {
			t.Fatalf("expected %v, got %v", mocks.ContainerRuntime, containerRuntime)
		}
	})
}

func TestMockController_ResolveAllGenerators(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new mock injector and mock controller
		mocks := setSafeControllerMocks()
		mockCtrl := NewMockController(mocks.Injector)
		// And the ResolveAllGeneratorsFunc is set to return a list of mock generators
		mockCtrl.ResolveAllGeneratorsFunc = func() []generators.Generator {
			return []generators.Generator{mocks.Generator}
		}
		// When ResolveAllGenerators is called
		generators := mockCtrl.ResolveAllGenerators()
		// Then the length of the returned generators list should be 1
		if len(generators) != 1 {
			t.Fatalf("expected %v, got %v", 1, len(generators))
		}
	})

	t.Run("NoResolveAllGeneratorsFunc", func(t *testing.T) {
		// Given a new mock injector and mock controller
		mocks := setSafeControllerMocks()
		mockCtrl := NewMockController(mocks.Injector)
		// When ResolveAllGenerators is called without setting ResolveAllGeneratorsFunc
		generators := mockCtrl.ResolveAllGenerators()
		// Then the length of the returned generators list should be 0
		if len(generators) != 1 {
			t.Fatalf("expected %v, got %v", 0, len(generators))
		}
	})
}

func TestMockController_ResolveStack(t *testing.T) {
	t.Run("ResolveStack", func(t *testing.T) {
		// Given a new mock injector and mock controller
		mocks := setSafeControllerMocks()
		mockCtrl := NewMockController(mocks.Injector)
		// And the ResolveStackFunc is set to return a mock stack
		mockCtrl.ResolveStackFunc = func() stack.Stack {
			return mocks.Stack
		}
		// When ResolveStack is called
		stackInstance := mockCtrl.ResolveStack()
		// Then the returned stack instance should not be nil
		if stackInstance == nil {
			t.Fatalf("expected %v, got %v", mocks.Stack, stackInstance)
		}
	})

	t.Run("NoResolveStackFunc", func(t *testing.T) {
		// Given a new mock injector and mock controller
		mocks := setSafeControllerMocks()
		// Register a nil stack with the injector
		mocks.Injector.Register("stack", nil)
		mockCtrl := NewMockController(mocks.Injector)
		// When ResolveStack is called without setting ResolveStackFunc
		stackInstance := mockCtrl.ResolveStack()
		// Then the returned stack instance should be nil
		if stackInstance != nil {
			t.Fatalf("expected nil, got %v", stackInstance)
		} else {
			t.Logf("expected nil, got nil")
		}
	})
}

func TestMockController_ResolveBlueprintHandler(t *testing.T) {
	t.Run("ResolveBlueprintHandler", func(t *testing.T) {
		// Given a new mock injector and mock controller
		mocks := setSafeControllerMocks()
		mockCtrl := NewMockController(mocks.Injector)
		// And the ResolveBlueprintHandlerFunc is set to return a mock blueprint handler
		mockCtrl.ResolveBlueprintHandlerFunc = func() blueprint.BlueprintHandler {
			return mocks.BlueprintHandler
		}
		// When ResolveBlueprintHandler is called
		blueprintHandler := mockCtrl.ResolveBlueprintHandler()
		// Then the returned blueprint handler should not be nil
		if blueprintHandler == nil {
			t.Fatalf("expected %v, got %v", mocks.BlueprintHandler, blueprintHandler)
		}
	})

	t.Run("NoResolveBlueprintHandlerFunc", func(t *testing.T) {
		// Given a new mock injector and mock controller
		mocks := setSafeControllerMocks()
		// Register a nil blueprint handler with the injector
		mocks.Injector.Register("blueprintHandler", nil)
		mockCtrl := NewMockController(mocks.Injector)
		// When ResolveBlueprintHandler is called without setting ResolveBlueprintHandlerFunc
		blueprintHandler := mockCtrl.ResolveBlueprintHandler()
		// Then the returned blueprint handler should be nil
		if blueprintHandler != nil {
			t.Fatalf("expected nil, got %v", blueprintHandler)
		} else {
			t.Logf("expected nil, got nil")
		}
	})
}

func TestMockController_SetEnvironmentVariables(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock controller with a SetEnvironmentVariables function
		mockInjector := di.NewInjector()

		// Set up the environment printer mock with complete implementation
		mockEnvPrinter := env.NewMockEnvPrinter()
		mockEnvPrinter.GetEnvVarsFunc = func() (map[string]string, error) {
			return map[string]string{
				"TEST_VAR":              "test_value",
				"WINDSOR_SESSION_TOKEN": "mock-token",
			}, nil
		}
		mockInjector.Register("env", mockEnvPrinter)

		// Set up a proper shell mock with GetSessionToken implementation
		mockShell := shell.NewMockShell()
		mockShell.GetSessionTokenFunc = func() (string, error) {
			return "mock-token", nil
		}
		// Mock WriteResetToken to prevent file operations
		mockShell.WriteResetTokenFunc = func() (string, error) {
			// Just pretend it worked without creating any files
			return "/mock/project/root/.windsor/.session.mock-token", nil
		}
		mockInjector.Register("shell", mockShell)

		mockController := NewMockController(mockInjector)

		// Create a map to track what environment variables were set
		setEnvCalls := make(map[string]string)

		// Mock the osSetenv function
		originalSetenv := osSetenv
		defer func() { osSetenv = originalSetenv }()
		osSetenv = func(key, value string) error {
			setEnvCalls[key] = value
			return nil
		}

		// When calling SetEnvironmentVariables
		err := mockController.SetEnvironmentVariables()

		// Then no error should be returned
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// Verify that environment variables were set
		if len(setEnvCalls) == 0 {
			t.Errorf("expected environment variables to be set")
		}
	})

	t.Run("NoSetEnvironmentVariablesFunc", func(t *testing.T) {
		// Given a new injector and mock controller
		mockInjector := di.NewInjector()

		// Set up the environment printer mock
		mockEnvPrinter := env.NewMockEnvPrinter()
		mockEnvPrinter.GetEnvVarsFunc = func() (map[string]string, error) {
			return map[string]string{
				"TEST_VAR":              "test_value",
				"WINDSOR_SESSION_TOKEN": "mock-token",
			}, nil
		}
		mockInjector.Register("env", mockEnvPrinter)

		// Set up the shell mock with GetSessionToken implementation
		mockShell := shell.NewMockShell()
		mockShell.GetSessionTokenFunc = func() (string, error) {
			return "mock-token", nil
		}
		// Mock WriteResetToken to prevent file operations
		mockShell.WriteResetTokenFunc = func() (string, error) {
			// Just pretend it worked without creating any files
			return "/mock/project/root/.windsor/.session.mock-token", nil
		}
		mockInjector.Register("shell", mockShell)

		mockCtrl := NewMockController(mockInjector)

		// Create a map to track what environment variables were set
		setEnvCalls := make(map[string]string)

		// Mock the osSetenv function
		originalSetenv := osSetenv
		defer func() { osSetenv = originalSetenv }()
		osSetenv = func(key, value string) error {
			setEnvCalls[key] = value
			return nil
		}

		// When SetEnvironmentVariables is called without setting SetEnvironmentVariablesFunc
		err := mockCtrl.SetEnvironmentVariables()

		// Then no error should be returned
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// Verify that environment variables were set
		if len(setEnvCalls) == 0 {
			t.Errorf("expected environment variables to be set")
		}
	})
}
