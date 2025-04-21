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

// =============================================================================
// Test Setup
// =============================================================================

// createDNSServiceMocks creates and returns mock components for DNS service tests
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

// =============================================================================
// Test Constructor
// =============================================================================

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

// =============================================================================
// Test Public Methods
// =============================================================================

func TestDNSService_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Create a mock injector with necessary mocks
		mocks := createDNSServiceMocks()

		// Given a DNSService with the mock injector
		service := NewDNSService(mocks.Injector)

		// When Initialize is called
		err := service.Initialize()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}
	})

	t.Run("ErrorResolvingConfigHandler", func(t *testing.T) {
		// Create a mock injector with necessary mocks
		mocks := createDNSServiceMocks()

		// Mock the Resolve method for configHandler to return an error
		mocks.Injector.Register("configHandler", "invalid")

		// Given a DNSService with the mock injector
		service := NewDNSService(mocks.Injector)

		// When Initialize is called
		err := service.Initialize()

		// Then an error should be returned
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

		// Given a DNSService with the mock injector
		service := NewDNSService(mocks.Injector)

		// When Initialize is called
		err := service.Initialize()

		// Then an error should be returned
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
		mocks.MockConfigHandler.SetContextValueFunc = func(key string, value any) error {
			if key == "dns.address" && value == "127.0.0.1" {
				setCalled = true
			}
			return nil
		}

		// Given a DNSService with the mock injector
		service := NewDNSService(mocks.Injector)

		// Initialize the service
		if err := service.Initialize(); err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// When SetAddress is called
		address := "127.0.0.1"
		err := service.SetAddress(address)

		// Then no error should be returned
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
		mocks.MockConfigHandler.SetContextValueFunc = func(key string, value any) error {
			if key == "dns.address" {
				return fmt.Errorf("mocked error setting address")
			}
			return nil
		}

		// Given a DNSService with the mock injector
		service := NewDNSService(mocks.Injector)

		// Initialize the service
		if err := service.Initialize(); err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// When SetAddress is called
		address := "127.0.0.1"
		err := service.SetAddress(address)

		// Then an error should be returned
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

		// Given a DNSService with the mock injector
		service := NewDNSService(mocks.Injector)

		// Initialize the service
		if err := service.Initialize(); err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// Set the service name
		service.SetName("dns")

		// When GetComposeConfig is called
		cfg, err := service.GetComposeConfig()

		// Then no error should be returned, and cfg should be correctly populated
		if err != nil {
			t.Fatalf("GetComposeConfig() error = %v", err)
		}
		if cfg == nil {
			t.Fatalf("Expected cfg to be non-nil when GetComposeConfig succeeds")
		}
		if len(cfg.Services) != 1 {
			t.Errorf("Expected 1 service, got %d", len(cfg.Services))
		}
		if cfg.Services[0].Name != "dns" {
			t.Errorf("Expected service name to be 'dns', got %s", cfg.Services[0].Name)
		}
	})

	t.Run("LocalhostPorts", func(t *testing.T) {
		// Setup mock components
		mocks := createDNSServiceMocks()
		service := NewDNSService(mocks.Injector)
		service.Initialize()

		// Set vm.driver to docker-desktop to simulate localhost mode
		mocks.MockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "vm.driver" {
				return "docker-desktop"
			}
			if key == "dns.domain" {
				return "test"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		// When GetComposeConfig is called
		cfg, err := service.GetComposeConfig()

		// Then no error should be returned, and cfg should be correctly populated
		if err != nil {
			t.Fatalf("GetComposeConfig() error = %v", err)
		}
		if cfg == nil {
			t.Fatalf("Expected cfg to be non-nil when GetComposeConfig succeeds")
		}
		if len(cfg.Services) != 1 {
			t.Errorf("Expected 1 service, got %d", len(cfg.Services))
		}
		if len(cfg.Services[0].Ports) != 2 {
			t.Errorf("Expected 2 ports, got %d", len(cfg.Services[0].Ports))
		}
		if cfg.Services[0].Ports[0].Published != "53" || cfg.Services[0].Ports[0].Protocol != "tcp" {
			t.Errorf("Expected port 53 with protocol tcp, got port %s with protocol %s", cfg.Services[0].Ports[0].Published, cfg.Services[0].Ports[0].Protocol)
		}
		if cfg.Services[0].Ports[1].Published != "53" || cfg.Services[0].Ports[1].Protocol != "udp" {
			t.Errorf("Expected port 53 with protocol udp, got port %s with protocol %s", cfg.Services[0].Ports[1].Published, cfg.Services[0].Ports[1].Protocol)
		}
	})
}

func TestDNSService_WriteConfig(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Create mocks and set up the mock context
		mocks := createDNSServiceMocks()

		// Given a DNSService with the mock config handler, context, and real DockerService
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

		// When WriteConfig is called
		err := service.WriteConfig()

		// Then no error should be returned
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
    reload
    loop
    forward . 1.1.1.1 8.8.8.8
}
`
		if string(writtenContent) != expectedCorefileContent {
			t.Errorf("Expected Corefile content:\n%s\nGot:\n%s", expectedCorefileContent, string(writtenContent))
		}
	})

	t.Run("SuccessLocalhost", func(t *testing.T) {
		// Create mocks and set up the mock context
		mocks := createDNSServiceMocks()

		// Given a DNSService with the mock config handler, context, and real DockerService
		service := NewDNSService(mocks.Injector)

		// Initialize the service
		if err := service.Initialize(); err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// Set the address to localhost to mock IsLocalhost behavior
		service.SetAddress("127.0.0.1")

		// Mock the writeFile function to capture the content written
		var writtenContent []byte
		originalWriteFile := writeFile
		defer func() { writeFile = originalWriteFile }()
		writeFile = func(filename string, data []byte, perm os.FileMode) error {
			writtenContent = data
			return nil
		}

		// When WriteConfig is called
		err := service.WriteConfig()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("WriteConfig() error = %v", err)
		}

		// Verify that the Corefile content is correctly formatted for localhost
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
    reload
    loop
    forward . 1.1.1.1 8.8.8.8
}
`
		if string(writtenContent) != expectedCorefileContent {
			t.Errorf("Expected Corefile content:\n%s\nGot:\n%s", expectedCorefileContent, string(writtenContent))
		}
	})

	t.Run("SuccessLocalhostMode", func(t *testing.T) {
		// Setup mock components
		mocks := createDNSServiceMocks()
		service := NewDNSService(mocks.Injector)

		// Initialize the service
		if err := service.Initialize(); err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// Set vm.driver to docker-desktop to simulate localhost mode
		mocks.MockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "vm.driver" {
				return "docker-desktop"
			}
			if key == "dns.domain" {
				return "test"
			}
			if key == "network.cidr_block" {
				return "192.168.1.0/24"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		// Create a mock service with a hostname
		mockService := NewMockService()
		mockService.GetNameFunc = func() string {
			return "test-service"
		}
		mockService.GetAddressFunc = func() string {
			return "192.168.1.2"
		}
		mockService.GetComposeConfigFunc = func() (*types.Config, error) {
			return &types.Config{
				Services: []types.ServiceConfig{
					{Name: "test-service"},
				},
			}, nil
		}
		mockService.GetHostnameFunc = func() string {
			return "test-service.test"
		}
		mockService.SupportsWildcardFunc = func() bool {
			return false
		}

		// Register the mock service
		mocks.Injector.Register("test-service", mockService)
		service.services = []Service{mockService}

		// Mock the writeFile function to capture the content written
		var writtenContent []byte
		originalWriteFile := writeFile
		defer func() { writeFile = originalWriteFile }()
		writeFile = func(filename string, data []byte, perm os.FileMode) error {
			writtenContent = data
			return nil
		}

		// Call WriteConfig
		err := service.WriteConfig()

		// Assert no error occurred
		if err != nil {
			t.Fatalf("WriteConfig() error = %v", err)
		}

		// Verify that the Corefile content includes both regular and localhost entries
		content := string(writtenContent)
		expectedEntries := []string{
			"192.168.1.2 test-service.test",
			"127.0.0.1 test-service.test",
		}
		for _, entry := range expectedEntries {
			if !strings.Contains(content, entry) {
				t.Errorf("Expected Corefile to contain entry %q, got:\n%s", entry, content)
			}
		}

		// Verify that the internal view is present
		if !strings.Contains(content, "view internal") {
			t.Errorf("Expected Corefile to contain internal view, got:\n%s", content)
		}
	})

	t.Run("SuccessWithHostname", func(t *testing.T) {
		// Setup mock components
		mocks := createDNSServiceMocks()
		service := NewDNSService(mocks.Injector)

		// Initialize the service
		if err := service.Initialize(); err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// Create a mock service with a hostname
		mockService := NewMockService()
		mockService.GetNameFunc = func() string {
			return "test-service"
		}
		mockService.GetAddressFunc = func() string {
			return "192.168.1.2"
		}
		mockService.GetComposeConfigFunc = func() (*types.Config, error) {
			return &types.Config{
				Services: []types.ServiceConfig{
					{Name: "test-service"},
				},
			}, nil
		}
		mockService.GetHostnameFunc = func() string {
			return "test-service.test"
		}
		mockService.SupportsWildcardFunc = func() bool {
			return false
		}

		// Register the mock service
		mocks.Injector.Register("test-service", mockService)
		service.services = []Service{mockService}

		// Mock the writeFile function to capture the content written
		var writtenContent []byte
		originalWriteFile := writeFile
		defer func() { writeFile = originalWriteFile }()
		writeFile = func(filename string, data []byte, perm os.FileMode) error {
			writtenContent = data
			return nil
		}

		// Call WriteConfig
		err := service.WriteConfig()

		// Assert no error occurred
		if err != nil {
			t.Fatalf("WriteConfig() error = %v", err)
		}

		// Verify that the Corefile content includes the service hostname
		expectedHostEntry := "192.168.1.2 test-service.test"
		content := string(writtenContent)
		if !strings.Contains(content, expectedHostEntry) {
			t.Errorf("Expected Corefile to contain host entry %q, got:\n%s", expectedHostEntry, content)
		}
	})

	t.Run("SuccessWithWildcard", func(t *testing.T) {
		// Setup mock components
		mocks := createDNSServiceMocks()
		service := NewDNSService(mocks.Injector)

		// Initialize the service
		if err := service.Initialize(); err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// Create a mock service with wildcard support
		mockService := NewMockService()
		mockService.GetNameFunc = func() string {
			return "test-service"
		}
		mockService.GetAddressFunc = func() string {
			return "192.168.1.2"
		}
		mockService.GetComposeConfigFunc = func() (*types.Config, error) {
			return &types.Config{
				Services: []types.ServiceConfig{
					{Name: "test-service"},
				},
			}, nil
		}
		mockService.GetHostnameFunc = func() string {
			return "test-service.test"
		}
		mockService.SupportsWildcardFunc = func() bool {
			return true
		}

		// Register the mock service
		mocks.Injector.Register("test-service", mockService)
		service.services = []Service{mockService}

		// Mock the writeFile function to capture the content written
		var writtenContent []byte
		originalWriteFile := writeFile
		defer func() { writeFile = originalWriteFile }()
		writeFile = func(filename string, data []byte, perm os.FileMode) error {
			writtenContent = data
			return nil
		}

		// Call WriteConfig
		err := service.WriteConfig()

		// Assert no error occurred
		if err != nil {
			t.Fatalf("WriteConfig() error = %v", err)
		}

		// Verify that the Corefile content includes both the service hostname and wildcard entry
		expectedHostEntry := "192.168.1.2 test-service.test"
		expectedWildcardMatches := []string{
			"template IN A",
			"match ^(.*)\\.test-service\\.test\\.$",
			`answer "{{ .Name }} 60 IN A 192.168.1.2"`,
			"fallthrough",
		}

		content := string(writtenContent)
		if !strings.Contains(content, expectedHostEntry) {
			t.Errorf("Expected Corefile to contain host entry %q, got:\n%s", expectedHostEntry, content)
		}
		for _, expectedMatch := range expectedWildcardMatches {
			if !strings.Contains(content, expectedMatch) {
				t.Errorf("Expected Corefile to contain %q, got:\n%s", expectedMatch, content)
			}
		}
	})

	t.Run("SuccessWithMissingNameOrAddress", func(t *testing.T) {
		// Setup mock components
		mocks := createDNSServiceMocks()
		service := NewDNSService(mocks.Injector)

		// Initialize the service
		if err := service.Initialize(); err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// Create a mock service with missing name
		mockServiceNoName := NewMockService()
		mockServiceNoName.GetNameFunc = func() string {
			return ""
		}
		mockServiceNoName.GetAddressFunc = func() string {
			return "192.168.1.2"
		}
		mockServiceNoName.GetComposeConfigFunc = func() (*types.Config, error) {
			return &types.Config{
				Services: []types.ServiceConfig{
					{Name: ""},
				},
			}, nil
		}

		// Create a mock service with missing address
		mockServiceNoAddress := NewMockService()
		mockServiceNoAddress.GetNameFunc = func() string {
			return "test-service"
		}
		mockServiceNoAddress.GetAddressFunc = func() string {
			return ""
		}
		mockServiceNoAddress.GetComposeConfigFunc = func() (*types.Config, error) {
			return &types.Config{
				Services: []types.ServiceConfig{
					{Name: "test-service"},
				},
			}, nil
		}

		// Register the mock services
		mocks.Injector.Register("test-service-no-name", mockServiceNoName)
		mocks.Injector.Register("test-service-no-address", mockServiceNoAddress)
		service.services = []Service{mockServiceNoName, mockServiceNoAddress}

		// Mock the writeFile function to capture the content written
		var writtenContent []byte
		originalWriteFile := writeFile
		defer func() { writeFile = originalWriteFile }()
		writeFile = func(filename string, data []byte, perm os.FileMode) error {
			writtenContent = data
			return nil
		}

		// Call WriteConfig
		err := service.WriteConfig()

		// Assert no error occurred
		if err != nil {
			t.Fatalf("WriteConfig() error = %v", err)
		}

		// Verify that the Corefile content does not include entries for services with missing name or address
		content := string(writtenContent)
		unexpectedEntries := []string{
			"192.168.1.2",  // Should not appear since service has no name
			"test-service", // Should not appear since service has no address
		}
		for _, entry := range unexpectedEntries {
			if strings.Contains(content, entry) {
				t.Errorf("Expected Corefile to not contain %q, got:\n%s", entry, content)
			}
		}
	})

	t.Run("ErrorCreatingDirectory", func(t *testing.T) {
		// Setup mock components
		mocks := createDNSServiceMocks()
		service := NewDNSService(mocks.Injector)

		// Initialize the service
		if err := service.Initialize(); err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// Mock mkdirAll to return an error
		originalMkdirAll := mkdirAll
		defer func() { mkdirAll = originalMkdirAll }()
		mkdirAll = func(path string, perm os.FileMode) error {
			return fmt.Errorf("mocked error creating directory")
		}

		// Call WriteConfig
		err := service.WriteConfig()

		// Assert error occurred
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}
		expectedErrorMessage := "error creating parent folders: mocked error creating directory"
		if err.Error() != expectedErrorMessage {
			t.Errorf("Expected error message '%s', got %v", expectedErrorMessage, err)
		}
	})

	t.Run("ErrorWritingFile", func(t *testing.T) {
		// Setup mock components
		mocks := createDNSServiceMocks()
		service := NewDNSService(mocks.Injector)

		// Initialize the service
		if err := service.Initialize(); err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// Mock writeFile to return an error
		originalWriteFile := writeFile
		defer func() { writeFile = originalWriteFile }()
		writeFile = func(filename string, data []byte, perm os.FileMode) error {
			return fmt.Errorf("mocked error writing file")
		}

		// Call WriteConfig
		err := service.WriteConfig()

		// Assert error occurred
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}
		expectedErrorMessage := "error writing Corefile: mocked error writing file"
		if err.Error() != expectedErrorMessage {
			t.Errorf("Expected error message '%s', got %v", expectedErrorMessage, err)
		}
	})

	t.Run("SuccessLocalhostModeWithWildcard", func(t *testing.T) {
		// Setup mock components
		mocks := createDNSServiceMocks()
		service := NewDNSService(mocks.Injector)

		// Initialize the service
		if err := service.Initialize(); err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// Set vm.driver to docker-desktop to simulate localhost mode
		mocks.MockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "vm.driver" {
				return "docker-desktop"
			}
			if key == "dns.domain" {
				return "test"
			}
			if key == "network.cidr_block" {
				return "192.168.1.0/24"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		// Create a mock service with wildcard support
		mockService := NewMockService()
		mockService.GetNameFunc = func() string {
			return "test-service"
		}
		mockService.GetAddressFunc = func() string {
			return "192.168.1.2"
		}
		mockService.GetComposeConfigFunc = func() (*types.Config, error) {
			return &types.Config{
				Services: []types.ServiceConfig{
					{Name: "test-service"},
				},
			}, nil
		}
		mockService.GetHostnameFunc = func() string {
			return "test-service.test"
		}
		mockService.SupportsWildcardFunc = func() bool {
			return true
		}

		// Register the mock service
		mocks.Injector.Register("test-service", mockService)
		service.services = []Service{mockService}

		// Mock the writeFile function to capture the content written
		var writtenContent []byte
		originalWriteFile := writeFile
		defer func() { writeFile = originalWriteFile }()
		writeFile = func(filename string, data []byte, perm os.FileMode) error {
			writtenContent = data
			return nil
		}

		// Call WriteConfig
		err := service.WriteConfig()

		// Assert no error occurred
		if err != nil {
			t.Fatalf("WriteConfig() error = %v", err)
		}

		// Verify that the Corefile content includes both regular and localhost wildcard entries
		content := string(writtenContent)
		expectedWildcardMatches := []string{
			"template IN A",
			"match ^(.*)\\.test-service\\.test\\.$",
			`answer "{{ .Name }} 60 IN A 192.168.1.2"`,
			"fallthrough",
			`answer "{{ .Name }} 60 IN A 127.0.0.1"`,
		}
		for _, expectedMatch := range expectedWildcardMatches {
			if !strings.Contains(content, expectedMatch) {
				t.Errorf("Expected Corefile to contain %q, got:\n%s", expectedMatch, content)
			}
		}

		// Verify that the internal view is present
		if !strings.Contains(content, "view internal") {
			t.Errorf("Expected Corefile to contain internal view, got:\n%s", content)
		}
	})
}
