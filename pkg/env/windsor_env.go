package env

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/secrets"
)

var WindsorPrefixedVars = []string{
	"WINDSOR_CONTEXT",
	"WINDSOR_PROJECT_ROOT",
	"WINDSOR_SESSION_TOKEN",
	"WINDSOR_MANAGED_ENV",
	"WINDSOR_MANAGED_ALIAS",
}

// WindsorEnvPrinter is a struct that simulates a Kubernetes environment for testing purposes.
type WindsorEnvPrinter struct {
	BaseEnvPrinter
	secretsProviders []secrets.SecretsProvider
}

// NewWindsorEnvPrinter initializes a new WindsorEnvPrinter instance using the provided dependency injector.
func NewWindsorEnvPrinter(injector di.Injector) *WindsorEnvPrinter {
	return &WindsorEnvPrinter{
		BaseEnvPrinter: BaseEnvPrinter{
			injector: injector,
		},
	}
}

// Initialize sets up the WindsorEnvPrinter, including resolving secrets providers.
func (e *WindsorEnvPrinter) Initialize() error {
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

// GetEnvVars constructs a map of Windsor-specific environment variables including
// the current context, project root, and session token. It resolves secrets in custom
// environment variables using configured providers, handles caching of values, and
// manages environment variables and aliases. For secrets, it leverages the secrets cache
// to avoid unnecessary decryption while ensuring variables are properly tracked in the
// managed environment list. Windsor-prefixed variables are automatically included in
// the final environment setup to provide a comprehensive configuration.
func (e *WindsorEnvPrinter) GetEnvVars() (map[string]string, error) {
	envVars := make(map[string]string)

	currentContext := e.configHandler.GetContext()
	envVars["WINDSOR_CONTEXT"] = currentContext

	projectRoot, err := e.shell.GetProjectRoot()
	if err != nil {
		return nil, fmt.Errorf("error retrieving project root: %w", err)
	}
	envVars["WINDSOR_PROJECT_ROOT"] = projectRoot

	sessionToken, err := e.shell.GetSessionToken()
	if err != nil {
		return nil, fmt.Errorf("error retrieving session token: %w", err)
	}
	envVars["WINDSOR_SESSION_TOKEN"] = sessionToken

	originalEnvVars := e.configHandler.GetStringMap("environment")

	re := regexp.MustCompile(`\${{\s*(.*?)\s*}}`)

	_, managedEnvExists := osLookupEnv("WINDSOR_MANAGED_ENV")

	for k, v := range originalEnvVars {
		if !managedEnvExists {
			e.SetManagedEnv(k)
		}

		if re.MatchString(v) {
			if existingValue, exists := osLookupEnv(k); exists {
				if managedEnvExists {
					e.SetManagedEnv(k)
				}
				if shouldUseCache() && !strings.Contains(existingValue, "<ERROR") {
					continue
				}
			}
			parsedValue := e.parseAndCheckSecrets(v)
			envVars[k] = parsedValue
		} else {
			envVars[k] = v
		}
	}

	envVars["WINDSOR_MANAGED_ALIAS"] = strings.Join(e.GetManagedAlias(), ",")

	managedEnv := e.GetManagedEnv()

	combinedManagedEnv := append(managedEnv, WindsorPrefixedVars...)
	envVars["WINDSOR_MANAGED_ENV"] = strings.Join(combinedManagedEnv, ",")

	return envVars, nil
}

// parseAndCheckSecrets parses and replaces secret placeholders in the string value using the secrets provider.
// It checks for unparsed secrets and returns an error string if any are found.
func (e *WindsorEnvPrinter) parseAndCheckSecrets(strValue string) string {
	for _, secretsProvider := range e.secretsProviders {
		parsedValue, err := secretsProvider.ParseSecrets(strValue)
		if err == nil {
			strValue = parsedValue
		}
	}

	// #nosec G101 # This is just a regular expression not a secret
	re := regexp.MustCompile(`\${{\s*(.*?)\s*}}`)
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

// Print prints the environment variables for the Windsor environment.
func (e *WindsorEnvPrinter) Print() error {
	envVars, err := e.GetEnvVars()
	if err != nil {
		return fmt.Errorf("error getting environment variables: %w", err)
	}
	return e.BaseEnvPrinter.Print(envVars)
}

// Ensure WindsorEnvPrinter implements the EnvPrinter interface
var _ EnvPrinter = (*WindsorEnvPrinter)(nil)

// shouldUseCache determines if the cache should be used based on the current and Windsor context.
func shouldUseCache() bool {
	noCache := os.Getenv("NO_CACHE")
	return noCache == "" || noCache == "0" || noCache == "false" || noCache == "False"
}
