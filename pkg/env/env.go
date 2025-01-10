package env

import (
	"bufio"
	"fmt"
	"os"
	"os/user"
	"path"
	"strings"

	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
)

// EnvPrinter defines the method for printing environment variables.
type EnvPrinter interface {
	Initialize() error
	Print() error
	GetEnvVars() (map[string]string, error)
	PostEnvHook() error
}

// Env is a struct that implements the EnvPrinter interface.
type BaseEnvPrinter struct {
	injector      di.Injector
	shell         shell.Shell
	configHandler config.ConfigHandler
}

// NewBaseEnvPrinter creates a new BaseEnvPrinter instance.
func NewBaseEnvPrinter(injector di.Injector) *BaseEnvPrinter {
	return &BaseEnvPrinter{injector: injector}
}

// Initialize initializes the environment.
func (e *BaseEnvPrinter) Initialize() error {
	// Resolve the shell
	shell, ok := e.injector.Resolve("shell").(shell.Shell)
	if !ok {
		return fmt.Errorf("error resolving or casting shell to shell.Shell")
	}
	e.shell = shell

	// Resolve the configHandler
	configInterface, ok := e.injector.Resolve("configHandler").(config.ConfigHandler)
	if !ok {
		return fmt.Errorf("error resolving or casting configHandler to config.ConfigHandler")
	}
	e.configHandler = configInterface

	return nil
}

// Print prints the environment variables to the console.
// It can optionally take a map of key:value strings and prints those.
func (e *BaseEnvPrinter) Print(customVars ...map[string]string) error {
	var envVars map[string]string

	// Use only the passed vars
	if len(customVars) > 0 {
		envVars = customVars[0]
	} else {
		envVars = make(map[string]string)
	}

	return e.shell.PrintEnvVars(envVars)
}

// GetEnvVars is a placeholder for retrieving environment variables.
func (e *BaseEnvPrinter) GetEnvVars() (map[string]string, error) {
	// Placeholder implementation
	return map[string]string{}, nil
}

// PostEnvHook simulates running any necessary commands after the environment variables have been set.
func (e *BaseEnvPrinter) PostEnvHook() error {
	// Placeholder for post-processing logic
	return nil
}

// Add current directory to the trusted file list
func AddCurrentDirToTrustedFile() error {
	currentDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("Error getting current directory: %w", err)
	}

	usr, err := user.Current()
	if err != nil {
		return fmt.Errorf("Error getting user home directory: %w", err)
	}

	trustedDirPath := path.Join(usr.HomeDir, ".config", "windsor")
	err = os.MkdirAll(trustedDirPath, 0750)
	if err != nil {
		return fmt.Errorf("Error creating directories for trusted file: %w", err)
	}

	trustedFilePath := path.Join(trustedDirPath, ".trusted")
	// #nosec G304 - trustedFilePath is constructed safely and is not user-controlled
	file, err := os.OpenFile(trustedFilePath, os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return fmt.Errorf("Error opening or creating trusted file: %w", err)
	}
	defer file.Close()

	// Check if the current directory is already in the trusted file
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if strings.TrimSpace(scanner.Text()) == currentDir {
			return nil // Directory is already trusted, no need to add
		}
	}

	// Add the current directory to the trusted file
	if _, err := file.WriteString(currentDir + "\n"); err != nil {
		return fmt.Errorf("Error writing to trusted file: %w", err)
	}

	return nil
}

// Check if current directory is in the trusted file list
func CheckTrustedDirectory() error {
	currentDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("Error getting current directory: %w", err)
	}

	usr, err := user.Current()
	if err != nil {
		return fmt.Errorf("Error getting user home directory: %w", err)
	}

	trustedDirPath := path.Join(usr.HomeDir, ".config", "windsor")
	trustedFilePath := path.Join(trustedDirPath, ".trusted")
	// #nosec G304 - trustedFilePath is constructed safely and is not user-controlled
	file, err := os.OpenFile(trustedFilePath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	isTrusted := false
	for scanner.Scan() {
		trustedDir := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(currentDir, trustedDir) {
			isTrusted = true
			break
		}
	}

	if !isTrusted {
		return fmt.Errorf("Current directory not in the trusted list")
	}

	return nil
}
