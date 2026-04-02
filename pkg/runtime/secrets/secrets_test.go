package secrets

import (
	"fmt"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// =============================================================================
// Test Setup
// =============================================================================

func setupSecretsTestMocks(t *testing.T) *shell.MockShell {
	t.Helper()
	return shell.NewMockShell()
}

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewResolver(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		sh := setupSecretsTestMocks(t)
		r := NewResolver([]Provider{}, sh)
		if r == nil {
			t.Fatal("expected resolver, got nil")
		}
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestDeferredError(t *testing.T) {
	t.Run("ErrorWithMessage", func(t *testing.T) {
		err := &DeferredError{Expression: "secret(\"v\",\"i\",\"f\")", Message: "custom message"}
		if err.Error() != "custom message" {
			t.Errorf("expected %q, got %q", "custom message", err.Error())
		}
	})

	t.Run("ErrorWithoutMessage", func(t *testing.T) {
		err := &DeferredError{Expression: "secret(\"v\",\"i\",\"f\")"}
		want := "deferred expression: secret(\"v\",\"i\",\"f\")"
		if err.Error() != want {
			t.Errorf("expected %q, got %q", want, err.Error())
		}
	})
}

func TestResolver_Resolve(t *testing.T) {
	t.Run("DispatchesToMatchingProvider", func(t *testing.T) {
		sh := setupSecretsTestMocks(t)
		var registered string
		sh.RegisterSecretFunc = func(secret string) {
			registered = secret
		}

		p := &MockProvider{
			ResolveFunc: func(ref SecretRef) (string, bool, error) {
				if ref.Vault == "sops" {
					return "myvalue", true, nil
				}
				return "", false, nil
			},
		}
		r := NewResolver([]Provider{p}, sh)

		value, err := r.Resolve(SecretRef{Vault: "sops", Item: "key"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if value != "myvalue" {
			t.Errorf("expected %q, got %q", "myvalue", value)
		}
		if registered != "myvalue" {
			t.Error("expected secret to be registered with shell")
		}
	})

	t.Run("ReturnsErrorWhenNoProviderHandles", func(t *testing.T) {
		sh := setupSecretsTestMocks(t)
		r := NewResolver([]Provider{NewMockProvider()}, sh)

		_, err := r.Resolve(SecretRef{Vault: "unknown"})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "no provider found") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("PropagatesProviderError", func(t *testing.T) {
		sh := setupSecretsTestMocks(t)
		p := &MockProvider{
			ResolveFunc: func(ref SecretRef) (string, bool, error) {
				return "", true, fmt.Errorf("fetch failed")
			},
		}
		r := NewResolver([]Provider{p}, sh)

		_, err := r.Resolve(SecretRef{Vault: "any"})
		if err == nil || !strings.Contains(err.Error(), "fetch failed") {
			t.Errorf("expected fetch failed error, got %v", err)
		}
	})

	t.Run("SkipsUnhandledProviders", func(t *testing.T) {
		sh := setupSecretsTestMocks(t)
		unhandled := &MockProvider{
			ResolveFunc: func(ref SecretRef) (string, bool, error) {
				return "", false, nil
			},
		}
		handled := &MockProvider{
			ResolveFunc: func(ref SecretRef) (string, bool, error) {
				return "found", true, nil
			},
		}
		r := NewResolver([]Provider{unhandled, handled}, sh)

		value, err := r.Resolve(SecretRef{Vault: "x"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if value != "found" {
			t.Errorf("expected %q, got %q", "found", value)
		}
	})

	t.Run("NilShellDoesNotPanic", func(t *testing.T) {
		p := &MockProvider{
			ResolveFunc: func(ref SecretRef) (string, bool, error) {
				return "val", true, nil
			},
		}
		r := NewResolver([]Provider{p}, nil)
		value, err := r.Resolve(SecretRef{Vault: "v"})
		if err != nil || value != "val" {
			t.Errorf("expected val/nil, got %q/%v", value, err)
		}
	})
}

func TestResolver_EvaluateHelper(t *testing.T) {
	t.Run("ReturnsDeferredErrorWhenNotDeferred", func(t *testing.T) {
		r := NewResolver([]Provider{}, nil)
		result, err := r.EvaluateHelper([]any{"vault", "item", "field"}, false)
		if result != nil {
			t.Errorf("expected nil result, got %v", result)
		}
		if err == nil {
			t.Fatal("expected DeferredError, got nil")
		}
		var de *DeferredError
		if !isType(err, &de) {
			t.Errorf("expected *DeferredError, got %T", err)
		}
	})

	t.Run("ResolvesWhenDeferred", func(t *testing.T) {
		sh := setupSecretsTestMocks(t)
		p := &MockProvider{
			ResolveFunc: func(ref SecretRef) (string, bool, error) {
				return "resolved", true, nil
			},
		}
		r := NewResolver([]Provider{p}, sh)

		result, err := r.EvaluateHelper([]any{"vault", "item", "field"}, true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result != "resolved" {
			t.Errorf("expected %q, got %v", "resolved", result)
		}
	})

	t.Run("ReturnsErrorForBadParams", func(t *testing.T) {
		r := NewResolver([]Provider{}, nil)
		_, err := r.EvaluateHelper([]any{"only-one"}, true)
		if err == nil {
			t.Fatal("expected error for wrong param count")
		}
	})

	t.Run("ReturnsErrorForNonStringParams", func(t *testing.T) {
		r := NewResolver([]Provider{}, nil)
		_, err := r.EvaluateHelper([]any{123, "item", "field"}, true)
		if err == nil {
			t.Fatal("expected error for non-string param")
		}
	})
}

func TestResolver_LoadAll(t *testing.T) {
	t.Run("CallsLoadSecretsOnAllProviders", func(t *testing.T) {
		calls := 0
		p1 := &MockProvider{LoadSecretsFunc: func() error { calls++; return nil }}
		p2 := &MockProvider{LoadSecretsFunc: func() error { calls++; return nil }}
		r := NewResolver([]Provider{p1, p2}, nil)

		if err := r.LoadAll(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if calls != 2 {
			t.Errorf("expected 2 LoadSecrets calls, got %d", calls)
		}
	})

	t.Run("ReturnsFirstError", func(t *testing.T) {
		p := &MockProvider{LoadSecretsFunc: func() error { return fmt.Errorf("load error") }}
		r := NewResolver([]Provider{p}, nil)
		if err := r.LoadAll(); err == nil || !strings.Contains(err.Error(), "load error") {
			t.Errorf("expected load error, got %v", err)
		}
	})
}

func TestNormalizeExpression(t *testing.T) {
	tests := []struct {
		input    string
		wantOut  string
		wantBool bool
	}{
		// secret. prefix — op provider
		{"secret.op.myvault.myitem.myfield", `secret("myvault", "myitem", "myfield")`, true},
		// secrets. prefix — op provider
		{"secrets.op.myvault.myitem.myfield", `secret("myvault", "myitem", "myfield")`, true},
		// secret. prefix — sops provider
		{"secret.sops.database.password", `secret("sops", "database.password", "")`, true},
		// secrets. prefix — sops provider
		{"secrets.sops.nested.key.path", `secret("sops", "nested.key.path", "")`, true},
		// bare op. prefix (legacy)
		{"op.myvault.myitem.myfield", `secret("myvault", "myitem", "myfield")`, true},
		// bare sops. prefix (legacy)
		{"sops.database.password", `secret("sops", "database.password", "")`, true},
		// bracket notation
		{`secret.op.vault["item"].field`, `secret("vault", "item", "field")`, true},
		// already canonical — not rewritten
		{`secret("v", "i", "f")`, `secret("v", "i", "f")`, false},
		// plain string — not a secret
		{"SOME_ENV_VAR", "", false},
		// empty string
		{"", "", false},
		// wrong number of op parts
		{"secret.op.vault.item", "", false},
		// sops with only one part
		{"secret.sops", "", false},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			out, ok := NormalizeExpression(tc.input)
			if ok != tc.wantBool {
				t.Errorf("NormalizeExpression(%q) ok=%v, want %v", tc.input, ok, tc.wantBool)
			}
			if ok && out != tc.wantOut {
				t.Errorf("NormalizeExpression(%q) = %q, want %q", tc.input, out, tc.wantOut)
			}
		})
	}
}

func TestNormalizeLegacyBraces(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"${{ op.vault.item.field }}", "${ op.vault.item.field }"},
		{"${{ sops.key }}", "${ sops.key }"},
		{"${{ secret_name }}", "${ secret_name }"},
		{"prefix ${{ foo }} suffix", "prefix ${ foo } suffix"},
		{"no placeholders", "no placeholders"},
		{"${ already single }", "${ already single }"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := NormalizeLegacyBraces(tc.input)
			if got != tc.want {
				t.Errorf("NormalizeLegacyBraces(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestIsCacheable(t *testing.T) {
	cacheableInputs := []string{
		`secret("vault", "item", "field")`,
		`secret("vault", "path/to/item", "field")`,
		`secret("vault", "item", "field:subfield")`,
		"secret.op.vault.item.field",
		"secrets.sops.key.path",
	}
	notCacheableInputs := []string{
		`secret("v","i","f") + "suffix"`,
		"PLAIN_VAR",
		"",
	}

	for _, input := range cacheableInputs {
		if !IsCacheable(input) {
			t.Errorf("IsCacheable(%q) = false, want true", input)
		}
	}
	for _, input := range notCacheableInputs {
		if IsCacheable(input) {
			t.Errorf("IsCacheable(%q) = true, want false", input)
		}
	}
}

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

// =============================================================================
// Test Helpers
// =============================================================================

// isType checks error type without importing errors.
func isType(err error, target any) bool {
	if de, ok := target.(**DeferredError); ok {
		_, matched := err.(*DeferredError)
		*de, _ = err.(*DeferredError)
		return matched
	}
	return false
}

func TestMockProvider(t *testing.T) {
	t.Run("LoadSecretsDefaultsToNil", func(t *testing.T) {
		p := NewMockProvider()
		if err := p.LoadSecrets(); err != nil {
			t.Errorf("expected nil, got %v", err)
		}
	})

	t.Run("ResolveDefaultsToUnhandled", func(t *testing.T) {
		p := NewMockProvider()
		_, handled, err := p.Resolve(SecretRef{Vault: "any"})
		if err != nil || handled {
			t.Errorf("expected unhandled/nil, got handled=%v err=%v", handled, err)
		}
	})

	t.Run("LoadSecretsFuncCalled", func(t *testing.T) {
		called := false
		p := &MockProvider{LoadSecretsFunc: func() error { called = true; return nil }}
		_ = p.LoadSecrets()
		if !called {
			t.Error("expected LoadSecretsFunc to be called")
		}
	})

	t.Run("ResolveFuncCalled", func(t *testing.T) {
		p := &MockProvider{
			ResolveFunc: func(ref SecretRef) (string, bool, error) {
				return "val", true, nil
			},
		}
		v, h, err := p.Resolve(SecretRef{Vault: "x"})
		if err != nil || !h || v != "val" {
			t.Errorf("unexpected: v=%q h=%v err=%v", v, h, err)
		}
	})
}
