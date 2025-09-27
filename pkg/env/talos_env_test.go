package env

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/config"
)

// =============================================================================
// Test Public Methods
// =============================================================================

// TestTalosEnv_GetEnvVars tests the GetEnvVars method of the TalosEnvPrinter
func TestTalosEnv_GetEnvVars(t *testing.T) {
	setup := func(t *testing.T, provider string) (*TalosEnvPrinter, *Mocks) {
		t.Helper()

		// Create a mock config handler
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "provider" {
				return provider
			}
			return ""
		}

		mocks := setupMocks(t, &SetupOptions{
			ConfigHandler: mockConfigHandler,
		})

		// Set up GetConfigRoot to return the correct path
		mockConfigHandler.GetConfigRootFunc = func() (string, error) {
			projectRoot, err := mocks.Shell.GetProjectRoot()
			if err != nil {
				return "", err
			}
			return filepath.Join(projectRoot, "contexts", "mock-context"), nil
		}

		printer := NewTalosEnvPrinter(mocks.Injector)
		if err := printer.Initialize(); err != nil {
			t.Fatalf("Failed to initialize env: %v", err)
		}
		printer.shims = mocks.Shims

		return printer, mocks
	}

	t.Run("GenericProvider", func(t *testing.T) {
		// Given a new TalosOmniEnvPrinter with generic provider
		printer, mocks := setup(t, "generic")

		// Get the project root path
		projectRoot, err := mocks.Shell.GetProjectRoot()
		if err != nil {
			t.Fatalf("Failed to get project root: %v", err)
		}
		expectedTalosPath := filepath.Join(projectRoot, "contexts", "mock-context", ".talos", "config")

		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if name == expectedTalosPath {
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

		// And TALOSCONFIG should be set correctly
		if envVars["TALOSCONFIG"] != expectedTalosPath {
			t.Errorf("TALOSCONFIG = %v, want %v", envVars["TALOSCONFIG"], expectedTalosPath)
		}

		// And OMNICONFIG should not be set for generic provider
		if _, exists := envVars["OMNICONFIG"]; exists {
			t.Error("OMNICONFIG should not be set for generic provider")
		}
	})

	t.Run("OmniProvider", func(t *testing.T) {
		// Given a new TalosOmniEnvPrinter with omni provider
		printer, mocks := setup(t, "omni")

		// Get the project root path
		projectRoot, err := mocks.Shell.GetProjectRoot()
		if err != nil {
			t.Fatalf("Failed to get project root: %v", err)
		}
		expectedTalosPath := filepath.Join(projectRoot, "contexts", "mock-context", ".talos", "config")
		expectedOmniPath := filepath.Join(projectRoot, "contexts", "mock-context", ".omni", "config")

		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if name == expectedTalosPath || name == expectedOmniPath {
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

		// And TALOSCONFIG should be set correctly
		if envVars["TALOSCONFIG"] != expectedTalosPath {
			t.Errorf("TALOSCONFIG = %v, want %v", envVars["TALOSCONFIG"], expectedTalosPath)
		}

		// And OMNICONFIG should be set correctly for omni provider
		if envVars["OMNICONFIG"] != expectedOmniPath {
			t.Errorf("OMNICONFIG = %v, want %v", envVars["OMNICONFIG"], expectedOmniPath)
		}
	})

	t.Run("NoConfigFiles", func(t *testing.T) {
		// Given a new TalosOmniEnvPrinter without existing config files
		printer, mocks := setup(t, "generic")

		// Get the project root path
		projectRoot, err := mocks.Shell.GetProjectRoot()
		if err != nil {
			t.Fatalf("Failed to get project root: %v", err)
		}
		expectedPath := filepath.Join(projectRoot, "contexts", "mock-context", ".talos", "config")

		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		// When getting environment variables
		envVars, err := printer.GetEnvVars()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		// And TALOSCONFIG should still be set to default path
		if envVars["TALOSCONFIG"] != expectedPath {
			t.Errorf("TALOSCONFIG = %v, want %v", envVars["TALOSCONFIG"], expectedPath)
		}
	})

	t.Run("GetProjectRootError", func(t *testing.T) {
		// Given a new TalosOmniEnvPrinter with failing config root lookup
		printer, _ := setup(t, "generic")

		// Override the GetConfigRoot to return an error
		printer.configHandler.(*config.MockConfigHandler).GetConfigRootFunc = func() (string, error) {
			return "", errors.New("mock project root error")
		}

		// When getting environment variables
		_, err := printer.GetEnvVars()

		// Then appropriate error should be returned
		expectedError := "error retrieving configuration root directory: mock project root error"
		if err == nil || err.Error() != expectedError {
			t.Errorf("error = %v, want %v", err, expectedError)
		}
	})
}

// TestTalosEnv_Print tests the Print method of the TalosEnvPrinter
func TestTalosEnv_Print(t *testing.T) {
	setup := func(t *testing.T, provider string) (*TalosEnvPrinter, *Mocks) {
		t.Helper()

		// Create a mock config handler
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "provider" {
				return provider
			}
			return ""
		}

		mocks := setupMocks(t, &SetupOptions{
			ConfigHandler: mockConfigHandler,
		})

		// Set up GetConfigRoot to return the correct path
		mockConfigHandler.GetConfigRootFunc = func() (string, error) {
			projectRoot, err := mocks.Shell.GetProjectRoot()
			if err != nil {
				return "", err
			}
			return filepath.Join(projectRoot, "contexts", "mock-context"), nil
		}

		printer := NewTalosEnvPrinter(mocks.Injector)
		if err := printer.Initialize(); err != nil {
			t.Fatalf("Failed to initialize env: %v", err)
		}
		printer.shims = mocks.Shims

		return printer, mocks
	}

	t.Run("GenericProviderSuccess", func(t *testing.T) {
		// Given a new TalosOmniEnvPrinter with generic provider and existing Talos config
		printer, mocks := setup(t, "generic")

		// Get the project root path
		projectRoot, err := mocks.Shell.GetProjectRoot()
		if err != nil {
			t.Fatalf("Failed to get project root: %v", err)
		}
		expectedPath := filepath.Join(projectRoot, "contexts", "mock-context", ".talos", "config")

		// And Talos config file exists
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
			"TALOSCONFIG": expectedPath,
		}
		if !reflect.DeepEqual(capturedEnvVars, expectedEnvVars) {
			t.Errorf("capturedEnvVars = %v, want %v", capturedEnvVars, expectedEnvVars)
		}
	})

	t.Run("OmniProviderSuccess", func(t *testing.T) {
		// Given a new TalosOmniEnvPrinter with omni provider and existing config files
		printer, mocks := setup(t, "omni")

		// Get the project root path
		projectRoot, err := mocks.Shell.GetProjectRoot()
		if err != nil {
			t.Fatalf("Failed to get project root: %v", err)
		}
		expectedTalosPath := filepath.Join(projectRoot, "contexts", "mock-context", ".talos", "config")
		expectedOmniPath := filepath.Join(projectRoot, "contexts", "mock-context", ".omni", "config")

		// And config files exist
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if name == expectedTalosPath || name == expectedOmniPath {
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
			"TALOSCONFIG": expectedTalosPath,
			"OMNICONFIG":  expectedOmniPath,
		}
		if !reflect.DeepEqual(capturedEnvVars, expectedEnvVars) {
			t.Errorf("capturedEnvVars = %v, want %v", capturedEnvVars, expectedEnvVars)
		}
	})

	t.Run("GetProjectRootError", func(t *testing.T) {
		// Given a new TalosOmniEnvPrinter with failing project root lookup
		printer, mocks := setup(t, "generic")
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "", errors.New("mock project root error")
		}

		// When calling Print
		err := printer.Print()

		// Then appropriate error should be returned
		if err == nil {
			t.Error("expected error, got nil")
		} else if !strings.Contains(err.Error(), "mock project root error") {
			t.Errorf("unexpected error message: %v", err)
		}
	})
}
