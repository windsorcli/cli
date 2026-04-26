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
// platform is inferred from the driver. After platform resolution, the function
// fills in a sensible default for terraform.backend.type when one isn't already
// set in the override map:
//
//   - aws   → s3       (S3 is the canonical state store on AWS)
//   - azure → azurerm  (Azure Blob Storage via the azurerm backend)
//   - metal, docker, incus → kubernetes  (the cluster IS the state store;
//     each component's state lives as a Secret in the cluster it manages)
//
// The default only kicks in when terraform.backend.type is absent from
// overrides, so explicit --set values (which are merged into the same map by
// callers after this helper runs) always win. gcp is intentionally not
// defaulted today: GCS backend support requires a GCSBackend schema struct
// and provider.go branches that don't yet exist. Shared by `windsor init`,
// `windsor bootstrap`, and `windsor up` to guarantee consistent flag
// semantics across commands.
func applyWorkstationFlagOverrides(overrides map[string]any, vmDriver, platform string) {
	if platform != "" {
		overrides["platform"] = platform
	}
	if vmDriver != "" {
		runtimeVal := vmDriver
		if vmDriver == "colima-incus" {
			runtimeVal = "colima"
		}
		overrides["workstation.runtime"] = runtimeVal
		if platform == "" {
			switch vmDriver {
			case "colima-incus":
				overrides["platform"] = "incus"
			case "colima":
				overrides["platform"] = "docker"
			case "docker-desktop", "docker":
				overrides["platform"] = "docker"
			}
		}
	}

	if _, set := overrides["terraform.backend.type"]; !set {
		switch overrides["platform"] {
		case "aws":
			overrides["terraform.backend.type"] = "s3"
		case "azure":
			overrides["terraform.backend.type"] = "azurerm"
		case "metal", "docker", "incus":
			overrides["terraform.backend.type"] = "kubernetes"
		}
	}
}

// resolveBlueprintURL determines the blueprint URL (if any) that init or up
// should pass to Project.Initialize. An explicit --blueprint wins, then
// --platform falls back to the default OCI URL — but only when no local
// template root is present. A present contexts/_template directory is
// always authoritative, because in repos like windsorcli/core the template
// and the default OCI source are effectively the same blueprint and
// layering them produces duplicate template/core entries. The "local"
// context falls back to the default URL when no contexts/_template
// directory exists, but only when allowLocalBootstrap is true — init opts
// in to bootstrap a fresh local context, while up must not silently pull
// from the network on a bare invocation. Returns a nil slice when the
// caller should use whatever blueprint state is already on disk.
func resolveBlueprintURL(blueprintFlag, platformFlag, contextName, templateRoot string, allowLocalBootstrap bool) ([]string, error) {
	if blueprintFlag != "" {
		return []string{blueprintFlag}, nil
	}
	if platformFlag != "" {
		// Only consult the template root when the caller permits local bootstrap (init
		// and bootstrap flows). `up` passes allowLocalBootstrap=false and must retain
		// the pre-existing "always inject on --platform" behavior so it doesn't
		// accidentally observe stat errors on a badly-shaped templateRoot.
		if allowLocalBootstrap {
			if _, err := os.Stat(templateRoot); err == nil {
				return nil, nil
			} else if !os.IsNotExist(err) {
				return nil, fmt.Errorf("error checking template root: %w", err)
			}
		}
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
