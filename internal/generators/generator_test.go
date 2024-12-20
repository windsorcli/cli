package generators

import (
	"io/fs"
	"testing"

	"github.com/windsorcli/cli/internal/blueprint"
	"github.com/windsorcli/cli/internal/context"
	"github.com/windsorcli/cli/internal/di"
	sh "github.com/windsorcli/cli/internal/shell"
)

type MockComponents struct {
	Injector             di.Injector
	MockContextHandler   *context.MockContext
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
	mockContextHandler := context.NewMockContext()
	mockInjector.Register("contextHandler", mockContextHandler)

	// Mock the context handler methods
	mockContextHandler.GetConfigRootFunc = func() (string, error) {
		return "/mock/config/root", nil
	}

	// Create a new mock blueprint handler
	mockBlueprintHandler := blueprint.NewMockBlueprintHandler(mockInjector)
	mockInjector.Register("blueprintHandler", mockBlueprintHandler)

	// Mock the GetTerraformComponents method
	mockBlueprintHandler.GetTerraformComponentsFunc = func() []blueprint.TerraformComponentV1Alpha1 {
		// Common components setup
		remoteComponent := blueprint.TerraformComponentV1Alpha1{
			Source: "git::https://github.com/terraform-aws-modules/terraform-aws-vpc.git//terraform/remote/path@v1.0.0",
			Path:   "/mock/project/root/.tf_modules/remote/path",
			Values: map[string]interface{}{
				"remote_variable1": "default_value",
			},
		}

		localComponent := blueprint.TerraformComponentV1Alpha1{
			Source: "local/path",
			Path:   "/mock/project/root/terraform/local/path",
			Values: map[string]interface{}{
				"local_variable1": "default_value",
			},
		}

		return []blueprint.TerraformComponentV1Alpha1{remoteComponent, localComponent}
	}

	// Create a new mock shell
	mockShell := sh.NewMockShell()
	mockShell.GetProjectRootFunc = func() (string, error) {
		return "/mock/project/root", nil
	}
	mockInjector.Register("shell", mockShell)

	return MockComponents{
		Injector:             mockInjector,
		MockContextHandler:   mockContextHandler,
		MockBlueprintHandler: mockBlueprintHandler,
		MockShell:            mockShell,
	}
}

func TestGenerator_NewGenerator(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupSafeMocks()

		// Given a set of safe mocks
		generator := NewGenerator(mocks.Injector)

		// Then the generator should be non-nil
		if generator == nil {
			t.Errorf("Expected generator to be non-nil")
		}
	})
}

func TestGenerator_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
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

	t.Run("ErrorResolvingContextHandler", func(t *testing.T) {
		mocks := setupSafeMocks()

		// Given a mock injector with a nil context handler
		mocks.Injector.Register("contextHandler", nil)

		// When a new BaseGenerator is created
		generator := NewGenerator(mocks.Injector)

		// And the BaseGenerator is initialized
		err := generator.Initialize()

		// Then the initialization should fail
		if err == nil {
			t.Errorf("Expected Initialize to fail, but it succeeded")
		}
	})

	t.Run("ErrorResolvingBlueprintHandler", func(t *testing.T) {
		mocks := setupSafeMocks()

		// Given a mock injector with a nil blueprint handler
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
}

func TestGenerator_Write(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupSafeMocks()

		// Given a new BaseGenerator is created
		generator := NewGenerator(mocks.Injector)

		// When the Write method is called
		err := generator.Write()

		// Then the Write method should succeed
		if err != nil {
			t.Errorf("Expected Write to succeed, but got error: %v", err)
		}
	})
}
