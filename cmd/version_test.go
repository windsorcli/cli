package cmd

import (
	"fmt"
	"runtime"
	"testing"

	ctrl "github.com/windsorcli/cli/internal/controller"
	"github.com/windsorcli/cli/internal/di"
)

func TestVersionCommand(t *testing.T) {
	originalExitFunc := exitFunc
	exitFunc = mockExit
	t.Cleanup(func() {
		exitFunc = originalExitFunc
	})

	t.Run("VersionOutput", func(t *testing.T) {
		// Create a mock controller
		injector := di.NewInjector()
		mockController := ctrl.NewMockController(injector)

		// When: the version command is executed
		output := captureStdout(func() {
			rootCmd.SetArgs([]string{"version"})
			err := Execute(mockController)
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
}
