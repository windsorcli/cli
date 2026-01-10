package secrets

import (
	"fmt"
	"maps"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/windsorcli/cli/pkg/runtime/shell"
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
	secretsFileNameYaml    = "secrets.yaml"
	secretsFileNameYml     = "secrets.yml"
	secretsFileNameEncYaml = "secrets.enc.yaml"
	secretsFileNameEncYml  = "secrets.enc.yml"
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
func NewSopsSecretsProvider(configPath string, shell shell.Shell) *SopsSecretsProvider {
	if shell == nil {
		panic("shell is required")
	}

	return &SopsSecretsProvider{
		BaseSecretsProvider: NewBaseSecretsProvider(shell),
		configPath:          configPath,
	}
}

// =============================================================================
// Private Methods
// =============================================================================

// findSecretsFilePaths finds all existing secrets files and returns their paths.
// Checks for: secrets.yaml, secrets.yml, secrets.enc.yaml, secrets.enc.yml
// Returns all files that exist, or an empty slice if none exist.
func (s *SopsSecretsProvider) findSecretsFilePaths() []string {
	candidates := []string{
		filepath.Join(s.configPath, secretsFileNameYaml),
		filepath.Join(s.configPath, secretsFileNameYml),
		filepath.Join(s.configPath, secretsFileNameEncYaml),
		filepath.Join(s.configPath, secretsFileNameEncYml),
	}

	var existingPaths []string
	for _, path := range candidates {
		if _, err := s.shims.Stat(path); err == nil {
			existingPaths = append(existingPaths, path)
		}
	}
	return existingPaths
}

// =============================================================================
// Public Methods
// =============================================================================

// LoadSecrets loads and decrypts the secrets from all existing SOPS-encrypted files.
// If secrets are already loaded (unlocked), it returns immediately without re-decrypting.
// If no secrets files are found, it returns nil (secrets are optional).
// All existing secrets files are decrypted in parallel and merged together, with later files overriding earlier ones.
// Returns an error only if a file exists but cannot be decrypted or parsed.
func (s *SopsSecretsProvider) LoadSecrets() error {
	if s.unlocked {
		return nil
	}

	secretsFilePaths := s.findSecretsFilePaths()
	if len(secretsFilePaths) == 0 {
		return nil
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

	type fileResult struct {
		index   int
		secrets map[string]string
		err     error
	}

	results := make([]fileResult, len(secretsFilePaths))
	var wg sync.WaitGroup

	for i, secretsFilePath := range secretsFilePaths {
		wg.Add(1)
		go func(idx int, filePath string) {
			defer wg.Done()

			plaintextBytes, err := s.shims.DecryptFile(filePath, "yaml")
			if err != nil {
				results[idx] = fileResult{
					index: idx,
					err:   fmt.Errorf("failed to decrypt file %s: %w", filePath, err),
				}
				return
			}

			var sopsSecrets map[string]any
			if err := s.shims.YAMLUnmarshal(plaintextBytes, &sopsSecrets); err != nil {
				results[idx] = fileResult{
					index: idx,
					err:   fmt.Errorf("error converting YAML to secrets map from %s: %w", filePath, err),
				}
				return
			}

			fileSecrets := make(map[string]string)
			flatten(sopsSecrets, "", fileSecrets)

			results[idx] = fileResult{
				index:   idx,
				secrets: fileSecrets,
			}
		}(i, secretsFilePath)
	}

	wg.Wait()

	secretsMap := make(map[string]string)

	sort.Slice(results, func(i, j int) bool {
		return results[i].index < results[j].index
	})

	for _, result := range results {
		if result.err != nil {
			return result.err
		}

		maps.Copy(secretsMap, result.secrets)
	}

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

// =============================================================================
// Interface Compliance
// =============================================================================

// Ensure SopsSecretsProvider implements the SecretsProvider interface
var _ SecretsProvider = (*SopsSecretsProvider)(nil)
