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

// GetEnvVars retrieves the environment variables for the Windsor environment.
func (e *WindsorEnvPrinter) GetEnvVars() (map[string]string, error) {
	envVars := make(map[string]string)

	// Add WINDSOR_CONTEXT to the environment variables
	currentContext := e.configHandler.GetContext()
	envVars["WINDSOR_CONTEXT"] = currentContext

	// Get the project root and add WINDSOR_PROJECT_ROOT to the environment variables
	projectRoot, err := e.shell.GetProjectRoot()
	if err != nil {
		return nil, fmt.Errorf("error retrieving project root: %w", err)
	}
	envVars["WINDSOR_PROJECT_ROOT"] = projectRoot

	// Set WINDSOR_EXEC_MODE to "container" if the OS is Darwin
	if goos() == "darwin" {
		envVars["WINDSOR_EXEC_MODE"] = "container"
	}

	return envVars, nil
}

// Ensure WindsorEnvPrinter implements the EnvPrinter interface
var _ EnvPrinter = (*WindsorEnvPrinter)(nil)
