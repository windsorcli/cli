package services

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/compose-spec/compose-go/types"
	"github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/api/v1alpha1/docker"
	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
)

// Mock function for yamlMarshal to simulate an error
var originalYamlMarshal = yamlMarshal

func setupSafeRegistryServiceMocks(optionalInjector ...di.Injector) *MockComponents {
	var injector di.Injector
	if len(optionalInjector) > 0 {
		injector = optionalInjector[0]
	} else {
		injector = di.NewMockInjector()
	}

	mockShell := shell.NewMockShell(injector)
	mockConfigHandler := config.NewMockConfigHandler()
	mockService := NewMockService()

	// Register mock instances in the injector
	injector.Register("shell", mockShell)
	injector.Register("configHandler", mockConfigHandler)
	injector.Register("registryService", mockService)

	// Implement GetContextFunc on mock context
	mockConfigHandler.GetContextFunc = func() string {
		return "mock-context"
	}

	// Set up the mock config handler to return a safe default configuration for Registry
	mockConfigHandler.GetConfigFunc = func() *v1alpha1.Context {
		return &v1alpha1.Context{
			Docker: &docker.DockerConfig{
				Enabled: ptrBool(true),
				Registries: map[string]docker.RegistryConfig{
					"registry": {
						Remote: "registry.remote",
						Local:  "registry.local",
					},
				},
			},
		}
	}

	// Ensure the GetString method returns "test" for the key "dns.domain"
	mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
		switch key {
		case "dns.domain":
			return "test"
		default:
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}
	}

	// Mock mkdirAll to simulate success by default
	mkdirAll = func(path string, perm os.FileMode) error {
		return nil
	}

	return &MockComponents{
		Injector:          injector,
		MockShell:         mockShell,
		MockConfigHandler: mockConfigHandler,
		MockService:       mockService,
	}
}

func TestRegistryService_NewRegistryService(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a set of mock components
		mocks := setupSafeRegistryServiceMocks()

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

func TestRegistryService_GetComposeConfig(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock config handler, shell, context, and service
		mocks := setupSafeRegistryServiceMocks()
		registryService := NewRegistryService(mocks.Injector)
		registryService.SetName("registry")
		err := registryService.Initialize()
		if err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

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
		mocks := setupSafeRegistryServiceMocks()

		// When a new RegistryService is created and initialized
		registryService := NewRegistryService(mocks.Injector)
		registryService.SetName("nonexistent-registry")
		err := registryService.Initialize()
		if err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// When GetComposeConfig is called
		_, err = registryService.GetComposeConfig()

		// Then an error should be returned indicating no registry was found
		if err == nil || !strings.Contains(err.Error(), "no registry found with name") {
			t.Fatalf("expected error indicating no registry found, got %v", err)
		}
	})

	t.Run("MkdirAllFailure", func(t *testing.T) {
		// Given a mock config handler, shell, context, and service
		mocks := setupSafeRegistryServiceMocks()
		registryService := NewRegistryService(mocks.Injector)
		registryService.SetName("registry")
		err := registryService.Initialize()
		if err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// Mock mkdirAll to simulate a failure
		originalMkdirAll := mkdirAll
		defer func() { mkdirAll = originalMkdirAll }()
		mkdirAll = func(path string, perm os.FileMode) error {
			return fmt.Errorf("mock error creating directory")
		}

		// When GetComposeConfig is called
		_, err = registryService.GetComposeConfig()

		// Then an error should be returned indicating directory creation failure
		if err == nil || !strings.Contains(err.Error(), "mock error creating directory") {
			t.Fatalf("expected error indicating directory creation failure, got %v", err)
		}
	})

	t.Run("ProjectRootRetrievalFailure", func(t *testing.T) {
		// Given a mock config handler, shell, context, and service
		mocks := setupSafeRegistryServiceMocks()
		mocks.MockShell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("mock error retrieving project root")
		}
		registryService := NewRegistryService(mocks.Injector)
		registryService.SetName("registry")
		err := registryService.Initialize()
		if err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// When GetComposeConfig is called
		_, err = registryService.GetComposeConfig()

		// Then an error should be returned indicating project root retrieval failure
		if err == nil || !strings.Contains(err.Error(), "mock error retrieving project root") {
			t.Fatalf("expected error indicating project root retrieval failure, got %v", err)
		}
	})

	t.Run("LocalRegistry", func(t *testing.T) {
		// Given a mock config handler, shell, context, and service
		mocks := setupSafeRegistryServiceMocks()
		mocks.MockConfigHandler.GetConfigFunc = func() *v1alpha1.Context {
			return &v1alpha1.Context{
				Docker: &docker.DockerConfig{
					Registries: map[string]docker.RegistryConfig{
						"local-registry": {HostPort: 5000},
					},
				},
			}
		}
		// Set vm.driver to docker-desktop for localhost tests
		mocks.MockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
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
		registryService := NewRegistryService(mocks.Injector)
		registryService.SetName("local-registry")
		err := registryService.Initialize()
		if err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// Mock the registry configuration to ensure it exists without a remote value
		mocks.MockConfigHandler.GetConfigFunc = func() *v1alpha1.Context {
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
		// Given a mock config handler, shell, context, and service
		mocks := setupSafeRegistryServiceMocks()
		registryService := NewRegistryService(mocks.Injector)
		registryService.SetName("registry")
		err := registryService.Initialize()
		if err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// Mock the SetContextValue function to track if it's called
		setContextValueCalled := false
		mocks.MockConfigHandler.SetContextValueFunc = func(key string, value interface{}) error {
			setContextValueCalled = true
			return nil
		}

		// When SetAddress is called with a valid address
		address := "192.168.1.1"
		err = registryService.SetAddress(address)
		if err != nil {
			t.Fatalf("SetAddress() error = %v", err)
		}

		// Then verify SetContextValue was called
		if !setContextValueCalled {
			t.Errorf("expected SetContextValue to be called, but it was not")
		}
	})

	t.Run("SetAddressError", func(t *testing.T) {
		// Given a mock config handler, shell, context, and service
		mocks := setupSafeRegistryServiceMocks()
		registryService := NewRegistryService(mocks.Injector)
		registryService.SetName("registry")
		err := registryService.Initialize()
		if err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// When SetAddress is called with an invalid address
		address := "invalid-address"
		err = registryService.SetAddress(address)

		// Then an error should be returned indicating invalid IPv4 address
		if err == nil || !strings.Contains(err.Error(), "invalid IPv4 address") {
			t.Fatalf("expected error indicating invalid IPv4 address, got %v", err)
		}
	})

	t.Run("SetHostnameError", func(t *testing.T) {
		// Given a mock config handler that will fail to set context value
		mocks := setupSafeRegistryServiceMocks()
		mocks.MockConfigHandler.SetContextValueFunc = func(path string, value interface{}) error {
			return fmt.Errorf("failed to set context value")
		}
		registryService := NewRegistryService(mocks.Injector)
		registryService.SetName("registry")
		err := registryService.Initialize()
		if err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// When SetAddress is called
		address := "192.168.1.1"
		err = registryService.SetAddress(address)

		// Then an error should be returned
		if err == nil || !strings.Contains(err.Error(), "failed to set hostname for registry") {
			t.Fatalf("expected error indicating failure to set hostname, got %v", err)
		}
	})

	t.Run("NoHostPortSetAndLocalhost", func(t *testing.T) {
		// Given a mock config handler, shell, context, and service with no HostPort
		mocks := setupSafeRegistryServiceMocks()
		mocks.MockConfigHandler.GetConfigFunc = func() *v1alpha1.Context {
			return &v1alpha1.Context{
				Docker: &docker.DockerConfig{
					Registries: map[string]docker.RegistryConfig{
						"registry": {HostPort: 0},
					},
				},
			}
		}
		// Set vm.driver to docker-desktop for localhost tests
		mocks.MockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
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
		registryService := NewRegistryService(mocks.Injector)
		registryService.SetName("registry")
		err := registryService.Initialize()
		if err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// When SetAddress is called with localhost
		address := "127.0.0.1"
		err = registryService.SetAddress(address)
		if err != nil {
			t.Fatalf("SetAddress() error = %v", err)
		}

		// Then the default port should be set
		if registryService.hostPort != constants.REGISTRY_DEFAULT_HOST_PORT {
			t.Errorf("expected HostPort to be set to default, got %v", registryService.hostPort)
		}
	})

	t.Run("HostPortSetAndAvailable", func(t *testing.T) {
		// Given a mock config handler, shell, context, and service with HostPort set
		mocks := setupSafeRegistryServiceMocks()
		mocks.MockConfigHandler.GetConfigFunc = func() *v1alpha1.Context {
			return &v1alpha1.Context{
				Docker: &docker.DockerConfig{
					Registries: map[string]docker.RegistryConfig{
						"registry": {HostPort: 5000},
					},
				},
			}
		}
		registryService := NewRegistryService(mocks.Injector)
		registryService.SetName("registry")
		err := registryService.Initialize()
		if err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// When SetAddress is called
		address := "192.168.1.1"
		err = registryService.SetAddress(address)
		if err != nil {
			t.Fatalf("SetAddress() error = %v", err)
		}

		// Then the HostPort should be set to the configured port
		if registryService.hostPort != 5000 {
			t.Errorf("expected HostPort to be 5000, got %v", registryService.hostPort)
		}
	})

	t.Run("SetRegistryURLAndHostPort", func(t *testing.T) {
		// Reset global state
		localRegistry = nil
		registryNextPort = constants.REGISTRY_DEFAULT_HOST_PORT + 1

		// Setup mock components
		mockConfig := config.NewMockConfigHandler()
		mockConfig.GetConfigFunc = func() *v1alpha1.Context {
			return &v1alpha1.Context{
				Docker: &docker.DockerConfig{
					Registries: map[string]docker.RegistryConfig{
						"test-registry": {
							HostPort: 0,
							Remote:   "",
						},
					},
				},
			}
		}

		mockConfig.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "vm.driver" {
				return "docker-desktop"
			}
			if key == "dns.domain" {
				return "test"
			}
			return ""
		}

		var setContextValueCalls = make(map[string]interface{})
		mockConfig.SetContextValueFunc = func(key string, value interface{}) error {
			setContextValueCalls[key] = value
			return nil
		}

		// Initialize service
		service := NewRegistryService(di.NewInjector())
		service.name = "test-registry"
		service.configHandler = mockConfig

		// Set address
		err := service.SetAddress("127.0.0.1")
		if err != nil {
			t.Fatalf("SetAddress failed: %v", err)
		}

		// Verify default port was set
		if service.hostPort != constants.REGISTRY_DEFAULT_HOST_PORT {
			t.Errorf("Expected hostPort to be %d, got %d", constants.REGISTRY_DEFAULT_HOST_PORT, service.hostPort)
		}

		// Verify hostname was set
		expectedHostname := "test-registry.test"
		if value, exists := setContextValueCalls["docker.registries[test-registry].hostname"]; !exists {
			t.Error("Expected SetContextValue to be called for hostname, but it was not")
		} else if value != expectedHostname {
			t.Errorf("Expected hostname to be %q, got %q", expectedHostname, value)
		}

		// Verify registry URL was set
		if value, exists := setContextValueCalls["docker.registry_url"]; !exists {
			t.Error("Expected SetContextValue to be called for registry URL, but it was not")
		} else if value != expectedHostname {
			t.Errorf("Expected registry URL to be %q, got %q", expectedHostname, value)
		}

		// Verify hostport was set
		if value, exists := setContextValueCalls["docker.registries[test-registry].hostport"]; !exists {
			t.Error("Expected SetContextValue to be called for hostport, but it was not")
		} else if value != constants.REGISTRY_DEFAULT_HOST_PORT {
			t.Errorf("Expected hostport to be %d, got %d", constants.REGISTRY_DEFAULT_HOST_PORT, value)
		}
	})

	t.Run("SetContextValueErrorForRegistryURL", func(t *testing.T) {
		// Reset global state
		localRegistry = nil
		registryNextPort = constants.REGISTRY_DEFAULT_HOST_PORT + 1

		// Setup mock components
		mockConfig := config.NewMockConfigHandler()
		mockConfig.GetConfigFunc = func() *v1alpha1.Context {
			return &v1alpha1.Context{
				Docker: &docker.DockerConfig{
					Registries: map[string]docker.RegistryConfig{
						"test-registry": {
							HostPort: 0,
							Remote:   "",
						},
					},
				},
			}
		}

		mockConfig.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "vm.driver" {
				return "docker-desktop"
			}
			if key == "dns.domain" {
				return "test"
			}
			return ""
		}

		mockConfig.SetContextValueFunc = func(key string, value interface{}) error {
			if key == "docker.registry_url" {
				return fmt.Errorf("failed to set registry URL")
			}
			return nil
		}

		// Initialize service
		service := NewRegistryService(di.NewInjector())
		service.name = "test-registry"
		service.configHandler = mockConfig

		// Set address
		err := service.SetAddress("127.0.0.1")

		// Verify error
		if err == nil || !strings.Contains(err.Error(), "failed to set registry URL") {
			t.Errorf("Expected error containing 'failed to set registry URL', got %v", err)
		}
	})

	t.Run("SuccessWithNextPort", func(t *testing.T) {
		// Reset package-level variables
		registryNextPort = constants.REGISTRY_DEFAULT_HOST_PORT + 1
		localRegistry = nil

		// Given a mock config handler, shell, context, and service
		mocks := setupSafeRegistryServiceMocks()

		// Override GetConfig to return a config with an empty registry
		mocks.MockConfigHandler.GetConfigFunc = func() *v1alpha1.Context {
			return &v1alpha1.Context{
				Docker: &docker.DockerConfig{
					Registries: map[string]docker.RegistryConfig{
						"test-registry": {
							Remote: "",
						},
					},
				},
			}
		}

		// Override GetString to return docker-desktop for vm.driver
		mocks.MockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
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

		var setContextValueCalls = make(map[string]interface{})
		mocks.MockConfigHandler.SetContextValueFunc = func(key string, value interface{}) error {
			setContextValueCalls[key] = value
			return nil
		}

		// Initialize service
		service := NewRegistryService(mocks.Injector)
		service.name = "test-registry"
		err := service.Initialize()
		if err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// Call SetAddress
		err = service.SetAddress("127.0.0.1")

		// Assert no error occurred
		if err != nil {
			t.Fatalf("SetAddress() error = %v", err)
		}

		// Verify that SetContextValue was called for the registry host port
		if value, exists := setContextValueCalls["docker.registries[test-registry].hostport"]; !exists {
			t.Error("Expected SetContextValue to be called for host port")
		} else if value != constants.REGISTRY_DEFAULT_HOST_PORT {
			t.Errorf("Expected SetContextValue value to be %d, got %v", constants.REGISTRY_DEFAULT_HOST_PORT, value)
		}

		// Call SetAddress again to verify port increment
		err = service.SetAddress("127.0.0.1")

		// Assert no error occurred
		if err != nil {
			t.Fatalf("SetAddress() error = %v", err)
		}

		// Verify that SetContextValue was called for the registry host port with incremented value
		if value, exists := setContextValueCalls["docker.registries[test-registry].hostport"]; !exists {
			t.Error("Expected SetContextValue to be called for host port")
		} else if value != constants.REGISTRY_DEFAULT_HOST_PORT+1 {
			t.Errorf("Expected SetContextValue value to be %d, got %v", constants.REGISTRY_DEFAULT_HOST_PORT+1, value)
		}
	})

	t.Run("SetContextValueErrorForHostPort", func(t *testing.T) {
		// Reset global state
		localRegistry = nil
		registryNextPort = constants.REGISTRY_DEFAULT_HOST_PORT + 1

		// Setup mock components
		mockConfig := config.NewMockConfigHandler()
		mockConfig.GetConfigFunc = func() *v1alpha1.Context {
			return &v1alpha1.Context{
				Docker: &docker.DockerConfig{
					Registries: map[string]docker.RegistryConfig{
						"test-registry": {
							HostPort: 0,
							Remote:   "",
						},
					},
				},
			}
		}

		mockConfig.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "vm.driver" {
				return "docker-desktop"
			}
			if key == "dns.domain" {
				return "test"
			}
			return ""
		}

		mockConfig.SetContextValueFunc = func(key string, value interface{}) error {
			if key == "docker.registries[test-registry].hostport" {
				return fmt.Errorf("failed to set host port")
			}
			return nil
		}

		// Initialize service
		service := NewRegistryService(di.NewInjector())
		service.name = "test-registry"
		service.configHandler = mockConfig

		// Set address
		err := service.SetAddress("127.0.0.1")

		// Verify error
		if err == nil || !strings.Contains(err.Error(), "failed to set host port for registry test-registry") {
			t.Errorf("Expected error containing 'failed to set host port for registry test-registry', got %v", err)
		}
	})
}

func TestRegistryService_GetHostname(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Setup mock components
		mockConfig := config.NewMockConfigHandler()
		mockConfig.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "dns.domain" {
				return "test"
			}
			return ""
		}

		// Initialize service
		service := NewRegistryService(di.NewInjector())
		service.name = "registry.oldtld"
		service.configHandler = mockConfig

		// Get hostname
		hostname := service.GetHostname()

		// Verify hostname
		expectedHostname := "registry.test"
		if hostname != expectedHostname {
			t.Errorf("Expected hostname %q, got %q", expectedHostname, hostname)
		}
	})
}

func createRegistryServiceMocks() *MockComponents {
	mockShell := shell.NewMockShell(di.NewInjector())
	mockConfig := config.NewMockConfigHandler()
	mockService := NewMockService()
	injector := di.NewInjector()
	injector.Register("shell", mockShell)
	injector.Register("configHandler", mockConfig)
	injector.Register("registryService", mockService)
	return &MockComponents{
		Injector:          injector,
		MockShell:         mockShell,
		MockConfigHandler: mockConfig,
		MockService:       mockService,
	}
}

func ptrInt(i int) *int {
	return &i
}
