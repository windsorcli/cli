package env

import (
	"fmt"
	"maps"
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

// GetEnvVars constructs the environment variables map by combining Windsor-specific variables
// with additional environment variables provided via configuration, resolving secret placeholders when needed.
func (e *WindsorEnvPrinter) GetEnvVars() (map[string]string, error) {
	envVars := make(map[string]string)

	// Add Windsor-specific environment variables.
	currentContext := e.configHandler.GetContext()
	envVars["WINDSOR_CONTEXT"] = currentContext

	projectRoot, err := e.shell.GetProjectRoot()
	if err != nil {
		return nil, fmt.Errorf("error retrieving project root: %w", err)
	}
	envVars["WINDSOR_PROJECT_ROOT"] = projectRoot

	// On Darwin, set execution mode to "container" if not already defined.
	if goos() == "darwin" {
		if _, exists := envVars["WINDSOR_EXEC_MODE"]; !exists {
			envVars["WINDSOR_EXEC_MODE"] = "container"
		}
	}

	// Merge additional environment variables from the configuration.
	originalEnvVars := e.configHandler.GetStringMap("environment")
	if originalEnvVars == nil {
		originalEnvVars = make(map[string]string)
	}

	re := regexp.MustCompile(secretPlaceholderPattern)
	for k, v := range originalEnvVars {
		// If the value contains secret placeholders, resolve them.
		if re.MatchString(v) {
			// Check if there's already a cached value using os.LookupEnv.
			if existingValue, exists := os.LookupEnv(k); exists {
				// If caching is enabled and the value doesn't contain error markers, use it.
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

	// Merge any previously printed environment variables.
	mu.Lock()
	maps.Copy(envVars, printedEnvVars)
	mu.Unlock()

	// Build the WINDSOR_MANAGED_ENV variable as a comma-separated list of managed keys.
	managedEnvKeys := []string{"WINDSOR_CONTEXT", "WINDSOR_PROJECT_ROOT", "WINDSOR_EXEC_MODE"}
	for key := range printedEnvVars {
		managedEnvKeys = append(managedEnvKeys, key)
	}
	envVars["WINDSOR_MANAGED_ENV"] = strings.Join(managedEnvKeys, ",")

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
