package shell

import (
	"bytes"
	"errors"
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

func TestMockShell_GetProjectRoot(t *testing.T) {
	tests := []struct {
		name               string
		getProjectRootFunc func() (string, error)
		want               string
		wantErr            bool
	}{
		{
			name: "successful project root retrieval",
			getProjectRootFunc: func() (string, error) {
				return "/mock/project/root", nil
			},
			want:    "/mock/project/root",
			wantErr: false,
		},
		{
			name: "error in project root retrieval",
			getProjectRootFunc: func() (string, error) {
				return "", errors.New("failed to get project root")
			},
			want:    "",
			wantErr: true,
		},
		{
			name:               "GetProjectRootFunc not implemented",
			getProjectRootFunc: nil,
			want:               "",
			wantErr:            true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockShell := &MockShell{
				GetProjectRootFunc: tt.getProjectRootFunc,
			}
			got, err := mockShell.GetProjectRoot()
			if (err != nil) != tt.wantErr {
				t.Errorf("GetProjectRoot() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("GetProjectRoot() got = %v, want %v", got, tt.want)
			}
		})
	}
}
