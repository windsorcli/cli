package template

import "github.com/windsorcli/cli/pkg/di"

// MockTemplate is a mock implementation of the Template interface for testing
type MockTemplate struct {
	InitializeFunc func() error
	ProcessFunc    func(templateData map[string][]byte, renderedData map[string]any) error
}

// =============================================================================
// Constructor
// =============================================================================

// NewMockTemplate creates a new MockTemplate instance
func NewMockTemplate(injector di.Injector) *MockTemplate {
	return &MockTemplate{}
}

// =============================================================================
// Public Methods
// =============================================================================

// Initialize calls the mock InitializeFunc if set, otherwise returns nil
func (m *MockTemplate) Initialize() error {
	if m.InitializeFunc != nil {
		return m.InitializeFunc()
	}
	return nil
}

// Process calls the mock ProcessFunc if set, otherwise returns nil
func (m *MockTemplate) Process(templateData map[string][]byte, renderedData map[string]any) error {
	if m.ProcessFunc != nil {
		return m.ProcessFunc(templateData, renderedData)
	}
	return nil
}

// =============================================================================
// Interface Compliance
// =============================================================================

// Ensure MockTemplate implements Template interface
var _ Template = (*MockTemplate)(nil)
