package secrets

import (
	"fmt"
	"strings"

	secretsConfigType "github.com/windsorcli/cli/api/v1alpha1/secrets"
)

// =============================================================================
// Types
// =============================================================================

// OnePasswordCLIProvider implements the Provider interface using the 1Password CLI.
type OnePasswordCLIProvider struct {
	vault    secretsConfigType.OnePasswordVault
	unlocked bool
	shims    *Shims
}

// =============================================================================
// Constructor
// =============================================================================

// NewOnePasswordCLIProvider creates a new OnePasswordCLIProvider instance.
func NewOnePasswordCLIProvider(vault secretsConfigType.OnePasswordVault) *OnePasswordCLIProvider {
	return &OnePasswordCLIProvider{
		vault: vault,
		shims: NewShims(),
	}
}

// =============================================================================
// Provider Interface
// =============================================================================

// LoadSecrets marks the provider as unlocked.
func (s *OnePasswordCLIProvider) LoadSecrets() error {
	s.unlocked = true
	return nil
}

// Resolve fetches a secret from 1Password CLI.
// Returns handled=true only if the vault ID matches.
func (s *OnePasswordCLIProvider) Resolve(ref SecretRef) (string, bool, error) {
	if ref.Vault != s.vault.ID {
		return "", false, nil
	}
	if ref.Field == "" {
		return "", true, fmt.Errorf("secret() field is required for 1Password provider")
	}
	if !s.unlocked {
		return "********", true, nil
	}

	args := []string{"item", "get", ref.Item, "--vault", s.vault.Name, "--fields", ref.Field, "--reveal", "--account", s.vault.URL}
	cmd := s.shims.Command("op", args...)
	output, err := s.shims.CmdOutput(cmd)
	if err != nil {
		return "", true, fmt.Errorf("failed to retrieve secret from 1Password: %w", err)
	}

	value := strings.TrimSpace(string(output))
	return value, true, nil
}

// =============================================================================
// Interface Compliance
// =============================================================================

var _ Provider = (*OnePasswordCLIProvider)(nil)
