package env

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/windsorcli/cli/pkg/di"
)

const (
	SessionTokenPrefix = ".session."
	EnvSessionTokenVar = "WINDSOR_SESSION_TOKEN"
)

// WindsorEnvPrinter is a struct that simulates a Kubernetes environment for testing purposes.
type WindsorEnvPrinter struct {
	BaseEnvPrinter
	sessionToken string
}

// NewWindsorEnvPrinter initializes a new WindsorEnvPrinter instance using the provided dependency injector.
func NewWindsorEnvPrinter(injector di.Injector) *WindsorEnvPrinter {
	return &WindsorEnvPrinter{
		BaseEnvPrinter: BaseEnvPrinter{
			injector: injector,
		},
	}
}

// GetEnvVars returns a map of Windsor-specific environment variables, including the current context,
// project root, and session token. It retrieves the current context from the config handler, the
// project root from the shell, and generates or retrieves a session token.
func (e *WindsorEnvPrinter) GetEnvVars() (map[string]string, error) {
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

	return envVars, nil
}

// Print prints the environment variables for the Windsor environment.
func (e *WindsorEnvPrinter) Print() error {
	envVars, err := e.GetEnvVars()
	if err != nil {
		return fmt.Errorf("error getting environment variables: %w", err)
	}
	return e.BaseEnvPrinter.Print(envVars)
}

// getSessionToken retrieves or generates a session token. It first checks if a token is already stored in memory.
// If not, it looks for a token in the environment variable. If an environment token is found, it verifies the
// existence of a corresponding signal file. If the signal file exists, it deletes the file and generates a new token.
// If no token is found in the environment or no signal file exists, it generates a new token.
func (e *WindsorEnvPrinter) getSessionToken() (string, error) {
	if e.sessionToken != "" {
		return e.sessionToken, nil
	}

	envToken := os.Getenv(EnvSessionTokenVar)
	if envToken != "" {
		projectRoot, err := e.shell.GetProjectRoot()
		if err != nil {
			return "", fmt.Errorf("error getting project root: %w", err)
		}

		windsorDir := filepath.Join(projectRoot, ".windsor")
		tokenFilePath := filepath.Join(windsorDir, SessionTokenPrefix+envToken)
		if _, err := stat(tokenFilePath); err == nil {
			osRemoveAll(tokenFilePath)
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

// Ensure WindsorEnvPrinter implements the EnvPrinter interface
var _ EnvPrinter = (*WindsorEnvPrinter)(nil)
