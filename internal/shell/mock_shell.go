package shell

import (
	"github.com/windsorcli/cli/internal/di"
)

// MockShell is a struct that simulates a shell environment for testing purposes.
type MockShell struct {
	DefaultShell
	InitializeFunc     func() error
	PrintEnvVarsFunc   func(envVars map[string]string) error
	PrintAliasFunc     func(envVars map[string]string) error
	GetProjectRootFunc func() (string, error)
	ExecFunc           func(message string, command string, args ...string) (string, error)
}

// NewMockShell creates a new instance of MockShell. If injector is provided, it sets the injector on MockShell.
func NewMockShell(injectors ...di.Injector) *MockShell {
	var injector di.Injector
	if len(injectors) > 0 {
		injector = injectors[0]
	}
	return &MockShell{
		DefaultShell: DefaultShell{
			injector: injector,
		},
	}
}

// Initialize calls the custom InitializeFunc if provided.
func (s *MockShell) Initialize() error {
	if s.InitializeFunc != nil {
		return s.InitializeFunc()
	}
	return nil
}

// PrintEnvVars calls the custom PrintEnvVarsFunc if provided.
func (s *MockShell) PrintEnvVars(envVars map[string]string) error {
	if s.PrintEnvVarsFunc != nil {
		return s.PrintEnvVarsFunc(envVars)
	}
	return nil
}

// PrintAlias calls the custom PrintAliasFunc if provided.
func (s *MockShell) PrintAlias(envVars map[string]string) error {
	if s.PrintAliasFunc != nil {
		return s.PrintAliasFunc(envVars)
	}
	return nil
}

// GetProjectRoot calls the custom GetProjectRootFunc if provided.
func (s *MockShell) GetProjectRoot() (string, error) {
	if s.GetProjectRootFunc != nil {
		return s.GetProjectRootFunc()
	}
	return "", nil
}

// Exec calls the custom ExecFunc if provided.
func (s *MockShell) Exec(message string, command string, args ...string) (string, error) {
	if s.ExecFunc != nil {
		return s.ExecFunc(message, command, args...)
	}
	return "", nil
}

// Ensure MockShell implements the Shell interface
var _ Shell = (*MockShell)(nil)
