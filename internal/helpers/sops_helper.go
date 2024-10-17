package helpers

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/compose-spec/compose-go/types"
	"github.com/getsops/sops/v3/decrypt"
	"github.com/goccy/go-yaml"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
)

// SopsHelper is a helper struct that provides Kubernetes-specific utility functions
type SopsHelper struct {
	Context context.ContextInterface
}

// NewSopsHelper is a constructor for SopsHelper
func NewSopsHelper(di *di.DIContainer) (*SopsHelper, error) {
	resolvedContext, err := di.Resolve("context")
	if err != nil {
		return nil, fmt.Errorf("error resolving context: %w", err)
	}

	return &SopsHelper{
		Context: resolvedContext.(context.ContextInterface),
	}, nil
}

// GetEnvVars retrieves Kubernetes-specific environment variables for the current context
func (h *SopsHelper) GetEnvVars() (map[string]string, error) {
	// Get the configuration root directory
	configRoot, err := h.Context.GetConfigRoot()
	if err != nil {
		return nil, fmt.Errorf("error retrieving config root: %w", err)
	}

	// Construct the path to the sops encrypted secrets file, return nils if it doesn't exist
	sopsEncSecretsPath := filepath.Join(configRoot, "secrets.enc.yaml")
	if _, err := os.Stat(sopsEncSecretsPath); os.IsNotExist(err) {
		return nil, nil
	}

	plaintextBytes, err := DecryptFile(sopsEncSecretsPath)
	if err != nil {
		return nil, fmt.Errorf("error decrypting sops file: %w", err)
	}

	envVars, err := YamlToEnvVars(plaintextBytes)

	return envVars, nil
}

// PostEnvExec runs any necessary commands after the environment variables have been set.
func (h *SopsHelper) PostEnvExec() error {
	return nil
}

// SetConfig sets the configuration value for the given key
func (h *SopsHelper) SetConfig(key, value string) error {
	// This is a stub implementation
	return nil
}

// GetContainerConfig returns a list of container data for docker-compose.
func (h *SopsHelper) GetContainerConfig() ([]types.ServiceConfig, error) {
	// Stub implementation
	return nil, nil
}

// Ensure SopsHelper implements Helper interface
var _ Helper = (*SopsHelper)(nil)

// DecryptFile decrypts a file using the SOPS package
func DecryptFile(filePath string) ([]byte, error) {
	// Check if the file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("file does not exist: %s", filePath)
	}

	// Decrypt the file using SOPS
	plaintextBytes, err := decrypt.File(filePath, "yaml")
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt file: %w", err)
	}

	return plaintextBytes, nil
}

// yamlToEnvVars retrieves Kubernetes-specific environment variables for the current context
func YamlToEnvVars(yamlData []byte) (map[string]string, error) {
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
