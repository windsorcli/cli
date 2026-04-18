package cmd

import (
	"fmt"
	"os"
	goruntime "runtime"

	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/runtime"
)

// =============================================================================
// Workstation Defaults
// =============================================================================

// defaultWorkstationRuntime returns the default workstation.runtime for the
// host OS. Docker Desktop is the canonical local workstation on macOS and
// Windows; Linux defaults to native Docker since Docker Desktop is optional
// there and native Docker is the typical baseline.
func defaultWorkstationRuntime() string {
	switch goruntime.GOOS {
	case "darwin", "windows":
		return "docker-desktop"
	default:
		return "docker"
	}
}

// applyWorkstationFlagOverrides maps --vm-driver and --platform flag values
// onto a config override map. --vm-driver sets workstation.runtime (with the
// colima-incus alias remapped to colima), and when no --platform is given the
// platform is inferred from the driver. Shared by `windsor init` and
// `windsor up` to guarantee consistent flag semantics across commands.
func applyWorkstationFlagOverrides(overrides map[string]any, vmDriver, platform string) {
	if platform != "" {
		overrides["platform"] = platform
	}
	if vmDriver == "" {
		return
	}
	runtimeVal := vmDriver
	if vmDriver == "colima-incus" {
		runtimeVal = "colima"
	}
	overrides["workstation.runtime"] = runtimeVal
	if platform != "" {
		return
	}
	switch vmDriver {
	case "colima-incus":
		overrides["platform"] = "incus"
	case "colima":
		overrides["platform"] = "docker"
	case "docker-desktop", "docker":
		overrides["platform"] = "docker"
	}
}

// resolveBlueprintURL determines the blueprint URL (if any) that init or up
// should pass to Project.Initialize, mirroring init's historical resolution:
// an explicit --blueprint wins, then --platform falls back to the default OCI
// URL, then the "local" context falls back to the default URL when no
// contexts/_template directory exists. Returns a nil slice when the caller
// should use whatever blueprint state is already on disk.
func resolveBlueprintURL(blueprintFlag, platformFlag, contextName, templateRoot string) ([]string, error) {
	if blueprintFlag != "" {
		return []string{blueprintFlag}, nil
	}
	if platformFlag != "" {
		return []string{constants.GetEffectiveBlueprintURL()}, nil
	}
	if contextName != "local" {
		return nil, nil
	}
	if _, err := os.Stat(templateRoot); err != nil {
		if os.IsNotExist(err) {
			return []string{constants.GetEffectiveBlueprintURL()}, nil
		}
		return nil, fmt.Errorf("error checking template root: %w", err)
	}
	return nil, nil
}

// applyLocalWorkstationDefault injects OS-appropriate workstation.runtime and
// platform defaults into the given overrides map when the current context is
// "local", no workstation.runtime flag was provided, and no workstation.runtime
// is already persisted in config. This lets a bare `windsor init` or
// `windsor up` bootstrap a local workstation without the user picking a driver
// up-front. The caller must ensure ConfigHandler.LoadConfig has been invoked
// so the persisted-value check is accurate.
func applyLocalWorkstationDefault(rt *runtime.Runtime, overrides map[string]any) {
	if rt == nil || overrides == nil {
		return
	}
	if rt.ContextName != "local" {
		return
	}
	if _, set := overrides["workstation.runtime"]; set {
		return
	}
	if rt.ConfigHandler.GetString("workstation.runtime", "") != "" {
		return
	}
	overrides["workstation.runtime"] = defaultWorkstationRuntime()
	if _, set := overrides["platform"]; !set {
		overrides["platform"] = "docker"
	}
}
