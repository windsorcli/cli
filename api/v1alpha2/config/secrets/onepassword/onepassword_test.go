package onepassword

import (
	"testing"
)

// TestOnePasswordConfig_Merge tests the Merge method of OnePasswordConfig
func TestOnePasswordConfig_Merge(t *testing.T) {
	t.Run("MergeWithNilOverlay", func(t *testing.T) {
		base := &OnePasswordConfig{
			Vaults: map[string]OnePasswordVault{
				"vault1": {ID: "id1", URL: "url1", Name: "Vault 1"},
			},
		}
		original := base.DeepCopy()

		base.Merge(nil)

		if len(base.Vaults) != len(original.Vaults) {
			t.Errorf("Expected vaults to remain unchanged")
		}
		if base.Vaults["vault1"].ID != original.Vaults["vault1"].ID {
			t.Errorf("Expected vault ID to remain unchanged")
		}
	})

	t.Run("MergeWithEmptyOverlay", func(t *testing.T) {
		base := &OnePasswordConfig{
			Vaults: map[string]OnePasswordVault{
				"vault1": {ID: "id1", URL: "url1", Name: "Vault 1"},
			},
		}
		overlay := &OnePasswordConfig{}

		base.Merge(overlay)

		if len(base.Vaults) != 1 {
			t.Errorf("Expected vaults to remain unchanged")
		}
		if base.Vaults["vault1"].ID != "id1" {
			t.Errorf("Expected vault ID to remain 'id1'")
		}
	})

	t.Run("MergeWithNewVault", func(t *testing.T) {
		base := &OnePasswordConfig{
			Vaults: map[string]OnePasswordVault{
				"vault1": {ID: "id1", URL: "url1", Name: "Vault 1"},
			},
		}
		overlay := &OnePasswordConfig{
			Vaults: map[string]OnePasswordVault{
				"vault2": {ID: "id2", URL: "url2", Name: "Vault 2"},
			},
		}

		base.Merge(overlay)

		if len(base.Vaults) != 2 {
			t.Errorf("Expected 2 vaults, got %d", len(base.Vaults))
		}
		if base.Vaults["vault1"].ID != "id1" {
			t.Errorf("Expected vault1 ID to remain 'id1'")
		}
		if base.Vaults["vault2"].ID != "id2" {
			t.Errorf("Expected vault2 ID to be 'id2'")
		}
	})

	t.Run("MergeWithExistingVault", func(t *testing.T) {
		base := &OnePasswordConfig{
			Vaults: map[string]OnePasswordVault{
				"vault1": {ID: "id1", URL: "url1", Name: "Vault 1"},
			},
		}
		overlay := &OnePasswordConfig{
			Vaults: map[string]OnePasswordVault{
				"vault1": {ID: "id1-new", URL: "url1-new", Name: "Vault 1 Updated"},
			},
		}

		base.Merge(overlay)

		if len(base.Vaults) != 1 {
			t.Errorf("Expected 1 vault, got %d", len(base.Vaults))
		}
		if base.Vaults["vault1"].ID != "id1-new" {
			t.Errorf("Expected vault1 ID to be 'id1-new', got %s", base.Vaults["vault1"].ID)
		}
		if base.Vaults["vault1"].URL != "url1-new" {
			t.Errorf("Expected vault1 URL to be 'url1-new', got %s", base.Vaults["vault1"].URL)
		}
		if base.Vaults["vault1"].Name != "Vault 1 Updated" {
			t.Errorf("Expected vault1 Name to be 'Vault 1 Updated', got %s", base.Vaults["vault1"].Name)
		}
	})

	t.Run("MergeWithPartialVaultUpdate", func(t *testing.T) {
		base := &OnePasswordConfig{
			Vaults: map[string]OnePasswordVault{
				"vault1": {ID: "id1", URL: "url1", Name: "Vault 1"},
			},
		}
		overlay := &OnePasswordConfig{
			Vaults: map[string]OnePasswordVault{
				"vault1": {ID: "id1-new", URL: "", Name: ""},
			},
		}

		base.Merge(overlay)

		if base.Vaults["vault1"].ID != "id1-new" {
			t.Errorf("Expected vault1 ID to be 'id1-new', got %s", base.Vaults["vault1"].ID)
		}
		if base.Vaults["vault1"].URL != "url1" {
			t.Errorf("Expected vault1 URL to remain 'url1', got %s", base.Vaults["vault1"].URL)
		}
		if base.Vaults["vault1"].Name != "Vault 1" {
			t.Errorf("Expected vault1 Name to remain 'Vault 1', got %s", base.Vaults["vault1"].Name)
		}
	})

	t.Run("MergeWithNilBaseVaults", func(t *testing.T) {
		base := &OnePasswordConfig{}
		overlay := &OnePasswordConfig{
			Vaults: map[string]OnePasswordVault{
				"vault1": {ID: "id1", URL: "url1", Name: "Vault 1"},
			},
		}

		base.Merge(overlay)

		if len(base.Vaults) != 1 {
			t.Errorf("Expected 1 vault, got %d", len(base.Vaults))
		}
		if base.Vaults["vault1"].ID != "id1" {
			t.Errorf("Expected vault1 ID to be 'id1'")
		}
	})
}

// TestOnePasswordConfig_Copy tests the Copy method of OnePasswordConfig
func TestOnePasswordConfig_Copy(t *testing.T) {
	t.Run("CopyNilConfig", func(t *testing.T) {
		var config *OnePasswordConfig
		copied := config.DeepCopy()

		if copied != nil {
			t.Error("Expected nil copy for nil config")
		}
	})

	t.Run("CopyEmptyConfig", func(t *testing.T) {
		config := &OnePasswordConfig{}
		copied := config.DeepCopy()

		if copied == nil {
			t.Error("Expected non-nil copy of empty config")
		}
		if copied.Vaults == nil {
			t.Error("Expected vaults map to be initialized")
		}
		if len(copied.Vaults) != 0 {
			t.Error("Expected empty vaults map")
		}
	})

	t.Run("CopyPopulatedConfig", func(t *testing.T) {
		config := &OnePasswordConfig{
			Vaults: map[string]OnePasswordVault{
				"vault1": {ID: "id1", URL: "url1", Name: "Vault 1"},
				"vault2": {ID: "id2", URL: "url2", Name: "Vault 2"},
			},
		}

		copied := config.DeepCopy()

		if copied == nil {
			t.Error("Expected non-nil copy")
		}
		if copied == config {
			t.Error("Expected copy to be a new instance")
		}
		if len(copied.Vaults) != len(config.Vaults) {
			t.Errorf("Expected same number of vaults, got %d vs %d", len(copied.Vaults), len(config.Vaults))
		}
		if copied.Vaults["vault1"].ID != config.Vaults["vault1"].ID {
			t.Errorf("Expected vault1 ID to be copied correctly")
		}
		if copied.Vaults["vault2"].ID != config.Vaults["vault2"].ID {
			t.Errorf("Expected vault2 ID to be copied correctly")
		}
	})

	t.Run("CopyWithIndependentValues", func(t *testing.T) {
		config := &OnePasswordConfig{
			Vaults: map[string]OnePasswordVault{
				"vault1": {ID: "id1", URL: "url1", Name: "Vault 1"},
			},
		}

		copied := config.DeepCopy()

		// Modify original to verify independence
		config.Vaults["vault1"] = OnePasswordVault{ID: "id1-modified", URL: "url1-modified", Name: "Vault 1 Modified"}

		if copied.Vaults["vault1"].ID != "id1" {
			t.Error("Expected copied vault1 ID to remain independent")
		}
		if copied.Vaults["vault1"].URL != "url1" {
			t.Error("Expected copied vault1 URL to remain independent")
		}
		if copied.Vaults["vault1"].Name != "Vault 1" {
			t.Error("Expected copied vault1 Name to remain independent")
		}
	})

	t.Run("CopyWithSingleVault", func(t *testing.T) {
		config := &OnePasswordConfig{
			Vaults: map[string]OnePasswordVault{
				"vault1": {ID: "id1", URL: "url1", Name: "Vault 1"},
			},
		}

		copied := config.DeepCopy()

		if copied == nil {
			t.Error("Expected non-nil copy")
		}
		if len(copied.Vaults) != 1 {
			t.Errorf("Expected 1 vault, got %d", len(copied.Vaults))
		}
		if copied.Vaults["vault1"].ID != config.Vaults["vault1"].ID {
			t.Errorf("Expected vault1 ID to be copied correctly")
		}
	})
}
