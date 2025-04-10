package virt

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/compose-spec/compose-go/types"
	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/services"
	"github.com/windsorcli/cli/pkg/shell"
)

func setupSafeDockerContainerMocks(optionalInjector ...di.Injector) *MockComponents {
	var injector di.Injector
	if len(optionalInjector) > 0 {
		injector = optionalInjector[0]
	} else {
		injector = di.NewMockInjector()
	}

	mockShell := shell.NewMockShell(injector)
	mockConfigHandler := config.NewMockConfigHandler()
	mockService := services.NewMockService()

	// Register mock instances in the injector
	injector.Register("shell", mockShell)
	injector.Register("configHandler", mockConfigHandler)
	injector.Register("service", mockService)

	// Set up default mock behaviors
	mockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
		if key == "docker.enabled" {
			return true
		}
		if key == "dns.enabled" {
			return true
		}
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return false
	}

	mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
		if key == "network.cidr_block" {
			return "10.0.0.0/24"
		}
		if key == "dns.address" {
			return ""
		}
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return ""
	}

	mockConfigHandler.GetContextFunc = func() string {
		return "mock-context"
	}

	// Mock the shell Exec function to return generic JSON structures for two containers
	mockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
		if command == "docker" && len(args) > 0 {
			switch args[0] {
			case "ps":
				return "container1\ncontainer2", nil
			case "inspect":
				if len(args) > 3 && args[2] == "--format" {
					switch args[3] {
					case "{{json .Config.Labels}}":
						// Return both matching and non-matching service names
						if args[1] == "container1" {
							return `{"com.docker.compose.service":"service1","managed_by":"windsor","context":"mock-context"}`, nil
						} else if args[1] == "container2" {
							return `{"com.docker.compose.service":"service2","managed_by":"windsor","context":"mock-context"}`, nil
						}
					case "{{json .NetworkSettings.Networks}}":
						if args[1] == "container1" {
							return `{"windsor-mock-context":{"IPAddress":"192.168.1.2"}}`, nil
						} else if args[1] == "container2" {
							return `{"windsor-mock-context":{"IPAddress":"192.168.1.3"}}`, nil
						}
					}
				}
			}
		}
		return "", fmt.Errorf("unknown command")
	}

	// Mock the service's GetComposeConfigFunc to return a default configuration for two services
	mockService.GetComposeConfigFunc = func() (*types.Config, error) {
		return &types.Config{
			Services: []types.ServiceConfig{
				{Name: "service1", Networks: map[string]*types.ServiceNetworkConfig{"windsor-mock-context": {Ipv4Address: "192.168.1.2"}}},
				{Name: "service2", Networks: map[string]*types.ServiceNetworkConfig{"windsor-mock-context": {Ipv4Address: "192.168.1.3"}}},
			},
			Volumes: map[string]types.VolumeConfig{
				"volume1": {},
				"volume2": {},
			},
			Networks: map[string]types.NetworkConfig{
				"network1": {
					Driver: "bridge",
				},
				"network2": {
					Driver: "bridge",
				},
			},
		}, nil
	}

	// Mock the GetAddress function to return specific IP addresses for services
	mockService.GetAddressFunc = func() string {
		return "192.168.1.2"
	}

	// Mock the GetProjectRootFunc to return a mock project root path
	mockShell.GetProjectRootFunc = func() (string, error) {
		return "/mock/project/root", nil
	}

	return &MockComponents{
		Injector:          injector,
		MockShell:         mockShell,
		MockConfigHandler: mockConfigHandler,
		MockService:       mockService,
	}
}

func TestDockerVirt_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeDockerContainerMocks()
		dockerVirt := NewDockerVirt(mocks.Injector)

		// Mock the shell's ExecSilent function to simulate a valid docker compose command
		mocks.MockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "docker-compose" && len(args) > 0 && args[0] == "--version" {
				return "docker-compose version 1.29.2, build 5becea4c", nil
			}
			return "", fmt.Errorf("unknown command")
		}

		// Call the Initialize method
		err := dockerVirt.Initialize()

		// Assert no error occurred
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// Verify that the services were resolved correctly
		if len(dockerVirt.services) == 0 {
			t.Errorf("expected services to be resolved, but got none")
		}
	})

	t.Run("ErrorInitializingBaseVirt", func(t *testing.T) {
		// Setup mock components
		injector := di.NewInjector()
		injector.Register("shell", "not a shell")
		dockerVirt := NewDockerVirt(injector)

		// Call the Initialize method
		err := dockerVirt.Initialize()

		// Assert that an error occurred
		if err == nil {
			t.Errorf("expected error, got none")
		}

		// Verify the error message contains the expected substring
		expectedErrorSubstring := "error resolving shell"
		if !strings.Contains(err.Error(), expectedErrorSubstring) {
			t.Errorf("expected error message to contain %q, got %q", expectedErrorSubstring, err.Error())
		}
	})

	t.Run("ErrorResolvingServices", func(t *testing.T) {
		// Setup mock components
		injector := di.NewMockInjector()
		mocks := setupSafeDockerContainerMocks(injector)
		dockerVirt := NewDockerVirt(mocks.Injector)

		// Simulate an error during service resolution
		injector.SetResolveAllError((*services.Service)(nil), fmt.Errorf("mock resolve services error"))

		// Call the Initialize method
		err := dockerVirt.Initialize()

		// Assert that an error occurred
		if err == nil {
			t.Errorf("expected error, got none")
		}

		// Verify the error message contains the expected substring
		expectedErrorSubstring := "error resolving services"
		if !strings.Contains(err.Error(), expectedErrorSubstring) {
			t.Errorf("expected error message to contain %q, got %q", expectedErrorSubstring, err.Error())
		}
	})
}

func TestDockerVirt_Up(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeDockerContainerMocks()
		dockerVirt := NewDockerVirt(mocks.Injector)
		dockerVirt.Initialize()

		// Mock the shell Exec function to simulate successful docker info and docker compose up
		mocks.MockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "docker" && len(args) > 0 && args[0] == "info" {
				return "docker info", nil
			}
			return "", fmt.Errorf("unknown command")
		}
		mocks.MockShell.ExecProgressFunc = func(message string, command string, args ...string) (string, error) {
			if command == dockerVirt.composeCommand && args[0] == "up" {
				return "docker compose up successful", nil
			}
			return "", fmt.Errorf("unknown command")
		}

		// Call the Up method
		err := dockerVirt.Up()

		// Assert that no error occurred
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("DockerDaemonNotRunning", func(t *testing.T) {
		// Setup mock components without mocking the container
		mocks := setupSafeDockerContainerMocks()
		dockerVirt := NewDockerVirt(mocks.Injector)
		dockerVirt.Initialize()

		// Mock the shell Exec function to simulate the Docker daemon not running
		mocks.MockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "docker" && len(args) > 0 && args[0] == "info" {
				return "", fmt.Errorf("Cannot connect to the Docker daemon")
			}
			return "", fmt.Errorf("unknown command")
		}

		// Call the Up method
		err := dockerVirt.Up()

		// Assert that an error occurred
		if err == nil {
			t.Errorf("expected an error, got nil")
		}

		// Verify that the error message is as expected
		expectedErrorMsg := "Docker daemon is not running"
		if err != nil && !strings.Contains(err.Error(), expectedErrorMsg) {
			t.Errorf("expected error message to contain %q, got %v", expectedErrorMsg, err)
		}
	})

	t.Run("ErrorGetConfigRoot", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeDockerContainerMocks()
		dockerVirt := NewDockerVirt(mocks.Injector)
		dockerVirt.Initialize()

		// Mock the GetProjectRoot function to simulate an error
		mocks.MockShell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("mock error retrieving project root")
		}

		// Mock the shell Exec function to simulate Docker daemon check
		mocks.MockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "docker" && len(args) > 0 && args[0] == "info" {
				return "docker info", nil
			}
			return "", fmt.Errorf("unknown command")
		}

		// Call the Up method
		err := dockerVirt.Up()

		// Assert that an error occurred
		if err == nil {
			t.Errorf("expected an error, got nil")
		}

		// Verify that the error message is as expected
		expectedErrorMsg := "error retrieving project root"
		if err != nil && !strings.Contains(err.Error(), expectedErrorMsg) {
			t.Errorf("expected error message to contain %q, got %v", expectedErrorMsg, err)
		}
	})

	t.Run("ErrorSettingComposeFileEnv", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeDockerContainerMocks()
		dockerVirt := NewDockerVirt(mocks.Injector)
		dockerVirt.Initialize()

		// Mock the GetConfigRoot function to return a valid path
		mocks.MockConfigHandler.GetConfigRootFunc = func() (string, error) {
			return "/valid/path", nil
		}

		// Mock the shell Exec function to simulate Docker daemon check
		mocks.MockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "docker" && len(args) > 0 && args[0] == "info" {
				return "docker info", nil
			}
			return "", fmt.Errorf("unknown command")
		}

		// Temporarily replace osSetenv with a mock function to simulate an error
		originalSetenv := osSetenv
		defer func() { osSetenv = originalSetenv }()
		osSetenv = func(key, value string) error {
			if key == "COMPOSE_FILE" {
				return fmt.Errorf("mock error setting COMPOSE_FILE environment variable")
			}
			return nil
		}

		// Call the Up method
		err := dockerVirt.Up()

		// Assert that an error occurred
		if err == nil {
			t.Errorf("expected an error, got nil")
		}

		// Verify that the error message is as expected
		expectedErrorMsg := "error setting COMPOSE_FILE environment variable"
		if err != nil && !strings.Contains(err.Error(), expectedErrorMsg) {
			t.Errorf("expected error message to contain %q, got %v", expectedErrorMsg, err)
		}
	})

	t.Run("RetryDockerComposeUp", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeDockerContainerMocks()
		dockerVirt := NewDockerVirt(mocks.Injector)
		dockerVirt.Initialize()

		// Counter to track the number of retries
		execCallCount := 0

		// Mock the shell Exec functions to simulate retry logic
		mocks.MockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "docker" && len(args) > 0 && args[0] == "info" {
				return "docker info", nil
			}
			if command == dockerVirt.composeCommand && len(args) > 0 && args[0] == "up" {
				execCallCount++
				if execCallCount < 3 {
					return "", fmt.Errorf("temporary error")
				}
				return "success", nil
			}
			return "", fmt.Errorf("unknown command")
		}
		mocks.MockShell.ExecProgressFunc = func(message string, command string, args ...string) (string, error) {
			if command == dockerVirt.composeCommand && len(args) > 0 && args[0] == "up" {
				execCallCount++
				if execCallCount < 3 {
					return "", fmt.Errorf("temporary error")
				}
				return "success", nil
			}
			return "", fmt.Errorf("unknown command")
		}

		// Call the Up method
		err := dockerVirt.Up()

		// Assert that no error occurred after retries
		if err != nil {
			t.Errorf("expected no error after retries, got %v", err)
		}

		// Verify that the Exec function was called 3 times
		if execCallCount != 3 {
			t.Errorf("expected Exec to be called 3 times, got %d", execCallCount)
		}
	})

	t.Run("DockerComposeUpRetryError", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeDockerContainerMocks()
		dockerVirt := NewDockerVirt(mocks.Injector)
		dockerVirt.Initialize()

		// Counter to track the number of retries
		execCallCount := 0

		// Mock the shell Exec functions to simulate retry logic with persistent error
		mocks.MockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "docker" && len(args) > 0 && args[0] == "info" {
				return "docker info", nil
			}
			if command == dockerVirt.composeCommand && len(args) > 0 && args[0] == "up" {
				execCallCount++
				return "", fmt.Errorf("persistent error")
			}
			return "", fmt.Errorf("unknown command")
		}
		mocks.MockShell.ExecProgressFunc = func(message string, command string, args ...string) (string, error) {
			if command == dockerVirt.composeCommand && len(args) > 0 && args[0] == "up" {
				execCallCount++
				return "", fmt.Errorf("persistent error")
			}
			return "", fmt.Errorf("unknown command")
		}

		// Call the Up method
		err := dockerVirt.Up()

		// Assert that an error occurred after retries
		if err == nil {
			t.Errorf("expected an error after retries, got nil")
		}

		// Verify that the Exec function was called 3 times
		if execCallCount != 3 {
			t.Errorf("expected Exec to be called 3 times, got %d", execCallCount)
		}

		// Verify that the error message is as expected
		expectedErrorMsg := "persistent error"
		if err != nil && !strings.Contains(err.Error(), expectedErrorMsg) {
			t.Errorf("expected error message to contain %q, got %v", expectedErrorMsg, err)
		}
	})
}

func TestDockerVirt_Down(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeDockerContainerMocks()
		dockerVirt := NewDockerVirt(mocks.Injector)
		dockerVirt.Initialize()

		// Mock the shell Exec function to simulate successful docker info and docker compose down commands
		mocks.MockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "docker" && len(args) > 0 && args[0] == "info" {
				return "docker info", nil
			}
			if command == "docker compose" && len(args) > 2 && args[2] == "down" {
				return "docker compose down", nil
			}
			return "", fmt.Errorf("unknown command")
		}

		// Call the Down method
		err := dockerVirt.Down()

		// Assert no error occurred
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("DockerDaemonNotRunning", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeDockerContainerMocks()
		dockerVirt := NewDockerVirt(mocks.Injector)
		dockerVirt.Initialize()

		// Mock the shell Exec function to simulate Docker daemon not running
		mocks.MockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "docker" && len(args) > 0 && args[0] == "info" {
				return "", fmt.Errorf("Docker daemon is not running")
			}
			return "", fmt.Errorf("unknown command")
		}

		// Call the Down method
		err := dockerVirt.Down()

		// Assert that an error occurred
		if err == nil {
			t.Errorf("expected an error, got nil")
		}

		// Verify that the error message is as expected
		expectedErrorMsg := "Docker daemon is not running"
		if err != nil && !strings.Contains(err.Error(), expectedErrorMsg) {
			t.Errorf("expected error message to contain %q, got %v", expectedErrorMsg, err)
		}
	})

	t.Run("ErrorGetConfigRoot", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeDockerContainerMocks()
		dockerVirt := NewDockerVirt(mocks.Injector)
		dockerVirt.Initialize()

		// Mock the GetConfigRootFunc to return an error
		mocks.MockShell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("error retrieving project root")
		}

		// Mock the shell Exec function to simulate successful docker info command
		mocks.MockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "docker" && len(args) > 0 && args[0] == "info" {
				return "docker info", nil
			}
			return "", fmt.Errorf("unknown command")
		}

		// Call the Down method
		err := dockerVirt.Down()

		// Assert that an error occurred
		if err == nil {
			t.Errorf("expected an error, got nil")
		}

		// Verify that the error message is as expected
		expectedErrorMsg := "error retrieving project root"
		if err != nil && !strings.Contains(err.Error(), expectedErrorMsg) {
			t.Errorf("expected error message to contain %q, got %v", expectedErrorMsg, err)
		}
	})

	t.Run("ErrorSettingComposeFileEnv", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeDockerContainerMocks()
		dockerVirt := NewDockerVirt(mocks.Injector)
		dockerVirt.Initialize()

		// Mock the shell Exec function to simulate successful docker info command
		mocks.MockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "docker" && len(args) > 0 && args[0] == "info" {
				return "docker info", nil
			}
			return "", fmt.Errorf("unknown command")
		}

		// Temporarily replace osSetenv with a mock function to simulate an error
		originalSetenv := osSetenv
		defer func() { osSetenv = originalSetenv }()
		osSetenv = func(key, value string) error {
			if key == "COMPOSE_FILE" {
				return fmt.Errorf("mock error setting COMPOSE_FILE environment variable")
			}
			return nil
		}

		// Call the Down method
		err := dockerVirt.Down()

		// Assert that an error occurred
		if err == nil {
			t.Errorf("expected an error, got nil")
		}

		// Verify that the error message is as expected
		expectedErrorMsg := "error setting COMPOSE_FILE environment variable"
		if err != nil && !strings.Contains(err.Error(), expectedErrorMsg) {
			t.Errorf("expected error message to contain %q, got %v", expectedErrorMsg, err)
		}
	})

	t.Run("ErrorDockerComposeDown", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeDockerContainerMocks()
		dockerVirt := NewDockerVirt(mocks.Injector)
		dockerVirt.Initialize()

		// Mock the shell Exec function to simulate successful docker info command
		mocks.MockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "docker" && len(args) > 0 && args[0] == "info" {
				return "docker info", nil
			}
			return "", fmt.Errorf("unknown command")
		}
		mocks.MockShell.ExecProgressFunc = func(message string, command string, args ...string) (string, error) {
			if command == dockerVirt.composeCommand && len(args) > 0 && args[0] == "down" {
				return "", fmt.Errorf("error executing docker compose down")
			}
			return "", fmt.Errorf("unknown command")
		}

		// Call the Down method
		err := dockerVirt.Down()

		// Assert that an error occurred
		if err == nil {
			t.Errorf("expected an error, got nil")
		}

		// Verify that the error message contains the expected substring
		expectedErrorSubstring := "docker compose down"
		if err != nil && !strings.Contains(err.Error(), expectedErrorSubstring) {
			t.Errorf("expected error message to contain %q, got %v", expectedErrorSubstring, err)
		}
	})
}

func TestDockerVirt_GetContainerInfo(t *testing.T) {
	t.Run("SuccessNoArguments", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeDockerContainerMocks()
		dockerVirt := NewDockerVirt(mocks.Injector)
		dockerVirt.Initialize()

		// When calling GetContainerInfo
		containerInfos, err := dockerVirt.GetContainerInfo()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// And the container info should be as expected
		if len(containerInfos) != 2 {
			t.Fatalf("Expected 2 container info, got %d", len(containerInfos))
		}

		// Create a map to store expected addresses for each service
		expectedAddresses := map[string]string{
			"service1": "192.168.1.2",
			"service2": "192.168.1.3",
		}

		for _, containerInfo := range containerInfos {
			expectedAddress, exists := expectedAddresses[containerInfo.Name]
			if !exists {
				t.Errorf("Unexpected container name %q", containerInfo.Name)
				continue
			}
			if containerInfo.Address != expectedAddress {
				t.Errorf("Expected container address %q for service %q, got %q", expectedAddress, containerInfo.Name, containerInfo.Address)
			}
		}
	})

	t.Run("SuccessWithNameArgument", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeDockerContainerMocks()
		dockerVirt := NewDockerVirt(mocks.Injector)
		dockerVirt.Initialize()

		// When calling GetContainerInfo with a specific name argument
		containerInfos, err := dockerVirt.GetContainerInfo("service2")

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// And the container info should be as expected
		if len(containerInfos) != 1 {
			t.Fatalf("Expected 1 container info, got %d", len(containerInfos))
		}
		expectedName := "service2"
		expectedAddress := "192.168.1.3"
		if containerInfos[0].Name != expectedName {
			t.Errorf("Expected container name %q, got %q", expectedName, containerInfos[0].Name)
		}
		if containerInfos[0].Address != expectedAddress {
			t.Errorf("Expected container address %q, got %q", expectedAddress, containerInfos[0].Address)
		}
	})

	t.Run("ErrorInspectingContainer", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeDockerContainerMocks()
		dockerVirt := NewDockerVirt(mocks.Injector)
		dockerVirt.Initialize()

		// Mock the necessary methods to simulate an error during container inspection
		originalExecFunc := mocks.MockShell.ExecSilentFunc
		mocks.MockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "docker" && len(args) > 0 {
				switch args[0] {
				case "inspect":
					if len(args) > 2 && args[2] == "--format" {
						return "", fmt.Errorf("mock error inspecting container")
					}
				}
			}
			// Call the original ExecFunc for any other cases
			return originalExecFunc(command, args...)
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

	t.Run("ErrorUnmarshallingContainerInfo", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeDockerContainerMocks()
		dockerVirt := NewDockerVirt(mocks.Injector)
		dockerVirt.Initialize()

		// Mock the necessary methods to simulate an error during JSON unmarshalling
		originalExecFunc := mocks.MockShell.ExecSilentFunc
		mocks.MockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "docker" && len(args) > 0 {
				switch args[0] {
				case "inspect":
					if len(args) > 2 && args[2] == "--format" {
						return "{invalid-json}", nil // Return invalid JSON to trigger unmarshalling error
					}
				}
			}
			// Call the original ExecFunc for any other cases
			return originalExecFunc(command, args...)
		}

		// When calling GetContainerInfo
		_, err := dockerVirt.GetContainerInfo()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected an error, got none")
		}
		if !strings.Contains(err.Error(), "invalid character") {
			t.Fatalf("Expected JSON unmarshalling error, got %v", err)
		}
	})

	t.Run("ErrorGettingContainerInfo", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeDockerContainerMocks()
		dockerVirt := NewDockerVirt(mocks.Injector)
		dockerVirt.Initialize()

		// Mock the shell Exec function to simulate an error when retrieving container info
		mocks.MockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "docker" && len(args) > 0 && args[0] == "ps" {
				return "", fmt.Errorf("mock error retrieving container info")
			}
			return "", fmt.Errorf("unknown command")
		}

		// When calling GetContainerInfo
		_, err := dockerVirt.GetContainerInfo()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected an error, got none")
		}
		if err.Error() != "mock error retrieving container info" {
			t.Fatalf("Expected error message 'mock error retrieving container info', got %v", err)
		}
	})

	t.Run("ErrorInspectingNetwork", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeDockerContainerMocks()
		dockerVirt := NewDockerVirt(mocks.Injector)
		dockerVirt.Initialize()

		// Mock the shell Exec function to simulate an error when inspecting network
		originalExecFunc := mocks.MockShell.ExecSilentFunc
		mocks.MockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "docker" && len(args) > 0 && args[0] == "inspect" && args[2] == "--format" && args[3] == "{{json .NetworkSettings.Networks}}" {
				return "", fmt.Errorf("mock error inspecting network")
			}
			return originalExecFunc(command, args...)
		}

		// When calling GetContainerInfo
		_, err := dockerVirt.GetContainerInfo()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected an error, got none")
		}
		if err.Error() != "mock error inspecting network" {
			t.Fatalf("Expected error message 'mock error inspecting network', got %v", err)
		}
	})

	t.Run("ErrorUnmarshallingNetworkInfo", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeDockerContainerMocks()
		dockerVirt := NewDockerVirt(mocks.Injector)
		dockerVirt.Initialize()

		// Mock the shell Exec function to simulate an error when unmarshalling network info
		originalExecFunc := mocks.MockShell.ExecSilentFunc
		mocks.MockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "docker" && len(args) > 0 && args[0] == "inspect" && args[2] == "--format" && args[3] == "{{json .NetworkSettings.Networks}}" {
				return `invalid json`, nil
			}
			return originalExecFunc(command, args...)
		}

		// When calling GetContainerInfo
		_, err := dockerVirt.GetContainerInfo()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected an error, got none")
		}
		if !strings.Contains(err.Error(), "invalid character") {
			t.Fatalf("Expected error message containing 'invalid character', got %v", err)
		}
	})
}

func TestDockerVirt_PrintInfo(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeDockerContainerMocks()
		dockerVirt := NewDockerVirt(mocks.Injector)
		dockerVirt.Initialize()

		// Capture the output of PrintInfo using captureStdout utility function
		output := captureStdout(func() {
			err := dockerVirt.PrintInfo()
			// Assert no error occurred
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
		})

		// Check for the presence of key elements in the output
		if !strings.Contains(output, "CONTAINER NAME") || !strings.Contains(output, "service1") || !strings.Contains(output, "192.168.1.2") {
			t.Fatalf("output does not contain expected elements, got %q", output)
		}
	})

	t.Run("ErrorGettingContainerInfo", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeDockerContainerMocks()
		dockerVirt := NewDockerVirt(mocks.Injector)
		dockerVirt.Initialize()

		// Mock the shell Exec function to simulate an error when fetching container IDs
		mocks.MockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "docker" && len(args) > 0 && args[0] == "ps" {
				return "", fmt.Errorf("error fetching container IDs")
			}
			return "", fmt.Errorf("unknown command")
		}

		// Call the PrintInfo method
		err := dockerVirt.PrintInfo()

		// Assert that an error occurred
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// Verify that the error message is as expected
		expectedErrorMsg := "error retrieving container info"
		if err != nil && !strings.Contains(err.Error(), expectedErrorMsg) {
			t.Fatalf("expected error message to contain %q, got %v", expectedErrorMsg, err)
		}
	})

	t.Run("NoContainers", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeDockerContainerMocks()
		dockerVirt := NewDockerVirt(mocks.Injector)
		dockerVirt.Initialize()

		// Mock the shell Exec function to simulate no running containers
		mocks.MockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "docker" && len(args) > 0 && args[0] == "ps" {
				return "\n", nil // Simulate no containers running by returning an empty line
			}
			return "", nil // Return no error for unknown commands to avoid unexpected errors
		}

		// Capture the output of PrintInfo using captureStdout utility function
		output := captureStdout(func() {
			err := dockerVirt.PrintInfo()
			// Assert no error occurred
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
		})

		// Check that the output contains the message for no running containers
		expectedOutput := "No Docker containers are currently running."
		if !strings.Contains(output, expectedOutput) {
			t.Fatalf("expected output to contain %q, got %q", expectedOutput, output)
		}
	})
}

func TestDockerVirt_WriteConfig(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeDockerContainerMocks()
		dockerVirt := NewDockerVirt(mocks.Injector)
		dockerVirt.Initialize()

		// Mock the mkdirAll function to simulate successful directory creation
		originalMkdirAll := mkdirAll
		defer func() { mkdirAll = originalMkdirAll }()
		mkdirAll = func(path string, perm os.FileMode) error {
			return nil
		}

		// Mock the writeFile function to simulate successful file writing
		originalWriteFile := writeFile
		defer func() { writeFile = originalWriteFile }()
		writeFile = func(filename string, data []byte, perm os.FileMode) error {
			return nil
		}

		// Call the WriteConfig method
		err := dockerVirt.WriteConfig()

		// Assert no error occurred
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("ErrorCreatingParentContextFolder", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeDockerContainerMocks()
		dockerVirt := NewDockerVirt(mocks.Injector)
		dockerVirt.Initialize()

		// Mock the mkdirAll function to simulate a read-only file system error
		originalMkdirAll := mkdirAll
		defer func() { mkdirAll = originalMkdirAll }()
		mkdirAll = func(path string, perm os.FileMode) error {
			// Use filepath.FromSlash to ensure compatibility with Windows file paths
			expectedPath := filepath.Join("/mock/project/root", ".windsor")
			if filepath.Clean(path) == filepath.FromSlash(expectedPath) {
				return fmt.Errorf("read-only file system")
			}
			return nil
		}

		// Call the WriteConfig method
		err := dockerVirt.WriteConfig()

		// Assert an error occurred
		if err == nil {
			t.Fatal("expected an error, got none")
		}
		if err.Error() != "error creating parent context folder: read-only file system" {
			t.Fatalf("expected error message 'error creating parent context folder: read-only file system', got %v", err)
		}
	})

	t.Run("ErrorGettingConfigRoot", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeDockerContainerMocks()
		mocks.MockShell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("error retrieving project root")
		}

		dockerVirt := NewDockerVirt(mocks.Injector)
		dockerVirt.Initialize()

		// Call the WriteConfig method
		err := dockerVirt.WriteConfig()

		// Assert that an error occurred
		if err == nil {
			t.Fatal("expected an error, got none")
		}

		// Assert the error message is as expected
		expectedErrorMsg := "error retrieving project root"
		if !strings.Contains(err.Error(), expectedErrorMsg) {
			t.Fatalf("expected error message to contain %q, got %v", expectedErrorMsg, err)
		}
	})

	t.Run("ErrorGettingFullComposeConfig", func(t *testing.T) {
		// Setup mock components
		mockInjector := di.NewMockInjector()
		mocks := setupSafeDockerContainerMocks(mockInjector)
		dockerVirt := NewDockerVirt(mockInjector)
		dockerVirt.Initialize()

		// Mock the mkdirAll function to prevent actual directory creation
		originalMkdirAll := mkdirAll
		defer func() { mkdirAll = originalMkdirAll }()
		mkdirAll = func(path string, perm os.FileMode) error {
			return nil
		}

		// Mock the service's GetComposeConfig to return an error
		mocks.MockService.GetComposeConfigFunc = func() (*types.Config, error) {
			return nil, fmt.Errorf("error getting compose config from service")
		}

		// Call the WriteConfig method
		err := dockerVirt.WriteConfig()

		// Assert that an error occurred
		if err == nil {
			t.Fatal("expected an error, got none")
		}

		// Assert the error message is as expected
		expectedErrorMsg := "error getting compose config from service"
		if !strings.Contains(err.Error(), expectedErrorMsg) {
			t.Fatalf("expected error message to contain %q, got %v", expectedErrorMsg, err)
		}
	})

	t.Run("ErrorMarshalingYAML", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeDockerContainerMocks()
		dockerVirt := NewDockerVirt(mocks.Injector)
		dockerVirt.Initialize()

		// Mock the mkdirAll function to prevent actual directory creation
		originalMkdirAll := mkdirAll
		defer func() { mkdirAll = originalMkdirAll }()
		mkdirAll = func(path string, perm os.FileMode) error {
			return nil // Return nil to bypass the read-only file system error
		}

		// Mock the yamlMarshal function to simulate an error
		originalYamlMarshal := yamlMarshal
		defer func() { yamlMarshal = originalYamlMarshal }()
		yamlMarshal = func(v interface{}) ([]byte, error) {
			return nil, fmt.Errorf("mock yamlMarshal error")
		}

		// Call the WriteConfig method
		err := dockerVirt.WriteConfig()

		// Assert that an error occurred
		if err == nil {
			t.Fatal("expected an error, got none")
		}

		// Assert the error message is as expected
		expectedErrorMsg := "mock yamlMarshal error"
		if !strings.Contains(err.Error(), expectedErrorMsg) {
			t.Fatalf("expected error message to contain %q, got %v", expectedErrorMsg, err)
		}
	})

	t.Run("ErrorWritingFile", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeDockerContainerMocks()
		dockerVirt := NewDockerVirt(mocks.Injector)
		dockerVirt.Initialize()

		// Mock the mkdirAll function to prevent actual directory creation
		originalMkdirAll := mkdirAll
		defer func() { mkdirAll = originalMkdirAll }()
		mkdirAll = func(path string, perm os.FileMode) error {
			return nil // Return nil to bypass the directory creation
		}

		// Mock the yamlMarshal function to return valid YAML data
		originalYamlMarshal := yamlMarshal
		defer func() { yamlMarshal = originalYamlMarshal }()
		yamlMarshal = func(v interface{}) ([]byte, error) {
			return []byte("valid: yaml"), nil
		}

		// Mock the writeFile function to simulate an error
		originalWriteFile := writeFile
		defer func() { writeFile = originalWriteFile }()
		writeFile = func(filename string, data []byte, perm os.FileMode) error {
			return fmt.Errorf("mock writeFile error")
		}

		// Call the WriteConfig method
		err := dockerVirt.WriteConfig()

		// Assert that an error occurred
		if err == nil {
			t.Fatal("expected an error, got none")
		}

		// Assert the error message is as expected
		expectedErrorMsg := "mock writeFile error"
		if !strings.Contains(err.Error(), expectedErrorMsg) {
			t.Fatalf("expected error message to contain %q, got %v", expectedErrorMsg, err)
		}
	})
}

func TestDockerVirt_checkDockerDaemon(t *testing.T) {
	t.Run("DockerDaemonRunning", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeDockerContainerMocks()
		dockerVirt := NewDockerVirt(mocks.Injector)
		dockerVirt.Initialize()

		// Mock the shell Exec function to simulate Docker daemon running
		mocks.MockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "docker" && len(args) > 0 && args[0] == "info" {
				return "docker info", nil
			}
			return "", fmt.Errorf("unknown command")
		}

		// Call the checkDockerDaemon method
		err := dockerVirt.checkDockerDaemon()

		// Assert that no error occurred
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("DockerDaemonNotRunning", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeDockerContainerMocks()
		dockerVirt := NewDockerVirt(mocks.Injector)
		dockerVirt.Initialize()

		// Mock the shell Exec function to simulate Docker daemon not running
		mocks.MockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "docker" && len(args) > 0 && args[0] == "info" {
				return "", fmt.Errorf("Docker daemon is not running")
			}
			return "", fmt.Errorf("unknown command")
		}

		// Call the checkDockerDaemon method
		err := dockerVirt.checkDockerDaemon()

		// Assert that an error occurred
		if err == nil {
			t.Errorf("expected an error, got nil")
		}

		// Verify that the error message is as expected
		expectedErrorMsg := "Docker daemon is not running"
		if err != nil && !strings.Contains(err.Error(), expectedErrorMsg) {
			t.Errorf("expected error message to contain %q, got %v", expectedErrorMsg, err)
		}
	})
}

func TestDockerVirt_getFullComposeConfig(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeDockerContainerMocks()
		dockerVirt := NewDockerVirt(mocks.Injector)
		dockerVirt.Initialize()

		// Call the getFullComposeConfig method
		project, err := dockerVirt.getFullComposeConfig()

		// Assert no error occurred
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// Assert the project is not nil
		if project == nil {
			t.Errorf("expected a project, got nil")
		}

		// Assert the project contains the expected services, volumes, and networks
		expectedServices := []string{"service1", "service2"}
		if len(project.Services) != len(expectedServices) {
			t.Errorf("expected %d services, got %d", len(expectedServices), len(project.Services))
		} else {
			for i, service := range project.Services {
				if service.Name != expectedServices[i] {
					t.Errorf("expected service '%s', got '%s'", expectedServices[i], service.Name)
				}
			}
		}

		if len(project.Volumes) != 2 {
			t.Errorf("expected 2 volumes, got %d", len(project.Volumes))
		}
		if len(project.Networks) != 3 {
			t.Errorf("expected 3 networks, got %d", len(project.Networks))
		}
	})

	t.Run("NoDockerDefined", func(t *testing.T) {
		// Setup mock components with a config handler that returns no Docker configuration
		mockInjector := di.NewMockInjector()
		mocks := setupSafeDockerContainerMocks(mockInjector)
		mocks.MockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "docker.enabled" {
				return false
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return false
		}
		dockerVirt := NewDockerVirt(mockInjector)
		dockerVirt.Initialize()

		// Call the getFullComposeConfig method
		project, err := dockerVirt.getFullComposeConfig()

		// Assert no error occurred
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// Assert the project is nil
		if project != nil {
			t.Errorf("expected project to be nil, got %v", project)
		}
	})

	t.Run("ErrorGettingComposeConfig", func(t *testing.T) {
		// Setup mock components
		mockInjector := di.NewMockInjector()
		mocks := setupSafeDockerContainerMocks(mockInjector)
		dockerVirt := NewDockerVirt(mockInjector)
		dockerVirt.Initialize()

		// Mock the git service's GetComposeConfigFunc to return an error
		mocks.MockService.GetComposeConfigFunc = func() (*types.Config, error) {
			return nil, fmt.Errorf("error getting compose config")
		}

		// Call the getFullComposeConfig method
		project, err := dockerVirt.getFullComposeConfig()

		// Assert that an error occurred
		if err == nil {
			t.Errorf("expected an error, got nil")
		}

		// Assert the error message is as expected
		expectedErrorMsg := "error getting compose config"
		if err != nil && !strings.Contains(err.Error(), expectedErrorMsg) {
			t.Errorf("expected error message to contain %q, got %v", expectedErrorMsg, err)
		}

		// Assert the project is nil
		if project != nil {
			t.Errorf("expected project to be nil, got %v", project)
		}
	})

	t.Run("EmptyContainerConfig", func(t *testing.T) {
		// Setup mock components
		mockInjector := di.NewMockInjector()
		mocks := setupSafeDockerContainerMocks(mockInjector)
		dockerVirt := NewDockerVirt(mockInjector)
		dockerVirt.Initialize()

		// Mock the service's GetComposeConfigFunc to return empty container configs and no error
		mocks.MockService.GetComposeConfigFunc = func() (*types.Config, error) {
			return nil, nil
		}

		// Call the getFullComposeConfig method
		project, err := dockerVirt.getFullComposeConfig()

		// Assert that no error occurred
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Assert the project is not nil
		if project == nil {
			t.Errorf("expected project to be non-nil, got nil")
		}

		// Assert the project has no services, volumes, or networks
		if len(project.Services) != 0 {
			t.Errorf("expected no services, got %d", len(project.Services))
		}
		if len(project.Volumes) != 0 {
			t.Errorf("expected no volumes, got %d", len(project.Volumes))
		}
		if len(project.Networks) != 1 {
			t.Errorf("expected no networks, got %d", len(project.Networks))
		}
	})

	t.Run("NetworkCIDRNotDefined", func(t *testing.T) {
		// Setup mock components
		mockInjector := di.NewMockInjector()
		mocks := setupSafeDockerContainerMocks(mockInjector)
		dockerVirt := NewDockerVirt(mockInjector)
		dockerVirt.Initialize()

		// Mock the network.cidr_block to return an empty string
		mocks.MockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "network.cidr_block" {
				return ""
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return "default-value"
		}

		// Call the getFullComposeConfig method
		project, err := dockerVirt.getFullComposeConfig()

		// Assert that no error occurred
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Assert the project is not nil
		if project == nil {
			t.Errorf("expected project to be non-nil, got nil")
		}

		// Assert the project has the expected number of services, volumes, and networks
		expectedServices := 2
		expectedVolumes := 2
		expectedNetworks := 3

		if len(project.Services) != expectedServices {
			t.Errorf("expected %d services, got %d", expectedServices, len(project.Services))
		}
		if len(project.Volumes) != expectedVolumes {
			t.Errorf("expected %d volumes, got %d", expectedVolumes, len(project.Volumes))
		}
		if len(project.Networks) != expectedNetworks {
			t.Errorf("expected %d networks, got %d", expectedNetworks, len(project.Networks))
		}
	})

	t.Run("WithDNS", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeDockerContainerMocks()
		dockerVirt := NewDockerVirt(mocks.Injector)
		dockerVirt.services = []services.Service{}         // Initialize empty services slice
		dockerVirt.configHandler = mocks.MockConfigHandler // Set the config handler

		// Configure mock behavior for DNS
		mocks.MockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "docker.enabled" {
				return true
			}
			if key == "dns.enabled" {
				return true
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return false
		}

		mocks.MockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "network.cidr_block" {
				return "10.0.0.0/24"
			}
			if key == "dns.address" {
				return "10.0.0.53"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		mocks.MockConfigHandler.GetContextFunc = func() string {
			return "mock-context"
		}

		// Setup mock DNS service
		mockDNS := services.NewMockService()
		mockDNS.GetAddressFunc = func() string {
			return "10.0.0.53"
		}
		mocks.Injector.Register("dns", mockDNS)

		// Call the function
		project, err := dockerVirt.getFullComposeConfig()

		// Assertions
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if project == nil {
			t.Fatal("expected project to be non-nil")
		}
		if project.Networks == nil {
			t.Fatal("expected networks to be non-nil")
		}

		// Check network configuration
		networkName := "windsor-mock-context"
		network, exists := project.Networks[networkName]
		if !exists {
			t.Fatalf("expected network %s to exist", networkName)
		}
		if network.Driver != "bridge" {
			t.Errorf("expected network driver to be bridge, got %s", network.Driver)
		}
		if network.Ipam.Config == nil {
			t.Fatal("expected Ipam config to be non-nil")
		}
		if len(network.Ipam.Config) != 1 {
			t.Fatalf("expected 1 Ipam config, got %d", len(network.Ipam.Config))
		}
		if network.Ipam.Config[0].Subnet != "10.0.0.0/24" {
			t.Errorf("expected subnet to be 10.0.0.0/24, got %s", network.Ipam.Config[0].Subnet)
		}
	})

	t.Run("Disabled", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeDockerContainerMocks()
		dockerVirt := NewDockerVirt(mocks.Injector)
		dockerVirt.services = []services.Service{}         // Initialize empty services slice
		dockerVirt.configHandler = mocks.MockConfigHandler // Set the config handler

		// Configure mock behavior
		mocks.MockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "docker.enabled" {
				return false
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return false
		}

		// Call the function
		project, err := dockerVirt.getFullComposeConfig()

		// Assertions
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if project != nil {
			t.Fatal("expected project to be nil")
		}
	})

	t.Run("WithServices", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeDockerContainerMocks()
		dockerVirt := NewDockerVirt(mocks.Injector)
		dockerVirt.Initialize()

		// Enable DNS in configuration
		mocks.MockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			if key == "docker.enabled" {
				return true
			}
			if key == "dns.enabled" {
				return true
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return false
		}

		// Create a mock service
		mockService := services.NewMockService()
		mockService.GetNameFunc = func() string {
			return "test-service"
		}
		mockService.GetAddressFunc = func() string {
			return "10.0.0.2"
		}
		mockService.GetComposeConfigFunc = func() (*types.Config, error) {
			return &types.Config{
				Services: []types.ServiceConfig{
					{
						Name:  "test-service",
						Image: "test-image:latest",
						Ports: []types.ServicePortConfig{
							{
								Published: "8080",
								Target:    80,
							},
						},
						Environment: map[string]*string{
							"ENV": ptrString("test"),
						},
						Volumes: []types.ServiceVolumeConfig{
							{
								Source: "/host:/container",
							},
						},
						DNS: []string{"10.0.0.53"},
					},
				},
			}, nil
		}

		// Register the mock service
		mocks.Injector.Register("test-service", mockService)
		dockerVirt.services = []services.Service{mockService}

		// Setup mock DNS service
		mockDNS := services.NewMockService()
		mockDNS.GetAddressFunc = func() string {
			return "10.0.0.53"
		}
		mocks.Injector.Register("dns", mockDNS)

		// Call the function
		project, err := dockerVirt.getFullComposeConfig()

		// Assertions
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if project == nil {
			t.Fatal("expected project to be non-nil")
		}
		if len(project.Services) != 1 {
			t.Fatalf("expected 1 service, got %d", len(project.Services))
		}

		service := project.Services[0]
		if service.Name != "test-service" {
			t.Errorf("expected service name to be test-service, got %s", service.Name)
		}
		if service.Image != "test-image:latest" {
			t.Errorf("expected image to be test-image:latest, got %s", service.Image)
		}
		if len(service.Ports) != 1 {
			t.Fatalf("expected 1 port, got %d", len(service.Ports))
		}
		if service.Ports[0].Published != "8080" || service.Ports[0].Target != 80 {
			t.Errorf("expected port to be 8080:80, got %s:%d", service.Ports[0].Published, service.Ports[0].Target)
		}
		if service.Environment["ENV"] == nil || *service.Environment["ENV"] != "test" {
			t.Errorf("expected environment ENV to be test, got %v", service.Environment["ENV"])
		}
		if len(service.Volumes) != 1 {
			t.Fatalf("expected 1 volume, got %d", len(service.Volumes))
		}
		if service.Volumes[0].Source != "/host:/container" {
			t.Errorf("expected volume source to be /host:/container, got %s", service.Volumes[0].Source)
		}
		if len(service.Networks) != 1 {
			t.Fatalf("expected 1 network, got %d", len(service.Networks))
		}
		if service.Networks["windsor-mock-context"] == nil {
			t.Error("expected network windsor-mock-context to exist")
		}
	})
}
