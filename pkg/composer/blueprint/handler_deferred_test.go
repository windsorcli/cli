package blueprint

import (
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
)

// =============================================================================
// Test Private Methods
// =============================================================================

func TestHandler_deriveConfigMapDeferredPaths(t *testing.T) {
	t.Run("MarksConfigMapEntriesContainingDeferredPlaceholder", func(t *testing.T) {
		// Given a handler with a composed blueprint containing deferred ConfigMap values
		handler := &BaseBlueprintHandler{
			deferredPaths: map[string]bool{
				"substitutions.MY_KEY": true,
			},
			composedBlueprint: &blueprintv1alpha1.Blueprint{
				ConfigMaps: map[string]map[string]string{
					"values-common": {
						"MY_KEY":       "<deferred>",
						"RESOLVED_KEY": "resolved-value",
					},
					"values-my-app": {
						"APP_KEY": "<deferred>",
					},
				},
			},
		}

		// When deriving ConfigMap deferred paths
		handler.deriveConfigMapDeferredPaths()

		// Then deferred entries are marked with the configmaps prefix
		paths := handler.GetDeferredPaths()
		if !paths["configmaps.values-common.MY_KEY"] {
			t.Error("Expected configmaps.values-common.MY_KEY to be marked deferred")
		}
		if paths["configmaps.values-common.RESOLVED_KEY"] {
			t.Error("Expected configmaps.values-common.RESOLVED_KEY not to be marked deferred")
		}
		if !paths["configmaps.values-my-app.APP_KEY"] {
			t.Error("Expected configmaps.values-my-app.APP_KEY to be marked deferred")
		}
	})

	t.Run("NoOpWhenNilBlueprint", func(t *testing.T) {
		// Given a handler with no composed blueprint
		handler := &BaseBlueprintHandler{}

		// When deriving ConfigMap deferred paths
		handler.deriveConfigMapDeferredPaths()

		// Then no paths are added
		paths := handler.GetDeferredPaths()
		if len(paths) != 0 {
			t.Errorf("Expected empty deferred paths, got %v", paths)
		}
	})

	t.Run("NoOpWhenNoConfigMaps", func(t *testing.T) {
		// Given a handler with a composed blueprint but no ConfigMaps
		handler := &BaseBlueprintHandler{
			composedBlueprint: &blueprintv1alpha1.Blueprint{},
		}

		// When deriving ConfigMap deferred paths
		handler.deriveConfigMapDeferredPaths()

		// Then no paths are added
		paths := handler.GetDeferredPaths()
		if len(paths) != 0 {
			t.Errorf("Expected empty deferred paths, got %v", paths)
		}
	})
}
