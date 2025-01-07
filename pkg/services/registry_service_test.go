package services

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/config/docker"
	"github.com/windsorcli/cli/pkg/context"
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

	mockContext := context.NewMockContext()
	mockShell := shell.NewMockShell(injector)
	mockConfigHandler := config.NewMockConfigHandler()
	mockService := NewMockService()

	// Register mock instances in the injector
	injector.Register("contextHandler", mockContext)
	injector.Register("shell", mockShell)
	injector.Register("configHandler", mockConfigHandler)
	injector.Register("registryService", mockService)

	// Implement GetContextFunc on mock context
	mockContext.GetContextFunc = func() string {
		return "mock-context"
	}

	// Set up the mock config handler to return a safe default configuration for Registry
	mockConfigHandler.GetConfigFunc = func() *config.Context {
		return &config.Context{
			Docker: &docker.DockerConfig{
				Enabled: ptrBool(true),
				Registries: map[string]docker.RegistryConfig{
					"registry": {
						Remote: "registry.remote",
						Local:  "registry.local",
					},
				},
				NetworkCIDR: ptrString("10.5.0.0/16"),
			},
		}
	}

	// Ensure the GetString method returns "test" for the key "dns.name"
	mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
		switch key {
		case "dns.name":
			return "test"
		default:
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}
	}

	return &MockComponents{
		Injector:          injector,
		MockContext:       mockContext,
		MockShell:         mockShell,
		MockConfigHandler: mockConfigHandler,
		MockService:       mockService,
	}
}

func TestRegistryService_NewRegistryService(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given: a set of mock components
		mocks := setupSafeRegistryServiceMocks()

		// When: a new RegistryService is created
		registryService := NewRegistryService(mocks.Injector)

		// Then: the RegistryService should not be nil
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
		// Given: a mock config handler, shell, context, and service
		mocks := setupSafeRegistryServiceMocks()
		registryService := NewRegistryService(mocks.Injector)
		registryService.SetName("registry")
		err := registryService.Initialize()
		if err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// When: GetComposeConfig is called
		composeConfig, err := registryService.GetComposeConfig()
		if err != nil {
			t.Fatalf("GetComposeConfig() error = %v", err)
		}

		// Then: check for characteristic properties in the configuration
		expectedName := "registry.test"
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
		// Given: a mock config handler, shell, context, and service
		mocks := setupSafeRegistryServiceMocks()

		// When: a new RegistryService is created and initialized
		registryService := NewRegistryService(mocks.Injector)
		registryService.SetName("nonexistent-registry")
		err := registryService.Initialize()
		if err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// When: GetComposeConfig is called
		_, err = registryService.GetComposeConfig()

		// Then: an error should be returned indicating no registry was found
		if err == nil || !strings.Contains(err.Error(), "no registry found with name") {
			t.Fatalf("expected error indicating no registry found, got %v", err)
		}
	})

	t.Run("MkdirAllFailure", func(t *testing.T) {
		// Mock mkdirAll to simulate a failure
		originalMkdirAll := mkdirAll
		defer func() { mkdirAll = originalMkdirAll }()
		mkdirAll = func(path string, perm os.FileMode) error {
			return fmt.Errorf("mock error creating directory")
		}

		// Given: a mock config handler, shell, context, and service
		mocks := setupSafeRegistryServiceMocks()
		registryService := NewRegistryService(mocks.Injector)
		registryService.SetName("registry")
		err := registryService.Initialize()
		if err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// When: GetComposeConfig is called
		_, err = registryService.GetComposeConfig()

		// Then: an error should be returned indicating directory creation failure
		if err == nil || !strings.Contains(err.Error(), "mock error creating directory") {
			t.Fatalf("expected error indicating directory creation failure, got %v", err)
		}
	})

	t.Run("LocalhostPortAssignment", func(t *testing.T) {
		// Mock mkdirAll to simulate successful directory creation
		originalMkdirAll := mkdirAll
		defer func() { mkdirAll = originalMkdirAll }()
		mkdirAll = func(path string, perm os.FileMode) error {
			return nil
		}

		// Given: a mock config handler, shell, context, and service
		mocks := setupSafeRegistryServiceMocks()
		mocks.MockConfigHandler.GetConfigFunc = func() *config.Context {
			return &config.Context{
				Docker: &docker.DockerConfig{
					Enabled: ptrBool(true),
					Registries: map[string]docker.RegistryConfig{
						"registry": {
							Remote: "",
							Local:  "",
						},
					},
					NetworkCIDR: ptrString("10.5.0.0/16"),
				},
			}
		}
		registryService := NewRegistryService(mocks.Injector)
		err := registryService.Initialize()
		if err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}
		registryService.SetName("registry")
		registryService.SetAddress("127.0.0.1")

		// When: GetComposeConfig is called
		composeConfig, err := registryService.GetComposeConfig()
		if err != nil {
			t.Fatalf("GetComposeConfig() error = %v", err)
		}

		// Then: check if a port greater than or equal to 5000 is assigned
		found := false
		for _, config := range composeConfig.Services {
			if config.Name == "registry.test" {
				for _, portConfig := range config.Ports {
					if portConfig.Published >= "5000" {
						found = true
						break
					}
				}
			}
		}

		if !found {
			t.Errorf("expected a port >= 5000 to be assigned for localhost, but none was found in the configuration:\n%+v", composeConfig.Services)
		}
	})
}

func TestRegistryService_SetAddress(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given: a mock config handler, shell, context, and service
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

		// When: SetAddress is called with a valid address
		address := "192.168.1.1"
		err = registryService.SetAddress(address)
		if err != nil {
			t.Fatalf("SetAddress() error = %v", err)
		}

		// Then: verify SetContextValue was called
		if !setContextValueCalled {
			t.Errorf("expected SetContextValue to be called, but it was not")
		}
	})

	t.Run("SetAddressError", func(t *testing.T) {
		// Given: a mock config handler, shell, context, and service
		mocks := setupSafeRegistryServiceMocks()
		registryService := NewRegistryService(mocks.Injector)
		registryService.SetName("registry")
		err := registryService.Initialize()
		if err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// When: SetAddress is called with an invalid address
		address := "invalid-address"
		err = registryService.SetAddress(address)

		// Then: an error should be returned indicating invalid IPv4 address
		if err == nil || !strings.Contains(err.Error(), "invalid IPv4 address") {
			t.Fatalf("expected error indicating invalid IPv4 address, got %v", err)
		}
	})

	t.Run("SetHostnameError", func(t *testing.T) {
		// Given: a mock config handler that will fail to set context value
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

		// When: SetAddress is called
		address := "192.168.1.1"
		err = registryService.SetAddress(address)

		// Then: an error should be returned
		if err == nil || !strings.Contains(err.Error(), "failed to set hostname for registry") {
			t.Fatalf("expected error indicating failure to set hostname, got %v", err)
		}
	})
}

func TestRegistryService_GetHostname(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given: a mock config handler, shell, context, and service
		mocks := setupSafeRegistryServiceMocks()
		registryService := NewRegistryService(mocks.Injector)
		registryService.SetName("registry.oldtld")
		err := registryService.Initialize()
		if err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// When: GetHostname is called
		hostname := registryService.GetHostname()

		// Then: the hostname should be as expected, with the old TLD removed
		expectedHostname := "registry.test"
		if hostname != expectedHostname {
			t.Fatalf("expected hostname '%s', got %v", expectedHostname, hostname)
		}
	})
}
