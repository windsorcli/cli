package env

import (
	"reflect"
	"strings"
	"testing"

	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/shell"
)

// TestEnv_GetEnvVars tests the GetEnvVars method of the Env struct
func TestEnv_GetEnvVars(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Create a mock injector and Env instance
		mockInjector := di.NewMockInjector()
		env := &BaseEnvPrinter{injector: mockInjector}

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
		env := &BaseEnvPrinter{injector: mockInjector}

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

	t.Run("ShellResolveError", func(t *testing.T) {
		// Create a mock injector without registering the shell
		mockInjector := di.NewMockInjector()
		env := &BaseEnvPrinter{injector: mockInjector}

		// Call Print and expect an error due to missing shell
		err := env.Print()
		if err == nil {
			t.Error("expected error, got nil")
		} else if !strings.Contains(err.Error(), "error resolving shell") {
			t.Errorf("unexpected error message: %v", err)
		}
	})

	t.Run("CastShellError", func(t *testing.T) {
		// Create a mock injector and Env instance
		mockInjector := di.NewMockInjector()
		mockInjector.Register("shell", "invalid-shell")
		env := &BaseEnvPrinter{injector: mockInjector}

		// Call Print and expect an error due to invalid shell type
		err := env.Print()
		if err == nil {
			t.Error("expected error, got nil")
		} else if !strings.Contains(err.Error(), "shell is not of type Shell") {
			t.Errorf("unexpected error message: %v", err)
		}
	})
}
