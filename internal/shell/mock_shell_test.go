package shell

import (
	"errors"
	"fmt"
	"testing"
)

func TestNewMockShell(t *testing.T) {
	t.Run("ValidShellTypes", func(t *testing.T) {
		// Given valid shell types
		validShellTypes := []string{"cmd", "powershell", "unix"}
		for _, shellType := range validShellTypes {
			// When creating a new mock shell
			_, err := NewMockShell(shellType)
			// Then it should not return an error
			if err != nil {
				t.Errorf("NewMockShell(%v) returned an error: %v", shellType, err)
			}
		}
	})

	t.Run("InvalidShellType", func(t *testing.T) {
		// Given an invalid shell type
		// When creating a new mock shell
		_, err := NewMockShell("invalid")
		// Then it should return an error
		if err == nil {
			t.Errorf("Expected error for invalid shell type, got nil")
		}
	})
}

func TestMockShell_PrintEnvVars(t *testing.T) {
	t.Run("DefaultPrintEnvVars", func(t *testing.T) {
		// Given environment variables
		envVars := map[string]string{
			"VAR1": "value1",
			"VAR2": "value2",
		}
		wantOutput := "VAR1=value1\nVAR2=value2\n"

		// When creating a new mock shell
		mockShell, err := NewMockShell("cmd")
		if err != nil {
			t.Fatalf("NewMockShell(cmd) error = %v", err)
		}

		// Then the output should match the expected output
		output := captureStdout(t, func() {
			mockShell.PrintEnvVars(envVars)
		})

		if output != wantOutput {
			t.Errorf("PrintEnvVars() output = %v, want %v", output, wantOutput)
		}
	})

	t.Run("CustomPrintEnvVars", func(t *testing.T) {
		// Given environment variables and a custom print function
		envVars := map[string]string{
			"VAR1": "value1",
			"VAR2": "value2",
		}
		wantOutput := "VAR1=value1\nVAR2=value2\n"

		// When creating a new mock shell with a custom print function
		mockShell, err := NewMockShell("cmd")
		if err != nil {
			t.Fatalf("NewMockShell(cmd) error = %v", err)
		}
		mockShell.PrintEnvVarsFn = func(envVars map[string]string) {
			for key, value := range envVars {
				fmt.Printf("%s=%s\n", key, value)
			}
		}

		// Then the output should match the expected output
		output := captureStdout(t, func() {
			mockShell.PrintEnvVars(envVars)
		})

		if output != wantOutput {
			t.Errorf("PrintEnvVars() output = %v, want %v", output, wantOutput)
		}
	})
}

func TestMockShell_GetProjectRoot(t *testing.T) {
	t.Run("SuccessfulProjectRootRetrieval", func(t *testing.T) {
		// Given a mock shell with a successful GetProjectRootFunc
		mockShell := &MockShell{
			GetProjectRootFunc: func() (string, error) {
				return "/mock/project/root", nil
			},
		}
		// When calling GetProjectRoot
		got, err := mockShell.GetProjectRoot()
		// Then it should return the expected project root without an error
		if err != nil {
			t.Errorf("GetProjectRoot() error = %v, wantErr %v", err, false)
			return
		}
		want := "/mock/project/root"
		if got != want {
			t.Errorf("GetProjectRoot() got = %v, want %v", got, want)
		}
	})

	t.Run("ErrorInProjectRootRetrieval", func(t *testing.T) {
		// Given a mock shell with a failing GetProjectRootFunc
		mockShell := &MockShell{
			GetProjectRootFunc: func() (string, error) {
				return "", errors.New("failed to get project root")
			},
		}
		// When calling GetProjectRoot
		_, err := mockShell.GetProjectRoot()
		// Then it should return an error
		if err == nil {
			t.Errorf("Expected error, got nil")
		}
	})

	t.Run("GetProjectRootFuncNotImplemented", func(t *testing.T) {
		// Given a mock shell without a GetProjectRootFunc implementation
		mockShell := &MockShell{}
		// When calling GetProjectRoot
		_, err := mockShell.GetProjectRoot()
		// Then it should return an error
		if err == nil {
			t.Errorf("Expected error, got nil")
		}
	})
}
