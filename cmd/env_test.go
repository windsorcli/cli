package cmd

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/windsor-hotel/cli/internal/config"
)

func TestEnvCmd(t *testing.T) {
	tests := []struct {
		name           string
		mockGOOS       string
		mockHandler    *config.MockConfigHandler
		expectedOutput string
		expectedError  string
	}{
		{
			name:     "Success_Linux",
			mockGOOS: "linux",
			mockHandler: &config.MockConfigHandler{
				GetConfigValueFunc: func(key string) (string, error) {
					if key == "context" {
						return "test-context", nil
					}
					return "", errors.New("key not found")
				},
			},
			expectedOutput: "export WINDSORCONTEXT=test-context\n",
		},
		{
			name:     "Success_Windows",
			mockGOOS: "windows",
			mockHandler: &config.MockConfigHandler{
				GetConfigValueFunc: func(key string) (string, error) {
					if key == "context" {
						return "test-context", nil
					}
					return "", errors.New("key not found")
				},
			},
			expectedOutput: "set WINDSORCONTEXT=test-context\n",
		},
		{
			name:     "GetConfigValueError",
			mockGOOS: "linux",
			mockHandler: &config.MockConfigHandler{
				GetConfigValueFunc: func(key string) (string, error) {
					return "", errors.New("get config value error")
				},
			},
			expectedError: "get config value error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configHandler = tt.mockHandler

			// Mock the OS
			originalGOOS := goos
			goos = tt.mockGOOS
			defer func() { goos = originalGOOS }()

			// Capture stdout or stderr
			var oldOutput *os.File
			var r, w *os.File
			if tt.expectedError == "" {
				oldOutput = os.Stdout
				r, w, _ = os.Pipe()
				os.Stdout = w
			} else {
				oldOutput = os.Stderr
				r, w, _ = os.Pipe()
				os.Stderr = w
			}

			// Add the env command
			rootCmd.AddCommand(envCmd)
			rootCmd.SetArgs([]string{"env"})

			// Execute the command
			err := rootCmd.Execute()
			w.Close()
			var buf bytes.Buffer
			io.Copy(&buf, r)
			if tt.expectedError == "" {
				os.Stdout = oldOutput
			} else {
				os.Stderr = oldOutput
			}
			actualOutput := buf.String()

			if tt.expectedError == "" {
				if err != nil {
					t.Fatalf("Execute() error = %v", err)
				}
				if actualOutput != tt.expectedOutput {
					t.Errorf("Expected output '%s', got '%s'", tt.expectedOutput, actualOutput)
				}
			} else {
				if err == nil {
					t.Fatalf("Expected error, got nil")
				}
				if !strings.Contains(actualOutput, tt.expectedError) {
					t.Errorf("Expected error message to contain '%s', got '%s'", tt.expectedError, actualOutput)
				}
			}

			// Remove the env command after the test
			rootCmd.RemoveCommand(envCmd)
		})
	}
}
