package terraform

// MockModuleResolver is a mock implementation of the ModuleResolver interface
type MockModuleResolver struct {
	ProcessModulesFunc func() error
	GenerateTfvarsFunc func(overwrite bool) error
}

// =============================================================================
// Constructor
// =============================================================================

// NewMockModuleResolver creates a new MockModuleResolver instance
func NewMockModuleResolver() *MockModuleResolver {
	return &MockModuleResolver{}
}

// =============================================================================
// Public Methods
// =============================================================================

// ProcessModules calls the mock ProcessModulesFunc if set, otherwise returns nil
func (m *MockModuleResolver) ProcessModules() error {
	if m.ProcessModulesFunc != nil {
		return m.ProcessModulesFunc()
	}
	return nil
}

// GenerateTfvars calls the mock GenerateTfvarsFunc if set, otherwise returns nil
func (m *MockModuleResolver) GenerateTfvars(overwrite bool) error {
	if m.GenerateTfvarsFunc != nil {
		return m.GenerateTfvarsFunc(overwrite)
	}
	return nil
}

// =============================================================================
// Interface Compliance
// =============================================================================

// Ensure MockModuleResolver implements ModuleResolver interface
var _ ModuleResolver = (*MockModuleResolver)(nil)
