package config

import (
	"errors"
	"reflect"
	"testing"
)

// Helper function for error assertion
func assertError(t *testing.T, err error, expectedErr error) {
	if err != expectedErr {
		t.Errorf("Expected error = %v, got = %v", expectedErr, err)
	}
}

// Helper function for value assertion
func assertEqual(t *testing.T, expected, actual interface{}, name string) {
	if !reflect.DeepEqual(expected, actual) {
		t.Errorf("Expected %s = %v, got = %v", name, expected, actual)
	}
}

func TestMockConfigHandler_LoadConfig(t *testing.T) {
	mockLoadErr := errors.New("mock load config error")

	t.Run("WithPath", func(t *testing.T) {
		handler := NewMockConfigHandler()
		handler.LoadConfigFunc = func(path string) error {
			return mockLoadErr
		}
		err := handler.LoadConfig("some/path")
		assertError(t, err, mockLoadErr)
	})

	t.Run("WithNoFuncSet", func(t *testing.T) {
		handler := NewMockConfigHandler()
		err := handler.LoadConfig("some/path")
		assertError(t, err, nil)
	})
}

func TestMockConfigHandler_GetString(t *testing.T) {
	t.Run("WithKey", func(t *testing.T) {
		handler := NewMockConfigHandler()
		handler.GetStringFunc = func(key string, defaultValue ...string) string { return "" }
		value := handler.GetString("someKey")
		assertEqual(t, "", value, "GetString with key")
	})

	t.Run("WithNoFuncSet", func(t *testing.T) {
		handler := NewMockConfigHandler()
		value := handler.GetString("someKey")
		assertEqual(t, "mock-string", value, "GetString with no func set")
	})

	t.Run("WithDefaultValue", func(t *testing.T) {
		handler := NewMockConfigHandler()
		defaultValue := "default"
		value := handler.GetString("someKey", defaultValue)
		assertEqual(t, defaultValue, value, "GetString with default")
	})
}

func TestMockConfigHandler_GetInt(t *testing.T) {
	t.Run("WithKey", func(t *testing.T) {
		handler := NewMockConfigHandler()
		handler.GetIntFunc = func(key string, defaultValue ...int) int { return 0 }
		value := handler.GetInt("someKey")
		assertEqual(t, 0, value, "GetInt with key")
	})

	t.Run("WithNoFuncSet", func(t *testing.T) {
		handler := NewMockConfigHandler()
		value := handler.GetInt("someKey")
		assertEqual(t, 42, value, "GetInt with no func set")
	})

	t.Run("WithDefaultValue", func(t *testing.T) {
		handler := NewMockConfigHandler()
		defaultValue := 42
		value := handler.GetInt("someKey", defaultValue)
		assertEqual(t, defaultValue, value, "GetInt with default")
	})
}

func TestMockConfigHandler_GetBool(t *testing.T) {
	t.Run("WithKey", func(t *testing.T) {
		handler := NewMockConfigHandler()
		handler.GetBoolFunc = func(key string, defaultValue ...bool) bool { return false }
		value := handler.GetBool("someKey")
		assertEqual(t, false, value, "GetBool with key")
	})

	t.Run("WithNoFuncSet", func(t *testing.T) {
		handler := NewMockConfigHandler()
		value := handler.GetBool("someKey")
		assertEqual(t, true, value, "GetBool with no func set")
	})

	t.Run("WithDefaultValue", func(t *testing.T) {
		handler := NewMockConfigHandler()
		defaultValue := true
		value := handler.GetBool("someKey", defaultValue)
		assertEqual(t, defaultValue, value, "GetBool with default")
	})
}

func TestMockConfigHandler_Set(t *testing.T) {
	mockSetErr := errors.New("mock set value error")

	t.Run("WithKeyAndValue", func(t *testing.T) {
		handler := NewMockConfigHandler()
		handler.SetFunc = func(key string, value interface{}) error { return mockSetErr }
		err := handler.Set("someKey", "someValue")
		assertError(t, err, mockSetErr)
	})

	t.Run("WithNoFuncSet", func(t *testing.T) {
		handler := NewMockConfigHandler()
		err := handler.Set("someKey", "someValue")
		assertError(t, err, nil)
	})
}

func TestMockConfigHandler_SaveConfig(t *testing.T) {
	mockSaveErr := errors.New("mock save config error")

	t.Run("WithPath", func(t *testing.T) {
		handler := NewMockConfigHandler()
		handler.SaveConfigFunc = func(path string) error { return mockSaveErr }
		err := handler.SaveConfig("some/path")
		assertError(t, err, mockSaveErr)
	})

	t.Run("WithNoFuncSet", func(t *testing.T) {
		handler := NewMockConfigHandler()
		err := handler.SaveConfig("some/path")
		assertError(t, err, nil)
	})
}

func TestMockConfigHandler_Get(t *testing.T) {
	mockGetErr := errors.New("mock get error")

	t.Run("WithKey", func(t *testing.T) {
		handler := NewMockConfigHandler()
		handler.GetFunc = func(key string) (interface{}, error) { return nil, mockGetErr }
		_, err := handler.Get("someKey")
		assertError(t, err, mockGetErr)
	})

	t.Run("WithNoFuncSet", func(t *testing.T) {
		handler := NewMockConfigHandler()
		value, err := handler.Get("someKey")
		assertError(t, err, nil)
		assertEqual(t, "mock-value", value, "Get")
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
			expectedValue := DefaultLocalConfig
			if !reflect.DeepEqual(context, expectedValue) {
				t.Errorf("Expected value %v, got %v", expectedValue, context)
			}
			return nil
		}

		// Act: Call SetDefault
		err := mockHandler.SetDefault(DefaultLocalConfig)
		assertError(t, err, nil)

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
		err := mockHandler.SetDefault(DefaultLocalConfig)
		if err != nil {
			t.Errorf("Expected nil error when no SetDefaultFunc is set, got %v", err)
		}
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
		assertEqual(t, mockContext, config, "GetConfig")

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
		assertEqual(t, &Context{}, config, "GetConfig")
	})
}
