package secrets

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"

	secretsConfigType "github.com/windsorcli/cli/api/v1alpha1/secrets"
)

// =============================================================================
// Constructor
// =============================================================================

func TestNewOnePasswordCLIProvider(t *testing.T) {
	vault := secretsConfigType.OnePasswordVault{ID: "vault-id", Name: "My Vault", URL: "my.1p.com"}
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

// =============================================================================
// LoadSecrets
// =============================================================================

func TestOnePasswordCLIProvider_LoadSecrets(t *testing.T) {
	p := NewOnePasswordCLIProvider(secretsConfigType.OnePasswordVault{ID: "v"})
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

func TestOnePasswordCLIProvider_Resolve(t *testing.T) {
	vault := secretsConfigType.OnePasswordVault{ID: "my-vault", Name: "My Vault", URL: "my.1p.com"}

	t.Run("ReturnsUnhandledForDifferentVault", func(t *testing.T) {
		p := NewOnePasswordCLIProvider(vault)
		_, handled, err := p.Resolve(SecretRef{Vault: "other-vault", Item: "item", Field: "field"})
		if err != nil || handled {
			t.Errorf("expected unhandled/nil, got handled=%v err=%v", handled, err)
		}
	})

	t.Run("ReturnsMaskedWhenLocked", func(t *testing.T) {
		p := NewOnePasswordCLIProvider(vault)
		// not unlocked
		value, handled, err := p.Resolve(SecretRef{Vault: "my-vault", Item: "item", Field: "password"})
		if err != nil || !handled || value != "********" {
			t.Errorf("expected (********, true, nil), got (%q, %v, %v)", value, handled, err)
		}
	})

	t.Run("ReturnsErrorWhenFieldEmpty", func(t *testing.T) {
		p := NewOnePasswordCLIProvider(vault)
		p.unlocked = true
		_, handled, err := p.Resolve(SecretRef{Vault: "my-vault", Item: "item", Field: ""})
		if !handled || err == nil {
			t.Errorf("expected handled=true and error for empty field, got handled=%v err=%v", handled, err)
		}
	})

	t.Run("ResolvesSuccessfully", func(t *testing.T) {
		p := NewOnePasswordCLIProvider(vault)
		p.unlocked = true

		p.shims.Command = func(name string, args ...string) *exec.Cmd {
			return &exec.Cmd{}
		}
		p.shims.CmdOutput = func(cmd *exec.Cmd) ([]byte, error) {
			return []byte("supersecret"), nil
		}

		value, handled, err := p.Resolve(SecretRef{Vault: "my-vault", Item: "myitem", Field: "password"})
		if err != nil || !handled || value != "supersecret" {
			t.Errorf("expected (supersecret, true, nil), got (%q, %v, %v)", value, handled, err)
		}
	})

	t.Run("PropagatesCommandError", func(t *testing.T) {
		p := NewOnePasswordCLIProvider(vault)
		p.unlocked = true

		p.shims.Command = func(name string, args ...string) *exec.Cmd {
			return &exec.Cmd{}
		}
		p.shims.CmdOutput = func(cmd *exec.Cmd) ([]byte, error) {
			return nil, fmt.Errorf("op command failed")
		}

		_, handled, err := p.Resolve(SecretRef{Vault: "my-vault", Item: "item", Field: "password"})
		if !handled || err == nil || !strings.Contains(err.Error(), "op command failed") {
			t.Errorf("expected handled error, got handled=%v err=%v", handled, err)
		}
	})

	t.Run("TrimsWhitespaceFromOutput", func(t *testing.T) {
		p := NewOnePasswordCLIProvider(vault)
		p.unlocked = true

		p.shims.Command = func(name string, args ...string) *exec.Cmd { return &exec.Cmd{} }
		p.shims.CmdOutput = func(cmd *exec.Cmd) ([]byte, error) {
			return []byte("  secret-value  \n"), nil
		}

		value, _, err := p.Resolve(SecretRef{Vault: "my-vault", Item: "item", Field: "field"})
		if err != nil || value != "secret-value" {
			t.Errorf("expected trimmed value, got %q err=%v", value, err)
		}
	})

	t.Run("InterfaceCompliance", func(t *testing.T) {
		var _ Provider = NewOnePasswordCLIProvider(vault)
	})
}
