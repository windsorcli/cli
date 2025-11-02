package services

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/api/v1alpha1/docker"
	"github.com/windsorcli/cli/pkg/context/config"
	"github.com/windsorcli/cli/pkg/constants"
)

// =============================================================================
// Test Constructor
// =============================================================================

func TestRegistryService_NewRegistryService(t *testing.T) {
	setup := func(t *testing.T) (*RegistryService, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		service := NewRegistryService(mocks.Injector)
		service.shims = mocks.Shims
		service.Initialize()
		return service, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a set of mock components
		service, mocks := setup(t)

		// Then the RegistryService should not be nil
		if service == nil {
			t.Fatalf("expected RegistryService, got nil")
		}

		// And: the RegistryService should have the correct injector
		if service.injector != mocks.Injector {
			t.Errorf("expected injector %v, got %v", mocks.Injector, service.injector)
		}
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestRegistryService_GetComposeConfig(t *testing.T) {
	setup := func(t *testing.T) (*RegistryService, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		service := NewRegistryService(mocks.Injector)
		service.shims = mocks.Shims
		service.Initialize()
		service.SetName("registry")
		return service, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a mock config handler, shell, context, and service
		service, _ := setup(t)

		// When GetComposeConfig is called
		composeConfig, err := service.GetComposeConfig()
		if err != nil {
			t.Fatalf("GetComposeConfig() error = %v", err)
		}

		// Then: the compose configuration should include the registry service
		if composeConfig == nil || len(composeConfig.Services) == 0 {
			t.Fatalf("expected non-nil composeConfig with services, got %v", composeConfig)
		}

		expectedName := "registry"
		expectedRemoteURL := "registry.test"
		expectedLocalURL := "registry.test"
		found := false

		for _, config := range composeConfig.Services {
			if config.Name == expectedName {
				remoteURL, remoteExists := config.Environment["REGISTRY_PROXY_REMOTEURL"]
				localURL, localExists := config.Environment["REGISTRY_PROXY_LOCALURL"]

				if remoteExists && localExists && *remoteURL == expectedRemoteURL && *localURL == expectedLocalURL {
					found = true
					break
				}
			}
		}

		if !found {
			t.Errorf("expected service with name %q and environment variables REGISTRY_PROXY_REMOTEURL=%q and REGISTRY_PROXY_LOCALURL=%q to be in the list of configurations:\n%+v", expectedName, expectedRemoteURL, expectedLocalURL, composeConfig.Services)
		}
	})

	t.Run("NoRegistryFound", func(t *testing.T) {
		// Given a mock config handler, shell, context, and service
		service, _ := setup(t)
		service.SetName("nonexistent-registry")

		// When GetComposeConfig is called
		_, err := service.GetComposeConfig()

		// Then an error should be returned indicating no registry was found
		if err == nil || !strings.Contains(err.Error(), "no registry found with name") {
			t.Fatalf("expected error indicating no registry found, got %v", err)
		}
	})

	t.Run("MkdirAllFailure", func(t *testing.T) {
		// Given a mock config handler, shell, context, and service
		service, mocks := setup(t)

		// Mock mkdirAll to simulate a failure
		originalMkdirAll := mocks.Shims.MkdirAll
		defer func() { mocks.Shims.MkdirAll = originalMkdirAll }()
		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error {
			return fmt.Errorf("mock error creating directory")
		}

		// When GetComposeConfig is called
		_, err := service.GetComposeConfig()

		// Then an error should be returned indicating directory creation failure
		if err == nil || !strings.Contains(err.Error(), "mock error creating directory") {
			t.Fatalf("expected error indicating directory creation failure, got %v", err)
		}
	})

	t.Run("ProjectRootRetrievalFailure", func(t *testing.T) {
		// Given a mock config handler, shell, context, and service
		service, mocks := setup(t)
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("mock error getting project root")
		}

		// When a new RegistryService is created and initialized
		service.SetName("registry")

		// When GetComposeConfig is called
		_, err := service.GetComposeConfig()

		// Then an error should be returned indicating project root retrieval failure
		if err == nil || !strings.Contains(err.Error(), "mock error getting project root") {
			t.Fatalf("expected error indicating project root retrieval failure, got %v", err)
		}
	})

	t.Run("LocalRegistry", func(t *testing.T) {
		// Given a mock config handler, shell, context, and service
		service, mocks := setup(t)

		// Set up the registry configuration
		mocks.ConfigHandler.Set("vm.driver", "docker-desktop")
		mocks.ConfigHandler.Set("docker.registries.local-registry.hostport", 5000)

		// Configure service for local registry testing
		service.address = "localhost"
		service.name = "local-registry"

		// When GetComposeConfig is called
		composeConfig, err := service.GetComposeConfig()
		if err != nil {
			t.Fatalf("GetComposeConfig() error = %v", err)
		}

		// Then check that the service has the expected port configuration
		expectedPortConfig := types.ServicePortConfig{
			Target:    5000,
			Published: fmt.Sprintf("%d", service.hostPort),
			Protocol:  "tcp",
		}
		found := false

		for _, config := range composeConfig.Services {
			if config.Name == "local-registry" {
				for _, portConfig := range config.Ports {
					if portConfig.Target == expectedPortConfig.Target &&
						portConfig.Published == expectedPortConfig.Published &&
						portConfig.Protocol == expectedPortConfig.Protocol {
						found = true
						break
					}
				}
			}
		}

		if !found {
			t.Errorf("expected service with name %q to have port configuration %+v in the list of configurations:\n%+v", "local-registry", expectedPortConfig, composeConfig.Services)
		}
	})
}

func TestRegistryService_SetAddress(t *testing.T) {
	setup := func(t *testing.T) (*RegistryService, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		// Load initial config
		configYAML := `
version: v1alpha1
contexts:
  mock-context:
    dns:
      domain: test
    docker:
      registries:
        registry:
          remote: ""
          local: ""
        registry1:
          remote: ""
          local: ""
        registry2:
          remote: ""
          local: ""
`
		if err := mocks.ConfigHandler.LoadConfigString(configYAML); err != nil {
			t.Fatalf("Failed to load config: %v", err)
		}

		// Verify config loaded correctly
		domain := mocks.ConfigHandler.GetString("dns.domain")
		if domain != "test" {
			t.Fatalf("Config not loaded correctly, dns.domain = '%s', expected 'test'", domain)
		}

		service := NewRegistryService(mocks.Injector)
		service.shims = mocks.Shims
		service.Initialize()
		service.SetName("registry")
		// Reset package-level variables
		registryNextPort = constants.RegistryDefaultHostPort + 1
		localRegistry = nil
		return service, mocks
	}

	t.Run("SuccessLocalRegistry", func(t *testing.T) {
		// Given a registry service with mock components
		service, mocks := setup(t)

		// And localhost mode
		if err := mocks.ConfigHandler.Set("vm.driver", "docker-desktop"); err != nil {
			t.Fatalf("Failed to set vm.driver: %v", err)
		}

		// Verify vm.driver was set
		if vmDriver := mocks.ConfigHandler.GetString("vm.driver"); vmDriver != "docker-desktop" {
			t.Fatalf("vm.driver not set correctly, got '%s'", vmDriver)
		}

		// Manually set the expected values to verify Get/Set works
		if err := mocks.ConfigHandler.Set("docker.registries.registry.hostname", "manual.test"); err != nil {
			t.Fatalf("Manual set failed: %v", err)
		}
		if manual := mocks.ConfigHandler.GetString("docker.registries.registry.hostname"); manual != "manual.test" {
			t.Fatalf("Manual get failed, expected 'manual.test', got '%s'", manual)
		}

		// When SetAddress is called with localhost
		err := service.SetAddress("localhost")

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the hostname should be set correctly
		expectedHostname := "registry.test"
		actualHostname := mocks.ConfigHandler.GetString("docker.registries.registry.hostname", "")
		if actualHostname != expectedHostname {
			t.Errorf("expected hostname %s, got %s", expectedHostname, actualHostname)
		}

		// And the host port should be set to default
		expectedHostPort := constants.RegistryDefaultHostPort
		actualHostPort := mocks.ConfigHandler.GetInt("docker.registries.registry.hostport", 0)
		if actualHostPort != expectedHostPort {
			t.Errorf("expected host port %d, got %d", expectedHostPort, actualHostPort)
		}

		// And the registry URL should be set
		expectedRegistryURL := "registry.test"
		actualRegistryURL := mocks.ConfigHandler.GetString("docker.registry_url", "")
		if actualRegistryURL != expectedRegistryURL {
			t.Errorf("expected registry URL %s, got %s", expectedRegistryURL, actualRegistryURL)
		}
	})

	t.Run("SuccessRemoteRegistry", func(t *testing.T) {
		// Given a registry service with mock components
		service, mocks := setup(t)

		// And remote registry configuration
		if err := mocks.ConfigHandler.Set("docker.registries.registry.remote", "remote.registry:5000"); err != nil {
			t.Fatalf("Failed to set remote registry: %v", err)
		}

		// And localhost mode
		if err := mocks.ConfigHandler.Set("vm.driver", "docker-desktop"); err != nil {
			t.Fatalf("Failed to set vm.driver: %v", err)
		}

		// When SetAddress is called
		err := service.SetAddress("192.168.1.1")

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the hostname should be set correctly
		expectedHostname := "registry.test"
		actualHostname := mocks.ConfigHandler.GetString("docker.registries.registry.hostname", "")
		if actualHostname != expectedHostname {
			t.Errorf("expected hostname %s, got %s", expectedHostname, actualHostname)
		}

		// And no host port should be set
		actualHostPort := mocks.ConfigHandler.GetInt("docker.registries.registry.hostport", 0)
		if actualHostPort != 0 {
			t.Errorf("expected no host port, got %d", actualHostPort)
		}
	})

	t.Run("SuccessWithCustomHostPort", func(t *testing.T) {
		// Given a registry service with mock components
		service, mocks := setup(t)

		// And localhost mode
		if err := mocks.ConfigHandler.Set("vm.driver", "docker-desktop"); err != nil {
			t.Fatalf("Failed to set vm.driver: %v", err)
		}

		// And custom host port
		customHostPort := 5001
		if err := mocks.ConfigHandler.Set("docker.registries.registry.hostport", customHostPort); err != nil {
			t.Fatalf("Failed to set custom host port: %v", err)
		}

		// When SetAddress is called
		err := service.SetAddress("192.168.1.1")

		// Then there should be no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And the custom host port should be preserved
		actualHostPort := mocks.ConfigHandler.GetInt("docker.registries.registry.hostport", 0)
		if actualHostPort != customHostPort {
			t.Errorf("expected host port %d, got %d", customHostPort, actualHostPort)
		}
	})

	t.Run("SuccessMultipleLocalRegistries", func(t *testing.T) {
		// Given a registry service with mock components
		service1, mocks := setup(t)

		// And localhost mode
		if err := mocks.ConfigHandler.Set("vm.driver", "docker-desktop"); err != nil {
			t.Fatalf("Failed to set vm.driver: %v", err)
		}

		service1.SetName("registry1")

		// When SetAddress is called for first registry
		err := service1.SetAddress("localhost")
		if err != nil {
			t.Fatalf("Failed to set address for first registry: %v", err)
		}

		// Create second registry
		service2 := NewRegistryService(mocks.Injector)
		service2.shims = mocks.Shims
		service2.Initialize()
		service2.SetName("registry2")

		// When SetAddress is called for second registry
		err = service2.SetAddress("localhost")
		if err != nil {
			t.Fatalf("Failed to set address for second registry: %v", err)
		}

		// Then the first registry should have default port
		expectedHostPort1 := constants.RegistryDefaultHostPort
		actualHostPort1 := mocks.ConfigHandler.GetInt("docker.registries.registry1.hostport", 0)
		if actualHostPort1 != expectedHostPort1 {
			t.Errorf("expected host port %d for first registry, got %d", expectedHostPort1, actualHostPort1)
		}

		// And the second registry should have incremented port
		expectedHostPort2 := constants.RegistryDefaultHostPort + 1
		actualHostPort2 := mocks.ConfigHandler.GetInt("docker.registries.registry2.hostport", 0)
		if actualHostPort2 != expectedHostPort2 {
			t.Errorf("expected host port %d for second registry, got %d", expectedHostPort2, actualHostPort2)
		}
	})

	t.Run("BaseServiceError", func(t *testing.T) {
		// Given a registry service with mock components
		mockConfigHandler := config.NewMockConfigHandler()
		mocks := setupMocks(t, &SetupOptions{
			ConfigHandler: mockConfigHandler,
		})

		service := NewRegistryService(mocks.Injector)
		service.shims = mocks.Shims
		service.Initialize()
		service.SetName("registry")

		// When SetAddress is called with invalid address
		err := service.SetAddress("invalid-address")

		// Then there should be an error
		if err == nil {
			t.Error("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "invalid IPv4 address") {
			t.Errorf("expected error containing %q, got %v", "invalid IPv4 address", err)
		}
	})

	t.Run("ErrorSettingHostname", func(t *testing.T) {
		// Given a registry service with mock components
		mockConfigHandler := config.NewMockConfigHandler()
		mocks := setupMocks(t, &SetupOptions{
			ConfigHandler: mockConfigHandler,
		})

		service := NewRegistryService(mocks.Injector)
		service.shims = mocks.Shims
		service.Initialize()
		service.SetName("registry")

		// And mock error when setting hostname
		mockConfigHandler.SetFunc = func(key string, value any) error {
			return fmt.Errorf("mock error setting hostname")
		}

		// When SetAddress is called
		err := service.SetAddress("localhost")

		// Then there should be an error
		if err == nil {
			t.Error("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "mock error setting hostname") {
			t.Errorf("expected error containing %q, got %v", "mock error setting hostname", err)
		}
	})

	t.Run("ErrorSettingHostPort", func(t *testing.T) {
		// Given a registry service with mock components
		mockConfigHandler := config.NewMockConfigHandler()
		mocks := setupMocks(t, &SetupOptions{
			ConfigHandler: mockConfigHandler,
		})

		service := NewRegistryService(mocks.Injector)
		service.shims = mocks.Shims
		service.Initialize()
		service.SetName("registry")

		// And mock configuration
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "vm.driver" {
				return "docker-desktop"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		mockConfigHandler.SetFunc = func(key string, value any) error {
			if strings.Contains(key, "hostport") {
				return fmt.Errorf("mock error setting host port")
			}
			return nil
		}

		mockConfigHandler.GetConfigFunc = func() *v1alpha1.Context {
			return &v1alpha1.Context{
				Docker: &docker.DockerConfig{
					Registries: map[string]docker.RegistryConfig{
						"registry": {},
					},
				},
			}
		}

		// When SetAddress is called
		err := service.SetAddress("localhost")

		// Then there should be an error
		if err == nil {
			t.Error("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "mock error setting host port") {
			t.Errorf("expected error containing %q, got %v", "mock error setting host port", err)
		}
	})

	t.Run("ErrorSettingRegistryURL", func(t *testing.T) {
		// Given a registry service with mock components
		mockConfigHandler := config.NewMockConfigHandler()
		mocks := setupMocks(t, &SetupOptions{
			ConfigHandler: mockConfigHandler,
		})

		service := NewRegistryService(mocks.Injector)
		service.shims = mocks.Shims
		service.Initialize()
		service.SetName("registry")

		// Reset package-level variables
		registryNextPort = constants.RegistryDefaultHostPort + 1
		localRegistry = nil

		// And mock configuration
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "vm.driver" {
				return "docker-desktop"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		mockConfigHandler.SetFunc = func(key string, value any) error {
			if key == "docker.registry_url" {
				return fmt.Errorf("mock error setting registry URL")
			}
			return nil
		}

		mockConfigHandler.GetConfigFunc = func() *v1alpha1.Context {
			return &v1alpha1.Context{
				Docker: &docker.DockerConfig{
					Registries: map[string]docker.RegistryConfig{
						"registry": {},
					},
				},
			}
		}

		// When SetAddress is called
		err := service.SetAddress("localhost")

		// Then there should be an error
		if err == nil {
			t.Error("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "mock error setting registry URL") {
			t.Errorf("expected error containing %q, got %v", "mock error setting registry URL", err)
		}
	})
}

func TestRegistryService_GetHostname(t *testing.T) {
	setup := func(t *testing.T) (*RegistryService, *Mocks) {
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "dns.domain" {
				return "test"
			}
			return defaultValue[0]
		}
		mocks := setupMocks(t, &SetupOptions{
			ConfigHandler: mockConfigHandler,
		})
		service := NewRegistryService(mocks.Injector)
		service.Initialize()
		return service, mocks
	}

	t.Run("SimpleName", func(t *testing.T) {
		// Given a registry service with a simple name
		service, _ := setup(t)
		service.SetName("registry")

		// When getting the hostname
		name := service.GetHostname()

		// Then it should return the name with the TLD
		expected := "registry.test"
		if name != expected {
			t.Errorf("expected hostname %q, got %q", expected, name)
		}
	})

	t.Run("DomainName", func(t *testing.T) {
		// Given a registry service with a domain name
		service, _ := setup(t)
		service.SetName("gcr.io")

		// When getting the hostname
		name := service.GetHostname()

		// Then it should return the name with the last part replaced by TLD
		expected := "gcr.test"
		if name != expected {
			t.Errorf("expected hostname %q, got %q", expected, name)
		}
	})

	t.Run("MultiPartDomain", func(t *testing.T) {
		// Given a registry service with a multi-part domain name
		service, _ := setup(t)
		service.SetName("registry.k8s.io")

		// When getting the hostname
		name := service.GetHostname()

		// Then it should return the name with the last part replaced by TLD
		expected := "registry.k8s.test"
		if name != expected {
			t.Errorf("expected hostname %q, got %q", expected, name)
		}
	})

	t.Run("EmptyName", func(t *testing.T) {
		// Given a registry service with no name
		service, _ := setup(t)
		service.SetName("")

		// When getting the hostname
		name := service.GetHostname()

		// Then it should return an empty string
		if name != "" {
			t.Errorf("expected empty hostname, got %q", name)
		}
	})
}

func TestRegistryService_GetContainerName(t *testing.T) {
	setup := func(t *testing.T) (*RegistryService, *Mocks) {
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "dns.domain" {
				return "test"
			}
			return defaultValue[0]
		}
		mocks := setupMocks(t, &SetupOptions{
			ConfigHandler: mockConfigHandler,
		})
		service := NewRegistryService(mocks.Injector)
		service.Initialize()
		return service, mocks
	}

	t.Run("DomainName", func(t *testing.T) {
		// Given a registry service with a domain name
		service, _ := setup(t)
		service.SetName("gcr.io")

		// When getting the container name
		name := service.GetContainerName()

		// Then it should return the name with the last part replaced by TLD
		expected := "gcr.test"
		if name != expected {
			t.Errorf("expected container name %q, got %q", expected, name)
		}
	})
}

func TestRegistryService_GetName(t *testing.T) {
	setup := func(t *testing.T) (*RegistryService, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		service := NewRegistryService(mocks.Injector)
		service.shims = mocks.Shims
		service.Initialize()
		service.SetName("registry")
		return service, mocks
	}

	t.Run("Success", func(t *testing.T) {
		service, _ := setup(t)

		serviceName := service.GetName()
		if serviceName != "registry" {
			t.Errorf("GetName() = %v, want %v", serviceName, "registry")
		}
	})
}

func TestRegistryService_SupportsWildcard(t *testing.T) {
	setup := func(t *testing.T) (*RegistryService, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		service := NewRegistryService(mocks.Injector)
		service.shims = mocks.Shims
		service.Initialize()
		service.SetName("registry")
		return service, mocks
	}

	t.Run("Success", func(t *testing.T) {
		service, _ := setup(t)
		supports := service.SupportsWildcard()
		if supports {
			t.Errorf("SupportsWildcard() = %v, want %v", supports, false)
		}
	})
}
