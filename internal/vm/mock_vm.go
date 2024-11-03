package vm

import (
	"fmt"
)

// MockVM is a struct that simulates a VM environment for testing purposes.
type MockVM struct {
	UpFunc     func(verbose ...bool) error
	DownFunc   func(verbose ...bool) error
	DeleteFunc func(verbose ...bool) error
	InfoFunc   func() (interface{}, error)
}

// NewMockVM creates a new instance of MockVM.
func NewMockVM() *MockVM {
	return &MockVM{}
}

// Up starts the mock VM.
// If a custom UpFunc is provided, it will use that function instead.
func (m *MockVM) Up(verbose ...bool) error {
	if m.UpFunc != nil {
		return m.UpFunc(verbose...)
	}
	return fmt.Errorf("UpFunc not implemented")
}

// Down stops the mock VM.
// If a custom DownFunc is provided, it will use that function instead.
func (m *MockVM) Down(verbose ...bool) error {
	if m.DownFunc != nil {
		return m.DownFunc(verbose...)
	}
	return fmt.Errorf("DownFunc not implemented")
}

// Delete removes the mock VM.
// If a custom DeleteFunc is provided, it will use that function instead.
func (m *MockVM) Delete(verbose ...bool) error {
	if m.DeleteFunc != nil {
		return m.DeleteFunc(verbose...)
	}
	return fmt.Errorf("DeleteFunc not implemented")
}

// Info retrieves information about the mock VM.
// If a custom InfoFunc is provided, it will use that function instead.
func (m *MockVM) Info() (interface{}, error) {
	if m.InfoFunc != nil {
		return m.InfoFunc()
	}
	return nil, fmt.Errorf("InfoFunc not implemented")
}

// Ensure MockVM implements the VMInterface
var _ VMInterface = (*MockVM)(nil)
