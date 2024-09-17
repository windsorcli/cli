package di

import (
	"testing"
)

// MockService interface for testing
type MockService interface {
	DoSomething() string
}

// MockServiceImpl is a mock implementation of MockService
type MockServiceImpl struct{}

func (m *MockServiceImpl) DoSomething() string {
	return "done"
}

// AnotherMockServiceImpl is another mock implementation of MockService
type AnotherMockServiceImpl struct{}

func (a *AnotherMockServiceImpl) DoSomething() string {
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

func TestRegisterAndResolve_Success(t *testing.T) {
	container := NewContainer()

	mockService := &MockServiceImpl{}
	container.Register("mockService", mockService)

	resolvedInstance, err := container.Resolve("mockService")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	resolvedService, ok := resolvedInstance.(MockService)
	if !ok {
		t.Fatalf("expected MockService, got %T", resolvedInstance)
	}

	if resolvedService.DoSomething() != "done" {
		t.Fatalf("expected 'done', got %s", resolvedService.DoSomething())
	}
}

func TestResolve_NoInstanceRegistered(t *testing.T) {
	container := NewContainer()

	_, err := container.Resolve("nonExistentService")
	if err == nil {
		t.Fatalf("expected error, got nil")
	}

	expectedError := "no instance registered with name nonExistentService"
	if err.Error() != expectedError {
		t.Fatalf("expected error %q, got %q", expectedError, err.Error())
	}
}

func TestResolveAll_Success(t *testing.T) {
	container := NewContainer()

	mockService1 := &MockServiceImpl{}
	mockService2 := &AnotherMockServiceImpl{}
	container.Register("mockService1", mockService1)
	container.Register("mockService2", mockService2)

	resolvedInstances, err := container.ResolveAll((*MockService)(nil))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(resolvedInstances) != 2 {
		t.Fatalf("expected 2 instances, got %d", len(resolvedInstances))
	}

	for _, instance := range resolvedInstances {
		_, ok := instance.(MockService)
		if !ok {
			t.Fatalf("expected MockService, got %T", instance)
		}
	}
}

func TestResolveAll_NoInstancesFound(t *testing.T) {
	container := NewContainer()

	_, err := container.ResolveAll((*MockService)(nil))
	if err == nil {
		t.Fatalf("expected error, got nil")
	}

	expectedError := "no instances found for the given type"
	if err.Error() != expectedError {
		t.Fatalf("expected error %q, got %q", expectedError, err.Error())
	}
}

func TestResolveAll_TypeMismatch(t *testing.T) {
	container := NewContainer()

	mockService := &MockServiceImpl{}
	container.Register("mockService", mockService)

	_, err := container.ResolveAll((*UnimplementedService)(nil))
	if err == nil {
		t.Fatalf("expected error, got nil")
	}

	expectedError := "no instances found for the given type"
	if err.Error() != expectedError {
		t.Fatalf("expected error %q, got %q", expectedError, err.Error())
	}
}

func TestResolveAll_InvalidTargetType(t *testing.T) {
	container := NewContainer()

	_, err := container.ResolveAll("not a pointer to an interface")
	if err == nil {
		t.Fatalf("expected error, got nil")
	}

	expectedError := "targetType must be a pointer to an interface"
	if err.Error() != expectedError {
		t.Fatalf("expected error %q, got %q", expectedError, err.Error())
	}
}

func TestResolve_TargetNotPointer(t *testing.T) {
	container := NewContainer()

	mockService := &MockServiceImpl{}
	container.Register("mockService", mockService)

	resolvedInstance, err := container.Resolve("mockService")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	_, ok := resolvedInstance.(MockService)
	if !ok {
		t.Fatalf("expected MockService, got %T", resolvedInstance)
	}
}

func TestResolve_TargetNilPointer(t *testing.T) {
	container := NewContainer()

	mockService := &MockServiceImpl{}
	container.Register("mockService", mockService)

	resolvedInstance, err := container.Resolve("mockService")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	_, ok := resolvedInstance.(MockService)
	if !ok {
		t.Fatalf("expected MockService, got %T", resolvedInstance)
	}
}

func TestResolve_TypeMismatch(t *testing.T) {
	container := NewContainer()

	mockService := &MockServiceImpl{}
	container.Register("mockService", mockService)

	resolvedInstance, err := container.Resolve("mockService")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	_, ok := resolvedInstance.(string)
	if ok {
		t.Fatalf("expected type mismatch error, got %T", resolvedInstance)
	}
}

func TestRealContainer(t *testing.T) {
	container := NewContainer()

	// Test Register and Resolve
	instance1 := &MockHelper{}
	container.Register("instance1", instance1)

	resolvedInstance, err := container.Resolve("instance1")
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if resolvedInstance != instance1 {
		t.Fatalf("Expected %v, got %v", instance1, resolvedInstance)
	}

	// Test ResolveAll with valid instances
	instance2 := &MockHelper{}
	container.Register("instance2", instance2)

	helpers, err := container.ResolveAll((*Helper)(nil))
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if len(helpers) != 2 {
		t.Fatalf("Expected 2 helpers, got %d", len(helpers))
	}

	// Test ResolveAll with nil instance
	container.Register("nilInstance", nil)

	helpers, err = container.ResolveAll((*Helper)(nil))
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if len(helpers) != 2 {
		t.Fatalf("Expected 2 helpers, got %d", len(helpers))
	}

	// Test ResolveAll with non-helper instance
	container.Register("nonHelperInstance", struct{}{})

	helpers, err = container.ResolveAll((*Helper)(nil))
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if len(helpers) != 2 {
		t.Fatalf("Expected 2 helpers, got %d", len(helpers))
	}
}
