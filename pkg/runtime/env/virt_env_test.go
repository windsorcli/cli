package env_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/env"
	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// =============================================================================
// Test Setup
// =============================================================================

// VirtEnvPrinterMocks holds all mock objects used in Virtual machine and container runtime environment tests
type VirtEnvPrinterMocks struct {
	Shell         *shell.MockShell
	ConfigHandler *config.MockConfigHandler
}

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

// TestVirtEnvPrinter_GetEnvVars tests the GetEnvVars method of the VirtEnvPrinter
func TestVirtEnvPrinter_GetEnvVars(t *testing.T) {
	// Save original env var and restore after all tests
	originalDockerHost := os.Getenv("DOCKER_HOST")
	defer os.Setenv("DOCKER_HOST", originalDockerHost)

	t.Run("Success", func(t *testing.T) {
		// Given a new VirtEnvPrinter with default configuration
		mocks := setupVirtEnvMocks(t)

		printer := env.NewVirtEnvPrinter(mocks.Shell, mocks.ConfigHandler)
		printer.SetShims(mocks.Shims)

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
		// Given a new VirtEnvPrinter with Colima driver
		os.Unsetenv("DOCKER_HOST")
		mocks := setupVirtEnvMocks(t)

		printer := env.NewVirtEnvPrinter(mocks.Shell, mocks.ConfigHandler)
		printer.SetShims(mocks.Shims)

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
		// Given a new VirtEnvPrinter with Docker Desktop workstation runtime on Linux
		os.Unsetenv("DOCKER_HOST")
		mocks := setupVirtEnvMocks(t)
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
		_ = mocks.ConfigHandler.Set("workstation.runtime", "docker-desktop")

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

		printer := env.NewVirtEnvPrinter(mocks.Shell, mocks.ConfigHandler)
		printer.SetShims(mocks.Shims)

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
		// Given a new VirtEnvPrinter with Docker workstation runtime
		os.Unsetenv("DOCKER_HOST")
		mocks := setupVirtEnvMocks(t)
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
		_ = mocks.ConfigHandler.Set("workstation.runtime", "docker")

		printer := env.NewVirtEnvPrinter(mocks.Shell, mocks.ConfigHandler)
		printer.SetShims(mocks.Shims)

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
		// Given a new VirtEnvPrinter with failing user home directory lookup
		os.Unsetenv("DOCKER_HOST")
		mocks := setupVirtEnvMocks(t)

		// Override the UserHomeDir shim
		mocks.Shims.UserHomeDir = func() (string, error) {
			return "", errors.New("mock user home dir error")
		}

		printer := env.NewVirtEnvPrinter(mocks.Shell, mocks.ConfigHandler)
		printer.SetShims(mocks.Shims)

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
		// Given a new VirtEnvPrinter with failing directory creation
		os.Unsetenv("DOCKER_HOST")
		mocks := setupVirtEnvMocks(t)

		// Override the MkdirAll shim
		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error {
			return errors.New("mock mkdirAll error")
		}

		printer := env.NewVirtEnvPrinter(mocks.Shell, mocks.ConfigHandler)
		printer.SetShims(mocks.Shims)

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
		// Given a new VirtEnvPrinter with failing file write
		os.Unsetenv("DOCKER_HOST")
		mocks := setupVirtEnvMocks(t)

		// Override the WriteFile shim
		mocks.Shims.WriteFile = func(filename string, data []byte, perm os.FileMode) error {
			return errors.New("mock writeFile error")
		}

		printer := env.NewVirtEnvPrinter(mocks.Shell, mocks.ConfigHandler)
		printer.SetShims(mocks.Shims)

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
				// Given a new VirtEnvPrinter with specific OS
				os.Unsetenv("DOCKER_HOST")
				mocks := setupVirtEnvMocks(t)
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
				_ = mocks.ConfigHandler.Set("workstation.runtime", "docker-desktop")

				mocks.Shims.Goos = func() string {
					return tc.os
				}

				printer := env.NewVirtEnvPrinter(mocks.Shell, mocks.ConfigHandler)
				printer.SetShims(mocks.Shims)

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

	t.Run("DockerHostPreservedWhenNoRuntime", func(t *testing.T) {
		// Given a new VirtEnvPrinter with DOCKER_HOST in the environment but no workstation runtime configured
		os.Setenv("DOCKER_HOST", "tcp://custom-docker-host:2375")
		defer os.Unsetenv("DOCKER_HOST")

		mocks := setupVirtEnvMocks(t)
		// Clear workstation.runtime so Windsor does not manage DOCKER_HOST
		_ = mocks.ConfigHandler.Set("workstation.runtime", "")

		mocks.Shims.LookupEnv = func(key string) (string, bool) {
			if key == "DOCKER_HOST" {
				return "tcp://custom-docker-host:2375", true
			}
			return "", false
		}

		printer := env.NewVirtEnvPrinter(mocks.Shell, mocks.ConfigHandler)
		printer.SetShims(mocks.Shims)

		// When getting environment variables
		envVars, err := printer.GetEnvVars()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		// And DOCKER_HOST should not be in envVars (Windsor leaves user-owned values alone)
		if _, exists := envVars["DOCKER_HOST"]; exists {
			t.Error("Expected DOCKER_HOST not to be in envVars when workstation.runtime is empty")
		}

		// And DOCKER_CONFIG should not be set (no runtime configured)
		if _, exists := envVars["DOCKER_CONFIG"]; exists {
			t.Error("Expected DOCKER_CONFIG not to be set when workstation.runtime is empty")
		}
	})

	t.Run("DockerHostNotSet", func(t *testing.T) {
		// Given a new VirtEnvPrinter without DOCKER_HOST environment variable
		mocks := setupVirtEnvMocks(t)

		mocks.Shims.LookupEnv = func(key string) (string, bool) {
			return "", false
		}

		printer := env.NewVirtEnvPrinter(mocks.Shell, mocks.ConfigHandler)
		printer.SetShims(mocks.Shims)

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

	t.Run("DockerHostUserOwnedNotManagedDockerConfigStillSet", func(t *testing.T) {
		// Given a new VirtEnvPrinter with workstation.runtime configured and a user-owned DOCKER_HOST
		// that Windsor has never managed (not in WINDSOR_MANAGED_ENV)
		mocks := setupVirtEnvMocks(t)
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

		mocks.Shims.LookupEnv = func(key string) (string, bool) {
			if key == "DOCKER_HOST" {
				return "tcp://user-owned-host:2375", true
			}
			return "", false
		}

		printer := env.NewVirtEnvPrinter(mocks.Shell, mocks.ConfigHandler)
		printer.SetShims(mocks.Shims)

		// When getting environment variables
		envVars, err := printer.GetEnvVars()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		// And DOCKER_HOST should not be in envVars (Windsor leaves user-owned values alone)
		if _, exists := envVars["DOCKER_HOST"]; exists {
			t.Errorf("Expected DOCKER_HOST not to be in envVars, got %v", envVars["DOCKER_HOST"])
		}

		// And DOCKER_CONFIG should still be set (always set when workstation.runtime is configured)
		expectedDockerConfig := filepath.ToSlash("/mock/home/.config/windsor/docker")
		if envVars["DOCKER_CONFIG"] != expectedDockerConfig {
			t.Errorf("DOCKER_CONFIG = %v, want %v", envVars["DOCKER_CONFIG"], expectedDockerConfig)
		}

		// And DOCKER_HOST should not be marked as managed
		managedEnv := printer.GetManagedEnv()
		for _, subject := range managedEnv {
			if subject == "DOCKER_HOST" {
				t.Error("Expected DOCKER_HOST not to be marked as managed when user-owned")
				break
			}
		}
	})

	t.Run("DockerHostManagedByWindsorIsReinjectedWithDockerConfig", func(t *testing.T) {
		// Given a new VirtEnvPrinter with DOCKER_HOST previously managed by Windsor
		// (present in WINDSOR_MANAGED_ENV) and workstation.runtime=colima
		os.Unsetenv("DOCKER_HOST")
		mocks := setupVirtEnvMocks(t)
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

		mocks.Shims.LookupEnv = func(key string) (string, bool) {
			if key == "DOCKER_HOST" {
				return "tcp://old-managed-host:2375", true
			}
			return "", false
		}

		mocks.Shims.Getenv = func(key string) string {
			if key == "WINDSOR_MANAGED_ENV" {
				return "DOCKER_HOST,DOCKER_CONFIG"
			}
			return ""
		}

		// Force non-Windows so the colima socket path is derived, not the named pipe
		mocks.Shims.Goos = func() string { return "linux" }

		printer := env.NewVirtEnvPrinter(mocks.Shell, mocks.ConfigHandler)
		printer.SetShims(mocks.Shims)

		// When getting environment variables
		envVars, err := printer.GetEnvVars()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		// And DOCKER_HOST should be re-derived from workstation.runtime (Windsor re-asserts ownership)
		expectedDockerHost := fmt.Sprintf("unix://%s/.colima/windsor-test-context/docker.sock", filepath.ToSlash("/mock/home"))
		if envVars["DOCKER_HOST"] != expectedDockerHost {
			t.Errorf("DOCKER_HOST = %v, want %v", envVars["DOCKER_HOST"], expectedDockerHost)
		}

		// And DOCKER_CONFIG should be set (always set when workstation.runtime is configured)
		expectedDockerConfig := filepath.ToSlash("/mock/home/.config/windsor/docker")
		if envVars["DOCKER_CONFIG"] != expectedDockerConfig {
			t.Errorf("DOCKER_CONFIG = %v, want %v", envVars["DOCKER_CONFIG"], expectedDockerConfig)
		}

		// And DOCKER_HOST should be marked as managed
		managedEnv := printer.GetManagedEnv()
		isManaged := false
		for _, subject := range managedEnv {
			if subject == "DOCKER_HOST" {
				isManaged = true
				break
			}
		}
		if !isManaged {
			t.Error("Expected DOCKER_HOST to be marked as managed when re-injected")
		}
	})

	t.Run("DockerHostUserOwnedExplicitlyNotManagedDockerConfigStillSet", func(t *testing.T) {
		// Given a new VirtEnvPrinter with DOCKER_HOST set by external tooling, explicitly absent
		// from WINDSOR_MANAGED_ENV, and workstation.runtime=colima
		os.Unsetenv("DOCKER_HOST")
		mocks := setupVirtEnvMocks(t)
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

		mocks.Shims.LookupEnv = func(key string) (string, bool) {
			if key == "DOCKER_HOST" {
				return "tcp://custom-unmanaged-host:2375", true
			}
			return "", false
		}
		mocks.Shims.Getenv = func(key string) string {
			if key == "WINDSOR_MANAGED_ENV" {
				return "DOCKER_CONFIG" // DOCKER_HOST not in managed list
			}
			return ""
		}

		printer := env.NewVirtEnvPrinter(mocks.Shell, mocks.ConfigHandler)
		printer.SetShims(mocks.Shims)

		// When getting environment variables
		envVars, err := printer.GetEnvVars()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		// And DOCKER_HOST should not be in envVars (Windsor leaves user-owned values alone)
		if _, exists := envVars["DOCKER_HOST"]; exists {
			t.Errorf("Expected DOCKER_HOST not to be in envVars, got %v", envVars["DOCKER_HOST"])
		}

		// And DOCKER_CONFIG should be set (always set when workstation.runtime is configured)
		expectedDockerConfig := filepath.ToSlash("/mock/home/.config/windsor/docker")
		if envVars["DOCKER_CONFIG"] != expectedDockerConfig {
			t.Errorf("DOCKER_CONFIG = %v, want %v", envVars["DOCKER_CONFIG"], expectedDockerConfig)
		}

		// And DOCKER_HOST should not be marked as managed
		managedEnv := printer.GetManagedEnv()
		for _, subject := range managedEnv {
			if subject == "DOCKER_HOST" {
				t.Error("Expected DOCKER_HOST not to be marked as managed when user-owned")
				break
			}
		}
	})

	t.Run("DockerHostNotSetWithWorkstationRuntime", func(t *testing.T) {
		// Given a new VirtEnvPrinter with workstation.runtime set but no DOCKER_HOST
		os.Unsetenv("DOCKER_HOST")
		mocks := setupVirtEnvMocks(t)
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
		_ = mocks.ConfigHandler.Set("workstation.runtime", "docker-desktop")

		mocks.Shims.LookupEnv = func(key string) (string, bool) {
			return "", false // DOCKER_HOST doesn't exist
		}

		printer := env.NewVirtEnvPrinter(mocks.Shell, mocks.ConfigHandler)
		printer.SetShims(mocks.Shims)

		// When getting environment variables
		envVars, err := printer.GetEnvVars()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		// And DOCKER_HOST should be set based on workstation.runtime and OS
		var expectedDockerHost string
		if mocks.Shims.Goos() == "windows" {
			expectedDockerHost = "npipe:////./pipe/docker_engine"
		} else {
			expectedDockerHost = fmt.Sprintf("unix://%s/.docker/run/docker.sock", filepath.ToSlash("/mock/home"))
		}
		if envVars["DOCKER_HOST"] != expectedDockerHost {
			t.Errorf("DOCKER_HOST = %v, want %v", envVars["DOCKER_HOST"], expectedDockerHost)
		}

		// And DOCKER_HOST should be marked as managed
		managedEnv := printer.GetManagedEnv()
		isManaged := false
		for _, subject := range managedEnv {
			if subject == "DOCKER_HOST" {
				isManaged = true
				break
			}
		}
		if !isManaged {
			t.Error("Expected DOCKER_HOST to be marked as managed when set by workstation.runtime")
		}
	})

	t.Run("DockerHostNotSetWithoutWorkstationRuntime", func(t *testing.T) {
		// Given a new VirtEnvPrinter without workstation.runtime and no DOCKER_HOST
		os.Unsetenv("DOCKER_HOST")
		baseMocks := setupEnvMocks(t)
		mocks := setupVirtEnvMocks(t, &EnvTestMocks{
			ConfigHandler: config.NewConfigHandler(baseMocks.Shell),
		})
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

		mocks.Shims.LookupEnv = func(key string) (string, bool) {
			return "", false // DOCKER_HOST doesn't exist
		}

		printer := env.NewVirtEnvPrinter(mocks.Shell, mocks.ConfigHandler)
		printer.SetShims(mocks.Shims)

		// When getting environment variables
		envVars, err := printer.GetEnvVars()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		// And DOCKER_HOST should not be set
		if _, exists := envVars["DOCKER_HOST"]; exists {
			t.Errorf("Expected DOCKER_HOST not to be set when workstation.runtime is empty, got %v", envVars["DOCKER_HOST"])
		}

		// And DOCKER_HOST should not be marked as managed
		managedEnv := printer.GetManagedEnv()
		isManaged := false
		for _, subject := range managedEnv {
			if subject == "DOCKER_HOST" {
				isManaged = true
				break
			}
		}
		if isManaged {
			t.Error("Expected DOCKER_HOST not to be marked as managed when not set")
		}
	})

	t.Run("DockerHostEmptyStringNotPropagatedDockerConfigStillSet", func(t *testing.T) {
		// Given a new VirtEnvPrinter with workstation.runtime=colima and DOCKER_HOST set to empty string
		os.Unsetenv("DOCKER_HOST")
		mocks := setupVirtEnvMocks(t)
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

		mocks.Shims.LookupEnv = func(key string) (string, bool) {
			if key == "DOCKER_HOST" {
				return "", true // Empty string but exists
			}
			if key == "WINDSOR_MANAGED_ENV" {
				return "DOCKER_CONFIG", true
			}
			return "", false
		}
		mocks.Shims.Getenv = func(key string) string {
			if key == "WINDSOR_MANAGED_ENV" {
				return "DOCKER_CONFIG"
			}
			return ""
		}

		printer := env.NewVirtEnvPrinter(mocks.Shell, mocks.ConfigHandler)
		printer.SetShims(mocks.Shims)

		// When getting environment variables
		envVars, err := printer.GetEnvVars()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		// And DOCKER_HOST should not be propagated (empty value is not set)
		if dockerHost, exists := envVars["DOCKER_HOST"]; exists && dockerHost != "" {
			t.Errorf("DOCKER_HOST = %v, want empty or absent", dockerHost)
		}

		// And DOCKER_CONFIG should be set (always set when workstation.runtime is configured)
		expectedDockerConfig := filepath.ToSlash("/mock/home/.config/windsor/docker")
		if envVars["DOCKER_CONFIG"] != expectedDockerConfig {
			t.Errorf("DOCKER_CONFIG = %v, want %v", envVars["DOCKER_CONFIG"], expectedDockerConfig)
		}
	})

	t.Run("IncusRuntimeSetsIncusSocket", func(t *testing.T) {
		// Given a new VirtEnvPrinter with colima workstation runtime and incus platform
		os.Unsetenv("DOCKER_HOST")
		mocks := setupVirtEnvMocks(t)
		configStr := `
version: v1alpha1
contexts:
  test-context:
    platform: incus
    provider: incus
    docker:
      registries:
        mock-registry-url:
          hostport: 5000
`
		if err := mocks.ConfigHandler.LoadConfigString(configStr); err != nil {
			t.Fatalf("Failed to load config: %v", err)
		}

		printer := env.NewVirtEnvPrinter(mocks.Shell, mocks.ConfigHandler)
		printer.SetShims(mocks.Shims)

		// When getting environment variables
		envVars, err := printer.GetEnvVars()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		// And INCUS_SOCKET should be set
		expectedIncusSocket := filepath.ToSlash("/mock/home/.colima/windsor-test-context/incus.sock")
		if envVars["INCUS_SOCKET"] != expectedIncusSocket {
			t.Errorf("INCUS_SOCKET = %v, want %v", envVars["INCUS_SOCKET"], expectedIncusSocket)
		}

		// And DOCKER_HOST should not be set (since runtime is incus)
		if _, exists := envVars["DOCKER_HOST"]; exists {
			t.Error("Expected DOCKER_HOST not to be set when runtime is incus")
		}

		// And INCUS_SOCKET should be marked as managed
		managedEnv := printer.GetManagedEnv()
		isManaged := false
		for _, subject := range managedEnv {
			if subject == "INCUS_SOCKET" {
				isManaged = true
				break
			}
		}
		if !isManaged {
			t.Error("Expected INCUS_SOCKET to be marked as managed")
		}
	})

	t.Run("DockerRuntimeDoesNotSetIncusSocket", func(t *testing.T) {
		// Given a new VirtEnvPrinter with colima driver and docker runtime (default)
		os.Unsetenv("DOCKER_HOST")
		mocks := setupVirtEnvMocks(t)
		configStr := `
version: v1alpha1
contexts:
  test-context:
    vm:
      driver: colima
      runtime: docker
    docker:
      registries:
        mock-registry-url:
          hostport: 5000
`
		if err := mocks.ConfigHandler.LoadConfigString(configStr); err != nil {
			t.Fatalf("Failed to load config: %v", err)
		}

		printer := env.NewVirtEnvPrinter(mocks.Shell, mocks.ConfigHandler)
		printer.SetShims(mocks.Shims)

		// When getting environment variables
		envVars, err := printer.GetEnvVars()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		// And INCUS_SOCKET should not be set
		if _, exists := envVars["INCUS_SOCKET"]; exists {
			t.Error("Expected INCUS_SOCKET not to be set when runtime is docker")
		}

		// And DOCKER_HOST should be set
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
}

// TestVirtEnvPrinter_GetAlias tests the GetAlias method of the VirtEnvPrinter
func TestVirtEnvPrinter_GetAlias(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new VirtEnvPrinter with docker-compose plugin available
		mocks := setupVirtEnvMocks(t)

		mocks.Shims.LookPath = func(file string) (string, error) {
			if file == "docker-cli-plugin-docker-compose" {
				return "/usr/local/bin/docker-cli-plugin-docker-compose", nil
			}
			return "", fmt.Errorf("not found")
		}

		printer := env.NewVirtEnvPrinter(mocks.Shell, mocks.ConfigHandler)
		printer.SetShims(mocks.Shims)

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
		// Given a new VirtEnvPrinter without docker-compose plugin
		mocks := setupVirtEnvMocks(t)
		mocks.Shims.LookPath = func(file string) (string, error) {
			return "", fmt.Errorf("not found")
		}

		printer := env.NewVirtEnvPrinter(mocks.Shell, mocks.ConfigHandler)
		printer.SetShims(mocks.Shims)

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
