package env

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
)

// KubeEnv is a struct that simulates a Kubernetes environment for testing purposes.
type KubeEnv struct {
	Env
}

// NewKubeEnv initializes a new KubeEnv instance using the provided dependency injection container.
func NewKubeEnv(diContainer di.ContainerInterface) *KubeEnv {
	return &KubeEnv{
		Env: Env{
			diContainer: diContainer,
		},
	}
}

// GetEnvVars retrieves the environment variables for the Kubernetes environment.
func (k *KubeEnv) GetEnvVars() (map[string]string, error) {
	envVars := make(map[string]string)

	// Resolve necessary dependencies for context and shell operations.
	contextHandler, err := k.diContainer.Resolve("contextHandler")
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

	// Construct the path to the kubeconfig file and verify its existence.
	kubeConfigPath := filepath.Join(configRoot, ".kube", "config")
	if _, err := stat(kubeConfigPath); os.IsNotExist(err) {
		kubeConfigPath = ""
	}

	// Populate environment variables with Kubernetes configuration data.
	envVars["KUBECONFIG"] = kubeConfigPath
	envVars["KUBE_CONFIG_PATH"] = kubeConfigPath

	return envVars, nil
}

// Print prints the environment variables for the Kube environment.
func (e *KubeEnv) Print() error {
	envVars, err := e.GetEnvVars()
	if err != nil {
		// Return the error if GetEnvVars fails
		return fmt.Errorf("error getting environment variables: %w", err)
	}
	// Call the Print method of the embedded Env struct with the retrieved environment variables
	return e.Env.Print(envVars)
}

// Ensure KubeEnv implements the EnvPrinter interface
var _ EnvPrinter = (*KubeEnv)(nil)
