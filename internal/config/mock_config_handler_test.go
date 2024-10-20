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

// Helper function to create a new MockConfigHandler with default functions
func newMockConfigHandlerWithDefaults() *MockConfigHandler {
	return &MockConfigHandler{
		LoadConfigFunc:     func(path string) error { return nil },
		GetConfigValueFunc: func(key string) (string, error) { return "", nil },
		SetConfigValueFunc: func(key string, value interface{}) error { return nil },
		SaveConfigFunc:     func(path string) error { return nil },
		GetNestedMapFunc:   func(key string) (map[string]interface{}, error) { return nil, nil },
	}
}

func TestMockConfigHandler_LoadConfig(t *testing.T) {
	mockLoadErr := errors.New("mock load config error")

	t.Run("LoadConfigWithPath", func(t *testing.T) {
		handler := newMockConfigHandlerWithDefaults()
		handler.LoadConfigFunc = func(path string) error { return mockLoadErr }
		err := handler.LoadConfig("some/path")
		assertError(t, err, mockLoadErr)
	})

	t.Run("LoadConfigWithNoFuncSet", func(t *testing.T) {
		handler := NewMockConfigHandler()
		err := handler.LoadConfig("some/path")
		assertError(t, err, nil)
	})
}

func TestMockConfigHandler_GetConfigValue(t *testing.T) {
	mockGetErr := errors.New("mock get config value error")

	t.Run("GetConfigValueWithKey", func(t *testing.T) {
		handler := newMockConfigHandlerWithDefaults()
		handler.GetConfigValueFunc = func(key string) (string, error) { return "", mockGetErr }
		_, err := handler.GetConfigValue("someKey")
		assertError(t, err, mockGetErr)
	})

	t.Run("GetConfigValueWithNoFuncSet", func(t *testing.T) {
		handler := NewMockConfigHandler()
		value, err := handler.GetConfigValue("someKey")
		assertError(t, err, nil)
		assertEqual(t, "", value, "GetConfigValue")
	})
}

func TestMockConfigHandler_SetConfigValue(t *testing.T) {
	mockSetErr := errors.New("mock set config value error")

	t.Run("SetConfigValueWithKeyAndValue", func(t *testing.T) {
		handler := newMockConfigHandlerWithDefaults()
		handler.SetConfigValueFunc = func(key string, value interface{}) error { return mockSetErr }
		err := handler.SetConfigValue("someKey", "someValue")
		assertError(t, err, mockSetErr)
	})

	t.Run("SetConfigValueWithNoFuncSet", func(t *testing.T) {
		handler := NewMockConfigHandler()
		err := handler.SetConfigValue("someKey", "someValue")
		assertError(t, err, nil)
	})
}

func TestMockConfigHandler_SaveConfig(t *testing.T) {
	mockSaveErr := errors.New("mock save config error")

	t.Run("SaveConfigWithPath", func(t *testing.T) {
		handler := newMockConfigHandlerWithDefaults()
		handler.SaveConfigFunc = func(path string) error { return mockSaveErr }
		err := handler.SaveConfig("some/path")
		assertError(t, err, mockSaveErr)
	})

	t.Run("SaveConfigWithNoFuncSet", func(t *testing.T) {
		handler := NewMockConfigHandler()
		err := handler.SaveConfig("some/path")
		assertError(t, err, nil)
	})
}

func TestMockConfigHandler_GetNestedMap(t *testing.T) {
	mockGetNestedMapErr := errors.New("mock get nested map error")

	t.Run("GetNestedMapWithKey", func(t *testing.T) {
		handler := newMockConfigHandlerWithDefaults()
		handler.GetNestedMapFunc = func(key string) (map[string]interface{}, error) { return nil, mockGetNestedMapErr }
		_, err := handler.GetNestedMap("someKey")
		assertError(t, err, mockGetNestedMapErr)
	})

	t.Run("GetNestedMapWithNoFuncSet", func(t *testing.T) {
		handler := NewMockConfigHandler()
		value, err := handler.GetNestedMap("someKey")
		assertError(t, err, nil)
		assertEqual(t, map[string]interface{}(nil), value, "GetNestedMap")
	})
}
