package docker

import (
	"testing"
)

func TestDockerConfig_Merge(t *testing.T) {
	t.Run("MergeWithNonNilValues", func(t *testing.T) {
		base := &DockerConfig{
			Enabled:     ptrBool(true),
			RegistryURL: "base-registry-url",
			Registries: map[string]RegistryConfig{
				"base-registry1": {Remote: "base-remote1", Local: "base-local1", HostName: "base-hostname1"},
				"base-registry2": {Remote: "base-remote2", Local: "base-local2", HostName: "base-hostname2"},
			},
		}

		overlay := &DockerConfig{
			Enabled:     ptrBool(false),
			RegistryURL: "overlay-registry-url",
			Registries: map[string]RegistryConfig{
				"base-registry1": {Remote: "overlay-remote1", Local: "overlay-local1", HostName: "overlay-hostname1"},
				"new-registry":   {Remote: "overlay-remote2", Local: "overlay-local2", HostName: "overlay-hostname2"},
			},
		}

		base.Merge(overlay)

		if base.Enabled == nil || *base.Enabled != false {
			t.Errorf("Enabled mismatch: expected %v, got %v", false, *base.Enabled)
		}
		if base.RegistryURL != "overlay-registry-url" {
			t.Errorf("RegistryURL mismatch: expected %v, got %v", "overlay-registry-url", base.RegistryURL)
		}
		if len(base.Registries) != 2 {
			t.Errorf("Registries length mismatch: expected %v, got %v", 2, len(base.Registries))
		}

		// Create a map to verify registries without relying on order
		expectedRegistries := map[string]RegistryConfig{
			"base-registry1": {Remote: "overlay-remote1", Local: "overlay-local1", HostName: "overlay-hostname1"},
			"new-registry":   {Remote: "overlay-remote2", Local: "overlay-local2", HostName: "overlay-hostname2"},
		}

		for name, registry := range base.Registries {
			expected, exists := expectedRegistries[name]
			if !exists {
				t.Errorf("Unexpected registry: %v", name)
				continue
			}
			if registry.Remote != expected.Remote || registry.Local != expected.Local || registry.HostName != expected.HostName {
				t.Errorf("Registry %v mismatch: expected remote %v, local %v, hostname %v, got remote %v, local %v, hostname %v", name, expected.Remote, expected.Local, expected.HostName, registry.Remote, registry.Local, registry.HostName)
			}
		}
	})

	t.Run("MergeWithNilValues", func(t *testing.T) {
		base := &DockerConfig{
			Enabled:     ptrBool(true),
			RegistryURL: "base-registry-url",
			Registries: map[string]RegistryConfig{
				"base-registry1": {Remote: "base-remote1", Local: "base-local1", HostName: "base-hostname1"},
			},
		}

		overlay := &DockerConfig{
			Enabled:     nil,
			RegistryURL: "",
			Registries:  nil,
		}

		base.Merge(overlay)

		if base.Enabled == nil || *base.Enabled != true {
			t.Errorf("Enabled mismatch: expected %v, got %v", true, *base.Enabled)
		}
		if base.RegistryURL != "base-registry-url" {
			t.Errorf("RegistryURL mismatch: expected %v, got %v", "base-registry-url", base.RegistryURL)
		}
		if len(base.Registries) != 1 || base.Registries["base-registry1"].Remote != "base-remote1" || base.Registries["base-registry1"].Local != "base-local1" || base.Registries["base-registry1"].HostName != "base-hostname1" {
			t.Errorf("Registries mismatch: expected %v, got %v", "base-registry1", base.Registries["base-registry1"])
		}
	})
}

func TestDockerConfig_Copy(t *testing.T) {
	t.Run("CopyWithNonNilValues", func(t *testing.T) {
		original := &DockerConfig{
			Enabled: ptrBool(true),
			Registries: map[string]RegistryConfig{
				"registry1": {Remote: "remote1", Local: "local1", HostName: "hostname1"},
				"registry2": {Remote: "remote2", Local: "local2", HostName: "hostname2"},
			},
		}

		copy := original.Copy()

		if original.Enabled == nil || copy.Enabled == nil || *original.Enabled != *copy.Enabled {
			t.Errorf("Enabled mismatch: expected %v, got %v", *original.Enabled, *copy.Enabled)
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
			if registry.HostName != copy.Registries[name].HostName {
				t.Errorf("Registry HostName mismatch for %v: expected %v, got %v", name, registry.HostName, copy.Registries[name].HostName)
			}
		}

		// Modify the copy and ensure original is unchanged
		copy.Enabled = ptrBool(false)
		if original.Enabled == nil || *original.Enabled == *copy.Enabled {
			t.Errorf("Original Enabled was modified: expected %v, got %v", true, *copy.Enabled)
		}

		copy.Registries["new-registry"] = RegistryConfig{Remote: "new-remote", Local: "new-local", HostName: "new-hostname"}
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
