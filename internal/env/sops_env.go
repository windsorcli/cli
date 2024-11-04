package env

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/getsops/sops/v3/decrypt"
	"github.com/goccy/go-yaml"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/shell"
)

var decryptFileFunc = decrypt.File

// SopsEnv is a struct that simulates a Kubernetes environment for testing purposes.
type SopsEnv struct {
	EnvInterface
	diContainer di.ContainerInterface
}

// NewSopsEnv initializes a new SopsEnv instance using the provided dependency injection container.
func NewSopsEnv(diContainer di.ContainerInterface) *SopsEnv {
	return &SopsEnv{
		diContainer: diContainer,
	}
}

// Print displays the provided environment variables to the console.
func (e *SopsEnv) Print(envVars map[string]string) error {
	// Resolve necessary dependencies for context and shell operations.
	contextHandler, err := e.diContainer.Resolve("contextHandler")
	if err != nil {
		return fmt.Errorf("error resolving contextHandler: %w", err)
	}
	context, ok := contextHandler.(context.ContextInterface)
	if !ok {
		return fmt.Errorf("failed to cast contextHandler to context.ContextInterface")
	}

	// Resolve the shell instance
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

	// Construct the path to the sops encrypted secrets file, return nils if it doesn't exist
	sopsEncSecretsPath := filepath.Join(configRoot, "secrets.enc.yaml")
	if _, err := stat(sopsEncSecretsPath); os.IsNotExist(err) {
		return fmt.Errorf("SOPS encrypted secrets file does not exist")
	}

	// Decrypt the file using SOPS
	plaintextBytes, err := decryptFile(sopsEncSecretsPath)
	if err != nil {
		return fmt.Errorf("error decrypting sops file: %w", err)
	}

	// Convert the decrypted YAML content into environment variables
	envVarsFromYaml, err := yamlToEnvVars(plaintextBytes)
	if err != nil {
		return fmt.Errorf("error converting YAML to environment variables: %w", err)
	}

	// Merge the decrypted environment variables into the provided envVars map
	for key, value := range envVarsFromYaml {
		envVars[key] = value
	}

	// Display the environment variables using the Shell's PrintEnvVars method.
	return shell.PrintEnvVars(envVars)
}

// Ensure SopsEnv implements the EnvInterface
var _ EnvInterface = (*SopsEnv)(nil)

// decryptFile decrypts a file using the SOPS package
func decryptFile(filePath string) ([]byte, error) {
	// Check if the file exists
	if _, err := stat(filePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("file does not exist: %s", filePath)
	}

	// Decrypt the file using SOPS
	plaintextBytes, err := decryptFileFunc(filePath, "yaml")
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt file: %w", err)
	}

	return plaintextBytes, nil
}

// yamlToEnvVars retrieves Kubernetes-specific environment variables for the current context
func yamlToEnvVars(yamlData []byte) (map[string]string, error) {
	// Parse the decrypted YAML content into a map
	var sopsSecrets map[string]interface{}
	if err := yaml.Unmarshal(yamlData, &sopsSecrets); err != nil {
		return nil, err
	}

	// Populate envVars from the decrypted secrets file
	envVars := make(map[string]string)
	for key, value := range sopsSecrets {
		envVars[key] = value.(string)
	}

	return envVars, nil
}
