package vm

import (
	"testing"
)

// TestMockVM_Up tests the Up method of MockVM.
func TestMockVM_Up(t *testing.T) {
	t.Run("UpFuncImplemented", func(t *testing.T) {
		// Given a MockVM with a custom UpFunc
		mockVM := NewMockVM()
		mockVM.UpFunc = func(verbose ...bool) error {
			return nil
		}

		// When calling Up
		err := mockVM.Up()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
	})

	t.Run("UpFuncNotImplemented", func(t *testing.T) {
		// Given a MockVM without a custom UpFunc
		mockVM := NewMockVM()

		// When calling Up
		err := mockVM.Up()

		// Then an error should be returned
		if err == nil || err.Error() != "UpFunc not implemented" {
			t.Fatalf("Expected 'UpFunc not implemented' error, got %v", err)
		}
	})
}

// TestMockVM_Down tests the Down method of MockVM.
func TestMockVM_Down(t *testing.T) {
	t.Run("DownFuncImplemented", func(t *testing.T) {
		// Given a MockVM with a custom DownFunc
		mockVM := NewMockVM()
		mockVM.DownFunc = func(verbose ...bool) error {
			return nil
		}

		// When calling Down
		err := mockVM.Down()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
	})

	t.Run("DownFuncNotImplemented", func(t *testing.T) {
		// Given a MockVM without a custom DownFunc
		mockVM := NewMockVM()

		// When calling Down
		err := mockVM.Down()

		// Then an error should be returned
		if err == nil || err.Error() != "DownFunc not implemented" {
			t.Fatalf("Expected 'DownFunc not implemented' error, got %v", err)
		}
	})
}

// TestMockVM_Delete tests the Delete method of MockVM.
func TestMockVM_Delete(t *testing.T) {
	t.Run("DeleteFuncImplemented", func(t *testing.T) {
		// Given a MockVM with a custom DeleteFunc
		mockVM := NewMockVM()
		mockVM.DeleteFunc = func(verbose ...bool) error {
			return nil
		}

		// When calling Delete
		err := mockVM.Delete()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
	})

	t.Run("DeleteFuncNotImplemented", func(t *testing.T) {
		// Given a MockVM without a custom DeleteFunc
		mockVM := NewMockVM()

		// When calling Delete
		err := mockVM.Delete()

		// Then an error should be returned
		if err == nil || err.Error() != "DeleteFunc not implemented" {
			t.Fatalf("Expected 'DeleteFunc not implemented' error, got %v", err)
		}
	})
}

// TestMockVM_Info tests the Info method of MockVM.
func TestMockVM_Info(t *testing.T) {
	t.Run("InfoFuncImplemented", func(t *testing.T) {
		// Given a MockVM with a custom InfoFunc
		mockVM := NewMockVM()
		mockVM.InfoFunc = func() (interface{}, error) {
			return "mock info", nil
		}

		// When calling Info
		info, err := mockVM.Info()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		// And the info should be as expected
		if info != "mock info" {
			t.Errorf("Expected info 'mock info', got %v", info)
		}
	})

	t.Run("InfoFuncNotImplemented", func(t *testing.T) {
		// Given a MockVM without a custom InfoFunc
		mockVM := NewMockVM()

		// When calling Info
		info, err := mockVM.Info()

		// Then an error should be returned
		if err == nil || err.Error() != "InfoFunc not implemented" {
			t.Fatalf("Expected 'InfoFunc not implemented' error, got %v", err)
		}
		// And the info should be nil
		if info != nil {
			t.Errorf("Expected info to be nil, got %v", info)
		}
	})
}
