package vsphere

// VSphereConfig represents the vSphere configuration. vSphere integration activates whenever this
// block is present in a context (or when platform is "vsphere"); no separate enabled flag.
type VSphereConfig struct {
	// Server is the vCenter server hostname or IP address, exported as VSPHERE_SERVER.
	Server *string `yaml:"server,omitempty"`

	// User is the vCenter username, exported as VSPHERE_USER.
	User *string `yaml:"user,omitempty"`

	// Datacenter is the vSphere datacenter name for resource provisioning.
	Datacenter *string `yaml:"datacenter,omitempty"`

	// Cluster is the vSphere compute cluster name where VMs are scheduled.
	Cluster *string `yaml:"cluster,omitempty"`

	// Datastore is the vSphere datastore or datastore cluster name for VM disk placement.
	Datastore *string `yaml:"datastore,omitempty"`

	// Network is the default vSphere network/port group for VM interfaces.
	Network *string `yaml:"network,omitempty"`

	// ResourcePool is the default vSphere resource pool for VM placement.
	ResourcePool *string `yaml:"resource_pool,omitempty"`

	// Folder is the default vSphere folder path for VM inventory placement.
	Folder *string `yaml:"folder,omitempty"`

	// Insecure disables TLS certificate verification when connecting to vCenter.
	// Exported as VSPHERE_ALLOW_UNVERIFIED_SSL.
	Insecure *bool `yaml:"insecure,omitempty"`
}

// Merge performs a deep merge of the current VSphereConfig with another VSphereConfig.
func (base *VSphereConfig) Merge(overlay *VSphereConfig) {
	if overlay == nil {
		return
	}
	if overlay.Server != nil {
		base.Server = overlay.Server
	}
	if overlay.User != nil {
		base.User = overlay.User
	}
	if overlay.Datacenter != nil {
		base.Datacenter = overlay.Datacenter
	}
	if overlay.Cluster != nil {
		base.Cluster = overlay.Cluster
	}
	if overlay.Datastore != nil {
		base.Datastore = overlay.Datastore
	}
	if overlay.Network != nil {
		base.Network = overlay.Network
	}
	if overlay.ResourcePool != nil {
		base.ResourcePool = overlay.ResourcePool
	}
	if overlay.Folder != nil {
		base.Folder = overlay.Folder
	}
	if overlay.Insecure != nil {
		base.Insecure = overlay.Insecure
	}
}

// DeepCopy creates a deep copy of the VSphereConfig object.
func (c *VSphereConfig) DeepCopy() *VSphereConfig {
	if c == nil {
		return nil
	}
	copied := &VSphereConfig{}

	if c.Server != nil {
		serverCopy := *c.Server
		copied.Server = &serverCopy
	}
	if c.User != nil {
		userCopy := *c.User
		copied.User = &userCopy
	}
	if c.Datacenter != nil {
		datacenterCopy := *c.Datacenter
		copied.Datacenter = &datacenterCopy
	}
	if c.Cluster != nil {
		clusterCopy := *c.Cluster
		copied.Cluster = &clusterCopy
	}
	if c.Datastore != nil {
		datastoreCopy := *c.Datastore
		copied.Datastore = &datastoreCopy
	}
	if c.Network != nil {
		networkCopy := *c.Network
		copied.Network = &networkCopy
	}
	if c.ResourcePool != nil {
		resourcePoolCopy := *c.ResourcePool
		copied.ResourcePool = &resourcePoolCopy
	}
	if c.Folder != nil {
		folderCopy := *c.Folder
		copied.Folder = &folderCopy
	}
	if c.Insecure != nil {
		insecureCopy := *c.Insecure
		copied.Insecure = &insecureCopy
	}

	return copied
}
