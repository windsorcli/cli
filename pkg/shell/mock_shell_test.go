package shell

import (
	"fmt"
	"testing"

	"github.com/windsorcli/cli/pkg/di"
)

// Helper function for error assertion
func assertError(t *testing.T, err error, shouldError bool) {
	if shouldError && err == nil {
		t.Errorf("Expected error, got nil")
	} else if !shouldError && err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestMockShell_NewMockShell(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock injector
		mockInjector := di.NewMockInjector()
		// When creating a new mock shell with the injector
		mockShell := NewMockShell(mockInjector)
		// Then no error should be returned and the injector should be set
		if mockShell == nil {
			t.Errorf("Expected mockShell, got nil")
		}
		if mockShell.injector != mockInjector {
			t.Errorf("Expected injector to be set, got %v", mockShell.injector)
		}
	})
}

func TestMockShell_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock shell with a custom Initialize function
		mockShell := NewMockShell()
		mockShell.InitializeFunc = func() error {
			return nil
		}

		// When calling Initialize
		err := mockShell.Initialize()

		// Then no error should be returned
		assertError(t, err, false)
	})

	t.Run("DefaultInitialize", func(t *testing.T) {
		// Given a mock shell without a custom Initialize function
		mockShell := NewMockShell()

		// When calling Initialize
		err := mockShell.Initialize()

		// Then no error should be returned as the default implementation does nothing
		assertError(t, err, false)
	})
}

func TestMockShell_GetProjectRoot(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock shell that successfully retrieves the project root
		injector := di.NewInjector()
		mockShell := NewMockShell(injector)
		mockShell.GetProjectRootFunc = func() (string, error) {
			return "/mock/project/root", nil
		}
		// When calling GetProjectRoot
		got, err := mockShell.GetProjectRoot()
		// Then the project root should be returned without error
		want := "/mock/project/root"
		if err != nil || got != want {
			t.Errorf("GetProjectRoot() got = %v, want %v, err = %v", got, want, err)
		}
	})

	t.Run("Error", func(t *testing.T) {
		// Given a mock shell that returns an error when retrieving the project root
		injector := di.NewInjector()
		mockShell := NewMockShell(injector)
		mockShell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("error retrieving project root")
		}
		// When calling GetProjectRoot
		got, err := mockShell.GetProjectRoot()
		// Then an error should be returned
		if err == nil {
			t.Errorf("Expected an error but got none")
		}
		if got != "" {
			t.Errorf("GetProjectRoot() got = %v, want %v", got, "")
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock shell with no GetProjectRoot implementation
		injector := di.NewInjector()
		mockShell := NewMockShell(injector)
		// When calling GetProjectRoot
		got, err := mockShell.GetProjectRoot()
		// Then no error should be returned and the result should be empty
		if err != nil {
			t.Errorf("GetProjectRoot() error = %v, want nil", err)
		}
		if got != "" {
			t.Errorf("GetProjectRoot() got = %v, want %v", got, "")
		}
	})
}

func TestMockShell_Exec(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock shell with a custom ExecFn implementation
		injector := di.NewInjector()
		mockShell := NewMockShell(injector)
		mockShell.ExecFunc = func(command string, args ...string) (string, error) {
			// Simulate command execution and return a mocked output
			return "mocked output", nil
		}
		// When calling Exec
		output, err := mockShell.Exec("Executing command", "somecommand", "arg1", "arg2")
		// Then no error should be returned and output should be as expected
		expectedOutput := "mocked output"
		if err != nil {
			t.Errorf("Exec() error = %v, want nil", err)
		}
		if output != expectedOutput {
			t.Errorf("Exec() output = %v, want %v", output, expectedOutput)
		}
	})

	t.Run("Error", func(t *testing.T) {
		// Given a mock shell whose ExecFn returns an error
		injector := di.NewInjector()
		mockShell := NewMockShell(injector)
		mockShell.ExecFunc = func(command string, args ...string) (string, error) {
			// Simulate command failure
			return "", fmt.Errorf("execution error")
		}
		// When calling Exec
		output, err := mockShell.Exec("somecommand", "arg1", "arg2")
		// Then an error should be returned
		if err == nil {
			t.Errorf("Expected an error but got none")
		}
		if output != "" {
			t.Errorf("Exec() output = %v, want %v", output, "")
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock shell with no ExecFn implementation
		injector := di.NewInjector()
		mockShell := NewMockShell(injector)
		// When calling Exec
		output, err := mockShell.Exec("Executing command", "somecommand", "arg1", "arg2")
		// Then no error should be returned and the result should be empty
		if err != nil {
			t.Errorf("Exec() error = %v, want nil", err)
		}
		if output != "" {
			t.Errorf("Exec() output = %v, want %v", output, "")
		}
	})
}

func TestMockShell_ExecSilent(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock shell with a custom ExecSilentFn implementation
		injector := di.NewInjector()
		mockShell := NewMockShell(injector)
		mockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			return "mocked output", nil
		}
		// When calling ExecSilent
		output, err := mockShell.ExecSilent("Executing command", "somecommand", "arg1", "arg2")
		// Then no error should be returned and output should be as expected
		expectedOutput := "mocked output"
		if err != nil {
			t.Errorf("ExecSilent() error = %v, want nil", err)
		}
		if output != expectedOutput {
			t.Errorf("ExecSilent() output = %v, want %v", output, expectedOutput)
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock shell with no ExecSilentFn implementation
		injector := di.NewInjector()
		mockShell := NewMockShell(injector)
		// When calling ExecSilent
		output, err := mockShell.ExecSilent("Executing command", "somecommand", "arg1", "arg2")
		// Then no error should be returned and the result should be empty
		if err != nil {
			t.Errorf("ExecSilent() error = %v, want nil", err)
		}
		if output != "" {
			t.Errorf("ExecSilent() output = %v, want %v", output, "")
		}
	})
}

func TestMockShell_ExecProgress(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock shell with a custom ExecProgressFn implementation
		injector := di.NewInjector()
		mockShell := NewMockShell(injector)
		mockShell.ExecProgressFunc = func(message string, command string, args ...string) (string, error) {
			return "mocked output", nil
		}
		// When calling ExecProgress
		output, err := mockShell.ExecProgress("Executing command", "somecommand", "arg1", "arg2")
		// Then no error should be returned and output should be as expected
		expectedOutput := "mocked output"
		if err != nil {
			t.Errorf("ExecProgress() error = %v, want nil", err)
		}
		if output != expectedOutput {
			t.Errorf("ExecProgress() output = %v, want %v", output, expectedOutput)
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock shell with no ExecProgressFn implementation
		injector := di.NewInjector()
		mockShell := NewMockShell(injector)
		// When calling ExecProgress
		output, err := mockShell.ExecProgress("Executing command", "somecommand", "arg1", "arg2")
		// Then no error should be returned and the result should be empty
		if err != nil {
			t.Errorf("ExecProgress() error = %v, want nil", err)
		}
		if output != "" {
			t.Errorf("ExecProgress() output = %v, want %v", output, "")
		}
	})
}

func TestMockShell_ExecSudo(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock shell with a custom ExecSudoFn implementation
		injector := di.NewInjector()
		mockShell := NewMockShell(injector)
		mockShell.ExecSudoFunc = func(message string, command string, args ...string) (string, error) {
			return "mocked sudo output", nil
		}
		// When calling ExecSudo
		output, err := mockShell.ExecSudo("Executing sudo command", "somecommand", "arg1", "arg2")
		// Then no error should be returned and output should be as expected
		expectedOutput := "mocked sudo output"
		if err != nil {
			t.Errorf("ExecSudo() error = %v, want nil", err)
		}
		if output != expectedOutput {
			t.Errorf("ExecSudo() output = %v, want %v", output, expectedOutput)
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock shell with no ExecSudoFn implementation
		injector := di.NewInjector()
		mockShell := NewMockShell(injector)
		// When calling ExecSudo
		output, err := mockShell.ExecSudo("Executing sudo command", "somecommand", "arg1", "arg2")
		// Then no error should be returned and the result should be empty
		if err != nil {
			t.Errorf("ExecSudo() error = %v, want nil", err)
		}
		if output != "" {
			t.Errorf("ExecSudo() output = %v, want %v", output, "")
		}
	})
}

func TestMockShell_InstallHook(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock shell with a custom InstallHookFunc implementation
		injector := di.NewInjector()
		mockShell := NewMockShell(injector)
		mockShell.InstallHookFunc = func(shellName string) error {
			return nil
		}
		// When calling InstallHook
		err := mockShell.InstallHook("bash")
		// Then no error should be returned
		if err != nil {
			t.Errorf("InstallHook() error = %v, want nil", err)
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock shell with no InstallHookFunc implementation
		mockShell := NewMockShell()
		// When calling InstallHook
		err := mockShell.InstallHook("bash")
		// Then no error should be returned
		assertError(t, err, false)
	})
}

func TestMockShell_SetVerbosity(t *testing.T) {
	t.Run("SetVerbosityTrue", func(t *testing.T) {
		// Given a mock shell with a custom SetVerbosityFunc implementation
		injector := di.NewInjector()
		mockShell := NewMockShell(injector)
		var verbositySet bool
		mockShell.SetVerbosityFunc = func(verbose bool) {
			verbositySet = verbose
		}
		// When setting verbosity to true
		mockShell.SetVerbosity(true)
		// Then verbosity should be set to true
		if !verbositySet {
			t.Errorf("Expected verbosity to be set to true, but it was not")
		}
	})

	t.Run("SetVerbosityFalse", func(t *testing.T) {
		// Given a mock shell with a custom SetVerbosityFunc implementation
		injector := di.NewInjector()
		mockShell := NewMockShell(injector)
		var verbositySet bool
		mockShell.SetVerbosityFunc = func(verbose bool) {
			verbositySet = verbose
		}
		// When setting verbosity to false
		mockShell.SetVerbosity(false)
		// Then verbosity should be set to false
		if verbositySet {
			t.Errorf("Expected verbosity to be set to false, but it was not")
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock shell with no SetVerbosityFunc implementation
		injector := di.NewInjector()
		mockShell := NewMockShell(injector)
		// When calling SetVerbosity
		mockShell.SetVerbosity(true)
		// Then no panic or error should occur
	})
}

func TestMockShell_AddCurrentDirToTrustedFile(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock shell with a custom AddCurrentDirToTrustedFileFunc implementation
		injector := di.NewInjector()
		mockShell := NewMockShell(injector)
		mockShell.AddCurrentDirToTrustedFileFunc = func() error {
			return nil
		}
		// When calling AddCurrentDirToTrustedFile
		err := mockShell.AddCurrentDirToTrustedFile()
		// Then no error should be returned
		assertError(t, err, false)
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock shell with no AddCurrentDirToTrustedFileFunc implementation
		injector := di.NewInjector()
		mockShell := NewMockShell(injector)
		// When calling AddCurrentDirToTrustedFile
		err := mockShell.AddCurrentDirToTrustedFile()
		// Then no error should be returned
		assertError(t, err, false)
	})
}

func TestMockShell_CheckTrustedDirectory(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock shell with a custom CheckTrustedDirectoryFunc implementation
		injector := di.NewInjector()
		mockShell := NewMockShell(injector)
		mockShell.CheckTrustedDirectoryFunc = func() error {
			return nil
		}

		// When calling CheckTrustedDirectory
		err := mockShell.CheckTrustedDirectory()

		// Then no error should be returned
		assertError(t, err, false)
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock shell with no CheckTrustedDirectoryFunc implementation
		injector := di.NewInjector()
		mockShell := NewMockShell(injector)

		// When calling CheckTrustedDirectory
		err := mockShell.CheckTrustedDirectory()

		// Then no error should be returned
		assertError(t, err, false)
	})
}

func TestMockShell_UnsetEnvs(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock shell with a custom UnsetEnvsFunc implementation
		injector := di.NewInjector()
		mockShell := NewMockShell(injector)

		// Track which variables were passed to the function
		var capturedEnvVars []string
		mockShell.UnsetEnvsFunc = func(envVars []string) {
			capturedEnvVars = envVars
		}

		// When calling UnsetEnvs with a list of environment variables
		envVarsToUnset := []string{"VAR1", "VAR2", "VAR3"}
		mockShell.UnsetEnvs(envVarsToUnset)

		// Then the function should be called with the correct variables
		if len(capturedEnvVars) != len(envVarsToUnset) {
			t.Errorf("Expected %d env vars, got %d", len(envVarsToUnset), len(capturedEnvVars))
		}

		for i, envVar := range envVarsToUnset {
			if capturedEnvVars[i] != envVar {
				t.Errorf("Expected env var %s at position %d, got %s", envVar, i, capturedEnvVars[i])
			}
		}
	})

	t.Run("EmptyList", func(t *testing.T) {
		// Given a mock shell with a custom UnsetEnvsFunc implementation
		injector := di.NewInjector()
		mockShell := NewMockShell(injector)

		// Track if the function was called
		functionCalled := false
		mockShell.UnsetEnvsFunc = func(envVars []string) {
			functionCalled = true
		}

		// When calling UnsetEnvs with an empty list
		mockShell.UnsetEnvs([]string{})

		// Then the function should still be called
		if !functionCalled {
			t.Errorf("Expected UnsetEnvsFunc to be called even with empty list")
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock shell with no UnsetEnvsFunc implementation
		injector := di.NewInjector()
		mockShell := NewMockShell(injector)

		// When calling UnsetEnvs
		// Then no error should occur (function should not panic)
		mockShell.UnsetEnvs([]string{"VAR1", "VAR2"})
	})
}

func TestMockShell_UnsetAlias(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock shell with a custom UnsetAliasFunc implementation
		injector := di.NewInjector()
		mockShell := NewMockShell(injector)

		// Track which aliases were passed to the function
		var capturedAliases []string
		mockShell.UnsetAliasFunc = func(aliases []string) {
			capturedAliases = aliases
		}

		// When calling UnsetAlias with a list of aliases
		aliasesToUnset := []string{"alias1", "alias2", "alias3"}
		mockShell.UnsetAlias(aliasesToUnset)

		// Then the function should be called with the correct aliases
		if len(capturedAliases) != len(aliasesToUnset) {
			t.Errorf("Expected %d aliases, got %d", len(aliasesToUnset), len(capturedAliases))
		}

		for i, alias := range aliasesToUnset {
			if capturedAliases[i] != alias {
				t.Errorf("Expected alias %s at position %d, got %s", alias, i, capturedAliases[i])
			}
		}
	})

	t.Run("EmptyList", func(t *testing.T) {
		// Given a mock shell with a custom UnsetAliasFunc implementation
		injector := di.NewInjector()
		mockShell := NewMockShell(injector)

		// Track if the function was called
		functionCalled := false
		mockShell.UnsetAliasFunc = func(aliases []string) {
			functionCalled = true
		}

		// When calling UnsetAlias with an empty list
		mockShell.UnsetAlias([]string{})

		// Then the function should still be called
		if !functionCalled {
			t.Errorf("Expected UnsetAliasFunc to be called even with empty list")
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock shell with no UnsetAliasFunc implementation
		injector := di.NewInjector()
		mockShell := NewMockShell(injector)

		// When calling UnsetAlias
		// Then no error should occur (function should not panic)
		mockShell.UnsetAlias([]string{"alias1", "alias2"})
	})
}

func TestMockShell_PrintEnvVars(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock shell with a custom PrintEnvVarsFunc implementation
		injector := di.NewInjector()
		mockShell := NewMockShell(injector)

		// Track which environment variables were passed to the function
		var capturedEnvVars map[string]string
		mockShell.PrintEnvVarsFunc = func(envVars map[string]string) {
			capturedEnvVars = envVars
		}

		// When calling PrintEnvVars with a map of environment variables
		envVarsToPrint := map[string]string{
			"VAR1": "value1",
			"VAR2": "value2",
			"VAR3": "value3",
		}
		mockShell.PrintEnvVars(envVarsToPrint)

		// Then the function should be called with the correct variables
		if len(capturedEnvVars) != len(envVarsToPrint) {
			t.Errorf("Expected %d env vars, got %d", len(envVarsToPrint), len(capturedEnvVars))
		}

		for key, value := range envVarsToPrint {
			capturedValue, exists := capturedEnvVars[key]
			if !exists {
				t.Errorf("Expected env var %s to be passed to PrintEnvVarsFunc, but it wasn't", key)
			}
			if capturedValue != value {
				t.Errorf("Expected env var %s to have value %s, got %s", key, value, capturedValue)
			}
		}
	})

	t.Run("EmptyMap", func(t *testing.T) {
		// Given a mock shell with a custom PrintEnvVarsFunc implementation
		injector := di.NewInjector()
		mockShell := NewMockShell(injector)

		// Track if the function was called
		functionCalled := false
		mockShell.PrintEnvVarsFunc = func(envVars map[string]string) {
			functionCalled = true
			if len(envVars) != 0 {
				t.Errorf("Expected empty map, got map with %d elements", len(envVars))
			}
		}

		// When calling PrintEnvVars with an empty map
		mockShell.PrintEnvVars(map[string]string{})

		// Then the function should still be called
		if !functionCalled {
			t.Errorf("Expected PrintEnvVarsFunc to be called even with empty map")
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock shell with no PrintEnvVarsFunc implementation
		injector := di.NewInjector()
		mockShell := NewMockShell(injector)

		// When calling PrintEnvVars
		// Then no error should occur (function should not panic)
		mockShell.PrintEnvVars(map[string]string{"VAR1": "value1"})
	})
}

func TestMockShell_PrintAlias(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock shell with a custom PrintAliasFunc implementation
		injector := di.NewInjector()
		mockShell := NewMockShell(injector)

		// Track which aliases were passed to the function
		var capturedAliases map[string]string
		mockShell.PrintAliasFunc = func(aliases map[string]string) {
			capturedAliases = aliases
		}

		// When calling PrintAlias with a map of aliases
		aliasesToPrint := map[string]string{
			"alias1": "command1",
			"alias2": "command2",
			"alias3": "command3",
		}
		mockShell.PrintAlias(aliasesToPrint)

		// Then the function should be called with the correct aliases
		if len(capturedAliases) != len(aliasesToPrint) {
			t.Errorf("Expected %d aliases, got %d", len(aliasesToPrint), len(capturedAliases))
		}

		for key, value := range aliasesToPrint {
			capturedValue, exists := capturedAliases[key]
			if !exists {
				t.Errorf("Expected alias %s to be passed to PrintAliasFunc, but it wasn't", key)
			}
			if capturedValue != value {
				t.Errorf("Expected alias %s to have value %s, got %s", key, value, capturedValue)
			}
		}
	})

	t.Run("EmptyMap", func(t *testing.T) {
		// Given a mock shell with a custom PrintAliasFunc implementation
		injector := di.NewInjector()
		mockShell := NewMockShell(injector)

		// Track if the function was called
		functionCalled := false
		mockShell.PrintAliasFunc = func(aliases map[string]string) {
			functionCalled = true
			if len(aliases) != 0 {
				t.Errorf("Expected empty map, got map with %d elements", len(aliases))
			}
		}

		// When calling PrintAlias with an empty map
		mockShell.PrintAlias(map[string]string{})

		// Then the function should still be called
		if !functionCalled {
			t.Errorf("Expected PrintAliasFunc to be called even with empty map")
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock shell with no PrintAliasFunc implementation
		injector := di.NewInjector()
		mockShell := NewMockShell(injector)

		// When calling PrintAlias
		// Then no error should occur (function should not panic)
		mockShell.PrintAlias(map[string]string{"alias1": "command1"})
	})
}
