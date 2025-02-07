package cluster

// ClusterConfig represents the cluster configuration
type ClusterConfig struct {
	Enabled       *bool   `yaml:"enabled"`
	Driver        *string `yaml:"driver"`
	ControlPlanes struct {
		Count     *int                  `yaml:"count,omitempty"`
		CPU       *int                  `yaml:"cpu,omitempty"`
		Memory    *int                  `yaml:"memory,omitempty"`
		Nodes     map[string]NodeConfig `yaml:"nodes,omitempty"`
		HostPorts []string              `yaml:"hostports,omitempty"`
	} `yaml:"controlplanes,omitempty"`
	Workers struct {
		Count     *int                  `yaml:"count,omitempty"`
		CPU       *int                  `yaml:"cpu,omitempty"`
		Memory    *int                  `yaml:"memory,omitempty"`
		Nodes     map[string]NodeConfig `yaml:"nodes,omitempty"`
		HostPorts []string              `yaml:"hostports,omitempty"`
	} `yaml:"workers,omitempty"`
}

// NodeConfig represents the node configuration
type NodeConfig struct {
	Hostname  *string  `yaml:"hostname"`
	Node      *string  `yaml:"node,omitempty"`
	Endpoint  *string  `yaml:"endpoint,omitempty"`
	HostPorts []string `yaml:"hostports,omitempty"`
}

// Merge performs a deep merge of the current ClusterConfig with another ClusterConfig.
func (base *ClusterConfig) Merge(overlay *ClusterConfig) {
	if overlay.Enabled != nil {
		base.Enabled = overlay.Enabled
	}
	if overlay.Driver != nil {
		base.Driver = overlay.Driver
	}
	if overlay.ControlPlanes.Count != nil {
		base.ControlPlanes.Count = overlay.ControlPlanes.Count
	}
	if overlay.ControlPlanes.CPU != nil {
		base.ControlPlanes.CPU = overlay.ControlPlanes.CPU
	}
	if overlay.ControlPlanes.Memory != nil {
		base.ControlPlanes.Memory = overlay.ControlPlanes.Memory
	}
	if overlay.ControlPlanes.Nodes != nil {
		base.ControlPlanes.Nodes = make(map[string]NodeConfig, len(overlay.ControlPlanes.Nodes))
		for key, node := range overlay.ControlPlanes.Nodes {
			base.ControlPlanes.Nodes[key] = node
		}
	}
	if overlay.Workers.Count != nil {
		base.Workers.Count = overlay.Workers.Count
	}
	if overlay.Workers.CPU != nil {
		base.Workers.CPU = overlay.Workers.CPU
	}
	if overlay.Workers.Memory != nil {
		base.Workers.Memory = overlay.Workers.Memory
	}
	if overlay.Workers.Nodes != nil {
		base.Workers.Nodes = make(map[string]NodeConfig, len(overlay.Workers.Nodes))
		for key, node := range overlay.Workers.Nodes {
			base.Workers.Nodes[key] = node
		}
	}
	if overlay.Workers.HostPorts != nil {
		base.Workers.HostPorts = make([]string, len(overlay.Workers.HostPorts))
		copy(base.Workers.HostPorts, overlay.Workers.HostPorts)
	}
}

// Copy creates a deep copy of the ClusterConfig object
func (c *ClusterConfig) Copy() *ClusterConfig {
	if c == nil {
		return nil
	}

	controlPlanesNodesCopy := make(map[string]NodeConfig, len(c.ControlPlanes.Nodes))
	for key, node := range c.ControlPlanes.Nodes {
		controlPlanesNodesCopy[key] = NodeConfig{
			Hostname:  node.Hostname,
			Node:      node.Node,
			Endpoint:  node.Endpoint,
			HostPorts: append([]string{}, node.HostPorts...), // Copy HostPorts for each node
		}
	}
	controlPlanesHostPortsCopy := make([]string, len(c.ControlPlanes.HostPorts))
	copy(controlPlanesHostPortsCopy, c.ControlPlanes.HostPorts)

	workersNodesCopy := make(map[string]NodeConfig, len(c.Workers.Nodes))
	for key, node := range c.Workers.Nodes {
		workersNodesCopy[key] = NodeConfig{
			Hostname:  node.Hostname,
			Node:      node.Node,
			Endpoint:  node.Endpoint,
			HostPorts: append([]string{}, node.HostPorts...), // Copy HostPorts for each node
		}
	}
	workersHostPortsCopy := make([]string, len(c.Workers.HostPorts))
	copy(workersHostPortsCopy, c.Workers.HostPorts)

	return &ClusterConfig{
		Enabled: c.Enabled,
		Driver:  c.Driver,
		ControlPlanes: struct {
			Count     *int                  `yaml:"count,omitempty"`
			CPU       *int                  `yaml:"cpu,omitempty"`
			Memory    *int                  `yaml:"memory,omitempty"`
			Nodes     map[string]NodeConfig `yaml:"nodes,omitempty"`
			HostPorts []string              `yaml:"hostports,omitempty"`
		}{
			Count:     c.ControlPlanes.Count,
			CPU:       c.ControlPlanes.CPU,
			Memory:    c.ControlPlanes.Memory,
			Nodes:     controlPlanesNodesCopy,
			HostPorts: controlPlanesHostPortsCopy,
		},
		Workers: struct {
			Count     *int                  `yaml:"count,omitempty"`
			CPU       *int                  `yaml:"cpu,omitempty"`
			Memory    *int                  `yaml:"memory,omitempty"`
			Nodes     map[string]NodeConfig `yaml:"nodes,omitempty"`
			HostPorts []string              `yaml:"hostports,omitempty"`
		}{
			Count:     c.Workers.Count,
			CPU:       c.Workers.CPU,
			Memory:    c.Workers.Memory,
			Nodes:     workersNodesCopy,
			HostPorts: workersHostPortsCopy,
		},
	}
}
