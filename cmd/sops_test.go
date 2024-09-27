package cmd

import (
	"errors"
	"strings"
	"testing"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/shell"
)

// Helper function to set up the container with mock handlers
func setupMockSopsContainer(mockCliConfigHandler, mockProjectConfigHandler config.ConfigHandler, mockShell shell.Shell) di.ContainerInterface {
	container := di.NewContainer()
	container.Register("cliConfigHandler", mockCliConfigHandler)
	container.Register("projectConfigHandler", mockProjectConfigHandler)
	container.Register("shell", mockShell)
	Initialize(container)
	return container
}

func TestDecryptSopsCmd(t *testing.T) {
	originalExitFunc := exitFunc
	exitFunc = mockExit
	t.Cleanup(func() {
		exitFunc = originalExitFunc
	})

	// t.Run("Success", func(t *testing.T) {
	// 	// Given: a valid config handler
	// 	mockHandler := config.NewMockConfigHandler(
	// 		nil,
	// 		func(key string) (string, error) { return "test-sops", nil },
	// 		nil, nil, nil, nil,
	// 	)
	// 	mockShell, err := shell.NewMockShell("cmd")
	// 	if err != nil {
	// 		t.Fatalf("NewMockShell() error = %v", err)
	// 	}
	// 	setupMockContextContainer(mockHandler, mockHandler, mockShell)

	// 	// When: the get context command is executed
	// 	output := captureStdout(func() {
	// 		rootCmd.SetArgs([]string{"sops", "decrypt"})
	// 		err := rootCmd.Execute()
	// 		if err != nil {
	// 			t.Fatalf("Execute() error = %v", err)
	// 		}
	// 	})

	// 	// Then: the output should indicate the current context
	// 	expectedOutput := "test-sops\n"
	// 	if output != expectedOutput {
	// 		t.Errorf("Expected output %q, got %q", expectedOutput, output)
	// 	}
	// })

	t.Run("decryptSopsFileError", func(t *testing.T) {
		// Given: a config handler that returns an error on GetConfigValue
		mockHandler := config.NewMockConfigHandler(
			nil,
			func(key string) (string, error) { return "", errors.New("accepts 1 arg(s), received 0") },
			nil, nil, nil, nil,
		)
		mockShell, err := shell.NewMockShell("cmd")
		if err != nil {
			t.Fatalf("NewMockShell() error = %v", err)
		}
		setupMockContextContainer(mockHandler, mockHandler, mockShell)

		// When: the get context command is executed
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"sops", "decrypt"})
			err := rootCmd.Execute()
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then: the output should indicate the error
		expectedOutput := "accepts 1 arg(s), received 0"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})
}
