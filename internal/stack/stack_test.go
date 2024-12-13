package stack

import (
	"testing"

	"github.com/windsorcli/cli/internal/blueprint"
	"github.com/windsorcli/cli/internal/di"
)

type MockSafeComponents struct {
	Injector di.Injector
}

// setupSafeMocks function creates safe mocks for the stack
func setupSafeMocks(injector ...di.Injector) MockSafeComponents {
	var mockInjector di.Injector
	if len(injector) > 0 {
		mockInjector = injector[0]
	} else {
		mockInjector = di.NewMockInjector()
	}

	// Create a mock blueprint handler
	mockBlueprintHandler := blueprint.NewMockBlueprintHandler(mockInjector)
	mockInjector.Register("blueprintHandler", mockBlueprintHandler)

	return MockSafeComponents{Injector: mockInjector}
}

func TestStack_NewStack(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		injector := di.NewInjector()
		stack := NewBaseStack(injector)
		if stack == nil {
			t.Errorf("Expected stack to be non-nil")
		}
	})
}

func TestStack_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mockComponents := setupSafeMocks()
		stack := NewBaseStack(mockComponents.Injector)
		if err := stack.Initialize(); err != nil {
			t.Errorf("Expected Initialize to return nil, got %v", err)
		}
	})

	t.Run("ErrorResolvingBlueprintHandler", func(t *testing.T) {
		mockComponents := setupSafeMocks()
		// Unregister the blueprintHandler to simulate the error
		mockComponents.Injector.Register("blueprintHandler", nil)
		stack := NewBaseStack(mockComponents.Injector)
		if err := stack.Initialize(); err == nil {
			t.Errorf("Expected Initialize to return an error")
		}
	})
}

func TestStack_Up(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mockComponents := setupSafeMocks()
		stack := NewBaseStack(mockComponents.Injector)
		if err := stack.Up(); err != nil {
			t.Errorf("Expected Up to return nil, got %v", err)
		}
	})
}
