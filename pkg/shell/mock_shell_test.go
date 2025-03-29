package shell

import (
	"fmt"
	"strings"
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

func TestMockShell_PrintEnvVars(t *testing.T) {
	envVars := map[string]string{
		"VAR1": "value1",
		"VAR2": "value2",
	}

	t.Run("DefaultPrintEnvVars", func(t *testing.T) {
		injector := di.NewInjector()

		// Given a mock shell with default PrintEnvVars implementation
		mockShell := NewMockShell(injector)
		// When calling PrintEnvVars
		output := captureStdout(t, func() {
			mockShell.PrintEnvVars(envVars)
		})
		// Then the output should be empty as the default implementation does nothing
		if output != "" {
			t.Errorf("PrintEnvVars() output = %v, want %v", output, "")
		}
	})

	t.Run("CustomPrintEnvVars", func(t *testing.T) {
		injector := di.NewInjector()
		// Given a mock shell with custom PrintEnvVars implementation
		mockShell := NewMockShell(injector)
		mockShell.PrintEnvVarsFunc = func(envVars map[string]string) error {
			for key, value := range envVars {
				fmt.Printf("%s=%s\n", key, value)
			}
			return nil
		}
		// When calling PrintEnvVars
		output := captureStdout(t, func() {
			mockShell.PrintEnvVars(envVars)
		})
		// Then the output should contain all expected environment variables
		for key, value := range envVars {
			expectedLine := fmt.Sprintf("%s=%s\n", key, value)
			if !strings.Contains(output, expectedLine) {
				t.Errorf("PrintEnvVars() output missing expected line: %v", expectedLine)
			}
		}
	})
}

func TestMockShell_PrintAlias(t *testing.T) {
	aliasVars := map[string]string{
		"ALIAS1": "command1",
		"ALIAS2": "command2",
	}

	t.Run("DefaultPrintAlias", func(t *testing.T) {
		injector := di.NewInjector()
		// Given a mock shell with default PrintAlias implementation
		mockShell := NewMockShell(injector)
		// When calling PrintAlias
		output := captureStdout(t, func() {
			mockShell.PrintAlias(aliasVars)
		})
		// Then the output should be empty as the default implementation does nothing
		if output != "" {
			t.Errorf("PrintAlias() output = %v, want %v", output, "")
		}
	})

	t.Run("CustomPrintAlias", func(t *testing.T) {
		injector := di.NewInjector()
		// Given a mock shell with custom PrintAlias implementation
		mockShell := NewMockShell(injector)
		mockShell.PrintAliasFunc = func(aliasVars map[string]string) error {
			for key, value := range aliasVars {
				fmt.Printf("%s=%s\n", key, value)
			}
			return nil
		}
		// When calling PrintAlias
		output := captureStdout(t, func() {
			mockShell.PrintAlias(aliasVars)
		})
		// Then the output should contain all expected alias variables
		for key, value := range aliasVars {
			expectedLine := fmt.Sprintf("%s=%s\n", key, value)
			if !strings.Contains(output, expectedLine) {
				t.Errorf("PrintAlias() output missing expected line: %v", expectedLine)
			}
		}
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
		mockShell.ExecFunc = func(command string, args ...string) (string, int, error) {
			// Simulate command execution and return a mocked output
			return "mocked output", 0, nil
		}
		// When calling Exec
		output, _, err := mockShell.Exec("Executing command", "somecommand", "arg1", "arg2")
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
		mockShell.ExecFunc = func(command string, args ...string) (string, int, error) {
			// Simulate command failure
			return "", 1, fmt.Errorf("execution error")
		}
		// When calling Exec
		output, _, err := mockShell.Exec("somecommand", "arg1", "arg2")
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
		output, _, err := mockShell.Exec("Executing command", "somecommand", "arg1", "arg2")
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
		mockShell.ExecSilentFunc = func(command string, args ...string) (string, int, error) {
			return "mocked output", 0, nil
		}
		// When calling ExecSilent
		output, _, err := mockShell.ExecSilent("Executing command", "somecommand", "arg1", "arg2")
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
		output, _, err := mockShell.ExecSilent("Executing command", "somecommand", "arg1", "arg2")
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
		mockShell.ExecProgressFunc = func(message string, command string, args ...string) (string, int, error) {
			return "mocked output", 0, nil
		}
		// When calling ExecProgress
		output, _, err := mockShell.ExecProgress("Executing command", "somecommand", "arg1", "arg2")
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
		output, _, err := mockShell.ExecProgress("Executing command", "somecommand", "arg1", "arg2")
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
		mockShell.ExecSudoFunc = func(message string, command string, args ...string) (string, int, error) {
			return "mocked sudo output", 0, nil
		}
		// When calling ExecSudo
		output, _, err := mockShell.ExecSudo("Executing sudo command", "somecommand", "arg1", "arg2")
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
		output, _, err := mockShell.ExecSudo("Executing sudo command", "somecommand", "arg1", "arg2")
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

// Added test for the new Reset method in MockShell
func TestMockShell_Reset(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		injector := di.NewInjector()
		mockShell := NewMockShell(injector)
		expectedError := "reset error"
		mockShell.ResetFunc = func() error {
			return fmt.Errorf(expectedError)
		}
		err := mockShell.Reset()
		if err == nil || err.Error() != expectedError {
			t.Errorf("Expected error %q, got %v", expectedError, err)
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		injector := di.NewInjector()
		mockShell := NewMockShell(injector)
		err := mockShell.Reset()
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}
