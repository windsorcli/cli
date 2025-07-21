package secrets

import (
	"fmt"
	"strings"

	secretsConfigType "github.com/windsorcli/cli/api/v1alpha1/secrets"
	"github.com/windsorcli/cli/pkg/di"
)

// The OnePasswordCLISecretsProvider is an implementation of the SecretsProvider interface
// It provides integration with 1Password CLI for secrets management with automatic shell scrubbing registration
// It serves as a bridge between the application and 1Password CLI with built-in security features
// It enables secure storage and retrieval of secrets using 1Password while automatically registering secrets for output scrubbing

// =============================================================================
// Types
// =============================================================================

// OnePasswordCLISecretsProvider is a struct that implements the SecretsProvider interface using 1Password CLI.
type OnePasswordCLISecretsProvider struct {
	*BaseSecretsProvider
	vault secretsConfigType.OnePasswordVault
}

// =============================================================================
// Constructor
// =============================================================================

// NewOnePasswordCLISecretsProvider creates a new OnePasswordCLISecretsProvider instance
func NewOnePasswordCLISecretsProvider(vault secretsConfigType.OnePasswordVault, injector di.Injector) *OnePasswordCLISecretsProvider {
	return &OnePasswordCLISecretsProvider{
		BaseSecretsProvider: NewBaseSecretsProvider(injector),
		vault:               vault,
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// GetSecret retrieves a secret value for the given key using 1Password CLI. Registers the secret for shell scrubbing to
// prevent exposure in output. Executes the CLI directly to avoid leaking secrets before registration. If locked, returns
// a masked value. Key format: 'secret.field'. Returns the secret or error on failure.
func (s *OnePasswordCLISecretsProvider) GetSecret(key string) (string, error) {
	if !s.unlocked {
		return "********", nil
	}

	parts := strings.SplitN(key, ".", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid key notation: %s. Expected format is 'secret.field'", key)
	}

	args := []string{"item", "get", parts[0], "--vault", s.vault.Name, "--fields", parts[1], "--reveal", "--account", s.vault.URL}
	cmd := s.shims.Command("op", args...)
	output, err := s.shims.CmdOutput(cmd)
	if err != nil {
		return "", fmt.Errorf("failed to retrieve secret from 1Password: %w", err)
	}

	value := strings.TrimSpace(string(output))
	s.shell.RegisterSecret(value)
	return value, nil
}

// ParseSecrets identifies and replaces ${{ op.<id>.<secret>.<field> }} patterns in the input
// with corresponding secret values from 1Password, ensuring the id matches the vault ID.
func (s *OnePasswordCLISecretsProvider) ParseSecrets(input string) (string, error) {
	pattern := `(?i)\${{\s*op(?:\.|\[)?\s*([^}]+)\s*}}`
	result := parseSecrets(input, pattern, func(keys []string) bool {
		return len(keys) == 3
	}, func(keys []string) (string, bool) {
		id, secret, field := keys[0], keys[1], keys[2]
		if id != s.vault.ID {
			return "", false
		}
		value, err := s.GetSecret(fmt.Sprintf("%s.%s", secret, field))
		if err != nil {
			return "<ERROR: secret not found>", true
		}
		return value, true
	})
	return result, nil
}

// Ensure OnePasswordCLISecretsProvider implements SecretsProvider
var _ SecretsProvider = (*OnePasswordCLISecretsProvider)(nil)
