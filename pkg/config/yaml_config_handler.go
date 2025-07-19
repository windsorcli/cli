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
)

// YamlConfigHandler extends BaseConfigHandler to implement YAML-based configuration
// management. It handles serialization/deserialization of v1alpha1.Context objects
// to/from YAML files, with version validation and context-specific overrides. The
// handler maintains configuration state through file-based persistence, implementing
// atomic writes and proper error handling. Configuration values can be accessed through
// strongly-typed getters with support for default values.

type YamlConfigHandler struct {
	BaseConfigHandler
	path                 string
	defaultContextConfig v1alpha1.Context
}

// =============================================================================
// Constructor
// =============================================================================

// NewYamlConfigHandler creates a new instance of YamlConfigHandler with default context configuration.
func NewYamlConfigHandler(injector di.Injector) *YamlConfigHandler {
	return &YamlConfigHandler{
		BaseConfigHandler: *NewBaseConfigHandler(injector),
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// LoadConfigString loads the configuration from the provided string content.
func (y *YamlConfigHandler) LoadConfigString(content string) error {
	if content == "" {
		return nil
	}

	if err := y.shims.YamlUnmarshal([]byte(content), &y.BaseConfigHandler.config); err != nil {
		return fmt.Errorf("error unmarshalling yaml: %w", err)
	}

	// Check and set the config version
	if y.BaseConfigHandler.config.Version == "" {
		y.BaseConfigHandler.config.Version = "v1alpha1"
	} else if y.BaseConfigHandler.config.Version != "v1alpha1" {
		return fmt.Errorf("unsupported config version: %s", y.BaseConfigHandler.config.Version)
	}

	y.BaseConfigHandler.loaded = true
	return nil
}

// LoadConfig loads the configuration from the specified path. If the file does not exist, it does nothing.
func (y *YamlConfigHandler) LoadConfig(path string) error {
	y.path = path
	if _, err := y.shims.Stat(path); os.IsNotExist(err) {
		return nil
	}

	data, err := y.shims.ReadFile(path)
	if err != nil {
		return fmt.Errorf("error reading config file: %w", err)
	}

	return y.LoadConfigString(string(data))
}

// SaveConfig saves the current configuration to the specified path. If the path is empty, it uses the previously loaded path.
// If overwrite is false and the file exists, it will not overwrite the file
func (y *YamlConfigHandler) SaveConfig(path string, overwrite ...bool) error {
	shouldOverwrite := true
	if len(overwrite) > 0 {
		shouldOverwrite = overwrite[0]
	}

	if path == "" {
		if y.path == "" {
			return fmt.Errorf("path cannot be empty")
		}
		path = y.path
	}

	dir := filepath.Dir(path)
	if err := y.shims.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("error creating directories: %w", err)
	}

	// Check if file exists and we should not overwrite
	if !shouldOverwrite {
		if _, err := y.shims.Stat(path); err == nil {
			return nil
		}
	}

	// Ensure the config version is set to "v1alpha1" before saving
	y.config.Version = "v1alpha1"

	data, err := y.shims.YamlMarshal(y.config)
	if err != nil {
		return fmt.Errorf("error marshalling yaml: %w", err)
	}

	if err := y.shims.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("error writing config file: %w", err)
	}
	return nil
}

// SetDefault sets the given context configuration as the default and merges it with any
// existing context configuration. If no context exists, the default becomes the context.
// If a context exists, it merges the default with the existing context, with existing
// values taking precedence over defaults.
func (y *YamlConfigHandler) SetDefault(context v1alpha1.Context) error {
	y.defaultContextConfig = context
	currentContext := y.GetContext()

	contextKey := fmt.Sprintf("contexts.%s", currentContext)

	if y.Get(contextKey) == nil {
		return y.Set(contextKey, &context)
	} else {
		if y.config.Contexts == nil {
			y.config.Contexts = make(map[string]*v1alpha1.Context)
		}

		if y.config.Contexts[currentContext] == nil {
			y.config.Contexts[currentContext] = &v1alpha1.Context{}
		}

		defaultCopy := context.DeepCopy()
		existingCopy := y.config.Contexts[currentContext].DeepCopy()
		defaultCopy.Merge(existingCopy)
		y.config.Contexts[currentContext] = defaultCopy
	}

	return nil
}

// Get retrieves the value at the specified path in the configuration. It checks both the current and default context configurations.
func (y *YamlConfigHandler) Get(path string) any {
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
// If the key is not found, it returns the provided default value or an empty string if no default is provided.
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
	strSlice, ok := value.([]string)
	if !ok {
		return []string{}
	}
	return strSlice
}

// GetStringMap retrieves a map of string key-value pairs for the specified key from the configuration.
// If the key is not found, it returns the provided default value or an empty map if no default is provided.
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

	return strMap
}

// Set updates the value at the specified path in the configuration using reflection.
func (y *YamlConfigHandler) Set(path string, value any) error {
	if path == "" {
		return nil
	}

	pathKeys := parsePath(path)
	if len(pathKeys) == 0 {
		return fmt.Errorf("invalid path: %s", path)
	}

	// If the value is a string, try to convert it based on the target type
	if strValue, ok := value.(string); ok {
		currentValue := y.Get(path)
		if currentValue != nil {
			targetType := reflect.TypeOf(currentValue)
			convertedValue, err := convertValue(strValue, targetType)
			if err != nil {
				return fmt.Errorf("error converting value for %s: %w", path, err)
			}
			value = convertedValue
		}
	}

	configValue := reflect.ValueOf(&y.config)
	return setValueByPath(configValue, pathKeys, value, path)
}

// SetContextValue sets a configuration value within the current context.
func (y *YamlConfigHandler) SetContextValue(path string, value any) error {
	if path == "" {
		return fmt.Errorf("path cannot be empty")
	}

	// Initialize contexts map if it doesn't exist
	if y.config.Contexts == nil {
		y.config.Contexts = make(map[string]*v1alpha1.Context)
	}

	// Get or create the current context
	contextName := y.GetContext()
	if y.config.Contexts[contextName] == nil {
		y.config.Contexts[contextName] = &v1alpha1.Context{}
	}

	// Use the generic Set method with the full context path
	fullPath := fmt.Sprintf("contexts.%s.%s", contextName, path)
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

// =============================================================================
// Private Methods
// =============================================================================

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

// GenerateContextID generates a random context ID if one doesn't exist
func (y *YamlConfigHandler) GenerateContextID() error {
	if y.GetString("id") != "" {
		return nil
	}

	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 7)
	if _, err := y.shims.CryptoRandRead(b); err != nil {
		return fmt.Errorf("failed to generate random context ID: %w", err)
	}

	for i := range b {
		b[i] = charset[int(b[i])%len(charset)]
	}

	id := "w" + string(b)
	return y.SetContextValue("id", id)
}

// YamlMarshalWithDefinedPaths marshals data to YAML format while ensuring all parent paths are defined.
// It handles struct fields, slices, maps, and primitive types with proper YAML tag handling and nil value representation.
// This method ensures that empty slices and maps are explicitly defined as empty rather than being omitted,
// which is important for configuration templates that need to show structure even when empty.
func (y *YamlConfigHandler) YamlMarshalWithDefinedPaths(v any) ([]byte, error) {
	if v == nil {
		return nil, fmt.Errorf("invalid input: nil value")
	}

	var convert func(reflect.Value) (any, error)
	convert = func(val reflect.Value) (any, error) {
		switch val.Kind() {
		case reflect.Ptr, reflect.Interface:
			if val.IsNil() {
				if val.Kind() == reflect.Interface || (val.Kind() == reflect.Ptr && val.Type().Elem().Kind() == reflect.Struct) {
					return make(map[string]any), nil
				}
				return nil, nil
			}
			return convert(val.Elem())
		case reflect.Struct:
			result := make(map[string]any)
			typ := val.Type()
			for i := range make([]int, val.NumField()) {
				fieldValue := val.Field(i)
				fieldType := typ.Field(i)

				if fieldType.PkgPath != "" {
					continue
				}

				yamlTag := strings.Split(fieldType.Tag.Get("yaml"), ",")[0]
				if yamlTag == "-" {
					continue
				}
				if yamlTag == "" {
					yamlTag = fieldType.Name
				}

				fieldInterface, err := convert(fieldValue)
				if err != nil {
					return nil, fmt.Errorf("error converting field %s: %w", fieldType.Name, err)
				}
				if fieldInterface != nil || fieldType.Type.Kind() == reflect.Interface || fieldType.Type.Kind() == reflect.Slice || fieldType.Type.Kind() == reflect.Map || fieldType.Type.Kind() == reflect.Struct {
					result[yamlTag] = fieldInterface
				}
			}
			return result, nil
		case reflect.Slice, reflect.Array:
			if val.Len() == 0 {
				return []any{}, nil
			}
			slice := make([]any, val.Len())
			for i := 0; i < val.Len(); i++ {
				elemVal := val.Index(i)
				if elemVal.Kind() == reflect.Ptr || elemVal.Kind() == reflect.Interface {
					if elemVal.IsNil() {
						slice[i] = nil
						continue
					}
				}
				elemInterface, err := convert(elemVal)
				if err != nil {
					return nil, fmt.Errorf("error converting slice element at index %d: %w", i, err)
				}
				slice[i] = elemInterface
			}
			return slice, nil
		case reflect.Map:
			result := make(map[string]any)
			for _, key := range val.MapKeys() {
				keyStr := fmt.Sprintf("%v", key.Interface())
				elemVal := val.MapIndex(key)
				if elemVal.Kind() == reflect.Interface && elemVal.IsNil() {
					result[keyStr] = nil
					continue
				}
				elemInterface, err := convert(elemVal)
				if err != nil {
					return nil, fmt.Errorf("error converting map value for key %s: %w", keyStr, err)
				}
				if elemInterface != nil || elemVal.Kind() == reflect.Interface || elemVal.Kind() == reflect.Slice || elemVal.Kind() == reflect.Map || elemVal.Kind() == reflect.Struct {
					result[keyStr] = elemInterface
				}
			}
			return result, nil
		case reflect.String:
			return val.String(), nil
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return val.Int(), nil
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			return val.Uint(), nil
		case reflect.Float32, reflect.Float64:
			return val.Float(), nil
		case reflect.Bool:
			return val.Bool(), nil
		default:
			return nil, fmt.Errorf("unsupported value type %s", val.Kind())
		}
	}

	val := reflect.ValueOf(v)
	if val.Kind() == reflect.Func {
		return nil, fmt.Errorf("unsupported value type func")
	}

	processed, err := convert(val)
	if err != nil {
		return nil, err
	}

	yamlData, err := y.shims.YamlMarshal(processed)
	if err != nil {
		return nil, fmt.Errorf("error marshalling yaml: %w", err)
	}

	return yamlData, nil
}
