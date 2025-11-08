package terraform

import "github.com/windsorcli/cli/pkg/di"

// MockModuleResolver is a mock implementation of the ModuleResolver interface
type MockModuleResolver struct {
	InitializeFunc     func() error
	ProcessModulesFunc func() error
	GenerateTfvarsFunc func(overwrite bool) error
}

// =============================================================================
// Constructor
// =============================================================================

// NewMockModuleResolver creates a new MockModuleResolver instance
func NewMockModuleResolver(injector di.Injector) *MockModuleResolver {
	return &MockModuleResolver{}
}

// =============================================================================
// Public Methods
// =============================================================================

// Initialize calls the mock InitializeFunc if set, otherwise returns nil
func (m *MockModuleResolver) Initialize() error {
	if m.InitializeFunc != nil {
		return m.InitializeFunc()
	}
	return nil
}

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
