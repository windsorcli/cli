// The config functions of the Runtime run the config pipeline of a command.
// It provides the config defaults, the config load, the flag overrides, and the config save.
// In dev mode, the defaults also set the platform and the workstation runtime if the config does not set them.
// ResolveConfig runs the full pipeline one time for each command.

package runtime

import (
	"fmt"
	"maps"
	"os"

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

// ResolveConfig runs the full config pipeline: pre-load defaults, load from disk, post-load
// defaults, flag overrides, then dev-mode platform normalization. Call once per command.
// Mutates flagOverrides when inferring platform. Returns error on config load or set failure.
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
		return fmt.Errorf("failed to apply config defaults: %w", err)
	}
	if err := rt.ConfigHandler.LoadConfig(); err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	if err := rt.setConfigDefaults(preLoadOverrides); err != nil {
		return fmt.Errorf("failed to apply post-load config defaults: %w", err)
	}
	for key, value := range flagOverrides {
		if key == "provider" {
			continue
		}
		if key == "platform" && !hasExplicitPlatform {
			continue
		}
		if err := rt.ConfigHandler.Set(key, value); err != nil {
			return fmt.Errorf("failed to set %s: %w", key, err)
		}
	}
	if hasExplicitPlatform && explicitPlatform != "" {
		if err := rt.ConfigHandler.Set("platform", explicitPlatform); err != nil {
			return fmt.Errorf("failed to set platform: %w", err)
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
				return fmt.Errorf("failed to set platform to incus: %w", err)
			}
		}
	}
	return nil
}

// SaveConfig delegates persistence to the config handler. Workstation-managed keys are written to
// .windsor/contexts/<context>/workstation.yaml and excluded from values.yaml.
func (rt *Runtime) SaveConfig(overwrite ...bool) error {
	if rt.ConfigHandler == nil {
		return fmt.Errorf("runtime not fully initialized: config handler missing")
	}
	if err := rt.ConfigHandler.SaveWorkstationState(); err != nil {
		return fmt.Errorf("failed to save workstation state: %w", err)
	}
	return rt.ConfigHandler.SaveConfig(overwrite...)
}

// =============================================================================
// Private Methods
// =============================================================================

// setConfigDefaults applies the shared config defaults.
// ApplyConfigDefaults calls this once, before ResolveConfig loads the config file.
// ResolveConfig calls this again right after the load. The second call never defers.
//
// It sets workstation.runtime in dev mode. It also sets workstation.runtime when
// this run passes an explicit docker or incus platform. A platform saved from an
// earlier run does not trigger this default.
func (rt *Runtime) setConfigDefaults(flagOverrides ...map[string]any) error {
	existingPlatform := rt.canonicalPlatform()
	isDevMode := rt.ConfigHandler.IsDevMode(rt.ContextName)

	if isDevMode {
		if err := rt.ConfigHandler.Set("dev", true); err != nil {
			return fmt.Errorf("failed to set dev mode: %w", err)
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
			return fmt.Errorf("failed to set workstation.runtime: %w", err)
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
				return fmt.Errorf("failed to set platform from overrides: %w", err)
			}
		} else if workstationRuntime == "colima" && vmRuntime == "incus" {
			fmt.Fprintln(os.Stderr, "\033[33mWarning: vm.runtime is deprecated; use platform: incus in your context configuration instead. Support for vm.runtime will be removed in a future version.\033[0m")
			if err := rt.ConfigHandler.Set("platform", "incus"); err != nil {
				return fmt.Errorf("failed to set platform to incus: %w", err)
			}
		} else {
			if err := rt.ConfigHandler.Set("platform", "docker"); err != nil {
				return fmt.Errorf("failed to set platform: %w", err)
			}
		}
	}

	platform := rt.canonicalPlatform()
	defaultConfig := config.DefaultConfig
	if isDevMode && platform != "none" {
		defaultConfig = config.DefaultConfig_Dev
	}
	if err := rt.ConfigHandler.SetDefault(defaultConfig); err != nil {
		return fmt.Errorf("failed to set default config: %w", err)
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
