package helpers

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
)

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
}

func TestDockerHelper_SetConfig(t *testing.T) {
	t.Run("SetEnabledConfigSuccess", func(t *testing.T) {
		// Given: a mock config handler, shell, and context
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
		if err == nil || err.Error() != expectedError {
			t.Fatalf("expected error %v, got %v", expectedError, err)
		}
	})

	t.Run("ErrorRetrievingDockerEnabledConfig", func(t *testing.T) {
		// Given: a mock config handler that returns an error when retrieving the docker.enabled config
		mockConfigHandler := &config.MockConfigHandler{
			GetConfigValueFunc: func(key string) (string, error) {
				if key == "contexts.test-context.docker.enabled" {
					return "", errors.New("mock error retrieving docker.enabled config")
				}
				return "", nil
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

		helper, err := NewDockerHelper(diContainer)
		if err != nil {
			t.Fatalf("NewDockerHelper() error = %v", err)
		}

		// When: SetConfig is called with "enabled" key
		err = helper.SetConfig("enabled", "true")

		// Then: it should return an error indicating the failure to retrieve the docker.enabled config
		if err == nil || !strings.Contains(err.Error(), "error retrieving docker.enabled config") {
			t.Fatalf("expected error containing 'error retrieving docker.enabled config', got %v", err)
		}
	})

	t.Run("ErrorWritingDockerComposeFile", func(t *testing.T) {
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
		expectedError := "mock error writing file"
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
		mockHelper.GetContainerConfigFunc = func() ([]map[string]interface{}, error) {
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

	t.Run("ErrorMarshalingDockerComposeConfig", func(t *testing.T) {
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

		// Register MockHelper with GetContainerConfigFunc that returns a non-nil config
		expectedConfig := []map[string]interface{}{
			{
				"service1": map[string]interface{}{
					"image": "nginx:latest",
				},
			},
		}
		mockHelper := NewMockHelper(func() (map[string]string, error) {
			return map[string]string{
				"service1": "nginx:latest",
			}, nil
		})
		mockHelper.GetContainerConfigFunc = func() ([]map[string]interface{}, error) {
			return expectedConfig, nil
		}
		diContainer.Register("helper", mockHelper)

		// Create DockerHelper
		helper, err := NewDockerHelper(diContainer)
		if err != nil {
			t.Fatalf("NewDockerHelper() error = %v", err)
		}

		// Mock the yaml.Marshal function to return an error
		originalYamlMarshal := yamlMarshal
		yamlMarshal = func(v interface{}) ([]byte, error) {
			return nil, fmt.Errorf("mock error marshaling YAML")
		}
		defer func() { yamlMarshal = originalYamlMarshal }()

		// When: SetConfig is called with "enabled" key
		err = helper.SetConfig("enabled", "true")

		// Then: it should return an error indicating the failure to marshal the docker-compose config
		expectedError := "error marshaling docker-compose config to YAML: mock error marshaling YAML"
		if err == nil || !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("expected error containing %v, got %v", expectedError, err)
		}
	})

	t.Run("ErrorRetrievingConfigRoot", func(t *testing.T) {
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

	t.Run("AutoEnableDockerForLocalContext", func(t *testing.T) {
		// Given: a mock context that returns "local" and an undefined docker enabled flag
		mockContext := &context.MockContext{
			GetContextFunc: func() (string, error) {
				return "local", nil
			},
		}
		mockConfigHandler := &config.MockConfigHandler{
			GetConfigValueFunc: func(key string) (string, error) {
				if key == "contexts.local.docker.enabled" {
					return "", nil // Simulate undefined flag
				}
				return "", fmt.Errorf("key not found: %s", key)
			},
			SetConfigValueFunc: func(key string, value interface{}) error {
				if key == "contexts.local.docker.enabled" && value != true {
					t.Fatalf("expected docker enabled to be set to 'true', got %v", value)
				}
				return nil
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

		// When: SetConfig is called with "enabled" key and empty value
		err = helper.SetConfig("enabled", "")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
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
		mockHelper.GetContainerConfigFunc = func() ([]map[string]interface{}, error) {
			return []map[string]interface{}{
				{
					"registry.test": map[string]interface{}{
						"image":   registryImage,
						"labels":  map[string]interface{}{"managedBy": "windsor", "role": "registry"},
						"restart": "always",
					},
				},
			}, nil
		}

		// Create DI container and register mocks
		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
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

		// Then: the result should match the expected configuration
		expectedConfig := []map[string]interface{}{
			{
				"registry.test": map[string]interface{}{
					"image":   registryImage,
					"labels":  map[string]interface{}{"managedBy": "windsor", "role": "registry"},
					"restart": "always",
				},
			},
		}
		if !reflect.DeepEqual(fmt.Sprintf("%v", containerConfig), fmt.Sprintf("%v", expectedConfig)) {
			t.Errorf("expected %+v, got %+v", expectedConfig, containerConfig)
		}
	})
}
