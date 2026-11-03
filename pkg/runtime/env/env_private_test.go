package env

import (
	"os"
	"testing"

	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/shell"
	"github.com/windsorcli/cli/pkg/runtime/tools"
)

// =============================================================================
// Test Setup
// =============================================================================

// EnvTestMocks holds all the mock objects used in the tests.
type EnvTestMocks struct {
	ConfigHandler config.ConfigHandler
	Shell         *shell.MockShell
	Shims         *Shims
	ToolsManager  tools.ToolsManager
}

// setupDefaultShims creates a new Shims instance with default implementations
func setupDefaultShims(tmpDir string) *Shims {
	shims := NewShims()

	shims.LookupEnv = func(key string) (string, bool) { return "", false }
	shims.Getenv = func(key string) string { return "" }
	shims.WriteFile = func(name string, data []byte, perm os.FileMode) error { return nil }
	shims.ReadFile = func(name string) ([]byte, error) { return []byte{}, nil }
	shims.MkdirAll = func(path string, perm os.FileMode) error { return nil }
	shims.Stat = func(name string) (os.FileInfo, error) { return nil, nil }
	shims.UserHomeDir = func() (string, error) { return tmpDir, nil }
	shims.Getwd = func() (string, error) { return tmpDir, nil }

	return shims
}

func setupEnvMocks(t *testing.T, overrides ...*EnvTestMocks) *EnvTestMocks {
	t.Helper()

	// Store original directory and create temp dir
	origDir, _ := os.Getwd()

	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to change to temp directory: %v", err)
	}

	// Set project root environment variable
	os.Setenv("WINDSOR_PROJECT_ROOT", tmpDir)

	// Set context default to local
	os.Setenv("WINDSOR_CONTEXT", "local")

	// Create shell with project root matching temp dir
	mockShell := shell.NewMockShell()
	mockShell.GetProjectRootFunc = func() (string, error) {
		return tmpDir, nil
	}

	// Setup shims
	shims := setupDefaultShims(tmpDir)

	// Create mock tools manager
	mockToolsManager := tools.NewMockToolsManager()
	mockToolsManager.GetTerraformCommandFunc = func() string {
		return "terraform"
	}

	// Create initial mocks with defaults
	mocks := &EnvTestMocks{
		Shell:         mockShell,
		ConfigHandler: config.NewConfigHandler(mockShell),
		Shims:         shims,
		ToolsManager:  mockToolsManager,
	}

	// Apply any selective overrides
	if len(overrides) > 0 && overrides[0] != nil {
		override := overrides[0]
		if override.Shell != nil {
			mocks.Shell = override.Shell
		}
		if override.ConfigHandler != nil {
			mocks.ConfigHandler = override.ConfigHandler
		}
		if override.Shims != nil {
			mocks.Shims = override.Shims
		}
		if override.ToolsManager != nil {
			mocks.ToolsManager = override.ToolsManager
		}
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
