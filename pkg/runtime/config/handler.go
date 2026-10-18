// The ConfigHandler is the entry point for context configuration in the CLI.
// It provides loading, saving, and access to the values of the current context.
// It resolves the current context from an override, the context file, or the environment.
// It delegates file reads and writes to the typed, values, and workstation sources.

package config

import (
	"maps"
	"path/filepath"
	"strings"
	"sync"

	cliconfig "github.com/windsorcli/cli/api/config/v1alpha2"
	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// =============================================================================
// Constants
// =============================================================================

// contextIDCharset is the character set GenerateContextID draws its random suffix from.
const contextIDCharset = "abcdefghijklmnopqrstuvwxyz0123456789"

const (
	windsorDirName  = ".windsor"
	contextDirName  = "contexts"
	contextFileName = "context"
)

// =============================================================================
// Types
// =============================================================================

// ConfigHandler loads, saves, and reads the configuration of the current context.
type ConfigHandler interface {
	LoadConfig() error
	LoadConfigForContext(contextName string) error
	LoadConfigString(content string) error
	GetString(key string, defaultValue ...string) string
	GetTerraformBackendType() string
	GetInt(key string, defaultValue ...int) int
	GetBool(key string, defaultValue ...bool) bool
	GetStringSlice(key string, defaultValue ...[]string) []string
	GetStringMap(key string, defaultValue ...map[string]string) map[string]string
	Set(key string, value any) error
	Get(key string) any
	SaveConfig(overwrite ...bool) error
	SaveWorkstationState() error
	SetDefault(context cliconfig.Context) error
	GetHeader() *cliconfig.Context
	GetContext() string
	WithContext(name string) ConfigHandler
	IsDevMode(contextName string) bool
	SetContext(context string) error
	GetConfigRoot() (string, error)
	GetWindsorScratchPath() (string, error)
	Clean() error
	IsLoaded() bool
	GenerateContextID() error
	LoadSchema(schemaPath string) error
	LoadSchemaFromBytes(schemaContent []byte) error
	GetSchema() map[string]any
	GetContextValues() (map[string]any, error)
	GetSetValues() map[string]any
	SetApplySchemaDefaults(enabled bool)
	GetSensitivePaths() []string
	IsSensitivePath(path string) bool
	ValidateContextValues() error
}

// configHandler is the concrete implementation of the ConfigHandler interface that provides
// YAML-based configuration management with support for contexts, schemas, and values files.
type configHandler struct {
	shell           shell.Shell
	context         string
	loaded          bool
	shims           *Shims
	schemaValidator *SchemaValidator
	policy          *persistencePolicy
	typed           *typedSource
	values          *valuesSource
	workstation     *workstationSource
	data            map[string]any

	// pendingOverrides and pendingDeletes feed SaveConfig's patch write.
	pendingOverrides map[string]any
	pendingDeletes   map[string]bool

	// applySchemaDefaults forces schema-default materialization in GetContextValues even for a
	// test context, which otherwise skips it. Consumers that want the production composition path
	// under a test context (the facet test runner, per case) set this; it is a no-op elsewhere.
	applySchemaDefaults bool

	// resolvedContext memoizes GetContext's file/env/default lookup; see GetContext.
	resolvedContext *contextCache

	// stringSets keeps the original string of each value Set converted; see reconvertStringSets.
	stringSets map[string]stringSet
}

// stringSet is the original string that Set received at keys, and the value Set stored for it.
type stringSet struct {
	keys    []string
	raw     string
	applied any
}

// contextCache memoizes GetContext's resolved value and guards concurrent access to it and to
// configHandler.context. It is a pointer so WithContext's shallow copy can swap in its own cache
// instead of sharing one.
type contextCache struct {
	mu       sync.RWMutex
	resolved bool
	value    string
}

// =============================================================================
// Constructor
// =============================================================================

// NewConfigHandler creates a new ConfigHandler instance with default context configuration.
func NewConfigHandler(shell shell.Shell) ConfigHandler {
	if shell == nil {
		panic("shell is required")
	}

	handler := &configHandler{
		shell:           shell,
		shims:           NewShims(),
		data:            make(map[string]any),
		resolvedContext: &contextCache{},
	}

	handler.schemaValidator = NewSchemaValidator(shell)
	handler.schemaValidator.Shims = handler.shims
	handler.policy = newPersistencePolicy()
	handler.workstation = newWorkstationSource(handler.shims, handler.policy)
	handler.values = newValuesSource(handler.shims, handler.schemaValidator, handler.policy)
	handler.typed = newTypedSource(handler.shims)

	return handler
}

// =============================================================================
// Public Methods
// =============================================================================

// SetDefault merges the default context into the handler's data. Values already present in data
// take precedence over the default, so this is safe to call regardless of load order or how many
// times it runs.
func (c *configHandler) SetDefault(context cliconfig.Context) error {
	contextData, err := c.shims.YamlMarshal(context)
	if err != nil {
		return werror.Wrap(err, "error marshalling context")
	}

	var contextMap map[string]any
	if err := c.shims.YamlUnmarshal(contextData, &contextMap); err != nil {
		return werror.Wrap(err, "error decoding context")
	}

	c.data = c.deepMerge(contextMap, c.data)

	return nil
}

// GetHeader decodes the handler's data into the v1alpha2 header. Keys the header does not define
// are left out. It returns an empty header when the data does not decode.
func (c *configHandler) GetHeader() *cliconfig.Context {
	contextData, err := c.shims.YamlMarshal(c.data)
	if err != nil {
		return &cliconfig.Context{}
	}

	var header cliconfig.Context
	if err := c.shims.YamlUnmarshal(contextData, &header); err != nil {
		return &cliconfig.Context{}
	}

	return &header
}

// GetContext retrieves the current context, checking (in priority order):
// 1. The in-memory context override set by SetContextName (used by windsor test for isolation)
// 2. The .windsor/context file (written by windsor set context / windsor init)
// 3. The WINDSOR_CONTEXT environment variable
// 4. The default value "local"
// File takes precedence over env var so that commands run immediately after
// "windsor set context" use the new context before the shell hook has a chance to update
// the WINDSOR_CONTEXT variable in the user's shell.
//
// Steps 2-4 run once per handler and cache in resolvedContext. This isolates a running command
// from a concurrent "windsor set context" changing the file mid-run. A new handler still reads
// fresh, so a later command sees the change.
//
// The override field c.context and resolvedContext share one lock (resolvedContext.mu): SetContext
// writes c.context after this handler is already in use, so reading it here needs the same
// synchronization the cache gets.
func (c *configHandler) GetContext() string {
	if c.resolvedContext == nil {
		if c.context != "" {
			return c.context
		}
		return c.resolveContextFromFileOrEnv()
	}

	c.resolvedContext.mu.RLock()
	if c.context != "" {
		defer c.resolvedContext.mu.RUnlock()
		return c.context
	}
	if c.resolvedContext.resolved {
		defer c.resolvedContext.mu.RUnlock()
		return c.resolvedContext.value
	}
	c.resolvedContext.mu.RUnlock()

	c.resolvedContext.mu.Lock()
	defer c.resolvedContext.mu.Unlock()
	if c.context != "" {
		return c.context
	}
	if c.resolvedContext.resolved {
		return c.resolvedContext.value
	}

	c.resolvedContext.value = c.resolveContextFromFileOrEnv()
	c.resolvedContext.resolved = true
	return c.resolvedContext.value
}

// WithContext returns a new ConfigHandler that is a shallow copy of the receiver with an
// in-memory context override applied. The override takes highest priority in GetContext,
// bypassing the .windsor/context file and the WINDSOR_CONTEXT env var. The original handler
// is not modified. The data map is copied shallowly to prevent aliasing. Pending
// Set paths are not carried over: they target the receiver's own values.yaml, not the copy's.
// The copy gets its own resolvedContext cache.
// Use this for ephemeral overrides (e.g. windsor test) that must not touch the filesystem.
func (c *configHandler) WithContext(name string) ConfigHandler {
	cp := *c
	cp.context = name
	cp.data = maps.Clone(c.data)
	cp.pendingOverrides = nil
	cp.pendingDeletes = nil
	cp.stringSets = nil
	cp.resolvedContext = &contextCache{}
	return &cp
}

// IsDevMode checks if the given context name represents a dev/local environment.
// It first checks if "dev" is explicitly set in the configuration, which takes precedence.
// If not set, it falls back to checking if the context name equals "local" or starts with "local-".
func (c *configHandler) IsDevMode(contextName string) bool {
	if devValue := c.Get("dev"); devValue != nil {
		if devBool, ok := devValue.(bool); ok {
			return devBool
		}
	}
	return contextName == "local" || strings.HasPrefix(contextName, "local-")
}

// SetContext writes the current context to the file and env var, and sets this handler's own
// override so GetContext returns the new value immediately, taking priority the same way an
// override from WithContext does. A different handler still sees the change only on its own
// next resolution.
func (c *configHandler) SetContext(context string) error {
	projectRoot, err := c.shell.GetProjectRoot()
	if err != nil {
		return werror.Wrap(err, "error getting project root")
	}

	contextDirPath := filepath.Join(projectRoot, windsorDirName)
	if err := c.shims.MkdirAll(contextDirPath, 0755); err != nil {
		return werror.Wrap(err, "error ensuring context directory exists")
	}

	contextFilePath := filepath.Join(contextDirPath, contextFileName)
	err = c.shims.WriteFile(contextFilePath, []byte(context), 0644)
	if err != nil {
		return werror.Wrap(err, "error writing context to file")
	}

	if err := c.shims.Setenv("WINDSOR_CONTEXT", context); err != nil {
		return werror.Wrap(err, "error setting WINDSOR_CONTEXT environment variable")
	}

	if c.resolvedContext == nil {
		c.context = context
		return nil
	}

	c.resolvedContext.mu.Lock()
	c.context = context
	c.resolvedContext.mu.Unlock()

	return nil
}

// SetApplySchemaDefaults forces GetContextValues to materialize schema defaults even under a test
// context, which otherwise skips them. It exists for the facet test runner to compose a case on the
// production path; in any non-test context schema defaults already apply, so this is a no-op there.
func (c *configHandler) SetApplySchemaDefaults(enabled bool) {
	c.applySchemaDefaults = enabled
}

// GetConfigRoot retrieves the configuration root path based on the current context
func (c *configHandler) GetConfigRoot() (string, error) {
	context := c.GetContext()

	projectRoot, err := c.shell.GetProjectRoot()
	if err != nil {
		return "", err
	}

	configRoot := filepath.Join(projectRoot, contextDirName, context)
	return configRoot, nil
}

// GetWindsorScratchPath retrieves the windsor scratch directory path based on the current context
func (c *configHandler) GetWindsorScratchPath() (string, error) {
	context := c.GetContext()
	projectRoot, err := c.shell.GetProjectRoot()
	if err != nil {
		return "", err
	}
	windsorScratchPath := filepath.Join(projectRoot, windsorDirName, contextDirName, context)
	return windsorScratchPath, nil
}

// Clean removes runtime caches (.terraform, .tfstate) from .windsor/contexts/<name>/. Cluster
// credentials (.kube, .talos) are removed separately during 'down' (see Project.PerformCleanup).
func (c *configHandler) Clean() error {
	windsorScratchPath, err := c.GetWindsorScratchPath()
	if err != nil {
		return werror.Wrap(err, "error getting windsor scratch path")
	}

	dirsToDelete := []string{".terraform", ".tfstate"}

	for _, dir := range dirsToDelete {
		path := filepath.Join(windsorScratchPath, dir)
		if _, err := c.shims.Stat(path); err == nil {
			if err := c.shims.RemoveAll(path); err != nil {
				return werror.Wrap(err, "error deleting %s", path)
			}
		}
	}

	return nil
}

// IsLoaded checks if the configuration has been loaded
func (c *configHandler) IsLoaded() bool {
	return c.loaded
}

// GenerateContextID generates a random context ID if one doesn't exist
func (c *configHandler) GenerateContextID() error {
	if c.GetString("id") != "" {
		return nil
	}

	b := make([]byte, 7)
	if _, err := c.shims.CryptoRandRead(b); err != nil {
		return werror.Wrap(err, "failed to generate random context ID")
	}

	for i := range b {
		b[i] = contextIDCharset[int(b[i])%len(contextIDCharset)]
	}

	id := "w" + string(b)
	return c.Set("id", id)
}

// =============================================================================
// Private Methods
// =============================================================================

// resolveContextFromFileOrEnv resolves context from file, env, or default. GetContext caches
// the result.
func (c *configHandler) resolveContextFromFileOrEnv() string {
	if c.shell != nil {
		projectRoot, err := c.shell.GetProjectRoot()
		if err == nil {
			contextFilePath := filepath.Join(projectRoot, windsorDirName, contextFileName)
			data, err := c.shims.ReadFile(contextFilePath)
			if err == nil {
				if fileContext := strings.TrimSpace(string(data)); fileContext != "" {
					return fileContext
				}
			}
		}
	}

	if envContext := c.shims.Getenv("WINDSOR_CONTEXT"); envContext != "" {
		return envContext
	}

	return "local"
}

// convertInterfaceMap recursively converts map[any]any to map[string]any
func (c *configHandler) convertInterfaceMap(m map[any]any) map[string]any {
	result := make(map[string]any)
	for k, v := range m {
		strKey, ok := k.(string)
		if !ok {
			continue
		}

		switch val := v.(type) {
		case map[any]any:
			result[strKey] = c.convertInterfaceMap(val)
		case map[string]any:
			result[strKey] = val
		default:
			result[strKey] = val
		}
	}
	return result
}

// deepMerge recursively merges two maps with overlay values taking precedence.
// Nested maps are merged rather than replaced. Non-map values in overlay replace base values.
func (c *configHandler) deepMerge(base, overlay map[string]any) map[string]any {
	result := maps.Clone(base)
	for k, overlayValue := range overlay {
		if baseValue, exists := result[k]; exists {
			if baseMap, baseIsMap := baseValue.(map[string]any); baseIsMap {
				if overlayMap, overlayIsMap := overlayValue.(map[string]any); overlayIsMap {
					result[k] = c.deepMerge(baseMap, overlayMap)
					continue
				}
			}
		}
		result[k] = overlayValue
	}
	return result
}

// Ensure configHandler implements ConfigHandler
var _ ConfigHandler = (*configHandler)(nil)
