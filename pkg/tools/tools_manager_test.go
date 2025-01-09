package tools

import (
	"testing"

	"github.com/windsorcli/cli/pkg/di"
)

type MockToolsComponents struct {
	Injector di.Injector
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

	return MockToolsComponents{
		Injector: mockInjector,
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
