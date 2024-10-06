package context

import (
	"errors"
	"testing"
)

func TestMockContext(t *testing.T) {
	t.Run("GetContext", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			// Given a mock context that returns a context
			mockContext := NewMockContext(nil, nil, nil)
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
			mockContext := NewMockContext(nil, nil, nil)
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
			mockContext := NewMockContext(nil, nil, nil)

			// When calling GetContext
			_, err := mockContext.GetContext()

			// Then an error should be returned
			assertError(t, err, true)
			if err.Error() != "GetContextFunc not implemented" {
				t.Fatalf("expected error 'GetContextFunc not implemented', got %v", err)
			}
		})
	})

	t.Run("SetContext", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			// Given a mock context that sets the context successfully
			mockContext := NewMockContext(nil, nil, nil)
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
			mockContext := NewMockContext(nil, nil, nil)
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
			mockContext := NewMockContext(nil, nil, nil)

			// When calling SetContext
			err := mockContext.SetContext("test-context")

			// Then an error should be returned
			assertError(t, err, true)
			if err.Error() != "SetContextFunc not implemented" {
				t.Fatalf("expected error 'SetContextFunc not implemented', got %v", err)
			}
		})
	})

	t.Run("GetConfigRoot", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			// Given a mock context that returns a config root
			mockContext := NewMockContext(nil, nil, nil)
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
			mockContext := NewMockContext(nil, nil, nil)
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
			mockContext := NewMockContext(nil, nil, nil)

			// When calling GetConfigRoot
			_, err := mockContext.GetConfigRoot()

			// Then an error should be returned
			assertError(t, err, true)
			if err.Error() != "GetConfigRootFunc not implemented" {
				t.Fatalf("expected error 'GetConfigRootFunc not implemented', got %v", err)
			}
		})
	})
}
