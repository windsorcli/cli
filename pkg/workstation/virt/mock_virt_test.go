// The mock_virt_test package is a test suite for the MockVirt implementation
// It provides comprehensive test coverage for all MockVirt interface methods
// It serves as a verification framework for the mock virtualization layer
// It enables testing of components that depend on virtualization without real VMs

package virt

import (
	"testing"

	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/workstation/services"
	"github.com/windsorcli/cli/pkg/shell"
)

// =============================================================================
// Test Setup
// =============================================================================

type MockComponents struct {
	Injector          di.Injector
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

// TestMockVirt_Initialize tests the Initialize method of MockVirt.
func TestMockVirt_Initialize(t *testing.T) {
	t.Run("InitializeFuncImplemented", func(t *testing.T) {
		// Given a MockVirt with a custom InitializeFunc
		mockVirt := NewMockVirt()
		mockVirt.InitializeFunc = func() error {
			return nil
		}

		// When calling Initialize
		err := mockVirt.Initialize()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
	})

	t.Run("InitializeFuncNotImplemented", func(t *testing.T) {
		// Given a MockVirt without a custom InitializeFunc
		mockVirt := NewMockVirt()

		// When calling Initialize
		err := mockVirt.Initialize()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
	})
}

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

// TestMockVirt_PrintInfo tests the PrintInfo method of MockVirt.
func TestMockVirt_PrintInfo(t *testing.T) {
	t.Run("PrintInfoFuncImplemented", func(t *testing.T) {
		// Given a MockVirt with a custom PrintInfoFunc
		mockVirt := NewMockVirt()
		mockVirt.PrintInfoFunc = func() error {
			return nil
		}

		// When calling PrintInfo
		err := mockVirt.PrintInfo()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
	})

	t.Run("PrintInfoFuncNotImplemented", func(t *testing.T) {
		// Given a MockVirt without a custom PrintInfoFunc
		mockVirt := NewMockVirt()

		// When calling PrintInfo
		err := mockVirt.PrintInfo()

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

// TestMockVirt_GetVMInfo tests the GetVMInfo method of MockVirt.
func TestMockVirt_GetVMInfo(t *testing.T) {
	t.Run("GetVMInfoFuncImplemented", func(t *testing.T) {
		// Given a MockVirt with a custom GetVMInfoFunc
		mockVirt := NewMockVirt()
		mockVirt.GetVMInfoFunc = func() (VMInfo, error) {
			return VMInfo{Address: "192.168.1.1"}, nil
		}

		// When calling GetVMInfo
		info, err := mockVirt.GetVMInfo()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		// And the info should be as expected
		if info.Address != "192.168.1.1" {
			t.Errorf("Expected address '192.168.1.1', got %v", info.Address)
		}
	})

	t.Run("GetVMInfoFuncNotImplemented", func(t *testing.T) {
		// Given a MockVirt without a custom GetVMInfoFunc
		mockVirt := NewMockVirt()

		// When calling GetVMInfo
		info, err := mockVirt.GetVMInfo()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		// And the info should be empty
		if info != (VMInfo{}) {
			t.Errorf("Expected empty VMInfo, got %v", info)
		}
	})
}

// TestMockVirt_GetContainerInfo tests the GetContainerInfo method of MockVirt.
func TestMockVirt_GetContainerInfo(t *testing.T) {
	t.Run("GetContainerInfoFuncImplemented", func(t *testing.T) {
		// Given a MockVirt with a custom GetContainerInfoFunc
		mockVirt := NewMockVirt()
		mockVirt.GetContainerInfoFunc = func(name ...string) ([]ContainerInfo, error) {
			return []ContainerInfo{{Name: "container1"}}, nil
		}

		// When calling GetContainerInfo
		info, err := mockVirt.GetContainerInfo()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		// And the info should be as expected
		if len(info) != 1 || info[0].Name != "container1" {
			t.Errorf("Expected container info with name 'container1', got %v", info)
		}
	})

	t.Run("GetContainerInfoFuncNotImplemented", func(t *testing.T) {
		// Given a MockVirt without a custom GetContainerInfoFunc
		mockVirt := NewMockVirt()

		// When calling GetContainerInfo
		info, err := mockVirt.GetContainerInfo()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		// And the info should be an empty list
		if len(info) != 0 {
			t.Errorf("Expected info to be an empty list, got %v", info)
		}
	})
}
