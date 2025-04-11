package services

import (
	"testing"

	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
)

type MockBaseServiceComponents struct {
	Injector          di.Injector
	MockShell         *shell.MockShell
	MockConfigHandler *config.MockConfigHandler
}

func setupSafeBaseServiceMocks(optionalInjector ...di.Injector) *MockBaseServiceComponents {
	var injector di.Injector
	if len(optionalInjector) > 0 {
		injector = optionalInjector[0]
	} else {
		injector = di.NewMockInjector()
	}

	mockShell := shell.NewMockShell(injector)
	mockConfigHandler := config.NewMockConfigHandler()

	// Register mock instances in the injector
	injector.Register("shell", mockShell)
	injector.Register("configHandler", mockConfigHandler)

	return &MockBaseServiceComponents{
		Injector:          injector,
		MockShell:         mockShell,
		MockConfigHandler: mockConfigHandler,
	}
}

func TestBaseService_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given: a set of mock components
		mocks := setupSafeBaseServiceMocks()

		// When: a new BaseService is created and initialized
		service := &BaseService{injector: mocks.Injector}
		err := service.Initialize()

		// Then: the initialization should succeed without errors
		if err != nil {
			t.Fatalf("expected no error during initialization, got %v", err)
		}

		// And: the resolved dependencies should be set correctly
		if service.configHandler == nil {
			t.Fatalf("expected configHandler to be set, got nil")
		}
		if service.shell == nil {
			t.Fatalf("expected shell to be set, got nil")
		}
	})

	t.Run("ErrorResolvingShell", func(t *testing.T) {
		// Given: a set of mock components
		mocks := setupSafeBaseServiceMocks()

		// And: the injector is set to return nil for the shell dependency
		mocks.Injector.Register("shell", nil)

		// When: a new BaseService is created and initialized
		service := &BaseService{injector: mocks.Injector}
		err := service.Initialize()

		// Then: the initialization should fail with an error
		if err == nil {
			t.Fatalf("expected an error during initialization, got nil")
		}
	})
}

func TestBaseService_WriteConfig(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given: a new BaseService
		service := &BaseService{}

		// When: WriteConfig is called
		err := service.WriteConfig()

		// Then: the WriteConfig should succeed without errors
		if err != nil {
			t.Fatalf("expected no error during WriteConfig, got %v", err)
		}
	})
}

func TestBaseService_SetAddress(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given: a new BaseService
		service := &BaseService{}

		// When: SetAddress is called with a valid IPv4 address
		err := service.SetAddress("192.168.1.1")

		// Then: the SetAddress should succeed without errors
		if err != nil {
			t.Fatalf("expected no error during SetAddress, got %v", err)
		}

		// And: the address should be set correctly
		expectedAddress := "192.168.1.1"
		if service.GetAddress() != expectedAddress {
			t.Fatalf("expected address '%s', got %v", expectedAddress, service.GetAddress())
		}
	})

	t.Run("InvalidAddress", func(t *testing.T) {
		// Given: a new BaseService
		service := &BaseService{}

		// When: SetAddress is called with an invalid IPv4 address
		err := service.SetAddress("invalid_address")

		// Then: the SetAddress should fail with an error
		if err == nil {
			t.Fatalf("expected an error during SetAddress, got nil")
		}

		// And: the error message should be as expected
		expectedErrorMessage := "invalid IPv4 address"
		if err.Error() != expectedErrorMessage {
			t.Fatalf("expected error message '%s', got %v", expectedErrorMessage, err)
		}
	})
}

func TestBaseService_GetAddress(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given: a new BaseService
		service := &BaseService{}
		service.SetAddress("192.168.1.1")

		// When: GetAddress is called
		address := service.GetAddress()

		// Then: the address should be as expected
		expectedAddress := "192.168.1.1"
		if address != expectedAddress {
			t.Fatalf("expected address '%s', got %v", expectedAddress, address)
		}
	})
}

func TestBaseService_GetName(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given: a new BaseService
		service := &BaseService{}
		service.SetName("TestService")

		// When: GetName is called
		name := service.GetName()

		// Then: the name should be as expected
		expectedName := "TestService"
		if name != expectedName {
			t.Fatalf("expected name '%s', got %v", expectedName, name)
		}
	})
}

func TestBaseService_GetHostname(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Setup mock components
		mockConfig := config.NewMockConfigHandler()
		mockConfig.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "dns.domain" {
				return "example.com"
			}
			return ""
		}

		// Initialize service
		service := &BaseService{
			name:          "test-service",
			configHandler: mockConfig,
		}

		// Get hostname
		hostname := service.GetHostname()

		// Verify hostname
		expectedHostname := "test-service.example.com"
		if hostname != expectedHostname {
			t.Errorf("Expected hostname %q, got %q", expectedHostname, hostname)
		}
	})

	t.Run("DefaultTLD", func(t *testing.T) {
		// Setup mock components
		mockConfig := config.NewMockConfigHandler()
		mockConfig.GetStringFunc = func(key string, defaultValue ...string) string {
			return defaultValue[0]
		}

		// Initialize service
		service := &BaseService{
			name:          "test-service",
			configHandler: mockConfig,
		}

		// Get hostname
		hostname := service.GetHostname()

		// Verify hostname uses default TLD
		expectedHostname := "test-service.test"
		if hostname != expectedHostname {
			t.Errorf("Expected hostname %q, got %q", expectedHostname, hostname)
		}
	})
}

func TestBaseService_IsLocalhostMode(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeBaseServiceMocks()
		service := &BaseService{
			injector: mocks.Injector,
		}
		service.Initialize()

		// Configure mock behavior
		mocks.MockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "vm.driver" {
				return "docker-desktop"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		// When: isLocalhostMode is called
		isLocal := service.isLocalhostMode()

		// Then: the result should be true for docker-desktop
		if !isLocal {
			t.Fatal("expected isLocalhostMode to be true for docker-desktop driver")
		}
	})

	t.Run("NotDockerDesktop", func(t *testing.T) {
		// Setup mock components
		mocks := setupSafeBaseServiceMocks()
		service := &BaseService{
			injector: mocks.Injector,
		}
		service.Initialize()

		// Configure mock behavior
		mocks.MockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "vm.driver" {
				return "lima"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		// When: isLocalhostMode is called
		isLocal := service.isLocalhostMode()

		// Then: the result should be false for non-docker-desktop driver
		if isLocal {
			t.Fatal("expected isLocalhostMode to be false for non-docker-desktop driver")
		}
	})
}

func TestBaseService_SupportsWildcard(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Initialize service
		service := &BaseService{}

		// Verify wildcard support is false by default
		if service.SupportsWildcard() {
			t.Error("Expected SupportsWildcard to return false")
		}
	})
}
