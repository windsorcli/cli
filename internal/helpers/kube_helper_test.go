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

func TestKubeHelper(t *testing.T) {
	t.Run("NewKubeHelper", func(t *testing.T) {
		t.Run("ErrorResolvingContext", func(t *testing.T) {
			// Given a DI container without registering context
			diContainer := di.NewContainer()
			mockConfigHandler := config.NewMockConfigHandler(nil, nil, nil, nil, nil, nil)
			diContainer.Register("cliConfigHandler", mockConfigHandler)

			// When attempting to create KubeHelper
			_, err := NewKubeHelper(diContainer)

			// Then it should return an error indicating context resolution failure
			if err == nil || !strings.Contains(err.Error(), "error resolving context") {
				t.Fatalf("expected error resolving context, got %v", err)
			}
		})
	})

	t.Run("GetEnvVars", func(t *testing.T) {
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
			mockContext := context.NewMockContext(nil, nil, nil)
			mockContext.GetConfigRootFunc = func() (string, error) {
				return contextPath, nil
			}

			// And a DI container with the mock context is created
			diContainer := di.NewContainer()
			diContainer.Register("context", mockContext)

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
			mockContext := context.NewMockContext(nil, nil, nil)
			mockContext.GetConfigRootFunc = func() (string, error) {
				return contextPath, nil
			}

			// And a DI container with the mock context is created
			diContainer := di.NewContainer()
			diContainer.Register("context", mockContext)

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
			mockContext := context.NewMockContext(nil, nil, nil)
			mockContext.GetConfigRootFunc = func() (string, error) {
				return "", errors.New("error retrieving config root")
			}

			// And a DI container with the mock context is created
			diContainer := di.NewContainer()
			diContainer.Register("context", mockContext)

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
	})

	t.Run("PostEnvExec", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			// Given a KubeHelper instance
			mockContext := context.NewMockContext(nil, nil, nil)
			mockContext.GetContextFunc = func() (string, error) { return "", nil }
			mockContext.GetConfigRootFunc = func() (string, error) { return "", nil }

			// And a DI container with the mock context is created
			diContainer := di.NewContainer()
			diContainer.Register("context", mockContext)

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
	})

	t.Run("SetConfig", func(t *testing.T) {
		t.Run("SetConfigStub", func(t *testing.T) {
			// Given a KubeHelper instance
			mockContext := context.NewMockContext(nil, nil, nil)

			// And a DI container with the mock context is created
			diContainer := di.NewContainer()
			diContainer.Register("context", mockContext)

			// When creating KubeHelper
			helper, err := NewKubeHelper(diContainer)
			if err != nil {
				t.Fatalf("NewKubeHelper() error = %v", err)
			}

			// And calling SetConfig
			err = helper.SetConfig("some_key", "some_value")

			// Then it should return no error
			assertError(t, err, false)
		})
	})

	t.Run("GetContainerConfig", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			// Given a mock context
			mockContext := context.NewMockContext(nil, nil, nil)
			container := di.NewContainer()
			container.Register("context", mockContext)

			// When creating KubeHelper
			kubeHelper, err := NewKubeHelper(container)
			if err != nil {
				t.Fatalf("NewKubeHelper() error = %v", err)
			}

			// And calling GetContainerConfig
			containerConfig, err := kubeHelper.GetContainerConfig()
			if err != nil {
				t.Fatalf("GetContainerConfig() error = %v", err)
			}

			// Then the result should be nil as per the stub implementation
			if containerConfig != nil {
				t.Errorf("expected nil, got %v", containerConfig)
			}
		})
	})
}
