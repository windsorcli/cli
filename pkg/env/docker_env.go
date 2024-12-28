package env

import (
	"fmt"
	"path/filepath"

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

// GetEnvVars retrieves the environment variables for the Docker environment.
func (e *DockerEnvPrinter) GetEnvVars() (map[string]string, error) {
	envVars := make(map[string]string)

	// Determine the root directory for configuration files.
	configRoot, err := e.contextHandler.GetConfigRoot()
	if err != nil {
		return nil, fmt.Errorf("error retrieving configuration root directory: %w", err)
	}

	// Check for the existence of compose.yaml or compose.yml
	var composeFilePath string
	yamlPath := filepath.Join(configRoot, "compose.yaml")
	ymlPath := filepath.Join(configRoot, "compose.yml")

	if _, err := stat(yamlPath); err == nil {
		composeFilePath = yamlPath
	} else if _, err := stat(ymlPath); err == nil {
		composeFilePath = ymlPath
	}

	// Populate environment variables with Docker configuration data.
	envVars["COMPOSE_FILE"] = composeFilePath

	// Set DOCKER_HOST if vm.driver is colima
	if e.configHandler.GetString("vm.driver") == "colima" {
		homeDir, err := osUserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("error retrieving user home directory: %w", err)
		}
		contextName := e.contextHandler.GetContext()
		dockerHostPath := fmt.Sprintf("unix://%s/.colima/windsor-%s/docker.sock", homeDir, contextName)
		envVars["DOCKER_HOST"] = dockerHostPath
	}

	return envVars, nil
}

// Print prints the environment variables for the Docker environment.
func (e *DockerEnvPrinter) Print() error {
	envVars, err := e.GetEnvVars()
	if err != nil {
		// Return the error if GetEnvVars fails
		return fmt.Errorf("error getting environment variables: %w", err)
	}
	// Call the Print method of the embedded envPrinter struct with the retrieved environment variables
	return e.BaseEnvPrinter.Print(envVars)
}

// Ensure dockerEnv implements the EnvPrinter interface
var _ EnvPrinter = (*DockerEnvPrinter)(nil)
