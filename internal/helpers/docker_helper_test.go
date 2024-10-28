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
	"github.com/windsor-hotel/cli/internal/shell"
)

// Mock function for yamlMarshal to simulate an error
var originalYamlMarshal = yamlMarshal

func TestDockerHelper_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given: a mock config handler, context, and helper
		mockConfigHandler := config.NewMockConfigHandler()
		mockContext := context.NewMockContext()
		mockHelper := NewMockHelper()

		// Create DI container and register mocks
		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("contextInstance", mockContext)
		diContainer.Register("helper", mockHelper)
		diContainer.Register("shell", shell.NewMockShell("unix"))

		// Create an instance of DockerHelper
		dockerHelper, err := NewDockerHelper(diContainer)
		if err != nil {
			t.Fatalf("NewDockerHelper() error = %v", err)
		}

		// When: Initialize is called
		err = dockerHelper.Initialize()
		if err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// Then: no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}

func TestDockerHelper_NewDockerHelper(t *testing.T) {
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

	t.Run("ErrorResolvingShell", func(t *testing.T) {
		// Create DI container and register cliConfigHandler and context
		diContainer := di.NewContainer()
		mockConfigHandler := config.NewMockConfigHandler()
		mockContext := context.NewMockContext()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("contextInstance", mockContext)

		// Attempt to create DockerHelper
		_, err := NewDockerHelper(diContainer)
		if err == nil || !strings.Contains(err.Error(), "error resolving shell") {
			t.Fatalf("expected error resolving shell, got %v", err)
		}
	})
}

func TestDockerHelper_GetEnvVars(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		tests := []struct {
			name            string
			contextPath     string
			composeFile     string
			expectedEnvVars map[string]string
			expectError     bool
		}{
			{
				name:        "ValidConfigRootWithYaml",
				contextPath: filepath.Join(os.TempDir(), "contexts", "test-context-yaml"),
				composeFile: "compose.yaml",
				expectedEnvVars: map[string]string{
					"COMPOSE_FILE": filepath.Join(os.TempDir(), "contexts", "test-context-yaml", "compose.yaml"),
				},
				expectError: false,
			},
			{
				name:        "ValidConfigRootWithYml",
				contextPath: filepath.Join(os.TempDir(), "contexts", "test-context-yml"),
				composeFile: "compose.yml",
				expectedEnvVars: map[string]string{
					"COMPOSE_FILE": filepath.Join(os.TempDir(), "contexts", "test-context-yml", "compose.yml"),
				},
				expectError: false,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				// Create the directory and compose file
				err := os.MkdirAll(tt.contextPath, os.ModePerm)
				if err != nil {
					t.Fatalf("Failed to create directories: %v", err)
				}
				_, err = os.Create(filepath.Join(tt.contextPath, tt.composeFile))
				if err != nil {
					t.Fatalf("Failed to create %s file: %v", tt.composeFile, err)
				}
				defer os.RemoveAll(tt.contextPath)

				// Mock context
				mockContext := context.NewMockContext()
				mockContext.GetConfigRootFunc = func() (string, error) {
					return tt.contextPath, nil
				}

				// Create DI container and register mocks
				diContainer := di.NewContainer()
				diContainer.Register("cliConfigHandler", config.NewMockConfigHandler())
				diContainer.Register("contextInstance", mockContext)

				// Register MockHelper
				mockHelper := NewMockHelper()
				mockHelper.GetEnvVarsFunc = func() (map[string]string, error) {
					return map[string]string{"COMPOSE_FILE": filepath.Join(tt.contextPath, tt.composeFile)}, nil
				}
				diContainer.Register("helper", mockHelper)

				// Register MockShell
				mockShell := shell.NewMockShell("unix")
				diContainer.Register("shell", mockShell)

				// Create DockerHelper
				dockerHelper, err := NewDockerHelper(diContainer)
				if err != nil {
					t.Fatalf("NewDockerHelper() error = %v", err)
				}

				// When: GetEnvVars is called
				envVars, err := dockerHelper.GetEnvVars()
				if (err != nil) != tt.expectError {
					t.Fatalf("GetEnvVars() error = %v, expectError %v", err, tt.expectError)
				}

				// Then: the environment variables should be set correctly
				if !reflect.DeepEqual(envVars, tt.expectedEnvVars) {
					t.Errorf("expected %v, got %v", tt.expectedEnvVars, envVars)
				}
			})
		}
	})

	t.Run("ErrorRetrievingConfigRoot", func(t *testing.T) {
		// Given: a mock context that returns an error when GetConfigRoot is called
		mockContext := context.NewMockContext()
		mockContext.GetConfigRootFunc = func() (string, error) {
			return "", fmt.Errorf("mock error retrieving config root")
		}

		// Create DI container and register mocks
		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", config.NewMockConfigHandler())
		diContainer.Register("contextInstance", mockContext)

		// Register MockHelper
		mockHelper := NewMockHelper()
		diContainer.Register("helper", mockHelper)

		// Register MockShell
		mockShell := shell.NewMockShell("unix")
		diContainer.Register("shell", mockShell)

		// Create DockerHelper
		dockerHelper, err := NewDockerHelper(diContainer)
		if err != nil {
			t.Fatalf("NewDockerHelper() error = %v", err)
		}

		// When: GetEnvVars is called
		_, err = dockerHelper.GetEnvVars()
		if err == nil || !strings.Contains(err.Error(), "mock error retrieving config root") {
			t.Fatalf("expected error retrieving config root, got %v", err)
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
		diContainer.Register("contextInstance", mockContext)

		// Register MockHelper
		mockHelper := NewMockHelper()
		mockHelper.GetEnvVarsFunc = func() (map[string]string, error) {
			return map[string]string{
				"COMPOSE_FILE": composeFilePath,
			}, nil
		}
		diContainer.Register("helper", mockHelper)

		// Register MockShell
		mockShell := shell.NewMockShell("unix")
		diContainer.Register("shell", mockShell)

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
		diContainer.Register("contextInstance", mockContext)

		// Register MockHelper
		mockHelper := NewMockHelper()
		mockHelper.GetEnvVarsFunc = func() (map[string]string, error) {
			return map[string]string{
				"service1": "nginx:latest",
			}, nil
		}
		diContainer.Register("helper", mockHelper)

		// Register MockShell
		mockShell := shell.NewMockShell("unix")
		diContainer.Register("shell", mockShell)

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

	t.Run("VolumesAndNetworks", func(t *testing.T) {
		// Given: a mock config handler and a mock helper that returns a config with volumes and networks
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Docker: &config.DockerConfig{
					Registries: []config.Registry{},
				},
			}, nil
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
		mockContext.GetConfigRootFunc = func() (string, error) {
			return "", fmt.Errorf("mock error retrieving config root")
		}
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}
		diContainer.Register("contextInstance", mockContext)

		// Register MockShell
		mockShell := shell.NewMockShell("unix")
		diContainer.Register("shell", mockShell)

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

func TestDockerHelper_PostEnvExec(t *testing.T) {
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
		diContainer.Register("contextInstance", mockContext)

		// Register MockHelper
		mockHelper := NewMockHelper()
		mockHelper.GetEnvVarsFunc = func() (map[string]string, error) {
			return map[string]string{
				"service1": "nginx:latest",
			}, nil
		}
		diContainer.Register("helper", mockHelper)

		// Register MockShell
		mockShell := shell.NewMockShell("unix")
		diContainer.Register("shell", mockShell)

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

func TestDockerHelper_GetComposeConfig(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given: a mock config handler, shell, context, and helper
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Docker: &config.DockerConfig{
					Registries: []config.Registry{
						{
							Name:   "registry.test",
							Remote: "registry.remote",
							Local:  "registry.local",
						},
					},
				},
			}, nil
		}

		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}

		// Create DI container and register mocks
		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("contextInstance", mockContext)

		// Register MockHelper
		mockHelper := NewMockHelper()
		mockHelper.GetEnvVarsFunc = func() (map[string]string, error) {
			return map[string]string{
				"service1": "nginx:latest",
			}, nil
		}
		diContainer.Register("helper", mockHelper)

		// Register MockShell
		mockShell := shell.NewMockShell("unix")
		diContainer.Register("shell", mockShell)

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
				"context":    "test-context",
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

	t.Run("ErrorRetrievingRegistries", func(t *testing.T) {
		// Given: a mock context and config handler that returns an error for GetConfig
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}
		mockConfigHandlerWithError := config.NewMockConfigHandler()
		mockConfigHandlerWithError.GetConfigFunc = func() (*config.Context, error) {
			return nil, errors.New("mock error retrieving registries")
		}

		// Create DI container and register mocks
		diContainer := di.NewContainer()
		diContainer.Register("contextInstance", mockContext)
		diContainer.Register("cliConfigHandler", mockConfigHandlerWithError)

		// Register MockHelper
		mockHelper := NewMockHelper()
		mockHelper.GetEnvVarsFunc = func() (map[string]string, error) {
			return map[string]string{
				"service1": "nginx:latest",
			}, nil
		}
		diContainer.Register("helper", mockHelper)

		// Register MockShell
		mockShell := shell.NewMockShell("unix")
		diContainer.Register("shell", mockShell)

		// Create DockerHelper
		helper, err := NewDockerHelper(diContainer)
		if err != nil {
			t.Fatalf("NewDockerHelper() error = %v", err)
		}

		// When: GetComposeConfig is called
		_, err = helper.GetComposeConfig()

		// Then: it should return an error indicating the failure to retrieve registries
		expectedError := "error retrieving context configuration: mock error retrieving registries"
		if err == nil || !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("expected error %v, got %v", expectedError, err)
		}
	})

	// Test error retrieving context
	t.Run("ErrorRetrievingContext", func(t *testing.T) {
		// Given: a mock context and config handler that returns an error for GetContext
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "", errors.New("mock error retrieving context")
		}
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Docker: &config.DockerConfig{
					Registries: []config.Registry{
						{Name: "registry1"},
					},
				},
			}, nil
		}

		// Create DI container and register mocks
		diContainer := di.NewContainer()
		diContainer.Register("contextInstance", mockContext)
		diContainer.Register("cliConfigHandler", mockConfigHandler)

		// Register MockHelper
		mockHelper := NewMockHelper()
		mockHelper.GetComposeConfigFunc = func() (*types.Config, error) {
			return &types.Config{
				Services: []types.ServiceConfig{
					{Name: "registry1"},
				},
			}, nil
		}
		diContainer.Register("helper", mockHelper)

		// Register MockShell
		mockShell := shell.NewMockShell("unix")
		diContainer.Register("shell", mockShell)

		// Create DockerHelper
		helper, err := NewDockerHelper(diContainer)
		if err != nil {
			t.Fatalf("NewDockerHelper() error = %v", err)
		}

		// When: GetComposeConfig is called
		_, err = helper.GetComposeConfig()

		// Then: it should return an error indicating the failure to retrieve context
		expectedError := "error retrieving context: mock error retrieving context"
		if err == nil || !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("expected error %v, got %v", expectedError, err)
		}
	})
}

func TestDockerHelper_WriteConfig(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given: a mock config handler and context
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Docker: &config.DockerConfig{
					Registries: []config.Registry{
						{
							Name:   "registry1",
							Local:  "http://localhost:5000",
							Remote: "https://registry1.example.com",
						},
					},
				},
			}, nil
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
		diContainer.Register("contextInstance", mockContext)

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

		// Register MockShell
		mockShell := shell.NewMockShell("unix")
		diContainer.Register("shell", mockShell)

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
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Docker: &config.DockerConfig{
					Registries: []config.Registry{},
				},
			}, nil
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
		diContainer.Register("contextInstance", mockContext)

		// Register MockHelper
		mockHelper := NewMockHelper()
		mockHelper.GetEnvVarsFunc = func() (map[string]string, error) {
			return map[string]string{
				"service1": "nginx:latest",
			}, nil
		}
		diContainer.Register("helper", mockHelper)

		// Register MockShell
		mockShell := shell.NewMockShell("unix")
		diContainer.Register("shell", mockShell)

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
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Docker: &config.DockerConfig{
					Registries: []config.Registry{},
				},
			}, nil
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
		diContainer.Register("contextInstance", mockContext)

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

		// Register MockShell
		mockShell := shell.NewMockShell("unix")
		diContainer.Register("shell", mockShell)

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
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Docker: &config.DockerConfig{
					Registries: []config.Registry{},
				},
			}, nil
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
		diContainer.Register("contextInstance", mockContext)

		// Register MockHelper
		mockHelper := NewMockHelper()
		mockHelper.GetEnvVarsFunc = func() (map[string]string, error) {
			return map[string]string{
				"service1": "nginx:latest",
			}, nil
		}
		diContainer.Register("helper", mockHelper)

		// Register MockShell
		mockShell := shell.NewMockShell("unix")
		diContainer.Register("shell", mockShell)

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
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Docker: &config.DockerConfig{
					Registries: []config.Registry{},
				},
			}, nil
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
		diContainer.Register("contextInstance", mockContext)

		// Register MockHelper
		mockHelper := NewMockHelper()
		mockHelper.GetEnvVarsFunc = func() (map[string]string, error) {
			return map[string]string{
				"service1": "nginx:latest",
			}, nil
		}
		diContainer.Register("helper", mockHelper)

		// Register MockShell
		mockShell := shell.NewMockShell("unix")
		diContainer.Register("shell", mockShell)

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
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Docker: &config.DockerConfig{
					Registries: []config.Registry{},
				},
			}, nil
		}

		// And a DI container with the mock context and config handler registered
		diContainer := di.NewContainer()
		diContainer.Register("contextInstance", mockContext)
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

		// Register MockShell
		mockShell := shell.NewMockShell("unix")
		diContainer.Register("shell", mockShell)

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
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Docker: &config.DockerConfig{
					Enabled:    ptrBool(true),       // Docker is enabled
					Registries: []config.Registry{}, // No registries defined
				},
			}, nil
		}

		// Mock the ContextInterface
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "test", nil
		}

		// Register mocks in the DI container
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("contextInstance", mockContext)

		// Register MockHelper to avoid error resolving helpers
		mockHelper := NewMockHelper()
		mockHelper.GetEnvVarsFunc = func() (map[string]string, error) {
			return map[string]string{
				"service1": "nginx:latest",
			}, nil
		}
		diContainer.Register("helper", mockHelper)

		// Register MockShell
		mockShell := shell.NewMockShell("unix")
		diContainer.Register("shell", mockShell)

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
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Docker: &config.DockerConfig{
					Registries: []config.Registry{},
				},
			}, nil
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
		diContainer.Register("contextInstance", mockContext)
		// Register MockHelper
		mockHelper := NewMockHelper()
		mockHelper.GetEnvVarsFunc = func() (map[string]string, error) {
			return map[string]string{
				"service1": "nginx:latest",
			}, nil
		}
		diContainer.Register("helper", mockHelper)

		// Register MockShell
		mockShell := shell.NewMockShell("unix")
		diContainer.Register("shell", mockShell)

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

	t.Run("VolumesAndNetworks", func(t *testing.T) {
		// Mock context and config handler
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "test-config-root", nil
		}

		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Docker: &config.DockerConfig{
					Registries: []config.Registry{},
				},
			}, nil
		}

		// Mock helper that returns container configs with volumes and networks
		mockHelper := NewMockHelper()
		mockHelper.GetComposeConfigFunc = func() (*types.Config, error) {
			return &types.Config{
				Volumes: map[string]types.VolumeConfig{
					"volume1": {Driver: "local"},
				},
				Networks: map[string]types.NetworkConfig{
					"network1": {Driver: "bridge"},
				},
			}, nil
		}

		// Create DI container and register mocks
		diContainer := createDIContainer(mockContext, mockConfigHandler)
		diContainer.Register("mockHelper", mockHelper)

		// Register MockShell
		mockShell := shell.NewMockShell("unix")
		diContainer.Register("shell", mockShell)

		// Create DockerHelper
		helper, err := NewDockerHelper(diContainer)
		if err != nil {
			t.Fatalf("NewDockerHelper() error = %v", err)
		}

		// Mock file operations
		originalWriteFile := writeFile
		writeFile = func(filename string, data []byte, perm os.FileMode) error {
			return nil
		}
		defer func() { writeFile = originalWriteFile }()

		originalMkdirAll := mkdirAll
		mkdirAll = func(path string, perm os.FileMode) error {
			return nil
		}
		defer func() { mkdirAll = originalMkdirAll }()

		// Execute WriteConfig
		err = helper.WriteConfig()
		if err == nil || !strings.Contains(err.Error(), "error retrieving config root: GetConfigRootFunc not implemented") {
			t.Fatalf("expected error containing 'error retrieving config root: GetConfigRootFunc not implemented', got %v", err)
		}
	})

	t.Run("ErrorRetrievingContext", func(t *testing.T) {
		// Given: a mock config handler and context that returns an error for GetContext
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Docker: &config.DockerConfig{
					Registries: []config.Registry{},
				},
			}, nil
		}
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "", fmt.Errorf("mock error retrieving context")
		}
		mockContext.GetConfigRootFunc = func() (string, error) {
			return filepath.Join(os.TempDir(), "contexts", "test-context"), nil
		}

		// Create DI container and register mocks
		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("contextInstance", mockContext)

		// Register MockHelper
		mockHelper := NewMockHelper()
		mockHelper.GetEnvVarsFunc = func() (map[string]string, error) {
			return map[string]string{
				"service1": "nginx:latest",
			}, nil
		}
		diContainer.Register("helper", mockHelper)

		// Register MockShell
		mockShell := shell.NewMockShell("unix")
		diContainer.Register("shell", mockShell)

		// Create DockerHelper
		helper, err := NewDockerHelper(diContainer)
		if err != nil {
			t.Fatalf("NewDockerHelper() error = %v", err)
		}

		// When: WriteConfig is called
		err = helper.WriteConfig()

		// Then: it should return an error indicating the failure to retrieve the context
		expectedError := "error retrieving context: mock error retrieving context"
		if err == nil || !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("expected error %v, got %v", expectedError, err)
		}
	})

	t.Run("ErrorRetrievingContextConfig", func(t *testing.T) {
		// Given: a mock config handler that returns an error
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return nil, fmt.Errorf("mock error retrieving context configuration")
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
		diContainer.Register("contextInstance", mockContext)

		// Register MockHelper
		mockHelper := NewMockHelper()
		mockHelper.GetEnvVarsFunc = func() (map[string]string, error) {
			return map[string]string{
				"service1": "nginx:latest",
			}, nil
		}
		diContainer.Register("helper", mockHelper)

		// Register MockShell
		mockShell := shell.NewMockShell("unix")
		diContainer.Register("shell", mockShell)

		// Create DockerHelper
		helper, err := NewDockerHelper(diContainer)
		if err != nil {
			t.Fatalf("NewDockerHelper() error = %v", err)
		}

		// When: WriteConfig is called
		err = helper.WriteConfig()

		// Then: it should return an error indicating the failure to retrieve the context configuration
		expectedError := "error retrieving context configuration: mock error retrieving context configuration"
		if err == nil || !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("expected error %v, got %v", expectedError, err)
		}
	})

	t.Run("DockerNotDefined", func(t *testing.T) {
		// Given: a mock config handler with Docker set to nil
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Docker: nil, // Docker is not defined
			}, nil
		}
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}
		mockContext.GetConfigRootFunc = func() (string, error) {
			return "", fmt.Errorf("GetConfigRootFunc not implemented")
		}

		// Create DI container and register mocks
		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("contextInstance", mockContext)

		// Register MockHelper
		mockHelper := NewMockHelper()
		diContainer.Register("helper", mockHelper)

		// Register MockShell
		mockShell := shell.NewMockShell("unix")
		diContainer.Register("shell", mockShell)

		// Create DockerHelper
		helper, err := NewDockerHelper(diContainer)
		if err != nil {
			t.Fatalf("NewDockerHelper() error = %v", err)
		}

		// When: WriteConfig is called
		err = helper.WriteConfig()

		// Then: it should return an error indicating the failure to retrieve the config root
		expectedError := "error retrieving config root: GetConfigRootFunc not implemented"
		if err == nil || !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("expected error %v, got %v", expectedError, err)
		}
	})

	t.Run("AssignIPAddresses_ValidCIDR", func(t *testing.T) {
		// Given: a mock config handler with a valid NetworkCIDR
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Docker: &config.DockerConfig{
					NetworkCIDR: ptrString("192.168.1.0/24"),
					Registries: []config.Registry{
						{Name: "registry1"},
						{Name: "registry2"},
					},
				},
			}, nil
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
		diContainer.Register("contextInstance", mockContext)

		// Register MockHelper
		mockHelper := NewMockHelper()
		mockHelper.GetComposeConfigFunc = func() (*types.Config, error) {
			return &types.Config{
				Services: []types.ServiceConfig{
					{Name: "registry1"},
					{Name: "registry2"},
				},
			}, nil
		}
		diContainer.Register("helper", mockHelper)

		// Register MockShell
		mockShell := shell.NewMockShell("unix")
		diContainer.Register("shell", mockShell)

		// Create DockerHelper
		helper, err := NewDockerHelper(diContainer)
		if err != nil {
			t.Fatalf("NewDockerHelper() error = %v", err)
		}

		// When: WriteConfig is called
		err = helper.WriteConfig()

		// Then: it should not return an error
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("AssignIPAddresses_InvalidCIDR", func(t *testing.T) {
		// Given: a mock config handler with an invalid NetworkCIDR
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Docker: &config.DockerConfig{
					NetworkCIDR: ptrString("invalid-cidr"),
				},
			}, nil
		}
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}

		// Create DI container and register mocks
		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("contextInstance", mockContext)

		// Register MockHelper
		mockHelper := NewMockHelper()
		diContainer.Register("helper", mockHelper)

		// Register MockShell
		mockShell := shell.NewMockShell("unix")
		diContainer.Register("shell", mockShell)

		// Create DockerHelper
		helper, err := NewDockerHelper(diContainer)
		if err != nil {
			t.Fatalf("NewDockerHelper() error = %v", err)
		}

		// When: WriteConfig is called
		err = helper.WriteConfig()

		// Then: it should return an error indicating the failure to parse the CIDR
		expectedError := "error parsing network CIDR"
		if err == nil || !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("expected error containing %v, got %v", expectedError, err)
		}
	})

	t.Run("AssignIPAddresses_InsufficientIPs", func(t *testing.T) {
		// Given: a mock config handler with a small NetworkCIDR
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Docker: &config.DockerConfig{
					NetworkCIDR: ptrString("192.168.1.0/31"), // Insufficient IPs for two services
					Registries: []config.Registry{
						{Name: "registry1"},
						{Name: "registry2"},
					},
				},
			}, nil
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
		diContainer.Register("contextInstance", mockContext)

		// Register MockHelper
		mockHelper := NewMockHelper()
		mockHelper.GetComposeConfigFunc = func() (*types.Config, error) {
			return &types.Config{
				Services: []types.ServiceConfig{
					{Name: "registry1"},
					{Name: "registry2"},
				},
			}, nil
		}
		diContainer.Register("helper", mockHelper)

		// Register MockShell
		mockShell := shell.NewMockShell("unix")
		diContainer.Register("shell", mockShell)

		// Create DockerHelper
		helper, err := NewDockerHelper(diContainer)
		if err != nil {
			t.Fatalf("NewDockerHelper() error = %v", err)
		}

		// When: WriteConfig is called
		err = helper.WriteConfig()

		// Then: it should return an error indicating not enough IP addresses
		expectedError := "not enough IP addresses in the CIDR range"
		if err == nil || !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("expected error containing %v, got %v", expectedError, err)
		}
	})

	t.Run("ErrorNotEnoughIPAddresses", func(t *testing.T) {
		// Given: a mock config handler and context with a CIDR that has insufficient IPs
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Docker: &config.DockerConfig{
					NetworkCIDR: ptrString("192.168.1.0/31"), // Insufficient IPs for two services
					Registries: []config.Registry{
						{Name: "registry1"},
						{Name: "registry2"},
					},
				},
			}, nil
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
		diContainer.Register("contextInstance", mockContext)

		// Register MockHelper
		mockHelper := NewMockHelper()
		mockHelper.GetComposeConfigFunc = func() (*types.Config, error) {
			return &types.Config{
				Services: []types.ServiceConfig{
					{Name: "registry1"},
					{Name: "registry2"},
				},
			}, nil
		}
		diContainer.Register("helper", mockHelper)

		// Register MockShell
		mockShell := shell.NewMockShell("unix")
		diContainer.Register("shell", mockShell)

		// Create DockerHelper
		helper, err := NewDockerHelper(diContainer)
		if err != nil {
			t.Fatalf("NewDockerHelper() error = %v", err)
		}

		// When: WriteConfig is called
		err = helper.WriteConfig()

		// Then: it should return an error indicating not enough IP addresses
		expectedError := "not enough IP addresses in the CIDR range"
		if err == nil || !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("expected error containing %v, got %v", expectedError, err)
		}
	})

	t.Run("ErrorResolvingHelpers", func(t *testing.T) {
		// Given: a mock config handler, context, and shell
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Docker: &config.DockerConfig{
					Registries: []config.Registry{},
				},
			}, nil
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
		diContainer.Register("contextInstance", mockContext)

		// Register MockShell
		mockShell := shell.NewMockShell("unix")
		diContainer.Register("shell", mockShell)

		// Create mock DI container and set the ResolveAllError to simulate an error
		mockDIContainer := di.NewMockContainer()
		mockDIContainer.SetResolveAllError(errors.New("no instances found for the given type"))

		// Create DockerHelper
		helper, err := NewDockerHelper(diContainer)
		if err != nil {
			t.Fatalf("NewDockerHelper() error = %v", err)
		}

		// Inject the mock DI container into the DockerHelper
		helper.DIContainer = mockDIContainer.DIContainer

		// When: WriteConfig is called
		err = helper.WriteConfig()

		// Then: it should return an error indicating the failure to resolve helpers
		expectedError := "error resolving helpers: no instances found for the given type"
		if err == nil || !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("expected error containing %v, got %v", expectedError, err)
		}
	})
}

func TestDockerHelper_Up(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Create DI container and register mocks
		diContainer := di.NewContainer()
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			enabled := true
			return &config.Context{
				Docker: &config.DockerConfig{
					Enabled: &enabled,
				},
			}, nil
		}
		mockContext := context.NewMockContext()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("contextInstance", mockContext)

		// Register MockShell
		mockShell := shell.NewMockShell("unix")
		mockShell.ExecFunc = func(verbose bool, message string, command string, args ...string) (string, error) {
			return "Mocked output", nil
		}
		diContainer.Register("shell", mockShell)

		// Mock checkDockerDaemon
		mockShell.ExecFunc = func(verbose bool, message string, command string, args ...string) (string, error) {
			if command == "docker" && args[0] == "info" {
				return "Docker daemon is running", nil
			}
			return "Mocked output", nil
		}

		// Create an instance of DockerHelper
		dockerHelper, err := NewDockerHelper(diContainer)
		if err != nil {
			t.Fatalf("NewDockerHelper() error = %v", err)
		}

		// When: Up is called
		err = dockerHelper.Up()
		if err != nil {
			t.Fatalf("Up() error = %v", err)
		}
	})

	t.Run("VerboseFlag", func(t *testing.T) {
		// Create DI container and register mocks
		diContainer := di.NewContainer()
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			enabled := true
			return &config.Context{
				Docker: &config.DockerConfig{
					Enabled: &enabled,
				},
			}, nil
		}
		mockContext := context.NewMockContext()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("contextInstance", mockContext)

		// Register MockShell
		mockShell := shell.NewMockShell("unix")
		mockShell.ExecFunc = func(verbose bool, message string, command string, args ...string) (string, error) {
			if verbose {
				return "Verbose output", nil
			}
			return "Mocked output", nil
		}
		diContainer.Register("shell", mockShell)

		// Mock checkDockerDaemon
		mockShell.ExecFunc = func(verbose bool, message string, command string, args ...string) (string, error) {
			if command == "docker" && args[0] == "info" {
				return "Docker daemon is running", nil
			}
			return "Mocked output", nil
		}

		// Create an instance of DockerHelper
		dockerHelper, err := NewDockerHelper(diContainer)
		if err != nil {
			t.Fatalf("NewDockerHelper() error = %v", err)
		}

		// When: Up is called with verbose flag set to true
		err = dockerHelper.Up(true)
		if err != nil {
			t.Fatalf("Up() error = %v", err)
		}
	})

	t.Run("ErrorRetrievingConfig", func(t *testing.T) {
		// Create DI container and register mocks
		diContainer := di.NewContainer()
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return nil, fmt.Errorf("error retrieving context configuration")
		}
		mockContext := context.NewMockContext()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("contextInstance", mockContext)

		// Register MockShell
		mockShell := shell.NewMockShell("unix")
		diContainer.Register("shell", mockShell)

		// Create an instance of DockerHelper
		dockerHelper, err := NewDockerHelper(diContainer)
		if err != nil {
			t.Fatalf("NewDockerHelper() error = %v", err)
		}

		// When: Up is called
		err = dockerHelper.Up()
		if err == nil || !strings.Contains(err.Error(), "error retrieving context configuration") {
			t.Fatalf("expected error retrieving context configuration, got %v", err)
		}
	})

	t.Run("ErrorCheckingDockerDaemon", func(t *testing.T) {
		// Create DI container and register mocks
		diContainer := di.NewContainer()
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Docker: &config.DockerConfig{
					Enabled: func(b bool) *bool { return &b }(true),
				},
			}, nil
		}
		mockContext := context.NewMockContext()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("contextInstance", mockContext)

		// Register MockShell with a failing docker info command
		mockShell := shell.NewMockShell("unix")
		mockShell.ExecFunc = func(verbose bool, message string, command string, args ...string) (string, error) {
			if command == "docker" && args[0] == "info" {
				return "", fmt.Errorf("Docker daemon is not running")
			}
			return "Mocked output", nil
		}
		diContainer.Register("shell", mockShell)

		// Create an instance of DockerHelper
		dockerHelper, err := NewDockerHelper(diContainer)
		if err != nil {
			t.Fatalf("NewDockerHelper() error = %v", err)
		}

		// When: Up is called
		err = dockerHelper.Up()
		if err == nil || !strings.Contains(err.Error(), "Docker daemon is not running") {
			t.Fatalf("expected error checking Docker daemon, got %v", err)
		}
	})

	t.Run("ErrorExecutingDockerComposeUp", func(t *testing.T) {
		// Create DI container and register mocks
		diContainer := di.NewContainer()
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Docker: &config.DockerConfig{
					Enabled: func(b bool) *bool { return &b }(true),
				},
			}, nil
		}
		mockContext := context.NewMockContext()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("contextInstance", mockContext)

		// Register MockShell with a failing docker-compose up command
		mockShell := shell.NewMockShell("unix")
		mockShell.ExecFunc = func(verbose bool, message string, command string, args ...string) (string, error) {
			if command == "docker-compose" && args[0] == "up" {
				return "Mocked output", fmt.Errorf("mock error executing docker-compose up")
			}
			return "Mocked output", nil
		}
		diContainer.Register("shell", mockShell)

		// Create an instance of DockerHelper
		dockerHelper, err := NewDockerHelper(diContainer)
		if err != nil {
			t.Fatalf("NewDockerHelper() error = %v", err)
		}

		// When: Up is called
		err = dockerHelper.Up()
		if err == nil || !strings.Contains(err.Error(), "mock error executing docker-compose up") {
			t.Fatalf("expected error executing docker-compose up, got %v", err)
		}
	})

	t.Run("SuccessfulDockerComposeUp", func(t *testing.T) {
		// Create DI container and register mocks
		diContainer := di.NewContainer()
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Docker: &config.DockerConfig{
					Enabled: func(b bool) *bool { return &b }(true),
				},
			}, nil
		}
		mockContext := context.NewMockContext()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("contextInstance", mockContext)

		// Register MockShell with a successful docker-compose up command
		mockShell := shell.NewMockShell("unix")
		mockShell.ExecFunc = func(verbose bool, message string, command string, args ...string) (string, error) {
			if command == "docker-compose" && args[0] == "up" {
				return "Mocked output", nil
			}
			return "Mocked output", nil
		}
		diContainer.Register("shell", mockShell)

		// Create an instance of DockerHelper
		dockerHelper, err := NewDockerHelper(diContainer)
		if err != nil {
			t.Fatalf("NewDockerHelper() error = %v", err)
		}

		// When: Up is called
		err = dockerHelper.Up()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestDockerHelper_Info(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Create DI container and register mocks
		diContainer := di.NewContainer()
		mockConfigHandler := config.NewMockConfigHandler()
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}
		mockShell := shell.NewMockShell("unix")
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("contextInstance", mockContext)
		diContainer.Register("shell", mockShell)

		// Mock the shell.Exec function to return container IDs and labels
		mockShell.ExecFunc = func(verbose bool, message string, command string, args ...string) (string, error) {
			if command == "docker" && args[0] == "ps" {
				return "container1\ncontainer2", nil
			}
			if command == "docker" && args[0] == "inspect" {
				if args[1] == "container1" {
					return `{"role": "web", "com.docker.compose.service": "service1"}`, nil
				}
				if args[1] == "container2" {
					return `{"role": "db", "com.docker.compose.service": "service2"}`, nil
				}
			}
			return "", nil
		}

		// Create an instance of DockerHelper
		dockerHelper, err := NewDockerHelper(diContainer)
		if err != nil {
			t.Fatalf("NewDockerHelper() error = %v", err)
		}

		// When: Info is called
		info, err := dockerHelper.Info()
		if err != nil {
			t.Fatalf("Info() error = %v", err)
		}

		// Then: no error should be returned and info should contain the expected data
		expectedInfo := &DockerInfo{
			Services: map[string][]string{
				"web": {"service1"},
				"db":  {"service2"},
			},
		}
		if !reflect.DeepEqual(info, expectedInfo) {
			t.Errorf("Expected info to be %v, got %v", expectedInfo, info)
		}
	})

	t.Run("NoContext", func(t *testing.T) {
		// Create DI container and register mocks
		diContainer := di.NewContainer()
		mockConfigHandler := config.NewMockConfigHandler()
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "", fmt.Errorf("error resolving context: no instance registered with name context")
		}
		mockShell := shell.NewMockShell("unix")
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("contextInstance", mockContext)
		diContainer.Register("shell", mockShell)

		// Create an instance of DockerHelper
		dockerHelper, err := NewDockerHelper(diContainer)
		if err != nil {
			t.Fatalf("NewDockerHelper() error = %v", err)
		}

		// When: Info is called
		_, err = dockerHelper.Info()
		if err == nil || !strings.Contains(err.Error(), "error resolving context: no instance registered with name context") {
			t.Fatalf("Expected error resolving context, got %v", err)
		}
	})

	t.Run("NoContainers", func(t *testing.T) {
		// Create DI container and register mocks
		diContainer := di.NewContainer()
		mockConfigHandler := config.NewMockConfigHandler()
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}
		mockShell := shell.NewMockShell("unix")
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("contextInstance", mockContext)
		diContainer.Register("shell", mockShell)

		// Mock the shell.Exec function to return no container IDs
		mockShell.ExecFunc = func(verbose bool, message string, command string, args ...string) (string, error) {
			if command == "docker" && args[0] == "ps" {
				return "", nil
			}
			return "", nil
		}

		// Create an instance of DockerHelper
		dockerHelper, err := NewDockerHelper(diContainer)
		if err != nil {
			t.Fatalf("NewDockerHelper() error = %v", err)
		}

		// When: Info is called
		info, err := dockerHelper.Info()
		if err != nil {
			t.Fatalf("Info() error = %v", err)
		}

		// Then: no error should be returned and info should be empty
		expectedInfo := &DockerInfo{
			Services: map[string][]string{},
		}
		if !reflect.DeepEqual(info, expectedInfo) {
			t.Errorf("Expected info to be %v, got %v", expectedInfo, info)
		}
	})

	t.Run("ErrorFetchingContainerIDs", func(t *testing.T) {
		// Create DI container and register mocks
		diContainer := di.NewContainer()
		mockConfigHandler := config.NewMockConfigHandler()
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}
		mockShell := shell.NewMockShell("unix")
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("contextInstance", mockContext)
		diContainer.Register("shell", mockShell)

		// Mock the shell.Exec function to return an error when fetching container IDs
		mockShell.ExecFunc = func(verbose bool, message string, command string, args ...string) (string, error) {
			if command == "docker" && args[0] == "ps" {
				return "", fmt.Errorf("mock error fetching container IDs")
			}
			return "", nil
		}

		// Create an instance of DockerHelper
		dockerHelper, err := NewDockerHelper(diContainer)
		if err != nil {
			t.Fatalf("NewDockerHelper() error = %v", err)
		}

		// When: Info is called
		_, err = dockerHelper.Info()
		if err == nil || !strings.Contains(err.Error(), "mock error fetching container IDs") {
			t.Fatalf("Expected error fetching container IDs, got %v", err)
		}
	})

	t.Run("ErrorInspectingContainer", func(t *testing.T) {
		// Create DI container and register mocks
		diContainer := di.NewContainer()
		mockConfigHandler := config.NewMockConfigHandler()
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}
		mockShell := shell.NewMockShell("unix")
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("contextInstance", mockContext)
		diContainer.Register("shell", mockShell)

		// Mock the shell.Exec function to return an error when inspecting a container
		mockShell.ExecFunc = func(verbose bool, message string, command string, args ...string) (string, error) {
			if command == "docker" && args[0] == "ps" {
				return "container1", nil
			}
			if command == "docker" && args[0] == "inspect" {
				return "", fmt.Errorf("mock error inspecting container")
			}
			return "", nil
		}

		// Create an instance of DockerHelper
		dockerHelper, err := NewDockerHelper(diContainer)
		if err != nil {
			t.Fatalf("NewDockerHelper() error = %v", err)
		}

		// When: Info is called
		_, err = dockerHelper.Info()
		if err == nil || !strings.Contains(err.Error(), "mock error inspecting container") {
			t.Fatalf("Expected error inspecting container, got %v", err)
		}
	})

	t.Run("ErrorUnmarshallingLabels", func(t *testing.T) {
		// Create DI container and register mocks
		diContainer := di.NewContainer()
		mockConfigHandler := config.NewMockConfigHandler()
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}
		mockShell := shell.NewMockShell("unix")
		mockShell.ExecFunc = func(verbose bool, message string, command string, args ...string) (string, error) {
			if command == "docker" && args[0] == "ps" {
				return "container1", nil
			}
			if command == "docker" && args[0] == "inspect" {
				return "invalid json", nil
			}
			return "", nil
		}
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("contextInstance", mockContext)
		diContainer.Register("shell", mockShell)

		// Create an instance of DockerHelper
		dockerHelper, err := NewDockerHelper(diContainer)
		if err != nil {
			t.Fatalf("NewDockerHelper() error = %v", err)
		}

		// When: Info is called
		_, err = dockerHelper.Info()
		if err == nil || !strings.Contains(err.Error(), "invalid character") {
			t.Fatalf("Expected error unmarshalling labels, got %v", err)
		}
	})

	t.Run("NoRoleLabel", func(t *testing.T) {
		// Create DI container and register mocks
		diContainer := di.NewContainer()
		mockConfigHandler := config.NewMockConfigHandler()
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}
		mockShell := shell.NewMockShell("unix")
		mockShell.ExecFunc = func(verbose bool, message string, command string, args ...string) (string, error) {
			if command == "docker" && args[0] == "ps" {
				return "container1", nil
			}
			if command == "docker" && args[0] == "inspect" {
				return `{"com.docker.compose.service": "test-service"}`, nil
			}
			return "", nil
		}
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("contextInstance", mockContext)
		diContainer.Register("shell", mockShell)

		// Create an instance of DockerHelper
		dockerHelper, err := NewDockerHelper(diContainer)
		if err != nil {
			t.Fatalf("NewDockerHelper() error = %v", err)
		}

		// When: Info is called
		info, err := dockerHelper.Info()
		if err != nil {
			t.Fatalf("Info() error = %v", err)
		}

		// Then: it should not include the container in the result
		dockerInfo, ok := info.(*DockerInfo)
		if !ok {
			t.Fatalf("Expected *DockerInfo, got %T", info)
		}
		if len(dockerInfo.Services) != 0 {
			t.Fatalf("Expected no services, got %v", dockerInfo.Services)
		}
	})

	t.Run("NoServiceLabel", func(t *testing.T) {
		// Create DI container and register mocks
		diContainer := di.NewContainer()
		mockConfigHandler := config.NewMockConfigHandler()
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}
		mockShell := shell.NewMockShell("unix")
		mockShell.ExecFunc = func(verbose bool, message string, command string, args ...string) (string, error) {
			if command == "docker" && args[0] == "ps" {
				return "container1", nil
			}
			if command == "docker" && args[0] == "inspect" {
				return `{"role": "test-role"}`, nil
			}
			return "", nil
		}
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("contextInstance", mockContext)
		diContainer.Register("shell", mockShell)

		// Create an instance of DockerHelper
		dockerHelper, err := NewDockerHelper(diContainer)
		if err != nil {
			t.Fatalf("NewDockerHelper() error = %v", err)
		}

		// When: Info is called
		info, err := dockerHelper.Info()
		if err != nil {
			t.Fatalf("Info() error = %v", err)
		}

		// Then: it should not include the container in the result
		dockerInfo, ok := info.(*DockerInfo)
		if !ok {
			t.Fatalf("Expected *DockerInfo, got %T", info)
		}
		if len(dockerInfo.Services) != 0 {
			t.Fatalf("Expected no services, got %v", dockerInfo.Services)
		}
	})
}
