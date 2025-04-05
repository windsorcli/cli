package env

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/secrets"
)

const (
	SessionTokenPrefix = ".session."
	EnvSessionTokenVar = "WINDSOR_SESSION_TOKEN"
)

// WindsorEnvPrinter is a struct that simulates a Kubernetes environment for testing purposes.
type WindsorEnvPrinter struct {
	BaseEnvPrinter
	sessionToken     string
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

// GetEnvVars returns a map of Windsor-specific environment variables, including the current context,
// project root, session token, and custom environment variables with resolved secrets.
func (e *WindsorEnvPrinter) GetEnvVars() (map[string]string, error) {
	// Get Windsor-specific environment variables
	envVars := make(map[string]string)

	currentContext := e.configHandler.GetContext()
	envVars["WINDSOR_CONTEXT"] = currentContext

	projectRoot, err := e.shell.GetProjectRoot()
	if err != nil {
		return nil, fmt.Errorf("error retrieving project root: %w", err)
	}
	envVars["WINDSOR_PROJECT_ROOT"] = projectRoot

	sessionToken, err := e.getSessionToken()
	if err != nil {
		return nil, fmt.Errorf("error retrieving session token: %w", err)
	}
	envVars[EnvSessionTokenVar] = sessionToken

	// Get custom environment variables from configuration
	originalEnvVars := e.configHandler.GetStringMap("environment")
	if originalEnvVars == nil {
		return envVars, nil
	}

	// #nosec G101 # This is just a regular expression not a secret
	re := regexp.MustCompile(`\${{\s*(.*?)\s*}}`)
	windsorContext := os.Getenv("WINDSOR_CONTEXT")

	useCache := true
	if windsorContext != "" && windsorContext != currentContext {
		useCache = false
	}

	for k, v := range originalEnvVars {
		if re.MatchString(v) {
			if existingValue, exists := osLookupEnv(k); exists {
				if os.Getenv("NO_CACHE") != "true" && useCache && !strings.Contains(existingValue, "<ERROR") {
					// Challenging to test this case, so we'll skip it for now
					continue
				}
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

// CreateSessionInvalidationSignal creates a signal file to invalidate the session token
// when the environment changes, ensuring a new token is generated during the next command
// execution.
func (e *WindsorEnvPrinter) CreateSessionInvalidationSignal() error {
	envToken := os.Getenv(EnvSessionTokenVar)
	if envToken == "" {
		return nil
	}

	projectRoot, err := e.shell.GetProjectRoot()
	if err != nil {
		return fmt.Errorf("failed to get project root: %w", err)
	}

	windsorDir := filepath.Join(projectRoot, ".windsor")
	if err := mkdirAll(windsorDir, 0755); err != nil {
		return fmt.Errorf("failed to create .windsor directory: %w", err)
	}

	signalFilePath := filepath.Join(windsorDir, SessionTokenPrefix+envToken)
	if err := writeFile(signalFilePath, []byte{}, 0644); err != nil {
		return fmt.Errorf("failed to create signal file: %w", err)
	}

	return nil
}

// getSessionToken retrieves or generates a session token. It first checks if a token is already stored in memory.
// If not, it looks for a token in the environment variable. If an environment token is found, it verifies the
// existence of a corresponding signal file. If the signal file exists, it deletes the file and generates a new token.
// If no token is found in the environment or no signal file exists, it generates a new token.
func (e *WindsorEnvPrinter) getSessionToken() (string, error) {
	envToken := os.Getenv(EnvSessionTokenVar)
	if envToken != "" {
		projectRoot, err := e.shell.GetProjectRoot()
		if err != nil {
			return "", fmt.Errorf("error getting project root: %w", err)
		}

		windsorDir := filepath.Join(projectRoot, ".windsor")
		tokenFilePath := filepath.Join(windsorDir, SessionTokenPrefix+envToken)
		if _, err := stat(tokenFilePath); err == nil {
			if err := osRemoveAll(tokenFilePath); err != nil {
				return "", fmt.Errorf("error removing token file: %w", err)
			}
			token, err := e.generateRandomString(7)
			if err != nil {
				return "", fmt.Errorf("error generating session token: %w", err)
			}

			e.sessionToken = token
			return token, nil
		}

		e.sessionToken = envToken
		return envToken, nil
	}

	token, err := e.generateRandomString(7)
	if err != nil {
		return "", fmt.Errorf("error generating session token: %w", err)
	}

	e.sessionToken = token
	return token, nil
}

// generateRandomString creates a secure random string of the given length using a predefined charset.
func (e *WindsorEnvPrinter) generateRandomString(length int) (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	randomBytes := make([]byte, length)

	_, err := cryptoRandRead(randomBytes)
	if err != nil {
		return "", err
	}

	// Map random bytes to charset
	for i, b := range randomBytes {
		randomBytes[i] = charset[b%byte(len(charset))]
	}

	return string(randomBytes), nil
}

// Print prints the environment variables for the Windsor environment.
func (e *WindsorEnvPrinter) Print(customVars ...map[string]string) error {
	// If customVars are provided, use them
	if len(customVars) > 0 {
		return e.BaseEnvPrinter.Print(customVars[0])
	}

	// Otherwise get the environment variables from this printer
	envVars, err := e.GetEnvVars()
	if err != nil {
		return fmt.Errorf("error getting environment variables: %w", err)
	}

	// Call the Print method of the embedded BaseEnvPrinter struct
	return e.BaseEnvPrinter.Print(envVars)
}

// Ensure WindsorEnvPrinter implements the EnvPrinter interface
var _ EnvPrinter = (*WindsorEnvPrinter)(nil)
