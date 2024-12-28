package docker

import (
	"testing"
)

func TestDockerConfig_Merge(t *testing.T) {
	t.Run("MergeWithNonNilValues", func(t *testing.T) {
		base := &DockerConfig{
			Enabled:     ptrBool(true),
			NetworkCIDR: ptrString("192.168.1.0/24"),
			Registries: map[string]RegistryConfig{
				"base-registry1": {Remote: "base-remote1", Local: "base-local1", Hostname: "base-hostname1"},
				"base-registry2": {Remote: "base-remote2", Local: "base-local2", Hostname: "base-hostname2"},
			},
		}

		overlay := &DockerConfig{
			Enabled:     ptrBool(false),
			NetworkCIDR: ptrString("10.0.0.0/16"),
			Registries: map[string]RegistryConfig{
				"base-registry1": {Remote: "overlay-remote1", Local: "overlay-local1", Hostname: "overlay-hostname1"},
				"new-registry":   {Remote: "overlay-remote2", Local: "overlay-local2", Hostname: "overlay-hostname2"},
			},
		}

		base.Merge(overlay)

		if base.Enabled == nil || *base.Enabled != false {
			t.Errorf("Enabled mismatch: expected %v, got %v", false, *base.Enabled)
		}
		if base.NetworkCIDR == nil || *base.NetworkCIDR != "10.0.0.0/16" {
			t.Errorf("NetworkCIDR mismatch: expected %v, got %v", "10.0.0.0/16", *base.NetworkCIDR)
		}
		if len(base.Registries) != 3 {
			t.Errorf("Registries length mismatch: expected %v, got %v", 3, len(base.Registries))
		}

		// Create a map to verify registries without relying on order
		expectedRegistries := map[string]RegistryConfig{
			"base-registry1": {Remote: "overlay-remote1", Local: "overlay-local1", Hostname: "overlay-hostname1"},
			"base-registry2": {Remote: "base-remote2", Local: "base-local2", Hostname: "base-hostname2"},
			"new-registry":   {Remote: "overlay-remote2", Local: "overlay-local2", Hostname: "overlay-hostname2"},
		}

		for name, registry := range base.Registries {
			expected, exists := expectedRegistries[name]
			if !exists {
				t.Errorf("Unexpected registry: %v", name)
				continue
			}
			if registry.Remote != expected.Remote || registry.Local != expected.Local || registry.Hostname != expected.Hostname {
				t.Errorf("Registry %v mismatch: expected remote %v, local %v, hostname %v, got remote %v, local %v, hostname %v", name, expected.Remote, expected.Local, expected.Hostname, registry.Remote, registry.Local, registry.Hostname)
			}
		}
	})

	t.Run("MergeWithNilValues", func(t *testing.T) {
		base := &DockerConfig{
			Enabled:     ptrBool(true),
			NetworkCIDR: ptrString("192.168.1.0/24"),
			Registries: map[string]RegistryConfig{
				"base-registry1": {Remote: "base-remote1", Local: "base-local1", Hostname: "base-hostname1"},
			},
		}

		overlay := &DockerConfig{
			Enabled:     nil,
			NetworkCIDR: nil,
			Registries:  nil,
		}

		base.Merge(overlay)

		if base.Enabled == nil || *base.Enabled != true {
			t.Errorf("Enabled mismatch: expected %v, got %v", true, *base.Enabled)
		}
		if base.NetworkCIDR == nil || *base.NetworkCIDR != "192.168.1.0/24" {
			t.Errorf("NetworkCIDR mismatch: expected %v, got %v", "192.168.1.0/24", *base.NetworkCIDR)
		}
		if len(base.Registries) != 1 || base.Registries["base-registry1"].Remote != "base-remote1" || base.Registries["base-registry1"].Local != "base-local1" || base.Registries["base-registry1"].Hostname != "base-hostname1" {
			t.Errorf("Registries mismatch: expected %v, got %v", "base-registry1", base.Registries["base-registry1"])
		}
	})
}

func TestDockerConfig_Copy(t *testing.T) {
	t.Run("CopyWithNonNilValues", func(t *testing.T) {
		original := &DockerConfig{
			Enabled:     ptrBool(true),
			NetworkCIDR: ptrString("192.168.1.0/24"),
			Registries: map[string]RegistryConfig{
				"registry1": {Remote: "remote1", Local: "local1", Hostname: "hostname1"},
				"registry2": {Remote: "remote2", Local: "local2", Hostname: "hostname2"},
			},
		}

		copy := original.Copy()

		if original.Enabled == nil || copy.Enabled == nil || *original.Enabled != *copy.Enabled {
			t.Errorf("Enabled mismatch: expected %v, got %v", *original.Enabled, *copy.Enabled)
		}
		if original.NetworkCIDR == nil || copy.NetworkCIDR == nil || *original.NetworkCIDR != *copy.NetworkCIDR {
			t.Errorf("NetworkCIDR mismatch: expected %v, got %v", *original.NetworkCIDR, *copy.NetworkCIDR)
		}
		if len(original.Registries) != len(copy.Registries) {
			t.Errorf("Registries length mismatch: expected %d, got %d", len(original.Registries), len(copy.Registries))
		}
		for name, registry := range original.Registries {
			if registry.Remote != copy.Registries[name].Remote {
				t.Errorf("Registry Remote mismatch for %v: expected %v, got %v", name, registry.Remote, copy.Registries[name].Remote)
			}
			if registry.Local != copy.Registries[name].Local {
				t.Errorf("Registry Local mismatch for %v: expected %v, got %v", name, registry.Local, copy.Registries[name].Local)
			}
			if registry.Hostname != copy.Registries[name].Hostname {
				t.Errorf("Registry Hostname mismatch for %v: expected %v, got %v", name, registry.Hostname, copy.Registries[name].Hostname)
			}
		}

		// Modify the copy and ensure original is unchanged
		copy.Enabled = ptrBool(false)
		if original.Enabled == nil || *original.Enabled == *copy.Enabled {
			t.Errorf("Original Enabled was modified: expected %v, got %v", true, *copy.Enabled)
		}

		copy.Registries["new-registry"] = RegistryConfig{Remote: "new-remote", Local: "new-local", Hostname: "new-hostname"}
		if len(original.Registries) == len(copy.Registries) {
			t.Errorf("Original Registries were modified: expected length %d, got %d", len(original.Registries), len(copy.Registries))
		}
	})

	t.Run("CopyNil", func(t *testing.T) {
		var original *DockerConfig = nil
		mockCopy := original.Copy()
		if mockCopy != nil {
			t.Errorf("Mock copy should be nil, got %v", mockCopy)
		}
	})
}
