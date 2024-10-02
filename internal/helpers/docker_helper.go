package helpers

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/shell"
)

// DockerHelper is a helper struct that provides Docker-specific utility functions
type DockerHelper struct {
	ConfigHandler config.ConfigHandler
	Shell         shell.Shell
	Context       context.ContextInterface
}

// NewDockerHelper is a constructor for DockerHelper
func NewDockerHelper(configHandler config.ConfigHandler, shell shell.Shell, ctx context.ContextInterface) *DockerHelper {
	return &DockerHelper{
		ConfigHandler: configHandler,
		Shell:         shell,
		Context:       ctx,
	}
}

// GetEnvVars retrieves Docker-specific environment variables for the current context
func (h *DockerHelper) GetEnvVars() (map[string]string, error) {
	// Get the configuration root directory
	configRoot, err := h.Context.GetConfigRoot()
	if err != nil {
		return nil, fmt.Errorf("error retrieving config root: %w", err)
	}

	// Check for the existence of docker-compose.yaml or docker-compose.yml
	var composeFilePath string
	yamlPath := filepath.Join(configRoot, "docker-compose.yaml")
	ymlPath := filepath.Join(configRoot, "docker-compose.yml")

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
	// This is a stub implementation
	return nil
}

// Ensure DockerHelper implements Helper interface
var _ Helper = (*DockerHelper)(nil)
