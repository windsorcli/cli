package env

import (
	"fmt"

	"github.com/windsorcli/cli/pkg/di"
)

// WindsorEnvPrinter is a struct that simulates a Kubernetes environment for testing purposes.
type WindsorEnvPrinter struct {
	BaseEnvPrinter
}

// NewWindsorEnvPrinter initializes a new WindsorEnvPrinter instance using the provided dependency injector.
func NewWindsorEnvPrinter(injector di.Injector) *WindsorEnvPrinter {
	windsorEnvPrinter := &WindsorEnvPrinter{}
	windsorEnvPrinter.BaseEnvPrinter = BaseEnvPrinter{
		injector:   injector,
		EnvPrinter: windsorEnvPrinter,
	}
	return windsorEnvPrinter
}

// GetEnvVars constructs a map of environment variables for the Windsor environment,
// including context, project root, execution mode based on the OS, and session token.
func (e *WindsorEnvPrinter) GetEnvVars() (map[string]string, error) {
	envVars := make(map[string]string)

	currentContext := e.configHandler.GetContext()
	envVars["WINDSOR_CONTEXT"] = currentContext

	projectRoot, err := e.shell.GetProjectRoot()
	if err != nil {
		return nil, fmt.Errorf("error retrieving project root: %w", err)
	}
	envVars["WINDSOR_PROJECT_ROOT"] = projectRoot

	if goos() == "darwin" {
		if _, exists := envVars["WINDSOR_EXEC_MODE"]; !exists {
			envVars["WINDSOR_EXEC_MODE"] = "container"
		}
	}

	// Set the WINDSOR_SESSION_TOKEN using the shell's GetSessionToken method
	sessionToken := e.shell.GetSessionToken()
	envVars["WINDSOR_SESSION_TOKEN"] = sessionToken

	return envVars, nil
}

// Ensure WindsorEnvPrinter implements the EnvPrinter interface
var _ EnvPrinter = (*WindsorEnvPrinter)(nil)
