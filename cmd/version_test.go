package cmd

import (
	"fmt"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestVersionCommand(t *testing.T) {
	originalExitFunc := exitFunc
	exitFunc = mockExit
	t.Cleanup(func() {
		exitFunc = originalExitFunc
	})

	t.Run("VersionOutput", func(t *testing.T) {
		// When: the version command is executed
		output := captureStdout(func() {
			rootCmd.SetArgs([]string{"version"})
			err := rootCmd.Execute()
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
		})

		// Then: the output should contain the version, commit SHA, and platform
		expectedOutput := fmt.Sprintf("Version: %s\nCommit SHA: %s\nPlatform: %s\n", version, commitSHA, fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH))
		if output != expectedOutput {
			t.Errorf("Expected output %q, got %q", expectedOutput, output)
		}
	})

	t.Run("VersionCommandError", func(t *testing.T) {
		// Given: a version command with an error
		cmd := &cobra.Command{
			Use:   "version",
			Short: "Display the current version",
			Long:  "Display the current version of the application",
			Run: func(cmd *cobra.Command, args []string) {
				exitFunc(1)
			},
		}

		// When: the version command is executed
		defer func() {
			if r := recover(); r != nil {
				// Then: the output should indicate the error
				expectedOutput := "exit code: 1"
				if !strings.Contains(fmt.Sprint(r), expectedOutput) {
					t.Errorf("Expected output to contain %q, got %q", expectedOutput, r)
				}
			} else {
				t.Fatalf("Expected panic, got nil")
			}
		}()

		cmd.Execute()
	})
}
