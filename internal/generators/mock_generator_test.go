package generators

import (
	"fmt"
	"testing"
)

func TestMockGenerator_Initialize(t *testing.T) {
	t.Run("Initialize", func(t *testing.T) {
		mock := NewMockGenerator()
		mock.InitializeFunc = func() error {
			return nil
		}
		err := mock.Initialize()
		if err != nil {
			t.Errorf("Expected error = %v, got = %v", nil, err)
		}
	})

	t.Run("NoInitializeFunc", func(t *testing.T) {
		mock := NewMockGenerator()
		err := mock.Initialize()
		if err != nil {
			t.Errorf("Expected error = %v, got = %v", nil, err)
		}
	})
}

func TestMockGenerator_Write(t *testing.T) {
	mockWriteErr := fmt.Errorf("mock write error")

	t.Run("WithFuncSet", func(t *testing.T) {
		mock := NewMockGenerator()
		mock.WriteFunc = func() error {
			return mockWriteErr
		}
		err := mock.Write()
		if err != mockWriteErr {
			t.Errorf("Expected error = %v, got = %v", mockWriteErr, err)
		}
	})

	t.Run("WithNoFuncSet", func(t *testing.T) {
		mock := NewMockGenerator()
		err := mock.Write()
		if err != nil {
			t.Errorf("Expected error = %v, got = %v", nil, err)
		}
	})
}
