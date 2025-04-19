package env

import (
	"os"
	"reflect"
	"slices"
	"testing"

	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
)

// =============================================================================
// Test Setup
// =============================================================================

// Mocks holds all the mock objects used in the tests.
type Mocks struct {
	Injector      *di.MockInjector
	Shell         *shell.MockShell
	ConfigHandler *config.MockConfigHandler
	Env           *MockEnvPrinter
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
	mockEnv := NewMockEnvPrinter()
	injector.Register("env", mockEnv)
	return &Mocks{
		Injector:      injector,
		Shell:         mockShell,
		ConfigHandler: mockConfigHandler,
		Env:           mockEnv,
	}
}

// =============================================================================
// Test Public Methods
// =============================================================================

// TestEnv_Initialize tests the Initialize method of the Env struct
func TestEnv_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new BaseEnvPrinter
		mocks := setupEnvMockTests(nil)
		envPrinter := NewBaseEnvPrinter(mocks.Injector)

		// When calling Initialize
		err := envPrinter.Initialize()

		// Then no error should be returned
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("ErrorResolvingShell", func(t *testing.T) {
		// Given a new BaseEnvPrinter with an invalid shell
		mocks := setupEnvMockTests(nil)
		mocks.Injector.Register("shell", "invalid")
		envPrinter := NewBaseEnvPrinter(mocks.Injector)

		// When calling Initialize
		err := envPrinter.Initialize()

		// Then an error should be returned
		if err == nil {
			t.Error("expected error, got nil")
		} else if err.Error() != "error resolving or casting shell to shell.Shell" {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("ErrorCastingCliConfigHandler", func(t *testing.T) {
		// Given a new BaseEnvPrinter with an invalid configHandler
		mocks := setupEnvMockTests(nil)
		mocks.Injector.Register("configHandler", "invalid")
		envPrinter := NewBaseEnvPrinter(mocks.Injector)

		// When calling Initialize
		err := envPrinter.Initialize()

		// Then an error should be returned
		if err == nil {
			t.Error("expected error, got nil")
		} else if err.Error() != "error resolving or casting configHandler to config.ConfigHandler" {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

// TestBaseEnvPrinter_GetEnvVars tests the GetEnvVars method of the BaseEnvPrinter struct
func TestBaseEnvPrinter_GetEnvVars(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new BaseEnvPrinter
		mocks := setupEnvMockTests(nil)
		envPrinter := NewBaseEnvPrinter(mocks.Injector)
		err := envPrinter.Initialize()
		if err != nil {
			t.Errorf("unexpected error during initialization: %v", err)
		}

		// When calling GetEnvVars
		envVars, err := envPrinter.GetEnvVars()

		// Then no error should be returned and envVars should be empty
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		expectedEnvVars := map[string]string{}
		if !reflect.DeepEqual(envVars, expectedEnvVars) {
			t.Errorf("envVars = %v, want %v", envVars, expectedEnvVars)
		}
	})
}

// TestEnv_Print tests the Print method of the Env struct
func TestEnv_Print(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new BaseEnvPrinter with test environment variables
		mocks := setupEnvMockTests(nil)
		envPrinter := NewBaseEnvPrinter(mocks.Injector)
		err := envPrinter.Initialize()
		if err != nil {
			t.Errorf("unexpected error during initialization: %v", err)
		}

		// And a mock PrintEnvVarsFunc
		var capturedEnvVars map[string]string
		mocks.Shell.PrintEnvVarsFunc = func(envVars map[string]string) {
			capturedEnvVars = envVars
		}

		// And test environment variables
		testEnvVars := map[string]string{"TEST_VAR": "test_value"}

		// When calling Print with test environment variables
		err = envPrinter.Print(testEnvVars)

		// Then no error should be returned and PrintEnvVarsFunc should be called with correct envVars
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		expectedEnvVars := map[string]string{"TEST_VAR": "test_value"}
		if !reflect.DeepEqual(capturedEnvVars, expectedEnvVars) {
			t.Errorf("capturedEnvVars = %v, want %v", capturedEnvVars, expectedEnvVars)
		}
	})

	t.Run("NoCustomVars", func(t *testing.T) {
		// Given a new BaseEnvPrinter
		mocks := setupEnvMockTests(nil)
		envPrinter := NewBaseEnvPrinter(mocks.Injector)
		err := envPrinter.Initialize()
		if err != nil {
			t.Errorf("unexpected error during initialization: %v", err)
		}

		// And a mock PrintEnvVarsFunc
		var capturedEnvVars map[string]string
		mocks.Shell.PrintEnvVarsFunc = func(envVars map[string]string) {
			capturedEnvVars = envVars
		}

		// When calling Print without custom vars
		err = envPrinter.Print()

		// Then no error should be returned and PrintEnvVarsFunc should be called with empty map
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		expectedEnvVars := map[string]string{}
		if !reflect.DeepEqual(capturedEnvVars, expectedEnvVars) {
			t.Errorf("capturedEnvVars = %v, want %v", capturedEnvVars, expectedEnvVars)
		}
	})
}

// TestEnv_PrintAlias tests the PrintAlias method of the Env struct
func TestEnv_PrintAlias(t *testing.T) {
	t.Run("SuccessWithCustomAlias", func(t *testing.T) {
		// Given a new BaseEnvPrinter with test alias
		mocks := setupEnvMockTests(nil)
		envPrinter := NewBaseEnvPrinter(mocks.Injector)
		err := envPrinter.Initialize()
		if err != nil {
			t.Errorf("unexpected error during initialization: %v", err)
		}

		// And a mock PrintAliasFunc
		var capturedAlias map[string]string
		mocks.Shell.PrintAliasFunc = func(alias map[string]string) {
			capturedAlias = alias
		}

		// And test alias
		testAlias := map[string]string{"alias1": "command1"}

		// When calling PrintAlias with test alias
		err = envPrinter.PrintAlias(testAlias)

		// Then no error should be returned and PrintAliasFunc should be called with correct alias
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		expectedAlias := map[string]string{"alias1": "command1"}
		if !reflect.DeepEqual(capturedAlias, expectedAlias) {
			t.Errorf("capturedAlias = %v, want %v", capturedAlias, expectedAlias)
		}
	})

	t.Run("SuccessWithoutCustomAlias", func(t *testing.T) {
		// Given a new BaseEnvPrinter
		mocks := setupEnvMockTests(nil)
		envPrinter := NewBaseEnvPrinter(mocks.Injector)
		err := envPrinter.Initialize()
		if err != nil {
			t.Errorf("unexpected error during initialization: %v", err)
		}

		// And a mock PrintAliasFunc
		var capturedAlias map[string]string
		mocks.Shell.PrintAliasFunc = func(alias map[string]string) {
			capturedAlias = alias
		}

		// When calling PrintAlias without custom alias
		err = envPrinter.PrintAlias()

		// Then no error should be returned and PrintAliasFunc should be called with empty map
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		expectedAlias := map[string]string{}
		if !reflect.DeepEqual(capturedAlias, expectedAlias) {
			t.Errorf("capturedAlias = %v, want %v", capturedAlias, expectedAlias)
		}
	})
}

// TestBaseEnvPrinter_GetManagedEnv tests the GetManagedEnv method of the BaseEnvPrinter struct
func TestBaseEnvPrinter_GetManagedEnv(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new BaseEnvPrinter
		mocks := setupEnvMockTests(nil)
		envPrinter := NewBaseEnvPrinter(mocks.Injector)
		err := envPrinter.Initialize()
		if err != nil {
			t.Errorf("unexpected error during initialization: %v", err)
		}

		// And test environment variables
		originalManagedEnv := make([]string, len(windsorManagedEnv))
		copy(originalManagedEnv, windsorManagedEnv)
		defer func() {
			windsorManagedMu.Lock()
			windsorManagedEnv = originalManagedEnv
			windsorManagedMu.Unlock()
		}()

		windsorManagedMu.Lock()
		windsorManagedEnv = []string{"TEST_VAR1", "TEST_VAR2"}
		windsorManagedMu.Unlock()

		// When calling GetManagedEnv
		managedEnv := envPrinter.GetManagedEnv()

		// Then the returned list should contain our tracked variables
		if len(managedEnv) != 2 {
			t.Errorf("expected 2 variables, got %d", len(managedEnv))
		}
		if managedEnv[0] != "TEST_VAR1" || managedEnv[1] != "TEST_VAR2" {
			t.Errorf("expected [TEST_VAR1, TEST_VAR2], got %v", managedEnv)
		}
	})
}

// TestBaseEnvPrinter_GetManagedAlias tests the GetManagedAlias method of the BaseEnvPrinter struct
func TestBaseEnvPrinter_GetManagedAlias(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new BaseEnvPrinter
		mocks := setupEnvMockTests(nil)
		envPrinter := NewBaseEnvPrinter(mocks.Injector)
		err := envPrinter.Initialize()
		if err != nil {
			t.Errorf("unexpected error during initialization: %v", err)
		}

		// And test aliases
		originalManagedAlias := make([]string, len(windsorManagedAlias))
		copy(originalManagedAlias, windsorManagedAlias)
		defer func() {
			windsorManagedMu.Lock()
			windsorManagedAlias = originalManagedAlias
			windsorManagedMu.Unlock()
		}()

		windsorManagedMu.Lock()
		windsorManagedAlias = []string{"alias1", "alias2"}
		windsorManagedMu.Unlock()

		// When calling GetManagedAlias
		managedAlias := envPrinter.GetManagedAlias()

		// Then the returned list should contain our tracked aliases
		if len(managedAlias) != 2 {
			t.Errorf("expected 2 aliases, got %d", len(managedAlias))
		}
		if managedAlias[0] != "alias1" || managedAlias[1] != "alias2" {
			t.Errorf("expected [alias1, alias2], got %v", managedAlias)
		}
	})
}

// TestBaseEnvPrinter_SetManagedEnv tests the SetManagedEnv method of the BaseEnvPrinter struct
func TestBaseEnvPrinter_SetManagedEnv(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new BaseEnvPrinter
		mocks := setupEnvMockTests(nil)
		envPrinter := NewBaseEnvPrinter(mocks.Injector)
		err := envPrinter.Initialize()
		if err != nil {
			t.Errorf("unexpected error during initialization: %v", err)
		}

		// And empty managed environment variables
		originalManagedEnv := make([]string, len(windsorManagedEnv))
		copy(originalManagedEnv, windsorManagedEnv)
		defer func() {
			windsorManagedMu.Lock()
			windsorManagedEnv = originalManagedEnv
			windsorManagedMu.Unlock()
		}()

		windsorManagedMu.Lock()
		windsorManagedEnv = []string{}
		windsorManagedMu.Unlock()

		// When setting a managed environment variable
		envPrinter.SetManagedEnv("SET_TEST_VAR1")

		// Then GetManagedEnv should return the variable
		managedEnv := envPrinter.GetManagedEnv()
		if len(managedEnv) != 1 {
			t.Errorf("expected 1 variable, got %d", len(managedEnv))
		}
		if managedEnv[0] != "SET_TEST_VAR1" {
			t.Errorf("expected [SET_TEST_VAR1], got %v", managedEnv)
		}
	})

	t.Run("Dedupe", func(t *testing.T) {
		// Given a new BaseEnvPrinter
		mocks := setupEnvMockTests(nil)
		envPrinter := NewBaseEnvPrinter(mocks.Injector)
		err := envPrinter.Initialize()
		if err != nil {
			t.Errorf("unexpected error during initialization: %v", err)
		}

		// And empty managed environment variables
		originalManagedEnv := make([]string, len(windsorManagedEnv))
		copy(originalManagedEnv, windsorManagedEnv)
		defer func() {
			windsorManagedMu.Lock()
			windsorManagedEnv = originalManagedEnv
			windsorManagedMu.Unlock()
		}()

		windsorManagedMu.Lock()
		windsorManagedEnv = []string{}
		windsorManagedMu.Unlock()

		// When setting duplicate managed environment variables
		envPrinter.SetManagedEnv("SET_TEST_VAR1")
		envPrinter.SetManagedEnv("SET_TEST_VAR1")

		// Then GetManagedEnv should return only one instance
		managedEnv := envPrinter.GetManagedEnv()
		if len(managedEnv) != 1 {
			t.Errorf("expected 1 variable, got %d", len(managedEnv))
		}
		if managedEnv[0] != "SET_TEST_VAR1" {
			t.Errorf("expected [SET_TEST_VAR1], got %v", managedEnv)
		}
	})
}

// TestBaseEnvPrinter_SetManagedAlias tests the SetManagedAlias method of the BaseEnvPrinter struct
func TestBaseEnvPrinter_SetManagedAlias(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new BaseEnvPrinter
		mocks := setupEnvMockTests(nil)
		envPrinter := NewBaseEnvPrinter(mocks.Injector)
		err := envPrinter.Initialize()
		if err != nil {
			t.Errorf("unexpected error during initialization: %v", err)
		}

		// And empty managed aliases
		originalManagedAlias := make([]string, len(windsorManagedAlias))
		copy(originalManagedAlias, windsorManagedAlias)
		defer func() {
			windsorManagedMu.Lock()
			windsorManagedAlias = originalManagedAlias
			windsorManagedMu.Unlock()
		}()

		windsorManagedMu.Lock()
		windsorManagedAlias = []string{}
		windsorManagedMu.Unlock()

		// When setting a managed alias
		envPrinter.SetManagedAlias("set_alias1")

		// Then GetManagedAlias should return the alias
		managedAlias := envPrinter.GetManagedAlias()
		if len(managedAlias) != 1 {
			t.Errorf("expected 1 alias, got %d", len(managedAlias))
		}
		if managedAlias[0] != "set_alias1" {
			t.Errorf("expected [set_alias1], got %v", managedAlias)
		}
	})

	t.Run("Dedupe", func(t *testing.T) {
		// Given a new BaseEnvPrinter
		mocks := setupEnvMockTests(nil)
		envPrinter := NewBaseEnvPrinter(mocks.Injector)
		err := envPrinter.Initialize()
		if err != nil {
			t.Errorf("unexpected error during initialization: %v", err)
		}

		// And empty managed aliases
		originalManagedAlias := make([]string, len(windsorManagedAlias))
		copy(originalManagedAlias, windsorManagedAlias)
		defer func() {
			windsorManagedMu.Lock()
			windsorManagedAlias = originalManagedAlias
			windsorManagedMu.Unlock()
		}()

		windsorManagedMu.Lock()
		windsorManagedAlias = []string{}
		windsorManagedMu.Unlock()

		// When setting duplicate managed aliases
		envPrinter.SetManagedAlias("set_alias1")
		envPrinter.SetManagedAlias("set_alias1")

		// Then GetManagedAlias should return only one instance
		managedAlias := envPrinter.GetManagedAlias()
		if len(managedAlias) != 1 {
			t.Errorf("expected 1 alias, got %d", len(managedAlias))
		}
		if managedAlias[0] != "set_alias1" {
			t.Errorf("expected [set_alias1], got %v", managedAlias)
		}
	})
}

// TestBaseEnvPrinter_Reset tests the Reset method of the BaseEnvPrinter struct
func TestBaseEnvPrinter_Reset(t *testing.T) {
	t.Run("ResetWithNoEnvVars", func(t *testing.T) {
		// Given a new BaseEnvPrinter
		mocks := setupEnvMockTests(nil)
		envPrinter := NewBaseEnvPrinter(mocks.Injector)
		err := envPrinter.Initialize()
		if err != nil {
			t.Errorf("unexpected error during initialization: %v", err)
		}

		// And a mock Reset function
		resetCalled := false
		mocks.Shell.ResetFunc = func() {
			resetCalled = true
		}

		// When calling Reset
		envPrinter.Reset()

		// Then shell.Reset should be called
		if !resetCalled {
			t.Errorf("expected Shell.Reset to be called, but it wasn't")
		}
	})

	t.Run("ResetWithEnvironmentVariables", func(t *testing.T) {
		// Given a new BaseEnvPrinter with environment variables
		mocks := setupEnvMockTests(nil)
		envPrinter := NewBaseEnvPrinter(mocks.Injector)
		err := envPrinter.Initialize()
		if err != nil {
			t.Errorf("unexpected error during initialization: %v", err)
		}

		// And environment variables set
		os.Setenv("WINDSOR_MANAGED_ENV", "ENV1,ENV2, ENV3")
		os.Setenv("WINDSOR_MANAGED_ALIAS", "alias1,alias2, alias3")
		defer func() {
			os.Unsetenv("WINDSOR_MANAGED_ENV")
			os.Unsetenv("WINDSOR_MANAGED_ALIAS")
		}()

		// And a mock Reset function
		resetCalled := false
		mocks.Shell.ResetFunc = func() {
			resetCalled = true
		}

		// When calling Reset
		envPrinter.Reset()

		// Then shell.Reset should be called
		if !resetCalled {
			t.Errorf("expected Shell.Reset to be called, but it wasn't")
		}
	})

	t.Run("InternalStatePersistsWithReset", func(t *testing.T) {
		// Given a new BaseEnvPrinter with managed environment variables and aliases
		mocks := setupEnvMockTests(nil)
		envPrinter := NewBaseEnvPrinter(mocks.Injector)
		err := envPrinter.Initialize()
		if err != nil {
			t.Errorf("unexpected error during initialization: %v", err)
		}

		// And managed environment variables and aliases set
		envPrinter.SetManagedEnv("TEST_ENV1")
		envPrinter.SetManagedEnv("TEST_ENV2")
		envPrinter.SetManagedAlias("test_alias1")
		envPrinter.SetManagedAlias("test_alias2")

		// And a mock Reset function
		resetCalled := false
		mocks.Shell.ResetFunc = func() {
			resetCalled = true
		}

		// When calling Reset
		envPrinter.Reset()

		// Then shell.Reset should be called
		if !resetCalled {
			t.Errorf("expected Shell.Reset to be called, but it wasn't")
		}

		// And the managed environment variables should still be available
		managedEnv := envPrinter.GetManagedEnv()
		for _, env := range []string{"TEST_ENV1", "TEST_ENV2"} {
			if !slices.Contains(managedEnv, env) {
				t.Errorf("expected GetManagedEnv to contain %s", env)
			}
		}

		// And the managed aliases should still be available
		managedAlias := envPrinter.GetManagedAlias()
		for _, alias := range []string{"test_alias1", "test_alias2"} {
			if !slices.Contains(managedAlias, alias) {
				t.Errorf("expected GetManagedAlias to contain %s", alias)
			}
		}
	})
}
