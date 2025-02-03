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

type DockerEnvPrinterMocks struct {
	Injector      di.Injector
	Shell         *shell.MockShell
	ConfigHandler *config.MockConfigHandler
}

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

func TestDockerEnvPrinter_GetEnvVars(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupSafeDockerEnvPrinterMocks()

		dockerEnvPrinter := NewDockerEnvPrinter(mocks.Injector)
		dockerEnvPrinter.Initialize()

		envVars, err := dockerEnvPrinter.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		expectedDockerHost := fmt.Sprintf("unix://%s/.colima/windsor-mock-context/docker.sock", filepath.ToSlash("/mock/home"))
		if envVars["DOCKER_HOST"] != expectedDockerHost {
			t.Errorf("DOCKER_HOST = %v, want %v", envVars["DOCKER_HOST"], expectedDockerHost)
		}

		if envVars["REGISTRY_URL"] != "mock-registry-url:5000" {
			t.Errorf("REGISTRY_URL = %v, want mock-registry-url:5000", envVars["REGISTRY_URL"])
		}
	})

	t.Run("ColimaDriver", func(t *testing.T) {
		mocks := setupSafeDockerEnvPrinterMocks()
		mocks.ConfigHandler.GetContextFunc = func() string {
			return "test-context"
		}

		dockerEnvPrinter := NewDockerEnvPrinter(mocks.Injector)
		dockerEnvPrinter.Initialize()

		envVars, err := dockerEnvPrinter.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		expectedDockerHost := fmt.Sprintf("unix://%s/.colima/windsor-%s/docker.sock", filepath.ToSlash("/mock/home"), "test-context")
		if envVars["DOCKER_HOST"] != expectedDockerHost {
			t.Errorf("DOCKER_HOST = %v, want %v", envVars["DOCKER_HOST"], expectedDockerHost)
		}

		if envVars["REGISTRY_URL"] != "mock-registry-url:5000" {
			t.Errorf("REGISTRY_URL = %v, want mock-registry-url:5000", envVars["REGISTRY_URL"])
		}
	})

	t.Run("DockerDesktopDriver", func(t *testing.T) {
		mocks := setupSafeDockerEnvPrinterMocks()

		// Mock goos function to simulate different OS environments
		originalGoos := goos
		defer func() { goos = originalGoos }()
		goos = func() string {
			return "linux"
		}

		// Mock mkdirAll function
		originalMkdirAll := mkdirAll
		defer func() { mkdirAll = originalMkdirAll }()
		mkdirAllCalled := false
		mkdirAllPath := ""
		mkdirAll = func(path string, perm os.FileMode) error {
			mkdirAllCalled = true
			mkdirAllPath = filepath.ToSlash(path)
			return nil
		}

		// Mock writeFile function
		originalWriteFile := writeFile
		defer func() { writeFile = originalWriteFile }()
		writeFileCalled := false
		writeFilePath := ""
		writeFile = func(filename string, data []byte, perm os.FileMode) error {
			writeFileCalled = true
			writeFilePath = filepath.ToSlash(filename)
			return nil
		}

		// Use the existing mockConfigHandler from mocks
		originalGetStringFunc := mocks.ConfigHandler.GetStringFunc
		mocks.ConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "vm.driver" {
				return "docker-desktop"
			}
			return originalGetStringFunc(key, defaultValue...)
		}

		dockerEnvPrinter := NewDockerEnvPrinter(mocks.Injector)
		dockerEnvPrinter.Initialize()

		envVars, err := dockerEnvPrinter.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		expectedDockerHost := fmt.Sprintf("unix://%s/.docker/run/docker.sock", filepath.ToSlash("/mock/home"))
		if envVars["DOCKER_HOST"] != expectedDockerHost {
			t.Errorf("DOCKER_HOST = %v, want %v", envVars["DOCKER_HOST"], expectedDockerHost)
		}

		expectedRegistryURL := "mock-registry-url:5000"
		if envVars["REGISTRY_URL"] != expectedRegistryURL {
			t.Errorf("REGISTRY_URL = %v, want %v", envVars["REGISTRY_URL"], expectedRegistryURL)
		}

		if !mkdirAllCalled {
			t.Error("mkdirAll was not called")
		} else {
			expectedMkdirAllPath := filepath.ToSlash("/mock/home/.config/windsor/docker")
			if mkdirAllPath != expectedMkdirAllPath {
				t.Errorf("mkdirAll path = %v, want %v", mkdirAllPath, expectedMkdirAllPath)
			}
		}

		if !writeFileCalled {
			t.Error("writeFile was not called")
		} else {
			expectedWriteFilePath := filepath.ToSlash("/mock/home/.config/windsor/docker/config.json")
			if writeFilePath != expectedWriteFilePath {
				t.Errorf("writeFile path = %v, want %v", writeFilePath, expectedWriteFilePath)
			}
		}
	})

	t.Run("GetUserHomeDirError", func(t *testing.T) {
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

		_, err := dockerEnvPrinter.GetEnvVars()
		if err == nil {
			t.Error("expected an error, got nil")
		} else if !strings.Contains(err.Error(), "mock user home dir error") {
			t.Errorf("error = %v, want error containing 'mock user home dir error'", err)
		}
	})

	t.Run("MkdirAllError", func(t *testing.T) {
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

		_, err := dockerEnvPrinter.GetEnvVars()
		if err == nil {
			t.Error("expected an error, got nil")
		} else if !strings.Contains(err.Error(), "mock mkdirAll error") {
			t.Errorf("error = %v, want error containing 'mock mkdirAll error'", err)
		}
	})

	t.Run("WriteFileError", func(t *testing.T) {
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

		_, err := dockerEnvPrinter.GetEnvVars()
		if err == nil {
			t.Error("expected an error, got nil")
		} else if !strings.Contains(err.Error(), "mock writeFile error") {
			t.Errorf("error = %v, want error containing 'mock writeFile error'", err)
		}
	})

	t.Run("DockerHostOSVariations", func(t *testing.T) {
		testCases := []struct {
			osName       string
			expectedHost string
		}{
			{"windows", "npipe:////./pipe/docker_engine"},
			{"linux", "unix:///home/user/.docker/run/docker.sock"},
			{"darwin", "unix:///home/user/.docker/run/docker.sock"},
		}

		for _, tc := range testCases {
			t.Run(tc.osName, func(t *testing.T) {
				mocks := setupSafeDockerEnvPrinterMocks()
				mocks.ConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
					switch key {
					case "vm.driver":
						return "docker-desktop"
					case "dns.domain":
						return "mock-domain"
					case "docker.registry_url":
						return "mock-registry-url"
					default:
						return ""
					}
				}

				originalGoos := goos
				defer func() { goos = originalGoos }()
				goos = func() string {
					return tc.osName
				}

				originalUserHomeDir := osUserHomeDir
				defer func() { osUserHomeDir = originalUserHomeDir }()
				osUserHomeDir = func() (string, error) {
					return filepath.ToSlash("/home/user"), nil
				}

				dockerEnvPrinter := NewDockerEnvPrinter(mocks.Injector)
				dockerEnvPrinter.Initialize()

				envVars, err := dockerEnvPrinter.GetEnvVars()
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}

				if envVars["DOCKER_HOST"] != tc.expectedHost {
					t.Fatalf("DOCKER_HOST = %v, want %v", envVars["DOCKER_HOST"], tc.expectedHost)
				}

				if envVars["REGISTRY_URL"] != "mock-registry-url:5000" {
					t.Errorf("REGISTRY_URL = %v, want mock-registry-url:5000", envVars["REGISTRY_URL"])
				}
			})
		}
	})
}

func TestDockerEnvPrinter_Print(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupSafeDockerEnvPrinterMocks()
		dockerEnvPrinter := NewDockerEnvPrinter(mocks.Injector)
		dockerEnvPrinter.Initialize()

		// Mock the Print method of BaseEnvPrinter to capture the envVars
		var capturedEnvVars map[string]string
		mocks.Shell.PrintEnvVarsFunc = func(envVars map[string]string) error {
			capturedEnvVars = envVars
			return nil
		}

		// Call Print and check for errors
		err := dockerEnvPrinter.Print()
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		// Verify that PrintEnvVarsFunc was called with the correct envVars
		expectedEnvVars, _ := dockerEnvPrinter.GetEnvVars()
		if !reflect.DeepEqual(capturedEnvVars, expectedEnvVars) {
			t.Errorf("capturedEnvVars = %v, want %v", capturedEnvVars, expectedEnvVars)
		}
	})

	t.Run("GetEnvVarsError", func(t *testing.T) {
		mocks := setupSafeDockerEnvPrinterMocks()
		mocks.ConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "vm.driver" {
				return "colima"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
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

		err := dockerEnvPrinter.Print()
		if err == nil {
			t.Error("expected an error, got nil")
		} else if !strings.Contains(err.Error(), "mock user home dir error") {
			t.Errorf("error = %v, want error containing 'mock user home dir error'", err)
		}
	})
}
