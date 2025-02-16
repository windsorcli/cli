package env

import (
	"reflect"
	"testing"

	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
)

type CustomEnvMocks struct {
	Injector      di.Injector
	ConfigHandler *config.MockConfigHandler
	Shell         *shell.MockShell
}

func setupSafeCustomEnvMocks(injector ...di.Injector) *CustomEnvMocks {
	// Use the provided injector or create a new one if not provided
	var mockInjector di.Injector
	if len(injector) > 0 {
		mockInjector = injector[0]
	} else {
		mockInjector = di.NewMockInjector()
	}

	// Create a mock ConfigHandler using its constructor
	mockConfigHandler := config.NewMockConfigHandler()
	mockConfigHandler.GetStringMapFunc = func(key string, defaultValue ...map[string]string) map[string]string {
		if key == "environment" {
			return map[string]string{
				"VAR1": "value1",
				"VAR2": "value2",
			}
		}
		return nil
	}

	// Create a mock Shell using its constructor
	mockShell := shell.NewMockShell()

	// Register the mocks in the DI injector
	mockInjector.Register("configHandler", mockConfigHandler)
	mockInjector.Register("shell", mockShell)

	return &CustomEnvMocks{
		Injector:      mockInjector,
		ConfigHandler: mockConfigHandler,
		Shell:         mockShell,
	}
}

func TestCustomEnv_GetEnvVars(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Use setupSafeCustomEnvMocks to create mocks
		mocks := setupSafeCustomEnvMocks()

		customEnvPrinter := NewCustomEnvPrinter(mocks.Injector)
		customEnvPrinter.Initialize()

		// When calling GetEnvVars
		envVars, err := customEnvPrinter.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		// Then the environment variables should be set correctly
		expectedEnvVars := map[string]string{
			"VAR1": "value1",
			"VAR2": "value2",
		}
		if !reflect.DeepEqual(envVars, expectedEnvVars) {
			t.Errorf("envVars = %v, want %v", envVars, expectedEnvVars)
		}
	})

	t.Run("MissingConfiguration", func(t *testing.T) {
		// Use setupSafeCustomEnvMocks to create mocks
		mocks := setupSafeCustomEnvMocks()

		// Override the GetStringMapFunc to return nil for environment configuration
		mocks.ConfigHandler.GetStringMapFunc = func(key string, defaultValue ...map[string]string) map[string]string {
			return nil
		}

		customEnvPrinter := NewCustomEnvPrinter(mocks.Injector)
		customEnvPrinter.Initialize()

		// When calling GetEnvVars
		envVars, err := customEnvPrinter.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		// Then the environment variables should be an empty map
		expectedEnvVars := map[string]string{}
		if !reflect.DeepEqual(envVars, expectedEnvVars) {
			t.Errorf("envVars = %v, want %v", envVars, expectedEnvVars)
		}
	})
}

func TestCustomEnv_Print(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Use setupSafeCustomEnvMocks to create mocks
		mocks := setupSafeCustomEnvMocks()
		customEnvPrinter := NewCustomEnvPrinter(mocks.Injector)
		customEnvPrinter.Initialize()

		// Mock the PrintEnvVarsFunc to verify it is called with the correct envVars
		var capturedEnvVars map[string]string
		mocks.Shell.PrintEnvVarsFunc = func(envVars map[string]string) error {
			capturedEnvVars = envVars
			return nil
		}

		// Call Print and check for errors
		err := customEnvPrinter.Print()
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		// Verify that PrintEnvVarsFunc was called with the correct envVars
		expectedEnvVars := map[string]string{
			"VAR1": "value1",
			"VAR2": "value2",
		}
		if !reflect.DeepEqual(capturedEnvVars, expectedEnvVars) {
			t.Errorf("capturedEnvVars = %v, want %v", capturedEnvVars, expectedEnvVars)
		}
	})
}
