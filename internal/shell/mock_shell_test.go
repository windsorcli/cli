package shell

import (
	"errors"
	"fmt"
	"testing"
)

// Helper function to create a new mock shell
func createMockShell(t *testing.T, shellType string) *MockShell {
	mockShell, err := NewMockShell(shellType)
	if err != nil {
		t.Fatalf("NewMockShell(%s) error = %v", shellType, err)
	}
	return mockShell
}

// Helper function for error assertion
func assertError(t *testing.T, err error, shouldError bool) {
	if shouldError && err == nil {
		t.Errorf("Expected error, got nil")
	} else if !shouldError && err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestNewMockShell(t *testing.T) {
	t.Run("ValidShellTypeCmd", func(t *testing.T) {
		_, err := NewMockShell("cmd")
		assertError(t, err, false)
	})

	t.Run("ValidShellTypePowershell", func(t *testing.T) {
		_, err := NewMockShell("powershell")
		assertError(t, err, false)
	})

	t.Run("ValidShellTypeUnix", func(t *testing.T) {
		_, err := NewMockShell("unix")
		assertError(t, err, false)
	})

	t.Run("InvalidShellType", func(t *testing.T) {
		_, err := NewMockShell("invalid")
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
		mockShell := createMockShell(t, "cmd")
		output := captureStdout(t, func() {
			mockShell.PrintEnvVars(envVars)
		})
		if output != wantOutput {
			t.Errorf("PrintEnvVars() output = %v, want %v", output, wantOutput)
		}
	})

	t.Run("CustomPrintEnvVars", func(t *testing.T) {
		mockShell := createMockShell(t, "cmd")
		mockShell.PrintEnvVarsFn = func(envVars map[string]string) {
			for key, value := range envVars {
				fmt.Printf("%s=%s\n", key, value)
			}
		}
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
		mockShell := createMockShell(t, "cmd")
		mockShell.GetProjectRootFunc = func() (string, error) {
			return "/mock/project/root", nil
		}
		got, err := mockShell.GetProjectRoot()
		assertError(t, err, false)
		want := "/mock/project/root"
		if got != want {
			t.Errorf("GetProjectRoot() got = %v, want %v", got, want)
		}
	})

	t.Run("ErrorInProjectRootRetrieval", func(t *testing.T) {
		mockShell := createMockShell(t, "cmd")
		mockShell.GetProjectRootFunc = func() (string, error) {
			return "", errors.New("failed to get project root")
		}
		_, err := mockShell.GetProjectRoot()
		assertError(t, err, true)
	})

	t.Run("GetProjectRootFuncNotImplemented", func(t *testing.T) {
		mockShell := createMockShell(t, "cmd")
		_, err := mockShell.GetProjectRoot()
		assertError(t, err, true)
	})
}
