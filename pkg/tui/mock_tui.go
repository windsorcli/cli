package tui

// MockSpinner is a mock implementation of the Spinner interface for testing.
type MockSpinner struct {
	StartFunc  func(message string)
	UpdateFunc func(message string)
	DoneFunc   func()
	FailFunc   func()
	PauseFunc  func()
	ResumeFunc func()
}

// =============================================================================
// Constructor
// =============================================================================

// NewMockSpinner creates a new MockSpinner instance.
func NewMockSpinner() *MockSpinner {
	return &MockSpinner{}
}

// =============================================================================
// Public Methods
// =============================================================================

// Start calls StartFunc if set, otherwise does nothing.
func (m *MockSpinner) Start(message string) {
	if m.StartFunc != nil {
		m.StartFunc(message)
	}
}

// Update calls UpdateFunc if set, otherwise does nothing.
func (m *MockSpinner) Update(message string) {
	if m.UpdateFunc != nil {
		m.UpdateFunc(message)
	}
}

// Done calls DoneFunc if set, otherwise does nothing.
func (m *MockSpinner) Done() {
	if m.DoneFunc != nil {
		m.DoneFunc()
	}
}

// Fail calls FailFunc if set, otherwise does nothing.
func (m *MockSpinner) Fail() {
	if m.FailFunc != nil {
		m.FailFunc()
	}
}

// Pause calls PauseFunc if set, otherwise does nothing.
func (m *MockSpinner) Pause() {
	if m.PauseFunc != nil {
		m.PauseFunc()
	}
}

// Resume calls ResumeFunc if set, otherwise does nothing.
func (m *MockSpinner) Resume() {
	if m.ResumeFunc != nil {
		m.ResumeFunc()
	}
}

// =============================================================================
// Interface Compliance
// =============================================================================

// Ensure MockSpinner implements Spinner.
var _ Spinner = (*MockSpinner)(nil)
