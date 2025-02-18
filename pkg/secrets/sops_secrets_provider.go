package secrets

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/windsorcli/cli/pkg/di"
)

// Define regex pattern for ${{ sops.<key> }} references as a constant
// Allow for any amount of spaces between the brackets and the "sops.<key>"
// We ignore the gosec G101 error here because this pattern is used for identifying secret placeholders,
// not for storing actual secret values. The pattern itself does not contain any hardcoded credentials.
// #nosec G101
const sopsPattern = `(?i)\${{\s*sops\.\s*([a-zA-Z0-9_]+(?:\.[a-zA-Z0-9_]+)*)\s*}}`

// #nosec G101
// This directive suppresses the gosec G101 warning, which is about hardcoded credentials.
// These are just paths, not secrets.
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
func NewSopsSecretsProvider(configPath string, injector di.Injector) *SopsSecretsProvider {
	return &SopsSecretsProvider{
		BaseSecretsProvider: *NewBaseSecretsProvider(injector),
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

// LoadSecrets checks for the existence of the SOPS encrypted file, decrypts it, converts the decrypted
// YAML content into a map of secrets, flattens the map to use full path keys, and stores the secrets in
// the BaseSecretsProvider, setting the provider to unlocked.
func (s *SopsSecretsProvider) LoadSecrets() error {
	if _, err := stat(s.secretsFilePath); os.IsNotExist(err) {
		return fmt.Errorf("file does not exist: %s", s.secretsFilePath)
	}

	plaintextBytes, err := decryptFileFunc(s.secretsFilePath, "yaml")
	if err != nil {
		return fmt.Errorf("failed to decrypt file: %w", err)
	}

	var sopsSecrets map[string]interface{}
	if err := yamlUnmarshal(plaintextBytes, &sopsSecrets); err != nil {
		return fmt.Errorf("error converting YAML to secrets map: %w", err)
	}

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

	secretsMap := make(map[string]string)
	flatten(sopsSecrets, "", secretsMap)

	s.secrets = secretsMap
	s.unlocked = true

	return nil
}

// GetSecret retrieves a secret value for the specified key
func (s *SopsSecretsProvider) GetSecret(key string) (string, error) {
	if !s.unlocked {
		return "********", nil
	}
	if value, ok := s.secrets[key]; ok {
		return value, nil
	}
	return "", fmt.Errorf("secret not found: %s", key)
}

// ParseSecrets parses a string and replaces ${{ sops.<key> }} references with their values
func (s *SopsSecretsProvider) ParseSecrets(input string) (string, error) {
	re := regexp.MustCompile(sopsPattern)

	input = re.ReplaceAllStringFunc(input, func(match string) string {
		// Extract the key from the match
		submatches := re.FindStringSubmatch(match)
		key := submatches[1]
		// Retrieve the secret value
		value, err := s.GetSecret(key)
		if err != nil {
			return "<ERROR: secret not found: " + key + ">"
		}
		return value
	})

	return input, nil
}

// Ensure SopsSecretsProvider implements the SecretsProvider interface
var _ SecretsProvider = (*SopsSecretsProvider)(nil)
