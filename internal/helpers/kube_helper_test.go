package helpers

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
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
		diContainer.Register("contextInstance", mockContext)

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

func TestKubeHelper_GetEnvVars(t *testing.T) {
	t.Run("ValidConfigRoot", func(t *testing.T) {
		// Given a valid context path
		contextPath := filepath.Join(os.TempDir(), "contexts", "test-context")
		kubeConfigPath := filepath.Join(contextPath, ".kube", "config")

		// And the directory and kubeconfig file are created
		err := os.MkdirAll(filepath.Join(contextPath, ".kube"), os.ModePerm)
		if err != nil {
			t.Fatalf("Failed to create directories: %v", err)
		}
		_, err = os.Create(kubeConfigPath)
		if err != nil {
			t.Fatalf("Failed to create kubeconfig file: %v", err)
		}
		defer os.RemoveAll(filepath.Join(os.TempDir(), "contexts"))

		// And a mock context is set up
		mockContext := context.NewMockContext()
		mockContext.GetConfigRootFunc = func() (string, error) {
			return contextPath, nil
		}

		// And a DI container with the mock context is created
		diContainer := di.NewContainer()
		diContainer.Register("contextInstance", mockContext)

		// When creating KubeHelper
		kubeHelper, err := NewKubeHelper(diContainer)
		if err != nil {
			t.Fatalf("NewKubeHelper() error = %v", err)
		}

		// And calling GetEnvVars
		envVars, err := kubeHelper.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars() error = %v", err)
		}

		// Then the environment variables should be set correctly
		expectedEnvVars := map[string]string{
			"KUBECONFIG":       kubeConfigPath,
			"KUBE_CONFIG_PATH": kubeConfigPath,
		}
		if !reflect.DeepEqual(envVars, expectedEnvVars) {
			t.Errorf("expected %v, got %v", expectedEnvVars, envVars)
		}
	})

	t.Run("FileNotExist", func(t *testing.T) {
		// Given a non-existent context path
		contextPath := filepath.Join(os.TempDir(), "contexts", "non-existent-context")
		kubeConfigPath := ""

		// And a mock context is set up
		mockContext := context.NewMockContext()
		mockContext.GetConfigRootFunc = func() (string, error) {
			return contextPath, nil
		}

		// And a DI container with the mock context is created
		diContainer := di.NewContainer()
		diContainer.Register("contextInstance", mockContext)

		// When creating KubeHelper
		kubeHelper, err := NewKubeHelper(diContainer)
		if err != nil {
			t.Fatalf("NewKubeHelper() error = %v", err)
		}

		// And calling GetEnvVars
		envVars, err := kubeHelper.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars() error = %v", err)
		}

		// Then the environment variables should be set correctly with an empty KUBECONFIG
		expectedEnvVars := map[string]string{
			"KUBECONFIG":       kubeConfigPath,
			"KUBE_CONFIG_PATH": kubeConfigPath,
		}
		if !reflect.DeepEqual(envVars, expectedEnvVars) {
			t.Errorf("expected %v, got %v", expectedEnvVars, envVars)
		}
	})

	t.Run("ErrorRetrievingProjectRoot", func(t *testing.T) {
		// Given a mock context that returns an error for config root
		mockContext := context.NewMockContext()
		mockContext.GetConfigRootFunc = func() (string, error) {
			return "", errors.New("error retrieving config root")
		}

		// And a DI container with the mock context is created
		diContainer := di.NewContainer()
		diContainer.Register("contextInstance", mockContext)

		// When creating KubeHelper
		kubeHelper, err := NewKubeHelper(diContainer)
		if err != nil {
			t.Fatalf("NewKubeHelper() error = %v", err)
		}

		// And calling GetEnvVars
		expectedError := "error retrieving config root"

		_, err = kubeHelper.GetEnvVars()

		// Then it should return an error indicating config root retrieval failure
		if err == nil || !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("expected error containing %v, got %v", expectedError, err)
		}
	})
}

func TestKubeHelper_PostEnvExec(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a KubeHelper instance
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) { return "", nil }
		mockContext.GetConfigRootFunc = func() (string, error) { return "", nil }

		// And a DI container with the mock context is created
		diContainer := di.NewContainer()
		diContainer.Register("contextInstance", mockContext)

		// When creating KubeHelper
		kubeHelper, err := NewKubeHelper(diContainer)
		if err != nil {
			t.Fatalf("NewKubeHelper() error = %v", err)
		}

		// And calling PostEnvExec
		err = kubeHelper.PostEnvExec()

		// Then no error should be returned
		assertError(t, err, false)
	})
}

func TestKubeHelper_GetContainerConfig(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock context
		mockContext := context.NewMockContext()
		container := di.NewContainer()
		container.Register("contextInstance", mockContext)

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
		diContainer.Register("contextInstance", mockContext)

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
		diContainer.Register("contextInstance", mockContext)

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
		diContainer.Register("contextInstance", mockContext)
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
