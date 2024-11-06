package helpers

import (
	"fmt"
	"os"

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
	resolvedContext, err := di.Resolve("contextHandler")
	if err != nil {
		return nil, fmt.Errorf("error resolving context: %w", err)
	}

	return &SopsHelper{
		Context: resolvedContext.(context.ContextInterface),
	}, nil
}

// Initialize performs any necessary initialization for the helper.
func (h *SopsHelper) Initialize() error {
	// Perform any necessary initialization here
	return nil
}

// GetComposeConfig returns a list of container data for docker-compose.
func (h *SopsHelper) GetComposeConfig() (*types.Config, error) {
	// Stub implementation
	return nil, nil
}

// WriteConfig writes any vendor specific configuration files that are needed for the helper.
func (h *SopsHelper) WriteConfig() error {
	return nil
}

// Up executes necessary commands to instantiate the tool or environment.
func (h *SopsHelper) Up(verbose ...bool) error {
	return nil
}

// Info returns information about the helper.
func (h *SopsHelper) Info() (interface{}, error) {
	return nil, nil
}

// Ensure SopsHelper implements Helper interface
var _ Helper = (*SopsHelper)(nil)

// DecryptFile decrypts a file using the SOPS package
func DecryptFile(filePath string) ([]byte, error) {
	// Check if the file exists
	if _, err := stat(filePath); os.IsNotExist(err) {
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
