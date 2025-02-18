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

// LoadSecrets loads the secrets from the specified path
func (s *OnePasswordCLISecretsProvider) LoadSecrets() error {
	// Sign in to the 1Password account using the vault details
	if _, err := s.shell.ExecSilent("op", "signin", "--account", s.vault.URL); err != nil {
		return fmt.Errorf("failed to sign in to 1Password: %w", err)
	}

	// Mark the provider as unlocked without loading secrets
	s.unlocked = true

	return nil
}

// GetSecret retrieves a secret value for the specified key
func (s *OnePasswordCLISecretsProvider) GetSecret(key string) (string, error) {
	// Split the key into secret and field parts
	parts := strings.SplitN(key, ".", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid key notation: %s. Expected format is 'secret.field'", key)
	}
	secret := parts[0]
	field := parts[1]

	// Construct the command to retrieve the secret from the vault using the op CLI
	output, err := s.shell.ExecSilent("op", "item", "get", secret, "--vault", s.vault.Name, "--fields", field)
	if err != nil {
		return "", fmt.Errorf("failed to retrieve secret from 1Password: %w", err)
	}

	return string(output), nil
}

// ParseSecrets parses a string and replaces ${{ op.<id>.<secret>.<field> }} references with their values
func (s *OnePasswordCLISecretsProvider) ParseSecrets(input string) (string, error) {
	// Dynamically generate the regex pattern for the specific vault ID
	opPattern := fmt.Sprintf(`(?i)\${{\s*op\.\s*%s\.\s*([a-zA-Z0-9_]+)\.\s*([a-zA-Z0-9_]+)\s*}}`, regexp.QuoteMeta(s.vault.ID))
	re := regexp.MustCompile(opPattern)

	input = re.ReplaceAllStringFunc(input, func(match string) string {
		// Extract the secret and field from the match
		submatches := re.FindStringSubmatch(match)
		secret := submatches[1]
		field := submatches[2]
		// Retrieve the secret value
		value, err := s.GetSecret(fmt.Sprintf("%s.%s", secret, field))
		if err != nil {
			return "<ERROR: secret not found: " + secret + "." + field + ">"
		}
		return value
	})

	return input, nil
}

// Ensure OnePasswordCLISecretsProvider implements SecretsProvider
var _ SecretsProvider = (*OnePasswordCLISecretsProvider)(nil)
