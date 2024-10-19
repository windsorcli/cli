package context

import (
	"fmt"
	"path/filepath"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/shell"
)

// ContextInterface defines the interface for context operations
type ContextInterface interface {
	GetContext() (string, error)     // Retrieves the current context
	SetContext(context string) error // Sets the current context
	GetConfigRoot() (string, error)  // Retrieves the configuration root path
}

// Context implements the ContextInterface
type Context struct {
	ConfigHandler config.ConfigHandler // Handles configuration operations
	Shell         shell.Shell          // Handles shell operations
}

// NewContext creates a new Context instance
func NewContext(cliConfigHandler config.ConfigHandler, shell shell.Shell) *Context {
	return &Context{
		ConfigHandler: cliConfigHandler,
		Shell:         shell,
	}
}

// GetContext retrieves the current context from the configuration
func (c *Context) GetContext() (string, error) {
	context, err := c.ConfigHandler.GetString("context")
	if err != nil {
		return "", fmt.Errorf("error retrieving context: %w", err)
	}
	if context == "" {
		return "local", nil
	}
	return context, nil
}

// SetContext sets the current context in the configuration and saves it
func (c *Context) SetContext(context string) error {
	if err := c.ConfigHandler.Set("context", context); err != nil {
		return fmt.Errorf("error setting context: %w", err)
	}
	return nil
}

// GetConfigRoot retrieves the configuration root path based on the current context
func (c *Context) GetConfigRoot() (string, error) {
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
