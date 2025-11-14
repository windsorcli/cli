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
	InitializeFunc       func() error
	UpFunc               func(verbose ...bool) error
	DownFunc             func() error
	WriteConfigFunc      func() error
	GetVMInfoFunc        func() (VMInfo, error)
	GetContainerInfoFunc func(name ...string) ([]ContainerInfo, error)
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

// Initialize initializes the mock virt.
// If a custom InitializeFunc is provided, it will use that function instead.
func (m *MockVirt) Initialize() error {
	if m.InitializeFunc != nil {
		return m.InitializeFunc()
	}
	return nil
}

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

// GetVMInfo retrieves information about the mock VM.
// If a custom GetVMInfoFunc is provided, it will use that function instead.
func (m *MockVirt) GetVMInfo() (VMInfo, error) {
	if m.GetVMInfoFunc != nil {
		return m.GetVMInfoFunc()
	}
	return VMInfo{}, nil
}

// GetContainerInfo retrieves information about the mock containers.
// If a custom GetContainerInfoFunc is provided, it will use that function instead.
func (m *MockVirt) GetContainerInfo(name ...string) ([]ContainerInfo, error) {
	if m.GetContainerInfoFunc != nil {
		return m.GetContainerInfoFunc(name...)
	}
	return []ContainerInfo{}, nil
}

// =============================================================================
// Interface Compliance
// =============================================================================

// Ensure MockVirt implements the Virt, VirtualMachine, and ContainerRuntime interfaces
var _ Virt = (*MockVirt)(nil)
var _ VirtualMachine = (*MockVirt)(nil)
var _ ContainerRuntime = (*MockVirt)(nil)
