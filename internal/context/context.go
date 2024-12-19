package context

import (
	"fmt"
	"path/filepath"

	"github.com/windsorcli/cli/internal/config"
	"github.com/windsorcli/cli/internal/di"
	"github.com/windsorcli/cli/internal/shell"
)

// ContextHandlerInterface defines the interface for context operations
type ContextHandler interface {
	Initialize() error
	GetContext() string              // Retrieves the current context
	SetContext(context string) error // Sets the current context
	GetConfigRoot() (string, error)  // Retrieves the configuration root path
	Clean() error                    // Cleans up context specific artifacts
}

// BaseContextHandler implements the ContextHandlerInterface
type BaseContextHandler struct {
	injector      di.Injector          // Handles dependency injection
	configHandler config.ConfigHandler // Handles configuration operations
	shell         shell.Shell          // Handles shell operations
}

// NewContextHandler creates a new ContextHandler instance
func NewContextHandler(injector di.Injector) *BaseContextHandler {
	return &BaseContextHandler{injector: injector}
}

// Initialize initializes the context handler
func (c *BaseContextHandler) Initialize() error {
	// Resolve the config handler
	configHandler, ok := c.injector.Resolve("configHandler").(config.ConfigHandler)
	if !ok {
		return fmt.Errorf("error resolving configHandler")
	}
	c.configHandler = configHandler

	// Resolve the shell
	shell, ok := c.injector.Resolve("shell").(shell.Shell)
	if !ok {
		return fmt.Errorf("error resolving shell")
	}
	c.shell = shell

	return nil
}

// GetContext retrieves the current context from the configuration
func (c *BaseContextHandler) GetContext() string {
	context := c.configHandler.Get("context")
	if context == nil {
		return "local"
	}
	return context.(string)
}

// SetContext sets the current context in the configuration and saves it
func (c *BaseContextHandler) SetContext(context string) error {
	err := c.configHandler.Set("context", context)
	if err != nil {
		return fmt.Errorf("error setting context: %w", err)
	}
	if err := c.configHandler.SaveConfig(""); err != nil {
		return fmt.Errorf("error saving config: %w", err)
	}
	return nil
}

// GetConfigRoot retrieves the configuration root path based on the current context
func (c *BaseContextHandler) GetConfigRoot() (string, error) {
	context := c.GetContext()

	projectRoot, err := c.shell.GetProjectRoot()
	if err != nil {
		return "", err
	}

	configRoot := filepath.Join(projectRoot, "contexts", context)
	return configRoot, nil
}

// Clean cleans up context specific artifacts
func (c *BaseContextHandler) Clean() error {
	configRoot, err := c.GetConfigRoot()
	if err != nil {
		return fmt.Errorf("error getting config root: %w", err)
	}

	dirsToDelete := []string{".kube", ".talos", ".omni", ".aws", ".terraform", ".tfstate"}

	for _, dir := range dirsToDelete {
		path := filepath.Join(configRoot, dir)
		if _, err := osStat(path); err == nil {
			if err := osRemoveAll(path); err != nil {
				return fmt.Errorf("error deleting %s: %w", path, err)
			}
		}
	}

	return nil
}
