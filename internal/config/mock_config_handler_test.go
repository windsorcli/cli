package config

import (
	"fmt"
	"reflect"
	"testing"
)

func TestMockConfigHandler_LoadConfig(t *testing.T) {
	mockLoadErr := fmt.Errorf("mock load config error")

	t.Run("WithPath", func(t *testing.T) {
		handler := NewMockConfigHandler()
		handler.LoadConfigFunc = func(path string) error {
			return mockLoadErr
		}
		err := handler.LoadConfig("some/path")
		if err != mockLoadErr {
			t.Errorf("Expected error = %v, got = %v", mockLoadErr, err)
		}
	})

	t.Run("WithNoFuncSet", func(t *testing.T) {
		handler := NewMockConfigHandler()
		err := handler.LoadConfig("some/path")
		if err != nil {
			t.Errorf("Expected error = %v, got = %v", nil, err)
		}
	})
}

func TestMockConfigHandler_GetString(t *testing.T) {
	t.Run("WithKey", func(t *testing.T) {
		handler := NewMockConfigHandler()
		handler.GetStringFunc = func(key string, defaultValue ...string) string { return "" }
		value := handler.GetString("someKey")
		if value != "" {
			t.Errorf("Expected GetString with key to return empty string, got %v", value)
		}
	})

	t.Run("WithNoFuncSet", func(t *testing.T) {
		handler := NewMockConfigHandler()
		value := handler.GetString("someKey")
		if value != "mock-string" {
			t.Errorf("Expected GetString with no func set to return 'mock-string', got %v", value)
		}
	})

	t.Run("WithDefaultValue", func(t *testing.T) {
		handler := NewMockConfigHandler()
		defaultValue := "default"
		value := handler.GetString("someKey", defaultValue)
		if value != defaultValue {
			t.Errorf("Expected GetString with default to return %v, got %v", defaultValue, value)
		}
	})
}

func TestMockConfigHandler_GetInt(t *testing.T) {
	t.Run("WithKey", func(t *testing.T) {
		handler := NewMockConfigHandler()
		handler.GetIntFunc = func(key string, defaultValue ...int) int { return 0 }
		value := handler.GetInt("someKey")
		if value != 0 {
			t.Errorf("Expected GetInt with key to return 0, got %v", value)
		}
	})

	t.Run("WithNoFuncSet", func(t *testing.T) {
		handler := NewMockConfigHandler()
		value := handler.GetInt("someKey")
		if value != 42 {
			t.Errorf("Expected GetInt with no func set to return 42, got %v", value)
		}
	})

	t.Run("WithDefaultValue", func(t *testing.T) {
		handler := NewMockConfigHandler()
		defaultValue := 42
		value := handler.GetInt("someKey", defaultValue)
		if value != defaultValue {
			t.Errorf("Expected GetInt with default to return %v, got %v", defaultValue, value)
		}
	})
}

func TestMockConfigHandler_GetBool(t *testing.T) {
	t.Run("WithKey", func(t *testing.T) {
		handler := NewMockConfigHandler()
		handler.GetBoolFunc = func(key string, defaultValue ...bool) bool { return false }
		value := handler.GetBool("someKey")
		if value != false {
			t.Errorf("Expected GetBool with key to return false, got %v", value)
		}
	})

	t.Run("WithNoFuncSet", func(t *testing.T) {
		handler := NewMockConfigHandler()
		value := handler.GetBool("someKey")
		if value != true {
			t.Errorf("Expected GetBool with no func set to return true, got %v", value)
		}
	})

	t.Run("WithDefaultValue", func(t *testing.T) {
		handler := NewMockConfigHandler()
		defaultValue := true
		value := handler.GetBool("someKey", defaultValue)
		if value != defaultValue {
			t.Errorf("Expected GetBool with default to return %v, got %v", defaultValue, value)
		}
	})
}

func TestMockConfigHandler_Set(t *testing.T) {
	t.Run("WithKeyAndValue", func(t *testing.T) {
		handler := NewMockConfigHandler()
		handler.SetFunc = func(key string, value interface{}) error { return nil }
		handler.Set("someKey", "someValue")
	})

	t.Run("WithNoFuncSet", func(t *testing.T) {
		handler := NewMockConfigHandler()
		handler.Set("someKey", "someValue")
	})
}

func TestMockConfigHandler_SaveConfig(t *testing.T) {
	mockSaveErr := fmt.Errorf("mock save config error")

	t.Run("WithPath", func(t *testing.T) {
		handler := NewMockConfigHandler()
		handler.SaveConfigFunc = func(path string) error { return mockSaveErr }
		err := handler.SaveConfig("some/path")
		if err != mockSaveErr {
			t.Errorf("Expected error = %v, got = %v", mockSaveErr, err)
		}
	})

	t.Run("WithNoFuncSet", func(t *testing.T) {
		handler := NewMockConfigHandler()
		err := handler.SaveConfig("some/path")
		if err != nil {
			t.Errorf("Expected error = %v, got = %v", nil, err)
		}
	})
}

func TestMockConfigHandler_Get(t *testing.T) {
	t.Run("WithKey", func(t *testing.T) {
		handler := NewMockConfigHandler()
		handler.GetFunc = func(key string) interface{} { return "mock-value" }
		value := handler.Get("someKey")
		if value != "mock-value" {
			t.Errorf("Expected Get to return 'mock-value', got %v", value)
		}
	})

	t.Run("WithNoFuncSet", func(t *testing.T) {
		handler := NewMockConfigHandler()
		value := handler.Get("someKey")
		if value != "mock-value" {
			t.Errorf("Expected Get to return 'mock-value', got %v", value)
		}
	})
}

func TestMockConfigHandler_SetDefault(t *testing.T) {
	t.Run("WithFuncSet", func(t *testing.T) {
		// Arrange: Create a mock config handler and a flag to check if the function was called
		mockHandler := NewMockConfigHandler()
		called := false

		// Set the SetDefaultFunc to update the flag and check the parameters
		mockHandler.SetDefaultFunc = func(context Context) error {
			called = true
			if !reflect.DeepEqual(context, DefaultLocalConfig) {
				t.Errorf("Expected value %v, got %v", DefaultLocalConfig, context)
			}
			return nil
		}

		// Act: Call SetDefault
		mockHandler.SetDefault(DefaultLocalConfig)

		// Assert: Verify that the function was called
		if !called {
			t.Error("Expected SetDefaultFunc to be called")
		}
	})

	t.Run("WithNoFuncSet", func(t *testing.T) {
		mockHandler := NewMockConfigHandler()

		// Ensure SetDefaultFunc is not set
		mockHandler.SetDefaultFunc = nil

		// Call SetDefault and expect no error
		mockHandler.SetDefault(DefaultLocalConfig)
	})
}

func TestMockConfigHandler_GetConfig(t *testing.T) {
	t.Run("WithFuncSet", func(t *testing.T) {
		// Arrange: Create a mock config handler and a flag to check if the function was called
		mockHandler := NewMockConfigHandler()
		called := false

		// Set the GetConfigFunc to update the flag and return a mock context
		mockContext := &Context{}
		mockHandler.GetConfigFunc = func() *Context {
			called = true
			return mockContext
		}

		// Act: Call GetConfig
		config := mockHandler.GetConfig()
		if !reflect.DeepEqual(config, mockContext) {
			t.Errorf("Expected GetConfig to return %v, got %v", mockContext, config)
		}

		// Assert: Verify that the function was called
		if !called {
			t.Error("Expected GetConfigFunc to be called")
		}
	})

	t.Run("GetConfig_NoFuncSet", func(t *testing.T) {
		mockHandler := NewMockConfigHandler()

		// Ensure GetConfigFunc is not set
		mockHandler.GetConfigFunc = nil

		// Call GetConfig and expect a reasonable default context
		config := mockHandler.GetConfig()
		if !reflect.DeepEqual(config, &Context{}) {
			t.Errorf("Expected GetConfig to return empty Context, got %v", config)
		}
	})
}
