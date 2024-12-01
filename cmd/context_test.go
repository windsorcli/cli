package cmd

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	ctrl "github.com/windsor-hotel/cli/internal/controller"
	"github.com/windsor-hotel/cli/internal/di"
)

func TestContext_Get(t *testing.T) {
	originalExitFunc := exitFunc
	exitFunc = mockExit
	t.Cleanup(func() {
		exitFunc = originalExitFunc
	})

	t.Run("Success", func(t *testing.T) {
		// Given a valid config handler
		injector := di.NewMockInjector()
		mockController := ctrl.NewMockController(injector)

		// When the get context command is executed
		output := captureStdout(func() {
			rootCmd.SetArgs([]string{"context", "get"})
			err := Execute(mockController)
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

	t.Run("ErrorInitializingController", func(t *testing.T) {
		// Given a controller with a mocked initializeController that returns an error
		originalInitializeController := initializeController
		initializeController = func() error {
			return fmt.Errorf("Error initializing controller")
		}
		defer func() {
			initializeController = originalInitializeController
		}()

		// When the get context command is executed
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"context", "get"})
			err := Execute(ctrl.NewMockController(di.NewMockInjector()))
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then the output should indicate the error
		expectedOutput := "Error initializing controller"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("Error", func(t *testing.T) {
		// Given a controller with a mocked ResolveContextHandler that returns an error
		injector := di.NewMockInjector()
		mockController := ctrl.NewMockController(injector)
		mockController.ResolveContextHandlerFunc = func() (context.ContextHandler, error) {
			return nil, errors.New("get context error")
		}

		// When the get context command is executed
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"context", "get"})
			err := Execute(mockController)
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

	t.Run("ErrorGettingContext", func(t *testing.T) {
		// Given a context instance that returns an error on GetContext
		injector := di.NewMockInjector()
		mockController := ctrl.NewMockController(injector)
		mockContextHandler := context.NewMockContext()
		mockContextHandler.GetContextFunc = func() (string, error) { return "", fmt.Errorf("get context error") }
		injector.Register("contextHandler", mockContextHandler)

		// Ensure the mock controller returns the mock context handler
		mockController.ResolveContextHandlerFunc = func() (context.ContextHandler, error) {
			return mockContextHandler, nil
		}

		// When the get context command is executed
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"context", "get"})
			err := Execute(mockController)
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
		injector := di.NewMockInjector()
		mockController := ctrl.NewMockController(injector)
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.SetFunc = func(key string, value interface{}) error { return nil }
		mockConfigHandler.SaveConfigFunc = func(path string) error { return nil }
		injector.Register("configHandler", mockConfigHandler)
		mockContextHandler := context.NewMockContext()
		mockContextHandler.SetContextFunc = func(contextName string) error { return nil }
		injector.Register("contextHandler", mockContextHandler)

		// When the set context command is executed with a valid context
		output := captureStdout(func() {
			rootCmd.SetArgs([]string{"context", "set", "new-context"})
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

	t.Run("ErrorInitializingController", func(t *testing.T) {
		// Given a controller with a mocked initializeController that returns an error
		originalInitializeController := initializeController
		initializeController = func() error {
			return fmt.Errorf("Error initializing controller")
		}
		defer func() { initializeController = originalInitializeController }()

		// When the set context command is executed
		mockController := ctrl.NewMockController(di.NewMockInjector())
		rootCmd.SetArgs([]string{"context", "set", "new-context"})
		err := Execute(mockController)
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		// Then the output should indicate the error
		expectedOutput := "Error initializing controller"
		if !strings.Contains(err.Error(), expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, err.Error())
		}
	})

	t.Run("ResolveContextHandlerError", func(t *testing.T) {
		// Given a controller that returns an error on ResolveContextHandler
		injector := di.NewInjector()
		mockController := ctrl.NewMockController(injector)

		// Mock the ResolveContextHandler to return an error
		mockController.ResolveContextHandlerFunc = func() (context.ContextHandler, error) {
			return nil, fmt.Errorf("resolve context handler error")
		}

		// When the set context command is executed
		rootCmd.SetArgs([]string{"context", "set", "new-context"})
		err := Execute(mockController)
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		// Then the output should indicate the error
		expectedOutput := "resolve context handler error"
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
		mockController.ResolveContextHandlerFunc = func() (context.ContextHandler, error) {
			return mockContextHandler, nil
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
		mockConfigHandler.SetFunc = func(key string, value interface{}) error { return nil }
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
		mockController.ResolveContextHandlerFunc = func() (context.ContextHandler, error) {
			return mockContextHandler, nil
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

func TestContext_initializeController(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		injector := di.NewMockInjector()
		originalController := controller
		controller = ctrl.NewMockController(injector)
		defer func() { controller = originalController }()

		// When the initializeController function is called
		err := initializeController()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
	})

	t.Run("ErrorInitializingController", func(t *testing.T) {
		injector := di.NewMockInjector()
		originalController := controller
		mockController := ctrl.NewMockController(injector)
		controller = mockController
		defer func() { controller = originalController }()

		// Mock the Initialize function to return an error
		mockController.InitializeFunc = func() error {
			return fmt.Errorf("Error initializing controller")
		}

		// When the initializeController function is called
		err := initializeController()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		expectedError := "Error initializing controller: Error initializing controller"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorCreatingCommonComponents", func(t *testing.T) {
		injector := di.NewMockInjector()
		originalController := controller
		mockController := ctrl.NewMockController(injector)
		controller = mockController
		defer func() { controller = originalController }()

		// Mock the CreateCommonComponents function to return an error
		mockController.CreateCommonComponentsFunc = func() error {
			return fmt.Errorf("Error creating common components")
		}

		// When the initializeController function is called
		err := initializeController()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		expectedError := "Error creating common components: Error creating common components"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorInitializingComponents", func(t *testing.T) {
		injector := di.NewMockInjector()
		originalController := controller
		mockController := ctrl.NewMockController(injector)
		controller = mockController
		defer func() { controller = originalController }()

		// Mock the InitializeComponents function to return an error
		mockController.InitializeComponentsFunc = func() error {
			return fmt.Errorf("Error initializing components")
		}

		// When the initializeController function is called
		err := initializeController()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		expectedError := "Error initializing components: Error initializing components"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})
}
