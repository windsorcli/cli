package services

import (
	"os"
	"testing"

	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
)

// The ServiceTest is a test suite for the Service interface and BaseService implementation
// It provides comprehensive test coverage for service initialization, configuration, and addressing
// The ServiceTest ensures proper service behavior across different scenarios and configurations
// enabling reliable service management and DNS resolution in the Windsor CLI

// =============================================================================
// Test Setup
// =============================================================================

type Mocks struct {
	Injector      di.Injector
	ConfigHandler config.ConfigHandler
	Shell         *shell.MockShell
	Shims         *Shims
}

type SetupOptions struct {
	Injector      di.Injector
	ConfigHandler config.ConfigHandler
	ConfigStr     string
}

func setupShims(t *testing.T) *Shims {
	t.Helper()
	shims := NewShims()

	shims.Getwd = func() (string, error) {
		return "/tmp", nil
	}
	shims.Glob = func(pattern string) ([]string, error) {
		return []string{}, nil
	}
	shims.WriteFile = func(filename string, data []byte, perm os.FileMode) error {
		return nil
	}
	shims.Stat = func(name string) (os.FileInfo, error) {
		return nil, nil
	}
	shims.Mkdir = func(path string, perm os.FileMode) error {
		return nil
	}
	shims.MkdirAll = func(path string, perm os.FileMode) error {
		return nil
	}
	shims.Rename = func(oldpath, newpath string) error {
		return nil
	}
	shims.YamlMarshal = func(in interface{}) ([]byte, error) {
		return []byte{}, nil
	}
	shims.YamlUnmarshal = func(in []byte, out interface{}) error {
		return nil
	}
	shims.JsonUnmarshal = func(data []byte, v interface{}) error {
		return nil
	}
	shims.UserHomeDir = func() (string, error) {
		return "/home/test", nil
	}

	return shims
}

func setupMocks(t *testing.T, opts ...*SetupOptions) *Mocks {
	t.Helper()

	// Store original directory and create temp dir
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}

	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to change to temp directory: %v", err)
	}

	// Set project root environment variable
	t.Setenv("WINDSOR_PROJECT_ROOT", tmpDir)

	// Register cleanup to restore original state
	t.Cleanup(func() {
		os.Unsetenv("WINDSOR_PROJECT_ROOT")
		if err := os.Chdir(origDir); err != nil {
			t.Logf("Warning: Failed to change back to original directory: %v", err)
		}
	})

	// Create injector if not provided
	var injector di.Injector
	if len(opts) > 0 && opts[0].Injector != nil {
		injector = opts[0].Injector
	} else {
		injector = di.NewInjector()
	}

	// Create config handler if not provided
	var configHandler config.ConfigHandler
	if len(opts) > 0 && opts[0].ConfigHandler != nil {
		configHandler = opts[0].ConfigHandler
	} else {
		configHandler = config.NewYamlConfigHandler(injector)
	}
	injector.Register("configHandler", configHandler)

	// Initialize config handler
	if err := configHandler.Initialize(); err != nil {
		t.Fatalf("Failed to initialize config handler: %v", err)
	}

	configHandler.SetContext("mock-context")

	// Load config
	configYAML := `
apiVersion: v1alpha1
contexts:
  mock-context:
    dns:
      domain: example.com
      enabled: true
      records:
        - 127.0.0.1 test
        - 192.168.1.1 test
    docker:
      enabled: true
`
	if err := configHandler.LoadConfigString(configYAML); err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Load optional config if provided
	if len(opts) > 0 && opts[0].ConfigStr != "" {
		if err := configHandler.LoadConfigString(opts[0].ConfigStr); err != nil {
			t.Fatalf("Failed to load config string: %v", err)
		}
	}

	// Create a mock shell
	mockShell := shell.NewMockShell(injector)
	injector.Register("shell", mockShell)

	return &Mocks{
		Injector:      injector,
		ConfigHandler: configHandler,
		Shell:         mockShell,
		Shims:         setupShims(t),
	}
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestBaseService_Initialize(t *testing.T) {
	setup := func(t *testing.T) (*BaseService, *Mocks) {
		mocks := setupMocks(t)
		service := NewBaseService(mocks.Injector)
		return service, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a set of mock components
		service, _ := setup(t)

		// When a new BaseService is created and initialized
		err := service.Initialize()

		// Then the initialization should succeed without errors
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// And the resolved dependencies should be set correctly
		if service.configHandler == nil {
			t.Fatalf("expected configHandler to be set, got nil")
		}
		if service.shell == nil {
			t.Fatalf("expected shell to be set, got nil")
		}
	})

	t.Run("ErrorResolvingShell", func(t *testing.T) {
		// Given a set of mock components
		service, mocks := setup(t)

		// And the injector is set to return nil for the shell dependency
		mocks.Injector.Register("shell", nil)

		// When a new BaseService is created and initialized
		err := service.Initialize()

		// Then the initialization should fail with an error
		if err == nil {
			t.Fatalf("expected an error during initialization, got nil")
		}
	})
}

func TestBaseService_WriteConfig(t *testing.T) {
	setup := func(t *testing.T) (*BaseService, *Mocks) {
		mocks := setupMocks(t)
		service := NewBaseService(mocks.Injector)
		service.shims = mocks.Shims
		service.Initialize()
		return service, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a new BaseService
		service, _ := setup(t)

		// When WriteConfig is called
		err := service.WriteConfig()

		// Then the WriteConfig should succeed without errors
		if err != nil {
			t.Fatalf("expected no error during WriteConfig, got %v", err)
		}
	})
}

func TestBaseService_SetAddress(t *testing.T) {
	setup := func(t *testing.T) (*BaseService, *Mocks) {
		mocks := setupMocks(t)
		service := NewBaseService(mocks.Injector)
		service.shims = mocks.Shims
		service.Initialize()
		return service, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a new BaseService
		service, _ := setup(t)

		// When SetAddress is called with a valid IPv4 address
		err := service.SetAddress("192.168.1.1")

		// Then the SetAddress should succeed without errors
		if err != nil {
			t.Fatalf("expected no error during SetAddress, got %v", err)
		}

		// And the address should be set correctly
		expectedAddress := "192.168.1.1"
		if service.GetAddress() != expectedAddress {
			t.Fatalf("expected address '%s', got %v", expectedAddress, service.GetAddress())
		}
	})

	t.Run("InvalidAddress", func(t *testing.T) {
		// Given a new BaseService
		service, _ := setup(t)

		// When SetAddress is called with an invalid IPv4 address
		err := service.SetAddress("invalid_address")

		// Then the SetAddress should fail with an error
		if err == nil {
			t.Fatalf("expected an error during SetAddress, got nil")
		}

		// And the error message should be as expected
		expectedErrorMessage := "invalid IPv4 address"
		if err.Error() != expectedErrorMessage {
			t.Fatalf("expected error message '%s', got %v", expectedErrorMessage, err)
		}
	})
}

func TestBaseService_GetAddress(t *testing.T) {
	setup := func(t *testing.T) (*BaseService, *Mocks) {
		mocks := setupMocks(t)
		service := NewBaseService(mocks.Injector)
		service.shims = mocks.Shims
		service.Initialize()
		return service, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a new BaseService
		service, _ := setup(t)
		service.SetAddress("192.168.1.1")

		// When GetAddress is called
		address := service.GetAddress()

		// Then the address should be as expected
		expectedAddress := "192.168.1.1"
		if address != expectedAddress {
			t.Fatalf("expected address '%s', got %v", expectedAddress, address)
		}
	})
}

func TestBaseService_GetName(t *testing.T) {
	setup := func(t *testing.T) (*BaseService, *Mocks) {
		mocks := setupMocks(t)
		service := NewBaseService(mocks.Injector)
		service.shims = mocks.Shims
		service.Initialize()
		return service, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a new BaseService
		service, _ := setup(t)
		service.SetName("TestService")

		// When GetName is called
		name := service.GetName()

		// Then the name should be as expected
		expectedName := "TestService"
		if name != expectedName {
			t.Fatalf("expected name '%s', got %v", expectedName, name)
		}
	})
}

func TestBaseService_GetHostname(t *testing.T) {
	setup := func(t *testing.T) (*BaseService, *Mocks) {
		mocks := setupMocks(t)
		service := NewBaseService(mocks.Injector)
		service.shims = mocks.Shims
		service.Initialize()
		return service, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a new BaseService
		service, _ := setup(t)

		// When GetHostname is called
		hostname := service.GetHostname()

		// Then the hostname should be correctly formatted
		expectedHostname := "test-service.example.com"
		if hostname != expectedHostname {
			t.Errorf("Expected hostname %q, got %q", expectedHostname, hostname)
		}
	})

	t.Run("DefaultTLD", func(t *testing.T) {
		// Given a new BaseService
		service, _ := setup(t)

		service.SetName("test-service")

		// When GetHostname is called
		hostname := service.GetHostname()

		// Then the hostname should use default TLD
		expectedHostname := "test-service.test"
		if hostname != expectedHostname {
			t.Errorf("Expected hostname %q, got %q", expectedHostname, hostname)
		}
	})
}

func TestBaseService_IsLocalhostMode(t *testing.T) {
	setup := func(t *testing.T) (*BaseService, *Mocks) {
		mocks := setupMocks(t)
		service := NewBaseService(mocks.Injector)
		service.shims = mocks.Shims
		service.Initialize()
		return service, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given mock components
		service, mocks := setup(t)

		// And mock behavior for docker-desktop driver
		mocks.ConfigHandler.SetContextValue("vm.driver", "docker-desktop")

		// When isLocalhostMode is called
		isLocal := service.isLocalhostMode()

		// Then the result should be true for docker-desktop
		if !isLocal {
			t.Fatal("expected isLocalhostMode to be true for docker-desktop driver")
		}
	})

	t.Run("NotDockerDesktop", func(t *testing.T) {
		// Given mock components
		service, mocks := setup(t)

		// And mock behavior for non-docker-desktop driver
		mocks.ConfigHandler.SetContextValue("vm.driver", "other-driver")

		// When isLocalhostMode is called
		isLocal := service.isLocalhostMode()

		// Then the result should be false for non-docker-desktop driver
		if isLocal {
			t.Fatal("expected isLocalhostMode to be false for non-docker-desktop driver")
		}
	})
}

func TestBaseService_SupportsWildcard(t *testing.T) {
	setup := func(t *testing.T) (*BaseService, *Mocks) {
		mocks := setupMocks(t)
		service := NewBaseService(mocks.Injector)
		service.shims = mocks.Shims
		service.Initialize()
		return service, mocks
	}

	t.Run("DefaultBehavior", func(t *testing.T) {
		// Given a new BaseService
		service, _ := setup(t)

		// When SupportsWildcard is called
		supports := service.SupportsWildcard()

		// Then the result should be false by default
		if supports {
			t.Fatal("expected SupportsWildcard to be false by default")
		}
	})
}
