package controller

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	secretsConfigType "github.com/windsorcli/cli/api/v1alpha1/secrets"
	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/secrets"
	"github.com/windsorcli/cli/pkg/shell"
)

func TestNewRealController(t *testing.T) {
	t.Run("NewRealController", func(t *testing.T) {
		injector := di.NewInjector()

		// When creating a new real controller
		controller := NewRealController(injector)

		// Then the controller should not be nil
		if controller == nil {
			t.Fatalf("expected controller, got nil")
		} else {
			t.Logf("Success: controller created")
		}
	})
}

func TestRealController_CreateCommonComponents(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new injector and a new real controller using mocks
		injector := di.NewInjector()
		controller := NewRealController(injector)

		// Initialize the controller
		if err := controller.Initialize(); err != nil {
			t.Fatalf("failed to initialize controller: %v", err)
		}

		// When creating common components
		err := controller.CreateCommonComponents()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the components should be registered in the injector
		if injector.Resolve("configHandler") == nil {
			t.Fatalf("expected configHandler to be registered, got error")
		}
		if injector.Resolve("shell") == nil {
			t.Fatalf("expected shell to be registered, got error")
		}

		t.Logf("Success: common components created and registered")
	})
}

func TestRealController_CreateSecretsProviders(t *testing.T) {
	t.Run("SopsSecretsProviderExists", func(t *testing.T) {
		// Given a new injector and a new real controller using mocks
		injector := di.NewInjector()
		controller := NewRealController(injector)

		// Override the existing configHandler with a mock configHandler
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigRootFunc = func() (string, error) {
			return "/mock/config/root", nil
		}
		injector.Register("configHandler", mockConfigHandler)
		controller.configHandler = mockConfigHandler

		// Mock the os.Stat function to simulate the presence of a secrets.enc.yaml file
		osStat = func(name string) (os.FileInfo, error) {
			if name == filepath.Join("/mock/config/root", "secrets.enc.yaml") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		// Initialize the controller
		if err := controller.Initialize(); err != nil {
			t.Fatalf("failed to initialize controller: %v", err)
		}

		// When creating the secrets provider
		err := controller.CreateSecretsProviders()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the Sops secrets provider should be registered
		if injector.Resolve("sopsSecretsProvider") == nil {
			t.Fatalf("expected sopsSecretsProvider to be registered, got error")
		}
	})

	t.Run("NoSecretsFile", func(t *testing.T) {
		// Given a new injector and a new real controller using mocks
		mocks := setSafeControllerMocks()
		controller := NewController(mocks.Injector)
		controller.Initialize()

		// Mock the os.Stat function to simulate the absence of secrets.enc files
		originalOsStat := osStat
		defer func() { osStat = originalOsStat }()
		osStat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		// When creating the secrets provider
		err := controller.CreateSecretsProviders()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the sopsSecretsProvider should not be registered since there are no secrets
		if mocks.Injector.Resolve("sopsSecretsProvider") != nil {
			t.Fatalf("expected no sopsSecretsProvider to be registered, got one")
		}
	})

	t.Run("ErrorGettingConfigRoot", func(t *testing.T) {
		// Given a new injector and a new real controller using mocks
		injector := di.NewInjector()
		controller := NewRealController(injector)

		// Override the existing configHandler with a mock configHandler
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigRootFunc = func() (string, error) {
			return "", fmt.Errorf("mock error getting config root")
		}
		injector.Register("configHandler", mockConfigHandler)
		controller.configHandler = mockConfigHandler

		// Initialize the controller
		if err := controller.Initialize(); err != nil {
			t.Fatalf("failed to initialize controller: %v", err)
		}

		// When creating the secrets provider
		err := controller.CreateSecretsProviders()

		// Then an error should occur
		if err == nil || err.Error() != "error getting config root: mock error getting config root" {
			t.Fatalf("expected error getting config root, got %v", err)
		}
	})

	t.Run("OnePasswordVaultsExist", func(t *testing.T) {
		// Given a new injector and a new real controller using mocks
		injector := di.NewInjector()
		controller := NewRealController(injector)

		// Override the existing configHandler with a mock configHandler
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetFunc = func(key string) interface{} {
			if key == "contexts.mock-context.secrets.onepassword.vaults" {
				return map[string]secretsConfigType.OnePasswordVault{
					"vault1": {ID: "vault1"},
					"vault2": {ID: "vault2"},
				}
			}
			return nil
		}
		injector.Register("configHandler", mockConfigHandler)
		controller.configHandler = mockConfigHandler

		// Create a mock shell instance and register it with the injector
		mockShell := shell.NewMockShell()
		injector.Register("shell", mockShell)

		// Initialize the controller
		if err := controller.Initialize(); err != nil {
			t.Fatalf("failed to initialize controller: %v", err)
		}

		// When creating the secrets provider
		err := controller.CreateSecretsProviders()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Validate the presence of vault1 and vault2
		for _, vaultID := range []string{"vault1", "vault2"} {
			providerName := "op" + strings.ToUpper(vaultID[:1]) + vaultID[1:] + "SecretsProvider"
			if provider := injector.Resolve(providerName); provider == nil {
				t.Fatalf("expected %s to be registered, got error", providerName)
			} else {
				// Validate the provider by checking if it can be initialized
				if err := provider.(secrets.SecretsProvider).Initialize(); err != nil {
					t.Fatalf("expected %s to be initialized without error, got %v", providerName, err)
				}
			}
		}
	})
}

func TestRealController_CreateProjectComponents(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new injector and a new real controller using mocks
		injector := di.NewInjector()
		controller := NewRealController(injector)

		// Initialize the controller
		if err := controller.Initialize(); err != nil {
			t.Fatalf("failed to initialize controller: %v", err)
		}

		// And common components are created
		controller.CreateCommonComponents()

		// When creating project components
		err := controller.CreateProjectComponents()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the components should be registered in the injector
		if injector.Resolve("gitGenerator") == nil {
			t.Fatalf("expected gitGenerator to be registered, got error")
		}
		if injector.Resolve("blueprintHandler") == nil {
			t.Fatalf("expected blueprintHandler to be registered, got error")
		}
		if injector.Resolve("terraformGenerator") == nil {
			t.Fatalf("expected terraformGenerator to be registered, got error")
		}

		t.Logf("Success: project components created and registered")
	})

	t.Run("DefaultToolsManagerCreation", func(t *testing.T) {
		// Given a new injector and a new real controller using mocks
		injector := di.NewInjector()
		controller := NewRealController(injector)

		// Override the existing configHandler with a mock configHandler
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "toolsManager" {
				return ""
			}
			return ""
		}
		injector.Register("configHandler", mockConfigHandler)

		// Initialize the controller
		if err := controller.Initialize(); err != nil {
			t.Fatalf("failed to initialize controller: %v", err)
		}

		// When creating project components
		err := controller.CreateProjectComponents()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the default tools manager should be registered
		if injector.Resolve("toolsManager") == nil {
			t.Fatalf("expected default toolsManager to be registered, got error")
		}
	})

	t.Run("ToolsManagerCreation", func(t *testing.T) {
		// Given a new injector and a new real controller using mocks
		injector := di.NewInjector()
		controller := NewRealController(injector)

		// Initialize the controller
		if err := controller.Initialize(); err != nil {
			t.Fatalf("failed to initialize controller: %v", err)
		}

		// And common components are created
		controller.CreateCommonComponents()

		// And the configuration is set for Tools Manager to be enabled
		controller.configHandler.SetContext("test")
		controller.configHandler.SetContextValue("toolsManager.enabled", true)

		// When creating project components
		err := controller.CreateProjectComponents()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the tools manager should be registered
		if injector.Resolve("toolsManager") == nil {
			t.Fatalf("expected toolsManager to be registered, got error")
		}
	})
}

func TestRealController_CreateEnvComponents(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new injector and a new real controller using mocks
		injector := di.NewInjector()
		controller := NewRealController(injector)

		// And the controller is initialized
		if err := controller.Initialize(); err != nil {
			t.Fatalf("failed to initialize controller: %v", err)
		}

		// And common components are created
		controller.CreateCommonComponents()

		// And the configuration is set for AWS and Docker to be enabled
		controller.configHandler.SetContext("test")
		controller.configHandler.SetContextValue("aws.enabled", true)
		controller.configHandler.SetContextValue("docker.enabled", true)

		// When creating environment components
		err := controller.CreateEnvComponents()

		// Then no error should occur
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestRealController_CreateServiceComponents(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new injector and a new real controller using mocks
		injector := di.NewInjector()
		controller := NewRealController(injector)

		// And the controller is initialized
		if err := controller.Initialize(); err != nil {
			t.Fatalf("failed to initialize controller: %v", err)
		}

		// And common components are created
		controller.CreateCommonComponents()

		// And the configuration is set for various services to be enabled
		controller.configHandler.SetContext("test")
		controller.configHandler.SetContextValue("docker.enabled", true)
		controller.configHandler.SetContextValue("dns.enabled", true)
		controller.configHandler.SetContextValue("git.livereload.enabled", true)
		controller.configHandler.SetContextValue("aws.localstack.enabled", true)
		controller.configHandler.SetContextValue("cluster.enabled", true)
		controller.configHandler.SetContextValue("cluster.driver", "talos")
		controller.configHandler.SetContextValue("cluster.controlplanes.count", 2)
		controller.configHandler.SetContextValue("cluster.workers.count", 3)

		// When creating service components
		err := controller.CreateServiceComponents()

		// Then no error should occur
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the DNS service should be registered
		if injector.Resolve("dnsService") == nil {
			t.Fatalf("expected dnsService to be registered, got error")
		}

		// And the Git livereload service should be registered
		if injector.Resolve("gitLivereloadService") == nil {
			t.Fatalf("expected gitLivereloadService to be registered, got error")
		}

		// And the Localstack service should be registered
		if injector.Resolve("localstackService") == nil {
			t.Fatalf("expected localstackService to be registered, got error")
		}

		// And the registry services should be registered if Docker registries are configured
		contextConfig := controller.configHandler.GetConfig()
		if contextConfig.Docker != nil && contextConfig.Docker.Registries != nil {
			for key := range contextConfig.Docker.Registries {
				serviceName := fmt.Sprintf("registryService.%s", key)
				if injector.Resolve(serviceName) == nil {
					t.Fatalf("expected %s to be registered, got error", serviceName)
				}
			}
		}

		// And the Talos cluster services should be registered
		controlPlaneCount := controller.configHandler.GetInt("cluster.controlplanes.count")
		workerCount := controller.configHandler.GetInt("cluster.workers.count")

		for i := 1; i <= controlPlaneCount; i++ {
			serviceName := fmt.Sprintf("clusterNode.controlplane-%d", i)
			if injector.Resolve(serviceName) == nil {
				t.Fatalf("expected %s to be registered, got error", serviceName)
			}
		}
		for i := 1; i <= workerCount; i++ {
			serviceName := fmt.Sprintf("clusterNode.worker-%d", i)
			if injector.Resolve(serviceName) == nil {
				t.Fatalf("expected %s to be registered, got error", serviceName)
			}
		}

		t.Logf("Success: service components created and registered")
	})

	t.Run("DockerDisabled", func(t *testing.T) {
		// Given a new injector and a new real controller
		injector := di.NewInjector()
		controller := NewRealController(injector)

		// When the controller is initialized
		if err := controller.Initialize(); err != nil {
			t.Fatalf("failed to initialize controller: %v", err)
		}

		// And common components are created
		controller.CreateCommonComponents()

		// And Docker is disabled in the configuration
		controller.configHandler.SetContext("test")
		controller.configHandler.SetContextValue("docker.enabled", false)

		// And service components are created
		err := controller.CreateServiceComponents()

		// Then no error should occur
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestRealController_CreateVirtualizationComponents(t *testing.T) {
	t.Run("SuccessWithColima", func(t *testing.T) {
		// Given a new injector and a new real controller using mocks
		injector := di.NewInjector()
		controller := NewRealController(injector)

		// Initialize the controller
		if err := controller.Initialize(); err != nil {
			t.Fatalf("failed to initialize controller: %v", err)
		}

		// And common components are created
		controller.CreateCommonComponents()

		// And the configuration is set for VM driver to be colima and Docker to be enabled
		controller.configHandler.SetContext("test")
		controller.configHandler.SetContextValue("vm.driver", "colima")
		controller.configHandler.SetContextValue("docker.enabled", true)

		// When creating virtualization components
		err := controller.CreateVirtualizationComponents()

		// Then no error should occur
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the network interface provider should be registered
		if injector.Resolve("networkInterfaceProvider") == nil {
			t.Fatalf("expected networkInterfaceProvider to be registered, got error")
		}

		// And the SSH client should be registered
		if injector.Resolve("sshClient") == nil {
			t.Fatalf("expected sshClient to be registered, got error")
		}

		// And the secure shell should be registered
		if injector.Resolve("secureShell") == nil {
			t.Fatalf("expected secureShell to be registered, got error")
		}

		// And the virtual machine should be registered
		if injector.Resolve("virtualMachine") == nil {
			t.Fatalf("expected virtualMachine to be registered, got error")
		}

		// And the network manager should be registered
		if injector.Resolve("networkManager") == nil {
			t.Fatalf("expected networkManager to be registered, got error")
		}

		// And the container runtime should be registered
		if injector.Resolve("containerRuntime") == nil {
			t.Fatalf("expected containerRuntime to be registered, got error")
		}

		t.Logf("Success: virtualization components created and registered with Colima")
	})

	t.Run("SuccessWithBaseNetworkManager", func(t *testing.T) {
		// Given a new injector and a new real controller using mocks
		injector := di.NewInjector()
		controller := NewRealController(injector)

		// Initialize the controller
		if err := controller.Initialize(); err != nil {
			t.Fatalf("failed to initialize controller: %v", err)
		}

		// And common components are created
		controller.CreateCommonComponents()

		// And the configuration is set for VM driver to be something other than colima
		controller.configHandler.SetContext("test")
		controller.configHandler.SetContextValue("vm.driver", "other")
		controller.configHandler.SetContextValue("docker.enabled", true)

		// When creating virtualization components
		err := controller.CreateVirtualizationComponents()

		// Then no error should occur
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the base network manager should be registered
		if injector.Resolve("networkManager") == nil {
			t.Fatalf("expected networkManager to be registered, got error")
		}

		// And the container runtime should be registered
		if injector.Resolve("containerRuntime") == nil {
			t.Fatalf("expected containerRuntime to be registered, got error")
		}

		t.Logf("Success: virtualization components created and registered with Base Network Manager")
	})

	t.Run("ErrorCreatingNetworkManager", func(t *testing.T) {
		// Given a new injector and a new real controller using mocks
		injector := di.NewInjector()
		controller := NewRealController(injector)

		// Initialize the controller
		if err := controller.Initialize(); err != nil {
			t.Fatalf("failed to initialize controller: %v", err)
		}

		// Register a nil network manager
		injector.Register("networkManager", nil)

		// Verify that the network manager is registered as nil
		if injector.Resolve("networkManager") != nil {
			t.Fatalf("expected networkManager to be nil, got non-nil")
		}

		t.Logf("Success: networkManager registered as nil")
	})
}

func TestRealController_CreateStackComponents(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new injector and a new real controller using mocks
		injector := di.NewInjector()
		controller := NewRealController(injector)

		// Initialize the controller
		if err := controller.Initialize(); err != nil {
			t.Fatalf("failed to initialize controller: %v", err)
		}

		// When creating stack components
		err := controller.CreateStackComponents()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the stack should be registered in the injector
		if injector.Resolve("stack") == nil {
			t.Fatalf("expected stack to be registered, got error")
		}

		t.Logf("Success: stack components created and registered")
	})
}
