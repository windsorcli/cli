package env

import (
	"os"
	"reflect"
	"testing"

	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// =============================================================================
// Test Setup
// =============================================================================

// EnvTestMocks holds all the mock objects used in the tests.
type EnvTestMocks struct {
	ConfigHandler config.ConfigHandler
	Shell         *shell.MockShell
	Shims         *Shims
}

// setupDefaultShims creates a new Shims instance with default implementations
func setupDefaultShims(tmpDir string) *Shims {
	shims := NewShims()

	shims.LookupEnv = func(key string) (string, bool) { return "", false }
	shims.WriteFile = func(name string, data []byte, perm os.FileMode) error { return nil }
	shims.ReadFile = func(name string) ([]byte, error) { return []byte{}, nil }
	shims.MkdirAll = func(path string, perm os.FileMode) error { return nil }
	shims.Stat = func(name string) (os.FileInfo, error) { return nil, nil }
	shims.UserHomeDir = func() (string, error) { return tmpDir, nil }
	shims.Getwd = func() (string, error) { return tmpDir, nil }

	return shims
}

func setupEnvMocks(t *testing.T, opts ...func(*EnvTestMocks)) *EnvTestMocks {
	t.Helper()

	// Store original directory and create temp dir
	origDir, _ := os.Getwd()

	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to change to temp directory: %v", err)
	}

	// Set project root environment variable
	os.Setenv("WINDSOR_PROJECT_ROOT", tmpDir)

	// Set context default to test-context
	os.Setenv("WINDSOR_CONTEXT", "test-context")

	// Create shell with project root matching temp dir
	mockShell := shell.NewMockShell()
	mockShell.GetProjectRootFunc = func() (string, error) {
		return tmpDir, nil
	}

	// Setup shims
	shims := setupDefaultShims(tmpDir)

	// Create initial mocks with defaults
	mocks := &EnvTestMocks{
		Shell:         mockShell,
		ConfigHandler: config.NewConfigHandler(mockShell),
		Shims:         shims,
	}

	// Apply any dependency injection overrides BEFORE using mocks
	for _, opt := range opts {
		opt(mocks)
	}

	// Register cleanup to restore original state
	t.Cleanup(func() {
		os.Unsetenv("WINDSOR_PROJECT_ROOT")
		os.Unsetenv("WINDSOR_CONTEXT")
		if err := os.Chdir(origDir); err != nil {
			t.Logf("Warning: Failed to change back to original directory: %v", err)
		}
	})

	return mocks
}

// =============================================================================
// Test Public Methods
// =============================================================================

// TestEnv_NewBaseEnvPrinter tests the NewBaseEnvPrinter constructor
func TestEnv_NewBaseEnvPrinter(t *testing.T) {
	setup := func(t *testing.T) (*BaseEnvPrinter, *EnvTestMocks) {
		t.Helper()
		mocks := setupEnvMocks(t)
		printer := NewBaseEnvPrinter(mocks.Shell, mocks.ConfigHandler)
		return printer, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a new BaseEnvPrinter
		printer, _ := setup(t)

		// Then it should be created
		if printer == nil {
			t.Error("Expected printer to be created")
		}
	})

	t.Run("WithValidDependencies", func(t *testing.T) {
		// Given a new BaseEnvPrinter with valid shell and configHandler
		mocks := setupEnvMocks(t)
		printer := NewBaseEnvPrinter(mocks.Shell, mocks.ConfigHandler)

		// Then it should be created successfully
		if printer == nil {
			t.Error("Expected printer to be created")
		}
	})
}

// TestBaseEnvPrinter_GetEnvVars tests the GetEnvVars method of the BaseEnvPrinter struct
func TestBaseEnvPrinter_GetEnvVars(t *testing.T) {
	setup := func(t *testing.T) (*BaseEnvPrinter, *EnvTestMocks) {
		t.Helper()
		mocks := setupEnvMocks(t)
		printer := NewBaseEnvPrinter(mocks.Shell, mocks.ConfigHandler)
		return printer, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a new BaseEnvPrinter
		printer, _ := setup(t)

		// When calling GetEnvVars
		envVars, _ := printer.GetEnvVars()

		// Then no error should be returned and envVars should be empty
		expectedEnvVars := map[string]string{}
		if !reflect.DeepEqual(envVars, expectedEnvVars) {
			t.Errorf("envVars = %v, want %v", envVars, expectedEnvVars)
		}
	})
}

// TestBaseEnvPrinter_GetManagedEnv tests the GetManagedEnv method of the BaseEnvPrinter struct
func TestBaseEnvPrinter_GetManagedEnv(t *testing.T) {
	setup := func(t *testing.T) (*BaseEnvPrinter, *EnvTestMocks) {
		t.Helper()
		mocks := setupEnvMocks(t)
		printer := NewBaseEnvPrinter(mocks.Shell, mocks.ConfigHandler)
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

// TestBaseEnvPrinter_GetAlias tests the GetAlias method of the BaseEnvPrinter struct
func TestBaseEnvPrinter_GetAlias(t *testing.T) {
	setup := func(t *testing.T) (*BaseEnvPrinter, *EnvTestMocks) {
		t.Helper()
		mocks := setupEnvMocks(t)
		printer := NewBaseEnvPrinter(mocks.Shell, mocks.ConfigHandler)
		return printer, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a new BaseEnvPrinter
		printer, _ := setup(t)

		// When calling GetAlias
		aliasMap, err := printer.GetAlias()

		// Then no error should be returned and aliasMap should be empty
		if err != nil {
			t.Errorf("GetAlias() error = %v, want nil", err)
		}
		expectedAliasMap := map[string]string{}
		if !reflect.DeepEqual(aliasMap, expectedAliasMap) {
			t.Errorf("aliasMap = %v, want %v", aliasMap, expectedAliasMap)
		}
	})
}

// TestBaseEnvPrinter_GetManagedAlias tests the GetManagedAlias method of the BaseEnvPrinter struct
func TestBaseEnvPrinter_GetManagedAlias(t *testing.T) {
	setup := func(t *testing.T) (*BaseEnvPrinter, *EnvTestMocks) {
		t.Helper()
		mocks := setupEnvMocks(t)
		printer := NewBaseEnvPrinter(mocks.Shell, mocks.ConfigHandler)
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
	setup := func(t *testing.T) (*BaseEnvPrinter, *EnvTestMocks) {
		t.Helper()
		mocks := setupEnvMocks(t)
		printer := NewBaseEnvPrinter(mocks.Shell, mocks.ConfigHandler)
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
	setup := func(t *testing.T) (*BaseEnvPrinter, *EnvTestMocks) {
		t.Helper()
		mocks := setupEnvMocks(t)
		printer := NewBaseEnvPrinter(mocks.Shell, mocks.ConfigHandler)
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
	setup := func(t *testing.T) (*BaseEnvPrinter, *EnvTestMocks) {
		t.Helper()
		mocks := setupEnvMocks(t)
		printer := NewBaseEnvPrinter(mocks.Shell, mocks.ConfigHandler)
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
