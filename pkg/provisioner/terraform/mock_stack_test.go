package terraform

// The MockStackTest provides test coverage for the MockStack implementation.
// It provides validation of the mock's function field behaviors,
// The MockStackTest ensures proper operation of the test double,
// verifying nil handling and custom function field behaviors.

import (
	"fmt"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestMockStack_Up(t *testing.T) {
	mockUpErr := fmt.Errorf("mock up error")

	t.Run("WithFuncSet", func(t *testing.T) {
		// Given a new MockStack with a custom UpFunc that returns an error
		mock := NewMockStack()
		mock.UpFunc = func(blueprint *blueprintv1alpha1.Blueprint, onApply ...func(id string) error) error {
			return mockUpErr
		}

		// When Up is called
		blueprint := &blueprintv1alpha1.Blueprint{}
		err := mock.Up(blueprint)

		// Then the custom error should be returned
		if err != mockUpErr {
			t.Errorf("Expected error = %v, got = %v", mockUpErr, err)
		}
	})

	t.Run("WithNoFuncSet", func(t *testing.T) {
		// Given a new MockStack without a custom UpFunc
		mock := NewMockStack()

		// When Up is called
		blueprint := &blueprintv1alpha1.Blueprint{}
		err := mock.Up(blueprint)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected error = %v, got = %v", nil, err)
		}
	})
}

func TestMockStack_Down(t *testing.T) {
	mockDownErr := fmt.Errorf("mock down error")

	t.Run("WithFuncSet", func(t *testing.T) {
		// Given a new MockStack with a custom DownFunc that returns an error
		mock := NewMockStack()
		mock.DownFunc = func(blueprint *blueprintv1alpha1.Blueprint) error {
			return mockDownErr
		}

		// When Down is called
		blueprint := &blueprintv1alpha1.Blueprint{}
		err := mock.Down(blueprint)

		// Then the custom error should be returned
		if err != mockDownErr {
			t.Errorf("Expected error = %v, got = %v", mockDownErr, err)
		}
	})

	t.Run("WithNoFuncSet", func(t *testing.T) {
		// Given a new MockStack without a custom DownFunc
		mock := NewMockStack()

		// When Down is called
		blueprint := &blueprintv1alpha1.Blueprint{}
		err := mock.Down(blueprint)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected error = %v, got = %v", nil, err)
		}
	})
}
