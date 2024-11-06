package env

import (
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/shell"
)

type KubeEnvMocks struct {
	Container      di.ContainerInterface
	ContextHandler *context.MockContext
	Shell          *shell.MockShell
}

func setupSafeKubeEnvMocks(container ...di.ContainerInterface) *KubeEnvMocks {
	var mockContainer di.ContainerInterface
	if len(container) > 0 {
		mockContainer = container[0]
	} else {
		mockContainer = di.NewContainer()
	}

	mockContext := context.NewMockContext()
	mockContext.GetConfigRootFunc = func() (string, error) {
		return "/mock/config/root", nil
	}

	mockShell := shell.NewMockShell()

	mockContainer.Register("contextHandler", mockContext)
	mockContainer.Register("shell", mockShell)

	return &KubeEnvMocks{
		Container:      mockContainer,
		ContextHandler: mockContext,
		Shell:          mockShell,
	}
}

func TestKubeEnv_GetEnvVars(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupSafeKubeEnvMocks()

		originalStat := stat
		defer func() { stat = originalStat }()
		stat = func(name string) (os.FileInfo, error) {
			if name == "/mock/config/root/.kube/config" {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		kubeEnv := NewKubeEnv(mocks.Container)

		envVars, err := kubeEnv.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		if envVars["KUBECONFIG"] != "/mock/config/root/.kube/config" || envVars["KUBE_CONFIG_PATH"] != "/mock/config/root/.kube/config" {
			t.Errorf("KUBECONFIG = %v, KUBE_CONFIG_PATH = %v, want both to be %v", envVars["KUBECONFIG"], envVars["KUBE_CONFIG_PATH"], "/mock/config/root/.kube/config")
		}
	})

	t.Run("NoKubeConfig", func(t *testing.T) {
		mocks := setupSafeKubeEnvMocks()

		originalStat := stat
		defer func() { stat = originalStat }()
		stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		kubeEnv := NewKubeEnv(mocks.Container)

		envVars, err := kubeEnv.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		if envVars["KUBECONFIG"] != "" || envVars["KUBE_CONFIG_PATH"] != "" {
			t.Errorf("KUBECONFIG = %v, KUBE_CONFIG_PATH = %v, want both to be empty", envVars["KUBECONFIG"], envVars["KUBE_CONFIG_PATH"])
		}
	})

	t.Run("ResolveContextHandlerError", func(t *testing.T) {
		mockContainer := di.NewMockContainer()
		setupSafeKubeEnvMocks(mockContainer)
		mockContainer.SetResolveError("contextHandler", fmt.Errorf("mock resolve error"))

		kubeEnv := NewKubeEnv(mockContainer)

		_, err := kubeEnv.GetEnvVars()
		expectedError := "error resolving contextHandler: mock resolve error"
		if err == nil || err.Error() != expectedError {
			t.Errorf("error = %v, want %v", err, expectedError)
		}
	})

	t.Run("AssertContextHandlerError", func(t *testing.T) {
		container := di.NewContainer()
		setupSafeKubeEnvMocks(container)
		container.Register("contextHandler", "invalidType")

		kubeEnv := NewKubeEnv(container)

		_, err := kubeEnv.GetEnvVars()
		expectedError := "failed to cast contextHandler to context.ContextInterface"
		if err == nil || err.Error() != expectedError {
			t.Errorf("error = %v, want %v", err, expectedError)
		}
	})

	t.Run("GetConfigRootError", func(t *testing.T) {
		mocks := setupSafeKubeEnvMocks()
		mocks.ContextHandler.GetConfigRootFunc = func() (string, error) {
			return "", errors.New("mock context error")
		}

		kubeEnv := NewKubeEnv(mocks.Container)

		_, err := kubeEnv.GetEnvVars()
		expectedError := "error retrieving configuration root directory: mock context error"
		if err == nil || err.Error() != expectedError {
			t.Errorf("error = %v, want %v", err, expectedError)
		}
	})
}
