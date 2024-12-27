package config

// DockerConfig represents the Docker configuration
type DockerConfig struct {
	Enabled     *bool            `yaml:"enabled"`
	Registries  []RegistryConfig `yaml:"registries,omitempty"`
	NetworkCIDR *string          `yaml:"network_cidr,omitempty"`
}

// RegistryConfig represents the registry configuration
type RegistryConfig struct {
	Name   string `yaml:"name"`
	Remote string `yaml:"remote,omitempty"`
	Local  string `yaml:"local,omitempty"`
}

// Merge performs a deep merge of the current DockerConfig with another DockerConfig.
func (base *DockerConfig) Merge(overlay *DockerConfig) {
	if overlay.Enabled != nil {
		base.Enabled = overlay.Enabled
	}
	if overlay.NetworkCIDR != nil {
		base.NetworkCIDR = overlay.NetworkCIDR
	}
	for i, overlayRegistry := range overlay.Registries {
		if i < len(base.Registries) {
			if overlayRegistry.Name != "" {
				base.Registries[i].Name = overlayRegistry.Name
			}
			if overlayRegistry.Remote != "" {
				base.Registries[i].Remote = overlayRegistry.Remote
			}
			if overlayRegistry.Local != "" {
				base.Registries[i].Local = overlayRegistry.Local
			}
		} else {
			base.Registries = append(base.Registries, overlayRegistry)
		}
	}
}

// Copy creates a deep copy of the DockerConfig object
func (c *DockerConfig) Copy() *DockerConfig {
	if c == nil {
		return nil
	}

	var enabledCopy *bool
	if c.Enabled != nil {
		enabledCopy = ptrBool(*c.Enabled)
	}

	var networkCIDRCopy *string
	if c.NetworkCIDR != nil {
		networkCIDRCopy = ptrString(*c.NetworkCIDR)
	}

	registriesCopy := make([]RegistryConfig, len(c.Registries))
	for i, registry := range c.Registries {
		registriesCopy[i] = RegistryConfig{
			Name:   registry.Name,
			Remote: registry.Remote,
			Local:  registry.Local,
		}
	}

	return &DockerConfig{
		Enabled:     enabledCopy,
		Registries:  registriesCopy,
		NetworkCIDR: networkCIDRCopy,
	}
}
