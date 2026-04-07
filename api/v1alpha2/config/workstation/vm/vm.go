package workstation

// VMConfig represents the VM configuration
type VMConfig struct {
	Address *string `yaml:"address,omitempty"`
	Arch    *string `yaml:"arch,omitempty"`
	CPU     *int    `yaml:"cpu,omitempty"`
	Disk    *int    `yaml:"disk,omitempty"`
	Driver  *string `yaml:"driver,omitempty"`
	Memory  *int    `yaml:"memory,omitempty"`
	Runtime *string `yaml:"runtime,omitempty"`
}

// Merge performs a deep merge of the current VMConfig with another VMConfig.
func (base *VMConfig) Merge(overlay *VMConfig) {
	if overlay == nil {
		return
	}
	if overlay.Address != nil {
		base.Address = overlay.Address
	}
	if overlay.Arch != nil {
		base.Arch = overlay.Arch
	}
	if overlay.CPU != nil {
		base.CPU = overlay.CPU
	}
	if overlay.Disk != nil {
		base.Disk = overlay.Disk
	}
	if overlay.Driver != nil {
		base.Driver = overlay.Driver
	}
	if overlay.Memory != nil {
		base.Memory = overlay.Memory
	}
	if overlay.Runtime != nil {
		base.Runtime = overlay.Runtime
	}
}

// DeepCopy creates a deep copy of the VMConfig object
func (c *VMConfig) DeepCopy() *VMConfig {
	if c == nil {
		return nil
	}
	copied := &VMConfig{}

	if c.Address != nil {
		addressCopy := *c.Address
		copied.Address = &addressCopy
	}
	if c.Arch != nil {
		archCopy := *c.Arch
		copied.Arch = &archCopy
	}
	if c.CPU != nil {
		cpuCopy := *c.CPU
		copied.CPU = &cpuCopy
	}
	if c.Disk != nil {
		diskCopy := *c.Disk
		copied.Disk = &diskCopy
	}
	if c.Driver != nil {
		driverCopy := *c.Driver
		copied.Driver = &driverCopy
	}
	if c.Memory != nil {
		memoryCopy := *c.Memory
		copied.Memory = &memoryCopy
	}
	if c.Runtime != nil {
		runtimeCopy := *c.Runtime
		copied.Runtime = &runtimeCopy
	}

	return copied
}
