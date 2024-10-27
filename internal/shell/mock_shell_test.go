package shell

import (
	"errors"
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
		_, err := NewMockShell("cmd")
		// Then no error should be returned
		assertError(t, err, false)
	})

	t.Run("ValidShellTypePowershell", func(t *testing.T) {
		// Given a valid shell type "powershell"
		// When creating a new mock shell
		_, err := NewMockShell("powershell")
		// Then no error should be returned
		assertError(t, err, false)
	})

	t.Run("ValidShellTypeUnix", func(t *testing.T) {
		// Given a valid shell type "unix"
		// When creating a new mock shell
		_, err := NewMockShell("unix")
		// Then no error should be returned
		assertError(t, err, false)
	})

	t.Run("InvalidShellType", func(t *testing.T) {
		// Given an invalid shell type
		// When creating a new mock shell
		_, err := NewMockShell("invalid")
		// Then an error should be returned
		assertError(t, err, true)
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
		mockShell, _ := NewMockShell("cmd")
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
		mockShell, _ := NewMockShell("cmd")
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
		mockShell, _ := NewMockShell("cmd")
		mockShell.GetProjectRootFunc = func() (string, error) {
			return "/mock/project/root", nil
		}
		// When calling GetProjectRoot
		got, err := mockShell.GetProjectRoot()
		// Then the project root should be returned without error
		assertError(t, err, false)
		want := "/mock/project/root"
		if got != want {
			t.Errorf("GetProjectRoot() got = %v, want %v", got, want)
		}
	})

	t.Run("Error", func(t *testing.T) {
		// Given a mock shell that returns an error when retrieving the project root
		mockShell, _ := NewMockShell("cmd")
		mockShell.GetProjectRootFunc = func() (string, error) {
			return "", errors.New("failed to get project root")
		}
		// When calling GetProjectRoot
		_, err := mockShell.GetProjectRoot()
		// Then an error should be returned
		assertError(t, err, true)
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock shell with no GetProjectRoot implementation
		mockShell, _ := NewMockShell("cmd")
		// When calling GetProjectRoot
		_, err := mockShell.GetProjectRoot()
		// Then an error should be returned
		assertError(t, err, true)
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
		assertError(t, err, false)
		expectedOutput := "mocked output"
		if output != expectedOutput {
			t.Errorf("Exec() output = %v, want %v", output, expectedOutput)
		}
	})

	t.Run("Error", func(t *testing.T) {
		// Given a mock shell whose ExecFn returns an error
		mockShell, _ := NewMockShell("cmd")
		mockShell.ExecFunc = func(verbose bool, message string, command string, args ...string) (string, error) {
			// Simulate command failure
			return "", errors.New("mocked error")
		}
		// When calling Exec
		output, err := mockShell.Exec(false, "Executing command", "somecommand", "arg1", "arg2")
		// Then an error should be returned
		assertError(t, err, true)
		// And output should be empty
		if output != "" {
			t.Errorf("Exec() output = %v, want %v", output, "")
		}
		// Error message should match
		expectedError := "mocked error"
		if err.Error() != expectedError {
			t.Errorf("Exec() error = %v, want %v", err.Error(), expectedError)
		}
	})
	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock shell with no ExecFn implementation
		mockShell, _ := NewMockShell("cmd")
		// When calling Exec
		output, err := mockShell.Exec(false, "Executing command", "somecommand", "arg1", "arg2")
		// Then an error should be returned indicating ExecFn is not implemented
		assertError(t, err, true)
		// And output should be empty
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
