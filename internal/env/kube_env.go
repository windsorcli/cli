package env

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/shell"
)

// KubeEnv is a struct that simulates a Kubernetes environment for testing purposes.
type KubeEnv struct {
	EnvInterface
	diContainer di.ContainerInterface
}

// NewKubeEnv initializes a new KubeEnv instance using the provided dependency injection container.
func NewKubeEnv(diContainer di.ContainerInterface) *KubeEnv {
	return &KubeEnv{
		diContainer: diContainer,
	}
}

// Print displays the provided environment variables to the console.
func (k *KubeEnv) Print(envVars map[string]string) error {
	// Resolve necessary dependencies for context and shell operations.
	contextHandler, err := k.diContainer.Resolve("contextHandler")
	if err != nil {
		return fmt.Errorf("error resolving contextHandler: %w", err)
	}
	context, ok := contextHandler.(context.ContextInterface)
	if !ok {
		return fmt.Errorf("failed to cast contextHandler to context.ContextInterface")
	}

	shellInstance, err := k.diContainer.Resolve("shell")
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
	kubeConfigPath := filepath.Join(configRoot, ".kube", "config")
	if _, err := stat(kubeConfigPath); os.IsNotExist(err) {
		kubeConfigPath = ""
	}

	// Populate environment variables with Kubernetes configuration data.
	envVars["KUBECONFIG"] = kubeConfigPath
	envVars["KUBE_CONFIG_PATH"] = kubeConfigPath

	// Display the environment variables using the Shell's PrintEnvVars method.
	return shell.PrintEnvVars(envVars)
}

// Ensure KubeEnv implements the EnvInterface
var _ EnvInterface = (*KubeEnv)(nil)
