package shell

import (
	"github.com/windsor-hotel/cli/internal/di"
)

// MockShell is a struct that simulates a shell environment for testing purposes.
type MockShell struct {
	Shell
	PrintEnvVarsFunc   func(envVars map[string]string) error
	PrintAliasFunc     func(envVars map[string]string) error
	GetProjectRootFunc func() (string, error)
	ExecFunc           func(verbose bool, message string, command string, args ...string) (string, error)
	Injector           di.Injector
}

// NewMockShell creates a new instance of MockShell. If injector is provided, it sets the injector on MockShell.
func NewMockShell(injector ...di.Injector) *MockShell {
	var diInjector di.Injector
	if len(injector) > 0 {
		diInjector = injector[0]
	}
	return &MockShell{
		Injector: diInjector,
	}
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
func (s *MockShell) Exec(verbose bool, message string, command string, args ...string) (string, error) {
	if s.ExecFunc != nil {
		return s.ExecFunc(verbose, message, command, args...)
	}
	return "", nil
}

// Ensure MockShell implements the Shell interface
var _ Shell = (*MockShell)(nil)
