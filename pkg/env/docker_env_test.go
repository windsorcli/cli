package env

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
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
		mockInjector = di.NewMockInjector()
	}

	mockShell := shell.NewMockShell()

	mockConfigHandler := config.NewMockConfigHandler()
	mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
		return "mock-value"
	}
	mockConfigHandler.GetConfigRootFunc = func() (string, error) {
		return filepath.FromSlash("/mock/config/root"), nil
	}
	mockConfigHandler.GetContextFunc = func() string {
		return "mock-context"
	}

	mockInjector.Register("shell", mockShell)
	mockInjector.Register("configHandler", mockConfigHandler)

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

		if envVars["DOCKER_HOST"] != "" {
			t.Errorf("DOCKER_HOST = %v, want empty", envVars["DOCKER_HOST"])
		}
	})

	t.Run("ColimaDriver", func(t *testing.T) {
		mocks := setupSafeDockerEnvPrinterMocks()
		mocks.ConfigHandler.GetContextFunc = func() string {
			return "test-context"
		}

		// Mock osUserHomeDir function
		originalUserHomeDir := osUserHomeDir
		defer func() { osUserHomeDir = originalUserHomeDir }()
		osUserHomeDir = func() (string, error) {
			return "/mock/home", nil
		}

		// Mock mkdirAll function
		originalMkdirAll := mkdirAll
		defer func() { mkdirAll = originalMkdirAll }()
		mkdirAllCalled := false
		mkdirAll = func(path string, perm os.FileMode) error {
			mkdirAllCalled = true
			return nil
		}

		// Mock writeFile function
		originalWriteFile := writeFile
		defer func() { writeFile = originalWriteFile }()
		writeFileCalled := false
		writeFile = func(filename string, data []byte, perm os.FileMode) error {
			writeFileCalled = true
			return nil
		}

		// Use the existing mockConfigHandler from mocks
		mocks.ConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "vm.driver" {
				return "colima"
			}
			return ""
		}

		dockerEnvPrinter := NewDockerEnvPrinter(mocks.Injector)
		dockerEnvPrinter.Initialize()

		envVars, err := dockerEnvPrinter.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		expectedDockerHost := fmt.Sprintf("unix://%s/.colima/windsor-%s/docker.sock", "/mock/home", "test-context")
		if envVars["DOCKER_HOST"] != expectedDockerHost {
			t.Errorf("DOCKER_HOST = %v, want %v", envVars["DOCKER_HOST"], expectedDockerHost)
		}

		if !mkdirAllCalled {
			t.Error("mkdirAll was not called")
		}

		if !writeFileCalled {
			t.Error("writeFile was not called")
		}
	})

	t.Run("DockerDesktopDriver", func(t *testing.T) {
		mocks := setupSafeDockerEnvPrinterMocks()

		// Mock osUserHomeDir function
		originalUserHomeDir := osUserHomeDir
		defer func() { osUserHomeDir = originalUserHomeDir }()
		osUserHomeDir = func() (string, error) {
			return "/mock/home", nil
		}

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
		mkdirAll = func(path string, perm os.FileMode) error {
			mkdirAllCalled = true
			return nil
		}

		// Mock writeFile function
		originalWriteFile := writeFile
		defer func() { writeFile = originalWriteFile }()
		writeFileCalled := false
		writeFile = func(filename string, data []byte, perm os.FileMode) error {
			writeFileCalled = true
			return nil
		}

		// Use the existing mockConfigHandler from mocks
		mocks.ConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "vm.driver" {
				return "docker-desktop"
			}
			return ""
		}

		dockerEnvPrinter := NewDockerEnvPrinter(mocks.Injector)
		dockerEnvPrinter.Initialize()

		envVars, err := dockerEnvPrinter.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		expectedDockerHost := fmt.Sprintf("unix://%s/.docker/run/docker.sock", "/mock/home")
		if envVars["DOCKER_HOST"] != expectedDockerHost {
			t.Errorf("DOCKER_HOST = %v, want %v", envVars["DOCKER_HOST"], expectedDockerHost)
		}

		if !mkdirAllCalled {
			t.Error("mkdirAll was not called")
		}

		if !writeFileCalled {
			t.Error("writeFile was not called")
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
					if key == "vm.driver" {
						return "docker-desktop"
					}
					return ""
				}

				originalGoos := goos
				defer func() { goos = originalGoos }()
				goos = func() string {
					return tc.osName
				}

				originalUserHomeDir := osUserHomeDir
				defer func() { osUserHomeDir = originalUserHomeDir }()
				osUserHomeDir = func() (string, error) {
					return "/home/user", nil
				}

				originalMkdirAll := mkdirAll
				defer func() { mkdirAll = originalMkdirAll }()
				mkdirAll = func(path string, perm os.FileMode) error {
					return nil
				}

				originalWriteFile := writeFile
				defer func() { writeFile = originalWriteFile }()
				writeFile = func(filename string, data []byte, perm os.FileMode) error {
					return nil
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
