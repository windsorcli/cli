package secrets

// =============================================================================
// Types
// =============================================================================

// MockProvider is a mock implementation of the Provider interface for testing.
type MockProvider struct {
	LoadSecretsFunc func() error
	ResolveFunc     func(ref SecretRef) (string, bool, error)
}

// =============================================================================
// Constructor
// =============================================================================

// NewMockProvider creates a new MockProvider instance.
func NewMockProvider() *MockProvider {
	return &MockProvider{}
}

// =============================================================================
// Provider Interface
// =============================================================================

// LoadSecrets calls the mock LoadSecretsFunc if set, otherwise returns nil.
func (m *MockProvider) LoadSecrets() error {
	if m.LoadSecretsFunc != nil {
		return m.LoadSecretsFunc()
	}
	return nil
}

// Resolve calls the mock ResolveFunc if set, otherwise returns unhandled.
func (m *MockProvider) Resolve(ref SecretRef) (string, bool, error) {
	if m.ResolveFunc != nil {
		return m.ResolveFunc(ref)
	}
	return "", false, nil
}

// =============================================================================
// Interface Compliance
// =============================================================================

var _ Provider = (*MockProvider)(nil)
