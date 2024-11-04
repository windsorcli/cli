package env

import (
	"fmt"

	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/shell"
)

// WindsorEnv is a struct that simulates a Kubernetes environment for testing purposes.
type WindsorEnv struct {
	EnvInterface
	diContainer di.ContainerInterface
}

// NewWindsorEnv initializes a new WindsorEnv instance using the provided dependency injection container.
func NewWindsorEnv(diContainer di.ContainerInterface) *WindsorEnv {
	return &WindsorEnv{
		diContainer: diContainer,
	}
}

// Print displays the provided environment variables to the console.
func (e *WindsorEnv) Print(envVars map[string]string) error {
	// Resolve necessary dependencies for context and shell operations.
	contextHandler, err := e.diContainer.Resolve("contextHandler")
	if err != nil {
		return fmt.Errorf("error resolving contextHandler: %w", err)
	}
	context, ok := contextHandler.(context.ContextInterface)
	if !ok {
		return fmt.Errorf("failed to cast contextHandler to context.ContextInterface")
	}

	shellInstance, err := e.diContainer.Resolve("shell")
	if err != nil {
		return fmt.Errorf("error resolving shell: %w", err)
	}
	shell, ok := shellInstance.(shell.Shell)
	if !ok {
		return fmt.Errorf("failed to cast shell to shell.Shell")
	}

	// Add WINDSOR_CONTEXT to the environment variables
	currentContext, err := context.GetContext()
	if err != nil {
		return fmt.Errorf("error retrieving current context: %w", err)
	}
	envVars["WINDSOR_CONTEXT"] = currentContext

	// Get the project root and add WINDSOR_PROJECT_ROOT to the environment variables
	projectRoot, err := shell.GetProjectRoot()
	if err != nil {
		return fmt.Errorf("error retrieving project root: %w", err)
	}
	envVars["WINDSOR_PROJECT_ROOT"] = projectRoot

	// Display the environment variables using the Shell's PrintEnvVars method.
	return shell.PrintEnvVars(envVars)
}

// Ensure WindsorEnv implements the EnvInterface
var _ EnvInterface = (*WindsorEnv)(nil)
