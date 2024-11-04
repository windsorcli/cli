package shell

import (
	"fmt"
	"strings"
	"testing"

	"github.com/windsor-hotel/cli/internal/di"
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
	t.Run("CreateMockShellWithoutContainer", func(t *testing.T) {
		// When creating a new mock shell without a container
		mockShell := NewMockShell()
		// Then no error should be returned
		if mockShell == nil {
			t.Errorf("Expected mockShell, got nil")
		}
	})

	t.Run("CreateMockShellWithContainer", func(t *testing.T) {
		// Given a mock DI container
		mockContainer := &di.MockContainer{}
		// When creating a new mock shell with the container
		mockShell := NewMockShell(mockContainer)
		// Then no error should be returned and the container should be set
		if mockShell == nil {
			t.Errorf("Expected mockShell, got nil")
		}
		if mockShell.container != mockContainer {
			t.Errorf("Expected container to be set, got %v", mockShell.container)
		}
	})
}

func TestMockShell_PrintEnvVars(t *testing.T) {
	envVars := map[string]string{
		"VAR1": "value1",
		"VAR2": "value2",
	}

	t.Run("DefaultPrintEnvVars", func(t *testing.T) {
		// Given a mock shell with default PrintEnvVars implementation
		mockShell := NewMockShell()
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
		// Given a mock shell with custom PrintEnvVars implementation
		mockShell := NewMockShell()
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
		// Given a mock shell with default PrintAlias implementation
		mockShell := NewMockShell()
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
		// Given a mock shell with custom PrintAlias implementation
		mockShell := NewMockShell()
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
		mockShell := NewMockShell()
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
		mockShell := NewMockShell()
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
		mockShell := NewMockShell()
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
		mockShell := NewMockShell()
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
		mockShell := NewMockShell()
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
		mockShell := NewMockShell()
		// When calling Exec
		output, err := mockShell.Exec(false, "Executing command", "somecommand", "arg1", "arg2")
		// Then no error should be returned and the result should be empty
		if err != nil {
			t.Errorf("Exec() error = %v, want nil", err)
		}
		if output != "" {
			t.Errorf("Exec() output = %v, want %v", output, "")
		}
	})
}
