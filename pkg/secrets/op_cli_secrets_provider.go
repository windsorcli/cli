package secrets

import (
	"fmt"
	"regexp"
	"strings"

	secretsConfigType "github.com/windsorcli/cli/api/v1alpha1/secrets"
	"github.com/windsorcli/cli/pkg/di"
)

// OnePasswordCLISecretsProvider is an implementation of the SecretsProvider interface
// that uses the 1Password CLI to manage secrets.
type OnePasswordCLISecretsProvider struct {
	BaseSecretsProvider
	vault secretsConfigType.OnePasswordVault
}

// NewOnePasswordCLISecretsProvider creates a new OnePasswordCLISecretsProvider instance
func NewOnePasswordCLISecretsProvider(vault secretsConfigType.OnePasswordVault, injector di.Injector) *OnePasswordCLISecretsProvider {
	baseProvider := NewBaseSecretsProvider(injector)
	return &OnePasswordCLISecretsProvider{
		BaseSecretsProvider: *baseProvider,
		vault:               vault,
	}
}

// GetSecret retrieves a secret value for the specified key
func (s *OnePasswordCLISecretsProvider) GetSecret(key string) (string, error) {
	if !s.unlocked {
		return "********", nil
	}
	parts := strings.SplitN(key, ".", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid key notation: %s. Expected format is 'secret.field'", key)
	}

	args := []string{"item", "get", parts[0], "--vault", s.vault.Name, "--fields", parts[1], "--reveal", "--account", s.vault.URL}

	output, err := s.shell.ExecSilent("op", args...)
	if err != nil {
		return "", fmt.Errorf("failed to retrieve secret from 1Password: %w", err)
	}

	secret := strings.TrimSpace(string(output))
	return secret, nil
}

// ParseSecrets parses a string and replaces ${{ op.<id>.<secret>.<field> }} references with their values
func (s *OnePasswordCLISecretsProvider) ParseSecrets(input string) (string, error) {
	// Simplified pattern to match the op secret format
	opPattern := `(?i)\${{\s*op\.\s*([^}]+)\s*}}`
	re := regexp.MustCompile(opPattern)

	input = re.ReplaceAllStringFunc(input, func(match string) string {
		// Extract the key path from the match
		submatches := re.FindStringSubmatch(match)
		keyPath := strings.TrimSpace(submatches[1])

		// Parse the key path using ParseKeys
		keys := ParseKeys(keyPath)
		if len(keys) != 3 {
			return "<ERROR: invalid key path>"
		}
		secret, field := keys[1], keys[2]

		// Retrieve the secret value
		value, err := s.GetSecret(fmt.Sprintf("%s.%s", secret, field))
		if err != nil {
			return "<ERROR: secret not found>"
		}
		return value
	})

	return input, nil
}

// Ensure OnePasswordCLISecretsProvider implements SecretsProvider
var _ SecretsProvider = (*OnePasswordCLISecretsProvider)(nil)
