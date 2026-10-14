package config

import (
	"fmt"
	"maps"
	"path/filepath"
	"strings"
	"sync"

	cliconfig "github.com/windsorcli/cli/api/config/v1alpha2"
	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// =============================================================================
// Constants
// =============================================================================

// contextIDCharset is the character set GenerateContextID draws its random suffix from.
const contextIDCharset = "abcdefghijklmnopqrstuvwxyz0123456789"

// The ConfigHandler is a core component that manages configuration state and context across the application.
// It provides a unified interface for loading, saving, and accessing configuration data, with support for
// multiple contexts and secret management. The handler facilitates environment-specific configurations,
// manages context switching, and integrates with various secret providers for secure credential handling.
// It maintains configuration persistence through YAML files and supports hierarchical configuration
// structures with default values and context-specific overrides.

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

const (
	windsorDirName  = ".windsor"
	contextDirName  = "contexts"
	contextFileName = "context"
)

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

// LoadConfigString loads YAML configuration directly into the internal data map for testing purposes.
// It unmarshals the provided YAML string and, if a "contexts" key exists, extracts and merges only
// the configuration for the current context. If no "contexts" structure is present, it merges the entire
// map. It then converts v1alpha1 values into the v1alpha2 header. This method is primarily intended for
// test helpers and not for production use; load configuration in production with LoadConfig instead.
// Returns an error if YAML unmarshalling fails.
func (c *configHandler) LoadConfigString(content string) error {
	if content == "" {
		return nil
	}

	var dataMap map[string]any
	if err := c.shims.YamlUnmarshal([]byte(content), &dataMap); err != nil {
		return fmt.Errorf("error unmarshalling yaml: %w", err)
	}

	contextName := c.GetContext()

	if contexts, ok := dataMap["contexts"]; ok {
		var contextsMap map[string]any
		switch v := contexts.(type) {
		case map[string]any:
			contextsMap = v
		case map[any]any:
			contextsMap = make(map[string]any)
			for k, val := range v {
				if strKey, ok := k.(string); ok {
					contextsMap[strKey] = val
				}
			}
		}

		if contextsMap != nil {
			if contextData, ok := contextsMap[contextName]; ok {
				var contextMap map[string]any
				switch v := contextData.(type) {
				case map[string]any:
					contextMap = v
				case map[any]any:
					contextMap = c.convertInterfaceMap(v)
				}

				if contextMap != nil {
					c.data = c.deepMerge(c.data, contextMap)
					convertV1alpha1(c.data)
					c.loaded = true
					return nil
				}
			}
		}
	}

	c.data = c.deepMerge(c.data, dataMap)
	convertV1alpha1(c.data)
	c.loaded = true

	return nil
}

// LoadConfig loads and merges all configuration sources for the current context into the internal data map.
// It performs the following actions, in order: loads schema defaults (if available); merges root windsor.yaml
// context section (if it exists); merges any context-specific windsor.yaml/yml file; then merges values.yaml for
// dynamic fields, validating values.yaml against the loaded schema (if one is present). All configuration is
// accumulated into one map structure, and v1alpha1 values convert into the v1alpha2 header. If any file is loaded,
// config state is marked as loaded. Returns an error for any I/O or validation failure. Must call Initialize() first.
func (c *configHandler) LoadConfig() error {
	if c.shell == nil {
		return fmt.Errorf("shell not initialized")
	}
	return c.LoadConfigForContext(c.GetContext())
}

// LoadConfigForContext loads and merges all configuration sources for the specified context into the internal data map
// without modifying the current context state. This method performs the same actions as LoadConfig but uses the
// provided contextName parameter directly instead of reading from the current context. It does not write to the
// .windsor/context file or set the WINDSOR_CONTEXT environment variable, making it safe for read-only operations
// like listing contexts. Returns an error for any I/O or validation failure.
func (c *configHandler) LoadConfigForContext(contextName string) error {
	if c.shell == nil {
		return fmt.Errorf("shell not initialized")
	}
	projectRoot, err := c.shell.GetProjectRoot()
	if err != nil {
		return fmt.Errorf("error retrieving project root: %w", err)
	}

	hasLoadedFiles := false

	if c.schemaValidator != nil && c.schemaValidator.Schema == nil {
		schemaPath := filepath.Join(projectRoot, "contexts", "_template", "schema.yaml")
		if _, err := c.shims.Stat(schemaPath); err == nil {
			if err := c.LoadSchema(schemaPath); err != nil {
				return fmt.Errorf("error loading schema: %w", err)
			}
		}
	}

	rootValues, rootLoaded, err := c.typed.LoadRoot(projectRoot, contextName)
	if err != nil {
		return err
	}
	if rootLoaded {
		hasLoadedFiles = true
	}
	if rootValues != nil {
		c.data = c.deepMerge(c.data, rootValues)
	}

	contextValues, contextLoaded, err := c.typed.LoadContext(projectRoot, contextName)
	if err != nil {
		return err
	}
	if contextLoaded {
		hasLoadedFiles = true
	}
	if contextValues != nil {
		c.data = c.deepMerge(c.data, contextValues)
	}

	workstationValues, _, err := c.workstation.Load(projectRoot, contextName)
	if err != nil {
		return err
	}
	if workstationValues != nil {
		c.data = c.deepMerge(c.data, workstationValues)
	}

	valuesData, valuesLoaded, err := c.values.Load(projectRoot, contextName)
	if err != nil {
		return err
	}
	if valuesLoaded {
		hasLoadedFiles = true
	}
	if valuesData != nil {
		c.data = c.deepMerge(c.data, valuesData)
	}
	convertV1alpha1(c.data)

	if hasLoadedFiles {
		c.loaded = true
	}

	return nil
}

// SaveConfig writes the current configuration to values.yaml for the context, creating the root
// windsor.yaml first if missing. A missing values.yaml gets the full config; an existing one is
// patched at Set-touched paths when overwrite is true, and left alone otherwise. A successful
// write clears the pending Set paths, so a later SaveConfig only patches what changed since.
func (c *configHandler) SaveConfig(overwrite ...bool) error {
	if c.shell == nil {
		return fmt.Errorf("shell not initialized")
	}

	shouldOverwrite := false
	if len(overwrite) > 0 {
		shouldOverwrite = overwrite[0]
	}

	projectRoot, err := c.shell.GetProjectRoot()
	if err != nil {
		return fmt.Errorf("error retrieving project root: %w", err)
	}

	if !c.shell.IsGlobal() {
		if err := c.typed.EnsureRoot(projectRoot); err != nil {
			return err
		}
	}

	deletes := make([]string, 0, len(c.pendingDeletes))
	for key := range c.pendingDeletes {
		deletes = append(deletes, key)
	}

	wrote, err := c.values.Save(
		projectRoot,
		c.GetContext(),
		c.data,
		c.pendingOverrides,
		deletes,
		shouldOverwrite,
		c.getPersistencePolicyInput(),
	)
	if err != nil {
		return err
	}
	if wrote {
		c.pendingOverrides = nil
		c.pendingDeletes = nil
	}

	return nil
}

// SaveWorkstationState extracts workstation-managed keys from config data and writes them
// to .windsor/contexts/<context>/workstation.yaml. This is the canonical persistence point for system-derived
// workstation configuration (runtime, address, platform, DNS settings, network CIDR block).
func (c *configHandler) SaveWorkstationState() error {
	if c.shell == nil {
		return fmt.Errorf("shell not initialized")
	}
	projectRoot, err := c.shell.GetProjectRoot()
	if err != nil {
		return fmt.Errorf("error retrieving project root: %w", err)
	}
	return c.workstation.Save(
		projectRoot,
		c.GetContext(),
		c.data,
		c.getPersistencePolicyInput(),
	)
}

// SetDefault merges the default context into the handler's data. Values already present in data
// take precedence over the default, so this is safe to call regardless of load order or how many
// times it runs.
func (c *configHandler) SetDefault(context cliconfig.Context) error {
	contextData, err := c.shims.YamlMarshal(context)
	if err != nil {
		return fmt.Errorf("error marshalling context: %w", err)
	}

	var contextMap map[string]any
	if err := c.shims.YamlUnmarshal(contextData, &contextMap); err != nil {
		return fmt.Errorf("error decoding context: %w", err)
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
		return fmt.Errorf("error getting project root: %w", err)
	}

	contextDirPath := filepath.Join(projectRoot, windsorDirName)
	if err := c.shims.MkdirAll(contextDirPath, 0755); err != nil {
		return fmt.Errorf("error ensuring context directory exists: %w", err)
	}

	contextFilePath := filepath.Join(contextDirPath, contextFileName)
	err = c.shims.WriteFile(contextFilePath, []byte(context), 0644)
	if err != nil {
		return fmt.Errorf("error writing context to file: %w", err)
	}

	if err := c.shims.Setenv("WINDSOR_CONTEXT", context); err != nil {
		return fmt.Errorf("error setting WINDSOR_CONTEXT environment variable: %w", err)
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
		return fmt.Errorf("error getting windsor scratch path: %w", err)
	}

	dirsToDelete := []string{".terraform", ".tfstate"}

	for _, dir := range dirsToDelete {
		path := filepath.Join(windsorScratchPath, dir)
		if _, err := c.shims.Stat(path); err == nil {
			if err := c.shims.RemoveAll(path); err != nil {
				return fmt.Errorf("error deleting %s: %w", path, err)
			}
		}
	}

	return nil
}

// IsLoaded checks if the configuration has been loaded
func (c *configHandler) IsLoaded() bool {
	return c.loaded
}

// GetSchema returns a shallow copy of the loaded schema map, or nil if no schema is loaded.
// A copy is returned so callers cannot mutate the validator's internal schema state.
func (c *configHandler) GetSchema() map[string]any {
	if c.schemaValidator == nil {
		return nil
	}
	src := c.schemaValidator.Schema
	if src == nil {
		return nil
	}
	out := make(map[string]any, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

// GetSensitivePaths returns the dotted config paths marked `sensitive: true` in the loaded
// schema, or nil if no schema is loaded. Consumers scrub these values from plaintext output
// and gate which properties may back a cluster Secret.
func (c *configHandler) GetSensitivePaths() []string {
	if c.schemaValidator == nil {
		return nil
	}
	return c.schemaValidator.GetSensitivePaths()
}

// IsSensitivePath reports whether the given dotted config path is marked sensitive in the loaded
// schema. A "*" segment in a schema path (from a free-form map region) matches any single
// segment of the queried path.
func (c *configHandler) IsSensitivePath(path string) bool {
	for _, sensitive := range c.GetSensitivePaths() {
		if sensitivePathMatches(sensitive, path) {
			return true
		}
	}
	return false
}

// LoadSchema reads the schema file at schemaPath and loads it as LoadSchemaFromBytes does.
// Returns an error if the file cannot be read or the schema is invalid.
func (c *configHandler) LoadSchema(schemaPath string) error {
	if c.schemaValidator == nil {
		return fmt.Errorf("runtime not fully initialized: schema validator missing")
	}
	schemaContent, err := c.shims.ReadFile(schemaPath)
	if err != nil {
		return fmt.Errorf("failed to read schema file %s: %w", schemaPath, err)
	}
	return c.LoadSchemaFromBytes(schemaContent)
}

// LoadSchemaFromBytes merges a schema fragment into the loaded schema, then merges the CLI
// fragment over the result. It first rejects a fragment that gives a CLI-owned path a different
// type, and leaves the loaded schema unchanged in that case. After the merge, values that Set
// converted from strings convert again against the new schema.
func (c *configHandler) LoadSchemaFromBytes(schemaContent []byte) error {
	if c.schemaValidator == nil {
		return fmt.Errorf("runtime not fully initialized: schema validator missing")
	}
	if err := checkCLIOwnedTypes(schemaContent); err != nil {
		return err
	}
	if err := c.schemaValidator.LoadSchemaFromBytes(schemaContent); err != nil {
		return err
	}
	if err := c.applyCLISchema(); err != nil {
		return err
	}
	c.reconvertStringSets()
	return nil
}

// ValidateContextValues runs schema validation on the full context configuration map. Cross-field
// rules spanning header and dynamic fields are evaluated against the unified map,
// matching the file-load validation path. Use before impactful operations (e.g. windsor up) and after
// compositional Set sequences. Returns nil if no schema is loaded or if validation passes; returns an
// error if validation fails. When the same invalid data was already warned about by valuesSource.Load
// earlier in this invocation, the error refers back to that warning instead of re-printing the
// identical error list a second time. It also checks the id field. This check runs even with no
// schema loaded, so an explicit --set id=... or a checked-in values.yaml fails immediately.
func (c *configHandler) ValidateContextValues() error {
	if err := validateContextID(c.GetString("id")); err != nil {
		return fmt.Errorf("context value validation failed: %w", err)
	}

	if c.schemaValidator == nil || c.schemaValidator.Schema == nil {
		return nil
	}
	if result, err := c.schemaValidator.Validate(c.data); err != nil {
		return fmt.Errorf("error validating context values: %w", err)
	} else if !result.Valid {
		if c.schemaValidator.ErrorsAlreadyReported(result.Errors) {
			return fmt.Errorf("context value validation failed (see the values.yaml warning above)")
		}
		return fmt.Errorf("context value validation failed:%s", FormatValidationErrors(result.Errors))
	}
	return nil
}

// GenerateContextID generates a random context ID if one doesn't exist
func (c *configHandler) GenerateContextID() error {
	if c.GetString("id") != "" {
		return nil
	}

	b := make([]byte, 7)
	if _, err := c.shims.CryptoRandRead(b); err != nil {
		return fmt.Errorf("failed to generate random context ID: %w", err)
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

// getPersistencePolicyInput builds policy input for persistence ownership decisions.
func (c *configHandler) getPersistencePolicyInput() persistencePolicyInput {
	contextName := c.GetContext()
	return persistencePolicyInput{
		IsDevMode:          c.IsDevMode(contextName),
		WorkstationRuntime: c.GetString("workstation.runtime"),
	}
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

// =============================================================================
// Helpers
// =============================================================================

// validateContextID checks id against DNS label rules (RFC 1123): a lowercase letter, then any
// mix of lowercase letters, digits, and hyphens, with no leading, trailing, or repeated hyphen.
// This keeps a user-supplied id safe to embed in a domain name (e.g. id.local). An empty id is
// valid; GenerateContextID fills it in later.
func validateContextID(id string) error {
	if id == "" {
		return nil
	}
	if id[0] < 'a' || id[0] > 'z' {
		return fmt.Errorf("id %q must start with a lowercase letter (a-z)", id)
	}
	if id[len(id)-1] == '-' {
		return fmt.Errorf("id %q must not end with a hyphen", id)
	}
	for i := 1; i < len(id); i++ {
		c := id[i]
		isLower := c >= 'a' && c <= 'z'
		isDigit := c >= '0' && c <= '9'
		if !isLower && !isDigit && c != '-' {
			return fmt.Errorf("id %q must contain only lowercase letters, digits, and hyphens", id)
		}
		if c == '-' && id[i-1] == '-' {
			return fmt.Errorf("id %q must not repeat a hyphen", id)
		}
	}
	return nil
}

// Ensure configHandler implements ConfigHandler
var _ ConfigHandler = (*configHandler)(nil)
