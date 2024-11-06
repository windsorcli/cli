package helpers

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/constants"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/shell"
)

func TestNewDNSHelper(t *testing.T) {
	// Create a mock DI container
	mockDIContainer := di.NewMockContainer()

	// Call NewDNSHelper with the mock DI container
	helper, err := NewDNSHelper(mockDIContainer.DIContainer)

	// Verify that no error is returned
	if err != nil {
		t.Fatalf("NewDNSHelper() error = %v, wantErr %v", err, false)
	}

	// Verify that the helper is not nil
	if helper == nil {
		t.Fatalf("NewDNSHelper() returned nil, expected non-nil DNSHelper")
	}

	// Verify that the DIContainer is correctly set
	if helper.DIContainer != mockDIContainer.DIContainer {
		t.Errorf("NewDNSHelper() DIContainer = %v, want %v", helper.DIContainer, mockDIContainer)
	}
}

func TestDNSHelper_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given: a mock config handler, shell, and helper
		mockConfigHandler := config.NewMockConfigHandler()
		mockShell := shell.NewMockShell()
		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("shell", mockShell)
		helper, err := NewDNSHelper(diContainer)
		if err != nil {
			t.Fatalf("NewDNSHelper() error = %v", err)
		}

		// When: Initialize is called
		err = helper.Initialize()

		// Then: no error should be returned
		if err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}
	})
}

func TestDNSHelper_GetComposeConfig(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Create a mock DI container
		mockDIContainer := di.NewMockContainer()

		// Create a mock context
		mockContext := context.NewMockContext()
		mockContext.GetConfigRootFunc = func() (string, error) {
			return "/mock/config/root", nil
		}
		mockContext.GetContextFunc = func() (string, error) {
			return "mock-context", nil
		}
		mockDIContainer.Register("contextHandler", mockContext)

		// Create a mock cliConfigHandler
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			enabled := true
			return &config.Context{
				Docker: &config.DockerConfig{
					Enabled: &enabled,
				},
				DNS: &config.DNSConfig{
					Create: &enabled,
					Name:   ptrString("test1"),
				},
			}, nil
		}
		// Create a mock shell
		mockShell := shell.NewMockShell()
		mockDIContainer.Register("shell", mockShell)
		mockDIContainer.Register("cliConfigHandler", mockConfigHandler)

		// Create a mock dockerHelper using MakeDockerHelper
		mockDockerHelper, err := NewDockerHelper(mockDIContainer.DIContainer)
		if err != nil {
			t.Fatalf("NewDockerHelper() error = %v", err)
		}
		mockDIContainer.Register("dockerHelper", mockDockerHelper)

		// Given: a DNSHelper with the mock DI container
		helper, err := NewDNSHelper(mockDIContainer.DIContainer)
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
		// Create a mock DI container that does not register contextHandler
		mockDIContainer := di.NewMockContainer()

		// Create a mock cliConfigHandler
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Docker: &config.DockerConfig{
					Enabled: ptrBool(true),
				},
				DNS: &config.DNSConfig{
					Create: ptrBool(true),
				},
			}, nil
		}
		mockDIContainer.Register("cliConfigHandler", mockConfigHandler)

		// Given: a DNSHelper with the mock DI container
		helper, err := NewDNSHelper(mockDIContainer.DIContainer)
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

		// Create a mock DI container
		mockDIContainer := di.NewContainer()
		mockDIContainer.Register("contextHandler", mockContext)

		// Given: a DNSHelper with the mock DI container
		helper := &DNSHelper{
			DIContainer: mockDIContainer,
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

		// Create a mock DI container that does not have cliConfigHandler registered
		mockDIContainer := di.NewMockContainer()
		mockDIContainer.Register("contextHandler", mockContext)

		// Given: a DNSHelper with the mock DI container
		helper := &DNSHelper{
			DIContainer: mockDIContainer.DIContainer,
		}

		// When: GetComposeConfig is called
		_, err := helper.GetComposeConfig()

		// Then: an error should be returned
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
		if !strings.Contains(err.Error(), "error resolving cliConfigHandler") {
			t.Errorf("Expected error message to contain 'error resolving cliConfigHandler', got %v", err)
		}
	})

	t.Run("ErrorRetrievingContextConfig", func(t *testing.T) {
		// Create a mock context instance
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "mock-context", nil
		}

		// Create a mock config handler that returns an error when GetConfig is called
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return nil, fmt.Errorf("error retrieving context configuration")
		}

		// Create a mock DI container
		mockDIContainer := di.NewContainer()
		mockDIContainer.Register("contextHandler", mockContext)
		mockDIContainer.Register("cliConfigHandler", mockConfigHandler)

		// Given: a DNSHelper with the mock DI container
		helper := &DNSHelper{
			DIContainer: mockDIContainer,
		}

		// When: GetComposeConfig is called
		_, err := helper.GetComposeConfig()

		// Then: an error should be returned
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
		if err.Error() != "error retrieving context configuration: error retrieving context configuration" {
			t.Errorf("Expected error message 'error retrieving context configuration: error retrieving context configuration', got %v", err)
		}
	})

	t.Run("DNSDisabled", func(t *testing.T) {
		// Create a mock config handler that returns DNS disabled
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				DNS: &config.DNSConfig{
					Create: ptrBool(false),
				},
			}, nil
		}

		// Create a mock shell
		mockShell := shell.NewMockShell()

		// Create a mock context instance
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "mock-context", nil
		}

		// Given: a DNSHelper with the mock config handler and context instance
		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("shell", mockShell)
		diContainer.Register("contextHandler", mockContext)
		helper, err := NewDNSHelper(diContainer)
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

	t.Run("DNSEnabled", func(t *testing.T) {
		// Create a mock config handler that returns DNS enabled
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				DNS: &config.DNSConfig{
					Create: ptrBool(true),
					Name:   ptrString("test"),
				},
			}, nil
		}

		// Create a mock context instance
		mockContext := context.NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "mock-context", nil
		}

		// Given: a DNSHelper with the mock config handler and context instance
		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("contextHandler", mockContext)
		helper, err := NewDNSHelper(diContainer)
		if err != nil {
			t.Fatalf("NewDNSHelper() error = %v", err)
		}

		// When: GetComposeConfig is called
		cfg, err := helper.GetComposeConfig()

		// Then: no error should be returned, and cfg should not be nil
		if err != nil {
			t.Fatalf("GetComposeConfig() error = %v", err)
		}
		if cfg == nil {
			t.Fatalf("Expected cfg to be non-nil when DNS is enabled")
		}

		// Check that cfg contains the expected service configuration
		if len(cfg.Services) != 1 {
			t.Errorf("Expected 1 service, got %d", len(cfg.Services))
		}
		service := cfg.Services[0]
		if service.Name != "dns.test" {
			t.Errorf("Expected service name 'dns.test', got '%s'", service.Name)
		}
		if service.Image != constants.DEFAULT_DNS_IMAGE {
			t.Errorf("Expected image '%s', got '%s'", constants.DEFAULT_DNS_IMAGE, service.Image)
		}

		// Additional assertions to verify Volumes, Labels, etc.
		if len(service.Volumes) != 1 {
			t.Errorf("Expected 1 volume, got %d", len(service.Volumes))
		}

		if service.Volumes[0].Source != "./Corefile" || service.Volumes[0].Target != "/etc/coredns/Corefile" {
			t.Errorf("Unexpected volume configuration")
		}

		if val, ok := service.Labels["managed_by"]; !ok || val != "windsor" {
			t.Errorf("Expected label 'managed_by' to be 'windsor'")
		}
		if val, ok := service.Labels["context"]; !ok || val != "mock-context" {
			t.Errorf("Expected label 'context' to be 'mock-context'")
		}
		if val, ok := service.Labels["role"]; !ok || val != "dns" {
			t.Errorf("Expected label 'role' to be 'dns'")
		}
	})
}

func TestDNSHelper_WriteConfig(t *testing.T) {
	// Shared resources
	mockContext := context.NewMockContext()
	mockConfigHandler := config.NewMockConfigHandler()
	mockDockerHelper := NewMockHelper()
	mockShell := shell.NewMockShell()

	t.Run("DockerDisabled", func(t *testing.T) {
		// Create a mock config handler that returns Docker disabled
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Docker: &config.DockerConfig{
					Enabled: ptrBool(false),
				},
			}, nil
		}

		// Create a mock context
		mockContext.GetConfigRootFunc = func() (string, error) {
			return "/mock/config/root", nil
		}

		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("contextHandler", mockContext)
		diContainer.Register("shell", mockShell)

		helper, err := NewDNSHelper(diContainer)
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
		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("contextHandler", mockContext)
		diContainer.Register("shell", mockShell)

		dockerHelper, err := NewDockerHelper(diContainer)
		if err != nil {
			t.Fatalf("NewDockerHelper() error = %v", err)
		}
		diContainer.Register("dockerHelper", dockerHelper)

		// Create a mock config handler that returns Docker enabled
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
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
			}, nil
		}

		// Given: a DNSHelper with the mock config handler, context, and real DockerHelper
		helper, err := NewDNSHelper(diContainer)
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
		// Create a mock DI container that resolves to an incorrect type for dockerHelper
		mockDIContainer := di.NewMockContainer()
		mockDIContainer.Register("dockerHelper", "notADockerHelper") // Incorrect type

		// Create a mock context
		mockContext := context.NewMockContext()
		mockContext.GetConfigRootFunc = func() (string, error) {
			return "/mock/config/root", nil
		}
		mockDIContainer.Register("contextHandler", mockContext)
		mockDIContainer.Register("shell", mockShell)

		// Create a mock cliConfigHandler
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			enabled := true
			return &config.Context{
				Docker: &config.DockerConfig{
					Enabled: &enabled,
				},
				DNS: &config.DNSConfig{
					Create: &enabled,
				},
			}, nil
		}
		mockDIContainer.Register("cliConfigHandler", mockConfigHandler)

		// Given: a DNSHelper with the mock DI container
		helper := &DNSHelper{
			DIContainer: mockDIContainer.DIContainer,
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

		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("contextHandler", mockContext)
		diContainer.Register("dockerHelper", mockDockerHelper)
		diContainer.Register("shell", mockShell)

		// Given: a DNSHelper with the mock context
		helper, err := NewDNSHelper(diContainer)
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

	t.Run("ErrorRetrievingContextConfig", func(t *testing.T) {
		// Create a mock context that returns a valid config root
		mockContext.GetConfigRootFunc = func() (string, error) {
			return "/mock/config/root", nil
		}

		// Create a mock config handler that returns an error on GetConfig
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return nil, fmt.Errorf("mock error retrieving context configuration")
		}

		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("contextHandler", mockContext)
		diContainer.Register("dockerHelper", mockDockerHelper)
		diContainer.Register("shell", mockShell)

		// Given: a DNSHelper with the mock config handler and context
		helper, err := NewDNSHelper(diContainer)
		if err != nil {
			t.Fatalf("NewDNSHelper() error = %v", err)
		}

		// When: WriteConfig is called
		err = helper.WriteConfig()

		// Then: an error should be returned
		if err == nil || !strings.Contains(err.Error(), "error retrieving context configuration") {
			t.Fatalf("expected error retrieving context configuration, got %v", err)
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
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Docker: &config.DockerConfig{
					Enabled: ptrBool(true),
				},
				DNS: &config.DNSConfig{
					Create: ptrBool(true),
				},
			}, nil
		}

		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("contextHandler", mockContext)
		diContainer.Register("shell", mockShell)

		// Create a real DockerHelper
		dockerHelper, err := NewDockerHelper(diContainer)
		if err != nil {
			t.Fatalf("NewDockerHelper() error = %v", err)
		}
		diContainer.Register("dockerHelper", dockerHelper)

		// Given: a DNSHelper with the mock config handler, context, and DockerHelper
		helper, err := NewDNSHelper(diContainer)
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
		// Create a mock DI container that fails to resolve context
		mockDIContainer := di.NewMockContainer()
		mockDIContainer.SetResolveError("contextHandler", fmt.Errorf("mock error resolving context"))

		// Create a mock cliConfigHandler
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Docker: &config.DockerConfig{
					Enabled: ptrBool(true),
				},
				DNS: &config.DNSConfig{
					Create: ptrBool(true),
				},
			}, nil
		}
		mockDIContainer.Register("cliConfigHandler", mockConfigHandler)
		mockDIContainer.Register("shell", mockShell)

		// Given: a DNSHelper with the mock DI container
		helper, err := NewDNSHelper(mockDIContainer.DIContainer)
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
		// Create a mock DI container that fails to resolve cliConfigHandler
		mockDIContainer := di.NewMockContainer()
		mockDIContainer.SetResolveError("cliConfigHandler", fmt.Errorf("mock error resolving cliConfigHandler"))

		// Create a mock context
		mockContext := context.NewMockContext()
		mockContext.GetConfigRootFunc = func() (string, error) {
			return "/mock/config/root", nil
		}
		mockDIContainer.Register("contextHandler", mockContext)

		// Create a mock DockerHelper
		mockDockerHelper := NewMockHelper()
		mockDIContainer.Register("dockerHelper", mockDockerHelper)
		mockDIContainer.Register("shell", mockShell)

		// Given: a DNSHelper with the mock DI container
		helper, err := NewDNSHelper(mockDIContainer.DIContainer)
		if err != nil {
			t.Fatalf("NewDNSHelper() error = %v", err)
		}

		// When: WriteConfig is called
		err = helper.WriteConfig()

		// Then: an error should be returned
		if err == nil || !strings.Contains(err.Error(), "error resolving cliConfigHandler") {
			t.Fatalf("expected error resolving cliConfigHandler, got %v", err)
		}
	})

	t.Run("ErrorResolvingDockerHelper", func(t *testing.T) {
		// Create a mock DI container that fails to resolve dockerHelper
		mockDIContainer := di.NewMockContainer()
		mockDIContainer.SetResolveError("dockerHelper", fmt.Errorf("mock error resolving dockerHelper"))

		// Create a mock context
		mockContext := context.NewMockContext()
		mockContext.GetConfigRootFunc = func() (string, error) {
			return "/mock/config/root", nil
		}
		mockDIContainer.Register("contextHandler", mockContext)

		// Create a mock cliConfigHandler
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			enabled := true
			return &config.Context{
				Docker: &config.DockerConfig{
					Enabled: &enabled,
				},
				DNS: &config.DNSConfig{
					Create: &enabled,
				},
			}, nil
		}
		mockDIContainer.Register("cliConfigHandler", mockConfigHandler)
		mockDIContainer.Register("shell", mockShell)

		// Given: a DNSHelper with the mock DI container
		helper, err := NewDNSHelper(mockDIContainer.DIContainer)
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
		// Create a mock DI container that resolves to an incorrect type for dockerHelper
		mockDIContainer := di.NewMockContainer()
		mockDIContainer.Register("dockerHelper", "notADockerHelper") // Incorrect type

		// Create a mock context
		mockContext := context.NewMockContext()
		mockContext.GetConfigRootFunc = func() (string, error) {
			return "/mock/config/root", nil
		}
		mockDIContainer.Register("contextHandler", mockContext)
		mockDIContainer.Register("shell", mockShell)

		// Create a mock cliConfigHandler
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			enabled := true
			return &config.Context{
				Docker: &config.DockerConfig{
					Enabled: &enabled,
				},
				DNS: &config.DNSConfig{
					Create: &enabled,
				},
			}, nil
		}
		mockDIContainer.Register("cliConfigHandler", mockConfigHandler)

		// Given: a DNSHelper with the mock DI container
		helper := &DNSHelper{
			DIContainer: mockDIContainer.DIContainer,
		}

		// When: WriteConfig is called
		err := helper.WriteConfig()

		// Then: an error should be returned
		if err == nil || !strings.Contains(err.Error(), "error casting to DockerHelper") {
			t.Fatalf("expected error casting to DockerHelper, got %v", err)
		}
	})

	t.Run("ErrorRetrievingComposeConfig", func(t *testing.T) {
		// Given: a DNSHelper with a DI container containing mocks
		diContainer := di.NewContainer()

		// Mock the cliConfigHandler
		mockConfigHandler := config.NewMockConfigHandler()
		callCount := 0
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			callCount++
			if callCount == 2 {
				return nil, fmt.Errorf("mock error retrieving context configuration")
			}
			enabled := true
			return &config.Context{
				Docker: &config.DockerConfig{
					Enabled: &enabled,
				},
				DNS: &config.DNSConfig{
					Create: &enabled,
				},
			}, nil
		}
		diContainer.Register("cliConfigHandler", mockConfigHandler)

		// Mock the context
		mockContext := context.NewMockContext()
		mockContext.GetConfigRootFunc = func() (string, error) {
			return "/mock/config/root", nil
		}
		diContainer.Register("contextHandler", mockContext)
		diContainer.Register("shell", mockShell)

		// Create the DockerHelper instance
		dockerHelper, err := NewDockerHelper(diContainer)
		if err != nil {
			t.Fatalf("NewDockerHelper() error = %v", err)
		}
		diContainer.Register("dockerHelper", dockerHelper)

		// Create the DNSHelper instance
		dnsHelper, err := NewDNSHelper(diContainer)
		if err != nil {
			t.Fatalf("NewDNSHelper() error = %v", err)
		}

		// When: WriteConfig is called
		err = dnsHelper.WriteConfig()

		// Then: it should return an error indicating the failure to retrieve the context configuration
		expectedError := "error retrieving context configuration: mock error retrieving context configuration"
		if err == nil || !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("expected error %v, got %v", expectedError, err)
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

		// Setup DI container with mocks
		diContainer := di.NewContainer()

		// Mock the cliConfigHandler
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			// Return a context config where Docker is enabled
			return &config.Context{
				Docker: &config.DockerConfig{
					Enabled: ptrBool(true),
				},
				DNS: &config.DNSConfig{
					Create: ptrBool(true),
				},
			}, nil
		}
		diContainer.Register("cliConfigHandler", mockConfigHandler)

		// Mock the context
		mockContext := context.NewMockContext()
		mockContext.GetConfigRootFunc = func() (string, error) {
			return "/invalid/path", nil
		}
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}
		diContainer.Register("contextHandler", mockContext)
		diContainer.Register("shell", mockShell)

		// Create the DockerHelper instance
		dockerHelper, err := NewDockerHelper(diContainer)
		if err != nil {
			t.Fatalf("NewDockerHelper() error = %v", err)
		}
		diContainer.Register("dockerHelper", dockerHelper)

		// Create the DNSHelper instance
		dnsHelper, err := NewDNSHelper(diContainer)
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
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Docker: &config.DockerConfig{
					Enabled: ptrBool(false),
				},
				DNS: &config.DNSConfig{
					Create: ptrBool(true),
				},
			}, nil
		}

		// Create a mock context that returns a valid config root
		mockContext := context.NewMockContext()
		mockContext.GetConfigRootFunc = func() (string, error) {
			return "/mock/config/root", nil
		}
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}

		// Create a mock shell
		mockShell := shell.NewMockShell()

		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("contextHandler", mockContext)
		diContainer.Register("shell", mockShell)

		// Register a real DockerHelper instance
		dockerHelper, err := NewDockerHelper(diContainer)
		if err != nil {
			t.Fatalf("NewDockerHelper() error = %v", err)
		}
		diContainer.Register("dockerHelper", dockerHelper)

		helper, err := NewDNSHelper(diContainer)
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
		mockConfigHandler.GetConfigFunc = func() (*config.Context, error) {
			return &config.Context{
				Docker: &config.DockerConfig{
					Enabled: ptrBool(true),
				},
				DNS: &config.DNSConfig{
					Create: ptrBool(true),
					Name:   ptrString("custom-dns"),
				},
			}, nil
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

		diContainer := di.NewContainer()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("contextHandler", mockContext)
		diContainer.Register("shell", mockShell)

		// Register a real DockerHelper instance
		dockerHelper, err := NewDockerHelper(diContainer)
		if err != nil {
			t.Fatalf("NewDockerHelper() error = %v", err)
		}
		diContainer.Register("dockerHelper", dockerHelper)

		helper, err := NewDNSHelper(diContainer)
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

func TestDNSHelper_Up(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Create DI container and register mocks
		diContainer := di.NewContainer()
		mockConfigHandler := config.NewMockConfigHandler()
		mockContext := context.NewMockContext()
		mockShell := shell.NewMockShell()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("contextHandler", mockContext)
		diContainer.Register("shell", mockShell)

		// Create an instance of DNSHelper
		dnsHelper, err := NewDNSHelper(diContainer)
		if err != nil {
			t.Fatalf("NewDNSHelper() error = %v", err)
		}

		// When: Up is called
		err = dnsHelper.Up()
		if err != nil {
			t.Fatalf("Up() error = %v", err)
		}
	})
}

func TestDNSHelper_Info(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Create DI container and register mocks
		diContainer := di.NewContainer()
		mockConfigHandler := config.NewMockConfigHandler()
		mockContext := context.NewMockContext()
		mockShell := shell.NewMockShell()
		diContainer.Register("cliConfigHandler", mockConfigHandler)
		diContainer.Register("contextHandler", mockContext)
		diContainer.Register("shell", mockShell)

		// Create an instance of DNSHelper
		dnsHelper, err := NewDNSHelper(diContainer)
		if err != nil {
			t.Fatalf("NewDNSHelper() error = %v", err)
		}

		// When: Info is called
		info, err := dnsHelper.Info()
		if err != nil {
			t.Fatalf("Info() error = %v", err)
		}

		// Then: no error should be returned and info should be nil
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if info != nil {
			t.Errorf("Expected info to be nil, got %v", info)
		}
	})
}
