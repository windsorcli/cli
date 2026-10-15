// This file defines default configurations that supplement schema and facet defaults.
// Only values NOT already provided by schema.yaml defaults or facet config blocks belong here.

package config

import (
	cliconfig "github.com/windsorcli/cli/api/config/v1alpha2"
)

// DefaultConfig is the base configuration for non-dev contexts (platform "none").
var DefaultConfig = cliconfig.Context{
	Platform: "none",
}

// DefaultConfig_Dev provides defaults for all dev contexts (docker-desktop, colima, incus).
var DefaultConfig_Dev = cliconfig.Context{}

// DefaultTerraformBackendTypeForPlatform returns the canonical terraform state backend for a
// platform, or "" when the platform has no default backend (the caller falls back to "local").
// Kept in sync with the platform description in configuration.yaml.
func DefaultTerraformBackendTypeForPlatform(platform string) string {
	switch platform {
	case "aws":
		return "s3"
	case "azure":
		return "azurerm"
	case "gcp":
		return "gcs"
	case "metal", "docker", "incus", "hetzner", "hyperv", "vsphere":
		return "kubernetes"
	default:
		return ""
	}
}
