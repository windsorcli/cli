package helpers

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/compose-spec/compose-go/types"
	"github.com/goccy/go-yaml"
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

var defaultRegistries = []map[string]string{
	{"name": "registry.test", "local": "", "remote": ""},
	{"name": "registry-1.docker.test", "local": "https://docker.io", "remote": "https://registry-1.docker.io"},
	{"name": "registry.k8s.test", "local": "", "remote": "https://registry.k8s.io"},
	{"name": "gcr.test", "local": "", "remote": "https://gcr.io"},
	{"name": "ghcr.test", "local": "", "remote": "https://ghcr.io"},
	{"name": "quay.test", "local": "", "remote": "https://quay.io"},
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
	return nil
}

// generateRegistryService creates a ServiceConfig for a Docker registry service
// with the specified name, remote URL, and local URL.
func generateRegistryService(name, remoteURL, localURL string) types.ServiceConfig {
	// Initialize the ServiceConfig with the provided name, a predefined image,
	// a restart policy, and labels indicating the role and manager.
	service := types.ServiceConfig{
		Name:    name,
		Image:   registryImage,
		Restart: "always",
		Labels: map[string]string{
			"role":       "registry",
			"managed_by": "windsor",
		},
	}

	// Initialize the environment variables map.
	env := make(types.MappingWithEquals)

	// Add the remote URL to the environment variables if specified.
	if remoteURL != "" {
		env["REGISTRY_PROXY_REMOTEURL"] = &remoteURL
	}

	// Add the local URL to the environment variables if specified.
	if localURL != "" {
		env["REGISTRY_PROXY_LOCALURL"] = &localURL
	}

	// If any environment variables were added, assign them to the service.
	if len(env) > 0 {
		service.Environment = env
	}

	// Return the configured ServiceConfig.
	return service
}

// GetContainerConfig returns a list of container data for docker-compose.
func (h *DockerHelper) GetContainerConfig() ([]types.ServiceConfig, error) {
	// Prepare the services slice for docker-compose
	var services []types.ServiceConfig

	// Retrieve the list of registries from the configuration
	context, err := h.Context.GetContext()
	if err != nil {
		return nil, fmt.Errorf("error retrieving context: %w", err)
	}

	registries, err := h.ConfigHandler.GetConfigValue(fmt.Sprintf("contexts.%s.docker.registries", context), "")
	if err != nil {
		return nil, fmt.Errorf("error retrieving registries from configuration: %w", err)
	}

	dockerEnabled, err := h.ConfigHandler.GetConfigValue(fmt.Sprintf("contexts.%s.docker.enabled", context), "")
	if err != nil {
		return nil, fmt.Errorf("error retrieving docker enabled status from configuration: %w", err)
	}

	var registriesList []types.ServiceConfig
	if registries == "" && dockerEnabled == "true" {
		// No registries defined but docker is enabled, use default registries
		for _, registry := range defaultRegistries {
			registriesList = append(registriesList, generateRegistryService(registry["name"], registry["remote"], registry["local"]))
		}
	} else if registries != "" {
		// Parse the registries YAML into a list of objects
		var parsedRegistries []map[string]string
		err := yaml.Unmarshal([]byte(registries), &parsedRegistries)
		if err != nil {
			return nil, fmt.Errorf("error unmarshaling registries YAML: %w", err)
		}

		// Create registriesList from the parsed registries
		for _, registry := range parsedRegistries {
			name := registry["name"]
			remote := registry["remote"]
			local := registry["local"]
			registriesList = append(registriesList, generateRegistryService(name, remote, local))
		}
	}

	// Iterate over the registries and create service definitions using generateRegistryService
	for _, registry := range registriesList {
		services = append(services, registry)
	}

	return services, nil
}

// WriteConfig writes any vendor specific configuration files that are needed for the helper.
func (h *DockerHelper) WriteConfig() error {
	var services []types.ServiceConfig

	// Iterate through each helper and collect container configs
	for _, helper := range h.Helpers {
		if helperInstance, ok := helper.(Helper); ok {
			helperName := fmt.Sprintf("%T", helperInstance)
			containerConfigs, err := helperInstance.GetContainerConfig()
			if err != nil {
				return fmt.Errorf("error getting container config from helper %s: %w", helperName, err)
			}
			for _, containerConfig := range containerConfigs {
				services = append(services, containerConfig)
			}
		}
	}

	// Create a Project using compose-go
	project := &types.Project{
		Services: services,
	}

	// Serialize the docker-compose config to YAML
	yamlData, err := yamlMarshal(project)
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

// Ensure DockerHelper implements Helper interface
var _ Helper = (*DockerHelper)(nil)
