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
)

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

		// Create DockerHelper
		dockerHelper := NewDockerHelper(nil, nil, mockContext)

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

		// Create DockerHelper
		dockerHelper := NewDockerHelper(nil, nil, mockContext)

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

		// Create DockerHelper
		dockerHelper := NewDockerHelper(nil, nil, mockContext)

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

		// Create DockerHelper
		dockerHelper := NewDockerHelper(nil, nil, mockContext)

		// When calling GetEnvVars
		expectedError := "error retrieving config root"

		_, err := dockerHelper.GetEnvVars()
		if err == nil || !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("expected error containing %v, got %v", expectedError, err)
		}
	})
}

func TestDockerHelper_PostEnvExec(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a DockerHelper instance
		mockConfigHandler := createMockConfigHandler(
			func(key string) (string, error) { return "", nil },
			func(key string) (map[string]interface{}, error) { return nil, nil },
		)
		mockShell := createMockShell(func() (string, error) { return "", nil })
		mockContext := &context.MockContext{
			GetContextFunc:    func() (string, error) { return "", nil },
			GetConfigRootFunc: func() (string, error) { return "", nil },
		}
		dockerHelper := NewDockerHelper(mockConfigHandler, mockShell, mockContext)

		// When calling PostEnvExec
		err := dockerHelper.PostEnvExec()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestDockerHelper_SetConfig(t *testing.T) {
	mockConfigHandler := &config.MockConfigHandler{}
	mockContext := &context.MockContext{}
	helper := NewDockerHelper(mockConfigHandler, nil, mockContext)

	t.Run("SetConfigStub", func(t *testing.T) {
		// When: SetConfig is called
		err := helper.SetConfig("some_key", "some_value")

		// Then: it should return no error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}