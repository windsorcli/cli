package di

import (
	"sync"
)

// MockInjector extends the RealInjector with additional testing functionality
type MockInjector struct {
	*BaseInjector
	resolveError    map[string]error
	resolveAllError error
	mu              sync.RWMutex
}

// NewMockInjector creates a new mock DI injector
func NewMockInjector() *MockInjector {
	return &MockInjector{
		BaseInjector: NewInjector(),
		resolveError: make(map[string]error),
	}
}

// SetResolveError sets a specific error to be returned when resolving a particular instance
func (m *MockInjector) SetResolveError(name string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.resolveError[name] = err
}

// SetResolveAllError sets a specific error to be returned when resolving all instances of a type
func (m *MockInjector) SetResolveAllError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.resolveAllError = err
}

// Resolve overrides the RealInjector's Resolve method to add error simulation
func (m *MockInjector) Resolve(name string) (interface{}, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if err, exists := m.resolveError[name]; exists {
		return nil, err
	}

	return m.BaseInjector.Resolve(name)
}

// ResolveAll overrides the RealInjector's ResolveAll method to add error simulation
func (m *MockInjector) ResolveAll(targetType interface{}) ([]interface{}, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.resolveAllError != nil {
		return nil, m.resolveAllError
	}

	return m.BaseInjector.ResolveAll(targetType)
}
