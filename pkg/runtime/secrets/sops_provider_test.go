package secrets

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// =============================================================================
// Test Setup
// =============================================================================

func setupSopsMocks(t *testing.T) *SopsProvider {
	t.Helper()
	p := NewSopsProvider("/valid/config/path")
	p.shims.Stat = func(name string) (os.FileInfo, error) {
		return nil, nil // all files exist
	}
	p.shims.DecryptFile = func(filePath string, format string) ([]byte, error) {
		return []byte("nested:\n  key: value\n  another:\n    deep: secret\n"), nil
	}
	return p
}

// =============================================================================
// Constructor
// =============================================================================

func TestNewSopsProvider(t *testing.T) {
	p := NewSopsProvider("/config/path")
	if p == nil {
		t.Fatal("expected provider, got nil")
	}
	if p.configPath != "/config/path" {
		t.Errorf("configPath = %q, want %q", p.configPath, "/config/path")
	}
	if p.unlocked {
		t.Error("expected locked initially")
	}
}

// =============================================================================
// LoadSecrets
// =============================================================================

func TestSopsProvider_LoadSecrets(t *testing.T) {
	t.Run("LoadsAndFlattensNestedYAML", func(t *testing.T) {
		p := setupSopsMocks(t)
		if err := p.LoadSecrets(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !p.unlocked {
			t.Error("expected unlocked after LoadSecrets")
		}
		if p.secrets["nested.key"] != "value" {
			t.Errorf("nested.key = %q, want %q", p.secrets["nested.key"], "value")
		}
		if p.secrets["nested.another.deep"] != "secret" {
			t.Errorf("nested.another.deep = %q, want %q", p.secrets["nested.another.deep"], "secret")
		}
	})

	t.Run("IdempotentWhenAlreadyUnlocked", func(t *testing.T) {
		p := setupSopsMocks(t)
		decryptCalls := 0
		p.shims.DecryptFile = func(_ string, _ string) ([]byte, error) {
			decryptCalls++
			return []byte("k: v\n"), nil
		}

		_ = p.LoadSecrets()
		decryptCalls = 0
		_ = p.LoadSecrets()

		if decryptCalls != 0 {
			t.Error("expected no DecryptFile calls on second LoadSecrets")
		}
	})

	t.Run("ReturnsNilWhenNoFilesExist", func(t *testing.T) {
		p := NewSopsProvider("/no/files")
		p.shims.Stat = func(_ string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		if err := p.LoadSecrets(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.unlocked {
			t.Error("expected locked when no files")
		}
	})

	t.Run("MergesMultipleFilesWithLaterOverriding", func(t *testing.T) {
		p := NewSopsProvider("/multi")
		var mu sync.Mutex
		calls := 0
		p.shims.Stat = func(name string) (os.FileInfo, error) {
			if filepath.Base(name) == "secrets.yaml" || filepath.Base(name) == "secrets.enc.yaml" {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}
		p.shims.DecryptFile = func(filePath string, _ string) ([]byte, error) {
			mu.Lock()
			calls++
			mu.Unlock()
			if filepath.Base(filePath) == "secrets.yaml" {
				return []byte("key1: value1\nkey2: first\n"), nil
			}
			return []byte("key2: second\nkey3: value3\n"), nil
		}

		if err := p.LoadSecrets(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.secrets["key1"] != "value1" {
			t.Errorf("key1 = %q, want %q", p.secrets["key1"], "value1")
		}
		if p.secrets["key2"] != "second" {
			t.Errorf("key2 = %q, want %q (later file should win)", p.secrets["key2"], "second")
		}
		if p.secrets["key3"] != "value3" {
			t.Errorf("key3 = %q, want %q", p.secrets["key3"], "value3")
		}
	})

	t.Run("ReturnsDecryptError", func(t *testing.T) {
		p := setupSopsMocks(t)
		p.shims.DecryptFile = func(_ string, _ string) ([]byte, error) {
			return nil, fmt.Errorf("decryption failed")
		}

		err := p.LoadSecrets()
		if err == nil || !containsStr(err.Error(), "decryption failed") {
			t.Errorf("expected decryption error, got %v", err)
		}
	})

	t.Run("ReturnsYAMLParseError", func(t *testing.T) {
		p := setupSopsMocks(t)
		// Override YAMLUnmarshal to simulate a parse error
		p.shims.YAMLUnmarshal = func(data []byte, v any) error {
			return fmt.Errorf("yaml parse error")
		}

		err := p.LoadSecrets()
		if err == nil || !containsStr(err.Error(), "yaml parse error") {
			t.Errorf("expected yaml parse error, got %v", err)
		}
	})
}

// =============================================================================
// Resolve
// =============================================================================

func TestSopsProvider_Resolve(t *testing.T) {
	t.Run("ReturnsUnhandledForNonSopsVault", func(t *testing.T) {
		p := setupSopsMocks(t)
		_, handled, err := p.Resolve(SecretRef{Vault: "op", Item: "item"})
		if err != nil || handled {
			t.Errorf("expected unhandled/nil, got handled=%v err=%v", handled, err)
		}
	})

	t.Run("ReturnsMaskedWhenLocked", func(t *testing.T) {
		p := setupSopsMocks(t)
		// do NOT call LoadSecrets → stays locked
		value, handled, err := p.Resolve(SecretRef{Vault: "sops", Item: "key"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !handled {
			t.Error("expected handled=true for sops vault")
		}
		if value != "********" {
			t.Errorf("expected masked value, got %q", value)
		}
	})

	t.Run("ResolvesSimpleKey", func(t *testing.T) {
		p := setupSopsMocks(t)
		_ = p.LoadSecrets()

		value, handled, err := p.Resolve(SecretRef{Vault: "sops", Item: "nested.key"})
		if err != nil || !handled || value != "value" {
			t.Errorf("expected (value, true, nil), got (%q, %v, %v)", value, handled, err)
		}
	})

	t.Run("ResolvesItemDotField", func(t *testing.T) {
		p := setupSopsMocks(t)
		_ = p.LoadSecrets()

		value, handled, err := p.Resolve(SecretRef{Vault: "sops", Item: "nested", Field: "key"})
		if err != nil || !handled || value != "value" {
			t.Errorf("expected (value, true, nil), got (%q, %v, %v)", value, handled, err)
		}
	})

	t.Run("ReturnsErrorForMissingKey", func(t *testing.T) {
		p := setupSopsMocks(t)
		_ = p.LoadSecrets()

		_, handled, err := p.Resolve(SecretRef{Vault: "sops", Item: "nonexistent"})
		if !handled || err == nil {
			t.Errorf("expected handled=true and error, got handled=%v err=%v", handled, err)
		}
	})

	t.Run("CaseInsensitiveVaultMatch", func(t *testing.T) {
		p := setupSopsMocks(t)
		_ = p.LoadSecrets()

		_, handled, _ := p.Resolve(SecretRef{Vault: "SOPS", Item: "nested.key"})
		if !handled {
			t.Error("expected sops vault to match case-insensitively")
		}
	})
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsSubstr(s, sub))
}

func containsSubstr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
