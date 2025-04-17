package di

import (
	"fmt"
	"sync"
)

// MockInjector extends the RealInjector with additional testing functionality
type MockInjector struct {
	*BaseInjector
	resolveAllErrors map[any]error
	mu               sync.RWMutex
}

// =============================================================================
// Constructor
// =============================================================================

// NewMockInjector creates a new mock DI injector
func NewMockInjector() *MockInjector {
	return &MockInjector{
		BaseInjector:     NewInjector(),
		resolveAllErrors: make(map[any]error),
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// SetResolveAllError sets a specific error to be returned when resolving all instances of a specific type
func (m *MockInjector) SetResolveAllError(targetType any, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.resolveAllErrors[targetType] = err
}

// Resolve overrides the RealInjector's Resolve method to add error simulation
func (m *MockInjector) Resolve(name string) any {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.BaseInjector.Resolve(name)
}

// ResolveAll overrides the RealInjector's ResolveAll method to add error simulation
func (m *MockInjector) ResolveAll(targetType any) ([]any, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for key, err := range m.resolveAllErrors {
		if fmt.Sprintf("%T", key) == fmt.Sprintf("%T", targetType) {
			return nil, err
		}
	}

	return m.BaseInjector.ResolveAll(targetType)
}
