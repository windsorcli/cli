package config

import (
	"fmt"
	"path/filepath"

	"github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/secrets"
	"github.com/windsorcli/cli/pkg/shell"
)

// The ConfigHandler is a core component that manages configuration state and context across the application.
// It provides a unified interface for loading, saving, and accessing configuration data, with support for
// multiple contexts and secret management. The handler facilitates environment-specific configurations,
// manages context switching, and integrates with various secret providers for secure credential handling.
// It maintains configuration persistence through YAML files and supports hierarchical configuration
// structures with default values and context-specific overrides.

type ConfigHandler interface {
	Initialize() error
	LoadConfig(path string) error
	LoadConfigString(content string) error
	GetString(key string, defaultValue ...string) string
	GetInt(key string, defaultValue ...int) int
	GetBool(key string, defaultValue ...bool) bool
	GetStringSlice(key string, defaultValue ...[]string) []string
	GetStringMap(key string, defaultValue ...map[string]string) map[string]string
	Set(key string, value any) error
	SetContextValue(key string, value any) error
	Get(key string) any
	SaveConfig(path string) error
	SetDefault(context v1alpha1.Context) error
	GetConfig() *v1alpha1.Context
	GetContext() string
	SetContext(context string) error
	GetConfigRoot() (string, error)
	Clean() error
	IsLoaded() bool
}

const (
	windsorDirName  = ".windsor"
	contextDirName  = "contexts"
	contextFileName = "context"
)

// BaseConfigHandler is a base implementation of the ConfigHandler interface
type BaseConfigHandler struct {
	injector         di.Injector
	shell            shell.Shell
	config           v1alpha1.Config
	context          string
	secretsProviders []secrets.SecretsProvider
	loaded           bool
}

// =============================================================================
// Constructor
// =============================================================================

// NewBaseConfigHandler creates a new BaseConfigHandler instance
func NewBaseConfigHandler(injector di.Injector) *BaseConfigHandler {
	return &BaseConfigHandler{injector: injector}
}

// =============================================================================
// Public Methods
// =============================================================================

// Initialize sets up the config handler by resolving and storing the shell dependency.
func (c *BaseConfigHandler) Initialize() error {
	shell, ok := c.injector.Resolve("shell").(shell.Shell)
	if !ok {
		return fmt.Errorf("error resolving shell")
	}
	c.shell = shell
	return nil
}

// GetContext retrieves the current context from the environment, file, or defaults to "local"
func (c *BaseConfigHandler) GetContext() string {
	contextName := "local"

	envContext := osGetenv("WINDSOR_CONTEXT")
	if envContext != "" {
		c.context = envContext
	} else {
		projectRoot, err := c.shell.GetProjectRoot()
		if err != nil {
			c.context = contextName
		} else {
			contextFilePath := filepath.Join(projectRoot, windsorDirName, contextFileName)
			data, err := osReadFile(contextFilePath)
			if err != nil {
				c.context = contextName
			} else {
				c.context = string(data)
			}
		}
	}

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

// IsLoaded checks if the configuration has been loaded
func (c *BaseConfigHandler) IsLoaded() bool {
	return c.loaded
}
