package config

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"

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
	LoadContextConfig() error
	GetString(key string, defaultValue ...string) string
	GetInt(key string, defaultValue ...int) int
	GetBool(key string, defaultValue ...bool) bool
	GetStringSlice(key string, defaultValue ...[]string) []string
	GetStringMap(key string, defaultValue ...map[string]string) map[string]string
	Set(key string, value any) error
	SetContextValue(key string, value any) error
	Get(key string) any
	SaveConfig(overwrite ...bool) error
	SetDefault(context v1alpha1.Context) error
	GetConfig() *v1alpha1.Context
	GetContext() string
	SetContext(context string) error
	GetConfigRoot() (string, error)
	Clean() error
	IsLoaded() bool
	IsContextConfigLoaded() bool
	SetSecretsProvider(provider secrets.SecretsProvider)
	GenerateContextID() error
	LoadSchema(schemaPath string) error
	LoadSchemaFromBytes(schemaContent []byte) error
	GetSchemaDefaults() (map[string]any, error)
	GetContextValues() (map[string]any, error)
}

const (
	windsorDirName  = ".windsor"
	contextDirName  = "contexts"
	contextFileName = "context"
)

// configHandler is the concrete implementation of the ConfigHandler interface that provides
// YAML-based configuration management with support for contexts, schemas, and values files.
type configHandler struct {
	injector             di.Injector
	shell                shell.Shell
	config               v1alpha1.Config
	context              string
	secretsProviders     []secrets.SecretsProvider
	loaded               bool
	shims                *Shims
	schemaValidator      *SchemaValidator
	contextValues        map[string]any
	path                 string
	defaultContextConfig v1alpha1.Context
	loadedContexts       map[string]bool
}

// =============================================================================
// Constructor
// =============================================================================

// NewConfigHandler creates a new ConfigHandler instance with default context configuration.
func NewConfigHandler(injector di.Injector) ConfigHandler {
	handler := &configHandler{
		injector:       injector,
		shims:          NewShims(),
		contextValues:  make(map[string]any),
		loadedContexts: make(map[string]bool),
	}

	handler.config.Version = "v1alpha1"

	return handler
}

// =============================================================================
// Public Methods
// =============================================================================

// Initialize sets up the config handler by resolving and storing the shell dependency.
func (c *configHandler) Initialize() error {
	shell, ok := c.injector.Resolve("shell").(shell.Shell)
	if !ok {
		return fmt.Errorf("error resolving shell")
	}
	c.shell = shell

	c.schemaValidator = NewSchemaValidator(c.shell)
	c.schemaValidator.Shims = c.shims

	return nil
}

// LoadConfigString loads configuration from a YAML string into the internal config structure.
// It unmarshals the YAML, records which contexts were present in the input, validates and sets
// the config version, and marks the configuration as loaded. Returns an error if unmarshalling
// fails or if the config version is unsupported.
func (c *configHandler) LoadConfigString(content string) error {
	if content == "" {
		return nil
	}

	var tempConfig v1alpha1.Config
	if err := c.shims.YamlUnmarshal([]byte(content), &tempConfig); err != nil {
		return fmt.Errorf("error unmarshalling yaml: %w", err)
	}

	if tempConfig.Contexts != nil {
		for contextName := range tempConfig.Contexts {
			c.loadedContexts[contextName] = true
		}
	}

	if err := c.shims.YamlUnmarshal([]byte(content), &c.config); err != nil {
		return fmt.Errorf("error unmarshalling yaml: %w", err)
	}

	if c.config.Version == "" {
		c.config.Version = "v1alpha1"
	} else if c.config.Version != "v1alpha1" {
		return fmt.Errorf("unsupported config version: %s", c.config.Version)
	}

	return nil
}

// LoadConfig loads the configuration from the specified path. If the file does not exist, it does nothing.
func (c *configHandler) LoadConfig(path string) error {
	c.path = path
	if _, err := c.shims.Stat(path); os.IsNotExist(err) {
		return nil
	}

	data, err := c.shims.ReadFile(path)
	if err != nil {
		return fmt.Errorf("error reading config file: %w", err)
	}

	return c.LoadConfigString(string(data))
}

// LoadContextConfig loads the context-specific windsor.yaml file for the current context.
// It is intended for pipelines that only require static context configuration, not dynamic values.
// The values.yaml file is loaded lazily upon first access via Get() or SetContextValue().
// Returns nil if already loaded, or if loading succeeds; otherwise returns an error for shell or file-related issues.
func (c *configHandler) LoadContextConfig() error {
	if c.loaded {
		return nil
	}
	if c.shell == nil {
		return fmt.Errorf("shell not initialized")
	}

	projectRoot, err := c.shell.GetProjectRoot()
	if err != nil {
		return fmt.Errorf("error retrieving project root: %w", err)
	}

	contextName := c.GetContext()
	contextConfigDir := filepath.Join(projectRoot, "contexts", contextName)

	yamlPath := filepath.Join(contextConfigDir, "windsor.yaml")
	ymlPath := filepath.Join(contextConfigDir, "windsor.yml")

	var contextConfigPath string
	if _, err := c.shims.Stat(yamlPath); err == nil {
		contextConfigPath = yamlPath
	} else if _, err := c.shims.Stat(ymlPath); err == nil {
		contextConfigPath = ymlPath
	}

	if contextConfigPath != "" {
		data, err := c.shims.ReadFile(contextConfigPath)
		if err != nil {
			return fmt.Errorf("error reading context config file: %w", err)
		}

		var contextConfig v1alpha1.Context
		if err := c.shims.YamlUnmarshal(data, &contextConfig); err != nil {
			return fmt.Errorf("error unmarshalling context yaml: %w", err)
		}

		if c.config.Contexts == nil {
			c.config.Contexts = make(map[string]*v1alpha1.Context)
		}

		if c.config.Contexts[contextName] == nil {
			c.config.Contexts[contextName] = &v1alpha1.Context{}
		}

		c.config.Contexts[contextName].Merge(&contextConfig)

		c.loadedContexts[contextName] = true
	}

	if len(c.config.Contexts) > 0 {
		c.loaded = true
	}

	return nil
}

// IsContextConfigLoaded returns true if the base configuration is loaded, the current context name is set,
// and a context-specific configuration has been loaded for the current context. Returns false otherwise.
func (c *configHandler) IsContextConfigLoaded() bool {
	if !c.loaded {
		return false
	}

	contextName := c.GetContext()
	if contextName == "" {
		return false
	}

	return c.loadedContexts[contextName]
}

// SaveConfig writes the current configuration state to disk. It writes the root windsor.yaml file with only the version field,
// and creates or updates the current context's windsor.yaml file and values.yaml containing dynamic schema-based values.
// The root windsor.yaml is created if missing; the context windsor.yaml is created if missing and not tracked in the root config;
// values.yaml is created if context values are present. If the overwrite parameter is true, existing files are updated with the
// current in-memory state. All operations are performed for the current context.
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

	rootConfigPath := filepath.Join(projectRoot, "windsor.yaml")
	contextName := c.GetContext()
	contextConfigPath := filepath.Join(projectRoot, "contexts", contextName, "windsor.yaml")

	rootExists := false
	if _, err := c.shims.Stat(rootConfigPath); err == nil {
		rootExists = true
	}

	contextExists := false
	if _, err := c.shims.Stat(contextConfigPath); err == nil {
		contextExists = true
	}

	contextExistsInRoot := c.loadedContexts[contextName]

	shouldCreateRootConfig := !rootExists
	shouldCreateContextConfig := !contextExists && !contextExistsInRoot
	shouldUpdateRootConfig := shouldOverwrite && rootExists
	shouldUpdateContextConfig := shouldOverwrite && contextExists

	if shouldCreateRootConfig || shouldUpdateRootConfig {
		rootConfig := struct {
			Version string `yaml:"version"`
		}{
			Version: c.config.Version,
		}

		data, err := c.shims.YamlMarshal(rootConfig)
		if err != nil {
			return fmt.Errorf("error marshalling root config: %w", err)
		}

		if err := c.shims.WriteFile(rootConfigPath, data, 0644); err != nil {
			return fmt.Errorf("error writing root config: %w", err)
		}
	}

	if shouldCreateContextConfig || shouldUpdateContextConfig {
		var contextConfig v1alpha1.Context

		if c.config.Contexts != nil && c.config.Contexts[contextName] != nil {
			contextConfig = *c.config.Contexts[contextName]
		} else {
			contextConfig = c.defaultContextConfig
		}

		contextDir := filepath.Join(projectRoot, "contexts", contextName)
		if err := c.shims.MkdirAll(contextDir, 0755); err != nil {
			return fmt.Errorf("error creating context directory: %w", err)
		}

		data, err := c.shims.YamlMarshal(contextConfig)
		if err != nil {
			return fmt.Errorf("error marshalling context config: %w", err)
		}

		if err := c.shims.WriteFile(contextConfigPath, data, 0644); err != nil {
			return fmt.Errorf("error writing context config: %w", err)
		}
	}

	if len(c.contextValues) > 0 {
		if err := c.saveContextValues(); err != nil {
			return fmt.Errorf("error saving values.yaml: %w", err)
		}
	}

	return nil
}

// SetDefault sets the given context configuration as the default and merges it with any
// existing context configuration. If no context exists, the default becomes the context.
// If a context exists, it merges the default with the existing context, with existing
// values taking precedence over defaults.
func (c *configHandler) SetDefault(context v1alpha1.Context) error {
	c.defaultContextConfig = context
	currentContext := c.GetContext()
	contextKey := fmt.Sprintf("contexts.%s", currentContext)

	if c.Get(contextKey) == nil {
		return c.Set(contextKey, &context)
	}

	if c.config.Contexts == nil {
		c.config.Contexts = make(map[string]*v1alpha1.Context)
	}
	if c.config.Contexts[currentContext] == nil {
		c.config.Contexts[currentContext] = &v1alpha1.Context{}
	}
	defaultCopy := context.DeepCopy()
	existingCopy := c.config.Contexts[currentContext].DeepCopy()
	defaultCopy.Merge(existingCopy)
	c.config.Contexts[currentContext] = defaultCopy

	return nil
}

// Get returns the value at the specified configuration path using the configured lookup precedence.
// Lookup order is: context configuration from windsor.yaml, then values.yaml, then schema defaults.
// Returns nil if the path is empty or if no value is found in any source.
func (c *configHandler) Get(path string) any {
	if path == "" {
		return nil
	}
	pathKeys := parsePath(path)

	value := getValueByPath(c.config, pathKeys)
	if value != nil {
		return value
	}

	if len(pathKeys) >= 2 && pathKeys[0] == "contexts" {
		if len(pathKeys) >= 3 && c.loaded {
			if err := c.ensureValuesYamlLoaded(); err != nil {
			}
			if c.contextValues != nil {
				key := pathKeys[2]
				if value, exists := c.contextValues[key]; exists {
					return value
				}
			}
		}

		value = getValueByPath(c.defaultContextConfig, pathKeys[2:])
		if value != nil {
			return value
		}
	}

	if len(pathKeys) == 1 && c.schemaValidator != nil && c.schemaValidator.Schema != nil {
		defaults, err := c.schemaValidator.GetSchemaDefaults()
		if err == nil {
			if value, exists := defaults[pathKeys[0]]; exists {
				return value
			}
		}
	}

	return nil
}

// GetString retrieves a string value for the specified key from the configuration, with an optional default value.
// If the key is not found, it returns the provided default value or an empty string if no default is provided.
func (c *configHandler) GetString(key string, defaultValue ...string) string {
	contextKey := fmt.Sprintf("contexts.%s.%s", c.context, key)
	value := c.Get(contextKey)
	if value == nil {
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return ""
	}
	strValue := fmt.Sprintf("%v", value)
	return strValue
}

// GetInt retrieves an integer value for the specified key from the configuration, with an optional default value.
func (c *configHandler) GetInt(key string, defaultValue ...int) int {
	contextKey := fmt.Sprintf("contexts.%s.%s", c.context, key)
	value := c.Get(contextKey)
	if value == nil {
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return 0
	}
	intValue, ok := value.(int)
	if !ok {
		return 0
	}
	return intValue
}

// GetBool retrieves a boolean value for the specified key from the configuration, with an optional default value.
func (c *configHandler) GetBool(key string, defaultValue ...bool) bool {
	contextKey := fmt.Sprintf("contexts.%s.%s", c.context, key)
	value := c.Get(contextKey)
	if value == nil {
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return false
	}
	if boolValue, ok := value.(bool); ok {
		return boolValue
	}
	return false
}

// GetStringSlice retrieves a slice of strings for the specified key from the configuration, with an optional default value.
// If the key is not found, it returns the provided default value or an empty slice if no default is provided.
func (c *configHandler) GetStringSlice(key string, defaultValue ...[]string) []string {
	contextKey := fmt.Sprintf("contexts.%s.%s", c.context, key)
	value := c.Get(contextKey)
	if value == nil {
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return []string{}
	}
	strSlice, ok := value.([]string)
	if !ok {
		return []string{}
	}
	return strSlice
}

// GetStringMap retrieves a map of string key-value pairs for the specified key from the configuration.
// If the key is not found, it returns the provided default value or an empty map if no default is provided.
func (c *configHandler) GetStringMap(key string, defaultValue ...map[string]string) map[string]string {
	contextKey := fmt.Sprintf("contexts.%s.%s", c.context, key)
	value := c.Get(contextKey)
	if value == nil {
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return map[string]string{}
	}

	strMap, ok := value.(map[string]string)
	if !ok {
		return map[string]string{}
	}

	return strMap
}

// Set updates the value at the specified path in the configuration using reflection.
// It parses the path, performs type conversion if necessary, and sets the value in the config structure.
// An error is returned if the path is invalid, conversion fails, or the update cannot be performed.
func (c *configHandler) Set(path string, value any) error {
	if path == "" {
		return nil
	}

	pathKeys := parsePath(path)
	if len(pathKeys) == 0 {
		return fmt.Errorf("invalid path: %s", path)
	}

	if strValue, ok := value.(string); ok {
		currentValue := c.Get(path)
		if currentValue != nil {
			targetType := reflect.TypeOf(currentValue)
			convertedValue, err := convertValue(strValue, targetType)
			if err != nil {
				return fmt.Errorf("error converting value for %s: %w", path, err)
			}
			value = convertedValue
		}
	}

	configValue := reflect.ValueOf(&c.config)
	return setValueByPath(configValue, pathKeys, value, path)
}

// SetContextValue sets a value at the given path for the current context, updating config or values.yaml in memory.
// The key must not be empty or have invalid dot notation. Values are schema-validated if available.
// Changes require SaveConfig to persist. Returns error if path or value is invalid, or conversion fails.
func (c *configHandler) SetContextValue(path string, value any) error {
	if path == "" {
		return fmt.Errorf("path cannot be empty")
	}
	if strings.Contains(path, "..") || strings.HasPrefix(path, ".") || strings.HasSuffix(path, ".") {
		return fmt.Errorf("invalid path format: %s", path)
	}
	if c.isKeyInStaticSchema(path) {
		if c.config.Contexts == nil {
			c.config.Contexts = make(map[string]*v1alpha1.Context)
		}
		contextName := c.GetContext()
		if c.config.Contexts[contextName] == nil {
			c.config.Contexts[contextName] = &v1alpha1.Context{}
		}
		fullPath := fmt.Sprintf("contexts.%s.%s", contextName, path)
		return c.Set(fullPath, value)
	}
	if err := c.ensureValuesYamlLoaded(); err != nil {
		return fmt.Errorf("error loading values.yaml: %w", err)
	}
	convertedValue := c.convertStringValue(value)
	c.contextValues[path] = convertedValue
	if c.schemaValidator != nil && c.schemaValidator.Schema != nil {
		if result, err := c.schemaValidator.Validate(c.contextValues); err != nil {
			return fmt.Errorf("error validating context value: %w", err)
		} else if !result.Valid {
			return fmt.Errorf("context value validation failed: %v", result.Errors)
		}
	}
	return nil
}

// GetConfig returns the context config object for the current context, or the default if none is set.
func (c *configHandler) GetConfig() *v1alpha1.Context {
	defaultConfigCopy := c.defaultContextConfig.DeepCopy()
	context := c.context

	if context == "" {
		return defaultConfigCopy
	}

	if ctx, ok := c.config.Contexts[context]; ok {
		mergedConfig := defaultConfigCopy
		mergedConfig.Merge(ctx)
		return mergedConfig
	}

	return defaultConfigCopy
}

// GetContext retrieves the current context from the environment, file, or defaults to "local"
func (c *configHandler) GetContext() string {
	contextName := "local"

	envContext := c.shims.Getenv("WINDSOR_CONTEXT")
	if envContext != "" {
		c.context = envContext
	} else {
		projectRoot, err := c.shell.GetProjectRoot()
		if err != nil {
			c.context = contextName
		} else {
			contextFilePath := filepath.Join(projectRoot, windsorDirName, contextFileName)
			data, err := c.shims.ReadFile(contextFilePath)
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

	c.context = context
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

// Clean cleans up context specific artifacts
func (c *configHandler) Clean() error {
	configRoot, err := c.GetConfigRoot()
	if err != nil {
		return fmt.Errorf("error getting config root: %w", err)
	}

	dirsToDelete := []string{".kube", ".talos", ".omni", ".aws", ".terraform", ".tfstate"}

	for _, dir := range dirsToDelete {
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

// SetSecretsProvider sets the secrets provider for the config handler
func (c *configHandler) SetSecretsProvider(provider secrets.SecretsProvider) {
	c.secretsProviders = append(c.secretsProviders, provider)
}

// LoadSchema loads the schema.yaml file from the specified directory
// Returns error if schema file doesn't exist or is invalid
func (c *configHandler) LoadSchema(schemaPath string) error {
	if c.schemaValidator == nil {
		return fmt.Errorf("schema validator not initialized")
	}
	return c.schemaValidator.LoadSchema(schemaPath)
}

// LoadSchemaFromBytes loads schema directly from byte content
// Returns error if schema content is invalid
func (c *configHandler) LoadSchemaFromBytes(schemaContent []byte) error {
	if c.schemaValidator == nil {
		return fmt.Errorf("schema validator not initialized")
	}
	return c.schemaValidator.LoadSchemaFromBytes(schemaContent)
}

// GetSchemaDefaults extracts default values from the loaded schema
// Returns defaults as a map suitable for merging with user values
func (c *configHandler) GetSchemaDefaults() (map[string]any, error) {
	if c.schemaValidator == nil {
		return nil, fmt.Errorf("schema validator not initialized")
	}
	return c.schemaValidator.GetSchemaDefaults()
}

// GetContextValues returns merged context values from schema defaults, windsor.yaml (via GetConfig), and values.yaml
// Merge order: schema defaults (base) -> context config -> values.yaml (highest priority)
func (c *configHandler) GetContextValues() (map[string]any, error) {
	if err := c.ensureValuesYamlLoaded(); err != nil {
		return nil, err
	}

	result := make(map[string]any)

	schemaDefaults, err := c.GetSchemaDefaults()
	if err == nil && schemaDefaults != nil {
		result = c.deepMerge(result, schemaDefaults)
	}

	contextConfig := c.GetConfig()
	contextData, err := c.shims.YamlMarshal(contextConfig)
	if err != nil {
		return nil, fmt.Errorf("error marshalling context config: %w", err)
	}

	var contextMap map[string]any
	if err := c.shims.YamlUnmarshal(contextData, &contextMap); err != nil {
		return nil, fmt.Errorf("error unmarshalling context config to map: %w", err)
	}

	result = c.deepMerge(result, contextMap)
	result = c.deepMerge(result, c.contextValues)

	return result, nil
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
	return c.SetContextValue("id", id)
}

// Ensure configHandler implements ConfigHandler
var _ ConfigHandler = (*configHandler)(nil)

// =============================================================================
// Private Methods
// =============================================================================

// ensureValuesYamlLoaded loads and validates the values.yaml file for the current context, and loads the schema if required.
// It initializes c.contextValues by reading values.yaml from the context directory, unless already loaded or not required.
// If values.yaml or the schema is missing, it initializes an empty map and performs schema validation if possible.
// Returns an error if any schema loading, reading, or unmarshaling fails.
func (c *configHandler) ensureValuesYamlLoaded() error {
	if c.contextValues != nil {
		return nil
	}
	if c.shell == nil || !c.loaded {
		c.contextValues = make(map[string]any)
		return nil
	}
	projectRoot, err := c.shell.GetProjectRoot()
	if err != nil {
		return fmt.Errorf("error retrieving project root: %w", err)
	}
	contextName := c.GetContext()
	contextConfigDir := filepath.Join(projectRoot, "contexts", contextName)
	valuesPath := filepath.Join(contextConfigDir, "values.yaml")
	if c.schemaValidator != nil && c.schemaValidator.Schema == nil {
		schemaPath := filepath.Join(projectRoot, "contexts", "_template", "schema.yaml")
		if _, err := c.shims.Stat(schemaPath); err == nil {
			if err := c.schemaValidator.LoadSchema(schemaPath); err != nil {
				return fmt.Errorf("error loading schema: %w", err)
			}
		}
	}
	if _, err := c.shims.Stat(valuesPath); err == nil {
		data, err := c.shims.ReadFile(valuesPath)
		if err != nil {
			return fmt.Errorf("error reading values.yaml: %w", err)
		}
		var values map[string]any
		if err := c.shims.YamlUnmarshal(data, &values); err != nil {
			return fmt.Errorf("error unmarshalling values.yaml: %w", err)
		}
		if c.schemaValidator != nil && c.schemaValidator.Schema != nil {
			if result, err := c.schemaValidator.Validate(values); err != nil {
				return fmt.Errorf("error validating values.yaml: %w", err)
			} else if !result.Valid {
				return fmt.Errorf("values.yaml validation failed: %v", result.Errors)
			}
		}
		c.contextValues = values
	} else {
		c.contextValues = make(map[string]any)
	}
	return nil
}

// saveContextValues writes the current contextValues map to a values.yaml file in the context directory for the current context.
// This function ensures that the target context directory exists. If a schema validator is configured, it validates the contextValues
// against the schema before saving. The function marshals the values to YAML, writes them to values.yaml, and returns an error if
// validation, marshalling, or file writing fails.
func (c *configHandler) saveContextValues() error {
	if c.schemaValidator != nil && c.schemaValidator.Schema != nil {
		if result, err := c.schemaValidator.Validate(c.contextValues); err != nil {
			return fmt.Errorf("error validating values.yaml: %w", err)
		} else if !result.Valid {
			return fmt.Errorf("values.yaml validation failed: %v", result.Errors)
		}
	}

	configRoot, err := c.GetConfigRoot()
	if err != nil {
		return fmt.Errorf("error getting config root: %w", err)
	}

	if err := c.shims.MkdirAll(configRoot, 0755); err != nil {
		return fmt.Errorf("error creating context directory: %w", err)
	}

	valuesPath := filepath.Join(configRoot, "values.yaml")
	data, err := c.shims.YamlMarshal(c.contextValues)
	if err != nil {
		return fmt.Errorf("error marshalling values.yaml: %w", err)
	}

	if err := c.shims.WriteFile(valuesPath, data, 0644); err != nil {
		return fmt.Errorf("error writing values.yaml: %w", err)
	}

	return nil
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

// convertStringValue infers and converts a string value to the appropriate type based on schema type information.
// It is used to correctly coerce command-line --set flags (which arrive as strings) to their target types.
// The function uses the configured schema validator, if present, to determine the expected type for the value.
// If type information cannot be found in the schema, it applies pattern-based type conversion heuristics.
// The returned value is properly typed if conversion is possible; otherwise, the original value is returned.
func (c *configHandler) convertStringValue(value any) any {
	str, ok := value.(string)
	if !ok {
		return value
	}
	if c.schemaValidator != nil && c.schemaValidator.Schema != nil {
		if expectedType := c.getExpectedTypeFromSchema(str); expectedType != "" {
			if convertedValue := c.convertStringToType(str, expectedType); convertedValue != nil {
				return convertedValue
			}
		}
	}
	return c.convertStringByPattern(str)
}

// getExpectedTypeFromSchema attempts to find the expected type for a key in the schema
func (c *configHandler) getExpectedTypeFromSchema(key string) string {
	if c.schemaValidator == nil || c.schemaValidator.Schema == nil {
		return ""
	}

	properties, ok := c.schemaValidator.Schema["properties"]
	if !ok {
		return ""
	}

	propertiesMap, ok := properties.(map[string]any)
	if !ok {
		return ""
	}

	propSchema, exists := propertiesMap[key]
	if !exists {
		return ""
	}

	propSchemaMap, ok := propSchema.(map[string]any)
	if !ok {
		return ""
	}

	expectedType, ok := propSchemaMap["type"]
	if !ok {
		return ""
	}

	expectedTypeStr, ok := expectedType.(string)
	if !ok {
		return ""
	}

	return expectedTypeStr
}

// convertStringToType converts a string value to the corresponding Go type based on the provided JSON schema type.
// It supports boolean, integer, number, and string schema types. Returns the converted value, or nil if conversion fails.
// The conversion follows JSON schema type expectations: booleans are case-insensitive, integers use strconv.Atoi,
// numbers use strconv.ParseFloat (64-bit), and unrecognized types or conversion failures return nil.
func (c *configHandler) convertStringToType(str, expectedType string) any {
	switch expectedType {
	case "boolean":
		switch strings.ToLower(str) {
		case "true":
			return true
		case "false":
			return false
		}
	case "integer":
		if intVal, err := strconv.Atoi(str); err == nil {
			return intVal
		}
	case "number":
		if floatVal, err := strconv.ParseFloat(str, 64); err == nil {
			return floatVal
		}
	case "string":
		return str
	}
	return nil
}

// convertStringByPattern attempts to infer and convert a string value to its most likely Go type.
// It recognizes "true"/"false" as booleans, parses numeric strings as integer or float as appropriate,
// and returns the original string if no conversion pattern matches.
func (c *configHandler) convertStringByPattern(str string) any {
	switch strings.ToLower(str) {
	case "true":
		return true
	case "false":
		return false
	}

	if intVal, err := strconv.Atoi(str); err == nil {
		return intVal
	}

	if floatVal, err := strconv.ParseFloat(str, 64); err == nil {
		return floatVal
	}

	return str
}

// deepMerge recursively merges two maps with overlay values taking precedence.
// Nested maps are merged rather than replaced. Non-map values in overlay replace base values.
func (c *configHandler) deepMerge(base, overlay map[string]any) map[string]any {
	result := make(map[string]any)
	for k, v := range base {
		result[k] = v
	}
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

// getValueByPath retrieves a value by navigating through a struct or map using YAML tags.
func getValueByPath(current any, pathKeys []string) any {
	if len(pathKeys) == 0 {
		return nil
	}

	currValue := reflect.ValueOf(current)
	if !currValue.IsValid() {
		return nil
	}

	for _, key := range pathKeys {
		for currValue.Kind() == reflect.Ptr && !currValue.IsNil() {
			currValue = currValue.Elem()
		}
		if currValue.Kind() == reflect.Ptr && currValue.IsNil() {
			return nil
		}

		switch currValue.Kind() {
		case reflect.Struct:
			fieldValue := getFieldByYamlTag(currValue, key)
			currValue = fieldValue

		case reflect.Map:
			mapKey := reflect.ValueOf(key)
			if !mapKey.Type().AssignableTo(currValue.Type().Key()) {
				return nil
			}
			mapValue := currValue.MapIndex(mapKey)
			if !mapValue.IsValid() {
				return nil
			}
			currValue = mapValue

		default:
			return nil
		}
	}

	if currValue.Kind() == reflect.Ptr {
		if currValue.IsNil() {
			return nil
		}
		currValue = currValue.Elem()
	}

	if currValue.IsValid() && currValue.CanInterface() {
		return currValue.Interface()
	}

	return nil
}

// getFieldByYamlTag retrieves a field from a struct by its YAML tag.
func getFieldByYamlTag(v reflect.Value, tag string) reflect.Value {
	t := v.Type()
	for i := range make([]struct{}, v.NumField()) {
		field := t.Field(i)
		yamlTag := strings.Split(field.Tag.Get("yaml"), ",")[0]
		if yamlTag == tag {
			return v.Field(i)
		}
	}
	return reflect.Value{}
}

// setValueByPath sets a value in a struct or map by navigating through it using YAML tags.
func setValueByPath(currValue reflect.Value, pathKeys []string, value any, fullPath string) error {
	if len(pathKeys) == 0 {
		return fmt.Errorf("pathKeys cannot be empty")
	}

	key := pathKeys[0]
	isLast := len(pathKeys) == 1

	if currValue.Kind() == reflect.Ptr {
		if currValue.IsNil() {
			currValue.Set(reflect.New(currValue.Type().Elem()))
		}
		currValue = currValue.Elem()
	}

	switch currValue.Kind() {
	case reflect.Struct:
		fieldValue := getFieldByYamlTag(currValue, key)
		if !fieldValue.IsValid() {
			return fmt.Errorf("field not found: %s", key)
		}

		if fieldValue.Kind() == reflect.Ptr && fieldValue.IsNil() {
			fieldValue.Set(reflect.New(fieldValue.Type().Elem()))
		}

		if fieldValue.Kind() == reflect.Map && fieldValue.IsNil() {
			fieldValue.Set(reflect.MakeMap(fieldValue.Type()))
		}

		if isLast {
			newFieldValue, err := assignValue(fieldValue, value)
			if err != nil {
				return err
			}
			fieldValue.Set(newFieldValue)
		} else {
			err := setValueByPath(fieldValue, pathKeys[1:], value, fullPath)
			if err != nil {
				return err
			}
		}

	case reflect.Map:
		if currValue.IsNil() {
			currValue.Set(reflect.MakeMap(currValue.Type()))
		}

		mapKey := reflect.ValueOf(key)
		if !mapKey.Type().AssignableTo(currValue.Type().Key()) {
			return fmt.Errorf("key type mismatch: expected %s, got %s", currValue.Type().Key(), mapKey.Type())
		}

		var nextValue reflect.Value

		if isLast {
			val := reflect.ValueOf(value)
			if !val.Type().AssignableTo(currValue.Type().Elem()) {
				if val.Type().ConvertibleTo(currValue.Type().Elem()) {
					val = val.Convert(currValue.Type().Elem())
				} else {
					return fmt.Errorf("value type mismatch for key %s: expected %s, got %s", key, currValue.Type().Elem(), val.Type())
				}
			}
			currValue.SetMapIndex(mapKey, val)
		} else {
			nextValue = currValue.MapIndex(mapKey)
			if !nextValue.IsValid() {
				nextValue = reflect.New(currValue.Type().Elem()).Elem()
			} else {
				nextValue = makeAddressable(nextValue)
			}

			err := setValueByPath(nextValue, pathKeys[1:], value, fullPath)
			if err != nil {
				return err
			}

			currValue.SetMapIndex(mapKey, nextValue)
		}

	default:
		return fmt.Errorf("Invalid path: %s", fullPath)
	}

	return nil
}

// assignValue assigns a value to a struct field, performing type conversion if necessary.
// It supports string-to-type conversion, pointer assignment, and type compatibility checks.
// Returns a reflect.Value suitable for assignment or an error if conversion is not possible.
func assignValue(fieldValue reflect.Value, value any) (reflect.Value, error) {
	if !fieldValue.CanSet() {
		return reflect.Value{}, fmt.Errorf("cannot set field")
	}

	fieldType := fieldValue.Type()
	valueType := reflect.TypeOf(value)

	if strValue, ok := value.(string); ok {
		convertedValue, err := convertValue(strValue, fieldType)
		if err == nil {
			return reflect.ValueOf(convertedValue), nil
		}
	}

	if fieldType.Kind() == reflect.Ptr {
		elemType := fieldType.Elem()
		newValue := reflect.New(elemType)
		val := reflect.ValueOf(value)

		if valueType.AssignableTo(fieldType) {
			return val, nil
		}

		if val.Type().ConvertibleTo(elemType) {
			val = val.Convert(elemType)
			newValue.Elem().Set(val)
			return newValue, nil
		}

		return reflect.Value{}, fmt.Errorf("cannot assign value of type %s to field of type %s", valueType, fieldType)
	}

	val := reflect.ValueOf(value)
	if valueType.AssignableTo(fieldType) {
		return val, nil
	}

	if valueType.ConvertibleTo(fieldType) {
		return val.Convert(fieldType), nil
	}

	return reflect.Value{}, fmt.Errorf("cannot assign value of type %s to field of type %s", valueType, fieldType)
}

// makeAddressable ensures a value is addressable by creating a new pointer if necessary.
func makeAddressable(v reflect.Value) reflect.Value {
	if !v.IsValid() {
		return v
	}
	if v.CanAddr() {
		return v
	}
	addr := reflect.New(v.Type())
	addr.Elem().Set(v)
	return addr.Elem()
}

// parsePath parses a path string into a slice of keys, supporting both dot and bracket notation.
func parsePath(path string) []string {
	var keys []string
	var currentKey strings.Builder
	inBracket := false

	for _, char := range path {
		switch char {
		case '.':
			if !inBracket {
				if currentKey.Len() > 0 {
					keys = append(keys, currentKey.String())
					currentKey.Reset()
				}
			} else {
				currentKey.WriteRune(char)
			}
		case '[':
			inBracket = true
			if currentKey.Len() > 0 {
				keys = append(keys, currentKey.String())
				currentKey.Reset()
			}
		case ']':
			inBracket = false
		default:
			currentKey.WriteRune(char)
		}
	}

	if currentKey.Len() > 0 {
		keys = append(keys, currentKey.String())
	}

	return keys
}

// convertValue attempts to convert a string value to the appropriate type based on the target field's type
func convertValue(value string, targetType reflect.Type) (any, error) {
	isPointer := targetType.Kind() == reflect.Ptr
	if isPointer {
		targetType = targetType.Elem()
	}

	var convertedValue any
	var err error

	switch targetType.Kind() {
	case reflect.Bool:
		convertedValue, err = strconv.ParseBool(value)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		var v int64
		v, err = strconv.ParseInt(value, 10, 64)
		if err == nil {
			switch targetType.Kind() {
			case reflect.Int:
				if v < math.MinInt || v > math.MaxInt {
					return nil, fmt.Errorf("integer overflow: %d is outside the range of int", v)
				}
				convertedValue = int(v)
			case reflect.Int8:
				if v < math.MinInt8 || v > math.MaxInt8 {
					return nil, fmt.Errorf("integer overflow: %d is outside the range of int8", v)
				}
				convertedValue = int8(v)
			case reflect.Int16:
				if v < math.MinInt16 || v > math.MaxInt16 {
					return nil, fmt.Errorf("integer overflow: %d is outside the range of int16", v)
				}
				convertedValue = int16(v)
			case reflect.Int32:
				if v < math.MinInt32 || v > math.MaxInt32 {
					return nil, fmt.Errorf("integer overflow: %d is outside the range of int32", v)
				}
				convertedValue = int32(v)
			case reflect.Int64:
				convertedValue = v
			}
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		var v uint64
		v, err = strconv.ParseUint(value, 10, 64)
		if err == nil {
			switch targetType.Kind() {
			case reflect.Uint:
				if v > math.MaxUint {
					return nil, fmt.Errorf("integer overflow: %d is outside the range of uint", v)
				}
				convertedValue = uint(v)
			case reflect.Uint8:
				if v > math.MaxUint8 {
					return nil, fmt.Errorf("integer overflow: %d is outside the range of uint8", v)
				}
				convertedValue = uint8(v)
			case reflect.Uint16:
				if v > math.MaxUint16 {
					return nil, fmt.Errorf("integer overflow: %d is outside the range of uint16", v)
				}
				convertedValue = uint16(v)
			case reflect.Uint32:
				if v > math.MaxUint32 {
					return nil, fmt.Errorf("integer overflow: %d is outside the range of uint32", v)
				}
				convertedValue = uint32(v)
			case reflect.Uint64:
				convertedValue = v
			}
		}
	case reflect.Float32, reflect.Float64:
		var v float64
		v, err = strconv.ParseFloat(value, 64)
		if err == nil {
			if targetType.Kind() == reflect.Float32 {
				if v < -math.MaxFloat32 || v > math.MaxFloat32 {
					return nil, fmt.Errorf("float overflow: %f is outside the range of float32", v)
				}
				convertedValue = float32(v)
			} else {
				convertedValue = v
			}
		}
	case reflect.String:
		convertedValue = value
	default:
		return nil, fmt.Errorf("unsupported type conversion from string to %v", targetType)
	}

	if err != nil {
		return nil, err
	}

	if isPointer {
		ptr := reflect.New(targetType)
		ptr.Elem().Set(reflect.ValueOf(convertedValue))
		return ptr.Interface(), nil
	}

	return convertedValue, nil
}
