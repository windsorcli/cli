package helpers

import (
	"strings"
	"testing"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/shell"
)

func TestKubeHelper_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given: a mock config handler and context
		mockConfigHandler := config.NewMockConfigHandler()
		mockContext := context.NewMockContext()

		// Create DI container and register mocks
		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("contextHandler", mockContext)

		// Create an instance of KubeHelper
		kubeHelper, err := NewKubeHelper(diContainer)
		if err != nil {
			t.Fatalf("NewKubeHelper() error = %v", err)
		}

		// When: Initialize is called
		err = kubeHelper.Initialize()
		if err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// Then: no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}

func TestKubeHelper_NewKubeHelper(t *testing.T) {
	t.Run("ErrorResolvingContext", func(t *testing.T) {
		// Given a DI container without registering context
		diContainer := di.NewContainer()
		mockConfigHandler := config.NewMockConfigHandler()
		diContainer.Register("cliConfigHandler", mockConfigHandler)

		// When attempting to create KubeHelper
		_, err := NewKubeHelper(diContainer)

		// Then it should return an error indicating context resolution failure
		if err == nil || !strings.Contains(err.Error(), "error resolving context") {
			t.Fatalf("expected error resolving context, got %v", err)
		}
	})
}

func TestKubeHelper_GetContainerConfig(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock context
		mockContext := context.NewMockContext()
		container := di.NewContainer()
		container.Register("contextHandler", mockContext)

		// When creating KubeHelper
		kubeHelper, err := NewKubeHelper(container)
		if err != nil {
			t.Fatalf("NewKubeHelper() error = %v", err)
		}

		// And calling GetComposeConfig
		composeConfig, err := kubeHelper.GetComposeConfig()
		if err != nil {
			t.Fatalf("GetComposeConfig() error = %v", err)
		}

		// Then the result should be nil as per the stub implementation
		if composeConfig != nil {
			t.Errorf("expected nil, got %v", composeConfig)
		}
	})
}

func TestKubeHelper_WriteConfig(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given: a mock config handler and context
		mockConfigHandler := config.NewMockConfigHandler()
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}
		mockContext.GetConfigRootFunc = func() (string, error) {
			return "/path/to/config", nil
		}

		// Create DI container and register mocks
		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("contextHandler", mockContext)

		// Create an instance of AwsHelper
		kubeHelper, err := NewKubeHelper(diContainer)
		if err != nil {
			t.Fatalf("NewKubeHelper() error = %v", err)
		}

		// When: WriteConfig is called
		err = kubeHelper.WriteConfig()
		if err != nil {
			t.Fatalf("WriteConfig() error = %v", err)
		}

		// Then: no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}

func TestKubeHelper_Up(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Create DI container and register mocks
		diContainer := di.NewContainer()
		mockConfigHandler := config.NewMockConfigHandler()
		mockContext := context.NewMockContext()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("contextHandler", mockContext)

		// Create an instance of KubeHelper
		kubeHelper, err := NewKubeHelper(diContainer)
		if err != nil {
			t.Fatalf("NewKubeHelper() error = %v", err)
		}

		// When: Up is called
		err = kubeHelper.Up()
		if err != nil {
			t.Fatalf("Up() error = %v", err)
		}
	})
}

func TestKubeHelper_Info(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Create DI container and register mocks
		diContainer := di.NewContainer()
		mockConfigHandler := config.NewMockConfigHandler()
		mockContext := context.NewMockContext()
		mockShell := shell.NewMockShell()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("contextHandler", mockContext)
		diContainer.Register("shell", mockShell)

		// Create an instance of KubeHelper
		kubeHelper, err := NewKubeHelper(diContainer)
		if err != nil {
			t.Fatalf("NewKubeHelper() error = %v", err)
		}

		// When: Info is called
		info, err := kubeHelper.Info()
		if err != nil {
			t.Fatalf("Info() error = %v", err)
		}

		// Then: no error should be returned and info should be nil
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if info != nil {
			t.Errorf("Expected info to be nil, got %v", info)
		}
	})
}
