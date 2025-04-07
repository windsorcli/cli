package env

import (
	"fmt"
	"slices"

	"sync"

	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
)

// These are the environment variables that are managed by Windsor.
// They are scoped to the current shell session.
var (
	windsorManagedEnv   = []string{}
	windsorManagedAlias = []string{}
	windsorManagedMu    sync.Mutex
)

// EnvPrinter defines the method for printing environment variables.
type EnvPrinter interface {
	Initialize() error
	Print() error
	GetEnvVars() (map[string]string, error)
	GetAlias() (map[string]string, error)
	PostEnvHook() error
	GetManagedEnv() []string
	GetManagedAlias() []string
	SetManagedEnv(env string)
	SetManagedAlias(alias string)
	Reset()
}

// Env is a struct that implements the EnvPrinter interface.
type BaseEnvPrinter struct {
	EnvPrinter
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

	for key := range envVars {
		e.SetManagedEnv(key)
	}

	e.shell.PrintEnvVars(envVars)
	return nil
}

// GetEnvVars is a placeholder for retrieving environment variables.
func (e *BaseEnvPrinter) GetEnvVars() (map[string]string, error) {
	// Placeholder implementation
	return map[string]string{}, nil
}

// PrintAlias retrieves and prints the shell alias.
// If a map of key:value strings is provided, it prints those instead.
func (e *BaseEnvPrinter) PrintAlias(customAlias ...map[string]string) error {
	var aliasMap map[string]string

	if len(customAlias) > 0 {
		aliasMap = customAlias[0]
	} else {
		var err error
		aliasMap, err = e.GetAlias()
		if err != nil {
			// Can't test as it just calls a stub
			return fmt.Errorf("error getting alias: %w", err)
		}
	}

	for key := range aliasMap {
		e.SetManagedAlias(key)
	}

	e.shell.PrintAlias(aliasMap)
	return nil
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

// GetManagedEnv returns the environment variables that are managed by Windsor.
func (e *BaseEnvPrinter) GetManagedEnv() []string {
	windsorManagedMu.Lock()
	defer windsorManagedMu.Unlock()
	return windsorManagedEnv
}

// GetManagedAlias returns the shell aliases that are managed by Windsor.
func (e *BaseEnvPrinter) GetManagedAlias() []string {
	windsorManagedMu.Lock()
	defer windsorManagedMu.Unlock()
	return windsorManagedAlias
}

// SetManagedEnv sets the environment variables that are managed by Windsor.
func (e *BaseEnvPrinter) SetManagedEnv(env string) {
	windsorManagedMu.Lock()
	defer windsorManagedMu.Unlock()
	if slices.Contains(windsorManagedEnv, env) {
		return
	}
	windsorManagedEnv = append(windsorManagedEnv, env)
}

// SetManagedAlias sets the shell aliases that are managed by Windsor.
func (e *BaseEnvPrinter) SetManagedAlias(alias string) {
	windsorManagedMu.Lock()
	defer windsorManagedMu.Unlock()
	if slices.Contains(windsorManagedAlias, alias) {
		return
	}
	windsorManagedAlias = append(windsorManagedAlias, alias)
}

// Reset removes all managed environment variables and aliases.
// It delegates to the shell's Reset method to handle the reset logic.
func (e *BaseEnvPrinter) Reset() {
	e.shell.Reset()
}
