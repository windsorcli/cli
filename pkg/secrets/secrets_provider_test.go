package secrets

import (
	"testing"
)

func TestBaseSecretsProvider_Initialize(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupSafeMocks()
		provider := NewBaseSecretsProvider(mocks.Injector)

		err := provider.Initialize()

		if err != nil {
			t.Errorf("Expected Initialize to succeed, but got error: %v", err)
		}
	})
}

func TestBaseSecretsProvider_LoadSecrets(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupSafeMocks()
		provider := NewBaseSecretsProvider(mocks.Injector)

		err := provider.LoadSecrets()

		if err != nil {
			t.Errorf("Expected LoadSecrets to succeed, but got error: %v", err)
		}

		if !provider.unlocked {
			t.Errorf("Expected provider to be unlocked after LoadSecrets, but it was not")
		}
	})
}

func TestBaseSecretsProvider_GetSecret(t *testing.T) {
	t.Run("SecretNotFound", func(t *testing.T) {
		mocks := setupSafeMocks()
		provider := NewBaseSecretsProvider(mocks.Injector)

		value, err := provider.GetSecret("non_existent_key")
		if err != nil {
			t.Errorf("Expected GetSecret to not return an error, but got: %v", err)
		}
		if value != "" {
			t.Errorf("Expected GetSecret to return an empty string for non-existent key, but got: %s", value)
		}
	})
}

func TestBaseSecretsProvider_ParseSecrets(t *testing.T) {
	t.Run("NoSecretsToParse", func(t *testing.T) {
		mocks := setupSafeMocks()
		provider := NewBaseSecretsProvider(mocks.Injector)
		input := "This is a test string with no secrets."
		expectedOutput := "This is a test string with no secrets."

		output, err := provider.ParseSecrets(input)
		if err != nil {
			t.Errorf("Expected ParseSecrets to not return an error, but got: %v", err)
		}
		if output != expectedOutput {
			t.Errorf("Expected ParseSecrets to return '%s', but got: '%s'", expectedOutput, output)
		}
	})
}
