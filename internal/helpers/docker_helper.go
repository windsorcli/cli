package helpers

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

const registryImage = "registry:2.8.3"

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

	// Register DockerHelper as a Helper
	dockerHelper := &DockerHelper{
		ConfigHandler: cliConfigHandler.(config.ConfigHandler),
		Context:       resolvedContext.(context.ContextInterface),
	}
	helperInstances := make([]Helper, len(helpers)+1) // Increase the slice size by 1
	for i, helper := range helpers {
		helperInstances[i] = helper.(Helper)
	}

	// Add DockerHelper to the list of helpers
	helperInstances[len(helpers)] = dockerHelper

	dockerHelper.Helpers = helperInstances

	return dockerHelper, nil
}

// GetEnvVars retrieves Docker-specific environment variables for the current context
func (h *DockerHelper) GetEnvVars() (map[string]string, error) {
	// Get the configuration root directory
	configRoot, err := h.Context.GetConfigRoot()
	if err != nil {
		return nil, fmt.Errorf("error retrieving config root: %w", err)
	}

	// Check for the existence of compose.yaml or compose.yml
	var composeFilePath string
	yamlPath := filepath.Join(configRoot, "compose.yaml")
	ymlPath := filepath.Join(configRoot, "compose.yml")

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
	if key != "enabled" {
		return fmt.Errorf("unsupported config key: %s", key)
	}

	context, err := h.Context.GetContext()
	if err != nil {
		return fmt.Errorf("error retrieving context: %w", err)
	}

	var boolValue bool
	if value == "" {
		if context == "local" || strings.HasPrefix(context, "local-") {
			boolValue = true
		} else {
			return nil
		}
	} else {
		boolValue = value == "true"
	}

	// Proceed with setting the configuration
	err = h.ConfigHandler.SetConfigValue(fmt.Sprintf("contexts.%s.docker.enabled", context), boolValue)
	if err != nil {
		return err
	}

	// Check if the "enabled" key is set to "true" before writing the docker compose file
	enabledValue, err := h.ConfigHandler.GetConfigValue(fmt.Sprintf("contexts.%s.docker.enabled", context))
	if err != nil {
		return fmt.Errorf("error retrieving docker enabled config: %w", err)
	}

	if enabledValue == "true" {
		return h.writeDockerComposeFile()
	}

	return nil
}

// writeDockerComposeFile is a private method to write the docker-compose configuration to a file.
func (h *DockerHelper) writeDockerComposeFile() error {
	services := make(map[string]interface{})

	// Iterate through each helper and collect container configs
	for _, helper := range h.Helpers {
		if helperInstance, ok := helper.(Helper); ok {
			helperName := fmt.Sprintf("%T", helperInstance)
			containerConfigs, err := helperInstance.GetContainerConfig()
			if err != nil {
				return fmt.Errorf("error getting container config from helper %s: %w", helperName, err)
			}
			for _, containerConfig := range containerConfigs {
				for key, value := range containerConfig {
					strKey := fmt.Sprintf("%v", key)
					services[strKey] = value
				}
			}
		}
	}

	// Structure the data for docker-compose
	dockerComposeConfig := map[string]interface{}{
		"services": services,
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
	composeFilePath := filepath.Join(configRoot, "compose.yaml")

	// Ensure the parent context folder exists
	if err := mkdirAll(filepath.Dir(composeFilePath), 0755); err != nil {
		return fmt.Errorf("error creating parent context folder: %w", err)
	}

	// Write the YAML data to the specified file
	err = writeFile(composeFilePath, yamlData, 0644)
	if err != nil {
		return fmt.Errorf("error writing docker-compose file: %w", err)
	}

	return nil
}

// GetContainerConfig returns a list of container data for docker-compose.
func (h *DockerHelper) GetContainerConfig() ([]map[string]interface{}, error) {
	registryConfig := map[string]interface{}{
		"image":   registryImage,
		"restart": "always",
		"labels": map[string]string{
			"role":      "registry",
			"managedBy": "windsor",
		},
	}

	return []map[string]interface{}{
		{"registry.test": registryConfig},
	}, nil
}

// Ensure DockerHelper implements Helper interface
var _ Helper = (*DockerHelper)(nil)
