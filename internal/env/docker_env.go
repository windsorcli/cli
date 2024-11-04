package env

import (
	"fmt"
	"path/filepath"

	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/shell"
)

// DockerEnv is a struct that simulates a Docker environment for testing purposes.
type DockerEnv struct {
	EnvInterface
	diContainer di.ContainerInterface
}

// NewTalosEnv initializes a new TalosEnv instance using the provided dependency injection container.
func NewDockerEnv(diContainer di.ContainerInterface) *DockerEnv {
	return &DockerEnv{
		diContainer: diContainer,
	}
}

// Print displays the provided environment variables to the console.
func (e *DockerEnv) Print(envVars map[string]string) error {
	// Resolve necessary dependencies for context and shell operations.
	contextHandler, err := e.diContainer.Resolve("contextHandler")
	if err != nil {
		return fmt.Errorf("error resolving contextHandler: %w", err)
	}
	context, ok := contextHandler.(context.ContextInterface)
	if !ok {
		return fmt.Errorf("failed to cast contextHandler to context.ContextInterface")
	}

	shellInstance, err := e.diContainer.Resolve("shell")
	if err != nil {
		return fmt.Errorf("error resolving shell: %w", err)
	}
	shell, ok := shellInstance.(shell.Shell)
	if !ok {
		return fmt.Errorf("failed to cast shell to shell.Shell")
	}

	// Determine the root directory for configuration files.
	configRoot, err := context.GetConfigRoot()
	if err != nil {
		return fmt.Errorf("error retrieving configuration root directory: %w", err)
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

	// Display the environment variables using the Shell's PrintEnvVars method.
	return shell.PrintEnvVars(envVars)
}

// Ensure DockerEnv implements the EnvInterface
var _ EnvInterface = (*DockerEnv)(nil)
