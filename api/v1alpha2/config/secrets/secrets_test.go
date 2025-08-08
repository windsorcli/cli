package secrets

import (
	"reflect"
	"testing"

	"github.com/windsorcli/cli/api/v1alpha2/config/secrets/onepassword"
)

func TestSecretsConfig_Merge(t *testing.T) {
	t.Run("MergeWithNonNilOverlay", func(t *testing.T) {
		base := &SecretsConfig{
			OnePassword: &onepassword.OnePasswordConfig{
				Vaults: map[string]onepassword.OnePasswordVault{
					"vault1": {URL: "https://old-url.com", Name: "Old Vault"},
				},
			},
		}
		overlay := &SecretsConfig{
			OnePassword: &onepassword.OnePasswordConfig{
				Vaults: map[string]onepassword.OnePasswordVault{
					"vault1": {URL: "https://new-url.com", Name: "New Vault"},
				},
			},
		}

		base.Merge(overlay)

		if base.OnePassword.Vaults["vault1"].URL != "https://new-url.com" {
			t.Errorf("URL mismatch: expected %v, got %v", "https://new-url.com", base.OnePassword.Vaults["vault1"].URL)
		}
		if base.OnePassword.Vaults["vault1"].Name != "New Vault" {
			t.Errorf("Name mismatch: expected %v, got %v", "New Vault", base.OnePassword.Vaults["vault1"].Name)
		}
	})

	t.Run("MergeWithNilOverlay", func(t *testing.T) {
		base := &SecretsConfig{
			OnePassword: &onepassword.OnePasswordConfig{
				Vaults: map[string]onepassword.OnePasswordVault{
					"vault1": {URL: "https://old-url.com", Name: "Old Vault"},
				},
			},
		}
		var overlay *SecretsConfig = nil

		base.Merge(overlay)

		if base.OnePassword.Vaults["vault1"].URL != "https://old-url.com" {
			t.Errorf("URL mismatch: expected %v, got %v", "https://old-url.com", base.OnePassword.Vaults["vault1"].URL)
		}
		if base.OnePassword.Vaults["vault1"].Name != "Old Vault" {
			t.Errorf("Name mismatch: expected %v, got %v", "Old Vault", base.OnePassword.Vaults["vault1"].Name)
		}
	})

	t.Run("MergeWithNilBaseOnePassword", func(t *testing.T) {
		base := &SecretsConfig{
			OnePassword: nil,
		}
		overlay := &SecretsConfig{
			OnePassword: &onepassword.OnePasswordConfig{
				Vaults: map[string]onepassword.OnePasswordVault{
					"vault1": {URL: "https://new-url.com", Name: "New Vault"},
				},
			},
		}

		base.Merge(overlay)

		if base.OnePassword == nil {
			t.Errorf("Base OnePassword should not be nil after merge")
		}
		if base.OnePassword.Vaults["vault1"].URL != "https://new-url.com" {
			t.Errorf("URL mismatch: expected %v, got %v", "https://new-url.com", base.OnePassword.Vaults["vault1"].URL)
		}
		if base.OnePassword.Vaults["vault1"].Name != "New Vault" {
			t.Errorf("Name mismatch: expected %v, got %v", "New Vault", base.OnePassword.Vaults["vault1"].Name)
		}
	})
}

func TestSecretsConfig_Copy(t *testing.T) {
	t.Run("CopyWithNonNilValues", func(t *testing.T) {
		original := &SecretsConfig{
			OnePassword: &onepassword.OnePasswordConfig{
				Vaults: map[string]onepassword.OnePasswordVault{
					"vault1": {URL: "https://url.com", Name: "Vault"},
				},
			},
		}

		copy := original.DeepCopy()

		if !reflect.DeepEqual(original, copy) {
			t.Errorf("Copy mismatch: expected %v, got %v", original, copy)
		}

		// Modify the copy and ensure original is unchanged
		copy.OnePassword.Vaults["vault1"] = onepassword.OnePasswordVault{URL: "https://new-url.com", Name: "New Vault"}
		if original.OnePassword.Vaults["vault1"].URL == copy.OnePassword.Vaults["vault1"].URL {
			t.Errorf("Original URL was modified: expected %v, got %v", "https://url.com", copy.OnePassword.Vaults["vault1"].URL)
		}
	})

	t.Run("CopyWithNilSecretsConfig", func(t *testing.T) {
		var original *SecretsConfig = nil

		copy := original.DeepCopy()

		// Ensure the copy is nil
		if copy != nil {
			t.Errorf("Copy is not nil, expected a nil copy")
		}
	})
}
