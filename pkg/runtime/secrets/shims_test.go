package secrets

import (
	"context"
	"os"
	"testing"
)

func TestNewShims(t *testing.T) {
	t.Run("InitializesAllShims", func(t *testing.T) {
		shims := NewShims()

		// Test Stat shim
		if shims.Stat == nil {
			t.Error("Expected Stat shim to be initialized")
		}

		// Test YAMLUnmarshal shim
		if shims.YAMLUnmarshal == nil {
			t.Error("Expected YAMLUnmarshal shim to be initialized")
		}

		// Test DecryptFile shim
		if shims.DecryptFile == nil {
			t.Error("Expected DecryptFile shim to be initialized")
		}

		// Test NewOnePasswordClient shim
		if shims.NewOnePasswordClient == nil {
			t.Error("Expected NewOnePasswordClient shim to be initialized")
		}

		// Test ResolveSecret shim
		if shims.ResolveSecret == nil {
			t.Error("Expected ResolveSecret shim to be initialized")
		}
	})

	t.Run("ResolveSecretHandlesNilClient", func(t *testing.T) {
		shims := NewShims()

		// Test ResolveSecret with nil client
		_, err := shims.ResolveSecret(nil, context.Background(), "test-ref")
		if err == nil {
			t.Error("Expected error when client is nil")
		}
		if err.Error() != "client is nil" {
			t.Errorf("Expected error message 'client is nil', got '%s'", err.Error())
		}
	})

	t.Run("StatUsesOsStat", func(t *testing.T) {
		shims := NewShims()

		// Test that Stat uses os.Stat
		_, err := shims.Stat("nonexistent-file")
		if !os.IsNotExist(err) {
			t.Error("Expected os.IsNotExist error for nonexistent file")
		}
	})
}
