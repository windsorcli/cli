package env

import (
	"fmt"
	"path/filepath"

	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
)

// DockerEnv is a struct that simulates a Docker environment for testing purposes.
type DockerEnv struct {
	Env
}

// NewDockerEnv initializes a new DockerEnv instance using the provided dependency injector.
func NewDockerEnv(injector di.Injector) *DockerEnv {
	return &DockerEnv{
		Env: Env{
			Injector: injector,
		},
	}
}

// GetEnvVars retrieves the environment variables for the Docker environment.
func (e *DockerEnv) GetEnvVars() (map[string]string, error) {
	envVars := make(map[string]string)

	// Resolve necessary dependencies for context operations.
	contextHandler, err := e.Injector.Resolve("contextHandler")
	if err != nil {
		return nil, fmt.Errorf("error resolving contextHandler: %w", err)
	}
	context, ok := contextHandler.(context.ContextInterface)
	if !ok {
		return nil, fmt.Errorf("failed to cast contextHandler to context.ContextInterface")
	}

	// Determine the root directory for configuration files.
	configRoot, err := context.GetConfigRoot()
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

	return envVars, nil
}

// Print prints the environment variables for the Docker environment.
func (e *DockerEnv) Print() error {
	envVars, err := e.GetEnvVars()
	if err != nil {
		// Return the error if GetEnvVars fails
		return fmt.Errorf("error getting environment variables: %w", err)
	}
	// Call the Print method of the embedded Env struct with the retrieved environment variables
	return e.Env.Print(envVars)
}

// Ensure DockerEnv implements the EnvInterface
var _ EnvPrinter = (*DockerEnv)(nil)
