package secrets_test

import (
	"testing"

	"github.com/windsorcli/cli/pkg/runtime/secrets"
)

// =============================================================================
// LoadSecrets
// =============================================================================

func TestOnePasswordSDKProvider_LoadSecrets(t *testing.T) {
	p := secrets.NewOnePasswordSDKProvider(secrets.OnePasswordVault{ID: "v"})
	if err := p.LoadSecrets(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !p.Unlocked() {
		t.Error("expected unlocked after LoadSecrets")
	}
}
