package di

import (
	"testing"
)

type MockService interface {
	DoSomething() string
}

type MockServiceImpl struct{}

func (m *MockServiceImpl) DoSomething() string {
	return "done"
}

func TestRegisterAndResolve_Success(t *testing.T) {
	// Create a new container for each test
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
	// Create a new container for each test
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

func TestResolve_TargetNotPointer(t *testing.T) {
	// Create a new container for each test
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
	// Create a new container for each test
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
	// Create a new container for each test
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
