package cmd

import (
	"fmt"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/config"
	ctrl "github.com/windsorcli/cli/pkg/controller"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
)

type MockSafeContextCmdComponents struct {
	Injector      di.Injector
	Controller    *ctrl.MockController
	ConfigHandler *config.MockConfigHandler
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

	// Setup mock config handler
	mockConfigHandler := config.NewMockConfigHandler()
	mockConfigHandler.SetFunc = func(key string, value any) error {
		return nil
	}
	mockConfigHandler.GetContextFunc = func() string {
		return "mock-context"
	}
	mockConfigHandler.SetContextFunc = func(contextName string) error {
		return nil // Simulate successful context setup
	}
	mockConfigHandler.IsLoadedFunc = func() bool { return true }
	injector.Register("configHandler", mockConfigHandler)

	// Set the ResolveConfigHandlerFunc to return the mock config handler
	mockController.ResolveConfigHandlerFunc = func() config.ConfigHandler {
		return mockConfigHandler
	}

	// Setup mock shell
	mockShell := shell.NewMockShell(injector)
	mockShell.WriteResetTokenFunc = func() (string, error) {
		return "/mock/project/root/.windsor/.session.mock-token", nil
	}
	injector.Register("shell", mockShell)

	// Set the ResolveShellFunc to return the mock shell
	mockController.ResolveShellFunc = func() shell.Shell {
		return mockShell
	}

	// Ensure the IsLoadedFunc returns true
	mockConfigHandler.IsLoadedFunc = func() bool {
		return true
	}

	return MockSafeContextCmdComponents{
		Injector:      injector,
		Controller:    mockController,
		ConfigHandler: mockConfigHandler,
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
			err := Execute(mocks.Controller)
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
		mocks.Controller.InitializeComponentsFunc = func() error {
			return fmt.Errorf("initialization error")
		}

		// When the get context command is executed
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"context", "get"})
			err := Execute(mocks.Controller)
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

	t.Run("ConfigNotLoaded", func(t *testing.T) {
		// Given a config handler that is not loaded
		mocks := setupSafeContextCmdMocks()
		mocks.ConfigHandler.IsLoadedFunc = func() bool { return false }

		// When the get context command is executed
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"context", "get"})
			err := Execute(mocks.Controller)
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then the output should indicate the config is not loaded
		expectedOutput := "No context is available. Have you run `windsor init`?"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("ErrorCreatingEnvComponents", func(t *testing.T) {
		// Given an error creating environment components
		mocks := setupSafeContextCmdMocks()
		mocks.Controller.CreateEnvComponentsFunc = func() error {
			return fmt.Errorf("env components error")
		}

		// When the get context command is executed
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"context", "get"})
			err := Execute(mocks.Controller)
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then the output should indicate the error
		expectedOutput := "Error initializing environment components: env components error"
		if !strings.Contains(output, expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, output)
		}
	})

	t.Run("ErrorSettingEnvironmentVariables", func(t *testing.T) {
		// Given an error when setting environment variables
		mocks := setupSafeContextCmdMocks()
		mocks.Controller.SetEnvironmentVariablesFunc = func() error {
			return fmt.Errorf("environment variables error")
		}

		// When the get context command is executed
		output := captureStderr(func() {
			rootCmd.SetArgs([]string{"context", "get"})
			err := Execute(mocks.Controller)
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
		})

		// Then the output should indicate the error
		expectedOutput := "Error setting environment variables: environment variables error"
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
		mocks := setupSafeContextCmdMocks()

		// When the set context command is executed with a valid context
		output := captureStdout(func() {
			rootCmd.SetArgs([]string{"context", "set", "new-context"})
			err := Execute(mocks.Controller)
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
		mocks := setupSafeContextCmdMocks()
		mocks.Controller.InitializeComponentsFunc = func() error {
			return fmt.Errorf("initialization error")
		}

		// When the set context command is executed
		rootCmd.SetArgs([]string{"context", "set", "new-context"})
		err := Execute(mocks.Controller)
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		// Then the output should indicate the error
		expectedOutput := "initialization error"
		if !strings.Contains(err.Error(), expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, err.Error())
		}
	})

	t.Run("SetContextError", func(t *testing.T) {
		// Given a config handler instance that returns an error on SetContext
		mocks := setupSafeContextCmdMocks()
		mocks.ConfigHandler.SetContextFunc = func(contextName string) error { return fmt.Errorf("set context error") }

		// When the set context command is executed
		rootCmd.SetArgs([]string{"context", "set", "new-context"})
		err := Execute(mocks.Controller)
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		// Then the output should indicate the error
		expectedOutput := "set context error"
		if !strings.Contains(err.Error(), expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, err.Error())
		}
	})

	t.Run("ConfigNotLoaded", func(t *testing.T) {
		// Given a config handler that is not loaded
		mocks := setupSafeContextCmdMocks()
		mocks.ConfigHandler.IsLoadedFunc = func() bool { return false }

		// When the set context command is executed
		rootCmd.SetArgs([]string{"context", "set", "new-context"})
		err := Execute(mocks.Controller)
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		// Then the output should indicate the config is not loaded
		expectedOutput := "Configuration is not loaded. Please ensure it is initialized."
		if !strings.Contains(err.Error(), expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, err.Error())
		}
	})

	t.Run("ErrorCreatingEnvComponents", func(t *testing.T) {
		// Given an error creating environment components
		mocks := setupSafeContextCmdMocks()
		mocks.Controller.CreateEnvComponentsFunc = func() error {
			return fmt.Errorf("env components error")
		}

		// When the set context command is executed
		rootCmd.SetArgs([]string{"context", "set", "new-context"})
		err := Execute(mocks.Controller)
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		// Then the output should indicate the error
		expectedOutput := "Error initializing environment components: env components error"
		if !strings.Contains(err.Error(), expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, err.Error())
		}
	})

	t.Run("ErrorWritingResetToken", func(t *testing.T) {
		// Given a mock shell that returns an error on WriteResetToken
		mocks := setupSafeContextCmdMocks()
		mockShell := shell.NewMockShell()
		mockShell.WriteResetTokenFunc = func() (string, error) {
			return "", fmt.Errorf("error writing reset token")
		}
		mocks.Controller.ResolveShellFunc = func() shell.Shell {
			return mockShell
		}

		// When the set context command is executed
		rootCmd.SetArgs([]string{"context", "set", "new-context"})
		err := Execute(mocks.Controller)
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		// Then the output should indicate the error
		expectedOutput := "Error writing reset token: error writing reset token"
		if !strings.Contains(err.Error(), expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, err.Error())
		}
	})

	t.Run("ErrorSettingEnvironmentVariables", func(t *testing.T) {
		// Given a controller that returns an error when setting environment variables
		mocks := setupSafeContextCmdMocks()
		mocks.Controller.SetEnvironmentVariablesFunc = func() error {
			return fmt.Errorf("error setting environment variables")
		}

		// When the set context command is executed
		rootCmd.SetArgs([]string{"context", "set", "new-context"})
		err := Execute(mocks.Controller)
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}

		// Then the output should indicate the error
		expectedOutput := "Error setting environment variables: error setting environment variables"
		if !strings.Contains(err.Error(), expectedOutput) {
			t.Errorf("Expected output to contain %q, got %q", expectedOutput, err.Error())
		}
	})

	t.Run("ResetTokenWrittenBeforeContextChange", func(t *testing.T) {
		// Given a controller that tracks execution order
		mocks := setupSafeContextCmdMocks()

		var executionOrder []string

		// Mock WriteResetToken to record when it's called
		mockShell := shell.NewMockShell()
		mockShell.WriteResetTokenFunc = func() (string, error) {
			executionOrder = append(executionOrder, "WriteResetToken")
			return "/mock/path", nil
		}
		mocks.Controller.ResolveShellFunc = func() shell.Shell {
			return mockShell
		}

		// Mock SetContext to record when it's called
		mocks.ConfigHandler.SetContextFunc = func(contextName string) error {
			executionOrder = append(executionOrder, "SetContext")
			return nil
		}

		// When the set context command is executed
		rootCmd.SetArgs([]string{"context", "set", "new-context"})
		err := Execute(mocks.Controller)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		// Then WriteResetToken should be called before SetContext
		if len(executionOrder) != 2 {
			t.Fatalf("Expected 2 operations, got %d: %v", len(executionOrder), executionOrder)
		}

		if executionOrder[0] != "WriteResetToken" || executionOrder[1] != "SetContext" {
			t.Errorf("Expected WriteResetToken to be called before SetContext, got order: %v", executionOrder)
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
		// Given a valid config handler
		mocks := setupSafeContextCmdMocks()

		// Log the state of the ConfigHandle

		// When the get-context alias command is executed
		output := captureStdout(func() {
			rootCmd.SetArgs([]string{"get-context"})
			err := Execute(mocks.Controller)
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
		defer resetRootCmd()

		// Given a valid config handler and context
		mocks := setupSafeContextCmdMocks()

		// When the set-context alias command is executed with a valid context
		output := captureStdout(func() {
			rootCmd.SetArgs([]string{"set-context", "new-context"})
			err := Execute(mocks.Controller)
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

	t.Run("SetContextError", func(t *testing.T) { // Given a config handler instance that returns an error on SetContext
		defer resetRootCmd()

		mocks := setupSafeContextCmdMocks()
		mocks.ConfigHandler.SetContextFunc = func(contextName string) error { return fmt.Errorf("set context error") }

		// When the set-context alias command is executed
		rootCmd.SetArgs([]string{"set-context", "new-context"})
		err := Execute(mocks.Controller)
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
