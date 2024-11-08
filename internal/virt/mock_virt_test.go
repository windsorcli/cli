package virt

import (
	"testing"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/helpers"
	"github.com/windsor-hotel/cli/internal/shell"
)

type MockComponents struct {
	Container         di.ContainerInterface
	MockContext       *context.MockContext
	MockShell         *shell.MockShell
	MockConfigHandler *config.MockConfigHandler
	MockHelper        *helpers.MockHelper
}

type mockYAMLEncoder struct {
	encodeFunc func(v interface{}) error
	closeFunc  func() error
}

func (m *mockYAMLEncoder) Encode(v interface{}) error {
	return m.encodeFunc(v)
}

func (m *mockYAMLEncoder) Close() error {
	return m.closeFunc()
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

		// Then an error should be returned
		if err == nil || err.Error() != "UpFunc not implemented" {
			t.Fatalf("Expected 'UpFunc not implemented' error, got %v", err)
		}
	})
}

// TestMockVirt_Down tests the Down method of MockVirt.
func TestMockVirt_Down(t *testing.T) {
	t.Run("DownFuncImplemented", func(t *testing.T) {
		// Given a MockVirt with a custom DownFunc
		mockVirt := NewMockVirt()
		mockVirt.DownFunc = func(verbose ...bool) error {
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

		// Then an error should be returned
		if err == nil || err.Error() != "DownFunc not implemented" {
			t.Fatalf("Expected 'DownFunc not implemented' error, got %v", err)
		}
	})
}

// TestMockVirt_Delete tests the Delete method of MockVirt.
func TestMockVirt_Delete(t *testing.T) {
	t.Run("DeleteFuncImplemented", func(t *testing.T) {
		// Given a MockVirt with a custom DeleteFunc
		mockVirt := NewMockVirt()
		mockVirt.DeleteFunc = func(verbose ...bool) error {
			return nil
		}

		// When calling Delete
		err := mockVirt.Delete()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
	})

	t.Run("DeleteFuncNotImplemented", func(t *testing.T) {
		// Given a MockVirt without a custom DeleteFunc
		mockVirt := NewMockVirt()

		// When calling Delete
		err := mockVirt.Delete()

		// Then an error should be returned
		if err == nil || err.Error() != "DeleteFunc not implemented" {
			t.Fatalf("Expected 'DeleteFunc not implemented' error, got %v", err)
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

		// Then an error should be returned
		if err == nil || err.Error() != "PrintInfoFunc not implemented" {
			t.Fatalf("Expected 'PrintInfoFunc not implemented' error, got %v", err)
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

		// Then an error should be returned
		if err == nil || err.Error() != "WriteConfigFunc not implemented" {
			t.Fatalf("Expected 'WriteConfigFunc not implemented' error, got %v", err)
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

		// Then an error should be returned
		if err == nil || err.Error() != "GetVMInfoFunc not implemented" {
			t.Fatalf("Expected 'GetVMInfoFunc not implemented' error, got %v", err)
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
		mockVirt.GetContainerInfoFunc = func() ([]ContainerInfo, error) {
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

		// Then an error should be returned
		if err == nil || err.Error() != "GetContainerInfoFunc not implemented" {
			t.Fatalf("Expected 'GetContainerInfoFunc not implemented' error, got %v", err)
		}
		// And the info should be nil
		if info != nil {
			t.Errorf("Expected info to be nil, got %v", info)
		}
	})
}
