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

// GetEnvVars sets environment variables for virtual machine and container runtimes. When
// workstation.runtime is configured, it writes a Docker config file and sets DOCKER_CONFIG.
// DOCKER_HOST is set from the runtime when it does not yet exist or was previously managed
// by Windsor (present in WINDSOR_MANAGED_ENV); if DOCKER_HOST was set by the user it is left
// untouched. Handles "colima", "docker-desktop", and "docker" workstation runtimes. Also sets
// REGISTRY_URL from docker registry config, and INCUS_SOCKET for colima-incus setups.
func (e *VirtEnvPrinter) GetEnvVars() (map[string]string, error) {
	envVars := make(map[string]string)

	workstationRuntime := e.configHandler.GetString("workstation.runtime")
	platform := e.configHandler.GetString("platform")

	isDockerHostManaged := false
	if managedEnvStr := e.shims.Getenv("WINDSOR_MANAGED_ENV"); managedEnvStr != "" {
		for _, key := range strings.Split(managedEnvStr, ",") {
			if strings.TrimSpace(key) == "DOCKER_HOST" {
				isDockerHostManaged = true
				break
			}
		}
	}

	_, dockerHostExists := e.shims.LookupEnv("DOCKER_HOST")
	shouldSetDockerHost := !dockerHostExists || isDockerHostManaged

	if platform != "incus" && workstationRuntime != "" {
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

		configContext := e.configHandler.GetContext()

		var contextName string
		if e.shims.Goos() == "windows" {
			contextName = "desktop-linux"
		} else {
			switch workstationRuntime {
			case "colima":
				contextName = fmt.Sprintf("colima-windsor-%s", configContext)
			case "docker-desktop":
				contextName = "desktop-linux"
			default:
				contextName = "default"
			}
		}

		if shouldSetDockerHost {
			if e.shims.Goos() == "windows" {
				envVars["DOCKER_HOST"] = "npipe:////./pipe/docker_engine"
				e.SetManagedEnv("DOCKER_HOST")
			} else {
				switch workstationRuntime {
				case "colima":
					envVars["DOCKER_HOST"] = fmt.Sprintf("unix://%s/.colima/windsor-%s/docker.sock", homeDir, configContext)
					e.SetManagedEnv("DOCKER_HOST")
				case "docker-desktop":
					envVars["DOCKER_HOST"] = fmt.Sprintf("unix://%s/.docker/run/docker.sock", homeDir)
					e.SetManagedEnv("DOCKER_HOST")
				case "docker":
					envVars["DOCKER_HOST"] = "unix:///var/run/docker.sock"
					e.SetManagedEnv("DOCKER_HOST")
				}
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
	}

	registryURL, _ := e.getRegistryURL()
	if registryURL != "" {
		envVars["REGISTRY_URL"] = registryURL
		e.SetManagedEnv("REGISTRY_URL")
	}

	if workstationRuntime == "colima" && platform == "incus" {
		homeDir, err := e.shims.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("error retrieving user home directory: %w", err)
		}
		configContext := e.configHandler.GetContext()
		incusSocketPath := filepath.Join(homeDir, ".colima", fmt.Sprintf("windsor-%s", configContext), "incus.sock")
		envVars["INCUS_SOCKET"] = filepath.ToSlash(incusSocketPath)
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
