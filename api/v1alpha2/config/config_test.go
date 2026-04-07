package v1alpha2

import (
	"testing"

	"github.com/windsorcli/cli/api/v1alpha2/config/providers"
	"github.com/windsorcli/cli/api/v1alpha2/config/secrets"
	"github.com/windsorcli/cli/api/v1alpha2/config/terraform"
	"github.com/windsorcli/cli/api/v1alpha2/config/workstation"
)

// TestConfig_Merge tests the Merge functionality of the Config struct
func TestConfig_Merge(t *testing.T) {
	t.Run("HandleNilOverlayGracefully", func(t *testing.T) {
		base := &Config{
			Version: "v1alpha2",
			Environment: map[string]string{
				"key1": "value1",
			},
		}
		original := base.DeepCopy()

		base.Merge(nil)

		if base.Version != original.Version {
			t.Errorf("expected version to remain unchanged, got %s", base.Version)
		}
		if base.Environment["key1"] != original.Environment["key1"] {
			t.Errorf("expected environment to remain unchanged")
		}
	})

	t.Run("MergeEnvironmentVariables", func(t *testing.T) {
		base := &Config{
			Version: "v1alpha2",
			Environment: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
		}
		overlay := &Config{
			Environment: map[string]string{
				"key2": "newvalue2",
				"key3": "value3",
			},
		}

		base.Merge(overlay)

		if base.Environment["key1"] != "value1" {
			t.Errorf("expected key1 to remain unchanged, got %s", base.Environment["key1"])
		}
		if base.Environment["key2"] != "newvalue2" {
			t.Errorf("expected key2 to be updated, got %s", base.Environment["key2"])
		}
		if base.Environment["key3"] != "value3" {
			t.Errorf("expected key3 to be added, got %s", base.Environment["key3"])
		}
	})

	t.Run("InitializeEnvironmentMapWhenNil", func(t *testing.T) {
		base := &Config{
			Version: "v1alpha2",
		}
		overlay := &Config{
			Environment: map[string]string{
				"key1": "value1",
			},
		}

		base.Merge(overlay)

		if base.Environment == nil {
			t.Error("expected environment map to be initialized")
		}
		if base.Environment["key1"] != "value1" {
			t.Errorf("expected key1 to be set, got %s", base.Environment["key1"])
		}
	})

	t.Run("MergeSecretsConfiguration", func(t *testing.T) {
		base := &Config{
			Version: "v1alpha2",
		}
		overlay := &Config{
			Secrets: &secrets.SecretsConfig{},
		}

		base.Merge(overlay)

		if base.Secrets == nil {
			t.Error("expected secrets to be initialized")
		}
	})

	t.Run("MergeProvidersConfiguration", func(t *testing.T) {
		base := &Config{
			Version: "v1alpha2",
		}
		overlay := &Config{
			Providers: &providers.ProvidersConfig{},
		}

		base.Merge(overlay)

		if base.Providers == nil {
			t.Error("expected Providers config to be initialized")
		}
	})

	t.Run("MergeTerraformConfiguration", func(t *testing.T) {
		base := &Config{
			Version: "v1alpha2",
		}
		overlay := &Config{
			Terraform: &terraform.TerraformConfig{},
		}

		base.Merge(overlay)

		if base.Terraform == nil {
			t.Error("expected Terraform config to be initialized")
		}
	})

	t.Run("MergeWorkstationConfiguration", func(t *testing.T) {
		base := &Config{
			Version: "v1alpha2",
		}
		overlay := &Config{
			Workstation: &workstation.WorkstationConfig{},
		}

		base.Merge(overlay)

		if base.Workstation == nil {
			t.Error("expected Workstation config to be initialized")
		}
	})
}

// TestConfig_Copy tests the Copy functionality of the Config struct
func TestConfig_Copy(t *testing.T) {
	t.Run("ReturnNilForNilConfig", func(t *testing.T) {
		var config *Config
		result := config.DeepCopy()

		if result != nil {
			t.Error("expected nil result for nil config")
		}
	})

	t.Run("CreateDeepCopyOfConfig", func(t *testing.T) {
		original := &Config{
			Version: "v1alpha2",
			Environment: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
		}

		copy := original.DeepCopy()

		if copy == nil {
			t.Fatal("expected non-nil copy")
		}
		if copy == original {
			t.Error("expected different pointer")
		}
		if copy.Version != original.Version {
			t.Errorf("expected version to be copied, got %s", copy.Version)
		}
		if copy.Environment == nil {
			t.Error("expected environment to be copied")
		}
		if copy.Environment["key1"] != original.Environment["key1"] {
			t.Errorf("expected environment values to be copied")
		}
	})

	t.Run("HandleNilEnvironmentInCopy", func(t *testing.T) {
		original := &Config{
			Version: "v1alpha2",
		}

		copy := original.DeepCopy()

		if copy.Environment != nil {
			t.Error("expected nil environment in copy")
		}
	})

	t.Run("CopyAllConfigSections", func(t *testing.T) {
		original := &Config{
			Version: "v1alpha2",
			Environment: map[string]string{
				"key1": "value1",
			},
			Secrets:     &secrets.SecretsConfig{},
			Providers:   &providers.ProvidersConfig{},
			Terraform:   &terraform.TerraformConfig{},
			Workstation: &workstation.WorkstationConfig{},
		}

		copy := original.DeepCopy()

		if copy.Secrets == nil {
			t.Error("expected secrets to be copied")
		}
		if copy.Providers == nil {
			t.Error("expected Providers config to be copied")
		}
		if copy.Terraform == nil {
			t.Error("expected Terraform config to be copied")
		}
		if copy.Workstation == nil {
			t.Error("expected Workstation config to be copied")
		}
	})

	t.Run("CreateIndependentEnvironmentMap", func(t *testing.T) {
		original := &Config{
			Version: "v1alpha2",
			Environment: map[string]string{
				"key1": "value1",
			},
		}

		copy := original.DeepCopy()
		copy.Environment["key1"] = "modified"

		if original.Environment["key1"] == "modified" {
			t.Error("expected original environment to remain unchanged")
		}
	})
}
