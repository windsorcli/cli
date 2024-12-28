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

	// The base.Registries is already a map, so no need to create a new map
	registryMap := base.Registries

	// Merge overlay registries into the base, uniquely indexed by registry key
	for key, overlayRegistry := range overlay.Registries {
		if baseRegistry, exists := registryMap[key]; exists {
			if overlayRegistry.Remote != "" {
				baseRegistry.Remote = overlayRegistry.Remote
			}
			if overlayRegistry.Local != "" {
				baseRegistry.Local = overlayRegistry.Local
			}
			if overlayRegistry.Hostname != "" {
				baseRegistry.Hostname = overlayRegistry.Hostname
			}
			registryMap[key] = baseRegistry
		} else {
			registryMap[key] = overlayRegistry
		}
	}

	// Update base.Registries with merged results
	base.Registries = registryMap
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
