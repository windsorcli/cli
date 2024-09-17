package shell

import (
	"bytes"
	"fmt"
	"os"
	"testing"
)

func TestNewMockShell(t *testing.T) {
	tests := []struct {
		shellType string
		wantErr   bool
	}{
		{"cmd", false},
		{"powershell", false},
		{"unix", false},
		{"invalid", true},
	}

	for _, tt := range tests {
		_, err := NewMockShell(tt.shellType)
		if (err != nil) != tt.wantErr {
			t.Errorf("NewMockShell(%v) error = %v, wantErr %v", tt.shellType, err, tt.wantErr)
		}
	}
}

func TestMockShell_DetermineShell(t *testing.T) {
	tests := []struct {
		shellType string
		want      string
	}{
		{"cmd", "cmd"},
		{"powershell", "powershell"},
		{"unix", "unix"},
	}

	for _, tt := range tests {
		mockShell, err := NewMockShell(tt.shellType)
		if err != nil {
			t.Fatalf("NewMockShell(%v) error = %v", tt.shellType, err)
		}
		if got := mockShell.DetermineShell(); got != tt.want {
			t.Errorf("DetermineShell() = %v, want %v", got, tt.want)
		}
	}
}

func TestMockShell_PrintEnvVars(t *testing.T) {
	tests := []struct {
		envVars        map[string]string
		printEnvVarsFn func(envVars map[string]string)
		wantOutput     string
	}{
		{
			envVars: map[string]string{
				"VAR1": "value1",
				"VAR2": "value2",
			},
			printEnvVarsFn: nil,
			wantOutput:     "VAR1=value1\nVAR2=value2\n",
		},
		{
			envVars: map[string]string{
				"VAR1": "value1",
				"VAR2": "value2",
			},
			printEnvVarsFn: func(envVars map[string]string) {
				for key, value := range envVars {
					fmt.Printf("%s=%s\n", key, value)
				}
			},
			wantOutput: "VAR1=value1\nVAR2=value2\n",
		},
	}

	for _, tt := range tests {
		mockShell, err := NewMockShell("cmd")
		if err != nil {
			t.Fatalf("NewMockShell(cmd) error = %v", err)
		}
		mockShell.PrintEnvVarsFn = tt.printEnvVarsFn

		// Capture the output
		var output bytes.Buffer
		originalStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		go func() {
			mockShell.PrintEnvVars(tt.envVars)
			w.Close()
		}()

		output.ReadFrom(r)
		os.Stdout = originalStdout

		if output.String() != tt.wantOutput {
			t.Errorf("PrintEnvVars() output = %v, want %v", output.String(), tt.wantOutput)
		}
	}
}
