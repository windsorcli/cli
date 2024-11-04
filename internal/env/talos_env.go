package env

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/shell"
)

// TalosEnv is a struct that simulates a Kubernetes environment for testing purposes.
type TalosEnv struct {
	EnvInterface
	diContainer di.ContainerInterface
}

// NewTalosEnv initializes a new TalosEnv instance using the provided dependency injection container.
func NewTalosEnv(diContainer di.ContainerInterface) *TalosEnv {
	return &TalosEnv{
		diContainer: diContainer,
	}
}

// Print displays the provided environment variables to the console.
func (e *TalosEnv) Print(envVars map[string]string) error {
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

	// Construct the path to the kubeconfig file and verify its existence.
	talosConfigPath := filepath.Join(configRoot, ".talos", "config")
	if _, err := stat(talosConfigPath); os.IsNotExist(err) {
		talosConfigPath = ""
	}

	// Populate environment variables with Kubernetes configuration data.
	envVars["TALOSCONFIG"] = talosConfigPath

	// Display the environment variables using the Shell's PrintEnvVars method.
	return shell.PrintEnvVars(envVars)
}

// Ensure TalosEnv implements the EnvInterface
var _ EnvInterface = (*TalosEnv)(nil)
