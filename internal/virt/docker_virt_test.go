package virt

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/compose-spec/compose-go/types"
	"github.com/windsorcli/cli/internal/config"
	"github.com/windsorcli/cli/internal/context"
	"github.com/windsorcli/cli/internal/di"
	"github.com/windsorcli/cli/internal/services"
	"github.com/windsorcli/cli/internal/shell"
)

func setupSafeDockerContainerMocks(optionalInjector ...di.Injector) *MockComponents {
	var injector di.Injector
	if len(optionalInjector) > 0 {
		injector = optionalInjector[0]
	} else {
		injector = di.NewMockInjector()
	}

	mockContext := context.NewMockContext()
	mockShell := shell.NewMockShell(injector)
	mockConfigHandler := config.NewMockConfigHandler()
	mockService := services.NewMockService()

	// Register mock instances in the injector
	injector.Register("contextHandler", mockContext)
	injector.Register("shell", mockShell)
	injector.Register("configHandler", mockConfigHandler)
	injector.Register("dockerService", mockService)

	// Register additional mock services
	mockService1 := services.NewMockService()
	mockService2 := services.NewMockService()
	injector.Register("service1", mockService1)
	injector.Register("service2", mockService2)

	// Implement GetContextFunc on mock context
	mockContext.GetContextFunc = func() string {
		return "mock-context"
	}

	// Set up the mock config handler to return a safe default configuration for Docker VMs
	mockConfigHandler.GetConfigFunc = func() *config.Context {
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
				NetworkCIDR: ptrString("10.5.0.0/16"),
			},
		}
	}

	// Mock the shell Exec function to return generic JSON structures for two containers
	mockShell.ExecFunc = func(message string, command string, args ...string) (string, error) {
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
				{Name: "service1"},
				{Name: "service2"},
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

	// Mock the GetConfigRootFunc to return a mock config root path
	mockContext.GetConfigRootFunc = func() (string, error) {
		return "/mock/config/root", nil
	}

	return &MockComponents{
		Injector:          injector,
		MockContext:       mockContext,
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

		// Mock the shell's Exec function to handle the callback
		execCalled := false
		mocks.MockShell.ExecFunc = func(message string, command string, args ...string) (string, error) {
			if command == "docker" && len(args) > 0 && args[0] == "info" {
				return "", nil // Simulate successful Docker daemon check
			}
			if command == "docker-compose" && len(args) > 0 && args[2] == "up" {
				execCalled = true
				return "", nil
			}
			return "", fmt.Errorf("unexpected command")
		}

		// Call the Up method
		err := dockerVirt.Up()

		// Assert no error occurred
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// Verify that the mock shell's Exec function was called with the expected command
		if !execCalled {
			t.Errorf("expected Exec to be called with 'docker-compose up', but it was not")
		}
	})

	t.Run("TestVerboseMode", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeDockerContainerMocks()
		dockerVirt := NewDockerVirt(mocks.Injector)
		dockerVirt.Initialize()

		// Mock the shell's Exec function to handle the callback
		execCalled := false
		mocks.MockShell.ExecFunc = func(message string, command string, args ...string) (string, error) {
			if command == "docker" && len(args) > 0 && args[0] == "info" {
				return "", nil // Simulate successful Docker daemon check
			}
			if command == "docker-compose" && len(args) > 0 && args[2] == "up" {
				execCalled = true
				return "", nil
			}
			return "", fmt.Errorf("unexpected command")
		}

		// Call the Up method with verbose mode enabled
		err := dockerVirt.Up()

		// Assert no error occurred
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// Verify that the mock shell's Exec function was called with the expected command
		if !execCalled {
			t.Errorf("expected Exec to be called with 'docker-compose up', but it was not")
		}
	})

	t.Run("DockerDaemonNotRunning", func(t *testing.T) {
		// Setup mock components without mocking the container
		mocks := setupSafeDockerContainerMocks()
		dockerVirt := NewDockerVirt(mocks.Injector)
		dockerVirt.Initialize()

		// Mock the shell Exec function to simulate the Docker daemon not running
		mocks.MockShell.ExecFunc = func(message string, command string, args ...string) (string, error) {
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

		// Mock the GetConfigRoot function to simulate an error
		mocks.MockContext.GetConfigRootFunc = func() (string, error) {
			return "", fmt.Errorf("mock error retrieving config root")
		}

		// Mock the shell Exec function to simulate Docker daemon check
		mocks.MockShell.ExecFunc = func(message string, command string, args ...string) (string, error) {
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
		expectedErrorMsg := "error retrieving config root"
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

		// Mock the shell Exec function to simulate retry logic
		mocks.MockShell.ExecFunc = func(message string, command string, args ...string) (string, error) {
			if command == "docker" && len(args) > 0 && args[0] == "info" {
				return "docker info", nil
			}
			if command == "docker-compose" && len(args) > 0 && args[2] == "up" {
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

		// Mock the shell Exec function to simulate retry logic with persistent error
		mocks.MockShell.ExecFunc = func(message string, command string, args ...string) (string, error) {
			if command == "docker" && len(args) > 0 && args[0] == "info" {
				return "docker info", nil
			}
			if command == "docker-compose" && len(args) > 2 && args[2] == "up" {
				execCallCount++
				if execCallCount < 3 {
					return "", fmt.Errorf("temporary error")
				}
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

		// Mock the shell Exec function to simulate successful docker info and docker-compose down commands
		mocks.MockShell.ExecFunc = func(message string, command string, args ...string) (string, error) {
			if command == "docker" && len(args) > 0 && args[0] == "info" {
				return "docker info", nil
			}
			if command == "docker-compose" && len(args) > 2 && args[2] == "down" {
				return "docker-compose down", nil
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
		mocks.MockShell.ExecFunc = func(message string, command string, args ...string) (string, error) {
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
		mocks.MockContext.GetConfigRootFunc = func() (string, error) {
			return "", fmt.Errorf("error retrieving config root")
		}

		// Mock the shell Exec function to simulate successful docker info command
		mocks.MockShell.ExecFunc = func(message string, command string, args ...string) (string, error) {
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
		expectedErrorMsg := "error retrieving config root"
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
		mocks.MockShell.ExecFunc = func(message string, command string, args ...string) (string, error) {
			if command == "docker" && len(args) > 0 && args[0] == "info" {
				return "docker info", nil
			}
			if command == "docker-compose" && len(args) > 0 && args[0] == "-f" && args[2] == "down" {
				return "", fmt.Errorf("error executing docker-compose down")
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
		expectedErrorMsg := "error executing docker-compose down"
		if err != nil && !strings.Contains(err.Error(), expectedErrorMsg) {
			t.Errorf("expected error message to contain %q, got %v", expectedErrorMsg, err)
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
		originalExecFunc := mocks.MockShell.ExecFunc
		mocks.MockShell.ExecFunc = func(message string, command string, args ...string) (string, error) {
			if command == "docker" && len(args) > 0 {
				switch args[0] {
				case "inspect":
					if len(args) > 2 && args[2] == "--format" {
						return "", fmt.Errorf("mock error inspecting container")
					}
				}
			}
			// Call the original ExecFunc for any other cases
			return originalExecFunc(message, command, args...)
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
		originalExecFunc := mocks.MockShell.ExecFunc
		mocks.MockShell.ExecFunc = func(message string, command string, args ...string) (string, error) {
			if command == "docker" && len(args) > 0 {
				switch args[0] {
				case "inspect":
					if len(args) > 2 && args[2] == "--format" {
						return "{invalid-json}", nil // Return invalid JSON to trigger unmarshalling error
					}
				}
			}
			// Call the original ExecFunc for any other cases
			return originalExecFunc(message, command, args...)
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
		mocks.MockShell.ExecFunc = func(message string, command string, args ...string) (string, error) {
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
		originalExecFunc := mocks.MockShell.ExecFunc
		mocks.MockShell.ExecFunc = func(message string, command string, args ...string) (string, error) {
			if command == "docker" && len(args) > 0 && args[0] == "inspect" && args[2] == "--format" && args[3] == "{{json .NetworkSettings.Networks}}" {
				return "", fmt.Errorf("mock error inspecting network")
			}
			return originalExecFunc(message, command, args...)
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
		originalExecFunc := mocks.MockShell.ExecFunc
		mocks.MockShell.ExecFunc = func(message string, command string, args ...string) (string, error) {
			if command == "docker" && len(args) > 0 && args[0] == "inspect" && args[2] == "--format" && args[3] == "{{json .NetworkSettings.Networks}}" {
				return `invalid json`, nil
			}
			return originalExecFunc(message, command, args...)
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
		mocks.MockShell.ExecFunc = func(message string, command string, args ...string) (string, error) {
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
		mocks.MockShell.ExecFunc = func(message string, command string, args ...string) (string, error) {
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
			if filepath.Clean(path) == filepath.FromSlash("/mock/config/root") {
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
		mocks.MockContext.GetConfigRootFunc = func() (string, error) {
			return "", fmt.Errorf("error retrieving config root")
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
		expectedErrorMsg := "error retrieving config root"
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
		mocks.MockShell.ExecFunc = func(message string, command string, args ...string) (string, error) {
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
		mocks.MockShell.ExecFunc = func(message string, command string, args ...string) (string, error) {
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
		mocks.MockConfigHandler.GetConfigFunc = func() *config.Context {
			return &config.Context{
				Docker: nil, // No Docker configuration
			}
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

		// Mock the context configuration to have no NetworkCIDR defined
		mocks.MockConfigHandler.GetConfigFunc = func() *config.Context {
			return &config.Context{
				Docker: &config.DockerConfig{
					NetworkCIDR: nil,
				},
			}
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
}
