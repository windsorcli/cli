package secrets

import (
	"reflect"
	"testing"
)

func TestSecretsConfig_Merge(t *testing.T) {
	t.Run("MergeWithNonNilOverlay", func(t *testing.T) {
		base := &SecretsConfig{
			OnePasswordConfig: OnePasswordConfig{
				Vaults: map[string]OnePasswordVault{
					"vault1": {URL: "https://old-url.com", Name: "Old Vault"},
				},
			},
		}
		overlay := &SecretsConfig{
			OnePasswordConfig: OnePasswordConfig{
				Vaults: map[string]OnePasswordVault{
					"vault1": {URL: "https://new-url.com", Name: "New Vault"},
				},
			},
		}

		base.Merge(overlay)

		if base.Vaults["vault1"].URL != "https://new-url.com" {
			t.Errorf("URL mismatch: expected %v, got %v", "https://new-url.com", base.Vaults["vault1"].URL)
		}
		if base.Vaults["vault1"].Name != "New Vault" {
			t.Errorf("Name mismatch: expected %v, got %v", "New Vault", base.Vaults["vault1"].Name)
		}
	})

	t.Run("MergeWithNilOverlay", func(t *testing.T) {
		base := &SecretsConfig{
			OnePasswordConfig: OnePasswordConfig{
				Vaults: map[string]OnePasswordVault{
					"vault1": {URL: "https://old-url.com", Name: "Old Vault"},
				},
			},
		}
		var overlay *SecretsConfig = nil

		base.Merge(overlay)

		if base.Vaults["vault1"].URL != "https://old-url.com" {
			t.Errorf("URL mismatch: expected %v, got %v", "https://old-url.com", base.Vaults["vault1"].URL)
		}
		if base.Vaults["vault1"].Name != "Old Vault" {
			t.Errorf("Name mismatch: expected %v, got %v", "Old Vault", base.Vaults["vault1"].Name)
		}
	})

	t.Run("MergeWithNilBaseVaults", func(t *testing.T) {
		base := &SecretsConfig{
			OnePasswordConfig: OnePasswordConfig{
				Vaults: nil,
			},
		}
		overlay := &SecretsConfig{
			OnePasswordConfig: OnePasswordConfig{
				Vaults: map[string]OnePasswordVault{
					"vault1": {URL: "https://new-url.com", Name: "New Vault"},
				},
			},
		}

		base.Merge(overlay)

		if base.Vaults == nil {
			t.Errorf("Base Vaults should not be nil after merge")
		}
		if base.Vaults["vault1"].URL != "https://new-url.com" {
			t.Errorf("URL mismatch: expected %v, got %v", "https://new-url.com", base.Vaults["vault1"].URL)
		}
		if base.Vaults["vault1"].Name != "New Vault" {
			t.Errorf("Name mismatch: expected %v, got %v", "New Vault", base.Vaults["vault1"].Name)
		}
	})
}

func TestSecretsConfig_Copy(t *testing.T) {
	t.Run("CopyWithNonNilValues", func(t *testing.T) {
		original := &SecretsConfig{
			OnePasswordConfig: OnePasswordConfig{
				Vaults: map[string]OnePasswordVault{
					"vault1": {URL: "https://url.com", Name: "Vault"},
				},
			},
		}

		copy := original.Copy()

		if !reflect.DeepEqual(original, copy) {
			t.Errorf("Copy mismatch: expected %v, got %v", original, copy)
		}

		// Modify the copy and ensure original is unchanged
		copy.Vaults["vault1"] = OnePasswordVault{URL: "https://new-url.com", Name: "New Vault"}
		if original.Vaults["vault1"].URL == copy.Vaults["vault1"].URL {
			t.Errorf("Original URL was modified: expected %v, got %v", "https://url.com", copy.Vaults["vault1"].URL)
		}
	})

	t.Run("CopyWithNilSecretsConfig", func(t *testing.T) {
		var original *SecretsConfig = nil

		copy := original.Copy()

		// Ensure the copy is nil
		if copy != nil {
			t.Errorf("Copy is not nil, expected a nil copy")
		}
	})
}

func TestSecretsConfig_Sops(t *testing.T) {
	enabled := true
	disabled := false

	t.Run("MergeSetsEnabledFromOverlay", func(t *testing.T) {
		base := &SecretsConfig{}
		base.Merge(&SecretsConfig{Sops: &SopsConfig{Enabled: &enabled}})

		if base.Sops == nil || base.Sops.Enabled == nil || !*base.Sops.Enabled {
			t.Errorf("Expected sops.enabled=true after merge, got %+v", base.Sops)
		}
	})

	t.Run("MergeKeepsBaseWhenOverlayLeavesEnabledUnset", func(t *testing.T) {
		base := &SecretsConfig{Sops: &SopsConfig{Enabled: &enabled}}
		base.Merge(&SecretsConfig{Sops: &SopsConfig{}})

		if base.Sops == nil || base.Sops.Enabled == nil || !*base.Sops.Enabled {
			t.Errorf("Expected sops.enabled to stay true, got %+v", base.Sops)
		}
	})

	t.Run("MergeLetsOverlayDisable", func(t *testing.T) {
		base := &SecretsConfig{Sops: &SopsConfig{Enabled: &enabled}}
		base.Merge(&SecretsConfig{Sops: &SopsConfig{Enabled: &disabled}})

		if base.Sops == nil || base.Sops.Enabled == nil || *base.Sops.Enabled {
			t.Errorf("Expected sops.enabled=false after merge, got %+v", base.Sops)
		}
	})

	t.Run("CopyDoesNotShareTheEnabledPointer", func(t *testing.T) {
		original := &SecretsConfig{Sops: &SopsConfig{Enabled: &enabled}}
		copied := original.Copy()
		*copied.Sops.Enabled = false

		if !*original.Sops.Enabled {
			t.Error("Expected the original to be unchanged by edits to the copy")
		}
	})
}
