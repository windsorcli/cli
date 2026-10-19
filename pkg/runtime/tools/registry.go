// The tool registry is the catalog of the external CLIs that Windsor checks for.
// It provides the display name, the minimum version, and the install page of each tool.
// The missing and outdated tool errors use it to tell the user how to install or upgrade a tool.
// Each key is the binary name that the matching check passes to LookPath.

package tools

import (
	"fmt"

	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/constants"
)

// =============================================================================
// Constants
// =============================================================================

// toolRegistry maps the on-PATH binary name to its toolInfo. Keys MUST match the argument
// passed to LookPath in the corresponding check* function — missingToolError and
// outdatedToolError look up by that key, and a mismatch silently degrades the error to a
// generic message without install hints.
var toolRegistry = map[string]toolInfo{
	"docker": {
		name:       "Docker",
		minVersion: constants.MinimumVersionDocker,
		download:   "https://docs.docker.com/engine/install/",
	},
	"colima": {
		name:       "Colima",
		minVersion: constants.MinimumVersionColima,
		download:   "https://colima.run/docs/installation/",
	},
	"limactl": {
		name:       "Lima",
		minVersion: constants.MinimumVersionLima,
		download:   "https://lima-vm.io/docs/installation/",
	},
	"terraform": {
		name:       "Terraform",
		minVersion: constants.MinimumVersionTerraform,
		download:   "https://developer.hashicorp.com/terraform/install",
	},
	"tofu": {
		name:       "OpenTofu",
		minVersion: constants.MinimumVersionTerraform,
		download:   "https://opentofu.org/docs/intro/install/",
	},
	"op": {
		name:       "1Password CLI",
		minVersion: constants.MinimumVersion1Password,
		download:   "https://developer.1password.com/docs/cli/get-started",
	},
	"sops": {
		name:       "SOPS",
		minVersion: constants.MinimumVersionSOPS,
		download:   "https://github.com/getsops/sops/releases",
	},
	"kubelogin": {
		name:       "kubelogin",
		minVersion: constants.MinimumVersionKubelogin,
		download:   "https://azure.github.io/kubelogin/install.html",
	},
	"aws": {
		name:       "AWS CLI",
		minVersion: constants.MinimumVersionAWS,
		download:   "https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html",
	},
	"az": {
		name:       "Azure CLI",
		minVersion: constants.MinimumVersionAzure,
		download:   "https://learn.microsoft.com/en-us/cli/azure/install-azure-cli",
	},
	"gcloud": {
		name:       "Google Cloud CLI",
		minVersion: constants.MinimumVersionGCP,
		download:   "https://cloud.google.com/sdk/docs/install",
	},
}

// =============================================================================
// Types
// =============================================================================

// toolInfo describes the user-facing identity of a checked tool: the display name used in
// error messages, the minimum version Windsor will accept, and the first-party
// install/download page (vendor docs, never a third-party mirror or distribution-manager
// package). We point operators at the vendor's own instructions so platform-specific
// nuances (brew vs apt vs winget vs direct binary, signing, GPG keys) are handled by the
// authoritative source rather than a half-correct copy here.
type toolInfo struct {
	name       string
	minVersion string
	download   string
}

// =============================================================================
// Helpers
// =============================================================================

// missingToolError returns the error for a tool that is not on PATH. The hint names the vendor
// install page. An unknown key gets a plain error with no hint, so a typo in a check stays readable.
func missingToolError(key string) error {
	info, ok := toolRegistry[key]
	if !ok {
		return fmt.Errorf("%s is required but was not found on PATH", key)
	}
	return werror.New(fmt.Sprintf("%s >= %s is required but was not found on PATH.", info.name, info.minVersion),
		fmt.Sprintf("Install %s from %s.", info.name, info.download), nil)
}

// outdatedToolError returns the error for a tool below its minimum version. found is the version
// that the tool printed, and an empty found shows as "(unknown)". The hint names the install page.
func outdatedToolError(key, found string) error {
	if found == "" {
		found = "(unknown)"
	}
	info, ok := toolRegistry[key]
	if !ok {
		return fmt.Errorf("%s version %s is below the required minimum", key, found)
	}
	return werror.New(fmt.Sprintf("%s %s is below the minimum required version %s.", info.name, found, info.minVersion),
		fmt.Sprintf("Upgrade %s to %s or later. See %s.", info.name, info.minVersion, info.download), nil)
}
