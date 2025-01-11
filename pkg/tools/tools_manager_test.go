package tools

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
)

type MockToolsComponents struct {
	Injector      di.Injector
	ConfigHandler *config.MockConfigHandler
}

// setupToolsMocks function creates safe mocks for the tools manager
func setupToolsMocks(injector ...di.Injector) MockToolsComponents {
	// Mock the dependencies for the tools manager
	var mockInjector di.Injector
	if len(injector) > 0 {
		mockInjector = injector[0]
	} else {
		mockInjector = di.NewInjector()
	}

	// Create a mock config handler
	mockConfigHandler := config.NewMockConfigHandler()

	// Register the mock config handler in the injector
	mockInjector.Register("configHandler", mockConfigHandler)

	return MockToolsComponents{
		Injector:      mockInjector,
		ConfigHandler: mockConfigHandler,
	}
}

func TestToolsManager_NewToolsManager(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupToolsMocks()

		// Given a set of safe mocks
		toolsManager := NewToolsManager(mocks.Injector)

		// Then the tools manager should be non-nil
		if toolsManager == nil {
			t.Errorf("Expected tools manager to be non-nil")
		}
	})
}

func TestToolsManager_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupToolsMocks()

		// When a new BaseToolsManager is created
		toolsManager := NewToolsManager(mocks.Injector)

		// And the BaseToolsManager is initialized
		err := toolsManager.Initialize()

		// Then the initialization should succeed
		if err != nil {
			t.Errorf("Expected Initialize to succeed, but got error: %v", err)
		}
	})
}

func TestToolsManager_WriteManifest(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupToolsMocks()

		// Given a new BaseToolsManager is created
		toolsManager := NewToolsManager(mocks.Injector)

		// When the WriteManifest method is called
		err := toolsManager.WriteManifest()

		// Then the WriteManifest method should succeed
		if err != nil {
			t.Errorf("Expected WriteManifest to succeed, but got error: %v", err)
		}
	})
}

func TestToolsManager_InstallTools(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupToolsMocks()

		// Given a new BaseToolsManager is created
		toolsManager := NewToolsManager(mocks.Injector)

		// When the Install method is called
		err := toolsManager.Install()

		// Then the InstallTools method should succeed
		if err != nil {
			t.Errorf("Expected InstallTools to succeed, but got error: %v", err)
		}
	})
}

func TestCheckExistingToolsManager(t *testing.T) {
	t.Run("NoToolsManager", func(t *testing.T) {
		projectRoot := "/path/to/project"
		managerName, err := CheckExistingToolsManager(projectRoot)
		if err != nil {
			t.Errorf("Expected CheckExistingToolsManager to succeed, but got error: %v", err)
		}
		if managerName != "" {
			t.Errorf("Expected manager name to be empty, but got: %v", managerName)
		}
	})

	t.Run("Detects Aqua", func(t *testing.T) {
		projectRoot := "/path/to/project/with/aqua"
		// Mock osStat to simulate the presence of "aqua.yaml"
		osStat = func(name string) (os.FileInfo, error) {
			if name == filepath.Join(projectRoot, "aqua.yaml") {
				return nil, nil // Simulate file exists
			}
			return nil, os.ErrNotExist
		}

		managerName, err := CheckExistingToolsManager(projectRoot)

		if err != nil {
			t.Errorf("Expected CheckExistingToolsManager to succeed, but got error: %v", err)
		}
		if managerName != "aqua" {
			t.Errorf("Expected manager name to be 'aqua', but got: %v", managerName)
		}
	})

	t.Run("Detects Asdf", func(t *testing.T) {
		projectRoot := "/path/to/project/with/asdf"
		// Mock osStat to simulate the presence of ".tool-versions"
		osStat = func(name string) (os.FileInfo, error) {
			if name == filepath.Join(projectRoot, ".tool-versions") {
				return nil, nil // Simulate file exists
			}
			return nil, os.ErrNotExist
		}

		managerName, err := CheckExistingToolsManager(projectRoot)

		if err != nil {
			t.Errorf("Expected CheckExistingToolsManager to succeed, but got error: %v", err)
		}
		if managerName != "asdf" {
			t.Errorf("Expected manager name to be 'asdf', but got: %v", managerName)
		}
	})
}
