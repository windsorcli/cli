package config

import (
	"embed"
	"fmt"
	"maps"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"

	"github.com/windsorcli/cli/api/v1alpha1"
	v1alpha2config "github.com/windsorcli/cli/api/v1alpha2/config"
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
	LoadConfigString(content string) error
	GetString(key string, defaultValue ...string) string
	GetInt(key string, defaultValue ...int) int
	GetBool(key string, defaultValue ...bool) bool
	GetStringSlice(key string, defaultValue ...[]string) []string
	GetStringMap(key string, defaultValue ...map[string]string) map[string]string
	Set(key string, value any) error
	Get(key string) any
	SaveConfig(overwrite ...bool) error
	SetDefault(context v1alpha1.Context) error
	GetConfig() *v1alpha1.Context
	GetContext() string
	IsDevMode(contextName string) bool
	SetContext(context string) error
	GetConfigRoot() (string, error)
	GetWindsorScratchPath() (string, error)
	Clean() error
	IsLoaded() bool
	GenerateContextID() error
	LoadSchema(schemaPath string) error
	LoadSchemaFromBytes(schemaContent []byte) error
	GetContextValues() (map[string]any, error)
	RegisterProvider(prefix string, provider ValueProvider)
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
	data            map[string]any
	defaultConfig   *v1alpha1.Context
	providers       map[string]ValueProvider
}

// =============================================================================
// Constructor
// =============================================================================

// NewConfigHandler creates a new ConfigHandler instance with default context configuration.
func NewConfigHandler(shell shell.Shell) ConfigHandler {
	handler := &configHandler{
		shell:     shell,
		shims:     NewShims(),
		data:      make(map[string]any),
		providers: make(map[string]ValueProvider),
	}

	handler.schemaValidator = NewSchemaValidator(shell)
	handler.schemaValidator.Shims = handler.shims

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

	projectRoot, err := c.shell.GetProjectRoot()
	if err != nil {
		return fmt.Errorf("error retrieving project root: %w", err)
	}

	contextName := c.GetContext()
	hasLoadedFiles := false

	if c.schemaValidator != nil && c.schemaValidator.Schema == nil {
		schemaPath := filepath.Join(projectRoot, "contexts", "_template", "schema.yaml")
		if _, err := c.shims.Stat(schemaPath); err == nil {
			if err := c.LoadSchema(schemaPath); err != nil {
				return fmt.Errorf("error loading schema: %w", err)
			}
		}
	}

	rootConfigPath := filepath.Join(projectRoot, "windsor.yaml")
	if _, err := c.shims.Stat(rootConfigPath); err == nil {
		fileData, err := c.shims.ReadFile(rootConfigPath)
		if err != nil {
			return fmt.Errorf("error reading root config file: %w", err)
		}

		var rootConfigMap map[string]any
		if err := c.shims.YamlUnmarshal(fileData, &rootConfigMap); err != nil {
			return fmt.Errorf("error unmarshalling root config: %w", err)
		}

		configVersion, _ := rootConfigMap["version"].(string)
		if configVersion != "" && configVersion != "v1alpha1" {
			if c.schemaValidator != nil {
				if err := v1alpha2config.LoadSchemas(c.LoadSchemaFromBytes); err != nil {
					return fmt.Errorf("error loading API schemas: %w", err)
				}
			}
		}

		var rootConfig v1alpha1.Config
		if err := c.shims.YamlUnmarshal(fileData, &rootConfig); err != nil {
			return fmt.Errorf("error unmarshalling root config: %w", err)
		}

		if rootConfig.Contexts != nil && rootConfig.Contexts[contextName] != nil {
			contextData, err := c.shims.YamlMarshal(rootConfig.Contexts[contextName])
			if err != nil {
				return fmt.Errorf("error marshalling context config: %w", err)
			}
			var contextMap map[string]any
			if err := c.shims.YamlUnmarshal(contextData, &contextMap); err != nil {
				return fmt.Errorf("error unmarshalling context config to map: %w", err)
			}
			c.data = c.deepMerge(c.data, contextMap)
		}
		hasLoadedFiles = true
	}

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
		fileData, err := c.shims.ReadFile(contextConfigPath)
		if err != nil {
			return fmt.Errorf("error reading context config file: %w", err)
		}

		var contextMap map[string]any
		if err := c.shims.YamlUnmarshal(fileData, &contextMap); err != nil {
			return fmt.Errorf("error unmarshalling context yaml: %w", err)
		}

		c.data = c.deepMerge(c.data, contextMap)
		hasLoadedFiles = true
	}

	valuesPath := filepath.Join(contextConfigDir, "values.yaml")
	if _, err := c.shims.Stat(valuesPath); err == nil {
		fileData, err := c.shims.ReadFile(valuesPath)
		if err != nil {
			return fmt.Errorf("error reading values.yaml: %w", err)
		}

		var values map[string]any
		if err := c.shims.YamlUnmarshal(fileData, &values); err != nil {
			return fmt.Errorf("error unmarshalling values.yaml: %w", err)
		}

		if c.schemaValidator != nil && c.schemaValidator.Schema != nil {
			if result, err := c.schemaValidator.Validate(values); err != nil {
				return fmt.Errorf("error validating values.yaml: %w", err)
			} else if !result.Valid {
				return fmt.Errorf("values.yaml validation failed: %v", result.Errors)
			}
		}

		c.data = c.deepMerge(c.data, values)
		hasLoadedFiles = true
	}

	if hasLoadedFiles {
		c.loaded = true
	}

	return nil
}

// SaveConfig writes the current configuration state to disk.
// This function separates the configuration fields of the active context into two distinct files
// within the context directory under the project root. Static fields that match the v1alpha1.Context
// schema are written to a windsor.yaml file, and dynamic fields that do not match the static schema are
// written to a values.yaml file. If overwrite is specified, existing windsor.yaml will be overwritten;
// otherwise, it is only created if missing. Returns an error if writing fails, or if required shims
// or shell are not initialized.
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

	contextName := c.GetContext()
	contextDir := filepath.Join(projectRoot, "contexts", contextName)

	if err := c.shims.MkdirAll(contextDir, 0755); err != nil {
		return fmt.Errorf("error creating context directory: %w", err)
	}

	rootConfigPath := filepath.Join(projectRoot, "windsor.yaml")
	if _, err := c.shims.Stat(rootConfigPath); err != nil {
		rootConfig := map[string]any{
			"version": "v1alpha1",
		}
		rootData, err := c.shims.YamlMarshal(rootConfig)
		if err != nil {
			return fmt.Errorf("error marshalling root config: %w", err)
		}
		if err := c.shims.WriteFile(rootConfigPath, rootData, 0644); err != nil {
			return fmt.Errorf("error writing root config: %w", err)
		}
	}

	staticFields, dynamicFields := c.separateStaticAndDynamicFields(c.data)

	if len(staticFields) > 0 {
		contextConfigPath := filepath.Join(contextDir, "windsor.yaml")
		contextExists := false
		if _, err := c.shims.Stat(contextConfigPath); err == nil {
			contextExists = true
		}

		if !contextExists || shouldOverwrite {
			var contextStruct *v1alpha1.Context
			if !contextExists && c.defaultConfig != nil {
				defaultCopy := c.defaultConfig.DeepCopy()
				if len(staticFields) > 0 {
					overlayStruct := c.mapToContext(staticFields)
					if overlayStruct != nil {
						defaultCopy.Merge(overlayStruct)
					}
				}
				contextStruct = defaultCopy
			} else {
				mergedStaticFields := staticFields
				if c.schemaValidator != nil && c.schemaValidator.Schema != nil {
					defaults, err := c.schemaValidator.GetSchemaDefaults()
					if err == nil && defaults != nil {
						defaultsStatic, _ := c.separateStaticAndDynamicFields(defaults)
						mergedStaticFields = c.deepMerge(defaultsStatic, staticFields)
					}
				}
				contextStruct = c.mapToContext(mergedStaticFields)
			}
			if contextStruct == nil {
				contextStruct = &v1alpha1.Context{}
			}
			data, err := c.shims.YamlMarshal(contextStruct)
			if err != nil {
				return fmt.Errorf("error marshalling context config: %w", err)
			}

			if err := c.shims.WriteFile(contextConfigPath, data, 0644); err != nil {
				return fmt.Errorf("error writing context config: %w", err)
			}
		}
	}

	if len(dynamicFields) > 0 {
		valuesPath := filepath.Join(contextDir, "values.yaml")
		data, err := c.shims.YamlMarshal(dynamicFields)
		if err != nil {
			return fmt.Errorf("error marshalling values.yaml: %w", err)
		}

		if err := c.shims.WriteFile(valuesPath, data, 0644); err != nil {
			return fmt.Errorf("error writing values.yaml: %w", err)
		}
	}

	return nil
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

// Get retrieves the value at the specified configuration path from the internal data map.
// If the value is not found in the current data, and the schema validator is available,
// it falls back to returning a default value from the schema for the top-level key or
// deeper nested keys as appropriate. If the key matches a provider pattern (e.g., terraform.*),
// it delegates to the appropriate provider. Returns nil if the path is empty or no value is found.
func (c *configHandler) Get(path string) any {
	if path == "" {
		return nil
	}

	pathKeys := parsePath(path)
	if len(pathKeys) == 0 {
		return nil
	}

	firstKey := pathKeys[0]
	if provider, exists := c.providers[firstKey]; exists {
		value, err := provider.GetValue(path)
		if err != nil {
			return nil
		}
		return value
	}

	value := getValueByPathFromMap(c.data, pathKeys)

	if value == nil && len(pathKeys) > 0 && c.schemaValidator != nil && c.schemaValidator.Schema != nil {
		defaults, err := c.schemaValidator.GetSchemaDefaults()
		if err == nil && defaults != nil {
			if topLevelDefault, exists := defaults[pathKeys[0]]; exists {
				if len(pathKeys) == 1 {
					return topLevelDefault
				}
				if defaultMap, ok := topLevelDefault.(map[string]any); ok {
					return getValueByPathFromMap(defaultMap, pathKeys[1:])
				}
				if interfaceMap, ok := topLevelDefault.(map[any]any); ok {
					convertedMap := c.convertInterfaceMap(interfaceMap)
					return getValueByPathFromMap(convertedMap, pathKeys[1:])
				}
			}
		}
	}

	return value
}

// RegisterProvider registers a value provider for the specified prefix.
// When Get encounters a key starting with the prefix,
// it will delegate to the registered provider to fetch the value.
func (c *configHandler) RegisterProvider(prefix string, provider ValueProvider) {
	if c.providers == nil {
		c.providers = make(map[string]ValueProvider)
	}
	c.providers[prefix] = provider
}

// getValueByPathFromMap returns the value in a nested map[string]any at the location specified by the pathKeys slice.
// It traverses the map according to the keys, returning the value found at the leaf, or nil if any key is missing or the value is not a map.
func getValueByPathFromMap(data map[string]any, pathKeys []string) any {
	if len(pathKeys) == 0 {
		return nil
	}

	current := any(data)
	for _, key := range pathKeys {
		if m, ok := current.(map[string]any); ok {
			val, exists := m[key]
			if !exists {
				return nil
			}
			current = val
		} else {
			return nil
		}
	}

	return current
}

// GetString retrieves a string value for the specified key from the configuration, with an optional default value.
// If the key is not found, it returns the provided default value or an empty string if no default is provided.
func (c *configHandler) GetString(key string, defaultValue ...string) string {
	value := c.Get(key)
	if value == nil {
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return ""
	}
	strValue := fmt.Sprintf("%v", value)
	return strValue
}

// GetInt retrieves an integer value for the specified key from the configuration.
// It accepts an optional default value. The function safely converts supported types (int, int64, uint64, uint)
// to int with appropriate overflow protection, and parses string values if they represent valid integer literals.
// Types that cannot be converted (such as float64 or invalid strings) are ignored and the default is used.
// If the key is not found or if conversion fails, the provided default value or 0 is returned.
func (c *configHandler) GetInt(key string, defaultValue ...int) int {
	value := c.Get(key)
	if value == nil {
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return 0
	}
	if intValue, ok := value.(int); ok {
		return intValue
	}
	if int64Value, ok := value.(int64); ok {
		maxInt := int64(^uint(0) >> 1)
		minInt := -maxInt - 1
		if int64Value > maxInt || int64Value < minInt {
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return 0
		}
		return int(int64Value)
	}
	if uint64Value, ok := value.(uint64); ok {
		if uint64Value > uint64(^uint(0)>>1) {
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return 0
		}
		return int(uint64Value)
	}
	if uintValue, ok := value.(uint); ok {
		if uintValue > uint(^uint(0)>>1) {
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return 0
		}
		return int(uintValue)
	}
	if strValue, ok := value.(string); ok {
		if intVal, err := strconv.Atoi(strValue); err == nil {
			return intVal
		}
	}
	if len(defaultValue) > 0 {
		return defaultValue[0]
	}
	return 0
}

// GetBool retrieves a boolean value for the specified key from the configuration, with an optional default value.
func (c *configHandler) GetBool(key string, defaultValue ...bool) bool {
	value := c.Get(key)
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

// GetStringSlice retrieves a slice of strings for the specified key from the configuration.
// It supports both []string and []any (such as from YAML unmarshaling).
// If the key is not found, the function returns the provided default value or an empty slice if no default is supplied.
func (c *configHandler) GetStringSlice(key string, defaultValue ...[]string) []string {
	value := c.Get(key)
	if value == nil {
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return []string{}
	}
	if strSlice, ok := value.([]string); ok {
		return strSlice
	}
	if interfaceSlice, ok := value.([]any); ok {
		strSlice := make([]string, 0, len(interfaceSlice))
		for _, v := range interfaceSlice {
			if str, ok := v.(string); ok {
				strSlice = append(strSlice, str)
			} else {
				strSlice = append(strSlice, fmt.Sprintf("%v", v))
			}
		}
		return strSlice
	}
	return []string{}
}

// GetStringMap retrieves a map of string key-value pairs for the specified key from the configuration.
// If the key is not found, it returns the provided default value or an empty map if no default is provided.
// The method handles values that are map[string]string, map[string]any, or map[any]any,
// converting all map values to strings as needed to produce a map[string]string result.
func (c *configHandler) GetStringMap(key string, defaultValue ...map[string]string) map[string]string {
	value := c.Get(key)
	if value == nil {
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return map[string]string{}
	}
	if strMap, ok := value.(map[string]string); ok {
		return strMap
	}
	if interfaceMap, ok := value.(map[string]any); ok {
		strMap := make(map[string]string, len(interfaceMap))
		for k, v := range interfaceMap {
			if str, ok := v.(string); ok {
				strMap[k] = str
			} else {
				strMap[k] = fmt.Sprintf("%v", v)
			}
		}
		return strMap
	}
	if interfaceMap, ok := value.(map[any]any); ok {
		strMap := make(map[string]string)
		for k, v := range interfaceMap {
			if strKey, ok := k.(string); ok {
				if strVal, ok := v.(string); ok {
					strMap[strKey] = strVal
				} else {
					strMap[strKey] = fmt.Sprintf("%v", v)
				}
			}
		}
		return strMap
	}
	return map[string]string{}
}

// Set assigns a configuration value at the specified hierarchical path in the configHandler's internal data map.
// The input value is automatically converted to the appropriate type according to the schema, if available.
// If a schema is present, Set validates only the dynamic fields of the configuration map after the new value is set.
// Returns an error if the path is invalid, schema validation fails, or if value assignment encounters an issue.
// Changes made by this method are in-memory and must be persisted separately via SaveConfig.
func (c *configHandler) Set(path string, value any) error {
	if path == "" {
		return fmt.Errorf("path cannot be empty")
	}
	if strings.Contains(path, "..") || strings.HasPrefix(path, ".") || strings.HasSuffix(path, ".") {
		return fmt.Errorf("invalid path format: %s", path)
	}

	convertedValue := c.convertStringValue(value)
	pathKeys := parsePath(path)
	setValueInMap(c.data, pathKeys, convertedValue)

	if c.schemaValidator != nil && c.schemaValidator.Schema != nil {
		_, dynamicFields := c.separateStaticAndDynamicFields(c.data)
		if result, err := c.schemaValidator.Validate(dynamicFields); err != nil {
			return fmt.Errorf("error validating context value: %w", err)
		} else if !result.Valid {
			return fmt.Errorf("context value validation failed: %v", result.Errors)
		}
	}
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

// GetContext retrieves the current context from the environment, file, or defaults to "local"
func (c *configHandler) GetContext() string {
	contextName := "local"

	envContext := c.shims.Getenv("WINDSOR_CONTEXT")
	if envContext != "" {
		return envContext
	} else if c.shell != nil {
		projectRoot, err := c.shell.GetProjectRoot()
		if err != nil {
			return contextName
		} else {
			contextFilePath := filepath.Join(projectRoot, windsorDirName, contextFileName)
			data, err := c.shims.ReadFile(contextFilePath)
			if err != nil {
				return contextName
			} else {
				return strings.TrimSpace(string(data))
			}
		}
	} else {
		return contextName
	}
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

// GetContextValues returns a merged configuration map composed of schema defaults and current config data.
// The result provides all configuration values, with schema defaults filled in for missing keys, ensuring
// downstream consumers (such as blueprint processing) always receive a complete set of config values.
// If the schema validator or schema is unavailable, only the currently loaded data is returned.
// It also ensures that cluster.controlplanes.nodes and cluster.workers.nodes are initialized as empty maps
// even though they are not serialized to YAML, so template expressions can safely evaluate them.
func (c *configHandler) GetContextValues() (map[string]any, error) {
	result := make(map[string]any)
	if c.schemaValidator != nil && c.schemaValidator.Schema != nil {
		defaults, err := c.schemaValidator.GetSchemaDefaults()
		if err == nil && defaults != nil {
			result = c.deepMerge(result, defaults)
		}
	}
	result = c.deepMerge(result, c.data)

	clusterVal, ok := result["cluster"].(map[string]any)
	if !ok {
		if _, exists := result["cluster"]; !exists || result["cluster"] == nil {
			result["cluster"] = map[string]any{
				"controlplanes": map[string]any{
					"nodes": make(map[string]any),
				},
				"workers": map[string]any{
					"nodes": make(map[string]any),
				},
			}
			return result, nil
		}
		return result, nil
	}

	if controlplanesVal, ok := clusterVal["controlplanes"].(map[string]any); ok {
		if _, exists := controlplanesVal["nodes"]; !exists {
			controlplanesVal["nodes"] = make(map[string]any)
		}
	} else {
		clusterVal["controlplanes"] = map[string]any{
			"nodes": make(map[string]any),
		}
	}

	if workersVal, ok := clusterVal["workers"].(map[string]any); ok {
		if _, exists := workersVal["nodes"]; !exists {
			workersVal["nodes"] = make(map[string]any)
		}
	} else {
		clusterVal["workers"] = map[string]any{
			"nodes": make(map[string]any),
		}
	}

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
	return c.Set("id", id)
}

// Ensure configHandler implements ConfigHandler
var _ ConfigHandler = (*configHandler)(nil)

// =============================================================================
// Private Methods
// =============================================================================

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
// Private Helpers
// =============================================================================

// setValueInMap sets a value in a nested map structure, creating any intermediate maps as needed.
// setValueInMap sets a value in a nested map structure at the specified path, creating intermediate maps as needed.
// It navigates or creates each intermediate key in the provided pathKeys slice, and assigns the final value at the leaf key.
// This function mutates the provided data map in place.
func setValueInMap(data map[string]any, pathKeys []string, value any) {
	if len(pathKeys) == 0 {
		return
	}

	if len(pathKeys) == 1 {
		data[pathKeys[0]] = value
		return
	}

	current := data
	for i := 0; i < len(pathKeys)-1; i++ {
		key := pathKeys[i]

		if existing, ok := current[key]; ok {
			if existingMap, ok := existing.(map[string]any); ok {
				current = existingMap
			} else {
				newMap := make(map[string]any)
				current[key] = newMap
				current = newMap
			}
		} else {
			newMap := make(map[string]any)
			current[key] = newMap
			current = newMap
		}
	}

	current[pathKeys[len(pathKeys)-1]] = value
}

// parsePath splits a hierarchical path string into its individual key segments.
// It supports dotted notation and bracket notation for keys, returning a slice of key strings.
// For example, "foo.bar[baz]" would be parsed into []string{"foo", "bar", "baz"}.
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
