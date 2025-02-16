package secrets

import (
	"fmt"
	"regexp"
)

// Define regex pattern for ${{ secrets.<key> }} references as a constant
// Allow for any amount of spaces between the brackets and the "secrets.<key>"
// We ignore the gosec G101 error here because this pattern is used for identifying secret placeholders,
// not for storing actual secret values. The pattern itself does not contain any hardcoded credentials.
// #nosec G101
const secretPattern = `(?i)\${{\s*secrets\.\s*([a-zA-Z0-9_]+)\s*}}`

// SecretsProvider defines the interface for handling secrets operations
type SecretsProvider interface {
	// Initialize initializes the secrets provider
	Initialize() error

	// LoadSecrets loads the secrets from the specified path
	LoadSecrets() error

	// GetSecret retrieves a secret value for the specified key
	GetSecret(key string) (string, error)

	// ParseSecrets parses a string and replaces ${{ secrets.<key> }} references with their values
	ParseSecrets(input string) (string, error)

	// Unlock unlocks the secrets provider to allow access to secrets
	Unlock() error
}

// BaseSecretsProvider is a base implementation of the SecretsProvider interface
type BaseSecretsProvider struct {
	secrets  map[string]string
	unlocked bool
}

// NewBaseSecretsProvider creates a new BaseSecretsProvider instance
func NewBaseSecretsProvider() *BaseSecretsProvider {
	return &BaseSecretsProvider{secrets: make(map[string]string), unlocked: false}
}

// Initialize initializes the secrets provider
func (s *BaseSecretsProvider) Initialize() error {
	// Placeholder for any initialization logic needed for the secrets provider
	// Currently, it does nothing and returns nil
	return nil
}

// LoadSecrets loads the secrets from the specified path
func (s *BaseSecretsProvider) LoadSecrets() error {
	// Placeholder for loading secrets logic
	// Currently, it does nothing and returns nil
	return nil
}

// Unlock unlocks the secrets provider to allow access to secrets
func (s *BaseSecretsProvider) Unlock() error {
	s.unlocked = true
	// Placeholder for any error that might occur during unlocking
	// Currently, it does nothing and returns nil
	return nil
}

// GetSecret retrieves a secret value for the specified key
func (s *BaseSecretsProvider) GetSecret(key string) (string, error) {
	if !s.unlocked {
		return "********", nil
	}
	if value, ok := s.secrets[key]; ok {
		return value, nil
	}
	return "", fmt.Errorf("secret not found: %s", key)
}

// ParseSecrets parses a string and replaces ${{ secrets.<key> }} references with their values
func (s *BaseSecretsProvider) ParseSecrets(input string) (string, error) {
	re := regexp.MustCompile(secretPattern)
	input = re.ReplaceAllStringFunc(input, func(match string) string {
		// Extract the key from the match
		key := re.FindStringSubmatch(match)[1]
		// Retrieve the secret value
		value, err := s.GetSecret(key)
		if err != nil {
			return match
		}
		return value
	})

	return input, nil
}

// Ensure BaseSecretsProvider implements SecretsProvider
var _ SecretsProvider = (*BaseSecretsProvider)(nil)
