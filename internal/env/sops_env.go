package env

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/getsops/sops/v3/decrypt"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
)

var decryptFileFunc = decrypt.File

// SopsEnv is a struct that simulates a Kubernetes environment for testing purposes.
type SopsEnv struct {
	Env
}

// NewSopsEnv initializes a new SopsEnv instance using the provided dependency injection container.
func NewSopsEnv(diContainer di.ContainerInterface) *SopsEnv {
	return &SopsEnv{
		Env: Env{
			diContainer: diContainer,
		},
	}
}

// GetEnvVars retrieves the environment variables for the SOPS environment.
func (e *SopsEnv) GetEnvVars() (map[string]string, error) {
	envVars := make(map[string]string)

	// Resolve necessary dependencies for context operations.
	contextHandler, err := e.diContainer.Resolve("contextHandler")
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

	// Construct the path to the sops encrypted secrets file, return if it doesn't exist
	sopsEncSecretsPath := filepath.Join(configRoot, "secrets.enc.yaml")
	if _, err := stat(sopsEncSecretsPath); os.IsNotExist(err) {
		return nil, nil
	}

	// Decrypt the file using SOPS
	plaintextBytes, err := decryptFile(sopsEncSecretsPath)
	if err != nil {
		return nil, fmt.Errorf("error decrypting sops file: %w", err)
	}

	// Convert the decrypted YAML content into environment variables
	envVarsFromYaml, err := yamlToEnvVars(plaintextBytes)
	if err != nil {
		return nil, fmt.Errorf("error converting YAML to environment variables: %w", err)
	}

	// Merge the decrypted environment variables into the envVars map
	for key, value := range envVarsFromYaml {
		envVars[key] = value
	}

	return envVars, nil
}

// Print prints the environment variables for the SOPS environment.
func (e *SopsEnv) Print() error {
	envVars, err := e.GetEnvVars()
	if err != nil {
		// Return the error if GetEnvVars fails
		return fmt.Errorf("error getting environment variables: %w", err)
	}
	// Call the Print method of the embedded Env struct with the retrieved environment variables
	return e.Env.Print(envVars)
}

// Ensure SopsEnv implements the EnvPrinter interface
var _ EnvPrinter = (*SopsEnv)(nil)

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
	if err := yamlUnmarshal(yamlData, &sopsSecrets); err != nil {
		return nil, err
	}

	// Populate envVars from the decrypted secrets file
	envVars := make(map[string]string)
	for key, value := range sopsSecrets {
		envVars[key] = value.(string)
	}

	return envVars, nil
}
