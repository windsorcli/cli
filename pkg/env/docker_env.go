package env

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/windsorcli/cli/pkg/di"
)

// DockerEnvPrinter is a struct that simulates a Docker environment for testing purposes.
type DockerEnvPrinter struct {
	BaseEnvPrinter
}

// NewDockerEnvPrinter initializes a new dockerEnv instance using the provided dependency injector.
func NewDockerEnvPrinter(injector di.Injector) *DockerEnvPrinter {
	return &DockerEnvPrinter{
		BaseEnvPrinter: BaseEnvPrinter{
			injector: injector,
		},
	}
}

// GetEnvVars returns Docker-specific env vars, setting DOCKER_HOST based on vm.driver config.
// It uses the user's home directory for Docker paths, defaulting WINDSORCONFIG if unset.
// Ensures Docker config directory exists and writes config if content differs.
// Adds DOCKER_CONFIG and REGISTRY_URL to env vars and returns the map.
func (e *DockerEnvPrinter) GetEnvVars() (map[string]string, error) {
	envVars := make(map[string]string)

	vmDriver := e.configHandler.GetString("vm.driver")
	homeDir, err := osUserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("error retrieving user home directory: %w", err)
	}

	windsorConfigDir := os.Getenv("WINDSORCONFIG")
	if windsorConfigDir == "" {
		windsorConfigDir = filepath.Join(homeDir, ".config", "windsor")
	}
	dockerConfigDir := filepath.Join(windsorConfigDir, "docker")
	dockerConfigPath := filepath.Join(dockerConfigDir, "config.json")
	dockerConfigContent := `{
		"auths": {},
		"currentContext": "%s",
		"plugins": {},
		"features": {}
	}`

	switch vmDriver {
	case "colima":
		contextName := e.configHandler.GetContext()
		dockerHostPath := fmt.Sprintf("unix://%s/.colima/windsor-%s/docker.sock", homeDir, contextName)
		envVars["DOCKER_HOST"] = dockerHostPath
		dockerConfigContent = fmt.Sprintf(dockerConfigContent, fmt.Sprintf("colima-windsor-%s", contextName))

	case "docker-desktop":
		if goos() == "windows" {
			envVars["DOCKER_HOST"] = "npipe:////./pipe/docker_engine"
		} else {
			dockerHostPath := fmt.Sprintf("unix://%s/.docker/run/docker.sock", homeDir)
			envVars["DOCKER_HOST"] = dockerHostPath
		}
		dockerConfigContent = fmt.Sprintf(dockerConfigContent, "desktop-linux")

	case "docker":
		envVars["DOCKER_HOST"] = "unix:///var/run/docker.sock"
		dockerConfigContent = fmt.Sprintf(dockerConfigContent, "default")
	}

	if err := mkdirAll(dockerConfigDir, 0755); err != nil {
		return nil, fmt.Errorf("error creating docker config directory: %w", err)
	}

	existingContent, err := readFile(dockerConfigPath)
	if err != nil || string(existingContent) != dockerConfigContent {
		if err := writeFile(dockerConfigPath, []byte(dockerConfigContent), 0644); err != nil {
			return nil, fmt.Errorf("error writing docker config file: %w", err)
		}
	}
	envVars["DOCKER_CONFIG"] = dockerConfigDir

	registryURL, err := e.getRegistryURL()
	if registryURL != "" {
		envVars["REGISTRY_URL"] = registryURL
	}

	return envVars, nil
}

// GetAlias creates an alias for a command and returns it in a map. In
// this case, it looks for docker-cli-plugin-docker-compose and creates an
// alias for docker-compose.
func (e *DockerEnvPrinter) GetAlias() (map[string]string, error) {
	aliasMap := make(map[string]string)
	if _, err := execLookPath("docker-cli-plugin-docker-compose"); err == nil {
		aliasMap["docker-compose"] = "docker-cli-plugin-docker-compose"
	}
	return aliasMap, nil
}

// Print retrieves and prints the environment variables for the Docker environment.
func (e *DockerEnvPrinter) Print(customVars ...map[string]string) error {
	envVars, err := e.GetEnvVars()
	if err != nil {
		return fmt.Errorf("error getting environment variables: %w", err)
	}

	// If customVars are provided, merge them with envVars
	if len(customVars) > 0 {
		for key, value := range customVars[0] {
			envVars[key] = strings.TrimSpace(value)
		}
	}

	return e.BaseEnvPrinter.Print(envVars)
}

// getRegistryURL retrieves a registry URL, appending a port if not present.
// It retrieves the URL from the configuration and checks if it already includes a port.
// If not, it looks for a matching registry configuration to append the host port.
// Returns the constructed URL or an empty string if no URL is configured.
func (e *DockerEnvPrinter) getRegistryURL() (string, error) {
	config := e.configHandler.GetConfig()
	registryURL := e.configHandler.GetString("docker.registry_url")
	if registryURL == "" {
		return "", nil
	}
	if _, _, err := net.SplitHostPort(registryURL); err == nil {
		return registryURL, nil
	}
	if config.Docker != nil && config.Docker.Registries != nil {
		if registryConfig, exists := config.Docker.Registries[registryURL]; exists {
			if registryConfig.HostPort != 0 {
				registryURL = fmt.Sprintf("%s:%d", registryURL, registryConfig.HostPort)
			}
		}
	}
	return registryURL, nil
}

// Ensure dockerEnv implements the EnvPrinter interface
var _ EnvPrinter = (*DockerEnvPrinter)(nil)
