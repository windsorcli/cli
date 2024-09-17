package di

import (
	"sync"
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
	// Clear the container before each test
	container = make(map[string]interface{})
	mu = sync.RWMutex{}

	mockService := &MockServiceImpl{}
	Register("mockService", mockService)

	var resolvedService MockService
	if err := Resolve("mockService", &resolvedService); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if resolvedService.DoSomething() != "done" {
		t.Fatalf("expected 'done', got %s", resolvedService.DoSomething())
	}
}

func TestResolve_NoInstanceRegistered(t *testing.T) {
	// Clear the container before each test
	container = make(map[string]interface{})
	mu = sync.RWMutex{}

	var resolvedService MockService
	err := Resolve("nonExistentService", &resolvedService)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}

	expectedError := "no instance registered with name nonExistentService"
	if err.Error() != expectedError {
		t.Fatalf("expected error %q, got %q", expectedError, err.Error())
	}
}

func TestResolve_TargetNotPointer(t *testing.T) {
	// Clear the container before each test
	container = make(map[string]interface{})
	mu = sync.RWMutex{}

	mockService := &MockServiceImpl{}
	Register("mockService", mockService)

	var resolvedService MockService
	err := Resolve("mockService", resolvedService) // Passing non-pointer
	if err == nil {
		t.Fatalf("expected error, got nil")
	}

	expectedError := "target must be a non-nil pointer"
	if err.Error() != expectedError {
		t.Fatalf("expected error %q, got %q", expectedError, err.Error())
	}
}

func TestResolve_TargetNilPointer(t *testing.T) {
	// Clear the container before each test
	container = make(map[string]interface{})
	mu = sync.RWMutex{}

	mockService := &MockServiceImpl{}
	Register("mockService", mockService)

	var resolvedService *MockService
	err := Resolve("mockService", resolvedService) // Passing nil pointer
	if err == nil {
		t.Fatalf("expected error, got nil")
	}

	expectedError := "target must be a non-nil pointer"
	if err.Error() != expectedError {
		t.Fatalf("expected error %q, got %q", expectedError, err.Error())
	}
}

func TestResolve_TypeMismatch(t *testing.T) {
	// Clear the container before each test
	container = make(map[string]interface{})
	mu = sync.RWMutex{}

	mockService := &MockServiceImpl{}
	Register("mockService", mockService)

	var resolvedService string
	err := Resolve("mockService", &resolvedService) // Type mismatch
	if err == nil {
		t.Fatalf("expected error, got nil")
	}

	expectedError := "cannot assign instance of type *di.MockServiceImpl to target of type string"
	if err.Error() != expectedError {
		t.Fatalf("expected error %q, got %q", expectedError, err.Error())
	}
}
