package config

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/api/v1alpha1/cluster"
	"github.com/windsorcli/cli/api/v1alpha1/vm"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/secrets"
	"github.com/windsorcli/cli/pkg/shell"
)

// TestGetValueByPath tests the getValueByPath function
func Test_getValueByPath(t *testing.T) {
	t.Run("EmptyPathKeys", func(t *testing.T) {
		// Given an empty pathKeys slice for value lookup
		var current any
		pathKeys := []string{}

		// When calling getValueByPath with empty pathKeys
		value := getValueByPath(current, pathKeys)

		// Then nil should be returned as the path is invalid
		if value != nil {
			t.Errorf("Expected value to be nil, got %v", value)
		}
	})

	t.Run("InvalidCurrentValue", func(t *testing.T) {
		// Given a nil current value and a valid path key
		var current any = nil
		pathKeys := []string{"key"}

		// When calling getValueByPath with nil current value
		value := getValueByPath(current, pathKeys)

		// Then nil should be returned as the current value is invalid
		if value != nil {
			t.Errorf("Expected value to be nil, got %v", value)
		}
	})

	t.Run("MapKeyTypeMismatch", func(t *testing.T) {
		// Given a map with int keys but attempting to access with a string key
		current := map[int]string{1: "one", 2: "two"}
		pathKeys := []string{"1"}

		// When calling getValueByPath with mismatched key type
		value := getValueByPath(current, pathKeys)

		// Then nil should be returned due to key type mismatch
		if value != nil {
			t.Errorf("Expected value to be nil, got %v", value)
		}
	})

	t.Run("MapSuccess", func(t *testing.T) {
		// Given a map with a string key and corresponding value
		current := map[string]string{"key": "testValue"}
		pathKeys := []string{"key"}

		// When calling getValueByPath with a valid key
		value := getValueByPath(current, pathKeys)

		// Then the corresponding value should be returned successfully
		if value == nil {
			t.Errorf("Expected value to be 'testValue', got nil")
		}
		expectedValue := "testValue"
		if value != expectedValue {
			t.Errorf("Expected value '%s', got '%v'", expectedValue, value)
		}
	})

	t.Run("CannotSetField", func(t *testing.T) {
		// Given a struct with an unexported field that cannot be set
		type TestStruct struct {
			unexportedField string `yaml:"unexportedfield"`
		}
		testStruct := &TestStruct{}
		currValue := reflect.ValueOf(testStruct)
		pathKeys := []string{"unexportedfield"}
		value := "testValue"
		fullPath := "unexportedfield"

		// When attempting to set a value on the unexported field
		err := setValueByPath(currValue, pathKeys, value, fullPath)

		// Then an error should be returned indicating the field cannot be set
		expectedErr := "cannot set field"
		if err == nil || err.Error() != expectedErr {
			t.Errorf("Expected error '%s', got '%v'", expectedErr, err)
		}
	})

	t.Run("RecursiveFailure", func(t *testing.T) {
		// Given a nested map structure without the target field
		level3Map := map[string]any{}
		level2Map := map[string]any{"level3": level3Map}
		level1Map := map[string]any{"level2": level2Map}
		testMap := map[string]any{"level1": level1Map}
		currValue := reflect.ValueOf(testMap)
		pathKeys := []string{"level1", "level2", "nonexistentfield"}
		value := "newValue"
		fullPath := "level1.level2.nonexistentfield"

		// When attempting to set a value at a non-existent nested path
		err := setValueByPath(currValue, pathKeys, value, fullPath)

		// Then an error should be returned indicating the invalid path
		expectedErr := "Invalid path: level1.level2.nonexistentfield"
		if err == nil || err.Error() != expectedErr {
			t.Errorf("Expected error '%s', got '%v'", expectedErr, err)
		}
	})

	t.Run("AssignValueTypeMismatch", func(t *testing.T) {
		// Given a struct with an int field that cannot accept a string slice
		type TestStruct struct {
			IntField int `yaml:"intfield"`
		}
		testStruct := &TestStruct{}
		currValue := reflect.ValueOf(testStruct)
		pathKeys := []string{"intfield"}
		value := []string{"incompatibleType"} // A slice, which is incompatible with int
		fullPath := "intfield"

		// When attempting to assign an incompatible value type
		err := setValueByPath(currValue, pathKeys, value, fullPath)

		// Then an error should be returned indicating the type mismatch
		expectedErr := "cannot assign value of type []string to field of type int"
		if err == nil || err.Error() != expectedErr {
			t.Errorf("Expected error '%s', got '%v'", expectedErr, err)
		}
	})

	t.Run("AssignPointerValueTypeMismatch", func(t *testing.T) {
		// Given a struct with a pointer field that cannot accept a string slice
		type TestStruct struct {
			IntPtrField *int `yaml:"intptrfield"`
		}
		testStruct := &TestStruct{}
		currValue := reflect.ValueOf(testStruct)
		pathKeys := []string{"intptrfield"}
		value := []string{"incompatibleType"} // A slice, which is incompatible with *int
		fullPath := "intptrfield"

		// When attempting to assign an incompatible value type to a pointer field
		err := setValueByPath(currValue, pathKeys, value, fullPath)

		// Then an error should be returned indicating the pointer type mismatch
		expectedErr := "cannot assign value of type []string to field of type *int"
		if err == nil || err.Error() != expectedErr {
			t.Errorf("Expected error '%s', got '%v'", expectedErr, err)
		}
	})

	t.Run("AssignNonPointerField", func(t *testing.T) {
		// Given a struct with a string field that can be directly assigned
		type TestStruct struct {
			StringField string `yaml:"stringfield"`
		}
		testStruct := &TestStruct{}
		currValue := reflect.ValueOf(testStruct)
		pathKeys := []string{"stringfield"}
		value := "testValue" // Directly assignable to string
		fullPath := "stringfield"

		// When assigning a compatible value to the field
		err := setValueByPath(currValue, pathKeys, value, fullPath)

		// Then the field should be set without error
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if testStruct.StringField != "testValue" {
			t.Errorf("Expected StringField to be 'testValue', got '%v'", testStruct.StringField)
		}
	})

	t.Run("AssignConvertibleType", func(t *testing.T) {
		// Given a struct with an int field that can accept a convertible float value
		type TestStruct struct {
			IntField int `yaml:"intfield"`
		}
		testStruct := &TestStruct{}
		currValue := reflect.ValueOf(testStruct)
		pathKeys := []string{"intfield"}
		value := 42.0 // A float64, which is convertible to int
		fullPath := "intfield"

		// When assigning a value that can be converted to the field's type
		err := setValueByPath(currValue, pathKeys, value, fullPath)

		// Then the field should be set without error
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if testStruct.IntField != 42 {
			t.Errorf("Expected IntField to be 42, got '%v'", testStruct.IntField)
		}
	})
}

func Test_parsePath(t *testing.T) {
	t.Run("EmptyPath", func(t *testing.T) {
		// Given an empty path string to parse
		path := ""

		// When calling parsePath with the empty string
		pathKeys := parsePath(path)

		// Then an empty slice should be returned
		if len(pathKeys) != 0 {
			t.Errorf("Expected pathKeys to be empty, got %v", pathKeys)
		}
	})

	t.Run("SingleKey", func(t *testing.T) {
		// Given a path with a single key
		path := "key"

		// When calling parsePath with a single key
		pathKeys := parsePath(path)

		// Then a slice with only that key should be returned
		expected := []string{"key"}
		if !reflect.DeepEqual(pathKeys, expected) {
			t.Errorf("Expected pathKeys to be %v, got %v", expected, pathKeys)
		}
	})

	t.Run("MultipleKeys", func(t *testing.T) {
		// Given a path with multiple keys separated by dots
		path := "key1.key2.key3"

		// When calling parsePath with dot notation
		pathKeys := parsePath(path)

		// Then a slice containing all the keys should be returned
		expected := []string{"key1", "key2", "key3"}
		if !reflect.DeepEqual(pathKeys, expected) {
			t.Errorf("Expected pathKeys to be %v, got %v", expected, pathKeys)
		}
	})

	t.Run("KeysWithBrackets", func(t *testing.T) {
		// Given a path with keys using bracket notation
		path := "key1[key2][key3]"

		// When calling parsePath with bracket notation
		pathKeys := parsePath(path)

		// Then a slice containing all the keys without brackets should be returned
		expected := []string{"key1", "key2", "key3"}
		if !reflect.DeepEqual(pathKeys, expected) {
			t.Errorf("Expected pathKeys to be %v, got %v", expected, pathKeys)
		}
	})

	t.Run("MixedDotAndBracketNotation", func(t *testing.T) {
		// Given a path with mixed dot and bracket notation
		path := "key1.key2[key3].key4[key5]"

		// When calling parsePath with mixed notation
		pathKeys := parsePath(path)

		// Then a slice with all keys regardless of notation should be returned
		expected := []string{"key1", "key2", "key3", "key4", "key5"}
		if !reflect.DeepEqual(pathKeys, expected) {
			t.Errorf("Expected pathKeys to be %v, got %v", expected, pathKeys)
		}
	})

	t.Run("DotInsideBrackets", func(t *testing.T) {
		// Given a path with a dot inside bracket notation
		path := "key1[key2.key3]"

		// When calling parsePath with a dot inside brackets
		pathKeys := parsePath(path)

		// Then the dot inside brackets should be treated as part of the key
		expected := []string{"key1", "key2.key3"}
		if !reflect.DeepEqual(pathKeys, expected) {
			t.Errorf("Expected pathKeys to be %v, got %v", expected, pathKeys)
		}
	})
}

func Test_assignValue(t *testing.T) {
	t.Run("CannotSetField", func(t *testing.T) {
		// Given an unexported field that cannot be set
		var unexportedField struct {
			unexported int
		}
		fieldValue := reflect.ValueOf(&unexportedField).Elem().Field(0)

		// When attempting to assign a value to it
		_, err := assignValue(fieldValue, 10)

		// Then an error should be returned
		if err == nil {
			t.Errorf("Expected an error for non-settable field, got nil")
		}
		expectedError := "cannot set field"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("PointerTypeMismatchNonConvertible", func(t *testing.T) {
		// Given a pointer field of type *int
		var field *int
		fieldValue := reflect.ValueOf(&field).Elem()

		// When attempting to assign a string value to it
		value := "not an int"
		_, err := assignValue(fieldValue, value)

		// Then an error should be returned indicating type mismatch
		if err == nil {
			t.Errorf("Expected an error for pointer type mismatch, got nil")
		}
		expectedError := "cannot assign value of type string to field of type *int"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("ValueTypeMismatchNonConvertible", func(t *testing.T) {
		// Given a field of type int
		var field int
		fieldValue := reflect.ValueOf(&field).Elem()

		// When attempting to assign a non-convertible string value to it
		value := "not convertible to int"
		_, err := assignValue(fieldValue, value)

		// Then an error should be returned indicating type mismatch
		if err == nil {
			t.Errorf("Expected an error for non-convertible type mismatch, got nil")
		}
		expectedError := "cannot assign value of type string to field of type int"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})
}

func Test_convertValue(t *testing.T) {
	t.Run("ConvertStringToBool", func(t *testing.T) {
		// Given a string value that can be converted to bool
		value := "true"
		targetType := reflect.TypeOf(true)

		// When converting the value
		result, err := convertValue(value, targetType)

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		// And the result should be a bool
		if result != true {
			t.Errorf("Expected true, got %v", result)
		}
	})

	t.Run("ConvertStringToInt", func(t *testing.T) {
		// Given a string value that can be converted to int
		value := "42"
		targetType := reflect.TypeOf(int(0))

		// When converting the value
		result, err := convertValue(value, targetType)

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		// And the result should be an int
		if result != 42 {
			t.Errorf("Expected 42, got %v", result)
		}
	})

	t.Run("ConvertStringToFloat", func(t *testing.T) {
		// Given a string value that can be converted to float
		value := "3.14"
		targetType := reflect.TypeOf(float64(0))

		// When converting the value
		result, err := convertValue(value, targetType)

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		// And the result should be a float
		if result != 3.14 {
			t.Errorf("Expected 3.14, got %v", result)
		}
	})

	t.Run("ConvertStringToPointer", func(t *testing.T) {
		// Given a string value that can be converted to a pointer type
		value := "42"
		targetType := reflect.TypeOf((*int)(nil))

		// When converting the value
		result, err := convertValue(value, targetType)

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		// And the result should be a pointer to int
		if ptr, ok := result.(*int); !ok || *ptr != 42 {
			t.Errorf("Expected *int(42), got %v", result)
		}
	})

	t.Run("UnsupportedType", func(t *testing.T) {
		// Given a string value and an unsupported target type
		value := "test"
		targetType := reflect.TypeOf([]string{})

		// When converting the value
		_, err := convertValue(value, targetType)

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error for unsupported type")
		}

		// And the error message should indicate the unsupported type
		expectedErr := "unsupported type conversion from string to []string"
		if err.Error() != expectedErr {
			t.Errorf("Expected error '%s', got '%s'", expectedErr, err.Error())
		}
	})

	t.Run("InvalidNumericValue", func(t *testing.T) {
		// Given an invalid numeric string value
		value := "not a number"
		targetType := reflect.TypeOf(int(0))

		// When converting the value
		_, err := convertValue(value, targetType)

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error for invalid numeric value")
		}
	})

	t.Run("UintTypes", func(t *testing.T) {
		// Given a string value and uint target types
		value := "42"
		targetTypes := []reflect.Type{
			reflect.TypeOf(uint(0)),
			reflect.TypeOf(uint8(0)),
			reflect.TypeOf(uint16(0)),
			reflect.TypeOf(uint32(0)),
			reflect.TypeOf(uint64(0)),
		}

		// When converting the value to each type
		for _, targetType := range targetTypes {
			result, err := convertValue(value, targetType)

			// Then no error should be returned
			if err != nil {
				t.Fatalf("convertValue() unexpected error for %v: %v", targetType, err)
			}

			// And the value should be correctly converted
			switch targetType.Kind() {
			case reflect.Uint:
				if result != uint(42) {
					t.Errorf("convertValue() = %v, want %v for %v", result, uint(42), targetType)
				}
			case reflect.Uint8:
				if result != uint8(42) {
					t.Errorf("convertValue() = %v, want %v for %v", result, uint8(42), targetType)
				}
			case reflect.Uint16:
				if result != uint16(42) {
					t.Errorf("convertValue() = %v, want %v for %v", result, uint16(42), targetType)
				}
			case reflect.Uint32:
				if result != uint32(42) {
					t.Errorf("convertValue() = %v, want %v for %v", result, uint32(42), targetType)
				}
			case reflect.Uint64:
				if result != uint64(42) {
					t.Errorf("convertValue() = %v, want %v for %v", result, uint64(42), targetType)
				}
			}
		}
	})

	t.Run("IntTypes", func(t *testing.T) {
		// Given a string value and int target types
		value := "42"
		targetTypes := []reflect.Type{
			reflect.TypeOf(int8(0)),
			reflect.TypeOf(int16(0)),
			reflect.TypeOf(int32(0)),
			reflect.TypeOf(int64(0)),
		}

		// When converting the value to each type
		for _, targetType := range targetTypes {
			result, err := convertValue(value, targetType)

			// Then no error should be returned
			if err != nil {
				t.Fatalf("convertValue() unexpected error for %v: %v", targetType, err)
			}

			// And the value should be correctly converted
			switch targetType.Kind() {
			case reflect.Int8:
				if result != int8(42) {
					t.Errorf("convertValue() = %v, want %v for %v", result, int8(42), targetType)
				}
			case reflect.Int16:
				if result != int16(42) {
					t.Errorf("convertValue() = %v, want %v for %v", result, int16(42), targetType)
				}
			case reflect.Int32:
				if result != int32(42) {
					t.Errorf("convertValue() = %v, want %v for %v", result, int32(42), targetType)
				}
			case reflect.Int64:
				if result != int64(42) {
					t.Errorf("convertValue() = %v, want %v for %v", result, int64(42), targetType)
				}
			}
		}
	})

	t.Run("Float32", func(t *testing.T) {
		// Given a string value and float32 target type
		value := "3.14"
		targetType := reflect.TypeOf(float32(0))

		// When converting the value
		result, err := convertValue(value, targetType)

		// Then no error should be returned
		if err != nil {
			t.Fatalf("convertValue() unexpected error: %v", err)
		}

		// And the value should be correctly converted
		if result != float32(3.14) {
			t.Errorf("convertValue() = %v, want %v", result, float32(3.14))
		}
	})

	t.Run("StringToFloatOverflow", func(t *testing.T) {
		// Given a string value that would overflow float32
		value := "3.4028236e+38"
		targetType := reflect.TypeOf(float32(0))

		// When converting the value
		_, err := convertValue(value, targetType)

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error for float overflow")
		}

		// And the error message should indicate overflow
		if !strings.Contains(err.Error(), "float overflow") {
			t.Errorf("Expected error containing 'float overflow', got '%s'", err.Error())
		}
	})
}

func TestConfigHandler_SetDefault(t *testing.T) {
	setup := func(t *testing.T) (ConfigHandler, *Mocks) {
		mocks := setupMocks(t)
		handler := NewConfigHandler(mocks.Injector)
		if err := handler.Initialize(); err != nil {
			t.Fatalf("Failed to initialize handler: %v", err)
		}
		handler.(*configHandler).shims = mocks.Shims
		return handler, mocks
	}

	t.Run("SetDefaultWithExistingContext", func(t *testing.T) {
		// Given a handler with a context set
		handler, _ := setup(t)
		defaultContext := v1alpha1.Context{
			Environment: map[string]string{
				"ENV_VAR": "value",
			},
		}

		// And a context is set
		handler.Set("context", "local")

		// When setting the default context
		err := handler.SetDefault(defaultContext)

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		// And the default context should be set correctly
		if handler.(*configHandler).defaultContextConfig.Environment["ENV_VAR"] != "value" {
			t.Errorf("SetDefault() = %v, expected %v", handler.(*configHandler).defaultContextConfig.Environment["ENV_VAR"], "value")
		}
	})

	t.Run("SetDefaultWithNoContext", func(t *testing.T) {
		// Given a handler with no context set
		handler, _ := setup(t)
		handler.(*configHandler).context = ""
		defaultContext := v1alpha1.Context{
			Environment: map[string]string{
				"ENV_VAR": "value",
			},
		}

		// When setting the default context
		err := handler.SetDefault(defaultContext)

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		// And the default context should be set correctly
		if handler.(*configHandler).defaultContextConfig.Environment["ENV_VAR"] != "value" {
			t.Errorf("SetDefault() = %v, expected %v", handler.(*configHandler).defaultContextConfig.Environment["ENV_VAR"], "value")
		}
	})

	t.Run("SetDefaultUsedInSubsequentOperations", func(t *testing.T) {
		// Given a handler with an existing context
		handler, _ := setup(t)
		handler.(*configHandler).context = "existing-context"
		handler.(*configHandler).config.Contexts = map[string]*v1alpha1.Context{
			"existing-context": {},
		}

		// And a default context configuration
		defaultConf := v1alpha1.Context{
			Environment: map[string]string{"DEFAULT_VAR": "default_val"},
		}

		// When setting the default context
		err := handler.SetDefault(defaultConf)

		// Then no error should be returned
		if err != nil {
			t.Fatalf("SetDefault() unexpected error: %v", err)
		}

		// And the default context should be set correctly
		if handler.(*configHandler).defaultContextConfig.Environment == nil || handler.(*configHandler).defaultContextConfig.Environment["DEFAULT_VAR"] != "default_val" {
			t.Errorf("Expected defaultContextConfig environment to be %v, got %v", defaultConf.Environment, handler.(*configHandler).defaultContextConfig.Environment)
		}

		// And the existing context should not be modified
		if handler.(*configHandler).config.Contexts["existing-context"] == nil {
			t.Errorf("SetDefault incorrectly overwrote existing context config")
		}
	})

	t.Run("SetDefaultMergesWithExistingContext", func(t *testing.T) {
		// Given a handler with an existing context containing some values
		handler, _ := setup(t)
		handler.(*configHandler).context = "test"
		handler.(*configHandler).config.Contexts = map[string]*v1alpha1.Context{
			"test": {
				ID: ptrString("existing-id"),
				VM: &vm.VMConfig{
					Driver: ptrString("docker-desktop"),
				},
				Environment: map[string]string{
					"EXISTING_VAR": "existing_value",
					"OVERRIDE_VAR": "context_value",
				},
			},
		}

		// And a default context with overlapping and additional values
		defaultContext := v1alpha1.Context{
			VM: &vm.VMConfig{
				CPU: ptrInt(4),
			},
			Environment: map[string]string{
				"DEFAULT_VAR":  "default_value",
				"OVERRIDE_VAR": "default_value",
			},
		}

		// When setting the default context
		err := handler.SetDefault(defaultContext)

		// Then no error should be returned
		if err != nil {
			t.Fatalf("SetDefault() unexpected error: %v", err)
		}

		// And the context should merge defaults with existing values
		ctx := handler.(*configHandler).config.Contexts["test"]
		if ctx == nil {
			t.Fatal("Context was removed during SetDefault")
		}

		// Existing values should be preserved
		if ctx.ID == nil || *ctx.ID != "existing-id" {
			t.Errorf("Expected ID to be preserved as 'existing-id', got %v", ctx.ID)
		}
		if ctx.VM.Driver == nil || *ctx.VM.Driver != "docker-desktop" {
			t.Errorf("Expected VM driver to be preserved as 'docker-desktop', got %v", ctx.VM.Driver)
		}
		if ctx.Environment["EXISTING_VAR"] != "existing_value" {
			t.Errorf("Expected EXISTING_VAR to be preserved as 'existing_value', got '%s'", ctx.Environment["EXISTING_VAR"])
		}
		if ctx.Environment["OVERRIDE_VAR"] != "context_value" {
			t.Errorf("Expected OVERRIDE_VAR to keep context value 'context_value', got '%s'", ctx.Environment["OVERRIDE_VAR"])
		}

		// Default values should be added where not present
		if ctx.VM.CPU == nil || *ctx.VM.CPU != 4 {
			t.Errorf("Expected VM CPU to be added from default as 4, got %v", ctx.VM.CPU)
		}
		if ctx.Environment["DEFAULT_VAR"] != "default_value" {
			t.Errorf("Expected DEFAULT_VAR to be added from default as 'default_value', got '%s'", ctx.Environment["DEFAULT_VAR"])
		}
	})

	t.Run("SetDefaultMergesComplexNestedStructures", func(t *testing.T) {
		// Given a handler with an existing context containing some values
		handler, _ := setup(t)
		handler.(*configHandler).context = "test"
		handler.(*configHandler).config.Contexts = map[string]*v1alpha1.Context{
			"test": {
				ID: ptrString("existing-id"),
				Environment: map[string]string{
					"EXISTING_VAR": "existing_value",
				},
			},
		}

		// And a default context with additional values
		defaultContext := v1alpha1.Context{
			Environment: map[string]string{
				"DEFAULT_VAR": "default_value",
			},
		}

		// When setting the default context
		err := handler.SetDefault(defaultContext)

		// Then no error should be returned
		if err != nil {
			t.Fatalf("SetDefault() unexpected error: %v", err)
		}

		// And the context should have both existing and default values
		ctx := handler.(*configHandler).config.Contexts["test"]
		if ctx == nil {
			t.Fatal("Context was removed during SetDefault")
		}

		// Existing values should be preserved
		if ctx.ID == nil || *ctx.ID != "existing-id" {
			t.Errorf("Expected ID to be preserved as 'existing-id', got %v", ctx.ID)
		}
		if ctx.Environment["EXISTING_VAR"] != "existing_value" {
			t.Errorf("Expected EXISTING_VAR to be preserved as 'existing_value', got '%s'", ctx.Environment["EXISTING_VAR"])
		}

		// Default values should be added where not present
		if ctx.Environment["DEFAULT_VAR"] != "default_value" {
			t.Errorf("Expected DEFAULT_VAR to be added from default as 'default_value', got '%s'", ctx.Environment["DEFAULT_VAR"])
		}
	})

	t.Run("SetDefaultWithNilContextsMap", func(t *testing.T) {
		// Given a handler with a nil contexts map
		handler, _ := setup(t)
		handler.(*configHandler).context = "test"
		handler.(*configHandler).config.Contexts = nil

		// And a default context
		defaultContext := v1alpha1.Context{
			Environment: map[string]string{
				"DEFAULT_VAR": "default_value",
			},
		}

		// When setting the default context
		err := handler.SetDefault(defaultContext)

		// Then no error should be returned
		if err != nil {
			t.Fatalf("SetDefault() unexpected error: %v", err)
		}

		// And the contexts map should be created with the default
		if handler.(*configHandler).config.Contexts == nil {
			t.Fatal("Expected contexts map to be created")
		}

		ctx := handler.(*configHandler).config.Contexts["test"]
		if ctx == nil {
			t.Fatal("Expected test context to be created")
		}
		if ctx.Environment["DEFAULT_VAR"] != "default_value" {
			t.Errorf("Expected DEFAULT_VAR to be 'default_value', got '%s'", ctx.Environment["DEFAULT_VAR"])
		}
	})
}

func TestConfigHandler_SetContextValue(t *testing.T) {
	setup := func(t *testing.T) (ConfigHandler, *Mocks) {
		mocks := setupMocks(t)
		handler := NewConfigHandler(mocks.Injector)
		if err := handler.Initialize(); err != nil {
			t.Fatalf("Failed to initialize handler: %v", err)
		}
		handler.(*configHandler).shims = mocks.Shims
		handler.(*configHandler).path = filepath.Join(t.TempDir(), "config.yaml")
		return handler, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a handler with a context set
		handler, _ := setup(t)
		handler.(*configHandler).context = "test"

		// And a context with an empty environment map
		actualContext := handler.GetContext()
		handler.(*configHandler).config.Contexts = map[string]*v1alpha1.Context{
			actualContext: {},
		}

		// When setting a value in the context environment
		err := handler.SetContextValue("environment.TEST_VAR", "test_value")

		// Then no error should be returned
		if err != nil {
			t.Fatalf("SetContextValue() unexpected error: %v", err)
		}

		// And the value should be correctly set in the context
		expected := "test_value"
		if val := handler.(*configHandler).config.Contexts[actualContext].Environment["TEST_VAR"]; val != expected {
			t.Errorf("SetContextValue() did not correctly set value, expected %s, got %s", expected, val)
		}
	})

	t.Run("EmptyPath", func(t *testing.T) {
		// Given a handler with a context set
		handler, _ := setup(t)

		// When attempting to set a value with an empty path
		err := handler.SetContextValue("", "test_value")

		// Then an error should be returned
		if err == nil {
			t.Errorf("SetContextValue() with empty path did not return an error")
		}

		// And the error message should be as expected
		expectedErr := "path cannot be empty"
		if err.Error() != expectedErr {
			t.Errorf("Expected error message '%s', got '%s'", expectedErr, err.Error())
		}
	})

	t.Run("SetFails", func(t *testing.T) {
		// Given a handler with a context set
		handler, _ := setup(t)
		handler.(*configHandler).context = "test"

		// When attempting to set a value with an invalid path
		err := handler.SetContextValue("invalid..path", "test_value")

		// Then an error should be returned
		if err == nil {
			t.Errorf("SetContextValue() with invalid path did not return an error")
		}
	})

	t.Run("ConvertStringToBool", func(t *testing.T) {
		handler, _ := setup(t)
		handler.(*configHandler).context = "default"
		handler.(*configHandler).config.Contexts = map[string]*v1alpha1.Context{
			"default": {},
		}

		// Set initial bool value
		if err := handler.SetContextValue("environment.BOOL_VAR", "true"); err != nil {
			t.Fatalf("Failed to set initial bool value: %v", err)
		}

		// Override with string "false"
		if err := handler.SetContextValue("environment.BOOL_VAR", "false"); err != nil {
			t.Fatalf("Failed to set string bool value: %v", err)
		}

		val := handler.GetString("environment.BOOL_VAR")
		if val != "false" {
			t.Errorf("Expected false, got %v", val)
		}
	})

	t.Run("ConvertStringToInt", func(t *testing.T) {
		handler, _ := setup(t)
		handler.(*configHandler).context = "default"
		handler.(*configHandler).config.Contexts = map[string]*v1alpha1.Context{
			"default": {},
		}

		// Set initial int value
		if err := handler.SetContextValue("environment.INT_VAR", "42"); err != nil {
			t.Fatalf("Failed to set initial int value: %v", err)
		}

		// Override with string "100"
		if err := handler.SetContextValue("environment.INT_VAR", "100"); err != nil {
			t.Fatalf("Failed to set string int value: %v", err)
		}

		val := handler.GetString("environment.INT_VAR")
		if val != "100" {
			t.Errorf("Expected 100, got %v", val)
		}
	})

	t.Run("ConvertStringToFloat", func(t *testing.T) {
		handler, _ := setup(t)
		handler.(*configHandler).context = "default"
		handler.(*configHandler).config.Contexts = map[string]*v1alpha1.Context{
			"default": {},
		}

		// Set initial float value
		if err := handler.SetContextValue("environment.FLOAT_VAR", "3.14"); err != nil {
			t.Fatalf("Failed to set initial float value: %v", err)
		}

		// Override with string "6.28"
		if err := handler.SetContextValue("environment.FLOAT_VAR", "6.28"); err != nil {
			t.Fatalf("Failed to set string float value: %v", err)
		}

		val := handler.GetString("environment.FLOAT_VAR")
		if val != "6.28" {
			t.Errorf("Expected 6.28, got %v", val)
		}
	})

	t.Run("ConvertStringToBoolPointer", func(t *testing.T) {
		// Given a handler with a default context
		handler, _ := setup(t)
		handler.(*configHandler).context = "default"
		handler.(*configHandler).config.Contexts = map[string]*v1alpha1.Context{
			"default": {},
		}

		// When setting a string "false" to a bool pointer field (dns.enabled)
		if err := handler.SetContextValue("dns.enabled", "false"); err != nil {
			t.Fatalf("Failed to set dns.enabled=false from string: %v", err)
		}

		// Then the value should be correctly set as a boolean
		config := handler.GetConfig()
		if config.DNS == nil || config.DNS.Enabled == nil || *config.DNS.Enabled != false {
			t.Errorf("Expected dns.enabled to be false, got %v", config.DNS.Enabled)
		}

		// And when setting "true" as well
		if err := handler.SetContextValue("dns.enabled", "true"); err != nil {
			t.Fatalf("Failed to set dns.enabled=true from string: %v", err)
		}

		config = handler.GetConfig()
		if config.DNS == nil || config.DNS.Enabled == nil || *config.DNS.Enabled != true {
			t.Errorf("Expected dns.enabled to be true, got %v", config.DNS.Enabled)
		}
	})

	t.Run("SchemaRoutingAndInitialization", func(t *testing.T) {
		handler, _ := setup(t)
		handler.(*configHandler).context = "test"

		// Test invalid path formats
		err := handler.SetContextValue("..invalid", "value")
		if err == nil {
			t.Error("Expected error for invalid path")
		}

		// Test static schema routing (goes to context config)
		err = handler.SetContextValue("environment.STATIC_VAR", "static_value")
		if err != nil {
			t.Fatalf("Failed to set static schema value: %v", err)
		}
		if handler.(*configHandler).config.Contexts["test"].Environment["STATIC_VAR"] != "static_value" {
			t.Error("Static value should be in context config")
		}

		// Test dynamic schema routing (goes to contextValues)
		err = handler.SetContextValue("dynamic_key", "dynamic_value")
		if err != nil {
			t.Fatalf("Failed to set dynamic schema value: %v", err)
		}
		if handler.(*configHandler).contextValues["dynamic_key"] != "dynamic_value" {
			t.Error("Dynamic value should be in contextValues")
		}

		// Test initialization when not loaded
		handler.(*configHandler).loaded = false
		handler.(*configHandler).contextValues = nil
		err = handler.SetContextValue("not_loaded_key", "not_loaded_value")
		if err != nil {
			t.Fatalf("Failed to set value when not loaded: %v", err)
		}
		if handler.(*configHandler).contextValues["not_loaded_key"] != "not_loaded_value" {
			t.Error("contextValues should be initialized even when not loaded")
		}
	})

	t.Run("SchemaAwareTypeConversion", func(t *testing.T) {
		handler, _ := setup(t)
		handler.(*configHandler).context = "test"
		handler.(*configHandler).loaded = true

		// Set up schema validator with type definitions
		handler.(*configHandler).schemaValidator = &SchemaValidator{
			Schema: map[string]any{
				"properties": map[string]any{
					"dev": map[string]any{
						"type": "boolean",
					},
					"port": map[string]any{
						"type": "integer",
					},
					"ratio": map[string]any{
						"type": "number",
					},
					"name": map[string]any{
						"type": "string",
					},
				},
			},
		}

		// Test boolean conversion
		err := handler.SetContextValue("dev", "true")
		if err != nil {
			t.Fatalf("Failed to set boolean value: %v", err)
		}
		if handler.(*configHandler).contextValues["dev"] != true {
			t.Errorf("Expected boolean true, got %v (%T)", handler.(*configHandler).contextValues["dev"], handler.(*configHandler).contextValues["dev"])
		}

		// Test integer conversion
		err = handler.SetContextValue("port", "8080")
		if err != nil {
			t.Fatalf("Failed to set integer value: %v", err)
		}
		if handler.(*configHandler).contextValues["port"] != 8080 {
			t.Errorf("Expected integer 8080, got %v (%T)", handler.(*configHandler).contextValues["port"], handler.(*configHandler).contextValues["port"])
		}

		// Test number conversion
		err = handler.SetContextValue("ratio", "3.14")
		if err != nil {
			t.Fatalf("Failed to set number value: %v", err)
		}
		if handler.(*configHandler).contextValues["ratio"] != 3.14 {
			t.Errorf("Expected number 3.14, got %v (%T)", handler.(*configHandler).contextValues["ratio"], handler.(*configHandler).contextValues["ratio"])
		}

		// Test string conversion (should remain string)
		err = handler.SetContextValue("name", "test")
		if err != nil {
			t.Fatalf("Failed to set string value: %v", err)
		}
		if handler.(*configHandler).contextValues["name"] != "test" {
			t.Errorf("Expected string 'test', got %v (%T)", handler.(*configHandler).contextValues["name"], handler.(*configHandler).contextValues["name"])
		}
	})

	t.Run("FallbackPatternConversion", func(t *testing.T) {
		handler, _ := setup(t)
		handler.(*configHandler).context = "test"
		handler.(*configHandler).loaded = true

		// No schema validator - should use pattern matching

		// Test boolean pattern matching
		err := handler.SetContextValue("enabled", "true")
		if err != nil {
			t.Fatalf("Failed to set boolean value: %v", err)
		}
		if handler.(*configHandler).contextValues["enabled"] != true {
			t.Errorf("Expected boolean true, got %v (%T)", handler.(*configHandler).contextValues["enabled"], handler.(*configHandler).contextValues["enabled"])
		}

		// Test integer pattern matching
		err = handler.SetContextValue("count", "42")
		if err != nil {
			t.Fatalf("Failed to set integer value: %v", err)
		}
		if handler.(*configHandler).contextValues["count"] != 42 {
			t.Errorf("Expected integer 42, got %v (%T)", handler.(*configHandler).contextValues["count"], handler.(*configHandler).contextValues["count"])
		}

		// Test float pattern matching
		err = handler.SetContextValue("rate", "2.5")
		if err != nil {
			t.Fatalf("Failed to set float value: %v", err)
		}
		if handler.(*configHandler).contextValues["rate"] != 2.5 {
			t.Errorf("Expected float 2.5, got %v (%T)", handler.(*configHandler).contextValues["rate"], handler.(*configHandler).contextValues["rate"])
		}
	})

	t.Run("SchemaConversionFailure", func(t *testing.T) {
		handler, _ := setup(t)
		handler.(*configHandler).context = "test"
		handler.(*configHandler).loaded = true

		// Set up schema validator with boolean type and validation support
		mockShell := handler.(*configHandler).shell
		mockValidator := NewSchemaValidator(mockShell)
		mockValidator.Schema = map[string]any{
			"properties": map[string]any{
				"dev": map[string]any{
					"type": "boolean",
				},
			},
		}
		handler.(*configHandler).schemaValidator = mockValidator

		// Test invalid boolean value - should now fail validation
		err := handler.SetContextValue("dev", "invalid")
		if err == nil {
			t.Fatal("Expected validation error for invalid boolean value, got nil")
		}
		if !strings.Contains(err.Error(), "validation failed") && !strings.Contains(err.Error(), "type mismatch") {
			t.Errorf("Expected validation error, got: %v", err)
		}
	})
}

func TestConfigHandler_convertStringValue(t *testing.T) {
	setup := func(t *testing.T) ConfigHandler {
		mocks := setupMocks(t)
		handler := NewConfigHandler(mocks.Injector)
		if err := handler.Initialize(); err != nil {
			t.Fatalf("Failed to initialize handler: %v", err)
		}
		handler.(*configHandler).shims = mocks.Shims
		return handler
	}

	t.Run("NonStringValue", func(t *testing.T) {
		handler := setup(t)

		// Non-string values should be returned as-is
		result := handler.(*configHandler).convertStringValue(42)
		if result != 42 {
			t.Errorf("Expected 42, got %v", result)
		}

		result = handler.(*configHandler).convertStringValue(true)
		if result != true {
			t.Errorf("Expected true, got %v", result)
		}
	})

	t.Run("SchemaAwareConversion", func(t *testing.T) {
		handler := setup(t)

		// Set up schema validator
		handler.(*configHandler).schemaValidator = &SchemaValidator{
			Schema: map[string]any{
				"properties": map[string]any{
					"enabled": map[string]any{
						"type": "boolean",
					},
					"count": map[string]any{
						"type": "integer",
					},
					"rate": map[string]any{
						"type": "number",
					},
				},
			},
		}

		// Test boolean conversion
		result := handler.(*configHandler).convertStringValue("true")
		if result != true {
			t.Errorf("Expected boolean true, got %v (%T)", result, result)
		}

		// Test integer conversion
		result = handler.(*configHandler).convertStringValue("42")
		if result != 42 {
			t.Errorf("Expected integer 42, got %v (%T)", result, result)
		}

		// Test number conversion
		result = handler.(*configHandler).convertStringValue("3.14")
		if result != 3.14 {
			t.Errorf("Expected number 3.14, got %v (%T)", result, result)
		}
	})

	t.Run("PatternMatchingFallback", func(t *testing.T) {
		handler := setup(t)
		// No schema validator - should use pattern matching

		// Test boolean pattern
		result := handler.(*configHandler).convertStringValue("true")
		if result != true {
			t.Errorf("Expected boolean true, got %v (%T)", result, result)
		}

		result = handler.(*configHandler).convertStringValue("false")
		if result != false {
			t.Errorf("Expected boolean false, got %v (%T)", result, result)
		}

		// Test integer pattern
		result = handler.(*configHandler).convertStringValue("123")
		if result != 123 {
			t.Errorf("Expected integer 123, got %v (%T)", result, result)
		}

		// Test float pattern
		result = handler.(*configHandler).convertStringValue("45.67")
		if result != 45.67 {
			t.Errorf("Expected float 45.67, got %v (%T)", result, result)
		}

		// Test string (no conversion)
		result = handler.(*configHandler).convertStringValue("hello")
		if result != "hello" {
			t.Errorf("Expected string 'hello', got %v (%T)", result, result)
		}
	})
}

func TestConfigHandler_getExpectedTypeFromSchema(t *testing.T) {
	setup := func(t *testing.T) ConfigHandler {
		mocks := setupMocks(t)
		handler := NewConfigHandler(mocks.Injector)
		if err := handler.Initialize(); err != nil {
			t.Fatalf("Failed to initialize handler: %v", err)
		}
		handler.(*configHandler).shims = mocks.Shims
		return handler
	}

	t.Run("ValidSchema", func(t *testing.T) {
		handler := setup(t)

		handler.(*configHandler).schemaValidator = &SchemaValidator{
			Schema: map[string]any{
				"properties": map[string]any{
					"enabled": map[string]any{
						"type": "boolean",
					},
					"count": map[string]any{
						"type": "integer",
					},
				},
			},
		}

		// Test existing property
		result := handler.(*configHandler).getExpectedTypeFromSchema("enabled")
		if result != "boolean" {
			t.Errorf("Expected 'boolean', got '%s'", result)
		}

		result = handler.(*configHandler).getExpectedTypeFromSchema("count")
		if result != "integer" {
			t.Errorf("Expected 'integer', got '%s'", result)
		}

		// Test non-existing property
		result = handler.(*configHandler).getExpectedTypeFromSchema("nonexistent")
		if result != "" {
			t.Errorf("Expected empty string, got '%s'", result)
		}
	})

	t.Run("NoSchemaValidator", func(t *testing.T) {
		handler := setup(t)
		// No schema validator

		result := handler.(*configHandler).getExpectedTypeFromSchema("anykey")
		if result != "" {
			t.Errorf("Expected empty string, got '%s'", result)
		}
	})

	t.Run("InvalidSchema", func(t *testing.T) {
		handler := setup(t)

		handler.(*configHandler).schemaValidator = &SchemaValidator{
			Schema: map[string]any{
				"properties": "invalid", // Should be map[string]any
			},
		}

		result := handler.(*configHandler).getExpectedTypeFromSchema("anykey")
		if result != "" {
			t.Errorf("Expected empty string, got '%s'", result)
		}
	})
}

func TestConfigHandler_convertStringToType(t *testing.T) {
	setup := func(t *testing.T) ConfigHandler {
		mocks := setupMocks(t)
		handler := NewConfigHandler(mocks.Injector)
		if err := handler.Initialize(); err != nil {
			t.Fatalf("Failed to initialize handler: %v", err)
		}
		handler.(*configHandler).shims = mocks.Shims
		return handler
	}

	handler := setup(t)

	t.Run("BooleanConversion", func(t *testing.T) {
		result := handler.(*configHandler).convertStringToType("true", "boolean")
		if result != true {
			t.Errorf("Expected true, got %v", result)
		}

		result = handler.(*configHandler).convertStringToType("false", "boolean")
		if result != false {
			t.Errorf("Expected false, got %v", result)
		}

		result = handler.(*configHandler).convertStringToType("invalid", "boolean")
		if result != nil {
			t.Errorf("Expected nil, got %v", result)
		}
	})

	t.Run("IntegerConversion", func(t *testing.T) {
		result := handler.(*configHandler).convertStringToType("42", "integer")
		if result != 42 {
			t.Errorf("Expected 42, got %v", result)
		}

		result = handler.(*configHandler).convertStringToType("invalid", "integer")
		if result != nil {
			t.Errorf("Expected nil, got %v", result)
		}
	})

	t.Run("NumberConversion", func(t *testing.T) {
		result := handler.(*configHandler).convertStringToType("3.14", "number")
		if result != 3.14 {
			t.Errorf("Expected 3.14, got %v", result)
		}

		result = handler.(*configHandler).convertStringToType("invalid", "number")
		if result != nil {
			t.Errorf("Expected nil, got %v", result)
		}
	})

	t.Run("StringConversion", func(t *testing.T) {
		result := handler.(*configHandler).convertStringToType("hello", "string")
		if result != "hello" {
			t.Errorf("Expected 'hello', got %v", result)
		}
	})
}

func TestConfigHandler_LoadConfigString(t *testing.T) {
	setup := func(t *testing.T) (ConfigHandler, *Mocks) {
		mocks := setupMocks(t)
		handler := NewConfigHandler(mocks.Injector)
		handler.(*configHandler).shims = mocks.Shims
		if err := handler.Initialize(); err != nil {
			t.Fatalf("Failed to initialize handler: %v", err)
		}
		return handler, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a handler with a context set
		handler, _ := setup(t)
		handler.SetContext("test")

		// And a valid YAML configuration string
		yamlContent := `
version: v1alpha1
contexts:
  test:
    environment:
      TEST_VAR: test_value`

		// When loading the configuration string
		err := handler.LoadConfigString(yamlContent)

		// Then no error should be returned
		if err != nil {
			t.Fatalf("LoadConfigString() unexpected error: %v", err)
		}

		// And the value should be correctly loaded
		value := handler.GetString("environment.TEST_VAR")
		if value != "test_value" {
			t.Errorf("Expected TEST_VAR = 'test_value', got '%s'", value)
		}
	})

	t.Run("EmptyContent", func(t *testing.T) {
		// Given a handler with a context set
		handler, _ := setup(t)

		// When loading an empty configuration string
		err := handler.LoadConfigString("")

		// Then no error should be returned
		if err != nil {
			t.Fatalf("LoadConfigString() unexpected error: %v", err)
		}
	})

	t.Run("InvalidYAML", func(t *testing.T) {
		// Given a handler with a context set
		handler, _ := setup(t)

		// And an invalid YAML string
		yamlContent := `invalid: yaml: content: [}`

		// When loading the invalid YAML
		err := handler.LoadConfigString(yamlContent)

		// Then an error should be returned
		if err == nil {
			t.Fatal("LoadConfigString() expected error for invalid YAML")
		}

		// And the error message should indicate YAML unmarshalling failure
		if !strings.Contains(err.Error(), "error unmarshalling yaml") {
			t.Errorf("Expected error about invalid YAML, got: %v", err)
		}
	})

	t.Run("UnsupportedVersion", func(t *testing.T) {
		// Given a handler with a context set
		handler, _ := setup(t)

		// And a YAML string with an unsupported version
		yamlContent := `
version: v2alpha1
contexts:
  test: {}`

		// When loading the YAML with unsupported version
		err := handler.LoadConfigString(yamlContent)

		// Then an error should be returned
		if err == nil {
			t.Fatal("LoadConfigString() expected error for unsupported version")
		}

		// And the error message should indicate unsupported version
		if !strings.Contains(err.Error(), "unsupported config version") {
			t.Errorf("Expected error about unsupported version, got: %v", err)
		}
	})
}

func Test_makeAddressable(t *testing.T) {
	t.Run("AlreadyAddressable", func(t *testing.T) {
		// Given an addressable value
		var x int = 42
		v := reflect.ValueOf(&x).Elem()

		// When making it addressable
		result := makeAddressable(v)

		// Then the same value should be returned
		if result.Interface() != v.Interface() {
			t.Errorf("makeAddressable() = %v, want %v", result.Interface(), v.Interface())
		}
	})

	t.Run("NonAddressable", func(t *testing.T) {
		// Given a non-addressable value
		v := reflect.ValueOf(42)

		// When making it addressable
		result := makeAddressable(v)

		// Then a new addressable value should be returned
		if !result.CanAddr() {
			t.Error("makeAddressable() returned non-addressable value")
		}
		if result.Interface() != v.Interface() {
			t.Errorf("makeAddressable() = %v, want %v", result.Interface(), v.Interface())
		}
	})

	t.Run("NilValue", func(t *testing.T) {
		// Given a nil value
		var v reflect.Value

		// When making it addressable
		result := makeAddressable(v)

		// Then a zero value should be returned
		if result.IsValid() {
			t.Error("makeAddressable() returned valid value for nil input")
		}
	})
}

func TestConfigHandler_ConvertValue(t *testing.T) {
	t.Run("StringToString", func(t *testing.T) {
		// Given a string value and target type
		value := "test"
		targetType := reflect.TypeOf("")

		// When converting the value
		result, err := convertValue(value, targetType)

		// Then no error should be returned
		if err != nil {
			t.Fatalf("convertValue() unexpected error: %v", err)
		}

		// And the value should be correctly converted
		if result != "test" {
			t.Errorf("convertValue() = %v, want %v", result, "test")
		}
	})

	t.Run("StringToInt", func(t *testing.T) {
		// Given a string value and target type
		value := "42"
		targetType := reflect.TypeOf(0)

		// When converting the value
		result, err := convertValue(value, targetType)

		// Then no error should be returned
		if err != nil {
			t.Fatalf("convertValue() unexpected error: %v", err)
		}

		// And the value should be correctly converted
		if result != 42 {
			t.Errorf("convertValue() = %v, want %v", result, 42)
		}
	})

	t.Run("StringToIntOverflow", func(t *testing.T) {
		// Given a string value that would overflow int8
		value := "128"
		targetType := reflect.TypeOf(int8(0))

		// When converting the value
		_, err := convertValue(value, targetType)

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error for integer overflow")
		}

		// And the error message should indicate overflow
		expectedErr := "integer overflow: 128 is outside the range of int8"
		if err.Error() != expectedErr {
			t.Errorf("Expected error '%s', got '%s'", expectedErr, err.Error())
		}
	})

	t.Run("StringToUintOverflow", func(t *testing.T) {
		// Given a string value that would overflow uint8
		value := "256"
		targetType := reflect.TypeOf(uint8(0))

		// When converting the value
		_, err := convertValue(value, targetType)
		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error for integer overflow")
		}

		// And the error message should indicate overflow
		expectedErr := "integer overflow: 256 is outside the range of uint8"
		if err.Error() != expectedErr {
			t.Errorf("Expected error '%s', got '%s'", expectedErr, err.Error())
		}
	})

	t.Run("StringToFloatOverflow", func(t *testing.T) {
		// Given a string value that would overflow float32
		value := "3.4028236e+38"
		targetType := reflect.TypeOf(float32(0))

		// When converting the value
		_, err := convertValue(value, targetType)

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error for float overflow")
		}

		// And the error message should indicate overflow
		if !strings.Contains(err.Error(), "float overflow") {
			t.Errorf("Expected error containing 'float overflow', got '%s'", err.Error())
		}
	})

	t.Run("StringToFloat", func(t *testing.T) {
		// Given a string value and target type
		value := "3.14"
		targetType := reflect.TypeOf(float64(0))

		// When converting the value
		result, err := convertValue(value, targetType)

		// Then no error should be returned
		if err != nil {
			t.Fatalf("convertValue() unexpected error: %v", err)
		}

		// And the value should be correctly converted
		if result != 3.14 {
			t.Errorf("convertValue() = %v, want %v", result, 3.14)
		}
	})

	t.Run("StringToBool", func(t *testing.T) {
		// Given a string value and target type
		value := "true"
		targetType := reflect.TypeOf(true)

		// When converting the value
		result, err := convertValue(value, targetType)

		// Then no error should be returned
		if err != nil {
			t.Fatalf("convertValue() unexpected error: %v", err)
		}

		// And the value should be correctly converted
		if result != true {
			t.Errorf("convertValue() = %v, want %v", result, true)
		}
	})
}

func TestConfigHandler_Set(t *testing.T) {
	setup := func(t *testing.T) (ConfigHandler, *Mocks) {
		mocks := setupMocks(t)
		handler := NewConfigHandler(mocks.Injector)
		handler.(*configHandler).shims = mocks.Shims
		if err := handler.Initialize(); err != nil {
			t.Fatalf("Failed to initialize handler: %v", err)
		}
		return handler, mocks
	}

	t.Run("InvalidPath", func(t *testing.T) {
		// Given a handler with a context set
		handler, _ := setup(t)

		// When setting a value with an invalid path
		err := handler.Set("", "value")

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Set() unexpected error: %v", err)
		}
	})

	t.Run("SetValueByPathError", func(t *testing.T) {
		// Given a handler with a context set
		handler, _ := setup(t)

		// And a mocked setValueByPath that returns an error
		handler.(*configHandler).shims.YamlMarshal = func(v any) ([]byte, error) {
			return nil, fmt.Errorf("mocked error")
		}

		// When setting a value
		err := handler.Set("test.path", "value")

		// Then an error should be returned
		if err == nil {
			t.Fatal("Set() expected error, got nil")
		}
	})
}

func Test_setValueByPath(t *testing.T) {
	t.Run("EmptyPathKeys", func(t *testing.T) {
		// Given empty pathKeys
		currValue := reflect.ValueOf(struct{}{})
		pathKeys := []string{}
		value := "test"
		fullPath := "test.path"

		// When calling setValueByPath with empty pathKeys
		err := setValueByPath(currValue, pathKeys, value, fullPath)

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error for empty pathKeys")
		}
		expectedErr := "pathKeys cannot be empty"
		if err.Error() != expectedErr {
			t.Errorf("Expected error '%s', got '%s'", expectedErr, err.Error())
		}
	})

	t.Run("StructFieldNotFound", func(t *testing.T) {
		// Given a struct and a non-existent field
		type TestStruct struct {
			Field string `yaml:"field"`
		}
		currValue := reflect.ValueOf(&TestStruct{}).Elem()
		pathKeys := []string{"nonexistent"}
		value := "test"
		fullPath := "nonexistent"

		// When calling setValueByPath with non-existent field
		err := setValueByPath(currValue, pathKeys, value, fullPath)

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error for non-existent field")
		}
		expectedErr := "field not found: nonexistent"
		if err.Error() != expectedErr {
			t.Errorf("Expected error '%s', got '%s'", expectedErr, err.Error())
		}
	})

	t.Run("StructFieldSuccess", func(t *testing.T) {
		// Given a struct with a field
		type TestStruct struct {
			Field string `yaml:"field"`
		}
		currValue := reflect.ValueOf(&TestStruct{}).Elem()
		pathKeys := []string{"field"}
		value := "test"
		fullPath := "field"

		// When calling setValueByPath with valid field
		err := setValueByPath(currValue, pathKeys, value, fullPath)

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		// And the field should be set correctly
		if currValue.Field(0).String() != "test" {
			t.Errorf("Expected field value 'test', got '%s'", currValue.Field(0).String())
		}
	})

	t.Run("MapKeyTypeMismatch", func(t *testing.T) {
		// Given a map with int keys but trying to set with string key
		currValue := reflect.ValueOf(&map[int]string{}).Elem()
		pathKeys := []string{"key"}
		value := "test"
		fullPath := "key"

		// When calling setValueByPath with mismatched key type
		err := setValueByPath(currValue, pathKeys, value, fullPath)

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error for key type mismatch")
		}
		expectedErr := "key type mismatch: expected int, got string"
		if err.Error() != expectedErr {
			t.Errorf("Expected error '%s', got '%s'", expectedErr, err.Error())
		}
	})

	t.Run("MapValueTypeMismatch", func(t *testing.T) {
		// Given a map with string values but trying to set with a non-convertible type
		currValue := reflect.ValueOf(&map[string]string{}).Elem()
		pathKeys := []string{"key"}
		value := struct{}{} // Use a struct{} which cannot be converted to string
		fullPath := "key"

		// When calling setValueByPath with mismatched value type
		err := setValueByPath(currValue, pathKeys, value, fullPath)

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error for value type mismatch")
		}
		expectedErr := "value type mismatch for key key: expected string, got struct {}"
		if err.Error() != expectedErr {
			t.Errorf("Expected error '%s', got '%s'", expectedErr, err.Error())
		}
	})

	t.Run("MapSuccess", func(t *testing.T) {
		// Given a map with string keys and values
		currValue := reflect.ValueOf(&map[string]string{}).Elem()
		pathKeys := []string{"key"}
		value := "test"
		fullPath := "key"

		// When calling setValueByPath with valid key and value
		err := setValueByPath(currValue, pathKeys, value, fullPath)

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		// And the value should be set correctly
		if currValue.MapIndex(reflect.ValueOf("key")).String() != "test" {
			t.Errorf("Expected map value 'test', got '%s'", currValue.MapIndex(reflect.ValueOf("key")).String())
		}
	})

	t.Run("InvalidPath", func(t *testing.T) {
		// Given an invalid path type
		currValue := reflect.ValueOf(42)
		pathKeys := []string{"key"}
		value := "test"
		fullPath := "key"

		// When calling setValueByPath with invalid path type
		err := setValueByPath(currValue, pathKeys, value, fullPath)

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error for invalid path")
		}
		expectedErr := "Invalid path: key"
		if err.Error() != expectedErr {
			t.Errorf("Expected error '%s', got '%s'", expectedErr, err.Error())
		}
	})

	t.Run("NestedStruct", func(t *testing.T) {
		// Given a nested struct
		type InnerStruct struct {
			Field string `yaml:"field"`
		}
		type OuterStruct struct {
			Inner InnerStruct `yaml:"inner"`
		}
		currValue := reflect.ValueOf(&OuterStruct{}).Elem()
		pathKeys := []string{"inner", "field"}
		value := "test"
		fullPath := "inner.field"

		// When calling setValueByPath with nested path
		err := setValueByPath(currValue, pathKeys, value, fullPath)

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		// And the nested field should be set correctly
		inner := currValue.Field(0)
		if inner.Field(0).String() != "test" {
			t.Errorf("Expected nested field value 'test', got '%s'", inner.Field(0).String())
		}
	})

	t.Run("NestedMap", func(t *testing.T) {
		// Given a nested map
		currValue := reflect.ValueOf(&map[string]map[string]string{}).Elem()
		pathKeys := []string{"outer", "inner"}
		value := "test"
		fullPath := "outer.inner"

		// When calling setValueByPath with nested path
		err := setValueByPath(currValue, pathKeys, value, fullPath)

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		// And the nested value should be set correctly
		outer := currValue.MapIndex(reflect.ValueOf("outer"))
		if !outer.IsValid() {
			t.Fatal("Expected outer map to exist")
		}
		inner := outer.MapIndex(reflect.ValueOf("inner"))
		if !inner.IsValid() {
			t.Fatal("Expected inner map to exist")
		}
		if inner.String() != "test" {
			t.Errorf("Expected nested value 'test', got '%s'", inner.String())
		}
	})

	t.Run("PointerField", func(t *testing.T) {
		// Given a struct with a pointer field
		type TestStruct struct {
			Field *string `yaml:"field"`
		}
		currValue := reflect.ValueOf(&TestStruct{}).Elem()
		pathKeys := []string{"field"}
		value := "test"
		fullPath := "field"

		// When calling setValueByPath with pointer field
		err := setValueByPath(currValue, pathKeys, value, fullPath)

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		// And the pointer field should be set correctly
		field := currValue.Field(0)
		if field.IsNil() {
			t.Fatal("Expected pointer field to be non-nil")
		}
		if field.Elem().String() != "test" {
			t.Errorf("Expected pointer field value 'test', got '%s'", field.Elem().String())
		}
	})

	t.Run("PointerMap", func(t *testing.T) {
		// Given a map with pointer values
		currValue := reflect.ValueOf(&map[string]*string{}).Elem()
		pathKeys := []string{"key"}
		str := "test"
		value := &str // Use a pointer to string
		fullPath := "key"

		// When calling setValueByPath with pointer map
		err := setValueByPath(currValue, pathKeys, value, fullPath)

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		// And the pointer value should be set correctly
		val := currValue.MapIndex(reflect.ValueOf("key"))
		if !val.IsValid() || val.IsNil() {
			t.Fatal("Expected map value to be non-nil")
		}
		if val.Elem().String() != "test" {
			t.Errorf("Expected pointer value 'test', got '%s'", val.Elem().String())
		}
	})

	t.Run("NestedMapWithNilValue", func(t *testing.T) {
		// Given a nested map with a nil value
		m := map[string]map[string]string{
			"outer": nil,
		}
		currValue := reflect.ValueOf(&m).Elem()
		pathKeys := []string{"outer", "inner"}
		value := "test"
		fullPath := "outer.inner"

		// When calling setValueByPath with nested path
		err := setValueByPath(currValue, pathKeys, value, fullPath)

		// Then no error should be returned
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		// And the nested value should be set correctly
		outer := currValue.MapIndex(reflect.ValueOf("outer"))
		if !outer.IsValid() {
			t.Fatal("Expected outer map to exist")
		}
		inner := outer.MapIndex(reflect.ValueOf("inner"))
		if !inner.IsValid() {
			t.Fatal("Expected inner map to exist")
		}
		if inner.String() != "test" {
			t.Errorf("Expected nested value 'test', got '%s'", inner.String())
		}
	})
}

func TestConfigHandler_GenerateContextID(t *testing.T) {
	setup := func(t *testing.T) (ConfigHandler, *Mocks) {
		mocks := setupMocks(t)
		handler := NewConfigHandler(mocks.Injector)
		handler.(*configHandler).shims = mocks.Shims
		if err := handler.Initialize(); err != nil {
			t.Fatalf("Failed to initialize handler: %v", err)
		}
		return handler, mocks
	}

	t.Run("WhenContextIDExists", func(t *testing.T) {
		// Given a set of safe mocks and a configHandler
		handler, _ := setup(t)

		// And an existing context ID
		existingID := "w1234567"
		handler.SetContextValue("id", existingID)

		// When GenerateContextID is called
		err := handler.GenerateContextID()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("GenerateContextID() unexpected error: %v", err)
		}

		// And the existing ID should remain unchanged
		if got := handler.GetString("id"); got != existingID {
			t.Errorf("Expected ID = %v, got = %v", existingID, got)
		}
	})

	t.Run("WhenContextIDDoesNotExist", func(t *testing.T) {
		// Given a set of safe mocks and a configHandler
		handler, _ := setup(t)

		// When GenerateContextID is called
		err := handler.GenerateContextID()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("GenerateContextID() unexpected error: %v", err)
		}

		// And a new ID should be generated
		id := handler.GetString("id")
		if id == "" {
			t.Fatal("Expected non-empty ID")
		}

		// And the ID should start with 'w' and be 8 characters long
		if len(id) != 8 || !strings.HasPrefix(id, "w") {
			t.Errorf("Expected ID to start with 'w' and be 8 characters long, got: %s", id)
		}
	})

	t.Run("WhenRandomGenerationFails", func(t *testing.T) {
		// Given a set of safe mocks and a configHandler
		handler, _ := setup(t)

		// And a mocked crypto/rand that fails
		handler.(*configHandler).shims.CryptoRandRead = func([]byte) (int, error) {
			return 0, fmt.Errorf("mocked crypto/rand error")
		}

		// When GenerateContextID is called
		err := handler.GenerateContextID()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}

		// And the error message should be as expected
		expectedError := "failed to generate random context ID: mocked crypto/rand error"
		if err.Error() != expectedError {
			t.Errorf("Expected error = %v, got = %v", expectedError, err)
		}
	})
}

// Test specifically for the flag override issue we're experiencing
func TestConfigHandler_FlagOverrideIssue(t *testing.T) {
	setup := func(t *testing.T) (ConfigHandler, *Mocks) {
		mocks := setupMocks(t)
		handler := NewConfigHandler(mocks.Injector)
		if err := handler.Initialize(); err != nil {
			t.Fatalf("Failed to initialize handler: %v", err)
		}
		handler.(*configHandler).shims = mocks.Shims
		return handler, mocks
	}

	t.Run("FlagValuePreservedAfterSetDefault", func(t *testing.T) {
		// Given a handler that simulates loading existing config (like init pipeline does)
		handler, _ := setup(t)
		handler.(*configHandler).context = "local"

		// Simulate existing config with different VM driver
		existingConfig := `version: v1alpha1
contexts:
  local:
    id: existing-id
    vm:
      driver: existing-driver`

		err := handler.LoadConfigString(existingConfig)
		if err != nil {
			t.Fatalf("Failed to load config: %v", err)
		}

		// Simulate flag being set (like cmd/init.go does)
		err = handler.SetContextValue("vm.driver", "colima")
		if err != nil {
			t.Fatalf("Failed to set flag value: %v", err)
		}

		// Verify flag value is set correctly before SetDefault
		vmDriver := handler.GetString("vm.driver")
		if vmDriver != "colima" {
			t.Errorf("Expected vm.driver to be 'colima' before SetDefault, got '%s'", vmDriver)
		}

		// Simulate SetDefault being called (like init pipeline does)
		// Use a default config that has no VM section (like DefaultConfig_Full)
		defaultConfig := v1alpha1.Context{
			Environment: map[string]string{
				"DEFAULT_VAR": "default_value",
			},
			Provider: ptrString("local"),
		}

		err = handler.SetDefault(defaultConfig)
		if err != nil {
			t.Fatalf("Failed to set default: %v", err)
		}

		// Then the flag value should still be preserved after SetDefault
		vmDriverAfter := handler.GetString("vm.driver")
		if vmDriverAfter != "colima" {
			t.Errorf("Expected vm.driver to remain 'colima' after SetDefault, got '%s'", vmDriverAfter)
		}

		// And the default values should be added
		provider := handler.GetString("provider")
		if provider != "local" {
			t.Errorf("Expected provider to be added as 'local', got '%s'", provider)
		}
	})
}

// =============================================================================
// LoadContextConfig Tests
// =============================================================================

func TestConfigHandler_LoadContextConfig(t *testing.T) {
	setup := func(t *testing.T) (ConfigHandler, *Mocks) {
		mocks := setupMocks(t)
		handler := NewConfigHandler(mocks.Injector)
		handler.(*configHandler).shims = mocks.Shims
		if err := handler.Initialize(); err != nil {
			t.Fatalf("Failed to initialize handler: %v", err)
		}
		return handler, mocks
	}

	t.Run("SuccessWithContextConfig", func(t *testing.T) {
		// Given a configHandler with existing config
		handler, mocks := setup(t)

		// Load base configuration
		baseConfig := `version: v1alpha1
contexts:
  production:
    provider: aws
    environment:
      BASE_VAR: base_value`
		if err := handler.LoadConfigString(baseConfig); err != nil {
			t.Fatalf("Failed to load base config: %v", err)
		}

		// Set current context to production
		if err := handler.SetContext("production"); err != nil {
			t.Fatalf("Failed to set context: %v", err)
		}

		// Override the shim to return the correct context
		handler.(*configHandler).shims.Getenv = func(key string) string {
			if key == "WINDSOR_CONTEXT" {
				return "production"
			}
			return ""
		}

		// Create context-specific config file
		projectRoot, _ := mocks.Shell.GetProjectRootFunc()
		contextDir := filepath.Join(projectRoot, "contexts", "production")
		if err := os.MkdirAll(contextDir, 0755); err != nil {
			t.Fatalf("Failed to create context directory: %v", err)
		}

		contextConfigPath := filepath.Join(contextDir, "windsor.yaml")
		contextConfig := `provider: local
environment:
  CONTEXT_VAR: context_value
  BASE_VAR: overridden_value
aws:
  enabled: true`

		if err := os.WriteFile(contextConfigPath, []byte(contextConfig), 0644); err != nil {
			t.Fatalf("Failed to write context config: %v", err)
		}

		// Override shims to allow reading the actual context file
		handler.(*configHandler).shims.Stat = func(name string) (os.FileInfo, error) {
			return os.Stat(name)
		}
		handler.(*configHandler).shims.ReadFile = func(filename string) ([]byte, error) {
			return os.ReadFile(filename)
		}

		// When LoadContextConfig is called
		err := handler.LoadContextConfig()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("LoadContextConfig() unexpected error: %v", err)
		}

		// And the context configuration should be merged
		if handler.GetString("provider") != "local" {
			t.Errorf("Expected provider to be overridden to 'local', got '%s'", handler.GetString("provider"))
		}
		if handler.GetString("environment.CONTEXT_VAR") != "context_value" {
			t.Errorf("Expected CONTEXT_VAR to be 'context_value', got '%s'", handler.GetString("environment.CONTEXT_VAR"))
		}
		if handler.GetString("environment.BASE_VAR") != "overridden_value" {
			t.Errorf("Expected BASE_VAR to be overridden to 'overridden_value', got '%s'", handler.GetString("environment.BASE_VAR"))
		}
		if !handler.GetBool("aws.enabled") {
			t.Error("Expected aws.enabled to be true")
		}
	})

	t.Run("SuccessWithYmlExtension", func(t *testing.T) {
		// Given a configHandler
		handler, mocks := setup(t)
		if err := handler.SetContext("local"); err != nil {
			t.Fatalf("Failed to set context: %v", err)
		}

		// Override the shim to return the correct context
		handler.(*configHandler).shims.Getenv = func(key string) string {
			if key == "WINDSOR_CONTEXT" {
				return "local"
			}
			return ""
		}

		// Create context-specific config file with .yml extension
		projectRoot, _ := mocks.Shell.GetProjectRootFunc()
		contextDir := filepath.Join(projectRoot, "contexts", "local")
		if err := os.MkdirAll(contextDir, 0755); err != nil {
			t.Fatalf("Failed to create context directory: %v", err)
		}

		contextConfigPath := filepath.Join(contextDir, "windsor.yml")
		contextConfig := `provider: local
environment:
  TEST_VAR: test_value`

		if err := os.WriteFile(contextConfigPath, []byte(contextConfig), 0644); err != nil {
			t.Fatalf("Failed to write context config: %v", err)
		}

		// Override shims to allow reading the actual context file
		handler.(*configHandler).shims.Stat = func(name string) (os.FileInfo, error) {
			return os.Stat(name)
		}
		handler.(*configHandler).shims.ReadFile = func(filename string) ([]byte, error) {
			return os.ReadFile(filename)
		}

		// When LoadContextConfig is called
		err := handler.LoadContextConfig()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("LoadContextConfig() unexpected error: %v", err)
		}

		// And the context configuration should be loaded
		if handler.GetString("provider") != "local" {
			t.Errorf("Expected provider to be 'local', got '%s'", handler.GetString("provider"))
		}
		if handler.GetString("environment.TEST_VAR") != "test_value" {
			t.Errorf("Expected TEST_VAR to be 'test_value', got '%s'", handler.GetString("environment.TEST_VAR"))
		}
	})

	t.Run("SuccessWithoutContextConfig", func(t *testing.T) {
		// Given a configHandler without context-specific config
		handler, _ := setup(t)
		if err := handler.SetContext("nonexistent"); err != nil {
			t.Fatalf("Failed to set context: %v", err)
		}

		// When LoadContextConfig is called
		err := handler.LoadContextConfig()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("LoadContextConfig() unexpected error: %v", err)
		}
	})

	t.Run("ErrorReadingContextConfigFile", func(t *testing.T) {
		// Given a configHandler
		handler, mocks := setup(t)
		if err := handler.SetContext("test"); err != nil {
			t.Fatalf("Failed to set context: %v", err)
		}

		// And a context config file that exists but cannot be read
		projectRoot, _ := mocks.Shell.GetProjectRootFunc()
		contextDir := filepath.Join(projectRoot, "contexts", "test")
		if err := os.MkdirAll(contextDir, 0755); err != nil {
			t.Fatalf("Failed to create context directory: %v", err)
		}

		contextConfigPath := filepath.Join(contextDir, "windsor.yaml")
		if err := os.WriteFile(contextConfigPath, []byte("test"), 0644); err != nil {
			t.Fatalf("Failed to write context config: %v", err)
		}

		// Mock ReadFile to return an error
		handler.(*configHandler).shims.ReadFile = func(filename string) ([]byte, error) {
			// Normalize path separators for cross-platform compatibility
			normalizedPath := filepath.ToSlash(filename)
			if strings.Contains(normalizedPath, "contexts/test/windsor.yaml") {
				return nil, fmt.Errorf("mocked read error")
			}
			return os.ReadFile(filename)
		}

		// When LoadContextConfig is called
		err := handler.LoadContextConfig()

		// Then an error should be returned
		if err == nil {
			t.Fatal("LoadContextConfig() expected error, got nil")
		}

		// The error should be from reading the context config file
		expectedError := "error reading context config file"
		if !strings.Contains(err.Error(), expectedError) {
			t.Errorf("LoadContextConfig() error = %v, expected to contain '%s'", err, expectedError)
		}
	})

	t.Run("ErrorUnmarshallingContextConfig", func(t *testing.T) {
		// Given a configHandler
		handler, mocks := setup(t)
		if err := handler.SetContext("test"); err != nil {
			t.Fatalf("Failed to set context: %v", err)
		}

		// Override the shim to return the correct context
		handler.(*configHandler).shims.Getenv = func(key string) string {
			if key == "WINDSOR_CONTEXT" {
				return "test"
			}
			return ""
		}

		// And a context config file with invalid YAML
		projectRoot, _ := mocks.Shell.GetProjectRootFunc()
		contextDir := filepath.Join(projectRoot, "contexts", "test")
		if err := os.MkdirAll(contextDir, 0755); err != nil {
			t.Fatalf("Failed to create context directory: %v", err)
		}

		contextConfigPath := filepath.Join(contextDir, "windsor.yaml")
		invalidYaml := `provider: aws
invalid yaml: [
`
		if err := os.WriteFile(contextConfigPath, []byte(invalidYaml), 0644); err != nil {
			t.Fatalf("Failed to write context config: %v", err)
		}

		// Override shims to allow reading the actual context file
		handler.(*configHandler).shims.Stat = func(name string) (os.FileInfo, error) {
			return os.Stat(name)
		}
		handler.(*configHandler).shims.ReadFile = func(filename string) ([]byte, error) {
			return os.ReadFile(filename)
		}

		// When LoadContextConfig is called
		err := handler.LoadContextConfig()

		// Then an error should be returned
		if err == nil {
			t.Fatal("LoadContextConfig() expected error, got nil")
		}

		// And the error message should contain the expected text
		if !strings.Contains(err.Error(), "error unmarshalling context yaml") {
			t.Errorf("LoadContextConfig() error = %v, expected to contain 'error unmarshalling context yaml'", err)
		}
	})

	t.Run("ErrorShellNotInitialized", func(t *testing.T) {
		// Given a configHandler without shell
		handler, _ := setup(t)
		handler.(*configHandler).shell = nil

		// When LoadContextConfig is called
		err := handler.LoadContextConfig()

		// Then an error should be returned
		if err == nil {
			t.Fatal("LoadContextConfig() expected error, got nil")
		}

		// And the error message should be as expected
		expectedError := "shell not initialized"
		if err.Error() != expectedError {
			t.Errorf("LoadContextConfig() error = %v, expected '%s'", err, expectedError)
		}
	})

	t.Run("ErrorGettingProjectRoot", func(t *testing.T) {
		// Given a configHandler with shell that returns error
		handler, mocks := setup(t)
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("mocked project root error")
		}

		// When LoadContextConfig is called
		err := handler.LoadContextConfig()

		// Then an error should be returned
		if err == nil {
			t.Fatal("LoadContextConfig() expected error, got nil")
		}

		// And the error message should be as expected
		expectedError := "error retrieving project root: mocked project root error"
		if err.Error() != expectedError {
			t.Errorf("LoadContextConfig() error = %v, expected '%s'", err, expectedError)
		}
	})

	t.Run("SimulateInitPipelineWorkflow", func(t *testing.T) {
		// Given a configHandler simulating the exact init pipeline workflow
		handler, mocks := setup(t)

		tempDir := t.TempDir()
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return tempDir, nil
		}

		// Create existing root config with only version (common in real scenarios)
		rootConfigPath := filepath.Join(tempDir, "windsor.yaml")
		rootConfig := `version: v1alpha1`
		os.WriteFile(rootConfigPath, []byte(rootConfig), 0644)

		// Step 1: Load existing config like init pipeline does in BasePipeline.Initialize
		if err := handler.LoadConfig(rootConfigPath); err != nil {
			t.Fatalf("Failed to load root config: %v", err)
		}

		// Step 2: Set context like init pipeline does
		if err := handler.SetContext("local"); err != nil {
			t.Fatalf("Failed to set context: %v", err)
		}

		// Step 3: Set default configuration like init pipeline does
		if err := handler.SetDefault(DefaultConfig); err != nil {
			t.Fatalf("Failed to set default config: %v", err)
		}

		// Step 4: Generate context ID like init pipeline does
		if err := handler.GenerateContextID(); err != nil {
			t.Fatalf("Failed to generate context ID: %v", err)
		}

		// Step 5: Save config like init pipeline does
		err := handler.SaveConfig()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And the context config should be created since context is not defined in root
		contextConfigPath := filepath.Join(tempDir, "contexts", "local", "windsor.yaml")
		if _, err := handler.(*configHandler).shims.Stat(contextConfigPath); os.IsNotExist(err) {
			t.Errorf("Context config file was not created at %s, this reproduces the user's issue", contextConfigPath)
		}

		// And the root config should not be overwritten
		rootContent, _ := os.ReadFile(rootConfigPath)
		if !strings.Contains(string(rootContent), "version: v1alpha1") {
			t.Errorf("Root config appears to have been overwritten")
		}
	})

	t.Run("ContextNotSetInRootConfigInitially", func(t *testing.T) {
		// Given a configHandler that mimics the exact init flow
		handler, mocks := setup(t)

		tempDir := t.TempDir()
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return tempDir, nil
		}

		// Create existing root config with only version (user's scenario)
		rootConfigPath := filepath.Join(tempDir, "windsor.yaml")
		rootConfig := `version: v1alpha1`
		os.WriteFile(rootConfigPath, []byte(rootConfig), 0644)

		// Load the existing root config
		if err := handler.LoadConfig(rootConfigPath); err != nil {
			t.Fatalf("Failed to load root config: %v", err)
		}

		// Set the context but DON'T call Set() to add context data yet
		handler.(*configHandler).context = "local"

		// Debug: Check state before adding any context data
		t.Logf("Config.Contexts before setting any context data: %+v", handler.(*configHandler).config.Contexts)

		// When SaveConfig is called without any context configuration being set
		err := handler.SaveConfig()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// Check if context config was created
		contextConfigPath := filepath.Join(tempDir, "contexts", "local", "windsor.yaml")
		if _, err := handler.(*configHandler).shims.Stat(contextConfigPath); os.IsNotExist(err) {
			t.Errorf("Context config file was NOT created at %s - this reproduces the user's issue", contextConfigPath)
		} else {
			t.Logf("Context config file WAS created at %s", contextConfigPath)
		}
	})

	t.Run("ReproduceActualIssue", func(t *testing.T) {
		// Given a real-world scenario where a root windsor.yaml exists with only version
		handler, mocks := setup(t)

		tempDir := t.TempDir()
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return tempDir, nil
		}

		// Create existing root config with only version (exact user scenario)
		rootConfigPath := filepath.Join(tempDir, "windsor.yaml")
		rootConfig := `version: v1alpha1`
		os.WriteFile(rootConfigPath, []byte(rootConfig), 0644)

		// Step 1: Load existing config like init pipeline does
		if err := handler.LoadConfig(rootConfigPath); err != nil {
			t.Fatalf("Failed to load root config: %v", err)
		}

		// Step 2: Set context
		if err := handler.SetContext("local"); err != nil {
			t.Fatalf("Failed to set context: %v", err)
		}

		// Step 3: Set default configuration (this would add context data)
		if err := handler.SetDefault(DefaultConfig); err != nil {
			t.Fatalf("Failed to set default config: %v", err)
		}

		// Step 4: Generate context ID
		if err := handler.GenerateContextID(); err != nil {
			t.Fatalf("Failed to generate context ID: %v", err)
		}

		// Debug: Check config state before SaveConfig
		t.Logf("Config before SaveConfig: %+v", handler.(*configHandler).config)
		if handler.(*configHandler).config.Contexts != nil {
			if ctx, exists := handler.(*configHandler).config.Contexts["local"]; exists {
				t.Logf("local context exists in config: %+v", ctx)
			} else {
				t.Logf("local context does NOT exist in config")
			}
		} else {
			t.Logf("Config.Contexts is nil")
		}

		// Step 5: Save config (the critical call)
		err := handler.SaveConfig()
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// Check if context config file was created
		contextConfigPath := filepath.Join(tempDir, "contexts", "local", "windsor.yaml")
		if _, err := handler.(*configHandler).shims.Stat(contextConfigPath); os.IsNotExist(err) {
			t.Errorf("Context config file was NOT created at %s - this is the bug!", contextConfigPath)
		} else {
			content, _ := os.ReadFile(contextConfigPath)
			t.Logf("Context config file WAS created with content: %s", string(content))
		}

		// Check root config wasn't overwritten
		rootContent, _ := os.ReadFile(rootConfigPath)
		if !strings.Contains(string(rootContent), "version: v1alpha1") {
			t.Errorf("Root config appears to have been overwritten: %s", string(rootContent))
		}
	})
}
func TestConfigHandler_saveContextValues(t *testing.T) {
	setup := func(t *testing.T) (ConfigHandler, *Mocks) {
		mocks := setupMocks(t)
		handler := NewConfigHandler(mocks.Injector)
		handler.(*configHandler).shims = mocks.Shims
		if err := handler.Initialize(); err != nil {
			t.Fatalf("Failed to initialize handler: %v", err)
		}
		return handler, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a configHandler with context values
		handler, mocks := setup(t)

		tempDir := t.TempDir()
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return tempDir, nil
		}

		handler.(*configHandler).context = "test"
		handler.(*configHandler).contextValues = map[string]any{
			"database_url": "postgres://localhost:5432/test",
			"api_key":      "secret123",
		}

		// When saveContextValues is called
		err := handler.(*configHandler).saveContextValues()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// And the values.yaml file should be created
		valuesPath := filepath.Join(tempDir, "contexts", "test", "values.yaml")
		if _, err := handler.(*configHandler).shims.Stat(valuesPath); os.IsNotExist(err) {
			t.Fatalf("values.yaml file was not created at %s", valuesPath)
		}
	})

	t.Run("GetConfigRootError", func(t *testing.T) {
		// Given a configHandler with a shell that returns an error
		handler, mocks := setup(t)

		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("failed to get project root")
		}

		handler.(*configHandler).context = "test"
		handler.(*configHandler).contextValues = map[string]any{"key": "value"}

		// When saveContextValues is called
		err := handler.(*configHandler).saveContextValues()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}

		expectedError := "error getting config root: failed to get project root"
		if err.Error() != expectedError {
			t.Errorf("Expected error: %s, got: %s", expectedError, err.Error())
		}
	})

	t.Run("MkdirAllError", func(t *testing.T) {
		// Given a configHandler with a shims that fails on MkdirAll
		handler, mocks := setup(t)

		tempDir := t.TempDir()
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return tempDir, nil
		}

		// Mock MkdirAll to return an error
		originalMkdirAll := mocks.Shims.MkdirAll
		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error {
			return fmt.Errorf("mkdir failed")
		}

		handler.(*configHandler).context = "test"
		handler.(*configHandler).contextValues = map[string]any{"key": "value"}

		// When saveContextValues is called
		err := handler.(*configHandler).saveContextValues()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}

		expectedError := "error creating context directory: mkdir failed"
		if err.Error() != expectedError {
			t.Errorf("Expected error: %s, got: %s", expectedError, err.Error())
		}

		// Restore original function
		mocks.Shims.MkdirAll = originalMkdirAll
	})
}

func TestConfigHandler_ensureValuesYamlLoaded(t *testing.T) {
	setup := func(t *testing.T) (ConfigHandler, *Mocks) {
		mocks := setupMocks(t)
		handler := NewConfigHandler(mocks.Injector)
		handler.(*configHandler).shims = mocks.Shims
		if err := handler.Initialize(); err != nil {
			t.Fatalf("Failed to initialize handler: %v", err)
		}
		return handler, mocks
	}

	t.Run("AlreadyLoaded", func(t *testing.T) {
		// Given a handler with contextValues already loaded
		handler, _ := setup(t)
		handler.(*configHandler).contextValues = map[string]any{"existing": "value"}

		// When ensureValuesYamlLoaded is called
		err := handler.(*configHandler).ensureValuesYamlLoaded()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		// And contextValues should remain unchanged
		if handler.(*configHandler).contextValues["existing"] != "value" {
			t.Error("contextValues should remain unchanged")
		}
	})

	t.Run("ShellNotInitialized", func(t *testing.T) {
		// Given a handler with no shell initialized
		handler := &configHandler{}

		// When ensureValuesYamlLoaded is called
		err := handler.ensureValuesYamlLoaded()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		// And contextValues should be initialized as empty
		if handler.contextValues == nil {
			t.Error("contextValues should be initialized")
		}
		if len(handler.contextValues) != 0 {
			t.Errorf("Expected empty contextValues, got: %v", handler.contextValues)
		}
	})

	t.Run("ConfigNotLoaded", func(t *testing.T) {
		// Given a handler with shell but not loaded
		handler, _ := setup(t)
		handler.(*configHandler).loaded = false

		// When ensureValuesYamlLoaded is called
		err := handler.(*configHandler).ensureValuesYamlLoaded()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		// And contextValues should be initialized as empty
		if handler.(*configHandler).contextValues == nil {
			t.Error("contextValues should be initialized")
		}
		if len(handler.(*configHandler).contextValues) != 0 {
			t.Errorf("Expected empty contextValues, got: %v", handler.(*configHandler).contextValues)
		}
	})

	t.Run("ErrorGettingProjectRoot", func(t *testing.T) {
		// Given a handler with shell returning error on GetProjectRoot
		handler, mocks := setup(t)
		handler.(*configHandler).loaded = true
		handler.(*configHandler).contextValues = nil // Ensure it's not already loaded
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("project root error")
		}

		// When ensureValuesYamlLoaded is called
		err := handler.(*configHandler).ensureValuesYamlLoaded()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error retrieving project root") {
			t.Errorf("Expected 'error retrieving project root', got: %v", err)
		}
	})

	t.Run("LoadsSchemaIfNotLoaded", func(t *testing.T) {
		// Given a handler with schema validator but no schema loaded
		handler, mocks := setup(t)
		handler.(*configHandler).loaded = true
		handler.(*configHandler).context = "test"
		handler.(*configHandler).contextValues = nil
		handler.(*configHandler).schemaValidator = NewSchemaValidator(mocks.Shell)
		handler.(*configHandler).schemaValidator.Shims = NewShims() // Use real filesystem for schema validator

		// Use real filesystem operations for this test
		handler.(*configHandler).shims = NewShims()

		// Create temp directory structure
		tmpDir := t.TempDir()
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return tmpDir, nil
		}

		// Create schema file
		schemaDir := filepath.Join(tmpDir, "contexts", "_template")
		if err := os.MkdirAll(schemaDir, 0755); err != nil {
			t.Fatalf("Failed to create schema directory: %v", err)
		}
		schemaPath := filepath.Join(schemaDir, "schema.yaml")
		schemaContent := `
$schema: https://json-schema.org/draft/2020-12/schema
type: object
properties:
  test_key:
    type: string
`
		if err := os.WriteFile(schemaPath, []byte(schemaContent), 0644); err != nil {
			t.Fatalf("Failed to write schema file: %v", err)
		}

		// Create context directory but no values.yaml
		contextDir := filepath.Join(tmpDir, "contexts", "test")
		if err := os.MkdirAll(contextDir, 0755); err != nil {
			t.Fatalf("Failed to create context directory: %v", err)
		}

		// When ensureValuesYamlLoaded is called
		err := handler.(*configHandler).ensureValuesYamlLoaded()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		// And schema should be loaded
		if handler.(*configHandler).schemaValidator.Schema == nil {
			t.Error("Schema should be loaded")
		}

		// And contextValues should be initialized as empty
		if len(handler.(*configHandler).contextValues) != 0 {
			t.Errorf("Expected empty contextValues, got: %v", handler.(*configHandler).contextValues)
		}
	})

	t.Run("ErrorLoadingSchema", func(t *testing.T) {
		// Given a handler with schema validator and malformed schema file
		handler, mocks := setup(t)
		handler.(*configHandler).loaded = true
		handler.(*configHandler).context = "test"
		handler.(*configHandler).contextValues = nil
		handler.(*configHandler).schemaValidator = NewSchemaValidator(mocks.Shell)
		handler.(*configHandler).schemaValidator.Shims = NewShims() // Use real filesystem for schema validator

		// Use real filesystem operations for this test
		handler.(*configHandler).shims = NewShims()

		// Create temp directory structure
		tmpDir := t.TempDir()
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return tmpDir, nil
		}

		// Create malformed schema file
		schemaDir := filepath.Join(tmpDir, "contexts", "_template")
		if err := os.MkdirAll(schemaDir, 0755); err != nil {
			t.Fatalf("Failed to create schema directory: %v", err)
		}
		schemaPath := filepath.Join(schemaDir, "schema.yaml")
		if err := os.WriteFile(schemaPath, []byte("invalid: yaml: content:"), 0644); err != nil {
			t.Fatalf("Failed to write schema file: %v", err)
		}

		// When ensureValuesYamlLoaded is called
		err := handler.(*configHandler).ensureValuesYamlLoaded()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error for malformed schema, got nil")
		}
		if !strings.Contains(err.Error(), "error loading schema") {
			t.Errorf("Expected 'error loading schema', got: %v", err)
		}
	})

	t.Run("LoadsValuesYamlSuccessfully", func(t *testing.T) {
		// Given a standalone handler with valid values.yaml
		tmpDir := t.TempDir()
		injector := di.NewInjector()

		mockShell := shell.NewMockShell(injector)
		mockShell.GetProjectRootFunc = func() (string, error) {
			return tmpDir, nil
		}
		injector.Register("shell", mockShell)

		handler := NewConfigHandler(injector)
		handler.(*configHandler).shims = NewShims()
		handler.(*configHandler).shims.Getenv = func(key string) string {
			if key == "WINDSOR_CONTEXT" {
				return "test"
			}
			return ""
		}
		if err := handler.Initialize(); err != nil {
			t.Fatalf("Failed to initialize: %v", err)
		}
		handler.(*configHandler).loaded = true
		handler.(*configHandler).context = "test"
		handler.(*configHandler).contextValues = nil // Ensure values aren't already loaded

		// Create context directory and values.yaml
		contextDir := filepath.Join(tmpDir, "contexts", "test")
		if err := os.MkdirAll(contextDir, 0755); err != nil {
			t.Fatalf("Failed to create context directory: %v", err)
		}
		valuesPath := filepath.Join(contextDir, "values.yaml")
		valuesContent := `test_key: test_value
another_key: 123
`
		if err := os.WriteFile(valuesPath, []byte(valuesContent), 0644); err != nil {
			t.Fatalf("Failed to write values.yaml: %v", err)
		}

		// When ensureValuesYamlLoaded is called
		err := handler.(*configHandler).ensureValuesYamlLoaded()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		// And contextValues should contain the loaded values
		if handler.(*configHandler).contextValues == nil {
			t.Fatal("contextValues is nil")
		}
		if len(handler.(*configHandler).contextValues) == 0 {
			t.Fatal("contextValues is empty")
		}
		if handler.(*configHandler).contextValues["test_key"] != "test_value" {
			t.Errorf("Expected test_key='test_value', got: %v", handler.(*configHandler).contextValues["test_key"])
		}
		anotherKey, ok := handler.(*configHandler).contextValues["another_key"]
		if !ok {
			t.Error("Expected another_key to be present")
		} else {
			// YAML unmarshals numbers as different types depending on their value
			// Check if it's 123 regardless of the specific integer type
			switch v := anotherKey.(type) {
			case int:
				if v != 123 {
					t.Errorf("Expected another_key=123, got: %v", v)
				}
			case int64:
				if v != 123 {
					t.Errorf("Expected another_key=123, got: %v", v)
				}
			case uint64:
				if v != 123 {
					t.Errorf("Expected another_key=123, got: %v", v)
				}
			default:
				t.Errorf("Expected another_key to be numeric, got: %v (type: %T)", v, v)
			}
		}
	})

	t.Run("ErrorReadingValuesYaml", func(t *testing.T) {
		// Given a standalone handler with values.yaml that cannot be read
		tmpDir := t.TempDir()
		injector := di.NewInjector()

		mockShell := shell.NewMockShell(injector)
		mockShell.GetProjectRootFunc = func() (string, error) {
			return tmpDir, nil
		}
		injector.Register("shell", mockShell)

		handler := NewConfigHandler(injector)
		handler.(*configHandler).shims = NewShims()
		handler.(*configHandler).shims.Getenv = func(key string) string {
			if key == "WINDSOR_CONTEXT" {
				return "test"
			}
			return ""
		}
		if err := handler.Initialize(); err != nil {
			t.Fatalf("Failed to initialize: %v", err)
		}
		handler.(*configHandler).loaded = true
		handler.(*configHandler).context = "test"
		handler.(*configHandler).contextValues = nil // Ensure values aren't already loaded

		// Create context directory and values.yaml
		contextDir := filepath.Join(tmpDir, "contexts", "test")
		if err := os.MkdirAll(contextDir, 0755); err != nil {
			t.Fatalf("Failed to create context directory: %v", err)
		}
		valuesPath := filepath.Join(contextDir, "values.yaml")
		if err := os.WriteFile(valuesPath, []byte("test"), 0644); err != nil {
			t.Fatalf("Failed to write values.yaml: %v", err)
		}

		// Mock ReadFile to return error for values.yaml
		handler.(*configHandler).shims.ReadFile = func(filename string) ([]byte, error) {
			if strings.Contains(filename, "values.yaml") {
				return nil, fmt.Errorf("read error")
			}
			return os.ReadFile(filename)
		}

		// When ensureValuesYamlLoaded is called
		err := handler.(*configHandler).ensureValuesYamlLoaded()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error reading values.yaml") {
			t.Errorf("Expected 'error reading values.yaml', got: %v", err)
		}
	})

	t.Run("ErrorUnmarshallingValuesYaml", func(t *testing.T) {
		// Given a standalone handler with malformed values.yaml
		tmpDir := t.TempDir()
		injector := di.NewInjector()

		mockShell := shell.NewMockShell(injector)
		mockShell.GetProjectRootFunc = func() (string, error) {
			return tmpDir, nil
		}
		injector.Register("shell", mockShell)

		handler := NewConfigHandler(injector)
		handler.(*configHandler).shims = NewShims()
		handler.(*configHandler).shims.Getenv = func(key string) string {
			if key == "WINDSOR_CONTEXT" {
				return "test"
			}
			return ""
		}
		if err := handler.Initialize(); err != nil {
			t.Fatalf("Failed to initialize: %v", err)
		}
		handler.(*configHandler).loaded = true
		handler.(*configHandler).context = "test"
		handler.(*configHandler).contextValues = nil // Ensure values aren't already loaded

		// Create context directory and malformed values.yaml
		contextDir := filepath.Join(tmpDir, "contexts", "test")
		if err := os.MkdirAll(contextDir, 0755); err != nil {
			t.Fatalf("Failed to create context directory: %v", err)
		}
		valuesPath := filepath.Join(contextDir, "values.yaml")
		if err := os.WriteFile(valuesPath, []byte("invalid: yaml: content:"), 0644); err != nil {
			t.Fatalf("Failed to write values.yaml: %v", err)
		}

		// When ensureValuesYamlLoaded is called
		err := handler.(*configHandler).ensureValuesYamlLoaded()

		// Then an error should be returned
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error unmarshalling values.yaml") {
			t.Errorf("Expected 'error unmarshalling values.yaml', got: %v", err)
		}
	})

	t.Run("ValidatesValuesYamlWithSchema", func(t *testing.T) {
		// Given a standalone handler with schema and values.yaml
		tmpDir := t.TempDir()
		injector := di.NewInjector()

		mockShell := shell.NewMockShell(injector)
		mockShell.GetProjectRootFunc = func() (string, error) {
			return tmpDir, nil
		}
		injector.Register("shell", mockShell)

		handler := NewConfigHandler(injector)
		handler.(*configHandler).shims = NewShims()
		handler.(*configHandler).shims.Getenv = func(key string) string {
			if key == "WINDSOR_CONTEXT" {
				return "test"
			}
			return ""
		}
		if err := handler.Initialize(); err != nil {
			t.Fatalf("Failed to initialize: %v", err)
		}
		handler.(*configHandler).loaded = true
		handler.(*configHandler).context = "test"
		handler.(*configHandler).contextValues = nil // Ensure values aren't already loaded
		handler.(*configHandler).schemaValidator.Shims = NewShims()

		// Create schema file
		schemaDir := filepath.Join(tmpDir, "contexts", "_template")
		if err := os.MkdirAll(schemaDir, 0755); err != nil {
			t.Fatalf("Failed to create schema directory: %v", err)
		}
		schemaPath := filepath.Join(schemaDir, "schema.yaml")
		schemaContent := `$schema: https://json-schema.org/draft/2020-12/schema
type: object
properties:
  test_key:
    type: string
additionalProperties: false
`
		if err := os.WriteFile(schemaPath, []byte(schemaContent), 0644); err != nil {
			t.Fatalf("Failed to write schema file: %v", err)
		}

		// Create context directory and values.yaml with valid content
		contextDir := filepath.Join(tmpDir, "contexts", "test")
		if err := os.MkdirAll(contextDir, 0755); err != nil {
			t.Fatalf("Failed to create context directory: %v", err)
		}
		valuesPath := filepath.Join(contextDir, "values.yaml")
		valuesContent := `test_key: test_value
`
		if err := os.WriteFile(valuesPath, []byte(valuesContent), 0644); err != nil {
			t.Fatalf("Failed to write values.yaml: %v", err)
		}

		// When ensureValuesYamlLoaded is called
		err := handler.(*configHandler).ensureValuesYamlLoaded()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		// And contextValues should contain validated values
		if handler.(*configHandler).contextValues["test_key"] != "test_value" {
			t.Errorf("Expected test_key='test_value', got: %v", handler.(*configHandler).contextValues["test_key"])
		}
	})

	t.Run("ValidationFailsForInvalidValuesYaml", func(t *testing.T) {
		// Given a standalone handler with schema and invalid values.yaml
		tmpDir := t.TempDir()
		injector := di.NewInjector()

		mockShell := shell.NewMockShell(injector)
		mockShell.GetProjectRootFunc = func() (string, error) {
			return tmpDir, nil
		}
		injector.Register("shell", mockShell)

		handler := NewConfigHandler(injector)
		handler.(*configHandler).shims = NewShims()
		handler.(*configHandler).shims.Getenv = func(key string) string {
			if key == "WINDSOR_CONTEXT" {
				return "test"
			}
			return ""
		}
		if err := handler.Initialize(); err != nil {
			t.Fatalf("Failed to initialize: %v", err)
		}
		handler.(*configHandler).loaded = true
		handler.(*configHandler).context = "test"
		handler.(*configHandler).contextValues = nil // Ensure values aren't already loaded
		handler.(*configHandler).schemaValidator.Shims = NewShims()

		// Create schema file
		schemaDir := filepath.Join(tmpDir, "contexts", "_template")
		if err := os.MkdirAll(schemaDir, 0755); err != nil {
			t.Fatalf("Failed to create schema directory: %v", err)
		}
		schemaPath := filepath.Join(schemaDir, "schema.yaml")
		schemaContent := `$schema: https://json-schema.org/draft/2020-12/schema
type: object
properties:
  test_key:
    type: string
additionalProperties: false
`
		if err := os.WriteFile(schemaPath, []byte(schemaContent), 0644); err != nil {
			t.Fatalf("Failed to write schema file: %v", err)
		}

		// Create context directory and values.yaml with invalid content
		contextDir := filepath.Join(tmpDir, "contexts", "test")
		if err := os.MkdirAll(contextDir, 0755); err != nil {
			t.Fatalf("Failed to create context directory: %v", err)
		}
		valuesPath := filepath.Join(contextDir, "values.yaml")
		valuesContent := `invalid_key: should_not_be_allowed
`
		if err := os.WriteFile(valuesPath, []byte(valuesContent), 0644); err != nil {
			t.Fatalf("Failed to write values.yaml: %v", err)
		}

		// When ensureValuesYamlLoaded is called
		err := handler.(*configHandler).ensureValuesYamlLoaded()

		// Then a validation error should be returned
		if err == nil {
			t.Fatal("Expected validation error, got nil")
		}
		if !strings.Contains(err.Error(), "validation failed") {
			t.Errorf("Expected 'validation failed', got: %v", err)
		}
	})

	t.Run("NoValuesYamlFileInitializesEmpty", func(t *testing.T) {
		// Given a handler with no values.yaml file
		handler, mocks := setup(t)
		handler.(*configHandler).loaded = true
		handler.(*configHandler).context = "test"
		handler.(*configHandler).contextValues = nil

		// Use real filesystem operations for this test
		handler.(*configHandler).shims = NewShims()

		// Create temp directory structure
		tmpDir := t.TempDir()
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return tmpDir, nil
		}

		// Create context directory but NO values.yaml
		contextDir := filepath.Join(tmpDir, "contexts", "test")
		if err := os.MkdirAll(contextDir, 0755); err != nil {
			t.Fatalf("Failed to create context directory: %v", err)
		}

		// When ensureValuesYamlLoaded is called
		err := handler.(*configHandler).ensureValuesYamlLoaded()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		// And contextValues should be initialized as empty
		if handler.(*configHandler).contextValues == nil {
			t.Error("contextValues should be initialized")
		}
		if len(handler.(*configHandler).contextValues) != 0 {
			t.Errorf("Expected empty contextValues, got: %v", handler.(*configHandler).contextValues)
		}
	})
}

// =============================================================================
// Additional Tests for Full Coverage (from config_handler_test.go)
// =============================================================================

func TestConfigHandler_IsLoaded(t *testing.T) {
	setup := func(t *testing.T) ConfigHandler {
		mocks := setupMocks(t)
		handler := NewConfigHandler(mocks.Injector)
		handler.(*configHandler).shims = mocks.Shims
		return handler
	}

	t.Run("IsLoadedTrue", func(t *testing.T) {
		handler := setup(t)
		handler.(*configHandler).loaded = true

		isLoaded := handler.IsLoaded()

		if !isLoaded {
			t.Errorf("expected IsLoaded to return true, got false")
		}
	})

	t.Run("IsLoadedFalse", func(t *testing.T) {
		handler := setup(t)
		handler.(*configHandler).loaded = false

		isLoaded := handler.IsLoaded()

		if isLoaded {
			t.Errorf("expected IsLoaded to return false, got true")
		}
	})
}

func TestConfigHandler_IsContextConfigLoaded(t *testing.T) {
	setup := func(t *testing.T) ConfigHandler {
		mocks := setupMocks(t)
		handler := NewConfigHandler(mocks.Injector)
		handler.(*configHandler).shims = mocks.Shims
		return handler
	}

	t.Run("ReturnsFalseWhenBaseConfigNotLoaded", func(t *testing.T) {
		handler := setup(t)
		handler.(*configHandler).loaded = false

		isLoaded := handler.IsContextConfigLoaded()

		if isLoaded {
			t.Errorf("expected IsContextConfigLoaded to return false when base config not loaded, got true")
		}
	})

	t.Run("ReturnsFalseWhenContextNotSet", func(t *testing.T) {
		handler, mocks := setup(t).(*configHandler), setupMocks(t)
		handler.loaded = true
		handler.shims = mocks.Shims

		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "/mock/project/root", nil
		}
		mocks.Shims.ReadFile = func(filename string) ([]byte, error) {
			return []byte(""), nil
		}
		mocks.Shims.Getenv = func(key string) string {
			return ""
		}
		handler.shell = mocks.Shell

		isLoaded := handler.IsContextConfigLoaded()

		if isLoaded {
			t.Errorf("expected IsContextConfigLoaded to return false when context not set, got true")
		}
	})

	t.Run("ReturnsTrueWhenContextExistsAndIsValid", func(t *testing.T) {
		handler := setup(t)
		handler.(*configHandler).loaded = true
		handler.(*configHandler).config = v1alpha1.Config{
			Contexts: map[string]*v1alpha1.Context{
				"test-context": {
					Cluster: &cluster.ClusterConfig{
						Workers: cluster.NodeGroupConfig{
							Volumes: []string{"/var/blah"},
						},
					},
				},
			},
		}

		mocks := setupMocks(t)
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "/mock/project/root", nil
		}
		mocks.Shims.ReadFile = func(filename string) ([]byte, error) {
			return []byte("test-context"), nil
		}
		mocks.Shims.Getenv = func(key string) string {
			return ""
		}
		handler.(*configHandler).shims = mocks.Shims
		handler.(*configHandler).shell = mocks.Shell
		handler.(*configHandler).loadedContexts["test-context"] = true

		isLoaded := handler.IsContextConfigLoaded()

		if !isLoaded {
			t.Errorf("expected IsContextConfigLoaded to return true when context exists and is valid, got false")
		}
	})
}

func TestConfigHandler_GetContext(t *testing.T) {
	setup := func(t *testing.T) ConfigHandler {
		mocks := setupMocks(t)
		handler := NewConfigHandler(mocks.Injector)
		handler.(*configHandler).shims = mocks.Shims
		return handler
	}

	t.Run("Success", func(t *testing.T) {
		handler := setup(t)
		mocks := setupMocks(t)

		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "/mock/project/root", nil
		}
		mocks.Shims.ReadFile = func(filename string) ([]byte, error) {
			if filename == filepath.Join("/mock/project/root", ".windsor", "context") {
				return []byte("test-context"), nil
			}
			return nil, fmt.Errorf("file not found")
		}
		mocks.Shims.Getenv = func(key string) string {
			return ""
		}
		handler.(*configHandler).shims = mocks.Shims
		handler.(*configHandler).shell = mocks.Shell

		contextValue := handler.GetContext()

		if contextValue != "test-context" {
			t.Errorf("expected context 'test-context', got %s", contextValue)
		}
	})

	t.Run("GetContextDefaultsToLocal", func(t *testing.T) {
		handler := setup(t)
		mocks := setupMocks(t)

		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "/mock/project/root", nil
		}
		mocks.Shims.ReadFile = func(_ string) ([]byte, error) {
			return nil, fmt.Errorf("file not found")
		}
		mocks.Shims.Getenv = func(key string) string {
			return ""
		}
		handler.(*configHandler).shims = mocks.Shims
		handler.(*configHandler).shell = mocks.Shell

		actualContext := handler.GetContext()

		expectedContext := "local"
		if actualContext != expectedContext {
			t.Errorf("Expected context %q, got %q", expectedContext, actualContext)
		}
	})

	t.Run("ContextFromEnvironment", func(t *testing.T) {
		handler := setup(t)
		mocks := setupMocks(t)

		mocks.Shims.Getenv = func(key string) string {
			if key == "WINDSOR_CONTEXT" {
				return "env-context"
			}
			return ""
		}
		handler.(*configHandler).shims = mocks.Shims

		actualContext := handler.GetContext()

		if actualContext != "env-context" {
			t.Errorf("Expected context 'env-context', got %q", actualContext)
		}
	})
}

func TestConfigHandler_SetContext(t *testing.T) {
	setup := func(t *testing.T) ConfigHandler {
		mocks := setupMocks(t)
		handler := NewConfigHandler(mocks.Injector)
		handler.(*configHandler).shims = mocks.Shims
		handler.(*configHandler).shell = mocks.Shell
		return handler
	}

	t.Run("Success", func(t *testing.T) {
		handler := setup(t)
		mocks := setupMocks(t)

		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "/mock/project/root", nil
		}
		mocks.Shims.MkdirAll = func(path string, perm os.FileMode) error {
			return nil
		}
		mocks.Shims.WriteFile = func(filename string, data []byte, perm os.FileMode) error {
			return nil
		}
		mocks.Shims.Setenv = func(key, value string) error {
			return nil
		}
		handler.(*configHandler).shims = mocks.Shims
		handler.(*configHandler).shell = mocks.Shell

		err := handler.SetContext("new-context")

		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("GetProjectRootError", func(t *testing.T) {
		handler := setup(t)
		mocks := setupMocks(t)

		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("mocked error inside GetProjectRoot")
		}
		handler.(*configHandler).shell = mocks.Shell

		err := handler.SetContext("new-context")

		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestConfigHandler_Clean(t *testing.T) {
	setup := func(t *testing.T) ConfigHandler {
		mocks := setupMocks(t)
		handler := NewConfigHandler(mocks.Injector)
		handler.(*configHandler).shims = mocks.Shims
		handler.(*configHandler).shell = mocks.Shell
		return handler
	}

	t.Run("Success", func(t *testing.T) {
		handler := setup(t)
		mocks := setupMocks(t)

		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "/mock/project/root", nil
		}
		mocks.Shims.RemoveAll = func(path string) error {
			return nil
		}
		handler.(*configHandler).shims = mocks.Shims
		handler.(*configHandler).shell = mocks.Shell

		err := handler.Clean()

		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("ErrorGettingConfigRoot", func(t *testing.T) {
		handler := setup(t)
		mocks := setupMocks(t)

		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("error getting project root")
		}
		handler.(*configHandler).shell = mocks.Shell

		err := handler.Clean()

		if err == nil {
			t.Fatalf("expected error, got none")
		}
	})
}

func TestConfigHandler_SetSecretsProvider(t *testing.T) {
	t.Run("AddsProvider", func(t *testing.T) {
		mocks := setupMocks(t)
		handler := NewConfigHandler(mocks.Injector)

		mockProvider := secrets.NewMockSecretsProvider(mocks.Injector)

		handler.SetSecretsProvider(mockProvider)

		if len(handler.(*configHandler).secretsProviders) != 1 {
			t.Errorf("Expected 1 secrets provider, got %d", len(handler.(*configHandler).secretsProviders))
		}
	})

	t.Run("AddsMultipleProviders", func(t *testing.T) {
		mocks := setupMocks(t)
		handler := NewConfigHandler(mocks.Injector)

		mockProvider1 := secrets.NewMockSecretsProvider(mocks.Injector)
		mockProvider2 := secrets.NewMockSecretsProvider(mocks.Injector)

		handler.SetSecretsProvider(mockProvider1)
		handler.SetSecretsProvider(mockProvider2)

		if len(handler.(*configHandler).secretsProviders) != 2 {
			t.Errorf("Expected 2 secrets providers, got %d", len(handler.(*configHandler).secretsProviders))
		}
	})
}

func TestConfigHandler_LoadSchema(t *testing.T) {
	setup := func(t *testing.T) ConfigHandler {
		mocks := setupMocks(t)
		handler := NewConfigHandler(mocks.Injector)
		handler.(*configHandler).shims = mocks.Shims
		err := handler.Initialize()
		if err != nil {
			t.Fatalf("failed to initialize: %v", err)
		}
		return handler
	}

	t.Run("Success", func(t *testing.T) {
		handler := setup(t)
		mocks := setupMocks(t)

		schemaContent := []byte(`
$schema: https://schemas.windsorcli.dev/blueprint-config/v1alpha1
type: object
properties:
  test_key:
    type: string`)

		mocks.Shims.ReadFile = func(filename string) ([]byte, error) {
			return schemaContent, nil
		}
		handler.(*configHandler).shims = mocks.Shims
		if handler.(*configHandler).schemaValidator != nil {
			handler.(*configHandler).schemaValidator.Shims = mocks.Shims
		}

		err := handler.LoadSchema("/path/to/schema.yaml")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("ErrorReadingFile", func(t *testing.T) {
		handler := setup(t)
		mocks := setupMocks(t)

		mocks.Shims.ReadFile = func(filename string) ([]byte, error) {
			return nil, fmt.Errorf("read error")
		}
		handler.(*configHandler).shims = mocks.Shims

		err := handler.LoadSchema("/path/to/schema.yaml")
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestConfigHandler_LoadSchemaFromBytes(t *testing.T) {
	setup := func(t *testing.T) ConfigHandler {
		mocks := setupMocks(t)
		handler := NewConfigHandler(mocks.Injector)
		handler.(*configHandler).shims = mocks.Shims
		err := handler.Initialize()
		if err != nil {
			t.Fatalf("failed to initialize: %v", err)
		}
		return handler
	}

	t.Run("Success", func(t *testing.T) {
		handler := setup(t)

		schemaContent := []byte(`{
			"$schema": "https://schemas.windsorcli.dev/blueprint-config/v1alpha1",
			"type": "object",
			"properties": {
				"test_key": {
					"type": "string"
				}
			}
		}`)

		err := handler.LoadSchemaFromBytes(schemaContent)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("ErrorInvalidSchema", func(t *testing.T) {
		handler := setup(t)

		schemaContent := []byte(`invalid json`)

		err := handler.LoadSchemaFromBytes(schemaContent)
		if err == nil {
			t.Fatal("expected error for invalid schema")
		}
	})
}

func TestConfigHandler_GetSchemaDefaults(t *testing.T) {
	setup := func(t *testing.T) ConfigHandler {
		mocks := setupMocks(t)
		handler := NewConfigHandler(mocks.Injector)
		handler.(*configHandler).shims = mocks.Shims
		err := handler.Initialize()
		if err != nil {
			t.Fatalf("failed to initialize: %v", err)
		}
		return handler
	}

	t.Run("ReturnsDefaults", func(t *testing.T) {
		handler := setup(t)

		schemaContent := []byte(`{
			"$schema": "https://schemas.windsorcli.dev/blueprint-config/v1alpha1",
			"type": "object",
			"properties": {
				"test_key": {
					"type": "string",
					"default": "test_value"
				}
			}
		}`)

		err := handler.LoadSchemaFromBytes(schemaContent)
		if err != nil {
			t.Fatalf("failed to load schema: %v", err)
		}

		defaults, err := handler.GetSchemaDefaults()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if defaults["test_key"] != "test_value" {
			t.Errorf("expected default value 'test_value', got %v", defaults["test_key"])
		}
	})

	t.Run("ErrorWhenSchemaNotLoaded", func(t *testing.T) {
		handler := setup(t)

		_, err := handler.GetSchemaDefaults()
		if err == nil {
			t.Fatal("expected error when schema not loaded")
		}
	})
}

func TestConfigHandler_GetContextValues(t *testing.T) {
	setup := func(t *testing.T) ConfigHandler {
		mocks := setupMocks(t)
		handler := NewConfigHandler(mocks.Injector)
		handler.(*configHandler).shims = mocks.Shims
		return handler
	}

	t.Run("MergesConfigAndValues", func(t *testing.T) {
		handler := setup(t)

		h := handler.(*configHandler)
		h.context = "test"
		h.loaded = true
		h.config.Contexts = map[string]*v1alpha1.Context{
			"test": {
				Cluster: &cluster.ClusterConfig{
					Enabled: ptrBool(true),
				},
			},
		}
		h.contextValues = map[string]any{
			"custom_key": "custom_value",
		}

		values, err := handler.GetContextValues()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if values["custom_key"] != "custom_value" {
			t.Error("expected custom_value to be in merged values")
		}
	})

	t.Run("IncludesSchemaDefaults", func(t *testing.T) {
		handler := setup(t)

		err := handler.Initialize()
		if err != nil {
			t.Fatalf("failed to initialize handler: %v", err)
		}

		h := handler.(*configHandler)
		h.context = "test"
		h.loaded = true
		h.contextValues = map[string]any{
			"override_key": "override_value",
		}

		schemaContent := []byte(`{
			"$schema": "https://schemas.windsorcli.dev/blueprint-config/v1alpha1",
			"type": "object",
			"properties": {
				"default_key": {
					"type": "string",
					"default": "default_value"
				},
				"override_key": {
					"type": "string",
					"default": "default_override"
				},
				"nested": {
					"type": "object",
					"properties": {
						"nested_default": {
							"type": "string",
							"default": "nested_value"
						}
					}
				}
			}
		}`)

		err = handler.LoadSchemaFromBytes(schemaContent)
		if err != nil {
			t.Fatalf("failed to load schema: %v", err)
		}

		values, err := handler.GetContextValues()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if values["default_key"] != "default_value" {
			t.Errorf("expected default_key from schema defaults, got %v", values["default_key"])
		}

		if values["override_key"] != "override_value" {
			t.Errorf("expected override_key from values.yaml, got %v", values["override_key"])
		}

		if nested, ok := values["nested"].(map[string]any); ok {
			if nested["nested_default"] != "nested_value" {
				t.Errorf("expected nested.nested_default from schema, got %v", nested["nested_default"])
			}
		} else {
			t.Error("expected nested to be a map")
		}
	})
}

func TestConfigHandler_deepMerge(t *testing.T) {
	setup := func(t *testing.T) *configHandler {
		mocks := setupMocks(t)
		handler := NewConfigHandler(mocks.Injector)
		return handler.(*configHandler)
	}

	t.Run("MergesSimpleValues", func(t *testing.T) {
		handler := setup(t)

		base := map[string]any{
			"key1": "value1",
			"key2": "value2",
		}
		overlay := map[string]any{
			"key2": "override2",
			"key3": "value3",
		}

		result := handler.deepMerge(base, overlay)

		if result["key1"] != "value1" {
			t.Errorf("expected key1 to remain from base")
		}
		if result["key2"] != "override2" {
			t.Errorf("expected key2 to be overridden")
		}
		if result["key3"] != "value3" {
			t.Errorf("expected key3 to be added from overlay")
		}
	})

	t.Run("MergesNestedMaps", func(t *testing.T) {
		handler := setup(t)

		base := map[string]any{
			"nested": map[string]any{
				"key1": "value1",
				"key2": "value2",
			},
		}
		overlay := map[string]any{
			"nested": map[string]any{
				"key2": "override2",
				"key3": "value3",
			},
		}

		result := handler.deepMerge(base, overlay)

		nested := result["nested"].(map[string]any)
		if nested["key1"] != "value1" {
			t.Errorf("expected nested.key1 to remain from base")
		}
		if nested["key2"] != "override2" {
			t.Errorf("expected nested.key2 to be overridden")
		}
		if nested["key3"] != "value3" {
			t.Errorf("expected nested.key3 to be added from overlay")
		}
	})
}
