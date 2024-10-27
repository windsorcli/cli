package shell

import (
	"fmt"
	"testing"
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
	t.Run("ValidShellTypeCmd", func(t *testing.T) {
		// Given a valid shell type "cmd"
		// When creating a new mock shell
		mockShell := NewMockShell("cmd")
		// Then no error should be returned
		if mockShell == nil {
			t.Errorf("Expected mockShell, got nil")
		}
	})

	t.Run("ValidShellTypePowershell", func(t *testing.T) {
		// Given a valid shell type "powershell"
		// When creating a new mock shell
		mockShell := NewMockShell("powershell")
		// Then no error should be returned
		if mockShell == nil {
			t.Errorf("Expected mockShell, got nil")
		}
	})

	t.Run("ValidShellTypeUnix", func(t *testing.T) {
		// Given a valid shell type "unix"
		// When creating a new mock shell
		mockShell := NewMockShell("unix")
		// Then no error should be returned
		if mockShell == nil {
			t.Errorf("Expected mockShell, got nil")
		}
	})

	t.Run("InvalidShellType", func(t *testing.T) {
		// Given an invalid shell type
		// When creating a new mock shell
		mockShell := NewMockShell("invalid")
		// Then no error should be returned
		if mockShell == nil {
			t.Errorf("Expected mockShell, got nil")
		}
	})
}

func TestMockShell_PrintEnvVars(t *testing.T) {
	envVars := map[string]string{
		"VAR1": "value1",
		"VAR2": "value2",
	}
	wantOutput := "VAR1=value1\nVAR2=value2\n"

	t.Run("DefaultPrintEnvVars", func(t *testing.T) {
		// Given a mock shell with default PrintEnvVars implementation
		mockShell := NewMockShell("cmd")
		// When calling PrintEnvVars
		output := captureStdout(t, func() {
			mockShell.PrintEnvVars(envVars)
		})
		// Then the output should match the expected output
		if output != wantOutput {
			t.Errorf("PrintEnvVars() output = %v, want %v", output, wantOutput)
		}
	})

	t.Run("CustomPrintEnvVars", func(t *testing.T) {
		// Given a mock shell with custom PrintEnvVars implementation
		mockShell := NewMockShell("cmd")
		mockShell.PrintEnvVarsFunc = func(envVars map[string]string) {
			for key, value := range envVars {
				fmt.Printf("%s=%s\n", key, value)
			}
		}
		// When calling PrintEnvVars
		output := captureStdout(t, func() {
			mockShell.PrintEnvVars(envVars)
		})
		// Then the output should match the expected output
		if output != wantOutput {
			t.Errorf("PrintEnvVars() output = %v, want %v", output, wantOutput)
		}
	})
}

func TestMockShell_GetProjectRoot(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock shell that successfully retrieves the project root
		mockShell := NewMockShell("cmd")
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
		mockShell := NewMockShell("cmd")
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
		mockShell := NewMockShell("cmd")
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
}

func TestMockShell_Exec(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock shell with a custom ExecFn implementation
		mockShell, _ := NewMockShell("cmd")
		mockShell.ExecFunc = func(verbose bool, message string, command string, args ...string) (string, error) {
			// Simulate command execution and return a mocked output
			return "mocked output", nil
		}
		// When calling Exec
		output, err := mockShell.Exec(false, "Executing command", "somecommand", "arg1", "arg2")
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
		mockShell, _ := NewMockShell("cmd")
		mockShell.ExecFunc = func(verbose bool, message string, command string, args ...string) (string, error) {
			// Simulate command failure
			return "", fmt.Errorf("execution error")
		}
		// When calling Exec
		output, err := mockShell.Exec(false, "Executing command", "somecommand", "arg1", "arg2")
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
		mockShell := NewMockShell("cmd")
		// When calling Exec
		output, err := mockShell.Exec(false, "Executing command", "somecommand", "arg1", "arg2")
		// Then an error should be returned indicating ExecFn is not implemented
		if err == nil {
			t.Errorf("Expected an error but got none")
		}
		if output != "" {
			t.Errorf("Exec() output = %v, want %v", output, "")
		}
		// Error message should match
		expectedError := "ExecFunc not implemented"
		if err.Error() != expectedError {
			t.Errorf("Exec() error = %v, want %v", err.Error(), expectedError)
		}
	})
}
