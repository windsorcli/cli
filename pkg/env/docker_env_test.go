package env

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/api/v1alpha1/docker"
	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/services"
	"github.com/windsorcli/cli/pkg/shell"
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

// setupSafeDockerEnvPrinterMocks creates and configures mock objects for Docker environment tests.
// It accepts an optional injector parameter and returns initialized DockerEnvPrinterMocks.
func setupSafeDockerEnvPrinterMocks(injector ...di.Injector) *DockerEnvPrinterMocks {
	var mockInjector di.Injector
	if len(injector) > 0 {
		mockInjector = injector[0]
	} else {
		mockInjector = di.NewInjector()
	}

	mockShell := shell.NewMockShell()

	mockConfigHandler := config.NewMockConfigHandler()
	mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
		switch key {
		case "vm.driver":
			return "colima"
		case "dns.domain":
			return "mock-domain"
		case "docker.registry_url":
			return "mock-registry-url"
		default:
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}
	}
	mockConfigHandler.GetConfigRootFunc = func() (string, error) {
		return filepath.Join("mock", "config", "root"), nil
	}
	mockConfigHandler.GetContextFunc = func() string {
		return "mock-context"
	}
	mockConfigHandler.GetConfigFunc = func() *v1alpha1.Context {
		return &v1alpha1.Context{
			Docker: &docker.DockerConfig{
				Registries: map[string]docker.RegistryConfig{
					"mock-registry-url": {
						HostPort: 5000,
					},
				},
			},
		}
	}

	mkdirAll = func(path string, perm os.FileMode) error {
		return nil
	}

	writeFile = func(filename string, data []byte, perm os.FileMode) error {
		return nil
	}

	readFile = func(_ string) ([]byte, error) {
		return nil, nil
	}

	osUserHomeDir = func() (string, error) {
		return filepath.ToSlash("/mock/home"), nil
	}

	// Use the real RegistryService
	registryService := services.NewRegistryService(mockInjector)
	registryService.SetName("mock-registry")
	registryService.SetAddress("mock-registry-url")

	mockInjector.Register("shell", mockShell)
	mockInjector.Register("configHandler", mockConfigHandler)
	mockInjector.Register("registryService", registryService)

	// Initialize the RegistryService
	registryService.Initialize()

	return &DockerEnvPrinterMocks{
		Injector:      mockInjector,
		Shell:         mockShell,
		ConfigHandler: mockConfigHandler,
	}
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
		// Given a new DockerEnvPrinter with no DOCKER_HOST set
		os.Unsetenv("DOCKER_HOST")
		mocks := setupSafeDockerEnvPrinterMocks()
		dockerEnvPrinter := NewDockerEnvPrinter(mocks.Injector)
		dockerEnvPrinter.Initialize()

		// When getting environment variables
		envVars, err := dockerEnvPrinter.GetEnvVars()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		// And DOCKER_HOST should be set correctly
		expectedDockerHost := fmt.Sprintf("unix://%s/.colima/windsor-mock-context/docker.sock", filepath.ToSlash("/mock/home"))
		if envVars["DOCKER_HOST"] != expectedDockerHost {
			t.Errorf("DOCKER_HOST = %v, want %v", envVars["DOCKER_HOST"], expectedDockerHost)
		}

		// And REGISTRY_URL should be set correctly
		if envVars["REGISTRY_URL"] != "mock-registry-url:5000" {
			t.Errorf("REGISTRY_URL = %v, want mock-registry-url:5000", envVars["REGISTRY_URL"])
		}
	})

	t.Run("ColimaDriver", func(t *testing.T) {
		// Given a new DockerEnvPrinter with Colima driver and test context
		os.Unsetenv("DOCKER_HOST")
		mocks := setupSafeDockerEnvPrinterMocks()
		mocks.ConfigHandler.GetContextFunc = func() string {
			return "test-context"
		}

		dockerEnvPrinter := NewDockerEnvPrinter(mocks.Injector)
		dockerEnvPrinter.Initialize()

		// When getting environment variables
		envVars, err := dockerEnvPrinter.GetEnvVars()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		// And DOCKER_HOST should be set correctly for Colima
		expectedDockerHost := fmt.Sprintf("unix://%s/.colima/windsor-%s/docker.sock", filepath.ToSlash("/mock/home"), "test-context")
		if envVars["DOCKER_HOST"] != expectedDockerHost {
			t.Errorf("DOCKER_HOST = %v, want %v", envVars["DOCKER_HOST"], expectedDockerHost)
		}

		// And REGISTRY_URL should be set correctly
		if envVars["REGISTRY_URL"] != "mock-registry-url:5000" {
			t.Errorf("REGISTRY_URL = %v, want mock-registry-url:5000", envVars["REGISTRY_URL"])
		}
	})

	t.Run("DockerDesktopDriver", func(t *testing.T) {
		// Given a new DockerEnvPrinter with Docker Desktop driver on Linux
		os.Unsetenv("DOCKER_HOST")
		mocks := setupSafeDockerEnvPrinterMocks()

		// And Linux OS environment
		originalGoos := goos
		defer func() { goos = originalGoos }()
		goos = func() string {
			return "linux"
		}

		// And mock filesystem operations
		originalMkdirAll := mkdirAll
		defer func() { mkdirAll = originalMkdirAll }()
		mkdirAllCalled := false
		mkdirAllPath := ""
		mkdirAll = func(path string, perm os.FileMode) error {
			mkdirAllCalled = true
			mkdirAllPath = filepath.ToSlash(path)
			return nil
		}

		originalWriteFile := writeFile
		defer func() { writeFile = originalWriteFile }()
		writeFileCalled := false
		writeFilePath := ""
		writeFile = func(filename string, data []byte, perm os.FileMode) error {
			writeFileCalled = true
			writeFilePath = filepath.ToSlash(filename)
			return nil
		}

		// And Docker Desktop driver configuration
		originalGetStringFunc := mocks.ConfigHandler.GetStringFunc
		mocks.ConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "vm.driver" {
				return "docker-desktop"
			}
			return originalGetStringFunc(key, defaultValue...)
		}

		dockerEnvPrinter := NewDockerEnvPrinter(mocks.Injector)
		dockerEnvPrinter.Initialize()

		// When getting environment variables
		envVars, err := dockerEnvPrinter.GetEnvVars()

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
		mocks := setupSafeDockerEnvPrinterMocks()
		mocks.ConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "vm.driver" {
				return "docker"
			}
			return ""
		}

		dockerEnvPrinter := NewDockerEnvPrinter(mocks.Injector)
		dockerEnvPrinter.Initialize()

		// When getting environment variables
		envVars, err := dockerEnvPrinter.GetEnvVars()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		// And DOCKER_HOST should be set correctly for Docker driver
		expectedDockerHost := "unix:///var/run/docker.sock"
		if envVars["DOCKER_HOST"] != expectedDockerHost {
			t.Errorf("DOCKER_HOST = %v, want %v", envVars["DOCKER_HOST"], expectedDockerHost)
		}

		// And REGISTRY_URL should be empty
		if envVars["REGISTRY_URL"] != "" {
			t.Errorf("REGISTRY_URL = %v, want empty", envVars["REGISTRY_URL"])
		}
	})

	t.Run("GetUserHomeDirError", func(t *testing.T) {
		// Given a new DockerEnvPrinter with failing user home directory lookup
		os.Unsetenv("DOCKER_HOST")
		mocks := setupSafeDockerEnvPrinterMocks()
		mocks.ConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "vm.driver" {
				return "colima"
			}
			return ""
		}

		originalUserHomeDir := osUserHomeDir
		defer func() { osUserHomeDir = originalUserHomeDir }()
		osUserHomeDir = func() (string, error) {
			return "", errors.New("mock user home dir error")
		}

		dockerEnvPrinter := NewDockerEnvPrinter(mocks.Injector)
		dockerEnvPrinter.Initialize()

		// When getting environment variables
		_, err := dockerEnvPrinter.GetEnvVars()

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
		mocks := setupSafeDockerEnvPrinterMocks()
		mocks.ConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "vm.driver" {
				return "colima"
			}
			return ""
		}

		originalMkdirAll := mkdirAll
		defer func() { mkdirAll = originalMkdirAll }()
		mkdirAll = func(path string, perm os.FileMode) error {
			return errors.New("mock mkdirAll error")
		}

		dockerEnvPrinter := NewDockerEnvPrinter(mocks.Injector)
		dockerEnvPrinter.Initialize()

		// When getting environment variables
		_, err := dockerEnvPrinter.GetEnvVars()

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
		mocks := setupSafeDockerEnvPrinterMocks()
		mocks.ConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "vm.driver" {
				return "colima"
			}
			return ""
		}

		originalWriteFile := writeFile
		defer func() { writeFile = originalWriteFile }()
		writeFile = func(filename string, data []byte, perm os.FileMode) error {
			return errors.New("mock writeFile error")
		}

		dockerEnvPrinter := NewDockerEnvPrinter(mocks.Injector)
		dockerEnvPrinter.Initialize()

		// When getting environment variables
		_, err := dockerEnvPrinter.GetEnvVars()

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
				mocks := setupSafeDockerEnvPrinterMocks()
				mocks.ConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
					if key == "vm.driver" {
						return "docker-desktop"
					}
					return ""
				}

				originalGoos := goos
				defer func() { goos = originalGoos }()
				goos = func() string {
					return tc.os
				}

				dockerEnvPrinter := NewDockerEnvPrinter(mocks.Injector)
				dockerEnvPrinter.Initialize()

				// When getting environment variables
				envVars, err := dockerEnvPrinter.GetEnvVars()

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

		mocks := setupSafeDockerEnvPrinterMocks()

		originalLookupEnv := osLookupEnv
		defer func() { osLookupEnv = originalLookupEnv }()

		osLookupEnv = func(key string) (string, bool) {
			if key == "DOCKER_HOST" {
				return "tcp://custom-docker-host:2375", true
			}
			return "", false
		}

		dockerEnvPrinter := NewDockerEnvPrinter(mocks.Injector)
		dockerEnvPrinter.Initialize()

		// When getting environment variables
		envVars, err := dockerEnvPrinter.GetEnvVars()

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
		mocks := setupSafeDockerEnvPrinterMocks()

		originalLookupEnv := osLookupEnv
		defer func() { osLookupEnv = originalLookupEnv }()

		osLookupEnv = func(key string) (string, bool) {
			return "", false
		}

		dockerEnvPrinter := NewDockerEnvPrinter(mocks.Injector)
		dockerEnvPrinter.Initialize()

		// When getting environment variables
		envVars, err := dockerEnvPrinter.GetEnvVars()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		// And DOCKER_HOST should be set based on vm driver
		expectedDockerHost := fmt.Sprintf("unix://%s/.colima/windsor-mock-context/docker.sock", filepath.ToSlash("/mock/home"))
		if envVars["DOCKER_HOST"] != expectedDockerHost {
			t.Errorf("DOCKER_HOST = %v, want %v", envVars["DOCKER_HOST"], expectedDockerHost)
		}
	})

	t.Run("DockerHostFromEnvironmentOverridesDriver", func(t *testing.T) {
		// Given a new DockerEnvPrinter with both DOCKER_HOST and vm driver
		mocks := setupSafeDockerEnvPrinterMocks()

		originalLookupEnv := osLookupEnv
		defer func() { osLookupEnv = originalLookupEnv }()

		osLookupEnv = func(key string) (string, bool) {
			if key == "DOCKER_HOST" {
				return "tcp://override-host:2375", true
			}
			return "", false
		}

		mocks.ConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			switch key {
			case "vm.driver":
				return "docker-desktop"
			default:
				if len(defaultValue) > 0 {
					return defaultValue[0]
				}
				return ""
			}
		}

		dockerEnvPrinter := NewDockerEnvPrinter(mocks.Injector)
		dockerEnvPrinter.Initialize()

		// When getting environment variables
		envVars, err := dockerEnvPrinter.GetEnvVars()

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
		mocks := setupSafeDockerEnvPrinterMocks()
		originalExecLookPath := execLookPath
		defer func() { execLookPath = originalExecLookPath }()
		execLookPath = func(file string) (string, error) {
			if file == "docker-cli-plugin-docker-compose" {
				return "/usr/local/bin/docker-cli-plugin-docker-compose", nil
			}
			return "", fmt.Errorf("not found")
		}

		dockerEnvPrinter := NewDockerEnvPrinter(mocks.Injector)
		dockerEnvPrinter.Initialize()

		// When getting aliases
		aliasMap, err := dockerEnvPrinter.GetAlias()

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
		mocks := setupSafeDockerEnvPrinterMocks()
		originalExecLookPath := execLookPath
		defer func() { execLookPath = originalExecLookPath }()
		execLookPath = func(file string) (string, error) {
			return "", fmt.Errorf("not found")
		}

		dockerEnvPrinter := NewDockerEnvPrinter(mocks.Injector)
		dockerEnvPrinter.Initialize()

		// When getting aliases
		aliasMap, err := dockerEnvPrinter.GetAlias()

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

// TestDockerEnvPrinter_Print tests the Print method of the DockerEnvPrinter
func TestDockerEnvPrinter_Print(t *testing.T) {
	// Save original env var and restore after all tests
	originalDockerHost := os.Getenv("DOCKER_HOST")
	defer os.Setenv("DOCKER_HOST", originalDockerHost)

	t.Run("Success", func(t *testing.T) {
		// Given a new DockerEnvPrinter
		mocks := setupSafeDockerEnvPrinterMocks()
		dockerEnvPrinter := NewDockerEnvPrinter(mocks.Injector)
		dockerEnvPrinter.Initialize()

		// And PrintEnvVarsFunc is mocked
		var capturedEnvVars map[string]string
		mocks.Shell.PrintEnvVarsFunc = func(envVars map[string]string) {
			capturedEnvVars = envVars
		}

		// When calling Print
		err := dockerEnvPrinter.Print()

		// Then no error should be returned
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		// And environment variables should be set correctly
		expectedEnvVars, _ := dockerEnvPrinter.GetEnvVars()
		if !reflect.DeepEqual(capturedEnvVars, expectedEnvVars) {
			t.Errorf("capturedEnvVars = %v, want %v", capturedEnvVars, expectedEnvVars)
		}
	})

	t.Run("GetEnvVarsError", func(t *testing.T) {
		// Given a new DockerEnvPrinter with failing user home directory lookup
		os.Unsetenv("DOCKER_HOST")
		mocks := setupSafeDockerEnvPrinterMocks()

		originalOsUserHomeDir := osUserHomeDir
		defer func() { osUserHomeDir = originalOsUserHomeDir }()
		osUserHomeDir = func() (string, error) {
			return "", errors.New("mock error")
		}

		dockerEnvPrinter := NewDockerEnvPrinter(mocks.Injector)
		dockerEnvPrinter.Initialize()

		// When calling Print
		err := dockerEnvPrinter.Print()

		// Then appropriate error should be returned
		if err == nil {
			t.Error("expected an error, got nil")
		}
	})
}
