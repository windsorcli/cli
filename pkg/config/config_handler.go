package config

import (
	"fmt"

	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
)

// ConfigHandler defines the interface for handling configuration operations
type ConfigHandler interface {
	// Initialize initializes the config handler
	Initialize() error

	// LoadConfig loads the configuration from the specified path
	LoadConfig(path string) error

	// GetString retrieves a string value for the specified key from the configuration
	GetString(key string, defaultValue ...string) string

	// GetInt retrieves an integer value for the specified key from the configuration
	GetInt(key string, defaultValue ...int) int

	// GetBool retrieves a boolean value for the specified key from the configuration
	GetBool(key string, defaultValue ...bool) bool

	// Set sets the value for the specified key in the configuration
	Set(key string, value interface{}) error

	// SetContextValue sets the value for the specified key in the configuration
	SetContextValue(key string, value interface{}) error

	// Get retrieves a value for the specified key from the configuration
	Get(key string) interface{}

	// SaveConfig saves the current configuration to the specified path
	SaveConfig(path string) error

	// SetDefault sets the default context configuration
	SetDefault(context Context) error

	// GetConfig returns the context config object
	GetConfig() *Context
}

// BaseConfigHandler is a base implementation of the ConfigHandler interface
type BaseConfigHandler struct {
	ConfigHandler
	injector di.Injector
	shell    shell.Shell
}

// NewBaseConfigHandler creates a new BaseConfigHandler instance
func NewBaseConfigHandler(injector di.Injector) *BaseConfigHandler {
	return &BaseConfigHandler{injector: injector}
}

// Initialize sets up the config handler by resolving and storing the shell dependency.
func (c *BaseConfigHandler) Initialize() error {
	shell, ok := c.injector.Resolve("shell").(shell.Shell)
	if !ok {
		return fmt.Errorf("error resolving shell")
	}
	c.shell = shell
	return nil
}
