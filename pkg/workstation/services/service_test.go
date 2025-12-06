package services

import (
	"os"
	"testing"
	"time"

	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// The ServiceTest is a test suite for the Service interface and BaseService implementation
// It provides comprehensive test coverage for service initialization, configuration, and addressing
// The ServiceTest ensures proper service behavior across different scenarios and configurations
// enabling reliable service management and DNS resolution in the Windsor CLI

// =============================================================================
// Test Setup
// =============================================================================

// mockFileInfo implements os.FileInfo for testing
type mockFileInfo struct {
	isDir bool
}

func (m *mockFileInfo) Name() string       { return "mockfile" }
func (m *mockFileInfo) Size() int64        { return 0 }
func (m *mockFileInfo) Mode() os.FileMode  { return 0644 }
func (m *mockFileInfo) ModTime() time.Time { return time.Now() }
func (m *mockFileInfo) IsDir() bool        { return m.isDir }
func (m *mockFileInfo) Sys() interface{}   { return nil }

type ServicesTestMocks struct {
	Runtime       *runtime.Runtime
	ConfigHandler config.ConfigHandler
	Shell         *shell.MockShell
	Shims         *Shims
}

func setupDefaultShims() *Shims {
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
		// Return a mock file info that indicates it's not a directory
		return &mockFileInfo{isDir: false}, nil
	}
	shims.Mkdir = func(path string, perm os.FileMode) error {
		return nil
	}
	shims.MkdirAll = func(path string, perm os.FileMode) error {
		return nil
	}
	shims.RemoveAll = func(path string) error {
		return nil
	}
	shims.Rename = func(oldpath, newpath string) error {
		return nil
	}
	shims.YamlMarshal = func(in any) ([]byte, error) {
		return []byte{}, nil
	}
	shims.YamlUnmarshal = func(in []byte, out any) error {
		return nil
	}
	shims.JsonUnmarshal = func(data []byte, v any) error {
		return nil
	}
	shims.UserHomeDir = func() (string, error) {
		return "/home/test", nil
	}

	return shims
}

func setupServicesMocks(t *testing.T, opts ...func(*ServicesTestMocks)) *ServicesTestMocks {
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
	t.Setenv("WINDSOR_CONTEXT", "mock-context")

	// Register cleanup to restore original state
	t.Cleanup(func() {
		os.Unsetenv("WINDSOR_PROJECT_ROOT")
		if err := os.Chdir(origDir); err != nil {
			t.Logf("Warning: Failed to change back to original directory: %v", err)
		}
	})

	// Create mock shell
	mockShell := shell.NewMockShell()
	mockShell.GetProjectRootFunc = func() (string, error) {
		return tmpDir, nil
	}

	// Create config handler
	configHandler := config.NewConfigHandler(mockShell)

	configHandler.SetContext("mock-context")

	// Load config
	configYAML := `
version: v1alpha1
contexts:
  mock-context:
    dns:
      domain: test
      enabled: true
      records:
        - 127.0.0.1 test
        - 192.168.1.1 test
    docker:
      enabled: true
      registries:
        registry:
          remote: registry.test
          local: registry.test
`
	if err := configHandler.LoadConfigString(configYAML); err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	rt := &runtime.Runtime{
		ProjectRoot:   tmpDir,
		ConfigRoot:    tmpDir,
		TemplateRoot:  tmpDir,
		ContextName:   "mock-context",
		ConfigHandler: configHandler,
		Shell:         mockShell,
	}

	mocks := &ServicesTestMocks{
		Runtime:       rt,
		ConfigHandler: configHandler,
		Shell:         mockShell,
		Shims:         setupDefaultShims(),
	}

	// Apply any overrides
	for _, opt := range opts {
		opt(mocks)
	}

	return mocks
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestBaseService_NewBaseService(t *testing.T) {
	setup := func(t *testing.T) (*BaseService, *ServicesTestMocks) {
		mocks := setupServicesMocks(t)
		service := NewBaseService(mocks.Runtime)
		return service, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a set of mock components
		service, mocks := setup(t)

		// Then the service should be created with dependencies set correctly
		if service.configHandler == nil {
			t.Fatalf("expected configHandler to be set, got nil")
		}
		if service.shell == nil {
			t.Fatalf("expected shell to be set, got nil")
		}
		if service.configHandler != mocks.ConfigHandler {
			t.Fatalf("expected configHandler to match mocks")
		}
		if service.shell != mocks.Shell {
			t.Fatalf("expected shell to match mocks")
		}
	})

}

func TestBaseService_WriteConfig(t *testing.T) {
	setup := func(t *testing.T) (*BaseService, *ServicesTestMocks) {
		mocks := setupServicesMocks(t)
		service := NewBaseService(mocks.Runtime)
		service.shims = mocks.Shims
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
	setup := func(t *testing.T) (*BaseService, *ServicesTestMocks) {
		mocks := setupServicesMocks(t)
		service := NewBaseService(mocks.Runtime)
		service.shims = mocks.Shims
		return service, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a new BaseService
		service, _ := setup(t)

		// When SetAddress is called with a valid IPv4 address
		err := service.SetAddress("192.168.1.1", nil)

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
		err := service.SetAddress("invalid_address", nil)

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
	setup := func(t *testing.T) (*BaseService, *ServicesTestMocks) {
		mocks := setupServicesMocks(t)
		service := NewBaseService(mocks.Runtime)
		service.shims = mocks.Shims
		return service, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a new BaseService
		service, _ := setup(t)
		service.SetAddress("192.168.1.1", nil)

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
	setup := func(t *testing.T) (*BaseService, *ServicesTestMocks) {
		mocks := setupServicesMocks(t)
		service := NewBaseService(mocks.Runtime)
		service.shims = mocks.Shims
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
	setup := func(t *testing.T) (*BaseService, *ServicesTestMocks) {
		mocks := setupServicesMocks(t)
		service := NewBaseService(mocks.Runtime)
		service.shims = mocks.Shims
		return service, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a new BaseService
		service, _ := setup(t)
		service.SetName("test-service")

		// When GetHostname is called
		hostname := service.GetHostname()

		// Then the hostname should be correctly formatted
		expectedHostname := "test-service.test"
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
	setup := func(t *testing.T) (*BaseService, *ServicesTestMocks) {
		mocks := setupServicesMocks(t)
		service := NewBaseService(mocks.Runtime)
		service.shims = mocks.Shims
		return service, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given mock components
		service, mocks := setup(t)

		// And mock behavior for docker-desktop driver
		mocks.ConfigHandler.Set("vm.driver", "docker-desktop")

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
		mocks.ConfigHandler.Set("vm.driver", "other-driver")

		// When isLocalhostMode is called
		isLocal := service.isLocalhostMode()

		// Then the result should be false for non-docker-desktop driver
		if isLocal {
			t.Fatal("expected isLocalhostMode to be false for non-docker-desktop driver")
		}
	})
}

func TestBaseService_SupportsWildcard(t *testing.T) {
	setup := func(t *testing.T) (*BaseService, *ServicesTestMocks) {
		mocks := setupServicesMocks(t)
		service := NewBaseService(mocks.Runtime)
		service.shims = mocks.Shims
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

func TestBaseService_GetContainerName(t *testing.T) {
	setup := func(t *testing.T) (*BaseService, *ServicesTestMocks) {
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "dns.domain" {
				return "test"
			}
			return defaultValue[0]
		}
		mocks := setupServicesMocks(t, func(m *ServicesTestMocks) {
			m.ConfigHandler = mockConfigHandler
			m.Runtime.ConfigHandler = mockConfigHandler
		})
		service := NewBaseService(mocks.Runtime)
		service.shims = mocks.Shims
		return service, mocks
	}

	t.Run("SimpleName", func(t *testing.T) {
		// Given a service with a simple name
		service, _ := setup(t)
		service.SetName("dns")

		// When getting the container name
		name := service.GetContainerName()

		// Then it should return the name with the TLD
		expected := "dns.test"
		if name != expected {
			t.Errorf("expected container name %q, got %q", expected, name)
		}
	})

	t.Run("EmptyName", func(t *testing.T) {
		// Given a service with no name
		service, _ := setup(t)
		service.SetName("")

		// When getting the container name
		name := service.GetContainerName()

		// Then it should return an empty string
		if name != "" {
			t.Errorf("expected empty container name, got %q", name)
		}
	})
}
