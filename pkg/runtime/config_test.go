package runtime_test

import (
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/runtime/config"
)

// =============================================================================
// Test Setup
// =============================================================================

// setupConfigStore backs the Get, GetString, and Set functions of the mock handler with a map.
func setupConfigStore(handler *config.MockConfigHandler, initial map[string]any) map[string]any {
	store := map[string]any{}
	for k, v := range initial {
		store[k] = v
	}
	handler.GetFunc = func(key string) any {
		return store[key]
	}
	handler.GetStringFunc = func(key string, defaultValue ...string) string {
		if v, ok := store[key].(string); ok {
			return v
		}
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return ""
	}
	handler.SetFunc = func(key string, value any) error {
		store[key] = value
		return nil
	}
	return store
}

// captureStderr returns what fn writes to os.Stderr.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	original := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = original }()
	fn()
	_ = w.Close()
	out, _ := io.ReadAll(r)
	return string(out)
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestRuntime_ResolveConfig(t *testing.T) {
	t.Run("LoadsConfigAndSetsFlagOverrides", func(t *testing.T) {
		// Given a runtime outside dev mode and a flag override
		mocks := setupRuntimeMocks(t)
		handler := mocks.ConfigHandler.(*config.MockConfigHandler)
		store := setupConfigStore(handler, nil)
		loaded := false
		handler.LoadConfigFunc = func() error {
			loaded = true
			return nil
		}

		// When ResolveConfig is called
		err := mocks.Runtime.ResolveConfig(map[string]any{"cluster.workers.count": 2})

		// Then the config is loaded and the override is set
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !loaded {
			t.Error("Expected LoadConfig to be called")
		}
		if store["cluster.workers.count"] != 2 {
			t.Errorf("Expected the override to be set, got %v", store["cluster.workers.count"])
		}
	})

	t.Run("AcceptsNilFlagOverrides", func(t *testing.T) {
		// Given a runtime outside dev mode
		mocks := setupRuntimeMocks(t)
		setupConfigStore(mocks.ConfigHandler.(*config.MockConfigHandler), nil)

		// When ResolveConfig is called with nil overrides
		err := mocks.Runtime.ResolveConfig(nil)

		// Then no error is returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("SetsProviderOverrideAsPlatform", func(t *testing.T) {
		// Given a legacy provider override
		mocks := setupRuntimeMocks(t)
		store := setupConfigStore(mocks.ConfigHandler.(*config.MockConfigHandler), nil)

		// When ResolveConfig is called
		err := mocks.Runtime.ResolveConfig(map[string]any{"provider": "aws"})

		// Then the platform is set and the provider key is not
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if store["platform"] != "aws" {
			t.Errorf("Expected platform aws, got %v", store["platform"])
		}
		if _, ok := store["provider"]; ok {
			t.Errorf("Expected no provider key, got %v", store["provider"])
		}
	})

	t.Run("SetsDNSDefaultsForDockerDesktop", func(t *testing.T) {
		// Given a config with the docker-desktop workstation runtime
		mocks := setupRuntimeMocks(t)
		store := setupConfigStore(mocks.ConfigHandler.(*config.MockConfigHandler), map[string]any{"workstation.runtime": "docker-desktop"})

		// When ResolveConfig is called
		err := mocks.Runtime.ResolveConfig(nil)

		// Then the DNS domain and the DNS address get their defaults
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if store["dns.domain"] != "test" {
			t.Errorf("Expected dns.domain test, got %v", store["dns.domain"])
		}
		if store["workstation.dns.address"] != "127.0.0.1" {
			t.Errorf("Expected workstation.dns.address 127.0.0.1, got %v", store["workstation.dns.address"])
		}
	})

	t.Run("KeepsDNSDefaultsUnsetWhenDNSIsDisabled", func(t *testing.T) {
		// Given a workstation config with DNS disabled
		mocks := setupRuntimeMocks(t)
		store := setupConfigStore(mocks.ConfigHandler.(*config.MockConfigHandler), map[string]any{
			"workstation.runtime": "docker-desktop",
			"dns.enabled":         false,
		})

		// When ResolveConfig is called
		err := mocks.Runtime.ResolveConfig(nil)

		// Then the DNS defaults are not set
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if _, ok := store["dns.domain"]; ok {
			t.Errorf("Expected no dns.domain, got %v", store["dns.domain"])
		}
	})

	t.Run("SetsIncusPlatformForColimaIncusInDevMode", func(t *testing.T) {
		// Given a dev-mode config with colima and the incus vm.runtime
		mocks := setupRuntimeMocks(t)
		handler := mocks.ConfigHandler.(*config.MockConfigHandler)
		handler.IsDevModeFunc = func(string) bool { return true }
		store := setupConfigStore(handler, map[string]any{
			"workstation.runtime": "colima",
			"vm.runtime":          "incus",
		})

		// When ResolveConfig is called
		err := mocks.Runtime.ResolveConfig(nil)

		// Then the platform is incus
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if store["platform"] != "incus" {
			t.Errorf("Expected platform incus, got %v", store["platform"])
		}
	})

	t.Run("ErrorWhenLoadConfigFails", func(t *testing.T) {
		// Given a config handler that fails to load
		mocks := setupRuntimeMocks(t)
		handler := mocks.ConfigHandler.(*config.MockConfigHandler)
		setupConfigStore(handler, nil)
		handler.LoadConfigFunc = func() error {
			return fmt.Errorf("bad yaml")
		}

		// When ResolveConfig is called
		err := mocks.Runtime.ResolveConfig(nil)

		// Then the load error is returned with a breadcrumb
		if err == nil || !strings.Contains(err.Error(), "failed to load config: bad yaml") {
			t.Errorf("Expected the load config error, got %v", err)
		}
	})

	t.Run("ErrorWhenBothRegistriesKeysAreSet", func(t *testing.T) {
		// Given a context that sets registries and docker.registries
		mocks := setupRuntimeMocks(t)
		handler := mocks.ConfigHandler.(*config.MockConfigHandler)
		setupConfigStore(handler, nil)
		handler.CheckRegistriesKeysFunc = func() (bool, error) {
			return false, fmt.Errorf("The context sets both registries and docker.registries.")
		}

		// When ResolveConfig is called
		err := mocks.Runtime.ResolveConfig(nil)

		// Then the error names both keys
		if err == nil || !strings.Contains(err.Error(), "both registries and docker.registries") {
			t.Errorf("Expected the registries keys error, got %v", err)
		}
	})

	t.Run("WarnsWhenOnlyDockerRegistriesIsSet", func(t *testing.T) {
		// Given a context that sets only docker.registries
		mocks := setupRuntimeMocks(t)
		handler := mocks.ConfigHandler.(*config.MockConfigHandler)
		setupConfigStore(handler, nil)
		handler.CheckRegistriesKeysFunc = func() (bool, error) { return true, nil }

		// When ResolveConfig is called
		var err error
		stderr := captureStderr(t, func() { err = mocks.Runtime.ResolveConfig(nil) })

		// Then the command goes on and the warning names registries
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !strings.Contains(stderr, "docker.registries is deprecated") || !strings.Contains(stderr, "top-level registries key") {
			t.Errorf("Expected the docker.registries warning, got %q", stderr)
		}
	})

	t.Run("ErrorWhenSetOverrideFails", func(t *testing.T) {
		// Given a config handler that fails to set one override
		mocks := setupRuntimeMocks(t)
		handler := mocks.ConfigHandler.(*config.MockConfigHandler)
		setupConfigStore(handler, nil)
		handler.SetFunc = func(key string, value any) error {
			if key == "cluster.workers.count" {
				return fmt.Errorf("invalid value")
			}
			return nil
		}

		// When ResolveConfig is called with that override
		err := mocks.Runtime.ResolveConfig(map[string]any{"cluster.workers.count": "x"})

		// Then the error names the key
		if err == nil || !strings.Contains(err.Error(), "failed to set cluster.workers.count") {
			t.Errorf("Expected the set override error, got %v", err)
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
