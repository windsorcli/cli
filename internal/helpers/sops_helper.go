package helpers

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/getsops/sops/v3/decrypt"
	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/shell"
	"gopkg.in/yaml.v2"
)

// SopsHelper is a helper struct that provides Kubernetes-specific utility functions
type SopsHelper struct {
	ConfigHandler config.ConfigHandler
	Shell         shell.Shell
	Context       context.ContextInterface
}

// NewSopsHelper is a constructor for SopsHelper
func NewSopsHelper(configHandler config.ConfigHandler, shell shell.Shell, ctx context.ContextInterface) *SopsHelper {
	return &SopsHelper{
		ConfigHandler: configHandler,
		Shell:         shell,
		Context:       ctx,
	}
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

	// Parse the decrypted YAML content into a map
	var sopsSecrets map[string]string
	if err := yaml.Unmarshal(plaintextBytes, &sopsSecrets); err != nil {
		return nil, fmt.Errorf("error parsing sops file: %w", err)
	}

	// Populate envVars from the decrypted secrets file
	envVars := make(map[string]string)
	for key, value := range sopsSecrets {
		envVars[key] = value
	}

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
