// The DockerEnvPrinter is a specialized component that manages Docker environment configuration.
// It provides Docker-specific environment variable management and configuration,
// The DockerEnvPrinter handles Docker host, context, and registry configuration settings,
// ensuring proper Docker CLI integration and environment setup for container operations.

package env

import (
	"fmt"
	"net"
	"os"
	"path/filepath"

	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// =============================================================================
// Types
// =============================================================================

// DockerEnvPrinter is a struct that implements Docker environment configuration
type DockerEnvPrinter struct {
	BaseEnvPrinter
}

// =============================================================================
// Constructor
// =============================================================================

// NewDockerEnvPrinter creates a new DockerEnvPrinter instance
func NewDockerEnvPrinter(shell shell.Shell, configHandler config.ConfigHandler) *DockerEnvPrinter {
	return &DockerEnvPrinter{
		BaseEnvPrinter: *NewBaseEnvPrinter(shell, configHandler),
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// GetEnvVars sets Docker-specific env vars, using DOCKER_HOST from vm.driver config or existing env.
// Defaults to WINDSORCONFIG or home dir for Docker paths, ensuring config directory exists.
// Writes config if content changes, adds DOCKER_CONFIG and REGISTRY_URL, and returns the map.
// Handles "colima", "docker-desktop", and "docker" vm.driver settings, defaulting to "default" if unrecognized.
func (e *DockerEnvPrinter) GetEnvVars() (map[string]string, error) {
	envVars := make(map[string]string)

	if dockerHostValue, dockerHostExists := e.shims.LookupEnv("DOCKER_HOST"); dockerHostExists {
		envVars["DOCKER_HOST"] = dockerHostValue
	} else {
		vmDriver := e.configHandler.GetString("vm.driver")
		homeDir, err := e.shims.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("error retrieving user home directory: %w", err)
		}

		windsorConfigDir := os.Getenv("WINDSORCONFIG")
		if windsorConfigDir == "" {
			windsorConfigDir = filepath.Join(homeDir, ".config", "windsor")
		}
		dockerConfigDir := filepath.Join(windsorConfigDir, "docker")
		dockerConfigPath := filepath.Join(dockerConfigDir, "config.json")

		// Determine the Docker context name based on the VM driver
		var contextName string
		configContext := e.configHandler.GetContext()

		if e.shims.Goos() == "windows" {
			contextName = "desktop-linux"
			envVars["DOCKER_HOST"] = "npipe:////./pipe/docker_engine"
		} else {
			switch vmDriver {
			case "colima":
				contextName = fmt.Sprintf("colima-windsor-%s", configContext)
				dockerHostPath := fmt.Sprintf("unix://%s/.colima/windsor-%s/docker.sock", homeDir, configContext)
				envVars["DOCKER_HOST"] = dockerHostPath

			case "docker-desktop":
				contextName = "desktop-linux"
				dockerHostPath := fmt.Sprintf("unix://%s/.docker/run/docker.sock", homeDir)
				envVars["DOCKER_HOST"] = dockerHostPath

			case "docker":
				contextName = "default"
				envVars["DOCKER_HOST"] = "unix:///var/run/docker.sock"

			default:
				contextName = "default"
			}
		}

		// Create Docker config content with the determined context name
		dockerConfigContent := fmt.Sprintf(`{
			"auths": {},
			"currentContext": "%s",
			"plugins": {},
			"features": {}
		}`, contextName)

		if err := e.shims.MkdirAll(dockerConfigDir, 0755); err != nil {
			return nil, fmt.Errorf("error creating docker config directory: %w", err)
		}

		existingContent, err := e.shims.ReadFile(dockerConfigPath)
		if err != nil || string(existingContent) != dockerConfigContent {
			if err := e.shims.WriteFile(dockerConfigPath, []byte(dockerConfigContent), 0644); err != nil {
				return nil, fmt.Errorf("error writing docker config file: %w", err)
			}
		}
		envVars["DOCKER_CONFIG"] = filepath.ToSlash(dockerConfigDir)
	}

	registryURL, _ := e.getRegistryURL()
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
	if _, err := e.shims.LookPath("docker-cli-plugin-docker-compose"); err == nil {
		aliasMap["docker-compose"] = "docker-cli-plugin-docker-compose"
	}
	return aliasMap, nil
}

// =============================================================================
// Private Methods
// =============================================================================

// getRegistryURL returns the configured Docker registry URL with port.
// Priority:
//  1. docker.registry_url setting (with port from registry config if needed)
//  2. First non-mirror registry from docker.registries
//
// Returns empty string if no registry is configured.
func (e *DockerEnvPrinter) getRegistryURL() (string, error) {
	registryURL := e.configHandler.GetString("docker.registry_url")
	if registryURL != "" {
		if _, _, err := net.SplitHostPort(registryURL); err == nil {
			return registryURL, nil
		}
		config := e.configHandler.GetConfig()
		if config.Docker != nil && config.Docker.Registries != nil {
			if registryConfig, exists := config.Docker.Registries[registryURL]; exists {
				if registryConfig.HostPort != 0 {
					return fmt.Sprintf("%s:%d", registryURL, registryConfig.HostPort), nil
				}
			}
		}
		return registryURL, nil
	}

	config := e.configHandler.GetConfig()
	if config.Docker != nil && config.Docker.Registries != nil {
		for url, registryConfig := range config.Docker.Registries {
			if registryConfig.Remote == "" {
				if registryConfig.HostPort != 0 {
					return fmt.Sprintf("%s:%d", url, registryConfig.HostPort), nil
				}
				return fmt.Sprintf("%s:5000", url), nil
			}
		}
	}

	return "", nil
}

// Ensure DockerEnvPrinter implements the EnvPrinter interface
var _ EnvPrinter = (*DockerEnvPrinter)(nil)
