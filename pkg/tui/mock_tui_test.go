package tui

import (
	"testing"
)

// =============================================================================
// Test Public Methods
// =============================================================================

// Tests for mock spinner Start behavior
func TestMockSpinner_Start(t *testing.T) {
	t.Run("CallsFunc", func(t *testing.T) {
		// Given a MockSpinner with StartFunc set
		var got string
		mock := NewMockSpinner()
		mock.StartFunc = func(message string) { got = message }

		// When Start is called
		mock.Start("loading")

		// Then StartFunc is invoked with the correct message
		if got != "loading" {
			t.Errorf("expected %q, got %q", "loading", got)
		}
	})

	t.Run("NoFunc", func(t *testing.T) {
		// Given a MockSpinner with no StartFunc
		mock := NewMockSpinner()

		// When Start is called
		// Then no panic occurs
		mock.Start("loading")
	})
}

// Tests for mock spinner Update behavior
func TestMockSpinner_Update(t *testing.T) {
	t.Run("CallsFunc", func(t *testing.T) {
		// Given a MockSpinner with UpdateFunc set
		var got string
		mock := NewMockSpinner()
		mock.UpdateFunc = func(message string) { got = message }

		// When Update is called
		mock.Update("refreshing")

		// Then UpdateFunc is invoked with the correct message
		if got != "refreshing" {
			t.Errorf("expected %q, got %q", "refreshing", got)
		}
	})

	t.Run("NoFunc", func(t *testing.T) {
		// Given a MockSpinner with no UpdateFunc
		mock := NewMockSpinner()

		// When Update is called
		// Then no panic occurs
		mock.Update("refreshing")
	})
}

// Tests for mock spinner Done behavior
func TestMockSpinner_Done(t *testing.T) {
	t.Run("CallsFunc", func(t *testing.T) {
		// Given a MockSpinner with DoneFunc set
		called := false
		mock := NewMockSpinner()
		mock.DoneFunc = func() { called = true }

		// When Done is called
		mock.Done()

		// Then DoneFunc is invoked
		if !called {
			t.Error("expected DoneFunc to be called")
		}
	})

	t.Run("NoFunc", func(t *testing.T) {
		// Given a MockSpinner with no DoneFunc
		mock := NewMockSpinner()

		// When Done is called
		// Then no panic occurs
		mock.Done()
	})
}

// Tests for mock spinner Fail behavior
func TestMockSpinner_Fail(t *testing.T) {
	t.Run("CallsFunc", func(t *testing.T) {
		// Given a MockSpinner with FailFunc set
		called := false
		mock := NewMockSpinner()
		mock.FailFunc = func() { called = true }

		// When Fail is called
		mock.Fail()

		// Then FailFunc is invoked
		if !called {
			t.Error("expected FailFunc to be called")
		}
	})

	t.Run("NoFunc", func(t *testing.T) {
		// Given a MockSpinner with no FailFunc
		mock := NewMockSpinner()

		// When Fail is called
		// Then no panic occurs
		mock.Fail()
	})
}
