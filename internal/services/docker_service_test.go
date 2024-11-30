package services

import (
	"fmt"
	"strings"
	"testing"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/shell"
)

// Mock function for yamlMarshal to simulate an error
var originalYamlMarshal = yamlMarshal

func setupSafeDockerServiceMocks(optionalInjector ...di.Injector) *MockComponents {
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
	injector.Register("dockerService", mockService)

	// Implement GetContextFunc on mock context
	mockContext.GetContextFunc = func() (string, error) {
		return "mock-context", nil
	}

	// Set up the mock config handler to return a safe default configuration for Docker
	mockConfigHandler.GetConfigFunc = func() *config.Context {
		return &config.Context{
			Docker: &config.DockerConfig{
				Enabled: ptrBool(true),
				Registries: []config.Registry{
					{
						Name:   "registry.test",
						Remote: "registry.remote",
						Local:  "registry.local",
					},
				},
				NetworkCIDR: ptrString("10.1.0.0/16"),
			},
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

func TestDockerService_NewDockerService(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given: a set of mock components
		mocks := setupSafeDockerServiceMocks()

		// When: a new DockerService is created
		dockerService := NewDockerService(mocks.Injector)

		// Then: the DockerService should not be nil
		if dockerService == nil {
			t.Fatalf("expected DockerService, got nil")
		}

		// And: the DockerService should have the correct injector
		if dockerService.injector != mocks.Injector {
			t.Errorf("expected injector %v, got %v", mocks.Injector, dockerService.injector)
		}
	})
}

func TestDockerService_GetComposeConfig(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given: a mock config handler, shell, context, and service
		mocks := setupSafeDockerServiceMocks()
		dockerService := NewDockerService(mocks.Injector)
		err := dockerService.Initialize()
		if err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// When: GetComposeConfig is called
		composeConfig, err := dockerService.GetComposeConfig()
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

	t.Run("ErrorGettingContext", func(t *testing.T) {
		// Given: a mock context that returns an error on GetContext
		mocks := setupSafeDockerServiceMocks()
		mocks.MockContext.GetContextFunc = func() (string, error) {
			return "", fmt.Errorf("mock error retrieving context")
		}

		// When: a new DockerService is created and initialized
		dockerService := NewDockerService(mocks.Injector)
		err := dockerService.Initialize()
		if err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// When: GetComposeConfig is called
		_, err = dockerService.GetComposeConfig()

		// Then: an error should be returned
		if err == nil || !strings.Contains(err.Error(), "mock error retrieving context") {
			t.Fatalf("expected error retrieving context, got %v", err)
		}
	})
}
