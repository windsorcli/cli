package workstation

import (
	"reflect"
	"testing"
)

// TestClusterConfig_Merge tests the Merge method of ClusterConfig
func TestClusterConfig_Merge(t *testing.T) {
	t.Run("MergeWithNilOverlay", func(t *testing.T) {
		base := &ClusterConfig{
			Enabled: ptrBool(true),
			Driver:  ptrString("talos"),
		}
		original := base.DeepCopy()

		base.Merge(nil)

		if !reflect.DeepEqual(base, original) {
			t.Errorf("Expected no change when merging with nil overlay")
		}
	})

	t.Run("MergeBasicFields", func(t *testing.T) {
		base := &ClusterConfig{
			Enabled: ptrBool(false),
			Driver:  ptrString("kind"),
		}

		overlay := &ClusterConfig{
			Enabled: ptrBool(true),
			Driver:  ptrString("talos"),
		}

		base.Merge(overlay)

		if !*base.Enabled {
			t.Errorf("Expected Enabled to be true")
		}
		if *base.Driver != "talos" {
			t.Errorf("Expected Driver to be 'talos', got %s", *base.Driver)
		}
	})

	t.Run("MergeControlPlanes", func(t *testing.T) {
		base := &ClusterConfig{
			ControlPlanes: NodeGroupConfig{
				Count:  ptrInt(1),
				CPU:    ptrInt(2),
				Memory: ptrInt(4),
			},
		}

		overlay := &ClusterConfig{
			ControlPlanes: NodeGroupConfig{
				Count:  ptrInt(3),
				CPU:    ptrInt(4),
				Memory: ptrInt(8),
				Image:  ptrString("talos:v1.0.0"),
			},
		}

		base.Merge(overlay)

		if *base.ControlPlanes.Count != 3 {
			t.Errorf("Expected ControlPlanes.Count to be 3, got %d", *base.ControlPlanes.Count)
		}
		if *base.ControlPlanes.CPU != 4 {
			t.Errorf("Expected ControlPlanes.CPU to be 4, got %d", *base.ControlPlanes.CPU)
		}
		if *base.ControlPlanes.Memory != 8 {
			t.Errorf("Expected ControlPlanes.Memory to be 8, got %d", *base.ControlPlanes.Memory)
		}
		if *base.ControlPlanes.Image != "talos:v1.0.0" {
			t.Errorf("Expected ControlPlanes.Image to be 'talos:v1.0.0', got %s", *base.ControlPlanes.Image)
		}
	})

	t.Run("MergeWorkers", func(t *testing.T) {
		base := &ClusterConfig{
			Workers: NodeGroupConfig{
				Count:  ptrInt(1),
				CPU:    ptrInt(2),
				Memory: ptrInt(4),
			},
		}

		overlay := &ClusterConfig{
			Workers: NodeGroupConfig{
				Count:  ptrInt(2),
				CPU:    ptrInt(4),
				Memory: ptrInt(8),
				Image:  ptrString("talos:v1.0.0"),
			},
		}

		base.Merge(overlay)

		if *base.Workers.Count != 2 {
			t.Errorf("Expected Workers.Count to be 2, got %d", *base.Workers.Count)
		}
		if *base.Workers.CPU != 4 {
			t.Errorf("Expected Workers.CPU to be 4, got %d", *base.Workers.CPU)
		}
		if *base.Workers.Memory != 8 {
			t.Errorf("Expected Workers.Memory to be 8, got %d", *base.Workers.Memory)
		}
		if *base.Workers.Image != "talos:v1.0.0" {
			t.Errorf("Expected Workers.Image to be 'talos:v1.0.0', got %s", *base.Workers.Image)
		}
	})

	t.Run("MergeWithNodes", func(t *testing.T) {
		base := &ClusterConfig{
			ControlPlanes: NodeGroupConfig{
				Nodes: map[string]NodeConfig{
					"cp1": {
						Hostname: ptrString("cp1.local"),
						Endpoint: ptrString("10.0.0.1"),
					},
				},
			},
		}

		overlay := &ClusterConfig{
			ControlPlanes: NodeGroupConfig{
				Nodes: map[string]NodeConfig{
					"cp2": {
						Hostname: ptrString("cp2.local"),
						Endpoint: ptrString("10.0.0.2"),
					},
				},
			},
		}

		base.Merge(overlay)

		if len(base.ControlPlanes.Nodes) != 1 {
			t.Errorf("Expected 1 node, got %d", len(base.ControlPlanes.Nodes))
		}
		if *base.ControlPlanes.Nodes["cp2"].Hostname != "cp2.local" {
			t.Errorf("Expected hostname to be 'cp2.local', got %s", *base.ControlPlanes.Nodes["cp2"].Hostname)
		}
	})

	t.Run("MergeWithHostPorts", func(t *testing.T) {
		base := &ClusterConfig{
			ControlPlanes: NodeGroupConfig{
				HostPorts: []string{"8080:80"},
			},
		}

		overlay := &ClusterConfig{
			ControlPlanes: NodeGroupConfig{
				HostPorts: []string{"8443:443", "3000:3000"},
			},
		}

		base.Merge(overlay)

		expected := []string{"8443:443", "3000:3000"}
		if !reflect.DeepEqual(base.ControlPlanes.HostPorts, expected) {
			t.Errorf("Expected HostPorts to be %v, got %v", expected, base.ControlPlanes.HostPorts)
		}
	})

	t.Run("MergeWithVolumes", func(t *testing.T) {
		base := &ClusterConfig{
			Workers: NodeGroupConfig{
				Volumes: []string{"/tmp/data:/var/data"},
			},
		}

		overlay := &ClusterConfig{
			Workers: NodeGroupConfig{
				Volumes: []string{"/tmp/logs:/var/logs", "/tmp/cache:/var/cache"},
			},
		}

		base.Merge(overlay)

		expected := []string{"/tmp/logs:/var/logs", "/tmp/cache:/var/cache"}
		if !reflect.DeepEqual(base.Workers.Volumes, expected) {
			t.Errorf("Expected Volumes to be %v, got %v", expected, base.Workers.Volumes)
		}
	})

	t.Run("MergeWithNodesAndSlices", func(t *testing.T) {
		base := &ClusterConfig{
			ControlPlanes: NodeGroupConfig{
				Nodes: map[string]NodeConfig{
					"cp1": {
						Hostname:  ptrString("cp1.local"),
						HostPorts: []string{"8080:80"},
					},
				},
				HostPorts: []string{"6443:6443"},
				Volumes:   []string{"/tmp/data:/var/data"},
			},
			Workers: NodeGroupConfig{
				Nodes: map[string]NodeConfig{
					"worker1": {
						Hostname:  ptrString("worker1.local"),
						HostPorts: []string{"3000:3000"},
					},
				},
				HostPorts: []string{"3000:3000"},
				Volumes:   []string{"/tmp/logs:/var/logs"},
			},
		}

		overlay := &ClusterConfig{
			ControlPlanes: NodeGroupConfig{
				Nodes: map[string]NodeConfig{
					"cp2": {
						Hostname:  ptrString("cp2.local"),
						HostPorts: []string{"8081:81"},
					},
				},
				HostPorts: []string{"8443:443"},
				Volumes:   []string{"/tmp/config:/var/config"},
			},
			Workers: NodeGroupConfig{
				Nodes: map[string]NodeConfig{
					"worker2": {
						Hostname:  ptrString("worker2.local"),
						HostPorts: []string{"3001:3001"},
					},
				},
				HostPorts: []string{"3001:3001"},
				Volumes:   []string{"/tmp/cache:/var/cache"},
			},
		}

		base.Merge(overlay)

		// Verify control planes - overlay replaces existing data
		if len(base.ControlPlanes.Nodes) != 1 {
			t.Errorf("Expected 1 control plane node, got %d", len(base.ControlPlanes.Nodes))
		}
		if base.ControlPlanes.Nodes["cp2"].Hostname == nil || *base.ControlPlanes.Nodes["cp2"].Hostname != "cp2.local" {
			t.Errorf("Expected cp2 hostname to be 'cp2.local'")
		}
		if len(base.ControlPlanes.HostPorts) != 1 {
			t.Errorf("Expected 1 control plane host port, got %d", len(base.ControlPlanes.HostPorts))
		}
		if base.ControlPlanes.HostPorts[0] != "8443:443" {
			t.Errorf("Expected control plane host port to be '8443:443', got %s", base.ControlPlanes.HostPorts[0])
		}
		if len(base.ControlPlanes.Volumes) != 1 {
			t.Errorf("Expected 1 control plane volume, got %d", len(base.ControlPlanes.Volumes))
		}
		if base.ControlPlanes.Volumes[0] != "/tmp/config:/var/config" {
			t.Errorf("Expected control plane volume to be '/tmp/config:/var/config', got %s", base.ControlPlanes.Volumes[0])
		}

		// Verify workers - overlay replaces existing data
		if len(base.Workers.Nodes) != 1 {
			t.Errorf("Expected 1 worker node, got %d", len(base.Workers.Nodes))
		}
		if base.Workers.Nodes["worker2"].Hostname == nil || *base.Workers.Nodes["worker2"].Hostname != "worker2.local" {
			t.Errorf("Expected worker2 hostname to be 'worker2.local'")
		}
		if len(base.Workers.HostPorts) != 1 {
			t.Errorf("Expected 1 worker host port, got %d", len(base.Workers.HostPorts))
		}
		if base.Workers.HostPorts[0] != "3001:3001" {
			t.Errorf("Expected worker host port to be '3001:3001', got %s", base.Workers.HostPorts[0])
		}
		if len(base.Workers.Volumes) != 1 {
			t.Errorf("Expected 1 worker volume, got %d", len(base.Workers.Volumes))
		}
		if base.Workers.Volumes[0] != "/tmp/cache:/var/cache" {
			t.Errorf("Expected worker volume to be '/tmp/cache:/var/cache', got %s", base.Workers.Volumes[0])
		}
	})

	t.Run("MergeWithAllFields", func(t *testing.T) {
		base := &ClusterConfig{
			Enabled:  ptrBool(false),
			Driver:   ptrString("kind"),
			Endpoint: ptrString("https://old.local:6443"),
			Image:    ptrString("kind:v1.0.0"),
		}

		overlay := &ClusterConfig{
			Enabled:  ptrBool(true),
			Driver:   ptrString("talos"),
			Endpoint: ptrString("https://new.local:6443"),
			Image:    ptrString("talos:v1.0.0"),
		}

		base.Merge(overlay)

		if !*base.Enabled {
			t.Errorf("Expected Enabled to be true")
		}
		if *base.Driver != "talos" {
			t.Errorf("Expected Driver to be 'talos'")
		}
		if *base.Endpoint != "https://new.local:6443" {
			t.Errorf("Expected Endpoint to be 'https://new.local:6443'")
		}
		if *base.Image != "talos:v1.0.0" {
			t.Errorf("Expected Image to be 'talos:v1.0.0'")
		}
	})
}

// TestClusterConfig_Copy tests the Copy method of ClusterConfig
func TestClusterConfig_Copy(t *testing.T) {
	t.Run("CopyNilConfig", func(t *testing.T) {
		var config *ClusterConfig
		copied := config.DeepCopy()

		if copied != nil {
			t.Errorf("Expected nil when copying nil config")
		}
	})

	t.Run("CopyEmptyConfig", func(t *testing.T) {
		config := &ClusterConfig{}
		copied := config.DeepCopy()

		if copied == nil {
			t.Errorf("Expected non-nil copy of empty config")
		}
		if !reflect.DeepEqual(config, copied) {
			t.Errorf("Expected copy to be equal to original")
		}
	})

	t.Run("CopyPopulatedConfig", func(t *testing.T) {
		config := &ClusterConfig{
			Enabled:  ptrBool(true),
			Driver:   ptrString("talos"),
			Endpoint: ptrString("https://cluster.local:6443"),
			Image:    ptrString("talos:v1.0.0"),
			ControlPlanes: NodeGroupConfig{
				Count:  ptrInt(3),
				CPU:    ptrInt(2),
				Memory: ptrInt(4),
				Image:  ptrString("talos:v1.0.0"),
				Nodes: map[string]NodeConfig{
					"cp1": {
						Hostname:  ptrString("cp1.local"),
						Endpoint:  ptrString("10.0.0.1"),
						Image:     ptrString("talos:v1.0.0"),
						HostPorts: []string{"8080:80", "8443:443"},
					},
				},
				HostPorts: []string{"6443:6443"},
				Volumes:   []string{"/tmp/data:/var/data"},
			},
			Workers: NodeGroupConfig{
				Count:  ptrInt(2),
				CPU:    ptrInt(4),
				Memory: ptrInt(8),
				Image:  ptrString("talos:v1.0.0"),
				Nodes: map[string]NodeConfig{
					"worker1": {
						Hostname:  ptrString("worker1.local"),
						Endpoint:  ptrString("10.0.0.10"),
						Image:     ptrString("talos:v1.0.0"),
						HostPorts: []string{"3000:3000"},
					},
				},
				HostPorts: []string{"3000:3000"},
				Volumes:   []string{"/tmp/logs:/var/logs"},
			},
		}

		copied := config.DeepCopy()

		if copied == nil {
			t.Errorf("Expected non-nil copy")
		}
		if !reflect.DeepEqual(config, copied) {
			t.Errorf("Expected copy to be equal to original")
		}

		// Verify deep copy by modifying original
		*config.Enabled = false
		if !*copied.Enabled {
			t.Errorf("Expected copy to be independent of original")
		}

		// Verify deep copy of slices
		config.ControlPlanes.HostPorts[0] = "9999:9999"
		if copied.ControlPlanes.HostPorts[0] == "9999:9999" {
			t.Errorf("Expected copy slices to be independent of original")
		}

		// Verify deep copy of maps
		*config.ControlPlanes.Nodes["cp1"].Hostname = "modified.local"
		if *copied.ControlPlanes.Nodes["cp1"].Hostname == "modified.local" {
			t.Errorf("Expected copy maps to be independent of original")
		}
	})

}

// Helper functions to create pointers for basic types
func ptrString(s string) *string {
	return &s
}

func ptrBool(b bool) *bool {
	return &b
}

func ptrInt(i int) *int {
	return &i
}
