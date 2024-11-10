package env

import (
	"fmt"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
	"github.com/windsor-hotel/cli/internal/shell"
)

// EnvPrinter defines the method for printing environment variables.
type EnvPrinter interface {
	Initialize() error
	Print() error
	GetEnvVars() (map[string]string, error)
	PostEnvHook() error
}

// Env is a struct that implements the EnvPrinter interface.
type BaseEnvPrinter struct {
	injector       di.Injector
	contextHandler context.ContextInterface
	shell          shell.Shell
	configHandler  config.ConfigHandler
}

// NewBaseEnvPrinter creates a new BaseEnvPrinter instance.
func NewBaseEnvPrinter(injector di.Injector) *BaseEnvPrinter {
	return &BaseEnvPrinter{injector: injector}
}

// Initialize initializes the environment.
func (e *BaseEnvPrinter) Initialize() error {
	// Resolve the contextHandler
	contextHandler, err := e.injector.Resolve("contextHandler")
	if err != nil {
		return fmt.Errorf("error resolving contextHandler: %w", err)
	}
	context, ok := contextHandler.(context.ContextInterface)
	if !ok {
		return fmt.Errorf("failed to cast contextHandler to context.ContextInterface")
	}
	e.contextHandler = context

	// Resolve the shell
	shellInstance, err := e.injector.Resolve("shell")
	if err != nil {
		return fmt.Errorf("error resolving shell: %w", err)
	}
	shell, ok := shellInstance.(shell.Shell)
	if !ok {
		return fmt.Errorf("shell is not of type Shell")
	}
	e.shell = shell

	// Resolve the configHandler
	configHandler, err := e.injector.Resolve("configHandler")
	if err != nil {
		return fmt.Errorf("error resolving configHandler: %w", err)
	}
	configInterface, ok := configHandler.(config.ConfigHandler)
	if !ok {
		return fmt.Errorf("configHandler is not of type ConfigHandler")
	}
	e.configHandler = configInterface
	return nil
}

// Print prints the environment variables to the console.
// It can optionally take a map of key:value strings and prints those.
func (e *BaseEnvPrinter) Print(customVars ...map[string]string) error {
	var envVars map[string]string

	// Use only the passed vars
	if len(customVars) > 0 {
		envVars = customVars[0]
	} else {
		envVars = make(map[string]string)
	}

	return e.shell.PrintEnvVars(envVars)
}

// GetEnvVars is a placeholder for retrieving environment variables.
func (e *BaseEnvPrinter) GetEnvVars() (map[string]string, error) {
	// Placeholder implementation
	return map[string]string{}, nil
}

// PostEnvHook simulates running any necessary commands after the environment variables have been set.
func (e *BaseEnvPrinter) PostEnvHook() error {
	// Placeholder for post-processing logic
	return nil
}
