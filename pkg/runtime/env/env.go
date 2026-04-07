// The EnvPrinter is a core component that manages environment variable state and context.
// It provides a unified interface for loading, printing, and managing environment variables,
// The EnvPrinter acts as the central environment orchestrator for the application,
// coordinating environment variable management, shell integration, and configuration persistence.

package env

import (
	"slices"

	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// =============================================================================
// Types
// =============================================================================

// EnvPrinter defines the method for printing environment variables.
type EnvPrinter interface {
	GetEnvVars() (map[string]string, error)
	GetAlias() (map[string]string, error)
	PostEnvHook(directory ...string) error
	GetManagedEnv() []string
	GetManagedAlias() []string
	SetManagedEnv(env string)
	SetManagedAlias(alias string)
	Reset()
}

// BaseEnvPrinter is a base implementation of the EnvPrinter interface
type BaseEnvPrinter struct {
	EnvPrinter
	shell         shell.Shell
	configHandler config.ConfigHandler
	shims         *Shims
	managedEnv    []string
	managedAlias  []string
}

// =============================================================================
// Constructor
// =============================================================================

// NewBaseEnvPrinter creates a new BaseEnvPrinter instance
func NewBaseEnvPrinter(shell shell.Shell, configHandler config.ConfigHandler) *BaseEnvPrinter {
	if shell == nil {
		panic("shell is required")
	}
	if configHandler == nil {
		panic("config handler is required")
	}

	return &BaseEnvPrinter{
		shell:         shell,
		configHandler: configHandler,
		shims:         NewShims(),
	}
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
func (e *BaseEnvPrinter) PostEnvHook(directory ...string) error {
	// Placeholder for post-processing logic
	return nil
}

// GetManagedEnv returns the environment variables that are managed by Windsor.
func (e *BaseEnvPrinter) GetManagedEnv() []string {
	return e.managedEnv
}

// GetManagedAlias returns the shell aliases that are managed by Windsor.
func (e *BaseEnvPrinter) GetManagedAlias() []string {
	return e.managedAlias
}

// SetManagedEnv sets the environment variables that are managed by Windsor.
func (e *BaseEnvPrinter) SetManagedEnv(env string) {
	if slices.Contains(e.managedEnv, env) {
		return
	}
	e.managedEnv = append(e.managedEnv, env)
}

// SetManagedAlias sets the shell aliases that are managed by Windsor.
func (e *BaseEnvPrinter) SetManagedAlias(alias string) {
	if slices.Contains(e.managedAlias, alias) {
		return
	}
	e.managedAlias = append(e.managedAlias, alias)
}

// Reset removes all managed environment variables and aliases.
// It delegates to the shell's Reset method to handle the reset logic.
func (e *BaseEnvPrinter) Reset() {
	e.managedEnv = make([]string, 0)
	e.managedAlias = make([]string, 0)
	e.shell.Reset()
}
