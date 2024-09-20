package config

import (
	"errors"
	"reflect"
	"testing"
)

// Helper function to create a default MockConfigHandler
func createDefaultMockHandler() *MockConfigHandler {
	return &MockConfigHandler{}
}

func TestMockConfigHandler_LoadConfig(t *testing.T) {
	mockLoadErr := errors.New("mock load config error")
	handler := &MockConfigHandler{
		LoadConfigFunc: func(path string) error {
			return mockLoadErr
		},
	}

	// Given a path to load config
	t.Run("LoadConfigWithPath", func(t *testing.T) {
		// When LoadConfig is called with a path
		err := handler.LoadConfig("some/path")
		// Then it should return the mock error
		if err != mockLoadErr {
			t.Errorf("LoadConfig() error = %v, wantErr %v", err, mockLoadErr)
		}
	})

	// Given no LoadConfigFunc is set
	t.Run("LoadConfigWithNoFuncSet", func(t *testing.T) {
		handler := &MockConfigHandler{}
		// When LoadConfig is called with a path and no function is set
		err := handler.LoadConfig("some/path")
		// Then it should return nil
		if err != nil {
			t.Errorf("LoadConfig() error = %v, wantErr nil", err)
		}
	})
}

func TestMockConfigHandler_GetConfigValue(t *testing.T) {
	mockGetErr := errors.New("mock get config value error")
	handler := &MockConfigHandler{
		GetConfigValueFunc: func(key string) (string, error) {
			return "", mockGetErr
		},
	}

	// Given a key to get config value
	t.Run("GetConfigValueWithKey", func(t *testing.T) {
		// When GetConfigValue is called with a key
		_, err := handler.GetConfigValue("someKey")
		// Then it should return the mock error
		if err != mockGetErr {
			t.Errorf("GetConfigValue() error = %v, wantErr %v", err, mockGetErr)
		}
	})

	// Given no GetConfigValueFunc is set
	t.Run("GetConfigValueWithNoFuncSet", func(t *testing.T) {
		handler := &MockConfigHandler{}
		// When GetConfigValue is called with a key and no function is set
		value, err := handler.GetConfigValue("someKey")
		// Then it should return an empty string and nil error
		if err != nil {
			t.Errorf("GetConfigValue() error = %v, wantErr nil", err)
		}
		if value != "" {
			t.Errorf("GetConfigValue() value = %v, wantValue \"\"", value)
		}
	})
}

func TestMockConfigHandler_SetConfigValue(t *testing.T) {
	mockSetErr := errors.New("mock set config value error")
	handler := &MockConfigHandler{
		SetConfigValueFunc: func(key, value string) error {
			return mockSetErr
		},
	}

	// Given a key and value to set config value
	t.Run("SetConfigValueWithKeyAndValue", func(t *testing.T) {
		// When SetConfigValue is called with a key and value
		err := handler.SetConfigValue("someKey", "someValue")
		// Then it should return the mock error
		if err != mockSetErr {
			t.Errorf("SetConfigValue() error = %v, wantErr %v", err, mockSetErr)
		}
	})

	// Given no SetConfigValueFunc is set
	t.Run("SetConfigValueWithNoFuncSet", func(t *testing.T) {
		handler := &MockConfigHandler{}
		// When SetConfigValue is called with a key and value and no function is set
		err := handler.SetConfigValue("someKey", "someValue")
		// Then it should return nil
		if err != nil {
			t.Errorf("SetConfigValue() error = %v, wantErr nil", err)
		}
	})
}

func TestMockConfigHandler_SaveConfig(t *testing.T) {
	mockSaveErr := errors.New("mock save config error")
	handler := &MockConfigHandler{
		SaveConfigFunc: func(path string) error {
			return mockSaveErr
		},
	}

	// Given a path to save config
	t.Run("SaveConfigWithPath", func(t *testing.T) {
		// When SaveConfig is called with a path
		err := handler.SaveConfig("some/path")
		// Then it should return the mock error
		if err != mockSaveErr {
			t.Errorf("SaveConfig() error = %v, wantErr %v", err, mockSaveErr)
		}
	})

	// Given no SaveConfigFunc is set
	t.Run("SaveConfigWithNoFuncSet", func(t *testing.T) {
		handler := &MockConfigHandler{}
		// When SaveConfig is called with a path and no function is set
		err := handler.SaveConfig("some/path")
		// Then it should return nil
		if err != nil {
			t.Errorf("SaveConfig() error = %v, wantErr nil", err)
		}
	})
}

func TestMockConfigHandler_GetNestedMap(t *testing.T) {
	mockGetNestedMapErr := errors.New("mock get nested map error")
	handler := &MockConfigHandler{
		GetNestedMapFunc: func(key string) (map[string]interface{}, error) {
			return nil, mockGetNestedMapErr
		},
	}

	// Given a key to get nested map
	t.Run("GetNestedMapWithKey", func(t *testing.T) {
		// When GetNestedMap is called with a key
		_, err := handler.GetNestedMap("someKey")
		// Then it should return the mock error
		if err != mockGetNestedMapErr {
			t.Errorf("GetNestedMap() error = %v, wantErr %v", err, mockGetNestedMapErr)
		}
	})

	// Given no GetNestedMapFunc is set
	t.Run("GetNestedMapWithNoFuncSet", func(t *testing.T) {
		handler := &MockConfigHandler{}
		// When GetNestedMap is called with a key and no function is set
		value, err := handler.GetNestedMap("someKey")
		// Then it should return a nil map and nil error
		if err != nil {
			t.Errorf("GetNestedMap() error = %v, wantErr nil", err)
		}
		if !reflect.DeepEqual(value, map[string]interface{}(nil)) {
			t.Errorf("GetNestedMap() value = %v, wantValue map[string]interface{}(nil)", value)
		}
	})
}

func TestMockConfigHandler_ListKeys(t *testing.T) {
	mockListKeysErr := errors.New("mock list keys error")
	handler := &MockConfigHandler{
		ListKeysFunc: func(key string) ([]string, error) {
			return nil, mockListKeysErr
		},
	}

	// Given a key to list keys
	t.Run("ListKeysWithKey", func(t *testing.T) {
		// When ListKeys is called with a key
		_, err := handler.ListKeys("someKey")
		// Then it should return the mock error
		if err != mockListKeysErr {
			t.Errorf("ListKeys() error = %v, wantErr %v", err, mockListKeysErr)
		}
	})

	// Given no ListKeysFunc is set
	t.Run("ListKeysWithNoFuncSet", func(t *testing.T) {
		handler := &MockConfigHandler{}
		// When ListKeys is called with a key and no function is set
		value, err := handler.ListKeys("someKey")
		// Then it should return a nil slice and nil error
		if err != nil {
			t.Errorf("ListKeys() error = %v, wantErr nil", err)
		}
		if !reflect.DeepEqual(value, []string(nil)) {
			t.Errorf("ListKeys() value = %v, wantValue []string(nil)", value)
		}
	})
}

func TestNewMockConfigHandler(t *testing.T) {
	mockError := errors.New("mock error")

	loadConfigFunc := func(path string) error { return mockError }
	getConfigValueFunc := func(key string) (string, error) { return "value", mockError }
	setConfigValueFunc := func(key, value string) error { return mockError }
	saveConfigFunc := func(path string) error { return mockError }
	getNestedMapFunc := func(key string) (map[string]interface{}, error) {
		return map[string]interface{}{"key": "value"}, mockError
	}
	listKeysFunc := func(key string) ([]string, error) { return []string{"key1", "key2"}, mockError }

	mockHandler := NewMockConfigHandler(
		loadConfigFunc,
		getConfigValueFunc,
		setConfigValueFunc,
		saveConfigFunc,
		getNestedMapFunc,
		listKeysFunc,
	)

	// Given a mock handler
	t.Run("LoadConfigFunc", func(t *testing.T) {
		// When LoadConfigFunc is called
		if err := mockHandler.LoadConfigFunc("path"); err != mockError {
			// Then it should return the mock error
			t.Errorf("expected LoadConfigFunc to return mock error")
		}
	})

	t.Run("GetConfigValueFunc", func(t *testing.T) {
		// When GetConfigValueFunc is called
		if value, err := mockHandler.GetConfigValueFunc("key"); value != "value" || err != mockError {
			// Then it should return the mock value and error
			t.Errorf("expected GetConfigValueFunc to return 'value' and mock error")
		}
	})

	t.Run("SetConfigValueFunc", func(t *testing.T) {
		// When SetConfigValueFunc is called
		if err := mockHandler.SetConfigValueFunc("key", "value"); err != mockError {
			// Then it should return the mock error
			t.Errorf("expected SetConfigValueFunc to return mock error")
		}
	})

	t.Run("SaveConfigFunc", func(t *testing.T) {
		// When SaveConfigFunc is called
		if err := mockHandler.SaveConfigFunc("path"); err != mockError {
			// Then it should return the mock error
			t.Errorf("expected SaveConfigFunc to return mock error")
		}
	})

	t.Run("GetNestedMapFunc", func(t *testing.T) {
		// When GetNestedMapFunc is called
		if nestedMap, err := mockHandler.GetNestedMapFunc("key"); !reflect.DeepEqual(nestedMap, map[string]interface{}{"key": "value"}) || err != mockError {
			// Then it should return the mock map and error
			t.Errorf("expected GetNestedMapFunc to return map and mock error")
		}
	})

	t.Run("ListKeysFunc", func(t *testing.T) {
		// When ListKeysFunc is called
		if keys, err := mockHandler.ListKeysFunc("key"); !reflect.DeepEqual(keys, []string{"key1", "key2"}) || err != mockError {
			// Then it should return the mock keys and error
			t.Errorf("expected ListKeysFunc to return keys and mock error")
		}
	})
}
