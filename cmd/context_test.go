package cmd

import (
	"errors"
	"strings"
	"testing"

	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/mocks"
)

func TestContext_Get(t *testing.T) {
	originalExitFunc := exitFunc
	exitFunc = mockExit
	t.Cleanup(func() {
		exitFunc = originalExitFunc
	})

	t.Run("Success", func(t *testing.T) {
		// Given a valid config handler
		mocks := mocks.CreateSuperMocks()
		mocks.ContextHandler.GetContextFunc = func() (string, error) { return "test-context", nil }

		// When the get context command is executed
		output := captureStdout(func() {
			rootCmd.SetArgs([]string{"context", "get"})
			err := Execute(mocks.Injector)
			if err != nil {
				if strings.Contains(err.Error(), "no instance registered with name contextHandler") {
					t.Fatalf("Error resolving contextHandler: %v", err)
				} else {
					t.Fatalf("Execute() error = %v", err)
				}
			}
		})

		// Then the output should indicate the current context
		expectedOutput := "test-context\n"
		if output != expectedOutput {
			t.Errorf("Expected output %q, got %q", expectedOutput, output)
		}
	})

	t.Run("ErrorGettingContextHandler", func(t *testing.T) {
		// Save the original getContextHandler function
		originalGetContextHandler := getContextHandler

		// Temporarily replace getContextHandler to return an error
		getContextHandler = func() (context.ContextInterface, error) {
			return nil, errors.New("mock error resolving context handler")
		}
		defer func() {
			getContextHandler = originalGetContextHandler // Restore original function after test
		}()

		// Given a mock injector
		mocks := mocks.CreateSuperMocks()

		// When the get context command is executed
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"context", "get"})
			err := Execute(mocks.Injector)
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then the output should indicate the error
		expectedOutput := "mock error resolving context handler"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("Error", func(t *testing.T) {
		// Given a config handler that returns an error on GetConfigValue
		mocks := mocks.CreateSuperMocks()
		mocks.ContextHandler.GetContextFunc = func() (string, error) {
			return "", errors.New("get context error")
		}

		// When the get context command is executed
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"context", "get"})
			err := Execute(mocks.Injector)
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
	exitFunc = mockExit
	t.Cleanup(func() {
		exitFunc = originalExitFunc
	})

	t.Run("Success", func(t *testing.T) {
		// Given a valid config handler
		mocks := mocks.CreateSuperMocks()
		mocks.ConfigHandler.SetFunc = func(key string, value interface{}) error { return nil }
		mocks.ConfigHandler.SaveConfigFunc = func(path string) error { return nil }
		mocks.ContextHandler.SetContextFunc = func(contextName string) error { return nil }
		// When the set context command is executed with a valid context
		output := captureStdout(func() {
			rootCmd.SetArgs([]string{"context", "set", "new-context"})
			err := Execute(mocks.Injector)
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

	t.Run("ErrorGettingContextHandler", func(t *testing.T) {
		// Given a valid config handler
		mocks := mocks.CreateSuperMocks()

		// Override the getContextHandler function to simulate an error
		originalGetContextHandler := getContextHandler
		getContextHandler = func() (context.ContextInterface, error) {
			return nil, errors.New("resolve context handler error")
		}
		defer func() { getContextHandler = originalGetContextHandler }()

		// When the set context command is executed
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"context", "set", "new-context"})
			err := Execute(mocks.Injector)
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then the output should indicate the error
		expectedOutput := "resolve context handler error"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("SetContextError", func(t *testing.T) {
		// Given a context instance that returns an error on SetContext
		mocks := mocks.CreateSuperMocks()
		mocks.ContextHandler.SetContextFunc = func(contextName string) error { return errors.New("set context error") }
		// When the set context command is executed
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"context", "set", "new-context"})
			err := Execute(mocks.Injector)
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

	// t.Run("SaveConfigError", func(t *testing.T) {
	// 	// Given a config handler that returns an error on SaveConfig
	// 	mocks := mocks.CreateSuperMocks()
	// 	mocks.ConfigHandler.SetFunc = func(key string, value interface{}) error { return nil }
	// 	mocks.ConfigHandler.SaveConfigFunc = func(path string) error { return errors.New("save config error") }
	// 	mocks.ContextHandler.SetContextFunc = func(contextName string) error { return nil }
	// 	Initialize(mocks.Container)

	// 	// When the set context command is executed
	// 	output := captureStderr(func() {
	// 		rootCmd.SetArgs([]string{"context", "set", "new-context"})
	// 		err := rootCmd.Execute()
	// 		if err == nil {
	// 			t.Fatalf("Expected error, got nil")
	// 		}
	// 	})

	// 	// Then the output should indicate the error
	// 	expectedOutput := "Error: save config error"
	// 	if !strings.Contains(output, expectedOutput) {
	// 		t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
	// 	}
	// })
}

func TestContext_GetAlias(t *testing.T) {
	originalExitFunc := exitFunc
	exitFunc = mockExit
	t.Cleanup(func() {
		exitFunc = originalExitFunc
	})

	t.Run("Success", func(t *testing.T) {
		// Given a valid context instance
		mocks := mocks.CreateSuperMocks()
		mocks.ContextHandler.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}
		// When the get-context alias command is executed
		output := captureStdout(func() {
			rootCmd.SetArgs([]string{"get-context"})
			err := Execute(mocks.Injector)
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
}

func TestContext_SetAlias(t *testing.T) {
	originalExitFunc := exitFunc
	exitFunc = mockExit
	t.Cleanup(func() {
		exitFunc = originalExitFunc
	})

	t.Run("Success", func(t *testing.T) {
		// Given a valid config handler and context
		mocks := mocks.CreateSuperMocks()
		mocks.ConfigHandler.SetFunc = func(key string, value interface{}) error { return nil }
		mocks.ConfigHandler.SaveConfigFunc = func(path string) error { return nil }
		mocks.ContextHandler.SetContextFunc = func(contextName string) error { return nil }

		// When the set-context alias command is executed with a valid context
		output := captureStdout(func() {
			rootCmd.SetArgs([]string{"set-context", "new-context"})
			err := Execute(mocks.Injector)
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
		// Given a context instance that returns an error on SetContext
		mocks := mocks.CreateSuperMocks()
		mocks.ContextHandler.SetContextFunc = func(contextName string) error { return errors.New("set context error") }

		// When the set-context alias command is executed
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"set-context", "new-context"})
			err := Execute(mocks.Injector)
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
}

func TestContext_getContextHandler(t *testing.T) {
	t.Run("ErrorResolvingContextHandler", func(t *testing.T) {
		// Given an error resolving the context handler
		mockInjector := di.NewMockInjector()
		mocks := mocks.CreateSuperMocks(mockInjector)
		mocks.Injector.SetResolveError("contextHandler", errors.New("resolve context handler error"))

		// Backup the original injector
		originalInjector := injector

		// Set the global injector to the mock injector
		injector = mocks.Injector

		// Ensure the global injector is reset after the test
		defer func() {
			injector = originalInjector
		}()

		// When getContextHandler is called
		_, err := getContextHandler()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		expectedError := "Error resolving contextHandler: resolve context handler error"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorCastingContextHandler", func(t *testing.T) {
		// Given an invalid context handler
		mocks := mocks.CreateSuperMocks()
		mocks.Injector.Register("contextHandler", "invalid")

		// Backup the original injector
		originalInjector := injector

		// Set the global injector to the mock injector
		injector = mocks.Injector

		// Ensure the global injector is reset after the test
		defer func() {
			injector = originalInjector
		}()

		// When getContextHandler is called
		_, err := getContextHandler()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		expectedError := "Error: resolved instance is not of type context.ContextInterface"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})
}
