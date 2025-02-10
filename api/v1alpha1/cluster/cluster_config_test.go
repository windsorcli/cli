package cluster

import (
	"testing"
)

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

func TestClusterConfig_Merge(t *testing.T) {
	t.Run("MergeWithNoNils", func(t *testing.T) {
		base := &ClusterConfig{
			Enabled: ptrBool(true),
			Driver:  ptrString("base-driver"),
			ControlPlanes: struct {
				Count     *int                  `yaml:"count,omitempty"`
				CPU       *int                  `yaml:"cpu,omitempty"`
				Memory    *int                  `yaml:"memory,omitempty"`
				Nodes     map[string]NodeConfig `yaml:"nodes,omitempty"`
				HostPorts []string              `yaml:"hostports,omitempty"`
				Volumes   []string              `yaml:"volumes,omitempty"`
			}{
				Count:  ptrInt(3),
				CPU:    ptrInt(4),
				Memory: ptrInt(8192),
				Nodes: map[string]NodeConfig{
					"node1": {
						Hostname: ptrString("base-node1"),
					},
				},
				HostPorts: []string{"1000:1000/tcp", "2000:2000/tcp"},
				Volumes:   []string{"${WINDSOR_PROJECT_ROOT}/base/volume1:/var/local/base1"},
			},
			Workers: struct {
				Count     *int                  `yaml:"count,omitempty"`
				CPU       *int                  `yaml:"cpu,omitempty"`
				Memory    *int                  `yaml:"memory,omitempty"`
				Nodes     map[string]NodeConfig `yaml:"nodes,omitempty"`
				HostPorts []string              `yaml:"hostports,omitempty"`
				Volumes   []string              `yaml:"volumes,omitempty"`
			}{
				Count:  ptrInt(5),
				CPU:    ptrInt(2),
				Memory: ptrInt(4096),
				Nodes: map[string]NodeConfig{
					"worker1": {
						Hostname: ptrString("base-worker1"),
					},
				},
				HostPorts: []string{"8080", "9090"},
				Volumes:   []string{"${WINDSOR_PROJECT_ROOT}/base/worker/volume1:/var/local/worker1"},
			},
		}

		overlay := &ClusterConfig{
			Enabled: ptrBool(false),
			Driver:  ptrString("overlay-driver"),
			ControlPlanes: struct {
				Count     *int                  `yaml:"count,omitempty"`
				CPU       *int                  `yaml:"cpu,omitempty"`
				Memory    *int                  `yaml:"memory,omitempty"`
				Nodes     map[string]NodeConfig `yaml:"nodes,omitempty"`
				HostPorts []string              `yaml:"hostports,omitempty"`
				Volumes   []string              `yaml:"volumes,omitempty"`
			}{
				Count:  ptrInt(1),
				CPU:    ptrInt(2),
				Memory: ptrInt(4096),
				Nodes: map[string]NodeConfig{
					"node2": {
						Hostname: ptrString("overlay-node2"),
					},
				},
				HostPorts: []string{"3000:3000/tcp", "4000:4000/tcp"},
				Volumes:   []string{"${WINDSOR_PROJECT_ROOT}/overlay/volume2:/var/local/overlay2"},
			},
			Workers: struct {
				Count     *int                  `yaml:"count,omitempty"`
				CPU       *int                  `yaml:"cpu,omitempty"`
				Memory    *int                  `yaml:"memory,omitempty"`
				Nodes     map[string]NodeConfig `yaml:"nodes,omitempty"`
				HostPorts []string              `yaml:"hostports,omitempty"`
				Volumes   []string              `yaml:"volumes,omitempty"`
			}{
				Count:  ptrInt(3),
				CPU:    ptrInt(1),
				Memory: ptrInt(2048),
				Nodes: map[string]NodeConfig{
					"worker2": {
						Hostname: ptrString("overlay-worker2"),
					},
				},
				HostPorts: []string{"8082", "9092"},
				Volumes:   []string{"${WINDSOR_PROJECT_ROOT}/overlay/worker/volume2:/var/local/worker2"},
			},
		}

		base.Merge(overlay)

		if base.Enabled == nil || *base.Enabled != false {
			t.Errorf("Enabled mismatch: expected false, got %v", *base.Enabled)
		}
		if base.Driver == nil || *base.Driver != "overlay-driver" {
			t.Errorf("Driver mismatch: expected 'overlay-driver', got '%s'", *base.Driver)
		}
		if len(base.ControlPlanes.HostPorts) != 2 || base.ControlPlanes.HostPorts[0] != "3000:3000/tcp" || base.ControlPlanes.HostPorts[1] != "4000:4000/tcp" {
			t.Errorf("ControlPlanes HostPorts mismatch: expected ['3000:3000/tcp', '4000:4000/tcp'], got %v", base.ControlPlanes.HostPorts)
		}
		if len(base.Workers.HostPorts) != 2 || base.Workers.HostPorts[0] != "8082" || base.Workers.HostPorts[1] != "9092" {
			t.Errorf("Workers HostPorts mismatch: expected ['8082', '9092'], got %v", base.Workers.HostPorts)
		}
		if base.ControlPlanes.Count == nil || *base.ControlPlanes.Count != 1 {
			t.Errorf("ControlPlanes Count mismatch: expected 1, got %v", *base.ControlPlanes.Count)
		}
		if base.ControlPlanes.CPU == nil || *base.ControlPlanes.CPU != 2 {
			t.Errorf("ControlPlanes CPU mismatch: expected 2, got %v", *base.ControlPlanes.CPU)
		}
		if base.ControlPlanes.Memory == nil || *base.ControlPlanes.Memory != 4096 {
			t.Errorf("ControlPlanes Memory mismatch: expected 4096, got %v", *base.ControlPlanes.Memory)
		}
		if len(base.ControlPlanes.Nodes) != 1 || base.ControlPlanes.Nodes["node2"].Hostname == nil || *base.ControlPlanes.Nodes["node2"].Hostname != "overlay-node2" {
			t.Errorf("ControlPlanes Nodes mismatch: expected 'overlay-node2', got %v", base.ControlPlanes.Nodes)
		}
		if base.Workers.Count == nil || *base.Workers.Count != 3 {
			t.Errorf("Workers Count mismatch: expected 3, got %v", *base.Workers.Count)
		}
		if base.Workers.CPU == nil || *base.Workers.CPU != 1 {
			t.Errorf("Workers CPU mismatch: expected 1, got %v", *base.Workers.CPU)
		}
		if base.Workers.Memory == nil || *base.Workers.Memory != 2048 {
			t.Errorf("Workers Memory mismatch: expected 2048, got %v", *base.Workers.Memory)
		}
		if len(base.Workers.Nodes) != 1 || base.Workers.Nodes["worker2"].Hostname == nil || *base.Workers.Nodes["worker2"].Hostname != "overlay-worker2" {
			t.Errorf("Workers Nodes mismatch: expected 'overlay-worker2', got %v", base.Workers.Nodes)
		}
		if len(base.Workers.Volumes) != 1 || base.Workers.Volumes[0] != "${WINDSOR_PROJECT_ROOT}/overlay/worker/volume2:/var/local/worker2" {
			t.Errorf("Workers Volumes mismatch: expected ['${WINDSOR_PROJECT_ROOT}/overlay/worker/volume2:/var/local/worker2'], got %v", base.Workers.Volumes)
		}
	})

	t.Run("MergeWithAllNils", func(t *testing.T) {
		base := &ClusterConfig{
			Enabled: nil,
			Driver:  nil,
			ControlPlanes: struct {
				Count     *int                  `yaml:"count,omitempty"`
				CPU       *int                  `yaml:"cpu,omitempty"`
				Memory    *int                  `yaml:"memory,omitempty"`
				Nodes     map[string]NodeConfig `yaml:"nodes,omitempty"`
				HostPorts []string              `yaml:"hostports,omitempty"`
				Volumes   []string              `yaml:"volumes,omitempty"`
			}{
				Count:     nil,
				CPU:       nil,
				Memory:    nil,
				Nodes:     nil,
				HostPorts: nil,
				Volumes:   nil,
			},
			Workers: struct {
				Count     *int                  `yaml:"count,omitempty"`
				CPU       *int                  `yaml:"cpu,omitempty"`
				Memory    *int                  `yaml:"memory,omitempty"`
				Nodes     map[string]NodeConfig `yaml:"nodes,omitempty"`
				HostPorts []string              `yaml:"hostports,omitempty"`
				Volumes   []string              `yaml:"volumes,omitempty"`
			}{
				Count:     nil,
				CPU:       nil,
				Memory:    nil,
				Nodes:     nil,
				HostPorts: nil,
				Volumes:   nil,
			},
		}

		overlay := &ClusterConfig{
			Enabled: nil,
			Driver:  nil,
			ControlPlanes: struct {
				Count     *int                  `yaml:"count,omitempty"`
				CPU       *int                  `yaml:"cpu,omitempty"`
				Memory    *int                  `yaml:"memory,omitempty"`
				Nodes     map[string]NodeConfig `yaml:"nodes,omitempty"`
				HostPorts []string              `yaml:"hostports,omitempty"`
				Volumes   []string              `yaml:"volumes,omitempty"`
			}{
				Count:     nil,
				CPU:       nil,
				Memory:    nil,
				Nodes:     nil,
				HostPorts: nil,
				Volumes:   nil,
			},
			Workers: struct {
				Count     *int                  `yaml:"count,omitempty"`
				CPU       *int                  `yaml:"cpu,omitempty"`
				Memory    *int                  `yaml:"memory,omitempty"`
				Nodes     map[string]NodeConfig `yaml:"nodes,omitempty"`
				HostPorts []string              `yaml:"hostports,omitempty"`
				Volumes   []string              `yaml:"volumes,omitempty"`
			}{
				Count:     nil,
				CPU:       nil,
				Memory:    nil,
				Nodes:     nil,
				HostPorts: nil,
				Volumes:   nil,
			},
		}

		base.Merge(overlay)

		if base.Enabled != nil {
			t.Errorf("Enabled mismatch: expected nil, got %v", base.Enabled)
		}
		if base.Driver != nil {
			t.Errorf("Driver mismatch: expected nil, got '%s'", *base.Driver)
		}
		if base.Workers.HostPorts != nil {
			t.Errorf("Workers HostPorts mismatch: expected nil, got %v", base.Workers.HostPorts)
		}
		if base.ControlPlanes.HostPorts != nil {
			t.Errorf("ControlPlanes HostPorts mismatch: expected nil, got %v", base.ControlPlanes.HostPorts)
		}
		if base.ControlPlanes.Count != nil {
			t.Errorf("ControlPlanes Count mismatch: expected nil, got %v", *base.ControlPlanes.Count)
		}
		if base.ControlPlanes.CPU != nil {
			t.Errorf("ControlPlanes CPU mismatch: expected nil, got %v", *base.ControlPlanes.CPU)
		}
		if base.ControlPlanes.Memory != nil {
			t.Errorf("ControlPlanes Memory mismatch: expected nil, got %v", *base.ControlPlanes.Memory)
		}
		if base.ControlPlanes.Nodes != nil {
			t.Errorf("ControlPlanes Nodes mismatch: expected nil, got %v", base.ControlPlanes.Nodes)
		}
		if base.Workers.Count != nil {
			t.Errorf("Workers Count mismatch: expected nil, got %v", *base.Workers.Count)
		}
		if base.Workers.CPU != nil {
			t.Errorf("Workers CPU mismatch: expected nil, got %v", *base.Workers.CPU)
		}
		if base.Workers.Memory != nil {
			t.Errorf("Workers Memory mismatch: expected nil, got %v", *base.Workers.Memory)
		}
		if base.Workers.Nodes != nil {
			t.Errorf("Workers Nodes mismatch: expected nil, got %v", base.Workers.Nodes)
		}
		if base.Workers.Volumes != nil {
			t.Errorf("Workers Volumes mismatch: expected nil, got %v", base.Workers.Volumes)
		}
	})
}

func TestClusterConfig_Copy(t *testing.T) {
	t.Run("CopyWithNonNilValues", func(t *testing.T) {
		original := &ClusterConfig{
			Enabled: ptrBool(true),
			Driver:  ptrString("original-driver"),
			ControlPlanes: struct {
				Count     *int                  `yaml:"count,omitempty"`
				CPU       *int                  `yaml:"cpu,omitempty"`
				Memory    *int                  `yaml:"memory,omitempty"`
				Nodes     map[string]NodeConfig `yaml:"nodes,omitempty"`
				HostPorts []string              `yaml:"hostports,omitempty"`
				Volumes   []string              `yaml:"volumes,omitempty"`
			}{
				Count:  ptrInt(3),
				CPU:    ptrInt(4),
				Memory: ptrInt(8192),
				Nodes: map[string]NodeConfig{
					"node1": {
						Hostname: ptrString("original-node1"),
					},
				},
				HostPorts: []string{"1000:1000/tcp", "2000:2000/tcp"},
				Volumes:   []string{"${WINDSOR_PROJECT_ROOT}/original/volume1:/var/local/original1"},
			},
			Workers: struct {
				Count     *int                  `yaml:"count,omitempty"`
				CPU       *int                  `yaml:"cpu,omitempty"`
				Memory    *int                  `yaml:"memory,omitempty"`
				Nodes     map[string]NodeConfig `yaml:"nodes,omitempty"`
				HostPorts []string              `yaml:"hostports,omitempty"`
				Volumes   []string              `yaml:"volumes,omitempty"`
			}{
				Count:  ptrInt(5),
				CPU:    ptrInt(2),
				Memory: ptrInt(4096),
				Nodes: map[string]NodeConfig{
					"worker1": {
						Hostname: ptrString("original-worker1"),
					},
				},
				HostPorts: []string{"3000:3000/tcp", "4000:4000/tcp"},
				Volumes:   []string{"${WINDSOR_PROJECT_ROOT}/original/worker/volume1:/var/local/worker1"},
			},
		}

		copy := original.Copy()

		if original.Enabled == nil || copy.Enabled == nil || *original.Enabled != *copy.Enabled {
			t.Errorf("Enabled mismatch: expected %v, got %v", *original.Enabled, *copy.Enabled)
		}
		if original.Driver == nil || copy.Driver == nil || *original.Driver != *copy.Driver {
			t.Errorf("Driver mismatch: expected %v, got %v", *original.Driver, *copy.Driver)
		}
		if len(original.Workers.HostPorts) != len(copy.Workers.HostPorts) {
			t.Errorf("Workers HostPorts length mismatch: expected %d, got %d", len(original.Workers.HostPorts), len(copy.Workers.HostPorts))
		}
		for i, port := range original.Workers.HostPorts {
			if port != copy.Workers.HostPorts[i] {
				t.Errorf("Workers HostPorts mismatch at index %d: expected %v, got %v", i, port, copy.Workers.HostPorts[i])
			}
		}
		if original.Workers.Count == nil || copy.Workers.Count == nil || *original.Workers.Count != *copy.Workers.Count {
			t.Errorf("Workers Count mismatch: expected %v, got %v", *original.Workers.Count, *copy.Workers.Count)
		}
		if original.Workers.CPU == nil || copy.Workers.CPU == nil || *original.Workers.CPU != *copy.Workers.CPU {
			t.Errorf("Workers CPU mismatch: expected %v, got %v", *original.Workers.CPU, *copy.Workers.CPU)
		}
		if original.Workers.Memory == nil || copy.Workers.Memory == nil || *original.Workers.Memory != *copy.Workers.Memory {
			t.Errorf("Workers Memory mismatch: expected %v, got %v", *original.Workers.Memory, *copy.Workers.Memory)
		}
		if len(original.Workers.Nodes) != len(copy.Workers.Nodes) {
			t.Errorf("Workers Nodes length mismatch: expected %d, got %d", len(original.Workers.Nodes), len(copy.Workers.Nodes))
		}
		for key, node := range original.Workers.Nodes {
			if node.Hostname == nil || copy.Workers.Nodes[key].Hostname == nil || *node.Hostname != *copy.Workers.Nodes[key].Hostname {
				t.Errorf("Workers Nodes mismatch for key %s: expected %v, got %v", key, *node.Hostname, *copy.Workers.Nodes[key].Hostname)
			}
		}

		if len(original.Workers.HostPorts) != len(copy.Workers.HostPorts) || original.Workers.HostPorts[0] != copy.Workers.HostPorts[0] || original.Workers.HostPorts[1] != copy.Workers.HostPorts[1] {
			t.Errorf("HostPorts mismatch: expected %v, got %v", original.Workers.HostPorts, copy.Workers.HostPorts)
		}
		if original.ControlPlanes.Count == nil || copy.ControlPlanes.Count == nil || *original.ControlPlanes.Count != *copy.ControlPlanes.Count {
			t.Errorf("ControlPlanes Count mismatch: expected %v, got %v", *original.ControlPlanes.Count, *copy.ControlPlanes.Count)
		}
		if original.ControlPlanes.CPU == nil || copy.ControlPlanes.CPU == nil || *original.ControlPlanes.CPU != *copy.ControlPlanes.CPU {
			t.Errorf("ControlPlanes CPU mismatch: expected %v, got %v", *original.ControlPlanes.CPU, *copy.ControlPlanes.CPU)
		}
		if original.ControlPlanes.Memory == nil || copy.ControlPlanes.Memory == nil || *original.ControlPlanes.Memory != *copy.ControlPlanes.Memory {
			t.Errorf("ControlPlanes Memory mismatch: expected %v, got %v", *original.ControlPlanes.Memory, *copy.ControlPlanes.Memory)
		}
		if len(original.ControlPlanes.Nodes) != len(copy.ControlPlanes.Nodes) {
			t.Errorf("ControlPlanes Nodes length mismatch: expected %d, got %d", len(original.ControlPlanes.Nodes), len(copy.ControlPlanes.Nodes))
		}
		for key, node := range original.ControlPlanes.Nodes {
			if node.Hostname == nil || copy.ControlPlanes.Nodes[key].Hostname == nil || *node.Hostname != *copy.ControlPlanes.Nodes[key].Hostname {
				t.Errorf("ControlPlanes Nodes mismatch for key %s: expected %v, got %v", key, *node.Hostname, *copy.ControlPlanes.Nodes[key].Hostname)
			}
		}
		if original.Workers.Count == nil || copy.Workers.Count == nil || *original.Workers.Count != *copy.Workers.Count {
			t.Errorf("Workers Count mismatch: expected %v, got %v", *original.Workers.Count, *copy.Workers.Count)
		}
		if original.Workers.CPU == nil || copy.Workers.CPU == nil || *original.Workers.CPU != *copy.Workers.CPU {
			t.Errorf("Workers CPU mismatch: expected %v, got %v", *original.Workers.CPU, *copy.Workers.CPU)
		}
		if original.Workers.Memory == nil || copy.Workers.Memory == nil || *original.Workers.Memory != *copy.Workers.Memory {
			t.Errorf("Workers Memory mismatch: expected %v, got %v", *original.Workers.Memory, *copy.Workers.Memory)
		}
		if len(original.Workers.Nodes) != len(copy.Workers.Nodes) {
			t.Errorf("Workers Nodes length mismatch: expected %d, got %d", len(original.Workers.Nodes), len(copy.Workers.Nodes))
		}
		for key, node := range original.Workers.Nodes {
			if node.Hostname == nil || copy.Workers.Nodes[key].Hostname == nil || *node.Hostname != *copy.Workers.Nodes[key].Hostname {
				t.Errorf("Workers Nodes mismatch for key %s: expected %v, got %v", key, *node.Hostname, *copy.Workers.Nodes[key].Hostname)
			}
		}

		// Modify the copy and ensure original is unchanged
		copy.Enabled = ptrBool(false)
		if original.Enabled == nil || *original.Enabled == *copy.Enabled {
			t.Errorf("Original Enabled was modified: expected %v, got %v", true, *copy.Enabled)
		}

		copy.ControlPlanes.Nodes["node1"] = NodeConfig{Hostname: ptrString("modified-node1")}
		if original.ControlPlanes.Nodes["node1"].Hostname == nil || *original.ControlPlanes.Nodes["node1"].Hostname == *copy.ControlPlanes.Nodes["node1"].Hostname {
			t.Errorf("Original ControlPlanes Nodes was modified: expected %v, got %v", "original-node1", *copy.ControlPlanes.Nodes["node1"].Hostname)
		}
	})

	t.Run("CopyNil", func(t *testing.T) {
		var original *ClusterConfig = nil
		mockCopy := original.Copy()
		if mockCopy != nil {
			t.Errorf("Mock copy should be nil, got %v", mockCopy)
		}
	})
}
