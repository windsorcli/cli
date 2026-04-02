package secrets

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/1password/onepassword-sdk-go"
	secretsConfigType "github.com/windsorcli/cli/api/v1alpha1/secrets"
)

// =============================================================================
// Constants
// =============================================================================

var (
	globalClient *onepassword.Client
	globalCtx    context.Context
	clientLock   sync.Mutex
)

// =============================================================================
// Types
// =============================================================================

// OnePasswordSDKProvider implements the Provider interface using the 1Password SDK.
type OnePasswordSDKProvider struct {
	vault    secretsConfigType.OnePasswordVault
	unlocked bool
	shims    *Shims
}

// =============================================================================
// Constructor
// =============================================================================

// NewOnePasswordSDKProvider creates a new OnePasswordSDKProvider instance.
func NewOnePasswordSDKProvider(vault secretsConfigType.OnePasswordVault) *OnePasswordSDKProvider {
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
		return "", true, fmt.Errorf("secret() field is required for 1Password provider")
	}
	if !s.unlocked {
		return "********", true, nil
	}

	token := os.Getenv("OP_SERVICE_ACCOUNT_TOKEN")
	if token == "" {
		return "", true, fmt.Errorf("OP_SERVICE_ACCOUNT_TOKEN environment variable is required for 1Password SDK")
	}

	clientLock.Lock()
	defer clientLock.Unlock()

	if globalClient == nil {
		globalCtx = context.Background()
		client, err := s.shims.NewOnePasswordClient(
			globalCtx,
			onepassword.WithServiceAccountToken(token),
			onepassword.WithIntegrationInfo("windsor-cli", version),
		)
		if err != nil {
			return "", true, fmt.Errorf("failed to create 1Password client: %w", err)
		}
		if client == nil {
			return "", true, fmt.Errorf("failed to create 1Password client: client is nil")
		}
		globalClient = client
	}

	secretRefURI := fmt.Sprintf("op://%s/%s/%s", s.vault.Name, ref.Item, ref.Field)

	value, err := s.shims.ResolveSecret(globalClient, globalCtx, secretRefURI)
	if err != nil {
		return "", true, fmt.Errorf("failed to resolve secret: %w", err)
	}

	return value, true, nil
}

// =============================================================================
// Interface Compliance
// =============================================================================

var _ Provider = (*OnePasswordSDKProvider)(nil)
