package docker

// DockerConfig represents the Docker configuration
type DockerConfig struct {
	Enabled     *bool                     `yaml:"enabled"`
	Registries  map[string]RegistryConfig `yaml:"registries,omitempty"`
	NetworkCIDR *string                   `yaml:"network_cidr,omitempty"`
}

// RegistryConfig represents the registry configuration
type RegistryConfig struct {
	Remote   string `yaml:"remote,omitempty"`
	Local    string `yaml:"local,omitempty"`
	Hostname string `yaml:"hostname,omitempty"`
}

// Merge performs a deep merge of the current DockerConfig with another DockerConfig.
func (base *DockerConfig) Merge(overlay *DockerConfig) {
	if overlay.Enabled != nil {
		base.Enabled = overlay.Enabled
	}
	if overlay.NetworkCIDR != nil {
		base.NetworkCIDR = overlay.NetworkCIDR
	}

	// Overwrite base.Registries entirely with overlay.Registries if defined, otherwise keep base.Registries
	if overlay.Registries != nil {
		base.Registries = overlay.Registries
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

	registriesCopy := make(map[string]RegistryConfig)
	for name, registry := range c.Registries {
		registriesCopy[name] = RegistryConfig{
			Remote:   registry.Remote,
			Local:    registry.Local,
			Hostname: registry.Hostname,
		}
	}

	return &DockerConfig{
		Enabled:     enabledCopy,
		Registries:  registriesCopy,
		NetworkCIDR: networkCIDRCopy,
	}
}

// Helper functions to create pointers for basic types
func ptrString(s string) *string {
	return &s
}

func ptrBool(b bool) *bool {
	return &b
}
