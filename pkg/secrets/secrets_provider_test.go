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
		err := provider.Unlock()
		if err != nil {
			t.Fatalf("Expected Unlock to succeed, but got error: %v", err)
		}

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
		err := provider.Unlock()
		if err != nil {
			t.Fatalf("Expected Unlock to succeed, but got error: %v", err)
		}

		_, err = provider.GetSecret("non_existent_key")

		if err == nil {
			t.Errorf("Expected GetSecret to fail, but it succeeded")
		}
	})
}

func TestBaseSecretsProvider_ParseSecrets(t *testing.T) {
	t.Run("ReplacesSecretSuccessfully", func(t *testing.T) {
		provider := NewBaseSecretsProvider()
		provider.secrets["test_key"] = "test_value"
		err := provider.Unlock()
		if err != nil {
			t.Fatalf("Expected Unlock to succeed, but got error: %v", err)
		}

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

	t.Run("ReturnsInputWhenSecretNotFound", func(t *testing.T) {
		provider := NewBaseSecretsProvider()
		err := provider.Unlock()
		if err != nil {
			t.Fatalf("Expected Unlock to succeed, but got error: %v", err)
		}

		// Test with standard notation
		input1 := "This is a secret: ${{ secrets.non_existent_key }}"
		expectedOutput1 := "This is a secret: ${{ secrets.non_existent_key }}"

		output1, err := provider.ParseSecrets(input1)

		if err != nil {
			t.Fatalf("ParseSecrets failed with error: %v", err)
		}

		if output1 != expectedOutput1 {
			t.Errorf("ParseSecrets returned '%s', expected '%s'", output1, expectedOutput1)
		}

		// Test with spaces in the notation
		input2 := "This is a secret: ${{  secrets.non_existent_key  }}"
		expectedOutput2 := "This is a secret: ${{  secrets.non_existent_key  }}"

		output2, err := provider.ParseSecrets(input2)

		if err != nil {
			t.Fatalf("ParseSecrets failed with error: %v", err)
		}

		if output2 != expectedOutput2 {
			t.Errorf("ParseSecrets returned '%s', expected '%s'", output2, expectedOutput2)
		}
	})
}

func TestBaseSecretsProvider_Unlock(t *testing.T) {
	t.Run("UnlocksSuccessfully", func(t *testing.T) {
		provider := NewBaseSecretsProvider()
		provider.secrets["test_key"] = "test_value"

		// Initially, secrets should be masked
		value, err := provider.GetSecret("test_key")
		if err != nil {
			t.Errorf("Expected GetSecret to succeed, but got error: %v", err)
		}
		if value != "********" {
			t.Errorf("Expected GetSecret to return masked value, but got: %s", value)
		}

		// Unlock the provider
		err = provider.Unlock()
		if err != nil {
			t.Fatalf("Expected Unlock to succeed, but got error: %v", err)
		}

		// Now, secrets should be accessible
		value, err = provider.GetSecret("test_key")
		if err != nil {
			t.Errorf("Expected GetSecret to succeed, but got error: %v", err)
		}
		if value != "test_value" {
			t.Errorf("Expected GetSecret to return 'test_value', but got: %s", value)
		}
	})
}
