package env

import (
	"fmt"
	"maps"
	"os"
	"strings"
	"sync"

	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
)

// EnvPrinter defines the method for printing environment variables and aliases.
type EnvPrinter interface {
	Initialize() error
	Print() error
	GetEnvVars() (map[string]string, error)
	GetAlias() (map[string]string, error)
	UnsetEnvVars() (map[string]string, error)
	PostEnvHook() error
}

// Global map to keep track of all printed environment variables
var printedEnvVars = make(map[string]string)
var mu sync.Mutex

// Env is a struct that implements the EnvPrinter interface.
type BaseEnvPrinter struct {
	injector      di.Injector
	shell         shell.Shell
	configHandler config.ConfigHandler
	envPrinter    EnvPrinter
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

// Print outputs the environment variables and aliases to the console and updates the global state.
func (e *BaseEnvPrinter) Print() error {
	if e.envPrinter == nil {
		return fmt.Errorf("error: EnvPrinter is not set in BaseEnvPrinter")
	}

	envVars, err := e.envPrinter.GetEnvVars()
	if err != nil {
		return fmt.Errorf("error getting environment variables: %w", err)
	}

	// Update the global map with the printed environment variables
	mu.Lock()
	maps.Copy(printedEnvVars, envVars)
	mu.Unlock()

	if err := e.shell.PrintEnvVars(envVars); err != nil {
		return fmt.Errorf("error printing environment variables: %w", err)
	}

	aliases, err := e.envPrinter.GetAlias()
	if err != nil {
		return fmt.Errorf("error getting aliases: %w", err)
	}

	return e.shell.PrintAlias(aliases)
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

// UnsetEnvVars uses the shell to print environment variables with empty values, triggering an unset
func (e *BaseEnvPrinter) UnsetEnvVars() (map[string]string, error) {
	managedEnvVars := os.Getenv("WINDSOR_MANAGED_ENV")
	if managedEnvVars == "" {
		return map[string]string{}, nil
	}

	envVarKeys := strings.Split(managedEnvVars, ",")
	envVars := make(map[string]string, len(envVarKeys))

	// Set all managed environment variables to empty strings
	for _, key := range envVarKeys {
		envVars[key] = ""
	}

	if err := e.shell.PrintEnvVars(envVars); err != nil {
		return nil, fmt.Errorf("error unsetting environment variables: %w", err)
	}

	return envVars, nil
}

// PostEnvHook simulates running any necessary commands after the environment variables have been set.
func (e *BaseEnvPrinter) PostEnvHook() error {
	// Placeholder for post-processing logic
	return nil
}
