package env

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/context"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
)

type DockerEnvPrinterMocks struct {
	Injector       di.Injector
	ContextHandler *context.MockContext
	Shell          *shell.MockShell
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

	mockInjector.Register("contextHandler", mockContext)
	mockInjector.Register("shell", mockShell)

	return &DockerEnvPrinterMocks{
		Injector:       mockInjector,
		ContextHandler: mockContext,
		Shell:          mockShell,
	}
}

func TestDockerEnvPrinter_GetEnvVars(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupSafeDockerEnvPrinterMocks()

		originalStat := stat
		defer func() { stat = originalStat }()
		stat = func(name string) (os.FileInfo, error) {
			if name == filepath.FromSlash("/mock/config/root/compose.yaml") || name == filepath.FromSlash("/mock/config/root/compose.yml") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		dockerEnvPrinter := NewDockerEnvPrinter(mocks.Injector)
		dockerEnvPrinter.Initialize()

		envVars, err := dockerEnvPrinter.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		if envVars["COMPOSE_FILE"] != filepath.FromSlash("/mock/config/root/compose.yaml") && envVars["COMPOSE_FILE"] != filepath.FromSlash("/mock/config/root/compose.yml") {
			t.Errorf("COMPOSE_FILE = %v, want %v or %v", envVars["COMPOSE_FILE"], filepath.FromSlash("/mock/config/root/compose.yaml"), filepath.FromSlash("/mock/config/root/compose.yml"))
		}
	})

	t.Run("NoComposeFile", func(t *testing.T) {
		mocks := setupSafeDockerEnvPrinterMocks()

		originalStat := stat
		defer func() { stat = originalStat }()
		stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		dockerEnvPrinter := NewDockerEnvPrinter(mocks.Injector)
		dockerEnvPrinter.Initialize()

		envVars, err := dockerEnvPrinter.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		if envVars["COMPOSE_FILE"] != "" {
			t.Errorf("COMPOSE_FILE = %v, want empty", envVars["COMPOSE_FILE"])
		}
	})

	t.Run("GetConfigRootError", func(t *testing.T) {
		mocks := setupSafeDockerEnvPrinterMocks()
		mocks.ContextHandler.GetConfigRootFunc = func() (string, error) {
			return "", errors.New("mock context error")
		}

		dockerEnvPrinter := NewDockerEnvPrinter(mocks.Injector)
		dockerEnvPrinter.Initialize()

		_, err := dockerEnvPrinter.GetEnvVars()
		expectedError := "error retrieving configuration root directory: mock context error"
		if err == nil || err.Error() != expectedError {
			t.Errorf("error = %v, want %v", err, expectedError)
		}
	})

	t.Run("YmlFileExists", func(t *testing.T) {
		mocks := setupSafeDockerEnvPrinterMocks()

		originalStat := stat
		defer func() { stat = originalStat }()
		stat = func(name string) (os.FileInfo, error) {
			if name == filepath.FromSlash("/mock/config/root/compose.yml") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		dockerEnvPrinter := NewDockerEnvPrinter(mocks.Injector)
		dockerEnvPrinter.Initialize()

		envVars, err := dockerEnvPrinter.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		if envVars["COMPOSE_FILE"] != filepath.FromSlash("/mock/config/root/compose.yml") {
			t.Errorf("COMPOSE_FILE = %v, want %v", envVars["COMPOSE_FILE"], filepath.FromSlash("/mock/config/root/compose.yml"))
		}
	})
}

func TestDockerEnvPrinter_Print(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Use setupSafeAwsEnvMocks to create mocks
		mocks := setupSafeDockerEnvPrinterMocks()
		mockInjector := mocks.Injector
		dockerEnvPrinter := NewDockerEnvPrinter(mockInjector)
		dockerEnvPrinter.Initialize()

		// Mock the stat function to simulate the existence of the Docker compose file
		stat = func(name string) (os.FileInfo, error) {
			if name == filepath.FromSlash("/mock/config/root/compose.yaml") || name == filepath.FromSlash("/mock/config/root/compose.yml") {
				return nil, nil // Simulate that the file exists
			}
			return nil, os.ErrNotExist
		}

		// Mock the PrintEnvVarsFunc to verify it is called with the correct envVars
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
		expectedEnvVars := map[string]string{
			"COMPOSE_FILE": filepath.FromSlash("/mock/config/root/compose.yaml"),
		}
		if !reflect.DeepEqual(capturedEnvVars, expectedEnvVars) {
			t.Errorf("capturedEnvVars = %v, want %v", capturedEnvVars, expectedEnvVars)
		}
	})

	t.Run("GetConfigError", func(t *testing.T) {
		// Use setupSafeAwsEnvMocks to create mocks
		mocks := setupSafeDockerEnvPrinterMocks()

		// Override the GetConfigFunc to simulate an error
		mocks.ContextHandler.GetConfigRootFunc = func() (string, error) {
			return "", errors.New("mock config error")
		}

		mockInjector := mocks.Injector

		dockerEnvPrinter := NewDockerEnvPrinter(mockInjector)
		dockerEnvPrinter.Initialize()

		// Call Print and check for errors
		err := dockerEnvPrinter.Print()
		if err == nil {
			t.Error("expected error, got nil")
		} else if !strings.Contains(err.Error(), "mock config error") {
			t.Errorf("unexpected error message: %v", err)
		}
	})
}
