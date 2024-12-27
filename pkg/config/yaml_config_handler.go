package config

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

// YamlConfigHandler implements the ConfigHandler interface using goccy/go-yaml
type YamlConfigHandler struct {
	ConfigHandler
	config               Config
	path                 string
	defaultContextConfig Context
}

// NewYamlConfigHandler creates a new instance of YamlConfigHandler with default context configuration.
func NewYamlConfigHandler() *YamlConfigHandler {
	return &YamlConfigHandler{
		// defaultContextConfig: DefaultConfig,
	}
}

// LoadConfig loads the configuration from the specified path. If the file does not exist, it does nothing.
func (y *YamlConfigHandler) LoadConfig(path string) error {
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
	y.path = path
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

	data, err := yamlMarshal(y.config)
	if err != nil {
		return fmt.Errorf("error marshalling yaml: %w", err)
	}

	if err := osWriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("error writing config file: %w", err)
	}
	return nil
}

// SetDefault sets the given context configuration as the default. Defaults to "local" if no current context is set.
func (y *YamlConfigHandler) SetDefault(context Context) error {
	y.defaultContextConfig = context
	currentContext := "local"

	if y.config.Context != nil {
		currentContext = *y.config.Context
	} else {
		if err := y.Set("context", currentContext); err != nil {
			return err
		}
	}

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
	pathKeys := strings.Split(path, ".")

	value := getValueByPath(y.config, pathKeys)
	if value == nil && len(pathKeys) >= 2 && pathKeys[0] == "contexts" {
		value = getValueByPath(y.defaultContextConfig, pathKeys[2:])
	}

	return value
}

// GetString retrieves a string value for the specified key from the configuration, with an optional default value.
func (y *YamlConfigHandler) GetString(key string, defaultValue ...string) string {
	contextKey := fmt.Sprintf("contexts.%s.%s", *y.config.Context, key)
	value := y.Get(contextKey)
	if value == nil {
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return ""
	}
	return fmt.Sprintf("%v", value)
}

// GetInt retrieves an integer value for the specified key from the configuration, with an optional default value.
func (y *YamlConfigHandler) GetInt(key string, defaultValue ...int) int {
	contextKey := fmt.Sprintf("contexts.%s.%s", *y.config.Context, key)
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
	contextKey := fmt.Sprintf("contexts.%s.%s", *y.config.Context, key)
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

// Set updates the value at the specified path in the configuration using reflection.
func (y *YamlConfigHandler) Set(path string, value interface{}) error {
	if path == "" {
		return nil
	}
	pathKeys := strings.Split(path, ".")
	configValue := reflect.ValueOf(&y.config)
	return setValueByPath(configValue, pathKeys, value, path)
}

// SetContextValue sets a configuration value within the current context.
func (y *YamlConfigHandler) SetContextValue(path string, value interface{}) error {
	if y.config.Context == nil {
		return fmt.Errorf("current context is not set")
	}

	currentContext := *y.config.Context
	fullPath := fmt.Sprintf("contexts.%s.%s", currentContext, path)
	return y.Set(fullPath, value)
}

// GetConfig returns the context config object for the current context, or the default if none is set.
func (y *YamlConfigHandler) GetConfig() *Context {
	defaultConfigCopy := y.defaultContextConfig.Copy() // Copy the internal default configuration
	context := y.config.Context

	if context == nil {
		return defaultConfigCopy
	}

	if ctx, ok := y.config.Contexts[*context]; ok {
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
