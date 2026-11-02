package env

import (
	"os"
	"testing"

	"github.com/windsorcli/cli/pkg/runtime/config"
)

// =============================================================================
// Test Setup
// =============================================================================

// setupVirtEnvMocks creates a new set of mocks for Virtual machine and container runtime environment tests
func setupVirtEnvMocks(t *testing.T, overrides ...*EnvTestMocks) *EnvTestMocks {
	t.Helper()
	mocks := setupEnvMocks(t, overrides...)

	// Only load default config if ConfigHandler wasn't overridden
	// If ConfigHandler was injected via overrides, assume test wants to control it
	if len(overrides) == 0 || overrides[0] == nil || overrides[0].ConfigHandler == nil {
		// Set the context environment variable first, before loading config
		os.Setenv("WINDSOR_CONTEXT", "test-context")

		configStr := `
version: v1alpha1
contexts:
  test-context:
    docker:
      registries:
        mock-registry-url:
          hostport: 5000
`
		if err := mocks.ConfigHandler.LoadConfigString(configStr); err != nil {
			t.Fatalf("Failed to load config: %v", err)
		}
		if mocks.ConfigHandler.GetString("workstation.runtime") == "" {
			_ = mocks.ConfigHandler.Set("workstation.runtime", "colima")
		}

		// Set the context (this also updates the environment variable)
		mocks.ConfigHandler.SetContext("test-context")
	}

	// Set up shims for Virtual machine operations
	mocks.Shims.UserHomeDir = func() (string, error) {
		return "/mock/home", nil
	}

	return mocks
}

// =============================================================================
// Test Public Methods
// =============================================================================

// TestVirtEnvPrinter_getRegistryURL tests the getRegistryURL method of the VirtEnvPrinter
func TestVirtEnvPrinter_getRegistryURL(t *testing.T) {
	// setup creates a new VirtEnvPrinter with the given configuration
	setup := func(t *testing.T, overrides ...*EnvTestMocks) (*VirtEnvPrinter, *EnvTestMocks) {
		t.Helper()
		mocks := setupVirtEnvMocks(t, overrides...)
		printer := NewVirtEnvPrinter(mocks.Shell, mocks.ConfigHandler)
		printer.shims = mocks.Shims
		return printer, mocks
	}

	t.Run("ValidRegistryURL", func(t *testing.T) {
		// Given a VirtEnvPrinter with a valid registry URL in config
		printer, mocks := setup(t)
		configStr := `
version: v1alpha1
contexts:
  test-context:
    vm:
      driver: colima
    docker:
      registry_url: registry.example.com:5000
`
		if err := mocks.ConfigHandler.LoadConfigString(configStr); err != nil {
			t.Fatalf("Failed to load config: %v", err)
		}

		// And the registry URL is set in the context
		printer.configHandler.Set("docker.registry_url", "registry.example.com:5000")

		// When getting the registry URL
		url, err := printer.getRegistryURL()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		// And the URL should match the config value
		if url != "registry.example.com:5000" {
			t.Errorf("Expected URL 'registry.example.com:5000', got %q", url)
		}
	})

	t.Run("RegistryURLWithConfig", func(t *testing.T) {
		// Given a VirtEnvPrinter with a registry URL and matching config
		printer, mocks := setup(t)
		configStr := `
version: v1alpha1
contexts:
  test-context:
    vm:
      driver: colima
    docker:
      registry_url: registry.example.com
      registries:
        registry.example.com:
          hostport: 5000
`
		if err := mocks.ConfigHandler.LoadConfigString(configStr); err != nil {
			t.Fatalf("Failed to load config: %v", err)
		}

		// And the registry URL is set in the context
		printer.configHandler.Set("docker.registry_url", "registry.example.com")

		// When getting the registry URL
		url, err := printer.getRegistryURL()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		// And the URL should include the hostport from config
		if url != "registry.example.com:5000" {
			t.Errorf("Expected URL 'registry.example.com:5000', got %q", url)
		}
	})

	t.Run("EmptyRegistryURL", func(t *testing.T) {
		// Given a VirtEnvPrinter with no registry URL but with registries config
		printer, mocks := setup(t)
		configStr := `
version: v1alpha1
contexts:
  test-context:
    vm:
      driver: colima
    docker:
      registries:
        mock-registry-url:
          hostport: 5000
`
		if err := mocks.ConfigHandler.LoadConfigString(configStr); err != nil {
			t.Fatalf("Failed to load config: %v", err)
		}

		// When getting the registry URL
		url, err := printer.getRegistryURL()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		// And the URL should be taken from the first registry in config
		if url != "mock-registry-url:5000" {
			t.Errorf("Expected URL 'mock-registry-url:5000', got %q", url)
		}
	})

	t.Run("EmptyConfig", func(t *testing.T) {
		// Given a VirtEnvPrinter with empty registries config
		baseMocks := setupEnvMocks(t)
		mocks := setupVirtEnvMocks(t, &EnvTestMocks{
			ConfigHandler: config.NewConfigHandler(baseMocks.Shell),
		})
		configStr := `
version: v1alpha1
contexts:
  test-context:
    vm:
      driver: colima
    docker:
      registries: {}
`
		if err := mocks.ConfigHandler.LoadConfigString(configStr); err != nil {
			t.Fatalf("Failed to load config: %v", err)
		}
		printer := NewVirtEnvPrinter(mocks.Shell, mocks.ConfigHandler)
		printer.shims = mocks.Shims

		// When getting the registry URL
		url, err := printer.getRegistryURL()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		// And the URL should be empty
		if url != "" {
			t.Errorf("Expected empty URL, got %q", url)
		}
	})

	t.Run("RegistryURLWithoutPortNoConfig", func(t *testing.T) {
		// Given a VirtEnvPrinter with a registry URL without port and no matching config
		printer, mocks := setup(t)
		configStr := `
version: v1alpha1
contexts:
  test-context:
    vm:
      driver: colima
    docker:
      registry_url: registry.example.com
      registries:
        other-registry:
          hostport: 5000
`
		if err := mocks.ConfigHandler.LoadConfigString(configStr); err != nil {
			t.Fatalf("Failed to load config: %v", err)
		}

		// And the registry URL is set in the context
		printer.configHandler.Set("docker.registry_url", "registry.example.com")

		// When getting the registry URL
		url, err := printer.getRegistryURL()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		// And the URL should be returned as-is without a port
		if url != "registry.example.com" {
			t.Errorf("Expected URL 'registry.example.com', got %q", url)
		}
	})

	t.Run("RegistryURLInvalidPort", func(t *testing.T) {
		// Given a VirtEnvPrinter with a registry URL with invalid port
		printer, mocks := setup(t)
		configStr := `
version: v1alpha1
contexts:
  test-context:
    vm:
      driver: colima
    docker:
      registry_url: registry.example.com:invalid
`
		if err := mocks.ConfigHandler.LoadConfigString(configStr); err != nil {
			t.Fatalf("Failed to load config: %v", err)
		}

		// When getting the registry URL
		url, err := printer.getRegistryURL()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		// And the URL should be the same as the config value
		if url != "registry.example.com:invalid" {
			t.Errorf("Expected URL 'registry.example.com:invalid', got %q", url)
		}
	})

	t.Run("RegistryURLNoPortNoHostPort", func(t *testing.T) {
		// Given a VirtEnvPrinter with a registry URL without port and no hostport in config
		printer, mocks := setup(t)
		configStr := `
version: v1alpha1
contexts:
  test-context:
    vm:
      driver: colima
    docker:
      registry_url: registry.example.com
      registries:
        registry.example.com: {}
`
		if err := mocks.ConfigHandler.LoadConfigString(configStr); err != nil {
			t.Fatalf("Failed to load config: %v", err)
		}

		// When getting the registry URL
		url, err := printer.getRegistryURL()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		// And the URL should be the same as the config value
		if url != "registry.example.com" {
			t.Errorf("Expected URL 'registry.example.com', got %q", url)
		}
	})

	t.Run("RegistryURLEmptyRegistries", func(t *testing.T) {
		// Given a VirtEnvPrinter with a registry URL and empty registries config
		printer, mocks := setup(t)
		configStr := `
version: v1alpha1
contexts:
  test-context:
    vm:
      driver: colima
    docker:
      registry_url: registry.example.com
      registries: {}
`
		if err := mocks.ConfigHandler.LoadConfigString(configStr); err != nil {
			t.Fatalf("Failed to load config: %v", err)
		}

		// When getting the registry URL
		url, err := printer.getRegistryURL()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		// And the URL should be the same as the config value
		if url != "registry.example.com" {
			t.Errorf("Expected URL 'registry.example.com', got %q", url)
		}
	})

	t.Run("NilDockerConfig", func(t *testing.T) {
		// Given a VirtEnvPrinter with no Docker config
		baseMocks := setupEnvMocks(t)
		mocks := setupVirtEnvMocks(t, &EnvTestMocks{
			ConfigHandler: config.NewConfigHandler(baseMocks.Shell),
		})
		configStr := `
version: v1alpha1
contexts:
  test-context:
    vm:
      driver: colima
`
		if err := mocks.ConfigHandler.LoadConfigString(configStr); err != nil {
			t.Fatalf("Failed to load config: %v", err)
		}
		printer := NewVirtEnvPrinter(mocks.Shell, mocks.ConfigHandler)
		printer.shims = mocks.Shims

		// When getting the registry URL
		url, err := printer.getRegistryURL()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		// And the URL should be empty
		if url != "" {
			t.Errorf("Expected empty URL, got %q", url)
		}
	})

	t.Run("NilRegistriesWithURL", func(t *testing.T) {
		// Given a VirtEnvPrinter with a registry URL but no registries config
		printer, mocks := setup(t)
		configStr := `
version: v1alpha1
contexts:
  test-context:
    vm:
      driver: colima
    docker:
      registry_url: registry.example.com
`
		if err := mocks.ConfigHandler.LoadConfigString(configStr); err != nil {
			t.Fatalf("Failed to load config: %v", err)
		}

		// When getting the registry URL
		url, err := printer.getRegistryURL()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		// And the URL should be the same as the config value
		if url != "registry.example.com" {
			t.Errorf("Expected URL 'registry.example.com', got %q", url)
		}
	})

	t.Run("RegistryWithoutHostPort", func(t *testing.T) {
		// Given a VirtEnvPrinter with a registry without hostport in config
		baseMocks := setupEnvMocks(t)
		mocks := setupVirtEnvMocks(t, &EnvTestMocks{
			ConfigHandler: config.NewConfigHandler(baseMocks.Shell),
		})
		// Set the context environment variable after setup (setupVirtEnvMocks may have reset it)
		os.Setenv("WINDSOR_CONTEXT", "test-context")

		configStr := `
version: v1alpha1
contexts:
  test-context:
    vm:
      driver: colima
    docker:
      registries:
        registry.example.com:
          remote: ""
`
		if err := mocks.ConfigHandler.LoadConfigString(configStr); err != nil {
			t.Fatalf("Failed to load config: %v", err)
		}
		// Set the context to ensure GetConfig() returns the right config
		mocks.ConfigHandler.SetContext("test-context")

		printer := NewVirtEnvPrinter(mocks.Shell, mocks.ConfigHandler)
		printer.shims = mocks.Shims

		// When getting the registry URL
		url, err := printer.getRegistryURL()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		// And the URL should be the registry name with default port 5000
		if url != "registry.example.com:5000" {
			t.Errorf("Expected URL 'registry.example.com:5000', got %q", url)
		}
	})
}
