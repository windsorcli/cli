package context

import (
	"fmt"
	"path/filepath"

	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/shell"
)

type ContextInterface interface {
	GetContext() (string, error)
	SetContext(context string) error
	GetConfigRoot() (string, error)
}

type Context struct {
	ConfigHandler config.ConfigHandler
	Shell         shell.Shell
}

func NewContext(configHandler config.ConfigHandler, shell shell.Shell) *Context {
	return &Context{
		ConfigHandler: configHandler,
		Shell:         shell,
	}
}

func (c *Context) GetContext() (string, error) {
	context, err := c.ConfigHandler.GetConfigValue("context")
	if err != nil {
		return "", fmt.Errorf("error retrieving context: %w", err)
	}
	return context, nil
}

func (c *Context) SetContext(context string) error {
	if err := c.ConfigHandler.SetConfigValue("context", context); err != nil {
		return fmt.Errorf("error setting context: %w", err)
	}
	if err := c.ConfigHandler.SaveConfig(""); err != nil {
		return fmt.Errorf("error saving config: %w", err)
	}
	return nil
}

func (c *Context) GetConfigRoot() (string, error) {
	context, err := c.GetContext()
	if err != nil {
		return "", err
	}

	projectRoot, err := c.Shell.GetProjectRoot()
	if err != nil {
		return "", fmt.Errorf("error retrieving project root: %w", err)
	}

	configRoot := filepath.Join(projectRoot, "contexts", context)
	return configRoot, nil
}
