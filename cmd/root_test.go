package cmd

import (
	"errors"
	"os"
	"testing"

	"github.com/spf13/cobra"
)

var originalExitFunc = os.Exit

// MockExitFunc mocks the os.Exit function for testing purposes.
func MockExitFunc() (called *bool) {
	called = new(bool)
	exitFunc = func(code int) {
		*called = true
	}
	return
}

// RestoreExitFunc restores the original os.Exit function.
func RestoreExitFunc() {
	exitFunc = originalExitFunc
}

func TestExecute(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		setup    func()
		wantExit bool
	}{
		{
			name: "successful execution",
			args: []string{},
			setup: func() {
				rootCmd.Run = func(cmd *cobra.Command, args []string) {
					// No-op
				}
			},
			wantExit: false,
		},
		{
			name: "execution with error",
			args: []string{"error"},
			setup: func() {
				errorCmd := &cobra.Command{
					Use: "error",
					RunE: func(cmd *cobra.Command, args []string) error {
						return errors.New("forced error")
					},
				}
				rootCmd.AddCommand(errorCmd)
			},
			wantExit: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			exitCalled := MockExitFunc()
			defer RestoreExitFunc()

			tt.setup()
			rootCmd.SetArgs(tt.args)

			// Execute
			Execute()

			// Verify
			if *exitCalled != tt.wantExit {
				t.Errorf("exitFunc called = %v, want %v", *exitCalled, tt.wantExit)
			}
		})
	}
}

func TestRootCmd(t *testing.T) {
	tests := []struct {
		name     string
		field    string
		expected string
		actual   string
	}{
		{
			name:     "Use field",
			field:    "Use",
			expected: "windsor",
			actual:   rootCmd.Use,
		},
		{
			name:     "Short field",
			field:    "Short",
			expected: "A command line interface to assist in a context flow development environment",
			actual:   rootCmd.Short,
		},
		{
			name:     "Long field",
			field:    "Long",
			expected: "A command line interface to assist in a context flow development environment",
			actual:   rootCmd.Long,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.actual != tt.expected {
				t.Errorf("Expected %s to be '%s', got '%s'", tt.field, tt.expected, tt.actual)
			}
		})
	}
}
