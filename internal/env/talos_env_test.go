package env

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/windsorcli/cli/internal/context"
	"github.com/windsorcli/cli/internal/di"
	"github.com/windsorcli/cli/internal/shell"
)

type TalosEnvMocks struct {
	Injector       di.Injector
	ContextHandler *context.MockContext
	Shell          *shell.MockShell
}

func setupSafeTalosEnvMocks(injector ...di.Injector) *TalosEnvMocks {
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

	return &TalosEnvMocks{
		Injector:       mockInjector,
		ContextHandler: mockContext,
		Shell:          mockShell,
	}
}

func TestTalosEnv_GetEnvVars(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
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

		envVars, err := talosEnvPrinter.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		expectedPath := filepath.FromSlash("/mock/config/root/.talos/config")
		if envVars["TALOSCONFIG"] != expectedPath {
			t.Errorf("TALOSCONFIG = %v, want %v", envVars["TALOSCONFIG"], expectedPath)
		}
	})

	t.Run("NoTalosConfig", func(t *testing.T) {
		mocks := setupSafeTalosEnvMocks()

		originalStat := stat
		defer func() { stat = originalStat }()
		stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		talosEnvPrinter := NewTalosEnvPrinter(mocks.Injector)
		talosEnvPrinter.Initialize()

		envVars, err := talosEnvPrinter.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		if envVars["TALOSCONFIG"] != "" {
			t.Errorf("TALOSCONFIG = %v, want empty", envVars["TALOSCONFIG"])
		}
	})

	t.Run("GetConfigRootError", func(t *testing.T) {
		mocks := setupSafeTalosEnvMocks()
		mocks.ContextHandler.GetConfigRootFunc = func() (string, error) {
			return "", errors.New("mock context error")
		}

		talosEnvPrinter := NewTalosEnvPrinter(mocks.Injector)
		talosEnvPrinter.Initialize()

		_, err := talosEnvPrinter.GetEnvVars()
		if err == nil || err.Error() != "error retrieving configuration root directory: mock context error" {
			t.Errorf("expected error retrieving configuration root directory, got %v", err)
		}
	})
}

func TestTalosEnv_Print(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Use setupSafeTalosEnvMocks to create mocks
		mocks := setupSafeTalosEnvMocks()
		mockInjector := mocks.Injector
		talosEnvPrinter := NewTalosEnvPrinter(mockInjector)
		talosEnvPrinter.Initialize()
		talosEnvPrinter.Initialize()

		// Mock the stat function to simulate the existence of the talos config file
		stat = func(name string) (os.FileInfo, error) {
			if name == filepath.FromSlash("/mock/config/root/.talos/config") {
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
		err := talosEnvPrinter.Print()
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		// Verify that PrintEnvVarsFunc was called with the correct envVars
		expectedEnvVars := map[string]string{
			"TALOSCONFIG": filepath.FromSlash("/mock/config/root/.talos/config"),
		}
		if !reflect.DeepEqual(capturedEnvVars, expectedEnvVars) {
			t.Errorf("capturedEnvVars = %v, want %v", capturedEnvVars, expectedEnvVars)
		}
	})

	t.Run("GetConfigError", func(t *testing.T) {
		// Use setupSafeTalosEnvMocks to create mocks
		mocks := setupSafeTalosEnvMocks()

		// Override the GetConfigFunc to simulate an error
		mocks.ContextHandler.GetConfigRootFunc = func() (string, error) {
			return "", errors.New("mock config error")
		}

		mockInjector := mocks.Injector

		talosEnvPrinter := NewTalosEnvPrinter(mockInjector)
		talosEnvPrinter.Initialize()

		// Call Print and check for errors
		err := talosEnvPrinter.Print()
		if err == nil {
			t.Error("expected error, got nil")
		} else if !strings.Contains(err.Error(), "mock config error") {
			t.Errorf("unexpected error message: %v", err)
		}
	})
}
