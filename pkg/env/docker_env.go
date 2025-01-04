package env

import (
	"fmt"
	"os"
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

	// Determine the appropriate DOCKER_HOST based on the vm.driver setting
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
		// Handle the "colima" case
		contextName := e.contextHandler.GetContext()
		dockerHostPath := fmt.Sprintf("unix://%s/.colima/windsor-%s/docker.sock", homeDir, contextName)
		envVars["DOCKER_HOST"] = dockerHostPath
		dockerConfigContent = fmt.Sprintf(dockerConfigContent, fmt.Sprintf("colima-windsor-%s", contextName))

	case "docker-desktop":
		// Handle the "docker-desktop" case
		if goos() == "windows" {
			envVars["DOCKER_HOST"] = "npipe:////./pipe/docker_engine"
		} else {
			dockerHostPath := fmt.Sprintf("unix://%s/.docker/run/docker.sock", homeDir)
			envVars["DOCKER_HOST"] = dockerHostPath
		}
		dockerConfigContent = fmt.Sprintf(dockerConfigContent, "desktop-linux")
	}

	// Ensure the directory exists
	if err := mkdirAll(dockerConfigDir, 0755); err != nil {
		return nil, fmt.Errorf("error creating docker config directory: %w", err)
	}

	// Write the docker config file idempotently
	existingContent, err := readFile(dockerConfigPath)
	if err != nil || string(existingContent) != dockerConfigContent {
		if err := writeFile(dockerConfigPath, []byte(dockerConfigContent), 0644); err != nil {
			return nil, fmt.Errorf("error writing docker config file: %w", err)
		}
	}
	envVars["DOCKER_CONFIG"] = dockerConfigDir

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
