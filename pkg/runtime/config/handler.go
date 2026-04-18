package config

import (
	"embed"
	"fmt"
	"maps"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/runtime/shell"
)

//go:embed schemas/*.yaml
var systemSchemasFS embed.FS

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
	GetInt(key string, defaultValue ...int) int
	GetBool(key string, defaultValue ...bool) bool
	GetStringSlice(key string, defaultValue ...[]string) []string
	GetStringMap(key string, defaultValue ...map[string]string) map[string]string
	Set(key string, value any) error
	Get(key string) any
	SaveConfig(overwrite ...bool) error
	SaveWorkstationState() error
	SetDefault(context v1alpha1.Context) error
	GetConfig() *v1alpha1.Context
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
	RegisterProvider(prefix string, provider ValueProvider)
	ValidateContextValues() error
}

// ValueProvider defines the interface for dynamic value providers that can resolve
// configuration keys with special prefixes (e.g., terraform.*, cluster.*).
type ValueProvider interface {
	GetValue(key string) (any, error)
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
	defaultConfig   *v1alpha1.Context
	providers       map[string]ValueProvider
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
		shell:     shell,
		shims:     NewShims(),
		data:      make(map[string]any),
		providers: make(map[string]ValueProvider),
	}

	handler.schemaValidator = NewSchemaValidator(shell)
	handler.schemaValidator.Shims = handler.shims
	handler.policy = newPersistencePolicy()
	handler.workstation = newWorkstationSource(handler.shims, handler.policy)
	handler.values = newValuesSource(handler.shims, handler.schemaValidator, handler.policy)
	handler.typed = newTypedSource(handler.shims, handler.schemaValidator)

	return handler
}

// LoadConfigString loads YAML configuration directly into the internal data map for testing purposes.
// It unmarshals the provided YAML string and, if a "contexts" key exists, extracts and merges only
// the configuration for the current context. If no "contexts" structure is present, it merges the entire
// map. This method is primarily intended for test helpers and not for production use; load configuration
// in production with LoadConfig instead. Returns an error if YAML unmarshalling fails.
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
					c.loaded = true
					return nil
				}
			}
		}
	}

	c.data = c.deepMerge(c.data, dataMap)
	c.loaded = true

	return nil
}

// LoadConfig loads and merges all configuration sources for the current context into the internal data map.
// It performs the following actions, in order: loads schema defaults (if available); merges root windsor.yaml
// context section (if it exists); merges any context-specific windsor.yaml/yml file; then merges values.yaml for
// dynamic fields, validating values.yaml against the loaded schema (if one is present). All configuration is
// accumulated into one map structure. If any file is loaded, config state is marked as loaded. Returns an error
// for any I/O or validation failure. Must call Initialize() first.
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

	rootValues, rootLoaded, err := c.typed.LoadRoot(projectRoot, contextName, c.LoadSchemaFromBytes)
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

	if hasLoadedFiles {
		c.loaded = true
	}

	return nil
}

// SaveConfig writes the current configuration state to a single values.yaml file within the
// context directory under the project root. The root windsor.yaml is created if missing.
// Context-level windsor.yaml is no longer generated; all context configuration goes to values.yaml.
// If overwrite is specified, an existing values.yaml will be overwritten; otherwise, it is only
// created if missing. Returns an error if writing fails or required dependencies are uninitialized.
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

	if err := c.values.Save(
		projectRoot,
		c.GetContext(),
		c.data,
		shouldOverwrite,
		c.getPersistencePolicyInput(),
	); err != nil {
		return err
	}

	return nil
}

// SaveWorkstationState extracts workstation-managed keys from config data and writes them
// to .windsor/contexts/<context>/workstation.yaml. This is the canonical persistence point for system-derived
// workstation configuration (runtime, address, platform, DNS settings).
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

// SetDefault sets the default context configuration in the config handler's internal data.
// It marshals the given v1alpha1.Context struct to a map and merges it into the handler's data.
// This method is typically used during initialization when context files do not yet exist.
// The original default config is stored so it can be used when saving a new config file to ensure
// all default values are preserved even if they were empty/nil (which would be omitted by omitempty tags).
func (c *configHandler) SetDefault(context v1alpha1.Context) error {
	contextData, err := c.shims.YamlMarshal(context)
	if err != nil {
		return fmt.Errorf("error marshalling context: %w", err)
	}

	var contextMap map[string]any
	if err := c.shims.YamlUnmarshal(contextData, &contextMap); err != nil {
		return fmt.Errorf("error unmarshalling context to map: %w", err)
	}

	c.data = c.deepMerge(c.data, contextMap)

	contextCopy := context
	c.defaultConfig = &contextCopy

	return nil
}

// GetConfig returns the context configuration as a v1alpha1.Context struct by marshalling
// the configHandler's internal data map to YAML and then unmarshalling it into the struct.
// This provides backward compatibility for code that relies on the statically typed Context.
// Returns a pointer to an empty Context if marshaling or unmarshaling fails.
func (c *configHandler) GetConfig() *v1alpha1.Context {
	contextData, err := c.shims.YamlMarshal(c.data)
	if err != nil {
		return &v1alpha1.Context{}
	}

	var context v1alpha1.Context
	if err := c.shims.YamlUnmarshal(contextData, &context); err != nil {
		return &v1alpha1.Context{}
	}

	return &context
}

// GetContext retrieves the current context, checking (in priority order):
// 1. The in-memory context override set by SetContextName (used by windsor test for isolation)
// 2. The .windsor/context file (written by windsor set context / windsor init)
// 3. The WINDSOR_CONTEXT environment variable
// 4. The default value "local"
// File takes precedence over env var so that commands run immediately after
// "windsor set context" use the new context before the shell hook has a chance to update
// the WINDSOR_CONTEXT variable in the user's shell.
func (c *configHandler) GetContext() string {
	if c.context != "" {
		return c.context
	}

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

// WithContext returns a new ConfigHandler that is a shallow copy of the receiver with an
// in-memory context override applied. The override takes highest priority in GetContext,
// bypassing the .windsor/context file and the WINDSOR_CONTEXT env var. The original handler
// is not modified. Maps (data, providers) are copied shallowly to prevent aliasing.
// Use this for ephemeral overrides (e.g. windsor test) that must not touch the filesystem.
func (c *configHandler) WithContext(name string) ConfigHandler {
	cp := *c
	cp.context = name
	cp.data = maps.Clone(c.data)
	cp.providers = maps.Clone(c.providers)
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

// SetContext sets the current context in the file and updates the cache
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

	return nil
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

// Clean cleans up context specific artifacts
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

	configRoot, err := c.GetConfigRoot()
	if err != nil {
		return fmt.Errorf("error getting config root: %w", err)
	}

	dirsToDeleteFromConfigRoot := []string{".kube", ".talos", ".omni", ".aws", ".gcp"}

	for _, dir := range dirsToDeleteFromConfigRoot {
		path := filepath.Join(configRoot, dir)
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

// LoadSchema loads the schema.yaml file from the specified directory.
// System-level schema plugins (like substitutions) are applied after loading.
// Returns error if schema file doesn't exist or is invalid.
func (c *configHandler) LoadSchema(schemaPath string) error {
	if c.schemaValidator == nil {
		return fmt.Errorf("schema validator not initialized")
	}
	if err := c.schemaValidator.LoadSchema(schemaPath); err != nil {
		return err
	}
	c.applySystemSchemaPlugins()
	return nil
}

// LoadSchemaFromBytes loads schema directly from byte content.
// If a schema already exists, the new schema is merged into it with the new schema's properties
// overriding existing properties with the same name. If no schema exists, it loads the new schema.
// System-level schema plugins (like substitutions) are applied after loading.
// Returns error if schema content is invalid.
func (c *configHandler) LoadSchemaFromBytes(schemaContent []byte) error {
	if c.schemaValidator == nil {
		return fmt.Errorf("schema validator not initialized")
	}
	if err := c.schemaValidator.LoadSchemaFromBytes(schemaContent); err != nil {
		return err
	}
	c.applySystemSchemaPlugins()
	return nil
}

// ValidateContextValues runs schema validation on the current context's dynamic fields.
// Use before impactful operations (e.g. windsor up) so invalid or typo'd values.yaml fails fast.
// Returns nil if no schema is loaded or if validation passes; returns an error if validation fails.
func (c *configHandler) ValidateContextValues() error {
	if c.schemaValidator == nil || c.schemaValidator.Schema == nil {
		return nil
	}
	_, dynamicFields := c.separateStaticAndDynamicFields(c.data)
	if result, err := c.schemaValidator.Validate(dynamicFields); err != nil {
		return fmt.Errorf("error validating context values: %w", err)
	} else if !result.Valid {
		return fmt.Errorf("context value validation failed: %v", result.Errors)
	}
	return nil
}

// GenerateContextID generates a random context ID if one doesn't exist
func (c *configHandler) GenerateContextID() error {
	if c.GetString("id") != "" {
		return nil
	}

	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 7)
	if _, err := c.shims.CryptoRandRead(b); err != nil {
		return fmt.Errorf("failed to generate random context ID: %w", err)
	}

	for i := range b {
		b[i] = charset[int(b[i])%len(charset)]
	}

	id := "w" + string(b)
	return c.Set("id", id)
}

// =============================================================================
// Private Methods
// =============================================================================

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

// mapToContext converts a map to a v1alpha1.Context struct by marshaling and unmarshaling.
// This ensures that yaml tags (like yaml:"-") are respected when saving to files.
func (c *configHandler) mapToContext(data map[string]any) *v1alpha1.Context {
	contextData, err := c.shims.YamlMarshal(data)
	if err != nil {
		return &v1alpha1.Context{}
	}

	var context v1alpha1.Context
	if err := c.shims.YamlUnmarshal(contextData, &context); err != nil {
		return &v1alpha1.Context{}
	}

	return &context
}

// separateStaticAndDynamicFields splits the data map into static fields (matching v1alpha1.Context schema)
// and dynamic fields (everything else). This is used when saving to separate windsor.yaml from values.yaml.
func (c *configHandler) separateStaticAndDynamicFields(data map[string]any) (static map[string]any, dynamic map[string]any) {
	static = make(map[string]any)
	dynamic = make(map[string]any)

	for key, value := range data {
		if c.isKeyInStaticSchema(key) {
			static[key] = value
		} else {
			dynamic[key] = value
		}
	}

	return static, dynamic
}

// isKeyInStaticSchema determines whether the provided key exists as a top-level field
// in the static windsor.yaml schema, represented by v1alpha1.Context. It checks both
// direct keys and those nested with dot notation (e.g., "environment.TEST_VAR" -> "environment").
// Returns true if the top-level key matches a YAML field tag in v1alpha1.Context, false otherwise.
func (c *configHandler) isKeyInStaticSchema(key string) bool {
	topLevelKey := key
	if dotIndex := strings.Index(key, "."); dotIndex != -1 {
		topLevelKey = key[:dotIndex]
	}
	contextType := reflect.TypeOf(v1alpha1.Context{})
	for i := 0; i < contextType.NumField(); i++ {
		field := contextType.Field(i)
		yamlTag := strings.Split(field.Tag.Get("yaml"), ",")[0]
		if yamlTag == topLevelKey {
			return true
		}
	}
	return false
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

// applySystemSchemaPlugins applies system-level schema plugins to the loaded schema.
// These plugins load schema definitions from embedded YAML files in schemas/ directory
// and merge them into the loaded schema. System schemas define features like substitutions
// that are always available regardless of the blueprint schema definition.
// This method is extensible - add new schema.yaml files to schemas/ to include additional system schemas.
func (c *configHandler) applySystemSchemaPlugins() {
	if c.schemaValidator == nil || c.schemaValidator.Schema == nil {
		return
	}

	entries, err := systemSchemasFS.ReadDir("schemas")
	if err != nil {
		return
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}

		schemaPath := "schemas/" + entry.Name()
		schemaContent, err := systemSchemasFS.ReadFile(schemaPath)
		if err != nil {
			continue
		}

		if err := c.schemaValidator.LoadSchemaFromBytes(schemaContent); err != nil {
			continue
		}
	}
}

// Ensure configHandler implements ConfigHandler
var _ ConfigHandler = (*configHandler)(nil)
