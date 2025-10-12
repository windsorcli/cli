package env

import (
	"os"
	"reflect"
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
	Injector      di.Injector
	ConfigHandler config.ConfigHandler
	Shell         *shell.MockShell
	Shims         *Shims
}

type SetupOptions struct {
	Injector      di.Injector
	ConfigHandler config.ConfigHandler
	ConfigStr     string
}

// setupShims creates a new Shims instance with default implementations
func setupShims(t *testing.T) *Shims {
	t.Helper()
	shims := NewShims()

	shims.LookupEnv = func(key string) (string, bool) { return "", false }
	shims.WriteFile = func(name string, data []byte, perm os.FileMode) error { return nil }
	shims.ReadFile = func(name string) ([]byte, error) { return []byte{}, nil }
	shims.MkdirAll = func(path string, perm os.FileMode) error { return nil }
	shims.UserHomeDir = func() (string, error) { return t.TempDir(), nil }
	shims.Stat = func(name string) (os.FileInfo, error) { return nil, nil }
	shims.Getwd = func() (string, error) { return t.TempDir(), nil }

	return shims
}

func setupMocks(t *testing.T, opts ...*SetupOptions) *Mocks {
	t.Helper()

	// Store original directory and create temp dir
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}

	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to change to temp directory: %v", err)
	}

	// Set project root environment variable
	os.Setenv("WINDSOR_PROJECT_ROOT", tmpDir)
	os.Setenv("WINDSOR_CONTEXT", "mock-context")

	// Process options with defaults
	options := &SetupOptions{}
	if len(opts) > 0 && opts[0] != nil {
		options = opts[0]
	}

	// Create injector
	var injector di.Injector
	if options.Injector == nil {
		injector = di.NewInjector()
	} else {
		injector = options.Injector
	}

	// Create shell with project root matching temp dir
	mockShell := shell.NewMockShell()
	mockShell.GetProjectRootFunc = func() (string, error) {
		return tmpDir, nil
	}
	injector.Register("shell", mockShell)

	// Create config handler
	var configHandler config.ConfigHandler
	if options.ConfigHandler == nil {
		configHandler = config.NewConfigHandler(injector)
	} else {
		configHandler = options.ConfigHandler
	}
	if options.ConfigStr != "" {
		configHandler.LoadConfigString(options.ConfigStr)
	}
	injector.Register("configHandler", configHandler)

	// Setup shims
	shims := setupShims(t)

	configHandler.Initialize()

	// Register cleanup to restore original state
	t.Cleanup(func() {
		os.Unsetenv("WINDSOR_PROJECT_ROOT")
		os.Unsetenv("WINDSOR_CONTEXT")
		if err := os.Chdir(origDir); err != nil {
			t.Logf("Warning: Failed to change back to original directory: %v", err)
		}
	})

	// Return mocks
	return &Mocks{
		Injector:      injector,
		Shell:         mockShell,
		ConfigHandler: configHandler,
		Shims:         shims,
	}
}

// =============================================================================
// Test Public Methods
// =============================================================================

// TestEnv_Initialize tests the Initialize method of the Env struct
func TestEnv_Initialize(t *testing.T) {
	setup := func(t *testing.T) (*BaseEnvPrinter, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		printer := NewBaseEnvPrinter(mocks.Injector)
		return printer, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a new BaseEnvPrinter
		printer, _ := setup(t)

		// When calling Initialize
		err := printer.Initialize()

		// Then no error should be returned
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("ErrorResolvingShell", func(t *testing.T) {
		// Given a new BaseEnvPrinter with an invalid shell
		injector := di.NewMockInjector()
		injector.Register("shell", "invalid")
		printer := NewBaseEnvPrinter(injector)

		// When calling Initialize
		err := printer.Initialize()

		// Then an error should be returned
		if err == nil {
			t.Error("expected error, got nil")
		} else if err.Error() != "error resolving or casting shell to shell.Shell" {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("ErrorCastingCliConfigHandler", func(t *testing.T) {
		// Given a new BaseEnvPrinter with an invalid configHandler
		injector := di.NewMockInjector()
		injector.Register("shell", shell.NewMockShell())
		injector.Register("configHandler", struct{}{})
		printer := NewBaseEnvPrinter(injector)

		// When calling Initialize
		err := printer.Initialize()

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
	setup := func(t *testing.T) (*BaseEnvPrinter, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		printer := NewBaseEnvPrinter(mocks.Injector)
		err := printer.Initialize()
		if err != nil {
			t.Errorf("unexpected error during initialization: %v", err)
		}
		return printer, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a new BaseEnvPrinter
		printer, _ := setup(t)

		// When calling GetEnvVars
		envVars, err := printer.GetEnvVars()

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
	setup := func(t *testing.T) (*BaseEnvPrinter, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		printer := NewBaseEnvPrinter(mocks.Injector)
		err := printer.Initialize()
		if err != nil {
			t.Errorf("unexpected error during initialization: %v", err)
		}
		return printer, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a new BaseEnvPrinter with test environment variables
		printer, mocks := setup(t)

		// And a mock PrintEnvVarsFunc
		var capturedEnvVars map[string]string
		mocks.Shell.PrintEnvVarsFunc = func(envVars map[string]string, export bool) {
			capturedEnvVars = envVars
		}

		// And test environment variables
		testEnvVars := map[string]string{"TEST_VAR": "test_value"}

		// When calling Print with test environment variables
		err := printer.Print(testEnvVars)

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
		printer, mocks := setup(t)

		// And a mock PrintEnvVarsFunc
		var capturedEnvVars map[string]string
		mocks.Shell.PrintEnvVarsFunc = func(envVars map[string]string, export bool) {
			capturedEnvVars = envVars
		}

		// When calling Print without custom vars
		err := printer.Print()

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
	setup := func(t *testing.T) (*BaseEnvPrinter, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		printer := NewBaseEnvPrinter(mocks.Injector)
		err := printer.Initialize()
		if err != nil {
			t.Errorf("unexpected error during initialization: %v", err)
		}
		return printer, mocks
	}

	t.Run("SuccessWithCustomAlias", func(t *testing.T) {
		// Given a new BaseEnvPrinter with test alias
		printer, mocks := setup(t)

		// And a mock PrintAliasFunc
		var capturedAlias map[string]string
		mocks.Shell.PrintAliasFunc = func(alias map[string]string) {
			capturedAlias = alias
		}

		// And test alias
		testAlias := map[string]string{"alias1": "command1"}

		// When calling PrintAlias with test alias
		err := printer.PrintAlias(testAlias)

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
		printer, mocks := setup(t)

		// And a mock PrintAliasFunc
		var capturedAlias map[string]string
		mocks.Shell.PrintAliasFunc = func(alias map[string]string) {
			capturedAlias = alias
		}

		// When calling PrintAlias without custom alias
		err := printer.PrintAlias()

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
	setup := func(t *testing.T) (*BaseEnvPrinter, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		printer := NewBaseEnvPrinter(mocks.Injector)
		err := printer.Initialize()
		if err != nil {
			t.Errorf("unexpected error during initialization: %v", err)
		}
		return printer, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a new BaseEnvPrinter
		printer, _ := setup(t)

		// And test environment variables
		// Store original managed environment variables
		originalManagedEnv := make([]string, len(printer.managedEnv))
		copy(originalManagedEnv, printer.managedEnv)
		defer func() {
			printer.managedEnv = originalManagedEnv
		}()

		// Set test environment variables
		printer.managedEnv = []string{"TEST_VAR1", "TEST_VAR2"}

		// When calling GetManagedEnv
		managedEnv := printer.GetManagedEnv()

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
	setup := func(t *testing.T) (*BaseEnvPrinter, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		printer := NewBaseEnvPrinter(mocks.Injector)
		err := printer.Initialize()
		if err != nil {
			t.Errorf("unexpected error during initialization: %v", err)
		}
		return printer, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a new BaseEnvPrinter
		printer, _ := setup(t)

		// And test aliases
		// Store original managed aliases
		originalManagedAlias := make([]string, len(printer.managedAlias))
		copy(originalManagedAlias, printer.managedAlias)
		defer func() {
			printer.managedAlias = originalManagedAlias
		}()

		// Set test aliases
		printer.managedAlias = []string{"alias1", "alias2"}

		// When calling GetManagedAlias
		managedAlias := printer.GetManagedAlias()

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
	setup := func(t *testing.T) (*BaseEnvPrinter, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		printer := NewBaseEnvPrinter(mocks.Injector)
		err := printer.Initialize()
		if err != nil {
			t.Errorf("unexpected error during initialization: %v", err)
		}
		return printer, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a new BaseEnvPrinter
		printer, _ := setup(t)

		// And empty managed environment variables
		// Store original managed environment variables
		originalManagedEnv := make([]string, len(printer.managedEnv))
		copy(originalManagedEnv, printer.managedEnv)
		defer func() {
			printer.managedEnv = originalManagedEnv
		}()

		// Set empty managed environment variables
		printer.managedEnv = []string{}

		// When setting a managed environment variable
		printer.SetManagedEnv("SET_TEST_VAR1")

		// Then GetManagedEnv should return the variable
		managedEnv := printer.GetManagedEnv()
		if len(managedEnv) != 1 {
			t.Errorf("expected 1 variable, got %d", len(managedEnv))
		}
		if managedEnv[0] != "SET_TEST_VAR1" {
			t.Errorf("expected [SET_TEST_VAR1], got %v", managedEnv)
		}
	})

	t.Run("Dedupe", func(t *testing.T) {
		// Given a new BaseEnvPrinter
		printer, _ := setup(t)

		// And empty managed environment variables
		// Store original managed environment variables
		originalManagedEnv := make([]string, len(printer.managedEnv))
		copy(originalManagedEnv, printer.managedEnv)
		defer func() {
			printer.managedEnv = originalManagedEnv
		}()

		// Set empty managed environment variables
		printer.managedEnv = []string{}

		// When setting duplicate managed environment variables
		printer.SetManagedEnv("SET_TEST_VAR1")
		printer.SetManagedEnv("SET_TEST_VAR1")

		// Then GetManagedEnv should return only one instance
		managedEnv := printer.GetManagedEnv()
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
	setup := func(t *testing.T) (*BaseEnvPrinter, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		printer := NewBaseEnvPrinter(mocks.Injector)
		err := printer.Initialize()
		if err != nil {
			t.Errorf("unexpected error during initialization: %v", err)
		}
		return printer, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a new BaseEnvPrinter
		printer, _ := setup(t)

		// And empty managed aliases
		// Store original managed aliases
		originalManagedAlias := make([]string, len(printer.managedAlias))
		copy(originalManagedAlias, printer.managedAlias)
		defer func() {
			printer.managedAlias = originalManagedAlias
		}()

		// Set empty managed aliases
		printer.managedAlias = []string{}

		// When setting a managed alias
		printer.SetManagedAlias("set_alias1")

		// Then GetManagedAlias should return the alias
		managedAlias := printer.GetManagedAlias()
		if len(managedAlias) != 1 {
			t.Errorf("expected 1 alias, got %d", len(managedAlias))
		}
		if managedAlias[0] != "set_alias1" {
			t.Errorf("expected [set_alias1], got %v", managedAlias)
		}
	})

	t.Run("Dedupe", func(t *testing.T) {
		// Given a new BaseEnvPrinter
		printer, _ := setup(t)

		// And empty managed aliases
		// Store original managed aliases
		originalManagedAlias := make([]string, len(printer.managedAlias))
		copy(originalManagedAlias, printer.managedAlias)
		defer func() {
			printer.managedAlias = originalManagedAlias
		}()

		// Set empty managed aliases
		printer.managedAlias = []string{}

		// When setting duplicate managed aliases
		printer.SetManagedAlias("set_alias1")
		printer.SetManagedAlias("set_alias1")

		// Then GetManagedAlias should return only one instance
		managedAlias := printer.GetManagedAlias()
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
	setup := func(t *testing.T) (*BaseEnvPrinter, *Mocks) {
		t.Helper()
		mocks := setupMocks(t)
		printer := NewBaseEnvPrinter(mocks.Injector)
		err := printer.Initialize()
		if err != nil {
			t.Errorf("unexpected error during initialization: %v", err)
		}
		return printer, mocks
	}

	t.Run("ResetWithNoEnvVars", func(t *testing.T) {
		// Given a new BaseEnvPrinter
		printer, mocks := setup(t)

		// And a mock Reset function
		resetCalled := false
		mocks.Shell.ResetFunc = func(...bool) {
			resetCalled = true
		}

		// When calling Reset
		printer.Reset()

		// Then shell.Reset should be called
		if !resetCalled {
			t.Errorf("expected Shell.Reset to be called, but it wasn't")
		}
	})

	t.Run("ResetWithEnvironmentVariables", func(t *testing.T) {
		// Given a new BaseEnvPrinter with environment variables
		printer, mocks := setup(t)

		// And environment variables set
		os.Setenv("WINDSOR_MANAGED_ENV", "ENV1,ENV2, ENV3")
		os.Setenv("WINDSOR_MANAGED_ALIAS", "alias1,alias2, alias3")
		defer func() {
			os.Unsetenv("WINDSOR_MANAGED_ENV")
			os.Unsetenv("WINDSOR_MANAGED_ALIAS")
		}()

		// And a mock Reset function
		resetCalled := false
		mocks.Shell.ResetFunc = func(...bool) {
			resetCalled = true
		}

		// When calling Reset
		printer.Reset()

		// Then shell.Reset should be called
		if !resetCalled {
			t.Errorf("expected Shell.Reset to be called, but it wasn't")
		}
	})

	t.Run("InternalStateResetsWithReset", func(t *testing.T) {
		// Given a new BaseEnvPrinter with managed environment variables and aliases
		printer, mocks := setup(t)

		// And managed environment variables and aliases set
		printer.SetManagedEnv("TEST_ENV1")
		printer.SetManagedEnv("TEST_ENV2")
		printer.SetManagedAlias("test_alias1")
		printer.SetManagedAlias("test_alias2")

		// And a mock Reset function
		resetCalled := false
		mocks.Shell.ResetFunc = func(...bool) {
			resetCalled = true
		}

		// When calling Reset
		printer.Reset()

		// Then shell.Reset should be called
		if !resetCalled {
			t.Errorf("expected Shell.Reset to be called, but it wasn't")
		}

		// And the managed environment variables should be empty
		managedEnv := printer.GetManagedEnv()
		if len(managedEnv) > 0 {
			t.Errorf("expected GetManagedEnv to be empty, got %v", managedEnv)
		}

		// And the managed aliases should be empty
		managedAlias := printer.GetManagedAlias()
		if len(managedAlias) > 0 {
			t.Errorf("expected GetManagedAlias to be empty, got %v", managedAlias)
		}
	})
}
