package env

import (
	"github.com/windsorcli/cli/pkg/di"
)

// CustomEnvPrinter is a struct that implements the EnvPrinter interface and handles custom environment variables.
type CustomEnvPrinter struct {
	BaseEnvPrinter
}

// NewCustomEnvPrinter initializes a new CustomEnvPrinter instance using the provided dependency injector.
func NewCustomEnvPrinter(injector di.Injector) *CustomEnvPrinter {
	return &CustomEnvPrinter{
		BaseEnvPrinter: BaseEnvPrinter{
			injector: injector,
		},
	}
}

// Print outputs the environment variables to the console.
func (e *CustomEnvPrinter) Print() error {
	envVars, _ := e.GetEnvVars()

	return e.shell.PrintEnvVars(envVars)
}

// GetEnvVars retrieves environment variables from the context config.
func (e *CustomEnvPrinter) GetEnvVars() (map[string]string, error) {
	envVars := e.configHandler.GetStringMap("environment")
	if envVars == nil {
		envVars = make(map[string]string)
	}
	return envVars, nil
}

// Ensure customEnv implements the EnvPrinter interface
var _ EnvPrinter = (*CustomEnvPrinter)(nil)
