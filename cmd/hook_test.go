package cmd

import (
	"fmt"
	"testing"

	ctrl "github.com/windsorcli/cli/pkg/controller"
	"github.com/windsorcli/cli/pkg/di"
)

func TestHookCmd(t *testing.T) {
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
}
