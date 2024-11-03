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
func (k *KubeEnv) Print(envVars map[string]string) {
	// Resolve necessary dependencies for context and shell operations.
	contextHandler, err := k.diContainer.Resolve("contextHandler")
	if err != nil {
		fmt.Printf("Error resolving contextHandler: %v\n", err)
		return
	}
	context, ok := contextHandler.(context.ContextInterface)
	if !ok {
		fmt.Println("Failed to cast contextHandler to context.ContextInterface")
		return
	}

	shellInstance, err := k.diContainer.Resolve("shell")
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
	kubeConfigPath := filepath.Join(configRoot, ".kube", "config")
	if _, err := stat(kubeConfigPath); os.IsNotExist(err) {
		kubeConfigPath = ""
	}

	// Populate environment variables with Kubernetes configuration data.
	envVars["KUBECONFIG"] = kubeConfigPath
	envVars["KUBE_CONFIG_PATH"] = kubeConfigPath

	// Display the environment variables using the Shell's PrintEnvVars method.
	shell.PrintEnvVars(envVars)
}

// Ensure KubeEnv implements the EnvInterface
var _ EnvInterface = (*KubeEnv)(nil)
