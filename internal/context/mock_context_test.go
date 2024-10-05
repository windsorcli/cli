package context

import (
	"errors"
	"testing"
)

func TestNewMockContext(t *testing.T) {
	mockContext := NewMockContext(
		func() (string, error) { return "", nil },
		func(context string) error { return nil },
		func() (string, error) { return "", nil },
	)
	if mockContext == nil {
		t.Fatalf("expected a new MockContext instance, got nil")
	}
}

func TestMockContext_GetContext(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mockContext := &MockContext{
			GetContextFunc: func() (string, error) {
				return "test-context", nil
			},
		}

		context, err := mockContext.GetContext()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if context != "test-context" {
			t.Fatalf("expected context 'test-context', got %s", context)
		}
	})

	t.Run("Error", func(t *testing.T) {
		mockContext := &MockContext{
			GetContextFunc: func() (string, error) {
				return "", errors.New("error retrieving context")
			},
		}

		_, err := mockContext.GetContext()
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}
		if err.Error() != "error retrieving context" {
			t.Fatalf("expected error 'error retrieving context', got %v", err)
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		mockContext := &MockContext{}

		_, err := mockContext.GetContext()
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}
		if err.Error() != "GetContextFunc not implemented" {
			t.Fatalf("expected error 'GetContextFunc not implemented', got %v", err)
		}
	})
}

func TestMockContext_SetContext(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mockContext := &MockContext{
			SetContextFunc: func(context string) error {
				return nil
			},
		}

		err := mockContext.SetContext("test-context")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("Error", func(t *testing.T) {
		mockContext := &MockContext{
			SetContextFunc: func(context string) error {
				return errors.New("error setting context")
			},
		}

		err := mockContext.SetContext("test-context")
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}
		if err.Error() != "error setting context" {
			t.Fatalf("expected error 'error setting context', got %v", err)
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		mockContext := &MockContext{}

		err := mockContext.SetContext("test-context")
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}
		if err.Error() != "SetContextFunc not implemented" {
			t.Fatalf("expected error 'SetContextFunc not implemented', got %v", err)
		}
	})
}

func TestMockContext_GetConfigRoot(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mockContext := &MockContext{
			GetConfigRootFunc: func() (string, error) {
				return "/mock/project/root/contexts/test-context", nil
			},
		}

		configRoot, err := mockContext.GetConfigRoot()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if configRoot != "/mock/project/root/contexts/test-context" {
			t.Fatalf("expected config root '/mock/project/root/contexts/test-context', got %s", configRoot)
		}
	})

	t.Run("Error", func(t *testing.T) {
		mockContext := &MockContext{
			GetConfigRootFunc: func() (string, error) {
				return "", errors.New("error retrieving config root")
			},
		}

		_, err := mockContext.GetConfigRoot()
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}
		if err.Error() != "error retrieving config root" {
			t.Fatalf("expected error 'error retrieving config root', got %v", err)
		}
	})

	t.Run("NotImplemented", func(t *testing.T) {
		mockContext := &MockContext{}

		_, err := mockContext.GetConfigRoot()
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}
		if err.Error() != "GetConfigRootFunc not implemented" {
			t.Fatalf("expected error 'GetConfigRootFunc not implemented', got %v", err)
		}
	})
}
