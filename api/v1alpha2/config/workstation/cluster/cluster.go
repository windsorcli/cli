package workstation

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
	Nodes       map[string]NodeConfig `yaml:"nodes,omitempty"`
	HostPorts   []string              `yaml:"hostports,omitempty"`
	Volumes     []string              `yaml:"volumes,omitempty"`
}

// Merge performs a deep merge of the current ClusterConfig with another ClusterConfig.
func (base *ClusterConfig) Merge(overlay *ClusterConfig) {
	if overlay == nil {
		return
	}
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

// DeepCopy creates a deep copy of the ClusterConfig object
func (c *ClusterConfig) DeepCopy() *ClusterConfig {
	if c == nil {
		return nil
	}

	var controlPlanesNodesCopy map[string]NodeConfig
	if len(c.ControlPlanes.Nodes) > 0 {
		controlPlanesNodesCopy = make(map[string]NodeConfig, len(c.ControlPlanes.Nodes))
		for key, node := range c.ControlPlanes.Nodes {
			var hostnameCopy *string
			if node.Hostname != nil {
				hostnameCopy = new(string)
				*hostnameCopy = *node.Hostname
			}
			var nodeCopy *string
			if node.Node != nil {
				nodeCopy = new(string)
				*nodeCopy = *node.Node
			}
			var endpointCopy *string
			if node.Endpoint != nil {
				endpointCopy = new(string)
				*endpointCopy = *node.Endpoint
			}
			var imageCopy *string
			if node.Image != nil {
				imageCopy = new(string)
				*imageCopy = *node.Image
			}
			controlPlanesNodesCopy[key] = NodeConfig{
				Hostname:  hostnameCopy,
				Node:      nodeCopy,
				Endpoint:  endpointCopy,
				Image:     imageCopy,
				HostPorts: append([]string{}, node.HostPorts...),
			}
		}
	}
	var controlPlanesHostPortsCopy []string
	if len(c.ControlPlanes.HostPorts) > 0 {
		controlPlanesHostPortsCopy = make([]string, len(c.ControlPlanes.HostPorts))
		copy(controlPlanesHostPortsCopy, c.ControlPlanes.HostPorts)
	}
	var controlPlanesVolumesCopy []string
	if len(c.ControlPlanes.Volumes) > 0 {
		controlPlanesVolumesCopy = make([]string, len(c.ControlPlanes.Volumes))
		copy(controlPlanesVolumesCopy, c.ControlPlanes.Volumes)
	}

	var workersNodesCopy map[string]NodeConfig
	if len(c.Workers.Nodes) > 0 {
		workersNodesCopy = make(map[string]NodeConfig, len(c.Workers.Nodes))
		for key, node := range c.Workers.Nodes {
			var hostnameCopy *string
			if node.Hostname != nil {
				hostnameCopy = new(string)
				*hostnameCopy = *node.Hostname
			}
			var nodeCopy *string
			if node.Node != nil {
				nodeCopy = new(string)
				*nodeCopy = *node.Node
			}
			var endpointCopy *string
			if node.Endpoint != nil {
				endpointCopy = new(string)
				*endpointCopy = *node.Endpoint
			}
			var imageCopy *string
			if node.Image != nil {
				imageCopy = new(string)
				*imageCopy = *node.Image
			}
			workersNodesCopy[key] = NodeConfig{
				Hostname:  hostnameCopy,
				Node:      nodeCopy,
				Endpoint:  endpointCopy,
				Image:     imageCopy,
				HostPorts: append([]string{}, node.HostPorts...),
			}
		}
	}
	var workersHostPortsCopy []string
	if len(c.Workers.HostPorts) > 0 {
		workersHostPortsCopy = make([]string, len(c.Workers.HostPorts))
		copy(workersHostPortsCopy, c.Workers.HostPorts)
	}
	var workersVolumesCopy []string
	if len(c.Workers.Volumes) > 0 {
		workersVolumesCopy = make([]string, len(c.Workers.Volumes))
		copy(workersVolumesCopy, c.Workers.Volumes)
	}

	var enabledCopy *bool
	if c.Enabled != nil {
		enabledCopy = new(bool)
		*enabledCopy = *c.Enabled
	}
	var driverCopy *string
	if c.Driver != nil {
		driverCopy = new(string)
		*driverCopy = *c.Driver
	}
	var endpointCopy *string
	if c.Endpoint != nil {
		endpointCopy = new(string)
		*endpointCopy = *c.Endpoint
	}
	var imageCopy *string
	if c.Image != nil {
		imageCopy = new(string)
		*imageCopy = *c.Image
	}

	return &ClusterConfig{
		Enabled:  enabledCopy,
		Driver:   driverCopy,
		Endpoint: endpointCopy,
		Image:    imageCopy,
		ControlPlanes: NodeGroupConfig{
			Count: func() *int {
				if c.ControlPlanes.Count != nil {
					count := *c.ControlPlanes.Count
					return &count
				}
				return nil
			}(),
			CPU: func() *int {
				if c.ControlPlanes.CPU != nil {
					cpu := *c.ControlPlanes.CPU
					return &cpu
				}
				return nil
			}(),
			Memory: func() *int {
				if c.ControlPlanes.Memory != nil {
					memory := *c.ControlPlanes.Memory
					return &memory
				}
				return nil
			}(),
			Image: func() *string {
				if c.ControlPlanes.Image != nil {
					image := *c.ControlPlanes.Image
					return &image
				}
				return nil
			}(),
			Schedulable: func() *bool {
				if c.ControlPlanes.Schedulable != nil {
					schedulable := *c.ControlPlanes.Schedulable
					return &schedulable
				}
				return nil
			}(),
			Nodes:     controlPlanesNodesCopy,
			HostPorts: controlPlanesHostPortsCopy,
			Volumes:   controlPlanesVolumesCopy,
		},
		Workers: NodeGroupConfig{
			Count: func() *int {
				if c.Workers.Count != nil {
					count := *c.Workers.Count
					return &count
				}
				return nil
			}(),
			CPU: func() *int {
				if c.Workers.CPU != nil {
					cpu := *c.Workers.CPU
					return &cpu
				}
				return nil
			}(),
			Memory: func() *int {
				if c.Workers.Memory != nil {
					memory := *c.Workers.Memory
					return &memory
				}
				return nil
			}(),
			Image: func() *string {
				if c.Workers.Image != nil {
					image := *c.Workers.Image
					return &image
				}
				return nil
			}(),
			Schedulable: func() *bool {
				if c.Workers.Schedulable != nil {
					schedulable := *c.Workers.Schedulable
					return &schedulable
				}
				return nil
			}(),
			Nodes:     workersNodesCopy,
			HostPorts: workersHostPortsCopy,
			Volumes:   workersVolumesCopy,
		},
	}
}
