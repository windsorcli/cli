package cmd

import (
	"errors"
	"strings"
	"testing"

	"github.com/windsor-hotel/cli/internal/mocks"
)

func TestContext_Get(t *testing.T) {
	originalExitFunc := exitFunc
	exitFunc = func(code int) {
		mockExit(code, "")
	}
	t.Cleanup(func() {
		exitFunc = originalExitFunc
	})

	t.Run("Success", func(t *testing.T) {
		// Given a valid config handler
		mocks := mocks.CreateSuperMocks()
		mocks.CLIConfigHandler.GetStringFunc = func(key string, defaultValue ...string) (string, error) { return "test-context", nil }
		Initialize(mocks.Container)

		// When the get context command is executed
		output := captureStdout(func() {
			rootCmd.SetArgs([]string{"context", "get"})
			err := rootCmd.Execute()
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
		})

		// Then the output should indicate the current context
		expectedOutput := "test-context\n"
		if output != expectedOutput {
			t.Errorf("Expected output %q, got %q", expectedOutput, output)
		}
	})

	t.Run("Error", func(t *testing.T) {
		// Given a config handler that returns an error on GetConfigValue
		mocks := mocks.CreateSuperMocks()
		mocks.CLIConfigHandler.GetStringFunc = func(key string, defaultValue ...string) (string, error) {
			return "", errors.New("get context error")
		}
		Initialize(mocks.Container)

		// When the get context command is executed
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"context", "get"})
			err := rootCmd.Execute()
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then the output should indicate the error
		expectedOutput := "get context error"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})
}

func TestContext_Set(t *testing.T) {
	originalExitFunc := exitFunc
	exitFunc = func(code int) {
		mockExit(code, "")
	}
	t.Cleanup(func() {
		exitFunc = originalExitFunc
	})

	t.Run("Success", func(t *testing.T) {
		// Given a valid config handler
		mocks := mocks.CreateSuperMocks()
		mocks.CLIConfigHandler.SetFunc = func(key string, value interface{}) error { return nil }
		mocks.CLIConfigHandler.SaveConfigFunc = func(path string) error { return nil }
		Initialize(mocks.Container)
		// When the set context command is executed with a valid context
		output := captureStdout(func() {
			rootCmd.SetArgs([]string{"context", "set", "new-context"})
			err := rootCmd.Execute()
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
		})

		// Then the output should indicate success
		expectedOutput := "Context set to: new-context\n"
		if output != expectedOutput {
			t.Errorf("Expected output %q, got %q", expectedOutput, output)
		}
	})

	t.Run("SetContextError", func(t *testing.T) {
		// Given a config handler that returns an error on SetConfigValue
		mocks := mocks.CreateSuperMocks()
		mocks.CLIConfigHandler.SetFunc = func(key string, value interface{}) error { return errors.New("set context error") }
		Initialize(mocks.Container)
		// When the set context command is executed
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"context", "set", "new-context"})
			err := rootCmd.Execute()
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then the output should indicate the error
		expectedOutput := "set context error"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("SaveConfigError", func(t *testing.T) {
		// Given a config handler that returns an error on SaveConfig
		mocks := mocks.CreateSuperMocks()
		mocks.CLIConfigHandler.SetFunc = func(key string, value interface{}) error { return nil }
		mocks.CLIConfigHandler.SaveConfigFunc = func(path string) error { return errors.New("save config error") }
		Initialize(mocks.Container)

		// When the set context command is executed
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"context", "set", "new-context"})
			err := rootCmd.Execute()
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then the output should indicate the error
		expectedOutput := "save config error"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})
}

func TestContext_GetAlias(t *testing.T) {
	originalExitFunc := exitFunc
	exitFunc = func(code int) {
		mockExit(code, "")
	}
	t.Cleanup(func() {
		exitFunc = originalExitFunc
	})

	t.Run("Success", func(t *testing.T) {
		// Given a valid config handler
		mocks := mocks.CreateSuperMocks()
		mocks.CLIConfigHandler.GetStringFunc = func(key string, defaultValue ...string) (string, error) {
			return "test-context", nil
		}
		Initialize(mocks.Container)
		// When the get-context alias command is executed
		output := captureStdout(func() {
			rootCmd.SetArgs([]string{"get-context"})
			err := rootCmd.Execute()
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
		})

		// Then the output should indicate the current context
		expectedOutput := "test-context\n"
		if output != expectedOutput {
			t.Errorf("Expected output %q, got %q", expectedOutput, output)
		}
	})

	t.Run("Error", func(t *testing.T) {
		// Given a config handler that returns an error on GetConfigValue
		mocks := mocks.CreateSuperMocks()
		mocks.CLIConfigHandler.GetStringFunc = func(key string, defaultValue ...string) (string, error) {
			return "", errors.New("get context error")
		}
		Initialize(mocks.Container)

		// When the get-context alias command is executed
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"get-context"})
			err := rootCmd.Execute()
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then the output should indicate the error
		expectedOutput := "get context error"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})
}

func TestContext_SetAlias(t *testing.T) {
	originalExitFunc := exitFunc
	exitFunc = func(code int) {
		mockExit(code, "")
	}
	t.Cleanup(func() {
		exitFunc = originalExitFunc
	})

	t.Run("Success", func(t *testing.T) {
		// Given a valid config handler
		mocks := mocks.CreateSuperMocks()
		mocks.CLIConfigHandler.SetFunc = func(key string, value interface{}) error { return nil }
		mocks.CLIConfigHandler.SaveConfigFunc = func(path string) error { return nil }
		Initialize(mocks.Container)

		// When the set-context alias command is executed with a valid context
		output := captureStdout(func() {
			rootCmd.SetArgs([]string{"set-context", "new-context"})
			err := rootCmd.Execute()
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
		})

		// Then the output should indicate success
		expectedOutput := "Context set to: new-context\n"
		if output != expectedOutput {
			t.Errorf("Expected output %q, got %q", expectedOutput, output)
		}
	})

	t.Run("SetContextError", func(t *testing.T) {
		// Given a config handler that returns an error on SetConfigValue
		mocks := mocks.CreateSuperMocks()
		mocks.CLIConfigHandler.SetFunc = func(key string, value interface{}) error { return errors.New("set context error") }
		Initialize(mocks.Container)

		// When the set-context alias command is executed
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"set-context", "new-context"})
			err := rootCmd.Execute()
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then the output should indicate the error
		expectedOutput := "set context error"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("SaveConfigError", func(t *testing.T) {
		// Given a config handler that returns an error on SaveConfig
		mocks := mocks.CreateSuperMocks()
		mocks.CLIConfigHandler.SetFunc = func(key string, value interface{}) error { return nil }
		mocks.CLIConfigHandler.SaveConfigFunc = func(path string) error { return errors.New("save config error") }
		Initialize(mocks.Container)

		// When the set-context alias command is executed
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"set-context", "new-context"})
			err := rootCmd.Execute()
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then the output should indicate the error
		expectedOutput := "save config error"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})
}
