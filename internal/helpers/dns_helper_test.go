package helpers

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

func TestNewDNSHelper(t *testing.T) {
	// Create a mock injector
	mockInjector := di.NewMockInjector()

	// Call NewDNSHelper with the mock injector
	helper, err := NewDNSHelper(mockInjector)

	// Verify that no error is returned
	if err != nil {
		t.Fatalf("NewDNSHelper() error = %v, wantErr %v", err, false)
	}

	// Verify that the helper is not nil
	if helper == nil {
		t.Fatalf("NewDNSHelper() returned nil, expected non-nil DNSHelper")
	}

	// Verify that the DIContainer is correctly set
	if helper.injector != mockInjector {
		t.Errorf("NewDNSHelper() injector = %v, want %v", helper.injector, mockInjector)
	}
}

func TestDNSHelper_GetComposeConfig(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Create a mock injector
		mockInjector := di.NewMockInjector()

		// Create a mock context
		mockContext := context.NewMockContext()
		mockContext.GetConfigRootFunc = func() (string, error) {
			return filepath.FromSlash("/mock/config/root"), nil
		}
		mockContext.GetContextFunc = func() (string, error) {
			return "mock-context", nil
		}
		mockInjector.Register("contextHandler", mockContext)

		// Create a mock configHandler
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
		// Create a mock shell
		mockShell := shell.NewMockShell()
		mockInjector.Register("shell", mockShell)
		mockInjector.Register("configHandler", mockConfigHandler)

		// Create a mock dockerHelper using MakeDockerHelper
		mockDockerHelper, err := NewDockerHelper(mockInjector)
		if err != nil {
			t.Fatalf("NewDockerHelper() error = %v", err)
		}
		mockInjector.Register("dockerHelper", mockDockerHelper)

		// Given: a DNSHelper with the mock injector
		helper, err := NewDNSHelper(mockInjector)
		if err != nil {
			t.Fatalf("NewDNSHelper() error = %v", err)
		}

		// When: GetComposeConfig is called
		cfg, err := helper.GetComposeConfig()

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

		// Given: a DNSHelper with the mock injector
		helper, err := NewDNSHelper(mockInjector)
		if err != nil {
			t.Fatalf("NewDNSHelper() error = %v", err)
		}

		// When: GetComposeConfig is called
		_, err = helper.GetComposeConfig()

		// Then: an error should be returned
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
		if !strings.Contains(err.Error(), "error resolving context") {
			t.Errorf("Expected error message to contain 'error resolving context', got %v", err)
		}
	})

	t.Run("ErrorRetrievingContextName", func(t *testing.T) {
		// Create a mock context instance that returns an error when GetContext is called
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "", fmt.Errorf("error retrieving context name")
		}

		// Create a mock injector
		mockInjector := di.NewMockInjector()
		mockInjector.Register("contextHandler", mockContext)

		// Given: a DNSHelper with the mock injector
		helper := &DNSHelper{
			injector: mockInjector,
		}

		// When: GetComposeConfig is called
		_, err := helper.GetComposeConfig()

		// Then: an error should be returned
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
		if err.Error() != "error retrieving context name: error retrieving context name" {
			t.Errorf("Expected error message 'error retrieving context name: error retrieving context name', got %v", err)
		}
	})

	t.Run("ErrorResolvingCliConfigHandler", func(t *testing.T) {
		// Create a mock context instance
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "mock-context", nil
		}

		// Create a mock injector that does not have configHandler registered
		mockInjector := di.NewMockInjector()
		mockInjector.Register("contextHandler", mockContext)

		// Given: a DNSHelper with the mock injector
		helper := &DNSHelper{
			injector: mockInjector,
		}

		// When: GetComposeConfig is called
		_, err := helper.GetComposeConfig()

		// Then: an error should be returned
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
		if !strings.Contains(err.Error(), "error resolving configHandler") {
			t.Errorf("Expected error message to contain 'error resolving configHandler', got %v", err)
		}
	})

	t.Run("DNSDisabled", func(t *testing.T) {
		// Create a mock config handler that returns DNS disabled
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigFunc = func() *config.Context {
			return &config.Context{
				DNS: &config.DNSConfig{
					Create: ptrBool(false),
				},
			}
		}

		// Create a mock shell
		mockShell := shell.NewMockShell()

		// Create a mock context instance
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "mock-context", nil
		}

		// Given: a DNSHelper with the mock config handler and context instance
		mockInjector := di.NewMockInjector()
		mockInjector.Register("configHandler", mockConfigHandler)
		mockInjector.Register("shell", mockShell)
		mockInjector.Register("contextHandler", mockContext)
		helper, err := NewDNSHelper(mockInjector)
		if err != nil {
			t.Fatalf("NewDNSHelper() error = %v", err)
		}

		// When: GetComposeConfig is called
		cfg, err := helper.GetComposeConfig()

		// Then: no error should be returned, and cfg should be nil
		if err != nil {
			t.Fatalf("GetComposeConfig() error = %v", err)
		}
		if cfg != nil {
			t.Errorf("Expected cfg to be nil when DNS is disabled, got %v", cfg)
		}
	})
}

func TestDNSHelper_WriteConfig(t *testing.T) {
	// Shared resources
	mockContext := context.NewMockContext()
	mockConfigHandler := config.NewMockConfigHandler()
	mockDockerHelper := NewMockHelper()
	mockShell := shell.NewMockShell()
	mockInjector := di.NewMockInjector()

	t.Run("DockerDisabled", func(t *testing.T) {
		// Create a mock config handler that returns Docker disabled
		mockConfigHandler.GetConfigFunc = func() *config.Context {
			return &config.Context{
				Docker: &config.DockerConfig{
					Enabled: ptrBool(false),
				},
			}
		}

		// Create a mock context
		mockContext.GetConfigRootFunc = func() (string, error) {
			return filepath.FromSlash("/mock/config/root"), nil
		}

		mockInjector.Register("configHandler", mockConfigHandler)
		mockInjector.Register("contextHandler", mockContext)
		mockInjector.Register("shell", mockShell)

		helper, err := NewDNSHelper(mockInjector)
		if err != nil {
			t.Fatalf("NewDNSHelper() error = %v", err)
		}

		// When: WriteConfig is called
		err = helper.WriteConfig()

		// Then: no error should be returned
		if err != nil {
			t.Fatalf("WriteConfig() error = %v", err)
		}
	})

	t.Run("DockerEnabled", func(t *testing.T) {
		// Create a temporary directory for config root
		tempDir := t.TempDir()

		// Create a mock context that returns the temp directory as config root
		mockContext.GetConfigRootFunc = func() (string, error) {
			return tempDir, nil
		}

		// Mock the GetContext function to avoid the error
		mockContext.GetContextFunc = func() (string, error) {
			return "test", nil
		}

		// Create a real DockerHelper
		mockInjector.Register("configHandler", mockConfigHandler)
		mockInjector.Register("contextHandler", mockContext)
		mockInjector.Register("shell", mockShell)

		dockerHelper, err := NewDockerHelper(mockInjector)
		if err != nil {
			t.Fatalf("NewDockerHelper() error = %v", err)
		}
		mockInjector.Register("dockerHelper", dockerHelper)

		// Create a mock config handler that returns Docker enabled
		mockConfigHandler.GetConfigFunc = func() *config.Context {
			return &config.Context{
				Docker: &config.DockerConfig{
					Enabled:     ptrBool(true),
					NetworkCIDR: ptrString("192.168.1.0/24"),
					Registries: []config.Registry{
						{
							Name:   "service1",
							Remote: "remote1",
							Local:  "local1",
						},
						{
							Name:   "service2",
							Remote: "remote2",
							Local:  "local2",
						},
					},
				},
				DNS: &config.DNSConfig{
					Create: ptrBool(true),
				},
			}
		}

		// Given: a DNSHelper with the mock config handler, context, and real DockerHelper
		helper, err := NewDNSHelper(mockInjector)
		if err != nil {
			t.Fatalf("NewDNSHelper() error = %v", err)
		}

		// When: WriteConfig is called
		err = helper.WriteConfig()

		// Then: no error should be returned
		if err != nil {
			t.Fatalf("WriteConfig() error = %v", err)
		}

		// Check that the Corefile is written correctly
		corefilePath := filepath.Join(tempDir, "Corefile")
		data, err := os.ReadFile(corefilePath)
		if err != nil {
			t.Fatalf("Failed to read Corefile: %v", err)
		}
		content := string(data)

		expectedHostEntries := "        192.168.1.2 service1\n        192.168.1.3 service2\n"
		if !strings.Contains(content, expectedHostEntries) {
			t.Errorf("Corefile does not contain expected host entries.\nExpected:\n%s\nGot:\n%s", expectedHostEntries, content)
		}

		// Additional assertions can be made to check the content of the Corefile
		expectedCorefileContent := fmt.Sprintf(`
%s:53 {
    hosts {
%s        fallthrough
    }

    forward . 1.1.1.1 8.8.8.8
}
`, "test", expectedHostEntries)

		if content != expectedCorefileContent {
			t.Errorf("Corefile content does not match expected content.\nExpected:\n%s\nGot:\n%s", expectedCorefileContent, content)
		}
	})

	t.Run("ErrorCastingToDockerHelper", func(t *testing.T) {
		// Create a mock injector that resolves to an incorrect type for dockerHelper
		mockInjector := di.NewMockInjector()
		mockInjector.Register("dockerHelper", "notADockerHelper") // Incorrect type

		// Create a mock context
		mockContext := context.NewMockContext()
		mockContext.GetConfigRootFunc = func() (string, error) {
			return filepath.FromSlash("/mock/config/root"), nil
		}
		mockInjector.Register("contextHandler", mockContext)
		mockInjector.Register("shell", mockShell)

		// Create a mock configHandler
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigFunc = func() *config.Context {
			enabled := true
			return &config.Context{
				Docker: &config.DockerConfig{
					Enabled: &enabled,
				},
				DNS: &config.DNSConfig{
					Create: &enabled,
				},
			}
		}
		mockInjector.Register("configHandler", mockConfigHandler)

		// Given: a DNSHelper with the mock injector
		helper := &DNSHelper{
			injector: mockInjector,
		}

		// When: WriteConfig is called
		err := helper.WriteConfig()

		// Then: an error should be returned
		if err == nil || !strings.Contains(err.Error(), "error casting to DockerHelper") {
			t.Fatalf("expected error casting to DockerHelper, got %v", err)
		}
	})

	t.Run("ErrorRetrievingConfigRoot", func(t *testing.T) {
		// Create a mock context that returns an error on GetConfigRoot
		mockContext.GetConfigRootFunc = func() (string, error) {
			return "", fmt.Errorf("mock error retrieving config root")
		}

		mockInjector.Register("configHandler", mockConfigHandler)
		mockInjector.Register("contextHandler", mockContext)
		mockInjector.Register("dockerHelper", mockDockerHelper)
		mockInjector.Register("shell", mockShell)

		// Given: a DNSHelper with the mock context
		helper, err := NewDNSHelper(mockInjector)
		if err != nil {
			t.Fatalf("NewDNSHelper() error = %v", err)
		}

		// When: WriteConfig is called
		err = helper.WriteConfig()

		// Then: an error should be returned
		if err == nil || !strings.Contains(err.Error(), "error retrieving config root") {
			t.Fatalf("expected error retrieving config root, got %v", err)
		}
	})

	t.Run("ErrorWritingCorefile", func(t *testing.T) {
		// Create a temporary directory for config root
		tempDir := t.TempDir()

		// Create a mock context that returns the temp directory as config root
		mockContext.GetConfigRootFunc = func() (string, error) {
			return tempDir, nil
		}

		// Create a mock config handler that returns Docker enabled
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
		mockInjector.Register("contextHandler", mockContext)
		mockInjector.Register("shell", mockShell)

		// Create a real DockerHelper
		dockerHelper, err := NewDockerHelper(mockInjector)
		if err != nil {
			t.Fatalf("NewDockerHelper() error = %v", err)
		}
		mockInjector.Register("dockerHelper", dockerHelper)

		// Given: a DNSHelper with the mock config handler, context, and DockerHelper
		helper, err := NewDNSHelper(mockInjector)
		if err != nil {
			t.Fatalf("NewDNSHelper() error = %v", err)
		}

		// Override the writeFile function to return an error
		originalWriteFile := writeFile
		writeFile = func(filename string, data []byte, perm os.FileMode) error {
			return fmt.Errorf("mock error writing file")
		}
		defer func() { writeFile = originalWriteFile }()

		// When: WriteConfig is called
		err = helper.WriteConfig()

		// Then: an error should be returned
		if err == nil || !strings.Contains(err.Error(), "error writing Corefile") {
			t.Fatalf("expected error writing Corefile, got %v", err)
		}
	})

	t.Run("ErrorResolvingContext", func(t *testing.T) {
		// Create a mock injector that fails to resolve context
		mockInjector := di.NewMockInjector()
		mockInjector.SetResolveError("contextHandler", fmt.Errorf("mock error resolving context"))

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
		mockInjector.Register("shell", mockShell)

		// Given: a DNSHelper with the mock injector
		helper, err := NewDNSHelper(mockInjector)
		if err != nil {
			t.Fatalf("NewDNSHelper() error = %v", err)
		}

		// When: WriteConfig is called
		err = helper.WriteConfig()

		// Then: an error should be returned
		if err == nil || !strings.Contains(err.Error(), "error resolving context") {
			t.Fatalf("expected error resolving context, got %v", err)
		}
	})

	t.Run("ErrorResolvingCliConfigHandler", func(t *testing.T) {
		// Create a mock injector that fails to resolve configHandler
		mockInjector := di.NewMockInjector()
		mockInjector.SetResolveError("configHandler", fmt.Errorf("mock error resolving configHandler"))

		// Create a mock context
		mockContext := context.NewMockContext()
		mockContext.GetConfigRootFunc = func() (string, error) {
			return filepath.FromSlash("/mock/config/root"), nil
		}
		mockInjector.Register("contextHandler", mockContext)

		// Create a mock DockerHelper
		mockDockerHelper := NewMockHelper()
		mockInjector.Register("dockerHelper", mockDockerHelper)
		mockInjector.Register("shell", mockShell)

		// Given: a DNSHelper with the mock injector
		helper, err := NewDNSHelper(mockInjector)
		if err != nil {
			t.Fatalf("NewDNSHelper() error = %v", err)
		}

		// When: WriteConfig is called
		err = helper.WriteConfig()

		// Then: an error should be returned
		if err == nil || !strings.Contains(err.Error(), "error resolving configHandler") {
			t.Fatalf("expected error resolving configHandler, got %v", err)
		}
	})

	t.Run("ErrorResolvingDockerHelper", func(t *testing.T) {
		// Create a mock injector that fails to resolve dockerHelper
		mockInjector := di.NewMockInjector()
		mockInjector.SetResolveError("dockerHelper", fmt.Errorf("mock error resolving dockerHelper"))

		// Create a mock context
		mockContext := context.NewMockContext()
		mockContext.GetConfigRootFunc = func() (string, error) {
			return filepath.FromSlash("/mock/config/root"), nil
		}
		mockInjector.Register("contextHandler", mockContext)

		// Create a mock configHandler
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigFunc = func() *config.Context {
			enabled := true
			return &config.Context{
				Docker: &config.DockerConfig{
					Enabled: &enabled,
				},
				DNS: &config.DNSConfig{
					Create: &enabled,
				},
			}
		}
		mockInjector.Register("configHandler", mockConfigHandler)
		mockInjector.Register("shell", mockShell)

		// Given: a DNSHelper with the mock injector
		helper, err := NewDNSHelper(mockInjector)
		if err != nil {
			t.Fatalf("NewDNSHelper() error = %v", err)
		}

		// When: WriteConfig is called
		err = helper.WriteConfig()

		// Then: an error should be returned
		if err == nil || !strings.Contains(err.Error(), "error resolving dockerHelper") {
			t.Fatalf("expected error resolving dockerHelper, got %v", err)
		}
	})

	t.Run("ErrorCastingToDockerHelper", func(t *testing.T) {
		// Create a mock injector that resolves to an incorrect type for dockerHelper
		mockInjector := di.NewMockInjector()
		mockInjector.Register("dockerHelper", "notADockerHelper") // Incorrect type

		// Create a mock context
		mockContext := context.NewMockContext()
		mockContext.GetConfigRootFunc = func() (string, error) {
			return filepath.FromSlash("/mock/config/root"), nil
		}
		mockInjector.Register("contextHandler", mockContext)
		mockInjector.Register("shell", mockShell)

		// Create a mock configHandler
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigFunc = func() *config.Context {
			enabled := true
			return &config.Context{
				Docker: &config.DockerConfig{
					Enabled: &enabled,
				},
				DNS: &config.DNSConfig{
					Create: &enabled,
				},
			}
		}
		mockInjector.Register("configHandler", mockConfigHandler)

		// Given: a DNSHelper with the mock injector
		helper := &DNSHelper{
			injector: mockInjector,
		}

		// When: WriteConfig is called
		err := helper.WriteConfig()

		// Then: an error should be returned
		if err == nil || !strings.Contains(err.Error(), "error casting to DockerHelper") {
			t.Fatalf("expected error casting to DockerHelper, got %v", err)
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
		mockInjector := di.NewMockInjector()

		// Mock the configHandler
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigFunc = func() *config.Context {
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
		mockInjector.Register("configHandler", mockConfigHandler)

		// Mock the context
		mockContext := context.NewMockContext()
		mockContext.GetConfigRootFunc = func() (string, error) {
			return filepath.FromSlash("/invalid/path"), nil
		}
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}
		mockInjector.Register("contextHandler", mockContext)
		mockInjector.Register("shell", mockShell)

		// Create the DockerHelper instance
		dockerHelper, err := NewDockerHelper(mockInjector)
		if err != nil {
			t.Fatalf("NewDockerHelper() error = %v", err)
		}
		mockInjector.Register("dockerHelper", dockerHelper)

		// Create the DNSHelper instance
		dnsHelper, err := NewDNSHelper(mockInjector)
		if err != nil {
			t.Fatalf("NewDNSHelper() error = %v", err)
		}

		// Call WriteConfig and expect an error
		err = dnsHelper.WriteConfig()

		// Check if the error matches the expected error
		expectedError := "error creating parent folders: mock error creating directories"
		if err == nil || err.Error() != expectedError {
			t.Fatalf("expected error %v, got %v", expectedError, err)
		}
	})

	t.Run("DNSEnabledDockerDisabled", func(t *testing.T) {
		// Create a mock config handler that returns DNS enabled and Docker disabled
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigFunc = func() *config.Context {
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
		mockContext := context.NewMockContext()
		mockContext.GetConfigRootFunc = func() (string, error) {
			return filepath.FromSlash("/mock/config/root"), nil
		}
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}

		// Create a mock shell
		mockShell := shell.NewMockShell()

		mockInjector := di.NewMockInjector()
		mockInjector.Register("configHandler", mockConfigHandler)
		mockInjector.Register("contextHandler", mockContext)
		mockInjector.Register("shell", mockShell)

		// Register a real DockerHelper instance
		dockerHelper, err := NewDockerHelper(mockInjector)
		if err != nil {
			t.Fatalf("NewDockerHelper() error = %v", err)
		}
		mockInjector.Register("dockerHelper", dockerHelper)

		helper, err := NewDNSHelper(mockInjector)
		if err != nil {
			t.Fatalf("NewDNSHelper() error = %v", err)
		}

		// When: WriteConfig is called
		err = helper.WriteConfig()

		// Then: no error should be returned
		if err != nil {
			t.Fatalf("WriteConfig() error = %v", err)
		}
	})

	t.Run("DNSEnabledDockerEnabledWithName", func(t *testing.T) {
		// Create a mock config handler that returns DNS enabled, Docker enabled, and a DNS name defined
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigFunc = func() *config.Context {
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

		// Create a temporary directory for config root
		tempDir := t.TempDir()

		// Create a mock context that returns the temp directory as config root
		mockContext := context.NewMockContext()
		mockContext.GetConfigRootFunc = func() (string, error) {
			return tempDir, nil
		}
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}

		// Create a mock shell
		mockShell := shell.NewMockShell()

		mockInjector := di.NewMockInjector()
		mockInjector.Register("configHandler", mockConfigHandler)
		mockInjector.Register("contextHandler", mockContext)
		mockInjector.Register("shell", mockShell)

		// Register a real DockerHelper instance
		dockerHelper, err := NewDockerHelper(mockInjector)
		if err != nil {
			t.Fatalf("NewDockerHelper() error = %v", err)
		}
		mockInjector.Register("dockerHelper", dockerHelper)

		helper, err := NewDNSHelper(mockInjector)
		if err != nil {
			t.Fatalf("NewDNSHelper() error = %v", err)
		}

		// When: WriteConfig is called
		err = helper.WriteConfig()

		// Then: no error should be returned
		if err != nil {
			t.Fatalf("WriteConfig() error = %v", err)
		}
	})
}
