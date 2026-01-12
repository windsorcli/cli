package shell

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/briandowns/spinner"
)

// The Shell package is a unified interface for shell operations across different platforms.
// It provides cross-platform command execution, environment variable management, session token handling, and secret scrubbing.
// This package serves as the core interface for all shell operations in the Windsor CLI with built-in security features.
// Key features include command execution, environment variable management, session token handling, and automatic secret scrubbing from command output.

// =============================================================================
// Constants
// =============================================================================

// maxFolderSearchDepth is the maximum depth to search for the project root
const maxFolderSearchDepth = 10

// SessionTokenPrefix is the prefix used for session token files
const SessionTokenPrefix = ".session."

// =============================================================================
// Types
// =============================================================================

// HookContext are the variables available during hook template evaluation
type HookContext struct {
	// SelfPath is the unescaped absolute path to direnv
	SelfPath string
}

// Shell is the interface that defines shell operations.
type Shell interface {
	SetVerbosity(verbose bool)
	RenderEnvVars(envVars map[string]string, export bool) string
	RenderAliases(aliases map[string]string) string
	GetProjectRoot() (string, error)
	Exec(command string, args ...string) (string, error)
	ExecSilent(command string, args ...string) (string, error)
	ExecSilentWithTimeout(command string, args []string, timeout time.Duration) (string, error)
	ExecSudo(message string, command string, args ...string) (string, error)
	ExecProgress(message string, command string, args ...string) (string, error)
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
}

// DefaultShell is the default implementation of the Shell interface
type DefaultShell struct {
	Shell
	projectRoot  string
	verbose      bool
	sessionToken string
	shims        *Shims
	secrets      []string
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

// SetVerbosity sets the verbosity flag
func (s *DefaultShell) SetVerbosity(verbose bool) {
	s.verbose = verbose
}

// GetProjectRoot finds the project root. It checks for a cached root first.
// If not found, it looks for "windsor.yaml" or "windsor.yml" in the current
// directory and its parents up to a maximum depth. Returns the root path or an empty string if not found.
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
		if depth > maxFolderSearchDepth {
			return originalDir, nil
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
			return originalDir, nil
		}
		currentDir = parentDir
		depth++
	}
}

// Exec runs a command with args, capturing stdout and stderr. It prints output and returns stdout as a string.
// If the command is "sudo", it connects stdin to the terminal for password input.
// All output is scrubbed to remove registered secrets before being displayed or returned.
func (s *DefaultShell) Exec(command string, args ...string) (string, error) {
	cmd := s.shims.Command(command, args...)
	var stdoutBuf, stderrBuf bytes.Buffer

	scrubbingStdoutWriter := &scrubbingWriter{writer: os.Stdout, scrubFunc: s.scrubString}
	scrubbingStderrWriter := &scrubbingWriter{writer: os.Stderr, scrubFunc: s.scrubString}

	cmd.Stdout = io.MultiWriter(scrubbingStdoutWriter, &stdoutBuf)
	cmd.Stderr = io.MultiWriter(scrubbingStderrWriter, &stderrBuf)
	if command == "sudo" {
		cmd.Stdin = os.Stdin
	}
	// Ensure the command inherits the current environment
	if cmd.Env == nil {
		cmd.Env = s.shims.Environ()
	}
	if err := s.shims.CmdStart(cmd); err != nil {
		return stdoutBuf.String(), fmt.Errorf("command start failed: %w", err)
	}
	if err := s.shims.CmdWait(cmd); err != nil {
		return s.scrubString(stdoutBuf.String()), fmt.Errorf("command execution failed: %w", err)
	}
	return s.scrubString(stdoutBuf.String()), nil
}

// ExecSudo runs a command with 'sudo', ensuring elevated privileges. It handles password prompts by
// connecting to the terminal and captures the command's output. If verbose mode is enabled, it prints
// a message to stderr. The function returns the command's stdout or an error if execution fails.
func (s *DefaultShell) ExecSudo(message string, command string, args ...string) (string, error) {
	if s.verbose {
		fmt.Fprintln(os.Stderr, message)
		return s.Exec("sudo", append([]string{command}, args...)...)
	}

	if command != "sudo" {
		args = append([]string{command}, args...)
		command = "sudo"
	}

	cmd := s.shims.Command(command, args...)
	if cmd == nil {
		return "", fmt.Errorf("failed to create command")
	}

	// Ensure the command inherits the current environment
	if cmd.Env == nil {
		cmd.Env = s.shims.Environ()
	}

	tty, err := s.shims.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return "", fmt.Errorf("failed to open /dev/tty: %w", err)
	}
	defer tty.Close()

	cmd.Stdin = tty
	cmd.Stderr = tty

	var stdoutBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf

	if err := s.shims.CmdStart(cmd); err != nil {
		fmt.Fprintf(os.Stderr, "\033[31m✗ %s - Failed\033[0m\n", message)
		return stdoutBuf.String(), err
	}

	err = s.shims.CmdWait(cmd)

	if err != nil {
		fmt.Fprintf(os.Stderr, "\033[31m✗ %s - Failed\033[0m\n", message)
		return stdoutBuf.String(), fmt.Errorf("command execution failed: %w", err)
	}

	fmt.Fprintf(os.Stderr, "\033[32m✔\033[0m %s - \033[32mDone\033[0m\n", message)

	return s.scrubString(stdoutBuf.String()), nil
}

// ExecSilent is a method that runs a command quietly, capturing its output.
// It returns the command's stdout as a string and any error encountered.
// Sets SysProcAttr.Setsid to prevent the process from accessing /dev/tty,
// ensuring all output goes through the redirected stdout/stderr buffers.
func (s *DefaultShell) ExecSilent(command string, args ...string) (string, error) {
	if s.verbose {
		return s.Exec(command, args...)
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd := s.shims.Command(command, args...)
	if cmd == nil {
		return "", fmt.Errorf("failed to create command")
	}

	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid:  true,
		Setctty: false,
	}
	// Ensure the command inherits the current environment
	if cmd.Env == nil {
		cmd.Env = s.shims.Environ()
	}

	if err := s.shims.CmdRun(cmd); err != nil {
		return s.scrubString(stdoutBuf.String()), fmt.Errorf("command execution failed: %w\n%s", err, s.scrubString(stderrBuf.String()))
	}

	return s.scrubString(stdoutBuf.String()), nil
}

// ExecSilentWithTimeout executes a command with a timeout and returns the output.
// If the command takes longer than the timeout, it kills the process and returns an error.
// Uses ExecSilent internally but wraps it with a timeout mechanism.
// Sets SysProcAttr.Setsid to prevent the process from accessing /dev/tty,
// ensuring all output goes through the redirected stdout/stderr buffers.
func (s *DefaultShell) ExecSilentWithTimeout(command string, args []string, timeout time.Duration) (string, error) {
	if s.verbose {
		return s.Exec(command, args...)
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd := s.shims.Command(command, args...)
	if cmd == nil {
		return "", fmt.Errorf("failed to create command")
	}

	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid:  true,
		Setctty: false,
	}
	if cmd.Env == nil {
		cmd.Env = s.shims.Environ()
	}

	if err := s.shims.CmdStart(cmd); err != nil {
		return "", fmt.Errorf("command start failed: %w", err)
	}

	var waitOnce sync.Once
	execFn := func() (string, error) {
		var waitErr error
		waitOnce.Do(func() {
			waitErr = s.shims.CmdWait(cmd)
		})
		if waitErr != nil {
			return s.scrubString(stdoutBuf.String()), fmt.Errorf("command execution failed: %w\n%s", waitErr, s.scrubString(stderrBuf.String()))
		}
		return s.scrubString(stdoutBuf.String()), nil
	}

	cleanupFn := func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			waitOnce.Do(func() {
				_ = s.shims.CmdWait(cmd)
			})
		}
	}

	return executeWithTimeout(execFn, cleanupFn, timeout)
}

// ExecProgress is a method of the DefaultShell struct that executes a command with a progress indicator.
// It takes a message, a command, and arguments, using the Exec method if verbose mode is enabled.
// Otherwise, it captures stdout and stderr with pipes and uses a spinner to show progress.
// The method returns the command's stdout as a string and any error encountered.
func (s *DefaultShell) ExecProgress(message string, command string, args ...string) (string, error) {
	if s.verbose {
		fmt.Fprintln(os.Stderr, message)
		return s.Exec(command, args...)
	}

	cmd := s.shims.Command(command, args...)
	if cmd == nil {
		return "", fmt.Errorf("failed to create command")
	}

	// Ensure the command inherits the current environment
	if cmd.Env == nil {
		cmd.Env = s.shims.Environ()
	}

	stdoutPipe, err := s.shims.StdoutPipe(cmd)
	if err != nil {
		return "", err
	}

	stderrPipe, err := s.shims.StderrPipe(cmd)
	if err != nil {
		return "", err
	}

	if err := s.shims.CmdStart(cmd); err != nil {
		return "", err
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	errChan := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(2)

	spin := spinner.New(spinner.CharSets[14], 100*time.Millisecond, spinner.WithColor("green"))
	spin.Suffix = " " + message
	spin.Start()

	go func() {
		defer wg.Done()
		scanner := s.shims.NewScanner(stdoutPipe)
		if scanner == nil {
			errChan <- fmt.Errorf("failed to create stdout scanner")
			return
		}
		for s.shims.ScannerScan(scanner) {
			line := s.shims.ScannerText(scanner)
			if line != "" {
				stdoutBuf.WriteString(line + "\n")
			}
		}
		if err := s.shims.ScannerErr(scanner); err != nil && err != io.EOF && !isClosedPipe(err) {
			errChan <- fmt.Errorf("error reading stdout: %w", err)
			return
		}
		errChan <- nil
	}()

	go func() {
		defer wg.Done()
		scanner := s.shims.NewScanner(stderrPipe)
		if scanner == nil {
			errChan <- fmt.Errorf("failed to create stderr scanner")
			return
		}
		for s.shims.ScannerScan(scanner) {
			line := s.shims.ScannerText(scanner)
			if line != "" {
				stderrBuf.WriteString(line + "\n")
			}
		}
		if err := s.shims.ScannerErr(scanner); err != nil && err != io.EOF && !isClosedPipe(err) {
			errChan <- fmt.Errorf("error reading stderr: %w", err)
			return
		}
		errChan <- nil
	}()

	wg.Wait()
	spin.Stop()

	var firstErr error
	for range [2]struct{}{} {
		err := <-errChan
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}

	cmdErr := s.shims.CmdWait(cmd)

	if firstErr != nil || cmdErr != nil {
		fmt.Fprintf(os.Stderr, "\n[ExecProgress ERROR]\nCommand: %s %v\nStdout:\n%s\nStderr:\n%s\nError: %v\n", command, s.scrubString(fmt.Sprintf("%v", args)), s.scrubString(stdoutBuf.String()), s.scrubString(stderrBuf.String()), firstErr)
		if cmdErr != nil {
			return s.scrubString(stdoutBuf.String()), fmt.Errorf("command execution failed: %w", cmdErr)
		}
		return s.scrubString(stdoutBuf.String()), firstErr
	}

	fmt.Fprintf(os.Stderr, "\033[32m✔\033[0m %s - \033[32mDone\033[0m\n", message)
	return s.scrubString(stdoutBuf.String()), nil
}

// InstallHook sets up a shell hook for a specified shell using a template with the Windsor path.
// It returns an error if the shell is unsupported. For PowerShell, it formats the script into a single line.
func (s *DefaultShell) InstallHook(shellName string) error {
	hookCommand, ok := shellHooks[shellName]
	if !ok {
		return fmt.Errorf("Unsupported shell: %s", shellName)
	}

	selfPath, err := s.shims.Executable()
	if err != nil {
		return err
	}

	ctx := HookContext{SelfPath: selfPath}

	hookTemplate := s.shims.NewTemplate("hook")
	if hookTemplate == nil {
		return fmt.Errorf("failed to create new template")
	}

	hookTemplate, err = s.shims.TemplateParse(hookTemplate, hookCommand)
	if err != nil {
		return fmt.Errorf("failed to parse hook template: %w", err)
	}

	var buf bytes.Buffer
	if err := s.shims.TemplateExecute(hookTemplate, &buf, ctx); err != nil {
		return fmt.Errorf("failed to execute hook template: %w", err)
	}

	hookScript := buf.String()

	if shellName == "powershell" {
		lines := strings.Split(hookScript, "\n")
		var cleaned []string
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line != "" {
				cleaned = append(cleaned, line)
			}
		}
		hookScript = strings.Join(cleaned, "; ")
	}

	_, err = os.Stdout.WriteString(hookScript)
	return err
}

// AddCurrentDirToTrustedFile adds the current directory to a trusted list stored in a file.
// Creates necessary directories if they don't exist.
// Checks if the directory is already trusted before adding.
func (s *DefaultShell) AddCurrentDirToTrustedFile() error {
	projectRoot, err := s.GetProjectRoot()
	if err != nil {
		return fmt.Errorf("Error getting project root directory: %w", err)
	}

	homeDir, err := s.shims.UserHomeDir()
	if err != nil {
		return fmt.Errorf("Error getting user home directory: %w", err)
	}

	trustedDirPath := path.Join(homeDir, ".config", "windsor")
	err = s.shims.MkdirAll(trustedDirPath, 0750)
	if err != nil {
		return fmt.Errorf("Error creating directories for trusted file: %w", err)
	}

	trustedFilePath := path.Join(trustedDirPath, ".trusted")

	data, err := s.shims.ReadFile(trustedFilePath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("Error reading trusted file: %w", err)
	}

	trustedDirs := strings.Split(strings.TrimSpace(string(data)), "\n")
	for _, dir := range trustedDirs {
		if strings.TrimSpace(dir) == projectRoot {
			return nil
		}
	}

	data = append(data, []byte(projectRoot+"\n")...)
	if err := s.shims.WriteFile(trustedFilePath, data, 0600); err != nil {
		return fmt.Errorf("Error writing to trusted file: %w", err)
	}

	return nil
}

// CheckTrustedDirectory verifies if the current directory is in the trusted file list.
func (s *DefaultShell) CheckTrustedDirectory() error {
	projectRoot, err := s.GetProjectRoot()
	if err != nil {
		return fmt.Errorf("Error getting project root directory: %w", err)
	}

	homeDir, err := s.shims.UserHomeDir()
	if err != nil {
		return fmt.Errorf("Error getting user home directory: %w", err)
	}

	trustedDirPath := path.Join(homeDir, ".config", "windsor")
	trustedFilePath := path.Join(trustedDirPath, ".trusted")

	data, err := s.shims.ReadFile(trustedFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("Trusted file does not exist")
		}
		return fmt.Errorf("Error reading trusted file: %w", err)
	}

	isTrusted := false
	trustedDirs := strings.Split(strings.TrimSpace(string(data)), "\n")
	for _, trustedDir := range trustedDirs {
		trimmedDir := strings.TrimSpace(trustedDir)
		if trimmedDir != "" && strings.HasPrefix(projectRoot, trimmedDir) {
			isTrusted = true
			break
		}
	}

	if !isTrusted {
		return fmt.Errorf("Current directory not in the trusted list")
	}

	return nil
}

// WriteResetToken writes a reset token file based on the WINDSOR_SESSION_TOKEN
// environment variable. If the environment variable doesn't exist, no file is written.
// Returns the path to the written file or an empty string if no file was written.
func (s *DefaultShell) WriteResetToken() (string, error) {
	sessionToken := s.shims.Getenv("WINDSOR_SESSION_TOKEN")
	if sessionToken == "" {
		return "", nil
	}

	projectRoot, err := s.GetProjectRoot()
	if err != nil {
		return "", fmt.Errorf("error getting project root: %w", err)
	}

	// Create .windsor directory if it doesn't exist
	windsorDir := filepath.Join(projectRoot, ".windsor")
	if err := s.shims.MkdirAll(windsorDir, 0750); err != nil {
		return "", fmt.Errorf("error creating .windsor directory: %w", err)
	}

	sessionFilePath := filepath.Join(windsorDir, SessionTokenPrefix+sessionToken)

	if err := s.shims.WriteFile(sessionFilePath, []byte{}, 0600); err != nil {
		return "", fmt.Errorf("error writing reset token file: %w", err)
	}

	return sessionFilePath, nil
}

// GetSessionToken retrieves or generates a session token. It first checks if a token is already stored in memory.
// If not, it looks for a token in the environment variable. If no token is found in the environment, it generates a new token.
func (s *DefaultShell) GetSessionToken() (string, error) {
	if s.sessionToken != "" {
		return s.sessionToken, nil
	}

	envToken := s.shims.Getenv("WINDSOR_SESSION_TOKEN")
	if envToken != "" {
		s.sessionToken = envToken
		return envToken, nil
	}

	token, err := s.generateRandomString(7)
	if err != nil {
		return "", fmt.Errorf("error generating session token: %w", err)
	}

	s.sessionToken = token
	return token, nil
}

// RegisterSecret adds a secret value to the internal list of secrets that will be scrubbed from all command output.
// Empty strings are ignored to prevent unnecessary processing.
// Duplicate values are automatically filtered out to maintain list efficiency.
func (s *DefaultShell) RegisterSecret(value string) {
	if value == "" {
		return
	}

	if slices.Contains(s.secrets, value) {
		return
	}

	s.secrets = append(s.secrets, value)
}

// Reset removes all managed environment variables and aliases.
// It uses the environment variables "WINDSOR_MANAGED_ENV" and "WINDSOR_MANAGED_ALIAS"
// to retrieve the previous set of managed environment variables and aliases, respectively.
// These environment variables represent the previous set of managed values that need to be reset.
// The optional quiet parameter controls whether shell commands are printed during reset.
func (s *DefaultShell) Reset(quiet ...bool) {
	isQuiet := len(quiet) > 0 && quiet[0]

	var managedEnvs []string
	if envStr := s.shims.Getenv("WINDSOR_MANAGED_ENV"); envStr != "" {
		for env := range strings.SplitSeq(envStr, ",") {
			env = strings.TrimSpace(env)
			if env != "" {
				managedEnvs = append(managedEnvs, env)
				os.Unsetenv(env)
			}
		}
	}

	var managedAliases []string
	if aliasStr := s.shims.Getenv("WINDSOR_MANAGED_ALIAS"); aliasStr != "" {
		for alias := range strings.SplitSeq(aliasStr, ",") {
			alias = strings.TrimSpace(alias)
			if alias != "" {
				managedAliases = append(managedAliases, alias)
			}
		}
	}

	if !isQuiet {
		if len(managedEnvs) > 0 {
			s.UnsetEnvs(managedEnvs)
		}
		if len(managedAliases) > 0 {
			s.UnsetAlias(managedAliases)
		}
	}
}

// CheckResetFlags checks if a reset signal file exists for the current session token.
// It returns true if the specific session token file exists and always removes all .session.* files.
func (s *DefaultShell) CheckResetFlags() (bool, error) {
	// Get current session token from environment
	envToken := s.shims.Getenv("WINDSOR_SESSION_TOKEN")
	if envToken == "" {
		return false, nil
	}

	projectRoot, err := s.GetProjectRoot()
	if err != nil {
		return false, fmt.Errorf("error getting project root: %w", err)
	}

	windsorDir := filepath.Join(projectRoot, ".windsor")
	tokenFilePath := filepath.Join(windsorDir, ".session."+envToken)

	// Check for the specific session token file
	tokenFileExists := false
	if _, err := s.shims.Stat(tokenFilePath); err == nil {
		tokenFileExists = true
	}

	sessionFiles, err := s.shims.Glob(filepath.Join(windsorDir, SessionTokenPrefix+"*"))
	if err != nil {
		return false, fmt.Errorf("error finding session files: %w", err)
	}

	for _, file := range sessionFiles {
		if err := s.shims.RemoveAll(file); err != nil {
			return false, fmt.Errorf("error removing session file %s: %w", file, err)
		}
	}

	return tokenFileExists, nil
}

// ResetSessionToken resets the session token - used primarily for testing
func (s *DefaultShell) ResetSessionToken() {
	s.sessionToken = ""
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

// =============================================================================
// Private Methods
// =============================================================================

// generateRandomString creates a secure random string of the given length using a predefined charset.
func (s *DefaultShell) generateRandomString(length int) (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	if _, err := s.shims.RandRead(b); err != nil {
		return "", err
	}
	for i := range b {
		b[i] = charset[int(b[i])%len(charset)]
	}
	return string(b), nil
}

// scrubString replaces all registered secret values with fixed "********" strings for security.
// It processes the input string and replaces any occurrence of registered secrets with asterisks.
// This method is used internally by all command execution methods to prevent secret leakage in output.
func (s *DefaultShell) scrubString(input string) string {
	result := input
	for _, secret := range s.secrets {
		if secret != "" {
			result = strings.ReplaceAll(result, secret, "********")
		}
	}

	return result
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
// Helper Functions
// =============================================================================

// executeWithTimeout executes a function with a timeout and ensures cleanup happens exactly once.
// It takes an execution function that returns output and error, a cleanup function to call on timeout,
// and a timeout duration. Returns the output and error from execution, or a timeout error if the timeout is exceeded.
func executeWithTimeout(execFn func() (string, error), cleanupFn func(), timeout time.Duration) (string, error) {
	type result struct {
		out string
		err error
	}
	resultChan := make(chan result, 1)
	var cleanupOnce sync.Once
	go func() {
		defer cleanupOnce.Do(cleanupFn)
		out, err := execFn()
		resultChan <- result{out: out, err: err}
	}()

	select {
	case res := <-resultChan:
		return res.out, res.err
	case <-time.After(timeout):
		cleanupOnce.Do(cleanupFn)
		return "", fmt.Errorf("command timed out after %v", timeout)
	}
}

// isClosedPipe returns true if the error is an io.ErrClosedPipe or equivalent
func isClosedPipe(err error) bool {
	return err != nil && (err == io.ErrClosedPipe || strings.Contains(err.Error(), "file already closed") || strings.Contains(err.Error(), "use of closed file"))
}

// =============================================================================
// Interface Compliance
// =============================================================================

// Ensure DefaultShell implements the Shell interface
var _ Shell = (*DefaultShell)(nil)
