package cmd

import (
	"fmt"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/context"
	ctrl "github.com/windsorcli/cli/pkg/controller"
	"github.com/windsorcli/cli/pkg/di"
)

type MockSafeContextCmdComponents struct {
	Injector           di.Injector
	MockController     *ctrl.MockController
	MockContextHandler *context.MockContext
	MockConfigHandler  *config.MockConfigHandler
}

// setupSafeContextCmdMocks creates mock components for testing the context command
func setupSafeContextCmdMocks(optionalInjector ...di.Injector) MockSafeContextCmdComponents {
	var mockController *ctrl.MockController
	var injector di.Injector

	// Use the provided injector if passed, otherwise create a new one
	if len(optionalInjector) > 0 {
		injector = optionalInjector[0]
	} else {
		injector = di.NewInjector()
	}

	// Use the injector to create a mock controller
	mockController = ctrl.NewMockController(injector)

	// Setup mock context handler
	mockContextHandler := context.NewMockContext()
	mockContextHandler.GetContextFunc = func() string {
		return "mock-context"
	}
	injector.Register("contextHandler", mockContextHandler)

	// Setup mock config handler
	mockConfigHandler := config.NewMockConfigHandler()
	mockConfigHandler.SetFunc = func(key string, value interface{}) error {
		return nil
	}
	injector.Register("configHandler", mockConfigHandler)

	return MockSafeContextCmdComponents{
		Injector:           injector,
		MockController:     mockController,
		MockContextHandler: mockContextHandler,
		MockConfigHandler:  mockConfigHandler,
	}
}

func TestContext_Get(t *testing.T) {
	originalExitFunc := exitFunc
	exitFunc = mockExit
	t.Cleanup(func() {
		exitFunc = originalExitFunc
	})

	t.Run("Success", func(t *testing.T) {
		// Given a valid config handler
		mocks := setupSafeContextCmdMocks()

		// When the get context command is executed
		output := captureStdout(func() {
			rootCmd.SetArgs([]string{"context", "get"})
			err := Execute(mocks.MockController)
			if err != nil {
				if strings.Contains(err.Error(), "no instance registered with name contextHandler") {
					t.Fatalf("Error resolving contextHandler: %v", err)
				} else {
					t.Fatalf("Execute() error = %v", err)
				}
			}
		})

		// Then the output should indicate the current context
		expectedOutput := "mock-context\n"
		if output != expectedOutput {
			t.Errorf("Expected output %q, got %q", expectedOutput, output)
		}
	})

	t.Run("ErrorInitializingComponents", func(t *testing.T) {
		// Given an error initializing components
		mocks := setupSafeContextCmdMocks()
		mocks.MockController.InitializeComponentsFunc = func() error {
			return fmt.Errorf("initialization error")
		}

		// When the get context command is executed
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"context", "get"})
			err := Execute(mocks.MockController)
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then the output should indicate the error
		expectedOutput := "Error initializing components: initialization error"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("ErrorResolvingContextHandler", func(t *testing.T) {
		// Given a controller with a mocked ResolveContextHandler that returns nil
		mocks := setupSafeContextCmdMocks()
		mocks.MockController.ResolveContextHandlerFunc = func() context.ContextHandler {
			return nil
		}

		// When the get context command is executed
		rootCmd.SetArgs([]string{"context", "get"})
		err := Execute(mocks.MockController)
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		// Then the error should indicate the issue
		expectedError := "No context handler found"
		if !strings.Contains(err.Error(), expectedError) {
			t.Errorf("Expected error to contain %q, got %q", expectedError, err.Error())
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
		mocks := setupSafeContextCmdMocks()

		// When the set context command is executed with a valid context
		output := captureStdout(func() {
			rootCmd.SetArgs([]string{"context", "set", "new-context"})
			err := Execute(mocks.MockController)
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

	t.Run("ErrorInitializingComponents", func(t *testing.T) {
		// Given an error initializing components
		mockController := ctrl.NewMockController(di.NewInjector())
		mockController.InitializeComponentsFunc = func() error {
			return fmt.Errorf("initialization error")
		}

		// When the set context command is executed
		rootCmd.SetArgs([]string{"context", "set", "new-context"})
		err := Execute(mockController)
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		// Then the output should indicate the error
		expectedOutput := "initialization error"
		if !strings.Contains(err.Error(), expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, err.Error())
		}
	})

	t.Run("ErrorResolvingContextHandler", func(t *testing.T) {
		// Given a controller that returns nil for ResolveContextHandler
		injector := di.NewInjector()
		mockController := ctrl.NewMockController(injector)
		mockController.ResolveContextHandlerFunc = func() context.ContextHandler {
			return nil
		}

		// When the set context command is executed
		rootCmd.SetArgs([]string{"context", "set", "new-context"})
		err := Execute(mockController)
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		// Then the output should indicate the error
		expectedOutput := "No context handler found"
		if !strings.Contains(err.Error(), expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, err.Error())
		}
	})

	t.Run("SetContextError", func(t *testing.T) {
		// Given a context instance that returns an error on SetContext
		injector := di.NewInjector()
		mockController := ctrl.NewMockController(injector)

		// Mock and register contextHandler
		mockContextHandler := context.NewMockContext()
		mockContextHandler.SetContextFunc = func(contextName string) error { return fmt.Errorf("set context error") }
		injector.Register("contextHandler", mockContextHandler)

		// Ensure the mock controller returns the mock context handler
		mockController.ResolveContextHandlerFunc = func() context.ContextHandler {
			return mockContextHandler
		}

		// When the set context command is executed
		rootCmd.SetArgs([]string{"context", "set", "new-context"})
		err := Execute(mockController)
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		// Then the output should indicate the error
		expectedOutput := "set context error"
		if !strings.Contains(err.Error(), expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, err.Error())
		}
	})
}

func TestContext_GetAlias(t *testing.T) {
	originalExitFunc := exitFunc
	exitFunc = mockExit
	t.Cleanup(func() {
		exitFunc = originalExitFunc
	})

	t.Run("Success", func(t *testing.T) {
		// Given a valid context instance
		injector := di.NewMockInjector()
		mockController := ctrl.NewMockController(injector)

		// When the get-context alias command is executed
		output := captureStdout(func() {
			rootCmd.SetArgs([]string{"get-context"})
			err := Execute(mockController)
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
		})

		// Then the output should indicate the current context
		expectedOutput := "mock-context\n"
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
		injector := di.NewMockInjector()
		mockController := ctrl.NewMockController(injector)
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.SaveConfigFunc = func(path string) error { return nil }
		injector.Register("configHandler", mockConfigHandler)
		mockContextHandler := context.NewMockContext()
		mockContextHandler.SetContextFunc = func(contextName string) error { return nil }
		injector.Register("contextHandler", mockContextHandler)

		// When the set-context alias command is executed with a valid context
		output := captureStdout(func() {
			rootCmd.SetArgs([]string{"set-context", "new-context"})
			err := Execute(mockController)
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

	t.Run("SetContextError", func(t *testing.T) { // Given a context instance that returns an error on SetContext
		injector := di.NewInjector()
		mockContextHandler := context.NewMockContext()
		mockContextHandler.SetContextFunc = func(contextName string) error { return fmt.Errorf("set context error") }
		injector.Register("contextHandler", mockContextHandler)

		// Modify MockController to return the mockContextHandler for ResolveContextHandler
		mockController := ctrl.NewMockController(injector)
		mockController.ResolveContextHandlerFunc = func() context.ContextHandler {
			return mockContextHandler
		}

		// When the set-context alias command is executed
		rootCmd.SetArgs([]string{"set-context", "new-context"})
		err := Execute(mockController)
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		// Then the error should indicate the issue
		expectedOutput := "set context error"
		if !strings.Contains(err.Error(), expectedOutput) {
			t.Errorf("Expected error to contain %q, got %q", expectedOutput, err.Error())
		}
	})
}
