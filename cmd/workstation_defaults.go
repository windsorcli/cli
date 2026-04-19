package cmd

import (
	"fmt"
	"os"

	"github.com/windsorcli/cli/pkg/constants"
)

// =============================================================================
// Shared flag helpers for init and up
// =============================================================================

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
// should pass to Project.Initialize. An explicit --blueprint wins, then
// --platform falls back to the default OCI URL. The "local" context falls
// back to the default URL when no contexts/_template directory exists, but
// only when allowLocalBootstrap is true — init opts in to bootstrap a fresh
// local context, while up must not silently pull from the network on a bare
// invocation. Returns a nil slice when the caller should use whatever
// blueprint state is already on disk.
func resolveBlueprintURL(blueprintFlag, platformFlag, contextName, templateRoot string, allowLocalBootstrap bool) ([]string, error) {
	if blueprintFlag != "" {
		return []string{blueprintFlag}, nil
	}
	if platformFlag != "" {
		return []string{constants.GetEffectiveBlueprintURL()}, nil
	}
	if !allowLocalBootstrap || contextName != "local" {
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
