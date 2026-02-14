package docker

// DockerConfig represents the Docker configuration
type DockerConfig struct {
	// Enabled toggles the internal workstation (docker-compose). Deprecated: defaults to false;
	// use provider and vm.driver for workstation mode. When false, omit from config (not written to windsor.yaml).
	Enabled     *bool                     `yaml:"enabled,omitempty"`
	RegistryURL string                    `yaml:"registry_url,omitempty"`
	Registries  map[string]RegistryConfig `yaml:"registries,omitempty"`
}

// RegistryConfig represents the registry configuration
type RegistryConfig struct {
	Remote   string `yaml:"remote,omitempty"`
	Local    string `yaml:"local,omitempty"`
	HostName string `yaml:"hostname,omitempty"`
	HostPort int    `yaml:"hostport,omitempty"`
}

// Merge performs a deep merge of the current DockerConfig with another DockerConfig.
func (base *DockerConfig) Merge(overlay *DockerConfig) {
	if overlay.Enabled != nil {
		base.Enabled = overlay.Enabled
	}

	if overlay.RegistryURL != "" {
		base.RegistryURL = overlay.RegistryURL
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

	registriesCopy := make(map[string]RegistryConfig)
	for name, registry := range c.Registries {
		registriesCopy[name] = RegistryConfig{
			Remote:   registry.Remote,
			Local:    registry.Local,
			HostName: registry.HostName,
			HostPort: registry.HostPort,
		}
	}

	return &DockerConfig{
		Enabled:     enabledCopy,
		RegistryURL: c.RegistryURL,
		Registries:  registriesCopy,
	}
}

func ptrBool(b bool) *bool {
	return &b
}
