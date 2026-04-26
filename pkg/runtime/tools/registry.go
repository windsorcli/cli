package tools

import (
	"fmt"

	"github.com/windsorcli/cli/pkg/constants"
)

// The toolRegistry is a static catalog of every external CLI Windsor checks for. It exists so
// "tool not on PATH" / "tool below minimum version" errors carry a copy-pasteable next step
// (first-party download URL plus the matching aqua package name) instead of just naming the
// missing binary. Entries are keyed by the actual lookup name passed to exec.LookPath so
// check* code paths and error formatting agree on a single source of truth.

// =============================================================================
// Types
// =============================================================================

// toolInfo describes the user-facing identity of a checked tool: the display name used in
// error messages, the minimum version Windsor will accept, the first-party install/download
// page (vendor docs, never a third-party mirror), and the aqua package name so operators
// already using aqua get a one-liner that fits their workflow.
type toolInfo struct {
	name       string
	minVersion string
	download   string
	aquaPkg    string
}

// =============================================================================
// Constants
// =============================================================================

// toolRegistry maps the on-PATH binary name to its toolInfo. Keys MUST match the argument
// passed to execLookPath in the corresponding check* function — missingToolError and
// outdatedToolError look up by that key, and a mismatch silently degrades the error to a
// generic message without install hints.
var toolRegistry = map[string]toolInfo{
	"docker": {
		name:       "Docker",
		minVersion: constants.MinimumVersionDocker,
		download:   "https://docs.docker.com/get-docker/",
		aquaPkg:    "docker/cli",
	},
	"colima": {
		name:       "Colima",
		minVersion: constants.MinimumVersionColima,
		download:   "https://github.com/abiosoft/colima#installation",
		aquaPkg:    "abiosoft/colima",
	},
	"limactl": {
		name:       "Lima",
		minVersion: constants.MinimumVersionLima,
		download:   "https://lima-vm.io/docs/installation/",
		aquaPkg:    "lima-vm/lima",
	},
	"terraform": {
		name:       "Terraform",
		minVersion: constants.MinimumVersionTerraform,
		download:   "https://developer.hashicorp.com/terraform/install",
		aquaPkg:    "hashicorp/terraform",
	},
	"tofu": {
		name:       "OpenTofu",
		minVersion: constants.MinimumVersionTerraform,
		download:   "https://opentofu.org/docs/intro/install/",
		aquaPkg:    "opentofu/opentofu",
	},
	"op": {
		name:       "1Password CLI",
		minVersion: constants.MinimumVersion1Password,
		download:   "https://developer.1password.com/docs/cli/get-started",
		aquaPkg:    "1password/cli",
	},
	"sops": {
		name:       "SOPS",
		minVersion: constants.MinimumVersionSOPS,
		download:   "https://github.com/getsops/sops/releases",
		aquaPkg:    "getsops/sops",
	},
	"kubelogin": {
		name:       "kubelogin",
		minVersion: constants.MinimumVersionKubelogin,
		download:   "https://azure.github.io/kubelogin/install.html",
		aquaPkg:    "Azure/kubelogin",
	},
	"aws": {
		name:       "AWS CLI",
		minVersion: constants.MinimumVersionAWS,
		download:   "https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html",
		aquaPkg:    "aws/aws-cli",
	},
}

// =============================================================================
// Helpers
// =============================================================================

// missingToolError formats the standard "<tool> is required but was not found on PATH" error
// with install guidance: the first-party download URL and the matching aqua package. Returned
// as a fmt.Errorf so callers can wrap it. Falls back to a bare message if key is unknown,
// which is the conservative thing to do — a typo in a check* function should still produce a
// readable error rather than a panic.
func missingToolError(key string) error {
	info, ok := toolRegistry[key]
	if !ok {
		return fmt.Errorf("%s is required but was not found on PATH", key)
	}
	return fmt.Errorf("%s >= %s is required but was not found on PATH.\n  Download:    %s\n  Or via aqua: aqua g -i %s",
		info.name, info.minVersion, info.download, info.aquaPkg)
}

// outdatedToolError formats the standard "<tool> <found> is below the minimum required version
// <min>" error with the same install guidance as missingToolError. found is the version string
// extracted from the tool's --version output; an empty found is rendered as "(unknown)" so the
// message stays readable when extractVersion returns nothing.
func outdatedToolError(key, found string) error {
	if found == "" {
		found = "(unknown)"
	}
	info, ok := toolRegistry[key]
	if !ok {
		return fmt.Errorf("%s version %s is below the required minimum", key, found)
	}
	return fmt.Errorf("%s %s is below the minimum required version %s.\n  Download:    %s\n  Or via aqua: aqua g -i %s",
		info.name, found, info.minVersion, info.download, info.aquaPkg)
}
