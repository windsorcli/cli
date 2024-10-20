package config

import (
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/goccy/go-yaml"
)

// YamlConfigHandler implements the ConfigHandler interface using goccy/go-yaml
type YamlConfigHandler struct {
	config               Config
	path                 string
	defaultContextConfig Context
}

// NewYamlConfigHandler is a constructor for YamlConfigHandler that accepts a path
func NewYamlConfigHandler(path string) (*YamlConfigHandler, error) {
	handler := &YamlConfigHandler{}
	if path != "" {
		if err := handler.LoadConfig(path); err != nil {
			return nil, fmt.Errorf("error loading config: %w", err)
		}
	}
	return handler, nil
}

// osReadFile is a variable to allow mocking os.ReadFile in tests
var osReadFile = os.ReadFile

// osWriteFile is a variable to allow mocking os.WriteFile in tests
var osWriteFile = os.WriteFile

// Override variable for yamlMarshal
var yamlMarshal = yaml.Marshal

// Override variable for yamlUnmarshal
var yamlUnmarshal = yaml.Unmarshal

// osStat is a variable to allow mocking os.Stat in tests
var osStat = os.Stat

// LoadConfig loads the configuration from the specified path
func (y *YamlConfigHandler) LoadConfig(path string) error {
	if _, err := osStat(path); os.IsNotExist(err) {
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

	// Marshal the config struct into YAML data
	data, err := yamlMarshal(y.config)
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
func (y *YamlConfigHandler) SetDefault(context Context) {
	y.defaultContextConfig = context
}

// GetString retrieves a string value for the specified key from the configuration
func (y *YamlConfigHandler) GetString(key string, defaultValue ...string) (string, error) {
	value, err := y.Get(key)
	if err != nil {
		if len(defaultValue) > 0 {
			return defaultValue[0], nil
		}
		return "", err
	}
	return fmt.Sprintf("%v", value), nil
}

// GetInt retrieves an integer value for the specified key from the configuration
func (y *YamlConfigHandler) GetInt(key string, defaultValue ...int) (int, error) {
	value, err := y.Get(key)
	if err != nil {
		if len(defaultValue) > 0 {
			return defaultValue[0], nil
		}
		return 0, err
	}
	if intValue, ok := value.(int); ok {
		return intValue, nil
	}
	return 0, fmt.Errorf("key %s is not an integer", key)
}

// GetBool retrieves a boolean value for the specified key from the configuration
func (y *YamlConfigHandler) GetBool(key string, defaultValue ...bool) (bool, error) {
	value, err := y.Get(key)
	if err != nil {
		if len(defaultValue) > 0 {
			return defaultValue[0], nil
		}
		return false, err
	}
	if boolValue, ok := value.(bool); ok {
		return boolValue, nil
	}
	return false, fmt.Errorf("key %s is not a boolean", key)
}

// Set updates the value at the specified path in the configuration
func (y *YamlConfigHandler) Set(path string, value interface{}) error {
	pathKeys := strings.Split(path, ".")
	if path == "" {
		return fmt.Errorf("invalid path")
	}

	// Pass a pointer to y.config to make it addressable
	configValue := reflect.ValueOf(&y.config)

	// Set the value in the configuration by reflection
	err := setValueByPath(configValue, pathKeys, value)
	if err != nil {
		return err
	}

	// y.config is already modified via the pointer
	return nil
}

// Get retrieves the value at the specified path in the configuration
func (y *YamlConfigHandler) Get(path string) (interface{}, error) {
	if path == "" {
		return nil, fmt.Errorf("invalid path")
	}
	pathKeys := strings.Split(path, ".")

	// Get the value in the configuration by path
	value, err := getValueByPath(y.config, pathKeys)
	if err != nil {
		return nil, err
	}

	return value, nil
}

// Ensure YamlConfigHandler implements ConfigHandler
var _ ConfigHandler = (*YamlConfigHandler)(nil)

// setValueByPath is a helper function to set a value by a path
func setValueByPath(currValue reflect.Value, pathKeys []string, value interface{}) error {
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
		fieldValue, err := getFieldByYamlTag(currValue, key)
		if err != nil {
			return fmt.Errorf("field %s not found in struct: %w", key, err)
		}

		if isLast {
			// Set the value
			newFieldValue, err := assignValue(fieldValue, value)
			if err != nil {
				return err
			}
			if !fieldValue.CanSet() {
				return fmt.Errorf("cannot set field %s", key)
			}
			fieldValue.Set(newFieldValue)
		} else {
			// Recurse into the field
			err := setValueByPath(fieldValue, pathKeys[1:], value)
			if err != nil {
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
			err := setValueByPath(nextValue, pathKeys[1:], value)
			if err != nil {
				return err
			}

			// Set the modified value back into the map
			currValue.SetMapIndex(mapKey, nextValue)
		}

	default:
		return fmt.Errorf("unsupported kind %s", currValue.Kind())
	}

	return nil
}

// getValueByPath is a helper function to get a value by a path
func getValueByPath(current interface{}, pathKeys []string) (interface{}, error) {
	if len(pathKeys) == 0 {
		return nil, fmt.Errorf("pathKeys cannot be empty")
	}

	currValue := reflect.ValueOf(current)
	if !currValue.IsValid() {
		return nil, fmt.Errorf("current value is invalid")
	}

	// Traverse the path to get the value
	for _, key := range pathKeys {
		switch currValue.Kind() {
		case reflect.Struct:
			fieldValue, err := getFieldByYamlTag(currValue, key)
			if err != nil {
				return nil, err
			}
			currValue = fieldValue

		case reflect.Map:
			mapKey := reflect.ValueOf(key)
			if !mapKey.Type().AssignableTo(currValue.Type().Key()) {
				return nil, fmt.Errorf("key type mismatch: expected %s, got %s", currValue.Type().Key(), mapKey.Type())
			}
			mapValue := currValue.MapIndex(mapKey)
			if !mapValue.IsValid() {
				return nil, fmt.Errorf("key %s not found", key)
			}
			currValue = mapValue

		default:
			return nil, fmt.Errorf("unsupported kind %s", currValue.Kind())
		}
	}

	return currValue.Interface(), nil
}

// getFieldByYamlTag retrieves a field from a struct by its YAML tag
func getFieldByYamlTag(s reflect.Value, tag string) (reflect.Value, error) {
	sType := s.Type()
	for i := 0; i < sType.NumField(); i++ {
		field := sType.Field(i)
		if field.PkgPath != "" {
			continue
		}
		yamlTag := field.Tag.Get("yaml")
		if yamlTag == "" {
			yamlTag = strings.ToLower(field.Name)
		}
		if yamlTag == tag {
			fieldValue := s.Field(i)
			// Initialize zero value if pointer and nil
			if fieldValue.Kind() == reflect.Ptr && fieldValue.IsNil() {
				fieldValue.Set(reflect.New(fieldValue.Type().Elem()))
			}
			return fieldValue, nil
		}
	}
	return reflect.Value{}, fmt.Errorf("field with yaml tag %s not found", tag)
}

// assignValue assigns a value to a field, converting types if necessary
func assignValue(field reflect.Value, value interface{}) (reflect.Value, error) {
	val := reflect.ValueOf(value)
	if !val.Type().AssignableTo(field.Type()) {
		if val.Type().ConvertibleTo(field.Type()) {
			val = val.Convert(field.Type())
		} else {
			return reflect.Value{}, fmt.Errorf("cannot assign value of type %s to field of type %s", val.Type(), field.Type())
		}
	}
	return val, nil
}

func makeAddressable(v reflect.Value) reflect.Value {
	if v.CanAddr() {
		return v
	}
	addr := reflect.New(v.Type())
	addr.Elem().Set(v)
	return addr.Elem()
}
