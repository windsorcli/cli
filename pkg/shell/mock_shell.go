package shell

import (
	"fmt"

	"github.com/windsorcli/cli/pkg/di"
)

// MockShell is a struct that simulates a shell environment for testing purposes.
type MockShell struct {
	DefaultShell
	InitializeFunc                 func() error
	PrintEnvVarsFunc               func(envVars map[string]string)
	PrintAliasFunc                 func(envVars map[string]string)
	GetProjectRootFunc             func() (string, error)
	ExecFunc                       func(command string, args ...string) (string, error)
	ExecSilentFunc                 func(command string, args ...string) (string, error)
	ExecProgressFunc               func(message string, command string, args ...string) (string, error)
	ExecSudoFunc                   func(message string, command string, args ...string) (string, error)
	InstallHookFunc                func(shellName string) error
	SetVerbosityFunc               func(verbose bool)
	AddCurrentDirToTrustedFileFunc func() error
	CheckTrustedDirectoryFunc      func() error
	UnsetEnvsFunc                  func(envVars []string)
	UnsetAliasFunc                 func(aliases []string)
	WriteResetTokenFunc            func() (string, error)
	GetSessionTokenFunc            func() (string, error)
	CheckResetFlagsFunc            func() (bool, error)
	ResetFunc                      func()
}

// NewMockShell creates a new instance of MockShell. If injector is provided, it sets the injector on MockShell.
func NewMockShell(injectors ...di.Injector) *MockShell {
	var injector di.Injector
	if len(injectors) > 0 {
		injector = injectors[0]
	}

	mockShell := &MockShell{
		DefaultShell: DefaultShell{
			injector: injector,
		},
	}

	return mockShell
}

// Initialize calls the custom InitializeFunc if provided.
func (s *MockShell) Initialize() error {
	if s.InitializeFunc != nil {
		return s.InitializeFunc()
	}
	return nil
}

// PrintEnvVars calls the custom PrintEnvVarsFunc if provided.
func (s *MockShell) PrintEnvVars(envVars map[string]string) {
	if s.PrintEnvVarsFunc != nil {
		s.PrintEnvVarsFunc(envVars)
	}
}

// PrintAlias calls the custom PrintAliasFunc if provided.
func (s *MockShell) PrintAlias(envVars map[string]string) {
	if s.PrintAliasFunc != nil {
		s.PrintAliasFunc(envVars)
	}
}

// GetProjectRoot calls the custom GetProjectRootFunc if provided.
func (s *MockShell) GetProjectRoot() (string, error) {
	if s.GetProjectRootFunc != nil {
		return s.GetProjectRootFunc()
	}
	return "", nil
}

// Exec calls the custom ExecFunc if provided.
func (s *MockShell) Exec(command string, args ...string) (string, error) {
	if s.ExecFunc != nil {
		return s.ExecFunc(command, args...)
	}
	return "", nil
}

// ExecSilent calls the custom ExecSilentFunc if provided.
func (s *MockShell) ExecSilent(command string, args ...string) (string, error) {
	if s.ExecSilentFunc != nil {
		return s.ExecSilentFunc(command, args...)
	}
	return "", nil
}

// ExecProgress calls the custom ExecProgressFunc if provided.
func (s *MockShell) ExecProgress(message string, command string, args ...string) (string, error) {
	if s.ExecProgressFunc != nil {
		return s.ExecProgressFunc(message, command, args...)
	}
	return "", nil
}

// ExecSudo calls the custom ExecSudoFunc if provided.
func (s *MockShell) ExecSudo(message string, command string, args ...string) (string, error) {
	if s.ExecSudoFunc != nil {
		return s.ExecSudoFunc(message, command, args...)
	}
	return "", nil
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

// UnsetEnvs calls the custom UnsetEnvsFunc if provided.
func (s *MockShell) UnsetEnvs(envVars []string) {
	if s.UnsetEnvsFunc != nil {
		s.UnsetEnvsFunc(envVars)
	}
}

// UnsetAlias calls the custom UnsetAliasFunc if provided.
func (s *MockShell) UnsetAlias(aliases []string) {
	if s.UnsetAliasFunc != nil {
		s.UnsetAliasFunc(aliases)
	}
}

// WriteResetToken writes a reset token file
func (s *MockShell) WriteResetToken() (string, error) {
	if s.WriteResetTokenFunc != nil {
		return s.WriteResetTokenFunc()
	}
	return "", fmt.Errorf("WriteResetToken not implemented")
}

// GetSessionToken retrieves or generates a session token
func (s *MockShell) GetSessionToken() (string, error) {
	if s.GetSessionTokenFunc != nil {
		return s.GetSessionTokenFunc()
	}
	return "", fmt.Errorf("GetSessionToken not implemented")
}

// CheckResetFlags checks if a reset signal file exists for the current session
func (s *MockShell) CheckResetFlags() (bool, error) {
	if s.CheckResetFlagsFunc != nil {
		return s.CheckResetFlagsFunc()
	}
	return false, nil
}

// Reset calls the custom ResetFunc if provided.
func (s *MockShell) Reset() {
	if s.ResetFunc != nil {
		s.ResetFunc()
	}
}

// Ensure MockShell implements the Shell interface
var _ Shell = (*MockShell)(nil)
