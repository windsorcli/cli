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

var defaultRegistries = []map[string]interface{}{
	generateRegistryService("registry.test", "", ""),
	generateRegistryService("registry-1.docker.test", "https://registry-1.docker.io", "https://docker.io"),
	generateRegistryService("registry.k8s.test", "https://registry.k8s.io", ""),
	generateRegistryService("gcr.test", "https://gcr.io", ""),
	generateRegistryService("ghcr.test", "https://ghcr.io", ""),
	generateRegistryService("quay.test", "https://quay.io", ""),
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
	if value == "" {
		return nil
	}

	context, err := h.Context.GetContext()
	if err != nil {
		return fmt.Errorf("error retrieving context: %w", err)
	}

	var configKey string
	switch key {
	case "enabled":
		configKey = fmt.Sprintf("contexts.%s.docker.enabled", context)
	case "registry_enabled":
		configKey = fmt.Sprintf("contexts.%s.docker.registry_enabled", context)
	default:
		return fmt.Errorf("unsupported config key: %s", key)
	}

	boolValue := value == "true"
	err = h.ConfigHandler.SetConfigValue(configKey, boolValue)
	if err != nil {
		return fmt.Errorf("error setting config value for %s: %w", key, err)
	}

	// If the "enabled" key is set to "true", write the docker compose file
	if key == "enabled" && boolValue {
		return h.writeDockerComposeFile()
	}

	return nil
}

func generateRegistryService(name, remoteURL, localURL string) map[string]interface{} {
	service := map[string]interface{}{
		"name":    name,
		"image":   registryImage,
		"restart": "always",
		"labels": map[string]string{
			"role":       "registry",
			"managed_by": "windsor",
		},
	}

	// Add environment variables if remote or local URLs are specified
	env := make(map[string]interface{})
	if remoteURL != "" {
		env["REGISTRY_PROXY_REMOTEURL"] = remoteURL
	}
	if localURL != "" {
		env["REGISTRY_PROXY_LOCALURL"] = localURL
	}
	if len(env) > 0 {
		service["environment"] = env
	}

	return service
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
	// Load the configuration
	err := h.ConfigHandler.LoadConfig("")
	if err != nil {
		return nil, fmt.Errorf("error loading configuration: %w", err)
	}

	// Prepare the services slice for docker-compose
	var services []map[string]interface{}

	// Retrieve the list of registries from the configuration
	context, err := h.Context.GetContext()
	if err != nil {
		return nil, fmt.Errorf("error retrieving context: %w", err)
	}

	registries, err := h.ConfigHandler.GetConfigValue(fmt.Sprintf("contexts.%s.docker.registries", context))
	if err != nil {
		return nil, fmt.Errorf("error retrieving registries from configuration: %w", err)
	}

	var registriesList []map[string]interface{}
	if registries == "" || context == "local" || strings.HasPrefix(context, "local-") {
		// No registries defined or context is local, use default registries
		registriesList = defaultRegistries
	} else {
		// Attempt to unmarshal the registries YAML into the registriesList
		var unmarshalledRegistries []map[string]interface{}
		if err := yamlUnmarshal([]byte(registries), &unmarshalledRegistries); err != nil {
			return nil, fmt.Errorf("error unmarshaling registries YAML: %w", err)
		}
		registriesList = unmarshalledRegistries
	}

	// Iterate over the registries and create service definitions
	for _, registry := range registriesList {
		name := registry["name"].(string)
		remoteURL := ""
		localURL := ""
		if env, ok := registry["environment"].(map[string]interface{}); ok {
			if remote, ok := env["REGISTRY_PROXY_REMOTEURL"].(string); ok {
				remoteURL = remote
			}
			if local, ok := env["REGISTRY_PROXY_LOCALURL"].(string); ok {
				localURL = local
			}
		}
		service := generateRegistryService(name, remoteURL, localURL)
		services = append(services, service)
	}

	return services, nil
}

// Ensure DockerHelper implements Helper interface
var _ Helper = (*DockerHelper)(nil)
