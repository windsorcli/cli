package cmd

import (
	"errors"
	"testing"

	"github.com/windsor-hotel/cli/internal/config"
)

func setupTestEnvironment(mockHandler config.ConfigHandler) {
	container := setupContainer(mockHandler)

	// Ensure configHandler is set correctly
	instance, err := container.Resolve("configHandler")
	if err != nil {
		panic("Error resolving configHandler: " + err.Error())
	}
	configHandler, _ = instance.(config.ConfigHandler)

	// Register the initCmd with rootCmd
	rootCmd.AddCommand(initCmd)
}

// TestInitCmd tests the init command
func TestInitCmd(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a valid config handler
		mockHandler := config.NewMockConfigHandler(
			nil,
			func(key string) (string, error) { return "value", nil },
			func(key, value string) error { return nil },
			func(path string) error { return nil },
			nil, nil,
		)
		setupTestEnvironment(mockHandler)

		// When the init command is executed with a valid context
		output := captureStdout(func() {
			rootCmd.SetArgs([]string{"init", "test-context"})
			err := rootCmd.Execute()
			if err != nil {
				t.Fatalf("rootCmd.Execute() error = %v", err)
			}
		})

		// Then the output should indicate success
		expectedOutput := "Initialization successful\n"
		if output != expectedOutput {
			t.Errorf("Expected output '%s', got '%s'", expectedOutput, output)
		}
	})

	t.Run("SetConfigValueError", func(t *testing.T) {
		// Given a config handler that returns an error on SetConfigValue
		mockHandler := config.NewMockConfigHandler(
			nil,
			func(key string) (string, error) { return "value", nil },
			func(key, value string) error { return errors.New("set config value error") },
			func(path string) error { return nil },
			nil, nil,
		)
		setupTestEnvironment(mockHandler)

		// When the init command is executed with a valid context
		rootCmd.SetArgs([]string{"init", "test-context"})
		err := rootCmd.Execute()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}
		expectedError := "Error setting config value: set config value error"
		if err.Error() != expectedError {
			t.Fatalf("Expected error '%s', got '%s'", expectedError, err.Error())
		}
	})

	t.Run("SaveConfigError", func(t *testing.T) {
		// Given a config handler that returns an error on SaveConfig
		mockHandler := config.NewMockConfigHandler(
			nil,
			func(key string) (string, error) { return "value", nil },
			func(key, value string) error { return nil },
			func(path string) error { return errors.New("save config error") },
			nil, nil,
		)
		setupTestEnvironment(mockHandler)

		// When the init command is executed with a valid context
		rootCmd.SetArgs([]string{"init", "test-context"})
		err := rootCmd.Execute()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}
		expectedError := "Error saving config file: save config error"
		if err.Error() != expectedError {
			t.Fatalf("Expected error '%s', got '%s'", expectedError, err.Error())
		}
	})
}
