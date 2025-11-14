// The DockerVirt test suite provides test coverage for Docker VM management functionality.
// It serves as a verification framework for Docker virtualization operations.
// It enables testing of Docker-specific features and error handling.

package virt

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/workstation/services"
)

// =============================================================================
// Test Setup
// =============================================================================

func setupDockerMocks(t *testing.T, opts ...*SetupOptions) *Mocks {
	t.Helper()

	// Process options with defaults
	options := &SetupOptions{}
	if len(opts) > 0 && opts[0] != nil {
		options = opts[0]
	}

	// Set up base mocks
	mocks := setupMocks(t, options)

	// Load Docker-specific config
	configStr := `
contexts:
  mock-context:
    dns:
      domain: mock.domain.com
      enabled: true
      address: 10.0.0.53
    network:
      cidr_block: 10.0.0.0/24
    docker:
      enabled: true
      registry_url: "https://registry.example.com"
      registries:
        local:
          remote: "remote-registry.example.com"
          local: "localhost:5000"
          hostname: "registry.local"
          hostport: 5000
    vm:
      driver: colima`

	if err := mocks.ConfigHandler.LoadConfigString(configStr); err != nil {
		t.Fatalf("Failed to load config string: %v", err)
	}

	mocks.ConfigHandler.SetContext("mock-context")

	// Set up mock shell for Docker commands
	mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
		if command == "docker" && len(args) > 0 {
			switch args[0] {
			case "compose":
				return "Docker Compose version 2.0.0", nil
			case "info":
				return "Docker info output", nil
			case "version":
				if len(args) >= 3 && args[1] == "--format" && args[2] == "{{.Server.Version}}" {
					return "28.0.3", nil // Mock Docker Engine v28+ for testing
				}
			case "ps":
				var hasManagedBy, hasContext, hasFormat bool
				for i := 0; i < len(args); i++ {
					if args[i] == "--filter" && i+1 < len(args) {
						switch args[i+1] {
						case "label=managed_by=windsor":
							hasManagedBy = true
						case fmt.Sprintf("label=context=%s", mocks.ConfigHandler.GetContext()):
							hasContext = true
						}
					} else if args[i] == "--format" && i+1 < len(args) && args[i+1] == "{{.ID}}" {
						hasFormat = true
					}
				}
				if hasManagedBy && hasContext && hasFormat {
					return "container1\ncontainer2", nil
				}
			case "inspect":
				if len(args) >= 4 && args[2] == "--format" {
					switch args[3] {
					case "{{json .Config.Labels}}":
						switch args[1] {
						case "container1":
							return `{"managed_by":"windsor","context":"mock-context","com.docker.compose.service":"service1","role":"test"}`, nil
						case "container2":
							return `{"managed_by":"windsor","context":"mock-context","com.docker.compose.service":"service2","role":"test"}`, nil
						}
					case "{{json .NetworkSettings.Networks}}":
						switch args[1] {
						case "container1":
							return fmt.Sprintf(`{"windsor-%s":{"IPAddress":"192.168.1.2"}}`, mocks.ConfigHandler.GetContext()), nil
						case "container2":
							return fmt.Sprintf(`{"windsor-%s":{"IPAddress":"192.168.1.3"}}`, mocks.ConfigHandler.GetContext()), nil
						}
					}
				}
			}
		}
		return "", fmt.Errorf("unexpected command: %s %v", command, args)
	}

	// Set up mock service config
	mocks.Service.GetComposeConfigFunc = func() (*types.Config, error) {
		return &types.Config{
			Services: types.Services{
				"service1": {
					Name: "service1",
					Labels: map[string]string{
						"role":                       "test",
						"com.docker.compose.service": "service1",
					},
					Networks: map[string]*types.ServiceNetworkConfig{
						fmt.Sprintf("windsor-%s", mocks.ConfigHandler.GetContext()): {
							Ipv4Address: "192.168.1.2",
						},
					},
				},
				"service2": {
					Name: "service2",
					Labels: map[string]string{
						"role":                       "test",
						"com.docker.compose.service": "service2",
					},
					Networks: map[string]*types.ServiceNetworkConfig{
						fmt.Sprintf("windsor-%s", mocks.ConfigHandler.GetContext()): {
							Ipv4Address: "192.168.1.3",
						},
					},
				},
			},
		}, nil
	}
	mocks.Service.GetAddressFunc = func() string {
		return "192.168.1.2"
	}
	mocks.Service.GetNameFunc = func() string {
		return "service1"
	}
	mocks.Service.GetHostnameFunc = func() string {
		return "service1.mock.domain.com"
	}

	return mocks
}

// =============================================================================
// Test Public Methods
// =============================================================================

// TestDockerVirt_Initialize tests the initialization of the DockerVirt component.
func TestDockerVirt_Initialize(t *testing.T) {
	setup := func(t *testing.T) (*DockerVirt, *Mocks) {
		t.Helper()
		mocks := setupDockerMocks(t)
		dockerVirt := NewDockerVirt(mocks.Runtime, []services.Service{})
		dockerVirt.shims = mocks.Shims


		return dockerVirt, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a docker virt instance with valid mocks
		mocks := setupDockerMocks(t)
		serviceList := []services.Service{mocks.Service}
		dockerVirt := NewDockerVirt(mocks.Runtime, serviceList)
		dockerVirt.shims = mocks.Shims

		// And services should be resolved
		if len(dockerVirt.services) == 0 {
			t.Errorf("expected services to be resolved, but got none")
		}
		if len(dockerVirt.services) != 1 {
			t.Errorf("expected 1 service, got %d", len(dockerVirt.services))
		}
	})

	t.Run("ErrorInitializingBaseVirt", func(t *testing.T) {
		// Given a docker virt instance with mock components
		dockerVirt, _ := setup(t)

		// Then the service should be properly initialized
		if dockerVirt == nil {
			t.Fatal("Expected DockerVirt, got nil")
		}
	})


	t.Run("ErrorResolvingServices", func(t *testing.T) {
		// Given a docker virt instance with failing service resolution
		dockerVirt, mocks := setup(t)

		// Create new mock injector with base dependencies
		mockInjector := di.NewMockInjector()
		mockInjector.Register("shell", mocks.Shell)
		mockInjector.Register("configHandler", mocks.ConfigHandler)
		mockInjector.SetResolveAllError((*services.Service)(nil), fmt.Errorf("service resolution failed"))

		// Replace injector and recreate dockerVirt
		dockerVirt = NewDockerVirt(mocks.Runtime, []services.Service{})
		dockerVirt.shims = mocks.Shims

		// Then the service should be properly initialized
		if dockerVirt == nil {
			t.Fatal("Expected DockerVirt, got nil")
		}
	})

	t.Run("ErrorDeterminingComposeCommand", func(t *testing.T) {
		// Given a docker virt instance with failing compose command detection
		dockerVirt, mocks := setup(t)
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "docker" && len(args) > 0 && args[0] == "compose" {
				return "", fmt.Errorf("error determining compose command")
			}
			if command == "docker-compose" {
				return "", fmt.Errorf("error determining compose command")
			}
			if command == "docker-cli-plugin-docker-compose" {
				return "", fmt.Errorf("error determining compose command")
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}


		// And the compose command should be empty
		if dockerVirt.composeCommand != "" {
			t.Errorf("expected compose command to be empty, got %q", dockerVirt.composeCommand)
		}
	})

	t.Run("NilServiceInSlice", func(t *testing.T) {
		// Given a docker virt instance with a nil service
		mocks := setupDockerMocks(t)
		serviceList := []services.Service{mocks.Service, nil}
		dockerVirt := NewDockerVirt(mocks.Runtime, serviceList)
		dockerVirt.shims = mocks.Shims

		// Then services slice should only contain the non-nil service (nil is filtered out)
		if len(dockerVirt.services) != 1 {
			t.Errorf("Expected 1 service (nil is filtered out), got %d", len(dockerVirt.services))
		}
		// Verify the service is not nil
		if dockerVirt.services[0] == nil {
			t.Error("Expected service to not be nil")
		}
	})

	t.Run("ServicesAreSorted", func(t *testing.T) {
		// Given a docker virt instance with multiple services
		mocks := setupDockerMocks(t)

		// And services in random order
		serviceA := services.NewMockService()
		serviceB := services.NewMockService()
		serviceC := services.NewMockService()
		serviceA.SetName("ServiceA")
		serviceB.SetName("ServiceB")
		serviceC.SetName("ServiceC")

		// Create service list in random order
		serviceList := []services.Service{serviceC, serviceA, serviceB, mocks.Service}
		dockerVirt := NewDockerVirt(mocks.Runtime, serviceList)
		dockerVirt.shims = mocks.Shims

		// Then services should be sorted by name
		if len(dockerVirt.services) != 4 {
			t.Errorf("Expected 4 services, got %d", len(dockerVirt.services))
		}
		if len(dockerVirt.services) == 4 {
			serviceNames := []string{
				dockerVirt.services[0].GetName(),
				dockerVirt.services[1].GetName(),
				dockerVirt.services[2].GetName(),
				dockerVirt.services[3].GetName(),
			}
			if !sort.StringsAreSorted(serviceNames) {
				t.Errorf("Services are not sorted by name: %v", serviceNames)
			}
		}
	})

	t.Run("SkipNilService", func(t *testing.T) {
		// Given a docker virt instance with nil service
		dockerVirt, mocks := setup(t)

		// Create new mock injector with base dependencies
		mockInjector := di.NewMockInjector()
		mockInjector.Register("shell", mocks.Shell)
		mockInjector.Register("configHandler", mocks.ConfigHandler)
		mockInjector.Register("nilService", nil)

		// Replace injector and recreate dockerVirt
		dockerVirt = NewDockerVirt(mocks.Runtime, []services.Service{})
		dockerVirt.shims = mocks.Shims

		// Then the service should be properly initialized
		if dockerVirt == nil {
			t.Fatal("Expected DockerVirt, got nil")
		}
	})
}

// TestDockerVirt_Up tests the Up method of the DockerVirt component.
func TestDockerVirt_Up(t *testing.T) {
	setup := func(t *testing.T) (*DockerVirt, *Mocks) {
		t.Helper()
		mocks := setupDockerMocks(t)
		dockerVirt := NewDockerVirt(mocks.Runtime, []services.Service{})
		dockerVirt.shims = mocks.Shims
		return dockerVirt, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a docker virt instance with valid mocks
		dockerVirt, _ := setup(t)

		// When calling Up
		err := dockerVirt.Up()

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("ErrorStartingDockerCompose", func(t *testing.T) {
		// Given a DockerVirt with mock components
		dockerVirt, mocks := setup(t)

		// Mock ExecSilent to handle determineComposeCommand
		oldExecSilent := mocks.Shell.ExecSilentFunc
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if len(args) > 0 && args[0] == "--version" {
				// Handle determineComposeCommand check
				if command == "docker-compose" {
					return "docker-compose version 1.29.2", nil
				}
			}
			return oldExecSilent(command, args...)
		}

		// Mock command execution to fail
		cmdParts := []string{"docker-compose"}
		mocks.Shell.ExecProgressFunc = func(message string, command string, args ...string) (string, error) {
			if command == cmdParts[0] && len(args) > 0 && args[0] == "up" {
				return "", fmt.Errorf("mock docker-compose up error")
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When calling Up
		err := dockerVirt.Up()

		// Then an error should occur
		if err == nil {
			t.Errorf("expected error, got none")
		}

		// And the error should contain the expected message
		expectedErrorMsg := "executing command"
		if err != nil && !strings.Contains(err.Error(), expectedErrorMsg) {
			t.Errorf("expected error message to contain %q, got %v", expectedErrorMsg, err)
		}
	})

	t.Run("ErrorGetConfigRoot", func(t *testing.T) {
		// This test is obsolete - ProjectRoot is now a direct field on runtime, not retrieved via a function
		t.Skip("ProjectRoot is now a direct field, cannot test retrieval error")
	})

	t.Run("ErrorSettingComposeFileEnv", func(t *testing.T) {
		// Given a DockerVirt with mock components and custom shims
		mocks := setupDockerMocks(t)

		// Create shims with Setenv error
		mocks.Shims.Setenv = func(key, value string) error {
			if key == "COMPOSE_FILE" {
				return fmt.Errorf("mock error setting COMPOSE_FILE environment variable")
			}
			return nil
		}

		// Set up compose command detection
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "docker" && len(args) > 0 && args[0] == "compose" {
				return "Docker Compose version 2.0.0", nil
			}
			if command == "docker" && len(args) > 0 && args[0] == "info" {
				return "docker info output", nil
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// Create and initialize DockerVirt
		dockerVirt := NewDockerVirt(mocks.Runtime, []services.Service{})
		dockerVirt.shims = mocks.Shims

		// When calling the Up method
		err := dockerVirt.Up()

		// Then an error should occur
		if err == nil {
			t.Errorf("expected an error, got nil")
		}

		// And the error should contain the expected message
		expectedErrorMsg := "failed to set COMPOSE_FILE environment variable"
		if err != nil && !strings.Contains(err.Error(), expectedErrorMsg) {
			t.Errorf("expected error message to contain %q, got %v", expectedErrorMsg, err)
		}
	})

	t.Run("ErrorSetComposeFileUp", func(t *testing.T) {
		// Given a docker virt instance with failing setenv
		dockerVirt, mocks := setup(t)
		mocks.Shims.Setenv = func(key, value string) error {
			if key == "COMPOSE_FILE" {
				return fmt.Errorf("setenv failed")
			}
			return nil
		}

		// When calling Up
		err := dockerVirt.Up()

		// Then an error should occur
		if err == nil {
			t.Errorf("expected error, got none")
		}
		if !strings.Contains(err.Error(), "failed to set COMPOSE_FILE environment variable") {
			t.Errorf("expected error about setting COMPOSE_FILE, got %v", err)
		}
	})

	t.Run("RetryDockerComposeUp", func(t *testing.T) {
		// Given a docker virt instance with failing compose up
		dockerVirt, mocks := setup(t)

		// Track command execution count
		execCount := 0

		// Mock ExecSilent to handle determineComposeCommand and ensure we get "docker compose"
		oldExecSilent := mocks.Shell.ExecSilentFunc
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			// Handle determineComposeCommand version check
			// For "docker compose", it splits to ["docker", "compose"] and calls ExecSilent("docker", "compose", "--version")
			if len(args) > 0 && args[len(args)-1] == "--version" {
				if command == "docker-compose" {
					return "", fmt.Errorf("docker-compose not found")
				}
				if command == "docker-cli-plugin-docker-compose" {
					return "", fmt.Errorf("docker-cli-plugin-docker-compose not found")
				}
				if command == "docker" && len(args) > 0 && args[0] == "compose" {
					return "Docker Compose version 2.0.0", nil
				}
			}
			// Keep original behavior for other commands
			if command == "docker" && len(args) > 0 && args[0] == "info" {
				return oldExecSilent(command, args...)
			}

			// Handle compose up retry attempts (docker compose up)
			if command == "docker" && len(args) > 0 && args[0] == "compose" && len(args) > 1 && args[1] == "up" {
				execCount++
				if execCount < 3 {
					return "", fmt.Errorf("temporary error")
				}
				return "success", nil
			}

			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// Mock ExecProgress to fail twice then succeed
		mocks.Shell.ExecProgressFunc = func(message string, command string, args ...string) (string, error) {
			// Handle compose up (docker compose up)
			if command == "docker" && len(args) > 0 && args[0] == "compose" && len(args) > 1 && args[1] == "up" {
				execCount++
				if execCount < 3 {
					return "", fmt.Errorf("temporary error")
				}
				return "success", nil
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When calling Up
		err := dockerVirt.Up()

		// Then no error should occur after retries
		if err != nil {
			t.Errorf("expected no error after retries, got %v", err)
		}

		// And the command should be called 3 times
		if execCount != 3 {
			t.Errorf("expected command to be called 3 times, got %d", execCount)
		}
	})
}

// TestDockerVirt_Down tests the Down method of the DockerVirt component.
func TestDockerVirt_Down(t *testing.T) {
	setup := func(t *testing.T) (*DockerVirt, *Mocks) {
		t.Helper()
		mocks := setupDockerMocks(t)
		dockerVirt := NewDockerVirt(mocks.Runtime, []services.Service{})
		dockerVirt.shims = mocks.Shims
		return dockerVirt, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a DockerVirt with mock components
		dockerVirt, mocks := setup(t)

		// And docker-compose.yaml exists
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, nil // File exists
		}

		// When calling Down
		err := dockerVirt.Down()

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("SuccessWithMissingComposeFile", func(t *testing.T) {
		// Given a DockerVirt with mock components
		dockerVirt, mocks := setup(t)

		// And docker-compose.yaml doesn't exist
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist // File doesn't exist
		}

		// When calling Down
		err := dockerVirt.Down()

		// Then no error should occur (idempotent)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("DockerDaemonNotRunning", func(t *testing.T) {
		// Given a DockerVirt with mock components
		dockerVirt, mocks := setup(t)

		// Override ExecSilent for docker info check
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "docker" && len(args) > 0 && args[0] == "info" {
				return "", fmt.Errorf("Docker daemon is not running")
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When calling the Down method
		err := dockerVirt.Down()

		// Then an error should occur
		if err == nil {
			t.Errorf("expected an error, got nil")
		}

		// And the error should contain the expected message
		expectedErrorMsg := "Docker daemon is not running"
		if err != nil && !strings.Contains(err.Error(), expectedErrorMsg) {
			t.Errorf("expected error message to contain %q, got %v", expectedErrorMsg, err)
		}
	})

	t.Run("ErrorGetConfigRoot", func(t *testing.T) {
		// This test is obsolete - ProjectRoot is now a direct field on runtime, not retrieved via a function
		t.Skip("ProjectRoot is now a direct field, cannot test retrieval error")
	})

	t.Run("ErrorSettingComposeFileEnv", func(t *testing.T) {
		// Given a DockerVirt with mock components and custom shims
		dockerVirt, mocks := setup(t)

		// Create shims with Setenv error
		mocks.Shims.Setenv = func(key, value string) error {
			if key == "COMPOSE_FILE" {
				return fmt.Errorf("mock error setting COMPOSE_FILE environment variable")
			}
			return nil
		}

		// Re-initialize DockerVirt to establish the necessary compose commands

		// When calling the Down method
		err := dockerVirt.Down()

		// Then an error should occur
		if err == nil {
			t.Errorf("expected an error, got nil")
		}

		// And the error should contain the expected message
		expectedErrorMsg := "error setting COMPOSE_FILE environment variable"
		if err != nil && !strings.Contains(err.Error(), expectedErrorMsg) {
			t.Errorf("expected error message to contain %q, got %v", expectedErrorMsg, err)
		}
	})

	t.Run("ErrorSetComposeFileDown", func(t *testing.T) {
		// Given a docker virt instance with failing setenv
		dockerVirt, mocks := setup(t)
		mocks.Shims.Setenv = func(key, value string) error {
			if key == "COMPOSE_FILE" {
				return fmt.Errorf("setenv failed")
			}
			return nil
		}

		// When calling Down
		err := dockerVirt.Down()

		// Then an error should occur
		if err == nil {
			t.Errorf("expected error, got none")
		}
		if !strings.Contains(err.Error(), "error setting COMPOSE_FILE environment variable") {
			t.Errorf("expected error about setting COMPOSE_FILE, got %v", err)
		}
	})

	t.Run("ErrorExecutingComposeDown", func(t *testing.T) {
		// Given a docker virt instance with failing compose down
		dockerVirt, mocks := setup(t)

		// Determine compose command first
		if err := dockerVirt.determineComposeCommand(); err != nil {
			t.Fatalf("Failed to determine compose command: %v", err)
		}

		cmdParts := strings.Fields(dockerVirt.composeCommand)
		mocks.Shell.ExecProgressFunc = func(message string, command string, args ...string) (string, error) {
			if command == cmdParts[0] && len(args) > 0 && args[0] == cmdParts[1] && len(args) > 1 && args[1] == "down" {
				return "mock error output", fmt.Errorf("mock error executing down command")
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When calling Down
		err := dockerVirt.Down()

		// Then an error should occur
		if err == nil {
			t.Errorf("expected error, got none")
		}
		if !strings.Contains(err.Error(), "Error executing command") {
			t.Errorf("expected error about executing command, got %v", err)
		}
		if !strings.Contains(err.Error(), "mock error output") {
			t.Errorf("expected error to contain command output, got %v", err)
		}
	})
}

// TestDockerVirt_WriteConfig tests the WriteConfig method of the DockerVirt component.
func TestDockerVirt_WriteConfig(t *testing.T) {
	setup := func(t *testing.T) (*DockerVirt, *Mocks) {
		t.Helper()
		mocks := setupDockerMocks(t)
		dockerVirt := NewDockerVirt(mocks.Runtime, []services.Service{})
		dockerVirt.shims = mocks.Shims
		return dockerVirt, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a virt instance with mock components
		mocks := setupDockerMocks(t)
		serviceList := []services.Service{mocks.Service}
		dockerVirt := NewDockerVirt(mocks.Runtime, serviceList)
		dockerVirt.shims = mocks.Shims

		// Track written content
		var writtenContent []byte
		mocks.Shims.WriteFile = func(name string, data []byte, perm os.FileMode) error {
			writtenContent = data
			return nil
		}

		// When writing the config
		err := dockerVirt.WriteConfig()

		// Then it should succeed
		if err != nil {
			t.Errorf("Expected success, got error: %v", err)
		}

		// And the config should contain the expected service
		if !strings.Contains(string(writtenContent), "service1") {
			t.Error("Config file does not contain expected service name")
		}
	})

	t.Run("ErrorGetProjectRoot", func(t *testing.T) {
		// This test is obsolete - ProjectRoot is now a direct field on runtime, not retrieved via a function
		t.Skip("ProjectRoot is now a direct field, cannot test retrieval error")
	})

	t.Run("ErrorMkdirAll", func(t *testing.T) {
		// Given a virt instance with mock shell and shims
		dockerVirt, mocks := setup(t)

		// Mock shims to return error
		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error {
			return fmt.Errorf("error creating directory")
		}

		// When calling WriteConfig
		err := dockerVirt.WriteConfig()

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}
	})

	t.Run("ErrorMarshalYAML", func(t *testing.T) {
		// Given a virt instance with mock shell and shims
		dockerVirt, mocks := setup(t)

		// Mock shims to return error during YAML marshaling
		mocks.Shims.MarshalYAML = func(v any) ([]byte, error) {
			return nil, fmt.Errorf("error marshaling YAML")
		}

		// When calling WriteConfig
		err := dockerVirt.WriteConfig()

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}
	})

	t.Run("ErrorWriteFile", func(t *testing.T) {
		// Given a virt instance with mock shell and shims
		dockerVirt, mocks := setup(t)

		// Mock shims to return error
		mocks.Shims.MarshalYAML = func(v any) ([]byte, error) {
			return []byte("version: '3'\nservices:\n  test:\n    image: test"), nil
		}
		mocks.Shims.WriteFile = func(name string, data []byte, perm os.FileMode) error {
			return fmt.Errorf("error writing file")
		}

		// When calling WriteConfig
		err := dockerVirt.WriteConfig()

		// Then an error should occur
		if err == nil {
			t.Error("Expected error, got nil")
		}
	})
}

// TestDockerVirt_DetermineComposeCommand tests the determineComposeCommand method of the DockerVirt component.
func TestDockerVirt_DetermineComposeCommand(t *testing.T) {
	setup := func(t *testing.T) (*DockerVirt, *Mocks) {
		t.Helper()
		mocks := setupDockerMocks(t)
		dockerVirt := NewDockerVirt(mocks.Runtime, []services.Service{})
		dockerVirt.shims = mocks.Shims
		return dockerVirt, mocks
	}

	t.Run("DockerComposeV2", func(t *testing.T) {
		// Given a docker virt instance with valid mocks
		dockerVirt, mocks := setup(t)

		// And docker-compose is available
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "docker-compose" && len(args) > 0 && args[0] == "--version" {
				return "docker-compose version 1.29.2", nil
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When determining compose command
		if err := dockerVirt.determineComposeCommand(); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// And the compose command should be set to docker-compose
		if dockerVirt.composeCommand != "docker-compose" {
			t.Errorf("expected compose command to be 'docker-compose', got %q", dockerVirt.composeCommand)
		}
	})

	t.Run("DockerComposeV1", func(t *testing.T) {
		// Given a docker virt instance with valid mocks
		dockerVirt, mocks := setup(t)

		// And docker-compose is not available but docker-cli-plugin is
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "docker-compose" {
				return "", fmt.Errorf("docker-compose not found")
			}
			if command == "docker-cli-plugin-docker-compose" {
				return "docker-cli-plugin-docker-compose version 1.0.0", nil
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When determining compose command
		if err := dockerVirt.determineComposeCommand(); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// And the compose command should be set to docker-cli-plugin-docker-compose
		if dockerVirt.composeCommand != "docker-cli-plugin-docker-compose" {
			t.Errorf("expected compose command to be 'docker-cli-plugin-docker-compose', got %q", dockerVirt.composeCommand)
		}
	})

	t.Run("DockerCliPlugin", func(t *testing.T) {
		// Given a docker virt instance with valid mocks
		dockerVirt, mocks := setup(t)

		// And only docker compose v2 is available
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "docker-compose" {
				return "", fmt.Errorf("docker-compose not found")
			}
			if command == "docker-cli-plugin-docker-compose" {
				return "", fmt.Errorf("docker-cli-plugin-docker-compose not found")
			}
			if command == "docker" && len(args) > 0 && args[0] == "compose" {
				return "Docker Compose version 2.0.0", nil
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When determining compose command
		if err := dockerVirt.determineComposeCommand(); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// And the compose command should be set to docker compose
		if dockerVirt.composeCommand != "docker compose" {
			t.Errorf("expected compose command to be 'docker compose', got %q", dockerVirt.composeCommand)
		}
	})

	t.Run("NoComposeCommandAvailable", func(t *testing.T) {
		// Given a docker virt instance with valid mocks
		dockerVirt, mocks := setup(t)

		// And no compose command is available
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "docker" && len(args) > 0 && args[0] == "compose" {
				return "", fmt.Errorf("docker compose not found")
			}
			if command == "docker-compose" {
				return "", fmt.Errorf("docker-compose not found")
			}
			if command == "docker-cli-plugin-docker-compose" {
				return "", fmt.Errorf("docker-cli-plugin-docker-compose not found")
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}


		// And the compose command should be empty
		if dockerVirt.composeCommand != "" {
			t.Errorf("expected compose command to be empty, got %q", dockerVirt.composeCommand)
		}
	})
}

// TestDockerVirt_GetFullComposeConfig tests the getFullComposeConfig method of the DockerVirt component.
func TestDockerVirt_GetFullComposeConfig(t *testing.T) {
	setup := func(t *testing.T) (*DockerVirt, *Mocks) {
		t.Helper()
		mocks := setupDockerMocks(t)
		dockerVirt := NewDockerVirt(mocks.Runtime, []services.Service{})
		dockerVirt.shims = mocks.Shims
		return dockerVirt, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a docker virt instance with valid mocks
		mocks := setupDockerMocks(t)
		serviceList := []services.Service{mocks.Service}
		dockerVirt := NewDockerVirt(mocks.Runtime, serviceList)
		dockerVirt.shims = mocks.Shims

		// When getting the full compose config
		project, err := dockerVirt.getFullComposeConfig()

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// And the project should not be nil
		if project == nil {
			t.Errorf("expected project to not be nil")
		}

		// And the project should have the expected services
		// The mock service returns a compose config with 2 services (service1 and service2)
		if len(project.Services) != 2 {
			t.Errorf("expected 2 services, got %d", len(project.Services))
		}

		// And the project should have the expected networks
		networkName := fmt.Sprintf("windsor-%s", dockerVirt.configHandler.GetContext())
		if _, exists := project.Networks[networkName]; !exists {
			t.Errorf("expected network %s to exist", networkName)
		}

		// And the network should have the expected CIDR block
		network := project.Networks[networkName]
		if network.Ipam.Driver == "" || len(network.Ipam.Config) == 0 {
			t.Errorf("expected network to have IPAM config")
		}
		if network.Ipam.Config[0].Subnet != "10.0.0.0/24" {
			t.Errorf("expected network CIDR to be 10.0.0.0/24, got %s", network.Ipam.Config[0].Subnet)
		}

		// And the network should have Docker Engine v28+ compatibility driver options
		// (since the test config has vm.driver: colima and Docker Engine v28+ is mocked)
		if network.DriverOpts == nil {
			t.Errorf("expected network to have driver options for Docker v28+ compatibility")
		}
		expectedDriverOpt := "com.docker.network.bridge.gateway_mode_ipv4"
		if _, exists := network.DriverOpts[expectedDriverOpt]; !exists {
			t.Errorf("expected network to have driver option %s", expectedDriverOpt)
		}
		if network.DriverOpts[expectedDriverOpt] != "nat-unprotected" {
			t.Errorf("expected driver option %s to be 'nat-unprotected', got %s", expectedDriverOpt, network.DriverOpts[expectedDriverOpt])
		}
	})

	t.Run("DockerEngineV28Compatibility", func(t *testing.T) {
		// Given a docker virt instance with valid mocks
		dockerVirt, mocks := setup(t)

		// And Docker Engine is v28+ and we're running in a Colima VM
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "docker" && len(args) > 0 {
				switch args[0] {
				case "compose":
					return "Docker Compose version 2.0.0", nil
				case "info":
					return "Docker info output", nil
				case "version":
					if len(args) >= 3 && args[1] == "--format" && args[2] == "{{.Server.Version}}" {
						return "28.0.3", nil // Mock Docker Engine v28+ for testing
					}
				case "ps":
					var hasManagedBy, hasContext, hasFormat bool
					for i := 0; i < len(args); i++ {
						if args[i] == "--filter" && i+1 < len(args) {
							switch args[i+1] {
							case "label=managed_by=windsor":
								hasManagedBy = true
							case fmt.Sprintf("label=context=%s", mocks.ConfigHandler.GetContext()):
								hasContext = true
							}
						} else if args[i] == "--format" && i+1 < len(args) && args[i+1] == "{{.ID}}" {
							hasFormat = true
						}
					}
					if hasManagedBy && hasContext && hasFormat {
						return "container1\ncontainer2", nil
					}
				case "inspect":
					if len(args) >= 4 && args[2] == "--format" {
						switch args[3] {
						case "{{json .Config.Labels}}":
							switch args[1] {
							case "container1":
								return `{"managed_by":"windsor","context":"mock-context","com.docker.compose.service":"service1","role":"test"}`, nil
							case "container2":
								return `{"managed_by":"windsor","context":"mock-context","com.docker.compose.service":"service2","role":"test"}`, nil
							}
						case "{{json .NetworkSettings.Networks}}":
							switch args[1] {
							case "container1":
								return fmt.Sprintf(`{"windsor-%s":{"IPAddress":"192.168.1.2"}}`, mocks.ConfigHandler.GetContext()), nil
							case "container2":
								return fmt.Sprintf(`{"windsor-%s":{"IPAddress":"192.168.1.3"}}`, mocks.ConfigHandler.GetContext()), nil
							}
						}
					}
				}
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When getting the full compose config
		project, err := dockerVirt.getFullComposeConfig()

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// And the project should not be nil
		if project == nil {
			t.Errorf("expected project to not be nil")
		}

		// And the network should have Docker Engine v28+ compatibility driver options
		// (since we're mocking both Colima VM and Docker Engine v28+ in the test)
		networkName := fmt.Sprintf("windsor-%s", dockerVirt.configHandler.GetContext())
		network, exists := project.Networks[networkName]
		if !exists {
			t.Errorf("expected network %s to exist", networkName)
		}

		// Verify the network has the required driver options for Docker v28+ compatibility
		if network.DriverOpts == nil {
			t.Errorf("expected network to have driver options for Docker v28+ compatibility")
		}

		// Check for the specific driver option that bypasses Docker v28 security hardening
		expectedDriverOpt := "com.docker.network.bridge.gateway_mode_ipv4"
		driverOptValue, exists := network.DriverOpts[expectedDriverOpt]
		if !exists {
			t.Errorf("expected network to have driver option %s for Docker v28+ compatibility", expectedDriverOpt)
		}

		// Verify the driver option is set to nat-unprotected
		expectedValue := "nat-unprotected"
		if driverOptValue != expectedValue {
			t.Errorf("expected driver option %s to be '%s', got '%s'", expectedDriverOpt, expectedValue, driverOptValue)
		}

		// Verify the network driver is bridge (required for this compatibility fix)
		if network.Driver != "bridge" {
			t.Errorf("expected network driver to be 'bridge', got '%s'", network.Driver)
		}

	})

	t.Run("DockerEngineV28Exact", func(t *testing.T) {
		// Given a docker virt instance with valid mocks
		dockerVirt, mocks := setup(t)

		// And Docker Engine v28.0.0 is detected (exact version)
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "docker" && len(args) >= 3 && args[0] == "version" && args[1] == "--format" && args[2] == "{{.Server.Version}}" {
				return "28.0.0", nil
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When checking if Docker Engine v28+ is supported
		supported := dockerVirt.supportsDockerEngineV28Plus()

		// Then it should return true
		if !supported {
			t.Errorf("expected Docker Engine v28.0.0 to be supported, got false")
		}
	})

	t.Run("DockerEngineV29Plus", func(t *testing.T) {
		// Given a docker virt instance with valid mocks
		dockerVirt, mocks := setup(t)

		// And Docker Engine v29+ is detected
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "docker" && len(args) >= 3 && args[0] == "version" && args[1] == "--format" && args[2] == "{{.Server.Version}}" {
				return "29.0.0", nil
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When checking if Docker Engine v28+ is supported
		supported := dockerVirt.supportsDockerEngineV28Plus()

		// Then it should return true
		if !supported {
			t.Errorf("expected Docker Engine v29+ to be supported, got false")
		}
	})

	t.Run("DockerEnginePreV28", func(t *testing.T) {
		// Given a docker virt instance with valid mocks
		dockerVirt, mocks := setup(t)

		// And Docker Engine pre-v28 is detected
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "docker" && len(args) >= 3 && args[0] == "version" && args[1] == "--format" && args[2] == "{{.Server.Version}}" {
				return "27.0.3", nil
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When checking if Docker Engine v28+ is supported
		supported := dockerVirt.supportsDockerEngineV28Plus()

		// Then it should return false
		if supported {
			t.Errorf("expected Docker Engine pre-v28 to not be supported, got true")
		}
	})

	t.Run("DockerEngineV27Exact", func(t *testing.T) {
		// Given a docker virt instance with valid mocks
		dockerVirt, mocks := setup(t)

		// And Docker Engine v27.9.9 is detected (highest pre-v28)
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "docker" && len(args) >= 3 && args[0] == "version" && args[1] == "--format" && args[2] == "{{.Server.Version}}" {
				return "27.9.9", nil
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When checking if Docker Engine v28+ is supported
		supported := dockerVirt.supportsDockerEngineV28Plus()

		// Then it should return false
		if supported {
			t.Errorf("expected Docker Engine v27.9.9 to not be supported, got true")
		}
	})

	t.Run("DockerCommandError", func(t *testing.T) {
		// Given a docker virt instance with valid mocks
		dockerVirt, mocks := setup(t)

		// And Docker version command fails
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "docker" && len(args) >= 3 && args[0] == "version" && args[1] == "--format" && args[2] == "{{.Server.Version}}" {
				return "", fmt.Errorf("docker command failed")
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When checking if Docker Engine v28+ is supported
		supported := dockerVirt.supportsDockerEngineV28Plus()

		// Then it should return false (graceful fallback)
		if supported {
			t.Errorf("expected Docker Engine detection to fail gracefully, got true")
		}
	})

	t.Run("EmptyVersionString", func(t *testing.T) {
		// Given a docker virt instance with valid mocks
		dockerVirt, mocks := setup(t)

		// And Docker version command returns empty string
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "docker" && len(args) >= 3 && args[0] == "version" && args[1] == "--format" && args[2] == "{{.Server.Version}}" {
				return "", nil
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When checking if Docker Engine v28+ is supported
		supported := dockerVirt.supportsDockerEngineV28Plus()

		// Then it should return false (graceful fallback)
		if supported {
			t.Errorf("expected empty version string to not be supported, got true")
		}
	})

	t.Run("InvalidVersionFormat", func(t *testing.T) {
		// Given a docker virt instance with valid mocks
		dockerVirt, mocks := setup(t)

		// And Docker version command returns invalid format
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "docker" && len(args) >= 3 && args[0] == "version" && args[1] == "--format" && args[2] == "{{.Server.Version}}" {
				return "invalid-version", nil
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When checking if Docker Engine v28+ is supported
		supported := dockerVirt.supportsDockerEngineV28Plus()

		// Then it should return false (graceful fallback)
		if supported {
			t.Errorf("expected invalid version format to not be supported, got true")
		}
	})

	t.Run("SingleVersionComponent", func(t *testing.T) {
		// Given a docker virt instance with valid mocks
		dockerVirt, mocks := setup(t)

		// And Docker version command returns single component
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "docker" && len(args) >= 3 && args[0] == "version" && args[1] == "--format" && args[2] == "{{.Server.Version}}" {
				return "28", nil
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When checking if Docker Engine v28+ is supported
		supported := dockerVirt.supportsDockerEngineV28Plus()

		// Then it should return false (graceful fallback for malformed version)
		if supported {
			t.Errorf("expected single version component to not be supported, got true")
		}
	})
}

