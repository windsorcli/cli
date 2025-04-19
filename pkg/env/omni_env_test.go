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

// =============================================================================
// Test Setup
// =============================================================================

// OmniEnvPrinterMocks holds all mock objects used in Omni environment tests
type OmniEnvPrinterMocks struct {
	Injector      di.Injector
	Shell         *shell.MockShell
	ConfigHandler *config.MockConfigHandler
}

// setupSafeOmniEnvPrinterMocks creates and configures mock objects for Omni environment tests.
// It accepts an optional injector parameter and returns initialized OmniEnvPrinterMocks.
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

// =============================================================================
// Test Public Methods
// =============================================================================

// TestOmniEnvPrinter_GetEnvVars tests the GetEnvVars method of the OmniEnvPrinter
func TestOmniEnvPrinter_GetEnvVars(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new OmniEnvPrinter with existing Omni config
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

		// When getting environment variables
		envVars, err := omniEnvPrinter.GetEnvVars()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		// And OMNICONFIG should be set correctly
		if envVars["OMNICONFIG"] != filepath.FromSlash("/mock/config/root/.omni/config") {
			t.Errorf("OMNICONFIG = %v, want %v", envVars["OMNICONFIG"], filepath.FromSlash("/mock/config/root/.omni/config"))
		}
	})

	t.Run("NoOmniConfig", func(t *testing.T) {
		// Given a new OmniEnvPrinter without existing Omni config
		mocks := setupSafeOmniEnvPrinterMocks()

		originalStat := stat
		defer func() { stat = originalStat }()
		stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		omniEnvPrinter := NewOmniEnvPrinter(mocks.Injector)
		omniEnvPrinter.Initialize()

		// When getting environment variables
		envVars, err := omniEnvPrinter.GetEnvVars()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		// And OMNICONFIG should still be set to default path
		expectedPath := filepath.FromSlash("/mock/config/root/.omni/config")
		if envVars["OMNICONFIG"] != expectedPath {
			t.Errorf("OMNICONFIG = %v, want %v", envVars["OMNICONFIG"], expectedPath)
		}
	})

	t.Run("GetConfigRootError", func(t *testing.T) {
		// Given a new OmniEnvPrinter with failing config root lookup
		mocks := setupSafeOmniEnvPrinterMocks()
		mocks.ConfigHandler.GetConfigRootFunc = func() (string, error) {
			return "", errors.New("mock context error")
		}

		omniEnvPrinter := NewOmniEnvPrinter(mocks.Injector)
		omniEnvPrinter.Initialize()

		// When getting environment variables
		_, err := omniEnvPrinter.GetEnvVars()

		// Then appropriate error should be returned
		expectedError := "error retrieving configuration root directory: mock context error"
		if err == nil || err.Error() != expectedError {
			t.Errorf("error = %v, want %v", err, expectedError)
		}
	})
}

// TestOmniEnvPrinter_Print tests the Print method of the OmniEnvPrinter
func TestOmniEnvPrinter_Print(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new OmniEnvPrinter with existing Omni config
		mocks := setupSafeOmniEnvPrinterMocks()
		omniEnvPrinter := NewOmniEnvPrinter(mocks.Injector)
		omniEnvPrinter.Initialize()

		// And Omni config file exists
		stat = func(name string) (os.FileInfo, error) {
			if name == filepath.FromSlash("/mock/config/root/.omni/config") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		// And PrintEnvVarsFunc is mocked
		var capturedEnvVars map[string]string
		mocks.Shell.PrintEnvVarsFunc = func(envVars map[string]string) {
			capturedEnvVars = envVars
		}

		// When calling Print
		err := omniEnvPrinter.Print()

		// Then no error should be returned
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		// And environment variables should be set correctly
		expectedEnvVars := map[string]string{
			"OMNICONFIG": filepath.FromSlash("/mock/config/root/.omni/config"),
		}
		if !reflect.DeepEqual(capturedEnvVars, expectedEnvVars) {
			t.Errorf("capturedEnvVars = %v, want %v", capturedEnvVars, expectedEnvVars)
		}
	})

	t.Run("GetConfigError", func(t *testing.T) {
		// Given a new OmniEnvPrinter with failing config lookup
		mocks := setupSafeOmniEnvPrinterMocks()
		mocks.ConfigHandler.GetConfigRootFunc = func() (string, error) {
			return "", errors.New("mock config error")
		}

		omniEnvPrinter := NewOmniEnvPrinter(mocks.Injector)
		omniEnvPrinter.Initialize()

		// When calling Print
		err := omniEnvPrinter.Print()

		// Then appropriate error should be returned
		if err == nil {
			t.Error("expected error, got nil")
		} else if !strings.Contains(err.Error(), "mock config error") {
			t.Errorf("unexpected error message: %v", err)
		}
	})
}
