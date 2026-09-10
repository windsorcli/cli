package config

import (
	"testing"
)

func TestDefaultConfigurations(t *testing.T) {
	t.Run("DefaultConfig_HasPlatformNone", func(t *testing.T) {
		if DefaultConfig.Platform == nil || *DefaultConfig.Platform != "none" {
			t.Error("Expected DefaultConfig platform to be 'none'")
		}
	})

	t.Run("DefaultConfig_DevIsEmpty", func(t *testing.T) {
		if DefaultConfig_Dev.Network != nil {
			t.Error("network config should not be set (schema default)")
		}
		if DefaultConfig_Dev.Terraform != nil {
			t.Error("terraform config should not be set (schema default)")
		}
		if DefaultConfig_Dev.Cluster != nil {
			t.Error("cluster config should not be set (schema/facet default)")
		}
		if DefaultConfig_Dev.Docker != nil {
			t.Error("docker config should not be set (facet default)")
		}
		if DefaultConfig_Dev.Git != nil {
			t.Error("git config should not be set (facet default)")
		}
		if DefaultConfig_Dev.DNS != nil {
			t.Error("dns config should not be set (schema/facet default)")
		}
	})
}

func TestDefaultTerraformBackendTypeForPlatform(t *testing.T) {
	testCases := []struct {
		platform string
		want     string
	}{
		{"aws", "s3"},
		{"azure", "azurerm"},
		{"gcp", "gcs"},
		{"metal", "kubernetes"},
		{"docker", "kubernetes"},
		{"incus", "kubernetes"},
		{"hetzner", "kubernetes"},
		{"hyperv", "kubernetes"},
		{"vsphere", "kubernetes"},
		{"none", ""},
		{"", ""},
		{"unrecognized", ""},
	}
	for _, tc := range testCases {
		t.Run(tc.platform, func(t *testing.T) {
			if got := DefaultTerraformBackendTypeForPlatform(tc.platform); got != tc.want {
				t.Errorf("Expected platform %q to map to %q, got %q", tc.platform, tc.want, got)
			}
		})
	}
}
