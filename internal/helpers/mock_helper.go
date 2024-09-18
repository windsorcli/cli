package helpers

// MockHelper is a mock implementation of the Helper interface
type MockHelper struct {
	GetEnvVarsFunc func() (map[string]string, error)
}

// NewMockHelper is a constructor for MockHelper
func NewMockHelper(
	getEnvVarsFunc func() (map[string]string, error),
) *MockHelper {
	return &MockHelper{
		GetEnvVarsFunc: getEnvVarsFunc,
	}
}

func (m *MockHelper) GetEnvVars() (map[string]string, error) {
	if m.GetEnvVarsFunc != nil {
		return m.GetEnvVarsFunc()
	}
	return nil, nil
}

// Ensure MockHelper implements Helper interface
var _ Helper = (*MockHelper)(nil)
