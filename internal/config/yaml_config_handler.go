package config

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/goccy/go-yaml"
)

// YamlConfigHandler implements the ConfigHandler interface using goccy/go-yaml
type YamlConfigHandler struct {
	ConfigHandler
	config               Config
	path                 string
	defaultContextConfig Context
}

// NewYamlConfigHandler is a constructor for YamlConfigHandler that accepts a path
func NewYamlConfigHandler() *YamlConfigHandler {
	return &YamlConfigHandler{
		defaultContextConfig: DefaultConfig,
	}
}

// LoadConfig loads the configuration from the specified path
func (y *YamlConfigHandler) LoadConfig(path string) error {
	if _, err := osStat(path); os.IsNotExist(err) {
		// Ensure the directory structure exists
		dir := filepath.Dir(path)
		if err := osMkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("error creating directories: %w", err)
		}

		// Create an empty config file if it does not exist
		if err := osWriteFile(path, []byte{}, 0644); err != nil {
			return fmt.Errorf("error creating config file: %w", err)
		}
	}

	// Read the config file
	data, err := osReadFile(path)
	if err != nil {
		return fmt.Errorf("error reading config file: %w", err)
	}

	// Unmarshal the YAML data into the config struct
	if err := yamlUnmarshal(data, &y.config); err != nil {
		return fmt.Errorf("error unmarshalling yaml: %w", err)
	}
	y.path = path
	return nil
}

// SaveConfig saves the current configuration to the specified path
func (y *YamlConfigHandler) SaveConfig(path string) error {
	if path == "" {
		if y.path == "" {
			return fmt.Errorf("path cannot be empty")
		}
		path = y.path
	}

	// Marshal the config struct into YAML data, omitting null values
	data, err := yamlMarshalNonNull(y.config)
	if err != nil {
		return fmt.Errorf("error marshalling yaml: %w", err)
	}

	// Write the YAML data to the config file
	if err := osWriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("error writing config file: %w", err)
	}
	return nil
}

// SetDefault sets the default context configuration
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

	// Check if the context is defined in the config
	contextKey := fmt.Sprintf("contexts.%s", currentContext)
	if y.Get(contextKey) == nil {
		// If the context is not defined, set it to defaultContextConfig
		return y.Set(contextKey, &context)
	}
	return nil
}

// Get retrieves the value at the specified path in the configuration
func (y *YamlConfigHandler) Get(path string) interface{} {
	if path == "" {
		return nil
	}
	pathKeys := strings.Split(path, ".")

	// Use getValueByPath to navigate the struct using YAML tags
	value := getValueByPath(y.config, pathKeys)
	if value == nil {
		// Value is invalid or nil, proceed to check defaultContextConfig
		if len(pathKeys) >= 2 && pathKeys[0] == "contexts" {
			// Attempt to get the value from defaultContextConfig
			value = getValueByPath(y.defaultContextConfig, pathKeys[2:])
			if value != nil {
				return value
			}
		}
	}

	// Return the value
	return value
}

// GetString retrieves a string value for the specified key from the configuration
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

// GetInt retrieves an integer value for the specified key from the configuration
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

// GetBool retrieves a boolean value for the specified key from the configuration
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

// Set updates the value at the specified path in the configuration
func (y *YamlConfigHandler) Set(path string, value interface{}) error {
	if path == "" {
		return nil
	}
	pathKeys := strings.Split(path, ".")

	// Pass a pointer to y.config to make it addressable
	configValue := reflect.ValueOf(&y.config)

	// Set the value in the configuration by reflection
	err := setValueByPath(configValue, pathKeys, value, path)
	if err != nil {
		return err
	}
	return nil
}

// SetContextValue sets a configuration value within the current context
func (y *YamlConfigHandler) SetContextValue(path string, value interface{}) error {
	if y.config.Context == nil {
		return fmt.Errorf("current context is not set")
	}

	currentContext := *y.config.Context
	fullPath := fmt.Sprintf("contexts.%s.%s", currentContext, path)
	return y.Set(fullPath, value)
}

// GetConfig returns the context config object for the current context
func (y *YamlConfigHandler) GetConfig() *Context {
	context := y.config.Context
	if context == nil {
		return &y.defaultContextConfig
	}
	if ctx, ok := y.config.Contexts[*context]; ok {
		return ctx
	}
	return &y.defaultContextConfig
}

// Ensure YamlConfigHandler implements ConfigHandler
var _ ConfigHandler = (*YamlConfigHandler)(nil)

// getValueByPath is a helper function to get a value by a path from an interface{}
func getValueByPath(current interface{}, pathKeys []string) interface{} {
	if len(pathKeys) == 0 {
		return nil
	}

	currValue := reflect.ValueOf(current)
	if !currValue.IsValid() {
		return nil
	}

	// Traverse the path to get the value
	for _, key := range pathKeys {
		// Handle pointers
		if currValue.Kind() == reflect.Ptr {
			if currValue.IsNil() {
				// Return nil to indicate the value is not set
				// NOTE: Untestable
				return nil
			}
			currValue = currValue.Elem()
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
				// Return nil to indicate the key is not found
				return nil
			}
			currValue = mapValue

		default:
			return nil
		}
	}

	// Handle final pointer
	if currValue.Kind() == reflect.Ptr {
		if currValue.IsNil() {
			// NOTE: Untestable
			return nil
		}
		currValue = currValue.Elem()
	}

	// Check if currValue is valid and can interface
	if currValue.IsValid() && currValue.CanInterface() {
		return currValue.Interface()
	}

	// Return nil since we cannot retrieve a valid value
	return nil
}

// getFieldByYamlTag retrieves a field from a struct by its YAML tag
func getFieldByYamlTag(v reflect.Value, tag string) reflect.Value {
	t := v.Type()
	for i := 0; i < v.NumField(); i++ {
		field := t.Field(i)
		yamlTag := strings.Split(field.Tag.Get("yaml"), ",")[0]
		if yamlTag == tag {
			return v.Field(i)
		}
	}
	// Return zero Value if not found
	return reflect.Value{}
}

// setValueByPath is a helper function to set a value by a path
func setValueByPath(currValue reflect.Value, pathKeys []string, value interface{}, fullPath string) error {
	if len(pathKeys) == 0 {
		return fmt.Errorf("pathKeys cannot be empty")
	}

	key := pathKeys[0]
	isLast := len(pathKeys) == 1

	// Handle pointers
	if currValue.Kind() == reflect.Ptr {
		if currValue.IsNil() {
			currValue.Set(reflect.New(currValue.Type().Elem()))
		}
		currValue = currValue.Elem()
	}

	switch currValue.Kind() {
	case reflect.Struct:
		// Get the field by YAML tag
		fieldValue := getFieldByYamlTag(currValue, key)

		// Initialize nil pointer fields
		if fieldValue.Kind() == reflect.Ptr && fieldValue.IsNil() {
			fieldValue.Set(reflect.New(fieldValue.Type().Elem()))
		}

		// If the field is a nil map, initialize it
		if fieldValue.Kind() == reflect.Map && fieldValue.IsNil() {
			fieldValue.Set(reflect.MakeMap(fieldValue.Type()))
		}

		if isLast {
			// Set the value
			newFieldValue, err := assignValue(fieldValue, value)
			if err != nil {
				return err
			}
			fieldValue.Set(newFieldValue)
		} else {
			// Recurse into the field
			err := setValueByPath(fieldValue, pathKeys[1:], value, fullPath)
			if err != nil {
				// NOTE: Untestable
				return err
			}
		}

	case reflect.Map:
		if currValue.IsNil() {
			// Initialize the map
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
			// Get or create the next value
			nextValue = currValue.MapIndex(mapKey)
			if !nextValue.IsValid() {
				// Need to create a new instance of the map's element type
				nextValue = reflect.New(currValue.Type().Elem()).Elem()
			} else {
				// Map values are unaddressable; make a copy
				nextValue = makeAddressable(nextValue)
			}

			// Recurse into the next value
			err := setValueByPath(nextValue, pathKeys[1:], value, fullPath)
			if err != nil {
				return err
			}

			// Set the modified value back into the map
			currValue.SetMapIndex(mapKey, nextValue)
		}

	default:
		return fmt.Errorf("Invalid path: %s", fullPath)
	}

	return nil
}

// assignValue assigns a value to a field, converting types if necessary
func assignValue(fieldValue reflect.Value, value interface{}) (reflect.Value, error) {
	if !fieldValue.CanSet() {
		return reflect.Value{}, fmt.Errorf("cannot set field")
	}

	fieldType := fieldValue.Type()
	valueType := reflect.TypeOf(value)

	// Handle pointer fields
	if fieldType.Kind() == reflect.Ptr {
		elemType := fieldType.Elem()

		// Create a new value of the element type
		newValue := reflect.New(elemType)

		// Set the value
		val := reflect.ValueOf(value)

		// Handle basic types
		if val.Type().ConvertibleTo(elemType) {
			val = val.Convert(elemType)
			newValue.Elem().Set(val)
			return newValue, nil
		}

		return reflect.Value{}, fmt.Errorf("cannot assign value of type %s to field of type %s", valueType, fieldType)
	}

	// Handle non-pointer fields
	if valueType.AssignableTo(fieldType) {
		val := reflect.ValueOf(value)
		return val, nil
	}

	// Handle convertible types
	if valueType.ConvertibleTo(fieldType) {
		val := reflect.ValueOf(value).Convert(fieldType)
		return val, nil
	}

	return reflect.Value{}, fmt.Errorf("cannot assign value of type %s to field of type %s", valueType, fieldType)
}

func makeAddressable(v reflect.Value) reflect.Value {
	if v.CanAddr() {
		return v
	}
	addr := reflect.New(v.Type())
	addr.Elem().Set(v)
	return addr.Elem()
}

// yamlMarshalNonNull is a custom function to marshal YAML without nil values
var yamlMarshalNonNull = func(v interface{}) ([]byte, error) {
	// Helper function to recursively process the struct
	var convert func(reflect.Value) (interface{}, error)
	convert = func(val reflect.Value) (interface{}, error) {
		switch val.Kind() {
		case reflect.Invalid:
			return nil, nil
		case reflect.Ptr:
			if val.IsNil() {
				// Omit nil pointer fields
				return nil, nil
			}
			// Dereference pointer and continue processing
			return convert(val.Elem())
		case reflect.Struct:
			result := make(map[string]interface{})
			typ := val.Type()
			for i := 0; i < val.NumField(); i++ {
				fieldValue := val.Field(i)
				fieldType := typ.Field(i)

				// Skip unexported fields
				if fieldType.PkgPath != "" {
					continue
				}

				yamlTag := strings.Split(fieldType.Tag.Get("yaml"), ",")[0]
				if yamlTag == "-" {
					// Omit fields with yaml:"-"
					continue
				}
				if yamlTag == "" {
					// Use field name if no YAML tag is specified
					yamlTag = fieldType.Name
				}

				fieldInterface, err := convert(fieldValue)
				if err != nil {
					// NOTE: Untestable
					return nil, err
				}
				if fieldInterface != nil {
					result[yamlTag] = fieldInterface
				}
			}
			// Omit empty structs
			if len(result) == 0 {
				return nil, nil
			}
			return result, nil
		case reflect.Slice, reflect.Array:
			if val.Len() == 0 {
				// Omit empty slices/arrays
				return nil, nil
			}
			var slice []interface{}
			for i := 0; i < val.Len(); i++ {
				elemVal := val.Index(i)
				elemInterface, err := convert(elemVal)
				if err != nil {
					// NOTE: Untestable
					return nil, err
				}
				slice = append(slice, elemInterface)
			}
			return slice, nil
		case reflect.Map:
			if val.Len() == 0 {
				// Omit empty maps
				return nil, nil
			}
			result := make(map[string]interface{})
			for _, key := range val.MapKeys() {
				keyStr := fmt.Sprintf("%v", key.Interface())
				elemVal := val.MapIndex(key)
				elemInterface, err := convert(elemVal)
				if err != nil {
					// NOTE: Untestable
					return nil, err
				}
				if elemInterface != nil {
					result[keyStr] = elemInterface
				}
			}
			return result, nil
		case reflect.Interface:
			if val.IsNil() {
				// Omit nil interfaces
				return nil, nil
			}
			return convert(val.Elem())
		case reflect.String:
			return val.String(), nil
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return val.Int(), nil
		case reflect.Bool:
			return val.Bool(), nil
		default:
			// For other kinds, return an error
			return val.Interface(), nil
		}
	}

	val := reflect.ValueOf(v)
	processed, err := convert(val)
	if err != nil {
		// NOTE: Untestable
		return nil, err
	}
	if processed == nil {
		return []byte{}, nil
	}

	// Using goccy/go-yaml for marshalling
	return yaml.Marshal(processed)
}

// Ensure YamlConfigHandler implements ConfigHandler
var _ ConfigHandler = (*YamlConfigHandler)(nil)
