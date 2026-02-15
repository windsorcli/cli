package services

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/runtime/config"
)

// =============================================================================
// Test Setup
// =============================================================================

// setupDnsMocks creates and returns mock components for DNS service tests
func setupDnsMocks(t *testing.T, opts ...func(*ServicesTestMocks)) *ServicesTestMocks {
	t.Helper()

	// Create base mocks using setupServicesMocks
	mocks := setupServicesMocks(t, opts...)

	// Set up shell project root
	mocks.Shell.GetProjectRootFunc = func() (string, error) {
		return "/mock/project/root", nil
	}

	return mocks
}

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewDNSService(t *testing.T) {
	setup := func(t *testing.T) (*DNSService, *ServicesTestMocks) {
		t.Helper()
		mocks := setupDnsMocks(t)
		service := NewDNSService(mocks.Runtime)
		service.shims = mocks.Shims

		return service, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a mock injector
		service, _ := setup(t)

		// Then the service should not be nil
		if service == nil {
			t.Fatalf("NewDNSService() returned nil")
		}
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestDNSService_Initialize(t *testing.T) {
	setup := func(t *testing.T) (*DNSService, *ServicesTestMocks) {
		t.Helper()
		mocks := setupDnsMocks(t)
		service := NewDNSService(mocks.Runtime)
		service.shims = mocks.Shims

		return service, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a DNSService with mock components
		service, _ := setup(t)

		// Then the service should be properly initialized
		if service == nil {
			t.Fatalf("service should not be nil")
		}
	})
}

func TestDNSService_SetAddress(t *testing.T) {
	setup := func(t *testing.T) (*DNSService, *ServicesTestMocks) {
		t.Helper()
		mocks := setupDnsMocks(t)
		service := NewDNSService(mocks.Runtime)
		service.shims = mocks.Shims

		return service, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a DNSService with mock components
		service, mocks := setup(t)

		// When SetAddress is called
		address := "127.0.0.1"
		err := service.SetAddress(address, nil)

		setAddress := mocks.ConfigHandler.GetString("dns.address")

		// Then no error should be returned
		if err != nil {
			t.Fatalf("SetAddress() error = %v", err)
		}

		if setAddress != address {
			t.Errorf("Expected address to be %s, got %s", address, setAddress)
		}
	})

	t.Run("ErrorSettingAddress", func(t *testing.T) {
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.SetFunc = func(key string, value any) error {
			return fmt.Errorf("mocked error setting address")
		}
		mocks := setupDnsMocks(t, func(m *ServicesTestMocks) {
			m.ConfigHandler = mockConfigHandler
			m.Runtime.ConfigHandler = mockConfigHandler
		})
		service := NewDNSService(mocks.Runtime)
		service.shims = mocks.Shims

		// When SetAddress is called
		address := "127.0.0.1"
		err := service.SetAddress(address, nil)

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
	setup := func(t *testing.T) (*DNSService, *ServicesTestMocks) {
		t.Helper()
		mocks := setupDnsMocks(t)
		service := NewDNSService(mocks.Runtime)
		service.SetName("dns")
		service.shims = mocks.Shims

		return service, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a DNSService with mock components
		service, _ := setup(t)

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
		if service, exists := cfg.Services["dns"]; !exists || service.Name != "dns" {
			t.Errorf("Expected service with name 'dns', got %+v", cfg.Services)
		}
	})

	t.Run("LocalhostPorts", func(t *testing.T) {
		// Given a DNSService with mock components
		service, mocks := setup(t)

		// Set vm.driver and workstation.runtime to docker-desktop to simulate localhost mode
		mocks.ConfigHandler.Set("vm.driver", "docker-desktop")
		mocks.ConfigHandler.Set("workstation.runtime", "docker-desktop")
		mocks.ConfigHandler.Set("dns.domain", "test")
		mocks.ConfigHandler.Set("network.cidr_block", "192.168.1.0/24")

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

		// Get the DNS service from the map
		var dnsService types.ServiceConfig
		var found bool
		for _, svc := range cfg.Services {
			dnsService = svc
			found = true
			break
		}
		if !found {
			t.Fatalf("No service found in Services map")
		}

		if len(dnsService.Ports) != 2 {
			t.Errorf("Expected 2 ports, got %d", len(dnsService.Ports))
		}
		if dnsService.Ports[0].Published != "53" || dnsService.Ports[0].Protocol != "tcp" {
			t.Errorf("Expected port 53 with protocol tcp, got port %s with protocol %s", dnsService.Ports[0].Published, dnsService.Ports[0].Protocol)
		}
		if dnsService.Ports[1].Published != "53" || dnsService.Ports[1].Protocol != "udp" {
			t.Errorf("Expected port 53 with protocol udp, got port %s with protocol %s", dnsService.Ports[1].Published, dnsService.Ports[1].Protocol)
		}
	})
}

func TestDNSService_WriteConfig(t *testing.T) {
	setup := func(t *testing.T) (*DNSService, *ServicesTestMocks) {
		t.Helper()
		mocks := setupDnsMocks(t)
		service := NewDNSService(mocks.Runtime)
		service.shims = mocks.Shims

		return service, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a DNSService with mock components
		service, mocks := setup(t)

		// When WriteConfig is called
		err := service.WriteConfig()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("WriteConfig() error = %v", err)
		}

		// Verify shims were called
		if mocks.Shims.WriteFile == nil {
			t.Error("WriteFile shim was not called")
		}
		if mocks.Shims.MkdirAll == nil {
			t.Error("MkdirAll shim was not called")
		}
	})

	t.Run("Failure", func(t *testing.T) {
		// Given a DNSService with mock components
		service, mocks := setup(t)

		// Set up mock to fail
		mocks.Shims.WriteFile = func(filename string, data []byte, perm os.FileMode) error {
			return fmt.Errorf("write error")
		}

		// When WriteConfig is called
		err := service.WriteConfig()

		// Then an error should be returned
		if err == nil {
			t.Error("WriteConfig() expected error, got nil")
		}
	})

	t.Run("SuccessLocalhost", func(t *testing.T) {
		// Given a DNSService with mock components
		service, mocks := setup(t)

		// Set the address to localhost to mock IsLocalhost behavior
		service.SetAddress("127.0.0.1", nil)

		// Set the DNS domain
		mocks.ConfigHandler.Set("dns.domain", "test")

		// Mock the writeFile function to capture the content written
		var writtenContent []byte
		mocks.Shims.WriteFile = func(filename string, data []byte, perm os.FileMode) error {
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
		// Setup
		service, mocks := setup(t)
		mocks.ConfigHandler.Set("vm.driver", "docker-desktop")

		var writtenContent []byte
		mocks.Shims.WriteFile = func(filename string, data []byte, perm os.FileMode) error {
			writtenContent = data
			return nil
		}

		service.SetName("test")
		service.SetAddress("192.168.1.1", nil)

		// Execute
		err := service.WriteConfig()

		// Assert
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		content := string(writtenContent)
		if !strings.Contains(content, "127.0.0.1 test") {
			t.Errorf("Expected Corefile to contain entry \"127.0.0.1 test\", got:\n%s", content)
		}
	})

	t.Run("SuccessWithHostname", func(t *testing.T) {
		// Given a DNSService with mock components
		service, mocks := setup(t)

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
				Services: types.Services{
					"test-service": {Name: "test-service"},
				},
			}, nil
		}
		mockService.GetHostnameFunc = func() string {
			return "test-service.test"
		}
		mockService.SupportsWildcardFunc = func() bool {
			return false
		}

		// Set the mock service directly
		service.services = []Service{mockService}

		// Mock the writeFile function to capture the content written
		var writtenContent []byte
		mocks.Shims.WriteFile = func(filename string, data []byte, perm os.FileMode) error {
			writtenContent = data
			return nil
		}

		// When WriteConfig is called
		err := service.WriteConfig()

		// Then no error should be returned
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
		// Given a DNSService with mock components
		service, mocks := setup(t)

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
				Services: types.Services{
					"test-service": {Name: "test-service"},
				},
			}, nil
		}
		mockService.GetHostnameFunc = func() string {
			return "test-service.test"
		}
		mockService.SupportsWildcardFunc = func() bool {
			return true
		}

		// Set the mock service directly
		service.services = []Service{mockService}

		// Mock the writeFile function to capture the content written
		var writtenContent []byte
		mocks.Shims.WriteFile = func(filename string, data []byte, perm os.FileMode) error {
			writtenContent = data
			return nil
		}

		// When WriteConfig is called
		err := service.WriteConfig()

		// Then no error should be returned
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
		// Given a DNSService with mock components
		service, mocks := setup(t)

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
				Services: types.Services{
					"": {Name: ""},
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
				Services: types.Services{
					"test-service": {Name: "test-service"},
				},
			}, nil
		}

		// Register the mock services
		service.services = []Service{mockServiceNoName, mockServiceNoAddress}

		// Mock the writeFile function to capture the content written
		var writtenContent []byte
		mocks.Shims.WriteFile = func(filename string, data []byte, perm os.FileMode) error {
			writtenContent = data
			return nil
		}

		// When WriteConfig is called
		err := service.WriteConfig()

		// Then no error should be returned
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
		// Given a DNSService with mock components
		service, mocks := setup(t)

		// Mock mkdirAll to return an error
		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error {
			return fmt.Errorf("mocked error creating directory")
		}

		// When WriteConfig is called
		err := service.WriteConfig()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}
		expectedErrorMessage := "error creating parent folders: mocked error creating directory"
		if err.Error() != expectedErrorMessage {
			t.Errorf("Expected error message '%s', got %v", expectedErrorMessage, err)
		}
	})

	t.Run("ErrorWritingFile", func(t *testing.T) {
		// Given a DNSService with mock components
		service, mocks := setup(t)

		// Mock writeFile to return an error
		mocks.Shims.WriteFile = func(filename string, data []byte, perm os.FileMode) error {
			return fmt.Errorf("mocked error writing file")
		}

		// When WriteConfig is called
		err := service.WriteConfig()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}
		expectedErrorMessage := "error writing Corefile: mocked error writing file"
		if err.Error() != expectedErrorMessage {
			t.Errorf("Expected error message '%s', got %v", expectedErrorMessage, err)
		}
	})

	t.Run("SuccessLocalhostModeWithWildcard", func(t *testing.T) {
		// Setup
		service, mocks := setup(t)
		mocks.ConfigHandler.Set("vm.driver", "docker-desktop")

		var writtenContent []byte
		mocks.Shims.WriteFile = func(filename string, data []byte, perm os.FileMode) error {
			writtenContent = data
			return nil
		}

		service.SetName("test")
		service.SetAddress("192.168.1.1", nil)

		// Execute
		err := service.WriteConfig()

		// Assert
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		content := string(writtenContent)
		if !strings.Contains(content, "127.0.0.1 test") {
			t.Errorf("Expected Corefile to contain \"127.0.0.1 test\", got:\n%s", content)
		}
	})

	t.Run("SuccessRemovingCorefileDirectory", func(t *testing.T) {
		// Given a DNSService with mock components
		service, mocks := setup(t)

		var removedPath string
		var statCalled bool

		// Mock Stat to return a directory
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			statCalled = true
			if strings.Contains(name, "Corefile") {
				return &mockFileInfo{isDir: true}, nil
			}
			return &mockFileInfo{isDir: false}, nil
		}

		// Mock RemoveAll to capture the removed path
		mocks.Shims.RemoveAll = func(path string) error {
			removedPath = path
			return nil
		}

		// When WriteConfig is called
		err := service.WriteConfig()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("WriteConfig() error = %v", err)
		}

		// And Stat should have been called
		if !statCalled {
			t.Error("Expected Stat to be called")
		}

		// And RemoveAll should have been called with the Corefile path
		if removedPath == "" {
			t.Error("Expected RemoveAll to be called")
		}
		if !strings.Contains(removedPath, "Corefile") {
			t.Errorf("Expected RemoveAll to be called with Corefile path, got: %s", removedPath)
		}
	})
}

func TestDNSService_SetName(t *testing.T) {
	setup := func(t *testing.T) (*DNSService, *ServicesTestMocks) {
		t.Helper()
		mocks := setupDnsMocks(t)
		service := NewDNSService(mocks.Runtime)
		service.shims = mocks.Shims

		return service, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a DNSService with mock components
		service, _ := setup(t)

		// When SetName is called
		name := "new-dns"
		service.SetName(name)

		// Then the service name should be correctly set
		if service.GetName() != name {
			t.Errorf("Expected service name to be '%s', got '%s'", name, service.GetName())
		}
	})
}

func TestDNSService_GetName(t *testing.T) {
	setupSuccess := func(t *testing.T) (*DNSService, *ServicesTestMocks) {
		t.Helper()
		mocks := setupDnsMocks(t)
		service := NewDNSService(mocks.Runtime)
		service.shims = mocks.Shims
		service.SetName("dns") // Set the name to "dns"

		return service, mocks
	}

	setupError := func(t *testing.T) (*DNSService, *ServicesTestMocks) {
		t.Helper()
		mocks := setupDnsMocks(t)
		service := NewDNSService(mocks.Runtime)
		service.shims = mocks.Shims
		// Don't set the name

		return service, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a DNSService with mock components
		service, _ := setupSuccess(t)

		// When GetName is called
		name := service.GetName()

		// Then no error should be returned
		if name == "" {
			t.Fatalf("GetName() returned empty string")
		}

		// And the service name should be correctly returned
		if name != "dns" {
			t.Errorf("Expected service name to be 'dns', got '%s'", name)
		}
	})

	t.Run("ErrorGettingName", func(t *testing.T) {
		// Given a DNSService with no name set
		service, _ := setupError(t)

		// When GetName is called
		name := service.GetName()

		// Then an empty string should be returned
		if name != "" {
			t.Fatalf("Expected empty string, got '%s'", name)
		}
	})
}

func TestDNSService_GetHostname(t *testing.T) {
	setup := func(t *testing.T) (*DNSService, *ServicesTestMocks) {
		t.Helper()
		mocks := setupDnsMocks(t)
		service := NewDNSService(mocks.Runtime)
		service.shims = mocks.Shims
		service.SetName("test")

		// Set the dns.domain configuration value
		mocks.ConfigHandler.Set("dns.domain", "test")

		return service, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a DNSService with mock components
		service, _ := setup(t)

		// When GetHostname is called
		hostname := service.GetHostname()

		// Then the hostname should be correctly formatted
		expectedHostname := "test.test"
		if hostname != expectedHostname {
			t.Errorf("Expected hostname to be '%s', got '%s'", expectedHostname, hostname)
		}
	})

	t.Run("ErrorGettingHostname", func(t *testing.T) {
		// Given a DNSService with no name set
		service, mocks := setup(t)
		service.SetName("")                       // Clear the name
		mocks.ConfigHandler.Set("dns.domain", "") // Clear the domain

		// When GetHostname is called
		hostname := service.GetHostname()

		// Then an empty string should be returned
		if hostname != "" {
			t.Fatalf("Expected empty string, got '%s'", hostname)
		}
	})
}

func TestDNSService_SupportsWildcard(t *testing.T) {
	setup := func(t *testing.T) (*DNSService, *ServicesTestMocks) {
		t.Helper()
		mocks := setupDnsMocks(t)
		service := NewDNSService(mocks.Runtime)
		service.shims = mocks.Shims
		service.SetName("test")

		return service, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a DNSService with mock components
		service, _ := setup(t)

		// When SupportsWildcard is called
		supportsWildcard := service.SupportsWildcard()

		// Then false should be returned (default from BaseService)
		if supportsWildcard {
			t.Fatalf("Expected false (default from BaseService), got true")
		}
	})

	t.Run("ErrorGettingSupportsWildcard", func(t *testing.T) {
		// Given a DNSService with no wildcard support
		service, _ := setup(t)

		// When SupportsWildcard is called
		supportsWildcard := service.SupportsWildcard()

		// Then false should be returned
		if supportsWildcard {
			t.Fatalf("Expected false, got true")
		}
	})
}

func TestDNSService_GetIncusConfig(t *testing.T) {
	setup := func(t *testing.T) (*DNSService, *ServicesTestMocks) {
		t.Helper()
		mocks := setupDnsMocks(t)
		service := NewDNSService(mocks.Runtime)
		service.SetName("dns")
		service.shims = mocks.Shims

		return service, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a DNSService with mock components
		service, _ := setup(t)

		// When GetIncusConfig is called
		incusConfig, err := service.GetIncusConfig()

		// Then no error should be returned, and config should be correctly populated
		if err != nil {
			t.Fatalf("GetIncusConfig() error = %v", err)
		}
		if incusConfig == nil {
			t.Fatalf("Expected incusConfig to be non-nil when GetIncusConfig succeeds")
		}
		if incusConfig.Type != "container" {
			t.Errorf("Expected Type to be 'container', got %q", incusConfig.Type)
		}
		if incusConfig.Image != "docker:"+constants.DefaultDNSImage {
			t.Errorf("Expected Image to be 'docker:%s', got %q", constants.DefaultDNSImage, incusConfig.Image)
		}
		if incusConfig.Config["raw.lxc"] != "lxc.apparmor.profile=unconfined" {
			t.Errorf("Expected raw.lxc config, got %q", incusConfig.Config["raw.lxc"])
		}
		if incusConfig.Config["oci.entrypoint"] != "/coredns -conf /etc/coredns/Corefile" {
			t.Errorf("Expected oci.entrypoint config, got %q", incusConfig.Config["oci.entrypoint"])
		}
		corefileDevice, exists := incusConfig.Devices["corefile"]
		if !exists {
			t.Fatalf("Expected 'corefile' device to exist")
		}
		if corefileDevice["type"] != "disk" {
			t.Errorf("Expected corefile device type to be 'disk', got %q", corefileDevice["type"])
		}
		if corefileDevice["path"] != "/etc/coredns/Corefile" {
			t.Errorf("Expected corefile device path to be '/etc/coredns/Corefile', got %q", corefileDevice["path"])
		}
		if corefileDevice["readonly"] != "true" {
			t.Errorf("Expected corefile device readonly to be 'true', got %q", corefileDevice["readonly"])
		}
	})
}
