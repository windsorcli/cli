package shell

import (
	"github.com/windsorcli/cli/pkg/di"
)

// MockShell is a struct that simulates a shell environment for testing purposes.
type MockShell struct {
	DefaultShell
	InitializeFunc                 func() error
	PrintEnvVarsFunc               func(envVars map[string]string) error
	PrintAliasFunc                 func(envVars map[string]string) error
	GetProjectRootFunc             func() (string, error)
	ExecFunc                       func(command string, args ...string) (string, int, error)
	ExecSilentFunc                 func(command string, args ...string) (string, int, error)
	ExecProgressFunc               func(message string, command string, args ...string) (string, int, error)
	ExecSudoFunc                   func(message string, command string, args ...string) (string, int, error)
	InstallHookFunc                func(shellName string) error
	SetVerbosityFunc               func(verbose bool)
	AddCurrentDirToTrustedFileFunc func() error
	CheckTrustedDirectoryFunc      func() error
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
func (s *MockShell) Exec(command string, args ...string) (string, int, error) {
	if s.ExecFunc != nil {
		output, exitCode, err := s.ExecFunc(command, args...)
		return output, exitCode, err
	}
	return "", 0, nil
}

// ExecSilent calls the custom ExecSilentFunc if provided.
func (s *MockShell) ExecSilent(command string, args ...string) (string, int, error) {
	if s.ExecSilentFunc != nil {
		return s.ExecSilentFunc(command, args...)
	}
	return "", 0, nil
}

// ExecProgress calls the custom ExecProgressFunc if provided.
func (s *MockShell) ExecProgress(message string, command string, args ...string) (string, int, error) {
	if s.ExecProgressFunc != nil {
		return s.ExecProgressFunc(message, command, args...)
	}
	return "", 0, nil
}

// ExecSudo calls the custom ExecSudoFunc if provided.
func (s *MockShell) ExecSudo(message string, command string, args ...string) (string, int, error) {
	if s.ExecSudoFunc != nil {
		return s.ExecSudoFunc(message, command, args...)
	}
	return "", 0, nil
}

// InstallHook calls the custom InstallHook if provided.
func (s *MockShell) InstallHook(shellName string) error {
	if s.InstallHookFunc != nil {
		return s.InstallHookFunc(shellName)
	}
	return nil
}

// SetVerbosity calls the custom SetVerbosityFunc if provided.
func (s *MockShell) SetVerbosity(verbose bool) {
	if s.SetVerbosityFunc != nil {
		s.SetVerbosityFunc(verbose)
	}
}

// AddCurrentDirToTrustedFile calls the custom AddCurrentDirToTrustedFileFunc if provided.
func (s *MockShell) AddCurrentDirToTrustedFile() error {
	if s.AddCurrentDirToTrustedFileFunc != nil {
		return s.AddCurrentDirToTrustedFileFunc()
	}
	return nil
}

// CheckTrustedDirectory calls the custom CheckTrustedDirectoryFunc if provided.
func (s *MockShell) CheckTrustedDirectory() error {
	if s.CheckTrustedDirectoryFunc != nil {
		return s.CheckTrustedDirectoryFunc()
	}
	return nil
}

// Ensure MockShell implements the Shell interface
var _ Shell = (*MockShell)(nil)
