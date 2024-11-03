package shell

import (
	"fmt"
	"sort"

	"github.com/windsor-hotel/cli/internal/di"
)

// MockShell is a struct that simulates a shell environment for testing purposes.
type MockShell struct {
	PrintEnvVarsFunc   func(envVars map[string]string)
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

// PrintEnvVars prints the environment variables in a sorted order.
// If a custom PrintEnvVarsFn is provided, it will use that function instead.
func (m *MockShell) PrintEnvVars(envVars map[string]string) {
	keys := make([]string, 0, len(envVars))
	for k := range envVars {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		fmt.Printf("%s=%s\n", k, envVars[k])
	}
}

// GetProjectRoot returns the project root directory.
// If a custom GetProjectRootFunc is provided, it will use that function instead.
func (m *MockShell) GetProjectRoot() (string, error) {
	if m.GetProjectRootFunc != nil {
		return m.GetProjectRootFunc()
	}
	return "", fmt.Errorf("GetProjectRootFunc not implemented")
}

// Exec executes a command with optional privilege elevation
func (m *MockShell) Exec(verbose bool, message string, command string, args ...string) (string, error) {
	if m.ExecFunc != nil {
		return m.ExecFunc(verbose, message, command, args...)
	}
	return "", fmt.Errorf("ExecFunc not implemented")
}

// Ensure MockShell implements the Shell interface
var _ Shell = (*MockShell)(nil)
