package services

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/shell"
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
	mockConfigHandler.GetConfigFunc = func() *config.Context {
		enabled := true
		return &config.Context{
			Docker: &config.DockerConfig{
				Enabled: &enabled,
			},
			DNS: &config.DNSConfig{
				Create: &enabled,
				Name:   ptrString("test1"),
			},
		}
	}

	mockShell := shell.NewMockShell()
	mockContext := context.NewMockContext()
	mockContext.GetConfigRootFunc = func() (string, error) {
		return filepath.FromSlash("/mock/config/root"), nil
	}
	mockContext.GetContextFunc = func() (string, error) {
		return "mock-context", nil
	}

	// Create a generic mock service
	mockService := NewMockService()
	mockService.Initialize()
	injector.Register("dockerService", mockService)

	// Register mocks in the injector
	injector.Register("configHandler", mockConfigHandler)
	injector.Register("contextHandler", mockContext)
	injector.Register("shell", mockShell)

	return &MockComponents{
		Injector:          injector,
		MockConfigHandler: mockConfigHandler,
		MockShell:         mockShell,
		MockContext:       mockContext,
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

func TestDNSService_GetComposeConfig(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Create a mock injector with necessary mocks
		mocks := createDNSServiceMocks()

		// Create a mock dockerService using MakeDockerService
		mockDockerService := NewDockerService(mocks.Injector)
		mockDockerService.Initialize()
		mocks.Injector.Register("dockerService", mockDockerService)

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
		if cfg.Services[0].Name != "dns.test1" {
			t.Errorf("Expected service name to be 'dns.test1', got %s", cfg.Services[0].Name)
		}
	})

	t.Run("ErrorResolvingContext", func(t *testing.T) {
		// Create a mock injector that does not register contextHandler
		mockInjector := di.NewMockInjector()

		// Create a mock configHandler
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigFunc = func() *config.Context {
			return &config.Context{
				Docker: &config.DockerConfig{
					Enabled: ptrBool(true),
				},
				DNS: &config.DNSConfig{
					Create: ptrBool(true),
				},
			}
		}
		mockInjector.Register("configHandler", mockConfigHandler)

		// Given: a DNSService with the mock injector
		service := NewDNSService(mockInjector)

		// Initialize the service and expect an error
		err := service.Initialize()
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
		if !strings.Contains(err.Error(), "error resolving context") {
			t.Errorf("Expected error message to contain 'error resolving context', got %v", err)
		}
	})

	t.Run("ErrorRetrievingContextName", func(t *testing.T) {
		// Create a mock injector with necessary mocks
		mocks := createDNSServiceMocks()
		mocks.MockContext.GetContextFunc = func() (string, error) {
			return "", fmt.Errorf("mock error retrieving context name")
		}

		// Given: a DNSService initialized with the mock injector
		service := NewDNSService(mocks.Injector)

		// Initialize the service
		if err := service.Initialize(); err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// When: GetComposeConfig is called
		_, err := service.GetComposeConfig()

		// Then: an error should be returned
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
		if !strings.Contains(err.Error(), "mock error retrieving context name") {
			t.Errorf("Expected error message to contain 'mock error retrieving context name', got %v", err)
		}
	})

	t.Run("ErrorResolvingConfigHandler", func(t *testing.T) {
		// Create a mock context instance
		mocks := createDNSServiceMocks()

		// Create a mock injector that does not have configHandler registered
		mockInjector := di.NewMockInjector()
		mockInjector.Register("contextHandler", mocks.MockContext)

		// Given: a DNSService with the mock injector
		service := NewDNSService(mockInjector)

		// Initialize the service and expect an error
		err := service.Initialize()
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
		if !strings.Contains(err.Error(), "error resolving configHandler") {
			t.Errorf("Expected error message to contain 'error resolving configHandler', got %v", err)
		}
	})

	t.Run("DNSDisabled", func(t *testing.T) {
		// Create a mock config handler that returns DNS disabled
		mocks := createDNSServiceMocks()
		mocks.MockConfigHandler.GetConfigFunc = func() *config.Context {
			return &config.Context{
				DNS: &config.DNSConfig{
					Create: ptrBool(false),
				},
			}
		}

		// Given: a DNSService with the mock config handler and context instance
		service := NewDNSService(mocks.Injector)

		// Initialize the service
		if err := service.Initialize(); err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// When: GetComposeConfig is called
		cfg, err := service.GetComposeConfig()

		// Then: no error should be returned, and cfg should be nil
		if err != nil {
			t.Fatalf("GetComposeConfig() error = %v", err)
		}
		if cfg != nil {
			t.Errorf("Expected cfg to be nil when DNS is disabled, got %v", cfg)
		}
	})
}

func TestDNSService_WriteConfig(t *testing.T) {
	t.Run("DockerDisabled", func(t *testing.T) {
		// Create a mock config handler that returns Docker disabled
		mocks := createDNSServiceMocks()
		mocks.MockConfigHandler.GetConfigFunc = func() *config.Context {
			return &config.Context{
				Docker: &config.DockerConfig{
					Enabled: ptrBool(false),
				},
			}
		}

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

	t.Run("DockerEnabled", func(t *testing.T) {
		// Create mocks and set up the mock context
		mocks := createDNSServiceMocks()
		mocks.MockContext.GetConfigRootFunc = func() (string, error) {
			return "/mock/config/root", nil
		}
		mocks.MockContext.GetContextFunc = func() (string, error) {
			return "test", nil
		}

		// Register a real DockerService instance
		dockerService := NewDockerService(mocks.Injector)
		dockerService.Initialize()
		mocks.Injector.Register("dockerService", dockerService)

		// Configure the mock config handler to return Docker enabled
		mocks.MockConfigHandler.GetConfigFunc = func() *config.Context {
			return &config.Context{
				Docker: &config.DockerConfig{
					Enabled:     ptrBool(true),
					NetworkCIDR: ptrString("192.168.1.0/24"),
					Registries: []config.Registry{
						{Name: "service1", Remote: "remote1", Local: "local1"},
						{Name: "service2", Remote: "remote2", Local: "local2"},
					},
				},
				DNS: &config.DNSConfig{
					Create: ptrBool(true),
				},
			}
		}

		// Given: a DNSService with the mock config handler, context, and real DockerService
		service := NewDNSService(mocks.Injector)

		// Initialize the service
		if err := service.Initialize(); err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// Mock the writeFile function to avoid writing to the real file system
		writeFile = func(filename string, data []byte, perm os.FileMode) error {
			return nil
		}

		// Mock the mkdirAll function to avoid creating directories in the real file system
		mkdirAll = func(path string, perm os.FileMode) error {
			return nil
		}

		// When: WriteConfig is called
		err := service.WriteConfig()

		// Then: no error should be returned
		if err != nil {
			t.Fatalf("WriteConfig() error = %v", err)
		}
	})

	t.Run("ErrorRetrievingConfigRoot", func(t *testing.T) {
		// Create a mock context that returns an error on GetConfigRoot
		mocks := createDNSServiceMocks()
		mocks.MockContext.GetConfigRootFunc = func() (string, error) {
			return "", fmt.Errorf("mock error retrieving config root")
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
		if err == nil || !strings.Contains(err.Error(), "error retrieving config root") {
			t.Fatalf("expected error retrieving config root, got %v", err)
		}
	})

	t.Run("ErrorWritingCorefile", func(t *testing.T) {
		// Mock the GetConfigRoot function to return a mock path
		mocks := createDNSServiceMocks()
		mocks.MockContext.GetConfigRootFunc = func() (string, error) {
			return "/mock/config/root", nil
		}

		// Create a mock config handler that returns Docker enabled
		mocks.MockConfigHandler.GetConfigFunc = func() *config.Context {
			return &config.Context{
				Docker: &config.DockerConfig{
					Enabled: ptrBool(true),
				},
				DNS: &config.DNSConfig{
					Create: ptrBool(true),
				},
			}
		}

		mocks.Injector.Register("dockerService", NewMockService())

		// Given: a DNSService with the mock config handler, context, and DockerService
		service := NewDNSService(mocks.Injector)

		// Initialize the service
		if err := service.Initialize(); err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// Override the writeFile function to return an error
		writeFile = func(filename string, data []byte, perm os.FileMode) error {
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

		// Setup injector with mocks
		mocks := createDNSServiceMocks()

		// Mock the configHandler
		mocks.MockConfigHandler.GetConfigFunc = func() *config.Context {
			// Return a context config where Docker is enabled
			return &config.Context{
				Docker: &config.DockerConfig{
					Enabled: ptrBool(true),
				},
				DNS: &config.DNSConfig{
					Create: ptrBool(true),
				},
			}
		}

		// Mock the context
		mocks.MockContext.GetConfigRootFunc = func() (string, error) {
			return filepath.FromSlash("/invalid/path"), nil
		}
		mocks.MockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}

		// Create the DockerService instance
		dockerService := NewDockerService(mocks.Injector)
		dockerService.Initialize()
		mocks.Injector.Register("dockerService", dockerService)

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

	t.Run("DNSEnabledDockerDisabled", func(t *testing.T) {
		// Create a mock config handler that returns DNS enabled and Docker disabled
		mocks := createDNSServiceMocks()
		mocks.MockConfigHandler.GetConfigFunc = func() *config.Context {
			return &config.Context{
				Docker: &config.DockerConfig{
					Enabled: ptrBool(false),
				},
				DNS: &config.DNSConfig{
					Create: ptrBool(true),
				},
			}
		}

		// Create a mock context that returns a valid config root
		mocks.MockContext.GetConfigRootFunc = func() (string, error) {
			return filepath.FromSlash("/mock/config/root"), nil
		}
		mocks.MockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}

		// Register a real DockerService instance
		dockerService := NewDockerService(mocks.Injector)
		dockerService.Initialize()
		mocks.Injector.Register("dockerService", dockerService)

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

	t.Run("DNSEnabledDockerEnabledWithName", func(t *testing.T) {
		// Create a mock config handler that returns DNS enabled, Docker enabled, and a DNS name defined
		mocks := createDNSServiceMocks()
		mocks.MockConfigHandler.GetConfigFunc = func() *config.Context {
			return &config.Context{
				Docker: &config.DockerConfig{
					Enabled: ptrBool(true),
				},
				DNS: &config.DNSConfig{
					Create: ptrBool(true),
					Name:   ptrString("custom-dns"),
				},
			}
		}

		// Mock the GetConfigRoot function to return a mock path
		mocks.MockContext.GetConfigRootFunc = func() (string, error) {
			return "/mock/config/root", nil
		}

		// Use a mock DockerService
		mockDockerService := NewMockService()
		mockDockerService.Initialize()
		mocks.Injector.Register("dockerService", mockDockerService)

		service := NewDNSService(mocks.Injector)

		// Initialize the service
		if err := service.Initialize(); err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}

		// Mock the writeFile function to avoid writing to the real file system
		writeFile = func(_ string, _ []byte, _ os.FileMode) error {
			return nil
		}

		// When: WriteConfig is called
		err := service.WriteConfig()

		// Then: no error should be returned
		if err != nil {
			t.Fatalf("WriteConfig() error = %v", err)
		}
	})
}
