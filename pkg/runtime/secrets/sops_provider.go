package secrets

import (
	"fmt"
	"maps"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// =============================================================================
// Constants
// =============================================================================

const (
	secretsFileNameYaml    = "secrets.yaml"
	secretsFileNameYml     = "secrets.yml"
	secretsFileNameEncYaml = "secrets.enc.yaml"
	secretsFileNameEncYml  = "secrets.enc.yml"
)

// =============================================================================
// Types
// =============================================================================

// SopsProvider implements the Provider interface using SOPS for decryption.
type SopsProvider struct {
	secrets    map[string]string
	unlocked   bool
	configPath string
	shims      *Shims
}

// =============================================================================
// Constructor
// =============================================================================

// NewSopsProvider creates a new SopsProvider instance.
func NewSopsProvider(configPath string) *SopsProvider {
	return &SopsProvider{
		secrets:    make(map[string]string),
		configPath: configPath,
		shims:      NewShims(),
	}
}

// =============================================================================
// Provider Interface
// =============================================================================

// LoadSecrets loads and decrypts secrets from all existing SOPS-encrypted files.
func (s *SopsProvider) LoadSecrets() error {
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

// Resolve fetches a SOPS secret by reference. Returns handled=true only if
// the vault is "sops" (case-insensitive).
func (s *SopsProvider) Resolve(ref SecretRef) (string, bool, error) {
	if !strings.EqualFold(ref.Vault, "sops") {
		return "", false, nil
	}

	keyPath := ref.Item
	if ref.Field != "" {
		keyPath = ref.Item + "." + ref.Field
	}

	if !s.unlocked {
		return "********", true, nil
	}

	value, ok := s.secrets[keyPath]
	if !ok {
		return "", true, fmt.Errorf("secret not found: %s", keyPath)
	}
	return value, true, nil
}

// =============================================================================
// Private Methods
// =============================================================================

func (s *SopsProvider) findSecretsFilePaths() []string {
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
// Interface Compliance
// =============================================================================

var _ Provider = (*SopsProvider)(nil)
