package helpers

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/compose-spec/compose-go/types"
	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/shell"
)

// Mock function for yamlMarshal to simulate an error
var originalYamlMarshal = yamlMarshal

func TestDockerHelper_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given: a mock config handler, context, and helper
		mockConfigHandler := config.NewMockConfigHandler()
		mockContext := context.NewMockContext()
		mockHelper := NewMockHelper()

		// Create injector and register mocks
		diContainer := di.NewInjector()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("contextHandler", mockContext)
		diContainer.Register("helper", mockHelper)
		diContainer.Register("shell", shell.NewMockShell())

		// Create an instance of DockerHelper
		dockerHelper, err := NewDockerHelper(diContainer)
		if err != nil {
			t.Fatalf("NewDockerHelper() error = %v", err)
		}

		// When: Initialize is called
		err = dockerHelper.Initialize()
		if err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// Then: no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}

func TestDockerHelper_NewDockerHelper(t *testing.T) {
	t.Run("ErrorResolvingConfigHandler", func(t *testing.T) {
		// Create injector without registering cliConfigHandler
		diContainer := di.NewInjector()

		// Attempt to create DockerHelper
		_, err := NewDockerHelper(diContainer)
		if err == nil || !strings.Contains(err.Error(), "error resolving cliConfigHandler") {
			t.Fatalf("expected error resolving cliConfigHandler, got %v", err)
		}
	})

	t.Run("ErrorResolvingContext", func(t *testing.T) {
		// Create injector and register only cliConfigHandler
		diContainer := di.NewInjector()
		mockConfigHandler := config.NewMockConfigHandler()
		diContainer.Register("cliConfigHandler", mockConfigHandler)

		// Attempt to create DockerHelper
		_, err := NewDockerHelper(diContainer)
		if err == nil || !strings.Contains(err.Error(), "error resolving context") {
			t.Fatalf("expected error resolving context, got %v", err)
		}
	})

	t.Run("ErrorResolvingShell", func(t *testing.T) {
		// Create injector and register cliConfigHandler and context
		diContainer := di.NewInjector()
		mockConfigHandler := config.NewMockConfigHandler()
		mockContext := context.NewMockContext()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("contextHandler", mockContext)

		// Attempt to create DockerHelper
		_, err := NewDockerHelper(diContainer)
		if err == nil || !strings.Contains(err.Error(), "error resolving shell") {
			t.Fatalf("expected error resolving shell, got %v", err)
		}
	})
}

func TestDockerHelper_GetComposeConfig(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given: a mock config handler, shell, context, and helper
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Docker: &config.DockerConfig{
					Registries: []config.Registry{
						{
							Name:   "registry.test",
							Remote: "registry.remote",
							Local:  "registry.local",
						},
					},
				},
			}, nil
		}

		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}

		// Create injector and register mocks
		diContainer := di.NewInjector()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("contextHandler", mockContext)

		// Register MockHelper
		mockHelper := NewMockHelper()
		diContainer.Register("helper", mockHelper)

		// Register MockShell
		mockShell := shell.NewMockShell()
		diContainer.Register("shell", mockShell)

		helper, err := NewDockerHelper(diContainer)
		if err != nil {
			t.Fatalf("NewDockerHelper() error = %v", err)
		}

		// When: GetComposeConfig is called
		composeConfig, err := helper.GetComposeConfig()
		if err != nil {
			t.Fatalf("GetComposeConfig() error = %v", err)
		}

		// Then: the result should match the expected configuration
		localURL := "registry.local"
		remoteURL := "registry.remote"
		expectedConfig := types.ServiceConfig{
			Name:    "registry.test",
			Image:   "registry:2.8.3",
			Restart: "always",
			Labels: map[string]string{
				"managed_by": "windsor",
				"role":       "registry",
				"context":    "test-context",
			},
			Environment: map[string]*string{
				"REGISTRY_PROXY_LOCALURL":  &localURL,
				"REGISTRY_PROXY_REMOTEURL": &remoteURL,
			},
		}

		found := false
		for _, config := range composeConfig.Services {
			if reflect.DeepEqual(config, expectedConfig) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected configuration:\n%+v\nto be in the list of configurations:\n%+v", expectedConfig, composeConfig.Services)
		}
	})

	t.Run("ErrorRetrievingRegistries", func(t *testing.T) {
		// Given: a mock context and config handler that returns an error for GetConfig
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}
		mockConfigHandlerWithError := config.NewMockConfigHandler()
		mockConfigHandlerWithError.GetConfigFunc = func() (*config.Context, error) {
			return nil, errors.New("mock error retrieving registries")
		}

		// Create injector and register mocks
		diContainer := di.NewInjector()
		diContainer.Register("contextHandler", mockContext)
		diContainer.Register("cliConfigHandler", mockConfigHandlerWithError)

		// Register MockHelper
		mockHelper := NewMockHelper()
		diContainer.Register("helper", mockHelper)

		// Register MockShell
		mockShell := shell.NewMockShell()
		diContainer.Register("shell", mockShell)

		// Create DockerHelper
		helper, err := NewDockerHelper(diContainer)
		if err != nil {
			t.Fatalf("NewDockerHelper() error = %v", err)
		}

		// When: GetComposeConfig is called
		_, err = helper.GetComposeConfig()

		// Then: it should return an error indicating the failure to retrieve registries
		expectedError := "error retrieving context configuration: mock error retrieving registries"
		if err == nil || !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("expected error %v, got %v", expectedError, err)
		}
	})

	// Test error retrieving context
	t.Run("ErrorRetrievingContext", func(t *testing.T) {
		// Given: a mock context and config handler that returns an error for GetContext
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "", errors.New("mock error retrieving context")
		}
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Docker: &config.DockerConfig{
					Registries: []config.Registry{
						{Name: "registry1"},
					},
				},
			}, nil
		}

		// Create injector and register mocks
		diContainer := di.NewInjector()
		diContainer.Register("contextHandler", mockContext)
		diContainer.Register("cliConfigHandler", mockConfigHandler)

		// Register MockHelper
		mockHelper := NewMockHelper()
		mockHelper.GetComposeConfigFunc = func() (*types.Config, error) {
			return &types.Config{
				Services: []types.ServiceConfig{
					{Name: "registry1"},
				},
			}, nil
		}
		diContainer.Register("helper", mockHelper)

		// Register MockShell
		mockShell := shell.NewMockShell()
		diContainer.Register("shell", mockShell)

		// Create DockerHelper
		helper, err := NewDockerHelper(diContainer)
		if err != nil {
			t.Fatalf("NewDockerHelper() error = %v", err)
		}

		// When: GetComposeConfig is called
		_, err = helper.GetComposeConfig()

		// Then: it should return an error indicating the failure to retrieve context
		expectedError := "error retrieving context: mock error retrieving context"
		if err == nil || !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("expected error %v, got %v", expectedError, err)
		}
	})
}
