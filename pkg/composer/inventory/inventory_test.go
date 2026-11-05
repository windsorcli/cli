package inventory_test

import (
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/composer/inventory"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/config"
)

// =============================================================================
// Test Setup
// =============================================================================

type mocks struct {
	runtime       *runtime.Runtime
	configHandler *config.MockConfigHandler
}

func setupMocks(t *testing.T) *mocks {
	t.Helper()
	m := &mocks{}

	m.configHandler = config.NewMockConfigHandler()
	m.configHandler.GetFunc = func(key string) any { return nil }

	m.runtime = &runtime.Runtime{
		ContextName:   "local",
		ProjectRoot:   "/project",
		ConfigHandler: m.configHandler,
	}
	return m
}

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewLister(t *testing.T) {
	t.Run("PanicsWithoutRuntime", func(t *testing.T) {
		// Given no runtime
		defer func() {
			// Then NewLister panics
			if recover() == nil {
				t.Fatal("expected a panic")
			}
		}()

		// When NewLister is called
		inventory.NewLister(nil)
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestBaseLister_List(t *testing.T) {
	t.Run("ListsEachOCIBlueprintSource", func(t *testing.T) {
		// Given a blueprint with two oci:// sources, a duplicate and a Git source
		m := setupMocks(t)
		lister := inventory.NewLister(m.runtime)
		blueprint := &blueprintv1alpha1.Blueprint{
			Sources: []blueprintv1alpha1.Source{
				{Name: "extra", Url: "oci://ghcr.io/windsorcli/extra:v1.0.0@sha256:abc"},
				{Name: "core", Url: "oci://ghcr.io/windsorcli/core:v0.9.0"},
				{Name: "core-again", Url: "oci://ghcr.io/windsorcli/core:v0.9.0"},
				{Name: "git", Url: "https://github.com/windsorcli/core"},
			},
		}

		// When List is called
		result, err := lister.List(blueprint, inventory.Options{ChartsTo: "registry.example.com/windsor"})

		// Then the inventory holds one sorted entry for each oci:// source, with no destination
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if result.Inventory.Version != inventory.InventoryVersion || result.Inventory.Context != "local" {
			t.Errorf("unexpected inventory header %+v", result.Inventory)
		}
		got := result.Inventory.Artifacts
		if len(got) != 2 {
			t.Fatalf("expected 2 entries, got %+v", got)
		}
		want := []inventory.Entry{
			{Kind: inventory.KindBlueprint, Transport: inventory.TransportOCI, Reference: "ghcr.io/windsorcli/core", Version: "v0.9.0", Source: inventory.Source{Object: "source/core"}},
			{Kind: inventory.KindBlueprint, Transport: inventory.TransportOCI, Reference: "ghcr.io/windsorcli/extra", Version: "v1.0.0", Digest: "sha256:abc", Source: inventory.Source{Object: "source/extra"}},
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("entry %d: expected %+v, got %+v", i, want[i], got[i])
			}
		}
	})

	t.Run("ReturnsErrorForNilBlueprint", func(t *testing.T) {
		// Given a lister
		m := setupMocks(t)
		lister := inventory.NewLister(m.runtime)

		// When List is called with no blueprint
		_, err := lister.List(nil, inventory.Options{})

		// Then an error is returned
		if err == nil {
			t.Fatal("expected an error")
		}
	})
}
