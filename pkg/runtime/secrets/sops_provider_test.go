package secrets_test

import (
	"testing"

	"github.com/windsorcli/cli/pkg/runtime/secrets"
)

// =============================================================================
// Constructor
// =============================================================================

func TestNewSopsProvider(t *testing.T) {
	p := secrets.NewSopsProvider("/config/path")
	if p == nil {
		t.Fatal("expected provider, got nil")
	}
	if p.ConfigPath() != "/config/path" {
		t.Errorf("configPath = %q, want %q", p.ConfigPath(), "/config/path")
	}
	if p.Unlocked() {
		t.Error("expected locked initially")
	}
}
