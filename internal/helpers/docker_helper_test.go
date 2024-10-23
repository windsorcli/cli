package helpers

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/compose-spec/compose-go/types"
	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
)

// Mock function for yamlMarshal to simulate an error
var originalYamlMarshal = yamlMarshal

func TestDockerHelper(t *testing.T) {
	t.Run("NewDockerHelper", func(t *testing.T) {
		t.Run("ErrorResolvingConfigHandler", func(t *testing.T) {
			// Create DI container without registering cliConfigHandler
			diContainer := di.NewContainer()

			// Attempt to create DockerHelper
			_, err := NewDockerHelper(diContainer)
			if err == nil || !strings.Contains(err.Error(), "error resolving cliConfigHandler") {
				t.Fatalf("expected error resolving cliConfigHandler, got %v", err)
			}
		})

		t.Run("ErrorResolvingContext", func(t *testing.T) {
			// Create DI container and register only cliConfigHandler
			diContainer := di.NewContainer()
			mockConfigHandler := config.NewMockConfigHandler()
			diContainer.Register("cliConfigHandler", mockConfigHandler)

			// Attempt to create DockerHelper
			_, err := NewDockerHelper(diContainer)
			if err == nil || !strings.Contains(err.Error(), "error resolving context") {
				t.Fatalf("expected error resolving context, got %v", err)
			}
		})

		t.Run("ErrorResolvingHelpers", func(t *testing.T) {
			// Create DI container and register only cliConfigHandler and context
			diContainer := di.NewContainer()
			mockConfigHandler := config.NewMockConfigHandler()
			mockContext := context.NewMockContext()
			mockContext.GetContextFunc = func() (string, error) {
				return "test-context", nil
			}
			diContainer.Register("cliConfigHandler", mockConfigHandler)
			diContainer.Register("context", mockContext)

			// Attempt to create DockerHelper without registering any helpers
			_, err := NewDockerHelper(diContainer)
			if err == nil || !strings.Contains(err.Error(), "error resolving helpers") {
				t.Fatalf("expected error resolving helpers, got %v", err)
			}
		})

		t.Run("GetEnvVars", func(t *testing.T) {
			t.Run("ValidConfigRootWithYaml", func(t *testing.T) {
				// Given: a valid context path with compose.yaml
				contextPath := filepath.Join(os.TempDir(), "contexts", "test-context-yaml")
				composeFilePath := filepath.Join(contextPath, "compose.yaml")

				// Create the directory and compose.yaml file
				err := os.MkdirAll(contextPath, os.ModePerm)
				if err != nil {
					t.Fatalf("Failed to create directories: %v", err)
				}
				_, err = os.Create(composeFilePath)
				if err != nil {
					t.Fatalf("Failed to create compose.yaml file: %v", err)
				}
				defer os.RemoveAll(filepath.Join(os.TempDir(), "contexts", "test-context-yaml"))

				// Mock context
				mockContext := context.NewMockContext()
				mockContext.GetConfigRootFunc = func() (string, error) {
					return contextPath, nil
				}

				// Create DI container and register mocks
				diContainer := di.NewContainer()
				diContainer.Register("cliConfigHandler", config.NewMockConfigHandler())
				diContainer.Register("context", mockContext)

				// Register MockHelper
				mockHelper := NewMockHelper()
				mockHelper.GetEnvVarsFunc = func() (map[string]string, error) {
					return map[string]string{"COMPOSE_FILE": composeFilePath}, nil
				}
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
				// Given: a valid context path with compose.yml
				contextPath := filepath.Join(os.TempDir(), "contexts", "test-context-yml")
				composeFilePath := filepath.Join(contextPath, "compose.yml")

				// Create the directory and compose.yml file
				err := os.MkdirAll(contextPath, os.ModePerm)
				if err != nil {
					t.Fatalf("Failed to create directories: %v", err)
				}
				_, err = os.Create(composeFilePath)
				if err != nil {
					t.Fatalf("Failed to create compose.yml file: %v", err)
				}
				defer os.RemoveAll(filepath.Join(os.TempDir(), "contexts", "test-context-yml"))

				// Mock context
				mockContext := context.NewMockContext()
				mockContext.GetConfigRootFunc = func() (string, error) {
					return contextPath, nil
				}

				// Create DI container and register mocks
				diContainer := di.NewContainer()
				diContainer.Register("cliConfigHandler", config.NewMockConfigHandler())
				diContainer.Register("context", mockContext)

				// Register MockHelper
				mockHelper := NewMockHelper()
				mockHelper.GetEnvVarsFunc = func() (map[string]string, error) {
					return map[string]string{"COMPOSE_FILE": composeFilePath}, nil
				}
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
				mockContext := context.NewMockContext()
				mockContext.GetConfigRootFunc = func() (string, error) {
					return contextPath, nil
				}

				// Create DI container and register mocks
				diContainer := di.NewContainer()
				diContainer.Register("cliConfigHandler", config.NewMockConfigHandler())
				diContainer.Register("context", mockContext)

				// Register MockHelper
				mockHelper := NewMockHelper()
				mockHelper.GetEnvVarsFunc = func() (map[string]string, error) {
					return map[string]string{
						"service1": "nginx:latest",
					}, nil
				}
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
				mockContext := context.NewMockContext()
				mockContext.GetConfigRootFunc = func() (string, error) {
					return "", errors.New("error retrieving config root")
				}

				// Create DI container and register mocks
				diContainer := di.NewContainer()
				diContainer.Register("cliConfigHandler", config.NewMockConfigHandler())
				diContainer.Register("context", mockContext)

				// Register MockHelper
				mockHelper := NewMockHelper()
				mockHelper.GetEnvVarsFunc = func() (map[string]string, error) {
					return map[string]string{
						"service1": "nginx:latest",
					}, nil
				}
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
		})
	})

	t.Run("PostEnvExec", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			// Given a DockerHelper instance
			mockConfigHandler := config.NewMockConfigHandler()
			mockContext := context.NewMockContext()
			mockContext.GetContextFunc = func() (string, error) {
				return "test-context", nil
			}

			// Create DI container and register mocks
			diContainer := di.NewContainer()
			diContainer.Register("cliConfigHandler", mockConfigHandler)
			diContainer.Register("context", mockContext)

			// Register MockHelper
			mockHelper := NewMockHelper()
			mockHelper.GetEnvVarsFunc = func() (map[string]string, error) {
				return map[string]string{
					"service1": "nginx:latest",
				}, nil
			}
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
	})

	t.Run("GetContainerConfig", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			// Given: a mock config handler, shell, context, and helper
			mockConfigHandler := config.NewMockConfigHandler()
			mockConfigHandler.GetFunc = func(key string) (interface{}, error) {
				if key == "contexts.test-context.docker.registries" {
					return []config.Registry{
						{
							Name:   "registry.test",
							Remote: "registry.remote",
							Local:  "registry.local",
						},
					}, nil
				}
				return nil, nil
			}

			mockContext := context.NewMockContext()
			mockContext.GetContextFunc = func() (string, error) {
				return "test-context", nil
			}

			// Create DI container and register mocks
			diContainer := di.NewContainer()
			diContainer.Register("cliConfigHandler", mockConfigHandler)
			diContainer.Register("context", mockContext)

			// Register MockHelper
			mockHelper := NewMockHelper()
			mockHelper.GetEnvVarsFunc = func() (map[string]string, error) {
				return map[string]string{
					"service1": "nginx:latest",
				}, nil
			}
			diContainer.Register("helper", mockHelper)

			helper, err := NewDockerHelper(diContainer)
			if err != nil {
				t.Fatalf("NewDockerHelper() error = %v", err)
			}

			// When: GetComposeConfig is called
			composeConfig, err := helper.GetComposeConfig()
			if err != nil {
				t.Fatalf("GetComposeConfig() error = %v", err)
			}

			// Then: the result should match the expected configuration
			localURL := "registry.local"
			remoteURL := "registry.remote"
			expectedConfig := types.ServiceConfig{
				Name:    "registry.test",
				Image:   "registry:2.8.3",
				Restart: "always",
				Labels: map[string]string{
					"managed_by": "windsor",
					"role":       "registry",
				},
				Environment: map[string]*string{
					"REGISTRY_PROXY_LOCALURL":  &localURL,
					"REGISTRY_PROXY_REMOTEURL": &remoteURL,
				},
			}

			found := false
			for _, config := range composeConfig.Services {
				if reflect.DeepEqual(config, expectedConfig) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected configuration:\n%+v\nto be in the list of configurations:\n%+v", expectedConfig, composeConfig.Services)
			}
		})

		t.Run("ErrorRetrievingContext", func(t *testing.T) {
			// Given: a mock context that returns an error
			mockContextWithError := context.NewMockContext()
			mockContextWithError.GetContextFunc = func() (string, error) {
				return "", errors.New("mock error retrieving context")
			}

			// Create DI container and register mocks
			diContainer := di.NewContainer()
			diContainer.Register("context", mockContextWithError)
			diContainer.Register("cliConfigHandler", config.NewMockConfigHandler())

			// Register MockHelper
			mockHelper := NewMockHelper()
			mockHelper.GetEnvVarsFunc = func() (map[string]string, error) {
				return map[string]string{
					"service1": "nginx:latest",
				}, nil
			}
			diContainer.Register("helper", mockHelper)

			// Create DockerHelper
			helper, err := NewDockerHelper(diContainer)
			if err != nil {
				t.Fatalf("NewDockerHelper() error = %v", err)
			}

			// When: GetComposeConfig is called
			_, err = helper.GetComposeConfig()

			// Then: it should return an error indicating the failure to retrieve the context
			expectedError := "error retrieving context: mock error retrieving context"
			if err == nil || !strings.Contains(err.Error(), expectedError) {
				t.Fatalf("expected error %v, got %v", expectedError, err)
			}
		})

		t.Run("ErrorRetrievingRegistries", func(t *testing.T) {
			// Given: a mock context and config handler that returns an error for Get
			mockContext := context.NewMockContext()
			mockContext.GetContextFunc = func() (string, error) {
				return "test-context", nil
			}
			mockConfigHandlerWithError := config.NewMockConfigHandler()
			mockConfigHandlerWithError.GetFunc = func(key string) (interface{}, error) {
				if key == "contexts.test-context.docker.registries" {
					return nil, errors.New("mock error retrieving registries")
				}
				return nil, nil
			}

			// Create DI container and register mocks
			diContainer := di.NewContainer()
			diContainer.Register("context", mockContext)
			diContainer.Register("cliConfigHandler", mockConfigHandlerWithError)

			// Register MockHelper
			mockHelper := NewMockHelper()
			mockHelper.GetEnvVarsFunc = func() (map[string]string, error) {
				return map[string]string{
					"service1": "nginx:latest",
				}, nil
			}
			diContainer.Register("helper", mockHelper)

			// Create DockerHelper
			helper, err := NewDockerHelper(diContainer)
			if err != nil {
				t.Fatalf("NewDockerHelper() error = %v", err)
			}

			// When: GetComposeConfig is called
			_, err = helper.GetComposeConfig()

			// Then: it should return an error indicating the failure to retrieve registries
			expectedError := "error retrieving registries from configuration: mock error retrieving registries"
			if err == nil || !strings.Contains(err.Error(), expectedError) {
				t.Fatalf("expected error %v, got %v", expectedError, err)
			}
		})

		t.Run("ErrorConvertingRegistries", func(t *testing.T) {
			// Mock the DI container
			diContainer := di.NewContainer()

			// Mock the ConfigHandler to return an invalid type for registries
			mockConfigHandler := config.NewMockConfigHandler()
			mockConfigHandler.GetFunc = func(key string) (interface{}, error) {
				if key == "contexts.test.docker.registries" {
					return "invalidType", nil // Return a string instead of a list
				}
				return nil, fmt.Errorf("unexpected key: %s", key)
			}
			mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) (bool, error) {
				if key == "contexts.test.docker.enabled" {
					return true, nil // Docker is enabled
				}
				return false, fmt.Errorf("unexpected key: %s", key)
			}

			// Mock the ContextInterface
			mockContext := context.NewMockContext()
			mockContext.GetContextFunc = func() (string, error) {
				return "test", nil
			}

			// Register mocks in the DI container
			diContainer.Register("cliConfigHandler", mockConfigHandler)
			diContainer.Register("context", mockContext)

			// Register MockHelper to avoid error resolving helpers
			mockHelper := NewMockHelper()
			mockHelper.GetEnvVarsFunc = func() (map[string]string, error) {
				return map[string]string{
					"service1": "nginx:latest",
				}, nil
			}
			diContainer.Register("helper", mockHelper)

			// Create a new DockerHelper
			dockerHelper, err := NewDockerHelper(diContainer)
			if err != nil {
				t.Fatalf("NewDockerHelper() error = %v", err)
			}

			// Call GetContainerConfig and expect an error
			_, err = dockerHelper.GetComposeConfig()
			expectedError := "error converting registries to expected format"
			if err == nil || !strings.Contains(err.Error(), expectedError) {
				t.Fatalf("expected error containing %v, got %v", expectedError, err)
			}
		})
	})

	t.Run("WriteConfig", func(t *testing.T) {

		t.Run("Success", func(t *testing.T) {
			// Given: a mock config handler and context
			mockConfigHandler := config.NewMockConfigHandler()
			mockConfigHandler.GetFunc = func(key string) (interface{}, error) {
				if key == "contexts.test-context.docker.registries" {
					return []config.Registry{
						{
							Name:   "registry1",
							Local:  "http://localhost:5000",
							Remote: "https://registry1.example.com",
						},
					}, nil
				}
				return nil, fmt.Errorf("key not found: %s", key)
			}
			mockConfigHandler.SetFunc = func(key string, value interface{}) error {
				return nil
			}
			mockContext := context.NewMockContext()
			mockContext.GetContextFunc = func() (string, error) {
				return "test-context", nil
			}
			mockContext.GetConfigRootFunc = func() (string, error) {
				return filepath.Join(os.TempDir(), "contexts", "test-context"), nil
			}

			// Create DI container and register mocks
			diContainer := di.NewContainer()
			diContainer.Register("cliConfigHandler", mockConfigHandler)
			diContainer.Register("context", mockContext)

			// Register MockHelper
			mockHelper := NewMockHelper()
			mockHelper.GetEnvVarsFunc = func() (map[string]string, error) {
				return map[string]string{
					"service1": "nginx:latest",
				}, nil
			}
			mockHelper.GetComposeConfigFunc = func() (*types.Config, error) {
				return &types.Config{
					Services: []types.ServiceConfig{
						{
							Name:    "registry1",
							Image:   "registry:2.8.3",
							Restart: "always",
						},
					},
				}, nil
			}
			diContainer.Register("helper", mockHelper)

			// Create DockerHelper
			helper, err := NewDockerHelper(diContainer)
			if err != nil {
				t.Fatalf("NewDockerHelper() error = %v", err)
			}

			// When: WriteConfig is called
			err = helper.WriteConfig()

			// Then: it should complete without errors
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
		})

		t.Run("ErrorWritingDockerComposeFile", func(t *testing.T) {
			// Given: a mock config handler and context
			mockConfigHandler := config.NewMockConfigHandler()
			mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) (bool, error) {
				if key == "contexts.test-context.docker.enabled" {
					return true, nil
				}
				return false, fmt.Errorf("key not found: %s", key)
			}
			mockConfigHandler.GetFunc = func(key string) (interface{}, error) {
				if key == "contexts.test-context.docker.registries" {
					return []config.Registry{}, nil // Empty list of registries
				}
				return nil, fmt.Errorf("key not found: %s", key)
			}
			mockConfigHandler.SetFunc = func(key string, value interface{}) error {
				return nil
			}
			mockContext := context.NewMockContext()
			mockContext.GetContextFunc = func() (string, error) {
				return "test-context", nil
			}
			mockContext.GetConfigRootFunc = func() (string, error) {
				return filepath.Join(os.TempDir(), "contexts", "test-context"), nil
			}

			// Create DI container and register mocks
			diContainer := di.NewContainer()
			diContainer.Register("cliConfigHandler", mockConfigHandler)
			diContainer.Register("context", mockContext)

			// Register MockHelper
			mockHelper := NewMockHelper()
			mockHelper.GetEnvVarsFunc = func() (map[string]string, error) {
				return map[string]string{
					"service1": "nginx:latest",
				}, nil
			}
			diContainer.Register("helper", mockHelper)

			// Create DockerHelper
			helper, err := NewDockerHelper(diContainer)
			if err != nil {
				t.Fatalf("NewDockerHelper() error = %v", err)
			}

			// Mock the writeFile function to return an error
			originalWriteFile := writeFile
			writeFile = func(filename string, data []byte, perm os.FileMode) error {
				return fmt.Errorf("mock error writing file")
			}
			defer func() { writeFile = originalWriteFile }()

			// When: WriteConfig is called
			err = helper.WriteConfig()

			// Then: it should return an error indicating the failure to write the docker-compose file
			expectedError := "error writing docker-compose file: mock error writing file"
			if err == nil || !strings.Contains(err.Error(), expectedError) {
				t.Fatalf("expected error containing %v, got %v", expectedError, err)
			}
		})

		t.Run("ErrorGettingContainerConfig", func(t *testing.T) {
			// Given: a mock config handler and context
			mockConfigHandler := config.NewMockConfigHandler()
			mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) (string, error) {
				if key == "contexts.test-context.docker.enabled" {
					return "true", nil
				}
				if key == "contexts.test-context.docker.registries" {
					return "", nil
				}
				return "", fmt.Errorf("key not found: %s", key)
			}
			mockConfigHandler.SetFunc = func(key string, value interface{}) error {
				return nil
			}
			mockContext := context.NewMockContext()
			mockContext.GetContextFunc = func() (string, error) {
				return "test-context", nil
			}
			mockContext.GetConfigRootFunc = func() (string, error) {
				return filepath.Join(os.TempDir(), "contexts", "test-context"), nil
			}

			// Create DI container and register mocks
			diContainer := di.NewContainer()
			diContainer.Register("cliConfigHandler", mockConfigHandler)
			diContainer.Register("context", mockContext)

			// Register MockHelper with GetContainerConfigFunc that returns an error
			expectedError := errors.New("mock error getting container config")
			mockHelper := NewMockHelper()
			mockHelper.GetEnvVarsFunc = func() (map[string]string, error) {
				return map[string]string{
					"service1": "nginx:latest",
				}, nil
			}
			mockHelper.GetComposeConfigFunc = func() (*types.Config, error) {
				return nil, expectedError
			}
			diContainer.Register("helper", mockHelper)

			// Create DockerHelper
			helper, err := NewDockerHelper(diContainer)
			if err != nil {
				t.Fatalf("NewDockerHelper() error = %v", err)
			}

			// When: WriteConfig is called
			err = helper.WriteConfig()

			// Then: it should return an error indicating the failure to get the container config
			if err == nil || !strings.Contains(err.Error(), "error getting container config") {
				t.Fatalf("expected error containing 'error getting container config', got %v", err)
			}
		})

		t.Run("ErrorRetrievingConfigRoot", func(t *testing.T) {
			// Given: a mock config handler and context
			mockConfigHandler := config.NewMockConfigHandler()
			mockConfigHandler.GetFunc = func(key string) (interface{}, error) {
				if key == "contexts.test-context.docker.enabled" {
					return true, nil
				}
				if key == "contexts.test-context.docker.registries" {
					return []config.Registry{}, nil
				}
				return nil, fmt.Errorf("key not found: %s", key)
			}
			mockConfigHandler.SetFunc = func(key string, value interface{}) error {
				return nil
			}
			mockContext := context.NewMockContext()
			mockContext.GetContextFunc = func() (string, error) {
				return "test-context", nil
			}
			mockContext.GetConfigRootFunc = func() (string, error) {
				return "", fmt.Errorf("mock error retrieving config root")
			}

			// Create DI container and register mocks
			diContainer := di.NewContainer()
			diContainer.Register("cliConfigHandler", mockConfigHandler)
			diContainer.Register("context", mockContext)

			// Register MockHelper
			mockHelper := NewMockHelper()
			mockHelper.GetEnvVarsFunc = func() (map[string]string, error) {
				return map[string]string{
					"service1": "nginx:latest",
				}, nil
			}
			diContainer.Register("helper", mockHelper)

			// Create DockerHelper
			helper, err := NewDockerHelper(diContainer)
			if err != nil {
				t.Fatalf("NewDockerHelper() error = %v", err)
			}

			// When: WriteConfig is called
			err = helper.WriteConfig()

			// Then: it should return an error indicating the failure to retrieve the config root
			expectedError := "error retrieving config root: mock error retrieving config root"
			if err == nil || !strings.Contains(err.Error(), expectedError) {
				t.Fatalf("expected error %v, got %v", expectedError, err)
			}
		})

		t.Run("ErrorCreatingParentContextFolder", func(t *testing.T) {
			// Given: a mock config handler and context
			mockConfigHandler := config.NewMockConfigHandler()
			mockConfigHandler.SetFunc = func(key string, value interface{}) error {
				return nil
			}
			mockConfigHandler.GetFunc = func(key string) (interface{}, error) {
				if key == "contexts.test-context.docker.registries" {
					return []config.Registry{}, nil
				}
				return nil, fmt.Errorf("key not found: %s", key)
			}
			mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) (bool, error) {
				if key == "contexts.test-context.docker.enabled" {
					return true, nil
				}
				return false, fmt.Errorf("key not found: %s", key)
			}
			mockContext := context.NewMockContext()
			mockContext.GetContextFunc = func() (string, error) {
				return "test-context", nil
			}
			mockContext.GetConfigRootFunc = func() (string, error) {
				return filepath.Join(os.TempDir(), "contexts", "test-context"), nil
			}

			// Create DI container and register mocks
			diContainer := di.NewContainer()
			diContainer.Register("cliConfigHandler", mockConfigHandler)
			diContainer.Register("context", mockContext)

			// Register MockHelper
			mockHelper := NewMockHelper()
			mockHelper.GetEnvVarsFunc = func() (map[string]string, error) {
				return map[string]string{
					"service1": "nginx:latest",
				}, nil
			}
			diContainer.Register("helper", mockHelper)

			// Create DockerHelper
			helper, err := NewDockerHelper(diContainer)
			if err != nil {
				t.Fatalf("NewDockerHelper() error = %v", err)
			}

			// Mock the os.MkdirAll function to return an error
			originalMkdirAll := mkdirAll
			mkdirAll = func(path string, perm os.FileMode) error {
				return fmt.Errorf("mock error creating directory")
			}
			defer func() { mkdirAll = originalMkdirAll }()

			// When: WriteConfig is called
			err = helper.WriteConfig()

			// Then: it should return an error indicating the failure to create the parent context folder
			expectedError := "error creating parent context folder: mock error creating directory"
			if err == nil || !strings.Contains(err.Error(), expectedError) {
				t.Fatalf("expected error containing %v, got %v", expectedError, err)
			}
		})

		t.Run("SetEnabled", func(t *testing.T) {
			// Given a mock context and config handler
			mockContext := context.NewMockContext()
			mockContext.GetContextFunc = func() (string, error) {
				return "test-context", nil
			}
			mockContext.GetConfigRootFunc = func() (string, error) {
				return "/mock/config/root", nil
			}
			mockConfigHandler := config.NewMockConfigHandler()
			mockConfigHandler.GetFunc = func(key string) (interface{}, error) {
				if key == "contexts.test-context.docker.registries" {
					return []config.Registry{}, nil // Empty registry
				}
				return nil, fmt.Errorf("key not found: %s", key)
			}

			// And a DI container with the mock context and config handler registered
			diContainer := di.NewContainer()
			diContainer.Register("context", mockContext)
			diContainer.Register("cliConfigHandler", mockConfigHandler)

			// Register MockHelper to avoid error resolving helpers
			mockHelper := NewMockHelper()
			mockHelper.GetComposeConfigFunc = func() (*types.Config, error) {
				return &types.Config{
					Services: []types.ServiceConfig{
						{
							Name:  "service1",
							Image: "nginx:latest",
						},
					},
				}, nil
			}
			diContainer.Register("helper", mockHelper)

			// When creating a new DockerHelper
			helper, err := NewDockerHelper(diContainer)
			if err != nil {
				t.Fatalf("NewDockerHelper() error = %v", err)
			}

			// Mock the os.MkdirAll function to return an error
			originalMkdirAll := mkdirAll
			mkdirAll = func(path string, perm os.FileMode) error {
				return fmt.Errorf("mock error creating parent context folder: mkdir /mock: read-only file system")
			}
			defer func() { mkdirAll = originalMkdirAll }()

			// And calling WriteConfig
			err = helper.WriteConfig()
			if err == nil || !strings.Contains(err.Error(), "mock error creating parent context folder: mkdir /mock: read-only file system") {
				t.Fatalf("WriteConfig() error = %v", err)
			}
		})

		t.Run("NoRegistriesDefinedButDockerEnabled", func(t *testing.T) {
			// Mock the DI container
			diContainer := di.NewContainer()

			// Mock the ConfigHandler
			mockConfigHandler := config.NewMockConfigHandler()
			mockConfigHandler.GetFunc = func(key string) (interface{}, error) {
				if key == "contexts.test.docker.registries" {
					return []config.Registry{}, nil // No registries defined
				}
				return nil, fmt.Errorf("unexpected key: %s", key)
			}
			mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) (bool, error) {
				if key == "contexts.test.docker.enabled" {
					return true, nil // Docker is enabled
				}
				return false, fmt.Errorf("unexpected key: %s", key)
			}

			// Mock the ContextInterface
			mockContext := context.NewMockContext()
			mockContext.GetContextFunc = func() (string, error) {
				return "test", nil
			}

			// Register mocks in the DI container
			diContainer.Register("cliConfigHandler", mockConfigHandler)
			diContainer.Register("context", mockContext)

			// Register MockHelper to avoid error resolving helpers
			mockHelper := NewMockHelper()
			mockHelper.GetEnvVarsFunc = func() (map[string]string, error) {
				return map[string]string{
					"service1": "nginx:latest",
				}, nil
			}
			diContainer.Register("helper", mockHelper)

			// Create a new DockerHelper
			dockerHelper, err := NewDockerHelper(diContainer)
			if err != nil {
				t.Fatalf("NewDockerHelper() error = %v", err)
			}

			// Call GetComposeConfig
			composeConfig, err := dockerHelper.GetComposeConfig()
			if err != nil {
				t.Fatalf("GetComposeConfig() error = %v", err)
			}

			// Verify that no registries are returned
			if len(composeConfig.Services) != 0 {
				t.Fatalf("Expected no services, got %d", len(composeConfig.Services))
			}
		})

		t.Run("ErrorMarshalingYAML", func(t *testing.T) {
			// Given: a mock config handler and context
			mockConfigHandler := config.NewMockConfigHandler()
			mockConfigHandler.SetFunc = func(key string, value interface{}) error {
				return nil
			}
			mockConfigHandler.GetFunc = func(key string) (interface{}, error) {
				if key == "contexts.test-context.docker.registries" {
					return []config.Registry{}, nil
				}
				return nil, fmt.Errorf("key not found: %s", key)
			}
			mockContext := context.NewMockContext()
			mockContext.GetContextFunc = func() (string, error) {
				return "test-context", nil
			}
			mockContext.GetConfigRootFunc = func() (string, error) {
				return filepath.Join(os.TempDir(), "contexts", "test-context"), nil
			}

			// Create DI container and register mocks
			diContainer := di.NewContainer()
			diContainer.Register("cliConfigHandler", mockConfigHandler)
			diContainer.Register("context", mockContext)
			// Register MockHelper
			mockHelper := NewMockHelper()
			mockHelper.GetEnvVarsFunc = func() (map[string]string, error) {
				return map[string]string{
					"service1": "nginx:latest",
				}, nil
			}
			diContainer.Register("helper", mockHelper)

			// Create DockerHelper
			helper, err := NewDockerHelper(diContainer)
			if err != nil {
				t.Fatalf("NewDockerHelper() error = %v", err)
			}

			// Mock the yamlMarshal function to return an error
			yamlMarshal = func(v interface{}) ([]byte, error) {
				return nil, errors.New("mock error marshaling YAML")
			}
			defer func() { yamlMarshal = originalYamlMarshal }() // Restore original function after test

			// When: WriteConfig is called
			err = helper.WriteConfig()

			// Then: it should return an error indicating the failure to marshal YAML
			expectedError := "error marshaling docker-compose config to YAML: mock error marshaling YAML"
			if err == nil || !strings.Contains(err.Error(), expectedError) {
				t.Fatalf("expected error containing %v, got %v", expectedError, err)
			}
		})
	})

	t.Run("VolumesAndNetworks", func(t *testing.T) {
		// Given: a mock config handler and a mock helper that returns a config with volumes and networks
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetFunc = func(key string) (interface{}, error) {
			if key == "contexts.test-context.docker.registries" {
				return []config.Registry{}, nil
			}
			return nil, fmt.Errorf("key not found: %s", key)
		}
		mockHelper := NewMockHelper()
		mockHelper.GetComposeConfigFunc = func() (*types.Config, error) {
			return &types.Config{
				Volumes: map[string]types.VolumeConfig{
					"volume1": {},
				},
				Networks: map[string]types.NetworkConfig{
					"network1": {},
				},
			}, nil
		}

		// Create a DI container and register the mock config handler and mock helper
		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("helper", mockHelper)

		// Attempt to create a new DockerHelper without registering context
		_, err := NewDockerHelper(diContainer)
		if err == nil || !strings.Contains(err.Error(), "error resolving context") {
			t.Fatalf("expected error resolving context, got %v", err)
		}

		// Register the mock context
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}
		mockContext.GetConfigRootFunc = func() (string, error) {
			return "", fmt.Errorf("mock error retrieving config root")
		}
		diContainer.Register("context", mockContext)

		// Create a new DockerHelper
		dockerHelper, err := NewDockerHelper(diContainer)
		if err != nil {
			t.Fatalf("NewDockerHelper() error = %v", err)
		}

		// When: WriteConfig is called
		err = dockerHelper.WriteConfig()
		if err == nil || !strings.Contains(err.Error(), "error retrieving config root") {
			t.Fatalf("expected error retrieving config root, got %v", err)
		}
	})
}
