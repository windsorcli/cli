package env

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/secrets"
)

// secretPlaceholderPattern is a regex pattern to match secret placeholders in the form ${{ SECRET_NAME }}.
const secretPlaceholderPattern = `\$\{\{\s*(.*?)\s*\}\}`

// WindsorEnvPrinter handles environment variables provided by the Windsor framework.
// It resolves secret placeholders using configured secrets providers and builds a Windsor-managed environment map,
// including keys such as WINDSOR_CONTEXT, WINDSOR_PROJECT_ROOT, and WINDSOR_EXEC_MODE.

// WindsorEnvPrinter implements the EnvPrinter interface.
type WindsorEnvPrinter struct {
	BaseEnvPrinter
	secretsProviders []secrets.SecretsProvider
}

// NewWindsorEnvPrinter initializes a new WindsorEnvPrinter instance using the provided dependency injector.
func NewWindsorEnvPrinter(injector di.Injector) *WindsorEnvPrinter {
	windsorEnvPrinter := &WindsorEnvPrinter{}
	windsorEnvPrinter.BaseEnvPrinter = BaseEnvPrinter{
		injector:   injector,
		envPrinter: windsorEnvPrinter,
	}
	return windsorEnvPrinter
}

// Initialize sets up the WindsorEnvPrinter and resolves available secrets providers.
func (e *WindsorEnvPrinter) Initialize() error {
	// Initialize the base printer first.
	if err := e.BaseEnvPrinter.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize BaseEnvPrinter: %w", err)
	}

	// Resolve secrets providers via dependency injection.
	instances, _ := e.injector.ResolveAll((*secrets.SecretsProvider)(nil))

	providers := make([]secrets.SecretsProvider, len(instances))
	for i, instance := range instances {
		providers[i] = instance.(secrets.SecretsProvider)
	}
	e.secretsProviders = providers
	return nil
}

// GetEnvVars builds a map of environment variables for Windsor. It combines Windsor-specific
// variables with additional ones from the configuration, resolving secret placeholders. It also
// manages context determination and caching, ensuring a comprehensive environment setup.
func (e *WindsorEnvPrinter) GetEnvVars() (map[string]string, error) {
	envVars := make(map[string]string)

	// Get the project root directory
	projectRoot, err := e.shell.GetProjectRoot()
	if err != nil {
		return nil, fmt.Errorf("error getting project root: %w", err)
	}

	// Get the context (the GetContext method now internally checks for PID-based triggers)
	context := e.configHandler.GetContext()

	// Get the session token directly from the shell
	sessionToken := e.shell.GetSessionToken()

	envVars["WINDSOR_CONTEXT"] = context
	envVars["WINDSOR_PROJECT_ROOT"] = projectRoot
	envVars["WINDSOR_SESSION_TOKEN"] = sessionToken

	// Merge additional environment variables from the configuration
	originalEnvVars := e.configHandler.GetStringMap("environment")
	re := regexp.MustCompile(secretPlaceholderPattern)
	for k, v := range originalEnvVars {
		if re.MatchString(v) {
			if existingValue, exists := os.LookupEnv(k); exists {
				if os.Getenv("NO_CACHE") != "true" && !strings.Contains(existingValue, "<ERROR") {
					envVars[k] = existingValue
					continue
				}
			}
			envVars[k] = e.parseAndCheckSecrets(v)
		} else {
			envVars[k] = v
		}
	}

	// Build the WINDSOR_MANAGED_ENV variable as a comma-separated list of managed keys
	managedEnvKeys := []string{"WINDSOR_CONTEXT", "WINDSOR_PROJECT_ROOT", "WINDSOR_EXEC_MODE", "WINDSOR_MANAGED_ENV", "WINDSOR_MANAGED_ALIASES", "WINDSOR_SESSION_TOKEN"}
	for key := range printedEnvVars {
		managedEnvKeys = append(managedEnvKeys, key)
	}
	envVars["WINDSOR_MANAGED_ENV"] = strings.Join(managedEnvKeys, ",")

	// Build the WINDSOR_MANAGED_ALIASES variable as a comma-separated list of managed keys
	managedAliasesKeys := []string{}
	for key := range printedAliases {
		managedAliasesKeys = append(managedAliasesKeys, key)
	}
	envVars["WINDSOR_MANAGED_ALIASES"] = strings.Join(managedAliasesKeys, ",")

	return envVars, nil
}

// parseAndCheckSecrets parses secret placeholders in strValue using the configured secrets providers.
// It returns the resolved string or an error string if secrets remain unparsed.
func (e *WindsorEnvPrinter) parseAndCheckSecrets(strValue string) string {
	for _, provider := range e.secretsProviders {
		parsed, err := provider.ParseSecrets(strValue)
		if err == nil {
			strValue = parsed
		}
	}

	re := regexp.MustCompile(secretPlaceholderPattern)
	unparsed := re.FindAllStringSubmatch(strValue, -1)
	if len(unparsed) > 0 {
		if len(e.secretsProviders) == 0 {
			return "<ERROR: No secrets providers configured>"
		}
		var secretNames []string
		for _, match := range unparsed {
			if len(match) > 1 {
				secretNames = append(secretNames, match[1])
			}
		}
		return fmt.Sprintf("<ERROR: failed to parse: %s>", strings.Join(secretNames, ", "))
	}
	return strValue
}

// Print outputs the constructed environment variables to the console.
func (e *WindsorEnvPrinter) Print() error {
	envVars, err := e.GetEnvVars()
	if err != nil {
		return err
	}
	return e.shell.PrintEnvVars(envVars)
}

// Ensure WindsorEnvPrinter implements the EnvPrinter interface.
var _ EnvPrinter = (*WindsorEnvPrinter)(nil)
