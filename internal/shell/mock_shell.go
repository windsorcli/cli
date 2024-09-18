package shell

import (
	"errors"
	"fmt"
	"sort"
)

type MockShell struct {
	ShellType          string
	PrintEnvVarsFn     func(envVars map[string]string)
	GetProjectRootFunc func() (string, error)
}

func NewMockShell(shellType string) (*MockShell, error) {
	if shellType != "cmd" && shellType != "powershell" && shellType != "unix" {
		return nil, errors.New("invalid shell type")
	}
	return &MockShell{ShellType: shellType}, nil
}

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

func (m *MockShell) GetProjectRoot() (string, error) {
	if m.GetProjectRootFunc != nil {
		return m.GetProjectRootFunc()
	}
	return "", errors.New("GetProjectRootFunc not implemented")
}
