package context

import (
	"errors"
	"testing"
)

func TestMockContext_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mockContext := NewMockContext()
		mockContext.InitializeFunc = func() error {
			return nil
		}
		err := mockContext.Initialize()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		mockContext := NewMockContext()
		err := mockContext.Initialize()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestMockContext_GetContext(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock context that returns a context
		mockContext := NewMockContext()
		mockContext.GetContextFunc = func() string {
			return "test-context"
		}

		// When calling GetContext
		context := mockContext.GetContext()

		// Then the context should be returned without error
		if context != "test-context" {
			t.Fatalf("expected context 'test-context', got %s", context)
		}
	})

	t.Run("Error", func(t *testing.T) {
		// Given a mock context that returns an error
		mockContext := NewMockContext()
		mockContext.GetContextFunc = func() string {
			return ""
		}

		// When calling GetContext
		context := mockContext.GetContext()

		// Then an error should be returned
		if context != "" {
			t.Fatalf("expected error, got none")
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock context with no implementation
		mockContext := NewMockContext()

		// When calling GetContext
		context := mockContext.GetContext()

		// Then no error should be returned and context should be "mock-context"
		if context != "mock-context" {
			t.Fatalf("expected context 'mock-context', got %s", context)
		}
	})
}

func TestMockContext_SetContext(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock context that sets the context successfully
		mockContext := NewMockContext()
		mockContext.SetContextFunc = func(context string) error {
			return nil
		}

		// When calling SetContext
		err := mockContext.SetContext("test-context")

		// Then no error should be returned
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("Error", func(t *testing.T) {
		// Given a mock context that returns an error when setting the context
		mockContext := NewMockContext()
		mockContext.SetContextFunc = func(context string) error {
			return errors.New("error setting context")
		}

		// When calling SetContext
		err := mockContext.SetContext("test-context")

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected error, got none")
		}
		if err.Error() != "error setting context" {
			t.Fatalf("expected error 'error setting context', got %v", err)
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock context with no implementation
		mockContext := NewMockContext()

		// When calling SetContext
		err := mockContext.SetContext("test-context")

		// Then no error should be returned
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}
