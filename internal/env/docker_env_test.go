package env

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
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
		return filepath.FromSlash("/mock/config/root"), nil
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
			if name == filepath.FromSlash("/mock/config/root/compose.yaml") || name == filepath.FromSlash("/mock/config/root/compose.yml") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		dockerEnv := NewDockerEnv(mocks.Container)

		envVars, err := dockerEnv.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		if envVars["COMPOSE_FILE"] != filepath.FromSlash("/mock/config/root/compose.yaml") && envVars["COMPOSE_FILE"] != filepath.FromSlash("/mock/config/root/compose.yml") {
			t.Errorf("COMPOSE_FILE = %v, want %v or %v", envVars["COMPOSE_FILE"], filepath.FromSlash("/mock/config/root/compose.yaml"), filepath.FromSlash("/mock/config/root/compose.yml"))
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
			if name == filepath.FromSlash("/mock/config/root/compose.yml") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		dockerEnv := NewDockerEnv(mocks.Container)

		envVars, err := dockerEnv.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		if envVars["COMPOSE_FILE"] != filepath.FromSlash("/mock/config/root/compose.yml") {
			t.Errorf("COMPOSE_FILE = %v, want %v", envVars["COMPOSE_FILE"], filepath.FromSlash("/mock/config/root/compose.yml"))
		}
	})
}

func TestDockerEnv_Print(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Use setupSafeAwsEnvMocks to create mocks
		mocks := setupSafeDockerEnvMocks()
		mockContainer := mocks.Container
		dockerEnv := NewDockerEnv(mockContainer)

		// Mock the stat function to simulate the existence of the Docker compose file
		stat = func(name string) (os.FileInfo, error) {
			if name == filepath.FromSlash("/mock/config/root/compose.yaml") || name == filepath.FromSlash("/mock/config/root/compose.yml") {
				return nil, nil // Simulate that the file exists
			}
			return nil, os.ErrNotExist
		}

		// Mock the PrintEnvVarsFunc to verify it is called with the correct envVars
		var capturedEnvVars map[string]string
		mocks.Shell.PrintEnvVarsFunc = func(envVars map[string]string) error {
			capturedEnvVars = envVars
			return nil
		}

		// Call Print and check for errors
		err := dockerEnv.Print()
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		// Verify that PrintEnvVarsFunc was called with the correct envVars
		expectedEnvVars := map[string]string{
			"COMPOSE_FILE": filepath.FromSlash("/mock/config/root/compose.yaml"),
		}
		if !reflect.DeepEqual(capturedEnvVars, expectedEnvVars) {
			t.Errorf("capturedEnvVars = %v, want %v", capturedEnvVars, expectedEnvVars)
		}
	})

	t.Run("GetConfigError", func(t *testing.T) {
		// Use setupSafeAwsEnvMocks to create mocks
		mocks := setupSafeDockerEnvMocks()

		// Override the GetConfigFunc to simulate an error
		mocks.ContextHandler.GetConfigRootFunc = func() (string, error) {
			return "", errors.New("mock config error")
		}

		mockContainer := mocks.Container

		dockerEnv := NewDockerEnv(mockContainer)

		// Call Print and check for errors
		err := dockerEnv.Print()
		if err == nil {
			t.Error("expected error, got nil")
		} else if !strings.Contains(err.Error(), "mock config error") {
			t.Errorf("unexpected error message: %v", err)
		}
	})
}
