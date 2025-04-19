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

// TalosEnvMocks holds all mock objects used in Talos environment tests
type TalosEnvMocks struct {
	Injector      di.Injector
	ConfigHandler *config.MockConfigHandler
	Shell         *shell.MockShell
}

// setupSafeTalosEnvMocks creates and configures mock objects for Talos environment tests.
// It accepts an optional injector parameter and returns initialized TalosEnvMocks.
func setupSafeTalosEnvMocks(injector ...di.Injector) *TalosEnvMocks {
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

	return &TalosEnvMocks{
		Injector:      mockInjector,
		ConfigHandler: mockConfigHandler,
		Shell:         mockShell,
	}
}

// =============================================================================
// Test Public Methods
// =============================================================================

// TestTalosEnv_GetEnvVars tests the GetEnvVars method of the TalosEnvPrinter
func TestTalosEnv_GetEnvVars(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new TalosEnvPrinter with existing Talos config
		mocks := setupSafeTalosEnvMocks()

		originalStat := stat
		defer func() { stat = originalStat }()
		stat = func(name string) (os.FileInfo, error) {
			if name == filepath.FromSlash("/mock/config/root/.talos/config") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		talosEnvPrinter := NewTalosEnvPrinter(mocks.Injector)
		talosEnvPrinter.Initialize()

		// When getting environment variables
		envVars, err := talosEnvPrinter.GetEnvVars()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		// And TALOSCONFIG should be set correctly
		expectedPath := filepath.FromSlash("/mock/config/root/.talos/config")
		if envVars["TALOSCONFIG"] != expectedPath {
			t.Errorf("TALOSCONFIG = %v, want %v", envVars["TALOSCONFIG"], expectedPath)
		}
	})

	t.Run("NoTalosConfig", func(t *testing.T) {
		// Given a new TalosEnvPrinter without existing Talos config
		mocks := setupSafeTalosEnvMocks()

		originalStat := stat
		defer func() { stat = originalStat }()
		stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		talosEnvPrinter := NewTalosEnvPrinter(mocks.Injector)
		talosEnvPrinter.Initialize()

		// When getting environment variables
		envVars, err := talosEnvPrinter.GetEnvVars()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		// And TALOSCONFIG should still be set to default path
		expectedPath := filepath.FromSlash("/mock/config/root/.talos/config")
		if envVars["TALOSCONFIG"] != expectedPath {
			t.Errorf("TALOSCONFIG = %v, want %v", envVars["TALOSCONFIG"], expectedPath)
		}
	})

	t.Run("GetConfigRootError", func(t *testing.T) {
		// Given a new TalosEnvPrinter with failing config root lookup
		mocks := setupSafeTalosEnvMocks()
		mocks.ConfigHandler.GetConfigRootFunc = func() (string, error) {
			return "", errors.New("mock config error")
		}

		talosEnvPrinter := NewTalosEnvPrinter(mocks.Injector)
		talosEnvPrinter.Initialize()

		// When getting environment variables
		_, err := talosEnvPrinter.GetEnvVars()

		// Then appropriate error should be returned
		if err == nil || err.Error() != "error retrieving configuration root directory: mock config error" {
			t.Errorf("expected error retrieving configuration root directory, got %v", err)
		}
	})
}

// TestTalosEnv_Print tests the Print method of the TalosEnvPrinter
func TestTalosEnv_Print(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new TalosEnvPrinter with existing Talos config
		mocks := setupSafeTalosEnvMocks()
		talosEnvPrinter := NewTalosEnvPrinter(mocks.Injector)
		talosEnvPrinter.Initialize()

		// And Talos config file exists
		stat = func(name string) (os.FileInfo, error) {
			if name == filepath.FromSlash("/mock/config/root/.talos/config") {
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
		err := talosEnvPrinter.Print()

		// Then no error should be returned
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		// And environment variables should be set correctly
		expectedEnvVars := map[string]string{
			"TALOSCONFIG": filepath.FromSlash("/mock/config/root/.talos/config"),
		}
		if !reflect.DeepEqual(capturedEnvVars, expectedEnvVars) {
			t.Errorf("capturedEnvVars = %v, want %v", capturedEnvVars, expectedEnvVars)
		}
	})

	t.Run("GetConfigError", func(t *testing.T) {
		// Given a new TalosEnvPrinter with failing config lookup
		mocks := setupSafeTalosEnvMocks()
		mocks.ConfigHandler.GetConfigRootFunc = func() (string, error) {
			return "", errors.New("mock config error")
		}

		talosEnvPrinter := NewTalosEnvPrinter(mocks.Injector)
		talosEnvPrinter.Initialize()

		// When calling Print
		err := talosEnvPrinter.Print()

		// Then appropriate error should be returned
		if err == nil {
			t.Error("expected error, got nil")
		} else if !strings.Contains(err.Error(), "mock config error") {
			t.Errorf("unexpected error message: %v", err)
		}
	})
}
