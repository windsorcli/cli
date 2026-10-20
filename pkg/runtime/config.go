// The config functions of the Runtime run the config pipeline of a command.
// It provides the config defaults, the config load, the flag overrides, and the config save.
// In dev mode, the defaults also set the platform and the workstation runtime if the config does not set them.
// ResolveConfig runs the full pipeline one time for each command.

package runtime

import (
	"fmt"
	"maps"
	"os"

	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/runtime/config"
)

// =============================================================================
// Public Methods
// =============================================================================

// ApplyConfigDefaults sets pre-load defaults: dev mode, workstation runtime, platform, and a
// baseline config set. Defers entirely when dev mode isn't knowable yet (no explicit platform,
// no confirmed dev mode, nothing loaded), so it doesn't commit platform: "none" before
// ResolveConfig's post-load call (setConfigDefaults) sees the real value.
func (rt *Runtime) ApplyConfigDefaults(flagOverrides ...map[string]any) error {
	if rt.canonicalPlatform() == "" && !rt.ConfigHandler.IsDevMode(rt.ContextName) && !rt.ConfigHandler.IsLoaded() {
		return nil
	}
	return rt.setConfigDefaults(flagOverrides...)
}

// ResolveConfig runs the config pipeline one time for a command. The steps are:
//   - Set the pre-load defaults, load the config from disk, and set the post-load defaults.
//   - Set the flag overrides. A legacy "provider" override sets the platform.
//   - Set the DNS defaults of a workstation, and in dev mode change a colima and incus vm.runtime to platform incus.
func (rt *Runtime) ResolveConfig(flagOverrides map[string]any) error {
	if flagOverrides == nil {
		flagOverrides = make(map[string]any)
	}
	explicitPlatform, hasExplicitPlatform := explicitPlatformFromOverrides(flagOverrides)
	preLoadOverrides := maps.Clone(flagOverrides)
	if preLoadOverrides == nil {
		preLoadOverrides = make(map[string]any)
	}
	delete(preLoadOverrides, "provider")
	if hasExplicitPlatform {
		preLoadOverrides["platform"] = explicitPlatform
	} else {
		delete(preLoadOverrides, "platform")
	}
	inferredPlatform := rt.inferDevPlatformOverride(preLoadOverrides)
	if !hasExplicitPlatform && inferredPlatform != "" {
		preLoadOverrides["platform"] = inferredPlatform
	}
	if err := rt.ApplyConfigDefaults(preLoadOverrides); err != nil {
		return werror.Wrap(err, "failed to apply config defaults")
	}
	if err := rt.ConfigHandler.LoadConfig(); err != nil {
		return werror.Wrap(err, "failed to load config")
	}
	if err := rt.setConfigDefaults(preLoadOverrides); err != nil {
		return werror.Wrap(err, "failed to apply post-load config defaults")
	}
	for key, value := range flagOverrides {
		if key == "provider" {
			continue
		}
		if key == "platform" && !hasExplicitPlatform {
			continue
		}
		if err := rt.ConfigHandler.Set(key, value); err != nil {
			return werror.Wrap(err, "failed to set %s", key)
		}
	}
	if hasExplicitPlatform && explicitPlatform != "" {
		if err := rt.ConfigHandler.Set("platform", explicitPlatform); err != nil {
			return werror.Wrap(err, "failed to set platform")
		}
	}
	dnsEnabled := rt.ConfigHandler.Get("dns.enabled")
	if (dnsEnabled == nil || dnsEnabled == true) && rt.ConfigHandler.GetString("workstation.runtime") != "" {
		if rt.ConfigHandler.GetString("dns.domain") == "" {
			_ = rt.ConfigHandler.Set("dns.domain", "test")
		}
		if rt.ConfigHandler.GetString("workstation.runtime") == "docker-desktop" && rt.ConfigHandler.GetString("workstation.dns.address") == "" {
			_ = rt.ConfigHandler.Set("workstation.dns.address", "127.0.0.1")
		}
	}
	if rt.ConfigHandler.IsDevMode(rt.ContextName) {
		platform := rt.ConfigHandler.GetString("platform")
		workstationRuntime := rt.ConfigHandler.GetString("workstation.runtime")
		vmRuntime := rt.ConfigHandler.GetString("vm.runtime", "docker")
		if (platform == "" || platform == "docker") && workstationRuntime == "colima" && vmRuntime == "incus" {
			fmt.Fprintln(os.Stderr, "\033[33mWarning: vm.runtime is deprecated; use platform: incus in your context configuration instead. Support for vm.runtime will be removed in a future version.\033[0m")
			if err := rt.ConfigHandler.Set("platform", "incus"); err != nil {
				return werror.Wrap(err, "failed to set platform to incus")
			}
		}
	}
	return nil
}

// SaveConfig saves the config through the config handler.
// The handler writes the workstation keys to .windsor/contexts/<context>/workstation.yaml and not to values.yaml.
func (rt *Runtime) SaveConfig(overwrite ...bool) error {
	if rt.ConfigHandler == nil {
		return fmt.Errorf("runtime not fully initialized: config handler missing")
	}
	if err := rt.ConfigHandler.SaveWorkstationState(); err != nil {
		return werror.Wrap(err, "failed to save workstation state")
	}
	return rt.ConfigHandler.SaveConfig(overwrite...)
}

// =============================================================================
// Private Methods
// =============================================================================

// setConfigDefaults sets the shared config defaults. ResolveConfig calls it before and after the config load.
// It sets workstation.runtime from the host OS in dev mode, or when this run sets a docker or incus platform.
// A platform saved from an earlier run does not set workstation.runtime.
// In dev mode, it also sets the platform if the config does not set it.
func (rt *Runtime) setConfigDefaults(flagOverrides ...map[string]any) error {
	existingPlatform := rt.canonicalPlatform()
	isDevMode := rt.ConfigHandler.IsDevMode(rt.ContextName)

	if isDevMode {
		if err := rt.ConfigHandler.Set("dev", true); err != nil {
			return werror.Wrap(err, "failed to set dev mode")
		}
	}

	workstationRuntime := rt.ConfigHandler.GetString("workstation.runtime")
	hadRuntime := workstationRuntime != ""
	if workstationRuntime == "" && len(flagOverrides) > 0 && flagOverrides[0] != nil {
		if driver, ok := flagOverrides[0]["workstation.runtime"].(string); ok && driver != "" {
			workstationRuntime = driver
		}
	}
	overridePlatform := ""
	if len(flagOverrides) > 0 && flagOverrides[0] != nil {
		if p, ok := flagOverrides[0]["platform"].(string); ok && p != "" {
			overridePlatform = p
		}
	}
	effectivePlatform := overridePlatform
	if effectivePlatform == "" {
		effectivePlatform = existingPlatform
	}
	needsWorkstationRuntime := effectivePlatform == "" || isWorkstationPlatform(effectivePlatform)
	explicitWorkstationPlatform := isWorkstationPlatform(overridePlatform)
	shouldDefaultWorkstationRuntime := isDevMode || explicitWorkstationPlatform
	if shouldDefaultWorkstationRuntime && workstationRuntime == "" && needsWorkstationRuntime {
		switch rt.Shims.Goos() {
		case "darwin", "windows":
			workstationRuntime = "docker-desktop"
		default:
			workstationRuntime = "docker"
		}
	}

	if shouldDefaultWorkstationRuntime && !hadRuntime && workstationRuntime != "" {
		if err := rt.ConfigHandler.Set("workstation.runtime", workstationRuntime); err != nil {
			return werror.Wrap(err, "failed to set workstation.runtime")
		}
	}
	vmRuntime := ""
	if len(flagOverrides) > 0 && flagOverrides[0] != nil {
		if r, ok := flagOverrides[0]["vm.runtime"].(string); ok && r != "" {
			vmRuntime = r
		}
	}
	if vmRuntime == "" {
		vmRuntime = rt.ConfigHandler.GetString("vm.runtime", "docker")
	}

	if existingPlatform == "" && isDevMode {
		if overridePlatform != "" {
			if err := rt.ConfigHandler.Set("platform", overridePlatform); err != nil {
				return werror.Wrap(err, "failed to set platform from overrides")
			}
		} else if workstationRuntime == "colima" && vmRuntime == "incus" {
			fmt.Fprintln(os.Stderr, "\033[33mWarning: vm.runtime is deprecated; use platform: incus in your context configuration instead. Support for vm.runtime will be removed in a future version.\033[0m")
			if err := rt.ConfigHandler.Set("platform", "incus"); err != nil {
				return werror.Wrap(err, "failed to set platform to incus")
			}
		} else {
			if err := rt.ConfigHandler.Set("platform", "docker"); err != nil {
				return werror.Wrap(err, "failed to set platform")
			}
		}
	}

	platform := rt.canonicalPlatform()
	defaultConfig := config.DefaultConfig
	if isDevMode && platform != "none" {
		defaultConfig = config.DefaultConfig_Dev
	}
	if err := rt.ConfigHandler.SetDefault(defaultConfig); err != nil {
		return werror.Wrap(err, "failed to set default config")
	}

	return nil
}

// inferDevPlatformOverride infers a dev-mode platform only when no explicit or configured platform exists.
// It preserves legacy colima+incus inference behavior and returns empty string when no inference is needed.
func (rt *Runtime) inferDevPlatformOverride(flagOverrides map[string]any) string {
	if !rt.ConfigHandler.IsDevMode(rt.ContextName) {
		return ""
	}
	if p, ok := explicitPlatformFromOverrides(flagOverrides); ok && p != "" {
		return ""
	}
	existingPlatform := rt.canonicalPlatform()
	if existingPlatform != "" {
		return ""
	}
	workstationRuntime := ""
	if driver, ok := flagOverrides["workstation.runtime"].(string); ok && driver != "" {
		workstationRuntime = driver
	}
	if workstationRuntime == "" {
		workstationRuntime = rt.ConfigHandler.GetString("workstation.runtime")
	}
	vmRuntime := ""
	if r, ok := flagOverrides["vm.runtime"].(string); ok {
		vmRuntime = r
	}
	if vmRuntime == "" {
		vmRuntime = rt.ConfigHandler.GetString("vm.runtime", "docker")
	}
	if workstationRuntime == "colima" && vmRuntime == "incus" {
		fmt.Fprintln(os.Stderr, "\033[33mWarning: vm.runtime is deprecated; use platform: incus in your context configuration instead. Support for vm.runtime will be removed in a future version.\033[0m")
		return "incus"
	}
	return "docker"
}

// canonicalPlatform returns the platform from config.
func (rt *Runtime) canonicalPlatform() string {
	return rt.ConfigHandler.GetString("platform")
}

// explicitPlatformFromOverrides extracts an explicitly provided platform selection from CLI overrides.
// It treats both "platform" and legacy "provider" override keys as explicit platform intent.
func explicitPlatformFromOverrides(flagOverrides map[string]any) (string, bool) {
	if flagOverrides == nil {
		return "", false
	}
	if p, ok := flagOverrides["platform"]; ok {
		if s, ok := p.(string); ok && s != "" {
			return s, true
		}
		return "", true
	}
	if p, ok := flagOverrides["provider"]; ok {
		if s, ok := p.(string); ok && s != "" {
			return s, true
		}
		return "", true
	}
	return "", false
}

// isWorkstationPlatform reports whether platform requires a workstation runtime.
func isWorkstationPlatform(platform string) bool {
	return platform == "docker" || platform == "incus"
}
