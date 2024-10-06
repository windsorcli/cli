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

func TestMockShell(t *testing.T) {
	t.Run("NewMockShell", func(t *testing.T) {
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
	})

	t.Run("PrintEnvVars", func(t *testing.T) {
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
			mockShell.PrintEnvVarsFn = func(envVars map[string]string) {
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
	})

	t.Run("GetProjectRoot", func(t *testing.T) {
		t.Run("SuccessfulProjectRootRetrieval", func(t *testing.T) {
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

		t.Run("ErrorInProjectRootRetrieval", func(t *testing.T) {
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

		t.Run("GetProjectRootFuncNotImplemented", func(t *testing.T) {
			// Given a mock shell with no GetProjectRoot implementation
			mockShell, _ := NewMockShell("cmd")
			// When calling GetProjectRoot
			_, err := mockShell.GetProjectRoot()
			// Then an error should be returned
			assertError(t, err, true)
		})
	})
}
