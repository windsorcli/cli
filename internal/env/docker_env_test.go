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

type DockerEnvMocks struct {
	Container      di.ContainerInterface
	ContextHandler *context.MockContext
	Shell          *shell.MockShell
}

func setupSafeDockerEnvMocks(container ...di.ContainerInterface) *DockerEnvMocks {
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

	return &DockerEnvMocks{
		Container:      mockContainer,
		ContextHandler: mockContext,
		Shell:          mockShell,
	}
}

func TestDockerEnv_GetEnvVars(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupSafeDockerEnvMocks()

		originalStat := stat
		defer func() { stat = originalStat }()
		stat = func(name string) (os.FileInfo, error) {
			if name == "/mock/config/root/compose.yaml" || name == "/mock/config/root/compose.yml" {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		dockerEnv := NewDockerEnv(mocks.Container)

		envVars, err := dockerEnv.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		if envVars["COMPOSE_FILE"] != "/mock/config/root/compose.yaml" && envVars["COMPOSE_FILE"] != "/mock/config/root/compose.yml" {
			t.Errorf("COMPOSE_FILE = %v, want %v or %v", envVars["COMPOSE_FILE"], "/mock/config/root/compose.yaml", "/mock/config/root/compose.yml")
		}
	})

	t.Run("NoComposeFile", func(t *testing.T) {
		mocks := setupSafeDockerEnvMocks()

		originalStat := stat
		defer func() { stat = originalStat }()
		stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		dockerEnv := NewDockerEnv(mocks.Container)

		envVars, err := dockerEnv.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		if envVars["COMPOSE_FILE"] != "" {
			t.Errorf("COMPOSE_FILE = %v, want empty", envVars["COMPOSE_FILE"])
		}
	})

	t.Run("ResolveContextHandlerError", func(t *testing.T) {
		mockContainer := di.NewMockContainer()
		setupSafeDockerEnvMocks(mockContainer)
		mockContainer.SetResolveError("contextHandler", fmt.Errorf("mock resolve error"))

		dockerEnv := NewDockerEnv(mockContainer)

		_, err := dockerEnv.GetEnvVars()
		expectedError := "error resolving contextHandler: mock resolve error"
		if err == nil || err.Error() != expectedError {
			t.Errorf("error = %v, want %v", err, expectedError)
		}
	})

	t.Run("AssertContextHandlerError", func(t *testing.T) {
		container := di.NewContainer()
		setupSafeDockerEnvMocks(container)
		container.Register("contextHandler", "invalidType")

		dockerEnv := NewDockerEnv(container)

		_, err := dockerEnv.GetEnvVars()
		expectedError := "failed to cast contextHandler to context.ContextInterface"
		if err == nil || err.Error() != expectedError {
			t.Errorf("error = %v, want %v", err, expectedError)
		}
	})

	t.Run("GetConfigRootError", func(t *testing.T) {
		mocks := setupSafeDockerEnvMocks()
		mocks.ContextHandler.GetConfigRootFunc = func() (string, error) {
			return "", errors.New("mock context error")
		}

		dockerEnv := NewDockerEnv(mocks.Container)

		_, err := dockerEnv.GetEnvVars()
		expectedError := "error retrieving configuration root directory: mock context error"
		if err == nil || err.Error() != expectedError {
			t.Errorf("error = %v, want %v", err, expectedError)
		}
	})

	t.Run("YmlFileExists", func(t *testing.T) {
		mocks := setupSafeDockerEnvMocks()

		originalStat := stat
		defer func() { stat = originalStat }()
		stat = func(name string) (os.FileInfo, error) {
			if name == "/mock/config/root/compose.yml" {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		dockerEnv := NewDockerEnv(mocks.Container)

		envVars, err := dockerEnv.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		if envVars["COMPOSE_FILE"] != "/mock/config/root/compose.yml" {
			t.Errorf("COMPOSE_FILE = %v, want %v", envVars["COMPOSE_FILE"], "/mock/config/root/compose.yml")
		}
	})
}
