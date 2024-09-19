package di

import (
	"errors"
	"testing"
)

// Helper functions for setup and common operations specific to MockContainer
func setupMockContainer() *MockContainer {
	return NewMockContainer()
}

func registerMockContainerService(container *MockContainer, name string, service MockService) {
	container.Register(name, service)
}

func resolveMockContainerService(t *testing.T, container *MockContainer, name string) MockService {
	resolvedInstance, err := container.Resolve(name)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	resolvedService, ok := resolvedInstance.(MockService)
	if !ok {
		t.Fatalf("expected MockService, got %T", resolvedInstance)
	}
	return resolvedService
}

func TestMockContainer(t *testing.T) {
	t.Run("RegisterAndResolve", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			// Given a new mock container
			container := setupMockContainer()

			// And a mock service registered
			mockService := &MockServiceImpl{}
			registerMockContainerService(container, "mockService", mockService)

			// When resolving the service
			resolvedService := resolveMockContainerService(t, container, "mockService")

			// Then the resolved service should perform as expected
			if resolvedService.DoSomething() != "done" {
				t.Fatalf("expected 'done', got %s", resolvedService.DoSomething())
			}
		})

		t.Run("NoInstanceRegistered", func(t *testing.T) {
			// Given a new mock container
			container := setupMockContainer()

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
			container := setupMockContainer()

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
	})

	t.Run("ResolveAll", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			// Given a new mock container
			container := setupMockContainer()

			// And multiple mock services registered
			mockService1 := &MockServiceImpl{}
			mockService2 := &AnotherMockServiceImpl{}
			registerMockContainerService(container, "mockService1", mockService1)
			registerMockContainerService(container, "mockService2", mockService2)

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
			container := setupMockContainer()

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
			container := setupMockContainer()

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
	})

	t.Run("HelperTests", func(t *testing.T) {
		// Given a new mock container
		container := setupMockContainer()

		// And a mock helper registered
		instance1 := &MockHelper{}
		container.Register("instance1", instance1)

		t.Run("RegisterAndResolveHelper", func(t *testing.T) {
			// When resolving the helper
			resolvedInstance, err := container.Resolve("instance1")
			if err != nil {
				t.Fatalf("Expected no error, got %v", err)
			}
			if resolvedInstance != instance1 {
				t.Fatalf("Expected %v, got %v", instance1, resolvedInstance)
			}
		})

		t.Run("ResolveAllHelpers", func(t *testing.T) {
			// And another mock helper registered
			instance2 := &MockHelper{}
			container.Register("instance2", instance2)

			// When resolving all helpers
			helpers, err := container.ResolveAll((*Helper)(nil))
			if err != nil {
				t.Fatalf("Expected no error, got %v", err)
			}
			if len(helpers) != 2 {
				t.Fatalf("Expected 2 helpers, got %d", len(helpers))
			}
		})

		t.Run("ResolveAllWithNilInstance", func(t *testing.T) {
			// And a nil instance registered
			container.Register("nilInstance", nil)

			// When resolving all helpers
			helpers, err := container.ResolveAll((*Helper)(nil))
			if err != nil {
				t.Fatalf("Expected no error, got %v", err)
			}
			if len(helpers) != 2 {
				t.Fatalf("Expected 2 helpers, got %d", len(helpers))
			}
		})

		t.Run("ResolveAllWithNonHelperInstance", func(t *testing.T) {
			// And a non-helper instance registered
			container.Register("nonHelperInstance", struct{}{})

			// When resolving all helpers
			helpers, err := container.ResolveAll((*Helper)(nil))
			if err != nil {
				t.Fatalf("Expected no error, got %v", err)
			}
			if len(helpers) != 2 {
				t.Fatalf("Expected 2 helpers, got %d", len(helpers))
			}
		})
	})
}
