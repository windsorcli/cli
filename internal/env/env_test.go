package env

import (
	"reflect"
	"testing"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/shell"
)

// TestEnv_GetEnvVars tests the GetEnvVars method of the Env struct
func TestEnv_GetEnvVars(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Create a mock injector and Env instance
		mockInjector := di.NewMockInjector()
		env := NewBaseEnvPrinter(mockInjector)
		env.Initialize()

		// Call GetEnvVars and check for errors
		envVars, err := env.GetEnvVars()
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		// Verify that the returned envVars is an empty map
		expectedEnvVars := map[string]string{}
		if !reflect.DeepEqual(envVars, expectedEnvVars) {
			t.Errorf("envVars = %v, want %v", envVars, expectedEnvVars)
		}
	})
}

// TestEnv_Print tests the Print method of the Env struct
func TestEnv_Print(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Create a mock injector and Env instance
		mockInjector := di.NewMockInjector()
		mockShell := shell.NewMockShell()
		mockInjector.Register("shell", mockShell)
		mockConfigHandler := config.NewMockConfigHandler()
		mockInjector.Register("configHandler", mockConfigHandler)
		mockContextHandler := context.NewMockContext()
		mockInjector.Register("contextHandler", mockContextHandler)
		env := NewBaseEnvPrinter(mockInjector)
		env.Initialize()

		// Mock the PrintEnvVarsFunc to verify it is called
		var capturedEnvVars map[string]string
		mockShell.PrintEnvVarsFunc = func(envVars map[string]string) error {
			capturedEnvVars = envVars
			return nil
		}

		// Call Print and check for errors
		err := env.Print(map[string]string{"TEST_VAR": "test_value"})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		// Verify that PrintEnvVarsFunc was called with the correct envVars
		expectedEnvVars := map[string]string{"TEST_VAR": "test_value"}
		if !reflect.DeepEqual(capturedEnvVars, expectedEnvVars) {
			t.Errorf("capturedEnvVars = %v, want %v", capturedEnvVars, expectedEnvVars)
		}
	})
}
