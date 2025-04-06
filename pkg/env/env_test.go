package env

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
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
		mocks.Shell.PrintEnvVarsFunc = func(envVars map[string]string) {
			capturedEnvVars = envVars
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
		mocks.Shell.PrintEnvVarsFunc = func(envVars map[string]string) {
			capturedEnvVars = envVars
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
		mocks.Shell.PrintAliasFunc = func(alias map[string]string) {
			capturedAlias = alias
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
		mocks.Shell.PrintAliasFunc = func(alias map[string]string) {
			capturedAlias = alias
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

		// Track calls to UnsetEnvs and UnsetAlias
		var capturedEnvs []string
		var capturedAliases []string
		mocks.Shell.UnsetEnvsFunc = func(envVars []string) {
			capturedEnvs = envVars
		}
		mocks.Shell.UnsetAliasFunc = func(aliases []string) {
			capturedAliases = aliases
		}

		// Make sure environment variables are not set
		os.Unsetenv("WINDSOR_MANAGED_ENV")
		os.Unsetenv("WINDSOR_MANAGED_ALIAS")

		// When calling Reset
		envPrinter.Reset()

		// Then UnsetEnvs and UnsetAlias should not be called
		if capturedEnvs != nil {
			t.Errorf("expected UnsetEnvs not to be called, but it was called with %v", capturedEnvs)
		}
		if capturedAliases != nil {
			t.Errorf("expected UnsetAlias not to be called, but it was called with %v", capturedAliases)
		}
	})

	t.Run("ResetWithEnvironmentVariables", func(t *testing.T) {
		// Given a new BaseEnvPrinter
		mocks := setupEnvMockTests(nil)
		envPrinter := NewBaseEnvPrinter(mocks.Injector)
		err := envPrinter.Initialize()
		if err != nil {
			t.Errorf("unexpected error during initialization: %v", err)
		}

		// Set environment variables
		os.Setenv("WINDSOR_MANAGED_ENV", "ENV1,ENV2, ENV3")
		os.Setenv("WINDSOR_MANAGED_ALIAS", "alias1,alias2, alias3")
		defer func() {
			os.Unsetenv("WINDSOR_MANAGED_ENV")
			os.Unsetenv("WINDSOR_MANAGED_ALIAS")
		}()

		// Track calls to UnsetEnvs and UnsetAlias
		var capturedEnvs []string
		var capturedAliases []string
		mocks.Shell.UnsetEnvsFunc = func(envVars []string) {
			capturedEnvs = envVars
		}
		mocks.Shell.UnsetAliasFunc = func(aliases []string) {
			capturedAliases = aliases
		}

		// When calling Reset
		envPrinter.Reset()

		// Then UnsetEnvs should be called with the correct environment variables
		expectedEnvs := []string{"ENV1", "ENV2", "ENV3"}
		if len(capturedEnvs) != len(expectedEnvs) {
			t.Errorf("expected UnsetEnvs to be called with %v items, got %v", len(expectedEnvs), len(capturedEnvs))
		}
		for _, env := range expectedEnvs {
			if !slices.Contains(capturedEnvs, env) {
				t.Errorf("expected UnsetEnvs to contain %s", env)
			}
		}

		// And UnsetAlias should be called with the correct aliases
		expectedAliases := []string{"alias1", "alias2", "alias3"}
		if len(capturedAliases) != len(expectedAliases) {
			t.Errorf("expected UnsetAlias to be called with %v items, got %v", len(expectedAliases), len(capturedAliases))
		}
		for _, alias := range expectedAliases {
			if !slices.Contains(capturedAliases, alias) {
				t.Errorf("expected UnsetAlias to contain %s", alias)
			}
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

		// Set up some managed environment variables and aliases
		envPrinter.SetManagedEnv("TEST_ENV1")
		envPrinter.SetManagedEnv("TEST_ENV2")
		envPrinter.SetManagedAlias("test_alias1")
		envPrinter.SetManagedAlias("test_alias2")

		// Make sure environment variables are not set
		os.Unsetenv("WINDSOR_MANAGED_ENV")
		os.Unsetenv("WINDSOR_MANAGED_ALIAS")

		// Track calls to UnsetEnvs and UnsetAlias
		var capturedEnvs []string
		var capturedAliases []string
		mocks.Shell.UnsetEnvsFunc = func(envVars []string) {
			capturedEnvs = envVars
		}
		mocks.Shell.UnsetAliasFunc = func(aliases []string) {
			capturedAliases = aliases
		}

		// When calling Reset
		envPrinter.Reset()

		// Then UnsetEnvs and UnsetAlias should not be called with internal values
		if capturedEnvs != nil {
			t.Errorf("expected UnsetEnvs not to be called, but it was called with %v", capturedEnvs)
		}
		if capturedAliases != nil {
			t.Errorf("expected UnsetAlias not to be called, but it was called with %v", capturedAliases)
		}
	})
}

// TestBaseEnvPrinter_WriteResetToken tests the WriteResetToken method of the BaseEnvPrinter struct
func TestBaseEnvPrinter_WriteResetToken(t *testing.T) {
	t.Run("NoSessionToken", func(t *testing.T) {
		// Given a new BaseEnvPrinter
		mocks := setupEnvMockTests(nil)
		envPrinter := NewBaseEnvPrinter(mocks.Injector)
		err := envPrinter.Initialize()
		if err != nil {
			t.Errorf("unexpected error during initialization: %v", err)
		}

		// Save the original environment variable and restore it after the test
		originalSessionToken := os.Getenv("WINDSOR_SESSION_TOKEN")
		defer os.Setenv("WINDSOR_SESSION_TOKEN", originalSessionToken)

		// Ensure the environment variable is not set
		os.Unsetenv("WINDSOR_SESSION_TOKEN")

		// When calling WriteResetToken
		path, err := envPrinter.WriteResetToken()

		// Then no error should be returned and path should be empty
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if path != "" {
			t.Errorf("expected empty path, got %s", path)
		}
	})

	t.Run("SuccessfulTokenWrite", func(t *testing.T) {
		// Given a new BaseEnvPrinter
		mocks := setupEnvMockTests(nil)
		envPrinter := NewBaseEnvPrinter(mocks.Injector)
		err := envPrinter.Initialize()
		if err != nil {
			t.Errorf("unexpected error during initialization: %v", err)
		}

		// Save the original environment variable and restore it after the test
		originalSessionToken := os.Getenv("WINDSOR_SESSION_TOKEN")
		defer os.Setenv("WINDSOR_SESSION_TOKEN", originalSessionToken)

		// Set up mock project root path
		testProjectRoot := "/test/project/root"
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return testProjectRoot, nil
		}

		// Mock MkdirAll and WriteFile functions
		originalMkdirAll := mkdirAll
		originalWriteFile := writeFile

		// Restore original functions after test
		defer func() {
			mkdirAll = originalMkdirAll
			writeFile = originalWriteFile
		}()

		// Track function calls
		mkdirAllCalled := false
		writeFileCalled := false
		expectedDirPath := filepath.Join(testProjectRoot, ".windsor")

		mkdirAll = func(path string, perm os.FileMode) error {
			mkdirAllCalled = true
			if path != expectedDirPath {
				t.Errorf("expected MkdirAll path %s, got %s", expectedDirPath, path)
			}
			if perm != 0750 {
				t.Errorf("expected MkdirAll permissions 0750, got %v", perm)
			}
			return nil
		}

		expectedTestToken := "test-token-123"
		expectedFilePath := filepath.Join(expectedDirPath, SessionTokenPrefix+expectedTestToken)

		writeFile = func(path string, data []byte, perm os.FileMode) error {
			writeFileCalled = true
			if path != expectedFilePath {
				t.Errorf("expected WriteFile path %s, got %s", expectedFilePath, path)
			}
			if len(data) != 0 {
				t.Errorf("expected empty file, got %v bytes", len(data))
			}
			if perm != 0600 {
				t.Errorf("expected WriteFile permissions 0600, got %v", perm)
			}
			return nil
		}

		// Set the environment variable for the test
		os.Setenv("WINDSOR_SESSION_TOKEN", expectedTestToken)

		// When calling WriteResetToken
		path, err := envPrinter.WriteResetToken()

		// Then no error should be returned and path should match expected value
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if path != expectedFilePath {
			t.Errorf("expected path %s, got %s", expectedFilePath, path)
		}

		// Verify that MkdirAll and WriteFile were called
		if !mkdirAllCalled {
			t.Error("expected MkdirAll to be called, but it wasn't")
		}
		if !writeFileCalled {
			t.Error("expected WriteFile to be called, but it wasn't")
		}
	})

	t.Run("ErrorGettingProjectRoot", func(t *testing.T) {
		// Given a new BaseEnvPrinter
		mocks := setupEnvMockTests(nil)
		envPrinter := NewBaseEnvPrinter(mocks.Injector)
		err := envPrinter.Initialize()
		if err != nil {
			t.Errorf("unexpected error during initialization: %v", err)
		}

		// Save the original environment variable and restore it after the test
		originalSessionToken := os.Getenv("WINDSOR_SESSION_TOKEN")
		defer os.Setenv("WINDSOR_SESSION_TOKEN", originalSessionToken)

		// Set up mock to return an error when getting project root
		expectedError := fmt.Errorf("error getting project root")
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "", expectedError
		}

		// Set the environment variable for the test
		os.Setenv("WINDSOR_SESSION_TOKEN", "test-token")

		// When calling WriteResetToken
		path, err := envPrinter.WriteResetToken()

		// Then the expected error should be returned and path should be empty
		if err == nil {
			t.Error("expected error, got nil")
		}
		if !strings.Contains(err.Error(), expectedError.Error()) {
			t.Errorf("expected error containing %q, got %q", expectedError.Error(), err.Error())
		}
		if path != "" {
			t.Errorf("expected empty path, got %s", path)
		}
	})

	t.Run("ErrorCreatingDirectory", func(t *testing.T) {
		// Given a new BaseEnvPrinter
		mocks := setupEnvMockTests(nil)
		envPrinter := NewBaseEnvPrinter(mocks.Injector)
		err := envPrinter.Initialize()
		if err != nil {
			t.Errorf("unexpected error during initialization: %v", err)
		}

		// Save the original environment variable and restore it after the test
		originalSessionToken := os.Getenv("WINDSOR_SESSION_TOKEN")
		defer os.Setenv("WINDSOR_SESSION_TOKEN", originalSessionToken)

		// Set up mock project root path
		testProjectRoot := "/test/project/root"
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return testProjectRoot, nil
		}

		// Mock MkdirAll function to return an error
		originalMkdirAll := mkdirAll
		defer func() {
			mkdirAll = originalMkdirAll
		}()

		expectedError := fmt.Errorf("error creating directory")
		mkdirAll = func(path string, perm os.FileMode) error {
			return expectedError
		}

		// Set the environment variable for the test
		os.Setenv("WINDSOR_SESSION_TOKEN", "test-token")

		// When calling WriteResetToken
		path, err := envPrinter.WriteResetToken()

		// Then the expected error should be returned and path should be empty
		if err == nil {
			t.Error("expected error, got nil")
		}
		if !strings.Contains(err.Error(), expectedError.Error()) {
			t.Errorf("expected error containing %q, got %q", expectedError.Error(), err.Error())
		}
		if path != "" {
			t.Errorf("expected empty path, got %s", path)
		}
	})

	t.Run("ErrorWritingFile", func(t *testing.T) {
		// Given a new BaseEnvPrinter
		mocks := setupEnvMockTests(nil)
		envPrinter := NewBaseEnvPrinter(mocks.Injector)
		err := envPrinter.Initialize()
		if err != nil {
			t.Errorf("unexpected error during initialization: %v", err)
		}

		// Save the original environment variable and restore it after the test
		originalSessionToken := os.Getenv("WINDSOR_SESSION_TOKEN")
		defer os.Setenv("WINDSOR_SESSION_TOKEN", originalSessionToken)

		// Set up mock project root path
		testProjectRoot := "/test/project/root"
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return testProjectRoot, nil
		}

		// Mock MkdirAll and WriteFile functions
		originalMkdirAll := mkdirAll
		originalWriteFile := writeFile

		// Restore original functions after test
		defer func() {
			mkdirAll = originalMkdirAll
			writeFile = originalWriteFile
		}()

		// Mock successful directory creation
		mkdirAll = func(path string, perm os.FileMode) error {
			return nil
		}

		// Mock file writing error
		expectedError := fmt.Errorf("error writing file")
		writeFile = func(path string, data []byte, perm os.FileMode) error {
			return expectedError
		}

		// Set the environment variable for the test
		os.Setenv("WINDSOR_SESSION_TOKEN", "test-token")

		// When calling WriteResetToken
		path, err := envPrinter.WriteResetToken()

		// Then the expected error should be returned and path should be empty
		if err == nil {
			t.Error("expected error, got nil")
		}
		if !strings.Contains(err.Error(), expectedError.Error()) {
			t.Errorf("expected error containing %q, got %q", expectedError.Error(), err.Error())
		}
		if path != "" {
			t.Errorf("expected empty path, got %s", path)
		}
	})
}
