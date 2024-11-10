package env

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/windsor-hotel/cli/internal/di"
)

// KubeEnvPrinter is a struct that simulates a Kubernetes environment for testing purposes.
type KubeEnvPrinter struct {
	BaseEnvPrinter
}

// NewKubeEnv initializes a new kubeEnv instance using the provided dependency injector.
func NewKubeEnvPrinter(injector di.Injector) *KubeEnvPrinter {
	return &KubeEnvPrinter{
		BaseEnvPrinter: BaseEnvPrinter{
			injector: injector,
		},
	}
}

// GetEnvVars retrieves the environment variables for the Kubernetes environment.
func (e *KubeEnvPrinter) GetEnvVars() (map[string]string, error) {
	envVars := make(map[string]string)

	// Determine the root directory for configuration files.
	configRoot, err := e.contextHandler.GetConfigRoot()
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
func (e *KubeEnvPrinter) Print() error {
	envVars, err := e.GetEnvVars()
	if err != nil {
		// Return the error if GetEnvVars fails
		return fmt.Errorf("error getting environment variables: %w", err)
	}
	// Call the Print method of the embedded BaseEnvPrinter struct with the retrieved environment variables
	return e.BaseEnvPrinter.Print(envVars)
}

// Ensure kubeEnv implements the EnvPrinter interface
var _ EnvPrinter = (*KubeEnvPrinter)(nil)
