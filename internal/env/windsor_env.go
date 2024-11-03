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
func (e *WindsorEnv) Print(envVars map[string]string) {
	// Resolve necessary dependencies for context and shell operations.
	contextHandler, err := e.diContainer.Resolve("contextHandler")
	if err != nil {
		fmt.Printf("Error resolving contextHandler: %v\n", err)
		return
	}
	context, ok := contextHandler.(context.ContextInterface)
	if !ok {
		fmt.Println("Failed to cast contextHandler to context.ContextInterface")
		return
	}

	shellInstance, err := e.diContainer.Resolve("shell")
	if err != nil {
		fmt.Printf("Error resolving shell: %v\n", err)
		return
	}
	shell, ok := shellInstance.(shell.Shell)
	if !ok {
		fmt.Println("Failed to cast shell to shell.Shell")
		return
	}

	// Add WINDSOR_CONTEXT to the environment variables
	currentContext, err := context.GetContext()
	if err != nil {
		fmt.Printf("Error retrieving current context: %v\n", err)
		return
	}
	envVars["WINDSOR_CONTEXT"] = currentContext

	// Get the project root and add WINDSOR_PROJECT_ROOT to the environment variables
	projectRoot, err := shell.GetProjectRoot()
	if err != nil {
		fmt.Printf("Error retrieving project root: %v\n", err)
		return
	}
	envVars["WINDSOR_PROJECT_ROOT"] = projectRoot

	// Display the environment variables using the Shell's PrintEnvVars method.
	shell.PrintEnvVars(envVars)
}

// Ensure WindsorEnv implements the EnvInterface
var _ EnvInterface = (*WindsorEnv)(nil)
