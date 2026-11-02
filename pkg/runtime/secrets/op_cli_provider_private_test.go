package secrets

import (
	"testing"
)

// =============================================================================
// Constructor
// =============================================================================

func TestNewOnePasswordCLIProvider(t *testing.T) {
	vault := OnePasswordVault{ID: "vault-id", Name: "My Vault", URL: "my.1p.com"}
	p := NewOnePasswordCLIProvider(vault)
	if p == nil {
		t.Fatal("expected provider, got nil")
	}
	if p.vault.ID != "vault-id" {
		t.Errorf("vault.ID = %q, want %q", p.vault.ID, "vault-id")
	}
	if p.unlocked {
		t.Error("expected locked initially")
	}
}
