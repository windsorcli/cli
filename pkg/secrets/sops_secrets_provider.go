package secrets

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/windsorcli/cli/pkg/di"
)

// The SopsSecretsProvider is an implementation of the SecretsProvider interface
// It provides integration with Mozilla SOPS for encrypted secrets management with automatic shell scrubbing registration
// It serves as a bridge between the application and SOPS-encrypted YAML files with built-in security features
// It enables secure storage and retrieval of secrets using SOPS encryption while automatically registering secrets for output scrubbing

// =============================================================================
// Constants
// =============================================================================

// Define regex pattern for ${{ sops.<key> }} references as a constant
// Allow for any amount of spaces between the brackets and the "sops.<key>"
// We ignore the gosec G101 error here because this pattern is used for identifying secret placeholders,
// not for storing actual secret values. The pattern itself does not contain any hardcoded credentials.
// #nosec G101
const sopsPattern = `(?i)\${{\s*sops\.\s*([^}\s]*?)\s*}}`

// #nosec G101
// This directive suppresses the gosec G101 warning, which is about hardcoded credentials.
// These are just paths, not secrets.
const (
	secretsFileNameYaml = "secrets.enc.yaml"
	secretsFileNameYml  = "secrets.enc.yml"
)

// =============================================================================
// Types
// =============================================================================

// SopsSecretsProvider is a struct that implements the SecretsProvider interface using SOPS for decryption.
type SopsSecretsProvider struct {
	*BaseSecretsProvider
	configPath string
}

// =============================================================================
// Constructor
// =============================================================================

// NewSopsSecretsProvider creates a new instance of SopsSecretsProvider.
func NewSopsSecretsProvider(configPath string, injector di.Injector) *SopsSecretsProvider {
	return &SopsSecretsProvider{
		BaseSecretsProvider: NewBaseSecretsProvider(injector),
		configPath:          configPath,
	}
}

// =============================================================================
// Private Methods
// =============================================================================

// findSecretsFilePath checks for the existence of the secrets file with either .yaml or .yml extension.
func (s *SopsSecretsProvider) findSecretsFilePath() string {
	yamlPath := filepath.Join(s.configPath, secretsFileNameYaml)
	ymlPath := filepath.Join(s.configPath, secretsFileNameYml)

	if _, err := s.shims.Stat(yamlPath); err == nil {
		return yamlPath
	}
	return ymlPath
}

// =============================================================================
// Public Methods
// =============================================================================

// LoadSecrets loads and decrypts the secrets from the SOPS-encrypted file.
func (s *SopsSecretsProvider) LoadSecrets() error {
	secretsFilePath := s.findSecretsFilePath()
	if _, err := s.shims.Stat(secretsFilePath); os.IsNotExist(err) {
		return fmt.Errorf("file does not exist: %s", secretsFilePath)
	}

	plaintextBytes, err := s.shims.DecryptFile(secretsFilePath, "yaml")
	if err != nil {
		return fmt.Errorf("failed to decrypt file: %w", err)
	}

	var sopsSecrets map[string]any
	if err := s.shims.YAMLUnmarshal(plaintextBytes, &sopsSecrets); err != nil {
		return fmt.Errorf("error converting YAML to secrets map: %w", err)
	}

	var flatten func(map[string]any, string, map[string]string)
	flatten = func(data map[string]any, prefix string, result map[string]string) {
		for key, value := range data {
			fullKey := key
			if prefix != "" {
				fullKey = prefix + "." + key
			}
			switch v := value.(type) {
			case map[string]any:
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

// GetSecret retrieves a secret value for the specified key and automatically registers it with the shell for output scrubbing.
// If the provider is locked, it returns a masked value. When unlocked, it returns the actual secret value
// and registers it with the shell's scrubbing system to prevent accidental exposure in command output.
func (s *SopsSecretsProvider) GetSecret(key string) (string, error) {
	if !s.unlocked {
		return "********", nil
	}

	if value, ok := s.secrets[key]; ok {
		s.shell.RegisterSecret(value)
		return value, nil
	}

	return "", fmt.Errorf("secret not found: %s", key)
}

// ParseSecrets parses a string and replaces ${{ sops.<key> }} references with their values
func (s *SopsSecretsProvider) ParseSecrets(input string) (string, error) {
	result := parseSecrets(input, sopsPattern, func(keys []string) bool {
		for _, key := range keys {
			if key == "" {
				return false
			}
		}
		return true
	}, func(keys []string) (string, bool) {
		key := strings.Join(keys, ".")
		value, err := s.GetSecret(key)
		if err != nil {
			return fmt.Sprintf("<ERROR: secret not found: %s>", key), true
		}
		return value, true
	})
	return result, nil
}

// Ensure SopsSecretsProvider implements the SecretsProvider interface
var _ SecretsProvider = (*SopsSecretsProvider)(nil)
