package secrets

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	secretsFileNameYaml = "secrets.enc.yaml"
	secretsFileNameYml  = "secrets.enc.yml"
)

// SopsSecretsProvider is a struct that implements the SecretsProvider interface using SOPS for decryption.
type SopsSecretsProvider struct {
	BaseSecretsProvider
	secretsFilePath string
}

// NewSopsSecretsProvider creates a new instance of SopsSecretsProvider.
func NewSopsSecretsProvider(configPath string) *SopsSecretsProvider {
	return &SopsSecretsProvider{
		BaseSecretsProvider: *NewBaseSecretsProvider(),
		secretsFilePath:     findSecretsFilePath(configPath),
	}
}

// findSecretsFilePath checks for the existence of the secrets file with either .yaml or .yml extension.
func findSecretsFilePath(configPath string) string {
	yamlPath := filepath.Join(configPath, secretsFileNameYaml)
	ymlPath := filepath.Join(configPath, secretsFileNameYml)

	if _, err := stat(yamlPath); err == nil {
		return yamlPath
	}
	return ymlPath
}

// LoadSecrets loads the secrets from the SOPS encrypted file located at the config path.
func (s *SopsSecretsProvider) LoadSecrets() error {
	// Check if the file exists
	if _, err := stat(s.secretsFilePath); os.IsNotExist(err) {
		return fmt.Errorf("file does not exist: %s", s.secretsFilePath)
	}

	// Decrypt the file using SOPS
	plaintextBytes, err := decryptFileFunc(s.secretsFilePath, "yaml")
	if err != nil {
		return fmt.Errorf("failed to decrypt file: %w", err)
	}

	// Convert the decrypted YAML content into a map of secrets
	var sopsSecrets map[string]interface{}
	if err := yamlUnmarshal(plaintextBytes, &sopsSecrets); err != nil {
		return fmt.Errorf("error converting YAML to secrets map: %w", err)
	}

	// Helper function to flatten the map with full path keys
	var flatten func(map[string]interface{}, string, map[string]string)
	flatten = func(data map[string]interface{}, prefix string, result map[string]string) {
		for key, value := range data {
			fullKey := key
			if prefix != "" {
				fullKey = prefix + "." + key
			}
			switch v := value.(type) {
			case map[string]interface{}:
				flatten(v, fullKey, result)
			default:
				result[fullKey] = fmt.Sprintf("%v", v)
			}
		}
	}

	// Populate secrets map from the decrypted secrets file
	secretsMap := make(map[string]string)
	flatten(sopsSecrets, "", secretsMap)

	// Store the secrets in the BaseSecretsProvider
	s.secrets = secretsMap

	// Set unlocked to true
	s.unlocked = true

	return nil
}

// Ensure SopsSecretsProvider implements the SecretsProvider interface
var _ SecretsProvider = (*SopsSecretsProvider)(nil)
