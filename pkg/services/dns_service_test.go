package services

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/compose-spec/compose-go/types"
	"github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/api/v1alpha1/dns"
	"github.com/windsorcli/cli/api/v1alpha1/docker"
	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
)

func createDNSServiceMocks(mockInjector ...di.Injector) *MockComponents {
	var injector di.Injector
	if len(mockInjector) > 0 {
		injector = mockInjector[0]
	} else {
		injector = di.NewMockInjector()
	}

	// Create mock instances
	mockConfigHandler := config.NewMockConfigHandler()
	mockConfigHandler.GetConfigFunc = func() *v1alpha1.Context {
		enabled := true
		return &v1alpha1.Context{
			Docker: &docker.DockerConfig{
				Enabled: &enabled,
			},
			DNS: &dns.DNSConfig{
				Enabled: &enabled,
				Domain:  ptrString("test"),
				Records: []string{"127.0.0.1 test", "192.168.1.1 test"},
			},
		}
	}
	mockConfigHandler.GetConfigRootFunc = func() (string, error) {
		return filepath.FromSlash("/invalid/path"), nil
	}
	mockConfigHandler.GetContextFunc = func() string {
		return "test-context"
	}

	mockConfigHandler.GetStringSliceFunc = func(key string, defaultValue ...[]string) []string {
		if key == "dns.records" {
			return []string{"127.0.0.1 test", "192.168.1.1 test"}
		}
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return nil
	}

	mockShell := shell.NewMockShell()
	mockConfigHandler.GetConfigRootFunc = func() (string, error) {
		return filepath.FromSlash("/mock/config/root"), nil
	}
	mockConfigHandler.GetContextFunc = func() string {
		return "mock-context"
	}

	// Create a generic mock service
	mockService := NewMockService()
	mockService.Initialize()
	injector.Register("dockerService", mockService)

	// Create a mock service that supports wildcard
	mockWildcardService := NewMockService()
	mockWildcardService.SupportsWildcardFunc = func() bool { return true }
	mockWildcardService.Initialize()
	injector.Register("wildcardService", mockWildcardService)

	// Register mocks in the injector
	injector.Register("configHandler", mockConfigHandler)
	injector.Register("shell", mockShell)

	// Mock the writeFile function to avoid writing to the real file system
	writeFile = func(filename string, data []byte, perm os.FileMode) error {
		return nil
	}

	// Mock the mkdirAll function to avoid creating directories in the real file system
	mkdirAll = func(path string, perm os.FileMode) error {
		return nil
	}

	return &MockComponents{
		Injector:          injector,
		MockConfigHandler: mockConfigHandler,
		MockShell:         mockShell,
		MockService:       mockService,
	}
}

func TestNewDNSService(t *testing.T) {
	// Create a mock injector
	mockInjector := di.NewMockInjector()

	// Call NewDNSService with the mock injector
	service := NewDNSService(mockInjector)

	// Verify that no error is returned
	if service == nil {
		t.Fatalf("NewDNSService() returned nil")
	}

	// Verify that the DIContainer is correctly set
	if service.injector != mockInjector {
		t.Errorf("NewDNSService() injector = %v, want %v", service.injector, mockInjector)
	}
}

func TestDNSService_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Create a mock injector with necessary mocks
		mocks := createDNSServiceMocks()

		// Given: a DNSService with the mock injector
		service := NewDNSService(mocks.Injector)

		// When: Initialize is called
		err := service.Initialize()

		// Then: no error should be returned
		if err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}
	})

	t.Run("ErrorResolvingConfigHandler", func(t *testing.T) {
		// Create a mock injector with necessary mocks
		mocks := createDNSServiceMocks()

		// Mock the Resolve method for configHandler to return an error
		mocks.Injector.Register("configHandler", "invalid")

		// Given: a DNSService with the mock injector
		service := NewDNSService(mocks.Injector)

		// When: Initialize is called
		err := service.Initialize()

		// Then: an error should be returned
		if err == nil {
			t.Fatalf("Expected error resolving configHandler, got nil")
		}
		expectedErrorMessage := "error resolving configHandler"
		if err.Error() != expectedErrorMessage {
			t.Errorf("Expected error message '%s', got %v", expectedErrorMessage, err)
		}
	})

	t.Run("ErrorResolvingServices", func(t *testing.T) {
		mockInjector := di.NewMockInjector()
		mocks := createDNSServiceMocks(mockInjector)

		// Set the resolve error for services using the correct type
		mockInjector.SetResolveAllError(new(Service), fmt.Errorf("error resolving services"))

		// Given: a DNSService with the mock injector
		service := NewDNSService(mocks.Injector)

		// When: Initialize is called
		err := service.Initialize()

		// Then: an error should be returned
		if err == nil {
			t.Fatalf("Expected error resolving services, got nil")
		}
		expectedErrorMessage := "error resolving services: error resolving services"
		if err.Error() != expectedErrorMessage {
			t.Errorf("Expected error message '%s', got %v", expectedErrorMessage, err)
		}
	})
}

func TestDNSService_SetAddress(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Create a mock injector with necessary mocks
		mocks := createDNSServiceMocks()

		// Mock the Set method of the config handler
		setCalled := false
		mocks.MockConfigHandler.SetContextValueFunc = func(key string, value interface{}) error {
			if key == "dns.address" && value == "127.0.0.1" {
				setCalled = true
			}
			return nil
		}

		// Given: a DNSService with the mock injector
		service := NewDNSService(mocks.Injector)

		// Initialize the service
		if err := service.Initialize(); err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// When: SetAddress is called
		address := "127.0.0.1"
		err := service.SetAddress(address)

		// Then: no error should be returned
		if err != nil {
			t.Fatalf("SetAddress() error = %v", err)
		}

		// And: the Set method should be called with the correct parameters
		if !setCalled {
			t.Errorf("Expected Set to be called with key 'dns.address' and value '%s'", address)
		}
	})

	t.Run("ErrorSettingAddress", func(t *testing.T) {
		// Create a mock injector with necessary mocks
		mocks := createDNSServiceMocks()

		// Mock the Set method of the config handler to return an error
		mocks.MockConfigHandler.SetContextValueFunc = func(key string, value interface{}) error {
			if key == "dns.address" {
				return fmt.Errorf("mocked error setting address")
			}
			return nil
		}

		// Given: a DNSService with the mock injector
		service := NewDNSService(mocks.Injector)

		// Initialize the service
		if err := service.Initialize(); err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// When: SetAddress is called
		address := "127.0.0.1"
		err := service.SetAddress(address)

		// Then: an error should be returned
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}
		expectedErrorMessage := "error setting DNS address: mocked error setting address"
		if err.Error() != expectedErrorMessage {
			t.Errorf("Expected error message '%s', got %v", expectedErrorMessage, err)
		}
	})
}

func TestDNSService_GetComposeConfig(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Create a mock injector with necessary mocks
		mocks := createDNSServiceMocks()

		// Given: a DNSService with the mock injector
		service := NewDNSService(mocks.Injector)

		// Initialize the service
		if err := service.Initialize(); err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// When: GetComposeConfig is called
		cfg, err := service.GetComposeConfig()

		// Then: no error should be returned, and cfg should be correctly populated
		if err != nil {
			t.Fatalf("GetComposeConfig() error = %v", err)
		}
		if cfg == nil {
			t.Fatalf("Expected cfg to be non-nil when GetComposeConfig succeeds")
		}
		if len(cfg.Services) != 1 {
			t.Errorf("Expected 1 service, got %d", len(cfg.Services))
		}
		if cfg.Services[0].Name != "dns.test" {
			t.Errorf("Expected service name to be 'dns.test', got %s", cfg.Services[0].Name)
		}
	})

	t.Run("UseHostNetwork", func(t *testing.T) {
		// Create a mock injector with necessary mocks
		mocks := createDNSServiceMocks()

		// Given: a DNSService with the mock injector
		service := NewDNSService(mocks.Injector)

		// Mock the config handler to return "docker-desktop" for vm.driver
		mocks.MockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "vm.driver" {
				return "docker-desktop"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		// Initialize the service
		if err := service.Initialize(); err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// When: GetComposeConfig is called
		cfg, err := service.GetComposeConfig()

		// Then: no error should be returned, and cfg should be correctly populated
		if err != nil {
			t.Fatalf("GetComposeConfig() error = %v", err)
		}
		if cfg == nil {
			t.Fatalf("Expected cfg to be non-nil when GetComposeConfig succeeds")
		}
		if len(cfg.Services) != 1 {
			t.Errorf("Expected 1 service, got %d", len(cfg.Services))
		}

		// Check if the service is using host network by verifying published ports
		if len(cfg.Services[0].Ports) != 2 {
			t.Errorf("Expected 2 ports to be published, got %d", len(cfg.Services[0].Ports))
		}
		for _, port := range cfg.Services[0].Ports {
			if port.Published != "53" {
				t.Errorf("Expected published port to be '53', got %s", port.Published)
			}
		}
	})

	t.Run("WildcardService", func(t *testing.T) {
		// Create a mock injector with necessary mocks
		mocks := createDNSServiceMocks()

		// Given: a DNSService with the mock injector
		service := NewDNSService(mocks.Injector)

		// Initialize the service
		if err := service.Initialize(); err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// When: GetComposeConfig is called for a wildcard service
		cfg, err := service.GetComposeConfig()

		// Then: no error should be returned, and cfg should be correctly populated
		if err != nil {
			t.Fatalf("GetComposeConfig() error = %v", err)
		}
		if cfg == nil {
			t.Fatalf("Expected cfg to be non-nil when GetComposeConfig succeeds")
		}
		if len(cfg.Services) != 1 {
			t.Errorf("Expected 1 service, got %d", len(cfg.Services))
		}

		// Check if the service supports wildcard
		wildcardSupported := false
		for _, svc := range service.services {
			if svc.SupportsWildcard() {
				wildcardSupported = true
				break
			}
		}
		if !wildcardSupported {
			t.Errorf("Expected at least one service to support wildcard")
		}
	})
}

func TestDNSService_WriteConfig(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Create mocks and set up the mock context
		mocks := createDNSServiceMocks()

		// Given: a DNSService with the mock config handler, context, and real DockerService
		service := NewDNSService(mocks.Injector)

		// Initialize the service
		if err := service.Initialize(); err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// Mock the writeFile function to capture the content written
		var writtenContent []byte
		originalWriteFile := writeFile
		defer func() { writeFile = originalWriteFile }()
		writeFile = func(filename string, data []byte, perm os.FileMode) error {
			writtenContent = data
			return nil
		}

		// When: WriteConfig is called
		err := service.WriteConfig()

		// Then: no error should be returned
		if err != nil {
			t.Fatalf("WriteConfig() error = %v", err)
		}

		// Verify that the Corefile content is correctly formatted
		expectedCorefileContent := `test:53 {
    hosts {
        127.0.0.1 test
        192.168.1.1 test
        fallthrough
    }

    reload
    loop
    forward . 1.1.1.1 8.8.8.8
}
.:53 {
    forward . 1.1.1.1 8.8.8.8
    reload
    loop
}
`
		if string(writtenContent) != expectedCorefileContent {
			t.Errorf("Expected Corefile content:\n%s\nGot:\n%s", expectedCorefileContent, string(writtenContent))
		}
	})

	t.Run("SuccessUseHostNetwork", func(t *testing.T) {
		// Create mocks and set up the mock context
		mocks := createDNSServiceMocks()

		// Given: a DNSService with the mock config handler, context, and real DockerService
		service := NewDNSService(mocks.Injector)

		// Initialize the service
		if err := service.Initialize(); err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// Mock the config handler to simulate UseHostNetwork returning true
		mocks.MockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "vm.driver" {
				return "docker-desktop"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		// Mock the config handler to provide a network CIDR
		mocks.MockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "network.cidr_block" {
				return "192.168.1.0/24"
			}
			if key == "vm.driver" {
				return "docker-desktop"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		// Mock the writeFile function to capture the content written
		var writtenContent []byte
		originalWriteFile := writeFile
		defer func() { writeFile = originalWriteFile }()
		writeFile = func(filename string, data []byte, perm os.FileMode) error {
			writtenContent = data
			return nil
		}

		// When: WriteConfig is called
		err := service.WriteConfig()

		// Then: no error should be returned
		if err != nil {
			t.Fatalf("WriteConfig() error = %v", err)
		}

		// Verify that the Corefile content is correctly formatted for UseHostNetwork
		expectedCorefileContent := `test:53 {
    view internal {
        expr incidr(client_ip(), '192.168.1.0/24')
    }
    hosts {
        127.0.0.1 test
        192.168.1.1 test
        fallthrough
    }

    reload
    loop
    forward . 1.1.1.1 8.8.8.8
}
test:53 {
    hosts {
        127.0.0.1 test
        192.168.1.1 test
        fallthrough
    }

    reload
    loop
    forward . 1.1.1.1 8.8.8.8
}
.:53 {
    forward . 1.1.1.1 8.8.8.8
    reload
    loop
}
`
		if string(writtenContent) != expectedCorefileContent {
			t.Errorf("Expected Corefile content:\n%s\nGot:\n%s", expectedCorefileContent, string(writtenContent))
		}
	})

	t.Run("ErrorRetrievingProjectRoot", func(t *testing.T) {
		// Create a mock context that returns an error on GetProjectRoot
		mocks := createDNSServiceMocks()
		mocks.MockShell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("mock error retrieving project root")
		}

		mocks.Injector.Register("dockerService", NewMockService())

		// Given: a DNSService with the mock context
		service := NewDNSService(mocks.Injector)

		// Initialize the service
		if err := service.Initialize(); err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// When: WriteConfig is called
		err := service.WriteConfig()

		// Then: an error should be returned
		if err == nil || !strings.Contains(err.Error(), "error retrieving project root") {
			t.Fatalf("expected error retrieving project root, got %v", err)
		}
	})

	t.Run("ValidAddress", func(t *testing.T) {
		// Create a mock context and config handler
		mocks := createDNSServiceMocks()

		// Create a mock service that returns a valid address
		mockService := NewMockService()
		mockService.GetComposeConfigFunc = func() (*types.Config, error) {
			return &types.Config{
				Services: []types.ServiceConfig{
					{Name: "mockService"},
				},
			}, nil
		}
		mockService.GetAddressFunc = func() string {
			return "192.168.1.1"
		}
		mockService.GetHostnameFunc = func() string {
			return "mockService.test"
		}
		mocks.Injector.Register("dockerService", mockService)

		// Given: a DNSService with the mock config handler, context, and DockerService
		service := NewDNSService(mocks.Injector)

		// Initialize the service
		if err := service.Initialize(); err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// When: WriteConfig is called
		err := service.WriteConfig()

		// Then: no error should be returned
		if err != nil {
			t.Fatalf("WriteConfig() error = %v", err)
		}
	})

	t.Run("ErrorWritingCorefile", func(t *testing.T) {
		// Mock the GetConfigRoot function to return a mock path
		mocks := createDNSServiceMocks()

		// Given: a DNSService with the mock config handler, context, and DockerService
		service := NewDNSService(mocks.Injector)

		// Initialize the service
		if err := service.Initialize(); err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// Mock the writeFile function to return an error
		writeFile = func(_ string, _ []byte, _ os.FileMode) error {
			return fmt.Errorf("mock error writing file")
		}

		// When: WriteConfig is called
		err := service.WriteConfig()

		// Then: an error should be returned
		if err == nil || !strings.Contains(err.Error(), "error writing Corefile") {
			t.Fatalf("expected error writing Corefile, got %v", err)
		}
	})

	t.Run("MkdirAllError", func(t *testing.T) {
		// Setup injector with mocks
		mocks := createDNSServiceMocks()

		// Save the original mkdirAll function
		originalMkdirAll := mkdirAll

		// Override mkdirAll to simulate an error
		mkdirAll = func(path string, perm os.FileMode) error {
			return fmt.Errorf("mock error creating directories")
		}

		// Restore the original mkdirAll after the test
		defer func() {
			mkdirAll = originalMkdirAll
		}()

		// Create the DNSService instance
		service := NewDNSService(mocks.Injector)

		// Initialize the service
		if err := service.Initialize(); err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// Call WriteConfig and expect an error
		err := service.WriteConfig()

		// Check if the error matches the expected error
		expectedError := "error creating parent folders: mock error creating directories"
		if err == nil || err.Error() != expectedError {
			t.Fatalf("expected error %v, got %v", expectedError, err)
		}
	})

	t.Run("NoServiceName", func(t *testing.T) {
		mocks := createDNSServiceMocks()

		// Create a mock service with no Name property
		mockService := NewMockService()
		mockService.GetComposeConfigFunc = func() (*types.Config, error) {
			return &types.Config{
				Services: []types.ServiceConfig{
					{Name: ""},
				},
			}, nil
		}
		mocks.Injector.Register("dockerService", mockService)

		service := NewDNSService(mocks.Injector)

		// Initialize the service
		if err := service.Initialize(); err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// Mock the writeFile function to capture the content written
		var writtenContent []byte
		originalWriteFile := writeFile
		defer func() { writeFile = originalWriteFile }()
		writeFile = func(filename string, data []byte, perm os.FileMode) error {
			writtenContent = data
			return nil
		}

		// When: WriteConfig is called
		err := service.WriteConfig()

		// Then: no error should be returned
		if err != nil {
			t.Fatalf("WriteConfig() error = %v", err)
		}

		// Verify that the Corefile content does not contain any additional entries for unnamed services
		expectedCorefileContent := `test:53 {
    hosts {
        127.0.0.1 test
        192.168.1.1 test
        fallthrough
    }

    reload
    loop
    forward . 1.1.1.1 8.8.8.8
}
.:53 {
    forward . 1.1.1.1 8.8.8.8
    reload
    loop
}
`
		if string(writtenContent) != expectedCorefileContent {
			t.Errorf("Expected Corefile content:\n%s\nGot:\n%s", expectedCorefileContent, string(writtenContent))
		}
	})

	t.Run("NoServiceAddress", func(t *testing.T) {
		// Create mocks and set up the mock context
		mocks := createDNSServiceMocks()

		// Create a mock service with GetComposeConfig returning a valid config
		mockService := NewMockService()
		mockService.GetComposeConfigFunc = func() (*types.Config, error) {
			return &types.Config{
				Services: []types.ServiceConfig{
					{
						Name:          "mockService",
						ContainerName: "mockServiceContainer",
					},
				},
			}, nil
		}
		mocks.Injector.Register("dockerService", mockService)

		// Given: a DNSService with the mock config handler, context, and real DockerService
		service := NewDNSService(mocks.Injector)

		// Initialize the service
		if err := service.Initialize(); err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// Mock the writeFile function to capture the content written
		var writtenContent []byte
		originalWriteFile := writeFile
		defer func() { writeFile = originalWriteFile }()
		writeFile = func(filename string, data []byte, perm os.FileMode) error {
			writtenContent = data
			return nil
		}

		// When: WriteConfig is called
		err := service.WriteConfig()

		// Then: no error should be returned
		if err != nil {
			t.Fatalf("WriteConfig() error = %v", err)
		}

		// Verify that the Corefile content contains the entry for the service with the specific config
		expectedCorefileContent := `test:53 {
    hosts {
        127.0.0.1 test
        192.168.1.1 test
        fallthrough
    }

    reload
    loop
    forward . 1.1.1.1 8.8.8.8
}
.:53 {
    forward . 1.1.1.1 8.8.8.8
    reload
    loop
}
`
		if string(writtenContent) != expectedCorefileContent {
			t.Errorf("Expected Corefile content:\n%s\nGot:\n%s", expectedCorefileContent, string(writtenContent))
		}
	})

	t.Run("UseHostNetwork", func(t *testing.T) {
		// Create mocks and set up the mock context
		mocks := createDNSServiceMocks()

		// Create a mock service with UseHostNetwork returning true, and providing Name and GetAddress
		mockService := NewMockService()
		mockService.UseHostNetworkFunc = func() bool {
			return true
		}
		mockService.GetComposeConfigFunc = func() (*types.Config, error) {
			return &types.Config{
				Services: []types.ServiceConfig{
					{
						Name:          "mockService",
						ContainerName: "mockServiceContainer",
					},
				},
			}, nil
		}
		mockService.GetAddressFunc = func() string {
			return "192.168.1.1"
		}
		mocks.Injector.Register("dockerService", mockService)

		// Given: a DNSService with the mock config handler, context, and mock DockerService
		service := NewDNSService(mocks.Injector)

		// Initialize the service
		if err := service.Initialize(); err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// Mock the writeFile function to capture the content written
		var writtenContent []byte
		originalWriteFile := writeFile
		defer func() { writeFile = originalWriteFile }()
		writeFile = func(filename string, data []byte, perm os.FileMode) error {
			writtenContent = data
			return nil
		}

		// When: WriteConfig is called
		err := service.WriteConfig()

		// Then: no error should be returned
		if err != nil {
			t.Fatalf("WriteConfig() error = %v", err)
		}

		// Verify that the Corefile content is correctly formatted for UseHostNetwork
		expectedCorefileContent := `test:53 {
    hosts {
        192.168.1.1 
        127.0.0.1 test
        192.168.1.1 test
        fallthrough
    }

    reload
    loop
    forward . 1.1.1.1 8.8.8.8
}
.:53 {
    forward . 1.1.1.1 8.8.8.8
    reload
    loop
}
`
		if string(writtenContent) != expectedCorefileContent {
			t.Errorf("Expected Corefile content:\n%s\nGot:\n%s", expectedCorefileContent, string(writtenContent))
		}
	})

	t.Run("SupportsWildcard", func(t *testing.T) {
		// Create a mock injector with necessary mocks
		mocks := createDNSServiceMocks()

		// Mock the SupportsWildcard function to return true
		mocks.MockService.SupportsWildcardFunc = func() bool {
			return true
		}

		// Mock the UseHostNetwork function to return true
		mocks.MockService.UseHostNetworkFunc = func() bool {
			return true
		}

		// Mock the GetComposeConfig function to return a valid config with a specific name
		mocks.MockService.GetComposeConfigFunc = func() (*types.Config, error) {
			return &types.Config{
				Services: []types.ServiceConfig{
					{
						Name: "test-service",
					},
				},
			}, nil
		}

		// Mock the GetAddress function to return a valid address
		mocks.MockService.GetAddressFunc = func() string {
			return "192.168.1.1"
		}

		// Mock the GetHostname function to return a valid hostname
		mocks.MockService.GetHostnameFunc = func() string {
			return "test-service.test"
		}

		// Given: a DNSService with the mock injector
		service := NewDNSService(mocks.Injector)

		// Initialize the service
		if err := service.Initialize(); err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// Mock the writeFile function to capture the content written
		var writtenContent []byte
		originalWriteFile := writeFile
		defer func() { writeFile = originalWriteFile }()
		writeFile = func(filename string, data []byte, perm os.FileMode) error {
			writtenContent = data
			return nil
		}

		// When: WriteConfig is called
		err := service.WriteConfig()

		// Then: no error should be returned
		if err != nil {
			t.Fatalf("WriteConfig() error = %v", err)
		}

		// Verify that the Corefile content includes the expected entries
		expectedCorefileContent := `test:53 {
    hosts {
        192.168.1.1 test-service.test
        127.0.0.1 test
        192.168.1.1 test
        fallthrough
    }

    template IN A {
        match ^(.*)\.test-service\.test\.$
        answer "{{ .Name }} 60 IN A 192.168.1.1"
        fallthrough
    }

    reload
    loop
    forward . 1.1.1.1 8.8.8.8
}
.:53 {
    forward . 1.1.1.1 8.8.8.8
    reload
    loop
}
`
		if string(writtenContent) != expectedCorefileContent {
			t.Errorf("Expected Corefile content:\n%s\nGot:\n%s", expectedCorefileContent, string(writtenContent))
		}
	})
}
