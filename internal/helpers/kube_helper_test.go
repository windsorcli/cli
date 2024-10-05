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
)

func TestKubeHelper_GetEnvVars(t *testing.T) {
	t.Run("ValidConfigRoot", func(t *testing.T) {
		// Given: a valid context path
		contextPath := filepath.Join(os.TempDir(), "contexts", "test-context")
		kubeConfigPath := filepath.Join(contextPath, ".kube", "config")

		// Create the directory and kubeconfig file
		err := mkdirAll(filepath.Join(contextPath, ".kube"), os.ModePerm)
		if err != nil {
			t.Fatalf("Failed to create directories: %v", err)
		}
		_, err = os.Create(kubeConfigPath)
		if err != nil {
			t.Fatalf("Failed to create kubeconfig file: %v", err)
		}
		defer os.RemoveAll(filepath.Join(os.TempDir(), "contexts"))

		// Mock context
		mockContext := &context.MockContext{
			GetConfigRootFunc: func() (string, error) {
				return contextPath, nil
			},
		}

		// Create DI container and register mock context
		diContainer := di.NewContainer()
		diContainer.Register("context", mockContext)

		// Create KubeHelper
		kubeHelper, err := NewKubeHelper(diContainer)
		if err != nil {
			t.Fatalf("NewKubeHelper() error = %v", err)
		}

		// When: GetEnvVars is called
		envVars, err := kubeHelper.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars() error = %v", err)
		}

		// Then: the environment variables should be set correctly
		expectedEnvVars := map[string]string{
			"KUBECONFIG":       kubeConfigPath,
			"KUBE_CONFIG_PATH": kubeConfigPath,
		}
		if !reflect.DeepEqual(envVars, expectedEnvVars) {
			t.Errorf("expected %v, got %v", expectedEnvVars, envVars)
		}
	})

	t.Run("FileNotExist", func(t *testing.T) {
		// Given: a non-existent context path
		contextPath := filepath.Join(os.TempDir(), "contexts", "non-existent-context")
		kubeConfigPath := ""

		// Mock context
		mockContext := &context.MockContext{
			GetConfigRootFunc: func() (string, error) {
				return contextPath, nil
			},
		}

		// Create DI container and register mock context
		diContainer := di.NewContainer()
		diContainer.Register("context", mockContext)

		// Create KubeHelper
		kubeHelper, err := NewKubeHelper(diContainer)
		if err != nil {
			t.Fatalf("NewKubeHelper() error = %v", err)
		}

		// When: GetEnvVars is called
		envVars, err := kubeHelper.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars() error = %v", err)
		}

		// Then: the environment variables should be set correctly with an empty KUBECONFIG
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
		mockContext := &context.MockContext{
			GetConfigRootFunc: func() (string, error) {
				return "", errors.New("error retrieving config root")
			},
		}

		// Create DI container and register mock context
		diContainer := di.NewContainer()
		diContainer.Register("context", mockContext)

		// Create KubeHelper
		kubeHelper, err := NewKubeHelper(diContainer)
		if err != nil {
			t.Fatalf("NewKubeHelper() error = %v", err)
		}

		// When calling GetEnvVars
		expectedError := "error retrieving config root"

		_, err = kubeHelper.GetEnvVars()
		if err == nil || !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("expected error containing %v, got %v", expectedError, err)
		}
	})
}

func TestKubeHelper_PostEnvExec(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a KubeHelper instance
		mockContext := &context.MockContext{
			GetContextFunc:    func() (string, error) { return "", nil },
			GetConfigRootFunc: func() (string, error) { return "", nil },
		}

		// Create DI container and register mock context
		diContainer := di.NewContainer()
		diContainer.Register("context", mockContext)

		// Create KubeHelper
		kubeHelper, err := NewKubeHelper(diContainer)
		if err != nil {
			t.Fatalf("NewKubeHelper() error = %v", err)
		}

		// When calling PostEnvExec
		err = kubeHelper.PostEnvExec()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestKubeHelper_SetConfig(t *testing.T) {
	mockContext := &context.MockContext{}

	// Create DI container and register mock context
	diContainer := di.NewContainer()
	diContainer.Register("context", mockContext)

	// Create KubeHelper
	helper, err := NewKubeHelper(diContainer)
	if err != nil {
		t.Fatalf("NewKubeHelper() error = %v", err)
	}

	t.Run("SetConfigStub", func(t *testing.T) {
		// When: SetConfig is called
		err := helper.SetConfig("some_key", "some_value")

		// Then: it should return no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestNewKubeHelper(t *testing.T) {
	t.Run("ErrorResolvingContext", func(t *testing.T) {
		// Create DI container without registering context
		diContainer := di.NewContainer()
		mockConfigHandler := &config.MockConfigHandler{}
		diContainer.Register("configHandler", mockConfigHandler)

		// Attempt to create KubeHelper
		_, err := NewKubeHelper(diContainer)
		if err == nil || !strings.Contains(err.Error(), "error resolving context") {
			t.Fatalf("expected error resolving context, got %v", err)
		}
	})
}

func TestKubeHelper_GetContainerConfig(t *testing.T) {
	// Given a mock context
	mockContext := &context.MockContext{}
	container := di.NewContainer()
	container.Register("context", mockContext)

	// Create KubeHelper
	kubeHelper, err := NewKubeHelper(container)
	if err != nil {
		t.Fatalf("NewKubeHelper() error = %v", err)
	}

	t.Run("Success", func(t *testing.T) {
		// When: GetContainerConfig is called
		containerConfig, err := kubeHelper.GetContainerConfig()
		if err != nil {
			t.Fatalf("GetContainerConfig() error = %v", err)
		}

		// Then: the result should be nil as per the stub implementation
		if containerConfig != nil {
			t.Errorf("expected nil, got %v", containerConfig)
		}
	})
}
