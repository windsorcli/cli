package config

import (
	"errors"
	"reflect"
	"testing"
)

func TestMockConfigHandler(t *testing.T) {
	mockLoadErr := errors.New("mock load config error")
	mockGetErr := errors.New("mock get config value error")
	mockSetErr := errors.New("mock set config value error")
	mockSaveErr := errors.New("mock save config error")
	mockGetNestedMapErr := errors.New("mock get nested map error")
	mockListKeysErr := errors.New("mock list keys error")

	handler := &MockConfigHandler{
		LoadConfigFunc: func(path string) error {
			return mockLoadErr
		},
		GetConfigValueFunc: func(key string) (string, error) {
			return "", mockGetErr
		},
		SetConfigValueFunc: func(key, value string) error {
			return mockSetErr
		},
		SaveConfigFunc: func(path string) error {
			return mockSaveErr
		},
		GetNestedMapFunc: func(key string) (map[string]interface{}, error) {
			return nil, mockGetNestedMapErr
		},
		ListKeysFunc: func(key string) ([]string, error) {
			return nil, mockListKeysErr
		},
	}

	tests := []struct {
		name     string
		testFunc func() error
		wantErr  error
	}{
		{
			name: "LoadConfig",
			testFunc: func() error {
				return handler.LoadConfig("some/path")
			},
			wantErr: mockLoadErr,
		},
		{
			name: "GetConfigValue",
			testFunc: func() error {
				_, err := handler.GetConfigValue("someKey")
				return err
			},
			wantErr: mockGetErr,
		},
		{
			name: "SetConfigValue",
			testFunc: func() error {
				return handler.SetConfigValue("someKey", "someValue")
			},
			wantErr: mockSetErr,
		},
		{
			name: "SaveConfig",
			testFunc: func() error {
				return handler.SaveConfig("some/path")
			},
			wantErr: mockSaveErr,
		},
		{
			name: "GetNestedMap",
			testFunc: func() error {
				_, err := handler.GetNestedMap("someKey")
				return err
			},
			wantErr: mockGetNestedMapErr,
		},
		{
			name: "ListKeys",
			testFunc: func() error {
				_, err := handler.ListKeys("someKey")
				return err
			},
			wantErr: mockListKeysErr,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.testFunc(); err != tt.wantErr {
				t.Errorf("%s() error = %v, wantErr %v", tt.name, err, tt.wantErr)
			}
		})
	}

	defaultHandler := &MockConfigHandler{}

	defaultTests := []struct {
		name      string
		testFunc  func() (interface{}, error)
		wantValue interface{}
		wantErr   error
	}{
		{
			name: "LoadConfig_Default",
			testFunc: func() (interface{}, error) {
				return nil, defaultHandler.LoadConfig("some/path")
			},
			wantValue: nil,
			wantErr:   nil,
		},
		{
			name: "GetConfigValue_Default",
			testFunc: func() (interface{}, error) {
				return defaultHandler.GetConfigValue("someKey")
			},
			wantValue: "",
			wantErr:   nil,
		},
		{
			name: "SetConfigValue_Default",
			testFunc: func() (interface{}, error) {
				return nil, defaultHandler.SetConfigValue("someKey", "someValue")
			},
			wantValue: nil,
			wantErr:   nil,
		},
		{
			name: "SaveConfig_Default",
			testFunc: func() (interface{}, error) {
				return nil, defaultHandler.SaveConfig("some/path")
			},
			wantValue: nil,
			wantErr:   nil,
		},
		{
			name: "GetNestedMap_Default",
			testFunc: func() (interface{}, error) {
				return defaultHandler.GetNestedMap("someKey")
			},
			wantValue: map[string]interface{}(nil),
			wantErr:   nil,
		},
		{
			name: "ListKeys_Default",
			testFunc: func() (interface{}, error) {
				return defaultHandler.ListKeys("someKey")
			},
			wantValue: []string(nil),
			wantErr:   nil,
		},
	}

	for _, tt := range defaultTests {
		t.Run(tt.name, func(t *testing.T) {
			value, err := tt.testFunc()
			if err != tt.wantErr {
				t.Errorf("%s() error = %v, wantErr %v", tt.name, err, tt.wantErr)
			}
			if !reflect.DeepEqual(value, tt.wantValue) {
				t.Errorf("%s() value = %v, wantValue %v", tt.name, value, tt.wantValue)
			}
		})
	}
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

	if err := mockHandler.LoadConfigFunc("path"); err != mockError {
		t.Errorf("expected LoadConfigFunc to return mock error")
	}
	if value, err := mockHandler.GetConfigValueFunc("key"); value != "value" || err != mockError {
		t.Errorf("expected GetConfigValueFunc to return 'value' and mock error")
	}
	if err := mockHandler.SetConfigValueFunc("key", "value"); err != mockError {
		t.Errorf("expected SetConfigValueFunc to return mock error")
	}
	if err := mockHandler.SaveConfigFunc("path"); err != mockError {
		t.Errorf("expected SaveConfigFunc to return mock error")
	}
	if nestedMap, err := mockHandler.GetNestedMapFunc("key"); !reflect.DeepEqual(nestedMap, map[string]interface{}{"key": "value"}) || err != mockError {
		t.Errorf("expected GetNestedMapFunc to return map and mock error")
	}
	if keys, err := mockHandler.ListKeysFunc("key"); !reflect.DeepEqual(keys, []string{"key1", "key2"}) || err != mockError {
		t.Errorf("expected ListKeysFunc to return keys and mock error")
	}
}
