package secrets

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/1password/onepassword-sdk-go"
	secretsConfigType "github.com/windsorcli/cli/api/v1alpha1/secrets"
)

// resetGlobalClient resets the SDK singleton for test isolation.
func resetGlobalClient() {
	clientLock.Lock()
	defer clientLock.Unlock()
	globalClient = nil
	globalCtx = nil
}

// =============================================================================
// Constructor
// =============================================================================

func TestNewOnePasswordSDKProvider(t *testing.T) {
	vault := secretsConfigType.OnePasswordVault{ID: "sdk-vault", Name: "SDK Vault"}
	p := NewOnePasswordSDKProvider(vault)
	if p == nil {
		t.Fatal("expected provider, got nil")
	}
	if p.vault.ID != "sdk-vault" {
		t.Errorf("vault.ID = %q, want %q", p.vault.ID, "sdk-vault")
	}
	if p.unlocked {
		t.Error("expected locked initially")
	}
}

// =============================================================================
// LoadSecrets
// =============================================================================

func TestOnePasswordSDKProvider_LoadSecrets(t *testing.T) {
	p := NewOnePasswordSDKProvider(secretsConfigType.OnePasswordVault{ID: "v"})
	if err := p.LoadSecrets(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !p.unlocked {
		t.Error("expected unlocked after LoadSecrets")
	}
}

// =============================================================================
// Resolve
// =============================================================================

func TestOnePasswordSDKProvider_Resolve(t *testing.T) {
	vault := secretsConfigType.OnePasswordVault{ID: "sdk-vault", Name: "SDK Vault"}

	t.Run("ReturnsUnhandledForDifferentVault", func(t *testing.T) {
		p := NewOnePasswordSDKProvider(vault)
		_, handled, err := p.Resolve(SecretRef{Vault: "other", Item: "item", Field: "field"})
		if err != nil || handled {
			t.Errorf("expected unhandled/nil, got handled=%v err=%v", handled, err)
		}
	})

	t.Run("ReturnsMaskedWhenLocked", func(t *testing.T) {
		p := NewOnePasswordSDKProvider(vault)
		value, handled, err := p.Resolve(SecretRef{Vault: "sdk-vault", Item: "item", Field: "password"})
		if err != nil || !handled || value != "********" {
			t.Errorf("expected (********, true, nil), got (%q, %v, %v)", value, handled, err)
		}
	})

	t.Run("ReturnsErrorWhenFieldEmpty", func(t *testing.T) {
		p := NewOnePasswordSDKProvider(vault)
		p.unlocked = true
		_, handled, err := p.Resolve(SecretRef{Vault: "sdk-vault", Item: "item", Field: ""})
		if !handled || err == nil {
			t.Errorf("expected handled=true and error, got handled=%v err=%v", handled, err)
		}
	})

	t.Run("ReturnsErrorWhenTokenMissing", func(t *testing.T) {
		t.Setenv("OP_SERVICE_ACCOUNT_TOKEN", "")
		p := NewOnePasswordSDKProvider(vault)
		p.unlocked = true

		_, handled, err := p.Resolve(SecretRef{Vault: "sdk-vault", Item: "item", Field: "field"})
		if !handled || err == nil || !strings.Contains(err.Error(), "OP_SERVICE_ACCOUNT_TOKEN") {
			t.Errorf("expected token error, got handled=%v err=%v", handled, err)
		}
	})

	t.Run("ResolvesSuccessfully", func(t *testing.T) {
		t.Setenv("OP_SERVICE_ACCOUNT_TOKEN", "test-token")
		resetGlobalClient()
		t.Cleanup(resetGlobalClient)

		p := NewOnePasswordSDKProvider(vault)
		p.unlocked = true

		mockClient := &onepassword.Client{}
		p.shims.NewOnePasswordClient = func(ctx context.Context, opts ...onepassword.ClientOption) (*onepassword.Client, error) {
			return mockClient, nil
		}
		p.shims.ResolveSecret = func(client *onepassword.Client, ctx context.Context, ref string) (string, error) {
			if strings.Contains(ref, "SDK Vault") && strings.Contains(ref, "myitem") {
				return "sdk-secret-value", nil
			}
			return "", fmt.Errorf("unexpected ref: %s", ref)
		}

		value, handled, err := p.Resolve(SecretRef{Vault: "sdk-vault", Item: "myitem", Field: "password"})
		if err != nil || !handled || value != "sdk-secret-value" {
			t.Errorf("expected (sdk-secret-value, true, nil), got (%q, %v, %v)", value, handled, err)
		}
	})

	t.Run("ReturnsErrorWhenClientCreationFails", func(t *testing.T) {
		t.Setenv("OP_SERVICE_ACCOUNT_TOKEN", "test-token")
		resetGlobalClient()
		t.Cleanup(resetGlobalClient)

		p := NewOnePasswordSDKProvider(vault)
		p.unlocked = true

		p.shims.NewOnePasswordClient = func(ctx context.Context, opts ...onepassword.ClientOption) (*onepassword.Client, error) {
			return nil, fmt.Errorf("client init failed")
		}

		_, handled, err := p.Resolve(SecretRef{Vault: "sdk-vault", Item: "item", Field: "field"})
		if !handled || err == nil || !strings.Contains(err.Error(), "client init failed") {
			t.Errorf("expected client error, got handled=%v err=%v", handled, err)
		}
	})

	t.Run("ReturnsErrorWhenClientIsNil", func(t *testing.T) {
		t.Setenv("OP_SERVICE_ACCOUNT_TOKEN", "test-token")
		resetGlobalClient()
		t.Cleanup(resetGlobalClient)

		p := NewOnePasswordSDKProvider(vault)
		p.unlocked = true

		p.shims.NewOnePasswordClient = func(ctx context.Context, opts ...onepassword.ClientOption) (*onepassword.Client, error) {
			return nil, nil // nil client, nil error
		}

		_, handled, err := p.Resolve(SecretRef{Vault: "sdk-vault", Item: "item", Field: "field"})
		if !handled || err == nil {
			t.Errorf("expected error for nil client, got handled=%v err=%v", handled, err)
		}
	})

	t.Run("PropagatesResolveError", func(t *testing.T) {
		t.Setenv("OP_SERVICE_ACCOUNT_TOKEN", "test-token")
		resetGlobalClient()
		t.Cleanup(resetGlobalClient)

		p := NewOnePasswordSDKProvider(vault)
		p.unlocked = true

		mockClient := &onepassword.Client{}
		p.shims.NewOnePasswordClient = func(ctx context.Context, opts ...onepassword.ClientOption) (*onepassword.Client, error) {
			return mockClient, nil
		}
		p.shims.ResolveSecret = func(_ *onepassword.Client, _ context.Context, _ string) (string, error) {
			return "", fmt.Errorf("resolve failed")
		}

		_, handled, err := p.Resolve(SecretRef{Vault: "sdk-vault", Item: "item", Field: "field"})
		if !handled || err == nil || !strings.Contains(err.Error(), "resolve failed") {
			t.Errorf("expected resolve error, got handled=%v err=%v", handled, err)
		}
	})

	t.Run("ReusesGlobalClientOnSecondCall", func(t *testing.T) {
		t.Setenv("OP_SERVICE_ACCOUNT_TOKEN", "test-token")
		resetGlobalClient()
		t.Cleanup(resetGlobalClient)

		p := NewOnePasswordSDKProvider(vault)
		p.unlocked = true

		clientCreations := 0
		mockClient := &onepassword.Client{}
		p.shims.NewOnePasswordClient = func(ctx context.Context, opts ...onepassword.ClientOption) (*onepassword.Client, error) {
			clientCreations++
			return mockClient, nil
		}
		p.shims.ResolveSecret = func(_ *onepassword.Client, _ context.Context, _ string) (string, error) {
			return "val", nil
		}

		_, _, _ = p.Resolve(SecretRef{Vault: "sdk-vault", Item: "item", Field: "f"})
		_, _, _ = p.Resolve(SecretRef{Vault: "sdk-vault", Item: "item", Field: "f"})

		if clientCreations != 1 {
			t.Errorf("expected 1 client creation, got %d", clientCreations)
		}
	})

	t.Run("InterfaceCompliance", func(t *testing.T) {
		var _ Provider = NewOnePasswordSDKProvider(vault)
	})
}
