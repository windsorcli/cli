package terraform

import (
	"fmt"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
)

// =============================================================================
// Test Methods
// =============================================================================

func TestMockTerraformProvider_FindRelativeProjectPath(t *testing.T) {
	t.Run("WithFuncSet", func(t *testing.T) {
		// Given a mock provider with FindRelativeProjectPathFunc set
		mock := &MockTerraformProvider{}
		expectedPath := "test/path"
		mock.FindRelativeProjectPathFunc = func(directory ...string) (string, error) {
			return expectedPath, nil
		}

		// When FindRelativeProjectPath is called
		path, err := mock.FindRelativeProjectPath()

		// Then the expected path should be returned
		if err != nil {
			t.Errorf("Expected no error, got = %v", err)
		}
		if path != expectedPath {
			t.Errorf("Expected path = %s, got = %s", expectedPath, path)
		}
	})

	t.Run("WithNoFuncSet", func(t *testing.T) {
		// Given a mock provider with no FindRelativeProjectPathFunc set
		mock := &MockTerraformProvider{}

		// When FindRelativeProjectPath is called
		path, err := mock.FindRelativeProjectPath()

		// Then empty path and no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got = %v", err)
		}
		if path != "" {
			t.Errorf("Expected empty path, got = %s", path)
		}
	})
}

func TestMockTerraformProvider_GenerateBackendOverride(t *testing.T) {
	t.Run("WithFuncSet", func(t *testing.T) {
		// Given a mock provider with GenerateBackendOverrideFunc set
		mock := &MockTerraformProvider{}
		expectedErr := fmt.Errorf("mock error")
		mock.GenerateBackendOverrideFunc = func(directory string) error {
			return expectedErr
		}

		// When GenerateBackendOverride is called
		err := mock.GenerateBackendOverride("test/dir")

		// Then the expected error should be returned
		if err != expectedErr {
			t.Errorf("Expected error = %v, got = %v", expectedErr, err)
		}
	})

	t.Run("WithNoFuncSet", func(t *testing.T) {
		// Given a mock provider with no GenerateBackendOverrideFunc set
		mock := &MockTerraformProvider{}

		// When GenerateBackendOverride is called
		err := mock.GenerateBackendOverride("test/dir")

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got = %v", err)
		}
	})
}

func TestMockTerraformProvider_GetTerraformComponent(t *testing.T) {
	t.Run("WithFuncSet", func(t *testing.T) {
		// Given a mock provider with GetTerraformComponentFunc set
		mock := &MockTerraformProvider{}
		expectedComponent := &blueprintv1alpha1.TerraformComponent{
			Path: "test/path",
		}
		mock.GetTerraformComponentFunc = func(componentID string) *blueprintv1alpha1.TerraformComponent {
			if componentID == "test/path" {
				return expectedComponent
			}
			return nil
		}

		// When GetTerraformComponent is called
		component := mock.GetTerraformComponent("test/path")

		// Then the expected component should be returned
		if component != expectedComponent {
			t.Errorf("Expected component = %v, got = %v", expectedComponent, component)
		}
	})

	t.Run("WithNoFuncSet", func(t *testing.T) {
		// Given a mock provider with no GetTerraformComponentFunc set
		mock := &MockTerraformProvider{}

		// When GetTerraformComponent is called
		component := mock.GetTerraformComponent("test/path")

		// Then nil should be returned
		if component != nil {
			t.Errorf("Expected nil, got = %v", component)
		}
	})
}

func TestMockTerraformProvider_GetTerraformComponents(t *testing.T) {
	t.Run("WithFuncSet", func(t *testing.T) {
		// Given a mock provider with GetTerraformComponentsFunc set
		mock := &MockTerraformProvider{}
		expectedComponents := []blueprintv1alpha1.TerraformComponent{
			{Path: "test/path1"},
			{Path: "test/path2"},
		}
		mock.GetTerraformComponentsFunc = func() []blueprintv1alpha1.TerraformComponent {
			return expectedComponents
		}

		// When GetTerraformComponents is called
		components := mock.GetTerraformComponents()

		// Then the expected components should be returned
		if len(components) != len(expectedComponents) {
			t.Errorf("Expected %d components, got = %d", len(expectedComponents), len(components))
		}
	})

	t.Run("WithNoFuncSet", func(t *testing.T) {
		// Given a mock provider with no GetTerraformComponentsFunc set
		mock := &MockTerraformProvider{}

		// When GetTerraformComponents is called
		components := mock.GetTerraformComponents()

		// Then empty slice should be returned
		if len(components) != 0 {
			t.Errorf("Expected empty slice, got = %d components", len(components))
		}
	})
}

func TestMockTerraformProvider_ClearCache(t *testing.T) {
	t.Run("WithFuncSet", func(t *testing.T) {
		// Given a mock provider with ClearCacheFunc set
		mock := &MockTerraformProvider{}
		cleared := false
		mock.ClearCacheFunc = func() {
			cleared = true
		}

		// When ClearCache is called
		mock.ClearCache()

		// Then the function should be called
		if !cleared {
			t.Error("Expected ClearCacheFunc to be called")
		}
	})

	t.Run("WithNoFuncSet", func(t *testing.T) {
		// Given a mock provider with no ClearCacheFunc set
		mock := &MockTerraformProvider{}

		// When ClearCache is called
		// Then no panic should occur
		mock.ClearCache()
	})
}


