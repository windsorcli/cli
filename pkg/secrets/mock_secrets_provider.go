package secrets

import "fmt"

// MockSecretsProvider is a mock implementation of the SecretsProvider interface for testing purposes
type MockSecretsProvider struct {
	InitializeFunc   func() error
	LoadSecretsFunc  func() error
	GetSecretFunc    func(key string) (string, error)
	ParseSecretsFunc func(input string) (string, error)
	UnlockFunc       func() error
}

// NewMockSecretsProvider creates a new instance of MockSecretsProvider
func NewMockSecretsProvider() *MockSecretsProvider {
	return &MockSecretsProvider{}
}

// Initialize calls the mock InitializeFunc if set, otherwise returns nil
func (m *MockSecretsProvider) Initialize() error {
	if m.InitializeFunc != nil {
		return m.InitializeFunc()
	}
	return nil
}

// LoadSecrets calls the mock LoadSecretsFunc if set, otherwise returns nil
func (m *MockSecretsProvider) LoadSecrets() error {
	if m.LoadSecretsFunc != nil {
		return m.LoadSecretsFunc()
	}
	return nil
}

// GetSecret calls the mock GetSecretFunc if set, otherwise returns an error indicating the secret was not found
func (m *MockSecretsProvider) GetSecret(key string) (string, error) {
	if m.GetSecretFunc != nil {
		return m.GetSecretFunc(key)
	}
	return "", fmt.Errorf("secret not found: %s", key)
}

// ParseSecrets calls the mock ParseSecretsFunc if set, otherwise returns the input unchanged
func (m *MockSecretsProvider) ParseSecrets(input string) (string, error) {
	if m.ParseSecretsFunc != nil {
		return m.ParseSecretsFunc(input)
	}
	return input, nil
}

// Unlock calls the mock UnlockFunc if set, otherwise returns nil
func (m *MockSecretsProvider) Unlock() error {
	if m.UnlockFunc != nil {
		return m.UnlockFunc()
	}
	return nil
}

// Ensure MockSecretsProvider implements SecretsProvider
var _ SecretsProvider = (*MockSecretsProvider)(nil)
