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
const secretPlaceholderPattern = `\${{\s*([^}\s]+)\s*}}`

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
	e.BaseEnvPrinter.Initialize()

	// Resolve secrets providers using dependency injection
	instances, _ := e.injector.ResolveAll((*secrets.SecretsProvider)(nil))
	secretsProviders := make([]secrets.SecretsProvider, 0, len(instances))

	for _, instance := range instances {
		secretsProvider, _ := instance.(secrets.SecretsProvider)
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

// GetEnvVars retrieves environment variables from the configuration, resolving any secret placeholders.
// If a variable is already set in the environment and caching is allowed, it skips parsing.
func (e *CustomEnvPrinter) GetEnvVars() (map[string]string, error) {
	envVars := e.configHandler.GetStringMap("environment")
	if envVars == nil {
		envVars = make(map[string]string)
	}

	re := regexp.MustCompile(secretPlaceholderPattern)

	for k, v := range envVars {
		if re.MatchString(v) {
			if cachedValue, exists := os.LookupEnv(k); exists && os.Getenv("NO_CACHE") != "true" {
				envVars[k] = cachedValue
				continue
			}
			parsedValue := e.parseAndCheckSecrets(v)
			envVars[k] = parsedValue
		} else {
			envVars[k] = v
		}
	}

	return envVars, nil
}

// parseAndCheckSecrets parses and replaces secret placeholders in the string value using the secrets provider.
// It also checks for remaining unparsed secrets and returns an error string if any are found.
func (e *CustomEnvPrinter) parseAndCheckSecrets(strValue string) string {
	fmt.Printf("Parsing secrets for value: %s\n", strValue)
	for _, secretsProvider := range e.secretsProviders {
		fmt.Printf("Using secrets provider: %v\n", secretsProvider)
		parsedValue, err := secretsProvider.ParseSecrets(strValue)
		if err == nil {
			fmt.Printf("Resolved secret: %s\n", parsedValue)
			strValue = parsedValue
		} else {
			fmt.Printf("Failed to resolve secret: %v\n", err)
		}
	}

	re := regexp.MustCompile(secretPlaceholderPattern)
	unparsedSecrets := re.FindAllStringSubmatch(strValue, -1)
	if len(unparsedSecrets) > 0 {
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
