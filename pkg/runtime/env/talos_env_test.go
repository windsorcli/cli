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
	setup := func(t *testing.T, clusterDriver string, platform string) (*TalosEnvPrinter, *EnvTestMocks) {
		t.Helper()

		// Create a mock config handler
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "cluster.driver" {
				return clusterDriver
			}
			if key == "platform" {
				return platform
			}
			return ""
		}

		mocks := setupEnvMocks(t, &EnvTestMocks{
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

	t.Run("TalosDriverSetsTalosConfig", func(t *testing.T) {
		// Given a TalosEnvPrinter with talos cluster driver
		printer, mocks := setup(t, "talos", "docker")

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

		// And OMNICONFIG should not be set for non-omni platform
		if _, exists := envVars["OMNICONFIG"]; exists {
			t.Error("OMNICONFIG should not be set for non-omni platform")
		}
	})

	t.Run("OmniPlatformSetsOmniConfigWithTalosDriver", func(t *testing.T) {
		// Given a TalosEnvPrinter with omni platform and talos cluster driver
		printer, mocks := setup(t, "talos", "omni")

		// Get the project root path
		projectRoot, err := mocks.Shell.GetProjectRoot()
		if err != nil {
			t.Fatalf("Failed to get project root: %v", err)
		}
		expectedOmniPath := filepath.Join(projectRoot, "contexts", "mock-context", ".omni", "config")

		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if name == expectedOmniPath {
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

		// And TALOSCONFIG should be set for talos cluster driver
		expectedTalosPath := filepath.Join(projectRoot, "contexts", "mock-context", ".talos", "config")
		if envVars["TALOSCONFIG"] != expectedTalosPath {
			t.Errorf("TALOSCONFIG = %v, want %v", envVars["TALOSCONFIG"], expectedTalosPath)
		}

		// And OMNICONFIG should be set correctly for omni platform
		if envVars["OMNICONFIG"] != expectedOmniPath {
			t.Errorf("OMNICONFIG = %v, want %v", envVars["OMNICONFIG"], expectedOmniPath)
		}
	})

	t.Run("NoTalosConfigWhenDriverMissing", func(t *testing.T) {
		// Given a TalosEnvPrinter without talos driver
		printer, mocks := setup(t, "", "docker")

		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		// When getting environment variables
		envVars, err := printer.GetEnvVars()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		// And TALOSCONFIG should not be set when driver is missing
		if _, exists := envVars["TALOSCONFIG"]; exists {
			t.Error("TALOSCONFIG should not be set when cluster.driver is missing")
		}
	})

	t.Run("GetProjectRootError", func(t *testing.T) {
		// Given a new TalosOmniEnvPrinter with failing config root lookup
		printer, _ := setup(t, "talos", "docker")

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
