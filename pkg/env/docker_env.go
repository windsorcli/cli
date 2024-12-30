package env

import (
	"fmt"

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
