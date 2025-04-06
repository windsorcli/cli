package env

import (
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

// TestEnv_Initialize tests the Initialize method of the Env struct
func TestEnv_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupEnvMockTests(nil)

		// Use a BaseEnvPrinter for real initialization
		envPrinter := NewBaseEnvPrinter(mocks.Injector)

		// Call Initialize and check for errors
		err := envPrinter.Initialize()
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("ErrorResolvingShell", func(t *testing.T) {
		mocks := setupEnvMockTests(nil)

		// Register an invalid shell that cannot be cast to shell.Shell
		mocks.Injector.Register("shell", "invalid")

		// Use a BaseEnvPrinter for real initialization
		envPrinter := NewBaseEnvPrinter(mocks.Injector)

		// Call Initialize and expect an error
		err := envPrinter.Initialize()
		if err == nil {
			t.Error("expected error, got nil")
		} else if err.Error() != "error resolving or casting shell to shell.Shell" {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("ErrorCastingCliConfigHandler", func(t *testing.T) {
		mocks := setupEnvMockTests(nil)

		// Register an invalid configHandler that cannot be cast to config.ConfigHandler
		mocks.Injector.Register("configHandler", "invalid")

		// Use a BaseEnvPrinter for real initialization
		envPrinter := NewBaseEnvPrinter(mocks.Injector)

		// Call Initialize and expect an error
		err := envPrinter.Initialize()
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
		mocks := setupEnvMockTests(nil)

		// Create a new BaseEnvPrinter and initialize it
		envPrinter := NewBaseEnvPrinter(mocks.Injector)
		err := envPrinter.Initialize()
		if err != nil {
			t.Errorf("unexpected error during initialization: %v", err)
		}

		// Call GetEnvVars and check for errors
		envVars, err := envPrinter.GetEnvVars()
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

		// Create a new BaseEnvPrinter and initialize it
		envPrinter := NewBaseEnvPrinter(mocks.Injector)
		err := envPrinter.Initialize()
		if err != nil {
			t.Errorf("unexpected error during initialization: %v", err)
		}

		// Mock the PrintEnvVarsFunc to verify it is called
		var capturedEnvVars map[string]string
		mocks.Shell.PrintEnvVarsFunc = func(envVars map[string]string) error {
			capturedEnvVars = envVars
			return nil
		}

		// Set up test environment variables
		testEnvVars := map[string]string{"TEST_VAR": "test_value"}

		// Call Print with test environment variables
		err = envPrinter.Print(testEnvVars)
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
		mocks := setupEnvMockTests(nil)

		// Create a new BaseEnvPrinter and initialize it
		envPrinter := NewBaseEnvPrinter(mocks.Injector)
		err := envPrinter.Initialize()
		if err != nil {
			t.Errorf("unexpected error during initialization: %v", err)
		}

		// Mock the PrintEnvVarsFunc to verify it is called
		var capturedEnvVars map[string]string
		mocks.Shell.PrintEnvVarsFunc = func(envVars map[string]string) error {
			capturedEnvVars = envVars
			return nil
		}

		// Call Print without custom vars and check for errors
		err = envPrinter.Print()
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

// TestEnv_PrintAlias tests the PrintAlias method of the Env struct
func TestEnv_PrintAlias(t *testing.T) {
	t.Run("SuccessWithCustomAlias", func(t *testing.T) {
		mocks := setupEnvMockTests(nil)

		// Create a new BaseEnvPrinter and initialize it
		envPrinter := NewBaseEnvPrinter(mocks.Injector)
		err := envPrinter.Initialize()
		if err != nil {
			t.Errorf("unexpected error during initialization: %v", err)
		}

		// Mock the PrintAliasFunc to verify it is called
		var capturedAlias map[string]string
		mocks.Shell.PrintAliasFunc = func(alias map[string]string) error {
			capturedAlias = alias
			return nil
		}

		// Set up test alias
		testAlias := map[string]string{"alias1": "command1"}

		// Call PrintAlias with test alias
		err = envPrinter.PrintAlias(testAlias)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		// Verify that PrintAliasFunc was called with the correct alias
		expectedAlias := map[string]string{"alias1": "command1"}
		if !reflect.DeepEqual(capturedAlias, expectedAlias) {
			t.Errorf("capturedAlias = %v, want %v", capturedAlias, expectedAlias)
		}
	})

	t.Run("SuccessWithoutCustomAlias", func(t *testing.T) {
		mocks := setupEnvMockTests(nil)

		// Create a new BaseEnvPrinter and initialize it
		envPrinter := NewBaseEnvPrinter(mocks.Injector)
		err := envPrinter.Initialize()
		if err != nil {
			t.Errorf("unexpected error during initialization: %v", err)
		}

		// Mock the PrintAliasFunc to verify it is called
		var capturedAlias map[string]string
		mocks.Shell.PrintAliasFunc = func(alias map[string]string) error {
			capturedAlias = alias
			return nil
		}

		// Call PrintAlias without custom alias
		err = envPrinter.PrintAlias()
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		// Verify that PrintAliasFunc was called with an empty map
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

		// Save original value to restore it after the test
		originalManagedEnv := make([]string, len(windsorManagedEnv))
		copy(originalManagedEnv, windsorManagedEnv)
		defer func() {
			windsorManagedMu.Lock()
			windsorManagedEnv = originalManagedEnv
			windsorManagedMu.Unlock()
		}()

		// Set test variables
		windsorManagedMu.Lock()
		windsorManagedEnv = []string{"TEST_VAR1", "TEST_VAR2"}
		windsorManagedMu.Unlock()

		// When calling GetManagedEnv
		managedEnv := envPrinter.GetManagedEnv()

		// Then the returned list should contain our tracked variables
		if len(managedEnv) != 2 {
			t.Errorf("expected 2 variables, got %d", len(managedEnv))
		}

		// Verify expected variables are present
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

		// Save original value to restore it after the test
		originalManagedAlias := make([]string, len(windsorManagedAlias))
		copy(originalManagedAlias, windsorManagedAlias)
		defer func() {
			windsorManagedMu.Lock()
			windsorManagedAlias = originalManagedAlias
			windsorManagedMu.Unlock()
		}()

		// Set test aliases
		windsorManagedMu.Lock()
		windsorManagedAlias = []string{"alias1", "alias2"}
		windsorManagedMu.Unlock()

		// When calling GetManagedAlias
		managedAlias := envPrinter.GetManagedAlias()

		// Then the returned list should contain our tracked aliases
		if len(managedAlias) != 2 {
			t.Errorf("expected 2 aliases, got %d", len(managedAlias))
		}

		// Verify expected aliases are present
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

		// Save original value to restore it after the test
		originalManagedEnv := make([]string, len(windsorManagedEnv))
		copy(originalManagedEnv, windsorManagedEnv)
		defer func() {
			windsorManagedMu.Lock()
			windsorManagedEnv = originalManagedEnv
			windsorManagedMu.Unlock()
		}()

		// Reset managed environment variables for this test
		windsorManagedMu.Lock()
		windsorManagedEnv = []string{}
		windsorManagedMu.Unlock()

		// Set test variables (one string at a time)
		envPrinter.SetManagedEnv("SET_TEST_VAR1")

		// When calling GetManagedEnv to verify
		managedEnv := envPrinter.GetManagedEnv()

		// Then the returned list should contain our variables
		if len(managedEnv) != 1 {
			t.Errorf("expected 1 variable, got %d", len(managedEnv))
		}

		// Verify expected variables are present
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

		// Save original value to restore it after the test
		originalManagedEnv := make([]string, len(windsorManagedEnv))
		copy(originalManagedEnv, windsorManagedEnv)
		defer func() {
			windsorManagedMu.Lock()
			windsorManagedEnv = originalManagedEnv
			windsorManagedMu.Unlock()
		}()

		// Reset managed environment variables for this test
		windsorManagedMu.Lock()
		windsorManagedEnv = []string{}
		windsorManagedMu.Unlock()

		// Set duplicate test variables
		envPrinter.SetManagedEnv("SET_TEST_VAR1")
		envPrinter.SetManagedEnv("SET_TEST_VAR1") // Attempt to add duplicate

		// When calling GetManagedEnv to verify
		managedEnv := envPrinter.GetManagedEnv()

		// Then the returned list should contain only one instance of the variable
		if len(managedEnv) != 1 {
			t.Errorf("expected 1 variable, got %d", len(managedEnv))
		}

		// Verify expected variables are present
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

		// Save original value to restore it after the test
		originalManagedAlias := make([]string, len(windsorManagedAlias))
		copy(originalManagedAlias, windsorManagedAlias)
		defer func() {
			windsorManagedMu.Lock()
			windsorManagedAlias = originalManagedAlias
			windsorManagedMu.Unlock()
		}()

		// Reset managed aliases for this test
		windsorManagedMu.Lock()
		windsorManagedAlias = []string{}
		windsorManagedMu.Unlock()

		// Set test aliases (one string at a time)
		envPrinter.SetManagedAlias("set_alias1")

		// When calling GetManagedAlias to verify
		managedAlias := envPrinter.GetManagedAlias()

		// Then the returned list should contain our aliases
		if len(managedAlias) != 1 {
			t.Errorf("expected 1 alias, got %d", len(managedAlias))
		}

		// Verify expected aliases are present
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

		// Save original value to restore it after the test
		originalManagedAlias := make([]string, len(windsorManagedAlias))
		copy(originalManagedAlias, windsorManagedAlias)
		defer func() {
			windsorManagedMu.Lock()
			windsorManagedAlias = originalManagedAlias
			windsorManagedMu.Unlock()
		}()

		// Reset managed aliases for this test
		windsorManagedMu.Lock()
		windsorManagedAlias = []string{}
		windsorManagedMu.Unlock()

		// Set duplicate test aliases
		envPrinter.SetManagedAlias("set_alias1")
		envPrinter.SetManagedAlias("set_alias1") // Attempt to add duplicate

		// When calling GetManagedAlias to verify
		managedAlias := envPrinter.GetManagedAlias()

		// Then the returned list should contain only one instance of the alias
		if len(managedAlias) != 1 {
			t.Errorf("expected 1 alias, got %d", len(managedAlias))
		}

		// Verify expected aliases are present
		if managedAlias[0] != "set_alias1" {
			t.Errorf("expected [set_alias1], got %v", managedAlias)
		}
	})
}
