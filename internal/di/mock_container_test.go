package di

import (
	"errors"
	"testing"
)

func TestMockContainer_RegisterAndResolve(t *testing.T) {
	container := NewMockContainer()

	// Register an instance
	instance := "test instance"
	container.Register("test", instance)

	// Resolve the instance
	resolved, err := container.Resolve("test")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if resolved != instance {
		t.Fatalf("expected %v, got %v", instance, resolved)
	}
}

func TestMockContainer_Resolve_NotFound(t *testing.T) {
	container := NewMockContainer()

	// Try to resolve a non-existent instance
	_, err := container.Resolve("nonexistent")
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	expectedError := "no instance registered with name nonexistent"
	if err.Error() != expectedError {
		t.Fatalf("expected error %v, got %v", expectedError, err)
	}
}

func TestMockContainer_SetResolveError(t *testing.T) {
	container := NewMockContainer()

	// Set a resolve error
	resolveError := errors.New("resolve error")
	container.SetResolveError("test", resolveError)

	// Try to resolve the instance
	_, err := container.Resolve("test")
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if err != resolveError {
		t.Fatalf("expected error %v, got %v", resolveError, err)
	}
}

func TestMockContainer_ResolveAll(t *testing.T) {
	container := NewMockContainer()

	// Register instances
	instance1 := "test instance 1"
	instance2 := "test instance 2"
	container.Register("test1", instance1)
	container.Register("test2", instance2)

	// Resolve all instances
	var targetType interface{} = (*interface{})(nil)
	resolved, err := container.ResolveAll(targetType)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(resolved) != 2 {
		t.Fatalf("expected 2 instances, got %d", len(resolved))
	}
}

func TestMockContainer_SetResolveAllError(t *testing.T) {
	container := NewMockContainer()

	// Set a resolve all error
	resolveAllError := errors.New("resolve all error")
	container.SetResolveAllError(resolveAllError)

	// Try to resolve all instances
	var targetType interface{} = (*interface{})(nil)
	_, err := container.ResolveAll(targetType)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if err != resolveAllError {
		t.Fatalf("expected error %v, got %v", resolveAllError, err)
	}
}
