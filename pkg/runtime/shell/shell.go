// The Shell is the interface for command execution and shell output in the Windsor CLI.
// It provides command execution, project root lookup, trust checks, session tokens, and env rendering.
// It removes registered secrets from all command output before it shows or returns the output.
// Platform files supply the shell syntax for Unix shells and for PowerShell.

package shell

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// =============================================================================
// Constants
// =============================================================================

// MaxFolderSearchDepth is the maximum depth to search for the project root,
// shared by GetProjectRoot and the EnsureGitRepository / EnsureProjectAnchor
// helpers so all walk-up traversals stop at the same boundary.
const MaxFolderSearchDepth = 10

// =============================================================================
// Types
// =============================================================================

// DefaultShell is the default implementation of the Shell interface
type DefaultShell struct {
	Shell
	projectRoot  string
	global       bool
	verbose      bool
	sessionToken string
	shims        *Shims
	secrets      []string
	secretsMu    sync.Mutex
}

// =============================================================================
// Interfaces
// =============================================================================

// Shell is the interface that defines shell operations.
type Shell interface {
	SetVerbosity(verbose bool)
	IsVerbose() bool
	IsGlobal() bool
	RenderEnvVars(envVars map[string]string, export bool) string
	RenderAliases(aliases map[string]string) string
	GetProjectRoot() (string, error)
	Exec(command string, args ...string) (string, error)
	ExecSilent(command string, args ...string) (string, error)
	ExecSilentWithEnv(command string, env map[string]string, args ...string) (string, error)
	ExecCaptureWithEnv(command string, env map[string]string, args ...string) (string, error)
	ExecSilentWithTimeout(command string, args []string, timeout time.Duration) (string, error)
	ExecSilentWithEnvAndTimeout(command string, env map[string]string, args []string, timeout time.Duration) (string, error)
	ExecSilentWithEnvAndIdleTimeout(command string, env map[string]string, args []string, idleTimeout, absoluteTimeout, gracePeriod time.Duration) (string, error)
	ExecSudo(message string, command string, args ...string) (string, error)
	ExecProgress(message string, command string, args ...string) (string, error)
	ExecProgressWithEnv(message string, command string, env map[string]string, args ...string) (string, error)
	InstallHook(shellName string) error
	AddCurrentDirToTrustedFile() error
	CheckTrustedDirectory() error
	UnsetEnvs(envVars []string)
	UnsetAlias(aliases []string)
	WriteResetToken() (string, error)
	GetSessionToken() (string, error)
	CheckResetFlags() (bool, error)
	Reset(quiet ...bool)
	RegisterSecret(value string)
	RegisterResolvedSecret(value string)
}

// =============================================================================
// Constructor
// =============================================================================

// NewDefaultShell creates a new instance of DefaultShell
func NewDefaultShell() *DefaultShell {
	return &DefaultShell{
		shims:   NewShims(),
		verbose: false,
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// SetVerbosity sets the verbosity flag
func (s *DefaultShell) SetVerbosity(verbose bool) {
	s.verbose = verbose
}

// IsVerbose returns the current verbosity flag value.
func (s *DefaultShell) IsVerbose() bool {
	return s.verbose
}

// IsGlobal reports whether the shell is operating in global mode. Global mode is set
// when GetProjectRoot could not locate a windsor.yaml/windsor.yml in the current
// directory or any of its parents, and the shell has fallen back to the user's
// $HOME/.config/windsor directory as the effective project root.
func (s *DefaultShell) IsGlobal() bool {
	return s.global
}

// GetProjectRoot finds the project root. It checks for a cached root first.
// If not found, it looks for "windsor.yaml" or "windsor.yml" in the current directory
// and its parents up to a maximum depth, returning the first match. When no project
// file is found, it falls back to $HOME/.config/windsor, creating that directory if
// needed, and marks the shell as operating in global mode.
func (s *DefaultShell) GetProjectRoot() (string, error) {
	if s.projectRoot != "" {
		return s.projectRoot, nil
	}
	originalDir, err := s.shims.Getwd()
	if err != nil {
		return "", err
	}
	currentDir := originalDir
	depth := 0
	for {
		if depth > MaxFolderSearchDepth {
			return s.fallbackToGlobal(originalDir)
		}
		windsorYaml := filepath.Join(currentDir, "windsor.yaml")
		windsorYml := filepath.Join(currentDir, "windsor.yml")
		if _, err := s.shims.Stat(windsorYaml); err == nil {
			s.projectRoot = currentDir
			return s.projectRoot, nil
		}
		if _, err := s.shims.Stat(windsorYml); err == nil {
			s.projectRoot = currentDir
			return s.projectRoot, nil
		}
		parentDir := filepath.Dir(currentDir)
		if parentDir == currentDir {
			return s.fallbackToGlobal(originalDir)
		}
		currentDir = parentDir
		depth++
	}
}

// RenderEnvVars returns the rendered environment variables as a string instead of printing them
// The export parameter controls whether to use OS-specific export commands or plain KEY=value format
func (s *DefaultShell) RenderEnvVars(envVars map[string]string, export bool) string {
	if export {
		return s.renderEnvVarsWithExport(envVars)
	} else {
		return s.renderEnvVarsPlain(envVars)
	}
}

// PrintEnvVars is a platform-specific method that will be implemented by Unix/Windows-specific files
// The export parameter controls whether to use OS-specific export commands or plain KEY=value format
func (s *DefaultShell) PrintEnvVars(envVars map[string]string, export bool) {
	if export {
		fmt.Print(s.renderEnvVarsWithExport(envVars))
	} else {
		fmt.Print(s.renderEnvVarsPlain(envVars))
	}
}

// =============================================================================
// Private Methods
// =============================================================================

// fallbackToGlobal resolves the global project root at $HOME/.config/windsor,
// creating the directory if it does not exist, and marks the shell as operating
// in global mode. The originalDir is returned as a best-effort fallback if the
// home directory cannot be resolved or the global directory cannot be created;
// in those degraded cases the originalDir is cached as the project root so
// subsequent calls do not re-traverse the directory tree or re-attempt the
// failing operation, and global mode is left unset to signal the degraded state.
func (s *DefaultShell) fallbackToGlobal(originalDir string) (string, error) {
	homeDir, err := s.shims.UserHomeDir()
	if err != nil {
		s.projectRoot = originalDir
		return originalDir, nil
	}
	globalRoot := filepath.Join(homeDir, ".config", "windsor")
	if err := s.shims.MkdirAll(globalRoot, 0750); err != nil {
		s.projectRoot = originalDir
		return originalDir, nil
	}
	s.projectRoot = globalRoot
	s.global = true
	return s.projectRoot, nil
}

// renderEnvVarsPlain returns environment variables in plain KEY=value format as a string, sorted by key.
// If a value is empty, it returns KEY= with no value. Used for non-export output scenarios.
// Values containing special characters are quoted with single quotes to ensure safe shell evaluation.
func (s *DefaultShell) renderEnvVarsPlain(envVars map[string]string) string {
	keys := make([]string, 0, len(envVars))
	for k := range envVars {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var result strings.Builder
	for _, k := range keys {
		if envVars[k] == "" {
			result.WriteString(fmt.Sprintf("%s=\n", k))
		} else {
			value := s.quoteValueForShell(envVars[k], false)
			result.WriteString(fmt.Sprintf("%s=%s\n", k, value))
		}
	}
	return result.String()
}

// quoteValueForShell quotes a value appropriately for shell evaluation.
// For Unix shells, uses single quotes with proper escaping. For Windows PowerShell, uses single quotes with PowerShell-style escaping.
// If useDoubleQuotes is true and the value doesn't need quoting, uses double quotes; otherwise uses single quotes when needed.
func (s *DefaultShell) quoteValueForShell(value string, useDoubleQuotes bool) string {
	if !strings.ContainsAny(value, "[]{}()\"'$`\\ \t\n&|;<>*?~#") {
		if useDoubleQuotes {
			return "\"" + value + "\""
		}
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

// =============================================================================
// Interface Compliance
// =============================================================================

// Ensure DefaultShell implements the Shell interface
var _ Shell = (*DefaultShell)(nil)
