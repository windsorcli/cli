// The VirtEnvPrinter is a specialized component that manages virtual machine and container runtime
// environment configuration. It provides environment variable management for Docker and Incus runtimes,
// handling Docker host, context, registry configuration, and Incus socket paths. The VirtEnvPrinter
// ensures proper CLI integration and environment setup for container operations across different
// virtualization backends.
package env

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// =============================================================================
// Types
// =============================================================================

// VirtEnvPrinter is a struct that implements virtual machine and container runtime environment configuration
type VirtEnvPrinter struct {
	BaseEnvPrinter
}

// =============================================================================
// Constructor
// =============================================================================

// NewVirtEnvPrinter creates a new VirtEnvPrinter instance
func NewVirtEnvPrinter(shell shell.Shell, configHandler config.ConfigHandler) *VirtEnvPrinter {
	if shell == nil {
		panic("shell is required")
	}
	if configHandler == nil {
		panic("config handler is required")
	}

	return &VirtEnvPrinter{
		BaseEnvPrinter: *NewBaseEnvPrinter(shell, configHandler),
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// GetEnvVars sets environment variables for virtual machine and container runtimes, using DOCKER_HOST
// from vm.driver config or existing env, and INCUS_SOCKET for colima-incus configurations. Defaults
// to WINDSORCONFIG or home dir for Docker paths, ensuring config directory exists. Writes config if
// content changes, adds DOCKER_CONFIG, REGISTRY_URL, and INCUS_SOCKET as appropriate, and returns the map.
// Handles "colima", "docker-desktop", and "docker" vm.driver settings, defaulting to "default" if unrecognized.
func (e *VirtEnvPrinter) GetEnvVars() (map[string]string, error) {
	envVars := make(map[string]string)

	vmDriver := e.configHandler.GetString("vm.driver")
	vmRuntime := e.configHandler.GetString("vm.runtime", "docker")
	_, dockerHostExists := e.shims.LookupEnv("DOCKER_HOST")
	_, managedEnvExists := e.shims.LookupEnv("WINDSOR_MANAGED_ENV")

	isDockerHostManaged := false
	if managedEnvExists {
		managedEnvStr := e.shims.Getenv("WINDSOR_MANAGED_ENV")
		if strings.Contains(managedEnvStr, "DOCKER_HOST") {
			isDockerHostManaged = true
		}
	}

	if vmRuntime != "incus" && vmDriver != "" && (!dockerHostExists || isDockerHostManaged) {
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

		var contextName string
		configContext := e.configHandler.GetContext()

		if e.shims.Goos() == "windows" {
			contextName = "desktop-linux"
			envVars["DOCKER_HOST"] = "npipe:////./pipe/docker_engine"
			e.SetManagedEnv("DOCKER_HOST")
		} else {
			switch vmDriver {
			case "colima":
				contextName = fmt.Sprintf("colima-windsor-%s", configContext)
				dockerHostPath := fmt.Sprintf("unix://%s/.colima/windsor-%s/docker.sock", homeDir, configContext)
				envVars["DOCKER_HOST"] = dockerHostPath
				e.SetManagedEnv("DOCKER_HOST")

			case "docker-desktop":
				contextName = "desktop-linux"
				dockerHostPath := fmt.Sprintf("unix://%s/.docker/run/docker.sock", homeDir)
				envVars["DOCKER_HOST"] = dockerHostPath
				e.SetManagedEnv("DOCKER_HOST")

			case "docker":
				contextName = "default"
				envVars["DOCKER_HOST"] = "unix:///var/run/docker.sock"
				e.SetManagedEnv("DOCKER_HOST")

			default:
				contextName = "default"
			}
		}

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
		e.SetManagedEnv("DOCKER_CONFIG")
	} else if vmRuntime != "incus" && dockerHostExists {
		if dockerHostValue, _ := e.shims.LookupEnv("DOCKER_HOST"); dockerHostValue != "" {
			envVars["DOCKER_HOST"] = dockerHostValue
		}
	}

	registryURL, _ := e.getRegistryURL()
	if registryURL != "" {
		envVars["REGISTRY_URL"] = registryURL
		e.SetManagedEnv("REGISTRY_URL")
	}

	if vmDriver == "colima" && vmRuntime == "incus" {
		homeDir, err := e.shims.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("error retrieving user home directory: %w", err)
		}
		configContext := e.configHandler.GetContext()
		incusSocketPath := filepath.Join(homeDir, ".colima", fmt.Sprintf("windsor-%s", configContext), "incus.sock")
		envVars["INCUS_SOCKET"] = incusSocketPath
		e.SetManagedEnv("INCUS_SOCKET")
	}

	return envVars, nil
}

// GetAlias creates an alias for a command and returns it in a map. In
// this case, it looks for docker-cli-plugin-docker-compose and creates an
// alias for docker-compose.
func (e *VirtEnvPrinter) GetAlias() (map[string]string, error) {
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
func (e *VirtEnvPrinter) getRegistryURL() (string, error) {
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

// =============================================================================
// Interface Compliance
// =============================================================================

// Ensure VirtEnvPrinter implements the EnvPrinter interface
var _ EnvPrinter = (*VirtEnvPrinter)(nil)
