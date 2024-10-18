package cmd

import (
	"errors"
	"strings"
	"testing"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/shell"
)

func TestContextSubcommand(t *testing.T) {
	originalExitFunc := exitFunc
	exitFunc = mockExit
	t.Cleanup(func() {
		exitFunc = originalExitFunc
	})

	t.Run("Get", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			// Given a valid config handler
			mockHandler := config.NewMockConfigHandler()
			mockHandler.GetStringFunc = func(key string) (string, error) { return "test-context", nil }
			mockShell, err := shell.NewMockShell("cmd")
			if err != nil {
				t.Fatalf("NewMockShell() error = %v", err)
			}
			setupContainer(mockHandler, mockHandler, mockShell, nil, nil, nil, nil)

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

		t.Run("GetContextError", func(t *testing.T) {
			// Given a config handler that returns an error on GetConfigValue
			mockHandler := config.NewMockConfigHandler()
			mockHandler.GetStringFunc = func(key string) (string, error) { return "", errors.New("get context error") }
			mockShell, err := shell.NewMockShell("cmd")
			if err != nil {
				t.Fatalf("NewMockShell() error = %v", err)
			}
			setupContainer(mockHandler, mockHandler, mockShell, nil, nil, nil, nil)

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
	})

	t.Run("Set", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			// Given a valid config handler
			mockHandler := config.NewMockConfigHandler()
			mockHandler.SetValueFunc = func(key string, value interface{}) error { return nil }
			mockHandler.SaveConfigFunc = func(path string) error { return nil }
			mockShell, err := shell.NewMockShell("cmd")
			if err != nil {
				t.Fatalf("NewMockShell() error = %v", err)
			}
			setupContainer(mockHandler, mockHandler, mockShell, nil, nil, nil, nil)

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
			mockHandler := config.NewMockConfigHandler()
			mockHandler.SetValueFunc = func(key string, value interface{}) error { return errors.New("set context error") }
			mockShell, err := shell.NewMockShell("cmd")
			if err != nil {
				t.Fatalf("NewMockShell() error = %v", err)
			}
			setupContainer(mockHandler, mockHandler, mockShell, nil, nil, nil, nil)

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
			mockHandler := config.NewMockConfigHandler()
			mockHandler.SetValueFunc = func(key string, value interface{}) error { return nil }
			mockHandler.SaveConfigFunc = func(path string) error { return errors.New("save config error") }
			mockShell, err := shell.NewMockShell("cmd")
			if err != nil {
				t.Fatalf("NewMockShell() error = %v", err)
			}
			setupContainer(mockHandler, mockHandler, mockShell, nil, nil, nil, nil)

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
	})

	t.Run("GetAlias", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			// Given a valid config handler
			mockHandler := config.NewMockConfigHandler()
			mockHandler.GetStringFunc = func(key string) (string, error) { return "test-context", nil }
			mockShell, err := shell.NewMockShell("cmd")
			if err != nil {
				t.Fatalf("NewMockShell() error = %v", err)
			}
			setupContainer(mockHandler, mockHandler, mockShell, nil, nil, nil, nil)

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

		t.Run("GetContextError", func(t *testing.T) {
			// Given a config handler that returns an error on GetConfigValue
			mockHandler := config.NewMockConfigHandler()
			mockHandler.GetStringFunc = func(key string) (string, error) { return "", errors.New("get context error") }
			mockShell, err := shell.NewMockShell("cmd")
			if err != nil {
				t.Fatalf("NewMockShell() error = %v", err)
			}
			setupContainer(mockHandler, mockHandler, mockShell, nil, nil, nil, nil)

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
	})

	t.Run("SetAlias", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			// Given a valid config handler
			mockHandler := config.NewMockConfigHandler()
			mockHandler.SetValueFunc = func(key string, value interface{}) error { return nil }
			mockHandler.SaveConfigFunc = func(path string) error { return nil }
			mockShell, err := shell.NewMockShell("cmd")
			if err != nil {
				t.Fatalf("NewMockShell() error = %v", err)
			}
			setupContainer(mockHandler, mockHandler, mockShell, nil, nil, nil, nil)

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
			mockHandler := config.NewMockConfigHandler()
			mockHandler.SetValueFunc = func(key string, value interface{}) error { return errors.New("set context error") }
			mockShell, err := shell.NewMockShell("cmd")
			if err != nil {
				t.Fatalf("NewMockShell() error = %v", err)
			}
			setupContainer(mockHandler, mockHandler, mockShell, nil, nil, nil, nil)

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
			mockHandler := config.NewMockConfigHandler()
			mockHandler.SetValueFunc = func(key string, value interface{}) error { return nil }
			mockHandler.SaveConfigFunc = func(path string) error { return errors.New("save config error") }
			mockShell, err := shell.NewMockShell("cmd")
			if err != nil {
				t.Fatalf("NewMockShell() error = %v", err)
			}
			setupContainer(mockHandler, mockHandler, mockShell, nil, nil, nil, nil)

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
	})
}
