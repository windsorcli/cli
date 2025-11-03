// The WindsorEnvPrinter is a specialized component that manages Windsor environment configuration.
// It provides Windsor-specific environment variable management and configuration,
// The WindsorEnvPrinter handles context, project root, and secrets management,
// ensuring proper Windsor CLI integration and environment setup for application operations.

package env

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/windsorcli/cli/pkg/context/secrets"
	"github.com/windsorcli/cli/pkg/di"
)

// =============================================================================
// Constants
// =============================================================================

// WindsorPrefixedVars are the environment variables that are managed by Windsor.
var WindsorPrefixedVars = []string{
	"WINDSOR_CONTEXT",
	"WINDSOR_CONTEXT_ID",
	"BUILD_ID",
	"WINDSOR_PROJECT_ROOT",
	"WINDSOR_SESSION_TOKEN",
	"WINDSOR_MANAGED_ENV",
	"WINDSOR_MANAGED_ALIAS",
}

// =============================================================================
// Types
// =============================================================================

// WindsorEnvPrinter is a struct that implements Windsor environment configuration
type WindsorEnvPrinter struct {
	BaseEnvPrinter
	secretsProviders []secrets.SecretsProvider
}

// =============================================================================
// Constructor
// =============================================================================

// NewWindsorEnvPrinter creates a new WindsorEnvPrinter instance
func NewWindsorEnvPrinter(injector di.Injector) *WindsorEnvPrinter {
	return &WindsorEnvPrinter{
		BaseEnvPrinter: *NewBaseEnvPrinter(injector),
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// Initialize performs dependency injection setup, resolves shell and configuration components,
// and initializes base functionality. It resolves secrets providers from the dependency injection
// container and handles environment variable management setup with proper error handling and validation.
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
		secretsProviders = append(secretsProviders, instance.(secrets.SecretsProvider))
	}

	e.secretsProviders = secretsProviders

	return nil
}

// GetEnvVars constructs a map of Windsor-specific environment variables by retrieving
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

	contextID := e.configHandler.GetString("id", "")
	envVars["WINDSOR_CONTEXT_ID"] = contextID

	// Get build ID from the .windsor/.build-id file
	buildID, err := e.getBuildIDFromFile()
	if err == nil && buildID != "" {
		envVars["BUILD_ID"] = buildID
	}

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

	_, managedEnvExists := e.shims.LookupEnv("WINDSOR_MANAGED_ENV")

	for k, v := range originalEnvVars {
		if !managedEnvExists {
			e.SetManagedEnv(k)
		}

		if re.MatchString(v) {
			if existingValue, exists := e.shims.LookupEnv(k); exists {
				if managedEnvExists {
					e.SetManagedEnv(k)
				}
				if e.shouldUseCache() && !strings.Contains(existingValue, "<ERROR") {
					continue
				}
			}
			parsedValue := e.parseAndCheckSecrets(v)
			envVars[k] = parsedValue
		} else {
			envVars[k] = v
		}
	}

	// Collect managed envs and aliases from all env printers
	instances, err := e.injector.ResolveAll((*EnvPrinter)(nil))
	if err != nil {
		return nil, fmt.Errorf("failed to resolve env printers: %w", err)
	}

	var allManagedEnv []string
	var allManagedAlias []string

	// Add our own managed envs and aliases
	allManagedEnv = append(allManagedEnv, e.GetManagedEnv()...)
	allManagedAlias = append(allManagedAlias, e.GetManagedAlias()...)

	// Collect from other env printers
	for _, instance := range instances {
		if printer, ok := instance.(EnvPrinter); ok && printer != e {
			allManagedEnv = append(allManagedEnv, printer.GetManagedEnv()...)
			allManagedAlias = append(allManagedAlias, printer.GetManagedAlias()...)
		}
	}

	// Add Windsor prefixed vars to managed env (excluding BUILD_ID if not available)
	windsorVars := make([]string, 0, len(WindsorPrefixedVars))
	for _, varName := range WindsorPrefixedVars {
		if varName == "BUILD_ID" {
			// Only include BUILD_ID if it's actually set
			if _, exists := envVars["BUILD_ID"]; exists {
				windsorVars = append(windsorVars, varName)
			}
		} else {
			windsorVars = append(windsorVars, varName)
		}
	}
	allManagedEnv = append(allManagedEnv, windsorVars...)

	// Set the combined managed env and alias
	envVars["WINDSOR_MANAGED_ENV"] = strings.Join(allManagedEnv, ",")
	envVars["WINDSOR_MANAGED_ALIAS"] = strings.Join(allManagedAlias, ",")

	return envVars, nil
}

// =============================================================================
// Private Methods
// =============================================================================

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

// shouldUseCache determines if the cache should be used based on NO_CACHE environment variable.
// Cache is enabled by default and can be disabled by setting NO_CACHE=1 or NO_CACHE=true.
func (e *WindsorEnvPrinter) shouldUseCache() bool {
	noCache, _ := e.shims.LookupEnv("NO_CACHE")
	return noCache == "" || noCache == "0" || noCache == "false" || noCache == "False"
}

// getBuildIDFromFile retrieves the build ID from the .windsor/.build-id file
func (e *WindsorEnvPrinter) getBuildIDFromFile() (string, error) {
	projectRoot, err := e.shell.GetProjectRoot()
	if err != nil {
		return "", fmt.Errorf("failed to get project root: %w", err)
	}

	buildIDPath := filepath.Join(projectRoot, ".windsor", ".build-id")

	if _, err := e.shims.Stat(buildIDPath); os.IsNotExist(err) {
		return "", nil
	}

	data, err := e.shims.ReadFile(buildIDPath)
	if err != nil {
		return "", fmt.Errorf("failed to read build ID file: %w", err)
	}

	return strings.TrimSpace(string(data)), nil
}

// Ensure WindsorEnvPrinter implements the EnvPrinter interface
var _ EnvPrinter = (*WindsorEnvPrinter)(nil)
