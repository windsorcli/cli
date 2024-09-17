//go:build windows
// +build windows

package shell

import (
	"bytes"
	"os"
	"testing"
)

func TestDefaultShell_PrintEnvVars_Windows(t *testing.T) {
	shell := NewDefaultShell()
	envVars := map[string]string{
		"VAR2": "value2",
		"VAR1": "value1",
	}

	// Expected output for PowerShell
	expectedOutputPowerShell := "$env:VAR1=\"value1\"\n$env:VAR2=\"value2\"\n"

	// Capture the output
	var output bytes.Buffer
	originalStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	go func() {
		shell.PrintEnvVars(envVars)
		w.Close()
	}()

	output.ReadFrom(r)
	os.Stdout = originalStdout

	// Check if the output matches PowerShell format
	if output.String() != expectedOutputPowerShell {
		t.Errorf("PrintEnvVars() output = %v, want %v", output.String(), expectedOutputPowerShell)
	}
}
