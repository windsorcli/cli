package env

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// =============================================================================
// Test Public Methods
// =============================================================================

// TestOmniEnvPrinter_GetEnvVars tests the GetEnvVars method of the OmniEnvPrinter
func TestOmniEnvPrinter_GetEnvVars(t *testing.T) {
	setup := func(t *testing.T) (*OmniEnvPrinter, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		printer := NewOmniEnvPrinter(mocks.Injector)
		if err := printer.Initialize(); err != nil {
			t.Fatalf("Failed to initialize env: %v", err)
		}
		printer.shims = mocks.Shims
		return printer, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a new OmniEnvPrinter with existing Omni config
		printer, mocks := setup(t)

		// Get the actual project root path
		projectRoot, err := mocks.Shell.GetProjectRoot()
		if err != nil {
			t.Fatalf("Failed to get project root: %v", err)
		}
		expectedPath := filepath.Join(projectRoot, "contexts", "mock-context", ".omni", "config")

		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if name == expectedPath {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		// When getting environment variables
		envVars, err := printer.GetEnvVars()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		// And OMNICONFIG should be set correctly
		if envVars["OMNICONFIG"] != expectedPath {
			t.Errorf("OMNICONFIG = %v, want %v", envVars["OMNICONFIG"], expectedPath)
		}
	})

	t.Run("NoOmniConfig", func(t *testing.T) {
		// Given a new OmniEnvPrinter without existing Omni config
		printer, mocks := setup(t)

		// Get the actual project root path
		projectRoot, err := mocks.Shell.GetProjectRoot()
		if err != nil {
			t.Fatalf("Failed to get project root: %v", err)
		}
		expectedPath := filepath.Join(projectRoot, "contexts", "mock-context", ".omni", "config")
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		// When getting environment variables
		envVars, err := printer.GetEnvVars()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		// And OMNICONFIG should still be set to default path
		if envVars["OMNICONFIG"] != expectedPath {
			t.Errorf("OMNICONFIG = %v, want %v", envVars["OMNICONFIG"], expectedPath)
		}
	})

	t.Run("GetProjectRootError", func(t *testing.T) {
		// Given a new OmniEnvPrinter with failing project root lookup
		printer, mocks := setup(t)
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "", errors.New("mock context error")
		}

		// When getting environment variables
		_, err := printer.GetEnvVars()

		// Then appropriate error should be returned
		expectedError := "error retrieving configuration root directory: mock context error"
		if err == nil || err.Error() != expectedError {
			t.Errorf("error = %v, want %v", err, expectedError)
		}
	})
}

// TestOmniEnvPrinter_Print tests the Print method of the OmniEnvPrinter
func TestOmniEnvPrinter_Print(t *testing.T) {
	setup := func(t *testing.T) (*OmniEnvPrinter, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		printer := NewOmniEnvPrinter(mocks.Injector)
		if err := printer.Initialize(); err != nil {
			t.Fatalf("Failed to initialize env: %v", err)
		}
		printer.shims = mocks.Shims
		return printer, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a new OmniEnvPrinter with existing Omni config
		printer, mocks := setup(t)

		// Get the actual project root path
		projectRoot, err := mocks.Shell.GetProjectRoot()
		if err != nil {
			t.Fatalf("Failed to get project root: %v", err)
		}
		expectedPath := filepath.Join(projectRoot, "contexts", "mock-context", ".omni", "config")

		// And Omni config file exists
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if name == expectedPath {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		// And PrintEnvVarsFunc is mocked
		var capturedEnvVars map[string]string
		mocks.Shell.PrintEnvVarsFunc = func(envVars map[string]string, export bool) {
			capturedEnvVars = envVars
		}

		// When calling Print
		err = printer.Print()

		// Then no error should be returned
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		// And environment variables should be set correctly
		expectedEnvVars := map[string]string{
			"OMNICONFIG": expectedPath,
		}
		if !reflect.DeepEqual(capturedEnvVars, expectedEnvVars) {
			t.Errorf("capturedEnvVars = %v, want %v", capturedEnvVars, expectedEnvVars)
		}
	})

	t.Run("GetProjectRootError", func(t *testing.T) {
		// Given a new OmniEnvPrinter with failing project root lookup
		printer, mocks := setup(t)
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "", errors.New("mock config error")
		}

		// When calling Print
		err := printer.Print()

		// Then appropriate error should be returned
		if err == nil {
			t.Error("expected error, got nil")
		} else if !strings.Contains(err.Error(), "mock config error") {
			t.Errorf("unexpected error message: %v", err)
		}
	})
}
