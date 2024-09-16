package helpers

import (
	"fmt"

	"github.com/windsor-hotel/cli/internal/config"
)

// CLIHelper is a helper struct that uses ConfigHandler for configuration management
type CLIHelper struct {
	configHandler config.ConfigHandler
}

// GetEnvVars retrieves environment variables from the config handler
func (h *CLIHelper) GetEnvVars() (map[string]string, error) {
	envVars, err := h.configHandler.GetNestedMap("environment")
	if err != nil {
		return nil, fmt.Errorf("error retrieving environment variables: %w", err)
	}
	return envVars, nil
}

// GetAlias retrieves aliases from the config handler
func (h *CLIHelper) GetAlias() (map[string]string, error) {
	aliases, err := h.configHandler.GetNestedMap("aliases")
	if err != nil {
		return nil, fmt.Errorf("error retrieving aliases: %w", err)
	}
	return aliases, nil
}

// PrintEnvVars prints the export or unset commands for each managed environment variable
func (h *CLIHelper) PrintEnvVars() error {
	envVars, err := h.GetEnvVars()
	if err != nil {
		return err
	}
	for key, value := range envVars {
		if value != "" {
			fmt.Printf("export %s='%s'\n", key, value)
		} else {
			fmt.Printf("unset %s\n", key)
		}
	}
	return nil
}

// PrintAlias prints the alias or unalias commands for each managed alias
func (h *CLIHelper) PrintAlias() error {
	aliases, err := h.GetAlias()
	if err != nil {
		return err
	}
	for key, value := range aliases {
		if value != "" {
			fmt.Printf("alias %s='%s'\n", key, value)
		} else {
			fmt.Printf("alias_output=$(type %s 2>/dev/null) && [[ $alias_output == *\"alias\"* ]] && unalias %s\n", key, key)
		}
	}
	return nil
}

// GetDockerComposeConfig returns a dictionary representing the configuration for a docker-compose.yaml file
func (h *CLIHelper) GetDockerComposeConfig() (map[string]interface{}, error) {
	config, err := h.configHandler.GetNestedMap("docker_compose")
	if err != nil {
		return nil, fmt.Errorf("error retrieving docker-compose configuration: %w", err)
	}
	return config, nil
}
