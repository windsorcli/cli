package workstation

// RegistriesConfig represents the container registries configuration
type RegistriesConfig struct {
	Enabled    *bool                     `yaml:"enabled,omitempty"`
	Registries map[string]RegistryConfig `yaml:"registries,omitempty"`
}

// RegistryConfig represents the registry configuration
type RegistryConfig struct {
	Remote   string `yaml:"remote,omitempty"`
	Local    string `yaml:"local,omitempty"`
	HostName string `yaml:"hostname,omitempty"`
	HostPort int    `yaml:"hostport,omitempty"`
}

// Merge performs a deep merge of the current RegistriesConfig with another RegistriesConfig.
func (base *RegistriesConfig) Merge(overlay *RegistriesConfig) {
	if overlay == nil {
		return
	}

	if overlay.Enabled != nil {
		base.Enabled = overlay.Enabled
	}

	// Overwrite base.Registries entirely with overlay.Registries if defined, otherwise keep base.Registries
	if overlay.Registries != nil {
		base.Registries = overlay.Registries
	}
}

// DeepCopy creates a deep copy of the RegistriesConfig object
func (c *RegistriesConfig) DeepCopy() *RegistriesConfig {
	if c == nil {
		return nil
	}

	var enabledCopy *bool
	if c.Enabled != nil {
		enabledCopy = ptrBool(*c.Enabled)
	}

	var registriesCopy map[string]RegistryConfig
	if c.Registries != nil {
		registriesCopy = make(map[string]RegistryConfig)
		for name, registry := range c.Registries {
			registriesCopy[name] = RegistryConfig{
				Remote:   registry.Remote,
				Local:    registry.Local,
				HostName: registry.HostName,
				HostPort: registry.HostPort,
			}
		}
	}

	return &RegistriesConfig{
		Enabled:    enabledCopy,
		Registries: registriesCopy,
	}
}

// Helper function to create boolean pointers
func ptrBool(b bool) *bool {
	return &b
}
