package generators

import (
	"io/fs"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/blueprint"
	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	sh "github.com/windsorcli/cli/pkg/shell"
)

// =============================================================================
// Test Setup
// =============================================================================

type MockComponents struct {
	Injector             di.Injector
	MockConfigHandler    *config.MockConfigHandler
	MockBlueprintHandler *blueprint.MockBlueprintHandler
	MockShell            *sh.MockShell
}

// setupSafeMocks function creates safe mocks for the generator
func setupSafeMocks(injector ...di.Injector) MockComponents {
	// Mock the dependencies for the generator
	var mockInjector di.Injector
	if len(injector) > 0 {
		mockInjector = injector[0]
	} else {
		mockInjector = di.NewInjector()
	}

	// Mock the osWriteFile function
	osWriteFile = func(_ string, _ []byte, _ fs.FileMode) error {
		return nil
	}

	// Mock the osMkdirAll function
	osMkdirAll = func(_ string, _ fs.FileMode) error {
		return nil
	}

	// Create a new mock context handler
	mockConfigHandler := config.NewMockConfigHandler()
	mockInjector.Register("configHandler", mockConfigHandler)

	// Mock the context handler methods
	mockConfigHandler.GetConfigRootFunc = func() (string, error) {
		return "/mock/config/root", nil
	}

	// Create a new mock blueprint handler
	mockBlueprintHandler := blueprint.NewMockBlueprintHandler(mockInjector)
	mockInjector.Register("blueprintHandler", mockBlueprintHandler)

	// Mock the GetTerraformComponents method
	mockBlueprintHandler.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
		// Common components setup
		remoteComponent := blueprintv1alpha1.TerraformComponent{
			Source: "git::https://github.com/terraform-aws-modules/terraform-aws-vpc.git//terraform/remote/path@v1.0.0",
			Path:   "/mock/project/root/.windsor/.tf_modules/remote/path",
			Values: map[string]any{
				"remote_variable1": "default_value",
			},
		}

		localComponent := blueprintv1alpha1.TerraformComponent{
			Source: "local/path",
			Path:   "/mock/project/root/terraform/local/path",
			Values: map[string]any{
				"local_variable1": "default_value",
			},
		}

		return []blueprintv1alpha1.TerraformComponent{remoteComponent, localComponent}
	}

	// Create a new mock shell
	mockShell := sh.NewMockShell()
	mockShell.GetProjectRootFunc = func() (string, error) {
		return "/mock/project/root", nil
	}
	mockInjector.Register("shell", mockShell)

	return MockComponents{
		Injector:             mockInjector,
		MockConfigHandler:    mockConfigHandler,
		MockBlueprintHandler: mockBlueprintHandler,
		MockShell:            mockShell,
	}
}

// =============================================================================
// Test Constructor
// =============================================================================

func TestGenerator_NewGenerator(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a set of safe mocks
		mocks := setupSafeMocks()

		// When a new generator is created
		generator := NewGenerator(mocks.Injector)

		// Then the generator should be non-nil
		if generator == nil {
			t.Errorf("Expected generator to be non-nil")
		}
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestGenerator_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a set of safe mocks
		mocks := setupSafeMocks()

		// When a new BaseGenerator is created
		generator := NewGenerator(mocks.Injector)

		// And the BaseGenerator is initialized
		err := generator.Initialize()

		// Then the initialization should succeed
		if err != nil {
			t.Errorf("Expected Initialize to succeed, but got error: %v", err)
		}
	})

	t.Run("ErrorResolvingBlueprintHandler", func(t *testing.T) {
		// Given a set of safe mocks
		mocks := setupSafeMocks()

		// And a mock injector with a nil blueprint handler
		mocks.Injector.Register("blueprintHandler", nil)

		// When a new BaseGenerator is created
		generator := NewGenerator(mocks.Injector)

		// And the BaseGenerator is initialized
		err := generator.Initialize()

		// Then the initialization should fail
		if err == nil {
			t.Errorf("Expected Initialize to fail, but it succeeded")
		}
	})

	t.Run("ErrorResolvingShell", func(t *testing.T) {
		// Given a set of safe mocks
		mocks := setupSafeMocks()

		// And a mock injector with a nil shell
		mocks.Injector.Register("shell", nil)

		// When a new BaseGenerator is created
		generator := NewGenerator(mocks.Injector)

		// And the BaseGenerator is initialized
		err := generator.Initialize()

		// Then the initialization should fail
		if err == nil {
			t.Errorf("Expected Initialize to fail, but it succeeded")
		}
	})
}

func TestGenerator_Write(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a set of safe mocks
		mocks := setupSafeMocks()

		// And a new BaseGenerator is created
		generator := NewGenerator(mocks.Injector)

		// When the Write method is called
		err := generator.Write()

		// Then the Write method should succeed
		if err != nil {
			t.Errorf("Expected Write to succeed, but got error: %v", err)
		}
	})
}
