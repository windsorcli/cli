package secrets

import (
	"testing"
)

func TestBaseSecretsProvider_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		provider := NewBaseSecretsProvider()

		err := provider.Initialize()

		if err != nil {
			t.Errorf("Expected Initialize to succeed, but got error: %v", err)
		}
	})
}

func TestBaseSecretsProvider_LoadSecrets(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		provider := NewBaseSecretsProvider()

		err := provider.LoadSecrets()

		if err != nil {
			t.Errorf("Expected LoadSecrets to succeed, but got error: %v", err)
		}
	})
}

func TestBaseSecretsProvider_GetSecret(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		provider := NewBaseSecretsProvider()
		provider.secrets["test_key"] = "test_value"

		value, err := provider.GetSecret("test_key")

		if err != nil {
			t.Errorf("Expected GetSecret to succeed, but got error: %v", err)
		}

		if value != "test_value" {
			t.Errorf("Expected GetSecret to return 'test_value', but got: %s", value)
		}
	})

	t.Run("SecretNotFound", func(t *testing.T) {
		provider := NewBaseSecretsProvider()

		_, err := provider.GetSecret("non_existent_key")

		if err == nil {
			t.Errorf("Expected GetSecret to fail, but it succeeded")
		}
	})
}
