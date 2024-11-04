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

func TestDockerEnv_Print(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupSafeDockerEnvMocks()

		mocks.Shell.PrintEnvVarsFunc = func(envVars map[string]string) error {
			t.Log("PrintEnvVarsFunc called successfully with envVars:", envVars)
			return nil
		}

		originalStat := stat
		defer func() { stat = originalStat }()
		stat = func(name string) (os.FileInfo, error) {
			if name == "/mock/config/root/compose.yaml" || name == "/mock/config/root/compose.yml" {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		dockerEnv := NewDockerEnv(mocks.Container)

		envVars := make(map[string]string)
		dockerEnv.Print(envVars)

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

		envVars := make(map[string]string)
		dockerEnv.Print(envVars)

		if envVars["COMPOSE_FILE"] != "" {
			t.Errorf("COMPOSE_FILE = %v, want empty", envVars["COMPOSE_FILE"])
		}
	})

	t.Run("ResolveContextHandlerError", func(t *testing.T) {
		mockContainer := di.NewMockContainer()
		setupSafeDockerEnvMocks(mockContainer)
		mockContainer.SetResolveError("contextHandler", fmt.Errorf("mock resolve error"))

		dockerEnv := NewDockerEnv(mockContainer)

		output := captureStdout(t, func() {
			envVars := make(map[string]string)
			dockerEnv.Print(envVars)
		})

		expectedOutput := "Error resolving contextHandler: mock resolve error\n"
		if output != expectedOutput {
			t.Errorf("output = %v, want %v", output, expectedOutput)
		}
	})

	t.Run("AssertContextHandlerError", func(t *testing.T) {
		container := di.NewContainer()
		setupSafeDockerEnvMocks(container)
		container.Register("contextHandler", "invalidType")

		dockerEnv := NewDockerEnv(container)

		output := captureStdout(t, func() {
			envVars := make(map[string]string)
			dockerEnv.Print(envVars)
		})

		expectedOutput := "Failed to cast contextHandler to context.ContextInterface\n"
		if output != expectedOutput {
			t.Errorf("output = %v, want %v", output, expectedOutput)
		}
	})

	t.Run("ResolveShellError", func(t *testing.T) {
		mockContainer := di.NewMockContainer()
		setupSafeDockerEnvMocks(mockContainer)
		mockContainer.SetResolveError("shell", fmt.Errorf("mock resolve error"))

		dockerEnv := NewDockerEnv(mockContainer)

		output := captureStdout(t, func() {
			envVars := make(map[string]string)
			dockerEnv.Print(envVars)
		})

		expectedOutput := "Error resolving shell: mock resolve error\n"
		if output != expectedOutput {
			t.Errorf("output = %v, want %v", output, expectedOutput)
		}
	})

	t.Run("AssertShellError", func(t *testing.T) {
		mockContainer := di.NewMockContainer()
		setupSafeDockerEnvMocks(mockContainer)
		mockContainer.Register("shell", "invalidType")

		dockerEnv := NewDockerEnv(mockContainer)

		output := captureStdout(t, func() {
			envVars := make(map[string]string)
			dockerEnv.Print(envVars)
		})

		expectedOutput := "Failed to cast shell to shell.Shell\n"
		if output != expectedOutput {
			t.Errorf("output = %v, want %v", output, expectedOutput)
		}
	})

	t.Run("GetConfigRootError", func(t *testing.T) {
		mocks := setupSafeDockerEnvMocks()
		mocks.ContextHandler.GetConfigRootFunc = func() (string, error) {
			return "", errors.New("mock context error")
		}

		dockerEnv := NewDockerEnv(mocks.Container)

		output := captureStdout(t, func() {
			envVars := make(map[string]string)
			dockerEnv.Print(envVars)
		})

		expectedOutput := "Error retrieving configuration root directory: mock context error\n"
		if output != expectedOutput {
			t.Errorf("output = %v, want %v", output, expectedOutput)
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

		envVars := make(map[string]string)
		dockerEnv.Print(envVars)

		if envVars["COMPOSE_FILE"] != "/mock/config/root/compose.yml" {
			t.Errorf("COMPOSE_FILE = %v, want %v", envVars["COMPOSE_FILE"], "/mock/config/root/compose.yml")
		}
	})
}
