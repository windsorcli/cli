package config

import (
	"fmt"
	"strconv"
	"strings"
)

// The ConfigAccessors are focused methods for path-based config retrieval and assignment.
// They provide typed getters, provider fallback, schema-default fallback, and validated writes,
// The ConfigAccessors isolate key-path access logic from source loading and persistence orchestration,
// and keep type coercion behavior centralized for consistent command/runtime interactions.

// =============================================================================
// Public Methods
// =============================================================================

// Get retrieves the value at the specified configuration path from the internal data map.
// If the value is not found in the current data, it checks registered providers for the prefix,
// and if the schema validator is available, it falls back to returning a default value from the schema
// for the top-level key or deeper nested keys as appropriate. Returns nil if the path is empty or no value is found.
func (c *configHandler) Get(path string) any {
	if path == "" {
		return nil
	}

	pathKeys := parsePath(path)
	if len(pathKeys) == 0 {
		return nil
	}

	firstKey := pathKeys[0]
	provider, hasProvider := c.providers[firstKey]

	value := getValueByPathFromMap(c.data, pathKeys)
	if value != nil {
		return value
	}

	if hasProvider {
		providerValue, err := provider.GetValue(path)
		if err == nil {
			return providerValue
		}
	}

	if len(pathKeys) > 0 && c.schemaValidator != nil && c.schemaValidator.Schema != nil {
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

	return nil
}

// RegisterProvider registers a value provider for the specified prefix.
// When Get encounters a key starting with the prefix and the value is not found in the data map,
// it delegates to the provider. If the provider returns an error, Get falls back to schema defaults.
func (c *configHandler) RegisterProvider(prefix string, provider ValueProvider) {
	if c.providers == nil {
		c.providers = make(map[string]ValueProvider)
	}
	c.providers[prefix] = provider
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
	return fmt.Sprintf("%v", value)
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

// =============================================================================
// Private Methods
// =============================================================================

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

// getExpectedTypeFromSchema attempts to find the expected type for a key in the schema.
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
