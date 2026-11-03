package terraform_test

import (
	"errors"
	"testing"

	"github.com/windsorcli/cli/pkg/composer/terraform"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestMockModuleResolver_NewMockModuleResolver(t *testing.T) {
	t.Run("CreatesMockModuleResolver", func(t *testing.T) {
		// When creating a new mock module resolver
		mock := terraform.NewMockModuleResolver()

		// Then the mock should not be nil
		if mock == nil {
			t.Fatal("Expected non-nil mock module resolver")
		}
	})
}

func TestMockModuleResolver_ProcessModules(t *testing.T) {
	setup := func(t *testing.T, fn func() error) *terraform.MockModuleResolver {
		t.Helper()
		return setupMockModuleResolver(t, &MockModuleResolverSetupOptions{ProcessModulesFunc: fn})
	}

	t.Run("DefaultBehavior", func(t *testing.T) {
		// Given a mock module resolver with no ProcessModulesFunc
		mock := setup(t, nil)

		// When calling ProcessModules
		err := mock.ProcessModules()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}
	})

	t.Run("CustomProcessModulesFunc", func(t *testing.T) {
		// Given a mock module resolver with a custom ProcessModulesFunc
		expectedErr := errors.New("process error")
		mock := setup(t, func() error { return expectedErr })

		// When calling ProcessModules
		err := mock.ProcessModules()

		// Then the custom error should be returned
		if err != expectedErr {
			t.Errorf("Expected process error, got %v", err)
		}
	})
}
