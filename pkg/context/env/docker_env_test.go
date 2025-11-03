package env

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/context/config"
	"github.com/windsorcli/cli/pkg/context/shell"
	"github.com/windsorcli/cli/pkg/di"
)

// =============================================================================
// Test Setup
// =============================================================================

// DockerEnvPrinterMocks holds all mock objects used in Docker environment tests
type DockerEnvPrinterMocks struct {
	Injector      di.Injector
	Shell         *shell.MockShell
	ConfigHandler *config.MockConfigHandler
}

// setupDockerEnvMocks creates a new set of mocks for Docker environment tests
func setupDockerEnvMocks(t *testing.T, opts ...*SetupOptions) *Mocks {
	t.Helper()
	if len(opts) == 0 || opts[0].ConfigStr == "" {
		opts = []*SetupOptions{{
			ConfigStr: `
version: v1alpha1
contexts:
  test-context:
    vm:
      driver: colima
    docker:
      registries:
        mock-registry-url:
          hostport: 5000
`,
		}}
	}

	// Create mocks with the config
	mocks := setupMocks(t, opts...)

	// Set the context
	mocks.ConfigHandler.SetContext("test-context")

	// Set up shims for Docker operations
	mocks.Shims.UserHomeDir = func() (string, error) {
		return "/mock/home", nil
	}

	return mocks
}

// =============================================================================
// Test Public Methods
// =============================================================================

// TestDockerEnvPrinter_GetEnvVars tests the GetEnvVars method of the DockerEnvPrinter
func TestDockerEnvPrinter_GetEnvVars(t *testing.T) {
	// Save original env var and restore after all tests
	originalDockerHost := os.Getenv("DOCKER_HOST")
	defer os.Setenv("DOCKER_HOST", originalDockerHost)

	t.Run("Success", func(t *testing.T) {
		// Given a new DockerEnvPrinter with default configuration
		mocks := setupDockerEnvMocks(t, &SetupOptions{
			ConfigStr: `
version: v1alpha1
contexts:
  test-context:
    vm:
      driver: colima
    docker:
      registries:
        mock-registry-url:
          hostport: 5000
`,
		})

		printer := NewDockerEnvPrinter(mocks.Injector)
		printer.shims = mocks.Shims
		printer.Initialize()

		// When getting environment variables
		envVars, err := printer.GetEnvVars()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		// And DOCKER_HOST should be set based on vm driver and OS
		var expectedDockerHost string
		if mocks.Shims.Goos() == "windows" {
			expectedDockerHost = "npipe:////./pipe/docker_engine"
		} else {
			expectedDockerHost = fmt.Sprintf("unix://%s/.colima/windsor-test-context/docker.sock", filepath.ToSlash("/mock/home"))
		}
		if envVars["DOCKER_HOST"] != expectedDockerHost {
			t.Errorf("DOCKER_HOST = %v, want %v", envVars["DOCKER_HOST"], expectedDockerHost)
		}

		// And REGISTRY_URL should be set based on registry configuration
		expectedRegistryURL := "mock-registry-url:5000"
		if envVars["REGISTRY_URL"] != expectedRegistryURL {
			t.Errorf("REGISTRY_URL = %v, want %v", envVars["REGISTRY_URL"], expectedRegistryURL)
		}

		// And DOCKER_CONFIG should be set
		expectedDockerConfig := filepath.ToSlash("/mock/home/.config/windsor/docker")
		if envVars["DOCKER_CONFIG"] != expectedDockerConfig {
			t.Errorf("DOCKER_CONFIG = %v, want %v", envVars["DOCKER_CONFIG"], expectedDockerConfig)
		}
	})

	t.Run("ColimaDriver", func(t *testing.T) {
		// Given a new DockerEnvPrinter with Colima driver
		os.Unsetenv("DOCKER_HOST")
		mocks := setupDockerEnvMocks(t, &SetupOptions{
			ConfigStr: `
version: v1alpha1
contexts:
  test-context:
    vm:
      driver: colima
    docker:
      registries:
        mock-registry-url:
          hostport: 5000
`,
		})

		printer := NewDockerEnvPrinter(mocks.Injector)
		printer.shims = mocks.Shims
		printer.Initialize()

		// When getting environment variables
		envVars, err := printer.GetEnvVars()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		// And DOCKER_HOST should be set correctly for Colima and OS
		var expectedDockerHost string
		if mocks.Shims.Goos() == "windows" {
			expectedDockerHost = "npipe:////./pipe/docker_engine"
		} else {
			expectedDockerHost = fmt.Sprintf("unix://%s/.colima/windsor-test-context/docker.sock", filepath.ToSlash("/mock/home"))
		}
		if envVars["DOCKER_HOST"] != expectedDockerHost {
			t.Errorf("DOCKER_HOST = %v, want %v", envVars["DOCKER_HOST"], expectedDockerHost)
		}

		// And REGISTRY_URL should be set correctly
		expectedRegistryURL := "mock-registry-url:5000"
		if envVars["REGISTRY_URL"] != expectedRegistryURL {
			t.Errorf("REGISTRY_URL = %v, want %v", envVars["REGISTRY_URL"], expectedRegistryURL)
		}

		// And DOCKER_CONFIG should be set correctly
		expectedDockerConfig := filepath.ToSlash("/mock/home/.config/windsor/docker")
		if envVars["DOCKER_CONFIG"] != expectedDockerConfig {
			t.Errorf("DOCKER_CONFIG = %v, want %v", envVars["DOCKER_CONFIG"], expectedDockerConfig)
		}
	})

	t.Run("DockerDesktopDriver", func(t *testing.T) {
		// Given a new DockerEnvPrinter with Docker Desktop driver on Linux
		os.Unsetenv("DOCKER_HOST")
		mocks := setupDockerEnvMocks(t, &SetupOptions{
			ConfigStr: `
version: v1alpha1
contexts:
  test-context:
    vm:
      driver: docker-desktop
    docker:
      registries:
        mock-registry-url:
          hostport: 5000
`,
		})

		// And Linux OS environment
		mocks.Shims.Goos = func() string {
			return "linux"
		}

		// And mock filesystem operations
		mkdirAllCalled := false
		mkdirAllPath := ""
		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error {
			mkdirAllCalled = true
			mkdirAllPath = filepath.ToSlash(path)
			return nil
		}

		writeFileCalled := false
		writeFilePath := ""
		mocks.Shims.WriteFile = func(filename string, data []byte, perm os.FileMode) error {
			writeFileCalled = true
			writeFilePath = filepath.ToSlash(filename)
			return nil
		}

		printer := NewDockerEnvPrinter(mocks.Injector)
		printer.shims = mocks.Shims
		printer.Initialize()

		// When getting environment variables
		envVars, err := printer.GetEnvVars()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		// And DOCKER_HOST should be set correctly for Docker Desktop
		expectedDockerHost := fmt.Sprintf("unix://%s/.docker/run/docker.sock", filepath.ToSlash("/mock/home"))
		if envVars["DOCKER_HOST"] != expectedDockerHost {
			t.Errorf("DOCKER_HOST = %v, want %v", envVars["DOCKER_HOST"], expectedDockerHost)
		}

		// And REGISTRY_URL should be set correctly
		expectedRegistryURL := "mock-registry-url:5000"
		if envVars["REGISTRY_URL"] != expectedRegistryURL {
			t.Errorf("REGISTRY_URL = %v, want %v", envVars["REGISTRY_URL"], expectedRegistryURL)
		}

		// And DOCKER_CONFIG should be set correctly
		expectedDockerConfig := filepath.ToSlash("/mock/home/.config/windsor/docker")
		if envVars["DOCKER_CONFIG"] != expectedDockerConfig {
			t.Errorf("DOCKER_CONFIG = %v, want %v", envVars["DOCKER_CONFIG"], expectedDockerConfig)
		}

		// And directory should be created
		if !mkdirAllCalled {
			t.Error("mkdirAll was not called")
		} else {
			expectedMkdirAllPath := filepath.ToSlash("/mock/home/.config/windsor/docker")
			if mkdirAllPath != expectedMkdirAllPath {
				t.Errorf("mkdirAll path = %v, want %v", mkdirAllPath, expectedMkdirAllPath)
			}
		}

		// And config file should be written
		if !writeFileCalled {
			t.Error("writeFile was not called")
		} else {
			expectedWriteFilePath := filepath.ToSlash("/mock/home/.config/windsor/docker/config.json")
			if writeFilePath != expectedWriteFilePath {
				t.Errorf("writeFile path = %v, want %v", writeFilePath, expectedWriteFilePath)
			}
		}
	})

	t.Run("DockerDriver", func(t *testing.T) {
		// Given a new DockerEnvPrinter with Docker driver
		os.Unsetenv("DOCKER_HOST")
		mocks := setupDockerEnvMocks(t, &SetupOptions{
			ConfigStr: `
version: v1alpha1
contexts:
  test-context:
    vm:
      driver: docker
    docker:
      registries:
        mock-registry-url:
          hostport: 5000
`,
		})

		printer := NewDockerEnvPrinter(mocks.Injector)
		printer.shims = mocks.Shims
		printer.Initialize()

		// When getting environment variables
		envVars, err := printer.GetEnvVars()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		// And DOCKER_HOST should be set correctly for Docker driver and OS
		var expectedDockerHost string
		if mocks.Shims.Goos() == "windows" {
			expectedDockerHost = "npipe:////./pipe/docker_engine"
		} else {
			expectedDockerHost = "unix:///var/run/docker.sock"
		}
		if envVars["DOCKER_HOST"] != expectedDockerHost {
			t.Errorf("DOCKER_HOST = %v, want %v", envVars["DOCKER_HOST"], expectedDockerHost)
		}

		// And REGISTRY_URL should be set correctly
		expectedRegistryURL := "mock-registry-url:5000"
		if envVars["REGISTRY_URL"] != expectedRegistryURL {
			t.Errorf("REGISTRY_URL = %v, want %v", envVars["REGISTRY_URL"], expectedRegistryURL)
		}
	})

	t.Run("GetUserHomeDirError", func(t *testing.T) {
		// Given a new DockerEnvPrinter with failing user home directory lookup
		os.Unsetenv("DOCKER_HOST")
		mocks := setupDockerEnvMocks(t)

		// Override the UserHomeDir shim
		mocks.Shims.UserHomeDir = func() (string, error) {
			return "", errors.New("mock user home dir error")
		}

		printer := NewDockerEnvPrinter(mocks.Injector)
		printer.shims = mocks.Shims
		printer.Initialize()

		// When getting environment variables
		_, err := printer.GetEnvVars()

		// Then appropriate error should be returned
		if err == nil {
			t.Error("expected an error, got nil")
		} else if !strings.Contains(err.Error(), "mock user home dir error") {
			t.Errorf("error = %v, want error containing 'mock user home dir error'", err)
		}
	})

	t.Run("MkdirAllError", func(t *testing.T) {
		// Given a new DockerEnvPrinter with failing directory creation
		os.Unsetenv("DOCKER_HOST")
		mocks := setupDockerEnvMocks(t)

		// Override the MkdirAll shim
		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error {
			return errors.New("mock mkdirAll error")
		}

		printer := NewDockerEnvPrinter(mocks.Injector)
		printer.shims = mocks.Shims
		printer.Initialize()

		// When getting environment variables
		_, err := printer.GetEnvVars()

		// Then appropriate error should be returned
		if err == nil {
			t.Error("expected an error, got nil")
		} else if !strings.Contains(err.Error(), "mock mkdirAll error") {
			t.Errorf("error = %v, want error containing 'mock mkdirAll error'", err)
		}
	})

	t.Run("WriteFileError", func(t *testing.T) {
		// Given a new DockerEnvPrinter with failing file write
		os.Unsetenv("DOCKER_HOST")
		mocks := setupDockerEnvMocks(t)

		// Override the WriteFile shim
		mocks.Shims.WriteFile = func(filename string, data []byte, perm os.FileMode) error {
			return errors.New("mock writeFile error")
		}

		printer := NewDockerEnvPrinter(mocks.Injector)
		printer.shims = mocks.Shims
		printer.Initialize()

		// When getting environment variables
		_, err := printer.GetEnvVars()

		// Then appropriate error should be returned
		if err == nil {
			t.Error("expected an error, got nil")
		} else if !strings.Contains(err.Error(), "mock writeFile error") {
			t.Errorf("error = %v, want error containing 'mock writeFile error'", err)
		}
	})

	t.Run("DockerHostOSVariations", func(t *testing.T) {
		testCases := []struct {
			name     string
			os       string
			expected string
		}{
			{
				name:     "windows",
				os:       "windows",
				expected: "npipe:////./pipe/docker_engine",
			},
			{
				name:     "linux",
				os:       "linux",
				expected: fmt.Sprintf("unix://%s/.docker/run/docker.sock", filepath.ToSlash("/mock/home")),
			},
			{
				name:     "darwin",
				os:       "darwin",
				expected: fmt.Sprintf("unix://%s/.docker/run/docker.sock", filepath.ToSlash("/mock/home")),
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// Given a new DockerEnvPrinter with specific OS
				os.Unsetenv("DOCKER_HOST")
				mocks := setupDockerEnvMocks(t, &SetupOptions{
					ConfigStr: `
version: v1alpha1
contexts:
  test-context:
    vm:
      driver: docker-desktop
    docker:
      registries:
        mock-registry-url:
          hostport: 5000
`,
				})

				mocks.Shims.Goos = func() string {
					return tc.os
				}

				printer := NewDockerEnvPrinter(mocks.Injector)
				printer.shims = mocks.Shims
				printer.Initialize()

				// When getting environment variables
				envVars, err := printer.GetEnvVars()

				// Then no error should be returned
				if err != nil {
					t.Fatalf("GetEnvVars returned an error: %v", err)
				}

				// And DOCKER_HOST should be set correctly for the OS
				if envVars["DOCKER_HOST"] != tc.expected {
					t.Errorf("DOCKER_HOST = %v, want %v", envVars["DOCKER_HOST"], tc.expected)
				}
			})
		}
	})

	t.Run("DockerHostFromEnvironment", func(t *testing.T) {
		// Given a new DockerEnvPrinter with DOCKER_HOST environment variable
		os.Setenv("DOCKER_HOST", "tcp://custom-docker-host:2375")
		defer os.Unsetenv("DOCKER_HOST")

		mocks := setupDockerEnvMocks(t, &SetupOptions{
			ConfigStr: `
version: v1alpha1
contexts:
  test-context:
    vm:
      driver: docker-desktop
    docker:
      registries:
        mock-registry-url:
          hostport: 5000
`,
		})

		mocks.Shims.LookupEnv = func(key string) (string, bool) {
			if key == "DOCKER_HOST" {
				return "tcp://custom-docker-host:2375", true
			}
			return "", false
		}

		printer := NewDockerEnvPrinter(mocks.Injector)
		printer.shims = mocks.Shims
		printer.Initialize()

		// When getting environment variables
		envVars, err := printer.GetEnvVars()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		// And DOCKER_HOST should match environment value
		expectedDockerHost := "tcp://custom-docker-host:2375"
		if envVars["DOCKER_HOST"] != expectedDockerHost {
			t.Errorf("DOCKER_HOST = %v, want %v", envVars["DOCKER_HOST"], expectedDockerHost)
		}
	})

	t.Run("DockerHostNotSet", func(t *testing.T) {
		// Given a new DockerEnvPrinter without DOCKER_HOST environment variable
		mocks := setupDockerEnvMocks(t, &SetupOptions{
			ConfigStr: `
version: v1alpha1
contexts:
  test-context:
    vm:
      driver: colima
    docker:
      registries:
        mock-registry-url:
          hostport: 5000
`,
		})

		mocks.Shims.LookupEnv = func(key string) (string, bool) {
			return "", false
		}

		printer := NewDockerEnvPrinter(mocks.Injector)
		printer.shims = mocks.Shims
		printer.Initialize()

		// When getting environment variables
		envVars, err := printer.GetEnvVars()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		// And DOCKER_HOST should be set based on vm driver and OS
		var expectedDockerHost string
		if mocks.Shims.Goos() == "windows" {
			expectedDockerHost = "npipe:////./pipe/docker_engine"
		} else {
			expectedDockerHost = fmt.Sprintf("unix://%s/.colima/windsor-test-context/docker.sock", filepath.ToSlash("/mock/home"))
		}
		if envVars["DOCKER_HOST"] != expectedDockerHost {
			t.Errorf("DOCKER_HOST = %v, want %v", envVars["DOCKER_HOST"], expectedDockerHost)
		}
	})

	t.Run("DockerHostFromEnvironmentOverridesDriver", func(t *testing.T) {
		// Given a new DockerEnvPrinter with both DOCKER_HOST and vm driver
		mocks := setupDockerEnvMocks(t, &SetupOptions{
			ConfigStr: `
version: v1alpha1
contexts:
  test-context:
    vm:
      driver: docker-desktop
    docker:
      registries:
        mock-registry-url:
          hostport: 5000
`,
		})

		mocks.Shims.LookupEnv = func(key string) (string, bool) {
			if key == "DOCKER_HOST" {
				return "tcp://override-host:2375", true
			}
			return "", false
		}

		printer := NewDockerEnvPrinter(mocks.Injector)
		printer.shims = mocks.Shims
		printer.Initialize()

		// When getting environment variables
		envVars, err := printer.GetEnvVars()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		// And DOCKER_HOST should match environment value, not driver value
		expectedDockerHost := "tcp://override-host:2375"
		if envVars["DOCKER_HOST"] != expectedDockerHost {
			t.Errorf("DOCKER_HOST = %v, want %v", envVars["DOCKER_HOST"], expectedDockerHost)
		}
	})
}

// TestDockerEnvPrinter_GetAlias tests the GetAlias method of the DockerEnvPrinter
func TestDockerEnvPrinter_GetAlias(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new DockerEnvPrinter with docker-compose plugin available
		mocks := setupDockerEnvMocks(t)

		mocks.Shims.LookPath = func(file string) (string, error) {
			if file == "docker-cli-plugin-docker-compose" {
				return "/usr/local/bin/docker-cli-plugin-docker-compose", nil
			}
			return "", fmt.Errorf("not found")
		}

		printer := NewDockerEnvPrinter(mocks.Injector)
		printer.shims = mocks.Shims
		printer.Initialize()

		// When getting aliases
		aliasMap, err := printer.GetAlias()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// And docker-compose alias should be set correctly
		expectedAlias := "docker-cli-plugin-docker-compose"
		if aliasMap["docker-compose"] != expectedAlias {
			t.Errorf("aliasMap[docker-compose] = %v, want %v", aliasMap["docker-compose"], expectedAlias)
		}
	})

	t.Run("Failure", func(t *testing.T) {
		// Given a new DockerEnvPrinter without docker-compose plugin
		mocks := setupDockerEnvMocks(t)
		mocks.Shims.LookPath = func(file string) (string, error) {
			return "", fmt.Errorf("not found")
		}

		printer := NewDockerEnvPrinter(mocks.Injector)
		printer.shims = mocks.Shims
		printer.Initialize()

		// When getting aliases
		aliasMap, err := printer.GetAlias()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// And alias map should be empty
		if len(aliasMap) != 0 {
			t.Errorf("aliasMap = %v, want empty map", aliasMap)
		}
	})
}

// TestDockerEnvPrinter_getRegistryURL tests the getRegistryURL method of the DockerEnvPrinter
func TestDockerEnvPrinter_getRegistryURL(t *testing.T) {
	// setup creates a new DockerEnvPrinter with the given configuration
	setup := func(t *testing.T, opts ...*SetupOptions) (*DockerEnvPrinter, *Mocks) {
		t.Helper()
		mocks := setupDockerEnvMocks(t, opts...)
		printer := NewDockerEnvPrinter(mocks.Injector)
		printer.shims = mocks.Shims
		if err := printer.Initialize(); err != nil {
			t.Fatalf("Failed to initialize printer: %v", err)
		}
		return printer, mocks
	}

	t.Run("ValidRegistryURL", func(t *testing.T) {
		// Given a DockerEnvPrinter with a valid registry URL in config
		printer, _ := setup(t, &SetupOptions{
			ConfigStr: `
version: v1alpha1
contexts:
  test-context:
    vm:
      driver: colima
    docker:
      registry_url: registry.example.com:5000
`,
		})

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
		// Given a DockerEnvPrinter with a registry URL and matching config
		printer, _ := setup(t, &SetupOptions{
			ConfigStr: `
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
`,
		})

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
		// Given a DockerEnvPrinter with no registry URL but with registries config
		printer, _ := setup(t, &SetupOptions{
			ConfigStr: `
version: v1alpha1
contexts:
  test-context:
    vm:
      driver: colima
    docker:
      registries:
        mock-registry-url:
          hostport: 5000
`,
		})

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
		// Given a DockerEnvPrinter with empty registries config
		printer, _ := setup(t, &SetupOptions{
			ConfigStr: `
version: v1alpha1
contexts:
  test-context:
    vm:
      driver: colima
    docker:
      registries: {}
`,
		})

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
		// Given a DockerEnvPrinter with a registry URL without port and no matching config
		printer, _ := setup(t, &SetupOptions{
			ConfigStr: `
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
`,
		})

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
		// Given a DockerEnvPrinter with a registry URL with invalid port
		printer, _ := setup(t, &SetupOptions{
			ConfigStr: `
version: v1alpha1
contexts:
  test-context:
    vm:
      driver: colima
    docker:
      registry_url: registry.example.com:invalid
`,
		})

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
		// Given a DockerEnvPrinter with a registry URL without port and no hostport in config
		printer, _ := setup(t, &SetupOptions{
			ConfigStr: `
version: v1alpha1
contexts:
  test-context:
    vm:
      driver: colima
    docker:
      registry_url: registry.example.com
      registries:
        registry.example.com: {}
`,
		})

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
		// Given a DockerEnvPrinter with a registry URL and empty registries config
		printer, _ := setup(t, &SetupOptions{
			ConfigStr: `
version: v1alpha1
contexts:
  test-context:
    vm:
      driver: colima
    docker:
      registry_url: registry.example.com
      registries: {}
`,
		})

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
		// Given a DockerEnvPrinter with no Docker config
		printer, _ := setup(t, &SetupOptions{
			ConfigStr: `
version: v1alpha1
contexts:
  test-context:
    vm:
      driver: colima
`,
		})

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
		// Given a DockerEnvPrinter with a registry URL but no registries config
		printer, _ := setup(t, &SetupOptions{
			ConfigStr: `
version: v1alpha1
contexts:
  test-context:
    vm:
      driver: colima
    docker:
      registry_url: registry.example.com
`,
		})

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
		// Given a DockerEnvPrinter with a registry without hostport in config
		printer, _ := setup(t, &SetupOptions{
			ConfigStr: `
version: v1alpha1
contexts:
  test-context:
    vm:
      driver: colima
    docker:
      registries:
        registry.example.com:
          remote: ""
`,
		})

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
