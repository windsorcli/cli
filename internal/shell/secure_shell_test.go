package shell

import (
	"testing"
)

func TestSecureShell_NewSecureShell(t *testing.T) {
	t.Run("ValidSSHParams", func(t *testing.T) {
		// Given valid SSH connection parameters
		sshParams := SSHConnectionParams{
			Host:         "localhost",
			Port:         22,
			Username:     "user",
			IdentityFile: "/path/to/identity/file",
		}
		// When creating a new secure shell
		secureShell := NewSecureShell(sshParams)
		// Then no error should be returned
		if secureShell == nil {
			t.Errorf("Expected secureShell, got nil")
		}
	})
}

func TestSecureShell_PrintEnvVars(t *testing.T) {
	envVars := map[string]string{
		"VAR1": "value1",
		"VAR2": "value2",
	}
	wantOutput := "VAR1=value1\nVAR2=value2\n"

	t.Run("DefaultPrintEnvVars", func(t *testing.T) {
		// Given a secure shell with default PrintEnvVars implementation
		sshParams := SSHConnectionParams{}
		secureShell := NewSecureShell(sshParams)
		// When calling PrintEnvVars
		output := captureStdout(t, func() {
			secureShell.PrintEnvVars(envVars)
		})
		// Then the output should match the expected output
		if output != wantOutput {
			t.Errorf("PrintEnvVars() output = %v, want %v", output, wantOutput)
		}
	})
}

func TestSecureShell_GetProjectRoot(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a secure shell that successfully retrieves the project root
		sshParams := SSHConnectionParams{}
		secureShell := NewSecureShell(sshParams)
		// When calling GetProjectRoot
		got, err := secureShell.GetProjectRoot()
		// Then the project root should be returned without error
		if err != nil {
			t.Errorf("GetProjectRoot() error = %v, want nil", err)
		}
		if got == "" {
			t.Errorf("GetProjectRoot() got = %v, want non-empty string", got)
		}
	})

	t.Run("Error", func(t *testing.T) {
		// Given a secure shell that returns an error when retrieving the project root
		sshParams := SSHConnectionParams{}
		secureShell := NewSecureShell(sshParams)
		// When calling GetProjectRoot
		got, err := secureShell.GetProjectRoot()
		// Then an error should be returned
		if err == nil {
			t.Errorf("Expected an error but got none")
		}
		if got != "" {
			t.Errorf("GetProjectRoot() got = %v, want %v", got, "")
		}
	})
}

func TestSecureShell_Exec(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a secure shell with a custom Exec implementation
		sshParams := SSHConnectionParams{}
		secureShell := NewSecureShell(sshParams)
		// When calling Exec
		output, err := secureShell.Exec(false, "Executing command", "echo", "mocked output")
		// Then no error should be returned and output should be as expected
		expectedOutput := "mocked output\n"
		if err != nil {
			t.Errorf("Exec() error = %v, want nil", err)
		}
		if output != expectedOutput {
			t.Errorf("Exec() output = %v, want %v", output, expectedOutput)
		}
	})

	t.Run("Error", func(t *testing.T) {
		// Given a secure shell whose Exec returns an error
		sshParams := SSHConnectionParams{}
		secureShell := NewSecureShell(sshParams)
		// When calling Exec with an invalid command
		output, err := secureShell.Exec(false, "Executing command", "invalidcommand")
		// Then an error should be returned
		if err == nil {
			t.Errorf("Expected an error but got none")
		}
		if output != "" {
			t.Errorf("Exec() output = %v, want %v", output, "")
		}
	})
}
