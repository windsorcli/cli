package services

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/compose-spec/compose-go/types"
	"github.com/stretchr/testify/require"
	"github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/api/v1alpha1/docker"
)

// =============================================================================
// Test Setup
// =============================================================================

// setupRegistryServiceMocks creates and returns mock components for RegistryService tests
func setupRegistryServiceMocks(t *testing.T, opts ...*SetupOptions) *Mocks {
	t.Helper()

	// Create base mocks using setupMocks
	mocks := setupMocks(t, opts...)

	// Set registry-specific configuration
	mocks.ConfigHandler.GetConfigFunc = func() *v1alpha1.Context {
		return &v1alpha1.Context{
			Docker: &docker.DockerConfig{
				Registries: map[string]docker.RegistryConfig{
					"registry": {
						Remote: "registry.remote",
						Local:  "registry.local",
					},
				},
			},
		}
	}
	mocks.ConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
		if key == "vm.driver" {
			return "docker-desktop"
		}
		if key == "dns.domain" {
			return "test"
		}
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return ""
	}

	// Create a generic mock service
	mockService := NewMockService()
	mockService.Initialize()
	mocks.Injector.Register("registryService", mockService)

	// Set up shell project root
	mocks.Shell.GetProjectRootFunc = func() (string, error) {
		return "/mock/project/root", nil
	}

	return mocks
}

// setupRegistryService creates a RegistryService with the given mocks
func setupRegistryService(t *testing.T, mocks *Mocks) *RegistryService {
	t.Helper()

	service := NewRegistryService(mocks.Injector)
	err := service.Initialize()
	require.NoError(t, err)

	return service
}

// =============================================================================
// Test Constructor
// =============================================================================

func TestRegistryService_NewRegistryService(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a set of mock components
		mocks := setupRegistryServiceMocks(t)

		// When a new RegistryService is created
		registryService := NewRegistryService(mocks.Injector)

		// Then the RegistryService should not be nil
		if registryService == nil {
			t.Fatalf("expected RegistryService, got nil")
		}

		// And: the RegistryService should have the correct injector
		if registryService.injector != mocks.Injector {
			t.Errorf("expected injector %v, got %v", mocks.Injector, registryService.injector)
		}
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestRegistryService_GetComposeConfig(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock config handler, shell, context, and service
		mocks := setupRegistryServiceMocks(t)
		registryService := setupRegistryService(t, mocks)
		registryService.SetName("registry")

		// When GetComposeConfig is called
		composeConfig, err := registryService.GetComposeConfig()
		if err != nil {
			t.Fatalf("GetComposeConfig() error = %v", err)
		}

		// Then: the compose configuration should include the registry service
		if composeConfig == nil || len(composeConfig.Services) == 0 {
			t.Fatalf("expected non-nil composeConfig with services, got %v", composeConfig)
		}

		expectedName := "registry"
		expectedRemoteURL := "registry.remote"
		expectedLocalURL := "registry.local"
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
		mocks := setupRegistryServiceMocks(t)
		registryService := setupRegistryService(t, mocks)
		registryService.SetName("nonexistent-registry")

		// When GetComposeConfig is called
		_, err := registryService.GetComposeConfig()

		// Then an error should be returned indicating no registry was found
		if err == nil || !strings.Contains(err.Error(), "no registry found with name") {
			t.Fatalf("expected error indicating no registry found, got %v", err)
		}
	})

	t.Run("MkdirAllFailure", func(t *testing.T) {
		// Given a mock config handler, shell, context, and service
		mocks := setupRegistryServiceMocks(t)
		registryService := setupRegistryService(t, mocks)
		registryService.SetName("registry")

		// Mock mkdirAll to simulate a failure
		originalMkdirAll := mocks.Shims.MkdirAll
		defer func() { mocks.Shims.MkdirAll = originalMkdirAll }()
		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error {
			return fmt.Errorf("mock error creating directory")
		}

		// When GetComposeConfig is called
		_, err := registryService.GetComposeConfig()

		// Then an error should be returned indicating directory creation failure
		if err == nil || !strings.Contains(err.Error(), "mock error creating directory") {
			t.Fatalf("expected error indicating directory creation failure, got %v", err)
		}
	})

	t.Run("ProjectRootRetrievalFailure", func(t *testing.T) {
		// Given a mock config handler, shell, context, and service
		mocks := setupRegistryServiceMocks(t)
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("mock error getting project root")
		}

		// When a new RegistryService is created and initialized
		registryService := setupRegistryService(t, mocks)
		registryService.SetName("registry")

		// When GetComposeConfig is called
		_, err := registryService.GetComposeConfig()

		// Then an error should be returned indicating project root retrieval failure
		if err == nil || !strings.Contains(err.Error(), "mock error getting project root") {
			t.Fatalf("expected error indicating project root retrieval failure, got %v", err)
		}
	})

	t.Run("LocalRegistry", func(t *testing.T) {
		// Given a mock config handler, shell, context, and service
		mocks := setupRegistryServiceMocks(t)
		mocks.ConfigHandler.GetConfigFunc = func() *v1alpha1.Context {
			return &v1alpha1.Context{
				Docker: &docker.DockerConfig{
					Registries: map[string]docker.RegistryConfig{
						"local-registry": {HostPort: 5000},
					},
				},
			}
		}
		// Set vm.driver to docker-desktop for localhost tests
		mocks.ConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "vm.driver" {
				return "docker-desktop"
			}
			if key == "dns.domain" {
				return "test"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}
		registryService := setupRegistryService(t, mocks)
		registryService.SetName("local-registry")

		// Mock the registry configuration to ensure it exists without a remote value
		mocks.ConfigHandler.GetConfigFunc = func() *v1alpha1.Context {
			return &v1alpha1.Context{
				Docker: &docker.DockerConfig{
					Registries: map[string]docker.RegistryConfig{
						"local-registry": {
							HostPort: 5000, // Ensure HostPort is set
						},
					},
				},
			}
		}

		// Set the address to localhost directly
		registryService.address = "localhost"

		// When GetComposeConfig is called
		composeConfig, err := registryService.GetComposeConfig()
		if err != nil {
			t.Fatalf("GetComposeConfig() error = %v", err)
		}

		// Then check that the service has the expected port configuration
		expectedPortConfig := types.ServicePortConfig{
			Target:    5000,
			Published: fmt.Sprintf("%d", registryService.hostPort),
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
	t.Run("Success", func(t *testing.T) {
		mocks := setupRegistryServiceMocks(t)
		registryService := setupRegistryService(t, mocks)

		address := "test-registry:5000"
		err := registryService.SetAddress(address)
		require.NoError(t, err)

		got := registryService.GetAddress()
		require.Equal(t, address, got)
	})

	t.Run("LocalRegistry", func(t *testing.T) {
		mocks := setupRegistryServiceMocks(t)
		registryService := setupRegistryService(t, mocks)

		address := "local-registry:5000"
		err := registryService.SetAddress(address)
		require.NoError(t, err)

		got := registryService.GetAddress()
		require.Equal(t, address, got)
	})

	t.Run("RemoteRegistry", func(t *testing.T) {
		mocks := setupRegistryServiceMocks(t)
		registryService := setupRegistryService(t, mocks)

		address := "remote-registry:5000"
		err := registryService.SetAddress(address)
		require.NoError(t, err)

		got := registryService.GetAddress()
		require.Equal(t, address, got)
	})
}

func TestRegistryService_GetHostname(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupRegistryServiceMocks(t)
		registryService := setupRegistryService(t, mocks)
		registryService.SetName("test-registry")

		hostname := registryService.GetHostname()
		require.Equal(t, "test-registry.test", hostname)
	})

	t.Run("LocalRegistry", func(t *testing.T) {
		mocks := setupRegistryServiceMocks(t)
		registryService := setupRegistryService(t, mocks)
		registryService.SetName("local-registry")

		hostname := registryService.GetHostname()
		require.Equal(t, "local-registry.test", hostname)
	})

	t.Run("RemoteRegistry", func(t *testing.T) {
		mocks := setupRegistryServiceMocks(t)
		registryService := setupRegistryService(t, mocks)
		registryService.SetName("remote-registry")

		hostname := registryService.GetHostname()
		require.Equal(t, "remote-registry.test", hostname)
	})
}

func TestRegistryService_GetName(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupRegistryServiceMocks(t)
		registryService := setupRegistryService(t, mocks)
		registryService.SetName("registry")

		serviceName := registryService.GetName()
		require.Equal(t, "registry", serviceName)
	})
}

func TestRegistryService_SupportsWildcard(t *testing.T) {
	mocks := setupRegistryServiceMocks(t)
	registryService := setupRegistryService(t, mocks)

	supports := registryService.SupportsWildcard()
	require.False(t, supports)
}
