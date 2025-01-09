package config

import (
	"fmt"
	"path/filepath"

	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
)

const (
	windsorDirName  = ".windsor"
	contextDirName  = "contexts"
	contextFileName = "context"
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
	// GetContext retrieves the current context
	GetContext() string

	// SetContext sets the current context
	SetContext(context string) error

	// GetConfigRoot retrieves the configuration root path based on the current context
	GetConfigRoot() (string, error)

	// Clean cleans up context specific artifacts
	Clean() error
}

// BaseConfigHandler is a base implementation of the ConfigHandler interface
type BaseConfigHandler struct {
	ConfigHandler
	injector di.Injector
	shell    shell.Shell
	context  string
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

// GetContext retrieves the current context from the file or cache
func (c *BaseConfigHandler) GetContext() string {
	if c.context != "" {
		return c.context
	}

	projectRoot, err := c.shell.GetProjectRoot()
	if err != nil {
		return "local"
	}

	contextFilePath := filepath.Join(projectRoot, windsorDirName, contextFileName)
	data, err := osReadFile(contextFilePath)
	if err != nil {
		return "local"
	}

	c.context = string(data)
	return c.context
}

// SetContext sets the current context in the file and updates the cache
func (c *BaseConfigHandler) SetContext(context string) error {
	projectRoot, err := c.shell.GetProjectRoot()
	if err != nil {
		return fmt.Errorf("error getting project root: %w", err)
	}

	contextDirPath := filepath.Join(projectRoot, windsorDirName)
	if err := osMkdirAll(contextDirPath, 0755); err != nil {
		return fmt.Errorf("error ensuring context directory exists: %w", err)
	}

	contextFilePath := filepath.Join(contextDirPath, contextFileName)
	err = osWriteFile(contextFilePath, []byte(context), 0644)
	if err != nil {
		return fmt.Errorf("error writing context to file: %w", err)
	}

	c.context = context
	return nil
}

// GetConfigRoot retrieves the configuration root path based on the current context
func (c *BaseConfigHandler) GetConfigRoot() (string, error) {
	context := c.GetContext()

	projectRoot, err := c.shell.GetProjectRoot()
	if err != nil {
		return "", err
	}

	configRoot := filepath.Join(projectRoot, contextDirName, context)
	return configRoot, nil
}

// Clean cleans up context specific artifacts
func (c *BaseConfigHandler) Clean() error {
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
