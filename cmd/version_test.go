package cmd

import (
	"fmt"
	"runtime"
	"strings"
	"testing"

	"github.com/windsor-hotel/cli/internal/config"
)

func TestVersionCommand(t *testing.T) {
	originalExitFunc := exitFunc
	exitFunc = func(code int) {
		mockExit(code, "")
	}
	t.Cleanup(func() {
		exitFunc = originalExitFunc
	})

	t.Run("VersionOutput", func(t *testing.T) {
		// Mock the cliConfigHandler
		mockCliConfigHandler := config.NewMockConfigHandler()

		// Setup container with mock dependencies
		deps := MockDependencies{
			CLIConfigHandler: mockCliConfigHandler,
		}
		setupContainer(deps)

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
		// Setup container with mock dependencies
		mockCliConfigHandler := config.NewMockConfigHandler()
		deps := MockDependencies{
			CLIConfigHandler: mockCliConfigHandler,
		}
		setupContainer(deps)

		// When: the version command is executed with an error
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

		rootCmd.SetArgs([]string{"version"})
		exitFunc(1)
		rootCmd.Execute()
	})
}
