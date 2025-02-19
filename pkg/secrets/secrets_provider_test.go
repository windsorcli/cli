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

	t.Run("ErrorResolvingShell", func(t *testing.T) {
		mocks := setupSafeMocks()
		mocks.Injector.Register("shell", nil)
		provider := NewBaseSecretsProvider(mocks.Injector)

		err := provider.Initialize()

		if err == nil || err.Error() != "failed to resolve shell instance from injector" {
			t.Errorf("Expected error 'failed to resolve shell instance from injector', but got: %v", err)
		}
	})

	t.Run("ErrorCastingShell", func(t *testing.T) {
		mocks := setupSafeMocks()
		mocks.Injector.Register("shell", "invalid")
		provider := NewBaseSecretsProvider(mocks.Injector)

		err := provider.Initialize()

		if err == nil || err.Error() != "resolved shell instance is not of type shell.Shell" {
			t.Errorf("Expected error 'resolved shell instance is not of type shell.Shell', but got: %v", err)
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

func TestParseKeys(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "SimpleDotNotation",
			input:    "key1.key2.key3",
			expected: []string{"key1", "key2", "key3"},
		},
		{
			name:     "BracketNotation",
			input:    "key1.[key2].key3",
			expected: []string{"key1", "key2", "key3"},
		},
		{
			name:     "MixedNotation",
			input:    "key1.[key2].key3.[key4]",
			expected: []string{"key1", "key2", "key3", "key4"},
		},
		{
			name:     "EmptyKeys",
			input:    "key1..key3",
			expected: []string{"key1", "", "key3"},
		},
		{
			name:     "LeadingAndTrailingDots",
			input:    ".key1.key2.",
			expected: []string{"", "key1", "key2", ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseKeys(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("Expected length %d, but got %d", len(tt.expected), len(result))
			}
			for i, v := range result {
				if v != tt.expected[i] {
					t.Errorf("Expected %s at index %d, but got %s", tt.expected[i], i, v)
				}
			}
		})
	}
}
