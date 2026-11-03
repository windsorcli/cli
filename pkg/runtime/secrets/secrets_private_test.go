package secrets

import (
	"testing"
)

// =============================================================================
// Test Private Methods
// =============================================================================

func TestIsSecretExpression(t *testing.T) {
	trueInputs := []string{
		"secret.op.vault.item.field",
		"secrets.sops.key",
		`secret("v", "i", "f")`,
		"  secret.op.vault.item.field  ",
	}
	falseInputs := []string{
		"",
		"op.vault.item.field",
		"sops.key",
		"SOME_ENV_VAR",
		"plain string",
	}

	for _, input := range trueInputs {
		if !isSecretExpression(input) {
			t.Errorf("isSecretExpression(%q) = false, want true", input)
		}
	}
	for _, input := range falseInputs {
		if isSecretExpression(input) {
			t.Errorf("isSecretExpression(%q) = true, want false", input)
		}
	}
}

func TestParseKeys(t *testing.T) {
	f := func(input string, expected []string) {
		t.Helper()
		keys := parseKeys(input)
		if len(keys) != len(expected) {
			t.Errorf("parseKeys(%q): got %d keys %v, want %d keys %v", input, len(keys), keys, len(expected), expected)
			return
		}
		for i := range expected {
			if keys[i] != expected[i] {
				t.Errorf("parseKeys(%q)[%d]: got %q, want %q", input, i, keys[i], expected[i])
			}
		}
	}

	// Simple dot notation
	f("key1", []string{"key1"})
	f("key1.key2", []string{"key1", "key2"})
	f("key1.key2.key3", []string{"key1", "key2", "key3"})

	// Bracket notation
	f("key1.[key2]", []string{"key1", "key2"})
	f("[key1]", []string{"key1"})
	f("[key1].[key2]", []string{"key1", "key2"})

	// Quoted keys in brackets (quotes stripped)
	f(`key1.["key2"].key3`, []string{"key1", "key2", "key3"})
	f(`key1.['key2'].key3`, []string{"key1", "key2", "key3"})

	// Special characters inside quotes
	f(`key1.["key@2"].key3`, []string{"key1", "key@2", "key3"})

	// Nested brackets
	f("key1.[key2.[key3]].key4", []string{"key1", "key2.[key3]", "key4"})

	// Empty keys
	f("key1..key3", []string{"key1", "", "key3"})
	f("key1.[].key3", []string{"key1", "", "key3"})

	// Trailing dot after bracket — no extra empty key
	f("key1.[key2].", []string{"key1", "key2", ""})

	// Edge: empty string produces one empty key
	f("", []string{""})

	// Multiple dots
	f(".", []string{"", ""})
}
