package cmd

import (
	"errors"
	"strings"
	"testing"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/shell"
)

func TestInitCmd(t *testing.T) {
	originalArgs := rootCmd.Args
	t.Cleanup(func() {
		rootCmd.Args = originalArgs
	})

	t.Run("Success", func(t *testing.T) {
		// Given: a valid config handler
		mockHandler := config.NewMockConfigHandler(
			func(path string) error { return nil },
			func(key string) (string, error) { return "value", nil },
			func(key, value string) error { return nil },
			func(path string) error { return nil },
			func(key string) (map[string]interface{}, error) { return nil, nil },
			func(key string) ([]string, error) { return nil, nil },
		)
		mockShell, err := shell.NewMockShell("cmd")
		if err != nil {
			t.Fatalf("NewMockShell() error = %v", err)
		}
		setupContainer(mockHandler, mockHandler, mockShell, nil)

		// When: the init command is executed with a valid context
		output := captureStdout(func() {
			rootCmd.SetArgs([]string{"init", "test-context"})
			err := rootCmd.Execute()
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
		})

		// Then: the output should indicate success
		expectedOutput := "Initialization successful\n"
		if output != expectedOutput {
			t.Errorf("Expected output %q, got %q", expectedOutput, output)
		}
	})

	t.Run("SetConfigValueError", func(t *testing.T) {
		// Given: a config handler that returns an error on SetConfigValue
		mockHandler := config.NewMockConfigHandler(
			func(path string) error { return nil },
			func(key string) (string, error) { return "value", nil },
			func(key, value string) error { return errors.New("set config value error") },
			func(path string) error { return nil },
			func(key string) (map[string]interface{}, error) { return nil, nil },
			func(key string) ([]string, error) { return nil, nil },
		)
		mockShell, err := shell.NewMockShell("cmd")
		if err != nil {
			t.Fatalf("NewMockShell() error = %v", err)
		}
		setupContainer(mockHandler, mockHandler, mockShell, nil)

		// When: the init command is executed
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"init", "test-context"})
			err := rootCmd.Execute()
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then: the output should indicate the error
		expectedOutput := "Error setting config value: set config value error"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("SaveConfigError", func(t *testing.T) {
		// Given: a config handler that returns an error on SaveConfig
		mockHandler := config.NewMockConfigHandler(
			func(path string) error { return nil },
			func(key string) (string, error) { return "value", nil },
			func(key, value string) error { return nil },
			func(path string) error { return errors.New("save config error") },
			func(key string) (map[string]interface{}, error) { return nil, nil },
			func(key string) ([]string, error) { return nil, nil },
		)
		mockShell, err := shell.NewMockShell("cmd") // Ensure valid shell type
		if err != nil {
			t.Fatalf("NewMockShell() error = %v", err)
		}
		setupContainer(mockHandler, mockHandler, mockShell, nil)

		// When: the init command is executed
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"init", "test-context"})
			err := rootCmd.Execute()
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then: the output should indicate the error
		expectedOutput := "Error saving config file: save config error"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("ProjectConfigSaveError", func(t *testing.T) {
		// Given: a CLI config handler that succeeds and a project config handler that returns an error on SaveConfig
		mockCLIHandler := config.NewMockConfigHandler(
			func(path string) error { return nil },
			func(key string) (string, error) { return "value", nil },
			func(key, value string) error { return nil },
			func(path string) error { return nil },
			func(key string) (map[string]interface{}, error) { return nil, nil },
			func(key string) ([]string, error) { return nil, nil },
		)
		mockProjectHandler := config.NewMockConfigHandler(
			func(path string) error { return nil },
			func(key string) (string, error) { return "value", nil },
			func(key, value string) error { return nil },
			func(path string) error { return errors.New("save project config error") },
			func(key string) (map[string]interface{}, error) { return nil, nil },
			func(key string) ([]string, error) { return nil, nil },
		)
		mockShell, err := shell.NewMockShell("cmd") // Ensure valid shell type
		if err != nil {
			t.Fatalf("NewMockShell() error = %v", err)
		}
		setupContainer(mockCLIHandler, mockProjectHandler, mockShell, nil)

		// When: the init command is executed
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"init", "test-context"})
			err := rootCmd.Execute()
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then: the output should indicate the error
		expectedOutput := "Error saving project config file: save project config error"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})
}
