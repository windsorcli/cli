package docker

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

	// Create a map to uniquely index base registries by Name
	registryMap := make(map[string]RegistryConfig)
	for _, registry := range base.Registries {
		registryMap[registry.Name] = registry
	}

	// Merge overlay registries into the base, uniquely indexed by Name
	for _, overlayRegistry := range overlay.Registries {
		if baseRegistry, exists := registryMap[overlayRegistry.Name]; exists {
			if overlayRegistry.Remote != "" {
				baseRegistry.Remote = overlayRegistry.Remote
			}
			if overlayRegistry.Local != "" {
				baseRegistry.Local = overlayRegistry.Local
			}
			registryMap[overlayRegistry.Name] = baseRegistry
		} else {
			registryMap[overlayRegistry.Name] = overlayRegistry
		}
	}

	// Update base.Registries with merged results
	base.Registries = make([]RegistryConfig, 0, len(registryMap))
	for _, registry := range registryMap {
		base.Registries = append(base.Registries, registry)
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

// Helper functions to create pointers for basic types
func ptrString(s string) *string {
	return &s
}

func ptrBool(b bool) *bool {
	return &b
}
