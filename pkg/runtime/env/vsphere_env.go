// The VsphereEnvPrinter is a specialized component that manages vSphere environment configuration.
// It provides vSphere-specific environment variable management so that Terraform and other tools
// can connect to vCenter without requiring inline credential flags.

package env

import (
	"strconv"

	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// =============================================================================
// Types
// =============================================================================

// VsphereEnvPrinter is a struct that implements vSphere environment configuration
type VsphereEnvPrinter struct {
	BaseEnvPrinter
}

// =============================================================================
// Constructor
// =============================================================================

// NewVsphereEnvPrinter creates a new VsphereEnvPrinter instance
func NewVsphereEnvPrinter(shell shell.Shell, configHandler config.ConfigHandler) *VsphereEnvPrinter {
	if shell == nil {
		panic("shell is required")
	}
	if configHandler == nil {
		panic("config handler is required")
	}

	return &VsphereEnvPrinter{
		BaseEnvPrinter: *NewBaseEnvPrinter(shell, configHandler),
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// GetEnvVars retrieves the environment variables for the vSphere environment.
// Only the three env vars consumed by the HashiCorp vSphere Terraform provider are
// emitted: VSPHERE_SERVER, VSPHERE_USER, and VSPHERE_ALLOW_UNVERIFIED_SSL.
// Inventory pointers (datacenter, cluster, datastore, network, etc.) are Terraform
// variable inputs wired by the facet — they are not read from the environment.
// VSPHERE_PASSWORD must be supplied via secrets or the ambient environment; plaintext
// passwords are never written to the shell config file.
func (e *VsphereEnvPrinter) GetEnvVars() (map[string]string, error) {
	envVars := make(map[string]string)

	cfg := e.configHandler.GetConfig()
	if cfg == nil || cfg.VSphere == nil {
		return envVars, nil
	}

	v := cfg.VSphere
	if v.Server != nil {
		envVars["VSPHERE_SERVER"] = *v.Server
	}
	if v.User != nil {
		envVars["VSPHERE_USER"] = *v.User
	}
	if v.Insecure != nil {
		envVars["VSPHERE_ALLOW_UNVERIFIED_SSL"] = strconv.FormatBool(*v.Insecure)
	}

	return envVars, nil
}

// =============================================================================
// Interface Compliance
// =============================================================================

// Ensure VsphereEnvPrinter implements the EnvPrinter interface
var _ EnvPrinter = (*VsphereEnvPrinter)(nil)
