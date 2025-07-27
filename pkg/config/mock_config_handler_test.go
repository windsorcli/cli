package config

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/secrets"
)

func TestMockConfigHandler_Initialize(t *testing.T) {
	mockInitializeErr := fmt.Errorf("mock initialize error")

	t.Run("WithFuncSet", func(t *testing.T) {
		// Given a mock config handler with InitializeFunc set to return an error
		handler := NewMockConfigHandler()
		handler.InitializeFunc = func() error { return mockInitializeErr }

		// When Initialize is called
		err := handler.Initialize()

		// Then the error should match the expected mock error
		if err != mockInitializeErr {
			t.Errorf("Expected error = %v, got = %v", mockInitializeErr, err)
		}
	})

	t.Run("WithNoFuncSet", func(t *testing.T) {
		// Given a mock config handler without InitializeFunc set
		handler := NewMockConfigHandler()

		// When Initialize is called
		err := handler.Initialize()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected error = %v, got = %v", nil, err)
		}
	})
}

func TestMockConfigHandler_LoadConfig(t *testing.T) {
	mockLoadErr := fmt.Errorf("mock load config error")

	t.Run("WithPath", func(t *testing.T) {
		// Given a mock config handler with LoadConfigFunc set to return an error
		handler := NewMockConfigHandler()
		handler.LoadConfigFunc = func(path string) error {
			return mockLoadErr
		}

		// When LoadConfig is called with a path
		err := handler.LoadConfig("some/path")

		// Then the error should match the expected mock error
		if err != mockLoadErr {
			t.Errorf("Expected error = %v, got = %v", mockLoadErr, err)
		}
	})

	t.Run("WithNoFuncSet", func(t *testing.T) {
		// Given a mock config handler without LoadConfigFunc set
		handler := NewMockConfigHandler()

		// When LoadConfig is called with a path
		err := handler.LoadConfig("some/path")

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected error = %v, got = %v", nil, err)
		}
	})
}

func TestMockConfigHandler_GetString(t *testing.T) {
	t.Run("WithKey", func(t *testing.T) {
		// Given a mock config handler with GetStringFunc set to return an empty string
		handler := NewMockConfigHandler()
		handler.GetStringFunc = func(key string, defaultValue ...string) string { return "" }

		// When GetString is called with a key
		value := handler.GetString("someKey")

		// Then the returned value should be an empty string
		if value != "" {
			t.Errorf("Expected GetString with key to return empty string, got %v", value)
		}
	})

	t.Run("WithNoFuncSet", func(t *testing.T) {
		// Given a mock config handler without GetStringFunc set
		handler := NewMockConfigHandler()

		// When GetString is called with a key
		value := handler.GetString("someKey")

		// Then the returned value should be 'mock-string'
		if value != "mock-string" {
			t.Errorf("Expected GetString with no func set to return 'mock-string', got %v", value)
		}
	})

	t.Run("WithDefaultValue", func(t *testing.T) {
		// Given a mock config handler
		handler := NewMockConfigHandler()
		defaultValue := "default"

		// When GetString is called with a key and a default value
		value := handler.GetString("someKey", defaultValue)

		// Then the returned value should match the default value
		if value != defaultValue {
			t.Errorf("Expected GetString with default to return %v, got %v", defaultValue, value)
		}
	})
}

func TestMockConfigHandler_GetInt(t *testing.T) {
	t.Run("WithKey", func(t *testing.T) {
		// Given a mock config handler with GetIntFunc set to return 0
		handler := NewMockConfigHandler()
		handler.GetIntFunc = func(key string, defaultValue ...int) int { return 0 }

		// When GetInt is called with a key
		value := handler.GetInt("someKey")

		// Then the returned value should be 0
		if value != 0 {
			t.Errorf("Expected GetInt with key to return 0, got %v", value)
		}
	})

	t.Run("WithNoFuncSet", func(t *testing.T) {
		// Given a mock config handler without GetIntFunc set
		handler := NewMockConfigHandler()

		// When GetInt is called with a key
		value := handler.GetInt("someKey")

		// Then the returned value should be 42
		if value != 42 {
			t.Errorf("Expected GetInt with no func set to return 42, got %v", value)
		}
	})

	t.Run("WithDefaultValue", func(t *testing.T) {
		// Given a mock config handler
		handler := NewMockConfigHandler()
		defaultValue := 42

		// When GetInt is called with a key and a default value
		value := handler.GetInt("someKey", defaultValue)

		// Then the returned value should match the default value
		if value != defaultValue {
			t.Errorf("Expected GetInt with default to return %v, got %v", defaultValue, value)
		}
	})
}

func TestMockConfigHandler_GetBool(t *testing.T) {
	t.Run("WithKey", func(t *testing.T) {
		// Given a mock config handler with GetBoolFunc set to return false
		handler := NewMockConfigHandler()
		handler.GetBoolFunc = func(key string, defaultValue ...bool) bool { return false }

		// When GetBool is called with a key
		value := handler.GetBool("someKey")

		// Then the returned value should be false
		if value != false {
			t.Errorf("Expected GetBool with key to return false, got %v", value)
		}
	})

	t.Run("WithNoFuncSet", func(t *testing.T) {
		// Given a mock config handler without GetBoolFunc set
		handler := NewMockConfigHandler()

		// When GetBool is called with a key
		value := handler.GetBool("someKey")

		// Then the returned value should be true
		if value != true {
			t.Errorf("Expected GetBool with no func set to return true, got %v", value)
		}
	})

	t.Run("WithDefaultValue", func(t *testing.T) {
		// Given a mock config handler
		handler := NewMockConfigHandler()
		defaultValue := true

		// When GetBool is called with a key and a default value
		value := handler.GetBool("someKey", defaultValue)

		// Then the returned value should match the default value
		if value != defaultValue {
			t.Errorf("Expected GetBool with default to return %v, got %v", defaultValue, value)
		}
	})
}

func TestMockConfigHandler_GetStringSlice(t *testing.T) {
	t.Run("WithKey", func(t *testing.T) {
		// Given a mock config handler with GetStringSliceFunc set to return a specific slice
		handler := NewMockConfigHandler()
		expectedSlice := []string{"value1", "value2"}
		handler.GetStringSliceFunc = func(key string, defaultValue ...[]string) []string { return expectedSlice }

		// When GetStringSlice is called with a key
		value := handler.GetStringSlice("someKey")

		// Then the returned value should match the expected slice
		if !reflect.DeepEqual(value, expectedSlice) {
			t.Errorf("Expected GetStringSlice with key to return %v, got %v", expectedSlice, value)
		}
	})

	t.Run("WithNoFuncSet", func(t *testing.T) {
		// Given a mock config handler without GetStringSliceFunc set
		handler := NewMockConfigHandler()

		// When GetStringSlice is called with a key
		value := handler.GetStringSlice("someKey")

		// Then the returned value should be the default empty slice
		if len(value) != 0 {
			t.Errorf("Expected GetStringSlice with no func set to return an empty slice, got %v", value)
		}
	})

	t.Run("WithDefaultValue", func(t *testing.T) {
		// Given a mock config handler
		handler := NewMockConfigHandler()
		defaultValue := []string{"default1", "default2"}

		// When GetStringSlice is called with a key and a default value
		value := handler.GetStringSlice("someKey", defaultValue)

		// Then the returned value should match the default value
		if !reflect.DeepEqual(value, defaultValue) {
			t.Errorf("Expected GetStringSlice with default to return %v, got %v", defaultValue, value)
		}
	})
}

func TestMockConfigHandler_GetStringMap(t *testing.T) {
	t.Run("WithKey", func(t *testing.T) {
		// Given a mock config handler with GetStringMapFunc set to return a specific map
		handler := NewMockConfigHandler()
		expectedMap := map[string]string{"key1": "value1", "key2": "value2"}
		handler.GetStringMapFunc = func(key string, defaultValue ...map[string]string) map[string]string { return expectedMap }

		// When GetStringMap is called with a key
		value := handler.GetStringMap("someKey")

		// Then the returned value should match the expected map
		if !reflect.DeepEqual(value, expectedMap) {
			t.Errorf("Expected GetStringMap with key to return %v, got %v", expectedMap, value)
		}
	})

	t.Run("WithNoFuncSet", func(t *testing.T) {
		// Given a mock config handler without GetStringMapFunc set
		handler := NewMockConfigHandler()

		// When GetStringMap is called with a key
		value := handler.GetStringMap("someKey")

		// Then the returned value should be the default empty map
		if len(value) != 0 {
			t.Errorf("Expected GetStringMap with no func set to return an empty map, got %v", value)
		}
	})

	t.Run("WithDefaultValue", func(t *testing.T) {
		// Given a mock config handler
		handler := NewMockConfigHandler()
		defaultValue := map[string]string{"defaultKey1": "defaultValue1", "defaultKey2": "defaultValue2"}

		// When GetStringMap is called with a key and a default value
		value := handler.GetStringMap("someKey", defaultValue)

		// Then the returned value should match the default value
		if !reflect.DeepEqual(value, defaultValue) {
			t.Errorf("Expected GetStringMap with default to return %v, got %v", defaultValue, value)
		}
	})
}

func TestMockConfigHandler_Set(t *testing.T) {
	t.Run("WithKeyAndValue", func(t *testing.T) {
		// Given a mock config handler with SetFunc set to do nothing
		handler := NewMockConfigHandler()
		handler.SetFunc = func(key string, value any) error { return nil }

		// When Set is called with a key and a value
		handler.Set("someKey", "someValue")

		// Then no error should occur
	})

	t.Run("WithNoFuncSet", func(t *testing.T) {
		// Given a mock config handler without SetFunc set
		handler := NewMockConfigHandler()

		// When Set is called with a key and a value
		handler.Set("someKey", "someValue")

		// Then no error should occur
	})
}

func TestMockConfigHandler_SetContextValue(t *testing.T) {
	t.Run("WithKeyAndValue", func(t *testing.T) {
		// Given a mock config handler with SetContextValueFunc set to do nothing
		handler := NewMockConfigHandler()
		handler.SetContextValueFunc = func(key string, value any) error { return nil }

		// When SetContextValue is called with a key and a value
		err := handler.SetContextValue("someKey", "someValue")

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected SetContextValue to return nil, got %v", err)
		}
	})

	t.Run("WithNoFuncSet", func(t *testing.T) {
		// Given a mock config handler without SetContextValueFunc set
		handler := NewMockConfigHandler()

		// When SetContextValue is called with a key and a value
		err := handler.SetContextValue("someKey", "someValue")

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected SetContextValue to return nil, got %v", err)
		}
	})
}

func TestMockConfigHandler_SaveConfig(t *testing.T) {
	mockSaveErr := fmt.Errorf("mock save config error")

	t.Run("WithPath", func(t *testing.T) {
		// Given a mock config handler with SaveConfigFunc set to return an error
		handler := NewMockConfigHandler()
		handler.SaveConfigFunc = func(overwrite ...bool) error { return mockSaveErr }

		// When SaveConfig is called
		err := handler.SaveConfig()

		// Then the error should match the expected mock error
		if err != mockSaveErr {
			t.Errorf("Expected error = %v, got = %v", mockSaveErr, err)
		}
	})

	t.Run("WithNoFuncSet", func(t *testing.T) {
		// Given a mock config handler without SaveConfigFunc set
		handler := NewMockConfigHandler()

		// When SaveConfig is called
		err := handler.SaveConfig()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected SaveConfig to return nil, got %v", err)
		}
	})
}

func TestMockConfigHandler_Get(t *testing.T) {
	t.Run("WithKey", func(t *testing.T) {
		// Given a mock config handler with GetFunc set to return 'mock-value'
		handler := NewMockConfigHandler()
		handler.GetFunc = func(key string) any { return "mock-value" }

		// When Get is called with a key
		value := handler.Get("someKey")

		// Then the returned value should be 'mock-value'
		if value != "mock-value" {
			t.Errorf("Expected Get to return 'mock-value', got %v", value)
		}
	})

	t.Run("WithNoFuncSet", func(t *testing.T) {
		// Given a mock config handler without GetFunc set
		handler := NewMockConfigHandler()

		// When Get is called with a key
		value := handler.Get("someKey")

		// Then the returned value should be 'mock-value'
		if value != "mock-value" {
			t.Errorf("Expected Get to return 'mock-value', got %v", value)
		}
	})
}

func TestMockConfigHandler_SetDefault(t *testing.T) {
	t.Run("WithFuncSet", func(t *testing.T) {
		// Given a mock config handler with SetDefaultFunc set to verify parameters
		mockHandler := NewMockConfigHandler()
		called := false

		// And SetDefaultFunc updates the flag and checks the parameters
		mockHandler.SetDefaultFunc = func(context v1alpha1.Context) error {
			called = true
			if !reflect.DeepEqual(context, DefaultConfig_Localhost) {
				t.Errorf("Expected value %v, got %v", DefaultConfig_Localhost, context)
			}
			return nil
		}

		// When SetDefault is called
		mockHandler.SetDefault(DefaultConfig_Localhost)

		// Then the function should be called
		if !called {
			t.Error("Expected SetDefaultFunc to be called")
		}
	})

	t.Run("WithNoFuncSet", func(t *testing.T) {
		// Given a mock config handler without SetDefaultFunc set
		mockHandler := NewMockConfigHandler()

		// When SetDefault is called
		mockHandler.SetDefault(DefaultConfig_Localhost)

		// Then no error should occur
	})
}

func TestMockConfigHandler_GetConfig(t *testing.T) {
	t.Run("WithFuncSet", func(t *testing.T) {
		// Given a mock config handler with GetConfigFunc set to return a mock context
		mockHandler := NewMockConfigHandler()
		called := false

		// And GetConfigFunc updates the flag and returns a mock context
		mockContext := &v1alpha1.Context{}
		mockHandler.GetConfigFunc = func() *v1alpha1.Context {
			called = true
			return mockContext
		}

		// When GetConfig is called
		config := mockHandler.GetConfig()

		// Then the returned config should match the mock context
		if !reflect.DeepEqual(config, mockContext) {
			t.Errorf("Expected GetConfig to return %v, got %v", mockContext, config)
		}

		// And the function should be called
		if !called {
			t.Error("Expected GetConfigFunc to be called")
		}
	})

	t.Run("NoFuncSet", func(t *testing.T) {
		// Given a mock config handler without GetConfigFunc set
		mockHandler := NewMockConfigHandler()
		mockHandler.GetConfigFunc = nil

		// When GetConfig is called
		config := mockHandler.GetConfig()

		// Then an empty Context should be returned
		if !reflect.DeepEqual(config, &v1alpha1.Context{}) {
			t.Errorf("Expected GetConfig to return empty Context, got %v", config)
		}
	})
}

func TestMockConfigHandler_GetContext(t *testing.T) {
	t.Run("WithFuncSet", func(t *testing.T) {
		// Given a new mock config handler
		handler := NewMockConfigHandler()

		// And the GetContextFunc is set to return a specific mock context string
		handler.GetContextFunc = func() string {
			return "mock-context"
		}

		// When GetContext is called to retrieve the context
		context := handler.GetContext()

		// Then the returned context should match the expected mock context
		if context != "mock-context" {
			t.Errorf("Expected GetContext to return 'mock-context', got %v", context)
		}
	})

	t.Run("WithNoFuncSet", func(t *testing.T) {
		// Given a new mock config handler without setting GetContextFunc
		handler := NewMockConfigHandler()

		// When GetContext is called to retrieve the context
		context := handler.GetContext()

		// Then the returned context should match the default mock context
		if context != "mock-context" {
			t.Errorf("Expected GetContext to return 'mock-context', got %v", context)
		}
	})
}

func TestMockConfigHandler_SetContext(t *testing.T) {
	t.Run("WithFuncSet", func(t *testing.T) {
		// Given a new mock config handler
		handler := NewMockConfigHandler()

		// And the SetContextFunc is set to a function that returns no error
		handler.SetContextFunc = func(context string) error {
			return nil
		}

		// When SetContext is called with a mock context string
		err := handler.SetContext("mock-context")

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected error = %v, got = %v", nil, err)
		}
	})

	t.Run("WithNoFuncSet", func(t *testing.T) {
		// Given a new mock config handler without setting SetContextFunc
		handler := NewMockConfigHandler()

		// When SetContext is called with a mock context string
		err := handler.SetContext("mock-context")

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected error = %v, got = %v", nil, err)
		}
	})
}

func TestMockConfigHandler_GetConfigRoot(t *testing.T) {
	t.Run("WithFuncSet", func(t *testing.T) {
		// Given a new mock config handler with GetConfigRootFunc set
		handler := NewMockConfigHandler()
		handler.GetConfigRootFunc = func() (string, error) { return "mock-config-root", nil }

		// When GetConfigRoot is called
		root, err := handler.GetConfigRoot()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected error = %v, got = %v", nil, err)
		}

		// And the root should be 'mock-config-root'
		if root != "mock-config-root" {
			t.Errorf("Expected GetConfigRoot to return 'mock-config-root', got %v", root)
		}
	})

	t.Run("WithNoFuncSet", func(t *testing.T) {
		// Given a new mock config handler without GetConfigRootFunc set
		handler := NewMockConfigHandler()

		// When GetConfigRoot is called
		root, err := handler.GetConfigRoot()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected error = %v, got = %v", nil, err)
		}

		// And the root should be 'mock-config-root'
		if root != "mock-config-root" {
			t.Errorf("Expected GetConfigRoot to return 'mock-config-root', got %v", root)
		}
	})
}

func TestMockConfigHandler_Clean(t *testing.T) {
	t.Run("WithFuncSet", func(t *testing.T) {
		// Given a new mock config handler with CleanFunc set
		handler := NewMockConfigHandler()
		handler.CleanFunc = func() error { return nil }

		// When Clean is called
		err := handler.Clean()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected error = %v, got = %v", nil, err)
		}
	})

	t.Run("WithNoFuncSet", func(t *testing.T) {
		// Given a new mock config handler without CleanFunc set
		handler := NewMockConfigHandler()

		// When Clean is called
		err := handler.Clean()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected error = %v, got = %v", nil, err)
		}
	})
}

func TestMockConfigHandler_IsLoaded(t *testing.T) {
	t.Run("WithFuncSet", func(t *testing.T) {
		// Given a new mock config handler with IsLoadedFunc set
		handler := NewMockConfigHandler()
		handler.IsLoadedFunc = func() bool { return true }

		// When IsLoaded is called
		loaded := handler.IsLoaded()

		// Then the returned value should be true
		if !loaded {
			t.Errorf("Expected IsLoaded to return true, got %v", loaded)
		}
	})

	t.Run("WithNoFuncSet", func(t *testing.T) {
		// Given a new mock config handler without IsLoadedFunc set
		handler := NewMockConfigHandler()

		// When IsLoaded is called
		loaded := handler.IsLoaded()

		// Then the returned value should be false
		if loaded {
			t.Errorf("Expected IsLoaded to return false, got %v", loaded)
		}
	})
}

func TestMockConfigHandler_LoadConfigString(t *testing.T) {
	t.Run("WithFuncSet", func(t *testing.T) {
		// Given a mock config handler with LoadConfigStringFunc set
		handler := NewMockConfigHandler()
		mockErr := fmt.Errorf("mock load config string error")
		handler.LoadConfigStringFunc = func(content string) error { return mockErr }

		// When LoadConfigString is called
		err := handler.LoadConfigString("some content")

		// Then the error should match the expected mock error
		if err != mockErr {
			t.Errorf("Expected error = %v, got = %v", mockErr, err)
		}
	})

	t.Run("WithNoFuncSet", func(t *testing.T) {
		// Given a mock config handler without LoadConfigStringFunc set
		handler := NewMockConfigHandler()

		// When LoadConfigString is called
		err := handler.LoadConfigString("some content")

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected error = %v, got = %v", nil, err)
		}
	})
}

func TestMockConfigHandler_SetSecretsProvider(t *testing.T) {
	t.Run("WithFuncSet", func(t *testing.T) {
		// Given a mock config handler with SetSecretsProviderFunc set
		handler := NewMockConfigHandler()
		var calledProvider secrets.SecretsProvider
		handler.SetSecretsProviderFunc = func(provider secrets.SecretsProvider) {
			calledProvider = provider
		}

		// And a mock secrets provider
		mockProvider := secrets.NewMockSecretsProvider(nil)

		// When setting the secrets provider
		handler.SetSecretsProvider(mockProvider)

		// Then the function should be called with the provider
		if calledProvider != mockProvider {
			t.Errorf("Expected SetSecretsProviderFunc to be called with %v, got %v", mockProvider, calledProvider)
		}
	})

	t.Run("WithNoFuncSet", func(t *testing.T) {
		// Given a mock config handler without SetSecretsProviderFunc set
		handler := NewMockConfigHandler()

		// And a mock secrets provider
		mockProvider := secrets.NewMockSecretsProvider(nil)

		// When setting the secrets provider
		// Then it should not panic
		handler.SetSecretsProvider(mockProvider)
	})
}

func TestMockConfigHandler_GenerateContextID(t *testing.T) {
	t.Run("WithMockFunction", func(t *testing.T) {
		// Given a mock config handler with GenerateContextIDFunc set
		handler := NewMockConfigHandler()
		mockErr := fmt.Errorf("mock generate context ID error")
		handler.GenerateContextIDFunc = func() error { return mockErr }

		// When GenerateContextID is called
		err := handler.GenerateContextID()

		// Then the error should match the expected mock error
		if err != mockErr {
			t.Errorf("Expected error = %v, got = %v", mockErr, err)
		}
	})

	t.Run("WithNoFuncSet", func(t *testing.T) {
		// Given a mock config handler without GenerateContextIDFunc set
		handler := NewMockConfigHandler()

		// When GenerateContextID is called
		err := handler.GenerateContextID()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected error = %v, got = %v", nil, err)
		}
	})
}

func TestMockConfigHandler_LoadContextConfig(t *testing.T) {
	t.Run("WithFuncSet", func(t *testing.T) {
		// Given a MockConfigHandler with LoadContextConfigFunc set
		mockHandler := NewMockConfigHandler()
		expectedError := fmt.Errorf("mocked load context config error")
		mockHandler.LoadContextConfigFunc = func() error {
			return expectedError
		}

		// When LoadContextConfig is called
		err := mockHandler.LoadContextConfig()

		// Then it should return the mocked error
		if err != expectedError {
			t.Errorf("LoadContextConfig() error = %v, expected %v", err, expectedError)
		}
	})

	t.Run("WithNoFuncSet", func(t *testing.T) {
		// Given a MockConfigHandler with no LoadContextConfigFunc set
		mockHandler := NewMockConfigHandler()

		// When LoadContextConfig is called
		err := mockHandler.LoadContextConfig()

		// Then it should return nil
		if err != nil {
			t.Errorf("LoadContextConfig() error = %v, expected nil", err)
		}
	})
}

func TestMockConfigHandler_YamlMarshalWithDefinedPaths(t *testing.T) {
	t.Run("WithFuncSet", func(t *testing.T) {
		// Given a MockConfigHandler with YamlMarshalWithDefinedPathsFunc set
		mockHandler := NewMockConfigHandler()
		expectedResult := []byte("mocked: yaml")
		expectedError := fmt.Errorf("mocked marshal error")
		mockHandler.YamlMarshalWithDefinedPathsFunc = func(v any) ([]byte, error) {
			return expectedResult, expectedError
		}

		// When YamlMarshalWithDefinedPaths is called
		result, err := mockHandler.YamlMarshalWithDefinedPaths("test")

		// Then it should return the mocked result and error
		if string(result) != string(expectedResult) {
			t.Errorf("YamlMarshalWithDefinedPaths() result = %v, expected %v", result, expectedResult)
		}
		if err != expectedError {
			t.Errorf("YamlMarshalWithDefinedPaths() error = %v, expected %v", err, expectedError)
		}
	})

	t.Run("WithNoFuncSet", func(t *testing.T) {
		// Given a MockConfigHandler with no YamlMarshalWithDefinedPathsFunc set
		mockHandler := NewMockConfigHandler()

		// When YamlMarshalWithDefinedPaths is called
		result, err := mockHandler.YamlMarshalWithDefinedPaths("test")

		// Then it should return the default YAML content and no error
		expectedDefault := []byte("contexts:\n  mock-context:\n    dns:\n      domain: mock.domain.com")
		if string(result) != string(expectedDefault) {
			t.Errorf("YamlMarshalWithDefinedPaths() result = %v, expected %v", result, expectedDefault)
		}
		if err != nil {
			t.Errorf("YamlMarshalWithDefinedPaths() error = %v, expected nil", err)
		}
	})
}
