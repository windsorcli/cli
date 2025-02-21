package secrets

import (
	"fmt"
	"strings"

	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
)

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
}

// BaseSecretsProvider is a base implementation of the SecretsProvider interface
type BaseSecretsProvider struct {
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
	// Placeholder logic for retrieving a secret
	return "", nil
}

// ParseSecrets is a placeholder function for parsing secrets
func (s *BaseSecretsProvider) ParseSecrets(input string) (string, error) {
	// Placeholder logic for parsing secrets
	return input, nil
}

// ParseKeys processes a string path that may contain mixed dot and bracket notations,
// extracting and returning an array of keys. It handles quoted strings within brackets
// and treats consecutive dots as empty keys unless they follow a closing bracket.
func ParseKeys(path string) []string {
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

// Ensure BaseSecretsProvider implements SecretsProvider
var _ SecretsProvider = (*BaseSecretsProvider)(nil)
