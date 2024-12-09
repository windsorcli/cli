package generators

import (
	"testing"

	"github.com/windsorcli/cli/internal/blueprint"
	"github.com/windsorcli/cli/internal/context"
	"github.com/windsorcli/cli/internal/di"
)

type MockComponents struct {
	Injector             di.Injector
	MockContextHandler   context.ContextHandler
	MockBlueprintHandler blueprint.BlueprintHandler
}

// setupSafeMocks function creates safe mocks for the generator
func setupSafeMocks(injector ...di.Injector) MockComponents {
	// Mock the dependencies for the generator
	var mockInjector di.Injector
	if len(injector) > 0 {
		mockInjector = injector[0]
	} else {
		mockInjector = di.NewMockInjector()
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

	return MockComponents{
		Injector:             mockInjector,
		MockContextHandler:   mockContextHandler,
		MockBlueprintHandler: mockBlueprintHandler,
	}
}

func TestGenerator_NewGenerator(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupSafeMocks()

		generator := NewGenerator(mocks.Injector)
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

		mocks.Injector.Register("contextHandler", nil)

		generator := NewGenerator(mocks.Injector)
		err := generator.Initialize()
		if err == nil {
			t.Errorf("Expected Initialize to fail, but it succeeded")
		}
	})

	t.Run("ErrorResolvingBlueprintHandler", func(t *testing.T) {
		mocks := setupSafeMocks()

		mocks.Injector.Register("blueprintHandler", nil)

		generator := NewGenerator(mocks.Injector)
		err := generator.Initialize()
		if err == nil {
			t.Errorf("Expected Initialize to fail, but it succeeded")
		}
	})
}

func TestGenerator_Write(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupSafeMocks()

		generator := NewGenerator(mocks.Injector)
		err := generator.Write()
		if err != nil {
			t.Errorf("Expected Write to succeed, but got error: %v", err)
		}
	})
}
