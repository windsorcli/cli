package env

import (
	"reflect"
	"testing"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/shell"
)

// TestEnv_Initialize tests the Initialize method of the Env struct
func TestEnv_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Create a mock injector and Env instance
		mockInjector := di.NewMockInjector()

		// Create and register mock versions of contextHandler, shell, and configHandler
		mockContextHandler := context.NewMockContext()
		mockInjector.Register("contextHandler", mockContextHandler)
		mockShell := shell.NewMockShell()
		mockInjector.Register("shell", mockShell)
		mockConfigHandler := config.NewMockConfigHandler()
		mockInjector.Register("configHandler", mockConfigHandler)

		env := NewBaseEnvPrinter(mockInjector)

		// Call Initialize and check for errors
		err := env.Initialize()
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("ErrorResolvingContextHandler", func(t *testing.T) {
		// Create a mock injector and Env instance
		mockInjector := di.NewMockInjector()

		// Register an invalid contextHandler that cannot be cast to context.ContextHandler
		mockInjector.Register("contextHandler", "invalid")

		env := NewBaseEnvPrinter(mockInjector)

		// Call Initialize and expect an error
		err := env.Initialize()
		if err == nil {
			t.Error("expected error, got nil")
		} else if err.Error() != "error resolving or casting contextHandler to context.ContextHandler" {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("ErrorResolvingShell", func(t *testing.T) {
		// Create a mock injector and Env instance
		mockInjector := di.NewMockInjector()

		// Register mock versions of contextHandler and configHandler
		mockContextHandler := context.NewMockContext()
		mockInjector.Register("contextHandler", mockContextHandler)
		mockConfigHandler := config.NewMockConfigHandler()
		mockInjector.Register("configHandler", mockConfigHandler)

		// Register an invalid shell that cannot be cast to shell.Shell
		mockInjector.Register("shell", "invalid")

		env := NewBaseEnvPrinter(mockInjector)

		// Call Initialize and expect an error
		err := env.Initialize()
		if err == nil {
			t.Error("expected error, got nil")
		} else if err.Error() != "error resolving or casting shell to shell.Shell" {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("ErrorCastingCliConfigHandler", func(t *testing.T) {
		// Create a mock injector and Env instance
		mockInjector := di.NewMockInjector()

		// Register mock versions of contextHandler and shell
		mockContextHandler := context.NewMockContext()
		mockInjector.Register("contextHandler", mockContextHandler)
		mockShell := shell.NewMockShell()
		mockInjector.Register("shell", mockShell)

		// Register an invalid configHandler that cannot be cast to config.ConfigHandler
		mockInjector.Register("configHandler", "invalid")

		env := NewBaseEnvPrinter(mockInjector)

		// Call Initialize and expect an error
		err := env.Initialize()
		if err == nil {
			t.Error("expected error, got nil")
		} else if err.Error() != "error resolving or casting configHandler to config.ConfigHandler" {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

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

	t.Run("NoCustomVars", func(t *testing.T) {
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

		// Call Print without custom vars and check for errors
		err := env.Print()
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		// Verify that PrintEnvVarsFunc was called with an empty map
		expectedEnvVars := map[string]string{}
		if !reflect.DeepEqual(capturedEnvVars, expectedEnvVars) {
			t.Errorf("capturedEnvVars = %v, want %v", capturedEnvVars, expectedEnvVars)
		}
	})
}
