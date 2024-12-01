package context

// MockContext is a mock implementation of the ContextInterface
type MockContext struct {
	InitializeFunc    func() error               // Function to mock Initialize
	GetContextFunc    func() (string, error)     // Function to mock GetContext
	SetContextFunc    func(context string) error // Function to mock SetContext
	GetConfigRootFunc func() (string, error)     // Function to mock GetConfigRoot
}

// NewMockContext creates a new instance of MockContext
func NewMockContext() *MockContext {
	return &MockContext{}
}

// Initialize calls the mock InitializeFunc if set, otherwise returns nil
func (m *MockContext) Initialize() error {
	if m.InitializeFunc != nil {
		return m.InitializeFunc()
	}
	return nil
}

// GetContext calls the mock GetContextFunc if set, otherwise returns a reasonable default context and nil error
func (m *MockContext) GetContext() (string, error) {
	if m.GetContextFunc != nil {
		return m.GetContextFunc()
	}
	return "mock-context", nil
}

// SetContext calls the mock SetContextFunc if set, otherwise returns nil
func (m *MockContext) SetContext(context string) error {
	if m.SetContextFunc != nil {
		return m.SetContextFunc(context)
	}
	return nil
}

// GetConfigRoot calls the mock GetConfigRootFunc if set, otherwise returns a reasonable default config root and nil error
func (m *MockContext) GetConfigRoot() (string, error) {
	if m.GetConfigRootFunc != nil {
		return m.GetConfigRootFunc()
	}
	return "/mock/config/root", nil
}

// Ensure MockContext implements the ContextHandler interface
var _ ContextHandler = (*MockContext)(nil)
