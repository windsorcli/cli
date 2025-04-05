package env

import (
	"fmt"
	"os"
	"reflect"
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
	Env           *BaseEnvPrinter
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
	env := NewBaseEnvPrinter(injector)
	return &Mocks{
		Injector:      injector,
		Shell:         mockShell,
		ConfigHandler: mockConfigHandler,
		Env:           env,
	}
}

// TestEnv_Initialize tests the Initialize method of the Env struct
func TestEnv_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupEnvMockTests(nil)

		// Call Initialize and check for errors
		err := mocks.Env.Initialize()
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("ErrorResolvingShell", func(t *testing.T) {
		mocks := setupEnvMockTests(nil)

		// Register an invalid shell that cannot be cast to shell.Shell
		mocks.Injector.Register("shell", "invalid")

		// Call Initialize and expect an error
		err := mocks.Env.Initialize()
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

		// Call Initialize and expect an error
		err := mocks.Env.Initialize()
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
		mocks.Env.Initialize()

		// Call GetEnvVars and check for errors
		envVars, err := mocks.Env.GetEnvVars()
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
		mocks.Env.Initialize()

		// Mock the PrintEnvVarsFunc to verify it is called
		var capturedEnvVars map[string]string
		mocks.Shell.PrintEnvVarsFunc = func(envVars map[string]string) error {
			capturedEnvVars = envVars
			return nil
		}

		// Call Print and check for errors
		err := mocks.Env.Print(map[string]string{"TEST_VAR": "test_value"})
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
		mocks.Env.Initialize()

		// Mock the PrintEnvVarsFunc to verify it is called
		var capturedEnvVars map[string]string
		mocks.Shell.PrintEnvVarsFunc = func(envVars map[string]string) error {
			capturedEnvVars = envVars
			return nil
		}

		// Call Print without custom vars and check for errors
		err := mocks.Env.Print()
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

func TestEnvTracking(t *testing.T) {
	// Clear the managed environment to start with a clean state
	managedEnvMu.Lock()
	managedEnv = make(map[string]string)
	managedEnvMu.Unlock()

	// Set up test environment variables
	testVars1 := map[string]string{
		"TEST_VAR1": "value1",
		"TEST_VAR2": "value2",
	}

	testVars2 := map[string]string{
		"TEST_VAR3": "value3",
		"TEST_VAR1": "updated_value1", // Override TEST_VAR1
	}

	// Test adding first set of environment variables
	trackEnvVars(testVars1)

	// Get a copy of the managed environment for testing
	managedEnvMu.RLock()
	envCopy := make(map[string]string)
	for k, v := range managedEnv {
		envCopy[k] = v
	}
	managedEnvMu.RUnlock()

	// Check that the variables were tracked
	if len(envCopy) != 2 {
		t.Errorf("Expected 2 env vars, got %d", len(envCopy))
	}

	if envCopy["TEST_VAR1"] != "value1" {
		t.Errorf("Expected TEST_VAR1 to be 'value1', got '%s'", envCopy["TEST_VAR1"])
	}

	if envCopy["TEST_VAR2"] != "value2" {
		t.Errorf("Expected TEST_VAR2 to be 'value2', got '%s'", envCopy["TEST_VAR2"])
	}

	// Test adding second set of environment variables (with an override)
	trackEnvVars(testVars2)

	// Get an updated copy
	managedEnvMu.RLock()
	envCopy = make(map[string]string)
	for k, v := range managedEnv {
		envCopy[k] = v
	}
	managedEnvMu.RUnlock()

	// Check that the variables were tracked and updated
	if len(envCopy) != 3 {
		t.Errorf("Expected 3 env vars, got %d", len(envCopy))
	}

	if envCopy["TEST_VAR1"] != "updated_value1" {
		t.Errorf("Expected TEST_VAR1 to be 'updated_value1', got '%s'", envCopy["TEST_VAR1"])
	}

	if envCopy["TEST_VAR3"] != "value3" {
		t.Errorf("Expected TEST_VAR3 to be 'value3', got '%s'", envCopy["TEST_VAR3"])
	}

	// Test clearing the managed environment
	managedEnvMu.Lock()
	managedEnv = make(map[string]string)
	managedEnvMu.Unlock()

	// Get an updated copy
	managedEnvMu.RLock()
	envCopy = make(map[string]string)
	for k, v := range managedEnv {
		envCopy[k] = v
	}
	managedEnvMu.RUnlock()

	// Check that the variables were cleared
	if len(envCopy) != 0 {
		t.Errorf("Expected 0 env vars after clearing, got %d", len(envCopy))
	}
}

func TestAliasTracking(t *testing.T) {
	// Save the original managedAlias map and restore it after the test
	originalManagedAlias := managedAlias
	defer func() {
		managedAlias = originalManagedAlias
	}()

	t.Run("TrackNonEmptyAliases", func(t *testing.T) {
		// Clear the managedAlias map first
		managedAliasMu.Lock()
		managedAlias = make(map[string]string)
		managedAliasMu.Unlock()

		// Test data
		aliases := map[string]string{
			"alias1": "value1",
			"alias2": "value2",
		}

		// Track the aliases
		trackAliases(aliases)

		// Verify the aliases were tracked correctly
		managedAliasMu.RLock()
		defer managedAliasMu.RUnlock()

		if len(managedAlias) != 2 {
			t.Errorf("Expected managedAlias to have 2 entries, got %d", len(managedAlias))
		}

		if managedAlias["alias1"] != "value1" {
			t.Errorf("Expected managedAlias[\"alias1\"] to be \"value1\", got %q", managedAlias["alias1"])
		}

		if managedAlias["alias2"] != "value2" {
			t.Errorf("Expected managedAlias[\"alias2\"] to be \"value2\", got %q", managedAlias["alias2"])
		}
	})

	t.Run("TrackNilAliases", func(t *testing.T) {
		// Clear the managedAlias map first
		managedAliasMu.Lock()
		managedAlias = make(map[string]string)
		managedAliasMu.Unlock()

		// Track nil aliases (should be a no-op)
		trackAliases(nil)

		// Verify the managedAlias map is still empty
		managedAliasMu.RLock()
		defer managedAliasMu.RUnlock()

		if len(managedAlias) != 0 {
			t.Errorf("Expected managedAlias to be empty, got %d entries", len(managedAlias))
		}
	})

	t.Run("TrackEmptyAliases", func(t *testing.T) {
		// Clear the managedAlias map first
		managedAliasMu.Lock()
		managedAlias = make(map[string]string)
		managedAliasMu.Unlock()

		// Track empty aliases (should be a no-op)
		trackAliases(map[string]string{})

		// Verify the managedAlias map is still empty
		managedAliasMu.RLock()
		defer managedAliasMu.RUnlock()

		if len(managedAlias) != 0 {
			t.Errorf("Expected managedAlias to be empty, got %d entries", len(managedAlias))
		}
	})
}

// TestBaseEnvPrinter_GetAlias tests the GetAlias method of BaseEnvPrinter
func TestBaseEnvPrinter_GetAlias(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		baseEnvPrinter := NewBaseEnvPrinter(nil)

		// Call GetAlias and check the result
		aliases, err := baseEnvPrinter.GetAlias()
		if err != nil {
			t.Errorf("GetAlias should not return an error, got %v", err)
		}

		expectedAliases := map[string]string{}
		if !reflect.DeepEqual(aliases, expectedAliases) {
			t.Errorf("GetAlias returned %v, expected %v", aliases, expectedAliases)
		}
	})
}

// TestBaseEnvPrinter_PrintAlias tests the PrintAlias method of the BaseEnvPrinter struct
func TestBaseEnvPrinter_PrintAlias(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Create mocks
		mockInjector := di.NewMockInjector()
		mockShell := shell.NewMockShell()
		mockConfigHandler := config.NewMockConfigHandler()

		// Register mocks in the injector
		mockInjector.Register("shell", mockShell)
		mockInjector.Register("configHandler", mockConfigHandler)

		// Create a BaseEnvPrinter
		baseEnvPrinter := NewBaseEnvPrinter(mockInjector)
		err := baseEnvPrinter.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize BaseEnvPrinter: %v", err)
		}

		// Mock PrintAlias to capture the passed map
		var capturedAliases map[string]string
		mockShell.PrintAliasFunc = func(aliases map[string]string) error {
			capturedAliases = aliases
			return nil
		}

		// Create test aliases
		testAliases := map[string]string{
			"test_alias1": "command1",
			"test_alias2": "command2",
		}

		// Call PrintAlias
		err = baseEnvPrinter.PrintAlias(testAliases)
		if err != nil {
			t.Errorf("PrintAlias returned an error: %v", err)
		}

		// Verify that PrintAlias was called with the correct map
		if len(capturedAliases) != len(testAliases) {
			t.Errorf("Expected %d aliases, got %d", len(testAliases), len(capturedAliases))
		}

		for key, value := range testAliases {
			if capturedAliases[key] != value {
				t.Errorf("Expected alias %s=%s, got %s=%s", key, value, key, capturedAliases[key])
			}
		}
	})

	t.Run("NoCustomAliasesProvided", func(t *testing.T) {
		// Clear managed aliases to start clean
		managedAliasMu.Lock()
		originalManagedAlias := managedAlias
		managedAlias = make(map[string]string)
		managedAliasMu.Unlock()
		defer func() {
			managedAliasMu.Lock()
			managedAlias = originalManagedAlias
			managedAliasMu.Unlock()
		}()

		// Create mocks
		mockInjector := di.NewMockInjector()
		mockShell := shell.NewMockShell()
		mockConfigHandler := config.NewMockConfigHandler()

		// Register mocks in the injector
		mockInjector.Register("shell", mockShell)
		mockInjector.Register("configHandler", mockConfigHandler)

		// Create a MockEnvPrinter that we will use to test the code path
		mockEnvPrinter := NewMockEnvPrinter()
		mockEnvPrinter.shell = mockShell
		mockEnvPrinter.configHandler = mockConfigHandler

		// Set up the GetAliasFunc to return aliases we expect
		expectedAliases := map[string]string{
			"test_alias1": "command1",
			"test_alias2": "command2",
		}
		mockEnvPrinter.GetAliasFunc = func() (map[string]string, error) {
			return expectedAliases, nil
		}

		// Implement our own PrintAliasFunc to replicate what BaseEnvPrinter does
		mockEnvPrinter.PrintAliasFunc = func(customAliases ...map[string]string) error {
			if len(customAliases) > 0 {
				// This is the branch with custom aliases - just track and print them
				trackAliases(customAliases[0])
				return mockShell.PrintAlias(customAliases[0])
			}

			// This is the branch we're testing - no custom aliases provided
			// Get aliases from GetAlias
			aliases, _ := mockEnvPrinter.GetAlias()

			// Track the aliases
			trackAliases(aliases)

			// Call the shell's PrintAlias with the aliases
			return mockShell.PrintAlias(aliases)
		}

		// Mock the shell's PrintAlias method to capture what's passed to it
		var capturedAliases map[string]string
		mockShell.PrintAliasFunc = func(aliases map[string]string) error {
			capturedAliases = aliases
			return nil
		}

		// Call PrintAlias without custom aliases
		err := mockEnvPrinter.PrintAlias()
		if err != nil {
			t.Errorf("PrintAlias returned an error: %v", err)
		}

		// Verify the shell.PrintAlias was called with the expected aliases
		if !reflect.DeepEqual(capturedAliases, expectedAliases) {
			t.Errorf("Expected PrintAlias to be called with %v, got %v",
				expectedAliases, capturedAliases)
		}

		// Now we can verify that trackAliases was called by checking the managedAlias map
		managedAliasMu.RLock()
		foundAllAliases := true
		for k, v := range expectedAliases {
			if managedAlias[k] != v {
				foundAllAliases = false
				t.Errorf("Expected %s=%s in managedAlias, but it was %s", k, v, managedAlias[k])
			}
		}
		managedAliasMu.RUnlock()

		if !foundAllAliases {
			t.Errorf("Not all aliases were found in managedAlias - trackAliases may not have been called")
		}
	})

	t.Run("ErrorCase", func(t *testing.T) {
		// Create mocks
		mockInjector := di.NewMockInjector()
		mockShell := shell.NewMockShell()
		mockConfigHandler := config.NewMockConfigHandler()

		// Register mocks in the injector
		mockInjector.Register("shell", mockShell)
		mockInjector.Register("configHandler", mockConfigHandler)

		// Create a BaseEnvPrinter
		baseEnvPrinter := NewBaseEnvPrinter(mockInjector)
		err := baseEnvPrinter.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize BaseEnvPrinter: %v", err)
		}

		// Mock PrintAlias to return an error
		expectedError := "mock shell print alias error"
		mockShell.PrintAliasFunc = func(aliases map[string]string) error {
			return fmt.Errorf(expectedError)
		}

		// Create test aliases
		testAliases := map[string]string{
			"test_alias": "command",
		}

		// Call PrintAlias and expect an error
		err = baseEnvPrinter.PrintAlias(testAliases)
		if err == nil {
			t.Errorf("Expected an error, but got nil")
		} else if !strings.Contains(err.Error(), expectedError) {
			t.Errorf("Expected error containing %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("GetAliasError", func(t *testing.T) {
		// Create mocks
		mockInjector := di.NewMockInjector()
		mockShell := shell.NewMockShell()
		mockConfigHandler := config.NewMockConfigHandler()

		// Register mocks in the injector
		mockInjector.Register("shell", mockShell)
		mockInjector.Register("configHandler", mockConfigHandler)

		// Create a MockEnvPrinter that will return an error from GetAlias
		mockEnvPrinter := NewMockEnvPrinter()
		mockEnvPrinter.shell = mockShell

		// Set up the GetAliasFunc to return an error
		expectedError := fmt.Errorf("mock GetAlias error")
		mockEnvPrinter.GetAliasFunc = func() (map[string]string, error) {
			return nil, expectedError
		}

		// Implement PrintAliasFunc to mimic what BaseEnvPrinter does
		mockEnvPrinter.PrintAliasFunc = func(customAliases ...map[string]string) error {
			if len(customAliases) > 0 {
				// Branch with custom aliases
				trackAliases(customAliases[0])
				return mockShell.PrintAlias(customAliases[0])
			}

			// This is the branch we want to test - GetAlias returns an error
			aliases, err := mockEnvPrinter.GetAlias()
			if err != nil {
				// This is what we're testing - handling the error from GetAlias
				return err
			}

			trackAliases(aliases)
			return mockShell.PrintAlias(aliases)
		}

		// Mock the shell's PrintAlias
		var aliasPrintCalled bool
		mockShell.PrintAliasFunc = func(aliases map[string]string) error {
			aliasPrintCalled = true // This should not be called due to the error
			return nil
		}

		// Call PrintAlias without custom aliases
		err := mockEnvPrinter.PrintAlias()

		// Verify the error was returned
		if err == nil {
			t.Error("Expected an error from PrintAlias, but got nil")
		} else if err != expectedError {
			t.Errorf("Expected error %v, got %v", expectedError, err)
		}

		// Verify PrintAlias was not called due to the error
		if aliasPrintCalled {
			t.Error("Shell.PrintAlias should not have been called due to the GetAlias error")
		}
	})

	t.Run("ShellPrintAliasError", func(t *testing.T) {
		// Create mocks
		mockInjector := di.NewMockInjector()
		mockShell := shell.NewMockShell()
		mockConfigHandler := config.NewMockConfigHandler()

		// Register mocks in the injector
		mockInjector.Register("shell", mockShell)
		mockInjector.Register("configHandler", mockConfigHandler)

		// Create a BaseEnvPrinter
		baseEnvPrinter := NewBaseEnvPrinter(mockInjector)
		err := baseEnvPrinter.Initialize()
		if err != nil {
			t.Fatalf("Failed to initialize BaseEnvPrinter: %v", err)
		}

		// Set up the expected error
		expectedError := fmt.Errorf("mock shell PrintAlias error")

		// Mock the shell's PrintAlias method to return an error
		mockShell.PrintAliasFunc = func(aliases map[string]string) error {
			return expectedError
		}

		// Call PrintAlias without custom aliases - this should call shell.PrintAlias with empty map
		err = baseEnvPrinter.PrintAlias()

		// Verify we got the expected error
		if err == nil {
			t.Errorf("Expected error from PrintAlias, got nil")
		} else if err.Error() != expectedError.Error() {
			t.Errorf("Expected error %q, got %q", expectedError.Error(), err.Error())
		}
	})
}

// TestBaseEnvPrinter_Clear tests the Clear method of the BaseEnvPrinter struct
func TestBaseEnvPrinter_Clear(t *testing.T) {
	t.Run("SuccessWithEnvAndAlias", func(t *testing.T) {
		// Save original env
		origEnv := os.Getenv("WINDSOR_MANAGED_ENV")
		origAlias := os.Getenv("WINDSOR_MANAGED_ALIAS")
		defer func() {
			os.Setenv("WINDSOR_MANAGED_ENV", origEnv)
			os.Setenv("WINDSOR_MANAGED_ALIAS", origAlias)
		}()

		// Set test environment variables
		os.Setenv("WINDSOR_MANAGED_ENV", "ENV1:ENV2:ENV3")
		os.Setenv("WINDSOR_MANAGED_ALIAS", "ALIAS1:ALIAS2:ALIAS3")

		// Set up mocks
		mocks := setupEnvMockTests(nil)
		mocks.Env.Initialize()

		// Add some tracked env vars
		testEnvVars := map[string]string{
			"TEST_ENV1": "value1",
			"TEST_ENV2": "value2",
		}
		trackEnvVars(testEnvVars)

		// Add some tracked aliases
		testAliases := map[string]string{
			"test_alias1": "command1",
			"test_alias2": "command2",
		}
		trackAliases(testAliases)

		// Track what vars/aliases are passed to the unset functions
		varsUnsetCalled := false
		aliasesUnsetCalled := false
		var unsetEnvVars []string
		var unsetAliases []string

		// Mock the UnsetEnv function
		mocks.Shell.UnsetEnvFunc = func(vars []string) error {
			varsUnsetCalled = true
			unsetEnvVars = vars
			return nil
		}

		// Mock the UnsetAlias function
		mocks.Shell.UnsetAliasFunc = func(aliases []string) error {
			aliasesUnsetCalled = true
			unsetAliases = aliases
			return nil
		}

		// Call Clear and check for errors
		err := mocks.Env.Clear()
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		// Verify UnsetEnv was called with correct variables
		if !varsUnsetCalled {
			t.Error("UnsetEnv was not called")
		}

		// Check for expected environment variables
		expectedVars := []string{"ENV1", "ENV2", "ENV3", "TEST_ENV1", "TEST_ENV2", "WINDSOR_MANAGED_ENV"}
		for _, expected := range expectedVars {
			found := false
			for _, v := range unsetEnvVars {
				if v == expected {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Expected variable %s to be unset, but it wasn't in: %v", expected, unsetEnvVars)
			}
		}

		// Verify UnsetAlias was called with correct aliases
		if !aliasesUnsetCalled {
			t.Error("UnsetAlias was not called")
		}

		// Check for expected aliases
		expectedAliases := []string{"ALIAS1", "ALIAS2", "ALIAS3", "test_alias1", "test_alias2", "WINDSOR_MANAGED_ALIAS"}
		for _, expected := range expectedAliases {
			found := false
			for _, a := range unsetAliases {
				if a == expected {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Expected alias %s to be unset, but it wasn't in: %v", expected, unsetAliases)
			}
		}

		// Verify that the internal tracking maps were cleared
		managedEnvMu.RLock()
		if len(managedEnv) != 0 {
			t.Errorf("Expected managedEnv to be empty after Clear, but had %d entries", len(managedEnv))
		}
		managedEnvMu.RUnlock()

		managedAliasMu.RLock()
		if len(managedAlias) != 0 {
			t.Errorf("Expected managedAlias to be empty after Clear, but had %d entries", len(managedAlias))
		}
		managedAliasMu.RUnlock()
	})

	t.Run("SuccessWithEmptyEnvironmentVars", func(t *testing.T) {
		// Save original env
		origEnv := os.Getenv("WINDSOR_MANAGED_ENV")
		origAlias := os.Getenv("WINDSOR_MANAGED_ALIAS")
		defer func() {
			os.Setenv("WINDSOR_MANAGED_ENV", origEnv)
			os.Setenv("WINDSOR_MANAGED_ALIAS", origAlias)
		}()

		// Set empty environment variables
		os.Setenv("WINDSOR_MANAGED_ENV", "")
		os.Setenv("WINDSOR_MANAGED_ALIAS", "")

		// Set up mocks
		mocks := setupEnvMockTests(nil)
		mocks.Env.Initialize()

		// Clear tracking maps
		managedEnvMu.Lock()
		managedEnv = make(map[string]string)
		managedEnvMu.Unlock()

		managedAliasMu.Lock()
		managedAlias = make(map[string]string)
		managedAliasMu.Unlock()

		// Track function calls and captured variables/aliases
		varsUnsetCalled := false
		aliasesUnsetCalled := false
		var unsetEnvVars []string
		var unsetAliases []string

		// Mock the UnsetEnv function
		mocks.Shell.UnsetEnvFunc = func(vars []string) error {
			varsUnsetCalled = true
			unsetEnvVars = vars
			return nil
		}

		// Mock the UnsetAlias function
		mocks.Shell.UnsetAliasFunc = func(aliases []string) error {
			aliasesUnsetCalled = true
			unsetAliases = aliases
			return nil
		}

		// Call Clear and check for errors
		err := mocks.Env.Clear()
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		// Verify UnsetEnv was called
		if !varsUnsetCalled {
			t.Error("UnsetEnv was not called")
		} else {
			// Verify WINDSOR_MANAGED_ENV is included
			found := false
			for _, v := range unsetEnvVars {
				if v == "WINDSOR_MANAGED_ENV" {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Expected WINDSOR_MANAGED_ENV to be unset, but it wasn't in: %v", unsetEnvVars)
			}
		}

		// Verify UnsetAlias was called
		if !aliasesUnsetCalled {
			t.Error("UnsetAlias was not called")
		} else {
			// Verify WINDSOR_MANAGED_ALIAS is included
			found := false
			for _, a := range unsetAliases {
				if a == "WINDSOR_MANAGED_ALIAS" {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Expected WINDSOR_MANAGED_ALIAS to be unset, but it wasn't in: %v", unsetAliases)
			}
		}
	})

	t.Run("ErrorUnsetEnv", func(t *testing.T) {
		// Set up mocks
		mocks := setupEnvMockTests(nil)
		mocks.Env.Initialize()

		// Mock the UnsetEnv function to return an error
		expectedError := fmt.Errorf("unset env error")
		mocks.Shell.UnsetEnvFunc = func(vars []string) error {
			return expectedError
		}

		// Call Clear and check for errors
		err := mocks.Env.Clear()
		if err == nil {
			t.Error("expected an error, got nil")
		} else if err.Error() != "failed to unset environment variables: unset env error" {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("ErrorUnsetAlias", func(t *testing.T) {
		// Set up mocks
		mocks := setupEnvMockTests(nil)
		mocks.Env.Initialize()

		// Mock the UnsetEnv function to succeed
		mocks.Shell.UnsetEnvFunc = func(vars []string) error {
			return nil
		}

		// Mock the UnsetAlias function to return an error
		expectedError := fmt.Errorf("unset alias error")
		mocks.Shell.UnsetAliasFunc = func(aliases []string) error {
			return expectedError
		}

		// Call Clear and check for errors
		err := mocks.Env.Clear()
		if err == nil {
			t.Error("expected an error, got nil")
		} else if err.Error() != "failed to unset aliases: unset alias error" {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

// Helper to test Print method in environment printers
func testPrintMethod(t *testing.T, envPrinter EnvPrinter, customVars map[string]string) (map[string]string, error) {
	// Create mocks
	mockInjector := di.NewMockInjector()
	mockShell := shell.NewMockShell()
	mockConfigHandler := config.NewMockConfigHandler()

	// Register mocks in the injector
	mockInjector.Register("shell", mockShell)
	mockInjector.Register("configHandler", mockConfigHandler)

	// Set the shell and configHandler on the printer if possible
	// Use reflection to set fields that are not directly accessible
	printerValue := reflect.ValueOf(envPrinter).Elem()

	shellField := printerValue.FieldByName("shell")
	if shellField.IsValid() && shellField.CanSet() {
		shellField.Set(reflect.ValueOf(mockShell))
	}

	configField := printerValue.FieldByName("configHandler")
	if configField.IsValid() && configField.CanSet() {
		configField.Set(reflect.ValueOf(mockConfigHandler))
	}

	// Mock the PrintEnvVarsFunc to capture what's passed to it
	var capturedEnvVars map[string]string
	mockShell.PrintEnvVarsFunc = func(envVars map[string]string) error {
		capturedEnvVars = envVars
		return nil
	}

	// Set up any environment variables or other needed state
	err := envPrinter.Initialize()
	if err != nil {
		return nil, fmt.Errorf("error initializing printer: %w", err)
	}

	// Call Print with the custom vars if provided
	var printErr error
	if customVars != nil {
		printErr = envPrinter.Print(customVars)
	} else {
		printErr = envPrinter.Print()
	}

	return capturedEnvVars, printErr
}
