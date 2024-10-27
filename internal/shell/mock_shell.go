package shell

import (
	"errors"
	"fmt"
	"sort"
)

// MockShell is a struct that simulates a shell environment for testing purposes.
type MockShell struct {
	ShellType          string
	PrintEnvVarsFunc   func(envVars map[string]string)
	GetProjectRootFunc func() (string, error)
	ExecFunc           func(verbose bool, message string, command string, args ...string) (string, error)
}

// NewMockShell creates a new instance of MockShell based on the provided shell type.
// Returns an error if the shell type is invalid.
func NewMockShell(shellType string) (*MockShell, error) {
	if shellType != "cmd" && shellType != "powershell" && shellType != "unix" {
		return nil, errors.New("invalid shell type")
	}
	return &MockShell{ShellType: shellType}, nil
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
	return "", errors.New("GetProjectRootFunc not implemented")
}

// Exec executes a command with optional privilege elevation
func (m *MockShell) Exec(verbose bool, message string, command string, args ...string) (string, error) {
	if m.ExecFunc != nil {
		return m.ExecFunc(verbose, message, command, args...)
	}
	return "", errors.New("ExecFunc not implemented")
}

// Ensure MockShell implements the Shell interface
var _ Shell = (*MockShell)(nil)
