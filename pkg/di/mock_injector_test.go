package di

import (
	"errors"
	"testing"
)

func TestMockInjector_RegisterAndResolve(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new mock diContainer
		injector := NewMockInjector()

		// And a mock service registered
		mockService := &MockItemImpl{}
		injector.Register("mockService", mockService)

		// When resolving the service
		resolvedInstance := injector.Resolve("mockService")
		if resolvedInstance == nil {
			t.Fatalf("expected no error, got %v", resolvedInstance)
		}
		resolvedService, ok := resolvedInstance.(MockItem)
		if !ok {
			t.Fatalf("expected MockItem, got %T", resolvedInstance)
		}

		// Then the resolved service should perform as expected
		if resolvedService.DoSomething() != "done" {
			t.Fatalf("expected 'done', got %s", resolvedService.DoSomething())
		}
	})

	t.Run("NoInstanceRegistered", func(t *testing.T) {
		// Given a new mock diContainer
		injector := NewMockInjector()

		// When resolving a non-existent service
		resolvedInstance := injector.Resolve("nonExistentService")

		// Then the resolved instance should be nil
		if resolvedInstance != nil {
			t.Fatalf("expected nil, got %v", resolvedInstance)
		}
	})
}

func TestMockContainer_ResolveAll(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new mock diContainer
		injector := NewMockInjector()

		// And multiple mock services registered
		mockService1 := &MockItemImpl{}
		mockService2 := &AnotherMockItemImpl{}
		injector.Register("mockService1", mockService1)
		injector.Register("mockService2", mockService2)

		// When resolving all services of type MockItem
		resolvedInstances, err := injector.ResolveAll((*MockItem)(nil))
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Then the correct number of instances should be returned
		if len(resolvedInstances) != 2 {
			t.Fatalf("expected 2 instances, got %d", len(resolvedInstances))
		}

		// And all instances should be of type MockService
		for _, instance := range resolvedInstances {
			_, ok := instance.(MockItem)
			if !ok {
				t.Fatalf("expected MockItem, got %T", instance)
			}
		}
	})

	t.Run("ResolveAllError", func(t *testing.T) {
		// Given a new mock diContainer
		injector := NewMockInjector()

		// And a resolve all error set
		expectedError := errors.New("resolve all error")
		injector.SetResolveAllError((*MockItem)(nil), expectedError)

		// When resolving all services
		_, err := injector.ResolveAll((*MockItem)(nil))

		// Then the expected error should be returned
		if err == nil || err.Error() != expectedError.Error() {
			t.Fatalf("expected error %q, got %v", expectedError, err)
		}
	})
}
