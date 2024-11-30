package context

import (
	"fmt"
	"path/filepath"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/shell"
)

// ContextHandlerInterface defines the interface for context operations
type ContextHandler interface {
	GetContext() (string, error)     // Retrieves the current context
	SetContext(context string) error // Sets the current context
	GetConfigRoot() (string, error)  // Retrieves the configuration root path
}

// BaseContextHandler implements the ContextHandlerInterface
type BaseContextHandler struct {
	ConfigHandler config.ConfigHandler // Handles configuration operations
	Shell         shell.Shell          // Handles shell operations
}

// NewContextHandler creates a new ContextHandler instance
func NewBaseContextHandler(configHandler config.ConfigHandler, shell shell.Shell) *BaseContextHandler {
	return &BaseContextHandler{
		ConfigHandler: configHandler,
		Shell:         shell,
	}
}

// GetContext retrieves the current context from the configuration
func (c *BaseContextHandler) GetContext() (string, error) {
	context, err := c.ConfigHandler.Get("context")
	if err != nil {
		return "", fmt.Errorf("error retrieving context: %w", err)
	}
	if context == nil {
		return "local", nil
	}
	return context.(string), nil
}

// SetContext sets the current context in the configuration and saves it
func (c *BaseContextHandler) SetContext(context string) error {
	if err := c.ConfigHandler.Set("context", context); err != nil {
		return fmt.Errorf("error setting context: %w", err)
	}
	if err := c.ConfigHandler.SaveConfig(""); err != nil {
		return fmt.Errorf("error saving config: %w", err)
	}
	return nil
}

// GetConfigRoot retrieves the configuration root path based on the current context
func (c *BaseContextHandler) GetConfigRoot() (string, error) {
	context, err := c.GetContext()
	if err != nil {
		return "", fmt.Errorf("error retrieving context: %w", err)
	}

	projectRoot, err := c.Shell.GetProjectRoot()
	if err != nil {
		return "", fmt.Errorf("error retrieving project root: %w", err)
	}

	configRoot := filepath.Join(projectRoot, "contexts", context)
	return configRoot, nil
}
