package env

import (
	"fmt"
	"os"
	"reflect"
	"testing"

	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
)

// Mocks holds all the mock objects used in the tests.
type Mocks struct {
	Injector      *di.MockInjector
	Shell         *shell.MockShell
	ConfigHandler *config.MockConfigHandler
	EnvPrinter    *MockEnvPrinter
	MockShell     *shell.MockShell
}

// setupEnvMockTests sets up the mock injector and returns the Mocks object.
// It takes an optional injector and only creates one if it's not provided.
func setupEnvMockTests(injector *di.MockInjector) *Mocks {
	if injector == nil {
		injector = di.NewMockInjector()
	}
	mockShell := shell.NewMockShell()
	mockConfigHandler := config.NewMockConfigHandler()
	injector.Register("shell", mockShell)
	injector.Register("configHandler", mockConfigHandler)
	envPrinter := NewMockEnvPrinter()
	return &Mocks{
		Injector:      injector,
		Shell:         mockShell,
		ConfigHandler: mockConfigHandler,
		EnvPrinter:    envPrinter,
		MockShell:     mockShell,
	}
}

// TestEnv_Initialize tests the Initialize method of the Env struct
func TestEnv_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupEnvMockTests(nil)
		env := NewBaseEnvPrinter(mocks.Injector)

		// Call Initialize and check for errors
		err := env.Initialize()
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("ErrorResolvingShell", func(t *testing.T) {
		mocks := setupEnvMockTests(nil)
		env := NewBaseEnvPrinter(mocks.Injector)

		// Register an invalid shell that cannot be cast to shell.Shell
		mocks.Injector.Register("shell", 123) // Use a non-string invalid type

		// Call Initialize and expect an error
		err := env.Initialize()
		if err == nil {
			t.Error("expected error, got nil")
		} else if err.Error() != "error resolving or casting shell to shell.Shell" {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("ErrorCastingCliConfigHandler", func(t *testing.T) {
		mocks := setupEnvMockTests(nil)
		env := NewBaseEnvPrinter(mocks.Injector)

		// Register an invalid configHandler that cannot be cast to config.ConfigHandler
		mocks.Injector.Register("configHandler", "invalid")

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
		mocks := setupEnvMockTests(nil)
		env := NewBaseEnvPrinter(mocks.Injector)
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
		mocks := setupEnvMockTests(nil)
		env := NewBaseEnvPrinter(mocks.Injector)
		env.envPrinter = mocks.EnvPrinter // Ensure EnvPrinter is set
		err := env.Initialize()
		if err != nil {
			t.Fatalf("unexpected error during initialization: %v", err)
		}

		// Mock the GetEnvVars method to return the expected map
		mocks.EnvPrinter.GetEnvVarsFunc = func() (map[string]string, error) {
			return map[string]string{"TEST_VAR": "test_value"}, nil
		}

		// Mock the PrintEnvVars method of the shell to verify it is called
		mocks.MockShell.PrintEnvVarsFunc = func(envVars map[string]string) error {
			if !reflect.DeepEqual(envVars, map[string]string{"TEST_VAR": "test_value"}) {
				return fmt.Errorf("unexpected envVars: %v", envVars)
			}
			return nil
		}

		// Call Print and check for errors
		err = env.Print()
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("NoCustomVars", func(t *testing.T) {
		mocks := setupEnvMockTests(nil)
		env := NewBaseEnvPrinter(mocks.Injector)
		env.envPrinter = mocks.EnvPrinter // Ensure EnvPrinter is set
		err := env.Initialize()
		if err != nil {
			t.Fatalf("unexpected error during initialization: %v", err)
		}

		// Mock the GetEnvVars method to return an empty map
		mocks.EnvPrinter.GetEnvVarsFunc = func() (map[string]string, error) {
			return map[string]string{}, nil
		}

		// Mock the PrintEnvVars method of the shell to verify it is called with an empty map
		mocks.MockShell.PrintEnvVarsFunc = func(envVars map[string]string) error {
			if len(envVars) != 0 {
				return fmt.Errorf("expected empty envVars, got: %v", envVars)
			}
			return nil
		}

		// Call Print and check for errors
		err = env.Print()
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("ErrorGettingEnvVars", func(t *testing.T) {
		mocks := setupEnvMockTests(nil)
		env := NewBaseEnvPrinter(mocks.Injector)
		env.envPrinter = mocks.EnvPrinter // Ensure EnvPrinter is set
		err := env.Initialize()
		if err != nil {
			t.Fatalf("unexpected error during initialization: %v", err)
		}

		// Mock the GetEnvVars method to return an error
		mocks.EnvPrinter.GetEnvVarsFunc = func() (map[string]string, error) {
			return nil, fmt.Errorf("mock error getting env vars")
		}

		// Call Print and expect an error
		err = env.Print()
		if err == nil || err.Error() != "error getting environment variables: mock error getting env vars" {
			t.Errorf("expected error 'error getting environment variables: mock error getting env vars', got %v", err)
		}
	})

	t.Run("ErrorPrintingEnvVars", func(t *testing.T) {
		mocks := setupEnvMockTests(nil)
		env := NewBaseEnvPrinter(mocks.Injector)
		env.envPrinter = mocks.EnvPrinter // Ensure EnvPrinter is set
		err := env.Initialize()
		if err != nil {
			t.Fatalf("unexpected error during initialization: %v", err)
		}

		// Mock the GetEnvVars method to return a valid map
		mocks.EnvPrinter.GetEnvVarsFunc = func() (map[string]string, error) {
			return map[string]string{"TEST_VAR": "test_value"}, nil
		}

		// Mock the PrintEnvVars method of the shell to return an error
		mocks.MockShell.PrintEnvVarsFunc = func(envVars map[string]string) error {
			return fmt.Errorf("mock error printing env vars")
		}

		// Call Print and expect an error
		err = env.Print()
		if err == nil || err.Error() != "error printing environment variables: mock error printing env vars" {
			t.Errorf("expected error 'error printing environment variables: mock error printing env vars', got %v", err)
		}
	})

	t.Run("ErrorEnvPrinterNotSet", func(t *testing.T) {
		mocks := setupEnvMockTests(nil)
		env := NewBaseEnvPrinter(mocks.Injector)
		// Do not set EnvPrinter to simulate the error
		err := env.Initialize()
		if err != nil {
			t.Fatalf("unexpected error during initialization: %v", err)
		}

		// Call Print and expect an error
		err = env.Print()
		if err == nil || err.Error() != "error: EnvPrinter is not set in BaseEnvPrinter" {
			t.Errorf("expected error 'error: EnvPrinter is not set in BaseEnvPrinter', got %v", err)
		}
	})

	t.Run("ErrorGettingAliases", func(t *testing.T) {
		mocks := setupEnvMockTests(nil)
		env := NewBaseEnvPrinter(mocks.Injector)
		env.envPrinter = mocks.EnvPrinter // Ensure EnvPrinter is set
		err := env.Initialize()
		if err != nil {
			t.Fatalf("unexpected error during initialization: %v", err)
		}

		// Mock the GetAlias method to return an error
		mocks.EnvPrinter.GetAliasFunc = func() (map[string]string, error) {
			return nil, fmt.Errorf("mock error getting aliases")
		}

		// Call Print and expect an error
		err = env.Print()
		if err == nil || err.Error() != "error getting aliases: mock error getting aliases" {
			t.Errorf("expected error 'error getting aliases: mock error getting aliases', got %v", err)
		}
	})
}

// TestUnsetEnvVars tests the UnsetEnvVars method of the Env struct
func TestEnv_UnsetEnvVars(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupEnvMockTests(nil)
		env := NewBaseEnvPrinter(mocks.Injector)
		env.envPrinter = mocks.EnvPrinter
		err := env.Initialize()
		if err != nil {
			t.Fatalf("unexpected error during initialization: %v", err)
		}

		// Set the WINDSOR_MANAGED_ENV environment variable for testing
		os.Setenv("WINDSOR_MANAGED_ENV", "TEST_VAR")
		defer os.Unsetenv("WINDSOR_MANAGED_ENV")

		mocks.MockShell.PrintEnvVarsFunc = func(envVars map[string]string) error {
			if envVars["TEST_VAR"] != "" {
				return fmt.Errorf("expected TEST_VAR to be unset, got %v", envVars["TEST_VAR"])
			}
			return nil
		}

		out, err := env.UnsetEnvVars()
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		expectedOutput := map[string]string{"TEST_VAR": ""}
		if !reflect.DeepEqual(out, expectedOutput) {
			t.Errorf("unexpected output: got %v, want %v", out, expectedOutput)
		}
	})

	t.Run("NoCustomVars", func(t *testing.T) {
		mocks := setupEnvMockTests(nil)
		env := NewBaseEnvPrinter(mocks.Injector)
		env.envPrinter = mocks.EnvPrinter
		err := env.Initialize()
		if err != nil {
			t.Fatalf("unexpected error during initialization: %v", err)
		}

		// Ensure WINDSOR_MANAGED_ENV is not set
		os.Unsetenv("WINDSOR_MANAGED_ENV")

		mocks.MockShell.PrintEnvVarsFunc = func(envVars map[string]string) error {
			if len(envVars) != 0 {
				return fmt.Errorf("expected no env vars to be unset, got %v", envVars)
			}
			return nil
		}

		out, err := env.UnsetEnvVars()
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		expectedOutput := map[string]string{}
		if !reflect.DeepEqual(out, expectedOutput) {
			t.Errorf("unexpected output: got %v, want %v", out, expectedOutput)
		}
	})

	t.Run("ErrorUnsettingEnvVars", func(t *testing.T) {
		mocks := setupEnvMockTests(nil)
		env := NewBaseEnvPrinter(mocks.Injector)
		env.envPrinter = mocks.EnvPrinter
		err := env.Initialize()
		if err != nil {
			t.Fatalf("unexpected error during initialization: %v", err)
		}

		// Set the WINDSOR_MANAGED_ENV environment variable for testing
		os.Setenv("WINDSOR_MANAGED_ENV", "TEST_VAR")
		defer os.Unsetenv("WINDSOR_MANAGED_ENV")

		mocks.MockShell.PrintEnvVarsFunc = func(envVars map[string]string) error {
			return fmt.Errorf("mock error unsetting env vars")
		}

		_, err = env.UnsetEnvVars()
		if err == nil || err.Error() != "error unsetting environment variables: mock error unsetting env vars" {
			t.Errorf("expected error 'error unsetting environment variables: mock error unsetting env vars', got %v", err)
		}
	})
}
