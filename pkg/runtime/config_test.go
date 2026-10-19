package runtime

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	cliconfig "github.com/windsorcli/cli/api/config/v1alpha2"
	"github.com/windsorcli/cli/pkg/runtime/config"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestRuntime_ApplyConfigDefaults(t *testing.T) {
	t.Run("AppliesDevDefaultsAfterConfigIsLoaded", func(t *testing.T) {
		// Given a runtime whose config is already loaded and dev mode is now known
		// (e.g. "dev: true" was only discoverable once values.yaml was read)
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime
		rt.ContextName = "remote"

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.IsLoadedFunc = func() bool {
			return true
		}
		mockConfigHandler.IsDevModeFunc = func(contextName string) bool {
			return true
		}
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			return ""
		}

		setDefaultCalled := false
		mockConfigHandler.SetDefaultFunc = func(cfg cliconfig.Context) error {
			setDefaultCalled = true
			return nil
		}

		// When ApplyConfigDefaults is called
		err := rt.ApplyConfigDefaults()

		// Then it should still apply dev-mode defaults rather than skip them
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if !setDefaultCalled {
			t.Error("Expected SetDefault to be called even though config was already loaded")
		}
	})

	t.Run("SkipsBaselineSelectionWhenNotLoadedAndDevModeUnknown", func(t *testing.T) {
		// Given a runtime that has not yet loaded config and whose dev mode cannot be
		// determined by name (only a persisted "dev: true" would answer it, and nothing
		// has been loaded yet)
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime
		rt.ContextName = "remote"

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.IsLoadedFunc = func() bool {
			return false
		}
		mockConfigHandler.IsDevModeFunc = func(contextName string) bool {
			return false
		}
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			return ""
		}

		setDefaultCalled := false
		mockConfigHandler.SetDefaultFunc = func(cfg cliconfig.Context) error {
			setDefaultCalled = true
			return nil
		}

		// When ApplyConfigDefaults is called
		err := rt.ApplyConfigDefaults()

		// Then it must not commit a baseline (e.g. platform: "none") before dev mode can
		// be confirmed one way or the other, since that baseline would look like an
		// explicit choice to a later call made once config is actually loaded
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if setDefaultCalled {
			t.Error("Expected SetDefault not to be called before load with dev mode still unknown")
		}
	})

	t.Run("BackfillNeverSkipsEvenWhenLoadFoundNothing", func(t *testing.T) {
		// Given a from-zero context (e.g. global mode with no windsor.yaml/values.yaml
		// anywhere), where LoadConfig has already run but found no files to merge, so
		// IsLoaded() stays false even though the load was genuinely attempted
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime
		rt.ContextName = "prod"

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.IsLoadedFunc = func() bool {
			return false
		}
		mockConfigHandler.IsDevModeFunc = func(contextName string) bool {
			return false
		}
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			return ""
		}

		setDefaultCalled := false
		mockConfigHandler.SetDefaultFunc = func(cfg cliconfig.Context) error {
			setDefaultCalled = true
			return nil
		}

		// When the post-load backfill runs (as ResolveConfig does right after LoadConfig)
		err := rt.setConfigDefaults()

		// Then it must still commit the baseline: by this point a load was attempted and
		// dev mode is as resolved as it will ever be, regardless of whether any file existed
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if !setDefaultCalled {
			t.Error("Expected SetDefault to be called on backfill even when nothing was found to load")
		}
	})

	t.Run("SetsDefaultsForDevMode", func(t *testing.T) {
		// Given a runtime in dev mode
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime
		rt.ContextName = "local"

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.IsLoadedFunc = func() bool {
			return false
		}
		mockConfigHandler.IsDevModeFunc = func(contextName string) bool {
			return true
		}
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			return ""
		}

		setDefaultCalled := false
		mockConfigHandler.SetDefaultFunc = func(cfg cliconfig.Context) error {
			setDefaultCalled = true
			return nil
		}

		setCalls := make(map[string]interface{})
		mockConfigHandler.SetFunc = func(key string, value interface{}) error {
			setCalls[key] = value
			return nil
		}

		// When ApplyConfigDefaults is called
		err := rt.ApplyConfigDefaults()

		// Then dev mode defaults should be set

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		if !setDefaultCalled {
			t.Error("Expected SetDefault to be called")
		}

		if setCalls["platform"] != "docker" {
			t.Error("Expected platform to be set to docker")
		}
	})

	t.Run("SetsIncusPlatformInDevModeWhenColimaIncus", func(t *testing.T) {
		// Given a runtime in dev mode with colima-incus configuration
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime
		rt.ContextName = "local"

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.IsLoadedFunc = func() bool {
			return false
		}
		mockConfigHandler.IsDevModeFunc = func(contextName string) bool {
			return true
		}
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "workstation.runtime" {
				return "colima"
			}
			if key == "vm.runtime" {
				return "incus"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}

		setCalls := make(map[string]interface{})
		mockConfigHandler.SetFunc = func(key string, value interface{}) error {
			setCalls[key] = value
			return nil
		}
		mockConfigHandler.SetDefaultFunc = func(cfg cliconfig.Context) error {
			return nil
		}

		// When ApplyConfigDefaults is called
		err := rt.ApplyConfigDefaults()

		// Then platform should be set to incus
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		if setCalls["platform"] != "incus" {
			t.Errorf("Expected platform to be set to incus, got: %v", setCalls["platform"])
		}
	})

	t.Run("PanicsWhenConfigHandlerNotAvailable", func(t *testing.T) {
		// Given a runtime with nil config handler
		rt := &Runtime{}

		// When ApplyConfigDefaults is called
		// Then it should panic
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected panic when ConfigHandler is nil")
			}
		}()
		_ = rt.ApplyConfigDefaults()
	})

	t.Run("SetsDefaultsForNonDevMode", func(t *testing.T) {
		// Given a runtime not in dev mode, confirmed by a completed config load
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime
		rt.ContextName = "prod"

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.IsLoadedFunc = func() bool {
			return true
		}
		mockConfigHandler.IsDevModeFunc = func(contextName string) bool {
			return false
		}
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			return ""
		}

		setDefaultCalled := false
		mockConfigHandler.SetDefaultFunc = func(cfg cliconfig.Context) error {
			setDefaultCalled = true
			return nil
		}

		mockConfigHandler.SetFunc = func(key string, value interface{}) error {
			return nil
		}

		// When ApplyConfigDefaults is called
		err := rt.ApplyConfigDefaults()

		// Then defaults should be set
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		if !setDefaultCalled {
			t.Error("Expected SetDefault to be called")
		}
	})

	t.Run("SetsWorkstationRuntimeForDockerDesktop", func(t *testing.T) {
		// Given a runtime in dev mode with Docker Desktop
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime
		rt.ContextName = "local"

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.IsLoadedFunc = func() bool {
			return false
		}
		mockConfigHandler.IsDevModeFunc = func(contextName string) bool {
			return true
		}

		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			return ""
		}

		mockConfigHandler.SetDefaultFunc = func(cfg cliconfig.Context) error {
			return nil
		}

		setCalls := make(map[string]interface{})
		mockConfigHandler.SetFunc = func(key string, value interface{}) error {
			setCalls[key] = value
			return nil
		}

		// When ApplyConfigDefaults is called
		err := rt.ApplyConfigDefaults()

		// Then workstation runtime should be set
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		if setCalls["workstation.runtime"] == nil {
			t.Error("Expected workstation.runtime to be set")
		}
	})

	t.Run("UsesFullConfigForDevModeWithNonDockerDesktop", func(t *testing.T) {
		// Given a runtime in dev mode with non-Docker Desktop VM driver
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime
		rt.ContextName = "local"

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.IsLoadedFunc = func() bool {
			return false
		}
		mockConfigHandler.IsDevModeFunc = func(contextName string) bool {
			return true
		}

		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "workstation.runtime" {
				return "colima"
			}
			return ""
		}

		setDefaultCalled := false
		mockConfigHandler.SetDefaultFunc = func(cfg cliconfig.Context) error {
			setDefaultCalled = true
			return nil
		}

		mockConfigHandler.SetFunc = func(key string, value interface{}) error {
			return nil
		}

		// When ApplyConfigDefaults is called
		err := rt.ApplyConfigDefaults()

		// Then full config should be set
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		if !setDefaultCalled {
			t.Error("Expected SetDefault to be called")
		}
	})

	t.Run("UsesStandardConfigForNonDevMode", func(t *testing.T) {
		// Given a runtime not in dev mode, confirmed by a completed config load
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime
		rt.ContextName = "prod"

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.IsLoadedFunc = func() bool {
			return true
		}
		mockConfigHandler.IsDevModeFunc = func(contextName string) bool {
			return false
		}

		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			return ""
		}

		setDefaultCalled := false
		mockConfigHandler.SetDefaultFunc = func(cfg cliconfig.Context) error {
			setDefaultCalled = true
			return nil
		}

		mockConfigHandler.SetFunc = func(key string, value interface{}) error {
			return nil
		}

		// When ApplyConfigDefaults is called
		err := rt.ApplyConfigDefaults()

		// Then standard config should be set
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		if !setDefaultCalled {
			t.Error("Expected SetDefault to be called")
		}
	})

	t.Run("ErrorWhenSetDefaultFails", func(t *testing.T) {
		// Given a runtime with a config handler that fails to set default
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime
		rt.ContextName = "local"

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.IsLoadedFunc = func() bool {
			return false
		}
		mockConfigHandler.IsDevModeFunc = func(contextName string) bool {
			return true
		}
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			return ""
		}

		mockConfigHandler.SetDefaultFunc = func(cfg cliconfig.Context) error {
			return fmt.Errorf("set default failed")
		}

		mockConfigHandler.SetFunc = func(key string, value interface{}) error {
			return nil
		}

		// When ApplyConfigDefaults is called
		err := rt.ApplyConfigDefaults()

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error when SetDefault fails")
		}

		if !strings.Contains(err.Error(), "failed to set default config") {
			t.Errorf("Expected error about set default config, got: %v", err)
		}
	})

	t.Run("ErrorWhenSetWorkstationRuntimeFails", func(t *testing.T) {
		// Given a runtime with a config handler that fails to set workstation runtime
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime
		rt.ContextName = "local"

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.IsLoadedFunc = func() bool {
			return false
		}
		mockConfigHandler.IsDevModeFunc = func(contextName string) bool {
			return true
		}
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			return ""
		}

		mockConfigHandler.SetDefaultFunc = func(cfg cliconfig.Context) error {
			return nil
		}

		mockConfigHandler.SetFunc = func(key string, value interface{}) error {
			if key == "workstation.runtime" {
				return fmt.Errorf("set workstation.runtime failed")
			}
			return nil
		}

		// When ApplyConfigDefaults is called
		err := rt.ApplyConfigDefaults()

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error when Set workstation.runtime fails")
		}

		if !strings.Contains(err.Error(), "failed to set workstation.runtime") {
			t.Errorf("Expected error about set workstation.runtime, got: %v", err)
		}
	})
	t.Run("ErrorWhenSetPlatformFails", func(t *testing.T) {
		// Given a runtime with a config handler that fails to set platform
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime
		rt.ContextName = "local"

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.IsLoadedFunc = func() bool {
			return false
		}
		mockConfigHandler.IsDevModeFunc = func(contextName string) bool {
			return true
		}
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			return ""
		}

		mockConfigHandler.SetDefaultFunc = func(cfg cliconfig.Context) error {
			return nil
		}

		mockConfigHandler.SetFunc = func(key string, value interface{}) error {
			if key == "platform" {
				return fmt.Errorf("set platform failed")
			}
			return nil
		}

		// When ApplyConfigDefaults is called
		err := rt.ApplyConfigDefaults()

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error when Set platform fails")
		}

		if !strings.Contains(err.Error(), "failed to set platform") {
			t.Errorf("Expected error about set platform, got: %v", err)
		}
	})

	t.Run("UsesDevConfigInDevMode", func(t *testing.T) {
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime
		rt.ContextName = "local"

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.IsLoadedFunc = func() bool {
			return false
		}
		mockConfigHandler.IsDevModeFunc = func(contextName string) bool {
			return true
		}
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			return ""
		}

		var setDefaultCalled bool
		mockConfigHandler.SetDefaultFunc = func(cfg cliconfig.Context) error {
			setDefaultCalled = true
			if cfg.Platform != "" {
				t.Error("Expected DefaultConfig_Dev with no platform set")
			}
			return nil
		}

		mockConfigHandler.SetFunc = func(key string, value interface{}) error {
			return nil
		}

		flagOverrides := map[string]any{
			"workstation.runtime": "colima",
		}

		err := rt.ApplyConfigDefaults(flagOverrides)

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if !setDefaultCalled {
			t.Error("Expected SetDefault to be called with DefaultConfig_Dev")
		}
	})

	t.Run("UsesDefaultConfigNoneWhenPlatformIsNone", func(t *testing.T) {
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime
		rt.ContextName = "test"

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.IsLoadedFunc = func() bool {
			return false
		}
		mockConfigHandler.IsDevModeFunc = func(contextName string) bool {
			return false
		}
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "platform" {
				return "none"
			}
			return ""
		}

		var setDefaultConfig cliconfig.Context
		mockConfigHandler.SetDefaultFunc = func(cfg cliconfig.Context) error {
			setDefaultConfig = cfg
			return nil
		}

		mockConfigHandler.SetFunc = func(key string, value interface{}) error {
			return nil
		}

		err := rt.ApplyConfigDefaults(nil)

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		if setDefaultConfig.Platform != "none" {
			t.Error("Expected DefaultConfig to be set with platform 'none'")
		}
		if setDefaultConfig.DNS != nil {
			t.Error("Expected DefaultConfig to have no DNS config")
		}
		if setDefaultConfig.Terraform != nil {
			t.Error("Expected DefaultConfig to have no terraform config (inline default at call site)")
		}
	})

	t.Run("IgnoresEmptyFlagOverrides", func(t *testing.T) {
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime
		rt.ContextName = "local"

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.IsLoadedFunc = func() bool {
			return false
		}
		mockConfigHandler.IsDevModeFunc = func(contextName string) bool {
			return true
		}
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			return ""
		}

		setDefaultCalled := false
		mockConfigHandler.SetDefaultFunc = func(cfg cliconfig.Context) error {
			setDefaultCalled = true
			return nil
		}

		mockConfigHandler.SetFunc = func(key string, value interface{}) error {
			return nil
		}

		flagOverrides := map[string]any{}

		// When ApplyConfigDefaults is called with empty flag overrides
		err := rt.ApplyConfigDefaults(flagOverrides)

		// Then defaults should still be applied (using OS default)
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		if !setDefaultCalled {
			t.Error("Expected SetDefault to be called even with empty flag overrides")
		}
	})

	t.Run("SetsPlatformToIncusWhenColimaIncusInFlagOverrides", func(t *testing.T) {
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime
		rt.ContextName = "local"

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.IsLoadedFunc = func() bool {
			return false
		}
		mockConfigHandler.IsDevModeFunc = func(contextName string) bool {
			return true
		}
		mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			return ""
		}

		var platformSet string
		mockConfigHandler.SetFunc = func(key string, value interface{}) error {
			if key == "platform" {
				platformSet = value.(string)
			}
			return nil
		}

		mockConfigHandler.SetDefaultFunc = func(cfg cliconfig.Context) error {
			return nil
		}

		flagOverrides := map[string]any{
			"workstation.runtime": "colima",
			"vm.runtime":          "incus",
		}

		err := rt.ApplyConfigDefaults(flagOverrides)

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		if platformSet != "incus" {
			t.Errorf("Expected platform to be set to 'incus', got: %s", platformSet)
		}
	})

	t.Run("SkipsWorkstationRuntimeDefaultWhenPlatformIsNonWorkstationViaFlagOverride", func(t *testing.T) {
		for _, platform := range []string{"aws", "azure", "gcp", "metal", "hetzner", "none", "hyperv", "proxmox"} {
			t.Run(platform, func(t *testing.T) {
				mocks := setupRuntimeMocks(t)
				rt := mocks.Runtime
				rt.ContextName = "local"

				mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
				mockConfigHandler.IsLoadedFunc = func() bool { return false }
				mockConfigHandler.IsDevModeFunc = func(_ string) bool { return true }
				mockConfigHandler.GetStringFunc = func(_ string, defaultValue ...string) string {
					if len(defaultValue) > 0 {
						return defaultValue[0]
					}
					return ""
				}
				mockConfigHandler.SetDefaultFunc = func(_ cliconfig.Context) error { return nil }

				setCalls := make(map[string]any)
				mockConfigHandler.SetFunc = func(key string, value any) error {
					setCalls[key] = value
					return nil
				}

				err := rt.ApplyConfigDefaults(map[string]any{"platform": platform})
				if err != nil {
					t.Fatalf("Expected no error, got: %v", err)
				}
				if _, ok := setCalls["workstation.runtime"]; ok {
					t.Errorf("Expected workstation.runtime NOT to be set for platform=%q, got setCalls=%v", platform, setCalls)
				}
			})
		}
	})

	t.Run("SkipsWorkstationRuntimeDefaultWhenPlatformIsNonWorkstationViaSavedConfig", func(t *testing.T) {
		for _, platform := range []string{"aws", "azure", "gcp", "metal", "hetzner", "none", "hyperv", "proxmox"} {
			t.Run(platform, func(t *testing.T) {
				mocks := setupRuntimeMocks(t)
				rt := mocks.Runtime
				rt.ContextName = "local"

				mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
				mockConfigHandler.IsLoadedFunc = func() bool { return false }
				mockConfigHandler.IsDevModeFunc = func(_ string) bool { return true }
				mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
					if key == "platform" {
						return platform
					}
					if len(defaultValue) > 0 {
						return defaultValue[0]
					}
					return ""
				}
				mockConfigHandler.SetDefaultFunc = func(_ cliconfig.Context) error { return nil }

				setCalls := make(map[string]any)
				mockConfigHandler.SetFunc = func(key string, value any) error {
					setCalls[key] = value
					return nil
				}

				if err := rt.ApplyConfigDefaults(); err != nil {
					t.Fatalf("Expected no error, got: %v", err)
				}
				if _, ok := setCalls["workstation.runtime"]; ok {
					t.Errorf("Expected workstation.runtime NOT to be set for platform=%q, got setCalls=%v", platform, setCalls)
				}
			})
		}
	})

	t.Run("DefaultsWorkstationRuntimeWhenPlatformIsWorkstationViaFlagOverride", func(t *testing.T) {
		for _, platform := range []string{"docker", "incus"} {
			t.Run(platform, func(t *testing.T) {
				mocks := setupRuntimeMocks(t)
				rt := mocks.Runtime
				rt.ContextName = "local"

				mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
				mockConfigHandler.IsLoadedFunc = func() bool { return false }
				mockConfigHandler.IsDevModeFunc = func(_ string) bool { return true }
				mockConfigHandler.GetStringFunc = func(_ string, defaultValue ...string) string {
					if len(defaultValue) > 0 {
						return defaultValue[0]
					}
					return ""
				}
				mockConfigHandler.SetDefaultFunc = func(_ cliconfig.Context) error { return nil }

				setCalls := make(map[string]any)
				mockConfigHandler.SetFunc = func(key string, value any) error {
					setCalls[key] = value
					return nil
				}

				if err := rt.ApplyConfigDefaults(map[string]any{"platform": platform}); err != nil {
					t.Fatalf("Expected no error, got: %v", err)
				}
				if _, ok := setCalls["workstation.runtime"]; !ok {
					t.Errorf("Expected workstation.runtime to be set for platform=%q, got setCalls=%v", platform, setCalls)
				}
			})
		}
	})

	t.Run("DefaultsWorkstationRuntimeWhenPlatformIsWorkstationViaSavedConfig", func(t *testing.T) {
		for _, platform := range []string{"docker", "incus"} {
			t.Run(platform, func(t *testing.T) {
				mocks := setupRuntimeMocks(t)
				rt := mocks.Runtime
				rt.ContextName = "local"

				mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
				mockConfigHandler.IsLoadedFunc = func() bool { return false }
				mockConfigHandler.IsDevModeFunc = func(_ string) bool { return true }
				mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
					if key == "platform" {
						return platform
					}
					if len(defaultValue) > 0 {
						return defaultValue[0]
					}
					return ""
				}
				mockConfigHandler.SetDefaultFunc = func(_ cliconfig.Context) error { return nil }

				setCalls := make(map[string]any)
				mockConfigHandler.SetFunc = func(key string, value any) error {
					setCalls[key] = value
					return nil
				}

				if err := rt.ApplyConfigDefaults(); err != nil {
					t.Fatalf("Expected no error, got: %v", err)
				}
				if _, ok := setCalls["workstation.runtime"]; !ok {
					t.Errorf("Expected workstation.runtime to be set for platform=%q, got setCalls=%v", platform, setCalls)
				}
			})
		}
	})

	t.Run("DefaultsWorkstationRuntimeForFlagOverridePlatformOutsideDevMode", func(t *testing.T) {
		for _, platform := range []string{"docker", "incus"} {
			t.Run(platform, func(t *testing.T) {
				mocks := setupRuntimeMocks(t)
				rt := mocks.Runtime
				rt.ContextName = "docker"

				mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
				// Config is loaded here, matching ResolveConfig's post-load call to
				// setConfigDefaults, which is the call bootstrap actually reaches.
				mockConfigHandler.IsLoadedFunc = func() bool { return true }
				mockConfigHandler.IsDevModeFunc = func(_ string) bool { return false }
				mockConfigHandler.GetStringFunc = func(_ string, defaultValue ...string) string {
					if len(defaultValue) > 0 {
						return defaultValue[0]
					}
					return ""
				}
				mockConfigHandler.SetDefaultFunc = func(_ cliconfig.Context) error { return nil }

				setCalls := make(map[string]any)
				mockConfigHandler.SetFunc = func(key string, value any) error {
					setCalls[key] = value
					return nil
				}

				if err := rt.ApplyConfigDefaults(map[string]any{"platform": platform}); err != nil {
					t.Fatalf("Expected no error, got: %v", err)
				}
				if _, ok := setCalls["workstation.runtime"]; !ok {
					t.Errorf("Expected workstation.runtime to be set for platform=%q outside dev mode, got setCalls=%v", platform, setCalls)
				}
			})
		}
	})

	t.Run("SkipsWorkstationRuntimeDefaultWhenNoPlatformOutsideDevMode", func(t *testing.T) {
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime
		rt.ContextName = "docker"

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.IsLoadedFunc = func() bool { return true }
		mockConfigHandler.IsDevModeFunc = func(_ string) bool { return false }
		mockConfigHandler.GetStringFunc = func(_ string, defaultValue ...string) string {
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}
		mockConfigHandler.SetDefaultFunc = func(_ cliconfig.Context) error { return nil }

		setCalls := make(map[string]any)
		mockConfigHandler.SetFunc = func(key string, value any) error {
			setCalls[key] = value
			return nil
		}

		if err := rt.ApplyConfigDefaults(); err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if _, ok := setCalls["workstation.runtime"]; ok {
			t.Errorf("Expected workstation.runtime NOT to be set with no platform outside dev mode, got setCalls=%v", setCalls)
		}
	})

	t.Run("SkipsWorkstationRuntimeDefaultForPersistedPlatformOutsideDevMode", func(t *testing.T) {
		for _, platform := range []string{"docker", "incus"} {
			t.Run(platform, func(t *testing.T) {
				mocks := setupRuntimeMocks(t)
				rt := mocks.Runtime
				rt.ContextName = "docker"

				mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
				mockConfigHandler.IsLoadedFunc = func() bool { return true }
				mockConfigHandler.IsDevModeFunc = func(_ string) bool { return false }
				mockConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
					if key == "platform" {
						return platform
					}
					if len(defaultValue) > 0 {
						return defaultValue[0]
					}
					return ""
				}
				mockConfigHandler.SetDefaultFunc = func(_ cliconfig.Context) error { return nil }

				setCalls := make(map[string]any)
				mockConfigHandler.SetFunc = func(key string, value any) error {
					setCalls[key] = value
					return nil
				}

				// No --platform flag this run; platform comes only from persisted config.
				if err := rt.ApplyConfigDefaults(); err != nil {
					t.Fatalf("Expected no error, got: %v", err)
				}
				if _, ok := setCalls["workstation.runtime"]; ok {
					t.Errorf("Expected workstation.runtime NOT to be set for persisted platform=%q outside dev mode, got setCalls=%v", platform, setCalls)
				}
			})
		}
	})

	t.Run("DefaultsWorkstationRuntimeWhenNoPlatformInDevMode", func(t *testing.T) {
		mocks := setupRuntimeMocks(t)
		rt := mocks.Runtime
		rt.ContextName = "local"

		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.IsLoadedFunc = func() bool { return false }
		mockConfigHandler.IsDevModeFunc = func(_ string) bool { return true }
		mockConfigHandler.GetStringFunc = func(_ string, defaultValue ...string) string {
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}
		mockConfigHandler.SetDefaultFunc = func(_ cliconfig.Context) error { return nil }

		setCalls := make(map[string]any)
		mockConfigHandler.SetFunc = func(key string, value any) error {
			setCalls[key] = value
			return nil
		}

		if err := rt.ApplyConfigDefaults(); err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if _, ok := setCalls["workstation.runtime"]; !ok {
			t.Errorf("Expected workstation.runtime to be set when no platform is chosen, got setCalls=%v", setCalls)
		}
	})

	t.Run("DefaultsWorkstationRuntimeByHostOS", func(t *testing.T) {
		for goos, expected := range map[string]string{"darwin": "docker-desktop", "windows": "docker-desktop", "linux": "docker"} {
			// Given a dev-mode runtime on the host OS
			mocks := setupRuntimeMocks(t)
			rt := mocks.Runtime
			rt.ContextName = "local"
			rt.Shims.Goos = func() string { return goos }

			mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
			mockConfigHandler.IsLoadedFunc = func() bool { return false }
			mockConfigHandler.IsDevModeFunc = func(_ string) bool { return true }
			mockConfigHandler.GetStringFunc = func(_ string, defaultValue ...string) string {
				if len(defaultValue) > 0 {
					return defaultValue[0]
				}
				return ""
			}
			mockConfigHandler.SetDefaultFunc = func(_ cliconfig.Context) error { return nil }

			setCalls := make(map[string]any)
			mockConfigHandler.SetFunc = func(key string, value any) error {
				setCalls[key] = value
				return nil
			}

			// When ApplyConfigDefaults is called
			if err := rt.ApplyConfigDefaults(); err != nil {
				t.Fatalf("Expected no error, got: %v", err)
			}

			// Then workstation.runtime is the default of that OS
			if setCalls["workstation.runtime"] != expected {
				t.Errorf("Expected workstation.runtime %q on %s, got %v", expected, goos, setCalls["workstation.runtime"])
			}
		}
	})
}

func TestRuntime_SaveConfig(t *testing.T) {
	t.Run("SavesWorkstationStateThenValues", func(t *testing.T) {
		// Given a runtime whose config handler records save calls
		mocks := setupRuntimeMocks(t)
		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		var calls []string
		mockConfigHandler.SaveWorkstationStateFunc = func() error {
			calls = append(calls, "workstation")
			return nil
		}
		mockConfigHandler.SaveConfigFunc = func(overwrite ...bool) error {
			calls = append(calls, fmt.Sprintf("values overwrite=%v", overwrite))
			return nil
		}

		// When SaveConfig is called with overwrite
		err := mocks.Runtime.SaveConfig(true)

		// Then workstation state is saved before values, and overwrite is passed through
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		want := []string{"workstation", "values overwrite=[true]"}
		if !reflect.DeepEqual(calls, want) {
			t.Errorf("Expected calls %v, got %v", want, calls)
		}
	})

	t.Run("ReturnsTheWorkstationStateError", func(t *testing.T) {
		// Given a config handler that fails to save workstation state
		mocks := setupRuntimeMocks(t)
		mockConfigHandler := mocks.ConfigHandler.(*config.MockConfigHandler)
		mockConfigHandler.SaveWorkstationStateFunc = func() error {
			return fmt.Errorf("disk full")
		}

		// When SaveConfig is called
		err := mocks.Runtime.SaveConfig()

		// Then the error is returned
		if err == nil || !strings.Contains(err.Error(), "disk full") {
			t.Errorf("Expected the workstation state error, got %v", err)
		}
	})
}
