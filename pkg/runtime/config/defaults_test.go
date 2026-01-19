package config

import (
	"testing"
)

func TestDefaultConfigurations_HostPorts(t *testing.T) {
	t.Run("DefaultConfig_Localhost_ControlPlanes_HasHostPorts", func(t *testing.T) {
		// Given the localhost default configuration
		config := DefaultConfig_Localhost

		// Then cluster configuration should be present
		if config.Cluster == nil {
			t.Fatal("Expected cluster configuration to be present")
		}

		// And control planes should have expected host ports for local development
		expectedHostPorts := []string{"8080:30080/tcp", "8443:30443/tcp", "9292:30292/tcp", "8053:30053/udp"}
		actualHostPorts := config.Cluster.ControlPlanes.HostPorts

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
		// Given the full default configuration
		config := DefaultConfig_Full

		// Then cluster configuration should be present
		if config.Cluster == nil {
			t.Fatal("Expected cluster configuration to be present")
		}

		// And control planes should have no host ports for production use
		actualControlPlaneHostPorts := config.Cluster.ControlPlanes.HostPorts

		if len(actualControlPlaneHostPorts) != 0 {
			t.Errorf("Expected no hostports for DefaultConfig_Full controlplanes, got %d: %v", len(actualControlPlaneHostPorts), actualControlPlaneHostPorts)
		}
	})

	t.Run("DefaultConfig_Localhost_HasZeroWorkers", func(t *testing.T) {
		// Given the localhost default configuration
		config := DefaultConfig_Localhost

		// Then cluster configuration should be present
		if config.Cluster == nil {
			t.Fatal("Expected cluster configuration to be present")
		}

		// And workers count should be explicitly set to zero
		if config.Cluster.Workers.Count == nil {
			t.Fatal("Expected workers.count to be explicitly set")
		}
		if *config.Cluster.Workers.Count != 0 {
			t.Errorf("Expected workers.count to be 0, got: %d", *config.Cluster.Workers.Count)
		}
	})

	t.Run("DefaultConfig_Full_HasZeroWorkers", func(t *testing.T) {
		// Given the full default configuration
		config := DefaultConfig_Full

		// Then cluster configuration should be present
		if config.Cluster == nil {
			t.Fatal("Expected cluster configuration to be present")
		}

		// And workers count should be explicitly set to zero
		if config.Cluster.Workers.Count == nil {
			t.Fatal("Expected workers.count to be explicitly set")
		}
		if *config.Cluster.Workers.Count != 0 {
			t.Errorf("Expected workers.count to be 0, got: %d", *config.Cluster.Workers.Count)
		}
	})

	t.Run("DefaultConfig_ControlPlanes_HaveVolumes", func(t *testing.T) {
		// Given both default configurations
		configLocalhost := DefaultConfig_Localhost
		configFull := DefaultConfig_Full

		// Then cluster configurations should be present
		if configLocalhost.Cluster == nil || configFull.Cluster == nil {
			t.Fatal("Expected cluster configuration to be present")
		}

		// And control planes should have the project volumes mount
		expectedVolume := "${WINDSOR_PROJECT_ROOT}/.volumes:/var/local"

		if len(configLocalhost.Cluster.ControlPlanes.Volumes) == 0 {
			t.Error("Expected DefaultConfig_Localhost controlplanes to have volumes")
		} else if configLocalhost.Cluster.ControlPlanes.Volumes[0] != expectedVolume {
			t.Errorf("Expected volume %s, got %s", expectedVolume, configLocalhost.Cluster.ControlPlanes.Volumes[0])
		}

		if len(configFull.Cluster.ControlPlanes.Volumes) == 0 {
			t.Error("Expected DefaultConfig_Full controlplanes to have volumes")
		} else if configFull.Cluster.ControlPlanes.Volumes[0] != expectedVolume {
			t.Errorf("Expected volume %s, got %s", expectedVolume, configFull.Cluster.ControlPlanes.Volumes[0])
		}
	})

	t.Run("DefaultConfig_ControlPlanes_AreSchedulable", func(t *testing.T) {
		// Given both default configurations
		configLocalhost := DefaultConfig_Localhost
		configFull := DefaultConfig_Full

		// Then cluster configurations should be present
		if configLocalhost.Cluster == nil || configFull.Cluster == nil {
			t.Fatal("Expected cluster configuration to be present")
		}

		// And control planes should be schedulable for workloads
		if configLocalhost.Cluster.ControlPlanes.Schedulable == nil || !*configLocalhost.Cluster.ControlPlanes.Schedulable {
			t.Error("Expected DefaultConfig_Localhost controlplanes to be schedulable")
		}

		if configFull.Cluster.ControlPlanes.Schedulable == nil || !*configFull.Cluster.ControlPlanes.Schedulable {
			t.Error("Expected DefaultConfig_Full controlplanes to be schedulable")
		}
	})
}
