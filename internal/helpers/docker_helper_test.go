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
			mockConfigHandler := &config.MockConfigHandler{}
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
			mockConfigHandler := &config.MockConfigHandler{}
			mockContext := &context.MockContext{
				GetContextFunc: func() (string, error) {
					return "test-context", nil
				},
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
				mockContext := &context.MockContext{
					GetConfigRootFunc: func() (string, error) {
						return contextPath, nil
					},
				}

				// Create DI container and register mocks
				diContainer := di.NewContainer()
				diContainer.Register("cliConfigHandler", &config.MockConfigHandler{})
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
				mockContext := &context.MockContext{
					GetConfigRootFunc: func() (string, error) {
						return contextPath, nil
					},
				}

				// Create DI container and register mocks
				diContainer := di.NewContainer()
				diContainer.Register("cliConfigHandler", &config.MockConfigHandler{})
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
				diContainer.Register("cliConfigHandler", &config.MockConfigHandler{})
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
				diContainer.Register("cliConfigHandler", &config.MockConfigHandler{})
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
		})
	})

	t.Run("PostEnvExec", func(t *testing.T) {
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
			diContainer.Register("cliConfigHandler", mockConfigHandler)
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
	})

	t.Run("SetConfig", func(t *testing.T) {
		t.Run("SetEnabledConfigSuccess", func(t *testing.T) {
			// Given: a mock config handler, shell, and context
			mockConfigHandler := &config.MockConfigHandler{
				GetConfigValueFunc: func(key string) (string, error) {
					if key == "contexts.test-context.docker.enabled" {
						return "true", nil
					}
					if key == "contexts.test-context.docker.registries" {
						return `
						- name: registry1
						  local: "http://localhost:5000"
						  remote: "https://registry1.example.com"
						`, nil
					}
					return "", fmt.Errorf("key not found: %s", key)
				},
				SetConfigValueFunc: func(key string, value interface{}) error {
					return nil
				},
			}
			mockContext := &context.MockContext{
				GetContextFunc: func() (string, error) {
					return "test-context", nil
				},
				GetConfigRootFunc: func() (string, error) {
					return filepath.Join(os.TempDir(), "contexts", "test-context"), nil
				},
			}

			// Create DI container and register mocks
			diContainer := di.NewContainer()
			diContainer.Register("cliConfigHandler", mockConfigHandler)
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
			diContainer.Register("cliConfigHandler", mockConfigHandler)

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
			mockContext := &context.MockContext{
				GetContextFunc: func() (string, error) {
					return "local", nil
				},
			}
			diContainer := di.NewContainer()
			diContainer.Register("cliConfigHandler", mockConfigHandler)
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
				SetConfigValueFunc: func(key string, value interface{}) error {
					return errors.New("mock error setting config value")
				},
			}
			mockContext := &context.MockContext{
				GetContextFunc: func() (string, error) {
					return "local", nil
				},
			}
			diContainer := di.NewContainer()
			diContainer.Register("cliConfigHandler", mockConfigHandlerWithError)
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
			expectedError := "mock error setting config value"
			if err == nil || !strings.Contains(err.Error(), expectedError) {
				t.Fatalf("expected error %v, got %v", expectedError, err)
			}
		})

		t.Run("ErrorWritingDockerComposeFile", func(t *testing.T) {
			// Given: a mock config handler and context
			mockConfigHandler := &config.MockConfigHandler{
				GetConfigValueFunc: func(key string) (string, error) {
					if key == "contexts.test-context.docker.enabled" {
						return "true", nil
					}
					if key == "contexts.test-context.docker.registries" {
						registries := `
- name: registry.test
  local: ""
  remote: ""
`
						return registries, nil
					}
					return "", fmt.Errorf("key not found: %s", key)
				},
				SetConfigValueFunc: func(key string, value interface{}) error {
					return nil
				},
			}
			mockContext := &context.MockContext{
				GetContextFunc: func() (string, error) {
					return "test-context", nil
				},
				GetConfigRootFunc: func() (string, error) {
					return filepath.Join(os.TempDir(), "contexts", "test-context"), nil
				},
			}

			// Create DI container and register mocks
			diContainer := di.NewContainer()
			diContainer.Register("cliConfigHandler", mockConfigHandler)
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

			// Mock the writeFile function to return an error
			originalWriteFile := writeFile
			writeFile = func(filename string, data []byte, perm os.FileMode) error {
				return fmt.Errorf("mock error writing file")
			}
			defer func() { writeFile = originalWriteFile }()

			// When: SetConfig is called with "enabled" key
			err = helper.SetConfig("enabled", "true")

			// Then: it should return an error indicating the failure to write the docker-compose file
			expectedError := "error writing docker-compose file: mock error writing file"
			if err == nil || !strings.Contains(err.Error(), expectedError) {
				t.Fatalf("expected error containing %v, got %v", expectedError, err)
			}
		})

		t.Run("ErrorGettingContainerConfig", func(t *testing.T) {
			// Given: a mock config handler and context
			mockConfigHandler := &config.MockConfigHandler{
				GetConfigValueFunc: func(key string) (string, error) {
					if key == "contexts.test-context.docker.enabled" {
						return "true", nil
					}
					return "", fmt.Errorf("key not found: %s", key)
				},
				SetConfigValueFunc: func(key string, value interface{}) error {
					return nil
				},
			}
			mockContext := &context.MockContext{
				GetContextFunc: func() (string, error) {
					return "test-context", nil
				},
				GetConfigRootFunc: func() (string, error) {
					return filepath.Join(os.TempDir(), "contexts", "test-context"), nil
				},
			}

			// Create DI container and register mocks
			diContainer := di.NewContainer()
			diContainer.Register("cliConfigHandler", mockConfigHandler)
			diContainer.Register("context", mockContext)

			// Register MockHelper with GetContainerConfigFunc that returns an error
			expectedError := errors.New("mock error getting container config")
			mockHelper := NewMockHelper(func() (map[string]string, error) {
				return map[string]string{
					"service1": "nginx:latest",
				}, nil
			})
			mockHelper.GetContainerConfigFunc = func() ([]types.ServiceConfig, error) {
				return nil, expectedError
			}
			diContainer.Register("helper", mockHelper)

			// Create DockerHelper
			helper, err := NewDockerHelper(diContainer)
			if err != nil {
				t.Fatalf("NewDockerHelper() error = %v", err)
			}

			// When: SetConfig is called with "enabled" key
			err = helper.SetConfig("enabled", "true")

			// Then: it should return an error indicating the failure to get the container config
			if err == nil || !strings.Contains(err.Error(), "error getting container config") {
				t.Fatalf("expected error containing 'error getting container config', got %v", err)
			}
		})

		t.Run("ErrorRetrievingConfigRoot", func(t *testing.T) {
			// Given: a mock config handler and context
			mockConfigHandler := &config.MockConfigHandler{
				GetConfigValueFunc: func(key string) (string, error) {
					if key == "contexts.test-context.docker.enabled" {
						return "true", nil
					}
					if key == "contexts.test-context.docker.registries" {
						return `
						- name: registry1
						  local: "http://localhost:5000"
						  remote: "https://registry1.example.com"
						- name: registry2
						  local: "http://localhost:5001"
						  remote: "https://registry2.example.com"
						`, nil
					}
					return "", fmt.Errorf("key not found: %s", key)
				},
				SetConfigValueFunc: func(key string, value interface{}) error {
					return nil
				},
			}
			mockContext := &context.MockContext{
				GetContextFunc: func() (string, error) {
					return "test-context", nil
				},
				GetConfigRootFunc: func() (string, error) {
					return "", fmt.Errorf("mock error retrieving config root")
				},
			}

			// Create DI container and register mocks
			diContainer := di.NewContainer()
			diContainer.Register("cliConfigHandler", mockConfigHandler)
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

			// Then: it should return an error indicating the failure to retrieve the config root
			expectedError := "error retrieving config root: mock error retrieving config root"
			if err == nil || !strings.Contains(err.Error(), expectedError) {
				t.Fatalf("expected error %v, got %v", expectedError, err)
			}
		})

		t.Run("EnabledKeySetToFalse", func(t *testing.T) {
			// Given: a mock config handler and context
			mockConfigHandler := &config.MockConfigHandler{
				SetConfigValueFunc: func(key string, value interface{}) error {
					return nil
				},
				GetConfigValueFunc: func(key string) (string, error) {
					if key == "contexts.test-context.docker.enabled" {
						return "false", nil
					}
					return "", fmt.Errorf("key not found: %s", key)
				},
			}
			mockContext := &context.MockContext{
				GetContextFunc: func() (string, error) {
					return "test-context", nil
				},
			}

			// Create DI container and register mocks
			diContainer := di.NewContainer()
			diContainer.Register("cliConfigHandler", mockConfigHandler)
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

			// When: SetConfig is called with "enabled" key set to nil
			err = helper.SetConfig("enabled", "")

			// Then: it should return no error and not call writeDockerComposeFile
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
		})

		t.Run("ErrorCreatingParentContextFolder", func(t *testing.T) {
			// Given: a mock config handler and context
			mockConfigHandler := &config.MockConfigHandler{
				SetConfigValueFunc: func(key string, value interface{}) error {
					return nil
				},
				GetConfigValueFunc: func(key string) (string, error) {
					if key == "contexts.test-context.docker.enabled" {
						return "true", nil
					}
					if key == "contexts.test-context.docker.registries" {
						return `
						- name: registry.test
						  local: ""
						  remote: ""
						- name: registry-1.docker.test
						  local: "https://docker.io"
						  remote: "https://registry-1.docker.io"
						`, nil
					}
					return "", fmt.Errorf("key not found: %s", key)
				},
			}
			mockContext := &context.MockContext{
				GetContextFunc: func() (string, error) {
					return "test-context", nil
				},
				GetConfigRootFunc: func() (string, error) {
					return filepath.Join(os.TempDir(), "contexts", "test-context"), nil
				},
			}

			// Create DI container and register mocks
			diContainer := di.NewContainer()
			diContainer.Register("cliConfigHandler", mockConfigHandler)
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

			// Mock the os.MkdirAll function to return an error
			originalMkdirAll := mkdirAll
			mkdirAll = func(path string, perm os.FileMode) error {
				return fmt.Errorf("mock error creating directory")
			}
			defer func() { mkdirAll = originalMkdirAll }()

			// When: SetConfig is called with "enabled" key
			err = helper.SetConfig("enabled", "true")

			// Then: it should return an error indicating the failure to create the parent context folder
			expectedError := "error creating parent context folder: mock error creating directory"
			if err == nil || !strings.Contains(err.Error(), expectedError) {
				t.Fatalf("expected error containing %v, got %v", expectedError, err)
			}
		})

		t.Run("SetEnabled", func(t *testing.T) {
			// Given a mock context and config handler
			mockContext := &context.MockContext{
				GetContextFunc: func() (string, error) {
					return "test-context", nil
				},
				GetConfigRootFunc: func() (string, error) {
					return "/mock/config/root", nil
				},
			}
			mockConfigHandler := config.NewMockConfigHandler()

			// And a DI container with the mock context and config handler registered
			diContainer := di.NewContainer()
			diContainer.Register("context", mockContext)
			diContainer.Register("cliConfigHandler", mockConfigHandler)

			// Register MockHelper to avoid error resolving helpers
			mockHelper := NewMockHelper(func() (map[string]string, error) {
				return map[string]string{
					"service1": "nginx:latest",
				}, nil
			})
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

			// And setting the enabled config
			err = helper.SetConfig("enabled", "true")
			if err == nil || !strings.Contains(err.Error(), "mock error creating parent context folder: mkdir /mock: read-only file system") {
				t.Fatalf("SetConfig() error = %v", err)
			}
		})

		t.Run("SetRegistryEnabled", func(t *testing.T) {
			// Given a mock context and config handler
			mockContext := &context.MockContext{
				GetContextFunc: func() (string, error) {
					return "test-context", nil
				},
			}
			mockConfigHandler := &config.MockConfigHandler{
				GetConfigValueFunc: func(key string) (string, error) {
					if key == "contexts.test-context.docker.registry_enabled" {
						return "true", nil
					}
					return "", fmt.Errorf("key not found: %s", key)
				},
			}

			// And a DI container with the mock context and config handler registered
			diContainer := di.NewContainer()
			diContainer.Register("context", mockContext)
			diContainer.Register("cliConfigHandler", mockConfigHandler)

			// Register MockHelper to avoid error resolving helpers
			mockHelper := NewMockHelper(func() (map[string]string, error) {
				return map[string]string{
					"service1": "nginx:latest",
				}, nil
			})
			diContainer.Register("helper", mockHelper)

			// When creating a new DockerHelper
			helper, err := NewDockerHelper(diContainer)
			if err != nil {
				t.Fatalf("NewDockerHelper() error = %v", err)
			}

			// And setting the registry_enabled config
			err = helper.SetConfig("registry_enabled", "true")
			if err != nil {
				t.Fatalf("SetConfig() error = %v", err)
			}

			// Then the config value should be set correctly
			value, err := mockConfigHandler.GetConfigValue("contexts.test-context.docker.registry_enabled")
			if err != nil {
				t.Fatalf("GetConfigValue() error = %v", err)
			}
			if value != "true" {
				t.Fatalf("expected registry_enabled to be 'true', got '%s'", value)
			}
		})

		t.Run("UnsupportedConfigKey", func(t *testing.T) {
			// Given a mock context and config handler
			mockContext := &context.MockContext{
				GetContextFunc: func() (string, error) {
					return "test-context", nil
				},
			}
			mockConfigHandler := config.NewMockConfigHandler()

			// And a DI container with the mock context and config handler registered
			diContainer := di.NewContainer()
			diContainer.Register("context", mockContext)
			diContainer.Register("cliConfigHandler", mockConfigHandler)

			// Register MockHelper to avoid error resolving helpers
			mockHelper := NewMockHelper(func() (map[string]string, error) {
				return map[string]string{
					"service1": "nginx:latest",
				}, nil
			})
			diContainer.Register("helper", mockHelper)

			// When creating a new DockerHelper
			helper, err := NewDockerHelper(diContainer)
			if err != nil {
				t.Fatalf("NewDockerHelper() error = %v", err)
			}

			// And setting an unsupported config key
			err = helper.SetConfig("unsupported_key", "some_value")

			// Then it should return an error
			if err == nil || err.Error() != "unsupported config key: unsupported_key" {
				t.Fatalf("expected error 'unsupported config key: unsupported_key', got '%v'", err)
			}
		})
	})

	t.Run("ErrorMarshalingYAML", func(t *testing.T) {
		// Given: a mock config handler and context
		mockConfigHandler := &config.MockConfigHandler{
			SetConfigValueFunc: func(key string, value interface{}) error {
				return nil
			},
		}
		mockContext := &context.MockContext{
			GetContextFunc: func() (string, error) {
				return "test-context", nil
			},
			GetConfigRootFunc: func() (string, error) {
				return filepath.Join(os.TempDir(), "contexts", "test-context"), nil
			},
		}

		// Create DI container and register mocks
		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
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

		// Mock the yamlMarshal function to return an error
		yamlMarshal = func(v interface{}) ([]byte, error) {
			return nil, errors.New("mock error marshaling YAML")
		}
		defer func() { yamlMarshal = originalYamlMarshal }() // Restore original function after test

		// When: SetConfig is called with "enabled" key set to "true"
		err = helper.SetConfig("enabled", "true")

		// Then: it should return an error indicating the failure to marshal YAML
		expectedError := "error marshaling docker-compose config to YAML: mock error marshaling YAML"
		if err == nil || !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("expected error containing %v, got %v", expectedError, err)
		}
	})

	t.Run("GetContainerConfig", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			// Given: a mock config handler, shell, context, and helper
			mockConfigHandler := config.NewMockConfigHandler()
			mockConfigHandler.GetConfigValueFunc = func(key string) (string, error) {
				if key == "contexts.test-context.docker.registries" {
					return `
- name: registry.test
  remote: registry.remote
  local: registry.local
`, nil
				}
				return "", nil
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

			// When: GetContainerConfig is called
			containerConfig, err := helper.GetContainerConfig()
			if err != nil {
				t.Fatalf("GetContainerConfig() error = %v", err)
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
			for _, config := range containerConfig {
				if reflect.DeepEqual(config, expectedConfig) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected configuration:\n%+v\nto be in the list of configurations:\n%+v", expectedConfig, containerConfig)
			}
		})

		t.Run("ErrorRetrievingContext", func(t *testing.T) {
			// Given: a mock context that returns an error
			mockContextWithError := &context.MockContext{
				GetContextFunc: func() (string, error) {
					return "", errors.New("mock error retrieving context")
				},
			}

			// Create DI container and register mocks
			diContainer := di.NewContainer()
			diContainer.Register("context", mockContextWithError)
			diContainer.Register("cliConfigHandler", &config.MockConfigHandler{})

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

			// When: GetContainerConfig is called
			_, err = helper.GetContainerConfig()

			// Then: it should return an error indicating the failure to retrieve the context
			expectedError := "error retrieving context: mock error retrieving context"
			if err == nil || !strings.Contains(err.Error(), expectedError) {
				t.Fatalf("expected error %v, got %v", expectedError, err)
			}
		})

		t.Run("ErrorRetrievingRegistries", func(t *testing.T) {
			// Given: a mock context and config handler that returns an error for GetConfigValue
			mockContext := &context.MockContext{
				GetContextFunc: func() (string, error) {
					return "test-context", nil
				},
			}
			mockConfigHandlerWithError := &config.MockConfigHandler{
				GetConfigValueFunc: func(key string) (string, error) {
					if key == "contexts.test-context.docker.registries" {
						return "", errors.New("mock error retrieving registries")
					}
					return "", nil
				},
			}

			// Create DI container and register mocks
			diContainer := di.NewContainer()
			diContainer.Register("context", mockContext)
			diContainer.Register("cliConfigHandler", mockConfigHandlerWithError)

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

			// When: GetContainerConfig is called
			_, err = helper.GetContainerConfig()

			// Then: it should return an error indicating the failure to retrieve registries
			expectedError := "error retrieving registries from configuration: mock error retrieving registries"
			if err == nil || !strings.Contains(err.Error(), expectedError) {
				t.Fatalf("expected error %v, got %v", expectedError, err)
			}
		})

		t.Run("UseDefaultRegistriesForLocalContext", func(t *testing.T) {
			// Given: a mock context that returns a local context
			mockContext := &context.MockContext{
				GetContextFunc: func() (string, error) {
					return "local", nil
				},
			}

			// And a mock config handler that returns an empty string for registries
			mockConfigHandler := &config.MockConfigHandler{
				GetConfigValueFunc: func(key string) (string, error) {
					if key == "contexts.local.docker.registries" {
						return "", nil
					}
					return "", fmt.Errorf("key not found: %s", key)
				},
			}

			// Create DI container and register mocks
			diContainer := di.NewContainer()
			diContainer.Register("context", mockContext)
			diContainer.Register("cliConfigHandler", mockConfigHandler)

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

			// When: GetContainerConfig is called
			containerConfig, err := helper.GetContainerConfig()
			if err != nil {
				t.Fatalf("GetContainerConfig() error = %v", err)
			}

			// Then: the default registries should be used
			if len(containerConfig) != len(defaultRegistries) {
				t.Fatalf("expected %d default registries, got %d", len(defaultRegistries), len(containerConfig))
			}

			for i, registry := range defaultRegistries {
				expectedService := generateRegistryService(registry["name"], registry["remote"], registry["local"])
				if !reflect.DeepEqual(containerConfig[i], expectedService) {
					t.Errorf("expected service %v, got %v", expectedService, containerConfig[i])
				}
			}
		})

		t.Run("ErrorUnmarshalingRegistriesYAML", func(t *testing.T) {
			// Given: a mock context that returns a valid context
			mockContext := &context.MockContext{
				GetContextFunc: func() (string, error) {
					return "test-context", nil
				},
			}

			// And a mock config handler that returns malformed YAML for registries
			mockConfigHandler := &config.MockConfigHandler{
				GetConfigValueFunc: func(key string) (string, error) {
					if key == "contexts.test-context.docker.registries" {
						return "invalid_yaml: [", nil // Malformed YAML
					}
					return "", fmt.Errorf("key not found: %s", key)
				},
			}

			// Create DI container and register mocks
			diContainer := di.NewContainer()
			diContainer.Register("context", mockContext)
			diContainer.Register("cliConfigHandler", mockConfigHandler)

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

			// When: GetContainerConfig is called
			_, err = helper.GetContainerConfig()

			// Then: it should return an error indicating the failure to unmarshal YAML
			expectedError := "error unmarshaling registries YAML"
			if err == nil || !strings.Contains(err.Error(), expectedError) {
				t.Fatalf("expected error containing %v, got %v", expectedError, err)
			}
		})
	})
}
