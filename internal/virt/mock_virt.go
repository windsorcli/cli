package virt

// MockVirt is a struct that simulates a virt environment for testing purposes.
type MockVirt struct {
	InitializeFunc       func() error
	UpFunc               func(verbose ...bool) error
	DownFunc             func(verbose ...bool) error
	DeleteFunc           func(verbose ...bool) error
	PrintInfoFunc        func() error
	WriteConfigFunc      func() error
	GetVMInfoFunc        func() (VMInfo, error)
	GetContainerInfoFunc func() ([]ContainerInfo, error)
}

// NewMockVirt creates a new instance of MockVirt.
func NewMockVirt() *MockVirt {
	return &MockVirt{}
}

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
func (m *MockVirt) Up(verbose ...bool) error {
	if m.UpFunc != nil {
		return m.UpFunc(verbose...)
	}
	return nil
}

// Down stops the mock virt.
// If a custom DownFunc is provided, it will use that function instead.
func (m *MockVirt) Down(verbose ...bool) error {
	if m.DownFunc != nil {
		return m.DownFunc(verbose...)
	}
	return nil
}

// Delete removes the mock virt.
// If a custom DeleteFunc is provided, it will use that function instead.
func (m *MockVirt) Delete(verbose ...bool) error {
	if m.DeleteFunc != nil {
		return m.DeleteFunc(verbose...)
	}
	return nil
}

// PrintInfo prints information about the mock virt.
// If a custom PrintInfoFunc is provided, it will use that function instead.
func (m *MockVirt) PrintInfo() error {
	if m.PrintInfoFunc != nil {
		return m.PrintInfoFunc()
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
func (m *MockVirt) GetContainerInfo() ([]ContainerInfo, error) {
	if m.GetContainerInfoFunc != nil {
		return m.GetContainerInfoFunc()
	}
	return nil, nil
}

// Ensure MockVirt implements the Virt, VirtualMachine, and ContainerRuntime interfaces
var _ Virt = (*MockVirt)(nil)
var _ VirtualMachine = (*MockVirt)(nil)
var _ ContainerRuntime = (*MockVirt)(nil)
