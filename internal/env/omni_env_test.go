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

type OmniEnvPrinterMocks struct {
	Injector       di.Injector
	ContextHandler *context.MockContext
	Shell          *shell.MockShell
}

func setupSafeOmniEnvPrinterMocks(injector ...di.Injector) *OmniEnvPrinterMocks {
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

	return &OmniEnvPrinterMocks{
		Injector:       mockInjector,
		ContextHandler: mockContext,
		Shell:          mockShell,
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

		if envVars["OMNICONFIG"] != "" {
			t.Errorf("OMNICONFIG = %v, want empty", envVars["OMNICONFIG"])
		}
	})

	t.Run("GetConfigRootError", func(t *testing.T) {
		mocks := setupSafeOmniEnvPrinterMocks()
		mocks.ContextHandler.GetConfigRootFunc = func() (string, error) {
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

	t.Run("GetConfigError", func(t *testing.T) {
		// Use setupSafeOmniEnvPrinterMocks to create mocks
		mocks := setupSafeOmniEnvPrinterMocks()

		// Override the GetConfigFunc to simulate an error
		mocks.ContextHandler.GetConfigRootFunc = func() (string, error) {
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
