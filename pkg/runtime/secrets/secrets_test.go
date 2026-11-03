package secrets_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/runtime/secrets"
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
		r := secrets.NewResolver([]secrets.Provider{}, sh)
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
		err := &secrets.DeferredError{Expression: "secret(\"v\",\"i\",\"f\")", Message: "custom message"}
		if err.Error() != "custom message" {
			t.Errorf("expected %q, got %q", "custom message", err.Error())
		}
	})

	t.Run("ErrorWithoutMessage", func(t *testing.T) {
		err := &secrets.DeferredError{Expression: "secret(\"v\",\"i\",\"f\")"}
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
		sh.RegisterResolvedSecretFunc = func(secret string) {
			registered = secret
		}

		p := &secrets.MockProvider{
			ResolveFunc: func(ref secrets.SecretRef) (string, bool, error) {
				if ref.Vault == "sops" {
					return "myvalue", true, nil
				}
				return "", false, nil
			},
		}
		r := secrets.NewResolver([]secrets.Provider{p}, sh)

		value, err := r.Resolve(secrets.SecretRef{Vault: "sops", Item: "key"})
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
		r := secrets.NewResolver([]secrets.Provider{secrets.NewMockProvider()}, sh)

		_, err := r.Resolve(secrets.SecretRef{Vault: "unknown"})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), `No secrets provider handles the vault "unknown"`) {
			t.Errorf("unexpected error: %v", err)
		}
		if we := werror.Find(err); we == nil || !strings.Contains(we.Remediation, "secrets.onepassword.vaults") {
			t.Errorf("expected a remediation naming secrets.onepassword.vaults, got %v", we)
		}
	})

	t.Run("PropagatesProviderError", func(t *testing.T) {
		sh := setupSecretsTestMocks(t)
		p := &secrets.MockProvider{
			ResolveFunc: func(ref secrets.SecretRef) (string, bool, error) {
				return "", true, fmt.Errorf("fetch failed")
			},
		}
		r := secrets.NewResolver([]secrets.Provider{p}, sh)

		_, err := r.Resolve(secrets.SecretRef{Vault: "any"})
		if err == nil || !strings.Contains(err.Error(), "fetch failed") {
			t.Errorf("expected fetch failed error, got %v", err)
		}
	})

	t.Run("SkipsUnhandledProviders", func(t *testing.T) {
		sh := setupSecretsTestMocks(t)
		unhandled := &secrets.MockProvider{
			ResolveFunc: func(ref secrets.SecretRef) (string, bool, error) {
				return "", false, nil
			},
		}
		handled := &secrets.MockProvider{
			ResolveFunc: func(ref secrets.SecretRef) (string, bool, error) {
				return "found", true, nil
			},
		}
		r := secrets.NewResolver([]secrets.Provider{unhandled, handled}, sh)

		value, err := r.Resolve(secrets.SecretRef{Vault: "x"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if value != "found" {
			t.Errorf("expected %q, got %q", "found", value)
		}
	})

	t.Run("NilShellDoesNotPanic", func(t *testing.T) {
		p := &secrets.MockProvider{
			ResolveFunc: func(ref secrets.SecretRef) (string, bool, error) {
				return "val", true, nil
			},
		}
		r := secrets.NewResolver([]secrets.Provider{p}, nil)
		value, err := r.Resolve(secrets.SecretRef{Vault: "v"})
		if err != nil || value != "val" {
			t.Errorf("expected val/nil, got %q/%v", value, err)
		}
	})
}

func TestResolver_EvaluateHelper(t *testing.T) {
	t.Run("ReturnsDeferredErrorWhenNotDeferred", func(t *testing.T) {
		r := secrets.NewResolver([]secrets.Provider{}, nil)
		result, err := r.EvaluateHelper([]any{"vault", "item", "field"}, false)
		if result != nil {
			t.Errorf("expected nil result, got %v", result)
		}
		if err == nil {
			t.Fatal("expected DeferredError, got nil")
		}
		var de *secrets.DeferredError
		if !isType(err, &de) {
			t.Errorf("expected *DeferredError, got %T", err)
		}
	})

	t.Run("ResolvesWhenDeferred", func(t *testing.T) {
		sh := setupSecretsTestMocks(t)
		p := &secrets.MockProvider{
			ResolveFunc: func(ref secrets.SecretRef) (string, bool, error) {
				return "resolved", true, nil
			},
		}
		r := secrets.NewResolver([]secrets.Provider{p}, sh)

		result, err := r.EvaluateHelper([]any{"vault", "item", "field"}, true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result != "resolved" {
			t.Errorf("expected %q, got %v", "resolved", result)
		}
	})

	t.Run("ResolvesTwoArgumentFormWithEmptyField", func(t *testing.T) {
		sh := setupSecretsTestMocks(t)
		var got secrets.SecretRef
		p := &secrets.MockProvider{
			ResolveFunc: func(ref secrets.SecretRef) (string, bool, error) {
				got = ref
				return "resolved", true, nil
			},
		}
		r := secrets.NewResolver([]secrets.Provider{p}, sh)

		result, err := r.EvaluateHelper([]any{"sops", "database.password"}, true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result != "resolved" {
			t.Errorf("expected %q, got %v", "resolved", result)
		}
		if got.Item != "database.password" || got.Field != "" {
			t.Errorf("expected item %q with empty field, got %+v", "database.password", got)
		}
	})

	t.Run("ReturnsErrorForTooManyParams", func(t *testing.T) {
		r := secrets.NewResolver([]secrets.Provider{}, nil)
		_, err := r.EvaluateHelper([]any{"vault", "item", "field", "extra"}, true)
		if err == nil {
			t.Fatal("expected error for four arguments")
		}
	})

	t.Run("ReturnsErrorForBadParams", func(t *testing.T) {
		r := secrets.NewResolver([]secrets.Provider{}, nil)
		_, err := r.EvaluateHelper([]any{"only-one"}, true)
		if err == nil {
			t.Fatal("expected error for wrong param count")
		}
	})

	t.Run("ReturnsErrorForNonStringParams", func(t *testing.T) {
		r := secrets.NewResolver([]secrets.Provider{}, nil)
		_, err := r.EvaluateHelper([]any{123, "item", "field"}, true)
		if err == nil {
			t.Fatal("expected error for non-string param")
		}
	})
}

func TestResolver_LoadAll(t *testing.T) {
	t.Run("CallsLoadSecretsOnAllProviders", func(t *testing.T) {
		calls := 0
		p1 := &secrets.MockProvider{LoadSecretsFunc: func() error { calls++; return nil }}
		p2 := &secrets.MockProvider{LoadSecretsFunc: func() error { calls++; return nil }}
		r := secrets.NewResolver([]secrets.Provider{p1, p2}, nil)

		if err := r.LoadAll(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if calls != 2 {
			t.Errorf("expected 2 LoadSecrets calls, got %d", calls)
		}
	})

	t.Run("ReturnsFirstError", func(t *testing.T) {
		p := &secrets.MockProvider{LoadSecretsFunc: func() error { return fmt.Errorf("load error") }}
		r := secrets.NewResolver([]secrets.Provider{p}, nil)
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
			out, ok := secrets.NormalizeExpression(tc.input)
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
			got := secrets.NormalizeLegacyBraces(tc.input)
			if got != tc.want {
				t.Errorf("NormalizeLegacyBraces(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestIsCacheable(t *testing.T) {
	cacheableInputs := []string{
		`secret("vault", "item", "field")`,
		`secret("sops", "database.password")`,
		`secret("vault", "path/to/item", "field")`,
		`secret("vault", "item", "field:subfield")`,
		"secret.op.vault.item.field",
		"secrets.sops.key.path",
	}
	notCacheableInputs := []string{
		`secret("v","i","f") + "suffix"`,
		`secret("v","i") + "suffix"`,
		`secret("v")`,
		`secret("v","i","f","x")`,
		"PLAIN_VAR",
		"",
	}

	for _, input := range cacheableInputs {
		if !secrets.IsCacheable(input) {
			t.Errorf("IsCacheable(%q) = false, want true", input)
		}
	}
	for _, input := range notCacheableInputs {
		if secrets.IsCacheable(input) {
			t.Errorf("IsCacheable(%q) = true, want false", input)
		}
	}
}

// =============================================================================
// Test Helpers
// =============================================================================

// isType checks error type without importing errors.
func isType(err error, target any) bool {
	if de, ok := target.(**secrets.DeferredError); ok {
		_, matched := err.(*secrets.DeferredError)
		*de, _ = err.(*secrets.DeferredError)
		return matched
	}
	return false
}

func TestMockProvider(t *testing.T) {
	t.Run("LoadSecretsDefaultsToNil", func(t *testing.T) {
		p := secrets.NewMockProvider()
		if err := p.LoadSecrets(); err != nil {
			t.Errorf("expected nil, got %v", err)
		}
	})

	t.Run("ResolveDefaultsToUnhandled", func(t *testing.T) {
		p := secrets.NewMockProvider()
		_, handled, err := p.Resolve(secrets.SecretRef{Vault: "any"})
		if err != nil || handled {
			t.Errorf("expected unhandled/nil, got handled=%v err=%v", handled, err)
		}
	})

	t.Run("LoadSecretsFuncCalled", func(t *testing.T) {
		called := false
		p := &secrets.MockProvider{LoadSecretsFunc: func() error { called = true; return nil }}
		_ = p.LoadSecrets()
		if !called {
			t.Error("expected LoadSecretsFunc to be called")
		}
	})

	t.Run("ResolveFuncCalled", func(t *testing.T) {
		p := &secrets.MockProvider{
			ResolveFunc: func(ref secrets.SecretRef) (string, bool, error) {
				return "val", true, nil
			},
		}
		v, h, err := p.Resolve(secrets.SecretRef{Vault: "x"})
		if err != nil || !h || v != "val" {
			t.Errorf("unexpected: v=%q h=%v err=%v", v, h, err)
		}
	})
}
