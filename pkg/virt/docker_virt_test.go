// The DockerVirt test suite provides test coverage for Docker VM management functionality.
// It serves as a verification framework for Docker virtualization operations.
// It enables testing of Docker-specific features and error handling.

package virt

import (
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/services"
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
		dockerVirt := NewDockerVirt(mocks.Injector)
		dockerVirt.shims = mocks.Shims

		// Register default mock service
		mocks.Injector.Register("defaultService", mocks.Service)

		return dockerVirt, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a docker virt instance with valid mocks
		dockerVirt, _ := setup(t)

		// When initializing
		err := dockerVirt.Initialize()

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// And services should be resolved
		if len(dockerVirt.services) == 0 {
			t.Errorf("expected services to be resolved, but got none")
		}
	})

	t.Run("ErrorInitializingBaseVirt", func(t *testing.T) {
		// Given a docker virt instance with invalid shell
		dockerVirt, mocks := setup(t)
		mocks.Injector.Register("shell", "not a shell")

		// When initializing
		err := dockerVirt.Initialize()

		// Then an error should occur
		if err == nil {
			t.Errorf("expected error, got none")
		}

		// And the error should contain the expected message
		expectedErrorSubstring := "error resolving shell"
		if !strings.Contains(err.Error(), expectedErrorSubstring) {
			t.Errorf("expected error message to contain %q, got %q", expectedErrorSubstring, err.Error())
		}
	})

	t.Run("ErrorDockerNotEnabled", func(t *testing.T) {
		// Given a docker virt instance with docker disabled
		dockerVirt, mocks := setup(t)
		if err := mocks.ConfigHandler.SetContextValue("docker.enabled", false); err != nil {
			t.Fatalf("Failed to set docker.enabled: %v", err)
		}

		// When initializing
		err := dockerVirt.Initialize()

		// Then an error should occur
		if err == nil {
			t.Errorf("expected error, got none")
		}
		if !strings.Contains(err.Error(), "Docker configuration is not defined") {
			t.Errorf("expected error about Docker not being enabled, got %v", err)
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
		dockerVirt = NewDockerVirt(mockInjector)
		dockerVirt.shims = mocks.Shims

		// When initializing
		err := dockerVirt.Initialize()

		// Then an error should occur
		if err == nil {
			t.Errorf("expected error, got none")
		}
		if !strings.Contains(err.Error(), "error resolving services") {
			t.Errorf("expected error about resolving services, got %v", err)
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

		// When initializing
		err := dockerVirt.Initialize()

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// And the compose command should be empty
		if dockerVirt.composeCommand != "" {
			t.Errorf("expected compose command to be empty, got %q", dockerVirt.composeCommand)
		}
	})

	t.Run("NilServiceInSlice", func(t *testing.T) {
		// Given a docker virt instance with a nil service
		dockerVirt, mocks := setup(t)
		mocks.Injector.Register("nilService", nil)

		// When initializing
		err := dockerVirt.Initialize()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		// And services slice should only contain the default service
		if len(dockerVirt.services) != 2 {
			t.Errorf("Expected 2 services (default + nil), got %d", len(dockerVirt.services))
		}
	})

	t.Run("ServicesAreSorted", func(t *testing.T) {
		// Given a docker virt instance with multiple services
		dockerVirt, mocks := setup(t)

		// And services in random order
		serviceA := services.NewMockService()
		serviceB := services.NewMockService()
		serviceC := services.NewMockService()
		serviceA.SetName("ServiceA")
		serviceB.SetName("ServiceB")
		serviceC.SetName("ServiceC")
		mocks.Injector.Register("serviceA", serviceA)
		mocks.Injector.Register("serviceB", serviceB)
		mocks.Injector.Register("serviceC", serviceC)

		// When initializing
		err := dockerVirt.Initialize()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		// And services should be sorted by name
		if len(dockerVirt.services) != 5 {
			t.Errorf("Expected 5 services (default + 3 registered + 1 from config), got %d", len(dockerVirt.services))
		}
		if len(dockerVirt.services) == 5 {
			serviceNames := []string{
				dockerVirt.services[0].GetName(),
				dockerVirt.services[1].GetName(),
				dockerVirt.services[2].GetName(),
				dockerVirt.services[3].GetName(),
				dockerVirt.services[4].GetName(),
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
		dockerVirt = NewDockerVirt(mockInjector)
		dockerVirt.shims = mocks.Shims

		// When initializing
		err := dockerVirt.Initialize()

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})
}

// TestDockerVirt_Up tests the Up method of the DockerVirt component.
func TestDockerVirt_Up(t *testing.T) {
	setup := func(t *testing.T) (*DockerVirt, *Mocks) {
		t.Helper()
		mocks := setupDockerMocks(t)
		dockerVirt := NewDockerVirt(mocks.Injector)
		dockerVirt.shims = mocks.Shims
		if err := dockerVirt.Initialize(); err != nil {
			t.Fatalf("Failed to initialize DockerVirt: %v", err)
		}
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

		// Mock command execution to fail
		mocks.Shell.ExecProgressFunc = func(message string, command string, args ...string) (string, error) {
			if command == dockerVirt.composeCommand && args[0] == "up" {
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
		// Given a DockerVirt with mock components
		dockerVirt, mocks := setup(t)

		// Override GetProjectRoot to return an error
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("mock error retrieving project root")
		}

		// When calling the Up method
		err := dockerVirt.Up()

		// Then an error should occur
		if err == nil {
			t.Errorf("expected an error, got nil")
		}

		// And the error should contain the expected message
		expectedErrorMsg := "error retrieving project root"
		if err != nil && !strings.Contains(err.Error(), expectedErrorMsg) {
			t.Errorf("expected error message to contain %q, got %v", expectedErrorMsg, err)
		}
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
		dockerVirt := NewDockerVirt(mocks.Injector)
		dockerVirt.shims = mocks.Shims
		if err := dockerVirt.Initialize(); err != nil {
			t.Fatalf("Failed to initialize DockerVirt: %v", err)
		}

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

		// Mock command execution to fail twice then succeed
		mocks.Shell.ExecProgressFunc = func(message string, command string, args ...string) (string, error) {
			if command == dockerVirt.composeCommand && args[0] == "up" {
				execCount++
				if execCount < 3 {
					return "", fmt.Errorf("temporary error")
				}
				return "success", nil
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// And ExecSilent for retries also fails twice then succeeds
		oldExecSilent := mocks.Shell.ExecSilentFunc
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			// Keep original behavior for commands used in setup
			if command == "docker" && (len(args) > 0 && args[0] == "compose" || len(args) > 0 && args[0] == "info") {
				return oldExecSilent(command, args...)
			}

			// Handle compose up retry attempts
			if command == dockerVirt.composeCommand && len(args) > 0 && args[0] == "up" {
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
		dockerVirt := NewDockerVirt(mocks.Injector)
		dockerVirt.shims = mocks.Shims
		if err := dockerVirt.Initialize(); err != nil {
			t.Fatalf("Failed to initialize DockerVirt: %v", err)
		}
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
		// Given a DockerVirt with mock components
		dockerVirt, mocks := setup(t)

		// Override GetProjectRoot to return an error
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("mock error retrieving project root")
		}

		// When calling the Down method
		err := dockerVirt.Down()

		// Then an error should occur
		if err == nil {
			t.Errorf("expected an error, got nil")
		}

		// And the error should contain the expected message
		expectedErrorMsg := "error retrieving project root"
		if err != nil && !strings.Contains(err.Error(), expectedErrorMsg) {
			t.Errorf("expected error message to contain %q, got %v", expectedErrorMsg, err)
		}
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
		if err := dockerVirt.Initialize(); err != nil {
			t.Fatalf("Failed to initialize DockerVirt: %v", err)
		}

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
		mocks.Shell.ExecProgressFunc = func(message string, command string, args ...string) (string, error) {
			if command == dockerVirt.composeCommand && args[0] == "down" {
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

// TestDockerVirt_PrintInfo tests the PrintInfo method of the DockerVirt component.
func TestDockerVirt_PrintInfo(t *testing.T) {
	setup := func(t *testing.T) (*DockerVirt, *Mocks) {
		t.Helper()
		mocks := setupDockerMocks(t)
		dockerVirt := NewDockerVirt(mocks.Injector)
		dockerVirt.shims = mocks.Shims
		if err := dockerVirt.Initialize(); err != nil {
			t.Fatalf("Failed to initialize DockerVirt: %v", err)
		}
		return dockerVirt, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a docker virt instance with valid mocks
		dockerVirt, _ := setup(t)

		// When calling PrintInfo
		err := dockerVirt.PrintInfo()

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("NoContainersRunning", func(t *testing.T) {
		// Given a docker virt instance with valid mocks
		dockerVirt, mocks := setup(t)

		// And no containers are running
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "docker" && len(args) > 0 && args[0] == "ps" {
				return "", nil
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When calling PrintInfo
		err := dockerVirt.PrintInfo()

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("ErrorGettingContainerInfo", func(t *testing.T) {
		// Given a docker virt instance with valid mocks
		dockerVirt, mocks := setup(t)

		// And an error occurs when getting container info
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "docker" && len(args) > 0 && args[0] == "ps" {
				return "", fmt.Errorf("mock error getting container info")
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When calling PrintInfo
		err := dockerVirt.PrintInfo()

		// Then an error should occur
		if err == nil {
			t.Errorf("expected error, got none")
		}

		// And the error should contain the expected message
		expectedErrorSubstring := "error retrieving container info"
		if !strings.Contains(err.Error(), expectedErrorSubstring) {
			t.Errorf("expected error message to contain %q, got %q", expectedErrorSubstring, err.Error())
		}
	})

	t.Run("ErrorInspectLabels", func(t *testing.T) {
		// Given a docker virt instance with failing inspect
		dockerVirt, mocks := setup(t)
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "docker" && len(args) > 0 {
				if args[0] == "ps" {
					return "container1", nil
				}
				if args[0] == "inspect" && args[3] == "{{json .Config.Labels}}" {
					return "", fmt.Errorf("inspect failed")
				}
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When getting container info
		_, err := dockerVirt.GetContainerInfo()

		// Then an error should occur
		if err == nil {
			t.Errorf("expected error, got none")
		}
	})

	t.Run("ErrorInspectNetworks", func(t *testing.T) {
		// Given a docker virt instance with failing network inspect
		dockerVirt, mocks := setup(t)
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "docker" && len(args) > 0 {
				if args[0] == "ps" {
					return "container1", nil
				}
				if args[0] == "inspect" && args[3] == "{{json .Config.Labels}}" {
					return `{"managed_by":"windsor","context":"mock-context","com.docker.compose.service":"service1","role":"test"}`, nil
				}
				if args[0] == "inspect" && args[3] == "{{json .NetworkSettings.Networks}}" {
					return "", fmt.Errorf("network inspect failed")
				}
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When getting container info
		_, err := dockerVirt.GetContainerInfo()

		// Then an error should occur
		if err == nil {
			t.Errorf("expected error, got none")
		}
		if !strings.Contains(err.Error(), "error inspecting container networks") {
			t.Errorf("expected error about network inspection, got %v", err)
		}
	})

	t.Run("ErrorUnmarshalLabels", func(t *testing.T) {
		// Given a docker virt instance with invalid JSON labels
		dockerVirt, mocks := setup(t)
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "docker" && len(args) > 0 {
				if args[0] == "ps" {
					return "container1", nil
				}
				if args[0] == "inspect" && args[3] == "{{json .Config.Labels}}" {
					return "invalid json", nil
				}
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When getting container info
		_, err := dockerVirt.GetContainerInfo()

		// Then an error should occur
		if err == nil {
			t.Errorf("expected error, got none")
		}
		if !strings.Contains(err.Error(), "error unmarshaling container labels") {
			t.Errorf("expected error about unmarshaling labels, got %v", err)
		}
	})

	t.Run("ErrorUnmarshalNetworks", func(t *testing.T) {
		// Given a docker virt instance with invalid JSON networks
		dockerVirt, mocks := setup(t)
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "docker" && len(args) > 0 {
				if args[0] == "ps" {
					return "container1", nil
				}
				if args[0] == "inspect" && args[3] == "{{json .Config.Labels}}" {
					return `{"managed_by":"windsor","context":"mock-context","com.docker.compose.service":"service1","role":"test"}`, nil
				}
				if args[0] == "inspect" && args[3] == "{{json .NetworkSettings.Networks}}" {
					return "invalid json", nil
				}
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When getting container info
		_, err := dockerVirt.GetContainerInfo()

		// Then an error should occur
		if err == nil {
			t.Errorf("expected error, got none")
		}
		if !strings.Contains(err.Error(), "error unmarshaling container networks") {
			t.Errorf("expected error about unmarshaling networks, got %v", err)
		}
	})

	t.Run("FilterByServiceName", func(t *testing.T) {
		// Given a docker virt instance with valid mocks
		dockerVirt, mocks := setup(t)

		// And multiple containers with different service names
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "docker" && len(args) > 0 {
				if args[0] == "ps" {
					return "container1\ncontainer2\ncontainer3", nil
				}
				if args[0] == "inspect" && args[3] == "{{json .Config.Labels}}" {
					switch args[1] {
					case "container1":
						return `{"managed_by":"windsor","context":"mock-context","com.docker.compose.service":"service1","role":"test"}`, nil
					case "container2":
						return `{"managed_by":"windsor","context":"mock-context","com.docker.compose.service":"service2","role":"test"}`, nil
					case "container3":
						return `{"managed_by":"windsor","context":"mock-context","com.docker.compose.service":"service3","role":"test"}`, nil
					}
				}
				if args[0] == "inspect" && args[3] == "{{json .NetworkSettings.Networks}}" {
					switch args[1] {
					case "container1":
						return fmt.Sprintf(`{"windsor-%s":{"IPAddress":"192.168.1.2"}}`, mocks.ConfigHandler.GetContext()), nil
					case "container2":
						return fmt.Sprintf(`{"windsor-%s":{"IPAddress":"192.168.1.3"}}`, mocks.ConfigHandler.GetContext()), nil
					case "container3":
						return fmt.Sprintf(`{"windsor-%s":{"IPAddress":"192.168.1.4"}}`, mocks.ConfigHandler.GetContext()), nil
					}
				}
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When getting container info for specific services
		info, err := dockerVirt.GetContainerInfo([]string{"service1", "service3"}...)

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// And only containers for specified services should be returned
		if len(info) != 2 {
			t.Errorf("expected 2 containers, got %d", len(info))
		}

		// And the containers should be for the specified services
		serviceNames := []string{}
		for _, container := range info {
			serviceNames = append(serviceNames, container.Name)
		}
		if !slices.Contains(serviceNames, "service1") {
			t.Error("expected container for service1 to be included")
		}
		if !slices.Contains(serviceNames, "service3") {
			t.Error("expected container for service3 to be included")
		}
		if slices.Contains(serviceNames, "service2") {
			t.Error("expected container for service2 to be excluded")
		}
	})
}

// TestDockerVirt_GetContainerInfo tests the GetContainerInfo method of the DockerVirt component.
func TestDockerVirt_GetContainerInfo(t *testing.T) {
	setup := func(t *testing.T) (*DockerVirt, *Mocks) {
		t.Helper()
		mocks := setupDockerMocks(t)
		dockerVirt := NewDockerVirt(mocks.Injector)
		dockerVirt.shims = mocks.Shims
		if err := dockerVirt.Initialize(); err != nil {
			t.Fatalf("Failed to initialize DockerVirt: %v", err)
		}
		return dockerVirt, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a docker virt instance with valid mocks
		dockerVirt, _ := setup(t)

		// When getting container info
		info, err := dockerVirt.GetContainerInfo()

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// And containers should be returned
		if len(info) != 2 {
			t.Errorf("expected 2 containers, got %d", len(info))
		}
	})

	t.Run("NoContainersFound", func(t *testing.T) {
		// Given a docker virt instance with no containers
		dockerVirt, mocks := setup(t)
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "docker" && len(args) > 0 && args[0] == "ps" {
				return "", nil
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When getting container info
		info, err := dockerVirt.GetContainerInfo()

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		// And no containers should be returned
		if len(info) != 0 {
			t.Errorf("expected no containers, got %d", len(info))
		}
	})
}

// TestDockerVirt_WriteConfig tests the WriteConfig method of the DockerVirt component.
func TestDockerVirt_WriteConfig(t *testing.T) {
	setup := func(t *testing.T) (*DockerVirt, *Mocks) {
		t.Helper()
		mocks := setupDockerMocks(t)
		dockerVirt := NewDockerVirt(mocks.Injector)
		dockerVirt.shims = mocks.Shims
		if err := dockerVirt.Initialize(); err != nil {
			t.Fatalf("Failed to initialize DockerVirt: %v", err)
		}
		return dockerVirt, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a virt instance with mock components
		dockerVirt, mocks := setup(t)

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
		// Given a docker virt instance with failing shell
		dockerVirt, mocks := setup(t)
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("failed to get project root")
		}

		// When writing config
		err := dockerVirt.WriteConfig()

		// Then an error should occur
		if err == nil {
			t.Errorf("expected error, got none")
		}
		if !strings.Contains(err.Error(), "error retrieving project root") {
			t.Errorf("expected error about project root, got %v", err)
		}
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
		dockerVirt := NewDockerVirt(mocks.Injector)
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

		// When initializing
		err := dockerVirt.Initialize()

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
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

		// When initializing
		err := dockerVirt.Initialize()

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
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
			if command == "docker compose" {
				return "Docker Compose version 2.0.0", nil
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When initializing
		err := dockerVirt.Initialize()

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
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

		// When initializing
		err := dockerVirt.Initialize()

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
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
		dockerVirt := NewDockerVirt(mocks.Injector)
		dockerVirt.shims = mocks.Shims
		if err := dockerVirt.Initialize(); err != nil {
			t.Fatalf("Failed to initialize DockerVirt: %v", err)
		}
		return dockerVirt, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a docker virt instance with valid mocks
		dockerVirt, _ := setup(t)

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
