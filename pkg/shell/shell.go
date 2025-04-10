package shell

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"time"

	"github.com/briandowns/spinner"
	"github.com/windsorcli/cli/pkg/di"
)

// maxFolderSearchDepth is the maximum depth to search for the project root
const maxFolderSearchDepth = 10

// Constants
const (
	SessionTokenPrefix = ".session."
)

// Static private variable to store the session token
var sessionToken string

// Reset the session token - used primarily for testing
func ResetSessionToken() {
	sessionToken = ""
}

// HookContext are the variables available during hook template evaluation
type HookContext struct {
	// SelfPath is the unescaped absolute path to direnv
	SelfPath string
}

// Shell interface defines methods for shell operations
type Shell interface {
	// Initialize initializes the shell environment
	Initialize() error
	// SetVerbosity sets the verbosity flag
	SetVerbosity(verbose bool)
	// PrintEnvVars prints the provided environment variables
	PrintEnvVars(envVars map[string]string)
	// PrintAlias retrieves the shell alias
	PrintAlias(envVars map[string]string)
	// GetProjectRoot retrieves the project root directory
	GetProjectRoot() (string, error)
	// Exec executes a command with optional privilege elevation
	Exec(command string, args ...string) (string, error)
	// ExecSilent executes a command and returns its output as a string without printing to stdout or stderr
	ExecSilent(command string, args ...string) (string, error)
	// ExecSudo executes a command with sudo if not already present and returns its output as a string while suppressing it from being printed
	ExecSudo(message string, command string, args ...string) (string, error)
	// ExecProgress executes a command and returns its output as a string while displaying progress status
	ExecProgress(message string, command string, args ...string) (string, error)
	// InstallHook installs a shell hook for the specified shell name
	InstallHook(shellName string) error
	// AddCurrentDirToTrustedFile adds the current directory to a trusted list stored in a file.
	AddCurrentDirToTrustedFile() error
	// CheckTrustedDirectory verifies if the current directory is in the trusted file list.
	CheckTrustedDirectory() error
	// UnsetEnvs generates a command to unset multiple environment variables
	UnsetEnvs(envVars []string)
	// UnsetAlias generates commands to unset multiple aliases
	UnsetAlias(aliases []string)
	// WriteResetToken writes a reset token file based on the session token
	WriteResetToken() (string, error)
	// GetSessionToken retrieves or generates a session token
	GetSessionToken() (string, error)
	// CheckResetFlags checks if a reset signal file exists for the current session
	CheckResetFlags() (bool, error)
	// Reset removes all managed environment variables and aliases
	Reset()
}

// DefaultShell is the default implementation of the Shell interface
type DefaultShell struct {
	Shell
	projectRoot string
	injector    di.Injector
	verbose     bool
}

// NewDefaultShell creates a new instance of DefaultShell
func NewDefaultShell(injector di.Injector) *DefaultShell {
	return &DefaultShell{
		injector: injector,
	}
}

// Initialize initializes the shell
func (s *DefaultShell) Initialize() error {
	return nil
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

	originalDir, err := getwd()
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

		if _, err := osStat(windsorYaml); err == nil {
			s.projectRoot = currentDir
			return s.projectRoot, nil
		}
		if _, err := osStat(windsorYml); err == nil {
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
func (s *DefaultShell) Exec(command string, args ...string) (string, error) {
	cmd := execCommand(command, args...)
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = io.MultiWriter(os.Stdout, &stdoutBuf)
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderrBuf)
	if command == "sudo" {
		cmd.Stdin = os.Stdin
	}
	if err := cmdStart(cmd); err != nil {
		return stdoutBuf.String(), fmt.Errorf("command start failed: %w", err)
	}
	if err := cmdWait(cmd); err != nil {
		return stdoutBuf.String(), fmt.Errorf("command execution failed: %w", err)
	}
	return stdoutBuf.String(), nil
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

	cmd := execCommand(command, args...)
	tty, err := osOpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return "", fmt.Errorf("failed to open /dev/tty: %w", err)
	}
	defer tty.Close()

	cmd.Stdin = tty
	cmd.Stderr = tty

	var stdoutBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf

	if err := cmdStart(cmd); err != nil {
		fmt.Fprintf(os.Stderr, "\033[31m✗ %s - Failed\033[0m\n", message)
		return stdoutBuf.String(), err
	}

	err = cmdWait(cmd)

	if err != nil {
		fmt.Fprintf(os.Stderr, "\033[31m✗ %s - Failed\033[0m\n", message)
		return stdoutBuf.String(), fmt.Errorf("command execution failed: %w", err)
	}

	fmt.Fprintf(os.Stderr, "\033[32m✔\033[0m %s - \033[32mDone\033[0m\n", message)

	return stdoutBuf.String(), nil
}

// ExecSilent is a method that runs a command quietly, capturing its output.
// It returns the command's stdout as a string and any error encountered.
func (s *DefaultShell) ExecSilent(command string, args ...string) (string, error) {
	if s.verbose {
		return s.Exec(command, args...)
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd := execCommand(command, args...)

	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	if err := cmdRun(cmd); err != nil {
		return stdoutBuf.String(), fmt.Errorf("command execution failed: %w\n%s", err, stderrBuf.String())
	}

	return stdoutBuf.String(), nil
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

	cmd := execCommand(command, args...)

	stdoutPipe, err := cmdStdoutPipe(cmd)
	if err != nil {
		return "", err
	}

	stderrPipe, err := cmdStderrPipe(cmd)
	if err != nil {
		return "", err
	}

	if err := cmdStart(cmd); err != nil {
		return "", err
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	errChan := make(chan error, 2)

	spin := spinner.New(spinner.CharSets[14], 100*time.Millisecond, spinner.WithColor("green"))
	spin.Suffix = " " + message
	spin.Start()

	go func() {
		scanner := bufio.NewScanner(stdoutPipe)
		for bufioScannerScan(scanner) {
			line := scanner.Text()
			stdoutBuf.WriteString(line + "\n")
		}
		if err := bufioScannerErr(scanner); err != nil {
			errChan <- fmt.Errorf("error reading stdout: %w", err)
			return
		}
		errChan <- nil
	}()

	go func() {
		scanner := bufio.NewScanner(stderrPipe)
		for bufioScannerScan(scanner) {
			line := scanner.Text()
			stderrBuf.WriteString(line + "\n")
		}
		if err := bufioScannerErr(scanner); err != nil {
			errChan <- fmt.Errorf("error reading stderr: %w", err)
			return
		}
		errChan <- nil
	}()

	if err := cmdWait(cmd); err != nil {
		spin.Stop()
		fmt.Fprintf(os.Stderr, "\033[31m✗ %s - Failed\033[0m\n%s", message, stderrBuf.String())
		return stdoutBuf.String(), fmt.Errorf("command execution failed: %w\n%s", err, stderrBuf.String())
	}

	for i := 0; i < 2; i++ {
		if err := <-errChan; err != nil {
			spin.Stop()
			fmt.Fprintf(os.Stderr, "\033[31m✗ %s - Failed\033[0m\n%s", message, stderrBuf.String())
			return stdoutBuf.String(), err
		}
	}

	spin.Stop()
	fmt.Fprintf(os.Stderr, "\033[32m✔\033[0m %s - \033[32mDone\033[0m\n", message)

	return stdoutBuf.String(), nil
}

// InstallHook sets up a shell hook for a specified shell using a template with the Windsor path.
// It returns an error if the shell is unsupported. For PowerShell, it formats the script into a single line.
func (s *DefaultShell) InstallHook(shellName string) error {
	hookCommand, exists := shellHooks[shellName]
	if !exists {
		return fmt.Errorf("Unsupported shell: %s", shellName)
	}

	selfPath, err := osExecutable()
	if err != nil {
		return err
	}
	selfPath = strings.ReplaceAll(selfPath, "\\", "/")

	ctx := HookContext{SelfPath: selfPath}

	hookTemplate := hookTemplateNew("hook")
	if hookTemplate == nil {
		return fmt.Errorf("failed to create new template")
	}

	hookTemplate, err = hookTemplateParse(hookTemplate, hookCommand)
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	if err := hookTemplateExecute(hookTemplate, &buf, ctx); err != nil {
		return err
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

// Adds the current directory to a trusted list stored in a file.
// Creates necessary directories if they don't exist.
// Checks if the directory is already trusted before adding.
func (s *DefaultShell) AddCurrentDirToTrustedFile() error {
	projectRoot, err := s.GetProjectRoot()
	if err != nil {
		return fmt.Errorf("Error getting project root directory: %w", err)
	}

	homeDir, err := osUserHomeDir()
	if err != nil {
		return fmt.Errorf("Error getting user home directory: %w", err)
	}

	trustedDirPath := path.Join(homeDir, ".config", "windsor")
	err = osMkdirAll(trustedDirPath, 0750)
	if err != nil {
		return fmt.Errorf("Error creating directories for trusted file: %w", err)
	}

	trustedFilePath := path.Join(trustedDirPath, ".trusted")

	data, err := osReadFile(trustedFilePath)
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
	if err := osWriteFile(trustedFilePath, data, 0600); err != nil {
		return fmt.Errorf("Error writing to trusted file: %w", err)
	}

	return nil
}

// Check if the current directory is in the trusted file list.
func (s *DefaultShell) CheckTrustedDirectory() error {
	projectRoot, err := s.GetProjectRoot()
	if err != nil {
		return fmt.Errorf("Error getting project root directory: %w", err)
	}

	homeDir, err := osUserHomeDir()
	if err != nil {
		return fmt.Errorf("Error getting user home directory: %w", err)
	}

	trustedDirPath := path.Join(homeDir, ".config", "windsor")
	trustedFilePath := path.Join(trustedDirPath, ".trusted")

	data, err := osReadFile(trustedFilePath)
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
	sessionToken := os.Getenv("WINDSOR_SESSION_TOKEN")
	if sessionToken == "" {
		return "", nil
	}

	projectRoot, err := s.GetProjectRoot()
	if err != nil {
		return "", fmt.Errorf("error getting project root: %w", err)
	}

	// Create .windsor directory if it doesn't exist
	windsorDir := filepath.Join(projectRoot, ".windsor")
	if err := osMkdirAll(windsorDir, 0750); err != nil {
		return "", fmt.Errorf("error creating .windsor directory: %w", err)
	}

	sessionFilePath := filepath.Join(windsorDir, SessionTokenPrefix+sessionToken)

	if err := osWriteFile(sessionFilePath, []byte{}, 0600); err != nil {
		return "", fmt.Errorf("error writing reset token file: %w", err)
	}

	return sessionFilePath, nil
}

// GetSessionToken retrieves or generates a session token. It first checks if a token is already stored in memory.
// If not, it looks for a token in the environment variable. If no token is found in the environment, it generates a new token.
func (s *DefaultShell) GetSessionToken() (string, error) {
	// If we already have a token in memory, return it
	if sessionToken != "" {
		return sessionToken, nil
	}

	envToken := osGetenv("WINDSOR_SESSION_TOKEN")
	if envToken != "" {
		sessionToken = envToken
		return envToken, nil
	}

	token, err := s.generateRandomString(7)
	if err != nil {
		return "", fmt.Errorf("error generating session token: %w", err)
	}

	sessionToken = token
	return token, nil
}

// Reset removes all managed environment variables and aliases.
// It uses the environment variables "WINDSOR_MANAGED_ENV" and "WINDSOR_MANAGED_ALIAS"
// to retrieve the previous set of managed environment variables and aliases, respectively.
// These environment variables represent the previous set of managed values that need to be reset.
func (s *DefaultShell) Reset() {
	var managedEnvs []string
	if envStr := os.Getenv("WINDSOR_MANAGED_ENV"); envStr != "" {
		for _, env := range strings.Split(envStr, ",") {
			env = strings.TrimSpace(env)
			if env != "" {
				managedEnvs = append(managedEnvs, env)
				os.Unsetenv(env)
			}
		}
	}

	var managedAliases []string
	if aliasStr := os.Getenv("WINDSOR_MANAGED_ALIAS"); aliasStr != "" {
		for _, alias := range strings.Split(aliasStr, ",") {
			alias = strings.TrimSpace(alias)
			if alias != "" {
				managedAliases = append(managedAliases, alias)
			}
		}
	}

	if len(managedEnvs) > 0 {
		s.UnsetEnvs(managedEnvs)
	}

	if len(managedAliases) > 0 {
		s.UnsetAlias(managedAliases)
	}
}

// generateRandomString creates a secure random string of the given length using a predefined charset.
func (s *DefaultShell) generateRandomString(length int) (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	randomBytes := make([]byte, length)

	_, err := randRead(randomBytes)
	if err != nil {
		return "", err
	}

	// Map random bytes to charset
	for i, b := range randomBytes {
		randomBytes[i] = charset[b%byte(len(charset))]
	}

	return string(randomBytes), nil
}

// CheckResetFlags checks if a reset signal file exists for the current session token.
// It returns true if the specific session token file exists and always removes all .session.* files.
func (s *DefaultShell) CheckResetFlags() (bool, error) {
	// Get current session token from environment
	envToken := osGetenv("WINDSOR_SESSION_TOKEN")
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
	if _, err := osStat(tokenFilePath); err == nil {
		tokenFileExists = true
	}

	// Remove all .session.* files
	sessionFiles, err := filepathGlob(filepath.Join(windsorDir, SessionTokenPrefix+"*"))
	if err != nil {
		return false, fmt.Errorf("error finding session files: %w", err)
	}

	for _, file := range sessionFiles {
		if err := osRemoveAll(file); err != nil {
			return false, fmt.Errorf("error removing session file %s: %w", file, err)
		}
	}

	return tokenFileExists, nil
}

// Ensure DefaultShell implements the Shell interface
var _ Shell = (*DefaultShell)(nil)
