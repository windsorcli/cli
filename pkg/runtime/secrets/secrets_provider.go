package secrets

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// The SecretsProvider is a core interface for secrets management
// It provides a unified interface for retrieving and parsing secrets
// It serves as the foundation for all secrets provider implementations
// It enables secure access to sensitive configuration data

// =============================================================================
// Vars
// =============================================================================

var version = "dev"

// =============================================================================
// Interfaces
// =============================================================================

// SecretsProvider defines the interface for handling secrets operations
type SecretsProvider interface {
	// LoadSecrets loads the secrets from the specified path
	LoadSecrets() error

	// GetSecret retrieves a secret value for the specified key
	GetSecret(key string) (string, error)

	// ParseSecrets parses a string and replaces ${{ secrets.<key> }} references with their values
	ParseSecrets(input string) (string, error)
}

// =============================================================================
// Types
// =============================================================================

// BaseSecretsProvider is a base implementation of the SecretsProvider interface
type BaseSecretsProvider struct {
	SecretsProvider
	secrets  map[string]string
	unlocked bool
	shell    shell.Shell
	shims    *Shims
}

// =============================================================================
// Constructor
// =============================================================================

// NewBaseSecretsProvider creates a new BaseSecretsProvider instance
func NewBaseSecretsProvider(shell shell.Shell) *BaseSecretsProvider {
	return &BaseSecretsProvider{
		secrets:  make(map[string]string),
		unlocked: false,
		shell:    shell,
		shims:    NewShims(),
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// LoadSecrets loads the secrets from the specified path
func (s *BaseSecretsProvider) LoadSecrets() error {
	s.unlocked = true
	return nil
}

// GetSecret retrieves a secret value for the specified key
func (s *BaseSecretsProvider) GetSecret(key string) (string, error) {
	panic("GetSecret must be implemented by concrete provider")
}

// ParseSecrets is a placeholder function for parsing secrets
func (s *BaseSecretsProvider) ParseSecrets(input string) (string, error) {
	panic("ParseSecrets must be implemented by concrete provider")
}

// =============================================================================
// Helpers
// =============================================================================

// parseSecrets is a helper function that parses a string and replaces secret references with their values.
// It takes a pattern to match secret references, a function to validate the keys, and a function to get the secret value.
func parseSecrets(input string, pattern string, validateKeys func([]string) bool, getSecretValue func([]string) (string, bool)) string {
	re := regexp.MustCompile(pattern)

	return re.ReplaceAllStringFunc(input, func(match string) string {
		// Extract the key path from the match
		submatches := re.FindStringSubmatch(match)
		if len(submatches) < 2 || submatches[1] == "" {
			return "<ERROR: invalid secret format>"
		}
		keyPath := strings.TrimSpace(submatches[1])

		// Parse the key path using parseKeys
		keys := parseKeys(keyPath)

		// Validate the keys
		if !validateKeys(keys) {
			return fmt.Sprintf("<ERROR: invalid key path: %s>", keyPath)
		}

		// Get the secret value
		value, shouldReplace := getSecretValue(keys)
		if !shouldReplace {
			return match
		}
		return value
	})
}

// ParseKeys processes a string path that may contain mixed dot and bracket notations,
// extracting and returning an array of keys. It handles quoted strings within brackets
// and treats consecutive dots as empty keys unless they follow a closing bracket.
func parseKeys(path string) []string {
	var keys []string
	var currentKey strings.Builder
	var bracketDepth int
	inQuotes := false
	justClosedBracket := false

	trimmedPath := strings.TrimSpace(path)

	for i := 0; i < len(trimmedPath); i++ {
		char := rune(trimmedPath[i])
		switch char {
		case '[':
			if !inQuotes {
				if bracketDepth == 0 {
					// finalize current key if any
					if currentKey.Len() > 0 {
						keys = append(keys, currentKey.String())
						currentKey.Reset()
					}
					justClosedBracket = false
				} else {
					// store nested bracket
					currentKey.WriteRune(char)
				}
				bracketDepth++
			} else {
				currentKey.WriteRune(char)
			}
		case ']':
			if !inQuotes {
				bracketDepth--
				if bracketDepth < 0 {
					bracketDepth = 0
				}
				if bracketDepth == 0 {
					// finalize bracket key
					if currentKey.Len() > 0 {
						keys = append(keys, currentKey.String())
						currentKey.Reset()
					} else {
						// empty bracket
						keys = append(keys, "")
					}
					justClosedBracket = true
				} else {
					// store nested closing bracket
					currentKey.WriteRune(char)
				}
			} else {
				currentKey.WriteRune(char)
			}
		case '.':
			if bracketDepth == 0 && !inQuotes {
				if currentKey.Len() > 0 {
					keys = append(keys, currentKey.String())
					currentKey.Reset()
					justClosedBracket = false
				} else {
					if !justClosedBracket {
						keys = append(keys, "")
					}
					justClosedBracket = false
				}
			} else {
				currentKey.WriteRune(char)
			}
		case '"', '\'':
			if bracketDepth > 0 {
				inQuotes = !inQuotes
			}
			justClosedBracket = false
		case '\\':
			if bracketDepth > 0 && inQuotes && i+1 < len(trimmedPath) {
				// Handle escaped characters within quotes
				i++
				currentKey.WriteRune(rune(trimmedPath[i]))
			} else {
				currentKey.WriteRune(char)
			}
			justClosedBracket = false
		default:
			currentKey.WriteRune(char)
			justClosedBracket = false
		}
	}

	if currentKey.Len() > 0 || !justClosedBracket {
		keys = append(keys, currentKey.String())
	}

	return keys
}

// =============================================================================
// Interface Compliance
// =============================================================================

// Ensure BaseSecretsProvider implements SecretsProvider
var _ SecretsProvider = (*BaseSecretsProvider)(nil)
