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

func TestInitCmd(t *testing.T) {
	tests := []struct {
		name             string
		mockHandler      *config.MockConfigHandler
		args             []string
		expectedExitCode int
		expectedOutput   string
		expectedError    string
	}{
		{
			name: "Success",
			mockHandler: &config.MockConfigHandler{
				LoadConfigFunc:     func(path string) error { return nil },
				SetConfigValueFunc: func(key, value string) error { return nil },
				SaveConfigFunc:     func(path string) error { return nil },
			},
			args:             []string{"init", "testContext"},
			expectedExitCode: 0,
			expectedOutput:   "Initialization successful\n",
		},
		{
			name: "SetConfigValueError",
			mockHandler: &config.MockConfigHandler{
				SetConfigValueFunc: func(key, value string) error { return errors.New("set config value error") },
			},
			args:             []string{"init", "testContext"},
			expectedExitCode: 1,
			expectedError:    "set config value error",
		},
		{
			name: "SaveConfigError",
			mockHandler: &config.MockConfigHandler{
				SaveConfigFunc: func(path string) error { return errors.New("save config error") },
			},
			args:             []string{"init", "testContext"},
			expectedExitCode: 1,
			expectedError:    "save config error",
		},
	}

	// Store the original exitFunc to restore it later
	originalExitFunc := exitFunc

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configHandler = tt.mockHandler

			// Initialize exitCode for each test case
			var exitCode int
			exitFunc = func(code int) {
				exitCode = code
			}
			// Ensure exitFunc is restored after the test case
			defer func() { exitFunc = originalExitFunc }()

			// Capture stdout or stderr based on the expected outcome
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

			// Add the init command
			rootCmd.AddCommand(initCmd)
			rootCmd.SetArgs(tt.args)

			// Execute the command
			Execute() // Use Execute() instead of rootCmd.Execute()
			w.Close()
			var buf bytes.Buffer
			io.Copy(&buf, r)
			if tt.expectedError == "" {
				os.Stdout = oldOutput
			} else {
				os.Stderr = oldOutput
			}
			actualOutput := buf.String()

			// Validate exit code
			if exitCode != tt.expectedExitCode {
				t.Errorf("exitFunc was called with code %d, expected %d", exitCode, tt.expectedExitCode)
			}

			// Validate output or error
			if tt.expectedError == "" {
				if exitCode != 0 {
					t.Fatalf("Expected exit code 0, got %d", exitCode)
				}
				expectedOutput := tt.expectedOutput
				if actualOutput != expectedOutput {
					t.Errorf("Expected output '%s', got '%s'", expectedOutput, actualOutput)
				}
			} else {
				if exitCode == 0 {
					t.Fatalf("Expected non-zero exit code, got %d", exitCode)
				}
				if !strings.Contains(actualOutput, tt.expectedError) {
					t.Errorf("Expected error message to contain '%s', got '%s'", tt.expectedError, actualOutput)
				}
			}

			// Remove the init command after the test
			rootCmd.RemoveCommand(initCmd)
		})
	}
}
