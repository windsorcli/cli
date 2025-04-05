package env

import (
	"fmt"
	"os"
	"slices"
	"strings"
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

// managedAlias tracks all aliases set by the system
var (
	managedAlias   = make(map[string]string)
	managedAliasMu sync.RWMutex
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

// trackAliases adds aliases to the managed alias tracking
func trackAliases(aliases map[string]string) {
	if aliases == nil || len(aliases) == 0 {
		return
	}

	managedAliasMu.Lock()
	defer managedAliasMu.Unlock()

	for k, v := range aliases {
		managedAlias[k] = v
	}
}

// EnvPrinter defines the method for printing environment variables.
type EnvPrinter interface {
	Initialize() error
	Print(customVars ...map[string]string) error
	GetEnvVars() (map[string]string, error)
	GetAlias() (map[string]string, error)
	PrintAlias(customAliases ...map[string]string) error
	PostEnvHook() error
	Clear() error
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

// PrintAlias outputs the aliases to the console.
// If a map of key:value strings is provided, it prints those and returns.
func (e *BaseEnvPrinter) PrintAlias(customAliases ...map[string]string) error {
	if len(customAliases) > 0 {
		aliases := customAliases[0]
		trackAliases(aliases)
		return e.shell.PrintAlias(aliases)
	}

	aliases, _ := e.GetAlias()
	trackAliases(aliases)

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

// PostEnvHook simulates running any necessary commands after the environment variables have been set.
func (e *BaseEnvPrinter) PostEnvHook() error {
	// Placeholder for post-processing logic
	return nil
}

// Clear unsets all tracked environment variables and aliases
func (e *BaseEnvPrinter) Clear() error {
	// Get tracked environment variables from WINDSOR_MANAGED_ENV
	envList := os.Getenv("WINDSOR_MANAGED_ENV")
	envVars := []string{"WINDSOR_MANAGED_ENV"} // Always include the tracking variable itself
	if envList != "" {
		vars := strings.Split(envList, ":")
		for _, v := range vars {
			if v != "" && !contains(envVars, v) {
				envVars = append(envVars, v)
			}
		}
	}

	// Add all tracked environment variables from our internal tracking
	managedEnvMu.RLock()
	for key := range managedEnv {
		if !contains(envVars, key) {
			envVars = append(envVars, key)
		}
	}
	managedEnvMu.RUnlock()

	// Unset the environment variables
	if err := e.shell.UnsetEnv(envVars); err != nil {
		return fmt.Errorf("failed to unset environment variables: %w", err)
	}

	// Get tracked aliases from WINDSOR_MANAGED_ALIAS
	aliasList := os.Getenv("WINDSOR_MANAGED_ALIAS")
	aliases := []string{"WINDSOR_MANAGED_ALIAS"} // Always include the tracking variable itself
	if aliasList != "" {
		als := strings.Split(aliasList, ":")
		for _, a := range als {
			if a != "" && !contains(aliases, a) {
				aliases = append(aliases, a)
			}
		}
	}

	// Add all tracked aliases from our internal tracking
	managedAliasMu.RLock()
	for key := range managedAlias {
		if !contains(aliases, key) {
			aliases = append(aliases, key)
		}
	}
	managedAliasMu.RUnlock()

	// Unset the aliases
	if err := e.shell.UnsetAlias(aliases); err != nil {
		return fmt.Errorf("failed to unset aliases: %w", err)
	}

	// Reset tracking maps
	managedEnvMu.Lock()
	managedEnv = make(map[string]string)
	managedEnvMu.Unlock()

	managedAliasMu.Lock()
	managedAlias = make(map[string]string)
	managedAliasMu.Unlock()

	return nil
}

// contains checks if a string is in a slice
func contains(slice []string, item string) bool {
	return slices.Contains(slice, item)
}
