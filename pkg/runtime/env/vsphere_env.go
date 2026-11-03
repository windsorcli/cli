// The VsphereEnvPrinter is a specialized component that manages vSphere environment configuration.
// It provides vSphere-specific environment variable management so that Terraform and other tools
// can connect to vCenter without requiring inline credential flags.

package env

import (
	"path/filepath"
	"strconv"

	"github.com/windsorcli/cli/internal/werror"
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

// GetEnvVars returns the vSphere environment variables of the current context.
// In project mode, the VSPHERE_*_SESSION* variables keep the provider session cache in .vsphere/ of the context.
// Global mode omits them, so the provider uses its ambient defaults.
// It sets VSPHERE_SERVER, VSPHERE_USER, and VSPHERE_ALLOW_UNVERIFIED_SSL from config.
// VSPHERE_PASSWORD comes from secrets or the environment. It registers each key as managed.
func (e *VsphereEnvPrinter) GetEnvVars() (map[string]string, error) {
	envVars := make(map[string]string)
	global := e.shell.IsGlobal()

	if !global {
		configRoot, err := e.configHandler.GetConfigRoot()
		if err != nil {
			return nil, werror.Wrap(err, "error retrieving configuration root directory")
		}
		vsphereDir := filepath.Join(configRoot, ".vsphere")
		envVars["VSPHERE_PERSIST_SESSION"] = "true"
		envVars["VSPHERE_VIM_SESSION_PATH"] = filepath.ToSlash(filepath.Join(vsphereDir, "sessions"))
		envVars["VSPHERE_REST_SESSION_PATH"] = filepath.ToSlash(filepath.Join(vsphereDir, "rest_sessions"))
	}

	cfg := e.configHandler.GetHeader()
	if cfg == nil || cfg.VSphere == nil {
		for key := range envVars {
			e.SetManagedEnv(key)
		}
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

	for key := range envVars {
		e.SetManagedEnv(key)
	}

	return envVars, nil
}

// =============================================================================
// Interface Compliance
// =============================================================================

// Ensure VsphereEnvPrinter implements the EnvPrinter interface
var _ EnvPrinter = (*VsphereEnvPrinter)(nil)
