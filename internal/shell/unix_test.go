//go:build darwin || linux
// +build darwin linux

package shell

import (
	"bytes"
	"os"
	"testing"
)

func TestDefaultShell_PrintEnvVars_Unix(t *testing.T) {
	shell := NewDefaultShell()
	envVars := map[string]string{
		"VAR2": "value2",
		"VAR1": "value1",
	}
	expectedOutput := "export VAR1=\"value1\"\nexport VAR2=\"value2\"\n"

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

	if output.String() != expectedOutput {
		t.Errorf("PrintEnvVars() output = %v, want %v", output.String(), expectedOutput)
	}
}
