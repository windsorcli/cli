package cmd

import (
	"fmt"
	"testing"

	ctrl "github.com/windsorcli/cli/internal/controller"
	"github.com/windsorcli/cli/internal/di"
	"github.com/windsorcli/cli/internal/env"
)

func TestEnvCmd(t *testing.T) {
	originalExitFunc := exitFunc
	exitFunc = mockExit
	t.Cleanup(func() {
		exitFunc = originalExitFunc
	})

	t.Run("Success", func(t *testing.T) {
		defer resetRootCmd()

		// Initialize mocks and set the injector
		injector := di.NewInjector()
		mockController := ctrl.NewMockController(injector)

		// Mock the GetEnvPrinters method to return the mockEnv
		mockEnv := env.NewMockEnvPrinter()
		mockEnv.PrintFunc = func() error {
			fmt.Println("export VAR=value")
			return nil
		}
		mockController.ResolveAllEnvPrintersFunc = func() []env.EnvPrinter {
			return []env.EnvPrinter{mockEnv}
		}

		// Capture the output using captureStdout
		output := captureStdout(func() {
			rootCmd.SetArgs([]string{"env", "--verbose"})
			err := Execute(mockController)
			if err != nil {
				t.Fatalf("Expected no error, got %v", err)
			}
		})

		// Verify the output
		expectedOutput := "export VAR=value\n"
		if output != expectedOutput {
			t.Errorf("Expected output %q, got %q", expectedOutput, output)
		}
	})

	t.Run("ErrorCreatingEnvComponents", func(t *testing.T) {
		defer resetRootCmd()

		// Save the original controller and restore it after the test
		originalController := controller
		defer func() {
			controller = originalController
		}()

		// Given a mock controller that returns an error when creating environment components
		injector := di.NewInjector()
		mockController := ctrl.NewMockController(injector)
		mockController.CreateEnvComponentsFunc = func() error {
			return fmt.Errorf("error creating environment components")
		}

		// Set the global controller to the mock controller
		controller = mockController

		// When the env command is executed with verbose flag
		rootCmd.SetArgs([]string{"env", "--verbose"})
		err := Execute(mockController)

		// Then check the error contents
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
		expectedError := "Error creating environment components: error creating environment components"
		if err.Error() != expectedError {
			t.Fatalf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorCreatingEnvComponentsWithoutVerbose", func(t *testing.T) {
		defer resetRootCmd()

		// Given a mock controller that returns an error when creating environment components
		injector := di.NewInjector()
		mockController := ctrl.NewMockController(injector)
		mockController.CreateEnvComponentsFunc = func() error {
			return fmt.Errorf("error creating environment components")
		}

		// Set the global controller to the mock controller
		controller = mockController

		// When the env command is executed without verbose flag
		rootCmd.SetArgs([]string{"env"})
		err := Execute(mockController)

		// Then the error should be nil and no output should be produced
		if err != nil {
			t.Fatalf("Expected error nil, got %v", err)
		}
	})

	t.Run("ErrorInitializingComponents", func(t *testing.T) {
		defer resetRootCmd()

		// Given a mock controller that returns an error when initializing components
		injector := di.NewInjector()
		mockController := ctrl.NewMockController(injector)
		mockController.InitializeComponentsFunc = func() error {
			return fmt.Errorf("error initializing components")
		}

		// Set the global controller to the mock controller
		controller = mockController

		// When the env command is executed with verbose flag
		rootCmd.SetArgs([]string{"env", "--verbose"})
		err := Execute(mockController)

		// Then check the error contents
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
		expectedError := "Error initializing components: error initializing components"
		if err.Error() != expectedError {
			t.Fatalf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ErrorInitializingComponentsWithoutVerbose", func(t *testing.T) {
		defer resetRootCmd()

		// Given a mock controller that returns an error when initializing components
		injector := di.NewInjector()
		mockController := ctrl.NewMockController(injector)
		mockController.InitializeComponentsFunc = func() error {
			return fmt.Errorf("error initializing components")
		}

		// Set the global controller to the mock controller
		controller = mockController

		// When the env command is executed without verbose flag
		rootCmd.SetArgs([]string{"env"})
		err := Execute(mockController)

		// Then the error should be nil and no output should be produced
		if err != nil {
			t.Fatalf("Expected error nil, got %v", err)
		}
	})

	t.Run("ResolveAllEnvPrintersErrorWithoutVerbose", func(t *testing.T) {
		defer resetRootCmd()

		// Given a mock controller that returns an error when resolving all environment printers
		injector := di.NewInjector()
		mockController := ctrl.NewMockController(injector)
		mockController.ResolveAllEnvPrintersFunc = func() []env.EnvPrinter {
			return nil
		}

		// Set the global controller to the mock controller
		controller = mockController

		// When the env command is executed without verbose flag
		rootCmd.SetArgs([]string{"env"})
		err := Execute(mockController)

		// Then the error should be nil and no output should be produced
		if err != nil {
			t.Fatalf("Expected error nil, got %v", err)
		}
	})

	t.Run("ErrorResolvingAllEnvPrinters", func(t *testing.T) {
		defer resetRootCmd()

		// Given a mock controller that returns an empty list of environment printers
		injector := di.NewInjector()
		mockController := ctrl.NewMockController(injector)
		mockController.ResolveAllEnvPrintersFunc = func() []env.EnvPrinter {
			return []env.EnvPrinter{}
		}

		// Set the global controller to the mock controller
		controller = mockController

		// When the env command is executed with verbose flag
		rootCmd.SetArgs([]string{"env", "--verbose"})
		err := Execute(mockController)

		// Then check the error contents
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
		expectedError := "Error resolving environment printers: no printers returned"
		if err.Error() != expectedError {
			t.Fatalf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("PrintError", func(t *testing.T) {
		defer resetRootCmd()

		// Given a mock controller that returns a valid list of environment printers
		injector := di.NewInjector()
		mockController := ctrl.NewMockController(injector)
		mockEnvPrinter := env.NewMockEnvPrinter()
		mockEnvPrinter.PrintFunc = func() error {
			return fmt.Errorf("print error")
		}
		mockController.ResolveAllEnvPrintersFunc = func() []env.EnvPrinter {
			return []env.EnvPrinter{mockEnvPrinter}
		}

		// Set the global controller to the mock controller
		controller = mockController

		// When the env command is executed with verbose flag
		rootCmd.SetArgs([]string{"env", "--verbose"})
		err := Execute(mockController)

		// Then check the error contents
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
		expectedError := "Error executing Print: print error"
		if err.Error() != expectedError {
			t.Fatalf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("PrintErrorWithoutVerbose", func(t *testing.T) {
		defer resetRootCmd()

		// Given a mock controller that returns a valid list of environment printers
		injector := di.NewInjector()
		mockController := ctrl.NewMockController(injector)
		mockEnvPrinter := env.NewMockEnvPrinter()
		mockEnvPrinter.PrintFunc = func() error {
			return fmt.Errorf("print error")
		}
		mockController.ResolveAllEnvPrintersFunc = func() []env.EnvPrinter {
			return []env.EnvPrinter{mockEnvPrinter}
		}

		// Set the global controller to the mock controller
		controller = mockController

		// When the env command is executed without verbose flag
		rootCmd.SetArgs([]string{"env"})
		err := Execute(mockController)

		// Then the error should be nil and no output should be produced
		if err != nil {
			t.Fatalf("Expected error nil, got %v", err)
		}
	})

	t.Run("PostEnvHookError", func(t *testing.T) {
		defer resetRootCmd()

		// Given a mock controller that returns a valid list of environment printers
		injector := di.NewInjector()
		mockController := ctrl.NewMockController(injector)
		mockEnvPrinter := env.NewMockEnvPrinter()
		mockEnvPrinter.PostEnvHookFunc = func() error {
			return fmt.Errorf("post env hook error")
		}
		mockController.ResolveAllEnvPrintersFunc = func() []env.EnvPrinter {
			return []env.EnvPrinter{mockEnvPrinter}
		}

		// Set the global controller to the mock controller
		controller = mockController

		// When the env command is executed with verbose flag
		rootCmd.SetArgs([]string{"env", "--verbose"})
		err := Execute(mockController)

		// Then check the error contents
		if err == nil {
			t.Fatalf("Expected an error, got nil")
		}
		expectedError := "Error executing PostEnvHook: post env hook error"
		if err.Error() != expectedError {
			t.Fatalf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("PostEnvHookErrorWithoutVerbose", func(t *testing.T) {
		defer resetRootCmd()

		// Given a mock controller that returns a valid list of environment printers
		injector := di.NewInjector()
		mockController := ctrl.NewMockController(injector)
		mockEnvPrinter := env.NewMockEnvPrinter()
		mockEnvPrinter.PostEnvHookFunc = func() error {
			return fmt.Errorf("post env hook error")
		}
		mockController.ResolveAllEnvPrintersFunc = func() []env.EnvPrinter {
			return []env.EnvPrinter{mockEnvPrinter}
		}

		// Set the global controller to the mock controller
		controller = mockController

		// When the env command is executed without verbose flag
		rootCmd.SetArgs([]string{"env"})
		err := Execute(mockController)

		// Then the error should be nil and no output should be produced
		if err != nil {
			t.Fatalf("Expected error nil, got %v", err)
		}
	})
}

// resetRootCmd resets the root command to its initial state.
func resetRootCmd() {
	rootCmd.SetArgs([]string{})
	rootCmd.SetOut(nil)
	rootCmd.SetErr(nil)
	verbose = false // Reset the verbose flag
}
