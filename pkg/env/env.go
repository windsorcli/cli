package env

import (
	"fmt"
	"sync"

	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
)

// managedEnv tracks all environment variables set by the system
var (
	managedEnv   = make(map[string]string)
	managedEnvMu sync.RWMutex
)

// trackEnvVars adds environment variables to the managed environment tracking
func trackEnvVars(envVars map[string]string) {
	if envVars == nil || len(envVars) == 0 {
		return
	}

	managedEnvMu.Lock()
	defer managedEnvMu.Unlock()

	for k, v := range envVars {
		managedEnv[k] = v
	}
}

// ClearManagedEnv clears all managed environment variables
func ClearManagedEnv() {
	managedEnvMu.Lock()
	defer managedEnvMu.Unlock()

	// Clear the map by recreating it
	managedEnv = make(map[string]string)
}

// EnvPrinter defines the method for printing environment variables.
type EnvPrinter interface {
	Initialize() error
	Print(customVars ...map[string]string) error
	GetEnvVars() (map[string]string, error)
	GetAlias() (map[string]string, error)
	PostEnvHook() error
}

// Env is a struct that implements the EnvPrinter interface.
type BaseEnvPrinter struct {
	injector      di.Injector
	shell         shell.Shell
	configHandler config.ConfigHandler
}

// NewBaseEnvPrinter creates a new BaseEnvPrinter instance.
func NewBaseEnvPrinter(injector di.Injector) *BaseEnvPrinter {
	return &BaseEnvPrinter{injector: injector}
}

// Initialize resolves and assigns the shell and configHandler from the injector.
func (e *BaseEnvPrinter) Initialize() error {
	shell, ok := e.injector.Resolve("shell").(shell.Shell)
	if !ok {
		return fmt.Errorf("error resolving or casting shell to shell.Shell")
	}
	e.shell = shell

	configInterface, ok := e.injector.Resolve("configHandler").(config.ConfigHandler)
	if !ok {
		return fmt.Errorf("error resolving or casting configHandler to config.ConfigHandler")
	}
	e.configHandler = configInterface

	return nil
}

// Print outputs the environment variables to the console.
// If a map of key:value strings is provided, it prints those instead.
func (e *BaseEnvPrinter) Print(customVars ...map[string]string) error {
	var envVars map[string]string

	if len(customVars) > 0 {
		envVars = customVars[0]
	} else {
		envVars = make(map[string]string)
	}

	// Track the environment variables being printed
	trackEnvVars(envVars)

	return e.shell.PrintEnvVars(envVars)
}

// GetEnvVars is a placeholder for retrieving environment variables.
func (e *BaseEnvPrinter) GetEnvVars() (map[string]string, error) {
	// Placeholder implementation
	return map[string]string{}, nil
}

// GetAlias is a placeholder for creating an alias for a command.
func (e *BaseEnvPrinter) GetAlias() (map[string]string, error) {
	// Placeholder implementation
	return map[string]string{}, nil
}

// PostEnvHook simulates running any necessary commands after the environment variables have been set.
func (e *BaseEnvPrinter) PostEnvHook() error {
	// Placeholder for post-processing logic
	return nil
}
