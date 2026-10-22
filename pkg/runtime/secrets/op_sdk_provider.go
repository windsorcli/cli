// The OnePasswordSDKProvider is the secrets provider for a 1Password vault through the 1Password SDK.
// It provides one field of one item for each secret() call.
// It authenticates with OP_SERVICE_ACCOUNT_TOKEN and shares one client across providers.
// The runtime selects it when OP_SERVICE_ACCOUNT_TOKEN is set.

package secrets

import (
	"context"
	"fmt"
	"sync"

	"github.com/1password/onepassword-sdk-go"
	"github.com/windsorcli/cli/internal/werror"
)

// =============================================================================
// Constants
// =============================================================================

// globalClient is cached per process (the SDK client is expensive to construct) and keyed on
// globalClientToken, not just non-nil, so a changed OP_SERVICE_ACCOUNT_TOKEN between calls
// produces a fresh client instead of silently keeping the first vault's token for the process.
var (
	globalClient      *onepassword.Client
	globalClientToken string
	globalCtx         context.Context
	clientLock        sync.Mutex
)

// =============================================================================
// Types
// =============================================================================

// OnePasswordSDKProvider implements the Provider interface using the 1Password SDK.
type OnePasswordSDKProvider struct {
	vault    OnePasswordVault
	unlocked bool
	shims    *Shims
}

// =============================================================================
// Constructor
// =============================================================================

// NewOnePasswordSDKProvider creates a new OnePasswordSDKProvider instance.
func NewOnePasswordSDKProvider(vault OnePasswordVault) *OnePasswordSDKProvider {
	return &OnePasswordSDKProvider{
		vault: vault,
		shims: NewShims(),
	}
}

// =============================================================================
// Provider Interface
// =============================================================================

// LoadSecrets marks the provider as unlocked.
func (s *OnePasswordSDKProvider) LoadSecrets() error {
	s.unlocked = true
	return nil
}

// Resolve fetches a secret from 1Password SDK.
// Returns handled=true only if the vault ID matches.
func (s *OnePasswordSDKProvider) Resolve(ref SecretRef) (string, bool, error) {
	if ref.Vault != s.vault.ID {
		return "", false, nil
	}
	if ref.Field == "" {
		return "", true, missingOnePasswordFieldError(ref)
	}
	if !s.unlocked {
		return "********", true, nil
	}

	token := s.shims.Getenv("OP_SERVICE_ACCOUNT_TOKEN")
	if token == "" {
		return "", true, werror.New("The 1Password SDK needs the OP_SERVICE_ACCOUNT_TOKEN environment variable.",
			"Set OP_SERVICE_ACCOUNT_TOKEN to a 1Password service account token.", nil)
	}

	clientLock.Lock()
	defer clientLock.Unlock()

	if globalClient == nil || globalClientToken != token {
		globalCtx = context.Background()
		client, err := s.shims.NewOnePasswordClient(
			globalCtx,
			onepassword.WithServiceAccountToken(token),
			onepassword.WithIntegrationInfo("windsor-cli", version),
		)
		if err != nil {
			return "", true, werror.Wrap(err, "failed to create 1Password client")
		}
		if client == nil {
			return "", true, fmt.Errorf("failed to create 1Password client: client is nil")
		}
		globalClient = client
		globalClientToken = token
	}

	secretRefURI := fmt.Sprintf("op://%s/%s/%s", s.vault.Name, ref.Item, ref.Field)

	value, err := s.shims.ResolveSecret(globalClient, globalCtx, secretRefURI)
	if err != nil {
		return "", true, werror.Wrap(err, "failed to resolve secret %q", secretRefURI)
	}

	return value, true, nil
}

// =============================================================================
// Interface Compliance
// =============================================================================

var _ Provider = (*OnePasswordSDKProvider)(nil)
