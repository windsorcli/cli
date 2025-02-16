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
		provider.secrets["test_key"] = "test_value"

		err := provider.LoadSecrets()

		if err != nil {
			t.Errorf("Expected LoadSecrets to succeed, but got error: %v", err)
		}

		// After loading secrets, they should be accessible
		value, err := provider.GetSecret("test_key")
		if err != nil {
			t.Errorf("Expected GetSecret to succeed, but got error: %v", err)
		}
		if value != "test_value" {
			t.Errorf("Expected GetSecret to return 'test_value', but got: %s", value)
		}
	})
}

func TestBaseSecretsProvider_GetSecret(t *testing.T) {
	t.Run("ReturnsMaskedValueWhenLocked", func(t *testing.T) {
		provider := NewBaseSecretsProvider()
		provider.secrets["test_key"] = "test_value"
		provider.unlocked = false // Simulate that secrets are locked

		value, err := provider.GetSecret("test_key")

		if err != nil {
			t.Errorf("Expected GetSecret to succeed, but got error: %v", err)
		}

		if value != "********" {
			t.Errorf("Expected GetSecret to return '********', but got: %s", value)
		}
	})

	t.Run("ReturnsActualValueWhenUnlocked", func(t *testing.T) {
		provider := NewBaseSecretsProvider()
		provider.secrets["test_key"] = "test_value"
		provider.unlocked = true // Simulate that secrets have been unlocked

		value, err := provider.GetSecret("test_key")

		if err != nil {
			t.Errorf("Expected GetSecret to succeed, but got error: %v", err)
		}

		if value != "test_value" {
			t.Errorf("Expected GetSecret to return 'test_value', but got: %s", value)
		}
	})
}

func TestBaseSecretsProvider_ParseSecrets(t *testing.T) {
	t.Run("ReplacesSecretSuccessfully", func(t *testing.T) {
		provider := NewBaseSecretsProvider()
		provider.secrets["test_key"] = "test_value"
		provider.unlocked = true // Simulate that secrets have been unlocked

		// Test with standard notation
		input1 := "This is a secret: ${{ secrets.test_key }}"
		expectedOutput1 := "This is a secret: test_value"

		output1, err := provider.ParseSecrets(input1)

		if err != nil {
			t.Fatalf("ParseSecrets failed with error: %v", err)
		}

		if output1 != expectedOutput1 {
			t.Errorf("ParseSecrets returned '%s', expected '%s'", output1, expectedOutput1)
		}

		// Test with spaces in the notation
		input2 := "This is a secret: ${{  secrets.test_key  }}"
		expectedOutput2 := "This is a secret: test_value"

		output2, err := provider.ParseSecrets(input2)

		if err != nil {
			t.Fatalf("ParseSecrets failed with error: %v", err)
		}

		if output2 != expectedOutput2 {
			t.Errorf("ParseSecrets returned '%s', expected '%s'", output2, expectedOutput2)
		}
	})

	t.Run("ReturnsErrorWhenSecretNotFound", func(t *testing.T) {
		provider := NewBaseSecretsProvider()
		provider.unlocked = true // Simulate that secrets have been unlocked

		// Test with standard notation
		input1 := "This is a secret: ${{ secrets.non_existent_key }}"
		expectedOutput1 := "This is a secret: <ERROR: secret not found: non_existent_key>"

		output1, err := provider.ParseSecrets(input1)

		if err != nil {
			t.Fatalf("ParseSecrets failed with error: %v", err)
		}

		if output1 != expectedOutput1 {
			t.Errorf("ParseSecrets returned '%s', expected '%s'", output1, expectedOutput1)
		}

		// Test with spaces in the notation
		input2 := "This is a secret: ${{  secrets.non_existent_key  }}"
		expectedOutput2 := "This is a secret: <ERROR: secret not found: non_existent_key>"

		output2, err := provider.ParseSecrets(input2)

		if err != nil {
			t.Fatalf("ParseSecrets failed with error: %v", err)
		}

		if output2 != expectedOutput2 {
			t.Errorf("ParseSecrets returned '%s', expected '%s'", output2, expectedOutput2)
		}
	})
}
