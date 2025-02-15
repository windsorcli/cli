package secrets

import (
	"testing"
)

func TestBaseSecretsHandler_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		handler := NewBaseSecretsHandler()

		err := handler.Initialize()

		if err != nil {
			t.Errorf("Expected Initialize to succeed, but got error: %v", err)
		}
	})
}

func TestBaseSecretsHandler_LoadSecrets(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		handler := NewBaseSecretsHandler()

		err := handler.LoadSecrets()

		if err != nil {
			t.Errorf("Expected LoadSecrets to succeed, but got error: %v", err)
		}
	})
}

func TestBaseSecretsHandler_GetSecret(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		handler := NewBaseSecretsHandler()
		handler.secrets["test_key"] = "test_value"

		value, err := handler.GetSecret("test_key")

		if err != nil {
			t.Errorf("Expected GetSecret to succeed, but got error: %v", err)
		}

		if value != "test_value" {
			t.Errorf("Expected GetSecret to return 'test_value', but got: %s", value)
		}
	})

	t.Run("SecretNotFound", func(t *testing.T) {
		handler := NewBaseSecretsHandler()

		_, err := handler.GetSecret("non_existent_key")

		if err == nil {
			t.Errorf("Expected GetSecret to fail, but it succeeded")
		}
	})
}
