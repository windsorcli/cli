package virt

import (
	"fmt"
	"strings"
	"testing"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/shell"
)

func setupSafeDockerContainerMocks(optionalContainer ...di.ContainerInterface) *MockComponents {
	var container di.ContainerInterface
	if len(optionalContainer) > 0 {
		container = optionalContainer[0]
	} else {
		container = di.NewContainer()
	}

	mockContext := context.NewMockContext()
	mockShell := shell.NewMockShell()
	mockConfigHandler := config.NewMockConfigHandler()

	// Register mock instances in the container
	container.Register("contextHandler", mockContext)
	container.Register("shell", mockShell)
	container.Register("cliConfigHandler", mockConfigHandler)

	// Implement GetContextFunc on mock context
	mockContext.GetContextFunc = func() (string, error) {
		return "default-context", nil
	}

	// Set up the mock config handler to return a safe default configuration for Docker VMs
	mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
		return &config.Context{
			Docker: &config.DockerConfig{
				Enabled: ptrBool(true),
				Registries: []config.Registry{
					{
						Name:   "registry.test",
						Remote: "https://registry.test",
						Local:  "https://local.registry.test",
					},
				},
				NetworkCIDR: ptrString("10.1.0.0/16"),
			},
		}, nil
	}

	// Mock the shell Exec function to return generic JSON structures
	mockShell.ExecFunc = func(verbose bool, description string, command string, args ...string) (string, error) {
		if command == "docker" && len(args) > 0 {
			switch args[0] {
			case "ps":
				return "container1", nil
			case "inspect":
				if len(args) > 3 && args[2] == "--format" {
					switch args[3] {
					case "{{json .Config.Labels}}":
						return `{"com.docker.compose.service":"service1","managed_by":"windsor","context":"default-context"}`, nil
					case "{{json .NetworkSettings.Networks}}":
						return `{"bridge":{"IPAddress":"192.168.1.2"}}`, nil
					}
				}
			}
		}
		return "", fmt.Errorf("unknown command")
	}

	return &MockComponents{
		Container:         container,
		MockContext:       mockContext,
		MockShell:         mockShell,
		MockConfigHandler: mockConfigHandler,
	}
}

func TestDockerVirt_Up(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Stub test for successful Up
	})

	t.Run("ErrorConfiguringDocker", func(t *testing.T) {
		// Stub test for error during Docker configuration
	})

	t.Run("ErrorStartingColima", func(t *testing.T) {
		// Stub test for error starting Colima
	})
}

func TestDockerVirt_Down(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Stub test for successful Down
	})

	t.Run("Error", func(t *testing.T) {
		// Stub test for error during Down
	})
}

func TestDockerVirt_GetContainerInfo(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeDockerContainerMocks()
		dockerVirt := NewDockerVirt(mocks.Container)

		// Mock the necessary methods
		mocks.MockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}

		// Mock the shell Exec function to simulate successful docker ps and label inspection for two containers
		mocks.MockShell.ExecFunc = func(verbose bool, description string, command string, args ...string) (string, error) {
			if command == "docker" {
				if len(args) > 0 && args[0] == "ps" {
					return "container1\ncontainer2", nil // Simulate successful docker ps with two containers
				}
				if len(args) > 0 && args[0] == "inspect" && strings.Contains(description, "Inspecting container") {
					switch args[3] {
					case "{{json .Config.Labels}}":
						if args[1] == "container1" {
							return `{"com.docker.compose.service":"service1","managed_by":"windsor","context":"test-context"}`, nil
						} else if args[1] == "container2" {
							return `{"com.docker.compose.service":"service2","managed_by":"windsor","context":"test-context"}`, nil
						}
					case "{{json .NetworkSettings.Networks}}":
						if args[1] == "container1" {
							return `{"bridge":{"IPAddress":"192.168.1.2"}}`, nil
						} else if args[1] == "container2" {
							return `{"bridge":{"IPAddress":"192.168.1.3"}}`, nil
						}
					}
				}
			}
			return "", fmt.Errorf("unknown command")
		}

		// When calling GetContainerInfo
		containerInfo, err := dockerVirt.GetContainerInfo()

		// Then no error should be returned and container info should be as expected
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(containerInfo) != 2 {
			t.Fatalf("Expected two container infos, got %d", len(containerInfo))
		}
		if containerInfo[0].Address != "192.168.1.2" {
			t.Fatalf("Expected container1 IP '192.168.1.2', got %s", containerInfo[0].Address)
		}
		if containerInfo[0].Name != "service1" {
			t.Fatalf("Expected container1 name 'service1', got %s", containerInfo[0].Name)
		}
		if containerInfo[1].Address != "192.168.1.3" {
			t.Fatalf("Expected container2 IP '192.168.1.3', got %s", containerInfo[1].Address)
		}
		if containerInfo[1].Name != "service2" {
			t.Fatalf("Expected container2 name 'service2', got %s", containerInfo[1].Name)
		}
	})

	t.Run("Error", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeDockerContainerMocks()
		dockerVirt := NewDockerVirt(mocks.Container)

		// Mock the necessary methods to simulate an error
		mocks.MockContext.GetContextFunc = func() (string, error) {
			return "", fmt.Errorf("mock error retrieving context")
		}

		// When calling GetContainerInfo
		_, err := dockerVirt.GetContainerInfo()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected an error, got none")
		}
		if err.Error() != "error retrieving context: mock error retrieving context" {
			t.Fatalf("Expected error message 'error retrieving context: mock error retrieving context', got %v", err)
		}
	})

	t.Run("ErrorResolvingContext", func(t *testing.T) {
		// Setup mock components with a mock container that has SetResolveError
		mockContainer := di.NewMockContainer()
		mockContainer.SetResolveError("contextHandler", fmt.Errorf("mock error resolving context handler"))
		mocks := setupSafeDockerContainerMocks(mockContainer)
		dockerVirt := NewDockerVirt(mocks.Container)

		// When calling GetContainerInfo
		_, err := dockerVirt.GetContainerInfo()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected an error, got none")
		}
		if err.Error() != "error resolving context handler: mock error resolving context handler" {
			t.Fatalf("Expected error message 'error resolving context handler: mock error resolving context handler', got %v", err)
		}
	})

	t.Run("ErrorResolvingShell", func(t *testing.T) {
		// Setup mock components with a mock container that has SetResolveError
		mockContainer := di.NewMockContainer()
		mockContainer.SetResolveError("shell", fmt.Errorf("mock error resolving shell"))
		mocks := setupSafeDockerContainerMocks(mockContainer)
		dockerVirt := NewDockerVirt(mocks.Container)

		// When calling GetContainerInfo
		_, err := dockerVirt.GetContainerInfo()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected an error, got none")
		}
		if err.Error() != "error resolving shell: mock error resolving shell" {
			t.Fatalf("Expected error message 'error resolving shell: mock error resolving shell', got %v", err)
		}
	})

	t.Run("ErrorFetchingContainerIDs", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeDockerContainerMocks()
		dockerVirt := NewDockerVirt(mocks.Container)

		// Mock the shell Exec function to simulate an error when fetching container IDs
		mocks.MockShell.ExecFunc = func(verbose bool, description string, command string, args ...string) (string, error) {
			if command == "docker" && len(args) > 0 && args[0] == "ps" {
				return "", fmt.Errorf("mock error fetching container IDs")
			}
			return "", fmt.Errorf("unknown command")
		}

		// When calling GetContainerInfo
		_, err := dockerVirt.GetContainerInfo()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected an error, got none")
		}
		if err.Error() != "mock error fetching container IDs" {
			t.Fatalf("Expected error message 'mock error fetching container IDs', got %v", err)
		}
	})

	t.Run("ErrorInspectingContainer", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeDockerContainerMocks()
		dockerVirt := NewDockerVirt(mocks.Container)

		// Mock the shell Exec function to simulate an error when inspecting a container
		mocks.MockShell.ExecFunc = func(verbose bool, description string, command string, args ...string) (string, error) {
			if command == "docker" {
				if len(args) > 0 && args[0] == "ps" {
					return "mocked container ID", nil // Simulate successful docker ps
				}
				if len(args) > 0 && args[0] == "inspect" {
					return "", fmt.Errorf("mock error inspecting container")
				}
			}
			return "", fmt.Errorf("unknown command")
		}

		// When calling GetContainerInfo
		_, err := dockerVirt.GetContainerInfo()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected an error, got none")
		}
		if err.Error() != "mock error inspecting container" {
			t.Fatalf("Expected error message 'mock error inspecting container', got %v", err)
		}
	})

	t.Run("ErrorInspectingContainerNetworkSettings", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeDockerContainerMocks()
		dockerVirt := NewDockerVirt(mocks.Container)

		// Mock the shell Exec function to simulate an error when inspecting container network settings
		mocks.MockShell.ExecFunc = func(verbose bool, description string, command string, args ...string) (string, error) {
			if command == "docker" {
				if len(args) > 0 && args[0] == "ps" {
					return "mocked container ID", nil // Simulate successful docker ps
				}
				if len(args) > 0 && args[0] == "inspect" {
					if strings.Contains(description, "Inspecting container") {
						return `{"com.docker.compose.service": "mocked-service"}`, nil // Simulate successful label inspection
					}
					if strings.Contains(description, "network settings") {
						return "", fmt.Errorf("json: cannot unmarshal string into Go value of type struct { IPAddress string \"json:\\\"IPAddress\\\"\" }")
					}
				}
			}
			return "", fmt.Errorf("unknown command")
		}

		// When calling GetContainerInfo
		_, err := dockerVirt.GetContainerInfo()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected an error, got none")
		}
		if err.Error() != "json: cannot unmarshal string into Go value of type struct { IPAddress string \"json:\\\"IPAddress\\\"\" }" {
			t.Fatalf("Expected error message 'json: cannot unmarshal string into Go value of type struct { IPAddress string \"json:\\\"IPAddress\\\"\" }', got %v", err)
		}
	})

	t.Run("ErrorUnmarshallingLabels", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeDockerContainerMocks()
		dockerVirt := NewDockerVirt(mocks.Container)

		// Mock the jsonUnmarshal function to simulate an error when unmarshalling labels
		originalJsonUnmarshal := jsonUnmarshal
		defer func() { jsonUnmarshal = originalJsonUnmarshal }()
		jsonUnmarshal = func(data []byte, v interface{}) error {
			return fmt.Errorf("json: cannot unmarshal string into Go value of type map[string]string")
		}

		// Mock the shell Exec function to simulate successful docker ps and label inspection
		mocks.MockShell.ExecFunc = func(verbose bool, description string, command string, args ...string) (string, error) {
			if command == "docker" {
				if len(args) > 0 && args[0] == "ps" {
					return "mocked container ID", nil // Simulate successful docker ps
				}
				if len(args) > 0 && args[0] == "inspect" && strings.Contains(description, "Inspecting container") {
					return `{"com.docker.compose.service": "mocked-service"}`, nil // Simulate successful label inspection
				}
			}
			return "", fmt.Errorf("unknown command")
		}

		// When calling GetContainerInfo
		_, err := dockerVirt.GetContainerInfo()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected an error, got none")
		}
		if err.Error() != "json: cannot unmarshal string into Go value of type map[string]string" {
			t.Fatalf("Expected error message 'json: cannot unmarshal string into Go value of type map[string]string', got %v", err)
		}
	})

	t.Run("MissingServiceName", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeDockerContainerMocks()
		dockerVirt := NewDockerVirt(mocks.Container)

		// Mock the shell Exec function to simulate successful docker ps and label inspection
		mocks.MockShell.ExecFunc = func(verbose bool, description string, command string, args ...string) (string, error) {
			if command == "docker" {
				if len(args) > 0 && args[0] == "ps" {
					return "mocked container ID", nil // Simulate successful docker ps
				}
				if len(args) > 0 && args[0] == "inspect" && strings.Contains(description, "Inspecting container") {
					return `{}`, nil // Simulate missing service name in labels
				}
			}
			return "", fmt.Errorf("unknown command")
		}

		// When calling GetContainerInfo
		containerInfos, err := dockerVirt.GetContainerInfo()

		// Then no error should be returned and containerInfos should be empty
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(containerInfos) != 0 {
			t.Fatalf("Expected no container info, got %v", containerInfos)
		}
	})

	t.Run("ErrorInspectingNetworkSettings", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeDockerContainerMocks()
		dockerVirt := NewDockerVirt(mocks.Container)

		// Mock the shell Exec function to simulate an error during network settings inspection
		mocks.MockShell.ExecFunc = func(verbose bool, description string, command string, args ...string) (string, error) {
			if command == "docker" {
				if len(args) > 0 && args[0] == "ps" {
					return "mocked container ID", nil // Simulate successful docker ps
				}
				if len(args) > 0 && args[0] == "inspect" && strings.Contains(description, "Inspecting container network settings") {
					return "", fmt.Errorf("error inspecting network settings") // Simulate error during network settings inspection
				}
				if len(args) > 0 && args[0] == "inspect" && strings.Contains(description, "Inspecting container") {
					return `{"com.docker.compose.service": "mocked-service"}`, nil // Simulate successful label inspection
				}
			}
			return "", fmt.Errorf("unknown command")
		}

		// When calling GetContainerInfo
		_, err := dockerVirt.GetContainerInfo()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected an error, got none")
		}
		if err.Error() != "error inspecting network settings" {
			t.Fatalf("Expected error message 'error inspecting network settings', got %v", err)
		}
	})
}

func TestDockerVirt_Delete(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Stub test for successful Delete
	})

	t.Run("Error", func(t *testing.T) {
		// Stub test for error during Delete
	})
}

func TestDockerVirt_PrintInfo(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Stub test for successful PrintInfo
	})

	t.Run("Error", func(t *testing.T) {
		// Stub test for error during PrintInfo
	})
}

func TestDockerVirt_WriteConfig(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Stub test for successful WriteConfig
	})

	t.Run("ErrorResolvingContext", func(t *testing.T) {
		// Stub test for error resolving context
	})

	t.Run("ErrorResolvingConfigHandler", func(t *testing.T) {
		// Stub test for error resolving config handler
	})

	t.Run("ErrorRetrievingConfig", func(t *testing.T) {
		// Stub test for error retrieving config
	})

	t.Run("NoVMDefined", func(t *testing.T) {
		// Stub test for no VM defined
	})

	t.Run("AArchVM", func(t *testing.T) {
		// Stub test for AArch VM
	})

	t.Run("ErrorSavingConfig", func(t *testing.T) {
		// Stub test for error saving config
	})

	t.Run("ErrorRetrievingContext", func(t *testing.T) {
		// Stub test for error retrieving context
	})

	t.Run("ErrorGettingUserHomeDir", func(t *testing.T) {
		// Stub test for error getting user home directory
	})

	t.Run("ErrorCreatingParentDirectories", func(t *testing.T) {
		// Stub test for error creating parent directories
	})

	t.Run("ErrorCreatingColimaDirectory", func(t *testing.T) {
		// Stub test for error creating Colima directory
	})

	t.Run("ErrorEncodingYaml", func(t *testing.T) {
		// Stub test for error encoding YAML
	})

	t.Run("ErrorClosingEncoder", func(t *testing.T) {
		// Stub test for error closing encoder
	})

	t.Run("ErrorRenamingTemporaryFile", func(t *testing.T) {
		// Stub test for error renaming temporary file
	})
}
