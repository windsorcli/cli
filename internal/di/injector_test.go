package di

import (
	"testing"
)

// MockItem interface for testing
type MockItem interface {
	DoSomething() string
}

// MockItemImpl is a mock implementation of MockItem
type MockItemImpl struct{}

func (m *MockItemImpl) DoSomething() string {
	return "done"
}

// AnotherMockItemImpl is another mock implementation of MockItem
type AnotherMockItemImpl struct{}

func (a *AnotherMockItemImpl) DoSomething() string {
	return "done again"
}

// UnimplementedService is a new interface that is not implemented by any registered instances
type UnimplementedService interface {
	DoNothing() string
}

// MockHelper is a mock implementation of the Helper interface
type MockHelper struct {
	PrintEnvVarsFunc func() error
}

func (m *MockHelper) PrintEnvVars() error {
	if m.PrintEnvVarsFunc != nil {
		return m.PrintEnvVarsFunc()
	}
	return nil
}

// Ensure MockHelper implements Helper interface
var _ Helper = (*MockHelper)(nil)

// Helper interface for testing
type Helper interface {
	PrintEnvVars() error
}

// Helper functions for setup and common operations
func setupInjector() *BaseInjector {
	return NewInjector()
}

func registerMockItem(injector *BaseInjector, name string, service MockItem) {
	injector.Register(name, service)
}

func resolveService(t *testing.T, injector *BaseInjector, name string) MockItem {
	resolvedInstance, err := injector.Resolve(name)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	resolvedService, ok := resolvedInstance.(MockItem)
	if !ok {
		t.Fatalf("expected MockItem, got %T", resolvedInstance)
	}
	return resolvedService
}

func TestDIContainer(t *testing.T) {
	t.Run("RegisterAndResolve", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			// Given a new injector
			injector := setupInjector()

			// And a mock service registered
			mockService := &MockItemImpl{}
			registerMockItem(injector, "mockService", mockService)

			// When resolving the service
			resolvedService := resolveService(t, injector, "mockService")

			// Then the resolved service should perform as expected
			if resolvedService.DoSomething() != "done" {
				t.Fatalf("expected 'done', got %s", resolvedService.DoSomething())
			}
		})

		t.Run("NoInstanceRegistered", func(t *testing.T) {
			// Given a new injector
			injector := setupInjector()

			// When resolving a non-existent service
			_, err := injector.Resolve("nonExistentService")

			// Then an error should be returned
			expectedError := "no instance registered with name nonExistentService"
			if err == nil || err.Error() != expectedError {
				t.Fatalf("expected error %q, got %v", expectedError, err)
			}
		})
	})

	t.Run("ResolveAll", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			// Given a new injector
			injector := setupInjector()

			// And multiple mock services registered
			mockService1 := &MockItemImpl{}
			mockService2 := &AnotherMockItemImpl{}
			registerMockItem(injector, "mockService1", mockService1)
			registerMockItem(injector, "mockService2", mockService2)

			// When resolving all services of type MockItem
			resolvedInstances, err := injector.ResolveAll((*MockItem)(nil))
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}

			// Then the correct number of instances should be returned
			if len(resolvedInstances) != 2 {
				t.Fatalf("expected 2 instances, got %d", len(resolvedInstances))
			}

			// And all instances should be of type MockItem
			for _, instance := range resolvedInstances {
				_, ok := instance.(MockItem)
				if !ok {
					t.Fatalf("expected MockItem, got %T", instance)
				}
			}
		})

		t.Run("NoInstancesFound", func(t *testing.T) {
			// Given a new injector
			injector := setupInjector()

			// When resolving all services of type MockItem
			_, err := injector.ResolveAll((*MockItem)(nil))

			// Then an error should be returned
			expectedError := "no instances found for the given type"
			if err == nil || err.Error() != expectedError {
				t.Fatalf("expected error %q, got %v", expectedError, err)
			}
		})

		t.Run("TypeMismatch", func(t *testing.T) {
			// Given a new injector
			injector := setupInjector()

			// And a mock service registered
			mockService := &MockItemImpl{}
			registerMockItem(injector, "mockService", mockService)

			// When resolving all services of an unimplemented type
			_, err := injector.ResolveAll((*UnimplementedService)(nil))

			// Then an error should be returned
			expectedError := "no instances found for the given type"
			if err == nil || err.Error() != expectedError {
				t.Fatalf("expected error %q, got %v", expectedError, err)
			}
		})

		t.Run("InvalidTargetType", func(t *testing.T) {
			// Given a new injector
			injector := setupInjector()

			// When resolving all services with an invalid target type
			_, err := injector.ResolveAll("not a pointer to an interface")

			// Then an error should be returned
			expectedError := "targetType must be a pointer to an interface"
			if err == nil || err.Error() != expectedError {
				t.Fatalf("expected error %q, got %v", expectedError, err)
			}
		})
	})

	t.Run("Resolve", func(t *testing.T) {
		t.Run("TargetNotPointer", func(t *testing.T) {
			// Given a new injector
			injector := setupInjector()

			// And a mock service registered
			mockService := &MockItemImpl{}
			registerMockItem(injector, "mockService", mockService)

			// When resolving the service
			resolvedService := resolveService(t, injector, "mockService")

			// Then the resolved service should be of type MockItem
			if _, ok := resolvedService.(MockItem); !ok {
				t.Fatalf("expected MockItem, got %T", resolvedService)
			}
		})

		t.Run("TargetNilPointer", func(t *testing.T) {
			// Given a new injector
			injector := setupInjector()

			// And a mock service registered
			mockService := &MockItemImpl{}
			registerMockItem(injector, "mockService", mockService)

			// When resolving the service
			resolvedService := resolveService(t, injector, "mockService")

			// Then the resolved service should be of type MockItem
			if _, ok := resolvedService.(MockItem); !ok {
				t.Fatalf("expected MockItem, got %T", resolvedService)
			}
		})

		t.Run("TypeMismatch", func(t *testing.T) {
			// Given a new injector
			injector := setupInjector()

			// And a mock service registered
			mockService := &MockItemImpl{}
			registerMockItem(injector, "mockService", mockService)

			// When resolving the service
			resolvedInstance, err := injector.Resolve("mockService")
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}

			// Then the resolved instance should not be of type string
			if _, ok := resolvedInstance.(string); ok {
				t.Fatalf("expected type mismatch error, got %T", resolvedInstance)
			}
		})
	})

	t.Run("HelperTests", func(t *testing.T) {
		// Given a new injector
		injector := setupInjector()

		// And a mock helper registered
		instance1 := &MockHelper{}
		injector.Register("instance1", instance1)

		t.Run("RegisterAndResolveHelper", func(t *testing.T) {
			// When resolving the helper
			resolvedInstance, err := injector.Resolve("instance1")
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
			injector.Register("instance2", instance2)

			// When resolving all helpers
			helpers, err := injector.ResolveAll((*Helper)(nil))
			if err != nil {
				t.Fatalf("Expected no error, got %v", err)
			}
			if len(helpers) != 2 {
				t.Fatalf("Expected 2 helpers, got %d", len(helpers))
			}
		})

		t.Run("ResolveAllWithNilInstance", func(t *testing.T) {
			// And a nil instance registered
			injector.Register("nilInstance", nil)

			// When resolving all helpers
			helpers, err := injector.ResolveAll((*Helper)(nil))
			if err != nil {
				t.Fatalf("Expected no error, got %v", err)
			}
			if len(helpers) != 2 {
				t.Fatalf("Expected 2 helpers, got %d", len(helpers))
			}
		})

		t.Run("ResolveAllWithNonHelperInstance", func(t *testing.T) {
			// And a non-helper instance registered
			injector.Register("nonHelperInstance", struct{}{})

			// When resolving all helpers
			helpers, err := injector.ResolveAll((*Helper)(nil))
			if err != nil {
				t.Fatalf("Expected no error, got %v", err)
			}
			if len(helpers) != 2 {
				t.Fatalf("Expected 2 helpers, got %d", len(helpers))
			}
		})
	})
}
