package stack

import (
	"fmt"
	"testing"
)

func TestMockStack_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new MockStack with a custom InitializeFunc
		mock := NewMockStack(nil)
		mock.InitializeFunc = func() error {
			return nil
		}

		// When Initialize is called
		err := mock.Initialize()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected error = %v, got = %v", nil, err)
		}
	})

	t.Run("NoInitializeFunc", func(t *testing.T) {
		// Given a new MockStack without a custom InitializeFunc
		mock := NewMockStack(nil)

		// When Initialize is called
		err := mock.Initialize()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected error = %v, got = %v", nil, err)
		}
	})
}

func TestMockStack_Up(t *testing.T) {
	mockUpErr := fmt.Errorf("mock up error")

	t.Run("WithFuncSet", func(t *testing.T) {
		// Given a new MockStack with a custom UpFunc that returns an error
		mock := NewMockStack(nil)
		mock.UpFunc = func() error {
			return mockUpErr
		}

		// When Up is called
		err := mock.Up()

		// Then the custom error should be returned
		if err != mockUpErr {
			t.Errorf("Expected error = %v, got = %v", mockUpErr, err)
		}
	})

	t.Run("WithNoFuncSet", func(t *testing.T) {
		// Given a new MockStack without a custom UpFunc
		mock := NewMockStack(nil)

		// When Up is called
		err := mock.Up()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected error = %v, got = %v", nil, err)
		}
	})
}
