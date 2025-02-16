package config

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/di"
)

// YamlConfigHandler implements the ConfigHandler interface using goccy/go-yaml
type YamlConfigHandler struct {
	BaseConfigHandler
	config               v1alpha1.Config
	path                 string
	defaultContextConfig v1alpha1.Context
}

// NewYamlConfigHandler creates a new instance of YamlConfigHandler with default context configuration.
func NewYamlConfigHandler(injector di.Injector) *YamlConfigHandler {
	return &YamlConfigHandler{
		BaseConfigHandler: BaseConfigHandler{
			injector: injector,
		},
	}
}

// LoadConfig loads the configuration from the specified path. If the file does not exist, it does nothing.
func (y *YamlConfigHandler) LoadConfig(path string) error {
	y.path = path
	if _, err := osStat(path); os.IsNotExist(err) {
		return nil
	}

	data, err := osReadFile(path)
	if err != nil {
		return fmt.Errorf("error reading config file: %w", err)
	}

	if err := yamlUnmarshal(data, &y.config); err != nil {
		return fmt.Errorf("error unmarshalling yaml: %w", err)
	}

	// Check and set the config version
	if y.config.Version == "" {
		y.config.Version = "v1alpha1"
	} else if y.config.Version != "v1alpha1" {
		return fmt.Errorf("unsupported config version: %s", y.config.Version)
	}

	return nil
}

// SaveConfig saves the current configuration to the specified path. If the path is empty, it uses the previously loaded path.
// If the file does not exist, it creates an empty one.
func (y *YamlConfigHandler) SaveConfig(path string) error {
	if path == "" {
		if y.path == "" {
			return fmt.Errorf("path cannot be empty")
		}
		path = y.path
	}

	dir := filepath.Dir(path)
	if err := osMkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("error creating directories: %w", err)
	}

	// Ensure the config version is set to "v1alpha1" before saving
	y.config.Version = "v1alpha1"

	data, err := yamlMarshal(y.config)
	if err != nil {
		return fmt.Errorf("error marshalling yaml: %w", err)
	}

	if err := osWriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("error writing config file: %w", err)
	}
	return nil
}

// SetDefault sets the given context configuration as the default.
func (y *YamlConfigHandler) SetDefault(context v1alpha1.Context) error {
	y.defaultContextConfig = context
	currentContext := y.GetContext()

	contextKey := fmt.Sprintf("contexts.%s", currentContext)

	if y.Get(contextKey) == nil {
		return y.Set(contextKey, &context)
	}

	return nil
}

// Get retrieves the value at the specified path in the configuration. It checks both the current and default context configurations.
func (y *YamlConfigHandler) Get(path string) interface{} {
	if path == "" {
		return nil
	}
	pathKeys := parsePath(path)

	value := getValueByPath(y.config, pathKeys)
	if value == nil && len(pathKeys) >= 2 && pathKeys[0] == "contexts" {
		value = getValueByPath(y.defaultContextConfig, pathKeys[2:])
	}

	return value
}

// GetString retrieves a string value for the specified key from the configuration, with an optional default value.
func (y *YamlConfigHandler) GetString(key string, defaultValue ...string) string {
	contextKey := fmt.Sprintf("contexts.%s.%s", y.context, key)
	value := y.Get(contextKey)
	if value == nil {
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return ""
	}
	strValue := fmt.Sprintf("%v", value)
	secretsProvider := y.secretsProvider
	if secretsProvider != nil {
		parsedValue, err := secretsProvider.ParseSecrets(strValue)
		if err == nil {
			strValue = parsedValue
		}
	}
	return strValue
}

// GetInt retrieves an integer value for the specified key from the configuration, with an optional default value.
func (y *YamlConfigHandler) GetInt(key string, defaultValue ...int) int {
	contextKey := fmt.Sprintf("contexts.%s.%s", y.context, key)
	value := y.Get(contextKey)
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
func (y *YamlConfigHandler) GetBool(key string, defaultValue ...bool) bool {
	contextKey := fmt.Sprintf("contexts.%s.%s", y.context, key)
	value := y.Get(contextKey)
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
func (y *YamlConfigHandler) GetStringSlice(key string, defaultValue ...[]string) []string {
	contextKey := fmt.Sprintf("contexts.%s.%s", y.context, key)
	value := y.Get(contextKey)
	if value == nil {
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return []string{}
	}
	return value.([]string)
}

// GetStringMap retrieves a map of string key-value pairs for the specified key.
// It returns a default map if the key is not found. If a secrets provider is set,
// it parses and replaces secret placeholders in map values, ensuring sensitive data
// like API keys remain secure and not exposed in configuration files.
func (y *YamlConfigHandler) GetStringMap(key string, defaultValue ...map[string]string) map[string]string {
	contextKey := fmt.Sprintf("contexts.%s.%s", y.context, key)
	value := y.Get(contextKey)
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

	// If a secrets provider is available, parse the map values for secret placeholders
	secretsProvider := y.secretsProvider
	if secretsProvider != nil {
		for k, v := range strMap {
			parsedValue, err := secretsProvider.ParseSecrets(v)
			if err == nil {
				strMap[k] = parsedValue
			}
		}
	}

	return strMap
}

// Set updates the value at the specified path in the configuration using reflection.
func (y *YamlConfigHandler) Set(path string, value interface{}) error {
	if path == "" {
		return nil
	}
	pathKeys := parsePath(path)
	configValue := reflect.ValueOf(&y.config)
	return setValueByPath(configValue, pathKeys, value, path)
}

// SetContextValue sets a configuration value within the current context.
func (y *YamlConfigHandler) SetContextValue(path string, value interface{}) error {
	y.GetContext()

	currentContext := y.context
	fullPath := fmt.Sprintf("contexts.%s.%s", currentContext, path)
	return y.Set(fullPath, value)
}

// GetConfig returns the context config object for the current context, or the default if none is set.
func (y *YamlConfigHandler) GetConfig() *v1alpha1.Context {
	defaultConfigCopy := y.defaultContextConfig.DeepCopy()
	context := y.context

	if context == "" {
		return defaultConfigCopy
	}

	if ctx, ok := y.config.Contexts[context]; ok {
		mergedConfig := defaultConfigCopy
		mergedConfig.Merge(ctx)
		return mergedConfig
	}

	return defaultConfigCopy
}

// Ensure YamlConfigHandler implements ConfigHandler
var _ ConfigHandler = (*YamlConfigHandler)(nil)

// getValueByPath retrieves a value by navigating through a struct or map using YAML tags.
func getValueByPath(current interface{}, pathKeys []string) interface{} {
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
	for i := 0; i < v.NumField(); i++ {
		field := t.Field(i)
		yamlTag := strings.Split(field.Tag.Get("yaml"), ",")[0]
		if yamlTag == tag {
			return v.Field(i)
		}
	}
	return reflect.Value{}
}

// setValueByPath sets a value in a struct or map by navigating through it using YAML tags.
func setValueByPath(currValue reflect.Value, pathKeys []string, value interface{}, fullPath string) error {
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

// assignValue assigns a value to a field, converting types if necessary.
func assignValue(fieldValue reflect.Value, value interface{}) (reflect.Value, error) {
	if !fieldValue.CanSet() {
		return reflect.Value{}, fmt.Errorf("cannot set field")
	}

	fieldType := fieldValue.Type()
	valueType := reflect.TypeOf(value)

	if fieldType.Kind() == reflect.Ptr {
		elemType := fieldType.Elem()
		newValue := reflect.New(elemType)
		val := reflect.ValueOf(value)

		if val.Type().ConvertibleTo(elemType) {
			val = val.Convert(elemType)
			newValue.Elem().Set(val)
			return newValue, nil
		}

		return reflect.Value{}, fmt.Errorf("cannot assign value of type %s to field of type %s", valueType, fieldType)
	}

	if valueType.AssignableTo(fieldType) {
		val := reflect.ValueOf(value)
		return val, nil
	}

	if valueType.ConvertibleTo(fieldType) {
		val := reflect.ValueOf(value).Convert(fieldType)
		return val, nil
	}

	return reflect.Value{}, fmt.Errorf("cannot assign value of type %s to field of type %s", valueType, fieldType)
}

// makeAddressable ensures a value is addressable by creating a new pointer if necessary.
func makeAddressable(v reflect.Value) reflect.Value {
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
