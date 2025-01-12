package cmd

import (
	"fmt"
	"testing"

	ctrl "github.com/windsorcli/cli/pkg/controller"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/tools"
)

func TestCheckCommand(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Create a mock controller
		injector := di.NewInjector()
		mockController := ctrl.NewMockController(injector)

		// When: the check command is executed
		output := captureStdout(func() {
			rootCmd.SetArgs([]string{"check"})
			err := Execute(mockController)
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
		})

		// Then: the output should confirm that all tools are up to date
		expectedOutput := "All tools are up to date.\n"
		if output != expectedOutput {
			t.Errorf("Expected output %q, got %q", expectedOutput, output)
		}
	})

	t.Run("CheckToolsErrorNoToolsManager", func(t *testing.T) {
		// Create a mock controller that returns nil for tools manager
		injector := di.NewInjector()
		mockController := ctrl.NewMockController(injector)
		mockController.ResolveToolsManagerFunc = func() tools.ToolsManager {
			return nil
		}

		// When: the check command is executed
		err := Execute(mockController)

		// Then: it should return an error indicating no tools manager found
		if err == nil || err.Error() != "No tools manager found" {
			t.Errorf("Expected error 'No tools manager found', got %v", err)
		}
	})

	t.Run("CheckToolsErrorCheckingTools", func(t *testing.T) {
		// Create a mock controller with a tools manager that returns an error
		injector := di.NewInjector()
		mockController := ctrl.NewMockController(injector)
		mockToolsManager := &tools.MockToolsManager{
			CheckFunc: func() error {
				return fmt.Errorf("mock error checking tools")
			},
		}
		mockController.ResolveToolsManagerFunc = func() tools.ToolsManager {
			return mockToolsManager
		}

		// When: the check command is executed
		err := Execute(mockController)

		// Then: it should return an error indicating a problem checking tools
		if err == nil || err.Error() != "Error checking tools: mock error checking tools" {
			t.Errorf("Expected error 'Error checking tools: mock error checking tools', got %v", err)
		}
	})

	t.Run("ErrorCreatingProjectComponents", func(t *testing.T) {
		// Create a mock controller with an error in CreateProjectComponents
		injector := di.NewInjector()
		mockController := ctrl.NewMockController(injector)
		mockController.CreateProjectComponentsFunc = func() error {
			return fmt.Errorf("mock error creating project components")
		}

		// When: the check command is executed
		err := Execute(mockController)

		// Then: it should return an error indicating a problem creating project components
		if err == nil || err.Error() != "Error creating project components: mock error creating project components" {
			t.Errorf("Expected error 'Error creating project components: mock error creating project components', got %v", err)
		}
	})

	t.Run("ErrorInitializingComponents", func(t *testing.T) {
		// Create a mock controller with an error in InitializeComponents
		injector := di.NewInjector()
		mockController := ctrl.NewMockController(injector)
		mockController.InitializeComponentsFunc = func() error {
			return fmt.Errorf("mock error initializing components")
		}

		// When: the check command is executed
		err := Execute(mockController)

		// Then: it should return an error indicating a problem initializing components
		if err == nil || err.Error() != "Error initializing components: mock error initializing components" {
			t.Errorf("Expected error 'Error initializing components: mock error initializing components', got %v", err)
		}
	})
}
