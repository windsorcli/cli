package controller

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	secretsConfigType "github.com/windsorcli/cli/api/v1alpha1/secrets"
	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/secrets"
)

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewRealController(t *testing.T) {
	t.Run("NewRealController", func(t *testing.T) {
		// Given a new test setup
		mocks := setupMocks(t)

		// When creating a new real controller
		controller := NewRealController(mocks.Injector)

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

func TestRealController_CreateCommonComponents(t *testing.T) {
	setup := func(t *testing.T) (Controller, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		controller := NewRealController(mocks.Injector)
		err := controller.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize controller: %v", err)
		}
		return controller, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a new controller
		controller, mocks := setup(t)

		// When creating common components
		err := controller.CreateCommonComponents()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the components should be registered in the injector
		if mocks.Injector.Resolve("configHandler") == nil {
			t.Fatalf("expected configHandler to be registered, got error")
		}
		if mocks.Injector.Resolve("shell") == nil {
			t.Fatalf("expected shell to be registered, got error")
		}
	})
}

func TestRealController_CreateSecretsProviders(t *testing.T) {
	setup := func(t *testing.T) (Controller, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		controller := NewRealController(mocks.Injector)
		err := controller.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize controller: %v", err)
		}
		return controller, mocks
	}

	t.Run("SopsSecretsProviderExists", func(t *testing.T) {
		// Given a new controller
		controller, mocks := setup(t)

		// And a mock config handler that returns a config root
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigRootFunc = func() (string, error) {
			return "/mock/config/root", nil
		}
		mocks.Injector.Register("configHandler", mockConfigHandler)
		controller.(*RealController).configHandler = mockConfigHandler

		// And a mock file system that simulates presence of secrets.enc.yaml
		originalOsStat := osStat
		defer func() { osStat = originalOsStat }()
		osStat = func(name string) (os.FileInfo, error) {
			if name == filepath.Join("/mock/config/root", "secrets.enc.yaml") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		// When creating the secrets provider
		err := controller.CreateSecretsProviders()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the Sops secrets provider should be registered
		if mocks.Injector.Resolve("sopsSecretsProvider") == nil {
			t.Fatalf("expected sopsSecretsProvider to be registered, got error")
		}
	})

	t.Run("NoSecretsFile", func(t *testing.T) {
		// Given a new controller
		controller, mocks := setup(t)

		// And a mock file system that simulates absence of secrets.enc files
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
		// Given a new controller
		controller, mocks := setup(t)

		// And a mock config handler that returns an error
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigRootFunc = func() (string, error) {
			return "", fmt.Errorf("mock error getting config root")
		}
		mocks.Injector.Register("configHandler", mockConfigHandler)
		controller.(*RealController).configHandler = mockConfigHandler

		// When creating the secrets provider
		err := controller.CreateSecretsProviders()

		// Then an error should occur
		if err == nil || err.Error() != "error getting config root: mock error getting config root" {
			t.Fatalf("expected error getting config root, got %v", err)
		}
	})

	t.Run("OnePasswordVaultsExist", func(t *testing.T) {
		// Given a new controller
		controller, mocks := setup(t)

		// And a mock config handler that returns 1Password vaults
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
		mocks.Injector.Register("configHandler", mockConfigHandler)
		controller.(*RealController).configHandler = mockConfigHandler

		// When creating the secrets provider
		err := controller.CreateSecretsProviders()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Validate the presence of vault1 and vault2
		for _, vaultID := range []string{"vault1", "vault2"} {
			providerName := "op" + strings.ToUpper(vaultID[:1]) + vaultID[1:] + "SecretsProvider"
			if provider := mocks.Injector.Resolve(providerName); provider == nil {
				t.Fatalf("expected %s to be registered, got error", providerName)
			} else {
				// Validate the provider by checking if it can be initialized
				if err := provider.(secrets.SecretsProvider).Initialize(); err != nil {
					t.Fatalf("expected %s to be initialized without error, got %v", providerName, err)
				}
			}
		}
	})

	t.Run("OnePasswordSDKProviderUsedWhenTokenSet", func(t *testing.T) {
		// Given a new controller
		controller, mocks := setup(t)

		// And a mock config handler that returns 1Password vaults
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetFunc = func(key string) interface{} {
			if key == "contexts.mock-context.secrets.onepassword.vaults" {
				return map[string]secretsConfigType.OnePasswordVault{
					"vault1": {ID: "vault1", Name: "test-vault"},
				}
			}
			return nil
		}
		mocks.Injector.Register("configHandler", mockConfigHandler)
		controller.(*RealController).configHandler = mockConfigHandler

		// And OP_SERVICE_ACCOUNT_TOKEN is set
		originalToken := os.Getenv("OP_SERVICE_ACCOUNT_TOKEN")
		defer os.Setenv("OP_SERVICE_ACCOUNT_TOKEN", originalToken)
		os.Setenv("OP_SERVICE_ACCOUNT_TOKEN", "test-token")

		// When creating the secrets provider
		err := controller.CreateSecretsProviders()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the SDK provider should be registered
		providerName := "opVault1SecretsProvider"
		provider := mocks.Injector.Resolve(providerName)
		if provider == nil {
			t.Fatalf("expected %s to be registered, got error", providerName)
		}

		// And it should be an SDK provider
		if _, ok := provider.(*secrets.OnePasswordSDKSecretsProvider); !ok {
			t.Fatalf("expected provider to be *secrets.OnePasswordSDKSecretsProvider, got %T", provider)
		}
	})
}

func TestRealController_CreateProjectComponents(t *testing.T) {
	setup := func(t *testing.T) (Controller, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		controller := NewRealController(mocks.Injector)
		err := controller.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize controller: %v", err)
		}
		return controller, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a new controller
		controller, mocks := setup(t)

		// And common components are created
		controller.CreateCommonComponents()

		// When creating project components
		err := controller.CreateProjectComponents()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the components should be registered in the injector
		if mocks.Injector.Resolve("gitGenerator") == nil {
			t.Fatalf("expected gitGenerator to be registered, got error")
		}
		if mocks.Injector.Resolve("blueprintHandler") == nil {
			t.Fatalf("expected blueprintHandler to be registered, got error")
		}
		if mocks.Injector.Resolve("terraformGenerator") == nil {
			t.Fatalf("expected terraformGenerator to be registered, got error")
		}
	})

	t.Run("DefaultToolsManagerCreation", func(t *testing.T) {
		// Given a new controller
		controller, mocks := setup(t)

		// And a mock config handler that returns empty tools manager
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "toolsManager" {
				return ""
			}
			return ""
		}
		mocks.Injector.Register("configHandler", mockConfigHandler)
		controller.(*RealController).configHandler = mockConfigHandler

		// When creating project components
		err := controller.CreateProjectComponents()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the default tools manager should be registered
		if mocks.Injector.Resolve("toolsManager") == nil {
			t.Fatalf("expected default toolsManager to be registered, got error")
		}
	})

	t.Run("ToolsManagerCreation", func(t *testing.T) {
		// Given a new controller
		controller, mocks := setup(t)

		// And common components are created
		controller.CreateCommonComponents()

		// And the configuration is set for Tools Manager to be enabled
		controller.(*RealController).configHandler.SetContext("test")
		controller.(*RealController).configHandler.SetContextValue("toolsManager.enabled", true)

		// When creating project components
		err := controller.CreateProjectComponents()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the tools manager should be registered
		if mocks.Injector.Resolve("toolsManager") == nil {
			t.Fatalf("expected toolsManager to be registered, got error")
		}
	})
}

func TestRealController_CreateEnvComponents(t *testing.T) {
	setup := func(t *testing.T) (Controller, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		controller := NewRealController(mocks.Injector)
		err := controller.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize controller: %v", err)
		}
		return controller, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a new controller
		controller, _ := setup(t)

		// And common components are created
		controller.CreateCommonComponents()

		// And the configuration is set for AWS and Docker to be enabled
		controller.(*RealController).configHandler.SetContext("test")
		controller.(*RealController).configHandler.SetContextValue("aws.enabled", true)
		controller.(*RealController).configHandler.SetContextValue("docker.enabled", true)

		// When creating environment components
		err := controller.CreateEnvComponents()

		// Then no error should occur
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestRealController_CreateServiceComponents(t *testing.T) {
	setup := func(t *testing.T) (Controller, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		controller := NewRealController(mocks.Injector)
		err := controller.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize controller: %v", err)
		}
		return controller, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a new controller
		controller, mocks := setup(t)

		// And common components are created
		controller.CreateCommonComponents()

		// And the configuration is set for various services to be enabled
		controller.(*RealController).configHandler.SetContext("test")
		controller.(*RealController).configHandler.SetContextValue("docker.enabled", true)
		controller.(*RealController).configHandler.SetContextValue("dns.enabled", true)
		controller.(*RealController).configHandler.SetContextValue("git.livereload.enabled", true)
		controller.(*RealController).configHandler.SetContextValue("aws.localstack.enabled", true)
		controller.(*RealController).configHandler.SetContextValue("cluster.enabled", true)
		controller.(*RealController).configHandler.SetContextValue("cluster.driver", "talos")
		controller.(*RealController).configHandler.SetContextValue("cluster.controlplanes.count", 2)
		controller.(*RealController).configHandler.SetContextValue("cluster.workers.count", 3)

		// When creating service components
		err := controller.CreateServiceComponents()

		// Then no error should occur
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the DNS service should be registered
		if mocks.Injector.Resolve("dnsService") == nil {
			t.Fatalf("expected dnsService to be registered, got error")
		}

		// And the Git livereload service should be registered
		if mocks.Injector.Resolve("gitLivereloadService") == nil {
			t.Fatalf("expected gitLivereloadService to be registered, got error")
		}

		// And the Localstack service should be registered
		if mocks.Injector.Resolve("localstackService") == nil {
			t.Fatalf("expected localstackService to be registered, got error")
		}

		// And the registry services should be registered if Docker registries are configured
		contextConfig := controller.(*RealController).configHandler.GetConfig()
		if contextConfig.Docker != nil && contextConfig.Docker.Registries != nil {
			for key := range contextConfig.Docker.Registries {
				serviceName := fmt.Sprintf("registryService.%s", key)
				if mocks.Injector.Resolve(serviceName) == nil {
					t.Fatalf("expected %s to be registered, got error", serviceName)
				}
			}
		}

		// And the Talos cluster services should be registered
		controlPlaneCount := controller.(*RealController).configHandler.GetInt("cluster.controlplanes.count")
		workerCount := controller.(*RealController).configHandler.GetInt("cluster.workers.count")

		for i := 1; i <= controlPlaneCount; i++ {
			serviceName := fmt.Sprintf("clusterNode.controlplane-%d", i)
			if mocks.Injector.Resolve(serviceName) == nil {
				t.Fatalf("expected %s to be registered, got error", serviceName)
			}
		}
		for i := 1; i <= workerCount; i++ {
			serviceName := fmt.Sprintf("clusterNode.worker-%d", i)
			if mocks.Injector.Resolve(serviceName) == nil {
				t.Fatalf("expected %s to be registered, got error", serviceName)
			}
		}
	})

	t.Run("DockerDisabled", func(t *testing.T) {
		// Given a new controller
		controller, _ := setup(t)

		// And common components are created
		controller.CreateCommonComponents()

		// And Docker is disabled in the configuration
		controller.(*RealController).configHandler.SetContext("test")
		controller.(*RealController).configHandler.SetContextValue("docker.enabled", false)

		// When creating service components
		err := controller.CreateServiceComponents()

		// Then no error should occur
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestRealController_CreateVirtualizationComponents(t *testing.T) {
	setup := func(t *testing.T) (Controller, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		controller := NewRealController(mocks.Injector)
		err := controller.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize controller: %v", err)
		}
		return controller, mocks
	}

	t.Run("SuccessWithColima", func(t *testing.T) {
		// Given a new controller
		controller, mocks := setup(t)

		// And common components are created
		controller.CreateCommonComponents()

		// And the configuration is set for VM driver to be colima and Docker to be enabled
		controller.(*RealController).configHandler.SetContext("test")
		controller.(*RealController).configHandler.SetContextValue("vm.driver", "colima")
		controller.(*RealController).configHandler.SetContextValue("docker.enabled", true)

		// When creating virtualization components
		err := controller.CreateVirtualizationComponents()

		// Then no error should occur
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the network interface provider should be registered
		if mocks.Injector.Resolve("networkInterfaceProvider") == nil {
			t.Fatalf("expected networkInterfaceProvider to be registered, got error")
		}

		// And the SSH client should be registered
		if mocks.Injector.Resolve("sshClient") == nil {
			t.Fatalf("expected sshClient to be registered, got error")
		}

		// And the secure shell should be registered
		if mocks.Injector.Resolve("secureShell") == nil {
			t.Fatalf("expected secureShell to be registered, got error")
		}

		// And the virtual machine should be registered
		if mocks.Injector.Resolve("virtualMachine") == nil {
			t.Fatalf("expected virtualMachine to be registered, got error")
		}

		// And the network manager should be registered
		if mocks.Injector.Resolve("networkManager") == nil {
			t.Fatalf("expected networkManager to be registered, got error")
		}

		// And the container runtime should be registered
		if mocks.Injector.Resolve("containerRuntime") == nil {
			t.Fatalf("expected containerRuntime to be registered, got error")
		}
	})

	t.Run("SuccessWithBaseNetworkManager", func(t *testing.T) {
		// Given a new controller
		controller, mocks := setup(t)

		// And common components are created
		controller.CreateCommonComponents()

		// And the configuration is set for VM driver to be something other than colima
		controller.(*RealController).configHandler.SetContext("test")
		controller.(*RealController).configHandler.SetContextValue("vm.driver", "other")
		controller.(*RealController).configHandler.SetContextValue("docker.enabled", true)

		// When creating virtualization components
		err := controller.CreateVirtualizationComponents()

		// Then no error should occur
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the base network manager should be registered
		if mocks.Injector.Resolve("networkManager") == nil {
			t.Fatalf("expected networkManager to be registered, got error")
		}

		// And the container runtime should be registered
		if mocks.Injector.Resolve("containerRuntime") == nil {
			t.Fatalf("expected containerRuntime to be registered, got error")
		}
	})

	t.Run("ErrorCreatingNetworkManager", func(t *testing.T) {
		// Given a new controller setup
		_, mocks := setup(t)

		// Register a nil network manager
		mocks.Injector.Register("networkManager", nil)

		// Verify that the network manager is registered as nil
		if mocks.Injector.Resolve("networkManager") != nil {
			t.Fatalf("expected networkManager to be nil, got non-nil")
		}
	})
}

func TestRealController_CreateStackComponents(t *testing.T) {
	setup := func(t *testing.T) (Controller, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		controller := NewRealController(mocks.Injector)
		err := controller.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize controller: %v", err)
		}
		return controller, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a new controller
		controller, mocks := setup(t)

		// When creating stack components
		err := controller.CreateStackComponents()

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the stack should be registered in the injector
		if mocks.Injector.Resolve("stack") == nil {
			t.Fatalf("expected stack to be registered, got error")
		}
	})
}
