// The VsphereEnvPrinter is a specialized component that manages vSphere environment configuration.
// It provides vSphere-specific environment variable management so that Terraform and other tools
// can connect to vCenter without requiring inline credential flags.

package env

import (
	"fmt"
	"path/filepath"
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
// In project mode, VSPHERE_PERSIST_SESSION, VSPHERE_VIM_SESSION_PATH, and
// VSPHERE_REST_SESSION_PATH are always emitted, scoping the Terraform vSphere
// provider's SOAP/REST session cache to the context's .vsphere/ directory —
// mirrors how AZURE_CONFIG_DIR scopes az CLI state for the Azure env printer.
// In global mode these three are omitted so the provider falls back to its
// own ambient defaults (~/.govmomi/sessions, ~/.govmomi/rest_sessions)
// untouched. The remaining three env vars consumed by the provider are
// emitted from config: VSPHERE_SERVER, VSPHERE_USER, and
// VSPHERE_ALLOW_UNVERIFIED_SSL. Inventory pointers (datacenter, cluster,
// datastore, network, etc.) are Terraform variable inputs wired by the facet
// — they are not read from the environment. VSPHERE_PASSWORD must be
// supplied via secrets or the ambient environment; plaintext passwords are
// never written to the shell config file.
func (e *VsphereEnvPrinter) GetEnvVars() (map[string]string, error) {
	envVars := make(map[string]string)
	global := e.shell.IsGlobal()

	if !global {
		configRoot, err := e.configHandler.GetConfigRoot()
		if err != nil {
			return nil, fmt.Errorf("error retrieving configuration root directory: %w", err)
		}
		vsphereDir := filepath.Join(configRoot, ".vsphere")
		envVars["VSPHERE_PERSIST_SESSION"] = "true"
		envVars["VSPHERE_VIM_SESSION_PATH"] = filepath.ToSlash(filepath.Join(vsphereDir, "sessions"))
		envVars["VSPHERE_REST_SESSION_PATH"] = filepath.ToSlash(filepath.Join(vsphereDir, "rest_sessions"))
	}

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
