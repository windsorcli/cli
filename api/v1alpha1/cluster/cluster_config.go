package cluster

// ClusterConfig represents the cluster configuration
type ClusterConfig struct {
	Enabled       *bool           `yaml:"enabled,omitempty"`
	Driver        *string         `yaml:"driver,omitempty"`
	Endpoint      *string         `yaml:"endpoint,omitempty"`
	Image         *string         `yaml:"image,omitempty"`
	ControlPlanes NodeGroupConfig `yaml:"controlplanes,omitempty"`
	Workers       NodeGroupConfig `yaml:"workers,omitempty"`
}

// NodeConfig represents the node configuration
type NodeConfig struct {
	Hostname  *string  `yaml:"hostname,omitempty"`
	Node      *string  `yaml:"node,omitempty"`
	Endpoint  *string  `yaml:"endpoint,omitempty"`
	Image     *string  `yaml:"image,omitempty"`
	HostPorts []string `yaml:"hostports,omitempty"`
}

// NodeGroupConfig represents the configuration for a group of nodes
type NodeGroupConfig struct {
	Count       *int                  `yaml:"count,omitempty"`
	CPU         *int                  `yaml:"cpu,omitempty"`
	Memory      *int                  `yaml:"memory,omitempty"`
	Image       *string               `yaml:"image,omitempty"`
	Schedulable *bool                 `yaml:"schedulable,omitempty"`
	Nodes       map[string]NodeConfig `yaml:"-"`
	HostPorts   []string              `yaml:"hostports,omitempty"`
	Volumes     []string              `yaml:"volumes,omitempty"`
}

// Merge performs a deep merge of the current ClusterConfig with another ClusterConfig.
func (base *ClusterConfig) Merge(overlay *ClusterConfig) {
	if overlay.Enabled != nil {
		base.Enabled = overlay.Enabled
	}
	if overlay.Driver != nil {
		base.Driver = overlay.Driver
	}
	if overlay.Endpoint != nil {
		base.Endpoint = overlay.Endpoint
	}
	if overlay.Image != nil {
		base.Image = overlay.Image
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
	if overlay.ControlPlanes.Image != nil {
		base.ControlPlanes.Image = overlay.ControlPlanes.Image
	}
	if overlay.ControlPlanes.Schedulable != nil {
		base.ControlPlanes.Schedulable = overlay.ControlPlanes.Schedulable
	}
	if overlay.ControlPlanes.Nodes != nil {
		base.ControlPlanes.Nodes = make(map[string]NodeConfig, len(overlay.ControlPlanes.Nodes))
		for key, node := range overlay.ControlPlanes.Nodes {
			base.ControlPlanes.Nodes[key] = node
		}
	}
	if overlay.ControlPlanes.HostPorts != nil {
		base.ControlPlanes.HostPorts = make([]string, len(overlay.ControlPlanes.HostPorts))
		copy(base.ControlPlanes.HostPorts, overlay.ControlPlanes.HostPorts)
	}
	if overlay.ControlPlanes.Volumes != nil {
		base.ControlPlanes.Volumes = make([]string, len(overlay.ControlPlanes.Volumes))
		copy(base.ControlPlanes.Volumes, overlay.ControlPlanes.Volumes)
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
	if overlay.Workers.Image != nil {
		base.Workers.Image = overlay.Workers.Image
	}
	if overlay.Workers.Schedulable != nil {
		base.Workers.Schedulable = overlay.Workers.Schedulable
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
	if overlay.Workers.Volumes != nil {
		base.Workers.Volumes = make([]string, len(overlay.Workers.Volumes))
		copy(base.Workers.Volumes, overlay.Workers.Volumes)
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
			Image:     node.Image,
			HostPorts: append([]string{}, node.HostPorts...),
		}
	}
	controlPlanesHostPortsCopy := make([]string, len(c.ControlPlanes.HostPorts))
	copy(controlPlanesHostPortsCopy, c.ControlPlanes.HostPorts)
	controlPlanesVolumesCopy := make([]string, len(c.ControlPlanes.Volumes))
	copy(controlPlanesVolumesCopy, c.ControlPlanes.Volumes)

	workersNodesCopy := make(map[string]NodeConfig, len(c.Workers.Nodes))
	for key, node := range c.Workers.Nodes {
		workersNodesCopy[key] = NodeConfig{
			Hostname:  node.Hostname,
			Node:      node.Node,
			Endpoint:  node.Endpoint,
			Image:     node.Image,
			HostPorts: append([]string{}, node.HostPorts...),
		}
	}
	workersHostPortsCopy := make([]string, len(c.Workers.HostPorts))
	copy(workersHostPortsCopy, c.Workers.HostPorts)
	workersVolumesCopy := make([]string, len(c.Workers.Volumes))
	copy(workersVolumesCopy, c.Workers.Volumes)

	return &ClusterConfig{
		Enabled:  c.Enabled,
		Driver:   c.Driver,
		Endpoint: c.Endpoint,
		Image:    c.Image,
		ControlPlanes: NodeGroupConfig{
			Count:       c.ControlPlanes.Count,
			CPU:         c.ControlPlanes.CPU,
			Memory:      c.ControlPlanes.Memory,
			Image:       c.ControlPlanes.Image,
			Schedulable: c.ControlPlanes.Schedulable,
			Nodes:       controlPlanesNodesCopy,
			HostPorts:   controlPlanesHostPortsCopy,
			Volumes:     controlPlanesVolumesCopy,
		},
		Workers: NodeGroupConfig{
			Count:       c.Workers.Count,
			CPU:         c.Workers.CPU,
			Memory:      c.Workers.Memory,
			Image:       c.Workers.Image,
			Schedulable: c.Workers.Schedulable,
			Nodes:       workersNodesCopy,
			HostPorts:   workersHostPortsCopy,
			Volumes:     workersVolumesCopy,
		},
	}
}
