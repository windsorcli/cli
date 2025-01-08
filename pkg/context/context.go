package context

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
	injector di.Injector // Handles dependency injection
	shell    shell.Shell // Handles shell operations
	context  string      // Cached context value
}

// NewContextHandler creates a new ContextHandler instance
func NewContextHandler(injector di.Injector) *BaseContextHandler {
	return &BaseContextHandler{injector: injector}
}

// Initialize initializes the context handler
func (c *BaseContextHandler) Initialize() error {
	// Resolve the shell
	shell, ok := c.injector.Resolve("shell").(shell.Shell)
	if !ok {
		return fmt.Errorf("error resolving shell")
	}
	c.shell = shell

	return nil
}

// GetContext retrieves the current context from the file or cache
func (c *BaseContextHandler) GetContext() string {
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
func (c *BaseContextHandler) SetContext(context string) error {
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
func (c *BaseContextHandler) GetConfigRoot() (string, error) {
	context := c.GetContext()

	projectRoot, err := c.shell.GetProjectRoot()
	if err != nil {
		return "", err
	}

	configRoot := filepath.Join(projectRoot, contextDirName, context)
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
