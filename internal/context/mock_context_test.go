package context

import (
	"errors"
	"testing"
)

func TestMockContext_GetContext(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock context that returns a context
		mockContext := NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "test-context", nil
		}

		// When calling GetContext
		context, err := mockContext.GetContext()

		// Then the context should be returned without error
		assertError(t, err, false)
		if context != "test-context" {
			t.Fatalf("expected context 'test-context', got %s", context)
		}
	})

	t.Run("Error", func(t *testing.T) {
		// Given a mock context that returns an error
		mockContext := NewMockContext()
		mockContext.GetContextFunc = func() (string, error) {
			return "", errors.New("error retrieving context")
		}

		// When calling GetContext
		_, err := mockContext.GetContext()

		// Then an error should be returned
		assertError(t, err, true)
		if err.Error() != "error retrieving context" {
			t.Fatalf("expected error 'error retrieving context', got %v", err)
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock context with no implementation
		mockContext := NewMockContext()

		// When calling GetContext
		context, err := mockContext.GetContext()

		// Then no error should be returned and context should be "mock-context"
		assertError(t, err, false)
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
		assertError(t, err, false)
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
		assertError(t, err, true)
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
		assertError(t, err, false)
	})
}

func TestMockContext_GetConfigRoot(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a mock context that returns a config root
		mockContext := NewMockContext()
		mockContext.GetConfigRootFunc = func() (string, error) {
			return "/mock/project/root/contexts/test-context", nil
		}

		// When calling GetConfigRoot
		configRoot, err := mockContext.GetConfigRoot()

		// Then the config root should be returned without error
		assertError(t, err, false)
		if configRoot != "/mock/project/root/contexts/test-context" {
			t.Fatalf("expected config root '/mock/project/root/contexts/test-context', got %s", configRoot)
		}
	})

	t.Run("Error", func(t *testing.T) {
		// Given a mock context that returns an error when getting the config root
		mockContext := NewMockContext()
		mockContext.GetConfigRootFunc = func() (string, error) {
			return "", errors.New("error retrieving config root")
		}

		// When calling GetConfigRoot
		_, err := mockContext.GetConfigRoot()

		// Then an error should be returned
		assertError(t, err, true)
		if err.Error() != "error retrieving config root" {
			t.Fatalf("expected error 'error retrieving config root', got %v", err)
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		// Given a mock context with no implementation
		mockContext := NewMockContext()

		// When calling GetConfigRoot
		configRoot, err := mockContext.GetConfigRoot()

		// Then no error should be returned and config root should be "/mock/config/root"
		assertError(t, err, false)
		if configRoot != "/mock/config/root" {
			t.Fatalf("expected config root '/mock/config/root', got %s", configRoot)
		}
	})
}
