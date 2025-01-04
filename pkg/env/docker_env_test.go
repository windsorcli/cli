package env

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/context"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
)

type DockerEnvPrinterMocks struct {
	Injector       di.Injector
	ContextHandler *context.MockContext
	Shell          *shell.MockShell
	ConfigHandler  *config.MockConfigHandler
}

func setupSafeDockerEnvPrinterMocks(injector ...di.Injector) *DockerEnvPrinterMocks {
	var mockInjector di.Injector
	if len(injector) > 0 {
		mockInjector = injector[0]
	} else {
		mockInjector = di.NewMockInjector()
	}

	mockContext := context.NewMockContext()
	mockContext.GetConfigRootFunc = func() (string, error) {
		return filepath.FromSlash("/mock/config/root"), nil
	}

	mockShell := shell.NewMockShell()

	mockConfigHandler := config.NewMockConfigHandler()
	mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
		return "mock-value"
	}

	mockInjector.Register("contextHandler", mockContext)
	mockInjector.Register("shell", mockShell)
	mockInjector.Register("configHandler", mockConfigHandler)

	return &DockerEnvPrinterMocks{
		Injector:       mockInjector,
		ContextHandler: mockContext,
		Shell:          mockShell,
		ConfigHandler:  mockConfigHandler,
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

	// t.Run("ColimaDriver", func(t *testing.T) {
	// 	mocks := setupSafeDockerEnvPrinterMocks()
	// 	mocks.ContextHandler.GetContextFunc = func() string {
	// 		return "test-context"
	// 	}

	// 	originalUserHomeDir := osUserHomeDir
	// 	defer func() { osUserHomeDir = originalUserHomeDir }()
	// 	osUserHomeDir = func() (string, error) {
	// 		return "/mock/home", nil
	// 	}

	// 	mockConfigHandler := config.NewMockConfigHandler()
	// 	mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
	// 		if key == "vm.driver" {
	// 			return "colima"
	// 		}
	// 		return ""
	// 	}
	// 	mocks.Injector.Register("configHandler", mockConfigHandler)

	// 	dockerEnvPrinter := NewDockerEnvPrinter(mocks.Injector)
	// 	dockerEnvPrinter.Initialize()

	// 	envVars, err := dockerEnvPrinter.GetEnvVars()
	// 	if err != nil {
	// 		t.Fatalf("GetEnvVars returned an error: %v", err)
	// 	}

	// 	expectedDockerHost := fmt.Sprintf("unix://%s/.colima/windsor-%s/docker.sock", "/mock/home", "test-context")
	// 	if envVars["DOCKER_HOST"] != expectedDockerHost {
	// 		t.Errorf("DOCKER_HOST = %v, want %v", envVars["DOCKER_HOST"], expectedDockerHost)
	// 	}
	// })

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
}

func TestDockerEnvPrinter_Print(t *testing.T) {
	// t.Run("Success", func(t *testing.T) {
	// 	// Use setupSafeAwsEnvMocks to create mocks
	// 	mocks := setupSafeDockerEnvPrinterMocks()
	// 	mockInjector := mocks.Injector
	// 	dockerEnvPrinter := NewDockerEnvPrinter(mockInjector)
	// 	dockerEnvPrinter.Initialize()

	// 	// Mock the PrintEnvVarsFunc to verify it is called with the correct envVars
	// 	var capturedEnvVars map[string]string
	// 	mocks.Shell.PrintEnvVarsFunc = func(envVars map[string]string) error {
	// 		capturedEnvVars = envVars
	// 		return nil
	// 	}

	// 	// Call Print and check for errors
	// 	err := dockerEnvPrinter.Print()
	// 	if err != nil {
	// 		t.Errorf("unexpected error: %v", err)
	// 	}

	// 	// Verify that PrintEnvVarsFunc was called with the correct envVars
	// 	expectedEnvVars := map[string]string{}
	// 	if !reflect.DeepEqual(capturedEnvVars, expectedEnvVars) {
	// 		t.Errorf("capturedEnvVars = %v, want %v", capturedEnvVars, expectedEnvVars)
	// 	}
	// })

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
