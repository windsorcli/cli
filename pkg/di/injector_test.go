package di

import (
	"testing"
)

// =============================================================================
// Test Setup
// =============================================================================

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

// MockService is a mock implementation of the Service interface
type MockService struct {
	PrintEnvVarsFunc func() error
}

func (m *MockService) PrintEnvVars() error {
	if m.PrintEnvVarsFunc != nil {
		return m.PrintEnvVarsFunc()
	}
	return nil
}

// Ensure MockService implements Service interface
var _ Service = (*MockService)(nil)

// Service interface for testing
type Service interface {
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
	resolvedInstance := injector.Resolve(name)
	if resolvedInstance == nil {
		t.Fatalf("expected no error, got %v", resolvedInstance)
	}
	resolvedService, ok := resolvedInstance.(MockItem)
	if !ok {
		t.Fatalf("expected MockItem, got %T", resolvedInstance)
	}
	return resolvedService
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestInjector_Register(t *testing.T) {
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
		resolvedInstance := injector.Resolve("nonExistentService")

		// Then the resolved instance should be nil
		if resolvedInstance != nil {
			t.Fatalf("expected nil, got %v", resolvedInstance)
		}
	})
}

func TestInjector_ResolveAll(t *testing.T) {
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
}

func TestInjector_Resolve(t *testing.T) {
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
		resolvedInstance := injector.Resolve("mockService")
		if resolvedInstance == nil {
			t.Fatalf("expected no error, got %v", resolvedInstance)
		}

		// Then the resolved instance should not be of type string
		if _, ok := resolvedInstance.(string); ok {
			t.Fatalf("expected type mismatch error, got %T", resolvedInstance)
		}
	})
}

func TestInjector_Service(t *testing.T) {
	// Given a new injector
	injector := setupInjector()

	// And a mock service registered
	instance1 := &MockService{}
	injector.Register("instance1", instance1)

	t.Run("RegisterAndResolveService", func(t *testing.T) {
		// When resolving the service
		resolvedInstance := injector.Resolve("instance1")
		if resolvedInstance == nil {
			t.Fatalf("Expected no error, got %v", resolvedInstance)
		}
		if resolvedInstance != instance1 {
			t.Fatalf("Expected %v, got %v", instance1, resolvedInstance)
		}
	})

	t.Run("ResolveAllServices", func(t *testing.T) {
		// And another mock service registered
		instance2 := &MockService{}
		injector.Register("instance2", instance2)

		// When resolving all services
		services, err := injector.ResolveAll((*Service)(nil))
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(services) != 2 {
			t.Fatalf("Expected 2 services, got %d", len(services))
		}
	})

	t.Run("ResolveAllWithNilInstance", func(t *testing.T) {
		// And a nil instance registered
		injector.Register("nilInstance", nil)

		// When resolving all services
		services, err := injector.ResolveAll((*Service)(nil))
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(services) != 2 {
			t.Fatalf("Expected 2 services, got %d", len(services))
		}
	})

	t.Run("ResolveAllWithNonServiceInstance", func(t *testing.T) {
		// And a non-service instance registered
		injector.Register("nonServiceInstance", struct{}{})

		// When resolving all services
		services, err := injector.ResolveAll((*Service)(nil))
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(services) != 2 {
			t.Fatalf("Expected 2 services, got %d", len(services))
		}
	})
}
