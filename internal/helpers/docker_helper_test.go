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

func TestNewDockerHelper(t *testing.T) {
	t.Run("ErrorResolvingConfigHandler", func(t *testing.T) {
		// Create DI container without registering configHandler
		diContainer := di.NewContainer()

		// Attempt to create DockerHelper
		_, err := NewDockerHelper(diContainer)
		if err == nil || !strings.Contains(err.Error(), "error resolving configHandler") {
			t.Fatalf("expected error resolving configHandler, got %v", err)
		}
	})

	t.Run("ErrorResolvingContext", func(t *testing.T) {
		// Create DI container and register only configHandler
		diContainer := di.NewContainer()
		mockConfigHandler := &config.MockConfigHandler{}
		diContainer.Register("configHandler", mockConfigHandler)

		// Attempt to create DockerHelper
		_, err := NewDockerHelper(diContainer)
		if err == nil || !strings.Contains(err.Error(), "error resolving context") {
			t.Fatalf("expected error resolving context, got %v", err)
		}
	})
}

func TestDockerHelper_GetEnvVars(t *testing.T) {
	t.Run("ValidConfigRootWithYaml", func(t *testing.T) {
		// Given: a valid context path with docker-compose.yaml
		contextPath := filepath.Join(os.TempDir(), "contexts", "test-context-yaml")
		composeFilePath := filepath.Join(contextPath, "docker-compose.yaml")

		// Create the directory and docker-compose.yaml file
		err := os.MkdirAll(contextPath, os.ModePerm)
		if err != nil {
			t.Fatalf("Failed to create directories: %v", err)
		}
		_, err = os.Create(composeFilePath)
		if err != nil {
			t.Fatalf("Failed to create docker-compose.yaml file: %v", err)
		}
		defer os.RemoveAll(filepath.Join(os.TempDir(), "contexts", "test-context-yaml"))

		// Mock context
		mockContext := &context.MockContext{
			GetConfigRootFunc: func() (string, error) {
				return contextPath, nil
			},
		}

		// Create DI container and register mocks
		diContainer := di.NewContainer()
		diContainer.Register("configHandler", &config.MockConfigHandler{})
		diContainer.Register("context", mockContext)

		// Register MockHelper
		mockHelper := NewMockHelper(func() (map[string]string, error) {
			return map[string]string{"COMPOSE_FILE": composeFilePath}, nil
		})
		diContainer.Register("helper", mockHelper)

		// Create DockerHelper
		dockerHelper, err := NewDockerHelper(diContainer)
		if err != nil {
			t.Fatalf("NewDockerHelper() error = %v", err)
		}

		// When: GetEnvVars is called
		envVars, err := dockerHelper.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars() error = %v", err)
		}

		// Then: the environment variables should be set correctly
		expectedEnvVars := map[string]string{
			"COMPOSE_FILE": composeFilePath,
		}
		if !reflect.DeepEqual(envVars, expectedEnvVars) {
			t.Errorf("expected %v, got %v", expectedEnvVars, envVars)
		}
	})

	t.Run("ValidConfigRootWithYml", func(t *testing.T) {
		// Given: a valid context path with docker-compose.yml
		contextPath := filepath.Join(os.TempDir(), "contexts", "test-context-yml")
		composeFilePath := filepath.Join(contextPath, "docker-compose.yml")

		// Create the directory and docker-compose.yml file
		err := os.MkdirAll(contextPath, os.ModePerm)
		if err != nil {
			t.Fatalf("Failed to create directories: %v", err)
		}
		_, err = os.Create(composeFilePath)
		if err != nil {
			t.Fatalf("Failed to create docker-compose.yml file: %v", err)
		}
		defer os.RemoveAll(filepath.Join(os.TempDir(), "contexts", "test-context-yml"))

		// Mock context
		mockContext := &context.MockContext{
			GetConfigRootFunc: func() (string, error) {
				return contextPath, nil
			},
		}

		// Create DI container and register mocks
		diContainer := di.NewContainer()
		diContainer.Register("configHandler", &config.MockConfigHandler{})
		diContainer.Register("context", mockContext)

		// Register MockHelper
		mockHelper := NewMockHelper(func() (map[string]string, error) {
			return map[string]string{"COMPOSE_FILE": composeFilePath}, nil
		})
		diContainer.Register("helper", mockHelper)

		// Create DockerHelper
		dockerHelper, err := NewDockerHelper(diContainer)
		if err != nil {
			t.Fatalf("NewDockerHelper() error = %v", err)
		}

		// When: GetEnvVars is called
		envVars, err := dockerHelper.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars() error = %v", err)
		}

		// Then: the environment variables should be set correctly
		expectedEnvVars := map[string]string{
			"COMPOSE_FILE": composeFilePath,
		}
		if !reflect.DeepEqual(envVars, expectedEnvVars) {
			t.Errorf("expected %v, got %v", expectedEnvVars, envVars)
		}
	})

	t.Run("FileNotExist", func(t *testing.T) {
		// Given: a non-existent context path
		contextPath := filepath.Join(os.TempDir(), "contexts", "non-existent-context")
		composeFilePath := ""

		// Mock context
		mockContext := &context.MockContext{
			GetConfigRootFunc: func() (string, error) {
				return contextPath, nil
			},
		}

		// Create DI container and register mocks
		diContainer := di.NewContainer()
		diContainer.Register("configHandler", &config.MockConfigHandler{})
		diContainer.Register("context", mockContext)

		// Register MockHelper
		mockHelper := NewMockHelper(func() (map[string]string, error) {
			return map[string]string{
				"service1": "nginx:latest",
			}, nil
		})
		diContainer.Register("helper", mockHelper)

		// Create DockerHelper
		dockerHelper, err := NewDockerHelper(diContainer)
		if err != nil {
			t.Fatalf("NewDockerHelper() error = %v", err)
		}

		// When: GetEnvVars is called
		envVars, err := dockerHelper.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars() error = %v", err)
		}

		// Then: the environment variables should be set correctly with an empty COMPOSE_FILE
		expectedEnvVars := map[string]string{
			"COMPOSE_FILE": composeFilePath,
		}
		if !reflect.DeepEqual(envVars, expectedEnvVars) {
			t.Errorf("expected %v, got %v", expectedEnvVars, envVars)
		}
	})

	t.Run("ErrorRetrievingConfigRoot", func(t *testing.T) {
		// Given a mock context that returns an error for config root
		mockContext := &context.MockContext{
			GetConfigRootFunc: func() (string, error) {
				return "", errors.New("error retrieving config root")
			},
		}

		// Create DI container and register mocks
		diContainer := di.NewContainer()
		diContainer.Register("configHandler", &config.MockConfigHandler{})
		diContainer.Register("context", mockContext)

		// Register MockHelper
		mockHelper := NewMockHelper(func() (map[string]string, error) {
			return map[string]string{
				"service1": "nginx:latest",
			}, nil
		})
		diContainer.Register("helper", mockHelper)

		// Create DockerHelper
		dockerHelper, err := NewDockerHelper(diContainer)
		if err != nil {
			t.Fatalf("NewDockerHelper() error = %v", err)
		}

		// When calling GetEnvVars
		expectedError := "error retrieving config root"

		_, err = dockerHelper.GetEnvVars()
		if err == nil || !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("expected error containing %v, got %v", expectedError, err)
		}
	})
}

func TestDockerHelper_PostEnvExec(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a DockerHelper instance
		mockConfigHandler := &config.MockConfigHandler{}
		mockContext := &context.MockContext{
			GetContextFunc: func() (string, error) {
				return "test-context", nil
			},
		}

		// Create DI container and register mocks
		diContainer := di.NewContainer()
		diContainer.Register("configHandler", mockConfigHandler)
		diContainer.Register("context", mockContext)

		// Register MockHelper
		mockHelper := NewMockHelper(func() (map[string]string, error) {
			return map[string]string{
				"service1": "nginx:latest",
			}, nil
		})
		diContainer.Register("helper", mockHelper)

		dockerHelper, err := NewDockerHelper(diContainer)
		if err != nil {
			t.Fatalf("NewDockerHelper() error = %v", err)
		}

		// When calling PostEnvExec
		err = dockerHelper.PostEnvExec()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestDockerHelper_SetConfig(t *testing.T) {
	t.Run("SetEnabledConfigSuccess", func(t *testing.T) {
		// Given: a new DockerHelper instance for this test
		mockConfigHandler := &config.MockConfigHandler{}
		mockContext := &context.MockContext{
			GetContextFunc: func() (string, error) {
				return "test-context", nil
			},
		}
		diContainer := di.NewContainer()
		diContainer.Register("configHandler", mockConfigHandler)
		diContainer.Register("context", mockContext)

		// Register MockHelper
		mockHelper := NewMockHelper(func() (map[string]string, error) {
			return map[string]string{
				"service1": "nginx:latest",
			}, nil
		})
		diContainer.Register("helper", mockHelper)

		helper, err := NewDockerHelper(diContainer)
		if err != nil {
			t.Fatalf("NewDockerHelper() error = %v", err)
		}

		// When: SetConfig is called with "enabled" key
		err = helper.SetConfig("enabled", "true")

		// Then: it should return no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("SetEnabledConfigError", func(t *testing.T) {
		// Given: a mock context that returns an error
		mockContextWithError := &context.MockContext{
			GetContextFunc: func() (string, error) {
				return "", errors.New("error retrieving current context")
			},
		}
		mockConfigHandler := &config.MockConfigHandler{}
		diContainer := di.NewContainer()
		diContainer.Register("context", mockContextWithError)
		diContainer.Register("configHandler", mockConfigHandler)

		// Register MockHelper
		mockHelper := NewMockHelper(func() (map[string]string, error) {
			return map[string]string{
				"service1": "nginx:latest",
			}, nil
		})
		diContainer.Register("helper", mockHelper)

		// Create DockerHelper
		helper, err := NewDockerHelper(diContainer)
		if err != nil {
			t.Fatalf("NewDockerHelper() error = %v", err)
		}

		// When: SetConfig is called with "enabled" key
		err = helper.SetConfig("enabled", "true")

		// Then: it should return an error
		if err == nil || !strings.Contains(err.Error(), "error retrieving current context") {
			t.Fatalf("expected error containing 'error retrieving current context', got %v", err)
		}
	})

	t.Run("UnsupportedConfigKey", func(t *testing.T) {
		// Given: a new DockerHelper instance for this test
		mockConfigHandler := &config.MockConfigHandler{}
		mockContext := &context.MockContext{}
		diContainer := di.NewContainer()
		diContainer.Register("configHandler", mockConfigHandler)
		diContainer.Register("context", mockContext)

		// Register MockHelper
		mockHelper := NewMockHelper(func() (map[string]string, error) {
			return map[string]string{
				"service1": "nginx:latest",
			}, nil
		})
		diContainer.Register("helper", mockHelper)

		helper, err := NewDockerHelper(diContainer)
		if err != nil {
			t.Fatalf("NewDockerHelper() error = %v", err)
		}

		// When: SetConfig is called with an unsupported key
		err = helper.SetConfig("unsupported_key", "some_value")

		// Then: it should return an error
		expectedError := "unsupported config key: unsupported_key"
		if err == nil || err.Error() != expectedError {
			t.Fatalf("expected error %v, got %v", expectedError, err)
		}
	})

	t.Run("ErrorSettingDockerEnabled", func(t *testing.T) {
		// Given: a mock config handler that returns an error when setting the config value
		mockConfigHandlerWithError := &config.MockConfigHandler{
			SetConfigValueFunc: func(key, value string) error {
				return errors.New("mock error setting config value")
			},
		}
		mockContext := &context.MockContext{
			GetContextFunc: func() (string, error) {
				return "local", nil
			},
		}
		diContainer := di.NewContainer()
		diContainer.Register("configHandler", mockConfigHandlerWithError)
		diContainer.Register("context", mockContext)

		// Register MockHelper
		mockHelper := NewMockHelper(func() (map[string]string, error) {
			return map[string]string{
				"service1": "nginx:latest",
			}, nil
		})
		diContainer.Register("helper", mockHelper)

		// Create DockerHelper
		helper, err := NewDockerHelper(diContainer)
		if err != nil {
			t.Fatalf("NewDockerHelper() error = %v", err)
		}

		// When: SetConfig is called with "enabled" key
		err = helper.SetConfig("enabled", "true")

		// Then: it should return an error indicating the failure to set the config
		expectedError := "error setting docker.enabled: mock error setting config value"
		if err == nil || err.Error() != expectedError {
			t.Fatalf("expected error %v, got %v", expectedError, err)
		}
	})
}

func TestDockerHelper_GetContainerConfig(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given: a mock config handler, shell, context, and helper
		mockConfigHandler := &config.MockConfigHandler{}
		mockContext := &context.MockContext{}
		mockHelper := NewMockHelper(func() (map[string]string, error) {
			return map[string]string{
				"service1": "nginx:latest",
			}, nil
		})

		// Create DI container and register mocks
		diContainer := di.NewContainer()
		diContainer.Register("configHandler", mockConfigHandler)
		diContainer.Register("context", mockContext)
		diContainer.Register("helper", mockHelper)

		helper, err := NewDockerHelper(diContainer)
		if err != nil {
			t.Fatalf("NewDockerHelper() error = %v", err)
		}

		// When: GetContainerConfig is called
		containerConfig, err := helper.GetContainerConfig()
		if err != nil {
			t.Fatalf("GetContainerConfig() error = %v", err)
		}

		// Then: the result should be nil
		if containerConfig != nil {
			t.Errorf("expected nil, got %v", containerConfig)
		}
	})
}
