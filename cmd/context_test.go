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
func setupMockContextContainer(mockCliConfigHandler, mockProjectConfigHandler config.ConfigHandler, mockShell shell.Shell) di.ContainerInterface {
	container := di.NewContainer()
	container.Register("cliConfigHandler", mockCliConfigHandler)
	container.Register("projectConfigHandler", mockProjectConfigHandler)
	container.Register("shell", mockShell)
	Initialize(container)
	return container
}

func TestGetContextCmd(t *testing.T) {
	originalExitFunc := exitFunc
	exitFunc = mockExit
	t.Cleanup(func() {
		exitFunc = originalExitFunc
	})

	t.Run("Success", func(t *testing.T) {
		// Given: a valid config handler
		mockHandler := config.NewMockConfigHandler(
			nil,
			func(key string) (string, error) { return "test-context", nil },
			nil, nil, nil, nil,
		)
		mockShell, err := shell.NewMockShell("cmd")
		if err != nil {
			t.Fatalf("NewMockShell() error = %v", err)
		}
		setupMockContextContainer(mockHandler, mockHandler, mockShell)

		// When: the get context command is executed
		output := captureStdout(func() {
			rootCmd.SetArgs([]string{"context", "get"})
			err := rootCmd.Execute()
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
		})

		// Then: the output should indicate the current context
		expectedOutput := "test-context\n"
		if output != expectedOutput {
			t.Errorf("Expected output %q, got %q", expectedOutput, output)
		}
	})

	t.Run("GetContextError", func(t *testing.T) {
		// Given: a config handler that returns an error on GetConfigValue
		mockHandler := config.NewMockConfigHandler(
			nil,
			func(key string) (string, error) { return "", errors.New("get context error") },
			nil, nil, nil, nil,
		)
		mockShell, err := shell.NewMockShell("cmd")
		if err != nil {
			t.Fatalf("NewMockShell() error = %v", err)
		}
		setupMockContextContainer(mockHandler, mockHandler, mockShell)

		// When: the get context command is executed
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"context", "get"})
			err := rootCmd.Execute()
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then: the output should indicate the error
		expectedOutput := "get context error"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})
}

func TestSetContextCmd(t *testing.T) {
	originalExitFunc := exitFunc
	exitFunc = mockExit
	t.Cleanup(func() {
		exitFunc = originalExitFunc
	})

	t.Run("Success", func(t *testing.T) {
		// Given: a valid config handler
		mockHandler := config.NewMockConfigHandler(
			nil,
			nil,
			func(key, value string) error { return nil },
			func(path string) error { return nil },
			nil, nil,
		)
		mockShell, err := shell.NewMockShell("cmd")
		if err != nil {
			t.Fatalf("NewMockShell() error = %v", err)
		}
		setupMockContextContainer(mockHandler, mockHandler, mockShell)

		// When: the set context command is executed with a valid context
		output := captureStdout(func() {
			rootCmd.SetArgs([]string{"context", "set", "new-context"})
			err := rootCmd.Execute()
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
		})

		// Then: the output should indicate success
		expectedOutput := "Context set to: new-context\n"
		if output != expectedOutput {
			t.Errorf("Expected output %q, got %q", expectedOutput, output)
		}
	})

	t.Run("SetContextError", func(t *testing.T) {
		// Given: a config handler that returns an error on SetConfigValue
		mockHandler := config.NewMockConfigHandler(
			nil,
			nil,
			func(key, value string) error { return errors.New("set context error") },
			nil, nil, nil,
		)
		mockShell, err := shell.NewMockShell("cmd")
		if err != nil {
			t.Fatalf("NewMockShell() error = %v", err)
		}
		setupMockContextContainer(mockHandler, mockHandler, mockShell)

		// When: the set context command is executed
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"context", "set", "new-context"})
			err := rootCmd.Execute()
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then: the output should indicate the error
		expectedOutput := "set context error"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("SaveConfigError", func(t *testing.T) {
		// Given: a config handler that returns an error on SaveConfig
		mockHandler := config.NewMockConfigHandler(
			nil,
			nil,
			func(key, value string) error { return nil },
			func(path string) error { return errors.New("save config error") },
			nil, nil,
		)
		mockShell, err := shell.NewMockShell("cmd")
		if err != nil {
			t.Fatalf("NewMockShell() error = %v", err)
		}
		setupMockContextContainer(mockHandler, mockHandler, mockShell)

		// When: the set context command is executed
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"context", "set", "new-context"})
			err := rootCmd.Execute()
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then: the output should indicate the error
		expectedOutput := "save config error"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})
}

func TestGetContextAliasCmd(t *testing.T) {
	originalExitFunc := exitFunc
	exitFunc = mockExit
	t.Cleanup(func() {
		exitFunc = originalExitFunc
	})

	t.Run("Success", func(t *testing.T) {
		// Given: a valid config handler
		mockHandler := config.NewMockConfigHandler(
			nil,
			func(key string) (string, error) { return "test-context", nil },
			nil, nil, nil, nil,
		)
		mockShell, err := shell.NewMockShell("cmd")
		if err != nil {
			t.Fatalf("NewMockShell() error = %v", err)
		}
		setupMockContextContainer(mockHandler, mockHandler, mockShell)

		// When: the get-context alias command is executed
		output := captureStdout(func() {
			rootCmd.SetArgs([]string{"get-context"})
			err := rootCmd.Execute()
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
		})

		// Then: the output should indicate the current context
		expectedOutput := "test-context\n"
		if output != expectedOutput {
			t.Errorf("Expected output %q, got %q", expectedOutput, output)
		}
	})

	t.Run("GetContextError", func(t *testing.T) {
		// Given: a config handler that returns an error on GetConfigValue
		mockHandler := config.NewMockConfigHandler(
			nil,
			func(key string) (string, error) { return "", errors.New("get context error") },
			nil, nil, nil, nil,
		)
		mockShell, err := shell.NewMockShell("cmd")
		if err != nil {
			t.Fatalf("NewMockShell() error = %v", err)
		}
		setupMockContextContainer(mockHandler, mockHandler, mockShell)

		// When: the get-context alias command is executed
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"get-context"})
			err := rootCmd.Execute()
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then: the output should indicate the error
		expectedOutput := "get context error"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})
}

func TestSetContextAliasCmd(t *testing.T) {
	originalExitFunc := exitFunc
	exitFunc = mockExit
	t.Cleanup(func() {
		exitFunc = originalExitFunc
	})

	t.Run("Success", func(t *testing.T) {
		// Given: a valid config handler
		mockHandler := config.NewMockConfigHandler(
			nil,
			nil,
			func(key, value string) error { return nil },
			func(path string) error { return nil },
			nil, nil,
		)
		mockShell, err := shell.NewMockShell("cmd")
		if err != nil {
			t.Fatalf("NewMockShell() error = %v", err)
		}
		setupMockContextContainer(mockHandler, mockHandler, mockShell)

		// When: the set-context alias command is executed with a valid context
		output := captureStdout(func() {
			rootCmd.SetArgs([]string{"set-context", "new-context"})
			err := rootCmd.Execute()
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
		})

		// Then: the output should indicate success
		expectedOutput := "Context set to: new-context\n"
		if output != expectedOutput {
			t.Errorf("Expected output %q, got %q", expectedOutput, output)
		}
	})

	t.Run("SetContextError", func(t *testing.T) {
		// Given: a config handler that returns an error on SetConfigValue
		mockHandler := config.NewMockConfigHandler(
			nil,
			nil,
			func(key, value string) error { return errors.New("set context error") },
			nil, nil, nil,
		)
		mockShell, err := shell.NewMockShell("cmd")
		if err != nil {
			t.Fatalf("NewMockShell() error = %v", err)
		}
		setupMockContextContainer(mockHandler, mockHandler, mockShell)

		// When: the set-context alias command is executed
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"set-context", "new-context"})
			err := rootCmd.Execute()
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then: the output should indicate the error
		expectedOutput := "set context error"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("SaveConfigError", func(t *testing.T) {
		// Given: a config handler that returns an error on SaveConfig
		mockHandler := config.NewMockConfigHandler(
			nil,
			nil,
			func(key, value string) error { return nil },
			func(path string) error { return errors.New("save config error") },
			nil, nil,
		)
		mockShell, err := shell.NewMockShell("cmd")
		if err != nil {
			t.Fatalf("NewMockShell() error = %v", err)
		}
		setupMockContextContainer(mockHandler, mockHandler, mockShell)

		// When: the set-context alias command is executed
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"set-context", "new-context"})
			err := rootCmd.Execute()
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then: the output should indicate the error
		expectedOutput := "save config error"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})
}
