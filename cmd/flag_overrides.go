// The flag overrides change the init, bootstrap and up flags into config overrides.
// They give the three commands the same flag behavior.

package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/Masterminds/semver/v3"
	"github.com/windsorcli/cli/internal/logging"
	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/composer/artifact"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/runtime/config"
)

// =============================================================================
// Shared flag helpers for init and up
// =============================================================================

// applyWorkstationFlagOverrides maps the --vm-driver and --platform values onto overrides.
// --vm-driver sets workstation.runtime, and the colima-incus alias maps to colima. Without --platform,
// the platform comes from the driver. A resolved platform sets terraform.backend.type through
// defaultTerraformBackendType. windsor init, bootstrap, and up all use it, so the flags mean the same.
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

	defaultTerraformBackendType(overrides)
}

// defaultTerraformBackendType fills in terraform.backend.type from
// overrides["platform"] when the key is absent, so explicit --set values
// (merged into the same map by callers after this runs) always win. The
// platform-to-backend mapping lives in config.DefaultTerraformBackendTypeForPlatform,
// the same function ConfigHandler.GetTerraformBackendType falls back to when the
// key is unset at read time, so init-time and runtime defaulting can't drift apart.
func defaultTerraformBackendType(overrides map[string]any) {
	if _, set := overrides["terraform.backend.type"]; set {
		return
	}
	platform, _ := overrides["platform"].(string)
	if def := config.DefaultTerraformBackendTypeForPlatform(platform); def != "" {
		overrides["terraform.backend.type"] = def
	}
}

// resolveBlueprintURL returns the blueprint URL that init or up passes to Project.Initialize.
// An explicit --blueprint wins. With allowLocalBootstrap (init), --platform or the "local" context uses
// the default OCI URL only when templateRoot does not exist, because a local template is authoritative.
// Without allowLocalBootstrap (up), --platform always uses the default URL, the "local" context gets
// nil, and templateRoot is not read. A nil slice keeps the blueprint state on disk.
// resolveEffectiveBlueprintURL uses artifactBuilder, and nil or a registry failure uses the build-time pin.
func resolveBlueprintURL(ctx context.Context, blueprintFlag, platformFlag, contextName, templateRoot string, allowLocalBootstrap bool, artifactBuilder artifact.Artifact) ([]string, error) {
	if blueprintFlag != "" {
		return []string{blueprintFlag}, nil
	}
	if platformFlag != "" {
		if allowLocalBootstrap {
			if _, err := os.Stat(templateRoot); err == nil {
				return nil, nil
			} else if !os.IsNotExist(err) {
				return nil, werror.Wrap(err, "error checking template root")
			}
		}
		return []string{resolveEffectiveBlueprintURL(ctx, artifactBuilder)}, nil
	}
	if !allowLocalBootstrap || contextName != "local" {
		return nil, nil
	}
	if _, err := os.Stat(templateRoot); err != nil {
		if os.IsNotExist(err) {
			return []string{resolveEffectiveBlueprintURL(ctx, artifactBuilder)}, nil
		}
		return nil, werror.Wrap(err, "error checking template root")
	}
	return nil, nil
}

// resolveEffectiveBlueprintURL returns the highest core tag this CLI is compatible with, falling
// back to the build-time pin on any failure (nil builder, unreachable registry, no compatible
// tag) — bootstrap must never block on the network. The tag lookups read the registries mirror
// of the context, so a site with no internet access finds the tags that its mirror holds. The
// returned URL stays on the upstream host. A failed lookup writes a debug log line to the logger
// in ctx, which shows under --verbose. A warning would repeat on every init of a site whose
// mirror holds only the pinned tag. Also falls back when the pin itself isn't a
// concrete version (a dev build's mutable :latest): there's nothing to walk "newer than," and
// substituting the highest tagged release for an unreleased :latest can resolve to older or
// inconsistent content.
func resolveEffectiveBlueprintURL(ctx context.Context, artifactBuilder artifact.Artifact) string {
	pinned := constants.GetEffectiveBlueprintURL()
	if artifactBuilder == nil {
		return pinned
	}

	registry, repository, pinnedTag, err := artifactBuilder.ParseOCIRef(pinned)
	if err != nil {
		return pinned
	}
	if _, err := semver.NewVersion(pinnedTag); err != nil {
		return pinned
	}
	urlPrefix := fmt.Sprintf("oci://%s/%s", registry, repository)

	logger := logging.FromContext(ctx)
	tags, err := artifactBuilder.ListTags(pinned)
	if err != nil {
		logger.Debug("Blueprint tag lookup failed. Using the pinned version.", "url", urlPrefix, "pinned", pinnedTag, "error", err)
		return pinned
	}

	resolvedTag, _, ok, err := artifact.ResolveCompatibleTag(artifactBuilder, urlPrefix, tags)
	if err != nil || !ok {
		logger.Debug("No compatible blueprint tag found. Using the pinned version.", "url", urlPrefix, "pinned", pinnedTag, "error", err)
		return pinned
	}

	return urlPrefix + ":" + resolvedTag
}
