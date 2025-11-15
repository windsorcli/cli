package terraform

import (
	"errors"
	"testing"
)

// The MockModuleResolverTest is a test suite for the MockModuleResolver implementation
// It provides coverage for constructor, function field overrides, and interface compliance
// The MockModuleResolverTest ensures correct mock behavior for dependency injection and test isolation
// enabling reliable testing of consumers of the ModuleResolver interface

// =============================================================================
// Test Setup
// =============================================================================

type MockModuleResolverSetupOptions struct {
	ProcessModulesFunc func() error
}

func setupMockModuleResolver(t *testing.T, opts ...*MockModuleResolverSetupOptions) *MockModuleResolver {
	t.Helper()

	mock := NewMockModuleResolver()
	if len(opts) > 0 && opts[0] != nil {
		if opts[0].ProcessModulesFunc != nil {
			mock.ProcessModulesFunc = opts[0].ProcessModulesFunc
		}
	}
	return mock
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestMockModuleResolver_NewMockModuleResolver(t *testing.T) {
	t.Run("CreatesMockModuleResolver", func(t *testing.T) {
		// When creating a new mock module resolver
		mock := NewMockModuleResolver()

		// Then the mock should not be nil
		if mock == nil {
			t.Fatal("Expected non-nil mock module resolver")
		}
	})
}

func TestMockModuleResolver_ProcessModules(t *testing.T) {
	setup := func(t *testing.T, fn func() error) *MockModuleResolver {
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
