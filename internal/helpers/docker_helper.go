package helpers

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
)

// DockerHelper is a helper struct that provides Docker-specific utility functions
type DockerHelper struct {
	ConfigHandler config.ConfigHandler
	Context       context.ContextInterface
	Helpers       []Helper
}

// NewDockerHelper is a constructor for DockerHelper
func NewDockerHelper(di *di.DIContainer) (*DockerHelper, error) {
	cliConfigHandler, err := di.Resolve("cliConfigHandler")
	if err != nil {
		return nil, fmt.Errorf("error resolving cliConfigHandler: %w", err)
	}

	resolvedContext, err := di.Resolve("context")
	if err != nil {
		return nil, fmt.Errorf("error resolving context: %w", err)
	}

	helpers, err := di.ResolveAll((*Helper)(nil))
	if err != nil {
		return nil, fmt.Errorf("error resolving helpers: %w", err)
	}

	helperInstances := make([]Helper, len(helpers))
	for i, helper := range helpers {
		helperInstances[i] = helper.(Helper)
	}

	return &DockerHelper{
		ConfigHandler: cliConfigHandler.(config.ConfigHandler),
		Context:       resolvedContext.(context.ContextInterface),
		Helpers:       helperInstances,
	}, nil
}

// GetEnvVars retrieves Docker-specific environment variables for the current context
func (h *DockerHelper) GetEnvVars() (map[string]string, error) {
	// Get the configuration root directory
	configRoot, err := h.Context.GetConfigRoot()
	if err != nil {
		return nil, fmt.Errorf("error retrieving config root: %w", err)
	}

	// Check for the existence of docker-compose.yaml or docker-compose.yml
	var composeFilePath string
	yamlPath := filepath.Join(configRoot, "docker-compose.yaml")
	ymlPath := filepath.Join(configRoot, "docker-compose.yml")

	if _, err := os.Stat(yamlPath); err == nil {
		composeFilePath = yamlPath
	} else if _, err := os.Stat(ymlPath); err == nil {
		composeFilePath = ymlPath
	}

	envVars := map[string]string{
		"COMPOSE_FILE": composeFilePath,
	}

	return envVars, nil
}

// PostEnvExec runs any necessary commands after the environment variables have been set.
func (h *DockerHelper) PostEnvExec() error {
	return nil
}

// SetConfig sets the configuration value for the given key
func (h *DockerHelper) SetConfig(key, value string) error {
	currentContext, err := h.Context.GetContext()
	if err != nil {
		return fmt.Errorf("error retrieving current context: %w", err)
	}

	if key == "enabled" {
		if err := h.ConfigHandler.SetConfigValue(fmt.Sprintf("contexts.%s.docker.enabled", currentContext), value); err != nil {
			return fmt.Errorf("error setting docker.enabled: %w", err)
		}
	}

	enabled, err := h.ConfigHandler.GetConfigValue(fmt.Sprintf("contexts.%s.docker.enabled", currentContext))
	if err != nil {
		return fmt.Errorf("error retrieving docker.enabled config: %w", err)
	}
	if enabled == "true" {
		if err := h.writeDockerComposeFile(); err != nil {
			return fmt.Errorf("error writing docker-compose file: %w", err)
		}
		return nil
	}

	return fmt.Errorf("unsupported config key: %s", key)
}

// writeDockerComposeFile is a private method to write the docker-compose configuration to a file.
func (h *DockerHelper) writeDockerComposeFile() error {
	var allContainerConfigs []map[string]interface{}

	// Iterate through each helper and collect container configs
	for _, helper := range h.Helpers {
		if helperInstance, ok := helper.(Helper); ok {
			containerConfig, err := helperInstance.GetContainerConfig()
			if err != nil {
				return fmt.Errorf("error getting container config: %w", err)
			}
			if containerConfig != nil {
				allContainerConfigs = append(allContainerConfigs, containerConfig...)
			}
		}
	}

	// Structure the data for docker-compose
	dockerComposeConfig := map[string]interface{}{
		"services": allContainerConfigs,
	}

	// Serialize the docker-compose config to YAML
	yamlData, err := yamlMarshal(dockerComposeConfig)
	if err != nil {
		return fmt.Errorf("error marshaling docker-compose config to YAML: %w", err)
	}

	// Get the config root and construct the file path
	configRoot, err := h.Context.GetConfigRoot()
	if err != nil {
		return fmt.Errorf("error retrieving config root: %w", err)
	}
	composeFilePath := filepath.Join(configRoot, "docker-compose.yaml")

	// Write the YAML data to the specified file
	err = writeFile(composeFilePath, yamlData, 0644)
	if err != nil {
		return fmt.Errorf("error writing docker-compose file: %w", err)
	}

	return nil
}

// GetContainerConfig returns a list of container data for docker-compose.
func (h *DockerHelper) GetContainerConfig() ([]map[string]interface{}, error) {
	// Stub implementation
	return nil, nil
}

// Ensure DockerHelper implements Helper interface
var _ Helper = (*DockerHelper)(nil)
