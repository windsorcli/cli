package env

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
)

type OmniEnvPrinterMocks struct {
	Injector      di.Injector
	Shell         *shell.MockShell
	ConfigHandler *config.MockConfigHandler
}

func setupSafeOmniEnvPrinterMocks(injector ...di.Injector) *OmniEnvPrinterMocks {
	var mockInjector di.Injector
	if len(injector) > 0 {
		mockInjector = injector[0]
	} else {
		mockInjector = di.NewMockInjector()
	}

	mockConfigHandler := config.NewMockConfigHandler()
	mockConfigHandler.GetConfigRootFunc = func() (string, error) {
		return filepath.FromSlash("/mock/config/root"), nil
	}

	mockShell := shell.NewMockShell()

	mockInjector.Register("configHandler", mockConfigHandler)
	mockInjector.Register("shell", mockShell)

	return &OmniEnvPrinterMocks{
		Injector:      mockInjector,
		ConfigHandler: mockConfigHandler,
		Shell:         mockShell,
	}
}

func TestOmniEnvPrinter_GetEnvVars(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupSafeOmniEnvPrinterMocks()

		originalStat := stat
		defer func() { stat = originalStat }()
		stat = func(name string) (os.FileInfo, error) {
			if name == filepath.FromSlash("/mock/config/root/.omni/config") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		omniEnvPrinter := NewOmniEnvPrinter(mocks.Injector)
		omniEnvPrinter.Initialize()

		envVars, err := omniEnvPrinter.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		if envVars["OMNICONFIG"] != filepath.FromSlash("/mock/config/root/.omni/config") {
			t.Errorf("OMNICONFIG = %v, want %v", envVars["OMNICONFIG"], filepath.FromSlash("/mock/config/root/.omni/config"))
		}
	})

	t.Run("NoOmniConfig", func(t *testing.T) {
		mocks := setupSafeOmniEnvPrinterMocks()

		originalStat := stat
		defer func() { stat = originalStat }()
		stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		omniEnvPrinter := NewOmniEnvPrinter(mocks.Injector)
		omniEnvPrinter.Initialize()

		envVars, err := omniEnvPrinter.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		expectedPath := filepath.FromSlash("/mock/config/root/.omni/config")
		if envVars["OMNICONFIG"] != expectedPath {
			t.Errorf("OMNICONFIG = %v, want %v", envVars["OMNICONFIG"], expectedPath)
		}
	})

	t.Run("GetConfigRootError", func(t *testing.T) {
		mocks := setupSafeOmniEnvPrinterMocks()
		mocks.ConfigHandler.GetConfigRootFunc = func() (string, error) {
			return "", errors.New("mock context error")
		}

		omniEnvPrinter := NewOmniEnvPrinter(mocks.Injector)
		omniEnvPrinter.Initialize()

		_, err := omniEnvPrinter.GetEnvVars()
		expectedError := "error retrieving configuration root directory: mock context error"
		if err == nil || err.Error() != expectedError {
			t.Errorf("error = %v, want %v", err, expectedError)
		}
	})
}

func TestOmniEnvPrinter_Print(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Use setupSafeOmniEnvPrinterMocks to create mocks
		mocks := setupSafeOmniEnvPrinterMocks()
		mockInjector := mocks.Injector
		omniEnvPrinter := NewOmniEnvPrinter(mockInjector)
		omniEnvPrinter.Initialize()

		// Mock the stat function to simulate the existence of the omniconfig file
		stat = func(name string) (os.FileInfo, error) {
			if name == filepath.FromSlash("/mock/config/root/.omni/config") {
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
		err := omniEnvPrinter.Print()
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		// Verify that PrintEnvVarsFunc was called with the correct envVars
		expectedEnvVars := map[string]string{
			"OMNICONFIG": filepath.FromSlash("/mock/config/root/.omni/config"),
		}
		if !reflect.DeepEqual(capturedEnvVars, expectedEnvVars) {
			t.Errorf("capturedEnvVars = %v, want %v", capturedEnvVars, expectedEnvVars)
		}
	})

	t.Run("WithCustomVars", func(t *testing.T) {
		// Use setupSafeOmniEnvPrinterMocks to create mocks
		mocks := setupSafeOmniEnvPrinterMocks()
		mockInjector := mocks.Injector
		omniEnvPrinter := NewOmniEnvPrinter(mockInjector)
		omniEnvPrinter.Initialize()

		// Define custom variables
		customVars := map[string]string{
			"CUSTOM_OMNI_VAR1": "custom-value1",
			"CUSTOM_OMNI_VAR2": "custom-value2",
		}

		// Mock the stat function to simulate the existence of the omniconfig file
		stat = func(name string) (os.FileInfo, error) {
			if name == filepath.FromSlash("/mock/config/root/.omni/config") {
				return nil, nil // Simulate that the file exists
			}
			return nil, os.ErrNotExist
		}

		// Mock the PrintEnvVarsFunc to verify it is called with the merged vars
		var capturedEnvVars map[string]string
		mocks.Shell.PrintEnvVarsFunc = func(envVars map[string]string) error {
			capturedEnvVars = envVars
			return nil
		}

		// Call Print with custom vars and check for errors
		err := omniEnvPrinter.Print(customVars)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		// Verify that the customVars were merged with the environment vars
		for key, value := range customVars {
			if capturedEnvVars[key] != value {
				t.Errorf("capturedEnvVars[%s] = %v, want %v", key, capturedEnvVars[key], value)
			}
		}

		// Verify that default environment variables are still present
		if _, exists := capturedEnvVars["OMNICONFIG"]; !exists {
			t.Errorf("expected OMNICONFIG to exist in capturedEnvVars")
		}
	})

	t.Run("GetConfigError", func(t *testing.T) {
		// Use setupSafeOmniEnvPrinterMocks to create mocks
		mocks := setupSafeOmniEnvPrinterMocks()

		// Override the GetConfigFunc to simulate an error
		mocks.ConfigHandler.GetConfigRootFunc = func() (string, error) {
			return "", errors.New("mock config error")
		}

		mockInjector := mocks.Injector

		omniEnvPrinter := NewOmniEnvPrinter(mockInjector)
		omniEnvPrinter.Initialize()

		// Call Print and check for errors
		err := omniEnvPrinter.Print()
		if err == nil {
			t.Error("expected error, got nil")
		} else if !strings.Contains(err.Error(), "mock config error") {
			t.Errorf("unexpected error message: %v", err)
		}
	})
}
