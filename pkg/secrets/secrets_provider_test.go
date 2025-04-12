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

type mockSecretsProvider struct {
	*BaseSecretsProvider
}

func (m *mockSecretsProvider) GetSecret(key string) (string, error) {
	return "", nil
}

func TestBaseSecretsProvider_GetSecret(t *testing.T) {
	t.Run("SecretNotFound", func(t *testing.T) {
		mocks := setupSafeMocks()
		baseProvider := NewBaseSecretsProvider(mocks.Injector)
		provider := &mockSecretsProvider{BaseSecretsProvider: baseProvider}

		value, err := provider.GetSecret("non_existent_key")
		if err != nil {
			t.Errorf("Expected GetSecret to not return an error, but got: %v", err)
		}
		if value != "" {
			t.Errorf("Expected GetSecret to return an empty string for non-existent key, but got: %s", value)
		}
	})

	t.Run("PanicsWhenNotImplemented", func(t *testing.T) {
		mocks := setupSafeMocks()
		provider := NewBaseSecretsProvider(mocks.Injector)

		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected GetSecret to panic, but it did not")
			} else if r != "GetSecret must be implemented by concrete provider" {
				t.Errorf("Expected panic message 'GetSecret must be implemented by concrete provider', got '%v'", r)
			}
		}()

		provider.GetSecret("test")
	})
}

func TestBaseSecretsProvider_ParseSecrets(t *testing.T) {
	t.Run("PanicsWhenNotImplemented", func(t *testing.T) {
		mocks := setupSafeMocks()
		provider := NewBaseSecretsProvider(mocks.Injector)

		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected ParseSecrets to panic, but it did not")
			} else if r != "ParseSecrets must be implemented by concrete provider" {
				t.Errorf("Expected panic message 'ParseSecrets must be implemented by concrete provider', got '%v'", r)
			}
		}()

		provider.ParseSecrets("test")
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
		{
			name:     "SingleBracketKey",
			input:    "[key1]",
			expected: []string{"key1"},
		},
		{
			name:     "NestedBrackets",
			input:    "key1.[key2.[key3]]",
			expected: []string{"key1", "key2.[key3]"},
		},
		{
			name:     "QuotedKeys",
			input:    "key1.[\"key2\"].key3",
			expected: []string{"key1", "key2", "key3"},
		},
		{
			name:     "ConsecutiveDots",
			input:    "key1...key2",
			expected: []string{"key1", "", "", "key2"},
		},
		{
			name:     "EmptyString",
			input:    "",
			expected: []string{""},
		},
		{
			name:     "OnlyDots",
			input:    "...",
			expected: []string{"", "", "", ""},
		},
		{
			name:     "OnlyBrackets",
			input:    "[]",
			expected: []string{""},
		},
		{
			name:     "ComplexNestedBrackets",
			input:    "key1.[key2.[key3.[key4]]]",
			expected: []string{"key1", "key2.[key3.[key4]]"},
		},
		{
			name:     "SpacesInBracketKeys",
			input:    "op.personal[\"The Criterion Channel\"].password",
			expected: []string{"op", "personal", "The Criterion Channel", "password"},
		},
		{
			name:     "EscapedSpacesInBracketKeys",
			input:    "op.personal[\"The\\ Criterion\\ Channel\"].password",
			expected: []string{"op", "personal", "The Criterion Channel", "password"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseKeys(tt.input)
			if result == nil {
				t.Errorf("ParseKeys returned nil for input %s", tt.input)
				return
			}
			if len(result) != len(tt.expected) {
				t.Errorf("Expected length %d, but got %d", len(tt.expected), len(result))
				return
			}
			for i := range tt.expected {
				if i >= len(result) {
					t.Errorf("Expected %s at index %d, but result is shorter", tt.expected[i], i)
					continue
				}
				if result[i] != tt.expected[i] {
					t.Errorf("Expected %s at index %d, but got %s", tt.expected[i], i, result[i])
				}
			}
		})
	}
}
