package generators

import (
	"fmt"
	"testing"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestMockGenerator_Initialize(t *testing.T) {
	t.Run("Initialize", func(t *testing.T) {
		// Given a new MockGenerator
		mock := NewMockGenerator()

		// And the InitializeFunc is set to return nil
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
		// Given a new MockGenerator
		mock := NewMockGenerator()

		// When Initialize is called without setting InitializeFunc
		err := mock.Initialize()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected error = %v, got = %v", nil, err)
		}
	})
}

func TestMockGenerator_Write(t *testing.T) {
	// Given a mock write error
	mockWriteErr := fmt.Errorf("mock write error")

	t.Run("WithFuncSet", func(t *testing.T) {
		// Given a new MockGenerator
		mock := NewMockGenerator()

		// And the WriteFunc is set to return a mock error
		mock.WriteFunc = func(overwrite ...bool) error {
			return mockWriteErr
		}

		// When Write is called
		err := mock.Write()

		// Then the mock error should be returned
		if err != mockWriteErr {
			t.Errorf("Expected error = %v, got = %v", mockWriteErr, err)
		}
	})

	t.Run("WithNoFuncSet", func(t *testing.T) {
		// Given a new MockGenerator
		mock := NewMockGenerator()

		// When Write is called without setting WriteFunc
		err := mock.Write()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected error = %v, got = %v", nil, err)
		}
	})
}

func TestMockGenerator_Generate(t *testing.T) {
	// Given a mock generate error
	mockGenerateErr := fmt.Errorf("mock generate error")

	t.Run("WithFuncSet", func(t *testing.T) {
		// Given a new MockGenerator
		mock := NewMockGenerator()

		// And the GenerateFunc is set to return a mock error
		mock.GenerateFunc = func(data map[string]any, overwrite ...bool) error {
			return mockGenerateErr
		}

		// When Generate is called
		err := mock.Generate(map[string]any{"test": "data"})

		// Then the mock error should be returned
		if err != mockGenerateErr {
			t.Errorf("Expected error = %v, got = %v", mockGenerateErr, err)
		}
	})

	t.Run("WithNoFuncSet", func(t *testing.T) {
		// Given a new MockGenerator
		mock := NewMockGenerator()

		// When Generate is called without setting GenerateFunc
		err := mock.Generate(map[string]any{"test": "data"})

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected error = %v, got = %v", nil, err)
		}
	})
}
