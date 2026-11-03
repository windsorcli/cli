package secrets_test

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/runtime/secrets"
)

// =============================================================================
// LoadSecrets
// =============================================================================

func TestOnePasswordCLIProvider_LoadSecrets(t *testing.T) {
	p := secrets.NewOnePasswordCLIProvider(secrets.OnePasswordVault{ID: "v"})
	if err := p.LoadSecrets(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !p.Unlocked() {
		t.Error("expected unlocked after LoadSecrets")
	}
}

// =============================================================================
// Resolve
// =============================================================================

func TestOnePasswordCLIProvider_Resolve(t *testing.T) {
	vault := secrets.OnePasswordVault{ID: "my-vault", Name: "My Vault", URL: "my.1p.com"}

	t.Run("ReturnsUnhandledForDifferentVault", func(t *testing.T) {
		p := secrets.NewOnePasswordCLIProvider(vault)
		_, handled, err := p.Resolve(secrets.SecretRef{Vault: "other-vault", Item: "item", Field: "field"})
		if err != nil || handled {
			t.Errorf("expected unhandled/nil, got handled=%v err=%v", handled, err)
		}
	})

	t.Run("ReturnsMaskedWhenLocked", func(t *testing.T) {
		p := secrets.NewOnePasswordCLIProvider(vault)
		// not unlocked
		value, handled, err := p.Resolve(secrets.SecretRef{Vault: "my-vault", Item: "item", Field: "password"})
		if err != nil || !handled || value != "********" {
			t.Errorf("expected (********, true, nil), got (%q, %v, %v)", value, handled, err)
		}
	})

	t.Run("ReturnsErrorWhenFieldEmpty", func(t *testing.T) {
		p := secrets.NewOnePasswordCLIProvider(vault)
		p.SetUnlocked(true)
		_, handled, err := p.Resolve(secrets.SecretRef{Vault: "my-vault", Item: "item", Field: ""})
		if !handled || err == nil {
			t.Errorf("expected handled=true and error for empty field, got handled=%v err=%v", handled, err)
		}
	})

	t.Run("ResolvesSuccessfully", func(t *testing.T) {
		p := secrets.NewOnePasswordCLIProvider(vault)
		p.SetUnlocked(true)

		p.Shims().Command = func(name string, args ...string) *exec.Cmd {
			return &exec.Cmd{}
		}
		p.Shims().CmdOutput = func(cmd *exec.Cmd) ([]byte, error) {
			return []byte("supersecret"), nil
		}

		value, handled, err := p.Resolve(secrets.SecretRef{Vault: "my-vault", Item: "myitem", Field: "password"})
		if err != nil || !handled || value != "supersecret" {
			t.Errorf("expected (supersecret, true, nil), got (%q, %v, %v)", value, handled, err)
		}
	})

	t.Run("PropagatesCommandError", func(t *testing.T) {
		p := secrets.NewOnePasswordCLIProvider(vault)
		p.SetUnlocked(true)

		p.Shims().Command = func(name string, args ...string) *exec.Cmd {
			return &exec.Cmd{}
		}
		p.Shims().CmdOutput = func(cmd *exec.Cmd) ([]byte, error) {
			return nil, fmt.Errorf("op command failed")
		}

		_, handled, err := p.Resolve(secrets.SecretRef{Vault: "my-vault", Item: "item", Field: "password"})
		if !handled || err == nil || !strings.Contains(err.Error(), "op command failed") {
			t.Errorf("expected handled error, got handled=%v err=%v", handled, err)
		}
		// The error names the secret reference that failed so the operator knows
		// which vault/item/field to check rather than only that some secret failed.
		if err == nil || !strings.Contains(err.Error(), "op://My Vault/item/password") {
			t.Errorf("expected error to name the secret reference, got: %v", err)
		}
	})

	t.Run("PlacesItemAfterEndOfOptionsGuard", func(t *testing.T) {
		// A config-controlled item name starting with "-" must be parsed as the operand op
		// expects, not as an op flag (argument injection).
		p := secrets.NewOnePasswordCLIProvider(vault)
		p.SetUnlocked(true)

		var capturedArgs []string
		p.Shims().Command = func(name string, args ...string) *exec.Cmd {
			capturedArgs = args
			return &exec.Cmd{}
		}
		p.Shims().CmdOutput = func(cmd *exec.Cmd) ([]byte, error) {
			return []byte("value"), nil
		}

		if _, _, err := p.Resolve(secrets.SecretRef{Vault: "my-vault", Item: "--reveal", Field: "field"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(capturedArgs) < 2 || capturedArgs[len(capturedArgs)-2] != "--" || capturedArgs[len(capturedArgs)-1] != "--reveal" {
			t.Errorf("expected item as the last arg preceded by '--', got %v", capturedArgs)
		}
	})

	t.Run("TrimsWhitespaceFromOutput", func(t *testing.T) {
		p := secrets.NewOnePasswordCLIProvider(vault)
		p.SetUnlocked(true)

		p.Shims().Command = func(name string, args ...string) *exec.Cmd { return &exec.Cmd{} }
		p.Shims().CmdOutput = func(cmd *exec.Cmd) ([]byte, error) {
			return []byte("  secret-value  \n"), nil
		}

		value, _, err := p.Resolve(secrets.SecretRef{Vault: "my-vault", Item: "item", Field: "field"})
		if err != nil || value != "secret-value" {
			t.Errorf("expected trimmed value, got %q err=%v", value, err)
		}
	})

	t.Run("InterfaceCompliance", func(t *testing.T) {
		var _ secrets.Provider = secrets.NewOnePasswordCLIProvider(vault)
	})
}
