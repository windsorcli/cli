package secrets

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/1password/onepassword-sdk-go"
	secretsConfigType "github.com/windsorcli/cli/api/v1alpha1/secrets"
	"github.com/windsorcli/cli/pkg/di"
)

var (
	globalClient *onepassword.Client
	globalCtx    context.Context
	clientLock   sync.Mutex
)

// OnePasswordSDKSecretsProvider is an implementation of the SecretsProvider interface
// that uses the 1Password SDK to manage secrets.
type OnePasswordSDKSecretsProvider struct {
	*BaseSecretsProvider
	vault secretsConfigType.OnePasswordVault
}

// NewOnePasswordSDKSecretsProvider creates a new OnePasswordSDKSecretsProvider instance
func NewOnePasswordSDKSecretsProvider(vault secretsConfigType.OnePasswordVault, injector di.Injector) *OnePasswordSDKSecretsProvider {
	baseProvider := NewBaseSecretsProvider(injector)
	return &OnePasswordSDKSecretsProvider{
		BaseSecretsProvider: baseProvider,
		vault:               vault,
	}
}

// Initialize initializes the secrets provider
func (s *OnePasswordSDKSecretsProvider) Initialize() error {
	if err := s.BaseSecretsProvider.Initialize(); err != nil {
		return err
	}

	// Get the service account token from environment
	token := os.Getenv("OP_SERVICE_ACCOUNT_TOKEN")
	if token == "" {
		return fmt.Errorf("OP_SERVICE_ACCOUNT_TOKEN environment variable is required for 1Password SDK")
	}

	return nil
}

// GetSecret retrieves a secret value for the specified key. It first checks if the provider is unlocked.
// If not, it returns a masked value. It then ensures the 1Password client is initialized using a
// service account token from the environment. The key is split into item and field parts, and the
// item name is sanitized. A secret reference URI is constructed and used to resolve the secret value
// from 1Password. If successful, the secret value is returned; otherwise, an error is reported.
func (s *OnePasswordSDKSecretsProvider) GetSecret(key string) (string, error) {
	if !s.isUnlocked() {
		return "********", nil
	}

	token := os.Getenv("OP_SERVICE_ACCOUNT_TOKEN")
	if token == "" {
		return "", fmt.Errorf("OP_SERVICE_ACCOUNT_TOKEN environment variable is required for 1Password SDK")
	}

	clientLock.Lock()
	defer clientLock.Unlock()

	if globalClient == nil {
		globalCtx = context.Background()
		client, err := newOnePasswordClient(
			globalCtx,
			onepassword.WithServiceAccountToken(token),
			onepassword.WithIntegrationInfo("windsor-cli", version),
		)
		if err != nil {
			return "", fmt.Errorf("failed to create 1Password client: %w", err)
		}
		if client == nil {
			return "", fmt.Errorf("failed to create 1Password client: client is nil")
		}
		globalClient = client
	}

	parts := strings.SplitN(key, ".", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid key notation: %s. Expected format is 'secret.field'", key)
	}

	itemName := parts[0]
	fieldName := parts[1]

	// Construct the secret reference URI
	secretRef := fmt.Sprintf("op://%s/%s/%s", s.vault.Name, itemName, fieldName)

	// Resolve the secret using the SDK
	value, err := resolveSecret(globalClient, globalCtx, secretRef)
	if err != nil {
		return "", fmt.Errorf("failed to resolve secret: %w", err)
	}

	return value, nil
}

// ParseSecrets identifies and replaces ${{ op.<id>.<secret>.<field> }} patterns in the input
// with corresponding secret values from 1Password, ensuring the id matches the vault ID.
func (s *OnePasswordSDKSecretsProvider) ParseSecrets(input string) (string, error) {
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
			return fmt.Sprintf("<ERROR: failed to resolve: %s.%s: %s>", secret, field, err), true
		}
		return value, true
	})
	return result, nil
}

// Ensure OnePasswordSDKSecretsProvider implements SecretsProvider
var _ SecretsProvider = (*OnePasswordSDKSecretsProvider)(nil)
