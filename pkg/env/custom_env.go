package env

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/secrets"
)

// #nosec G101 # This is just a regular expression not a secret
const secretPlaceholderPattern = `\${{\s*(.*?)\s*}}`

// CustomEnvPrinter is a struct that implements the EnvPrinter interface and handles custom environment variables.
type CustomEnvPrinter struct {
	BaseEnvPrinter
	secretsProviders []secrets.SecretsProvider
}

// NewCustomEnvPrinter initializes a new CustomEnvPrinter instance using the provided dependency injector.
func NewCustomEnvPrinter(injector di.Injector) *CustomEnvPrinter {
	return &CustomEnvPrinter{
		BaseEnvPrinter: BaseEnvPrinter{
			injector: injector,
		},
	}
}

// Initialize sets up the CustomEnvPrinter, including resolving secrets providers.
func (e *CustomEnvPrinter) Initialize() error {
	// NOTE: These errors are not presently testable in a convenient manner

	if err := e.BaseEnvPrinter.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize BaseEnvPrinter: %w", err)
	}

	// Resolve secrets providers using dependency injection
	instances, err := e.injector.ResolveAll((*secrets.SecretsProvider)(nil))
	if err != nil {
		return fmt.Errorf("failed to resolve secrets providers: %w", err)
	}
	secretsProviders := make([]secrets.SecretsProvider, 0, len(instances))

	for _, instance := range instances {
		secretsProvider, ok := instance.(secrets.SecretsProvider)
		if !ok {
			return fmt.Errorf("failed to cast instance to SecretsProvider")
		}
		secretsProviders = append(secretsProviders, secretsProvider)
	}

	e.secretsProviders = secretsProviders

	return nil
}

// Print outputs the environment variables to the console.
func (e *CustomEnvPrinter) Print() error {
	envVars, _ := e.GetEnvVars()

	return e.shell.PrintEnvVars(envVars)
}

// GetEnvVars retrieves environment variables and resolves secret placeholders.
// Caching avoids re-parsing if the variable is set and context is unchanged.
// It checks WINDSOR_CONTEXT to decide on caching strategy.
// Skips parsing if caching is enabled and value lacks "<ERROR".
// Iterates over variables, resolves secrets, and returns the map.
func (e *CustomEnvPrinter) GetEnvVars() (map[string]string, error) {
	originalEnvVars := e.configHandler.GetStringMap("environment")
	if originalEnvVars == nil {
		originalEnvVars = make(map[string]string)
	}

	re := regexp.MustCompile(secretPlaceholderPattern)
	interpretedEnvVars := make(map[string]string, len(originalEnvVars))

	currentContext := e.configHandler.GetContext()
	windsorContext := os.Getenv("WINDSOR_CONTEXT")

	useCache := true
	if windsorContext != "" && windsorContext != currentContext {
		useCache = false
	}

	for k, v := range originalEnvVars {
		if re.MatchString(v) {
			if existingValue, exists := osLookupEnv(k); exists {
				if os.Getenv("NO_CACHE") != "true" && useCache && !strings.Contains(existingValue, "<ERROR") {
					continue
				}
			}
			parsedValue := e.parseAndCheckSecrets(v)
			interpretedEnvVars[k] = parsedValue
		} else {
			interpretedEnvVars[k] = v
		}
	}

	return interpretedEnvVars, nil
}

// parseAndCheckSecrets parses and replaces secret placeholders in the string value using the secrets provider.
// It also checks for remaining unparsed secrets and returns an error string if any are found.
func (e *CustomEnvPrinter) parseAndCheckSecrets(strValue string) string {
	for _, secretsProvider := range e.secretsProviders {
		parsedValue, err := secretsProvider.ParseSecrets(strValue)
		if err == nil {
			strValue = parsedValue
		}
	}

	re := regexp.MustCompile(secretPlaceholderPattern)
	unparsedSecrets := re.FindAllStringSubmatch(strValue, -1)
	if len(unparsedSecrets) > 0 {
		if len(e.secretsProviders) == 0 {
			return "<ERROR: No secrets providers configured>"
		}
		var secretNames []string
		for _, match := range unparsedSecrets {
			if len(match) > 1 {
				secretNames = append(secretNames, match[1])
			}
		}
		secrets := strings.Join(secretNames, ", ")
		return fmt.Sprintf("<ERROR: failed to parse: %s>", secrets)
	}

	return strValue
}

// Ensure customEnv implements the EnvPrinter interface
var _ EnvPrinter = (*CustomEnvPrinter)(nil)
