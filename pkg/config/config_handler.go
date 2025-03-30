package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/secrets"
	"github.com/windsorcli/cli/pkg/shell"
)

const (
	windsorDirName     = ".windsor"
	contextDirName     = "contexts"
	contextFileName    = "context"
	sessionTokenLength = 16
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

	// GetStringSlice retrieves a slice of strings for the specified key from the configuration
	GetStringSlice(key string, defaultValue ...[]string) []string

	// GetStringMap retrieves a map of string key-value pairs for the specified key from the configuration
	GetStringMap(key string, defaultValue ...map[string]string) map[string]string

	// Set sets the value for the specified key in the configuration
	Set(key string, value interface{}) error

	// SetContextValue sets the value for the specified key in the configuration
	SetContextValue(key string, value interface{}) error

	// Get retrieves a value for the specified key from the configuration
	Get(key string) interface{}

	// SaveConfig saves the current configuration to the specified path
	SaveConfig(path string) error

	// SetDefault sets the default context configuration
	SetDefault(context v1alpha1.Context) error

	// GetConfig returns the context config object
	GetConfig() *v1alpha1.Context

	// GetContext retrieves the current context
	// It uses a persistent session token to identify the terminal and ensure each terminal
	// maintains its own context.
	GetContext() string

	// SetContext sets the current context
	SetContext(context string) error

	// GetConfigRoot retrieves the configuration root path based on the current context
	GetConfigRoot() (string, error)

	// Clean cleans up context specific artifacts
	Clean() error

	// SetSecretsProvider sets the secrets provider for the config handler
	SetSecretsProvider(provider secrets.SecretsProvider)

	// IsLoaded checks if the configuration has been loaded
	IsLoaded() bool
}

// BaseConfigHandler is a base implementation of the ConfigHandler interface
type BaseConfigHandler struct {
	ConfigHandler
	injector         di.Injector
	shell            shell.Shell
	config           v1alpha1.Config
	context          string
	secretsProviders []secrets.SecretsProvider
	loaded           bool
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

// SetSecretsProvider sets the secrets provider for the config handler
func (c *BaseConfigHandler) SetSecretsProvider(provider secrets.SecretsProvider) {
	c.secretsProviders = append(c.secretsProviders, provider)
}

// GetContext retrieves the current context for the terminal session. It first checks if a context
// is already cached in memory. If not, it attempts to determine the context by checking a series
// of sources in order of priority: a session-specific context file, an environment variable, and
// a shared context file. The session-specific context file is named using a session token unique
// to the terminal, ensuring that each terminal maintains its own context. If a session-specific
// context is found, it is read, cached, and the file is deleted. If no context is found in any
// of these sources, the function defaults to returning "local" as the context.
func (c *BaseConfigHandler) GetContext() string {
	if c.context != "" {
		return c.context
	}

	projectRoot, err := c.shell.GetProjectRoot()
	if err != nil {
		c.context = "local"
		return c.context
	}

	contextDirPath := filepath.Join(projectRoot, windsorDirName)
	sessionToken := c.shell.GetSessionToken()
	sessionContextPath := filepath.Join(contextDirPath, fmt.Sprintf(".session-%s.ctx", sessionToken))

	if data, err := osReadFile(sessionContextPath); err == nil && len(data) > 0 {
		c.context = strings.TrimSpace(string(data))
		osRemove(sessionContextPath)
		return c.context
	}

	if envContext := os.Getenv("WINDSOR_CONTEXT"); envContext != "" {
		c.context = envContext
		return c.context
	}

	sharedContextPath := filepath.Join(contextDirPath, contextFileName)
	if data, err := osReadFile(sharedContextPath); err == nil && len(data) > 0 {
		c.context = strings.TrimSpace(string(data))
		return c.context
	}

	c.context = "local"
	return c.context
}

// SetContext sets the context for this terminal session. It first clears the cached context
// to ensure the new value is picked up. It then retrieves the project root directory and
// ensures the context directory exists. The context is saved to a session-specific file,
// which is unique to the terminal session, and also to a shared context file that is used
// by new shells. Finally, the in-memory context is updated with the new value.
func (c *BaseConfigHandler) SetContext(context string) error {
	c.context = ""

	projectRoot, err := c.shell.GetProjectRoot()
	if err != nil {
		return fmt.Errorf("error getting project root: %w", err)
	}
	contextDirPath := filepath.Join(projectRoot, windsorDirName)
	if err := osMkdirAll(contextDirPath, 0755); err != nil {
		return fmt.Errorf("error ensuring context directory exists: %w", err)
	}
	sessionToken := c.shell.GetSessionToken()
	sessionContextPath := filepath.Join(contextDirPath, fmt.Sprintf(".session-%s.ctx", sessionToken))
	if err := osWriteFile(sessionContextPath, []byte(context), 0644); err != nil {
		return fmt.Errorf("error writing session-specific context file: %w", err)
	}
	sharedContextPath := filepath.Join(contextDirPath, contextFileName)
	if err := osWriteFile(sharedContextPath, []byte(context), 0644); err != nil {
		return fmt.Errorf("error writing shared context file: %w", err)
	}
	c.context = context
	return nil
}

// SetContext clears the current context, writes the new context to session-specific and shared files,
// and updates the in-memory context for the terminal session.

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

// IsLoaded checks if the configuration has been loaded
func (c *BaseConfigHandler) IsLoaded() bool {
	return c.loaded
}
