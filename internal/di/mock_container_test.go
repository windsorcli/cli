package di

import (
	"errors"
	"testing"
)

func TestMockContainer_RegisterAndResolve(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new mock container
		container := NewMockContainer()

		// And a mock service registered
		mockService := &MockServiceImpl{}
		container.Register("mockService", mockService)

		// When resolving the service
		resolvedInstance, err := container.Resolve("mockService")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		resolvedService, ok := resolvedInstance.(MockService)
		if !ok {
			t.Fatalf("expected MockService, got %T", resolvedInstance)
		}

		// Then the resolved service should perform as expected
		if resolvedService.DoSomething() != "done" {
			t.Fatalf("expected 'done', got %s", resolvedService.DoSomething())
		}
	})

	t.Run("NoInstanceRegistered", func(t *testing.T) {
		// Given a new mock container
		container := NewMockContainer()

		// When resolving a non-existent service
		_, err := container.Resolve("nonExistentService")

		// Then an error should be returned
		expectedError := "no instance registered with name nonExistentService"
		if err == nil || err.Error() != expectedError {
			t.Fatalf("expected error %q, got %v", expectedError, err)
		}
	})

	t.Run("ResolveError", func(t *testing.T) {
		// Given a new mock container
		container := NewMockContainer()

		// And a resolve error set for a specific service
		expectedError := errors.New("resolve error")
		container.SetResolveError("mockService", expectedError)

		// When resolving the service
		_, err := container.Resolve("mockService")

		// Then the expected error should be returned
		if err == nil || err.Error() != expectedError.Error() {
			t.Fatalf("expected error %q, got %v", expectedError, err)
		}
	})
}

func TestMockContainer_ResolveAll(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a new mock container
		container := NewMockContainer()

		// And multiple mock services registered
		mockService1 := &MockServiceImpl{}
		mockService2 := &AnotherMockServiceImpl{}
		container.Register("mockService1", mockService1)
		container.Register("mockService2", mockService2)

		// When resolving all services of type MockService
		resolvedInstances, err := container.ResolveAll((*MockService)(nil))
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Then the correct number of instances should be returned
		if len(resolvedInstances) != 2 {
			t.Fatalf("expected 2 instances, got %d", len(resolvedInstances))
		}

		// And all instances should be of type MockService
		for _, instance := range resolvedInstances {
			_, ok := instance.(MockService)
			if !ok {
				t.Fatalf("expected MockService, got %T", instance)
			}
		}
	})

	t.Run("NoInstancesFound", func(t *testing.T) {
		// Given a new mock container
		container := NewMockContainer()

		// When resolving all services of type MockService
		_, err := container.ResolveAll((*MockService)(nil))

		// Then an error should be returned
		expectedError := "no instances found for the given type"
		if err == nil || err.Error() != expectedError {
			t.Fatalf("expected error %q, got %v", expectedError, err)
		}
	})

	t.Run("ResolveAllError", func(t *testing.T) {
		// Given a new mock container
		container := NewMockContainer()

		// And a resolve all error set
		expectedError := errors.New("resolve all error")
		container.SetResolveAllError(expectedError)

		// When resolving all services
		_, err := container.ResolveAll((*MockService)(nil))

		// Then the expected error should be returned
		if err == nil || err.Error() != expectedError.Error() {
			t.Fatalf("expected error %q, got %v", expectedError, err)
		}
	})
}
