package generators

// MockGenerator is a mock implementation of the Generator interface for testing purposes
type MockGenerator struct {
	InitializeFunc func() error
	WriteFunc      func() error
}

// NewMockGenerator creates a new instance of MockGenerator
func NewMockGenerator() *MockGenerator {
	return &MockGenerator{}
}

// Initialize calls the mock InitializeFunc if set, otherwise returns nil
func (m *MockGenerator) Initialize() error {
	if m.InitializeFunc != nil {
		return m.InitializeFunc()
	}
	return nil
}

// Write calls the mock WriteFunc if set, otherwise returns nil
func (m *MockGenerator) Write() error {
	if m.WriteFunc != nil {
		return m.WriteFunc()
	}
	return nil
}

// Ensure MockGenerator implements Generator
var _ Generator = (*MockGenerator)(nil)
