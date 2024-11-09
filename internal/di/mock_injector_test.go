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
		resolvedInstance, err := injector.Resolve("mockService")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
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
		_, err := injector.Resolve("nonExistentService")

		// Then an error should be returned
		expectedError := "no instance registered with name nonExistentService"
		if err == nil || err.Error() != expectedError {
			t.Fatalf("expected error %q, got %v", expectedError, err)
		}
	})

	t.Run("ResolveError", func(t *testing.T) {
		// Given a new mock diContainer
		injector := NewMockInjector()

		// And a resolve error set for a specific service
		expectedError := errors.New("resolve error")
		injector.SetResolveError("mockService", expectedError)

		// When resolving the service
		_, err := injector.Resolve("mockService")

		// Then the expected error should be returned
		if err == nil || err.Error() != expectedError.Error() {
			t.Fatalf("expected error %q, got %v", expectedError, err)
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

	t.Run("NoInstancesFound", func(t *testing.T) {
		// Given a new mock diContainer
		injector := NewMockInjector()

		// When resolving all services of type MockItem
		_, err := injector.ResolveAll((*MockItem)(nil))

		// Then an error should be returned
		expectedError := "no instances found for the given type"
		if err == nil || err.Error() != expectedError {
			t.Fatalf("expected error %q, got %v", expectedError, err)
		}
	})

	t.Run("ResolveAllError", func(t *testing.T) {
		// Given a new mock diContainer
		injector := NewMockInjector()

		// And a resolve all error set
		expectedError := errors.New("resolve all error")
		injector.SetResolveAllError(expectedError)

		// When resolving all services
		_, err := injector.ResolveAll((*MockItem)(nil))

		// Then the expected error should be returned
		if err == nil || err.Error() != expectedError.Error() {
			t.Fatalf("expected error %q, got %v", expectedError, err)
		}
	})
}
