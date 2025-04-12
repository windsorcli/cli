package secrets

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
)

var version = "dev"

// SecretsProvider defines the interface for handling secrets operations
type SecretsProvider interface {
	// Initialize initializes the secrets provider
	Initialize() error

	// LoadSecrets loads the secrets from the specified path
	LoadSecrets() error

	// GetSecret retrieves a secret value for the specified key
	GetSecret(key string) (string, error)

	// ParseSecrets parses a string and replaces ${{ secrets.<key> }} references with their values
	ParseSecrets(input string) (string, error)

	// isUnlocked returns true if the secrets provider is locked (not unlocked)
	isUnlocked() bool
}

// BaseSecretsProvider is a base implementation of the SecretsProvider interface
type BaseSecretsProvider struct {
	SecretsProvider
	secrets  map[string]string
	unlocked bool
	shell    shell.Shell
	injector di.Injector
}

// NewBaseSecretsProvider creates a new BaseSecretsProvider instance
func NewBaseSecretsProvider(injector di.Injector) *BaseSecretsProvider {
	return &BaseSecretsProvider{
		secrets:  make(map[string]string),
		unlocked: false,
		injector: injector,
	}
}

// Initialize initializes the secrets provider
func (s *BaseSecretsProvider) Initialize() error {
	// Retrieve the shell instance from the injector
	shellInstance := s.injector.Resolve("shell")
	if shellInstance == nil {
		return fmt.Errorf("failed to resolve shell instance from injector")
	}

	// Type assert the resolved instance to shell.Shell
	shell, ok := shellInstance.(shell.Shell)
	if !ok {
		return fmt.Errorf("resolved shell instance is not of type shell.Shell")
	}

	// Assign the resolved shell instance to the BaseSecretsProvider's shell field
	s.shell = shell

	return nil
}

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

// isUnlocked returns true if the secrets provider is unlocked
func (s *BaseSecretsProvider) isUnlocked() bool {
	return s.unlocked
}

// Ensure BaseSecretsProvider implements SecretsProvider
var _ SecretsProvider = (*BaseSecretsProvider)(nil)

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
