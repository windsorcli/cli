// The MockVirt is a test implementation of the Virt interface
// It provides mockable function fields for all Virt interface methods
// It serves as a testing aid by allowing test cases to control behavior
// It enables isolated testing of components that depend on virtualization

package virt

// =============================================================================
// Types
// =============================================================================

// MockVirt is a struct that simulates a virt environment for testing purposes.
type MockVirt struct {
	UpFunc          func(verbose ...bool) error
	DownFunc        func() error
	WriteConfigFunc func() error
}

// =============================================================================
// Constructor
// =============================================================================

// NewMockVirt creates a new instance of MockVirt.
func NewMockVirt() *MockVirt {
	return &MockVirt{}
}

// =============================================================================
// Public Methods
// =============================================================================

// Up starts the mock virt.
// If a custom UpFunc is provided, it will use that function instead.
func (m *MockVirt) Up() error {
	if m.UpFunc != nil {
		return m.UpFunc()
	}
	return nil
}

// Down stops the mock virt.
// If a custom DownFunc is provided, it will use that function instead.
func (m *MockVirt) Down() error {
	if m.DownFunc != nil {
		return m.DownFunc()
	}
	return nil
}

// WriteConfig writes the configuration of the mock virt.
// If a custom WriteConfigFunc is provided, it will use that function instead.
func (m *MockVirt) WriteConfig() error {
	if m.WriteConfigFunc != nil {
		return m.WriteConfigFunc()
	}
	return nil
}

// =============================================================================
// Interface Compliance
// =============================================================================

// Ensure MockVirt implements the Virt, VirtualMachine, and ContainerRuntime interfaces
var _ Virt = (*MockVirt)(nil)
var _ VirtualMachine = (*MockVirt)(nil)
var _ ContainerRuntime = (*MockVirt)(nil)
