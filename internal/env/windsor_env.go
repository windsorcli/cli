package env

import (
	"fmt"

	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/shell"
)

// WindsorEnvPrinter is a struct that simulates a Kubernetes environment for testing purposes.
type WindsorEnvPrinter struct {
	BaseEnvPrinter
}

// NewWindsorEnvPrinter initializes a new WindsorEnvPrinter instance using the provided dependency injector.
func NewWindsorEnvPrinter(injector di.Injector) *WindsorEnvPrinter {
	return &WindsorEnvPrinter{
		BaseEnvPrinter: BaseEnvPrinter{
			injector: injector,
		},
	}
}

// GetEnvVars retrieves the environment variables for the Windsor environment.
func (e *WindsorEnvPrinter) GetEnvVars() (map[string]string, error) {
	envVars := make(map[string]string)

	// Resolve necessary dependencies for context and shell operations.
	contextHandler, err := e.injector.Resolve("contextHandler")
	if err != nil {
		return nil, fmt.Errorf("error resolving contextHandler: %w", err)
	}
	context, ok := contextHandler.(context.ContextInterface)
	if !ok {
		return nil, fmt.Errorf("failed to cast contextHandler to context.ContextInterface")
	}

	shellInstance, err := e.injector.Resolve("shell")
	if err != nil {
		return nil, fmt.Errorf("error resolving shell: %w", err)
	}
	shell, ok := shellInstance.(shell.Shell)
	if !ok {
		return nil, fmt.Errorf("failed to cast shell to shell.Shell")
	}

	// Add WINDSOR_CONTEXT to the environment variables
	currentContext, err := context.GetContext()
	if err != nil {
		return nil, fmt.Errorf("error retrieving current context: %w", err)
	}
	envVars["WINDSOR_CONTEXT"] = currentContext

	// Get the project root and add WINDSOR_PROJECT_ROOT to the environment variables
	projectRoot, err := shell.GetProjectRoot()
	if err != nil {
		return nil, fmt.Errorf("error retrieving project root: %w", err)
	}
	envVars["WINDSOR_PROJECT_ROOT"] = projectRoot

	return envVars, nil
}

// Print prints the environment variables for the Windsor environment.
func (e *WindsorEnvPrinter) Print() error {
	envVars, err := e.GetEnvVars()
	if err != nil {
		// Return the error if GetEnvVars fails
		return fmt.Errorf("error getting environment variables: %w", err)
	}
	// Call the Print method of the embedded BaseEnvPrinter struct with the retrieved environment variables
	return e.BaseEnvPrinter.Print(envVars)
}

// Ensure WindsorEnvPrinter implements the EnvPrinter interface
var _ EnvPrinter = (*WindsorEnvPrinter)(nil)
