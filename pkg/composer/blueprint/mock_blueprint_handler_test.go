package blueprint_test

import (
	"errors"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/composer/blueprint"
)

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewMockBlueprintHandler(t *testing.T) {
	t.Run("CreatesMockHandler", func(t *testing.T) {
		// When creating a new mock handler
		mock := blueprint.NewMockBlueprintHandler()

		// Then mock should be created
		if mock == nil {
			t.Fatal("Expected mock to be created")
		}
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestMockBlueprintHandler_LoadBlueprint(t *testing.T) {
	t.Run("ReturnsNilWhenFuncNotSet", func(t *testing.T) {
		// Given a mock without LoadBlueprintFunc set
		mock := blueprint.NewMockBlueprintHandler()

		// When calling LoadBlueprint
		err := mock.LoadBlueprint()

		// Then should return nil
		if err != nil {
			t.Errorf("Expected nil, got %v", err)
		}
	})

	t.Run("CallsFuncWhenSet", func(t *testing.T) {
		// Given a mock with LoadBlueprintFunc set
		mock := blueprint.NewMockBlueprintHandler()
		called := false
		mock.LoadBlueprintFunc = func(urls ...string) error {
			called = true
			return nil
		}

		// When calling LoadBlueprint
		_ = mock.LoadBlueprint("test-url")

		// Then func should be called
		if !called {
			t.Error("Expected LoadBlueprintFunc to be called")
		}
	})

	t.Run("ReturnsErrorFromFunc", func(t *testing.T) {
		// Given a mock with LoadBlueprintFunc that returns error
		mock := blueprint.NewMockBlueprintHandler()
		mock.LoadBlueprintFunc = func(urls ...string) error {
			return errors.New("load failed")
		}

		// When calling LoadBlueprint
		err := mock.LoadBlueprint()

		// Then should return error
		if err == nil {
			t.Error("Expected error")
		}
	})
}

func TestMockBlueprintHandler_Write(t *testing.T) {
	t.Run("ReturnsNilWhenFuncNotSet", func(t *testing.T) {
		// Given a mock without WriteFunc set
		mock := blueprint.NewMockBlueprintHandler()

		// When calling Write
		err := mock.Write()

		// Then should return nil
		if err != nil {
			t.Errorf("Expected nil, got %v", err)
		}
	})

	t.Run("CallsFuncWhenSet", func(t *testing.T) {
		// Given a mock with WriteFunc set
		mock := blueprint.NewMockBlueprintHandler()
		called := false
		mock.WriteFunc = func(overwrite ...bool) error {
			called = true
			return nil
		}

		// When calling Write
		_ = mock.Write(true)

		// Then func should be called
		if !called {
			t.Error("Expected WriteFunc to be called")
		}
	})
}

func TestMockBlueprintHandler_GetTerraformComponents(t *testing.T) {
	t.Run("ReturnsEmptySliceWhenFuncNotSet", func(t *testing.T) {
		// Given a mock without GetTerraformComponentsFunc set
		mock := blueprint.NewMockBlueprintHandler()

		// When calling GetTerraformComponents
		result := mock.GetTerraformComponents()

		// Then should return empty slice
		if result == nil {
			t.Error("Expected non-nil slice")
		}
		if len(result) != 0 {
			t.Errorf("Expected empty slice, got %d elements", len(result))
		}
	})

	t.Run("CallsFuncWhenSet", func(t *testing.T) {
		// Given a mock with GetTerraformComponentsFunc set
		mock := blueprint.NewMockBlueprintHandler()
		mock.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
			return []blueprintv1alpha1.TerraformComponent{{Path: "vpc"}}
		}

		// When calling GetTerraformComponents
		result := mock.GetTerraformComponents()

		// Then should return components from func
		if len(result) != 1 {
			t.Errorf("Expected 1 component, got %d", len(result))
		}
	})
}

func TestMockBlueprintHandler_Generate(t *testing.T) {
	t.Run("ReturnsNilWhenFuncNotSet", func(t *testing.T) {
		// Given a mock without GenerateFunc set
		mock := blueprint.NewMockBlueprintHandler()

		// When calling Generate
		result := mock.Generate()

		// Then should return nil
		if result != nil {
			t.Error("Expected nil")
		}
	})

	t.Run("CallsFuncWhenSet", func(t *testing.T) {
		// Given a mock with GenerateFunc set
		mock := blueprint.NewMockBlueprintHandler()
		mock.GenerateFunc = func() *blueprintv1alpha1.Blueprint {
			return &blueprintv1alpha1.Blueprint{Metadata: blueprintv1alpha1.Metadata{Name: "test"}}
		}

		// When calling Generate
		result := mock.Generate()

		// Then should return blueprint from func
		if result == nil {
			t.Fatal("Expected non-nil blueprint")
		}
		if result.Metadata.Name != "test" {
			t.Errorf("Expected name='test', got '%s'", result.Metadata.Name)
		}
	})
}
