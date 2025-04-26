package cmd

import (
	"fmt"
	"testing"

	"github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/config"
	ctrl "github.com/windsorcli/cli/pkg/controller"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
	"github.com/windsorcli/cli/pkg/tools"
)

// MockSafeCheckCmdComponents holds the mock components for testing the check command
type MockSafeCheckCmdComponents struct {
	Injector          di.Injector
	MockController    *ctrl.MockController
	MockConfigHandler *config.MockConfigHandler
	MockShell         *shell.MockShell
}

// setupSafeCheckCmdMocks creates mock components for testing the check command
func setupSafeCheckCmdMocks(optionalInjector ...di.Injector) MockSafeCheckCmdComponents {
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

	// Set up controller mock functions
	mockController.InitializeFunc = func() error { return nil }
	mockController.CreateCommonComponentsFunc = func() error { return nil }
	mockController.InitializeComponentsFunc = func() error { return nil }
	mockController.CreateProjectComponentsFunc = func() error { return nil }

	// Initialize the controller
	if err := mockController.Initialize(); err != nil {
		panic(fmt.Sprintf("Failed to initialize controller: %v", err))
	}

	// Setup mock config handler
	mockConfigHandler := config.NewMockConfigHandler()
	mockConfigHandler.IsLoadedFunc = func() bool { return true }
	mockConfigHandler.SetContextFunc = func(context string) error { return nil }
	mockConfigHandler.GetContextFunc = func() string { return "local" }
	mockConfigHandler.SetDefaultFunc = func(cfg v1alpha1.Context) error { return nil }
	mockConfigHandler.SaveConfigFunc = func(path string) error { return nil }
	injector.Register("configHandler", mockConfigHandler)

	// Setup mock shell
	mockShell := shell.NewMockShell()
	mockShell.AddCurrentDirToTrustedFileFunc = func() error { return nil }
	mockShell.GetProjectRootFunc = func() (string, error) { return "/path/to/project/root", nil }
	injector.Register("shell", mockShell)

	// Setup mock tools manager
	mockToolsManager := tools.NewMockToolsManager()
	mockToolsManager.CheckFunc = func() error { return nil }
	injector.Register("toolsManager", mockToolsManager)

	mocks := MockSafeCheckCmdComponents{
		Injector:          injector,
		MockController:    mockController,
		MockConfigHandler: mockConfigHandler,
		MockShell:         mockShell,
	}

	mockController.ResolveConfigHandlerFunc = func() config.ConfigHandler {
		return mocks.MockConfigHandler
	}

	mockController.ResolveShellFunc = func() shell.Shell {
		return mocks.MockShell
	}

	mockController.ResolveToolsManagerFunc = func() tools.ToolsManager {
		return mockToolsManager
	}

	return mocks
}

func TestCheckCommand(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Create mock components
		mocks := setupSafeCheckCmdMocks()

		// When: the check command is executed
		output := captureStdout(func() {
			rootCmd.SetArgs([]string{"check"})
			err := Execute(mocks.MockController)
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
		// Create mock components
		mocks := setupSafeCheckCmdMocks()
		mocks.MockController.ResolveToolsManagerFunc = func() tools.ToolsManager {
			return nil
		}

		// When: the check command is executed
		err := Execute(mocks.MockController)

		// Then: it should return an error indicating no tools manager found
		if err == nil || err.Error() != "No tools manager found" {
			t.Errorf("Expected error 'No tools manager found', got %v", err)
		}
	})

	t.Run("CheckToolsErrorCheckingTools", func(t *testing.T) {
		// Create mock components
		mocks := setupSafeCheckCmdMocks()
		mockToolsManager := tools.NewMockToolsManager()
		mockToolsManager.CheckFunc = func() error {
			return fmt.Errorf("mock error checking tools")
		}
		mocks.MockController.ResolveToolsManagerFunc = func() tools.ToolsManager {
			return mockToolsManager
		}

		// When: the check command is executed
		err := Execute(mocks.MockController)

		// Then: it should return an error indicating a problem checking tools
		if err == nil || err.Error() != "Error checking tools: mock error checking tools" {
			t.Errorf("Expected error 'Error checking tools: mock error checking tools', got %v", err)
		}
	})

	t.Run("ErrorCreatingProjectComponents", func(t *testing.T) {
		// Create mock components
		mocks := setupSafeCheckCmdMocks()
		mocks.MockController.CreateProjectComponentsFunc = func() error {
			return fmt.Errorf("mock error creating project components")
		}

		// When: the check command is executed
		err := Execute(mocks.MockController)

		// Then: it should return an error indicating a problem creating project components
		if err == nil || err.Error() != "Error creating project components: mock error creating project components" {
			t.Errorf("Expected error 'Error creating project components: mock error creating project components', got %v", err)
		}
	})

	t.Run("ErrorInitializingComponents", func(t *testing.T) {
		// Create mock components
		mocks := setupSafeCheckCmdMocks()
		mocks.MockController.InitializeComponentsFunc = func() error {
			return fmt.Errorf("mock error initializing components")
		}

		// When: the check command is executed
		err := Execute(mocks.MockController)

		// Then: it should return an error indicating a problem initializing components
		if err == nil || err.Error() != "Error initializing components: mock error initializing components" {
			t.Errorf("Expected error 'Error initializing components: mock error initializing components', got %v", err)
		}
	})
}
