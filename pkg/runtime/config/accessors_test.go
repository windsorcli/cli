package config_test

import (
	"testing"

	"github.com/windsorcli/cli/pkg/runtime/config"
)

// =============================================================================
// Test Accessors
// =============================================================================

func TestConfigHandler_GetTerraformBackendType(t *testing.T) {
	t.Run("ExplicitValueWins", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := config.NewConfigHandler(mocks.Shell)
		handler.Set("platform", "gcp")
		handler.Set("terraform.backend.type", "local")

		if got := handler.GetTerraformBackendType(); got != "local" {
			t.Errorf("Expected the explicit value to win over the platform default, got %q", got)
		}
	})

	t.Run("FallsBackToTheCloudPlatformDefault", func(t *testing.T) {
		testCases := []struct {
			platform string
			want     string
		}{
			{"aws", "s3"},
			{"azure", "azurerm"},
			{"gcp", "gcs"},
		}
		for _, tc := range testCases {
			t.Run(tc.platform, func(t *testing.T) {
				mocks := setupConfigMocks(t)
				handler := config.NewConfigHandler(mocks.Shell)
				handler.Set("platform", tc.platform)

				if got := handler.GetTerraformBackendType(); got != tc.want {
					t.Errorf("Expected platform %q to default to %q, got %q", tc.platform, tc.want, got)
				}
			})
		}
	})

	t.Run("FallsBackToLocalForAWorkstationPlatformWithNoExplicitBackend", func(t *testing.T) {
		// The kubernetes default needs a live cluster, so it stays out of this fallback.
		for _, platform := range []string{"metal", "docker", "incus", "hetzner", "hyperv", "vsphere", "proxmox"} {
			t.Run(platform, func(t *testing.T) {
				mocks := setupConfigMocks(t)
				handler := config.NewConfigHandler(mocks.Shell)
				handler.Set("platform", platform)

				if got := handler.GetTerraformBackendType(); got != "local" {
					t.Errorf("Expected platform %q to fall back to local, not the kubernetes default, got %q", platform, got)
				}
			})
		}
	})

	t.Run("FallsBackToLocalForAnUnrecognizedPlatform", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := config.NewConfigHandler(mocks.Shell)
		handler.Set("platform", "none")

		if got := handler.GetTerraformBackendType(); got != "local" {
			t.Errorf("Expected local for a platform with no canonical backend, got %q", got)
		}
	})

	t.Run("FallsBackToLocalWhenNeitherPlatformNorProviderIsSet", func(t *testing.T) {
		mocks := setupConfigMocks(t)
		handler := config.NewConfigHandler(mocks.Shell)

		if got := handler.GetTerraformBackendType(); got != "local" {
			t.Errorf("Expected local with no platform information at all, got %q", got)
		}
	})
}
