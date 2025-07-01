package bundler

import (
	"testing"

	"github.com/windsorcli/cli/pkg/di"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestMockBundler_NewMockBundler(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given no preconditions
		// When creating a new mock bundler
		mock := NewMockBundler()

		// Then it should not be nil
		if mock == nil {
			t.Fatal("Expected non-nil mock bundler")
		}
	})
}

func TestMockBundler_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock with a custom initialize function
		mock := NewMockBundler()
		called := false
		mock.InitializeFunc = func(injector di.Injector) error {
			called = true
			return nil
		}

		// When calling Initialize
		err := mock.Initialize(di.NewInjector())

		// Then the mock function should be called
		if !called {
			t.Error("Expected InitializeFunc to be called")
		}
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock with no custom initialize function
		mock := NewMockBundler()

		// When calling Initialize
		err := mock.Initialize(di.NewInjector())

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}
	})
}

func TestMockBundler_Bundle(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock with a custom bundle function
		mock := NewMockBundler()
		called := false
		mock.BundleFunc = func(artifact Artifact) error {
			called = true
			return nil
		}

		// When calling Bundle
		artifact := NewMockArtifact()
		err := mock.Bundle(artifact)

		// Then the mock function should be called
		if !called {
			t.Error("Expected BundleFunc to be called")
		}
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock with no custom bundle function
		mock := NewMockBundler()

		// When calling Bundle
		artifact := NewMockArtifact()
		err := mock.Bundle(artifact)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}
	})
}
