// The OnePasswordCLIProvider is the secrets provider for a 1Password vault through the op CLI.
// It provides one field of one item for each secret() call.
// It runs op item get with the vault, account, and field of the reference.
// The runtime selects it when OP_SERVICE_ACCOUNT_TOKEN is not set.

package secrets

import (
	"context"
	"fmt"
	"strings"

	"github.com/windsorcli/cli/internal/werror"
)

// =============================================================================
// Types
// =============================================================================

// OnePasswordCLIProvider implements the Provider interface using the 1Password CLI.
type OnePasswordCLIProvider struct {
	vault    OnePasswordVault
	unlocked bool
	shims    *Shims
}

// =============================================================================
// Constructor
// =============================================================================

// NewOnePasswordCLIProvider creates a new OnePasswordCLIProvider instance.
func NewOnePasswordCLIProvider(vault OnePasswordVault) *OnePasswordCLIProvider {
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

// Resolve fetches a secret from 1Password CLI. ref.Item is config-controlled and positional;
// it's placed after "--" (end-of-options) in the op argv so an item name beginning with "-"
// is parsed as the operand op expects, not as an op flag.
// Returns handled=true only if the vault ID matches.
func (s *OnePasswordCLIProvider) Resolve(ref SecretRef) (string, bool, error) {
	if ref.Vault != s.vault.ID {
		return "", false, nil
	}
	if ref.Field == "" {
		return "", true, missingOnePasswordFieldError(ref)
	}
	if !s.unlocked {
		return "********", true, nil
	}

	secretRef := fmt.Sprintf("op://%s/%s/%s", s.vault.Name, ref.Item, ref.Field)
	args := []string{"item", "get", "--vault", s.vault.Name, "--fields", ref.Field, "--reveal", "--account", s.vault.URL, "--", ref.Item}
	output, err := s.runOp(fmt.Sprintf("secret %q", secretRef), args)
	if err != nil {
		return "", true, err
	}

	value := strings.TrimSpace(string(output))
	return value, true, nil
}

// Account returns the 1Password account this provider reads.
func (s *OnePasswordCLIProvider) Account() string {
	return s.vault.URL
}

// Check confirms that the account is reachable and signed in. It runs op whoami for the account.
func (s *OnePasswordCLIProvider) Check() error {
	_, err := s.runOp("the account check", []string{"whoami", "--account", s.vault.URL})
	return err
}

// =============================================================================
// Private Methods
// =============================================================================

// runOp runs op with args under the 1Password call guard and returns its stdout.
// A failure returns the stderr of op and the fix for the common causes. what names the
// lookup in the messages.
func (s *OnePasswordCLIProvider) runOp(what string, args []string) ([]byte, error) {
	var stdout, stderr []byte
	err := runOnePasswordCall(s.shims, s.vault.URL, what, func(ctx context.Context) error {
		var runErr error
		stdout, stderr, runErr = s.shims.CmdOutput(ctx, s.shims.Command("op", args...))
		return runErr
	})
	if err == nil {
		return stdout, nil
	}
	if werror.Find(err) != nil {
		return nil, err
	}
	return nil, classifyOpError(err, stderr, s.vault.URL, what)
}

// =============================================================================
// Interface Compliance
// =============================================================================

var _ Provider = (*OnePasswordCLIProvider)(nil)
