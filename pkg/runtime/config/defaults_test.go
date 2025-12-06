package config

import (
	"testing"
)

func TestDefaultConfigurations_HostPorts(t *testing.T) {
	t.Run("DefaultConfig_Localhost_HasHostPorts", func(t *testing.T) {
		// Given the DefaultConfig_Localhost configuration (used for docker-desktop)
		config := DefaultConfig_Localhost

		// Then the workers should have hostports configured
		if config.Cluster == nil {
			t.Fatal("Expected cluster configuration to be present")
		}

		expectedHostPorts := []string{"8080:30080/tcp", "8443:30443/tcp", "9292:30292/tcp", "8053:30053/udp"}
		actualHostPorts := config.Cluster.Workers.HostPorts

		if len(actualHostPorts) != len(expectedHostPorts) {
			t.Errorf("Expected %d hostports, got %d", len(expectedHostPorts), len(actualHostPorts))
		}

		for i, expected := range expectedHostPorts {
			if i >= len(actualHostPorts) || actualHostPorts[i] != expected {
				t.Errorf("Expected hostport %s at index %d, got %s", expected, i,
					func() string {
						if i < len(actualHostPorts) {
							return actualHostPorts[i]
						}
						return "missing"
					}())
			}
		}
	})

	t.Run("DefaultConfig_Full_HasNoHostPorts", func(t *testing.T) {
		// Given the DefaultConfig_Full configuration (used for colima/docker)
		config := DefaultConfig_Full

		// Then the workers should have no hostports configured
		if config.Cluster == nil {
			t.Fatal("Expected cluster configuration to be present")
		}

		actualHostPorts := config.Cluster.Workers.HostPorts

		if len(actualHostPorts) != 0 {
			t.Errorf("Expected no hostports for DefaultConfig_Full, got %d: %v", len(actualHostPorts), actualHostPorts)
		}

		// And the controlplanes should also have no hostports
		actualControlPlaneHostPorts := config.Cluster.ControlPlanes.HostPorts

		if len(actualControlPlaneHostPorts) != 0 {
			t.Errorf("Expected no hostports for DefaultConfig_Full controlplanes, got %d: %v", len(actualControlPlaneHostPorts), actualControlPlaneHostPorts)
		}
	})

	t.Run("DefaultConfig_Localhost_ControlPlanes_HasNoHostPorts", func(t *testing.T) {
		// Given the DefaultConfig_Localhost configuration
		config := DefaultConfig_Localhost

		// Then the controlplanes should have no hostports (only workers need them for docker-desktop)
		if config.Cluster == nil {
			t.Fatal("Expected cluster configuration to be present")
		}

		actualControlPlaneHostPorts := config.Cluster.ControlPlanes.HostPorts

		if len(actualControlPlaneHostPorts) != 0 {
			t.Errorf("Expected no hostports for DefaultConfig_Localhost controlplanes, got %d: %v", len(actualControlPlaneHostPorts), actualControlPlaneHostPorts)
		}
	})
}
