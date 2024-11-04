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
	container          di.ContainerInterface
}

// NewMockShell creates a new instance of MockShell. If a di container is provided, it sets the container on MockShell.
func NewMockShell(container ...di.ContainerInterface) *MockShell {
	var diContainer di.ContainerInterface
	if len(container) > 0 {
		diContainer = container[0]
	}
	return &MockShell{
		container: diContainer,
	}
}

// PrintEnvVars calls the custom PrintEnvVarsFunc if provided.
func (m *MockShell) PrintEnvVars(envVars map[string]string) error {
	if m.PrintEnvVarsFunc != nil {
		return m.PrintEnvVarsFunc(envVars)
	}
	return nil
}

// PrintAlias calls the custom PrintAliasFunc if provided.
func (m *MockShell) PrintAlias(envVars map[string]string) error {
	if m.PrintAliasFunc != nil {
		return m.PrintAliasFunc(envVars)
	}
	return nil
}

// GetProjectRoot calls the custom GetProjectRootFunc if provided.
func (m *MockShell) GetProjectRoot() (string, error) {
	if m.GetProjectRootFunc != nil {
		return m.GetProjectRootFunc()
	}
	return "", nil
}

// Exec calls the custom ExecFunc if provided.
func (m *MockShell) Exec(verbose bool, message string, command string, args ...string) (string, error) {
	if m.ExecFunc != nil {
		return m.ExecFunc(verbose, message, command, args...)
	}
	return "", nil
}

// Ensure MockShell implements the Shell interface
var _ Shell = (*MockShell)(nil)
