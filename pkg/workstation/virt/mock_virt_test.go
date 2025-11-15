// The mock_virt_test package is a test suite for the MockVirt implementation
// It provides comprehensive test coverage for all MockVirt interface methods
// It serves as a verification framework for the mock virtualization layer
// It enables testing of components that depend on virtualization without real VMs

package virt

import (
	"testing"

	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/shell"
	"github.com/windsorcli/cli/pkg/workstation/services"
)

// =============================================================================
// Test Setup
// =============================================================================

type MockComponents struct {
	MockShell         *shell.MockShell
	MockConfigHandler *config.MockConfigHandler
	MockService       *services.MockService
}

// mockYAMLEncoder is a mock implementation of YAMLEncoder for testing
type mockYAMLEncoder struct {
	encodeFunc func(v any) error
	closeFunc  func() error
}

func (m *mockYAMLEncoder) Encode(v any) error {
	if m.encodeFunc != nil {
		return m.encodeFunc(v)
	}
	return nil
}

func (m *mockYAMLEncoder) Close() error {
	if m.closeFunc != nil {
		return m.closeFunc()
	}
	return nil
}

// =============================================================================
// Test Public Methods
// =============================================================================

// TestMockVirt_Up tests the Up method of MockVirt.
func TestMockVirt_Up(t *testing.T) {
	t.Run("UpFuncImplemented", func(t *testing.T) {
		// Given a MockVirt with a custom UpFunc
		mockVirt := NewMockVirt()
		mockVirt.UpFunc = func(verbose ...bool) error {
			return nil
		}

		// When calling Up
		err := mockVirt.Up()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
	})

	t.Run("UpFuncNotImplemented", func(t *testing.T) {
		// Given a MockVirt without a custom UpFunc
		mockVirt := NewMockVirt()

		// When calling Up
		err := mockVirt.Up()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
	})
}

// TestMockVirt_Down tests the Down method of MockVirt.
func TestMockVirt_Down(t *testing.T) {
	t.Run("DownFuncImplemented", func(t *testing.T) {
		// Given a MockVirt with a custom DownFunc
		mockVirt := NewMockVirt()
		mockVirt.DownFunc = func() error {
			return nil
		}

		// When calling Down
		err := mockVirt.Down()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
	})

	t.Run("DownFuncNotImplemented", func(t *testing.T) {
		// Given a MockVirt without a custom DownFunc
		mockVirt := NewMockVirt()

		// When calling Down
		err := mockVirt.Down()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
	})
}

// TestMockVirt_WriteConfig tests the WriteConfig method of MockVirt.
func TestMockVirt_WriteConfig(t *testing.T) {
	t.Run("WriteConfigFuncImplemented", func(t *testing.T) {
		// Given a MockVirt with a custom WriteConfigFunc
		mockVirt := NewMockVirt()
		mockVirt.WriteConfigFunc = func() error {
			return nil
		}

		// When calling WriteConfig
		err := mockVirt.WriteConfig()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
	})

	t.Run("WriteConfigFuncNotImplemented", func(t *testing.T) {
		// Given a MockVirt without a custom WriteConfigFunc
		mockVirt := NewMockVirt()

		// When calling WriteConfig
		err := mockVirt.WriteConfig()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
	})
}
