package env

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/shell"
)

// OmniEnv is a struct that simulates a Kubernetes environment for testing purposes.
type OmniEnv struct {
	EnvInterface
	diContainer di.ContainerInterface
}

// NewTalosEnv initializes a new TalosEnv instance using the provided dependency injection container.
func NewOmniEnv(diContainer di.ContainerInterface) *OmniEnv {
	return &OmniEnv{
		diContainer: diContainer,
	}
}

// Print displays the provided environment variables to the console.
func (e *OmniEnv) Print(envVars map[string]string) {
	// Resolve necessary dependencies for context and shell operations.
	contextHandler, err := e.diContainer.Resolve("contextHandler")
	if err != nil {
		fmt.Printf("Error resolving contextHandler: %v\n", err)
		return
	}
	context, ok := contextHandler.(context.ContextInterface)
	if !ok {
		fmt.Println("Failed to cast contextHandler to context.ContextInterface")
		return
	}

	shellInstance, err := e.diContainer.Resolve("shell")
	if err != nil {
		fmt.Printf("Error resolving shell: %v\n", err)
		return
	}
	shell, ok := shellInstance.(shell.Shell)
	if !ok {
		fmt.Println("Failed to cast shell to shell.Shell")
		return
	}

	// Determine the root directory for configuration files.
	configRoot, err := context.GetConfigRoot()
	if err != nil {
		fmt.Printf("Error retrieving configuration root directory: %v\n", err)
		return
	}

	// Construct the path to the kubeconfig file and verify its existence.
	omniConfigPath := filepath.Join(configRoot, ".omni", "config")
	if _, err := stat(omniConfigPath); os.IsNotExist(err) {
		omniConfigPath = ""
	}

	// Populate environment variables with Kubernetes configuration data.
	envVars["OMNICONFIG"] = omniConfigPath

	// Display the environment variables using the Shell's PrintEnvVars method.
	shell.PrintEnvVars(envVars)
}

// Ensure OmniEnv implements the EnvInterface
var _ EnvInterface = (*OmniEnv)(nil)
