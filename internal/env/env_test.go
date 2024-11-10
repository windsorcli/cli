package env

import (
	"errors"
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

		// Create and register mock versions of contextHandler, shell, and cliConfigHandler
		mockContextHandler := context.NewMockContext()
		mockInjector.Register("contextHandler", mockContextHandler)
		mockShell := shell.NewMockShell()
		mockInjector.Register("shell", mockShell)
		mockConfigHandler := config.NewMockConfigHandler()
		mockInjector.Register("cliConfigHandler", mockConfigHandler)

		env := NewBaseEnvPrinter(mockInjector)

		// Call Initialize and check for errors
		err := env.Initialize()
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("ErrorCastingContextHandler", func(t *testing.T) {
		// Create a mock injector and Env instance
		mockInjector := di.NewMockInjector()

		// Register an invalid contextHandler that cannot be cast to context.ContextInterface
		mockInjector.Register("contextHandler", "invalid")

		env := NewBaseEnvPrinter(mockInjector)

		// Call Initialize and expect an error
		err := env.Initialize()
		if err == nil {
			t.Error("expected error, got nil")
		} else if err.Error() != "failed to cast contextHandler to context.ContextInterface" {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("ErrorResolvingShell", func(t *testing.T) {
		// Create a mock injector and Env instance
		mockInjector := di.NewMockInjector()

		// Register mock versions of contextHandler and cliConfigHandler
		mockContextHandler := context.NewMockContext()
		mockInjector.Register("contextHandler", mockContextHandler)
		mockConfigHandler := config.NewMockConfigHandler()
		mockInjector.Register("cliConfigHandler", mockConfigHandler)

		// Set an error for shell resolution to simulate resolution error
		mockInjector.SetResolveError("shell", errors.New("di: could not resolve dependency"))

		env := NewBaseEnvPrinter(mockInjector)

		// Call Initialize and expect an error
		err := env.Initialize()
		if err == nil {
			t.Error("expected error, got nil")
		} else if err.Error() != "error resolving shell: di: could not resolve dependency" {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("ErrorCastingShell", func(t *testing.T) {
		// Create a mock injector and Env instance
		mockInjector := di.NewMockInjector()

		// Register mock versions of contextHandler and cliConfigHandler
		mockContextHandler := context.NewMockContext()
		mockInjector.Register("contextHandler", mockContextHandler)
		mockConfigHandler := config.NewMockConfigHandler()
		mockInjector.Register("cliConfigHandler", mockConfigHandler)

		// Register an invalid shell that cannot be cast to shell.Shell
		mockInjector.Register("shell", "invalid")

		env := NewBaseEnvPrinter(mockInjector)

		// Call Initialize and expect an error
		err := env.Initialize()
		if err == nil {
			t.Error("expected error, got nil")
		} else if err.Error() != "shell is not of type Shell" {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("ErrorResolvingCliConfigHandler", func(t *testing.T) {
		// Create a mock injector and Env instance
		mockInjector := di.NewMockInjector()

		// Register mock versions of contextHandler and shell
		mockContextHandler := context.NewMockContext()
		mockInjector.Register("contextHandler", mockContextHandler)
		mockShell := shell.NewMockShell()
		mockInjector.Register("shell", mockShell)

		// Set an error for cliConfigHandler resolution to simulate resolution error
		mockInjector.SetResolveError("cliConfigHandler", errors.New("di: could not resolve dependency"))

		env := NewBaseEnvPrinter(mockInjector)

		// Call Initialize and expect an error
		err := env.Initialize()
		if err == nil {
			t.Error("expected error, got nil")
		} else if err.Error() != "error resolving cliConfigHandler: di: could not resolve dependency" {
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

		// Register an invalid cliConfigHandler that cannot be cast to config.ConfigHandler
		mockInjector.Register("cliConfigHandler", "invalid")

		env := NewBaseEnvPrinter(mockInjector)

		// Call Initialize and expect an error
		err := env.Initialize()
		if err == nil {
			t.Error("expected error, got nil")
		} else if err.Error() != "cliConfigHandler is not of type ConfigHandler" {
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
