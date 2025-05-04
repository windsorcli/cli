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

	"github.com/compose-spec/compose-go/types"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/services"
	"github.com/windsorcli/cli/pkg/tools"
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
          hostport: 5000`

	if err := mocks.ConfigHandler.LoadConfigString(configStr); err != nil {
		t.Fatalf("Failed to load config string: %v", err)
	}

	mocks.ConfigHandler.SetContext("mock-context")

	// Set up mock tools manager
	toolsManager := tools.NewMockToolsManager()
	toolsManager.GetDockerComposeCommandFunc = func() (string, error) {
		return "docker compose", nil
	}
	toolsManager.InitializeFunc = func() error {
		return nil
	}
	mocks.Injector.Register("toolsManager", toolsManager)

	// Set up mock shell for Docker commands
	mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
		if command == "docker" && len(args) > 0 {
			switch args[0] {
			case "info":
				return "Docker info output", nil
			case "compose":
				return "Docker Compose version 2.0.0", nil
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
			Services: []types.ServiceConfig{
				{
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
				{
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
		toolsManager := tools.NewMockToolsManager()
		toolsManager.GetDockerComposeCommandFunc = func() (string, error) {
			return "", fmt.Errorf("error determining compose command")
		}
		mocks.Injector.Register("toolsManager", toolsManager)

		// When initializing
		err := dockerVirt.Initialize()

		// Then an error should occur
		if err == nil {
			t.Errorf("expected error, got none")
		}
		if !strings.Contains(err.Error(), "error determining docker compose command") {
			t.Errorf("expected error about determining compose command, got %v", err)
		}
	})

	t.Run("SkipNilService", func(t *testing.T) {
		// Given a docker virt instance with nil service
		dockerVirt, mocks := setup(t)
		mocks.Injector.Register("nilService", nil)

		// When initializing
		err := dockerVirt.Initialize()

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// And services slice should only contain the default service
		if len(dockerVirt.services) != 2 {
			t.Errorf("expected 2 services (default + nil), got %d", len(dockerVirt.services))
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
			if command == "docker" && len(args) > 0 && args[0] == "compose" && args[1] == "up" {
				return "", fmt.Errorf("mock docker-compose up error")
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "docker" {
				switch args[0] {
				case "info":
					return "Docker info output", nil
				case "compose":
					if args[1] == "up" {
						return "", fmt.Errorf("mock docker-compose up error")
					}
					return "Docker Compose version 2.0.0", nil
				}
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
		expectedErrorMsg := "Error executing command"
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
		// Given a DockerVirt with mock components
		dockerVirt, mocks := setup(t)

		// Track number of retries
		retryCount := 0
		maxRetries := 3

		// Mock command execution to fail initially but succeed on last retry
		mocks.Shell.ExecProgressFunc = func(message string, command string, args ...string) (string, error) {
			if command == "docker" && len(args) > 0 && args[0] == "compose" && args[1] == "up" {
				retryCount++
				return "", fmt.Errorf("mock docker-compose up error")
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "docker" {
				switch args[0] {
				case "info":
					return "Docker info output", nil
				case "compose":
					if args[1] == "up" {
						retryCount++
						if retryCount < maxRetries {
							return "", fmt.Errorf("mock docker-compose up error")
						}
						return "", nil
					}
					return "Docker Compose version 2.0.0", nil
				}
			}
			return "", fmt.Errorf("unexpected command: %s %v", command, args)
		}

		// When calling Up
		err := dockerVirt.Up()

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// And the command should have been retried the expected number of times
		if retryCount != maxRetries {
			t.Errorf("expected %d retries, got %d", maxRetries, retryCount)
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
		dockerVirt, _ := setup(t)

		// When calling Down
		err := dockerVirt.Down()

		// Then no error should occur
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
			if command == "docker" && len(args) > 0 && args[0] == "compose" && args[1] == "down" {
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
		toolsManager := tools.NewMockToolsManager()
		toolsManager.GetDockerComposeCommandFunc = func() (string, error) {
			return "docker-compose", nil
		}
		mocks.Injector.Register("toolsManager", toolsManager)

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
		toolsManager := tools.NewMockToolsManager()
		toolsManager.GetDockerComposeCommandFunc = func() (string, error) {
			return "docker-cli-plugin-docker-compose", nil
		}
		mocks.Injector.Register("toolsManager", toolsManager)

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
		toolsManager := tools.NewMockToolsManager()
		toolsManager.GetDockerComposeCommandFunc = func() (string, error) {
			return "docker compose", nil
		}
		mocks.Injector.Register("toolsManager", toolsManager)

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
		toolsManager := tools.NewMockToolsManager()
		toolsManager.GetDockerComposeCommandFunc = func() (string, error) {
			return "", fmt.Errorf("no compose command available")
		}
		mocks.Injector.Register("toolsManager", toolsManager)

		// When initializing
		err := dockerVirt.Initialize()

		// Then an error should occur
		if err == nil {
			t.Errorf("expected error, got none")
		}

		// And the error should contain the expected message
		if !strings.Contains(err.Error(), "error determining docker compose command") {
			t.Errorf("expected error about determining compose command, got %v", err)
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
	})

	t.Run("DockerNotEnabled", func(t *testing.T) {
		// Given a docker virt instance with valid mocks
		dockerVirt, mocks := setup(t)

		// And Docker is not enabled
		// Create a new config handler with Docker disabled
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
      enabled: false
      registry_url: "https://registry.example.com"
      registries:
        local:
          remote: "remote-registry.example.com"
          local: "localhost:5000"
          hostname: "registry.local"
          hostport: 5000`

		if err := mocks.ConfigHandler.LoadConfigString(configStr); err != nil {
			t.Fatalf("Failed to load config string: %v", err)
		}

		// When getting the full compose config
		project, err := dockerVirt.getFullComposeConfig()

		// Then an error should occur
		if err == nil {
			t.Errorf("expected error, got none")
		}

		// And the error should contain the expected message
		expectedErrorSubstring := "Docker configuration is not defined"
		if !strings.Contains(err.Error(), expectedErrorSubstring) {
			t.Errorf("expected error message to contain %q, got %q", expectedErrorSubstring, err.Error())
		}

		// And the project should be nil
		if project != nil {
			t.Errorf("expected project to be nil")
		}
	})

	t.Run("ServiceGetComposeConfigError", func(t *testing.T) {
		// Given a docker virt instance with valid mocks
		dockerVirt, mocks := setup(t)

		// And a service returns an error when getting compose config
		mocks.Service.GetComposeConfigFunc = func() (*types.Config, error) {
			return nil, fmt.Errorf("mock error getting compose config")
		}

		// When getting the full compose config
		project, err := dockerVirt.getFullComposeConfig()

		// Then an error should occur
		if err == nil {
			t.Errorf("expected error, got none")
		}

		// And the error should contain the expected message
		expectedErrorSubstring := "error getting container config from service"
		if !strings.Contains(err.Error(), expectedErrorSubstring) {
			t.Errorf("expected error message to contain %q, got %q", expectedErrorSubstring, err.Error())
		}

		// And the project should be nil
		if project != nil {
			t.Errorf("expected project to be nil")
		}
	})

	t.Run("ServiceReturnsNilConfig", func(t *testing.T) {
		// Given a docker virt instance with valid mocks
		dockerVirt, mocks := setup(t)

		// And a service returns nil when getting compose config
		mocks.Service.GetComposeConfigFunc = func() (*types.Config, error) {
			return nil, nil
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

		// And the project should have no services
		if len(project.Services) != 0 {
			t.Errorf("expected 0 services, got %d", len(project.Services))
		}
	})

	t.Run("ServiceReturnsEmptyConfig", func(t *testing.T) {
		// Given a docker virt instance with valid mocks
		dockerVirt, mocks := setup(t)

		// And a service returns a config with no services
		mocks.Service.GetComposeConfigFunc = func() (*types.Config, error) {
			return &types.Config{
				Services: nil,
				Volumes: map[string]types.VolumeConfig{
					"test-volume": {},
				},
				Networks: map[string]types.NetworkConfig{
					"test-network": {},
				},
			}, nil
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

		// And the project should have no services
		if len(project.Services) != 0 {
			t.Errorf("expected 0 services, got %d", len(project.Services))
		}

		// And the project should have the volume
		if _, exists := project.Volumes["test-volume"]; !exists {
			t.Errorf("expected volume test-volume to exist")
		}

		// And the project should have the network
		if _, exists := project.Networks["test-network"]; !exists {
			t.Errorf("expected network test-network to exist")
		}
	})
}
