package generators

// The MockGenerator is a testing component that provides a mock implementation of the Generator interface.
// It provides customizable function fields for testing different Generator behaviors.
// The MockGenerator enables isolated testing of components that depend on the Generator interface,
// allowing for controlled simulation of Generator operations in test scenarios.

// =============================================================================
// Types
// =============================================================================

// MockGenerator is a mock implementation of the Generator interface for testing purposes
type MockGenerator struct {
	InitializeFunc func() error
	WriteFunc      func(overwrite ...bool) error
	GenerateFunc   func(data map[string]any) error
}

// =============================================================================
// Constructor
// =============================================================================

// NewMockGenerator creates a new instance of MockGenerator
func NewMockGenerator() *MockGenerator {
	return &MockGenerator{}
}

// =============================================================================
// Public Methods
// =============================================================================

// Initialize calls the mock InitializeFunc if set, otherwise returns nil
func (m *MockGenerator) Initialize() error {
	if m.InitializeFunc != nil {
		return m.InitializeFunc()
	}
	return nil
}

// Write calls the mock WriteFunc if set, otherwise returns nil
func (m *MockGenerator) Write(overwrite ...bool) error {
	if m.WriteFunc != nil {
		return m.WriteFunc(overwrite...)
	}
	return nil
}

// Generate calls the mock GenerateFunc if set, otherwise returns nil
func (m *MockGenerator) Generate(data map[string]any) error {
	if m.GenerateFunc != nil {
		return m.GenerateFunc(data)
	}
	return nil
}

// =============================================================================
// Interface Compliance
// =============================================================================

// Ensure MockGenerator implements Generator
var _ Generator = (*MockGenerator)(nil)
