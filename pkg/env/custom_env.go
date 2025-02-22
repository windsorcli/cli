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

// GetEnvVars retrieves environment variables from the configuration and resolves any secret placeholders.
// It uses caching to avoid unnecessary parsing if the variable is already set in the environment and the context hasn't changed.
// The function first checks the current context against the WINDSOR_CONTEXT environment variable to decide on caching.
// It then iterates over the configuration's environment variables, resolving secrets where needed, and returns the final map.
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
			if _, exists := os.LookupEnv(k); exists {
				if os.Getenv("NO_CACHE") != "true" && useCache {
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
