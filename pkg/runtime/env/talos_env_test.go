package env

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/windsorcli/cli/pkg/runtime/config"
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

		printer := NewTalosEnvPrinter(mocks.Shell, mocks.ConfigHandler)
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
