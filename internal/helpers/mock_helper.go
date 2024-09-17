package helpers

// MockHelper is a mock implementation of the Helper interface
type MockHelper struct {
	GetEnvVarsFunc   func() (map[string]string, error)
	PrintEnvVarsFunc func() error
}

// NewMockHelper is a constructor for MockHelper
func NewMockHelper(
	getEnvVarsFunc func() (map[string]string, error),
	printEnvVarsFunc func() error,
) *MockHelper {
	return &MockHelper{
		GetEnvVarsFunc:   getEnvVarsFunc,
		PrintEnvVarsFunc: printEnvVarsFunc,
	}
}

func (m *MockHelper) GetEnvVars() (map[string]string, error) {
	if m.GetEnvVarsFunc != nil {
		return m.GetEnvVarsFunc()
	}
	return nil, nil
}

func (m *MockHelper) PrintEnvVars() error {
	if m.PrintEnvVarsFunc != nil {
		return m.PrintEnvVarsFunc()
	}
	return nil
}

// Ensure MockHelper implements Helper interface
var _ Helper = (*MockHelper)(nil)
