package di

import (
	"sync"
)

// MockContainer extends the RealContainer with additional testing functionality
type MockContainer struct {
	*DIContainer
	resolveError    map[string]error
	resolveAllError error
	mu              sync.RWMutex
}

// NewMockContainer creates a new mock DI container
func NewMockContainer() *MockContainer {
	return &MockContainer{
		DIContainer:  NewContainer(),
		resolveError: make(map[string]error),
	}
}

// SetResolveError sets a specific error to be returned when resolving a particular instance
func (m *MockContainer) SetResolveError(name string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.resolveError[name] = err
}

// SetResolveAllError sets a specific error to be returned when resolving all instances of a type
func (m *MockContainer) SetResolveAllError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.resolveAllError = err
}

// Resolve overrides the RealContainer's Resolve method to add error simulation
func (m *MockContainer) Resolve(name string) (interface{}, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if err, exists := m.resolveError[name]; exists {
		return nil, err
	}

	return m.DIContainer.Resolve(name)
}

// ResolveAll overrides the RealContainer's ResolveAll method to add error simulation
func (m *MockContainer) ResolveAll(targetType interface{}) ([]interface{}, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.resolveAllError != nil {
		return nil, m.resolveAllError
	}

	return m.DIContainer.ResolveAll(targetType)
}
